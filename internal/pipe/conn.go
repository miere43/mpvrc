package pipe

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"syscall"

	"github.com/miere43/mpvrc/internal/winapi"
)

type Conn struct {
	pipeHandle syscall.Handle
	reads      chan<- []byte
	writes     chan []byte
	ctx        context.Context
	cancelCtx  context.CancelFunc
	canceled   bool
	wg         *sync.WaitGroup
}

func Dial(name string, reads chan<- []byte) (*Conn, error) {
	writes := make(chan []byte)
	var pipeHandle syscall.Handle
	defer func() {
		if pipeHandle != 0 {
			syscall.CloseHandle(pipeHandle)
			close(writes)
		}
	}()

	name16, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("failed to convert string to UTF16: %w", err)
	}

	pipeHandle, err = syscall.CreateFile(
		name16,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL|syscall.FILE_FLAG_OVERLAPPED,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to named pipe: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	conn := &Conn{
		pipeHandle: pipeHandle,
		reads:      reads,
		writes:     writes,
		ctx:        ctx,
		cancelCtx:  cancel,
		wg:         &sync.WaitGroup{},
	}

	conn.wg.Add(2)
	go conn.readFromPipe(ctx)
	go conn.writeToPipe(ctx)

	pipeHandle = 0 // Prevent closing the handle in the defer statement
	return conn, nil
}

func (conn *Conn) Context() context.Context {
	return conn.ctx
}

func (conn *Conn) readFromPipe(ctx context.Context) {
	defer conn.wg.Done()

	event, err := winapi.CreateEventW(0, false, false)
	if err != nil {
		panic(fmt.Sprintf("failed to create event: %v", err))
	}
	defer syscall.CloseHandle(event)

	overlapped := &syscall.Overlapped{
		HEvent: event,
	}
	overlappedDone := make(chan struct{})

	var buffer [1024 * 8]byte
	for {
		if err = syscall.ReadFile(conn.pipeHandle, buffer[:], nil, overlapped); err != nil {
			if !errors.Is(err, syscall.ERROR_IO_PENDING) {
				log.Printf("failed to read from pipe: %v", err)
				conn.cancel()
				return
			}
		}

		go func() {
			bytesRead, err := winapi.GetOverlappedResult(conn.pipeHandle, overlapped, true)
			if err != nil {
				log.Printf("failed to get overlapped result: %v\n", err)
				conn.cancel()
				return
			}

			responseJSON := make([]byte, int(bytesRead))
			copy(responseJSON, buffer[:bytesRead])

			fmt.Printf("Read operation completed, %d bytes read: %s\n", bytesRead, string(responseJSON))

			conn.reads <- responseJSON
			overlappedDone <- struct{}{}
		}()

		select {
		case <-overlappedDone:
			// Successfully read data
		case <-ctx.Done():
			return
		}
	}
}

func (conn *Conn) writeToPipe(ctx context.Context) {
	defer conn.wg.Done()

	event, err := winapi.CreateEventW(0, false, false)
	if err != nil {
		panic(fmt.Sprintf("failed to create event: %v", err))
	}
	defer syscall.CloseHandle(event)

	overlapped := &syscall.Overlapped{
		HEvent: event,
	}
	overlappedDone := make(chan struct{})

	for {
		select {
		case write := <-conn.writes:
			fmt.Printf("Preparing to write %d bytes to pipe: %s\n", len(write), string(write))
			if err = syscall.WriteFile(conn.pipeHandle, write, nil, overlapped); err != nil {
				panic(fmt.Sprintf("Failed to write to pipe: %v", err))
			}

			fmt.Printf("Write operation initiated, waiting for completion...\n")

			var bytesWritten uint32
			go func() {
				bytesWritten, err = winapi.GetOverlappedResult(conn.pipeHandle, overlapped, true)
				if err != nil {
					panic(fmt.Sprintf("failed to get overlapped result: %v", err))
				}
				fmt.Printf("Write operation completed, %d bytes written\n", bytesWritten)
				overlappedDone <- struct{}{}
			}()

			select {
			case <-overlappedDone:
				// Successfully wrote data
			case <-ctx.Done():
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

func (c *Conn) Write(data []byte) error {
	if c.pipeHandle == 0 {
		return errors.New("connection already closed")
	}
	if c.canceled {
		return errors.New("connection is closed")
	}
	// TODO: this is jank. c.canceled may become true after this check
	c.writes <- data
	return nil
}

func (c *Conn) Close() error {
	if c.pipeHandle == 0 {
		return errors.New("connection already closed")
	}

	c.cancelCtx()
	close(c.writes)
	c.wg.Wait()

	if err := syscall.CloseHandle(c.pipeHandle); err != nil {
		return fmt.Errorf("failed to close named pipe handle: %w", err)
	}

	c.pipeHandle = 0
	return nil
}

func (c *Conn) cancel() {
	c.canceled = true
	c.cancelCtx()
}
