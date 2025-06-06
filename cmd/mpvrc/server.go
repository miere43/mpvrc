package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/miere43/mpvrc/internal/util"
)

type httpServer struct {
	srv         *http.Server
	app         *App
	appDir      string
	shutdownSSE chan struct{}
}

func newHttpServer(app *App) *httpServer {
	exePath, err := os.Executable()
	if err != nil {
		util.Fatal("failed to get current executable path", "err", err)
	}

	h := http.NewServeMux()
	s := &httpServer{
		srv: &http.Server{
			Addr:    "0.0.0.0:8080",
			Handler: h,
		},
		app:         app,
		appDir:      filepath.Dir(exePath),
		shutdownSSE: make(chan struct{}),
	}

	h.HandleFunc("GET /", s.index)
	h.HandleFunc("GET /favicon.png", s.favicon)
	h.HandleFunc("GET /events", s.events)
	h.HandleFunc("POST /command", s.command)
	h.HandleFunc("GET /file-system", s.fileSystem)

	return s
}

func (s *httpServer) Shutdown() {
	slog.Debug("shutting down HTTP server...")
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	close(s.shutdownSSE)
	if err := s.srv.Shutdown(ctx); err != nil {
		slog.Error("failed to shutdown HTTP server", "err", err)
	}
}

func (s *httpServer) index(w http.ResponseWriter, r *http.Request) {
	s.cors(w)

	if err := s.app.ConnectToMPV(0); err != nil {
		slog.Error("failed to connect to mpv", "err", err)
	}

	source, err := os.ReadFile(filepath.Join(s.appDir, "front/dist/index.html"))
	if err != nil {
		s.handleError(w, err)
		return
	}

	w.Write(source)
}

func (s *httpServer) favicon(w http.ResponseWriter, r *http.Request) {
	s.cors(w)

	// TODO: extract icon from embed resources
	source, err := os.ReadFile(filepath.Join(s.appDir, "winres/icon_256.png"))
	if err != nil {
		s.handleError(w, err)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Write(source)
}

func (s *httpServer) events(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers to allow all origins. You may want to restrict this to specific origins in a production environment.
	s.cors(w)
	w.Header().Set("Access-Control-Expose-Headers", "Content-Type")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	listener := s.app.NewEventListener()

	for _, event := range s.app.StartupEvents() {
		eventJSON, err := json.Marshal(event)
		if err != nil {
			slog.Error("failed to marshal startup event", "err", err, "event", event)
			continue
		}

		fmt.Fprintf(w, "data: %s\n\n", eventJSON)
	}
	w.(http.Flusher).Flush()

	loop := true
	for loop {
		select {
		case event := <-listener.Events:
			fmt.Fprintf(w, "data: %s\n\n", event)
			w.(http.Flusher).Flush()

		case <-r.Context().Done():
			slog.Debug("Context done!")
			loop = false
			s.app.CloseEventListener(listener)

		case <-s.shutdownSSE:
			slog.Debug("Shutdown SSE!")
			loop = false
			s.app.CloseEventListener(listener)
		}
	}
}

func (s *httpServer) command(w http.ResponseWriter, r *http.Request) {
	s.cors(w)

	commandJSON := r.FormValue("command")
	var command []any
	if err := json.Unmarshal([]byte(commandJSON), &command); err != nil {
		s.handleError(w, err)
		return
	}

	response, err := s.app.SendCommand(command, false)
	if err != nil {
		s.handleError(w, err)
		return
	}

	s.writeJSON(w, response.Data)
}

func (s *httpServer) fileSystem(w http.ResponseWriter, r *http.Request) {
	s.cors(w)

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
		s.handleError(w, fmt.Errorf("path %q must be absolute", path))
		return
	} else {
		dirEntries, err := os.ReadDir(path)
		if err != nil {
			s.handleError(w, err)
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
			return entry.Name != ".." && s.isHiddenFile(entry.Path)
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

	s.writeJSON(w, struct {
		Path    string  `json:"path"`
		Entries []Entry `json:"entries"`
	}{
		Path:    path,
		Entries: entries,
	})
}

func (s *httpServer) handleError(w http.ResponseWriter, err error) {
	slog.Error("failed to handle http", "err", err)
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(err.Error()))
}

func (s *httpServer) writeJSON(w http.ResponseWriter, data any) {
	output, err := json.Marshal(data)
	if err != nil {
		s.handleError(w, err)
		return
	}
	w.Write(output)
}

func (s *httpServer) isHiddenFile(path string) bool {
	pathW, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		slog.Error("failed to convert path to utf16", "err", err)
		return false
	}

	attrs, err := syscall.GetFileAttributes(pathW)
	if err != nil {
		slog.Error("failed to get win32 file attributes", "err", err)
		return false
	}

	hidden := (attrs & syscall.FILE_ATTRIBUTE_HIDDEN) != 0

	return hidden
}

func (s *httpServer) cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
}
