//go:build unix

package hosted

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// MarkCloseOnExec sets the close-on-exec flag on one launcher-side
// descriptor (r6 §4). Go normally sets it at open time, but the mark
// is explicit here so the launcher — not the runtime — owns the
// contract obligation, and ValidateTerminal verifies it.
func MarkCloseOnExec(f *os.File) {
	if f == nil {
		return
	}
	unix.CloseOnExec(int(f.Fd()))
}

// ValidateTerminal checks the fd3 terminal before contact (r6 §4):
// an open character device (production /dev/tty) or regular file
// (test double), opened read-write, owned by the caller or root,
// and marked close-on-exec on the launcher side. Directories,
// pipes, sockets, read-only files, foreign-owned files, and
// unmarked descriptors refuse. Details carry the failure class,
// never paths or descriptor numbers.
func ValidateTerminal(f *os.File) error {
	if f == nil {
		return errors.New("terminal descriptor is missing")
	}
	st, err := f.Stat()
	if err != nil {
		return errors.New("terminal descriptor is not statable")
	}
	if st.IsDir() {
		return errors.New("terminal descriptor is a directory")
	}
	mode := st.Mode()
	if mode&os.ModeCharDevice == 0 && !mode.IsRegular() {
		return errors.New("terminal descriptor is not a terminal or file")
	}
	flags, err := unix.FcntlInt(f.Fd(), unix.F_GETFL, 0)
	if err != nil {
		return errors.New("terminal descriptor access is unknown")
	}
	if flags&unix.O_ACCMODE != unix.O_RDWR {
		return errors.New("terminal descriptor is not read-write")
	}
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		euid := uint32(os.Geteuid())
		if s.Uid != euid && s.Uid != 0 {
			return errors.New("terminal descriptor ownership is foreign")
		}
	}
	fdflags, err := unix.FcntlInt(f.Fd(), unix.F_GETFD, 0)
	if err != nil {
		return errors.New("terminal descriptor flags are unknown")
	}
	if fdflags&unix.FD_CLOEXEC == 0 {
		return errors.New("terminal descriptor is not close-on-exec")
	}
	return nil
}

// validateStatusPipe checks the fd4 status-pipe write end before
// contact: an open pipe, opened write-only, owned by the caller
// (the launcher created it), and marked close-on-exec.
func validateStatusPipe(f *os.File) error {
	if f == nil {
		return errors.New("status descriptor is missing")
	}
	st, err := f.Stat()
	if err != nil {
		return errors.New("status descriptor is not statable")
	}
	if st.Mode()&os.ModeNamedPipe == 0 {
		return errors.New("status descriptor is not a pipe")
	}
	flags, err := unix.FcntlInt(f.Fd(), unix.F_GETFL, 0)
	if err != nil {
		return errors.New("status descriptor access is unknown")
	}
	if flags&unix.O_ACCMODE != unix.O_WRONLY {
		return errors.New("status descriptor is not write-only")
	}
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		if s.Uid != uint32(os.Geteuid()) {
			return errors.New("status descriptor ownership is foreign")
		}
	}
	fdflags, err := unix.FcntlInt(f.Fd(), unix.F_GETFD, 0)
	if err != nil {
		return errors.New("status descriptor flags are unknown")
	}
	if fdflags&unix.FD_CLOEXEC == 0 {
		return errors.New("status descriptor is not close-on-exec")
	}
	return nil
}
