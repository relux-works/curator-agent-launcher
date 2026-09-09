// Package axconfig reads SPEC §4.6 machine-first tracking policy without writes.
package axconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

const CodeInvalid = "defaults_config_invalid"

type Error struct {
	Path string
	Err  error
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s: %v", CodeInvalid, e.Path, e.Err) }
func (e *Error) Unwrap() error { return e.Err }

// Load takes explicit configuration directories. Call before cli.Parse and pass
// the returned fact as cli.Options.AxConfigured. Machine policy wins even when
// false: the ignored operator path is never inspected. Missing directories are
// absence; dangling symlinks, non-directory ancestors and failed reads are not.
func Load(machineDir, operatorDir string) (bool, error) {
	for _, dir := range []string{machineDir, operatorDir} {
		path := filepath.Join(dir, "ax.json")
		present, err := exists(path)
		if err != nil {
			return false, &Error{path, err}
		}
		if !present {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return false, &Error{path, err}
		}
		enabled, err := parse(data)
		if err != nil {
			return false, &Error{path, err}
		}
		return enabled, nil
	}
	return false, nil
}

func exists(path string) (bool, error) {
	// Walk ancestors first so ENOENT through a dangling link cannot become absence.
	parent := filepath.Dir(path)
	if parent != path {
		present, err := directory(parent)
		if err != nil || !present {
			return false, err
		}
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		info, err = os.Stat(path)
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("not a regular file")
	}
	return true, nil
}

func directory(path string) (bool, error) {
	parent := filepath.Dir(path)
	if parent != path {
		present, err := directory(parent)
		if err != nil || !present {
			return false, err
		}
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		info, err = os.Stat(path)
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("ancestor %s is not a directory", path)
	}
	return true, nil
}

func parse(data []byte) (bool, error) {
	v, err := fragment.ParseJSON(data)
	if err != nil {
		return false, err
	}
	if v.Kind != fragment.KindObject || len(v.Obj) != 2 {
		return false, fmt.Errorf("expected exactly schema and enabled")
	}
	schema, _ := v.Get("schema")
	enabled, _ := v.Get("enabled")
	if schema.Kind != fragment.KindString || schema.Str != "curator-run-ax-v1" {
		return false, fmt.Errorf("invalid schema")
	}
	if enabled.Kind != fragment.KindBool {
		return false, fmt.Errorf("enabled must be a boolean")
	}
	return enabled.Bool, nil
}
