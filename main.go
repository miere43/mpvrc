package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/miere43/mpvrc/internal/pipe"
)

func redirectToExistingApplicationInstance() bool {
	client, err := pipe.Dial("\\\\.\\pipe\\mpvrc-unique", 0, nil)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		log.Print("existing mpvrc instance not found, continuing with normal execution")
		return false
	} else if err != nil {
		log.Fatalf("failed to connect to existing mpvrc instance: %v", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			log.Printf("failed to close pipe to existing mpvrc instance: %v", err)
		}
	}()

	log.Printf("found existing mpvrc instance, redirecting command line args to it: %v", os.Args)

	argsJSON, err := json.Marshal(os.Args)
	if err != nil {
		log.Fatalf("failed to marshal args: %v", err)
	}

	if err := client.Write(argsJSON); err != nil {
		log.Fatalf("failed to send data to existing mpvrc instance: %v", err)
	}
	return true
}

func registerUniqueApplicationInstance(app *App) {
	server, err := pipe.NewServer("\\\\.\\pipe\\mpvrc-unique", func(client *pipe.ConnectedClient) {
		argsJSON, err := client.ReadMessage()
		if err != nil {
			log.Fatalf("failed to read message from connected client: %v", err)
		}

		var args []string
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			log.Fatalf("failed to unmarshal json args: %v", err)
		}

		app.handleCommandLineFromFromOtherInstance(args)
	})
	if err != nil {
		log.Fatalf("failed to register unique application instance: %v", err)
	}

	log.Printf("created named pipe server for communication with other mpvrc instances")

	go func() {
		server.Serve()
	}()
}

func main() {
	if redirectToExistingApplicationInstance() {
		return
	}

	app := NewApp()

	registerUniqueApplicationInstance(app)

	args := []string{"--force-window", "--idle", "--input-ipc-server=mpvsocket"}
	if len(os.Args) > 1 {
		args = append(args, os.Args[1])
	}

	cmd := exec.Command("C:/soft/mpv/mpv.exe", args...)
	if err := cmd.Start(); err != nil {
		log.Fatalf("failed to start cmd: %v", err)
	}

	if err := app.ConnectToMPV(500 * time.Millisecond); err != nil {
		log.Printf("failed to connect to mpv after startup: %v", err)
	} else {
		log.Printf("connected to mpv after startup")
	}

	h := http.NewServeMux()
	srv := &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: h,
	}

	h.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if err := app.ConnectToMPV(0); err != nil {
			log.Printf("failed to connect to MPV: %v", err)
		}

		exePath, err := os.Executable()
		if err != nil {
			handleError(w, err)
			return
		}

		source, err := os.ReadFile(filepath.Join(filepath.Dir(exePath), "index.html"))
		if err != nil {
			handleError(w, err)
			return
		}

		t, err := template.New("index.html").Parse(string(source))
		if err != nil {
			handleError(w, err)
			return
		}

		err = t.Execute(w, struct {
			StartupEvents []any
		}{
			StartupEvents: app.StartupEvents(),
		})
		if err != nil {
			handleError(w, err)
			return
		}
	})

	shutdownSSE := make(chan struct{})

	h.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers to allow all origins. You may want to restrict this to specific origins in a production environment.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Type")

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		listener := app.NewEventListener()

		loop := true
		for loop {
			select {
			case event := <-listener.Events:
				fmt.Fprintf(w, "data: %s\n\n", event)
				w.(http.Flusher).Flush()

			case <-r.Context().Done():
				log.Printf("Context done!")
				loop = false
				app.CloseEventListener(listener)

			case <-shutdownSSE:
				log.Printf("Shutdown SSE!")
				loop = false
				app.CloseEventListener(listener)
			}
		}
	})

	h.HandleFunc("POST /command", func(w http.ResponseWriter, r *http.Request) {
		commandJSON := r.FormValue("command")
		var command []any
		if err := json.Unmarshal([]byte(commandJSON), &command); err != nil {
			handleError(w, err)
			return
		}

		response, err := app.SendCommand(command, false)
		if err != nil {
			handleError(w, err)
			return
		}

		writeJSON(w, response.Data)
	})

	h.HandleFunc("GET /file-system", func(w http.ResponseWriter, r *http.Request) {
		type Entry struct {
			Name  string `json:"name"`
			Path  string `json:"path"`
			IsDir bool   `json:"isDir"`
		}

		entries := make([]Entry, 0)

		path := filepath.Clean(r.URL.Query().Get("path"))
		takeDir, err := strconv.ParseBool(r.URL.Query().Get("dir"))
		if err != nil {
			takeDir = false
		}

		if takeDir {
			path = filepath.Dir(path)
		}

		if path == "." {
			// Get names of all drives (Windows)
			for _, drive := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
				drivePath := fmt.Sprintf("%c:/", drive)
				if _, err := os.Stat(drivePath); err == nil {
					entries = append(entries, Entry{
						Name:  drivePath,
						Path:  drivePath,
						IsDir: true,
					})
				}
			}
		} else if !filepath.IsAbs(path) {
			handleError(w, fmt.Errorf("path %q must be absolute", path))
			return
		} else {
			dirEntries, err := os.ReadDir(path)
			if err != nil {
				handleError(w, err)
				return
			}

			prevPath := filepath.Dir(path)
			if prevPath == path {
				prevPath = ""
			}
			entries = append(entries, Entry{
				Name:  "..",
				Path:  prevPath,
				IsDir: true,
			})

			for _, entry := range dirEntries {
				entries = append(entries, Entry{
					Name:  entry.Name(),
					Path:  filepath.Join(path, entry.Name()),
					IsDir: entry.IsDir(),
				})
			}

			entries = slices.DeleteFunc(entries, func(entry Entry) bool {
				return entry.Name != ".." && isHiddenFile(entry.Path)
			})
		}

		slices.SortFunc(entries, func(a, b Entry) int {
			if a.IsDir && !b.IsDir {
				return -1
			}
			if !a.IsDir && b.IsDir {
				return 1
			}
			return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		})

		for i, entry := range entries {
			if entry.IsDir && entry.Name != ".." {
				entries[i].Name = "[" + entry.Name + "]"
			}
		}

		writeJSON(w, struct {
			Path    string  `json:"path"`
			Entries []Entry `json:"entries"`
		}{
			Path:    path,
			Entries: entries,
		})
	})

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(fmt.Sprintf("Failed to start HTTP server: %v", err))
		}
	}()

	quit := make(chan struct{})
	go func() {
		// Wait for Ctrl+C (SIGINT)
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		fmt.Println("Shutting down HTTP server...")
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		close(shutdownSSE)
		if err := srv.Shutdown(ctx); err != nil {
			fmt.Printf("HTTP server Shutdown: %v\n", err)
		}
		// mpv.Disconnect()
		close(quit)
	}()

	<-quit

	fmt.Println("Waiting for MPV to exit...")

	if err := cmd.Wait(); err != nil {
		log.Printf("failed to wait for mpv to close: %v", err)
	}

	fmt.Println("Exiting application...")
}

func handleError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(err.Error()))
}

func writeJSON(w http.ResponseWriter, data any) {
	output, err := json.Marshal(data)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Write(output)
}

func isHiddenFile(path string) bool {
	pathW, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		log.Printf("failed to convert path to utf16: %v", err)
		return false
	}

	attrs, err := syscall.GetFileAttributes(pathW)
	if err != nil {
		log.Printf("failed to get win32 file attributes: %v", err)
		return false
	}

	hidden := (attrs & syscall.FILE_ATTRIBUTE_HIDDEN) != 0

	return hidden
}
