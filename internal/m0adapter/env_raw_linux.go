//go:build linux

package m0adapter

import (
	"io"
	"os"
	"syscall"
)

// procSuperMagic is PROC_SUPER_MAGIC, the fstatfs type of procfs.
const procSuperMagic = 0x9fa0

// openEnviron opens path without blocking on a non-regular file and admits
// only a regular file on procfs; every other outcome refuses env_read_failed.
func openEnviron(path string) (io.ReadCloser, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, refuse(reasonEnvRead)
	}
	var fs syscall.Statfs_t
	if err := syscall.Fstatfs(fd, &fs); err != nil || fs.Type != procSuperMagic {
		syscall.Close(fd)
		return nil, refuse(reasonEnvRead)
	}
	if _, st, err := fstatID(fd); err != nil || st.Mode&syscall.S_IFMT != syscall.S_IFREG {
		syscall.Close(fd)
		return nil, refuse(reasonEnvRead)
	}
	return os.NewFile(uintptr(fd), path), nil
}
