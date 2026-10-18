//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package sessionmanager

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

func openRegistryLock(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	fail := func(err error) (*os.File, error) { f.Close(); return nil, err }
	var info, dir unix.Stat_t
	if err = unix.Fstat(fd, &info); err != nil {
		return fail(err)
	}
	if err = unix.Stat(filepath.Dir(path), &dir); err != nil {
		return fail(err)
	}
	if info.Mode&unix.S_IFMT != unix.S_IFREG || info.Mode&0022 != 0 || info.Uid != dir.Uid {
		return fail(fmt.Errorf("unsafe registry lock file"))
	}
	return f, nil
}
func tryRegistryLock(f *os.File) (bool, error) {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == unix.EWOULDBLOCK || err == unix.EAGAIN || err == unix.EINTR {
		return false, nil
	}
	return err == nil, err
}

// Poll the terminal without a reader goroutine, descriptor flag changes, or
// closing the caller's stdin. A partial/continuously arriving line also expires.
func readPromptFile(f *os.File, timeout time.Duration) (string, error) {
	raw, err := f.SyscallConn()
	if err != nil {
		return "", err
	}
	var answer string
	var readErr error
	if err = raw.Control(func(fd uintptr) { answer, readErr = readPromptFD(int(fd), timeout) }); err != nil {
		return "", err
	}
	return answer, readErr
}
func readPromptFD(fd int, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var answer []byte
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return "", os.ErrDeadlineExceeded
		}
		poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		_, err := unix.Poll(poll, int((remaining+time.Millisecond-1)/time.Millisecond))
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return "", err
		}
		if poll[0].Revents == 0 {
			continue
		}
		if poll[0].Revents&unix.POLLNVAL != 0 {
			return "", os.ErrInvalid
		}
		var b [1]byte
		n, err := unix.Read(fd, b[:])
		if err == unix.EINTR || err == unix.EAGAIN {
			continue
		}
		if err != nil {
			return "", err
		}
		if n == 0 {
			return string(answer), io.EOF
		}
		answer = append(answer, b[0])
		if b[0] == '\n' {
			return string(answer), nil
		}
	}
}
