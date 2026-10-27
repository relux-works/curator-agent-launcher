package configfile

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func resolverFixture(t *testing.T) Resolver {
	t.Helper()
	root := t.TempDir()
	l := Locations{System: filepath.Join(root, "system"), User: filepath.Join(root, "user"), LegacySystem: filepath.Join(root, "old-system"), LegacyUser: filepath.Join(root, "old-user")}
	for _, d := range []string{l.System, l.User, l.LegacySystem, l.LegacyUser} {
		if err := os.Mkdir(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return Resolver{Locations: l}
}
func file(t *testing.T, p, data string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

// Every filename and layer independently prefers the new home and emits exactly
// one warning on a fallback read; a hostile old path is never opened when new exists.
func TestResolverFallbackPerFileAndLayer(t *testing.T) {
	for _, layer := range []Layer{System, User} {
		for _, name := range []string{"defaults.json", "launcher.json", "registry.json", "credentials.json"} {
			t.Run(filepath.Join(map[Layer]string{System: "system", User: "user"}[layer], name), func(t *testing.T) {
				r := resolverFixture(t)
				var warning bytes.Buffer
				r.Stderr = &warning
				current, old := r.Locations.Directories(layer)
				np, op := filepath.Join(current, name), filepath.Join(old, name)
				b, p, path, err := r.Read(layer, name)
				if err != nil || p || len(b) != 0 || path != np || warning.Len() != 0 {
					t.Fatalf("absent %q %v %s %v %s", b, p, path, err, &warning)
				}
				file(t, op, "old")
				b, p, path, err = r.Read(layer, name)
				want := "curator-run: deprecated config path " + op + "; move it to " + np + " (curator-run migrate)\n"
				if err != nil || !p || string(b) != "old" || path != op || warning.String() != want {
					t.Fatalf("fallback %q %v %s %v %s", b, p, path, err, &warning)
				}
				warning.Reset()
				file(t, np, "new")
				os.Remove(op)
				// A final-component dangling link would fail Read if inspected.
				if err := os.Symlink(filepath.Join(old, "missing"), op); err != nil {
					t.Fatal(err)
				}
				b, p, path, err = r.Read(layer, name)
				if err != nil || !p || string(b) != "new" || path != np || warning.Len() != 0 {
					t.Fatalf("new %q %v %s %v %s", b, p, path, err, &warning)
				}
			})
		}
	}
}

// Both old and new selected files reject symlinks and writable-by-others modes;
// an invalid new file must never silently downgrade to a valid legacy copy.
func TestResolverSecurityBothHomes(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, kind := range []string{"symlink", "group-write", "world-write"} {
			t.Run(map[bool]string{false: "new", true: "old"}[legacy]+"/"+kind, func(t *testing.T) {
				if runtime.GOOS == "windows" && kind != "symlink" {
					t.Skip("POSIX mode gate; Windows uses DACLs")
				}
				r := resolverFixture(t)
				dir, old := r.Locations.Directories(User)
				path := filepath.Join(dir, "defaults.json")
				if legacy {
					path = filepath.Join(old, "defaults.json")
				} else {
					file(t, filepath.Join(old, "defaults.json"), "valid-old")
				}
				file(t, path, "valid")
				b, p, _, err := r.Read(User, "defaults.json")
				if err != nil || !p || string(b) != "valid" {
					t.Fatal("positive", err)
				}
				want := "group/world-writable"
				if kind == "symlink" {
					os.Remove(path)
					target := filepath.Join(dir, "target")
					file(t, target, "valid")
					if err := os.Symlink(target, path); err != nil {
						t.Fatal(err)
					}
					want = "symlinked"
				} else {
					mode := os.FileMode(0620)
					if kind == "world-write" {
						mode = 0602
					}
					if err := os.Chmod(path, mode); err != nil {
						t.Fatal(err)
					}
				}
				_, _, selected, err := r.Read(User, "defaults.json")
				if err == nil || !strings.Contains(err.Error(), want) || selected != path {
					t.Fatalf("%s %v", selected, err)
				}
			})
		}
	}
}

// The resolver constructs the new and one-release legacy families from explicit
// XDG inputs; it never uses a child's managed HOME to select process config.
func TestConfigLocations(t *testing.T) {
	for _, xdg := range []string{"", "/config"} {
		l := ConfigLocations(xdg, "/home")
		base := xdg
		if base == "" {
			base = "/home/.config"
		}
		if l.System != filepath.Join("/etc", "swarma", "launcher") || l.User != filepath.Join(base, "swarma", "launcher") || l.LegacySystem != filepath.Join("/etc", "curator-run") || l.LegacyUser != filepath.Join(base, "curator-run") {
			t.Fatalf("%+v", l)
		}
	}
}
