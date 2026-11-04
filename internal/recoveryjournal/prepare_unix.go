//go:build darwin || linux

package recoveryjournal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type prepareOperations struct {
	write     func(int, []byte) (int, error)
	sync      func(int) error
	close     func(int) error
	renameNew func(int, string, string) error
}

func nativePrepareOperations() prepareOperations {
	return prepareOperations{unix.Write, unix.Fsync, unix.Close, renameNew}
}

// Prepare persists the frozen private envelope before any request. It creates
// only the namespace lock and a secret record/temp in an already-admitted
// layout. No caller configuration or native home is discovered or repaired.
// Failed/uncertain writes retain private incomplete evidence and return no
// Prepared result. A deadline is mandatory for the namespace lock.
func (n *Namespace) Prepare(ctx context.Context, e Envelope) (*Prepared, error) {
	return n.prepare(ctx, e, nativePrepareOperations())
}

func (n *Namespace) prepare(ctx context.Context, e Envelope, ops prepareOperations) (prepared *Prepared, failure error) {
	if !n.binding.valid() {
		return nil, ErrBinding
	}
	if err := e.Validate(); err != nil {
		return nil, err
	}
	if _, ok := ctx.Deadline(); !ok || ctx.Err() != nil {
		return nil, ErrLock
	}
	n.use.RLock()
	defer n.use.RUnlock()
	if n.closed || !n.sameLocation() {
		return nil, ErrUnsafeLayout
	}
	lock, err := n.lock(ctx)
	if err != nil {
		return nil, err
	}
	// Closing releases flock even after failure. No shared lock is unlinked.
	defer func() {
		if unix.Close(lock) != nil {
			prepared, failure = nil, ErrLock
		}
	}()
	if ctx.Err() != nil || !n.sameLocation() || !lockStillBound(n.directory, lock) {
		return nil, ErrLock
	}
	raw, err := json.Marshal(secretRecord{SecretVersion, e, n.binding.base, n.binding.profile, n.binding.socket, nil, "prepared", "not_observed"})
	if err != nil || len(raw) > MaxRecord {
		return nil, ErrPrepare
	}
	name := e.AttemptID + ".json"
	var st unix.Stat_t
	if err := unix.Fstatat(n.secrets, name, &st, unix.AT_SYMLINK_NOFOLLOW); err == nil {
		return nil, ErrExisting
	} else if !errors.Is(err, unix.ENOENT) {
		return nil, ErrUnsafeLayout
	}
	if err := n.capacity(len(raw)); err != nil {
		return nil, err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, ErrPrepare
	}
	temp := ".prepare-" + hex.EncodeToString(nonce[:]) + ".tmp"
	fd, err := unix.Openat(n.secrets, temp, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, ErrPrepare
	}
	closed := false
	defer func() {
		if !closed {
			unix.Close(fd)
		}
	}()
	if !privateFile(fd, 0) {
		return nil, ErrUnsafeLayout
	}
	if ctx.Err() != nil {
		return nil, ErrPrepare
	}
	written, err := ops.write(fd, raw)
	if err != nil || written != len(raw) || ctx.Err() != nil || !privateFile(fd, int64(len(raw))) {
		return nil, ErrPrepare
	}
	if err := ops.sync(fd); err != nil {
		return nil, ErrPrepare
	}
	var writtenStat unix.Stat_t
	if unix.Fstat(fd, &writtenStat) != nil || !privateStat(writtenStat, int64(len(raw))) {
		return nil, ErrPrepare
	}
	err = ops.close(fd)
	closed = true // a failed close is uncertain; never retry a potentially reused fd
	if err != nil || ctx.Err() != nil || !n.sameLocation() || !lockStillBound(n.directory, lock) || !entryMatches(n.secrets, temp, writtenStat) {
		return nil, ErrPrepare
	}
	if err := ops.renameNew(n.secrets, temp, name); err != nil {
		return nil, ErrPrepare
	}
	// Once published, complete the essential directory sync even if cancelled.
	if err := ops.sync(n.secrets); err != nil {
		return nil, ErrPrepare
	}
	if !n.sameLocation() || !lockStillBound(n.directory, lock) || !entryMatches(n.secrets, name, writtenStat) {
		return nil, ErrPrepare
	}
	return &Prepared{e}, nil
}

func entryMatches(dir int, name string, want unix.Stat_t) bool {
	var entry unix.Stat_t
	return unix.Fstatat(dir, name, &entry, unix.AT_SYMLINK_NOFOLLOW) == nil &&
		privateStat(entry, want.Size) && entry.Dev == want.Dev && entry.Ino == want.Ino
}

func privateStat(s unix.Stat_t, size int64) bool {
	return uint32(s.Mode) == unix.S_IFREG|0600 && s.Uid == uint32(os.Geteuid()) && s.Nlink == 1 && s.Size == size
}

func privateFile(fd int, size int64) bool {
	var s unix.Stat_t
	return unix.Fstat(fd, &s) == nil && privateStat(s, size)
}

func (n *Namespace) sameLocation() bool {
	fresh, err := openLocation(n.location)
	if err != nil {
		return false
	}
	for _, pair := range [][2]int{{n.base, fresh.base}, {n.directory, fresh.directory}, {n.secrets, fresh.secrets}, {n.receipts, fresh.receipts}} {
		var a, b unix.Stat_t
		if unix.Fstat(pair[0], &a) != nil || unix.Fstat(pair[1], &b) != nil || a.Dev != b.Dev || a.Ino != b.Ino {
			fresh.Close()
			return false
		}
	}
	return fresh.Close() == nil
}

func lockStillBound(dir, fd int) bool {
	var opened, entry unix.Stat_t
	return unix.Fstat(fd, &opened) == nil && privateStat(opened, 0) && unix.Fstatat(dir, "lock", &entry, unix.AT_SYMLINK_NOFOLLOW) == nil && privateStat(entry, 0) && entry.Dev == opened.Dev && entry.Ino == opened.Ino
}

func (n *Namespace) lock(ctx context.Context) (int, error) {
	fd, err := unix.Openat(n.directory, "lock", unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	created := err == nil
	if errors.Is(err, unix.EEXIST) {
		fd, err = unix.Openat(n.directory, "lock", unix.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	}
	if err != nil {
		return -1, ErrUnsafeLayout
	}
	fail := func(err error) (int, error) { unix.Close(fd); return -1, err }
	if !privateFile(fd, 0) {
		return fail(ErrUnsafeLayout)
	}
	if created && (unix.Fsync(fd) != nil || unix.Fsync(n.directory) != nil) {
		return fail(ErrLock)
	}
	for {
		if ctx.Err() != nil {
			return fail(ErrLock)
		}
		err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return fd, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return fail(ErrLock)
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fail(ErrLock)
		case <-timer.C:
		}
	}
}

func (n *Namespace) capacity(extra int) (result error) {
	if err := n.receiptCapacity(); err != nil {
		return err
	}
	fd, err := openDirectory(n.secrets, ".")
	if err != nil {
		return ErrUnsafeLayout
	}
	f := os.NewFile(uintptr(fd), "")
	defer func() {
		if f.Close() != nil && result == nil {
			result = ErrLayoutIO
		}
	}()
	names, err := f.Readdirnames(MaxAttempts + 1)
	if err != nil && err != io.EOF {
		return ErrUnsafeLayout
	}
	if len(names) >= MaxAttempts {
		return ErrCapacity
	}
	total := int64(extra)
	for _, name := range names {
		valid := strings.HasSuffix(name, ".json") && digest(strings.TrimSuffix(name, ".json"))
		if strings.HasPrefix(name, ".prepare-") && strings.HasSuffix(name, ".tmp") {
			random := strings.TrimSuffix(strings.TrimPrefix(name, ".prepare-"), ".tmp")
			_, e := hex.DecodeString(random)
			valid = e == nil && (len(random) == 32 || len(random) == 64) && strings.ToLower(random) == random
		}
		var st unix.Stat_t
		if !valid || unix.Fstatat(n.secrets, name, &st, unix.AT_SYMLINK_NOFOLLOW) != nil || !privateStat(st, st.Size) || st.Size < 0 || st.Size > MaxRecord {
			return ErrUnsafeLayout
		}
		total += st.Size
		if total > MaxSecretBytes {
			return ErrCapacity
		}
	}
	return nil
}
