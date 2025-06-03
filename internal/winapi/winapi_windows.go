package winapi

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	PIPE_READMODE_MESSAGE         = 0x00000002
	PIPE_TYPE_MESSAGE             = 0x00000004
	PIPE_REJECT_REMOTE_CLIENTS    = 0x00000008
	PIPE_ACCESS_INBOUND           = 0x00000001
	FILE_FLAG_FIRST_PIPE_INSTANCE = 0x00080000

	ERROR_PIPE_BUSY      syscall.Errno = 231
	ERROR_PIPE_CONNECTED syscall.Errno = 535
)

var kernel32 *syscall.LazyDLL
var createEvent *syscall.LazyProc
var getOverlappedResult *syscall.LazyProc
var createNamedPipe *syscall.LazyProc
var connectNamedPipe *syscall.LazyProc

func init() {
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	createEvent = kernel32.NewProc("CreateEventW")
	getOverlappedResult = kernel32.NewProc("GetOverlappedResult")
	createNamedPipe = kernel32.NewProc("CreateNamedPipeW")
	connectNamedPipe = kernel32.NewProc("ConnectNamedPipe")
}

func CreateEventW(lpEventAttributes uintptr, bManualReset, bInitialState bool) (syscall.Handle, error) {
	reset := uintptr(0)
	if bManualReset {
		reset = 1
	}
	initialState := uintptr(0)
	if bInitialState {
		initialState = 1
	}
	handle, _, err := createEvent.Call(
		uintptr(lpEventAttributes), // lpEventAttributes
		reset,
		initialState,
		0, // lpName
	)

	if handle == 0 {
		return 0, fmt.Errorf("CreateEvent failed: %w", err)
	}

	return syscall.Handle(handle), nil
}

func GetOverlappedResult(handle syscall.Handle, overlapped *syscall.Overlapped, wait bool) (uint32, error) {
	var bytesTransferred uint32
	var waitFlag uintptr = 0
	if wait {
		waitFlag = 1
	}

	ret, _, err := getOverlappedResult.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(overlapped)),
		uintptr(unsafe.Pointer(&bytesTransferred)),
		waitFlag,
	)

	if ret == 0 {
		return 0, fmt.Errorf("GetOverlappedResult failed: %w", err)
	}

	return bytesTransferred, nil
}

func CreateNamedPipe(
	name string,
	openMode uint32,
	pipeMode uint32,
	maxInstances uint32,
	outBufferSize uint32,
	inBufferSize uint32,
	defaultTimeout uint32,
	securityAttributes *syscall.SecurityAttributes,
) (syscall.Handle, error) {
	name16, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}

	handle, _, callErr := createNamedPipe.Call(
		uintptr(unsafe.Pointer(name16)),
		uintptr(openMode),
		uintptr(pipeMode),
		uintptr(maxInstances),
		uintptr(outBufferSize),
		uintptr(inBufferSize),
		uintptr(defaultTimeout),
		uintptr(unsafe.Pointer(securityAttributes)),
	)
	if handle == 0 || handle == uintptr(syscall.InvalidHandle) {
		return 0, fmt.Errorf("CreateNamedPipeW failed: %w", callErr)
	}
	return syscall.Handle(handle), nil
}

func ConnectNamedPipe(handle syscall.Handle, overlapped *syscall.Overlapped) error {
	ret, _, err := connectNamedPipe.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(overlapped)),
	)
	if ret == 0 {
		return fmt.Errorf("ConnectNamedPipe failed: %w", err)
	}
	return nil
}
