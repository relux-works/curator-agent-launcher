package memberobjects

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// Real O_PATH root/object handles refuse at metadata preflight, before bytes.
// The object hook opens the actual owned fixture with the platform-only flags;
// it does not turn an incidental Pread failure into the rejection oracle.
func TestPlatformUnreadableHandles(t *testing.T) {
	f := newFixture(t, []byte("abc"))
	fd, e := unix.Open(f.path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if e != nil {
		t.Fatal(e)
	}
	bad := os.NewFile(uintptr(fd), "fixture-unreadable")
	defer bad.Close()
	s := track(t)
	c, err := acquire(bad, f.refs, s.ops)
	expectTag(t, c, err, "bootstrap/descriptor_invalid")
	s.assertClosed(t)
	if s.reads != 0 {
		t.Fatal("unreadable root content accessed")
	}
	if _, e := bad.Stat(); e != nil {
		t.Fatal("caller unreadable root closed")
	}
	s = track(t)
	native := s.ops.openAt
	s.ops.openAt = func(root int, name string, flags int, mode uint32) (int, error) {
		if name == "." {
			return native(root, name, flags, mode)
		}
		fd, e := unix.Openat(root, name, unix.O_PATH|unix.O_CLOEXEC, 0)
		if e == nil {
			s.fds = append(s.fds, fd)
		}
		return fd, e
	}
	c, err = acquire(f.dir, f.refs, s.ops)
	expectTag(t, c, err, "admission/input_mismatch")
	s.assertClosed(t)
	if s.reads != 0 {
		t.Fatal("unreadable object reached content read")
	}
	mustAcquire(t, f)
}
