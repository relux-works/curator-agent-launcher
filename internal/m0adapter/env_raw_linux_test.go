//go:build linux

package m0adapter

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestOpenEnvironProcfsRegularOnly proves the real environ on procfs opens, beside env_read_failed for a non-procfs regular file and a non-regular procfs path.
func TestOpenEnvironProcfsRegularOnly(t *testing.T) {
	positive := func(t *testing.T) {
		t.Helper()
		f, err := openEnviron("/proc/self/environ")
		if err != nil {
			t.Fatalf("procfs environ refused: %v", err)
		}
		if _, err := io.ReadAll(f); err != nil {
			t.Fatalf("procfs environ unreadable: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("procfs environ close: %v", err)
		}
	}
	nonProc := filepath.Join(t.TempDir(), "environ")
	if err := os.WriteFile(nonProc, []byte("HOME=/example\x00"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, path string }{
		{"non-procfs", nonProc},
		{"non-regular", "/proc/self"},
	} {
		t.Run(c.name, func(t *testing.T) {
			positive(t)
			f, err := openEnviron(c.path)
			if f != nil {
				f.Close()
				t.Fatal("non-procfs or non-regular environ admitted")
			}
			requireReason(t, err, reasonEnvRead)
		})
	}
}
