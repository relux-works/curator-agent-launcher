//go:build darwin || linux

package recoveryjournal

import (
	"bytes"
	"context"
	"encoding/json"
	"golang.org/x/sys/unix"
)

type terminalOperations struct {
	write   func(int, []byte) (int, error)
	sync    func(int) error
	close   func(int) error
	replace func(int, string, string) error
	unlink  func(int, string) error
}

func nativeTerminalOperations() terminalOperations {
	return terminalOperations{unix.Write, unix.Fsync, unix.Close,
		func(dir int, old, next string) error { return unix.Renameat(dir, old, dir, next) },
		func(dir int, name string) error { return unix.Unlinkat(dir, name, 0) }}
}

// Replace only the exact guarded private entry, retaining every original E
// field. The caller holds the same lock from read through authenticated recover
// and durable receipt publication. Failures return no local success; the proof
// remains in either the original prepared or atomically replaced ended entry.
func (n *Namespace) commitTerminal(ctx context.Context, entry *Entry, receipt receiptRecord, anchor receiptRetention, lock int, ops terminalOperations) (failure error) {
	if entry == nil || !anchor.valid() || !n.receiptLocation(ctx, lock) {
		return ErrLocalEvidence
	}
	name := entry.envelope.AttemptID + ".json"
	var original unix.Stat_t
	if unix.Fstatat(n.secrets, name, &original, unix.AT_SYMLINK_NOFOLLOW) != nil || !privateStat(original, original.Size) {
		return ErrLocalEvidence
	}
	old, err := n.readRecordLocked(ctx, entry.envelope.AttemptID, nativeReadOperations(), lock)
	if err != nil || !bytes.Equal(old, entry.raw) || !entryMatches(n.secrets, name, original) {
		return ErrLocalEvidence
	}
	raw, err := json.Marshal(terminalRecord{secretRecord{TerminalSecretVersion, entry.envelope, n.binding.base, n.binding.profile, n.binding.socket, receipt.LaunchID, "ended", receipt.Result}, anchor})
	if err != nil || len(raw) > MaxRecord {
		return ErrLocalEvidence
	}
	if _, err := decodeEntry(raw, entry.envelope.AttemptID, n.binding); err != nil {
		return ErrLocalEvidence
	}
	// Fixed exclusive private preparation temp: one bounded residue per exact
	// attempt. Preserve old random 32-hex preparation temp compatibility too.
	temp := ".prepare-" + entry.envelope.AttemptID + ".tmp"
	fd, err := unix.Openat(n.secrets, temp, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if err != nil {
		return ErrLocalEvidence
	}
	var owned unix.Stat_t
	if unix.Fstat(fd, &owned) != nil || !privateStat(owned, 0) {
		_ = unix.Close(fd)
		return ErrLocalEvidence
	}
	closed, published := false, false
	defer func() {
		if !closed && unix.Close(fd) != nil {
			failure = ErrLocalEvidence
		}
		if !published {
			var st unix.Stat_t
			if n.sameLocation() && lockStillBound(n.directory, lock) && unix.Fstatat(n.secrets, temp, &st, unix.AT_SYMLINK_NOFOLLOW) == nil && privateStat(st, st.Size) && st.Dev == owned.Dev && st.Ino == owned.Ino {
				if ops.unlink(n.secrets, temp) != nil || ops.sync(n.secrets) != nil {
					failure = ErrLocalEvidence
				}
			}
		}
	}()
	if !n.receiptLocation(ctx, lock) {
		return ErrLocalEvidence
	}
	written, err := ops.write(fd, raw)
	if err != nil || written != len(raw) || !privateFile(fd, int64(len(raw))) || ctx.Err() != nil {
		return ErrLocalEvidence
	}
	if ops.sync(fd) != nil {
		return ErrLocalEvidence
	}
	var saved unix.Stat_t
	if unix.Fstat(fd, &saved) != nil || !privateStat(saved, int64(len(raw))) {
		return ErrLocalEvidence
	}
	err = ops.close(fd)
	closed = true
	if err != nil || !n.receiptLocation(ctx, lock) || !entryMatches(n.secrets, temp, saved) || !entryMatches(n.secrets, name, original) {
		return ErrLocalEvidence
	}
	if ops.replace(n.secrets, temp, name) != nil {
		return ErrLocalEvidence
	}
	published = true
	// Essential directory sync completes even after cancellation at rename.
	if ops.sync(n.secrets) != nil || !n.receiptLocation(ctx, lock) || !entryMatches(n.secrets, name, saved) {
		return ErrLocalEvidence
	}
	return nil
}
