//go:build darwin || linux

package recoveryjournal

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"

	"github.com/relux-works/curator-agent-launcher/internal/netproxy"
	"golang.org/x/sys/unix"
)

type pruneOperations struct {
	receipt   receiptOperations
	terminal  terminalOperations
	sync      func(int) error
	closeFile func(*os.File) error
	unlink    func(int, string) error
}

func nativePruneOperations() pruneOperations {
	return pruneOperations{nativeReceiptOperations(), nativeTerminalOperations(), unix.Fsync, (*os.File).Close, func(dir int, name string) error { return unix.Unlinkat(dir, name, 0) }}
}

// PruneExact considers only one original attempt. It sends no service request,
// frees no unfinished authority, and never enumerates another entry or home.
// This is a package operation; no installed cleanup command exposes it.
func (n *Namespace) PruneExact(ctx context.Context, attempt string, current Binding) (PruneObservation, error) {
	return n.pruneExact(ctx, attempt, current, nativePruneOperations(), nativeAwake)
}

func (n *Namespace) pruneExact(ctx context.Context, attempt string, current Binding, ops pruneOperations, awake func() (awakeObservation, error)) (observation PruneObservation, failure error) {
	if !digest(attempt) {
		return observation, ErrRead
	}
	if err := n.binding.Match(current); err != nil {
		return observation, err
	}
	if _, ok := ctx.Deadline(); !ok || ctx.Err() != nil {
		return observation, ErrLock
	}
	n.use.RLock()
	defer n.use.RUnlock()
	if n.closed || !n.sameLocation() {
		return observation, ErrUnsafeLayout
	}
	lock, err := n.lock(ctx)
	if err != nil {
		return observation, err
	}
	defer func() {
		if unix.Close(lock) != nil {
			failure = ErrLocalEvidence
		}
	}()
	if !n.receiptLocation(ctx, lock) {
		return observation, ErrLocalEvidence
	}
	entry, err := n.readLocked(ctx, attempt, nativeReadOperations(), lock)
	if err != nil {
		return observation, err
	}
	// A prepared entry includes uncertain issuance and can never be evicted.
	if entry.retention == nil {
		return observation, nil
	}
	original, err := n.syncPrivateEntry(ctx, entry, lock, ops)
	if err != nil {
		return observation, ErrLocalEvidence
	}
	name := attempt + ".json"
	fd, err := unix.Openat(n.receipts, name, unix.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return observation, ErrLocalEvidence
	}
	var receiptStat unix.Stat_t
	if unix.Fstat(fd, &receiptStat) != nil || !privateStat(receiptStat, receiptStat.Size) {
		_ = unix.Close(fd)
		return observation, ErrLocalEvidence
	}
	// Only correlation/outcome fields are expected here; the original public
	// observation's boot/operation/UTC remain unchanged by resyncReceipt.
	want := receiptRecord{Version: ReceiptVersion, RegistryID: entry.envelope.RegistryID, AttemptID: attempt, LaunchID: entry.launch, ProfileDigest: n.binding.profile, Session: entry.envelope.Session, Runtime: entry.envelope.Runtime, ServiceVersion: netproxy.RecoveryAPI, Result: entry.outcome, Status: 200, Durable: true}
	raw, err := n.resyncReceipt(ctx, name, fd, want, lock, ops.receipt)
	if err != nil || !entryMatches(n.receipts, name, receiptStat) {
		return observation, ErrLocalEvidence
	}
	clock, err := awake()
	if err != nil {
		return observation, ErrLocalEvidence
	}
	anchor, err := nextRetention(entry.retention, raw, clock)
	if err != nil {
		return observation, ErrLocalEvidence
	}
	if anchor.Boot != entry.retention.Boot {
		// Restart conservatively only after a checked receipt resync. No new
		// remote result is invented; the existing ended result is retained.
		receipt, ok := decodeReceipt(raw)
		if !ok {
			return observation, ErrLocalEvidence
		}
		if n.commitTerminal(ctx, entry, receipt, anchor, lock, ops.terminal) != nil {
			return observation, ErrLocalEvidence
		}
		return observation, nil
	}
	if !retentionElapsed(anchor, clock) {
		return observation, nil
	}
	if !n.receiptLocation(ctx, lock) || !entryMatches(n.secrets, name, original) || !entryMatches(n.receipts, name, receiptStat) {
		return observation, ErrLocalEvidence
	}
	removeErr := ops.unlink(n.secrets, name)
	// Complete the essential sync even when unlink reports an uncertain error.
	syncErr := ops.sync(n.secrets)
	if removeErr != nil || syncErr != nil || !n.sameLocation() || !lockStillBound(n.directory, lock) || !entryAbsent(n.secrets, name) {
		return observation, ErrLocalEvidence
	}
	observation.SecretRemoved = true
	// Never remove the receipt until the secret's durable absence is known.
	if !n.receiptLocation(ctx, lock) || !entryMatches(n.receipts, name, receiptStat) {
		return observation, ErrLocalEvidence
	}
	removeErr = ops.unlink(n.receipts, name)
	syncErr = ops.sync(n.receipts)
	if removeErr != nil || syncErr != nil || !n.sameLocation() || !lockStillBound(n.directory, lock) || !entryAbsent(n.receipts, name) {
		return observation, ErrLocalEvidence
	}
	observation.ReceiptRemoved = true
	return observation, nil
}

func entryAbsent(dir int, name string) bool {
	var st unix.Stat_t
	return errors.Is(unix.Fstatat(dir, name, &st, unix.AT_SYMLINK_NOFOLLOW), unix.ENOENT)
}

// Existence alone never certifies a prior terminal publication. Verify exact
// bytes and inode, then sync both file and parent before any age-based removal.
func (n *Namespace) syncPrivateEntry(ctx context.Context, entry *Entry, lock int, ops pruneOperations) (saved unix.Stat_t, failure error) {
	name := entry.envelope.AttemptID + ".json"
	fd, err := unix.Openat(n.secrets, name, unix.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return saved, ErrLocalEvidence
	}
	f := os.NewFile(uintptr(fd), "")
	closed := false
	defer func() {
		if !closed && f.Close() != nil {
			failure = ErrLocalEvidence
		}
	}()
	if unix.Fstat(fd, &saved) != nil || !privateStat(saved, int64(len(entry.raw))) || !entryMatches(n.secrets, name, saved) {
		return saved, ErrLocalEvidence
	}
	info, err := f.Stat()
	if err != nil {
		return saved, ErrLocalEvidence
	}
	raw, err := io.ReadAll(io.LimitReader(f, MaxRecord+1))
	updated, statErr := f.Stat()
	if err != nil || statErr != nil || !bytes.Equal(raw, entry.raw) || !updated.ModTime().Equal(info.ModTime()) || !n.receiptLocation(ctx, lock) || !entryMatches(n.secrets, name, saved) {
		return saved, ErrLocalEvidence
	}
	if ops.sync(fd) != nil {
		return saved, ErrLocalEvidence
	}
	err = ops.closeFile(f)
	closed = true
	if err != nil {
		return saved, ErrLocalEvidence
	}
	if ops.sync(n.secrets) != nil || !n.receiptLocation(ctx, lock) || !entryMatches(n.secrets, name, saved) {
		return saved, ErrLocalEvidence
	}
	return saved, nil
}
