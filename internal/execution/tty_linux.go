//go:build linux

package execution

import (
	"syscall"
	"unsafe"
)

func isTerminalFD(fd uintptr) bool {
	var state syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&state)))
	return errno == 0
}
