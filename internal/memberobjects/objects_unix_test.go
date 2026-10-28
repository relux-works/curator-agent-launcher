//go:build darwin || linux

package memberobjects

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

const abcDigest = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
const emptyDigest = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

type fixture struct {
	path string
	dir  *os.File
	refs []Reference
	raw  map[string][]byte
}

func newFixture(t *testing.T, raw ...[]byte) fixture {
	t.Helper()
	path := t.TempDir()
	if e := os.Chmod(path, 0700); e != nil {
		t.Fatal(e)
	}
	f := fixture{path: path, raw: map[string][]byte{}}
	for i, b := range raw {
		digest := fmt.Sprintf("%x", sha256.Sum256(b))
		name := fmt.Sprintf("object%d", i)
		if e := os.WriteFile(filepath.Join(path, digest), b, 0600); e != nil {
			t.Fatal(e)
		}
		if e := os.Chmod(filepath.Join(path, digest), 0600); e != nil {
			t.Fatal(e)
		}
		f.refs = append(f.refs, Reference{name, uint64(len(b)), digest})
		f.raw[digest] = bytes.Clone(b)
	}
	var e error
	f.dir, e = os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { f.dir.Close() })
	return f
}
func expectTag(t *testing.T, c *Collection, e *Error, tag string) {
	t.Helper()
	if c != nil || e == nil || e.Error() != tag || e.Stage()+"/"+e.Code() != tag || len(e.Error()) > 256 {
		t.Fatalf("want nil collection/%s, got collection=%t error=%v", tag, c != nil, e)
	}
}
func checkSnapshot(t *testing.T, s *Snapshot, b []byte) {
	t.Helper()
	if s == nil || s.Length() != uint64(len(b)) || s.SHA256() != sha256.Sum256(b) || !bytes.Equal(s.Raw(), b) {
		t.Fatal("independent immutable bytes/length/hash lost")
	}
}
func mustAcquire(t *testing.T, f fixture) *Collection {
	t.Helper()
	c, e := Acquire(f.dir, f.refs)
	if e != nil || c == nil {
		t.Fatalf("valid collection refused: %v", e)
	}
	t.Cleanup(func() {
		if e := c.Close(); e != nil {
			t.Error(e)
		}
	})
	for _, r := range f.refs {
		s, e := c.Snapshot(r.Name)
		if e != nil {
			t.Fatal(e)
		}
		checkSnapshot(t, s, f.raw[r.SHA256])
	}
	return c
}

// Every descriptor recorded here comes from the real production openat path.
// CLOEXEC and eventual EBADF are native observations, not an implementation leak
// counter or a scan of unrelated descriptors. No fixture opens occur between
// the operation under test and assertClosed.
type tracked struct {
	fds   []int
	reads int
	ops   operations
}

func track(t *testing.T) *tracked {
	t.Helper()
	s := &tracked{ops: nativeOperations()}
	s.ops.openAt = func(fd int, name string, flags int, mode uint32) (int, error) {
		if flags&unix.O_CLOEXEC == 0 || flags&unix.O_NOFOLLOW == 0 {
			t.Fatal("unsafe created descriptor flags")
		}
		if name != "." && flags&unix.O_NONBLOCK == 0 {
			t.Fatal("special file open could block")
		}
		n, e := unix.Openat(fd, name, flags, mode)
		if e == nil {
			s.fds = append(s.fds, n)
			fl, e := unix.FcntlInt(uintptr(n), unix.F_GETFD, 0)
			if e != nil || fl&unix.FD_CLOEXEC == 0 {
				t.Fatal("owned descriptor not actually CLOEXEC")
			}
		}
		return n, e
	}
	s.ops.readAt = func(fd int, b []byte, off int64) (int, error) { s.reads++; return unix.Pread(fd, b, off) }
	return s
}
func (s *tracked) assertClosed(t *testing.T) {
	t.Helper()
	for _, fd := range s.fds {
		if _, e := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); e != unix.EBADF {
			t.Fatalf("owned descriptor %d survived: %v", fd, e)
		}
	}
}

type callerState struct {
	pos            int64
	flags, fdflags int
	observed       observation
}

func saveCaller(t *testing.T, f *os.File) callerState {
	t.Helper()
	fd := int(f.Fd())
	pos, e := unix.Seek(fd, 0, io.SeekCurrent)
	if e != nil {
		t.Fatal(e)
	}
	flags, e := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if e != nil {
		t.Fatal(e)
	}
	ff, e := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	if e != nil {
		t.Fatal(e)
	}
	o, err := observe(fd)
	if err != nil {
		t.Fatal(err)
	}
	return callerState{pos, flags, ff, o}
}
func checkCaller(t *testing.T, f *os.File, s callerState) {
	t.Helper()
	if now := saveCaller(t, f); now != s {
		t.Fatalf("caller handle/offset/flags/metadata changed: before=%+v after=%+v", s, now)
	}
}
func refuse(t *testing.T, f fixture, tag string) {
	t.Helper()
	before := saveCaller(t, f.dir)
	s := track(t)
	c, e := acquire(f.dir, f.refs, s.ops)
	expectTag(t, c, e, tag)
	s.assertClosed(t)
	checkCaller(t, f.dir, before)
}

// Real files prove independent fixed bytes, aliases counted once, explicit owned
// custody/CLOEXEC, shared caller offsets and immutable returned views. Successful
// recheck remains only a cooperative observation; it is not admission or freeze.
func TestSnapshotsAliasesAndLifetime(t *testing.T) {
	f := newFixture(t, []byte("abc"), nil)
	if f.refs[0].SHA256 != abcDigest || f.refs[1].SHA256 != emptyDigest {
		t.Fatal("fixed independent fixture oracle mismatch")
	}
	f.refs = append(f.refs, Reference{"alias", 3, abcDigest})
	if _, e := f.dir.Readdirnames(1); e != nil {
		t.Fatal(e)
	}
	if _, e := unix.FcntlInt(f.dir.Fd(), unix.F_SETFD, 0); e != nil {
		t.Fatal(e)
	}
	before := saveCaller(t, f.dir)
	if before.pos == 0 {
		t.Fatal("directory offset witness was not advanced")
	}
	dup, e := unix.FcntlInt(f.dir.Fd(), unix.F_DUPFD_CLOEXEC, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer unix.Close(dup)
	s := track(t)
	c, err := acquire(f.dir, f.refs, s.ops)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if len(c.custody.objects) != 2 {
		t.Fatal("aliases created duplicate owned objects")
	}
	for _, o := range c.custody.objects {
		if _, e := unix.Seek(o.fd, 17, io.SeekStart); e != nil {
			t.Fatal(e)
		}
	}
	snap, err := c.Snapshot("object0")
	if err != nil {
		t.Fatal(err)
	}
	early := snap.Raw()
	returned := snap.Raw()
	returned[0] = 'x'
	f.refs[0] = Reference{"changed", 0, emptyDigest}
	rs, err := c.References()
	if err != nil {
		t.Fatal(err)
	}
	rs[0].Name = "changed"
	again, err := c.References()
	if err != nil || again[0].Name != "object0" {
		t.Fatal("caller projection mutation reached retained refs")
	}
	alias, err := c.Snapshot("alias")
	if err != nil {
		t.Fatal(err)
	}
	alias.Raw()[0] = 'y'
	checkSnapshot(t, snap, []byte("abc"))
	checkSnapshot(t, alias, []byte("abc"))
	if !bytes.Equal(early, []byte("abc")) {
		t.Fatal("earlier snapshot changed")
	}
	readsBefore := s.reads
	if e := c.Recheck(); e != nil {
		t.Fatal(e)
	}
	if s.reads <= readsBefore {
		t.Fatal("recheck did not actually reread retained objects")
	}
	for _, o := range c.custody.objects {
		pos, e := unix.Seek(o.fd, 0, io.SeekCurrent)
		if e != nil || pos != 17 {
			t.Fatal("shared file offset changed")
		}
	}
	checkCaller(t, f.dir, before)
	pos, e := unix.Seek(dup, 0, io.SeekCurrent)
	if e != nil || pos != before.pos {
		t.Fatal("shared directory offset changed")
	}
	if e := f.dir.Close(); e != nil {
		t.Fatal(e)
	} // The owned anchor survives caller closure.
	if e := c.Recheck(); e != nil {
		t.Fatal(e)
	}
	if e := c.Close(); e != nil {
		t.Fatal(e)
	}
	s.assertClosed(t)
	if e := c.Close(); e != nil {
		t.Fatal(e)
	}
	s.assertClosed(t)
	if e := c.Recheck(); e == nil || e.Error() != "admission/input_mismatch" {
		t.Fatal("closed custody revalidated")
	}
	if got, e := c.Snapshot("object0"); got != nil || e == nil || e.Error() != "admission/input_mismatch" {
		t.Fatal("closed custody consumed")
	}
	if got, e := c.References(); got != nil || e == nil {
		t.Fatal("closed custody returned refs")
	}
	checkSnapshot(t, snap, []byte("abc")) // Prior bytes are observations only.
}

// Inclusive count/object/unique-total limits have real positives. Empty O is an
// explicit raw-collection positive, not a valid request/admission by itself.
func TestInclusiveCapsAndEmpty(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		f := newFixture(t)
		c := mustAcquire(t, f)
		if e := c.Recheck(); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("128-alias-names", func(t *testing.T) {
		f := newFixture(t, []byte("abc"))
		f.refs = nil
		for i := 0; i < NameLimit; i++ {
			f.refs = append(f.refs, Reference{fmt.Sprintf("n%d", i), 3, abcDigest})
		}
		f.refs[0].Name = "A" + strings.Repeat("._-", 21)
		mustAcquire(t, f)
	})
	t.Run("4MiB-each-16MiB-total", func(t *testing.T) {
		raw := make([][]byte, 4)
		for i := range raw {
			raw[i] = bytes.Repeat([]byte{byte(i)}, ObjectLimit)
		}
		f := newFixture(t, raw...)
		c := mustAcquire(t, f)
		if e := c.Recheck(); e != nil {
			t.Fatal(e)
		}
	})
}

// Invalid typed declarations refuse before ANY root open or content read, with
// exact tags and nil result. UINT64_MAX and total limits cannot wrap into small
// allocations. Distinct valid names may legitimately alias one digest.
func TestProjectionRefusalsBeforeAccess(t *testing.T) {
	good := Reference{"ok", 3, abcDigest}
	cases := map[string]struct {
		refs []Reference
		tag  string
	}{}
	for _, name := range []string{"", ".", "../a", "a/b", "a\\b", "é", strings.Repeat("a", 65), "-a", "_a", "a\x00b", "a\nb"} {
		r := good
		r.Name = name
		cases["name-"+name] = struct {
			refs []Reference
			tag  string
		}{[]Reference{r}, "admission/input_mismatch"}
	}
	for _, digest := range []string{"", strings.ToUpper(abcDigest), abcDigest[:63], abcDigest + "0", strings.Repeat("g", 64), "../" + abcDigest[3:]} {
		r := good
		r.SHA256 = digest
		cases["digest-"+digest] = struct {
			refs []Reference
			tag  string
		}{[]Reference{r}, "admission/input_mismatch"}
	}
	cases["duplicate-name"] = struct {
		refs []Reference
		tag  string
	}{[]Reference{good, good}, "admission/input_mismatch"}
	cases["conflicting-length"] = struct {
		refs []Reference
		tag  string
	}{[]Reference{good, {"other", 2, abcDigest}}, "admission/input_mismatch"}
	for _, n := range []uint64{ObjectLimit + 1, ^uint64(0)} {
		r := good
		r.Length = n
		cases[fmt.Sprint("length-", n)] = struct {
			refs []Reference
			tag  string
		}{[]Reference{r}, "parse/bounds_exceeded"}
	}
	many := make([]Reference, NameLimit+1)
	for i := range many {
		many[i] = Reference{fmt.Sprintf("n%d", i), 3, abcDigest}
	}
	cases["name-count"] = struct {
		refs []Reference
		tag  string
	}{many, "parse/bounds_exceeded"}
	total := make([]Reference, 5)
	for i := range total {
		total[i] = Reference{fmt.Sprintf("n%d", i), ObjectLimit, fmt.Sprintf("%064x", i)}
	}
	total[4].Length = 1 // Exactly one byte over the unique-total ceiling.
	cases["unique-total"] = struct {
		refs []Reference
		tag  string
	}{total, "parse/bounds_exceeded"}
	f := newFixture(t, []byte("abc"))
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := track(t)
			c, e := acquire(f.dir, tc.refs, s.ops)
			expectTag(t, c, e, tc.tag)
			if len(s.fds) != 0 || s.reads != 0 {
				t.Fatal("invalid projection accessed objects/root")
			}
		})
	}
	mustAcquire(t, f)
}

// Actual invalid root handles/modes refuse before content. Wrong-owner/access
// predicates below are synthetic only: no chown or writable-directory invention.
func TestRootContractRefusals(t *testing.T) {
	f := newFixture(t, []byte("abc"))
	file, e := os.Open(filepath.Join(f.path, abcDigest))
	if e != nil {
		t.Fatal(e)
	}
	defer file.Close()
	closed, e := os.Open(f.path)
	if e != nil {
		t.Fatal(e)
	}
	closed.Close()
	rw, e := os.OpenFile(filepath.Join(f.path, abcDigest), os.O_RDWR, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer rw.Close()
	wo, e := os.OpenFile(filepath.Join(f.path, abcDigest), os.O_WRONLY, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer wo.Close()
	rd, wr, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer rd.Close()
	defer wr.Close()
	for _, bad := range []*os.File{nil, closed, file, rw, wo, rd, wr} {
		s := track(t)
		c, e := acquire(bad, f.refs, s.ops)
		expectTag(t, c, e, "bootstrap/descriptor_invalid")
		s.assertClosed(t)
		if s.reads != 0 {
			t.Fatal("invalid root read contents")
		}
		if bad != nil && bad != closed {
			if _, e := bad.Stat(); e != nil {
				t.Fatal("caller invalid handle closed")
			}
		}
	}
	for _, mode := range []os.FileMode{0750, 0701, 0000, 0700 | os.ModeSticky, 0700 | os.ModeSetuid, 0700 | os.ModeSetgid} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			if e := os.Chmod(f.path, mode); e != nil {
				t.Fatal(e)
			}
			defer os.Chmod(f.path, 0700)
			info, e := f.dir.Stat()
			if e != nil {
				t.Fatal(e)
			}
			if info.Mode().Perm() != mode.Perm() || info.Mode()&(os.ModeSticky|os.ModeSetuid|os.ModeSetgid) != mode&(os.ModeSticky|os.ModeSetuid|os.ModeSetgid) {
				t.Skip("fixture filesystem did not retain requested special bits")
			}
			refuse(t, f, "bootstrap/descriptor_invalid")
		})
	}
	mustAcquire(t, f)
}

// Exact flatness and no-symlink/nonblocking opening use real owned fixtures.
// Refusals leave the caller and deliberate fixture state intact, with every
// production-created descriptor natively closed; nearby good O prevents blanket
// refusal. Opaque content is not archive or role-policy validation.
func TestObjectRefusals(t *testing.T) {
	socketParent := t.TempDir() // Short owned socket basename; digest link stays in O.
	cases := []string{"missing", "extra", "noncanonical", "subdirectory", "symlink", "fifo", "socket", "mode0644", "mode0000", "setuid", "setgid", "sticky", "hardlink", "short", "long", "hash", "actual-over-cap", "empty-extra"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, []byte("abc"))
			path := filepath.Join(f.path, abcDigest)
			external := filepath.Join(t.TempDir(), "owned-external")
			if e := os.WriteFile(external, []byte("outside"), 0600); e != nil {
				t.Fatal(e)
			}
			// Open before mode-negative chmod so preservation doesn't rely on reopening
			// a mode0000 path. This is a TEST-owned handle, never handed to production.
			witness, e := os.Open(path)
			if e != nil {
				t.Fatal(e)
			}
			defer witness.Close()
			tag := "admission/input_mismatch"
			switch name {
			case "missing":
				e = os.Remove(path)
			case "extra":
				e = os.WriteFile(filepath.Join(f.path, emptyDigest), nil, 0600)
			case "empty-extra":
				f.refs = nil
			case "noncanonical":
				e = os.Rename(path, filepath.Join(f.path, "not-a-digest"))
			case "subdirectory", "symlink", "fifo", "socket":
				if e = os.Remove(path); e != nil {
					t.Fatal(e)
				}
				switch name {
				case "subdirectory":
					e = os.Mkdir(path, 0700)
				case "symlink":
					e = os.Symlink(external, path)
				case "fifo":
					e = unix.Mkfifo(path, 0600)
				case "socket":
					var fd int
					fd, e = unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM, 0)
					if e == nil {
						unix.CloseOnExec(fd)
						defer unix.Close(fd)
						socketPath := filepath.Join(socketParent, "s")
						e = unix.Bind(fd, &unix.SockaddrUnix{Name: socketPath})
						if e == nil {
							e = os.Link(socketPath, path)
						}
					}
				}
			case "mode0644":
				e = os.Chmod(path, 0644)
			case "mode0000":
				e = os.Chmod(path, 0000)
			case "setuid", "setgid", "sticky":
				bit := os.ModeSetuid
				if name == "setgid" {
					bit = os.ModeSetgid
				}
				if name == "sticky" {
					bit = os.ModeSticky
				}
				e = os.Chmod(path, 0600|bit)
				if e == nil {
					info, x := os.Stat(path)
					if x != nil {
						t.Fatal(x)
					}
					if info.Mode()&bit == 0 {
						t.Skip("fixture filesystem did not retain requested special bit")
					}
				}
			case "hardlink":
				e = os.Link(path, external+"-link")
			case "short":
				e = os.Truncate(path, 2)
			case "long":
				e = os.WriteFile(path, []byte("abcd"), 0600)
			case "hash":
				e = os.WriteFile(path, []byte("xyz"), 0600)
			case "actual-over-cap":
				e = os.Truncate(path, ObjectLimit+1)
				tag = "parse/bounds_exceeded"
			}
			if e != nil {
				t.Fatal(e)
			}
			info, e := os.Lstat(path)
			if e != nil && !os.IsNotExist(e) {
				t.Fatal(e)
			}
			before, _ := observe(int(witness.Fd()))
			original := make([]byte, int(before.size)+1)
			n, readErr := witness.ReadAt(original, 0)
			if readErr != nil && readErr != io.EOF {
				t.Fatal(readErr)
			}
			original = original[:n]
			refuse(t, f, tag)
			after, _ := observe(int(witness.Fd()))
			if before != after {
				t.Fatal("fixture metadata repaired/mutated")
			}
			retained := make([]byte, len(original)+1)
			n, readErr = witness.ReadAt(retained, 0)
			if (readErr != nil && readErr != io.EOF) || n != len(original) || !bytes.Equal(retained[:n], original) {
				t.Fatal("fixture bytes repaired/mutated")
			}
			next, x := os.Lstat(path)
			if os.IsNotExist(e) != os.IsNotExist(x) || (info != nil && (next == nil || next.Mode() != info.Mode() || next.Size() != info.Size())) {
				t.Fatal("fixture entry repaired/removed")
			}
			b, e := os.ReadFile(external)
			if e != nil || !bytes.Equal(b, []byte("outside")) {
				t.Fatal("external symlink target changed")
			}
		})
	}
	f := newFixture(t, []byte("abc"))
	if e := os.Chmod(filepath.Join(f.path, abcDigest), 0400); e != nil {
		t.Fatal(e)
	}
	mustAcquire(t, f)
}

// Synthetic predicate negatives isolate UID and native-invalid metadata; they
// are NOT foreign-owner OS qualification. Real readonly metadata is the control.
func TestSyntheticMetadataPredicates(t *testing.T) {
	f := newFixture(t, []byte("abc"))
	c := mustAcquire(t, f)
	o := c.custody.objects[0].observed
	if e := validObject(o, uint32(os.Getuid()), 3); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []observation{func() observation { x := o; x.uid ^= 1; return x }(), func() observation { x := o; x.size = -1; return x }(), func() observation { x := o; x.flags = unix.O_WRONLY; return x }()} {
		if e := validObject(bad, uint32(os.Getuid()), 3); e == nil || e.Error() != "admission/input_mismatch" {
			t.Fatal("synthetic invalid metadata accepted")
		}
	}
	root, e := observe(int(f.dir.Fd()))
	if e != nil || !validRoot(root) {
		t.Fatal("real root positive failed")
	}
	root.uid ^= 1
	if validRoot(root) {
		t.Fatal("synthetic wrong-root-owner accepted")
	}
	root.uid ^= 1
	root.flags = unix.O_WRONLY
	if validRoot(root) {
		t.Fatal("synthetic writable-root flags accepted")
	}
}

// Real short/growth/same-size changes and a later read failure MUST fire through
// the production positional-read path. Exact static refusal, nil collection,
// native descriptor cleanup and no repair are required. Partial progress is a
// nearby positive, so a blanket read refusal cannot satisfy this group.
func TestReadFaultsAndAllOrNothing(t *testing.T) {
	for _, name := range []string{"short", "growth", "same-size-after-read", "later-error", "over-cap-growth"} {
		t.Run(name, func(t *testing.T) {
			raw := []byte("abc")
			if name == "over-cap-growth" {
				raw = bytes.Repeat([]byte("x"), ObjectLimit)
			}
			f := newFixture(t, raw, []byte("a"))
			ds := []declaration{}
			_, ds, _ = project(f.refs)
			path := filepath.Join(f.path, ds[0].digest)
			if name == "over-cap-growth" {
				f = newFixture(t, raw)
				path = filepath.Join(f.path, f.refs[0].SHA256)
			}
			if _, e := f.dir.Readdirnames(1); e != nil {
				t.Fatal(e)
			}
			before := saveCaller(t, f.dir)
			s := track(t)
			native := s.ops.readAt
			fired := false
			first := -1
			shared := -1
			sharedFlags := 0
			defer func() {
				if shared >= 0 {
					unix.Close(shared)
				}
			}()
			s.ops.readAt = func(fd int, b []byte, off int64) (int, error) {
				if first < 0 {
					first = fd
					if name == "later-error" {
						if _, e := unix.Seek(fd, 17, io.SeekStart); e != nil {
							t.Fatal(e)
						}
						var e error
						shared, e = unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
						if e != nil {
							t.Fatal(e)
						}
						sharedFlags, e = unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
						if e != nil {
							t.Fatal(e)
						}
					}
				}
				if !fired && ((name == "later-error" && fd != first) || (name == "same-size-after-read" && off > 0) || (name != "later-error" && name != "same-size-after-read")) {
					fired = true
					switch name {
					case "short":
						if e := os.Truncate(path, 2); e != nil {
							t.Fatal(e)
						}
					case "growth":
						if e := os.WriteFile(path, []byte("abcd"), 0600); e != nil {
							t.Fatal(e)
						}
					case "over-cap-growth":
						if e := os.Truncate(path, ObjectLimit+1); e != nil {
							t.Fatal(e)
						}
					case "same-size-after-read":
						if e := os.WriteFile(path, []byte("xyz"), 0600); e != nil {
							t.Fatal(e)
						}
						if e := os.Chtimes(path, time.Unix(123, 0), time.Unix(123, 0)); e != nil {
							t.Fatal(e)
						}
					case "later-error":
						return 0, errors.New("private-upstream-marker")
					}
				}
				return native(fd, b, off)
			}
			tag := "admission/input_mismatch"
			if name == "over-cap-growth" {
				tag = "parse/bounds_exceeded"
			}
			c, e := acquire(f.dir, f.refs, s.ops)
			expectTag(t, c, e, tag)
			if !fired {
				t.Fatal("intended fault did not fire")
			}
			s.assertClosed(t)
			if shared >= 0 {
				pos, e := unix.Seek(shared, 0, io.SeekCurrent)
				flags, x := unix.FcntlInt(uintptr(shared), unix.F_GETFL, 0)
				if e != nil || x != nil || pos != 17 || flags != sharedFlags {
					t.Fatal("shared file offset/flags/open state changed on failure")
				}
			}
			checkCaller(t, f.dir, before)
			if name == "short" || name == "growth" || name == "same-size-after-read" {
				b, e := os.ReadFile(path)
				want := map[string]string{"short": "ab", "growth": "abcd", "same-size-after-read": "xyz"}[name]
				if e != nil || string(b) != want {
					t.Fatal("fault state repaired or missing")
				}
			}
			if name == "over-cap-growth" {
				info, e := os.Stat(path)
				if e != nil || info.Size() != ObjectLimit+1 {
					t.Fatal("growth repaired")
				}
			}
		})
	}
	t.Run("later-invalid-object", func(t *testing.T) {
		f := newFixture(t, []byte("abc"), []byte("a"))
		if e := os.Chmod(filepath.Join(f.path, f.refs[1].SHA256), 0644); e != nil {
			t.Fatal(e)
		}
		s := track(t)
		c, e := acquire(f.dir, f.refs, s.ops)
		expectTag(t, c, e, "admission/input_mismatch")
		if s.reads == 0 {
			t.Fatal("did not exercise cleanup after earlier object read")
		}
		s.assertClosed(t)
	})
	t.Run("partial-progress-positive", func(t *testing.T) {
		f := newFixture(t, []byte("abc"))
		s := track(t)
		native := s.ops.readAt
		s.ops.readAt = func(fd int, b []byte, off int64) (int, error) {
			if len(b) > 1 {
				b = b[:1]
			}
			return native(fd, b, off)
		}
		c, e := acquire(f.dir, f.refs, s.ops)
		if e != nil {
			t.Fatal(e)
		}
		snap, e := c.Snapshot("object0")
		if e != nil {
			t.Fatal(e)
		}
		checkSnapshot(t, snap, []byte("abc"))
		if s.reads < 4 {
			t.Fatal("partial progress path not reached")
		}
		c.Close()
		s.assertClosed(t)
	})
}

// Recheck rejects observed real file/namespace changes and disposes all live
// custody, while original byte observations survive unchanged. It never repairs
// or adopts the changed input; unchanged recheck and stable offsets are controls.
func TestConsumingRecheckAndFailureDisposal(t *testing.T) {
	for _, name := range []string{"bytes", "rename-replace", "extra", "mode", "root-mode", "hardlink", "missing"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, []byte("abc"))
			s := track(t)
			c, e := acquire(f.dir, f.refs, s.ops)
			if e != nil {
				t.Fatal(e)
			}
			snap, e := c.Snapshot("object0")
			if e != nil {
				t.Fatal(e)
			}
			early := snap.Raw()
			if e := c.Recheck(); e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(f.path, abcDigest)
			other := filepath.Join(t.TempDir(), "moved")
			switch name {
			case "bytes":
				if e := os.WriteFile(path, []byte("xyz"), 0600); e != nil {
					t.Fatal(e)
				}
			case "rename-replace":
				if e := os.Rename(path, other); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(path, []byte("abc"), 0600); e != nil {
					t.Fatal(e)
				}
			case "extra":
				if e := os.WriteFile(filepath.Join(f.path, emptyDigest), nil, 0600); e != nil {
					t.Fatal(e)
				}
			case "mode":
				if e := os.Chmod(path, 0644); e != nil {
					t.Fatal(e)
				}
			case "root-mode":
				if e := os.Chmod(f.path, 0750); e != nil {
					t.Fatal(e)
				}
			case "hardlink":
				if e := os.Link(path, other); e != nil {
					t.Fatal(e)
				}
			case "missing":
				if e := os.Remove(path); e != nil {
					t.Fatal(e)
				}
			}
			before := saveCaller(t, f.dir)
			if e := c.Recheck(); e == nil || e.Error() != "admission/input_mismatch" {
				t.Fatalf("changed custody accepted: %v", e)
			}
			s.assertClosed(t)
			checkCaller(t, f.dir, before)
			if e := c.Close(); e != nil {
				t.Fatal(e)
			}
			s.assertClosed(t)
			checkSnapshot(t, snap, []byte("abc"))
			if !bytes.Equal(early, []byte("abc")) {
				t.Fatal("earlier snapshot replaced")
			}
			if next, e := c.Snapshot("object0"); next != nil || e == nil {
				t.Fatal("failed custody still consumable")
			}
			if name == "bytes" {
				b, e := os.ReadFile(path)
				if e != nil || string(b) != "xyz" {
					t.Fatal("input was repaired")
				}
			}
			if name == "rename-replace" {
				b, e := os.ReadFile(other)
				if e != nil || string(b) != "abc" {
					t.Fatal("retained old source altered")
				}
			}
		})
	}
}

// Ambiguous close failure still closes remaining owned handles and permanently
// invalidates custody. Repeated Close cannot close a newly reused FD number.
// The injected error is AFTER a real close; no real resource is deliberately leaked.
func TestCloseFailureAndFDReuse(t *testing.T) {
	f := newFixture(t, []byte("abc"))
	s := track(t)
	calls := 0
	s.ops.closeFD = func(fd int) error {
		calls++
		e := unix.Close(fd)
		if e != nil {
			return e
		}
		if calls == 1 {
			return unix.EINTR
		}
		return nil
	}
	c, e := acquire(f.dir, f.refs, s.ops)
	if e != nil {
		t.Fatal(e)
	}
	if e := c.Close(); e == nil || e.Error() != "admission/input_mismatch" {
		t.Fatal("ambiguous close not reported")
	}
	s.assertClosed(t)
	n := calls
	fd, e2 := unix.Open(f.path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e2 != nil {
		t.Fatal(e2)
	}
	defer unix.Close(fd)
	reused := false
	for _, old := range s.fds {
		if fd == old {
			reused = true
		}
	}
	if !reused {
		t.Fatal("owned FD reuse witness not reached")
	}
	if e := c.Close(); e == nil || e.Error() != "admission/input_mismatch" || calls != n {
		t.Fatal("Close retried an ambiguous descriptor")
	}
	if _, e := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); e != nil {
		t.Fatal("reused caller descriptor closed")
	}
	if e := c.Recheck(); e == nil {
		t.Fatal("partially closed custody revalidated")
	}
}

// Per-call native-boundary errors expose only static tags, with no partial
// collection or surviving owned descriptors. Recheck read errors invalidate an
// already acquired collection without changing its earlier byte observation.
func TestOpenAndRecheckReadErrors(t *testing.T) {
	for _, where := range []string{"root", "enumeration", "object"} {
		t.Run(where, func(t *testing.T) {
			f := newFixture(t, []byte("abc"))
			s := track(t)
			native := s.ops.openAt
			calls := 0
			fired := false
			s.ops.openAt = func(fd int, name string, flags int, mode uint32) (int, error) {
				calls++
				if (where == "root" && calls == 1) || (where == "enumeration" && calls == 2) || (where == "object" && name != ".") {
					fired = true
					return -1, errors.New("private-open-marker")
				}
				return native(fd, name, flags, mode)
			}
			tag := "admission/input_mismatch"
			if where == "root" {
				tag = "bootstrap/descriptor_invalid"
			}
			before := saveCaller(t, f.dir)
			c, e := acquire(f.dir, f.refs, s.ops)
			expectTag(t, c, e, tag)
			if !fired {
				t.Fatal("open fault not reached")
			}
			s.assertClosed(t)
			checkCaller(t, f.dir, before)
		})
	}
	f := newFixture(t, []byte("abc"))
	s := track(t)
	c, e := acquire(f.dir, f.refs, s.ops)
	if e != nil {
		t.Fatal(e)
	}
	old, e := c.Snapshot("object0")
	if e != nil {
		t.Fatal(e)
	}
	fired := false
	c.custody.ops.readAt = func(int, []byte, int64) (int, error) { fired = true; return 0, errors.New("private-recheck-marker") }
	if e := c.Recheck(); e == nil || e.Error() != "admission/input_mismatch" || !fired {
		t.Fatal("recheck read error not rejected")
	}
	s.assertClosed(t)
	checkSnapshot(t, old, []byte("abc"))
	if e := c.Close(); e != nil {
		t.Fatal(e)
	}
	mustAcquire(t, f)
}
