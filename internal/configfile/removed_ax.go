package configfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// AXPaths lists retired policy locations independently; it never selects a
// fallback or gives a current policy precedence over a legacy leftover.
func (l Locations) AXPaths() []string {
	var paths []string
	for _, dir := range []string{l.System, l.User, l.LegacySystem, l.LegacyUser} {
		if dir != "" {
			paths = append(paths, filepath.Join(dir, "ax.json"))
		}
	}
	return paths
}

// RemovedAX checks existence only. Even disabled, malformed, dangling or
// non-regular leftovers must never silently become a native launch.
func RemovedAX(l Locations) (string, error) {
	for _, path := range l.AXPaths() {
		if _, err := os.Lstat(path); err == nil {
			return path, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect retired policy path %q: %w", path, err)
		}
	}
	return "", nil
}
