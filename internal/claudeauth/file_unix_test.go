//go:build darwin || linux

package claudeauth

import (
	"context"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// A FIFO refuses without waiting for a writer; the valid regular-file control
// lives in TestPrivateFileChildOnly. This operates only on an isolated fixture.
func TestFIFORefuses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fifo")
	if err := unix.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	child, err := (Resolver{}).Inject(context.Background(), fileSelection(path), nil)
	if child != nil || err != ErrRefused {
		t.Fatal("FIFO accepted")
	}
}
