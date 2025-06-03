package pipe

import (
	"errors"
	"fmt"
	"log"
	"syscall"

	"github.com/miere43/mpvrc/internal/winapi"
)

type Server struct {
	name          string
	pipeHandle    syscall.Handle
	newClientFunc func(client *ConnectedClient)
}

func NewServer(name string, newClientFunc func(client *ConnectedClient)) (*Server, error) {
	s := &Server{
		name:          name,
		newClientFunc: newClientFunc,
	}

	pipeHandle, err := s.createPipe(true)
	if err != nil {
		return nil, err
	}
	s.pipeHandle = pipeHandle
	return s, nil
}

func (s *Server) createPipe(first bool) (syscall.Handle, error) {
	var pipeMode uint32 = winapi.PIPE_ACCESS_INBOUND
	if first {
		pipeMode |= winapi.FILE_FLAG_FIRST_PIPE_INSTANCE
	}

	pipeHandle, err := winapi.CreateNamedPipe(
		s.name,
		pipeMode,
		winapi.PIPE_TYPE_MESSAGE|winapi.PIPE_READMODE_MESSAGE|winapi.PIPE_REJECT_REMOTE_CLIENTS,
		8,
		4096,
		4096,
		0,
		nil,
	)
	if err != nil {
		return syscall.Handle(0), fmt.Errorf("failed to create named pipe %q: %w", s.name, err)
	}
	return pipeHandle, nil
}

func (s *Server) Serve() {
	for {
		client, err := s.connectClient()
		if err != nil {
			log.Fatalf("pipe server failed to connect client: %v", err)
		}

		go func(client *ConnectedClient) {
			s.newClientFunc(client)

			syscall.CloseHandle(client.pipeHandle)
			client.pipeHandle = 0
		}(client)
	}
}

type ConnectedClient struct {
	pipeHandle syscall.Handle
}

func (s *Server) connectClient() (*ConnectedClient, error) {
	var clientPipeHandle = s.pipeHandle
	if err := winapi.ConnectNamedPipe(clientPipeHandle, nil); err != nil && !errors.Is(err, winapi.ERROR_PIPE_CONNECTED) {
		return nil, fmt.Errorf("wait for client failed: %v", err)
	}

	log.Printf("received new named pipe client")

	s.pipeHandle = 0
	newPipeHandle, err := s.createPipe(false)
	if err != nil {
		return nil, fmt.Errorf("failed to create pipe for next client connection: %v", err)
	}
	s.pipeHandle = newPipeHandle

	return &ConnectedClient{
		clientPipeHandle,
	}, nil
}

func (s *ConnectedClient) ReadMessage() ([]byte, error) {
	var message []byte
	var buffer [4096]byte
	var bytesRead uint32

	for {
		if err := syscall.ReadFile(s.pipeHandle, buffer[:], &bytesRead, nil); err != nil {
			log.Printf("read file %d", bytesRead)
			if errors.Is(err, syscall.ERROR_MORE_DATA) {
				message = append(message, buffer[:bytesRead]...)
				continue
			}
			return nil, fmt.Errorf("failed to read from pipe: %w", err)
		}
		message = append(message, buffer[:bytesRead]...)
		return message, nil
	}
}
