package main

import (
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"sync"

	"github.com/miere43/mpv-web-go/internal/mpv"
)

type App struct {
	m                    sync.Mutex
	eventListeners       []*AppEventListener
	eventListenerCounter int

	mpv       *mpv.Conn
	mpvEvents chan any

	playbackTime string
	pause        bool
}

type AppEventListener struct {
	Events chan []byte
	ID     int
}

func NewApp() *App {
	app := &App{
		mpvEvents: make(chan any),

		playbackTime: mpv.PlaybackTimeToString(0),
	}
	go app.handleEvents()
	return app
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
		switch e.Name {
		case "playback-time":
			playbackTime, ok := e.Data.(float64)
			if !ok {
				log.Printf("handleEvent: unexpected type for playback-time: %T", e.Data)
				break
			}

			playbackTimeString := mpv.PlaybackTimeToString(playbackTime)
			if app.playbackTime != playbackTimeString {
				app.playbackTime = playbackTimeString
				app.sendEvent(app.makeGlobalPropertyEvent("playbackTime", app.playbackTime))
			}

		case "pause":
			pause, ok := e.Data.(bool)
			if !ok {
				log.Printf("handleEvent: unexpected type for pause: %T", e.Data)
				break
			}

			if app.pause != pause {
				app.pause = pause
				app.sendEvent(app.makeGlobalPropertyEvent("pause", app.pause))
			}

		default:
			log.Printf("handleEvent: unhandled property change: %s", e.Name)
		}

	default:
		log.Printf("handleEvent: unhandled event type: %v", e)
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
		log.Printf("sendEvent: failed to marshal event: %v", err)
		return
	}

	for _, listener := range app.eventListeners {
		listener.Events <- eventJSON
	}
}

func (app *App) ConnectToMPV() error {
	app.m.Lock()
	defer app.m.Unlock()

	if app.mpv != nil {
		log.Printf("ConnectToMPV: mpv was already connected")
		// Already connected
		return nil
	}

	mpv, err := mpv.Dial(app.mpvEvents)
	if err != nil {
		return fmt.Errorf("failed to connect to mpv: %v", err)
	}

	mpv.ObserveProperties("playback-time", "pause")

	app.mpv = mpv
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

	return []any{
		app.makeGlobalPropertyEvent("connected", app.mpv != nil),
		app.makeGlobalPropertyEvent("playbackTime", app.playbackTime),
		app.makeGlobalPropertyEvent("pause", app.pause),
	}
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

	log.Printf("created event listener %d", listener.ID)
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

	log.Printf("closed event listener %d", closeListener.ID)
}
