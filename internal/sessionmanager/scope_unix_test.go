//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package sessionmanager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// A valid registry remains readable without directory write access. Explicit
// scopes diagnose write authority before locks/effects; restoring permissions
// permits the same use, while corrupt content still refuses as registry_invalid.
func TestExplicitScopeReadOnlyDirectory(t *testing.T) {
	for _, scope := range []string{"system", "user"} {
		t.Run(scope, func(t *testing.T) {
			s, out, _ := service(t)
			s.CanWriteSystem = nil
			dir, other := s.Resolver.Locations.System, s.Resolver.Locations.User
			if scope == "user" {
				dir, other = other, dir
			}
			r := Empty()
			r.Managers["board"] = entry(t)
			r.Managers["peer"] = r.Managers["board"]
			r.Selected = "peer"
			putRegistry(t, dir, r)
			if scope == "system" {
				putRegistry(t, other, r)
			}
			before, err := os.ReadFile(filepath.Join(dir, Filename))
			if err != nil {
				t.Fatal(err)
			}
			otherBefore, err := os.ReadFile(filepath.Join(other, Filename))
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, 0500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.Chmod(dir, 0700); err != nil {
					t.Error(err)
				}
			})
			if writableDirectory(dir) {
				t.Skip("identity can write chmod-read-only directories")
			}
			if err := s.Run(Command{Operation: "list", JSON: true}); err != nil {
				t.Fatal("valid registry is readable", err)
			}
			var view struct {
				Registry
				Layer string `json:"layer"`
			}
			if err := json.Unmarshal(out.Bytes(), &view); err != nil || view.Layer != scope || view.Selected != "peer" {
				t.Fatalf("readable control: %s, %v", out.String(), err)
			}
			c := Command{Operation: "use", Scope: scope, ID: "board"}
			err = s.Run(c)
			requireCode(t, err, "session_manager_registry_unwritable", "")
			want := fmt.Sprintf("session-manager registry directory is not writable path=%q scope=%s", dir, scope)
			if err.Error() != want {
				t.Fatalf("write-authority diagnostic=%q, want %q", err, want)
			}
			for i, d := range []string{dir, other} {
				data, err := os.ReadFile(filepath.Join(d, Filename))
				if err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				previous := before
				if i == 1 {
					previous = otherBefore
				}
				if !bytes.Equal(previous, data) {
					t.Fatal("refusal changed registry")
				}
				if _, err := os.Stat(filepath.Join(d, Filename+".lock")); !os.IsNotExist(err) {
					t.Fatal("refusal created lock", err)
				}
			}
			if scope == "user" {
				// Machine shadowing still wins even when the explicit user target
				// is unwritable, before inspecting that user destination.
				putRegistry(t, other, r)
				requireCode(t, s.Run(c), CodeShadowed, "")
				if err := os.Remove(filepath.Join(other, Filename)); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(filepath.Join(dir, Filename))
				if err != nil || !bytes.Equal(data, before) {
					t.Fatal("shadowed refusal changed user registry", err)
				}
				if _, err := os.Stat(filepath.Join(dir, Filename+".lock")); !os.IsNotExist(err) {
					t.Fatal("shadowed refusal created lock", err)
				}
			}
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := s.Run(c); err != nil {
				t.Fatal("writable control", err)
			}
			got := readTarget(t, s)
			if got.Selected != "board" {
				t.Fatal("writable control did not select")
			}
			corrupt := []byte("{")
			if err := os.WriteFile(filepath.Join(dir, Filename), corrupt, 0600); err != nil {
				t.Fatal(err)
			}
			requireCode(t, s.Run(c), CodeInvalid, "parse")
			data, err := os.ReadFile(filepath.Join(dir, Filename))
			if err != nil || !bytes.Equal(data, corrupt) {
				t.Fatal("corrupt-registry refusal changed bytes", err)
			}
			data, err = os.ReadFile(filepath.Join(other, Filename))
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if !bytes.Equal(data, otherBefore) {
				t.Fatal("control changed other layer")
			}
		})
	}
}
