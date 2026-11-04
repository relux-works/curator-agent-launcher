//go:build darwin

package recoveryjournal

import "golang.org/x/sys/unix"

func renameNew(dir int, old, next string) error {
	return unix.RenameatxNp(dir, old, dir, next, unix.RENAME_EXCL)
}
