package sessionmanager

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
)

type atomicHooks struct {
	beforeRename  func() error
	syncDirectory func(string) error
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	return atomicWriteWithHooks(path, data, mode, atomicHooks{})
}
func atomicWriteWithHooks(path string, data []byte, mode os.FileMode, hooks atomicHooks) error {
	before, present, err := configfile.Read(path)
	if err != nil {
		return err
	}
	oldMode := mode
	if present {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		oldMode = info.Mode().Perm()
	}
	dir := filepath.Dir(path)
	dirMode := os.FileMode(0700)
	if mode == 0644 {
		dirMode = 0755
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	scratch, err := prepareFile(dir, data, mode)
	if err != nil {
		return err
	}
	defer os.Remove(scratch)
	if hooks.beforeRename != nil {
		if err := hooks.beforeRename(); err != nil {
			return err
		}
	}
	now, exists, err := configfile.Read(path)
	if err != nil {
		return err
	}
	if exists != present || !bytes.Equal(now, before) {
		return fmt.Errorf("registry changed during atomic write")
	}
	if err := os.Rename(scratch, path); err != nil {
		return err
	}
	sync := hooks.syncDirectory
	if sync == nil {
		sync = syncDirectory
	}
	if err := sync(dir); err != nil {
		// A post-rename durability failure is still a failed command: restore the
		// old visible file with an independent writer before returning the error.
		var restoreErr error
		if present {
			var rollback string
			rollback, restoreErr = prepareFile(dir, before, oldMode)
			if restoreErr == nil {
				restoreErr = os.Rename(rollback, path)
				os.Remove(rollback)
			}
		} else {
			restoreErr = os.Remove(path)
		}
		return errors.Join(err, restoreErr, syncDirectory(dir))
	}
	return nil
}
func prepareFile(dir string, data []byte, mode os.FileMode) (string, error) {
	f, err := os.CreateTemp(dir, ".session-managers-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(name)
		}
	}()
	if err = f.Chmod(mode); err != nil {
		return "", err
	}
	if _, err = f.Write(data); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	ok = true
	return name, nil
}
func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
