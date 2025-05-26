package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/miere43/mpv-web-go/internal/winapi"
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
	pipeHandle     syscall.Handle
	m              sync.RWMutex
	wg             *sync.WaitGroup
	commands       chan *mpvCommand
	done           chan struct{}
	_nextRequestID atomic.Int32

	waitingForResponse      map[int32]*mpvCommand
	waitingForResponseMutex sync.Mutex
}

func NewMPV() *MPV {
	return &MPV{
		wg:                 &sync.WaitGroup{},
		commands:           make(chan *mpvCommand),
		done:               make(chan struct{}),
		waitingForResponse: map[int32]*mpvCommand{},
	}
}

func (mpv *MPV) IsConnected() bool {
	mpv.m.RLock()
	defer mpv.m.RUnlock()

	return mpv.isConnectedNoMutex()
}

func (mpv *MPV) isConnectedNoMutex() bool {
	return mpv.pipeHandle != 0
}

func (mpv *MPV) Connect() error {
	mpv.m.Lock()
	defer mpv.m.Unlock()

	if mpv.isConnectedNoMutex() {
		return nil
	}

	pipeHandle, err := syscall.CreateFile(
		utf16("\\\\.\\pipe\\mpvsocket"),
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL|syscall.FILE_FLAG_OVERLAPPED,
		0,
	)
	if err != nil {
		return fmt.Errorf("failed to connect to MPV named pipe: %w", err)
	}

	mpv.wg.Add(1)
	go mpv.readFromPipe(mpv.wg, pipeHandle, mpv.done)
	go mpv.writeToPipe(mpv.wg, pipeHandle, mpv.commands, mpv.done)

	mpv.pipeHandle = pipeHandle
	return nil
}

func (mpv *MPV) Disconnect() {
	mpv.m.Lock()
	defer mpv.m.Unlock()

	if !mpv.isConnectedNoMutex() {
		return
	}

	mpv.done <- struct{}{}
	mpv.wg.Wait()

	fmt.Println("Closing MPV named pipe handle...")
	if err := syscall.CloseHandle(mpv.pipeHandle); err != nil {
		panic(fmt.Errorf("failed to close MPV named pipe handle: %w", err))
	}

	mpv.pipeHandle = 0
	fmt.Println("Disconnected from MPV named pipe.")
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
	mpv.m.RLock()
	defer mpv.m.RUnlock()

	if !mpv.isConnectedNoMutex() {
		return mpvResponse{}, errors.New("mpv disconnected")
	}

	cmd := &mpvCommand{
		Command:       command,
		RequestID:     mpv.nextRequestID(),
		Async:         async,
		responseReady: make(chan mpvResponse),
	}

	mpv.registerWaitForResponse(cmd)
	mpv.commands <- cmd
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

func (mpv *MPV) readFromPipe(wg *sync.WaitGroup, pipeHandle syscall.Handle, stop <-chan struct{}) {
	defer wg.Done()

	event, err := winapi.CreateEventW(0, false, false)
	if err != nil {
		panic(fmt.Sprintf("Failed to create event: %v", err))
	}
	defer syscall.CloseHandle(event)

	overlapped := &syscall.Overlapped{
		HEvent: event,
	}
	overlappedDone := make(chan bool)

	for {
		// TODO: merge incomplete reads
		var buffer [1024 * 8]byte
		if err = syscall.ReadFile(pipeHandle, buffer[:], nil, overlapped); err != nil {
			if !errors.Is(err, syscall.ERROR_IO_PENDING) {
				panic(fmt.Sprintf("Failed to read from pipe: %v", err))
			}
		}
		fmt.Printf("Read operation initiated, waiting for completion...\n")

		go func() {
			bytesRead, err := winapi.GetOverlappedResult(pipeHandle, overlapped, true)
			if err != nil {
				if errors.Is(err, syscall.ERROR_BROKEN_PIPE) {
					log.Printf("Pipe broken, stopping read loop: %v", err)
					overlappedDone <- true
					return
				}
			}

			responseJSON := make([]byte, int(bytesRead))
			copy(responseJSON, buffer[:bytesRead])

			fmt.Printf("Read operation completed, %d bytes read: %s\n", bytesRead, string(responseJSON))

			var response mpvResponse
			if err = json.Unmarshal(responseJSON, &response); err != nil {
				panic(fmt.Sprintf("unmarshal response: %v", err))
			}

			if response.RequestID != 0 {
				mpv.setResponse(response.RequestID, response)
			}

			overlappedDone <- false
		}()

		select {
		case quit := <-overlappedDone:
			if quit {
				return
			}
		case <-stop:
			return
		}
	}
}

func (mpv *MPV) nextRequestID() int32 {
	return mpv._nextRequestID.Add(1)
}

func (mpv *MPV) writeToPipe(wg *sync.WaitGroup, pipeHandle syscall.Handle, commands <-chan *mpvCommand, stop <-chan struct{}) {
	defer wg.Done()

	event, err := winapi.CreateEventW(0, false, false)
	if err != nil {
		panic(fmt.Sprintf("Failed to create event: %v", err))
	}
	defer syscall.CloseHandle(event)

	overlapped := &syscall.Overlapped{
		HEvent: event,
	}
	overlappedDone := make(chan bool)

	for {
		select {
		case command := <-commands:
			commandJSON, err := json.Marshal(command)
			if err != nil {
				panic(fmt.Sprintf("marshal command: %v", err))
			}
			commandJSON = append(commandJSON, '\n')

			if err = syscall.WriteFile(pipeHandle, commandJSON, nil, overlapped); err != nil {
				panic(fmt.Sprintf("Failed to write to pipe: %v", err))
			}

			fmt.Printf("Write operation initiated, waiting for completion...\n")

			var bytesWritten uint32
			go func() {
				bytesWritten, err = winapi.GetOverlappedResult(pipeHandle, overlapped, true)
				if err != nil {
					panic(fmt.Sprintf("Failed to get overlapped result: %v", err))
				}
				fmt.Printf("Write operation completed, %d bytes written\n", bytesWritten)
				overlappedDone <- false
			}()

			select {
			case quit := <-overlappedDone:
				if quit {
					return
				}
			case <-stop:
				return
			}

		case <-stop:
			return
		}
	}
}
