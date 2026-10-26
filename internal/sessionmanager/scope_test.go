package sessionmanager

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// use and select commit identical registry bytes and stdout; only select emits
// the one-line deprecation warning, including when the registered id is absent.
func TestUseSelectAlias(t *testing.T) {
	for _, missing := range []bool{false, true} {
		s, out, stderr := service(t)
		r := Empty()
		r.Managers["board"] = entry(t)
		path := filepath.Join(s.Resolver.Locations.User, Filename)
		var canonical []byte
		for _, op := range []string{"use", "select"} {
			putRegistry(t, s.Resolver.Locations.User, r)
			out.Reset()
			stderr.Reset()
			id := "board"
			if missing {
				id = "missing"
			}
			err := s.Run(Command{Operation: op, ID: id})
			if missing {
				requireCode(t, err, CodeNotRegistered, "")
			} else if err != nil {
				t.Fatal(err)
			}
			data, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			if op == "use" {
				canonical = data
				if stderr.Len() != 0 {
					t.Fatal(stderr.String())
				}
			} else {
				if !bytes.Equal(canonical, data) {
					t.Fatal("alias effect differs")
				}
				const want = "curator-run: session-manager select is deprecated in 0.6.0; use curator-run session-manager use instead (removal no earlier than 0.8.0)\n"
				if stderr.String() != want {
					t.Fatalf("alias warning=%q", stderr.String())
				}
			}
			if out.Len() != 0 {
				t.Fatalf("unexpected stdout %q", out.String())
			}
		}
	}
}

// Each mutation writes only its explicit layer, preserving the other registry
// byte-for-byte. Unscoped mutations retain system-writable/user-fallback policy.
func TestMutationScopeTargets(t *testing.T) {
	for _, op := range []string{"register", "use", "select", "unregister"} {
		for _, tc := range []struct {
			name, scope string
			writable    bool
			target      string
		}{
			{"explicit-system", "system", true, "system"},
			{"explicit-user", "user", true, "user"},
			{"default-system", "", true, "system"},
			{"default-user-absent-system", "", false, "user"},
		} {
			t.Run(op+"/"+tc.name, func(t *testing.T) {
				s, _, _ := service(t)
				s.CanWriteSystem = func(string) bool { return tc.writable }
				r := Empty()
				r.Managers["board"] = entry(t)
				r.Managers["peer"] = r.Managers["board"]
				r.Selected = "peer"
				target, other := s.Resolver.Locations.User, s.Resolver.Locations.System
				if tc.target == "system" {
					target, other = other, target
					putRegistry(t, other, r)
				}
				putRegistry(t, target, r)
				before, _ := os.ReadFile(filepath.Join(other, Filename))
				err := s.Run(Command{Operation: op, ID: "board", Manifest: manifest(t, "board", r.Managers["board"]), Scope: tc.scope, Select: true})
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(filepath.Join(target, Filename))
				if err != nil {
					t.Fatal(err)
				}
				got, err := Parse(data)
				if err != nil {
					t.Fatal(err)
				} // File schema remains closed and unchanged.
				if op == "unregister" {
					if _, ok := got.Managers["board"]; ok {
						t.Fatal("not removed")
					}
				} else if got.Selected != "board" {
					t.Fatal("not selected")
				}
				after, _ := os.ReadFile(filepath.Join(other, Filename))
				if !bytes.Equal(before, after) {
					t.Fatal("other registry changed")
				}
				if _, err = os.Stat(filepath.Join(other, Filename+".lock")); !os.IsNotExist(err) {
					t.Fatalf("lock outside target: %v", err)
				}
			})
		}
	}
}

// A read-only system target never falls back, with or without a system file.
// User scopes and unscoped read-only-system mutations refuse system shadowing
// before touching either lock or registry; a writable-system control succeeds.
func TestMutationScopeRefusals(t *testing.T) {
	for _, op := range []string{"register", "use", "select", "unregister"} {
		for _, tc := range []struct {
			name, scope string
			present     bool
			code        string
		}{
			{"system-absent", "system", false, CodeInvalid},
			{"system-present-readonly", "system", true, CodeInvalid},
			{"user-shadowed", "user", true, CodeShadowed},
			{"default-present-readonly", "", true, CodeShadowed},
		} {
			t.Run(op+"/"+tc.name, func(t *testing.T) {
				s, _, _ := service(t)
				r := Empty()
				r.Managers["board"] = entry(t)
				putRegistry(t, s.Resolver.Locations.User, r)
				if tc.present {
					putRegistry(t, s.Resolver.Locations.System, r)
				}
				dirs := []string{s.Resolver.Locations.User, s.Resolver.Locations.System}
				before := make([][]byte, 2)
				for i, dir := range dirs {
					before[i], _ = os.ReadFile(filepath.Join(dir, Filename))
				}
				c := Command{Operation: op, Scope: tc.scope, ID: "board", Manifest: manifest(t, "board", r.Managers["board"])}
				err := s.Run(c)
				reason := ""
				if tc.code == CodeInvalid {
					reason = "unreadable"
				}
				requireCode(t, err, tc.code, reason)
				if tc.code == CodeInvalid {
					e := err.(*Error)
					if e.Reason != "unreadable" || e.Path != filepath.Join(s.Resolver.Locations.System, Filename) {
						t.Fatal(e)
					}
				}
				for i, dir := range dirs {
					data, _ := os.ReadFile(filepath.Join(dir, Filename))
					if !bytes.Equal(before[i], data) {
						t.Fatal("refusal changed registry")
					}
					if _, err := os.Stat(filepath.Join(dir, Filename+".lock")); !os.IsNotExist(err) {
						t.Fatalf("refusal created lock: %v", err)
					}
				}
				if tc.scope == "system" {
					s.CanWriteSystem = func(string) bool { return true }
					if !tc.present {
						putRegistry(t, s.Resolver.Locations.System, r)
					}
					if err := s.Run(c); err != nil {
						t.Fatalf("writable control: %v", err)
					}
				}
			})
		}
	}
}

// Text and CLI JSON expose the actual read layer, including legacy files,
// machine shadowing and an absent registry. Registry encoding gets no layer key.
func TestDisplayLayers(t *testing.T) {
	for _, kind := range []string{"none", "user", "system", "legacy-user", "legacy-system"} {
		t.Run(kind, func(t *testing.T) {
			s, out, _ := service(t)
			r := Empty()
			r.Managers["board"] = entry(t)
			layer := "none"
			switch kind {
			case "user":
				layer = "user"
				putRegistry(t, s.Resolver.Locations.User, r)
			case "system":
				layer = "system"
				putRegistry(t, s.Resolver.Locations.System, r)
				putRegistry(t, s.Resolver.Locations.User, Empty())
			case "legacy-user":
				layer = "user"
				s.Resolver.Locations.LegacyUser = t.TempDir()
				putRegistry(t, s.Resolver.Locations.LegacyUser, r)
			case "legacy-system":
				layer = "system"
				s.Resolver.Locations.LegacySystem = t.TempDir()
				putRegistry(t, s.Resolver.Locations.LegacySystem, r)
			}
			for _, op := range []string{"list", "show"} {
				if kind == "none" && op == "show" {
					continue
				}
				for _, asJSON := range []bool{false, true} {
					out.Reset()
					if err := s.Run(Command{Operation: op, ID: "board", JSON: asJSON}); err != nil {
						t.Fatal(err)
					}
					if asJSON {
						var obj map[string]json.RawMessage
						if err := json.Unmarshal(out.Bytes(), &obj); err != nil {
							t.Fatal(err)
						}
						if string(obj["layer"]) != `"`+layer+`"` {
							t.Fatal(out.String())
						}
						delete(obj, "layer")
						if op == "list" {
							data, _ := json.Marshal(obj)
							if _, err := Parse(data); err != nil {
								t.Fatal("CLI-only member changed file schema", err)
							}
						}
					} else if !strings.HasPrefix(out.String(), "layer: "+layer+"\n") {
						t.Fatal(out.String())
					}
				}
			}
		})
	}
}
