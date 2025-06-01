package main

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"slices"
	"sync"

	"github.com/miere43/mpvrc/internal/mpv"
)

type Globals struct {
	PlaybackTime string  `json:"playbackTime" mpv:"playback-time" setter:"FormatDuration"`
	Duration     string  `json:"duration" setter:"FormatDuration"`
	Pause        bool    `json:"pause"`
	Volume       float64 `json:"volume"`
	Path         string  `json:"path"`
	Speed        float64 `json:"speed"`
}

type GlobalFieldInfo struct {
	Type           reflect.Type
	SetterFunc     reflect.Value
	SerializedName string
	MPVName        string
	Index          []int
}

func (g *Globals) SetFieldValue(info GlobalFieldInfo, value any) (changed bool) {
	reflectValue := reflect.ValueOf(value)
	if value == nil {
		log.Printf("Globals.SetFieldValue: got nil value for %q, using default value", info.MPVName)
		reflectValue = reflect.New(info.Type).Elem()
	}

	if info.SetterFunc.IsValid() {
		result := info.SetterFunc.Call([]reflect.Value{reflectValue})
		if len(result) != 1 {
			panic(fmt.Sprintf("want single return value from setter, got %d", len(result)))
		}
		reflectValue = result[0]
	}

	fieldRef := reflect.ValueOf(g).Elem().FieldByIndex(info.Index)

	newValue := reflectValue.Interface()
	oldValue := fieldRef.Interface()

	if newValue != oldValue {
		fieldRef.Set(reflectValue)
		changed = true
	}

	return
}

func (g *Globals) FieldValue(info GlobalFieldInfo) any {
	r := reflect.ValueOf(g).Elem()
	return r.FieldByIndex(info.Index).Interface()
}

func (g *Globals) FormatDuration(value any) string {
	duration, ok := value.(float64)
	if !ok {
		log.Printf("unexpected type for duration: %T", value)
	}
	return mpv.FormatDuration(duration)
}

func (g *Globals) getFieldByIndex(fieldIndex int) GlobalFieldInfo {
	reflectG := reflect.ValueOf(g)
	typeG := reflectG.Elem().Type()

	field := typeG.Field(fieldIndex)

	info := GlobalFieldInfo{
		Type:           field.Type,
		SerializedName: field.Tag.Get("json"),
		Index:          field.Index,
	}
	if info.SerializedName == "" {
		panic(fmt.Errorf("field %s must have json tag", field.Name))
	}

	info.MPVName = field.Tag.Get("mpv")
	if info.MPVName == "" {
		info.MPVName = info.SerializedName
	}

	if setterName := field.Tag.Get("setter"); setterName != "" {
		info.SetterFunc = reflectG.MethodByName(setterName)
		if !info.SetterFunc.IsValid() {
			panic(fmt.Errorf("cannot find setter method %q for field %q", setterName, field.Name))
		}
	}
	return info
}

func (g *Globals) GetFieldByMPVName(mpvName string) (GlobalFieldInfo, bool) {
	typ := reflect.TypeOf(g).Elem()
	for fieldIndex := range typ.NumField() {
		info := g.getFieldByIndex(fieldIndex)
		if info.MPVName == mpvName {
			return info, true
		}
	}
	return GlobalFieldInfo{}, false
}

func (g *Globals) Fields() []GlobalFieldInfo {
	var fields []GlobalFieldInfo

	typ := reflect.TypeOf(g).Elem()
	for fieldIndex := range typ.NumField() {
		fields = append(fields, g.getFieldByIndex(fieldIndex))
	}
	return fields
}

type App struct {
	m                    sync.Mutex
	eventListeners       []*AppEventListener
	eventListenerCounter int

	mpv       *mpv.Conn
	mpvEvents chan any

	globals *Globals
}

type AppEventListener struct {
	Events chan []byte
	ID     int
}

func NewApp() *App {
	app := &App{
		mpvEvents: make(chan any),

		globals: &Globals{
			PlaybackTime: mpv.FormatDuration(0),
		},
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
		app.setMPVFieldValue(e.Name, e.Data)

	default:
		log.Printf("handleEvent: unhandled event type: %v", e)
	}
}

func (app *App) setMPVFieldValue(mpvName string, value any) {
	field, ok := app.globals.GetFieldByMPVName(mpvName)
	if !ok {
		log.Printf("setMPVFieldValue: unknown property name: %q", mpvName)
		return
	}

	if changed := app.globals.SetFieldValue(field, value); changed {
		app.sendEvent(app.makeGlobalPropertyEvent(field.SerializedName, app.globals.FieldValue(field)))
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

func (app *App) connectToMPVCore() (bool, error) {
	app.m.Lock()
	defer app.m.Unlock()

	if app.mpv != nil {
		log.Printf("ConnectToMPV: mpv was already connected")
		// Already connected
		return false, nil
	}

	mpv, err := mpv.Dial(app.mpvEvents)
	if err != nil {
		return false, fmt.Errorf("failed to connect to mpv: %v", err)
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

func (app *App) ConnectToMPV() error {
	wantInit, err := app.connectToMPVCore()
	if err != nil {
		return err
	}

	if wantInit {
		for _, field := range app.globals.Fields() {
			app.mpv.ObserveProperty(field.MPVName)
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

	for _, field := range app.globals.Fields() {
		events = append(events, app.makeGlobalPropertyEvent(field.SerializedName, app.globals.FieldValue(field)))
	}

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
