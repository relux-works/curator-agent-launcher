// Package configfile securely reads launcher-owned configuration files.
package configfile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var errSymlink = errors.New("symlinked configuration file")

// Read returns a file's complete contents and whether it was present. Only a
// genuinely absent file or directory is optional. Existing unreadable, linked,
// non-regular, foreign-owned, or writable-by-others files are refusals.
func Read(path string) ([]byte, bool, error) {
	if path == "" {
		return nil, false, fmt.Errorf("empty configuration path")
	}

	dirInfo, present, err := inspectDirectory(filepath.Dir(path))
	if err != nil {
		return nil, false, err
	}
	if !present {
		return nil, false, nil
	}

	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("cannot inspect configuration file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, true, errSymlink
	}
	if !info.Mode().IsRegular() {
		return nil, true, fmt.Errorf("configuration file is not a regular file")
	}

	f, err := openFile(path)
	if err != nil {
		if errors.Is(err, errSymlink) {
			return nil, true, errSymlink
		}
		return nil, true, fmt.Errorf("configuration file is unreadable: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
	}()

	info, err = f.Stat()
	if err != nil {
		return nil, true, fmt.Errorf("cannot inspect configuration file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, true, fmt.Errorf("configuration file is not a regular file")
	}
	if err := validateFile(path, f, info, dirInfo); err != nil {
		return nil, true, err
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, true, fmt.Errorf("configuration file is unreadable: %w", err)
	}
	closeErr := f.Close()
	closed = true
	if closeErr != nil {
		return nil, true, fmt.Errorf("configuration file is unreadable: %w", closeErr)
	}
	return data, true, nil
}

// inspectDirectory permits a valid directory symlink (for example macOS
// /etc), but a dangling link or a non-directory ancestor is never absence.
func inspectDirectory(path string) (os.FileInfo, bool, error) {
	parent := filepath.Dir(path)
	if parent != path {
		_, present, err := inspectDirectory(parent)
		if err != nil || !present {
			return nil, present, err
		}
	}

	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("cannot inspect configuration directory %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		info, err = os.Stat(path)
		if err != nil {
			return nil, false, fmt.Errorf("unreadable symlink ancestor %q: %v", path, err)
		}
	}
	if !info.IsDir() {
		return nil, false, fmt.Errorf("configuration ancestor %q is not a directory", path)
	}
	return info, true, nil
}
