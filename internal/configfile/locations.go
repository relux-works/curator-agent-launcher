package configfile

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// Locations is the shared launcher configuration home, captured from process
// inputs before the fragment modifies the child environment. Empty legacy
// directories disable fallback for explicit callers and isolated tests.
type Locations struct {
	System, User             string
	LegacySystem, LegacyUser string
}

// ConfigLocations does not inspect or create directories. Windows retains the
// existing platform path construction and file-security policy.
func ConfigLocations(xdgConfigHome, userHome string) Locations {
	if xdgConfigHome == "" {
		xdgConfigHome = filepath.Join(userHome, ".config")
	}
	return Locations{
		System:       filepath.Join("/etc", "swarma", "launcher"),
		User:         filepath.Join(xdgConfigHome, "swarma", "launcher"),
		LegacySystem: filepath.Join("/etc", "curator-run"),
		LegacyUser:   filepath.Join(xdgConfigHome, "curator-run"),
	}
}

type Layer bool

const (
	System Layer = false
	User   Layer = true
)

func (l Locations) Directories(layer Layer) (current, legacy string) {
	if layer == User {
		return l.User, l.LegacyUser
	}
	return l.System, l.LegacySystem
}

// Resolver applies per-file, per-layer fallback only after genuine absence.
// Every selected file goes through Read's security checks; a present current
// file wins without even inspecting its legacy counterpart.
type Resolver struct {
	Locations Locations
	Stderr    io.Writer
}

func (r Resolver) Read(layer Layer, name string) (data []byte, present bool, path string, err error) {
	current, legacy := r.Locations.Directories(layer)
	path = filepath.Join(current, name)
	data, present, err = Read(path)
	if err != nil || present || legacy == "" {
		return
	}
	old := filepath.Join(legacy, name)
	data, present, err = Read(old)
	if present || err != nil {
		path = old
		if r.Stderr != nil {
			// Preserve one physical warning line even for unusual directory names.
			linePath := func(p string) string { return strings.NewReplacer("\r", `\r`, "\n", `\n`).Replace(p) }
			_, warningErr := fmt.Fprintf(r.Stderr, "curator-run: deprecated config path %s; move it to %s (curator-run migrate)\n", linePath(old), linePath(filepath.Join(current, name)))
			if err == nil && warningErr != nil {
				err = warningErr
			}
		}
	}
	return
}
