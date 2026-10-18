package migrate

import (
	"os"
	"path/filepath"
)

func writableDirectory(path string) bool {
	for {
		info, err := os.Stat(path)
		if err == nil {
			return info.IsDir() && info.Mode().Perm()&0200 != 0
		}
		if !os.IsNotExist(err) {
			return false
		}
		parent := filepath.Dir(path)
		if parent == path {
			return false
		}
		path = parent
	}
}
