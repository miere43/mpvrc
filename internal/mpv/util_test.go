package mpv_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/miere43/mpvrc/internal/mpv"
)

func TestPlaybackTimeToString(t *testing.T) {
	for _, test := range []struct {
		seconds float64
		want    string
	}{
		{
			seconds: 4.004000,
			want:    "00:00:04",
		},
		{
			seconds: 0,
			want:    "00:00:00",
		},
		{
			seconds: 1,
			want:    "00:00:01",
		},
		{
			seconds: 59,
			want:    "00:00:59",
		},
		{
			seconds: 60,
			want:    "00:01:00",
		},
		{
			seconds: 61,
			want:    "00:01:01",
		},
		{
			seconds: 3599,
			want:    "00:59:59",
		},
		{
			seconds: 3600,
			want:    "01:00:00",
		},
		{
			seconds: 3661,
			want:    "01:01:01",
		},
		{
			seconds: 86399,
			want:    "23:59:59",
		},
		{
			seconds: 86400,
			want:    "24:00:00",
		},
		{
			seconds: -1,
			want:    "00:00:00",
		},
		{
			seconds: 3723.7,
			want:    "01:02:03",
		},
	} {
		t.Run(fmt.Sprintf("%v seconds must convert to %v", test.seconds, test.want), func(t *testing.T) {
			actual := mpv.FormatDuration(test.seconds)
			if test.want != actual {
				t.Errorf("got %v; want %v", actual, test.want)
			}
		})
	}
}

func TestNextIPCMessage(t *testing.T) {
	for _, test := range []struct {
		name          string
		buffer        []byte
		wantRemaining []byte
		wantMsg       []byte
	}{
		{
			name:          "Same buffer is returned when there is no message in buffer",
			buffer:        []byte("hello"),
			wantRemaining: []byte("hello"),
			wantMsg:       nil,
		},
		{
			name:          "Return empty buffer when there is single message",
			buffer:        []byte("hello\n"),
			wantRemaining: []byte(""),
			wantMsg:       []byte("hello"),
		},
		{
			name:          "Return remaining one message when there is incomplete message at the end",
			buffer:        []byte("first\nsecond"),
			wantRemaining: []byte("second"),
			wantMsg:       []byte("first"),
		},
		{
			name:          "Return remaining two messages when there is incomplete message at the end",
			buffer:        []byte("first\nsecond\nthird"),
			wantRemaining: []byte("second\nthird"),
			wantMsg:       []byte("first"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			remaining, msg := mpv.NextIPCMessage(test.buffer)
			if !bytes.Equal(remaining, test.wantRemaining) {
				t.Errorf("remaining: got %s; want %s", remaining, test.wantRemaining)
			}
			if !bytes.Equal(msg, test.wantMsg) {
				t.Errorf("msg: got %s; want %s", msg, test.wantMsg)
			}
		})
	}
}

func TestNextIPCMessageDrain(t *testing.T) {
	buffer := []byte("first\nsecond\nthird\n")
	msgs := make([][]byte, 0)
	for {
		var msg []byte
		buffer, msg = mpv.NextIPCMessage(buffer)
		if msg == nil {
			break
		}
		msgs = append(msgs, msg)
	}

	if len(buffer) != 0 {
		t.Errorf("expect buffer to be empty; got %v", buffer)
	}

	if len(msgs) != 3 {
		t.Fatalf("expect 3 results; got %v", len(msgs))
	}
	for i, want := range [][]byte{[]byte("first"), []byte("second"), []byte("third")} {
		if !bytes.Equal(msgs[i], want) {
			t.Errorf("at %d got %q; want %q", i, msgs[i], want)
		}
	}
}
