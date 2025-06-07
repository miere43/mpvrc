package mpv_test

import (
	"testing"

	"github.com/miere43/mpvrc/internal/mpv"
	"github.com/stretchr/testify/suite"
)

type mpvSuite struct {
	suite.Suite
}

func TestMpv(t *testing.T) {
	suite.Run(t, new(mpvSuite))
}

func (s *mpvSuite) TestParseEvent() {
	r := s.Require()
	event, err := mpv.ParseEvent([]byte(`{"event":"property-change","id":1,"name":"playback-time"}`))
	r.NoError(err)

	change, ok := event.(mpv.PropertyChange)
	r.True(ok)

	s.Equal("property-change", change.Event())
	s.Equal(1, change.Id)
	s.Equal("playback-time", change.Name)
	s.Nil(change.Data)
}
