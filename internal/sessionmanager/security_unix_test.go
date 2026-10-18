//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package sessionmanager

import (
	"os"
	"path/filepath"
	"testing"
)

// Both effective old/new registries retain owner and write-mode gates; the
// same bytes in an ordinary owned file are accepted before each violation.
func TestRegistrySecurityReasons(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, kind := range []string{"owner", "mode", "unreadable"} {
			t.Run(map[bool]string{false: "new", true: "old"}[legacy]+"/"+kind, func(t *testing.T) {
				s, _, _ := service(t)
				dir := s.Resolver.Locations.User
				if legacy {
					dir = s.Resolver.Locations.LegacyUser
				}
				putRegistry(t, dir, Empty())
				if _, _, err := Load(s.Resolver); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, Filename)
				switch kind {
				case "owner":
					if os.Geteuid() == 0 {
						if err := os.Chown(path, 65534, -1); err != nil {
							t.Skip(err)
						}
					} else {
						os.Remove(path)
						if err := os.Link("/etc/hosts", path); err != nil {
							t.Skip("foreign-owner hardlink unavailable:", err)
						}
					}
				case "mode":
					os.Chmod(path, 0620)
				case "unreadable":
					os.Remove(path)
					os.Mkdir(path, 0700)
				}
				_, _, err := Load(s.Resolver)
				requireCode(t, err, CodeInvalid, kind)
			})
		}
	}
}
