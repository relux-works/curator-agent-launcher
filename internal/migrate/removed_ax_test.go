package migrate

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Dry run lists all four independent leftovers; apply preserves exact bytes in
// .bak, removes them and is idempotent without changing unrelated config files.
func TestRemoveAXAllLocations(t *testing.T) {
	l := fixture(t)
	paths := l.AXPaths()
	for i, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		put(t, filepath.Dir(path), "ax.json", strings.Repeat("x", i))
	}
	put(t, l.LegacyUser, "unrelated.json", "untouched")
	for _, dry := range []bool{true, false} {
		var out bytes.Buffer
		if err := Run(l, dry, &out, Options{CanWriteSystem: allowed}); err != nil {
			t.Fatal(err)
		}
		for i, path := range paths {
			if !strings.Contains(out.String(), "retired ax.json: "+path) || !strings.Contains(out.String(), "remove "+path) {
				t.Fatal(out.String())
			}
			if dry {
				if get(t, filepath.Dir(path), "ax.json") != strings.Repeat("x", i) {
					t.Fatal("dry run changed input")
				}
				if _, err := os.Lstat(path + ".bak"); !os.IsNotExist(err) {
					t.Fatal("dry run created backup", err)
				}
			} else {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatal("retired policy remains", err)
				}
				if get(t, filepath.Dir(path), "ax.json.bak") != strings.Repeat("x", i) {
					t.Fatal("backup mismatch")
				}
			}
		}
	}
	if get(t, l.LegacyUser, "unrelated.json") != "untouched" || get(t, l.User, "unrelated.json") != "untouched" {
		t.Fatal("unrelated copy changed")
	}
	if err := Run(l, false, &bytes.Buffer{}, Options{CanWriteSystem: allowed, WriteFile: func(string, []byte, os.FileMode) error { t.Fatal("idempotent migration wrote"); return nil }}); err != nil {
		t.Fatal(err)
	}
}

// Conflicting backups refuse before writes; an injected removal failure rolls
// back earlier removals and new backups, preserving every original policy.
func TestRemoveAXConflictAndRollback(t *testing.T) {
	for _, kind := range []string{"backup-conflict", "remove-failure"} {
		t.Run(kind, func(t *testing.T) {
			l := fixture(t)
			put(t, l.LegacySystem, "ax.json", "system")
			put(t, l.LegacyUser, "ax.json", "user")
			opts := Options{CanWriteSystem: allowed}
			if kind == "backup-conflict" {
				put(t, l.LegacyUser, "ax.json.bak", "different")
			} else {
				opts.RemoveFile = func(path string) error {
					if path == filepath.Join(l.LegacyUser, "ax.json") {
						return errors.New("planted removal failure")
					}
					return os.Remove(path)
				}
			}
			err := Run(l, false, &bytes.Buffer{}, opts)
			if kind == "backup-conflict" {
				var e *Error
				if !errors.As(err, &e) || e.Code != CodeDestinationExists {
					t.Fatal("wrong backup conflict", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "planted removal failure") {
				t.Fatal("wrong removal failure", err)
			}
			if get(t, l.LegacySystem, "ax.json") != "system" || get(t, l.LegacyUser, "ax.json") != "user" {
				t.Fatal("failed migration removed original")
			}
			if _, err := os.Lstat(filepath.Join(l.LegacySystem, "ax.json.bak")); !os.IsNotExist(err) {
				t.Fatal("failed migration left new backup", err)
			}
		})
	}
}

// Symlinks (including dangling ones) and directories are listed without opening
// their contents; apply refuses the whole transaction pending manual removal.
func TestRemoveAXNonRegular(t *testing.T) {
	for _, kind := range []string{"symlink", "dangling", "directory"} {
		t.Run(kind, func(t *testing.T) {
			l := fixture(t)
			path := filepath.Join(l.LegacyUser, "ax.json")
			target := filepath.Join(l.LegacyUser, "target")
			put(t, l.LegacyUser, "target", "preserve-target")
			if kind == "directory" {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				if kind == "dangling" {
					target += "-missing"
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			}
			put(t, l.LegacySystem, "ax.json", "regular-neighbor")
			for _, dry := range []bool{true, false} {
				var out bytes.Buffer
				err := Run(l, dry, &out, Options{CanWriteSystem: allowed})
				if dry && err != nil || !dry && (err == nil || !strings.Contains(err.Error(), "remove it manually")) {
					t.Fatal("wrong non-regular outcome", err)
				}
				if !strings.Contains(out.String(), "remove manually (non-regular file; not followed): "+path) {
					t.Fatal(out.String())
				}
				if get(t, l.LegacyUser, "target") != "preserve-target" || get(t, l.LegacySystem, "ax.json") != "regular-neighbor" {
					t.Fatal("nonregular migration modified input")
				}
				if _, err := os.Lstat(path); err != nil {
					t.Fatal("entry removed", err)
				}
			}
		})
	}
}
