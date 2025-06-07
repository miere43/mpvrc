package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

func main() {
	setupLogging()

	app, ok := NewApp()
	if ok {
		<-app.Done()
	}
}

func setupLogging() {
	exePath, err := os.Executable()
	if err != nil {
		slog.Error("failed to get executable path", "err", err)
		return
	}

	logPath := filepath.Join(filepath.Dir(exePath), "mpvrc.log")
	file, err := os.Create(logPath)
	if err != nil {
		slog.Error("failed to create log file", "logPath", logPath, "err", err)
		return
	}

	writer := io.Writer(file)
	if os.Stdout != nil && os.Stdout.Fd() != 0 {
		writer = io.MultiWriter(os.Stdout, file)
	}

	logger := slog.New(slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)
}
