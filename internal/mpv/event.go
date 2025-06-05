package mpv

import (
	"encoding/json"
	"errors"
	"fmt"
)

type PropertyChange struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
	Data any    `json:"data"`
}

func (PropertyChange) Event() string {
	return "property-change"
}

var ErrUnknownEvent = errors.New("unknown mpv event")

// ParseEvent parses a raw mpv event JSON and returns the corresponding event structure.
// It returns an error if the event type is unknown or if parsing fails.
func ParseEvent(event []byte) (any, error) {
	var header struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(event, &header); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mpv event header: %w", err)
	}

	switch header.Event {
	case "property-change":
		var change PropertyChange
		if err := json.Unmarshal(event, &change); err != nil {
			return nil, fmt.Errorf("failed to unmarshal property-change event: %w", err)
		}
		return change, nil
	}

	return nil, fmt.Errorf(`%w: "%s"`, ErrUnknownEvent, header.Event)
}
