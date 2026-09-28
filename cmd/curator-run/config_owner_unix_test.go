//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package main

import (
	"os"
	"syscall"
	"testing"
)

func foreignOwnedConfigFile(t *testing.T, contents string) (string, bool) {
	t.Helper()
	const parent = "/tmp"
	parentInfo, err := os.Stat(parent)
	if err != nil {
		return "", false
	}
	parentStat, ok := parentInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return "", false
	}
	f, err := os.CreateTemp(parent, "curator-run-foreign-owner-")
	if err != nil {
		return "", false
	}
	path := f.Name()
	if _, err := f.WriteString(contents); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", false
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", false
	}
	if os.Geteuid() == 0 && parentStat.Uid == 0 {
		if err := os.Chown(path, 65534, -1); err != nil {
			_ = os.Remove(path)
			return "", false
		}
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		_ = os.Remove(path)
		return "", false
	}
	fileStat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok || fileStat.Uid == parentStat.Uid {
		_ = os.Remove(path)
		return "", false
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	return path, true
}

func currentUserIsRoot() bool { return os.Geteuid() == 0 }
