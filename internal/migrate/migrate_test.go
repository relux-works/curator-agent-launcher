package migrate

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
)

func fixture(t *testing.T) configfile.Locations {
	t.Helper()
	root := t.TempDir()
	l := configfile.Locations{System: filepath.Join(root, "system"), User: filepath.Join(root, "user"), LegacySystem: filepath.Join(root, "old-system"), LegacyUser: filepath.Join(root, "old-user")}
	for _, d := range []string{l.LegacySystem, l.LegacyUser} {
		if err := os.Mkdir(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return l
}
func put(t *testing.T, dir, name, data string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}
func get(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const legacy = `{"schema":"curator-run-defaults-v3","defaults":{"pi":{"host":"native","model":"m"},"claude_code":{"host":"native"}}}`

func allowed(string) bool { return true }

// Copying strips every host, removes host-only entries, saves original bytes,
// preserves old sources, and has an entirely write-free second run.
func TestCopyStripAndIdempotence(t *testing.T) {
	l := fixture(t)
	put(t, l.LegacySystem, "defaults.json", legacy)
	put(t, l.LegacyUser, "defaults.json", legacy)
	put(t, l.LegacyUser, "ax.json", `{"schema":"curator-run-ax-v1","enabled":true}`)
	var out bytes.Buffer
	if err := Run(l, false, &out, Options{CanWriteSystem: allowed}); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{l.System, l.User} {
		s := get(t, dir, "defaults.json")
		if strings.Contains(s, `"host"`) || strings.Contains(s, "claude_code") || !strings.Contains(s, `"model": "m"`) {
			t.Fatal(s)
		}
		if get(t, dir, "defaults.json.bak") != legacy {
			t.Fatal("backup lost original bytes")
		}
	}
	if get(t, l.LegacyUser, "defaults.json") != legacy || get(t, l.LegacySystem, "defaults.json") != legacy {
		t.Fatal("source changed")
	}
	if !strings.Contains(get(t, l.System, "launcher.json"), `"host_mode": "native"`) {
		t.Fatal("missing machine mode")
	}
	if !strings.Contains(out.String(), "user host values dropped: claude_code=native, pi=native") {
		t.Fatal(out.String())
	}
	if get(t, l.User, "ax.json") != get(t, l.LegacyUser, "ax.json") {
		t.Fatal("copy differs")
	}
	out.Reset()
	if err := Run(l, false, &out, Options{CanWriteSystem: allowed, WriteFile: func(string, []byte, os.FileMode) error { t.Fatal("second run wrote a file"); return nil }}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "nothing left to do") {
		t.Fatal(out.String())
	}
}

// Mixed machine host values and conflicting destinations reject the entire
// plan before even the otherwise-valid user file can be written.
func TestPlanRefusalsWriteNothing(t *testing.T) {
	for _, kind := range []string{"mixed", "conflict", "launcher-conflict", "backup-conflict"} {
		t.Run(kind, func(t *testing.T) {
			l := fixture(t)
			put(t, l.LegacyUser, "ax.json", `{"schema":"curator-run-ax-v1","enabled":true}`)
			var path string
			want := "migrate_destination_exists"
			switch kind {
			case "mixed":
				put(t, l.LegacySystem, "defaults.json", strings.Replace(legacy, `"host":"native","model"`, `"host":"hosted","model"`, 1))
				want = "host_member_withdrawn"
			case "conflict":
				os.Mkdir(l.User, 0700)
				path = filepath.Join(l.User, "ax.json")
				put(t, l.User, "ax.json", "different")
			case "launcher-conflict":
				os.Mkdir(l.System, 0700)
				path = filepath.Join(l.System, "launcher.json")
				put(t, l.System, "launcher.json", `{"schema":"curator-run-launcher-v1","host_mode":"hosted"}`)
				put(t, l.LegacySystem, "defaults.json", legacy)
			case "backup-conflict":
				os.Mkdir(l.User, 0700)
				path = filepath.Join(l.User, "defaults.json.bak")
				put(t, l.User, "defaults.json.bak", "other backup")
				put(t, l.LegacyUser, "defaults.json", legacy)
			}
			var out bytes.Buffer
			err := Run(l, false, &out, Options{CanWriteSystem: allowed})
			var e *Error
			if !errors.As(err, &e) || e.Code != want {
				t.Fatalf("%v", err)
			}
			if kind == "mixed" && e.Reason != "mixed_values" {
				t.Fatal(e)
			}
			if path != "" && get(t, filepath.Dir(path), filepath.Base(path)) == "" {
				t.Fatal("destination changed")
			}
			if _, err := os.Stat(filepath.Join(l.User, "ax.json")); kind != "conflict" && !os.IsNotExist(err) {
				t.Fatal("partial write", err)
			}
			if _, err := os.Stat(filepath.Join(l.System, "defaults.json")); !os.IsNotExist(err) {
				t.Fatal("partial system write", err)
			}
		})
	}
}

// Dry run reports the same plan without creating directories; unwritable system
// files are listed once and user files still migrate in the real run.
func TestDryRunAndUnwritableSystem(t *testing.T) {
	l := fixture(t)
	put(t, l.LegacySystem, "defaults.json", legacy)
	put(t, l.LegacySystem, "ax.json", "system")
	put(t, l.LegacyUser, "ax.json", "user")
	var out bytes.Buffer
	opt := Options{CanWriteSystem: func(string) bool { return false }}
	if err := Run(l, true, &out, opt); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "system files not copied:") != 1 {
		t.Fatal(out.String())
	}
	for _, d := range []string{l.System, l.User} {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Fatal("dry run created a directory", err)
		}
	}
	if err := Run(l, false, &out, opt); err != nil {
		t.Fatal(err)
	}
	if get(t, l.User, "ax.json") != "user" {
		t.Fatal("user not copied")
	}
	if _, err := os.Stat(l.System); !os.IsNotExist(err) {
		t.Fatal("system changed", err)
	}
}

// A failing commit after real writes restores existing files and backups,
// removes new files/directories, and never changes the legacy inputs.
func TestApplyFailureRollsBack(t *testing.T) {
	l := fixture(t)
	os.Mkdir(l.User, 0700)
	put(t, l.User, "defaults.json", legacy)
	put(t, l.User, "defaults.json.bak", legacy)
	put(t, l.LegacyUser, "defaults.json", legacy)
	put(t, l.LegacyUser, "a.json", "first")
	put(t, l.LegacyUser, "z.json", "last")
	calls := 0
	failure := errors.New("injected write failure")
	err := Run(l, false, &bytes.Buffer{}, Options{CanWriteSystem: allowed, WriteFile: func(p string, b []byte, m os.FileMode) error {
		calls++
		if calls == 3 {
			return failure
		}
		return atomicWrite(p, b, m)
	}})
	if !errors.Is(err, failure) || calls < 3 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	if get(t, l.User, "defaults.json") != legacy || get(t, l.User, "defaults.json.bak") != legacy || get(t, l.LegacyUser, "defaults.json") != legacy {
		t.Fatal("rollback did not preserve original")
	}
	for _, n := range []string{"a.json", "z.json"} {
		if _, err := os.Stat(filepath.Join(l.User, n)); !os.IsNotExist(err) {
			t.Fatal("partial write remains", n, err)
		}
	}
}

// A byte-identical final destination is already migrated. A raw legacy copy
// is rewritten once with its original bytes in .bak, then becomes a no-op too.
func TestIdenticalDestination(t *testing.T) {
	l := fixture(t)
	os.Mkdir(l.User, 0700)
	put(t, l.LegacyUser, "ax.json", "same")
	put(t, l.User, "ax.json", "same")
	if err := Run(l, false, &bytes.Buffer{}, Options{CanWriteSystem: allowed, WriteFile: func(string, []byte, os.FileMode) error { t.Fatal("identical file written"); return nil }}); err != nil {
		t.Fatal(err)
	}
	put(t, l.LegacyUser, "defaults.json", legacy)
	put(t, l.User, "defaults.json", legacy)
	if err := Run(l, false, &bytes.Buffer{}, Options{CanWriteSystem: allowed}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(get(t, l.User, "defaults.json"), `"host"`) || get(t, l.User, "defaults.json.bak") != legacy {
		t.Fatal("raw copy not migrated with original backup")
	}
	if err := Run(l, false, &bytes.Buffer{}, Options{CanWriteSystem: allowed, WriteFile: func(string, []byte, os.FileMode) error { t.Fatal("second run wrote"); return nil }}); err != nil {
		t.Fatal(err)
	}
}

// Generated launcher destinations compare exact final bytes, including
// formatting; semantic equality alone never authorizes an overwrite.
func TestSystemLauncherDestinationBytes(t *testing.T) {
	const desired = "{\n  \"host_mode\": \"native\",\n  \"schema\": \"curator-run-launcher-v1\"\n}\n"
	for _, same := range []bool{false, true} {
		t.Run(map[bool]string{false: "different-format", true: "identical"}[same], func(t *testing.T) {
			l := fixture(t)
			if err := os.Mkdir(l.System, 0700); err != nil {
				t.Fatal(err)
			}
			put(t, l.LegacySystem, "defaults.json", legacy)
			original := `{"schema":"curator-run-launcher-v1","host_mode":"native"}`
			if same {
				original = desired
			}
			put(t, l.System, "launcher.json", original)
			err := Run(l, false, &bytes.Buffer{}, Options{CanWriteSystem: allowed})
			if same {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(l.System, "defaults.json")); err != nil {
					t.Fatal(err)
				}
			} else {
				var e *Error
				if !errors.As(err, &e) || e.Code != CodeDestinationExists {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(l.System, "defaults.json")); !os.IsNotExist(err) {
					t.Fatal("partial write", err)
				}
			}
			if get(t, l.System, "launcher.json") != original {
				t.Fatal("existing launcher changed")
			}
		})
	}
}

// Unsafe legacy sources, destinations and backup paths refuse before any
// otherwise-valid file is copied; a dangling source home is never absence.
func TestUnsafeMigrationFiles(t *testing.T) {
	for _, kind := range []string{"source-symlink", "source-writable", "destination-symlink", "backup-symlink", "source-home-link"} {
		t.Run(kind, func(t *testing.T) {
			if runtime.GOOS == "windows" && kind == "source-writable" {
				t.Skip("POSIX mode gate; Windows uses DACLs")
			}
			l := fixture(t)
			put(t, l.LegacyUser, "defaults.json", legacy)
			put(t, l.LegacyUser, "a.json", "valid")
			path := filepath.Join(l.LegacyUser, "defaults.json")
			switch kind {
			case "source-writable":
				if err := os.Chmod(path, 0620); err != nil {
					t.Fatal(err)
				}
			case "source-home-link":
				os.Remove(filepath.Join(l.LegacySystem, "absent"))
				if err := os.Remove(l.LegacySystem); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), l.LegacySystem); err != nil {
					t.Fatal(err)
				}
			default:
				if kind == "source-symlink" {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.Mkdir(l.User, 0700); err != nil {
						t.Fatal(err)
					}
					path = filepath.Join(l.User, "defaults.json")
					if kind == "backup-symlink" {
						path += ".bak"
					}
				}
				if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), path); err != nil {
					t.Fatal(err)
				}
			}
			err := Run(l, false, &bytes.Buffer{}, Options{CanWriteSystem: allowed})
			var e *Error
			if !errors.As(err, &e) || e.Code != "defaults_config_invalid" {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(l.User, "a.json")); !os.IsNotExist(err) {
				t.Fatal("partial write", err)
			}
		})
	}
}

// Failure after creating a destination directory removes its newly copied
// files and the directory itself, while leaving the old directory untouched.
func TestFailureRemovesNewDirectory(t *testing.T) {
	l := fixture(t)
	put(t, l.LegacyUser, "a.json", "a")
	put(t, l.LegacyUser, "z.json", "z")
	calls := 0
	failure := errors.New("commit failed")
	err := Run(l, false, &bytes.Buffer{}, Options{CanWriteSystem: allowed, WriteFile: func(p string, b []byte, m os.FileMode) error {
		calls++
		if calls == 2 {
			return failure
		}
		return atomicWrite(p, b, m)
	}})
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, err := os.Stat(l.User); !os.IsNotExist(err) {
		t.Fatal("new directory not rolled back", err)
	}
	if get(t, l.LegacyUser, "a.json") != "a" {
		t.Fatal("source changed")
	}
}

// Migrating machine host defaults preserves the independent trial opt-in in
// an old launcher file and backs up its exact pre-rewrite bytes.
func TestPreserveDelegatedTrial(t *testing.T) {
	l := fixture(t)
	put(t, l.LegacySystem, "defaults.json", legacy)
	old := `{"schema":"curator-run-launcher-v1","delegated_trial":true}`
	put(t, l.LegacySystem, "launcher.json", old)
	if err := Run(l, false, &bytes.Buffer{}, Options{CanWriteSystem: allowed}); err != nil {
		t.Fatal(err)
	}
	parsed, err := launcherconfig.Parse([]byte(get(t, l.System, "launcher.json")))
	if err != nil || !parsed.DelegatedTrial || !parsed.DelegatedTrialSet || parsed.HostMode.Value != "native" {
		t.Fatal(parsed, err)
	}
	if get(t, l.System, "launcher.json.bak") != old || get(t, l.LegacySystem, "launcher.json") != old {
		t.Fatal("launcher original lost")
	}
	if err := Run(l, false, &bytes.Buffer{}, Options{CanWriteSystem: allowed, WriteFile: func(string, []byte, os.FileMode) error { t.Fatal("second run wrote"); return nil }}); err != nil {
		t.Fatal(err)
	}
}

// A write that changes a file and then fails restores both layers' old files
// and permissions, and removes all newly generated policy and backups.
func TestFailureAfterWriteRestoresBothLayers(t *testing.T) {
	l := fixture(t)
	for _, dir := range []string{l.System, l.User} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		put(t, dir, "defaults.json", legacy)
	}
	if err := os.Chmod(filepath.Join(l.System, "defaults.json"), 0640); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{l.LegacySystem, l.LegacyUser} {
		put(t, dir, "defaults.json", legacy)
	}
	failure := errors.New("failure after replacing destination")
	err := Run(l, false, &bytes.Buffer{}, Options{CanWriteSystem: allowed, WriteFile: func(p string, b []byte, m os.FileMode) error {
		if err := atomicWrite(p, b, m); err != nil {
			return err
		}
		if p == filepath.Join(l.User, "defaults.json") {
			return failure
		}
		return nil
	}})
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	for _, dir := range []string{l.System, l.User, l.LegacySystem, l.LegacyUser} {
		if get(t, dir, "defaults.json") != legacy {
			t.Fatal("file not restored", dir)
		}
	}
	for _, path := range []string{filepath.Join(l.System, "launcher.json"), filepath.Join(l.System, "defaults.json.bak"), filepath.Join(l.User, "defaults.json.bak")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("new file remains", path, err)
		}
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(l.System, "defaults.json"))
		if err != nil || info.Mode().Perm() != 0640 {
			t.Fatal("mode not restored", err)
		}
	}
}

// An unwritable system layer is listed on exactly one physical line even when
// an old filename contains a newline; the listing cannot inject another line.
func TestSystemListingSingleLine(t *testing.T) {
	l := fixture(t)
	put(t, l.LegacySystem, "one\nline.json", "data")
	put(t, l.LegacySystem, "second.json", "data")
	var out bytes.Buffer
	if err := Run(l, true, &out, Options{CanWriteSystem: func(string) bool { return false }}); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "system files not copied:") != 1 || !strings.Contains(out.String(), `one\nline.json`) || strings.Count(out.String(), "\n") != 2 {
		t.Fatal(out.String())
	}
}

// Aliased and nested destination homes refuse before writing legacy bytes or
// creating backups; the ordinary distinct-home migration is covered above.
func TestDestinationCannotOverlapLegacyHome(t *testing.T) {
	for _, kind := range []string{"same", "nested", "ancestor", "alias", "cross-layer-alias"} {
		t.Run(kind, func(t *testing.T) {
			l := fixture(t)
			put(t, l.LegacyUser, "defaults.json", legacy)
			switch kind {
			case "same":
				l.User = l.LegacyUser
			case "nested":
				l.User = filepath.Join(l.LegacyUser, "new", "launcher")
			case "ancestor":
				l.User = filepath.Dir(l.LegacyUser)
			case "alias", "cross-layer-alias":
				target := l.LegacyUser
				if kind == "cross-layer-alias" {
					target = l.LegacySystem
				}
				if err := os.Symlink(target, l.User); err != nil {
					if runtime.GOOS == "windows" {
						t.Skip("symlink privilege unavailable")
					}
					t.Fatal(err)
				}
			}
			err := Run(l, false, &bytes.Buffer{}, Options{CanWriteSystem: allowed, WriteFile: func(string, []byte, os.FileMode) error {
				t.Fatal("overlap wrote a file")
				return nil
			}})
			var e *Error
			if !errors.As(err, &e) || e.Code != "defaults_config_invalid" || !strings.Contains(e.Error(), "overlaps legacy") {
				t.Fatalf("unexpected refusal: %v", err)
			}
			if get(t, l.LegacyUser, "defaults.json") != legacy {
				t.Fatal("legacy source changed")
			}
			for _, old := range []string{l.LegacySystem, l.LegacyUser} {
				entries, err := os.ReadDir(old)
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range entries {
					if entry.Name() != "defaults.json" {
						t.Fatalf("unexpected legacy write: %s", entry.Name())
					}
				}
			}
		})
	}
}
