//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package configfile

import (
	"fmt"
	"os"
	"syscall"
)

func openFile(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		if err == syscall.ELOOP {
			return nil, errSymlink
		}
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func validateFile(_ string, _ *os.File, info, dirInfo os.FileInfo) error {
	fileStat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot establish configuration file owner")
	}
	dirStat, ok := dirInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot establish configuration directory owner")
	}
	if fileStat.Uid != dirStat.Uid {
		return fmt.Errorf("configuration file is owned by a different identity than its configuration directory")
	}
	if mode := info.Mode().Perm(); mode&0o022 != 0 {
		return fmt.Errorf("group/world-writable configuration file (mode %04o)", mode)
	}
	return nil
}
