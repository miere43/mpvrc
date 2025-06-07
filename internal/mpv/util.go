package mpv

import (
	"bytes"
)

func NextIPCMessage(buffer []byte) (remaining, msg []byte) {
	index := bytes.IndexByte(buffer, '\n')
	if index == -1 {
		return buffer, nil
	}

	msg = buffer[0:index]
	remaining = buffer[index+1:]
	return
}
