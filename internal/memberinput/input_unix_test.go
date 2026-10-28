//go:build darwin || linux

package memberinput

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type inputFixture struct {
	file   *os.File
	path   string
	raw    []byte
	offset int64
	mode   os.FileMode
}

func newInput(t *testing.T, raw []byte) inputFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	const offset = 17 // Beyond EOF is intentional for empty/small raw inputs.
	if _, err := f.Seek(offset, 0); err != nil {
		t.Fatal(err)
	}
	return inputFixture{f, path, bytes.Clone(raw), offset, 0600}
}
func unchanged(t *testing.T, in inputFixture) {
	t.Helper()
	info, err := in.file.Stat()
	if err != nil {
		t.Fatal("caller handle was closed", err)
	}
	if info.Mode() != in.mode {
		t.Fatal("input mode changed")
	}
	pos, err := in.file.Seek(0, 1)
	if err != nil || pos != in.offset {
		t.Fatal("shared input offset changed")
	}
	// Existing readable handles still read after a mode-negative fixture chmod;
	// reopening its path would incorrectly make that assertion depend on mode.
	raw := make([]byte, len(in.raw)+1)
	n, err := in.file.ReadAt(raw, 0)
	if (err != nil && err != io.EOF) || n != len(in.raw) || !bytes.Equal(raw[:n], in.raw) {
		t.Fatal("input content changed")
	}
}
func badPair(t *testing.T, pair *UntrustedPair, err *Error, tag string) {
	t.Helper()
	if pair != nil || err == nil || err.Error() != tag || err.Stage()+"/"+err.Code() != tag || len(err.Error()) > 256 {
		t.Fatalf("want no pair and static %s; got pair=%t error=%v", tag, pair != nil, err)
	}
}
func goodPair(t *testing.T, n, a inputFixture) *UntrustedPair {
	t.Helper()
	p, err := ReadPair(n.file, a.file)
	if err != nil || p == nil {
		t.Fatalf("valid distinct readonly pair refused: %v", err)
	}
	for i, raw := range [][]byte{n.raw, a.raw} {
		s := []*Snapshot{p.Request(), p.Authorization()}[i]
		if s.Length() != len(raw) || s.SHA256() != sha256.Sum256(raw) || !bytes.Equal(s.Raw(), raw) {
			t.Fatal("exact independent byte observation lost")
		}
	}
	unchanged(t, n)
	unchanged(t, a)
	return p
}
func rejectWithoutRead(t *testing.T, n, a *os.File, tag string) {
	t.Helper()
	calls := 0
	p, err := readPair(n, a, func(fd int, b []byte, off int64) (int, error) { calls++; return unix.Pread(fd, b, off) })
	badPair(t, p, err, tag)
	if calls != 0 {
		t.Fatal("content read before BOTH handle/size checks completed")
	}
}

// Real Darwin/Linux TempDir files preserve exact pinned N/A bytes, independent
// hash/length expectations, shared nonzero offsets and caller-open lifetimes.
// Expired A is only bytes here, never an authority or expiry witness.
func TestPublishedHandleSnapshots(t *testing.T) {
	aRaw, err := os.ReadFile("../../docs/same-user-v1/fixtures/authorization.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		file, digest string
		size         int
	}{
		{"submit.json", "3a5080f47c4d8af599c8ec654ae2d926924868938a882ed1c801b0978b928cbb", 3348},
		{"request-inspect.json", "6dd6628eb08395631cb0ac6641a954cf34e01d3a4acf9664d6e7adddf5fa3639", 178},
		{"request-cancel.json", "88cb5be74ff869be3933466b98bf46e1b177886acbbd2d762becaa5bf181d67f", 262},
		{"request-reconcile.json", "21fbe4e1ef4d9ac7f92ba563df1828f97f6c0a12af1af9a3c7cce4d723a439c2", 180},
		{"request-reserve.json", "feb1b2b8c4b02d3ef42c66fa6b97619e552979f3b94c41a9b0b43bf4c741d8fa", 392},
		{"request-ack.json", "f8e2762be1df6ec69da7c2fe4f7280dc0c94a7ad903741183bcd7a96467a85d3", 530},
	} {
		t.Run(c.file, func(t *testing.T) {
			raw, err := os.ReadFile("../../docs/same-user-v1/fixtures/" + c.file)
			if err != nil {
				t.Fatal(err)
			}
			n, a := newInput(t, raw), newInput(t, aRaw)
			// A third caller-owned descriptor shares N's open-file description/offset.
			dup, err := unix.FcntlInt(n.file.Fd(), unix.F_DUPFD_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			shared := os.NewFile(uintptr(dup), "fixture-shared-offset")
			defer shared.Close()
			// Caller CLOEXEC is not modified; only owned duplicates would need setting.
			if _, err := unix.FcntlInt(n.file.Fd(), unix.F_SETFD, 0); err != nil {
				t.Fatal(err)
			}
			p := goodPair(t, n, a)
			ns, as := p.Request().SHA256(), p.Authorization().SHA256()
			if p.Request().Length() != c.size || hex.EncodeToString(ns[:]) != c.digest || p.Authorization().Length() != 292 || hex.EncodeToString(as[:]) != "99965b37937380c1a38fd5e7070274f7a0b953b6b1f4b1a43e614e67584c27c4" {
				t.Fatal("published raw constants differ")
			}
			pos, err := shared.Seek(0, 1)
			if err != nil || pos != n.offset {
				t.Fatal("duplicate's shared offset changed")
			}
			flags, err := unix.FcntlInt(n.file.Fd(), unix.F_GETFD, 0)
			if err != nil || flags != 0 {
				t.Fatal("caller descriptor flags changed")
			}
			before := p.Request().Raw()
			changed := p.Request().Raw()
			changed[0] = '!'
			ar := p.Authorization().Raw()
			ar[0] = '?'
			raw[0] = '?'
			if err := n.file.Close(); err != nil {
				t.Fatal(err)
			}
			if err := a.file.Close(); err != nil {
				t.Fatal(err)
			}
			// Use frozen independent fixture bytes, not another potentially aliased view.
			if !bytes.Equal(p.Request().Raw(), n.raw) || !bytes.Equal(before, n.raw) || !bytes.Equal(p.Authorization().Raw(), a.raw) || p.Request().SHA256() != ns || p.Authorization().SHA256() != as {
				t.Fatal("snapshot depends on caller buffers or handle lifetime")
			}
		})
	}
}

// Raw byte ceilings are inclusive and independent of JSON structure. Empty and
// arbitrary non-JSON inputs succeed; one-over metadata refuses before ANY read.
func TestRawHandleBoundaries(t *testing.T) {
	for _, c := range []struct {
		name string
		n, a int
		bad  bool
	}{
		{"empty", 0, 0, false}, {"exact", RequestLimit, AuthorizationLimit, false},
		{"request-over", RequestLimit + 1, 1, true}, {"authorization-over", 1, AuthorizationLimit + 1, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			n, a := newInput(t, bytes.Repeat([]byte{'N'}, c.n)), newInput(t, bytes.Repeat([]byte{'A'}, c.a))
			if c.bad {
				rejectWithoutRead(t, n.file, a.file, "parse/bounds_exceeded")
			} else {
				goodPair(t, n, a)
			}
			unchanged(t, n)
			unchanged(t, a)
		})
	}
}

// Every invalid neighbor is an actual fixture handle, paired with a distinct
// valid file. Both argument positions must refuse before content reads and leave
// the good handle open/unchanged. Closed inputs remain closed, never repaired.
func TestHandleTypeAndAccessRefusals(t *testing.T) {
	makers := []struct {
		name string
		make func(*testing.T, inputFixture) *os.File
	}{
		{"nil", func(t *testing.T, _ inputFixture) *os.File { return nil }},
		{"closed", func(t *testing.T, in inputFixture) *os.File {
			f, e := os.Open(in.path)
			if e != nil {
				t.Fatal(e)
			}
			f.Close()
			return f
		}},
		{"write-only", func(t *testing.T, in inputFixture) *os.File {
			f, e := os.OpenFile(in.path, os.O_WRONLY, 0)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { f.Close() })
			return f
		}},
		{"read-write", func(t *testing.T, in inputFixture) *os.File {
			f, e := os.OpenFile(in.path, os.O_RDWR, 0)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { f.Close() })
			return f
		}},
		{"directory", func(t *testing.T, _ inputFixture) *os.File {
			f, e := os.Open(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { f.Close() })
			return f
		}},
		{"pipe-read", func(t *testing.T, _ inputFixture) *os.File {
			r, w, e := os.Pipe()
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { r.Close(); w.Close() })
			return r
		}},
		{"pipe-write", func(t *testing.T, _ inputFixture) *os.File {
			r, w, e := os.Pipe()
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { r.Close(); w.Close() })
			return w
		}},
		{"socket", func(t *testing.T, _ inputFixture) *os.File {
			fds, e := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
			if e != nil {
				t.Fatal(e)
			}
			var files [2]*os.File
			for i, fd := range fds {
				unix.CloseOnExec(fd)
				files[i] = os.NewFile(uintptr(fd), "fixture-socket")
			}
			t.Cleanup(func() { files[0].Close(); files[1].Close() })
			return files[0]
		}},
		{"fifo", func(t *testing.T, _ inputFixture) *os.File {
			path := filepath.Join(t.TempDir(), "fifo")
			if e := unix.Mkfifo(path, 0600); e != nil {
				t.Fatal(e)
			}
			f, e := os.OpenFile(path, os.O_RDONLY|unix.O_NONBLOCK, 0)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { f.Close() })
			return f
		}},
	}
	for _, c := range makers {
		for _, reverse := range []bool{false, true} {
			t.Run(c.name+map[bool]string{false: "-N", true: "-A"}[reverse], func(t *testing.T) {
				good, bad := newInput(t, []byte("good")), newInput(t, []byte("bad"))
				f := c.make(t, bad)
				wasOpen := false
				if f != nil {
					_, e := f.Stat()
					wasOpen = e == nil
				}
				if reverse {
					rejectWithoutRead(t, good.file, f, "bootstrap/descriptor_invalid")
				} else {
					rejectWithoutRead(t, f, good.file, "bootstrap/descriptor_invalid")
				}
				if f != nil {
					_, e := f.Stat()
					if (e == nil) != wasOpen {
						t.Fatal("invalid caller handle open-state changed")
					}
				}
				unchanged(t, good)
				unchanged(t, bad)
			})
		}
	}
	// A nearby valid pair ensures this group cannot pass by refusing all inputs.
	goodPair(t, newInput(t, []byte("N")), newInput(t, []byte("A")))
}

// Exact 0600 excludes extra permissions/special bits; link count and underlying
// identity (not descriptor number) are real OS observations. No chown is used.
func TestModesLinksAndAliases(t *testing.T) {
	for _, mode := range []os.FileMode{0, 0400, 0200, 0700, 0640, 0604, 0600 | os.ModeSetuid, 0600 | os.ModeSetgid, 0600 | os.ModeSticky} {
		t.Run(mode.String(), func(t *testing.T) {
			n, a := newInput(t, []byte("N")), newInput(t, []byte("A"))
			if err := os.Chmod(n.path, mode); err != nil {
				t.Fatal(err)
			}
			st, err := n.file.Stat()
			if err != nil {
				t.Fatal(err)
			}
			if st.Mode() != mode {
				t.Skip("fixture filesystem did not retain requested special permission bits")
			}
			n.mode = mode
			rejectWithoutRead(t, n.file, a.file, "bootstrap/descriptor_invalid")
			unchanged(t, n)
			unchanged(t, a)
		})
	}
	t.Run("hardlink", func(t *testing.T) {
		n, a := newInput(t, []byte("N")), newInput(t, []byte("A"))
		link := filepath.Join(filepath.Dir(n.path), "alias")
		if err := os.Link(n.path, link); err != nil {
			t.Fatal(err)
		}
		rejectWithoutRead(t, n.file, a.file, "bootstrap/descriptor_invalid")
		unchanged(t, n)
		unchanged(t, a)
		b, err := os.ReadFile(link)
		if err != nil || !bytes.Equal(b, n.raw) {
			t.Fatal("link changed")
		}
	})
	t.Run("same-file-object", func(t *testing.T) {
		n := newInput(t, []byte("N"))
		rejectWithoutRead(t, n.file, n.file, "bootstrap/descriptor_invalid")
		unchanged(t, n)
	})
	t.Run("duplicate-descriptor", func(t *testing.T) {
		n := newInput(t, []byte("N"))
		fd, err := unix.FcntlInt(n.file.Fd(), unix.F_DUPFD_CLOEXEC, 0)
		if err != nil {
			t.Fatal(err)
		}
		alias := os.NewFile(uintptr(fd), "fixture-duplicate")
		defer alias.Close()
		rejectWithoutRead(t, n.file, alias, "bootstrap/descriptor_invalid")
		unchanged(t, n)
		if _, err := alias.Stat(); err != nil {
			t.Fatal("caller duplicate closed")
		}
	})
	t.Run("separately-opened-alias", func(t *testing.T) {
		n := newInput(t, []byte("N"))
		alias, err := os.Open(n.path)
		if err != nil {
			t.Fatal(err)
		}
		defer alias.Close()
		rejectWithoutRead(t, n.file, alias, "bootstrap/descriptor_invalid")
		unchanged(t, n)
		pos, e := alias.Seek(0, 1)
		if e != nil || pos != 0 {
			t.Fatal("alias offset changed")
		}
	})
	goodPair(t, newInput(t, []byte("N")), newInput(t, []byte("N"))) // Equal bytes, distinct inodes are valid.
}

// This is a synthetic metadata predicate witness, NOT foreign-user/chown OS
// qualification. Other predicates stay valid so owner/negative size are isolated.
func TestSyntheticMetadataRefusals(t *testing.T) {
	n := newInput(t, []byte("N"))
	o, err := inspect(int(n.file.Fd()), RequestLimit)
	if err != nil {
		t.Fatal(err)
	}
	uid := uint32(os.Getuid())
	if e := validObservation(o, uid, RequestLimit); e != nil {
		t.Fatal("predicate positive refused")
	}
	for _, change := range []func(*observation){func(o *observation) { o.uid = uid ^ 1 }, func(o *observation) { o.size = -1 }} {
		bad := o
		change(&bad)
		e := validObservation(bad, uid, RequestLimit)
		if e == nil || e.Error() != "bootstrap/descriptor_invalid" {
			t.Fatal("synthetic invalid metadata accepted")
		}
	}
	unchanged(t, n)
}

// Deterministic per-call read seams delegate to real Pread and modify ONLY owned
// fixtures. They exercise production post-read/error paths, not a race timing or
// proof that every same-size transient writer can be detected.
func TestObservedChangesAndReadFailures(t *testing.T) {
	for _, name := range []string{"short-N", "growth-N", "growth-A", "same-size-N-after-read", "mode-N-after-read", "link-N-after-read", "read-error-A"} {
		t.Run(name, func(t *testing.T) {
			n, a := newInput(t, []byte("N-raw")), newInput(t, []byte("A-raw"))
			fired := false
			calls := 0
			p, e := readPair(n.file, a.file, func(fd int, b []byte, off int64) (int, error) {
				calls++
				target := int(n.file.Fd())
				if name == "growth-A" || name == "read-error-A" || name == "same-size-N-after-read" || name == "mode-N-after-read" || name == "link-N-after-read" {
					target = int(a.file.Fd())
				}
				if !fired && fd == target {
					fired = true
					switch name {
					case "short-N":
						if e := os.Truncate(n.path, 0); e != nil {
							t.Fatal(e)
						}
						n.raw = nil
					case "growth-N":
						n.raw = append(n.raw, '!')
						if e := os.WriteFile(n.path, n.raw, 0600); e != nil {
							t.Fatal(e)
						}
					case "growth-A":
						a.raw = append(a.raw, '!')
						if e := os.WriteFile(a.path, a.raw, 0600); e != nil {
							t.Fatal(e)
						}
					case "same-size-N-after-read":
						n.raw = []byte("Z-raw")
						if e := os.WriteFile(n.path, n.raw, 0600); e != nil {
							t.Fatal(e)
						}
						fixed := time.Unix(1, 0)
						if e := os.Chtimes(n.path, fixed, fixed); e != nil {
							t.Fatal(e)
						}
					case "mode-N-after-read":
						n.mode = 0640
						if e := os.Chmod(n.path, n.mode); e != nil {
							t.Fatal(e)
						}
					case "link-N-after-read":
						if e := os.Link(n.path, filepath.Join(filepath.Dir(n.path), "late-link")); e != nil {
							t.Fatal(e)
						}
					case "read-error-A":
						return 0, errors.New("fixture-private-cause-must-not-escape")
					}
				}
				return unix.Pread(fd, b, off)
			})
			badPair(t, p, e, "bootstrap/descriptor_invalid")
			if !fired || calls == 0 {
				t.Fatal("intended production read path not reached")
			}
			// Test mutations are expected; intake must not add writes, rewind, repair or close.
			unchanged(t, n)
			unchanged(t, a)
		})
	}
	t.Run("actual-growth-over-limit", func(t *testing.T) {
		n, a := newInput(t, bytes.Repeat([]byte{'N'}, RequestLimit)), newInput(t, []byte("A"))
		fired := false
		p, e := readPair(n.file, a.file, func(fd int, b []byte, off int64) (int, error) {
			if !fired {
				fired = true
				n.raw = append(n.raw, '!')
				if e := os.WriteFile(n.path, n.raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			return unix.Pread(fd, b, off)
		})
		badPair(t, p, e, "parse/bounds_exceeded")
		if !fired {
			t.Fatal("growth seam not applied")
		}
		unchanged(t, n)
		unchanged(t, a)
	})
	// Nearby positive: real partial Pread progress is assembled, still offset-independent.
	n, a := newInput(t, []byte("N-raw")), newInput(t, []byte("A-raw"))
	calls := 0
	p, e := readPair(n.file, a.file, func(fd int, b []byte, off int64) (int, error) {
		calls++
		if len(b) > 1 {
			b = b[:1]
		}
		return unix.Pread(fd, b, off)
	})
	if e != nil || p == nil || !bytes.Equal(p.Request().Raw(), n.raw) || !bytes.Equal(p.Authorization().Raw(), a.raw) || calls < 4 {
		t.Fatal("bounded partial progress positive failed", e)
	}
	unchanged(t, n)
	unchanged(t, a)
}

// A successful observation is retained independently of later same-UID writes.
// It cannot attest a frozen source or exclude another writer/mapping.
func TestLaterWriterDoesNotChangeSnapshot(t *testing.T) {
	n, a := newInput(t, []byte("N-raw")), newInput(t, []byte("A-raw"))
	p := goodPair(t, n, a)
	original := bytes.Clone(n.raw)
	sum := sha256.Sum256(original)
	if err := os.WriteFile(n.path, []byte("later"), 0600); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(p.Request().Raw(), original) || p.Request().SHA256() != sum {
		t.Fatal("retained snapshot changed with later writer")
	}
	n.raw = []byte("later")
	unchanged(t, n)
	unchanged(t, a)
}
