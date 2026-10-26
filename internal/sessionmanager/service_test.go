package sessionmanager

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func manifest(t *testing.T, id string, e Entry) string {
	t.Helper()
	doc := map[string]any{"id": id, "executable": e.Executable, "argv_prefix": e.ArgvPrefix, "protocol": e.Protocol, "registered_by": e.RegisteredBy}
	data, _ := json.Marshal(doc)
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func service(t *testing.T) (Service, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	out, errout := &bytes.Buffer{}, &bytes.Buffer{}
	return Service{Resolver: fixture(t), Stdout: out, Stderr: errout, Stdin: strings.NewReader(""), CanWriteSystem: func(string) bool { return false }, Now: func() time.Time { return time.Date(2026, 10, 18, 10, 0, 0, 0, time.FixedZone("offset", 7200)) }}, out, errout
}
func readTarget(t *testing.T, s Service) Registry {
	t.Helper()
	r, _, err := Load(s.Resolver)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Registration applies explicit flags before upgrade/first/TTY defaults,
// asks at most once for a new peer, and preserves all unrelated entries.
func TestRegistrationRules(t *testing.T) {
	for _, test := range []struct {
		name, existing, answer    string
		tty, selectFlag, noSelect bool
		want                      string
		prompt                    bool
		notice                    bool
	}{
		{name: "first-non-tty", want: "new"}, {name: "first-tty", tty: true, want: "new"},
		{name: "first-no-select", noSelect: true},
		{name: "peer-non-tty", existing: "old", want: "old", notice: true},
		{name: "peer-tty-default", existing: "old", tty: true, answer: "\n", want: "old", prompt: true},
		{name: "peer-tty-yes", existing: "old", tty: true, answer: "yes\n", want: "new", prompt: true},
		{name: "peer-tty-no", existing: "old", tty: true, answer: "no\n", want: "old", prompt: true},
		{name: "peer-select", existing: "old", selectFlag: true, want: "new"},
		{name: "peer-no-select", existing: "old", noSelect: true, want: "old"},
		{name: "same-id", existing: "new", want: "new"},
		{name: "same-id-no-select", existing: "new", noSelect: true, want: "new"},
		{name: "unselected-peer", existing: "unselected", notice: true},
		{name: "unselected-tty-yes", existing: "unselected", tty: true, answer: "y\n", want: "new", prompt: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, out, prompt := service(t)
			e := entry(t)
			r := Empty()
			if test.existing != "" {
				id := test.existing
				if id == "unselected" {
					id = "old"
				} else {
					r.Selected = id
				}
				r.Managers[id] = e
				putRegistry(t, s.Resolver.Locations.User, r)
			}
			ttyCalls := 0
			s.IsTerminal = func() bool { ttyCalls++; return test.tty }
			s.Stdin = strings.NewReader(test.answer)
			if err := s.Run(Command{Operation: "register", Manifest: manifest(t, "new", e), Select: test.selectFlag, NoSelect: test.noSelect}); err != nil {
				t.Fatal(err)
			}
			got := readTarget(t, s)
			if got.Selected != test.want || got.Managers["new"].RegisteredAt != "2026-10-18T08:00:00Z" {
				t.Fatal(got)
			}
			if test.existing != "" && test.existing != "new" {
				if _, ok := got.Managers["old"]; !ok {
					t.Fatal("lost other entry")
				}
			}
			if (prompt.Len() > 0) != test.prompt || strings.Count(prompt.String(), "[y/N]") > 1 {
				t.Fatal(prompt.String())
			}
			if test.prompt && (!strings.Contains(prompt.String(), "new") || !strings.Contains(prompt.String(), map[bool]string{true: "none", false: "old"}[test.existing == "unselected"])) {
				t.Fatal(prompt.String())
			}
			if test.notice {
				current := test.want
				if current == "" {
					current = "none"
				}
				want := "registered new; selection unchanged (" + current + "); run: curator-run session-manager use new\n"
				if out.String() != want {
					t.Fatal(out.String())
				}
			} else if out.Len() != 0 {
				t.Fatal(out.String())
			}
			if test.existing == "new" && ttyCalls != 0 {
				t.Fatal("upgrade checked terminal")
			}
		})
	}
}

// Changing an existing executable resolves symlinks, reports exactly one
// change line, and keeps selection; explicit selection still overrides it.
func TestExecutableChangeAndManifestStdin(t *testing.T) {
	for _, selectFlag := range []bool{false, true} {
		t.Run(map[bool]string{false: "keep", true: "explicit-select"}[selectFlag], func(t *testing.T) {
			s, out, prompt := service(t)
			old, newEntry := entry(t), entry(t)
			r := Empty()
			r.Managers["board"] = old
			r.Managers["other"] = old
			r.Selected = "other"
			putRegistry(t, s.Resolver.Locations.User, r)
			alias := filepath.Join(t.TempDir(), "alias")
			if err := os.Symlink(newEntry.Executable, alias); err != nil {
				t.Fatal(err)
			}
			newEntry.Executable = alias
			path := manifest(t, "board", newEntry)
			data, _ := os.ReadFile(path)
			s.Stdin = bytes.NewReader(data)
			s.IsTerminal = func() bool { t.Fatal("upgrade asked terminal"); return true }
			if err := s.Run(Command{Operation: "register", Manifest: "-", Select: selectFlag}); err != nil {
				t.Fatal(err)
			}
			got := readTarget(t, s)
			resolved, _ := filepath.EvalSymlinks(alias)
			wantSelected := "other"
			if selectFlag {
				wantSelected = "board"
			}
			if got.Selected != wantSelected || got.Managers["board"].Executable != resolved || prompt.Len() != 0 {
				t.Fatal(got, prompt.String())
			}
			want := "session manager board: executable changed " + old.Executable + " -> " + resolved + "\n"
			if out.String() != want {
				t.Fatal(out.String())
			}
		})
	}
}

// Target-layer rules never import the user's registry into a new system file;
// every user mutation under an existing system registry refuses without writes.
func TestMutationLayers(t *testing.T) {
	for _, op := range []string{"register", "select", "unregister"} {
		t.Run(op, func(t *testing.T) {
			s, _, _ := service(t)
			r := Empty()
			r.Managers["old"] = entry(t)
			r.Selected = "old"
			putRegistry(t, s.Resolver.Locations.System, r)
			c := Command{Operation: op, ID: "old", Manifest: manifest(t, "new", entry(t))}
			s.WriteFile = func(string, []byte, os.FileMode) error { t.Fatal("shadowed write"); return nil }
			err := s.Run(c)
			requireCode(t, err, CodeShadowed, "")
			if readTarget(t, s).Selected != "old" {
				t.Fatal("changed system")
			}
		})
	}
	t.Run("shadowed-with-unusable-user-home", func(t *testing.T) {
		// Machine authority refuses before touching an unusable user home. Repairing
		// that home and removing the system file permits the identical selection.
		s, _, _ := service(t)
		r := Empty()
		r.Managers["old"] = entry(t)
		putRegistry(t, s.Resolver.Locations.System, r)
		user := s.Resolver.Locations.User
		if err := os.Remove(user); err != nil {
			t.Fatal(err)
		}
		blocker := []byte("not a directory")
		if err := os.WriteFile(user, blocker, 0600); err != nil {
			t.Fatal(err)
		}
		command := Command{Operation: "select", ID: "old"}
		requireCode(t, s.Run(command), CodeShadowed, "")
		got, err := os.ReadFile(user)
		if err != nil || !bytes.Equal(blocker, got) {
			t.Fatal("shadowed mutation touched user home", err)
		}
		if err = os.Remove(user); err != nil {
			t.Fatal(err)
		}
		if err = os.Mkdir(user, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.Remove(filepath.Join(s.Resolver.Locations.System, Filename)); err != nil {
			t.Fatal(err)
		}
		putRegistry(t, user, r)
		if err = s.Run(command); err != nil {
			t.Fatal(err)
		}
		if readTarget(t, s).Selected != "old" {
			t.Fatal("repaired user control did not select")
		}
	})

	s, _, _ := service(t)
	r := Empty()
	r.Managers["old"] = entry(t)
	r.Selected = "old"
	putRegistry(t, s.Resolver.Locations.User, r)
	s.CanWriteSystem = func(string) bool { return true }
	if err := s.Run(Command{Operation: "register", Manifest: manifest(t, "new", entry(t))}); err != nil {
		t.Fatal(err)
	}
	got := readTarget(t, s)
	if got.Selected != "new" || len(got.Managers) != 1 {
		t.Fatal("imported user registry", got)
	}
	userData, _ := os.ReadFile(filepath.Join(s.Resolver.Locations.User, Filename))
	user, _ := Parse(userData)
	if user.Selected != "old" {
		t.Fatal("user changed")
	}
}

// Select/unregister unknown ids leave bytes intact; valid removal clears only
// the removed selection and never auto-selects a remaining peer.
func TestSelectAndUnregister(t *testing.T) {
	s, _, _ := service(t)
	r := Empty()
	r.Managers["one"] = entry(t)
	r.Managers["two"] = entry(t)
	r.Selected = "one"
	putRegistry(t, s.Resolver.Locations.User, r)
	for _, op := range []string{"select", "unregister", "show"} {
		before, _ := Encode(readTarget(t, s))
		err := s.Run(Command{Operation: op, ID: "unknown"})
		requireCode(t, err, CodeNotRegistered, "")
		after, _ := Encode(readTarget(t, s))
		if !bytes.Equal(before, after) {
			t.Fatal("unknown id wrote")
		}
	}
	if err := s.Run(Command{Operation: "select", ID: "two"}); err != nil {
		t.Fatal(err)
	}
	if readTarget(t, s).Selected != "two" {
		t.Fatal("selection failed")
	}
	if err := s.Run(Command{Operation: "unregister", ID: "one"}); err != nil {
		t.Fatal(err)
	}
	if readTarget(t, s).Selected != "two" {
		t.Fatal("removed unrelated selection")
	}
	if err := s.Run(Command{Operation: "register", Manifest: manifest(t, "one", entry(t)), NoSelect: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.Run(Command{Operation: "unregister", ID: "two"}); err != nil {
		t.Fatal(err)
	}
	state, err := Resolve(readTarget(t, s), ExecutableAvailable)
	if err != nil || state.Kind != "unselected" {
		t.Fatal(state, err)
	}
	if err := s.Run(Command{Operation: "unregister", ID: "one"}); err != nil {
		t.Fatal(err)
	}
	state, err = Resolve(readTarget(t, s), ExecutableAvailable)
	if err != nil || state.Kind != "empty" {
		t.Fatal(state, err)
	}
}

// Invalid manifests and conflicting selection flags cannot partially update a
// valid registry; a nearby valid manifest establishes the positive control.
func TestManifestRefusalsPreserveRegistry(t *testing.T) {
	for _, kind := range []string{"relative", "nonexec", "directory", "bad-id", "prefix", "extra", "timestamp", "by-empty", "by-control", "by-unicode", "by-long", "flags"} {
		t.Run(kind, func(t *testing.T) {
			s, _, _ := service(t)
			e := entry(t)
			if err := s.Run(Command{Operation: "register", Manifest: manifest(t, "valid", e)}); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(filepath.Join(s.Resolver.Locations.User, Filename))
			path := manifest(t, "new", e)
			data, _ := os.ReadFile(path)
			var doc map[string]any
			json.Unmarshal(data, &doc)
			c := Command{Operation: "register", Manifest: path}
			switch kind {
			case "relative":
				doc["executable"] = "manager"
			case "nonexec":
				os.Chmod(e.Executable, 0600)
			case "directory":
				doc["executable"] = t.TempDir()
			case "bad-id":
				doc["id"] = "Bad"
			case "prefix":
				doc["argv_prefix"] = []string{"has space"}
			case "extra":
				doc["extra"] = true
			case "timestamp":
				doc["registered_at"] = "2026-10-18T00:00:00Z"
			case "by-empty":
				doc["registered_by"] = ""
			case "by-control":
				doc["registered_by"] = "line\nbreak"
			case "by-unicode":
				doc["registered_by"] = "é"
			case "by-long":
				doc["registered_by"] = strings.Repeat("a", 129)
			case "flags":
				c.Select = true
				c.NoSelect = true
			}
			data, _ = json.Marshal(doc)
			os.WriteFile(path, data, 0600)
			err := s.Run(c)
			code := CodeInvalid
			if kind == "flags" {
				code = "usage"
			}
			var e2 *Error
			if !errors.As(err, &e2) || e2.Code != code {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(filepath.Join(s.Resolver.Locations.User, Filename))
			if !bytes.Equal(before, after) {
				t.Fatal("partial update")
			}
		})
	}
}

// JSON list/show preserve the closed registry/entry fields and human list is
// stable by id; listing does not require registered executables to still exist.
func TestListAndShow(t *testing.T) {
	s, out, _ := service(t)
	r := Empty()
	e := entry(t)
	r.Managers["z"] = e
	r.Managers["a"] = e
	r.Selected = "z"
	putRegistry(t, s.Resolver.Locations.User, r)
	os.Remove(e.Executable)
	if err := s.Run(Command{Operation: "list", JSON: true}); err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Registry
		Layer string `json:"layer"`
	}
	if err := json.Unmarshal(out.Bytes(), &listed); err != nil || listed.Layer != "user" || len(listed.Managers) != 2 {
		t.Fatal(err)
	}
	out.Reset()
	if err := s.Run(Command{Operation: "show", ID: "a", JSON: true}); err != nil {
		t.Fatal(err)
	}
	var shown struct {
		ID string
		Entry
	}
	if err := json.Unmarshal(out.Bytes(), &shown); err != nil || shown.ID != "a" || shown.Executable != e.Executable {
		t.Fatal(shown, err)
	}
	out.Reset()
	if err := s.Run(Command{Operation: "list"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "layer: user\n  a ") || !strings.Contains(out.String(), "\n* z ") {
		t.Fatal(out.String())
	}
}

// A failed atomic write before rename or at directory fsync restores the exact
// old bytes/mode, while a successful write syncs its containing directory once.
func TestAtomicFailurePreservesOldFile(t *testing.T) {
	for _, kind := range []string{"success", "before-rename", "directory-sync", "new-directory-sync"} {
		t.Run(kind, func(t *testing.T) {
			s, _, _ := service(t)
			path := filepath.Join(s.Resolver.Locations.User, Filename)
			original := Empty()
			putRegistry(t, s.Resolver.Locations.User, original)
			before, _ := os.ReadFile(path)
			if kind == "new-directory-sync" {
				os.Remove(path)
			}
			injected := errors.New("injected write failure")
			syncs := 0
			s.WriteFile = func(p string, b []byte, m os.FileMode) error {
				hooks := atomicHooks{syncDirectory: func(dir string) error {
					syncs++
					if dir != filepath.Dir(path) {
						t.Fatal(dir)
					}
					if strings.Contains(kind, "directory-sync") {
						return injected
					}
					return syncDirectory(dir)
				}}
				if kind == "before-rename" {
					hooks.beforeRename = func() error { return injected }
				}
				return atomicWriteWithHooks(p, b, m, hooks)
			}
			err := s.Run(Command{Operation: "register", Manifest: manifest(t, "new", entry(t))})
			if kind == "success" {
				if err != nil || syncs != 1 || readTarget(t, s).Selected != "new" {
					t.Fatal(err, syncs)
				}
				return
			}
			if !errors.Is(err, injected) {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if kind == "new-directory-sync" {
				if !os.IsNotExist(err) {
					t.Fatal("left new file", err)
				}
			} else {
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("old bytes changed", err)
				}
				info, _ := os.Stat(path)
				if info.Mode().Perm() != 0600 {
					t.Fatal(info.Mode())
				}
			}
			files, _ := os.ReadDir(filepath.Dir(path))
			for _, file := range files {
				if file.Name() != Filename && file.Name() != Filename+".lock" {
					t.Fatal("scratch leaked", file.Name())
				}
			}
		})
	}
}

// A terminal read failure refuses before writing, rather than interpreting an
// interrupted answer as consent; EOF alone retains the current selection.
func TestPromptReadFailure(t *testing.T) {
	s, _, _ := service(t)
	r := Empty()
	r.Managers["old"] = entry(t)
	r.Selected = "old"
	putRegistry(t, s.Resolver.Locations.User, r)
	s.IsTerminal = func() bool { return true }
	s.Stdin = failedReader{}
	err := s.Run(Command{Operation: "register", Manifest: manifest(t, "new", entry(t))})
	requireCode(t, err, CodeInvalid, "unreadable")
	if len(readTarget(t, s).Managers) != 1 {
		t.Fatal("partial write")
	}
	s.Stdin = strings.NewReader("")
	if err := s.Run(Command{Operation: "register", Manifest: manifest(t, "new", entry(t))}); err != nil {
		t.Fatal(err)
	}
	if readTarget(t, s).Selected != "old" {
		t.Fatal("EOF selected peer")
	}
}

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// Legacy registry mutations write the new home without modifying the legacy
// bytes; privileged select/remove start empty rather than importing user ids.
func TestMutationLegacyAndNewSystem(t *testing.T) {
	s, _, _ := service(t)
	r := Empty()
	r.Managers["one"] = entry(t)
	putRegistry(t, s.Resolver.Locations.LegacyUser, r)
	old := filepath.Join(s.Resolver.Locations.LegacyUser, Filename)
	before, _ := os.ReadFile(old)
	if err := s.Run(Command{Operation: "select", ID: "one"}); err != nil {
		t.Fatal(err)
	}
	if readTarget(t, s).Selected != "one" {
		t.Fatal("new selection missing")
	}
	after, _ := os.ReadFile(old)
	if !bytes.Equal(before, after) {
		t.Fatal("legacy rewritten")
	}
	s.CanWriteSystem = func(string) bool { return true }
	for _, op := range []string{"select", "unregister"} {
		requireCode(t, s.Run(Command{Operation: op, ID: "one"}), CodeNotRegistered, "")
	}
	if _, err := os.Stat(filepath.Join(s.Resolver.Locations.System, Filename)); !os.IsNotExist(err) {
		t.Fatal("unknown mutation created registry", err)
	}
}

// Output refusal occurs before commit, preserving bytes; retry with a working
// output writer registers the same peer and preserves selection.
func TestRegistrationOutputFailurePreservesRegistry(t *testing.T) {
	s, _, _ := service(t)
	r := Empty()
	e := entry(t)
	r.Managers["old"] = e
	r.Selected = "old"
	putRegistry(t, s.Resolver.Locations.User, r)
	path := filepath.Join(s.Resolver.Locations.User, Filename)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Stdout = failedWriter{}
	command := Command{Operation: "register", Manifest: manifest(t, "new", e), NoSelect: true}
	if err = s.Run(command); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("output failure changed registry", err)
	}
	s.Stdout = io.Discard
	if err = s.Run(command); err != nil {
		t.Fatal(err)
	}
	got := readTarget(t, s)
	if len(got.Managers) != 2 || got.Selected != "old" {
		t.Fatal(got)
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

// Re-registering through a symlinked parent is silent for the same executable;
// a distinct file through that parent emits exactly one line with resolved
// paths. Real registrations seed the registry; selection flags remain effective.
func TestRegistrationSymlinkedParent(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		selectFlag, noSelect bool
		selected             string
	}{
		{"keep", false, false, "other"},
		{"no-select", false, true, "other"},
		{"select", true, false, "board"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, out, prompt := service(t)
			e := entry(t)
			realParent, err := filepath.EvalSymlinks(filepath.Dir(e.Executable))
			if err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(t.TempDir(), "install-bin")
			if err := os.Symlink(realParent, alias); err != nil {
				t.Fatal(err)
			}
			e.Executable = filepath.Join(alias, "manager")
			oldResolved := filepath.Join(realParent, "manager")
			if e.Executable == oldResolved {
				t.Fatal("fixture does not traverse a symlink")
			}
			if err := s.Run(Command{Operation: "register", Manifest: manifest(t, "board", e)}); err != nil {
				t.Fatal(err)
			}
			if got := readTarget(t, s).Managers["board"].Executable; got != oldResolved {
				t.Fatalf("registration stored %q, want resolved %q", got, oldResolved)
			}
			if err := s.Run(Command{Operation: "register", Manifest: manifest(t, "other", e), Select: true}); err != nil {
				t.Fatal(err)
			}
			out.Reset()
			prompt.Reset()
			s.IsTerminal = func() bool { t.Fatal("upgrade asked terminal"); return true }
			command := Command{Operation: "register", Manifest: manifest(t, "board", e), Select: tc.selectFlag, NoSelect: tc.noSelect}
			if err := s.Run(command); err != nil {
				t.Fatal(err)
			}
			if out.Len() != 0 {
				t.Fatalf("same executable emitted a change line: %s", out)
			}
			got := readTarget(t, s)
			if got.Selected != tc.selected || got.Managers["board"].Executable != oldResolved || prompt.Len() != 0 {
				t.Fatal(got, prompt.String())
			}
			newResolved := filepath.Join(realParent, "replacement")
			if err := os.WriteFile(newResolved, []byte("fixture; never executed"), 0700); err != nil {
				t.Fatal(err)
			}
			e.Executable = filepath.Join(alias, "replacement")
			command.Manifest = manifest(t, "board", e)
			if err := s.Run(command); err != nil {
				t.Fatal(err)
			}
			want := "session manager board: executable changed " + oldResolved + " -> " + newResolved + "\n"
			if out.String() != want {
				t.Fatalf("change line = %q, want resolved paths %q", out.String(), want)
			}
			got = readTarget(t, s)
			if got.Selected != tc.selected || got.Managers["board"].Executable != newResolved || got.Managers["other"].Executable != oldResolved || prompt.Len() != 0 {
				t.Fatal(got, prompt.String())
			}
		})
	}
}
