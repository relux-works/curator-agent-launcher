//go:build unix

package doorbell

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

const settingsName = "settings.local.json"
const settingsBound = 1 << 20
const directoryFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC

type settingsTarget struct {
	directory int
	identity  unix.Stat_t
	original  os.FileInfo
	data      []byte
}

func pinSettingsDirectory(path string) (int, error) {
	fd, err := unix.Open(path, directoryFlags, 0)
	if err != nil {
		return -1, err
	}
	if _, err = checkDirectory(fd); err != nil {
		unix.Close(fd)
		return -1, err
	}
	return fd, nil
}
func checkDirectory(fd int) (unix.Stat_t, error) {
	var stat unix.Stat_t
	err := unix.Fstat(fd, &stat)
	if err != nil {
		return stat, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0022 != 0 {
		return stat, fmt.Errorf("unsafe settings directory")
	}
	return stat, nil
}
func pinSettings(workspace string) (*settingsTarget, error) {
	cwd, err := unix.Open(workspace, directoryFlags, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(cwd)
	fd, err := unix.Openat(cwd, ".claude", directoryFlags, 0)
	if err == unix.ENOENT {
		if err = unix.Mkdirat(cwd, ".claude", 0700); err != nil && err != unix.EEXIST {
			return nil, err
		}
		fd, err = unix.Openat(cwd, ".claude", directoryFlags, 0)
	}
	if err != nil {
		return nil, err
	}
	target := &settingsTarget{directory: fd}
	target.identity, err = checkDirectory(fd)
	if err == nil {
		target.data, target.original, err = readSettings(fd)
	}
	if err != nil {
		target.close()
		return nil, err
	}
	return target, nil
}
func (t *settingsTarget) close() { _ = unix.Close(t.directory) }

// All reads are descriptor-relative, no-follow, nonblocking and bounded. Missing
// entries are distinguished from empty files so commit can detect creation races.
func readSettings(directory int) ([]byte, os.FileInfo, error) {
	fd, err := unix.Openat(directory, settingsName, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err == unix.ENOENT {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(fd), settingsName)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !ownedRegular(info) {
		return nil, nil, fmt.Errorf("unsafe settings file")
	}
	data, err := io.ReadAll(io.LimitReader(file, settingsBound+1))
	if err != nil {
		return nil, nil, err
	}
	if len(data) > settingsBound {
		return nil, nil, fmt.Errorf("settings too large")
	}
	return data, info, nil
}
func (i *Installer) stageDirectory() (string, error) {
	state := envValue(i.Environment, "XDG_STATE_HOME")
	if state == "" {
		home := envValue(i.Environment, "HOME")
		if !filepath.IsAbs(home) {
			return "", fmt.Errorf("missing state home")
		}
		state = filepath.Join(home, ".local", "state")
	}
	if !filepath.IsAbs(state) {
		return "", fmt.Errorf("relative state home")
	}
	parent := filepath.Join(state, "swarma", "launcher", "doorbell")
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	return os.MkdirTemp(parent, "install-")
}
func (t *settingsTarget) commit(data []byte) error {
	name := fmt.Sprintf(".settings-%x", rand.Text())
	fd, err := unix.Openat(t.directory, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	defer unix.Unlinkat(t.directory, name, 0)
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	if _, err = file.Write(data); err != nil {
		return err
	}
	mode := uint32(0600)
	if t.original != nil {
		stat := t.original.Sys().(*syscall.Stat_t)
		mode = uint32(stat.Mode) & 07777
	}
	if err = unix.Fchmod(fd, mode); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	// This detects an entry replacement visible now; the specification accepts
	// an edit between this check and rename under its single-writer assumption.
	var current unix.Stat_t
	err = unix.Fstatat(t.directory, settingsName, &current, unix.AT_SYMLINK_NOFOLLOW)
	if t.original == nil {
		if err != unix.ENOENT {
			return fmt.Errorf("settings appeared")
		}
	} else {
		old := t.original.Sys().(*syscall.Stat_t)
		if err != nil || uint64(current.Dev) != uint64(old.Dev) || current.Ino != old.Ino {
			return fmt.Errorf("settings replaced")
		}
	}
	return unix.Renameat(t.directory, name, t.directory, settingsName)
}
func (t *settingsTarget) confirm(workspace string) bool {
	cwd, err := unix.Open(workspace, directoryFlags, 0)
	if err != nil {
		return false
	}
	defer unix.Close(cwd)
	fd, err := unix.Openat(cwd, ".claude", directoryFlags, 0)
	if err != nil {
		return false
	}
	defer unix.Close(fd)
	stat, err := checkDirectory(fd)
	return err == nil && stat.Dev == t.identity.Dev && stat.Ino == t.identity.Ino
}
