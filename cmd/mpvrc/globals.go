package main

import (
	"encoding/json"
	"log/slog"
)

type Globals struct {
	properties map[string]json.RawMessage
}

func NewGlobals() *Globals {
	null := json.RawMessage("null")
	return &Globals{
		properties: map[string]json.RawMessage{
			"playback-time": null,
			"duration":      null,
			"pause":         json.RawMessage("false"),
			"volume":        json.RawMessage("100.000000"),
			"path":          null,
			"speed":         json.RawMessage("1.000000"),
		},
	}
}

func (g *Globals) setValue(propertyName string, newValue json.RawMessage) (changed bool) {
	if newValue == nil {
		newValue = json.RawMessage("null")
	}

	l := slog.With("context", "Globals.setValue", "propertyName", propertyName, "newValue", newValue)

	oldValue, ok := g.properties[propertyName]
	if !ok {
		l.Error("unknown property")
		return false
	}
	l = l.With("oldValue", oldValue)

	if string(oldValue) == string(newValue) {
		l.Debug("value did not change")
		return false
	}

	g.properties[propertyName] = newValue
	l.Debug("new value was assigned")
	return true
}
