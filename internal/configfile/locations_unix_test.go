//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package configfile

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// Both selected homes refuse a foreign-owned opened file. The positive control
// uses identical ordinary bytes under the fixture owner's identity. Non-root
// hosts that disallow hard links to public root files cannot run this fixture.
func TestResolverForeignOwnerBothHomes(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "old"}[legacy], func(t *testing.T) {
			r := resolverFixture(t)
			current, old := r.Locations.Directories(User)
			dir := current
			if legacy {
				dir = old
			}
			path := filepath.Join(dir, "defaults.json")
			file(t, path, "control")
			b, p, _, err := r.Read(User, "defaults.json")
			if err != nil || !p || string(b) != "control" {
				t.Fatal("positive", err)
			}
			if os.Geteuid() == 0 {
				if err := os.Chown(path, 65534, -1); err != nil {
					t.Skip(err)
				}
			} else {
				os.Remove(path)
				if err := os.Link("/etc/hosts", path); err != nil {
					t.Skip("foreign-owner fixture unavailable:", err)
				}
			}
			_, _, selected, err := r.Read(User, "defaults.json")
			if err == nil || !strings.Contains(err.Error(), "different identity") || selected != path {
				t.Fatalf("%s %v", selected, err)
			}
		})
	}
}

type ownerInfo struct {
	os.FileInfo
	uid uint32
}

func (i ownerInfo) Sys() any { return &syscall.Stat_t{Uid: i.uid} }

// The metadata gate rejects differing owners independently of platform ability
// to create foreign files; matching owners with safe modes remain valid.
func TestOpenedOwnerGate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	file(t, path, "control")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateFile(path, nil, ownerInfo{info, 1}, ownerInfo{info, 1}); err != nil {
		t.Fatal("positive", err)
	}
	if err := validateFile(path, nil, ownerInfo{info, 2}, ownerInfo{info, 1}); err == nil || !strings.Contains(err.Error(), "different identity") {
		t.Fatal(err)
	}
}
