//go:build darwin || linux

package recoveryjournal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func endedPruneFixture(t *testing.T, cancelled bool) (Binding, *Namespace, Envelope) {
	t.Helper()
	b, n, e, _ := preparedReceiptFixture(t)
	confirm := confirmedFixture
	if cancelled {
		confirm = func(context.Context, Envelope) (serviceObservation, error) {
			return serviceObservation{nil, "cancelled_before_issue", "fixture-boot"}, nil
		}
	}
	got, err := n.recoverRetained(bounded(t), e.AttemptID, b, "recover", nativeReceiptOperations(), confirm, receiptTime, nativeTerminalOperations(), func() (awakeObservation, error) { return awakeObservation{fixtureBoot, 100}, nil })
	if err != nil || got.Receipt == nil {
		t.Fatal("ended fixture failed")
	}
	return b, n, e
}

func pruneClock(ns uint64) func() (awakeObservation, error) {
	return func() (awakeObservation, error) { return awakeObservation{fixtureBoot, ns}, nil }
}

// The independent interval boundary admits exactly seven awake days. Both
// ended outcomes delete secret, sync its parent, then delete/sync receipt.
func TestPruneExactOrderAndAge(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
			b, n, e := endedPruneFixture(t, cancelled)
			before, _ := os.ReadFile(finalPath(b, e))
			receiptBefore, _ := os.ReadFile(receiptPath(b, e))
			for _, now := range []uint64{100, 100 + 604800000000000 - 1} {
				got, err := n.pruneExact(bounded(t), e.AttemptID, b, nativePruneOperations(), pruneClock(now))
				if err != nil || got != (PruneObservation{}) {
					t.Fatal("PRUNE AGE ALARM: young evidence removed")
				}
				assertAuthorityUnchanged(t, b, e, before)
			}
			var order []string
			ops := nativePruneOperations()
			nativeSync := ops.sync
			removed := false
			ops.unlink = func(dir int, name string) error {
				if dir == n.secrets {
					order = append(order, "secret-unlink")
					removed = true
				} else {
					order = append(order, "receipt-unlink")
				}
				return unix.Unlinkat(dir, name, 0)
			}
			ops.sync = func(fd int) error {
				if removed {
					if fd == n.secrets {
						order = append(order, "secret-sync")
					} else if fd == n.receipts {
						order = append(order, "receipt-sync")
					}
				}
				return nativeSync(fd)
			}
			got, err := n.pruneExact(bounded(t), e.AttemptID, b, ops, pruneClock(100+604800000000000))
			if !reflect.DeepEqual(order, []string{"secret-unlink", "secret-sync", "receipt-unlink", "receipt-sync"}) {
				t.Fatal("PRUNE ORDER ALARM: receipt removed before checked secret absence")
			}
			if err != nil || !got.SecretRemoved || !got.ReceiptRemoved {
				t.Fatal("eligible exact removal refused")
			}
			for _, p := range []string{finalPath(b, e), receiptPath(b, e)} {
				if _, err := os.Stat(p); !os.IsNotExist(err) {
					t.Fatal("removed record remains")
				}
			}
			if len(receiptBefore) == 0 {
				t.Fatal("missing positive receipt control")
			}
		})
	}
}

// Unfinished, corrupt, young or unavailable evidence is retained. Different
// boot restarts a full interval only after exact receipt durability; it never
// gives the old clock's age to the new boot or sends a service request.
func TestPruneEvidenceRefusals(t *testing.T) {
	for _, kind := range []string{"prepared", "missing-receipt", "receipt-hash", "foreign-receipt", "receipt-symlink", "clock-error", "regress", "new-boot", "wrong-binding", "cancel", "bad-attempt"} {
		t.Run(kind, func(t *testing.T) {
			b, n, e := endedPruneFixture(t, false)
			before, _ := os.ReadFile(finalPath(b, e))
			receiptBefore, _ := os.ReadFile(receiptPath(b, e))
			current := b
			attempt := e.AttemptID
			ctx, cancel := context.WithCancel(bounded(t))
			defer cancel()
			clock := pruneClock(100 + 604800000000000)
			sentinel := filepath.Join(b.base, "sentinel")
			if os.WriteFile(sentinel, []byte("untouched"), 0600) != nil {
				t.Fatal("fixture")
			}
			switch kind {
			case "prepared":
				var r terminalRecord
				if json.Unmarshal(before, &r) != nil {
					t.Fatal("fixture")
				}
				r.secretRecord.Version = SecretVersion
				r.State = "prepared"
				r.LastOutcome = "not_observed"
				r.LaunchID = nil
				before, _ = json.Marshal(r.secretRecord)
				if os.WriteFile(finalPath(b, e), before, 0600) != nil {
					t.Fatal("fixture")
				}
			case "missing-receipt":
				if os.Remove(receiptPath(b, e)) != nil {
					t.Fatal("fixture")
				}
			case "receipt-hash", "foreign-receipt":
				r, ok := decodeReceipt(receiptBefore)
				if !ok {
					t.Fatal("fixture")
				}
				if kind == "receipt-hash" {
					r.ObservedUTC = "2099-01-01T00:00:00Z"
				} else {
					r.AttemptID = strings.Repeat("f", 64)
				}
				receiptBefore, _ = encodeReceipt(r)
				if os.WriteFile(receiptPath(b, e), receiptBefore, 0600) != nil {
					t.Fatal("fixture")
				}
			case "receipt-symlink":
				if os.Remove(receiptPath(b, e)) != nil || os.Symlink(sentinel, receiptPath(b, e)) != nil {
					t.Fatal("fixture")
				}
			case "clock-error":
				clock = func() (awakeObservation, error) { return awakeObservation{}, errors.New("private-clock-fault") }
			case "regress":
				clock = pruneClock(99)
			case "new-boot":
				clock = func() (awakeObservation, error) {
					return awakeObservation{"22345678-1234-4234-8234-123456789abc", 3}, nil
				}
			case "wrong-binding":
				current.profile = strings.Repeat("f", 64)
			case "cancel":
				cancel()
			case "bad-attempt":
				attempt = "invalid"
			}
			got, err := n.pruneExact(ctx, attempt, current, nativePruneOperations(), clock)
			if got != (PruneObservation{}) {
				t.Fatal("uncertain evidence removed")
			}
			if kind == "prepared" || kind == "new-boot" {
				if err != nil {
					t.Fatal("valid conservative retention refused")
				}
			} else if err == nil {
				t.Fatal("invalid evidence accepted")
			}
			if kind == "new-boot" {
				entry := terminalEntry(t, b, e)
				if entry.envelope != e || entry.retention.Boot != "22345678-1234-4234-8234-123456789abc" || entry.retention.AnchorNS != "3" {
					t.Fatal("new boot did not conservatively restart")
				}
			} else {
				assertAuthorityUnchanged(t, b, e, before)
			}
			if kind != "missing-receipt" && kind != "receipt-symlink" {
				after, _ := os.ReadFile(receiptPath(b, e))
				if !bytes.Equal(after, receiptBefore) {
					t.Fatal("retained receipt changed")
				}
			}
			raw, _ := os.ReadFile(sentinel)
			if string(raw) != "untouched" {
				t.Fatal("sentinel modified")
			}
		})
	}
}

// Removal and sync failures never authorize the next deletion. Uncertain
// secret removal preserves the receipt even when the first unlink took effect.
func TestPruneRemovalFaults(t *testing.T) {
	for _, kind := range []string{"secret-unlink", "secret-uncertain", "secret-sync", "secret-file-sync", "secret-close", "receipt-unlink", "receipt-uncertain", "receipt-sync", "cancel-after-secret", "receipt-substitution"} {
		t.Run(kind, func(t *testing.T) {
			b, n, e := endedPruneFixture(t, false)
			ctx, cancel := context.WithCancel(bounded(t))
			defer cancel()
			ops := nativePruneOperations()
			fault := errors.New("private-prune-fault")
			injected := false
			secretAttempted := false
			sentinel := filepath.Join(b.base, "sentinel")
			if os.WriteFile(sentinel, []byte("untouched"), 0600) != nil {
				t.Fatal("fixture")
			}
			ops.unlink = func(dir int, name string) error {
				if dir == n.secrets {
					secretAttempted = true
					if kind == "secret-unlink" {
						injected = true
						return fault
					}
					if kind == "secret-uncertain" {
						_ = unix.Unlinkat(dir, name, 0)
						injected = true
						return fault
					}
				}
				if dir == n.receipts {
					if kind == "receipt-unlink" {
						injected = true
						return fault
					}
					if kind == "receipt-uncertain" {
						_ = unix.Unlinkat(dir, name, 0)
						injected = true
						return fault
					}
				}
				return unix.Unlinkat(dir, name, 0)
			}
			ops.sync = func(fd int) error {
				if !secretAttempted && kind == "secret-file-sync" && fd != n.secrets {
					injected = true
					return fault
				}
				if secretAttempted && fd == n.secrets {
					switch kind {
					case "secret-sync":
						injected = true
						return fault
					case "cancel-after-secret":
						injected = true
						cancel()
					case "receipt-substitution":
						injected = true
						if os.Remove(receiptPath(b, e)) != nil || os.Symlink(sentinel, receiptPath(b, e)) != nil {
							t.Fatal("fixture")
						}
					}
				}
				if secretAttempted && fd == n.receipts && kind == "receipt-sync" {
					injected = true
					return fault
				}
				return unix.Fsync(fd)
			}
			if kind == "secret-close" {
				ops.closeFile = func(f *os.File) error { _ = f.Close(); injected = true; return fault }
			}
			got, err := n.pruneExact(ctx, e.AttemptID, b, ops, pruneClock(100+604800000000000))
			if !injected || !errors.Is(err, ErrLocalEvidence) || got.ReceiptRemoved {
				t.Fatal("PRUNE SECRET SYNC ALARM: failed removal or durability certified success")
			}
			keepReceipt := kind != "receipt-uncertain" && kind != "receipt-sync" && kind != "receipt-substitution"
			if keepReceipt {
				raw, readErr := os.ReadFile(receiptPath(b, e))
				if readErr != nil || len(raw) == 0 {
					t.Fatal("PRUNE RETENTION ALARM: failed secret removal erased receipt")
				}
			}
			if kind == "secret-unlink" || kind == "secret-file-sync" || kind == "secret-close" {
				if _, err := os.Stat(finalPath(b, e)); err != nil {
					t.Fatal("secret lost before admitted removal")
				}
			}
			if strings.Contains(err.Error(), "private-prune-fault") {
				t.Fatal("raw fault escaped")
			}
			raw, _ := os.ReadFile(sentinel)
			if string(raw) != "untouched" {
				t.Fatal("sentinel modified")
			}
		})
	}
}

// Orphan receipts remain counted rather than guessed old. The adjacent valid
// count permits a new private prepare; exhaustion permits zero new authority.
func TestPrepareReceiptCapacity(t *testing.T) {
	b, n := prepareFixture(t)
	e := fixtureEnvelope()
	for i := 0; i < MaxAttempts; i++ {
		name := fmt.Sprintf("%064x.json", i)
		if os.WriteFile(filepath.Join(b.Directory(), "receipts", name), []byte("retained orphan"), 0600) != nil {
			t.Fatal("fixture")
		}
	}
	if got, err := n.Prepare(bounded(t), e); got != nil || !errors.Is(err, ErrCapacity) {
		t.Fatal("ORPHAN CAPACITY ALARM: full retained receipts admitted authority")
	}
	if _, err := os.Stat(finalPath(b, e)); !os.IsNotExist(err) {
		t.Fatal("failed capacity wrote secret")
	}
	// Explicit fixture disposition creates one free slot; product code never
	// removes an orphan merely to pass capacity.
	if os.Remove(filepath.Join(b.Directory(), "receipts", fmt.Sprintf("%064x.json", 0))) != nil {
		t.Fatal("fixture")
	}
	if got, err := n.Prepare(bounded(t), e); err != nil || got == nil {
		t.Fatal("legitimate remaining capacity refused")
	}
}

// Capacity accepts the final/temp forms actually emitted by the receipt
// writer. Unknown names and unsafe inode metadata refuse without preparation
// or repair; a zero-length exclusive temp is legitimate retained residue.
func TestPrepareReceiptCapacityGuards(t *testing.T) {
	for _, kind := range []string{"final", "temp", "empty-temp", "unknown", "symlink", "hardlink", "mode", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			b, n := prepareFixture(t)
			e := fixtureEnvelope()
			name := strings.Repeat("d", 64) + ".json"
			if kind == "temp" || kind == "empty-temp" {
				name = ".receipt-" + strings.Repeat("d", 64) + ".tmp"
			}
			if kind == "unknown" {
				name = "unrecognized"
			}
			target := filepath.Join(b.Directory(), "receipts", name)
			sentinel := filepath.Join(b.base, "sentinel")
			if os.WriteFile(sentinel, []byte("untouched"), 0600) != nil {
				t.Fatal("fixture")
			}
			switch kind {
			case "symlink":
				if os.Symlink(sentinel, target) != nil {
					t.Fatal("fixture")
				}
			case "hardlink":
				if os.Link(sentinel, target) != nil {
					t.Fatal("fixture")
				}
			default:
				raw := []byte("retained")
				if kind == "empty-temp" {
					raw = nil
				}
				if kind == "oversize" {
					raw = bytes.Repeat([]byte("x"), MaxRecord+1)
				}
				mode := os.FileMode(0600)
				if kind == "mode" {
					mode = 0644
				}
				if os.WriteFile(target, raw, mode) != nil {
					t.Fatal("fixture")
				}
			}
			got, err := n.Prepare(bounded(t), e)
			valid := kind == "final" || kind == "temp" || kind == "empty-temp"
			if valid {
				if err != nil || got == nil {
					t.Fatal("legitimate retained receipt form refused")
				}
			} else if !errors.Is(err, ErrUnsafeLayout) || got != nil {
				t.Fatal("unsafe retained receipt namespace admitted")
			}
			raw, _ := os.ReadFile(sentinel)
			if string(raw) != "untouched" {
				t.Fatal("capacity repaired unsafe path")
			}
		})
	}
}
