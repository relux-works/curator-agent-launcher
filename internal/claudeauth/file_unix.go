//go:build darwin || linux

package claudeauth

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// Check and read one opened object: never stat a path and then reopen it.
func readFile(path string) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrRefused
	}
	f := os.NewFile(uintptr(fd), "oauth-store")
	defer f.Close()
	var before, after unix.Stat_t
	safe := func(s *unix.Stat_t) bool {
		return s.Mode&unix.S_IFMT == unix.S_IFREG && s.Mode&07777 == 0600 && s.Uid == uint32(os.Geteuid()) && s.Nlink == 1 && s.Size > 0 && s.Size <= maxBytes
	}
	if unix.Fstat(fd, &before) != nil || !safe(&before) {
		return nil, ErrRefused
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil || unix.Fstat(fd, &after) != nil || !safe(&after) ||
		before.Dev != after.Dev || before.Ino != after.Ino || before.Size != after.Size ||
		before.Mtim != after.Mtim || before.Ctim != after.Ctim || int64(len(raw)) != after.Size {
		clear(raw)
		return nil, ErrRefused
	}
	return raw, nil
}
