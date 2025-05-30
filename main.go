package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"
)

func main() {
	app := NewApp()

	h := http.NewServeMux()
	srv := &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: h,
	}

	h.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if err := app.ConnectToMPV(); err != nil {
			log.Printf("failed to connect to MPV: %v", err)
		}

		source, err := os.ReadFile("index.html")
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

			if prevPath := filepath.Dir(path); prevPath != "/" {
				entries = append(entries, Entry{
					Name:  "..",
					Path:  prevPath,
					IsDir: true,
				})
			}

			for _, entry := range dirEntries {
				entries = append(entries, Entry{
					Name:  entry.Name(),
					Path:  filepath.Join(path, entry.Name()),
					IsDir: entry.IsDir(),
				})
			}
		}

		writeJSON(w, entries)
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
