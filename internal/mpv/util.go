package mpv

import (
	"bytes"
	"fmt"
)

func FormatDuration(seconds float64) string {
	if seconds < 0 {
		return "00:00:00"
	}
	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, int(seconds)%60)
}

func NextIPCMessage(buffer []byte) (remaining, msg []byte) {
	index := bytes.IndexByte(buffer, '\n')
	if index == -1 {
		return buffer, nil
	}

	msg = buffer[0:index]
	remaining = buffer[index+1:]
	return
}
