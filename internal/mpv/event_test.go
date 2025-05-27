package mpv_test

import (
	"testing"

	"github.com/miere43/mpv-web-go/internal/mpv"
)

func TestParseEvent(t *testing.T) {
	event, err := mpv.ParseEvent([]byte(`{"event":"property-change","id":1,"name":"playback-time"}`))
	if err != nil {
		t.Fatalf("want no error; got %v", err)
	}

	change, ok := event.(mpv.PropertyChange)
	if !ok {
		t.Fatalf("want type %T, got %T", mpv.PropertyChange{}, event)
	}

	if change.Event() != "property-change" {
		t.Errorf("want event type %q, got %q", "property-change", change.Event())
	}
	if change.Id != 1 {
		t.Errorf("want id %d, got %d", 1, change.Id)
	}
	if change.Name != "playback-time" {
		t.Errorf("want name %q, got %q", "playback-time", change.Name)
	}
	if change.Data != nil {
		t.Errorf("want data %v, got %v", nil, change.Data)
	}
}
