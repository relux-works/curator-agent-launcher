//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package sessionmanager

import (
	"os"
	"path/filepath"
	"syscall"
)

func writableDirectory(path string) bool {
	for {
		info, err := os.Stat(path)
		if err == nil {
			return info.IsDir() && syscall.Access(path, 2) == nil
		}
		if !os.IsNotExist(err) {
			return false
		}
		parent := filepath.Dir(path)
		if parent == path {
			return false
		}
		path = parent
	}
}
