//go:build !windows

package selector

import (
	"errors"
	"io"
	"os"
	"syscall"
	"time"
)

func openRegular(path string, flags int) (*os.File, error) {
	f, err := os.OpenFile(path, flags|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("observation path is not regular")
	}
	return f, nil
}
func openAppend(path string) (*os.File, error) {
	return openRegular(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND)
}
func lockAppend(f *os.File, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if (!errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN)) || !time.Now().Before(deadline) {
			return err
		}
		time.Sleep(min(10*time.Millisecond, time.Until(deadline)))
	}
}
func appendLine(f *os.File, data []byte, wait time.Duration) error {
	if err := lockAppend(f, wait); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	n, err := f.Write(data)
	if err == nil && n != len(data) {
		return io.ErrShortWrite
	}
	return err
}
