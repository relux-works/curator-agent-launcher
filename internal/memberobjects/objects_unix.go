//go:build darwin || linux

package memberobjects

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

type timestamp struct{ seconds, nanos int64 }
type observation struct {
	device, inode     uint64
	uid, mode         uint32
	links             uint64
	size              int64
	flags             int
	modified, changed timestamp
}
type ownedObject struct {
	declaration
	fd       int
	observed observation
}
type retained struct {
	root     int
	observed observation
	objects  []ownedObject
	ops      operations
}

// Per-call syscall seams permit scoped resource/fault assertions in ordinary
// tests. No global hook, alternate policy or generic filesystem mock.
type operations struct {
	openAt  func(int, string, int, uint32) (int, error)
	readAt  func(int, []byte, int64) (int, error)
	closeFD func(int) error
}

func nativeOperations() operations { return operations{unix.Openat, unix.Pread, unix.Close} }
func acquirePlatform(dir *os.File, refs []Reference) (*Collection, *Error) {
	return acquire(dir, refs, nativeOperations())
}
func acquire(dir *os.File, refs []Reference, ops operations) (*Collection, *Error) {
	rs, ds, err := project(refs)
	if err != nil {
		return nil, err
	}
	if dir == nil {
		return nil, descriptorError()
	}
	conn, e := dir.SyscallConn()
	if e != nil {
		return nil, descriptorError()
	}
	var root = -1
	var initial observation
	var refusal *Error
	e = conn.Control(func(fd uintptr) {
		initial, refusal = observe(int(fd))
		if refusal != nil || !validRoot(initial) {
			refusal = descriptorError()
			return
		}
		// openat(".") creates an independent open-file description; dup would share
		// the caller's directory offset. The caller File is pinned during this open.
		root, e = ops.openAt(int(fd), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if e != nil {
			refusal = descriptorError()
		}
	})
	if e != nil || refusal != nil {
		if root >= 0 {
			ops.closeFD(root)
		}
		return nil, descriptorError()
	}
	r := &retained{root: root, ops: ops}
	fail := func(err *Error) (*Collection, *Error) { r.close(); return nil, err }
	r.observed, refusal = observe(root)
	if refusal != nil || !validRoot(r.observed) || !sameNative(initial, r.observed) {
		return fail(descriptorError())
	}
	if err = r.enumerate(ds); err != nil {
		return fail(err)
	}
	snapshots := make(map[string]*Snapshot, len(ds))
	for _, d := range ds {
		fd, e := ops.openAt(root, d.digest, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if e != nil {
			return fail(mismatchError())
		}
		r.objects = append(r.objects, ownedObject{declaration: d, fd: fd})
		o := &r.objects[len(r.objects)-1]
		o.observed, err = inspectObject(fd, d.length)
		if err != nil {
			return fail(err)
		}
		s, err := readObserved(fd, d.length, ops.readAt)
		if err != nil {
			return fail(err)
		}
		if hex.EncodeToString(s.digest[:]) != d.digest {
			return fail(mismatchError())
		}
		after, err := inspectObject(fd, d.length)
		if err != nil {
			return fail(err)
		}
		if after != o.observed || !r.entryMatches(*o) {
			return fail(mismatchError())
		}
		snapshots[d.digest] = s
	}
	// Detect changes to already-read files while later objects were being read,
	// plus namespace changes across the whole acquisition. No hostile-writer fence.
	for _, o := range r.objects {
		now, err := inspectObject(o.fd, o.length)
		if err != nil {
			return fail(err)
		}
		if now != o.observed || !r.entryMatches(o) {
			return fail(mismatchError())
		}
	}
	if err = r.enumerate(ds); err != nil {
		return fail(err)
	}
	after, err := observe(root)
	if err != nil || after != r.observed {
		return fail(mismatchError())
	}
	return &Collection{refs: rs, snapshots: snapshots, custody: r}, nil
}
func observe(fd int) (observation, *Error) {
	var s unix.Stat_t
	if unix.Fstat(fd, &s) != nil {
		return observation{}, mismatchError()
	}
	flags, e := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if e != nil {
		return observation{}, mismatchError()
	}
	m, c := statTimes(&s)
	return observation{uint64(s.Dev), uint64(s.Ino), s.Uid, uint32(s.Mode), uint64(s.Nlink), s.Size, flags, m, c}, nil
}
func validRoot(o observation) bool {
	return o.mode == uint32(unix.S_IFDIR|0700) && o.uid == uint32(os.Getuid()) && readableFlags(o.flags)
}
func sameNative(a, b observation) bool { a.flags = 0; b.flags = 0; return a == b }
func validObject(o observation, uid uint32, length uint64) *Error {
	if o.uid != uid || o.links != 1 || (o.mode != uint32(unix.S_IFREG|0400) && o.mode != uint32(unix.S_IFREG|0600)) || !readableFlags(o.flags) || o.size < 0 {
		return mismatchError()
	}
	if o.size > ObjectLimit {
		return boundsError()
	}
	if uint64(o.size) != length {
		return mismatchError()
	}
	return nil
}
func inspectObject(fd int, length uint64) (observation, *Error) {
	o, err := observe(fd)
	if err != nil {
		return o, err
	}
	return o, validObject(o, uint32(os.Getuid()), length)
}
func (r *retained) entryMatches(o ownedObject) bool {
	var s unix.Stat_t
	if unix.Fstatat(r.root, o.digest, &s, unix.AT_SYMLINK_NOFOLLOW) != nil {
		return false
	}
	return uint64(s.Dev) == o.observed.device && uint64(s.Ino) == o.observed.inode && uint32(s.Mode) == o.observed.mode && s.Uid == o.observed.uid && uint64(s.Nlink) == 1 && s.Size == o.observed.size
}
func (r *retained) enumerate(ds []declaration) (refusal *Error) {
	fd, e := r.ops.openAt(r.root, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return mismatchError()
	}
	f := os.NewFile(uintptr(fd), "")
	if f == nil {
		r.ops.closeFD(fd)
		return mismatchError()
	}
	defer func() {
		if f.Close() != nil && refusal == nil {
			refusal = mismatchError()
		}
	}()
	// Bounded names plus ONE detects arbitrary extras; never Readdir(-1).
	names, e := f.Readdirnames(len(ds) + 1)
	if (e != nil && e != io.EOF) || len(names) != len(ds) {
		return mismatchError()
	}
	want := make(map[string]bool, len(ds))
	for _, d := range ds {
		want[d.digest] = true
	}
	for _, name := range names {
		if !want[name] {
			return mismatchError()
		}
		delete(want, name)
	}
	if len(want) != 0 {
		return mismatchError()
	}
	return nil
}
func readObserved(fd int, size uint64, readAt func(int, []byte, int64) (int, error)) (*Snapshot, *Error) {
	// Projection and native size are bounded BEFORE this conversion/allocation.
	// The retained allocation is exactly size; the extra-byte probe is separate.
	raw := make([]byte, int(size))
	total := 0
	for total < len(raw) {
		n, e := readAt(fd, raw[total:], int64(total))
		if n < 0 || n > len(raw)-total || e != nil {
			return nil, mismatchError()
		}
		if n == 0 {
			return nil, mismatchError()
		}
		total += n
	}
	var probe [1]byte
	n, e := readAt(fd, probe[:], int64(total))
	if n < 0 || n > 1 {
		return nil, mismatchError()
	}
	if n > 0 {
		if total == ObjectLimit {
			return nil, boundsError()
		}
		return nil, mismatchError()
	}
	if e != nil {
		return nil, mismatchError()
	}
	return &Snapshot{raw, sha256.Sum256(raw)}, nil
}
func (r *retained) recheck(snapshots map[string]*Snapshot) *Error {
	now, err := observe(r.root)
	if err != nil || now != r.observed || !validRoot(now) {
		return mismatchError()
	}
	ds := make([]declaration, 0, len(r.objects))
	for _, o := range r.objects {
		ds = append(ds, o.declaration)
	}
	if err = r.enumerate(ds); err != nil {
		return err
	}
	for _, o := range r.objects {
		now, err = inspectObject(o.fd, o.length)
		if err != nil {
			return err
		}
		if now != o.observed || !r.entryMatches(o) {
			return mismatchError()
		}
		s, err := readObserved(o.fd, o.length, r.ops.readAt)
		if err != nil {
			return err
		}
		if s.digest != snapshots[o.digest].digest {
			return mismatchError()
		}
		after, err := inspectObject(o.fd, o.length)
		if err != nil {
			return err
		}
		if after != o.observed || !r.entryMatches(o) {
			return mismatchError()
		}
	}
	for _, o := range r.objects {
		now, err = inspectObject(o.fd, o.length)
		if err != nil {
			return err
		}
		if now != o.observed || !r.entryMatches(o) {
			return mismatchError()
		}
	}
	if err = r.enumerate(ds); err != nil {
		return err
	}
	now, err = observe(r.root)
	if err != nil || now != r.observed {
		return mismatchError()
	}
	return nil
}
func (r *retained) close() *Error {
	var err *Error
	for i := range r.objects {
		fd := r.objects[i].fd
		r.objects[i].fd = -1
		if fd >= 0 && r.ops.closeFD(fd) != nil {
			err = mismatchError()
		}
	}
	fd := r.root
	r.root = -1
	if fd >= 0 && r.ops.closeFD(fd) != nil {
		err = mismatchError()
	}
	return err
}
