//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package sessionmanager

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The real access probe refuses explicit system scope on a chmod-read-only
// directory without user fallback or locks; restoring write access permits it.
func TestSystemScopeReadOnlyDirectory(t *testing.T) {
	s, _, _ := service(t)
	s.CanWriteSystem = nil
	dir := s.Resolver.Locations.System
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
	r := Empty()
	r.Managers["board"] = entry(t)
	putRegistry(t, s.Resolver.Locations.User, r)
	userPath := filepath.Join(s.Resolver.Locations.User, Filename)
	before, err := os.ReadFile(userPath)
	if err != nil {
		t.Fatal(err)
	}
	c := Command{Operation: "register", Scope: "system", Manifest: manifest(t, "board", r.Managers["board"])}
	err = s.Run(c)
	requireCode(t, err, CodeInvalid, "unreadable")
	if err.(*Error).Reason != "unreadable" {
		t.Fatal(err)
	}
	after, err := os.ReadFile(userPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("user registry changed", err)
	}
	if _, err := os.Stat(filepath.Join(dir, Filename)); !os.IsNotExist(err) {
		t.Fatal("system registry created", err)
	}
	for _, d := range []string{dir, s.Resolver.Locations.User} {
		if _, err := os.Stat(filepath.Join(d, Filename+".lock")); !os.IsNotExist(err) {
			t.Fatal("refusal created lock", err)
		}
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.Run(c); err != nil {
		t.Fatal("writable control", err)
	}
}
