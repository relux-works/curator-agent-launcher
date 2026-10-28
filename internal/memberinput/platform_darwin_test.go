package memberinput

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// Darwin O_EVTONLY cannot supply file bytes despite read-only access-mode bits;
// reject before reading either input, with a normal readonly positive nearby.
func TestDarwinEventOnlyRefusal(t *testing.T) {
	n, a := newInput(t, []byte("N")), newInput(t, []byte("A"))
	fd, e := unix.Open(n.path, unix.O_EVTONLY|unix.O_CLOEXEC, 0)
	if e != nil {
		t.Fatal(e)
	}
	eventOnly := os.NewFile(uintptr(fd), "fixture-event-only")
	defer eventOnly.Close()
	rejectWithoutRead(t, eventOnly, a.file, "bootstrap/descriptor_invalid")
	rejectWithoutRead(t, a.file, eventOnly, "bootstrap/descriptor_invalid")
	if _, e := eventOnly.Stat(); e != nil {
		t.Fatal("caller event-only handle closed")
	}
	unchanged(t, n)
	unchanged(t, a)
	goodPair(t, n, a)
}
