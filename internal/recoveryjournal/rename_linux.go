//go:build linux

package recoveryjournal

import "golang.org/x/sys/unix"

func renameNew(dir int, old, next string) error {
	return unix.Renameat2(dir, old, dir, next, unix.RENAME_NOREPLACE)
}
