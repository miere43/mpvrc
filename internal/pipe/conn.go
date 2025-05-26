package pipe

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
)

type Conn struct {
	pipeHandle syscall.Handle
	reads      chan<- []byte
	writes     <-chan []byte
	wg         *sync.WaitGroup
}

func Dial(name string, reads chan<- []byte, writes <-chan []byte) (*Conn, error) {
	var pipeHandle syscall.Handle
	defer func() {
		if pipeHandle != 0 {
			syscall.CloseHandle(pipeHandle)
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

	conn := &Conn{
		pipeHandle: pipeHandle,
		reads:      reads,
		writes:     writes,
		wg:         &sync.WaitGroup{},
	}

	conn.wg.Add(2)
	go conn.writePump()
	go conn.readPump()

	pipeHandle = 0 // Prevent closing the handle in the defer statement
	return conn, nil
}

func (c *Conn) writePump() {
	defer c.wg.Done()

	for write := range c.writes {
		var bytesWritten uint32
		if err := syscall.WriteFile(c.pipeHandle, write, &bytesWritten, nil); err != nil {
			panic(fmt.Sprintf("failed to write to pipe: %v", err))
		}
	}
}

func (c *Conn) readPump() {
	defer c.wg.Done()

	var buffer [4096]byte
	for {
		var bytesRead uint32
		if err := syscall.ReadFile(c.pipeHandle, buffer[:], &bytesRead, nil); err != nil {
			panic(fmt.Sprintf("failed to read from pipe: %v", err))
		}

		data := make([]byte, bytesRead)
		copy(data, buffer[:bytesRead])

		c.reads <- data
	}
}

func (c *Conn) Close() error {
	if c.pipeHandle == 0 {
		return errors.New("connection already closed")
	}

	if err := syscall.CloseHandle(c.pipeHandle); err != nil {
		return fmt.Errorf("failed to close named pipe handle: %w", err)
	}

	c.wg.Wait()

	c.pipeHandle = 0
	return nil
}
