package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func utf16(s string) *uint16 {
	utf16Str, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		panic(fmt.Sprintf("Failed to convert string to UTF16: %v", err))
	}
	return utf16Str
}

func main() {
	mpv := NewMPV()
	if err := mpv.Connect(); err != nil {
		panic(fmt.Sprintf("Failed to connect to MPV: %v", err))
	}
	fmt.Printf("Successfully connected to MPV named pipe.\n")

	h := http.NewServeMux()
	srv := &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: h,
	}

	index := func(w http.ResponseWriter, r *http.Request, command string, response string) {
		source, err := os.ReadFile("index.html")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}

		t, err := template.New("index.html").Parse(string(source))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}

		err = t.Execute(w, struct {
			IsConnected bool
			Command     string
			Response    string
		}{
			IsConnected: mpv.IsConnected(),
			Command:     command,
			Response:    response,
		})
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
	}

	h.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		index(w, r, "", "")
	})

	h.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		commandJSON := r.FormValue("command")
		var command []any
		if err := json.Unmarshal([]byte(commandJSON), &command); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}

		var responseText string
		response, err := mpv.SendCommand(command, false)
		if err != nil {
			responseText = err.Error()
		} else {
			responseJSON, err := json.Marshal(response)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(err.Error()))
				return
			}
			responseText = string(responseJSON)
		}

		index(w, r, commandJSON, responseText)
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			fmt.Printf("HTTP server Shutdown: %v\n", err)
		}
		mpv.Disconnect()
		close(quit)
	}()

	<-quit
	fmt.Println("Exiting application...")
}
