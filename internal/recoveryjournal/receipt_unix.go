//go:build darwin || linux

package recoveryjournal

import (
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

type receiptOperations struct {
	write     func(int, []byte) (int, error)
	sync      func(int) error
	close     func(int) error
	closeFile func(*os.File) error
	renameNew func(int, string, string) error
	unlink    func(int, string) error
}

func nativeReceiptOperations() receiptOperations {
	return receiptOperations{unix.Write, unix.Fsync, unix.Close, (*os.File).Close, renameNew, func(dir int, name string) error { return unix.Unlinkat(dir, name, 0) }}
}

func (n *Namespace) receiptLocation(ctx context.Context, lock int) bool {
	return ctx.Err() == nil && n.sameLocation() && lockStillBound(n.directory, lock)
}

// One deterministic per-attempt temporary name and exclusive creation bound
// residue without enumeration. On failure remove only this call's own temp,
// checked by inode; the original private E remains the incomplete authority
// record. If that cleanup fails, leave the bounded residue and refuse reuse.
// An existing unrelated/unsafe temp is never overwritten, removed or repaired.
func (n *Namespace) commitReceipt(ctx context.Context, want receiptRecord, raw []byte, lock int, ops receiptOperations) (committed []byte, failure error) {
	if !n.receiptLocation(ctx, lock) {
		return nil, ErrLocalEvidence
	}
	name := want.AttemptID + ".json"
	fd, err := unix.Openat(n.receipts, name, unix.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err == nil {
		return n.resyncReceipt(ctx, name, fd, want, lock, ops)
	}
	if !errors.Is(err, unix.ENOENT) {
		return nil, ErrLocalEvidence
	}
	temp := ".receipt-" + want.AttemptID + ".tmp"
	fd, err = unix.Openat(n.receipts, temp, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, ErrLocalEvidence
	}
	var owned unix.Stat_t
	if unix.Fstat(fd, &owned) != nil || !privateStat(owned, 0) {
		_ = unix.Close(fd)
		return nil, ErrLocalEvidence
	}
	closed, published := false, false
	defer func() {
		if !closed {
			if unix.Close(fd) != nil {
				committed, failure = nil, ErrLocalEvidence
			}
		}
		if !published {
			var entry unix.Stat_t
			// Never delete a substituted entry or operate through a displaced
			// directory. No original private authority is deleted on this path.
			if n.sameLocation() && lockStillBound(n.directory, lock) && unix.Fstatat(n.receipts, temp, &entry, unix.AT_SYMLINK_NOFOLLOW) == nil && privateStat(entry, entry.Size) && entry.Dev == owned.Dev && entry.Ino == owned.Ino {
				if ops.unlink(n.receipts, temp) != nil || ops.sync(n.receipts) != nil {
					committed, failure = nil, ErrLocalEvidence
				}
			}
		}
	}()
	if !n.receiptLocation(ctx, lock) {
		return nil, ErrLocalEvidence
	}
	written, err := ops.write(fd, raw)
	if err != nil || written != len(raw) || !privateFile(fd, int64(len(raw))) || ctx.Err() != nil {
		return nil, ErrLocalEvidence
	}
	if ops.sync(fd) != nil {
		return nil, ErrLocalEvidence
	}
	var saved unix.Stat_t
	if unix.Fstat(fd, &saved) != nil || !privateStat(saved, int64(len(raw))) {
		return nil, ErrLocalEvidence
	}
	err = ops.close(fd)
	closed = true
	if err != nil || !n.receiptLocation(ctx, lock) || !entryMatches(n.receipts, temp, saved) {
		return nil, ErrLocalEvidence
	}
	if ops.renameNew(n.receipts, temp, name) != nil {
		return nil, ErrLocalEvidence
	}
	published = true
	// Complete the essential parent sync after publication, even on cancel.
	if ops.sync(n.receipts) != nil || !n.receiptLocation(ctx, lock) || !entryMatches(n.receipts, name, saved) {
		return nil, ErrLocalEvidence
	}
	return raw, nil
}

// A previous uncertain directory sync cannot become success merely because
// bytes exist. Validate the exact closed outcome, then sync file and directory
// again. Preserve its original observation/operation/boot without rewriting it.
func (n *Namespace) resyncReceipt(ctx context.Context, name string, fd int, want receiptRecord, lock int, ops receiptOperations) (committed []byte, failure error) {
	f := os.NewFile(uintptr(fd), "")
	closed := false
	defer func() {
		if !closed {
			if f.Close() != nil {
				committed, failure = nil, ErrLocalEvidence
			}
		}
	}()
	var before, after unix.Stat_t
	if unix.Fstat(fd, &before) != nil || !privateStat(before, before.Size) || before.Size <= 0 || before.Size > MaxRecord || !entryMatches(n.receipts, name, before) {
		return nil, ErrLocalEvidence
	}
	info, err := f.Stat()
	if err != nil {
		return nil, ErrLocalEvidence
	}
	raw, err := io.ReadAll(io.LimitReader(f, MaxRecord+1))
	updated, statErr := f.Stat()
	got, ok := decodeReceipt(raw)
	if err != nil || statErr != nil || !ok || !sameReceiptOutcome(got, want) || int64(len(raw)) != before.Size || !updated.ModTime().Equal(info.ModTime()) || unix.Fstat(fd, &after) != nil || after.Dev != before.Dev || after.Ino != before.Ino || !privateStat(after, before.Size) || !n.receiptLocation(ctx, lock) {
		return nil, ErrLocalEvidence
	}
	if ops.sync(fd) != nil {
		return nil, ErrLocalEvidence
	}
	err = ops.closeFile(f)
	closed = true
	if err != nil {
		return nil, ErrLocalEvidence
	}
	if ops.sync(n.receipts) != nil || !n.receiptLocation(ctx, lock) || !entryMatches(n.receipts, name, after) {
		return nil, ErrLocalEvidence
	}
	return raw, nil
}
