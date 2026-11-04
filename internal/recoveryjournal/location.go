package recoveryjournal

import "path/filepath"

// location routes only the exact namespace read. It is not a network binding
// and cannot authorize preparation, service contact or a different state root.
type location struct{ base string }

func resolveLocation(xdg, home string) (location, error) {
	base := xdg
	if base == "" {
		if !absolute(home) {
			return location{}, ErrBinding
		}
		base = filepath.Join(home, ".local", "state")
	}
	if !absolute(base) {
		return location{}, ErrBinding
	}
	return location{filepath.Clean(base)}, nil
}

func (l location) valid() bool { return absolute(l.base) && filepath.Clean(l.base) == l.base }
func (l location) directory() string {
	return filepath.Join(l.base, "swarma", "launcher", "netproxy-recovery", "v1")
}
