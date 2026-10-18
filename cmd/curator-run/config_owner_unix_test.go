//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package main

import (
	"os"
	"syscall"
	"testing"
)

func foreignOwnedConfigFile(t *testing.T, contents string) (string, bool) {
	t.Helper()
	parent := t.TempDir()
	path := parent + "/foreign-owned"
	if os.Geteuid() == 0 {
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			return "", false
		}
		if err := os.Chown(path, 65534, -1); err != nil {
			return "", false
		}
	} else if err := os.Link("/etc/hosts", path); err != nil {
		return "", false
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	fileStat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok || fileStat.Uid == uint32(os.Geteuid()) {
		return "", false
	}
	return path, true
}

func currentUserIsRoot() bool { return os.Geteuid() == 0 }
