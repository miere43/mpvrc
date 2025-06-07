package mpv_test

import (
	"bytes"
	"testing"

	"github.com/miere43/mpvrc/internal/mpv"
	"github.com/stretchr/testify/suite"
)

type utilSuite struct {
	suite.Suite
}

func TestUtil(t *testing.T) {
	suite.Run(t, new(utilSuite))
}

func (s *utilSuite) TestNextIPCMessage() {
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
		s.Run(test.name, func() {
			remaining, msg := mpv.NextIPCMessage(test.buffer)
			if !bytes.Equal(remaining, test.wantRemaining) {
				s.Fail("remaining: got %s; want %s", remaining, test.wantRemaining)
			}
			if !bytes.Equal(msg, test.wantMsg) {
				s.Fail("msg: got %s; want %s", msg, test.wantMsg)
			}
		})
	}
}

func (s *utilSuite) TestNextIPCMessageDrain() {
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

	r := s.Require()
	s.Empty(buffer)
	r.Len(msgs, 3)
	for i, want := range [][]byte{[]byte("first"), []byte("second"), []byte("third")} {
		if !bytes.Equal(msgs[i], want) {
			s.Fail("at %d got %q; want %q", i, msgs[i], want)
		}
	}
}
