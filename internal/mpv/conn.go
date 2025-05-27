package mpv

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"github.com/miere43/mpv-web-go/internal/pipe"
)

type MpvCommand struct {
	Command   []any `json:"command"`
	RequestID int32 `json:"request_id,omitempty"`
	Async     bool  `json:"async,omitempty"`

	responseReady chan MpvResponse `json:"-"`
}

type MpvResponse struct {
	RequestID int32  `json:"request_id"`
	Error     string `json:"error"`
	Data      any    `json:"data"`
}

type Conn struct {
	conn  *pipe.Conn
	reads chan []byte

	wg             *sync.WaitGroup
	commands       chan *MpvCommand
	done           chan struct{}
	_nextRequestID atomic.Int32

	waitingForResponse      map[int32]*MpvCommand
	waitingForResponseMutex sync.Mutex

	events chan any

	nextPropertyID atomic.Int32

	// mpvProcess *exec.Cmd
}

func Dial(events chan any) (*Conn, error) {
	const pipeName = "\\\\.\\pipe\\mpvsocket"

	reads := make(chan []byte)
	conn, err := pipe.Dial(pipeName, reads)
	if err != nil {
		return nil, err
	}

	mpv := &Conn{
		conn:  conn,
		reads: reads,

		wg:                 &sync.WaitGroup{},
		commands:           make(chan *MpvCommand),
		done:               make(chan struct{}),
		waitingForResponse: map[int32]*MpvCommand{},

		events: events,
	}

	mpv.wg.Add(1)
	go mpv.readResponses()

	return mpv, nil
}

func (mpv *Conn) Disconnect() {
	if err := mpv.conn.Close(); err != nil {
		panic(fmt.Sprintf("failed to close MPV named pipe: %v", err))
	}
	close(mpv.reads)
	mpv.wg.Wait()
}

func (mpv *Conn) registerWaitForResponse(cmd *MpvCommand) {
	mpv.waitingForResponseMutex.Lock()
	defer mpv.waitingForResponseMutex.Unlock()
	mpv.waitingForResponse[cmd.RequestID] = cmd
}

func (mpv *Conn) setResponse(requestID int32, response MpvResponse) {
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

func (mpv *Conn) waitForResponse(cmd *MpvCommand) MpvResponse {
	response := <-cmd.responseReady

	mpv.waitingForResponseMutex.Lock()
	defer mpv.waitingForResponseMutex.Unlock()
	delete(mpv.waitingForResponse, cmd.RequestID)

	return response
}

func (mpv *Conn) SendCommand(command []any, async bool) (MpvResponse, error) {
	cmd := &MpvCommand{
		Command:       command,
		RequestID:     mpv.nextRequestID(),
		Async:         async,
		responseReady: make(chan MpvResponse),
	}

	cmdJSON, err := json.Marshal(cmd)
	if err != nil {
		return MpvResponse{}, fmt.Errorf("marshal command: %w", err)
	}
	cmdJSON = append(cmdJSON, '\n')

	mpv.registerWaitForResponse(cmd)

	if err := mpv.conn.Write(cmdJSON); err != nil {
		// TODO: unregisterWaitForResponse(cmd)
		return MpvResponse{}, fmt.Errorf("write command to MPV: %w", err)
	}

	response := mpv.waitForResponse(cmd)
	if response.Error == "success" {
		return response, nil
	}
	return response, errors.New(response.Error)
}

func (mpv *Conn) ObserveProperty(property string) {
	_, err := mpv.SendCommand([]any{"observe_property", mpv.nextPropertyID.Add(1), property}, false)
	if err != nil {
		log.Printf("failed to observe property \"%v\": %v", property, err)
	}
}

func (mpv *Conn) readResponses() {
	defer mpv.wg.Done()

	// Stores incomplete messages
	var buffer []byte

	for partialRead := range mpv.reads {
		buffer = append(buffer, partialRead...)

		for {
			var completeRead []byte
			buffer, completeRead = NextIPCMessage(buffer)
			if completeRead == nil {
				break
			}

			event, err := ParseEvent(completeRead)
			if errors.Is(err, ErrUnknownEvent) {
				var response MpvResponse
				if err := json.Unmarshal(completeRead, &response); err != nil {
					panic(fmt.Sprintf("unmarshal response: %v \"%v\"", err, string(completeRead)))
				}

				if response.RequestID != 0 {
					mpv.setResponse(response.RequestID, response)
				}
			} else if err != nil {
				log.Printf("failed to parse MPV event: %v \"%v\"", err, string(completeRead))
			} else {
				mpv.events <- event
			}
		}
	}
}

func (mpv *Conn) nextRequestID() int32 {
	return mpv._nextRequestID.Add(1)
}
