package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/miere43/mpvrc/internal/mpv"
	"github.com/miere43/mpvrc/internal/pipe"
	"github.com/miere43/mpvrc/internal/util"
)

type App struct {
	m                    sync.Mutex
	eventListeners       []*AppEventListener
	eventListenerCounter int

	mpv       *mpv.Conn
	mpvEvents chan any

	globals *Globals

	quit    bool
	quitApp chan struct{}
	server  *httpServer

	mpvCmd *exec.Cmd
}

type AppEventListener struct {
	Events chan []byte
	ID     int
}

const uniqueInstancePipeName = "\\\\.\\pipe\\mpvrc-unique"

func NewApp() (*App, bool) {
	app := &App{
		mpvEvents: make(chan any),

		globals: NewGlobals(),
		quitApp: make(chan struct{}),
	}
	app.server = newHttpServer(app)

	if app.redirectToExistingApplicationInstance() {
		return nil, false
	}

	app.registerUniqueApplicationInstance()

	go app.handleEvents()
	app.startMPV()
	app.startHttpServer()
	app.installInterruptHandler()

	return app, true
}

func (app *App) RequestQuit() {
	app.m.Lock()
	defer app.m.Unlock()

	if app.quit {
		return
	}

	app.quit = true

	if app.mpvCmd != nil && app.mpvCmd.Process != nil {
		// TODO: implement graceful exit, SendCommand(["run", "exit"]) doesn't work.
		if err := app.mpvCmd.Process.Kill(); err != nil {
			slog.Error("failed to kill mpv", "err", err)
		}
	}

	close(app.quitApp)
}

func (app *App) Done() <-chan struct{} {
	return app.quitApp
}

func (app *App) startMPV() {
	args := []string{"--force-window", "--idle", "--input-ipc-server=mpvsocket"}
	if len(os.Args) > 1 {
		args = append(args, os.Args[1])
	}

	app.mpvCmd = exec.Command("C:/soft/mpv/mpv.exe", args...)
	if err := app.mpvCmd.Start(); err != nil {
		util.Fatal("failed to start cmd", "err", err)
	}

	if err := app.ConnectToMPV(500 * time.Millisecond); err != nil {
		util.Fatal("failed to connect to mpv after startup", "err", err)
	}

	go func() {
		slog.Info("waiting for mpv to exit...")
		if err := app.mpvCmd.Wait(); err != nil {
			slog.Error("failed to wait for mpv to close", "err", err)
		}
		app.RequestQuit()
	}()
}

func (app *App) startHttpServer() {
	go func() {
		if err := app.server.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			util.Fatal("failed to start HTTP server", "err", err)
		}
	}()
}

func (app *App) installInterruptHandler() {
	go func() {
		// Wait for Ctrl+C (SIGINT)
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig

		app.server.Shutdown()

		app.RequestQuit()
		// mpv.Disconnect()
	}()
}

func (app *App) redirectToExistingApplicationInstance() bool {
	client, err := pipe.Dial(uniqueInstancePipeName, 0, nil)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		slog.Debug("existing mpvrc instance not found, continuing with normal execution")
		return false
	} else if err != nil {
		util.Fatal("failed to connect to existing mpvrc instance", "err", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			slog.Error("failed to close pipe to existing mpvrc instance", "err", err)
		}
	}()

	slog.Info("found existing mpvrc instance, redirecting command line args to it", "args", os.Args)

	argsJSON, err := json.Marshal(os.Args)
	if err != nil {
		util.Fatal("failed to marshal args", "err", err)
	}

	if err := client.Write(argsJSON); err != nil {
		util.Fatal("failed to send data to existing mpvrc instance", "err", err)
	}
	return true
}

func (app *App) registerUniqueApplicationInstance() {
	server, err := pipe.NewServer(uniqueInstancePipeName, func(client *pipe.ConnectedClient) {
		argsJSON, err := client.ReadMessage()
		if err != nil {
			util.Fatal("failed to read message from connected client", "err", err)
		}

		var args []string
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			util.Fatal("failed to unmarshal json args", "err", err)
		}

		app.handleCommandLineFromFromOtherInstance(args)
	})
	if err != nil {
		util.Fatal("failed to register unique application instance", "err", err)
	}

	slog.Info("created named pipe server for communication with other mpvrc instances")

	go func() {
		server.Serve()
	}()
}

func (app *App) handleCommandLineFromFromOtherInstance(args []string) {
	if len(args) < 2 {
		return
	}

	loadfile := args[1]

	_, err := app.SendCommand([]any{"loadfile", loadfile}, false)
	if err != nil {
		slog.Error("failed to send loadfile command to mpv", "err", err)
	}
}

func (app *App) handleEvents() {
	for event := range app.mpvEvents {
		app.handleEvent(event)
	}
}

func (app *App) handleEvent(event any) {
	// TODO: we can reduce lock scope here
	app.m.Lock()
	defer app.m.Unlock()

	switch e := event.(type) {
	case mpv.PropertyChange:
		app.setGlobalPropertyValue(e.Name, e.Data)

	default:
		slog.Error("handleEvent: unhandled event type", "eventType", e)
	}
}

func (app *App) setGlobalPropertyValue(propertyName string, value json.RawMessage) {
	if changed := app.globals.setValue(propertyName, value); changed {
		app.sendEvent(app.makeGlobalPropertyEvent(propertyName, value))
	}
}

type globalPropertyEvent struct {
	Event        string `json:"event"`
	PropertyName string `json:"propertyName"`
	Value        any    `json:"value"`
}

func (app *App) makeGlobalPropertyEvent(propertyName string, value any) globalPropertyEvent {
	return globalPropertyEvent{
		Event:        "set-global-property",
		PropertyName: propertyName,
		Value:        value,
	}
}

func (app *App) sendEvent(event any) {
	eventJSON, err := json.Marshal(event)
	if err != nil {
		slog.Error("sendEvent: failed to marshal event", "err", err)
		return
	}

	for _, listener := range app.eventListeners {
		listener.Events <- eventJSON
	}
}

func (app *App) connectToMPVCore(timeout time.Duration) (bool, error) {
	app.m.Lock()
	defer app.m.Unlock()

	if app.mpv != nil {
		slog.Debug("ConnectToMPV: mpv was already connected")
		return false, nil
	}

	mpv, err := mpv.Dial(app.mpvEvents, timeout)
	if err != nil {
		return false, fmt.Errorf("failed to connect to mpv: %w", err)
	}

	app.mpv = mpv

	go func() {
		<-mpv.Context().Done()

		app.m.Lock()
		defer app.m.Unlock()

		app.sendEvent(app.makeGlobalPropertyEvent("connected", false))

		app.mpv = nil // Allow us to reconnect next time.
	}()

	return true, nil
}

func (app *App) ConnectToMPV(timeout time.Duration) error {
	wantInit, err := app.connectToMPVCore(timeout)
	if err != nil {
		return err
	}

	if wantInit {
		for propertyName := range app.globals.properties {
			app.mpv.ObserveProperty(propertyName)
		}
	}

	return nil
}

func (app *App) SendCommand(args []any, async bool) (mpv.MpvResponse, error) {
	app.m.Lock()
	defer app.m.Unlock()

	if app.mpv == nil {
		return mpv.MpvResponse{}, fmt.Errorf("not connected to mpv")
	}
	return app.mpv.SendCommand(args, async)
}

func (app *App) IsConnectedToMPV() bool {
	app.m.Lock()
	defer app.m.Unlock()

	return app.mpv != nil
}

func (app *App) StartupEvents() []any {
	app.m.Lock()
	defer app.m.Unlock()

	events := []any{
		app.makeGlobalPropertyEvent("connected", app.mpv != nil),
	}

	for propertyName, value := range app.globals.properties {
		events = append(events, app.makeGlobalPropertyEvent(propertyName, value))
	}

	events = append(events, app.makeGlobalPropertyEvent("ready", true))

	return events
}

func (app *App) NewEventListener() *AppEventListener {
	app.m.Lock()
	defer app.m.Unlock()

	app.eventListenerCounter++
	listener := &AppEventListener{
		Events: make(chan []byte),
		ID:     app.eventListenerCounter,
	}
	app.eventListeners = append(app.eventListeners, listener)

	slog.Debug("created event listener", "id", listener.ID)
	return listener
}

func (app *App) CloseEventListener(closeListener *AppEventListener) {
	app.m.Lock()
	defer app.m.Unlock()

	index := slices.Index(app.eventListeners, closeListener)
	if index == -1 {
		panic(fmt.Sprintf("unknown listener %v", closeListener))
	}

	app.eventListeners = slices.Delete(app.eventListeners, index, index+1)
	close(closeListener.Events)

	slog.Debug("closed event listener", "id", closeListener.ID)
}
