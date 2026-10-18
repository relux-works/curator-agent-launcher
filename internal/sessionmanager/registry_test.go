package sessionmanager

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
)

func fixture(t *testing.T) configfile.Resolver {
	t.Helper()
	root := t.TempDir()
	l := configfile.Locations{System: filepath.Join(root, "system"), User: filepath.Join(root, "user"), LegacySystem: filepath.Join(root, "old-system"), LegacyUser: filepath.Join(root, "old-user")}
	for _, dir := range []string{l.System, l.User, l.LegacySystem, l.LegacyUser} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return configfile.Resolver{Locations: l}
}

// Registry fixtures model persisted registrations, whose executable paths are
// resolved even when the temporary directory itself is reached through a link.
func entry(t *testing.T) Entry {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manager")
	if err := os.WriteFile(path, []byte("fixture; never executed"), 0700); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return Entry{Executable: resolved, ArgvPrefix: []string{"session-manager", "plugin"}, Protocol: Protocol, RegisteredAt: "2026-10-18T09:00:00Z", RegisteredBy: "test installer"}
}
func putRegistry(t *testing.T, dir string, r Registry) {
	t.Helper()
	data, err := Encode(r)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, Filename), data, 0600); err != nil {
		t.Fatal(err)
	}
}
func requireCode(t *testing.T, err error, code, reason string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != code || e.Reason != reason {
		t.Fatalf("got %v, want %s/%s", err, code, reason)
	}
}

// Closed schema rejects malformed/duplicate JSON, invalid ids, member types,
// timestamps and argument/protocol grammars; the unchanged document is valid.
func TestRegistryClosedSchema(t *testing.T) {
	base := Empty()
	base.Managers["board"] = entry(t)
	base.Selected = "board"
	data, _ := Encode(base)
	if _, err := Parse(data); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"root-extra", func(v map[string]any) { v["extra"] = true }},
		{"schema", func(v map[string]any) { v["schema"] = "other" }},
		{"missing-managers", func(v map[string]any) { delete(v, "managers") }},
		{"null-managers", func(v map[string]any) { v["managers"] = nil }},
		{"bad-id", func(v map[string]any) {
			v["managers"].(map[string]any)["Bad"] = v["managers"].(map[string]any)["board"]
		}},
		{"long-id", func(v map[string]any) {
			v["managers"].(map[string]any)[strings.Repeat("a", 33)] = v["managers"].(map[string]any)["board"]
		}},
		{"unknown-selected", func(v map[string]any) { v["selected"] = "absent" }},
		{"null-selected", func(v map[string]any) { v["selected"] = nil }},
		{"entry-extra", func(v map[string]any) { v["managers"].(map[string]any)["board"].(map[string]any)["extra"] = true }},
		{"relative", func(v map[string]any) {
			v["managers"].(map[string]any)["board"].(map[string]any)["executable"] = "manager"
		}},
		{"protocol", func(v map[string]any) { v["managers"].(map[string]any)["board"].(map[string]any)["protocol"] = "other" }},
		{"prefix-count", func(v map[string]any) {
			v["managers"].(map[string]any)["board"].(map[string]any)["argv_prefix"] = []string{"a", "b", "c", "d", "e"}
		}},
		{"prefix-value", func(v map[string]any) {
			v["managers"].(map[string]any)["board"].(map[string]any)["argv_prefix"] = []string{"Upper"}
		}},
		{"prefix-null", func(v map[string]any) { v["managers"].(map[string]any)["board"].(map[string]any)["argv_prefix"] = nil }},
		{"timestamp", func(v map[string]any) {
			v["managers"].(map[string]any)["board"].(map[string]any)["registered_at"] = "yesterday"
		}},
		{"timestamp-comma", func(v map[string]any) {
			v["managers"].(map[string]any)["board"].(map[string]any)["registered_at"] = "2026-10-18T09:00:00,5Z"
		}},
		{"timestamp-hour", func(v map[string]any) {
			v["managers"].(map[string]any)["board"].(map[string]any)["registered_at"] = "2026-10-18T9:00:00Z"
		}},
		{"non-utc", func(v map[string]any) {
			v["managers"].(map[string]any)["board"].(map[string]any)["registered_at"] = "2026-10-18T09:00:00+02:00"
		}},
		{"metadata-empty", func(v map[string]any) { v["managers"].(map[string]any)["board"].(map[string]any)["registered_by"] = "" }},
		{"metadata-type", func(v map[string]any) { v["managers"].(map[string]any)["board"].(map[string]any)["registered_by"] = 42 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var v map[string]any
			json.Unmarshal(data, &v)
			test.edit(v)
			bad, _ := json.Marshal(v)
			got, err := Parse(bad)
			requireCode(t, err, CodeInvalid, "schema")
			if got.Managers != nil {
				t.Fatal("partial registry")
			}
		})
	}
	for _, bad := range []string{`{`, string(data) + `{}`, `{"schema":"x","schema":"y"}`} {
		_, err := Parse([]byte(bad))
		requireCode(t, err, CodeInvalid, "parse")
	}
}

// State resolution distinguishes absence, no selection, valid selection and a
// broken selected executable; PATH contents never create a registration.
func TestRegistryStatesAndNoPATH(t *testing.T) {
	r := Empty()
	e := entry(t)
	t.Setenv("PATH", filepath.Dir(e.Executable))
	state, err := Resolve(r, func(string) bool { t.Fatal("empty probed executable"); return false })
	if err != nil || state.Kind != "empty" {
		t.Fatal(state, err)
	}
	if ExecutableAvailable(filepath.Base(e.Executable)) {
		t.Fatal("searched PATH")
	}
	r.Managers["board"] = e
	state, err = Resolve(r, func(string) bool { t.Fatal("unselected probed executable"); return false })
	if err != nil || state.Kind != "unselected" {
		t.Fatal(state, err)
	}
	r.Selected = "board"
	state, err = Resolve(r, ExecutableAvailable)
	if err != nil || state.Kind != "selected" || state.Entry.Executable != e.Executable {
		t.Fatal(state, err)
	}
	for _, kind := range []string{"absent", "directory", "nonexec"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "broken")
			switch kind {
			case "directory":
				os.Mkdir(path, 0700)
			case "nonexec":
				os.WriteFile(path, []byte("x"), 0600)
			}
			broken := e
			broken.Executable = path
			r.Managers["board"] = broken
			state, err := Resolve(r, ExecutableAvailable)
			requireCode(t, err, CodeHostMissing, "registered executable missing")
			if state.Kind != "selected" {
				t.Fatal(state)
			}
		})
	}
}

// Machine-first loading never opens a hostile user registry; selected old/new
// files retain secure-reader checks, and invalid machine data never falls back.
func TestRegistryLayersAndSecurity(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "fallback"}[legacy], func(t *testing.T) {
			resolver := fixture(t)
			r := Empty()
			r.Managers["board"] = entry(t)
			r.Selected = "board"
			dir := resolver.Locations.System
			if legacy {
				dir = resolver.Locations.LegacySystem
			}
			putRegistry(t, dir, r)
			if err := os.Symlink(filepath.Join(resolver.Locations.User, "missing"), filepath.Join(resolver.Locations.User, Filename)); err != nil {
				t.Fatal(err)
			}
			var warnings strings.Builder
			resolver.Stderr = &warnings
			got, path, err := Load(resolver)
			if err != nil || got.Selected != "board" || path != filepath.Join(dir, Filename) {
				t.Fatal(got, path, err)
			}
			if strings.Count(warnings.String(), "deprecated config path") != map[bool]int{false: 0, true: 1}[legacy] {
				t.Fatal(warnings.String())
			}
			os.WriteFile(path, []byte(`{`), 0600)
			_, _, err = Load(resolver)
			requireCode(t, err, CodeInvalid, "parse")
			os.Remove(path)
			os.Symlink(filepath.Join(dir, "missing"), path)
			_, _, err = Load(resolver)
			requireCode(t, err, CodeInvalid, "symlink")
		})
	}
	resolver := fixture(t)
	r := Empty()
	putRegistry(t, resolver.Locations.User, r)
	got, _, err := Load(resolver)
	if err != nil || len(got.Managers) != 0 {
		t.Fatal(got, err)
	}
}

// Exact documented id, prefix and metadata size boundaries are admitted;
// crossing any one grammar boundary refuses the entire registry.
func TestRegistryGrammarBoundaries(t *testing.T) {
	e := entry(t)
	e.ArgvPrefix = []string{strings.Repeat("a", 32), "-", "0", "four"}
	e.RegisteredBy = strings.Repeat("x", 128)
	e.RegisteredAt = "2026-10-18T09:00:00+00:00"
	r := Empty()
	r.Managers[strings.Repeat("a", 32)] = e
	data, _ := Encode(r)
	if _, err := Parse(data); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", strings.Repeat("a", 33), "space here", "UPPER"} {
		bad := e
		bad.ArgvPrefix = []string{value}
		r.Managers[strings.Repeat("a", 32)] = bad
		data, _ := Encode(r)
		_, err := Parse(data)
		requireCode(t, err, CodeInvalid, "schema")
	}
	for _, value := range []string{"", strings.Repeat("x", 129), "line\nbreak", "é"} {
		bad := e
		bad.RegisteredBy = value
		r.Managers[strings.Repeat("a", 32)] = bad
		data, _ := Encode(r)
		_, err := Parse(data)
		requireCode(t, err, CodeInvalid, "schema")
	}
	e.ArgvPrefix = []string{}
	r.Managers[strings.Repeat("a", 32)] = e
	data, _ = Encode(r)
	if _, err := Parse(data); err != nil {
		t.Fatal("empty prefix", err)
	}
}
