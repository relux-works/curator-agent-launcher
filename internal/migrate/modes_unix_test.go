//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package migrate

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Newly created system policy is readable by ordinary launcher users, while
// user policy remains private; neither layer permits group/world writes.
func TestMigrationLayerModes(t *testing.T) {
	l := fixture(t)
	put(t, l.LegacySystem, "defaults.json", legacy)
	put(t, l.LegacyUser, "defaults.json", legacy)
	if err := Run(l, false, &bytes.Buffer{}, Options{CanWriteSystem: allowed}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		path string
		mode os.FileMode
	}{{l.System, 0755}, {l.User, 0700}, {filepath.Join(l.System, "defaults.json"), 0644}, {filepath.Join(l.System, "launcher.json"), 0644}, {filepath.Join(l.User, "defaults.json"), 0600}} {
		info, err := os.Stat(row.path)
		if err != nil || info.Mode().Perm() != row.mode {
			t.Fatalf("%s: %v, want mode %04o", row.path, err, row.mode)
		}
	}
}
