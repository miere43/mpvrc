package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/miere43/mpv-web-go/internal/pipe"
)

type mpvCommand struct {
	Command   []any `json:"command"`
	RequestID int32 `json:"request_id,omitempty"`
	Async     bool  `json:"async,omitempty"`

	responseReady chan mpvResponse `json:"-"`
}

type mpvResponse struct {
	RequestID int32  `json:"request_id"`
	Error     string `json:"error"`
	Data      any    `json:"data"`
}

type MPV struct {
	conn  *pipe.Conn
	reads chan []byte

	wg             *sync.WaitGroup
	commands       chan *mpvCommand
	done           chan struct{}
	_nextRequestID atomic.Int32

	responseBuffer []byte

	waitingForResponse      map[int32]*mpvCommand
	waitingForResponseMutex sync.Mutex
}

func NewMPV() *MPV {
	reads := make(chan []byte)
	conn, err := pipe.Dial("\\\\.\\pipe\\mpvsocket", reads)
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to MPV named pipe: %v", err))
	}

	return &MPV{
		conn:  conn,
		reads: reads,

		wg:                 &sync.WaitGroup{},
		commands:           make(chan *mpvCommand),
		done:               make(chan struct{}),
		waitingForResponse: map[int32]*mpvCommand{},
	}
}

func (mpv *MPV) Connect() error {
	mpv.wg.Add(1)
	go mpv.readResponses()
	return nil
}

func (mpv *MPV) IsConnected() bool {
	return true
}

func (mpv *MPV) Disconnect() {
	if err := mpv.conn.Close(); err != nil {
		panic(fmt.Sprintf("failed to close MPV named pipe: %v", err))
	}
	close(mpv.reads)
	mpv.wg.Wait()
}

func (mpv *MPV) registerWaitForResponse(cmd *mpvCommand) {
	mpv.waitingForResponseMutex.Lock()
	defer mpv.waitingForResponseMutex.Unlock()
	mpv.waitingForResponse[cmd.RequestID] = cmd
}

func (mpv *MPV) setResponse(requestID int32, response mpvResponse) {
	mpv.waitingForResponseMutex.Lock()
	cmd, ok := mpv.waitingForResponse[requestID]
	if !ok {
		fmt.Printf("Got response for unknown request ID %d\n", requestID)
		return
	}

	delete(mpv.waitingForResponse, cmd.RequestID)
	mpv.waitingForResponseMutex.Unlock()

	cmd.responseReady <- response
}

func (mpv *MPV) waitForResponse(cmd *mpvCommand) mpvResponse {
	response := <-cmd.responseReady

	mpv.waitingForResponseMutex.Lock()
	defer mpv.waitingForResponseMutex.Unlock()
	delete(mpv.waitingForResponse, cmd.RequestID)

	return response
}

func (mpv *MPV) SendCommand(command []any, async bool) (mpvResponse, error) {
	cmd := &mpvCommand{
		Command:       command,
		RequestID:     mpv.nextRequestID(),
		Async:         async,
		responseReady: make(chan mpvResponse),
	}

	cmdJSON, err := json.Marshal(cmd)
	if err != nil {
		return mpvResponse{}, fmt.Errorf("marshal command: %w", err)
	}
	cmdJSON = append(cmdJSON, '\n')

	mpv.registerWaitForResponse(cmd)

	if err := mpv.conn.Write(cmdJSON); err != nil {
		// TODO: unregisterWaitForResponse(cmd)
		return mpvResponse{}, fmt.Errorf("write command to MPV: %w", err)
	}

	response := mpv.waitForResponse(cmd)
	if response.Error == "success" {
		return response, nil
	}
	return response, errors.New(response.Error)
}

func (mpv *MPV) ObserveProperty(property string, id int) error {
	_, err := mpv.SendCommand([]any{"observe_property", id, property}, false)
	return err
}

func (mpv *MPV) bufferRead(read []byte) []byte {
	mpv.responseBuffer = append(mpv.responseBuffer, read...)
	index := bytes.IndexByte(mpv.responseBuffer, '\n')
	if index == -1 {
		return nil // Not enough data to form a complete response
	}
	response := make([]byte, index)
	copy(response, mpv.responseBuffer[:index])
	mpv.responseBuffer = mpv.responseBuffer[index+1:] // Remove the processed part
	return response
}

func (mpv *MPV) readResponses() {
	defer mpv.wg.Done()

	for read := range mpv.reads {
		completeResponse := mpv.bufferRead(read)
		if completeResponse == nil {
			continue // Not enough data to form a complete response
		}

		var response mpvResponse
		if err := json.Unmarshal(completeResponse, &response); err != nil {
			panic(fmt.Sprintf("unmarshal response: %v \"%v\"", err, string(read)))
		}

		if response.RequestID != 0 {
			mpv.setResponse(response.RequestID, response)
		}
	}
}

func (mpv *MPV) nextRequestID() int32 {
	return mpv._nextRequestID.Add(1)
}
