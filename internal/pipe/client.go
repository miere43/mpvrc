package pipe

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"syscall"
	"time"

	"github.com/miere43/mpvrc/internal/util"
	"github.com/miere43/mpvrc/internal/winapi"
)

type Client struct {
	pipeHandle syscall.Handle
	reads      chan<- []byte
	writes     chan []byte
	ctx        context.Context
	cancelCtx  context.CancelFunc
	canceled   bool
	wg         *sync.WaitGroup
}

func Dial(name string, timeout time.Duration, reads chan<- []byte) (*Client, error) {
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

	var access uint32 = syscall.GENERIC_WRITE
	if reads != nil {
		access |= syscall.GENERIC_READ
	}

	maxInstant := time.Now().UTC().Add(timeout)

	for {
		pipeHandle, err = syscall.CreateFile(
			name16,
			access,
			0,
			nil,
			syscall.OPEN_EXISTING,
			syscall.FILE_ATTRIBUTE_NORMAL|syscall.FILE_FLAG_OVERLAPPED,
			0,
		)
		if err == nil {
			break
		} else if !(errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) || errors.Is(err, winapi.ERROR_PIPE_BUSY)) {
			return nil, fmt.Errorf("failed to connect to named pipe: %w", err)
		} else if time.Now().UTC().After(maxInstant) {
			return nil, fmt.Errorf("timed out while waiting for named pipe to become available: %w", err)
		}

		time.Sleep(10 * time.Millisecond)
	}

	ctx, cancel := context.WithCancel(context.Background())
	client := &Client{
		pipeHandle: pipeHandle,
		reads:      reads,
		writes:     writes,
		ctx:        ctx,
		cancelCtx:  cancel,
		wg:         &sync.WaitGroup{},
	}

	if reads != nil {
		client.wg.Add(1)
		go client.readFromPipe(ctx)
	}

	client.wg.Add(1)
	go client.writeToPipe(ctx)

	pipeHandle = 0 // Prevent closing the handle in the defer statement
	return client, nil
}

func (c *Client) Context() context.Context {
	return c.ctx
}

func (c *Client) readFromPipe(ctx context.Context) {
	defer c.wg.Done()

	event, err := winapi.CreateEventW(0, false, false)
	if err != nil {
		util.Fatal("failed to create event", "err", err)
	}
	defer syscall.CloseHandle(event)

	overlapped := &syscall.Overlapped{
		HEvent: event,
	}
	overlappedDone := make(chan struct{})

	var buffer [1024 * 8]byte
	for {
		if err = syscall.ReadFile(c.pipeHandle, buffer[:], nil, overlapped); err != nil {
			if !errors.Is(err, syscall.ERROR_IO_PENDING) {
				slog.Error("failed to read from pipe", "err", err)
				c.cancel()
				return
			}
		}

		go func() {
			bytesRead, err := winapi.GetOverlappedResult(c.pipeHandle, overlapped, true)
			if err != nil {
				slog.Error("failed to get overlapped result", "err", err)
				c.cancel()
				return
			}

			responseJSON := make([]byte, int(bytesRead))
			copy(responseJSON, buffer[:bytesRead])

			slog.Debug("Read operation completed", "bytesRead", bytesRead, "result", string(responseJSON))

			c.reads <- responseJSON
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

func (c *Client) writeToPipe(ctx context.Context) {
	defer c.wg.Done()

	event, err := winapi.CreateEventW(0, false, false)
	if err != nil {
		util.Fatal("failed to create event", "err", err)
	}
	defer syscall.CloseHandle(event)

	overlapped := &syscall.Overlapped{
		HEvent: event,
	}
	overlappedDone := make(chan struct{})

	for {
		select {
		case write := <-c.writes:
			slog.Debug("Preparing to write to pipe", "writeBytes", len(write), "data", string(write))
			if err = syscall.WriteFile(c.pipeHandle, write, nil, overlapped); err != nil {
				util.Fatal("failed to write to pipe", "err", err)
			}

			slog.Debug("Write operation initiated, waiting for completion...")

			var bytesWritten uint32
			go func() {
				bytesWritten, err = winapi.GetOverlappedResult(c.pipeHandle, overlapped, true)
				if err != nil {
					util.Fatal("failed to get overlapped result", "err", err)
				}
				slog.Debug("Write operation completed", "bytesWritten", bytesWritten)
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

func (c *Client) Write(data []byte) error {
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

func (c *Client) Close() error {
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

func (c *Client) cancel() {
	c.canceled = true
	c.cancelCtx()
}
