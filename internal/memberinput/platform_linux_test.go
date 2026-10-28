package memberinput

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// Linux O_PATH is stat-able and regular but cannot read bytes; reject it during
// preflight, before reading the other otherwise-valid file.
func TestLinuxPathOnlyRefusal(t *testing.T) {
	n, a := newInput(t, []byte("N")), newInput(t, []byte("A"))
	fd, e := unix.Open(n.path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if e != nil {
		t.Fatal(e)
	}
	pathOnly := os.NewFile(uintptr(fd), "fixture-path-only")
	defer pathOnly.Close()
	rejectWithoutRead(t, pathOnly, a.file, "bootstrap/descriptor_invalid")
	rejectWithoutRead(t, a.file, pathOnly, "bootstrap/descriptor_invalid")
	if _, e := pathOnly.Stat(); e != nil {
		t.Fatal("caller path-only handle closed")
	}
	unchanged(t, n)
	unchanged(t, a)
	goodPair(t, n, a)
}
