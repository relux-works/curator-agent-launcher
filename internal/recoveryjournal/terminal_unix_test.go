//go:build darwin || linux

package recoveryjournal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"golang.org/x/sys/unix"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func terminalEntry(t *testing.T, b Binding, e Envelope) *Entry {
	t.Helper()
	raw, err := os.ReadFile(finalPath(b, e))
	if err != nil {
		t.Fatal(err)
	}
	entry, err := decodeEntry(raw, e.AttemptID, b)
	if err != nil {
		t.Fatal("invalid saved private entry")
	}
	return entry
}

// Receipt directory sync is deliberately delayed in a fake awake clock. The
// anchor must observe the later time and give no credit for publication delay.
// Same-boot retries preserve it; new boots restart without rewriting receipt.
func TestTerminalAfterDurabilityAndRetry(t *testing.T) {
	b, n, e, before := preparedReceiptFixture(t)
	now := uint64(5)
	samples := 0
	receiptDurable := false
	ops := nativeReceiptOperations()
	ops.sync = func(fd int) error {
		if fd == n.receipts {
			now = 900
			receiptDurable = true
		}
		return unix.Fsync(fd)
	}
	awake := func() (awakeObservation, error) {
		samples++
		if !receiptDurable {
			t.Error("ANCHOR ORDER ALARM: sampled before durable receipt")
		}
		return awakeObservation{fixtureBoot, now}, nil
	}
	got, err := n.recoverRetained(bounded(t), e.AttemptID, b, "finish", ops, confirmedFixture, receiptTime, nativeTerminalOperations(), awake)
	if err != nil || got.Receipt == nil || samples != 1 {
		t.Fatal("durable terminal result refused")
	}
	entry := terminalEntry(t, b, e)
	rawReceipt, _ := os.ReadFile(receiptPath(b, e))
	hash := sha256.Sum256(rawReceipt)
	if entry.retention == nil || entry.retention.AnchorNS != "900" || entry.retention.ReceiptSHA256 != hex.EncodeToString(hash[:]) {
		t.Fatal("ANCHOR ORDER ALARM: publication delay consumed retention")
	}
	anchor := *entry.retention
	assertEnvelopeRetained(t, b, e, before)
	for _, tc := range []struct {
		boot     string
		ns       uint64
		expected string
	}{{fixtureBoot, 1000, "900"}, {"22345678-1234-4234-8234-123456789abc", 3, "3"}} {
		clock := func() (awakeObservation, error) { return awakeObservation{tc.boot, tc.ns}, nil }
		wallJump := func() time.Time { return receiptTime().Add(365 * 24 * time.Hour) }
		got, err := n.recoverRetained(bounded(t), e.AttemptID, b, "recover", nativeReceiptOperations(), confirmedFixture, wallJump, nativeTerminalOperations(), clock)
		if err != nil || got.Receipt == nil {
			t.Fatal("exact ended-result recovery refused")
		}
		retry := terminalEntry(t, b, e)
		if retry.retention.AnchorNS != tc.expected || retry.retention.Boot != tc.boot {
			t.Fatal("retry retention incorrect")
		}
		after, _ := os.ReadFile(receiptPath(b, e))
		if !bytes.Equal(rawReceipt, after) {
			t.Fatal("immutable prior receipt rewritten")
		}
	}
	if retentionElapsed(anchor, awakeObservation{fixtureBoot, 900 + 604800000000000 - 1}) {
		t.Fatal("early retention eligibility")
	}
}

// Every failed terminal publication/clock step keeps original E plus the
// durable receipt, withholds the success pointer, and supports a clean retry
// unless an explicitly retained unknown temp requires custodian disposition.
func TestTerminalPublicationFaults(t *testing.T) {
	for _, kind := range []string{"write", "short-write", "file-sync", "close", "replace", "directory-sync", "cancel-write", "cancel-replace", "clock-error", "clock-invalid", "existing-temp", "cleanup-failure"} {
		t.Run(kind, func(t *testing.T) {
			b, n, e, before := preparedReceiptFixture(t)
			ctx, cancel := context.WithCancel(bounded(t))
			defer cancel()
			ops := nativeTerminalOperations()
			injected := false
			fault := errors.New("private-terminal-fault")
			awake := func() (awakeObservation, error) { return awakeObservation{fixtureBoot, 100}, nil }
			switch kind {
			case "write":
				ops.write = func(int, []byte) (int, error) { injected = true; return 0, fault }
			case "short-write":
				ops.write = func(fd int, raw []byte) (int, error) { injected = true; return unix.Write(fd, raw[:len(raw)/2]) }
			case "file-sync", "directory-sync":
				ops.sync = func(fd int) error {
					if (fd == n.secrets) == (kind == "directory-sync") {
						injected = true
						return fault
					}
					return unix.Fsync(fd)
				}
			case "close":
				ops.close = func(fd int) error { _ = unix.Close(fd); injected = true; return fault }
			case "replace":
				ops.replace = func(int, string, string) error { injected = true; return fault }
			case "cancel-write":
				ops.write = func(fd int, raw []byte) (int, error) {
					w, err := unix.Write(fd, raw)
					injected = true
					cancel()
					return w, err
				}
			case "cancel-replace":
				ops.replace = func(dir int, old, next string) error {
					err := unix.Renameat(dir, old, dir, next)
					injected = true
					cancel()
					return err
				}
			case "clock-error":
				awake = func() (awakeObservation, error) { injected = true; return awakeObservation{}, fault }
			case "clock-invalid":
				awake = func() (awakeObservation, error) {
					injected = true
					return awakeObservation{Boot: "unknown", NS: 100}, nil
				}
			case "existing-temp":
				injected = true
				if os.WriteFile(filepath.Join(b.Directory(), "secrets", ".prepare-"+e.AttemptID+".tmp"), []byte("foreign"), 0600) != nil {
					t.Fatal("fixture")
				}
			case "cleanup-failure":
				ops.write = func(int, []byte) (int, error) { injected = true; return 0, fault }
				ops.unlink = func(int, string) error { return fault }
			}
			got, err := n.recoverRetained(ctx, e.AttemptID, b, "recover", nativeReceiptOperations(), confirmedFixture, receiptTime, ops, awake)
			if !injected || !errors.Is(err, ErrLocalEvidence) || !got.Confirmed || got.Receipt != nil {
				t.Fatal("TERMINAL FAILURE ALARM: uncertain publication certified success")
			}
			if strings.Contains(err.Error(), "private-terminal-fault") {
				t.Fatal("private diagnostic escaped")
			}
			entry := terminalEntry(t, b, e)
			if entry.envelope != e {
				t.Fatal("authority lost")
			}
			if kind != "directory-sync" && kind != "cancel-replace" {
				assertAuthorityUnchanged(t, b, e, before)
			}
			raw, _ := os.ReadFile(receiptPath(b, e))
			if _, ok := decodeReceipt(raw); !ok {
				t.Fatal("durable receipt lost after terminal failure")
			}
			retry, err := n.recoverRetained(bounded(t), e.AttemptID, b, "recover", nativeReceiptOperations(), confirmedFixture, receiptTime, nativeTerminalOperations(), func() (awakeObservation, error) { return awakeObservation{fixtureBoot, 200}, nil })
			if kind == "existing-temp" || kind == "cleanup-failure" {
				if !errors.Is(err, ErrLocalEvidence) || retry.Receipt != nil {
					t.Fatal("unknown residue overwritten")
				}
			} else if err != nil || retry.Receipt == nil {
				t.Fatal("clean exact retry failed")
			}
		})
	}
}

// Regressing clocks, changed immutable receipt bytes, or a conflicting ended
// outcome preserve both records. Retained proof is still used for exact result
// authentication and is never interpreted as new issuance authority.
func TestTerminalRetryRefusals(t *testing.T) {
	for _, kind := range []string{"regress", "hash", "outcome"} {
		t.Run(kind, func(t *testing.T) {
			b, n, e, _ := preparedReceiptFixture(t)
			clock := func() (awakeObservation, error) { return awakeObservation{fixtureBoot, 100}, nil }
			if got, err := n.recoverRetained(bounded(t), e.AttemptID, b, "recover", nativeReceiptOperations(), confirmedFixture, receiptTime, nativeTerminalOperations(), clock); err != nil || got.Receipt == nil {
				t.Fatal("fixture")
			}
			before, _ := os.ReadFile(finalPath(b, e))
			receiptBefore, _ := os.ReadFile(receiptPath(b, e))
			confirm := confirmedFixture
			switch kind {
			case "regress":
				clock = func() (awakeObservation, error) { return awakeObservation{fixtureBoot, 99}, nil }
			case "hash":
				r, ok := decodeReceipt(receiptBefore)
				if !ok {
					t.Fatal("fixture")
				}
				r.ObservedUTC = "2099-01-01T00:00:00Z"
				receiptBefore, _ = encodeReceipt(r)
				if os.WriteFile(receiptPath(b, e), receiptBefore, 0600) != nil {
					t.Fatal("fixture")
				}
			case "outcome":
				confirm = func(context.Context, Envelope) (serviceObservation, error) {
					return serviceObservation{nil, "cancelled_before_issue", "boot"}, nil
				}
			}
			got, err := n.recoverRetained(bounded(t), e.AttemptID, b, "recover", nativeReceiptOperations(), confirm, receiptTime, nativeTerminalOperations(), clock)
			want := ErrLocalEvidence
			if kind == "outcome" {
				want = ErrRecoveryUnknown
			}
			if !errors.Is(err, want) || got.Receipt != nil {
				t.Fatal("invalid terminal retry accepted")
			}
			assertAuthorityUnchanged(t, b, e, before)
			after, _ := os.ReadFile(receiptPath(b, e))
			if !bytes.Equal(receiptBefore, after) {
				t.Fatal("receipt replaced on uncertain retry")
			}
		})
	}
}

// Actual local kernel observations are available and nondecreasing in one
// boot. This does not simulate a suspend/reboot or execute on another OS.
func TestNativeAwakeObservation(t *testing.T) {
	a, err := nativeAwake()
	if err != nil {
		t.Fatal("native awake clock unavailable")
	}
	b, err := nativeAwake()
	if err != nil || !bootUUID(a.Boot) || a.Boot != b.Boot || b.NS < a.NS {
		t.Fatal("invalid native awake clock")
	}
	for _, tc := range []struct{ sec, ns int64 }{{-1, 0}, {0, -1}, {0, 1000000000}, {math.MaxInt64, 0}} {
		if _, err := awakeTimespec(fixtureBoot, tc.sec, tc.ns); err != ErrLocalEvidence {
			t.Fatal("invalid native time accepted")
		}
	}
	if a, err := awakeTimespec(fixtureBoot, 2, 3); err != nil || a.NS != 2000000003 {
		t.Fatal("valid native time refused")
	}
}

// A substituted original/temp cannot be overwritten even if it occurs after
// the guarded initial read. The fixture deliberately swaps only its own path;
// the external sentinel and substituted inode remain untouched on refusal.
func TestTerminalPublicationCustody(t *testing.T) {
	for _, kind := range []string{"original-symlink", "original-hardlink", "temp-mode", "temp-symlink"} {
		t.Run(kind, func(t *testing.T) {
			b, n, e, _ := preparedReceiptFixture(t)
			sentinel := filepath.Join(b.base, "sentinel")
			if err := os.WriteFile(sentinel, []byte("untouched"), 0600); err != nil {
				t.Fatal(err)
			}
			ops := nativeTerminalOperations()
			nativeClose := ops.close
			ops.close = func(fd int) error {
				if err := nativeClose(fd); err != nil {
					return err
				}
				target := finalPath(b, e)
				if strings.HasPrefix(kind, "temp-") {
					target = filepath.Join(b.Directory(), "secrets", ".prepare-"+e.AttemptID+".tmp")
				}
				if kind == "temp-mode" {
					return os.Chmod(target, 0644)
				}
				if err := os.Remove(target); err != nil {
					return err
				}
				if kind == "original-hardlink" {
					return os.Link(sentinel, target)
				}
				return os.Symlink(sentinel, target)
			}
			clock := func() (awakeObservation, error) { return awakeObservation{fixtureBoot, 100}, nil }
			got, err := n.recoverRetained(bounded(t), e.AttemptID, b, "recover", nativeReceiptOperations(), confirmedFixture, receiptTime, ops, clock)
			if !errors.Is(err, ErrLocalEvidence) || got.Receipt != nil {
				t.Fatal("substituted private path overwritten")
			}
			raw, _ := os.ReadFile(sentinel)
			if string(raw) != "untouched" {
				t.Fatal("sentinel modified")
			}
		})
	}
}
