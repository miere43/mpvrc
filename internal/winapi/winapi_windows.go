package winapi

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	PIPE_READMODE_MESSAGE = 0x00000002
)

var kernel32 *syscall.LazyDLL
var createEvent *syscall.LazyProc
var getOverlappedResult *syscall.LazyProc

func init() {
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	createEvent = kernel32.NewProc("CreateEventW")
	getOverlappedResult = kernel32.NewProc("GetOverlappedResult")
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
