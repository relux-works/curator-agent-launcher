//go:build darwin || linux

package recoveryjournal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/netproxy"
	"golang.org/x/sys/unix"
)

func receiptPath(b Binding, e Envelope) string {
	return filepath.Join(b.Directory(), "receipts", e.AttemptID+".json")
}
func receiptTime() time.Time { return time.Date(2026, 11, 4, 3, 30, 0, 0, time.UTC) }
func confirmedFixture(context.Context, Envelope) (serviceObservation, error) {
	launch := strings.Repeat("d", 64)
	return serviceObservation{&launch, "revoked", "observed-boot"}, nil
}
func preparedReceiptFixture(t *testing.T) (Binding, *Namespace, Envelope, []byte) {
	t.Helper()
	b, n := prepareFixture(t)
	e := fixtureEnvelope()
	if _, err := n.Prepare(bounded(t), e); err != nil {
		t.Fatal("fixture prepare failed")
	}
	raw, err := os.ReadFile(finalPath(b, e))
	if err != nil {
		t.Fatal(err)
	}
	return b, n, e, raw
}
func assertAuthorityUnchanged(t *testing.T, b Binding, e Envelope, before []byte) {
	t.Helper()
	raw, err := os.ReadFile(finalPath(b, e))
	if err != nil || !bytes.Equal(before, raw) {
		t.Fatal("original recovery authority changed or disappeared")
	}
}

// A successful format transition may replace bytes, but must preserve every
// original envelope field including the proof for authenticated idempotent readback.
func assertEnvelopeRetained(t *testing.T, b Binding, e Envelope, before []byte) {
	t.Helper()
	original, err := decodeEntry(before, e.AttemptID, b)
	raw, readErr := os.ReadFile(finalPath(b, e))
	retained, decodeErr := decodeEntry(raw, e.AttemptID, b)
	if err != nil || readErr != nil || decodeErr != nil || original.envelope != retained.envelope || retained.envelope != e || retained.retention == nil {
		t.Fatal("original E was lost in terminal transition")
	}
}

// The pointer follows complete receipt publication, never just service success.
// Repeated authenticated calls re-sync an identical prior record, retaining E.
func TestRecoverReceiptCommitAndRetry(t *testing.T) {
	b, n, e, before := preparedReceiptFixture(t)
	ops := nativeReceiptOperations()
	var order []string
	ops.write = func(fd int, raw []byte) (int, error) { order = append(order, "write"); return unix.Write(fd, raw) }
	ops.sync = func(fd int) error {
		if fd == n.receipts {
			order = append(order, "directory-sync")
		} else {
			order = append(order, "file-sync")
		}
		return unix.Fsync(fd)
	}
	ops.close = func(fd int) error { order = append(order, "close"); return unix.Close(fd) }
	ops.renameNew = func(dir int, old, next string) error {
		order = append(order, "rename")
		return renameNew(dir, old, next)
	}
	got, err := n.recoverExact(bounded(t), e.AttemptID, b, "finish", ops, confirmedFixture, receiptTime)
	if err != nil || !got.Confirmed || got.Receipt == nil || got.Receipt.AttemptID() != e.AttemptID || got.Receipt.Outcome() != "revoked" {
		t.Fatal("valid durable completion refused")
	}
	if !reflect.DeepEqual(order, []string{"write", "file-sync", "close", "rename", "directory-sync"}) {
		t.Fatal("RECEIPT ORDER ALARM: pointer returned before complete publication")
	}
	raw, err := os.ReadFile(receiptPath(b, e))
	r, ok := decodeReceipt(raw)
	if err != nil || !ok || r.Operation != "finish" || r.ServiceBoot != "observed-boot" {
		t.Fatal("committed receipt projection invalid")
	}
	for _, private := range []string{e.RecoveryProof, e.Root, b.base, b.socket} {
		if bytes.Contains(raw, []byte(private)) {
			t.Fatal("private authority/routing in receipt")
		}
	}
	writes, syncs := 0, 0
	retryOps := nativeReceiptOperations()
	retryOps.write = func(int, []byte) (int, error) { writes++; return 0, errors.New("unexpected rewrite") }
	retryOps.sync = func(fd int) error { syncs++; return unix.Fsync(fd) }
	later := func() time.Time { return receiptTime().Add(24 * time.Hour) }
	retry, err := n.recoverExact(bounded(t), e.AttemptID, b, "recover", retryOps, confirmedFixture, later)
	after, _ := os.ReadFile(receiptPath(b, e))
	if err != nil || retry.Receipt == nil || writes != 0 || syncs != 2 || !bytes.Equal(raw, after) {
		t.Fatal("exact receipt retry rewrote or skipped durability")
	}
	assertEnvelopeRetained(t, b, e, before)
}

// Every local publication fault keeps service confirmation separate, returns
// no committed pointer, and preserves original recovery authority for retry.
func TestRecoverReceiptPublicationFailures(t *testing.T) {
	for _, stage := range []string{"write", "short-write", "file-sync", "close", "rename", "directory-sync", "cancel-write", "cancel-after-rename"} {
		t.Run(stage, func(t *testing.T) {
			b, n, e, before := preparedReceiptFixture(t)
			ctx, cancel := context.WithCancel(bounded(t))
			defer cancel()
			ops := nativeReceiptOperations()
			fault := errors.New("private-fault-must-not-escape")
			injected := false
			switch stage {
			case "write":
				ops.write = func(int, []byte) (int, error) { injected = true; return 0, fault }
			case "short-write":
				ops.write = func(fd int, raw []byte) (int, error) { injected = true; return unix.Write(fd, raw[:len(raw)/2]) }
			case "file-sync", "directory-sync":
				ops.sync = func(fd int) error {
					if !injected && (fd == n.receipts) == (stage == "directory-sync") {
						injected = true
						return fault
					}
					return unix.Fsync(fd)
				}
			case "close":
				ops.close = func(fd int) error { _ = unix.Close(fd); injected = true; return fault }
			case "rename":
				ops.renameNew = func(int, string, string) error { injected = true; return fault }
			case "cancel-write":
				ops.write = func(fd int, raw []byte) (int, error) {
					injected = true
					n, err := unix.Write(fd, raw)
					cancel()
					return n, err
				}
			case "cancel-after-rename":
				ops.renameNew = func(dir int, old, next string) error {
					err := renameNew(dir, old, next)
					injected = true
					cancel()
					return err
				}
			}
			got, err := n.recoverExact(ctx, e.AttemptID, b, "recover", ops, confirmedFixture, receiptTime)
			if !injected || !errors.Is(err, ErrLocalEvidence) || !got.Confirmed || got.Outcome != "revoked" || got.Receipt != nil {
				t.Fatal("LOCAL EVIDENCE ALARM: failed receipt certified local success")
			}
			if strings.Contains(fmt.Sprintf("%+v %#v", err, got), "private-fault") {
				t.Fatal("raw local diagnostic escaped")
			}
			assertAuthorityUnchanged(t, b, e, before)
			// A clean later exact call can complete either an absent receipt or an
			// already published but previously unconfirmed directory-sync result.
			retry, err := n.recoverExact(bounded(t), e.AttemptID, b, "recover", nativeReceiptOperations(), confirmedFixture, receiptTime)
			if err != nil || retry.Receipt == nil {
				t.Fatal("later exact receipt retry failed")
			}
		})
	}
}

// Existing bytes are not proof of an earlier successful directory sync.
// Re-sync faults still withhold a pointer and preserve both prior receipt/E.
func TestRecoverExistingReceiptSyncFailures(t *testing.T) {
	for _, stage := range []string{"file-sync", "close", "directory-sync"} {
		t.Run(stage, func(t *testing.T) {
			b, n, e, before := preparedReceiptFixture(t)
			if got, err := n.recoverExact(bounded(t), e.AttemptID, b, "recover", nativeReceiptOperations(), confirmedFixture, receiptTime); err != nil || got.Receipt == nil {
				t.Fatal("fixture receipt failed")
			}
			before, _ = os.ReadFile(finalPath(b, e)) // compare against the already committed terminal bytes
			raw, _ := os.ReadFile(receiptPath(b, e))
			ops := nativeReceiptOperations()
			fault := errors.New("private-resync-fault")
			injected := false
			if stage == "close" {
				ops.closeFile = func(f *os.File) error { _ = f.Close(); injected = true; return fault }
			} else {
				ops.sync = func(fd int) error {
					if (fd == n.receipts) == (stage == "directory-sync") {
						injected = true
						return fault
					}
					return unix.Fsync(fd)
				}
			}
			got, err := n.recoverExact(bounded(t), e.AttemptID, b, "finish", ops, confirmedFixture, receiptTime)
			if !injected || !errors.Is(err, ErrLocalEvidence) || !got.Confirmed || got.Receipt != nil {
				t.Fatal("LOCAL EVIDENCE ALARM: failed re-sync certified local success")
			}
			after, _ := os.ReadFile(receiptPath(b, e))
			if !bytes.Equal(raw, after) {
				t.Fatal("prior receipt rewritten on re-sync failure")
			}
			assertAuthorityUnchanged(t, b, e, before)
		})
	}
}

// A local observation-clock formatting failure does not erase the independently
// confirmed service result, and cannot produce a persisted receipt pointer.
func TestRecoverReceiptClockFailure(t *testing.T) {
	b, n, e, before := preparedReceiptFixture(t)
	badClock := func() time.Time { return time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) }
	got, err := n.recoverExact(bounded(t), e.AttemptID, b, "recover", nativeReceiptOperations(), confirmedFixture, badClock)
	if !errors.Is(err, ErrLocalEvidence) || !got.Confirmed || got.Receipt != nil {
		t.Fatal("local clock failure confused with service confirmation")
	}
	if _, err := os.Stat(receiptPath(b, e)); !os.IsNotExist(err) {
		t.Fatal("invalid observation clock wrote receipt")
	}
	assertAuthorityUnchanged(t, b, e, before)
}

// Confirmation loss never writes a receipt. Bad selectors and changed binding
// never call the service. The original private record is unchanged in each case.
func TestRecoverUncertaintyAndInputGuards(t *testing.T) {
	for _, kind := range []string{"transport", "invalid-result", "binding", "attempt", "operation", "cancel", "no-deadline"} {
		t.Run(kind, func(t *testing.T) {
			b, n, e, before := preparedReceiptFixture(t)
			current := b
			attempt := e.AttemptID
			operation := "recover"
			ctx := bounded(t)
			calls := 0
			switch kind {
			case "binding":
				current.profile = strings.Repeat("e", 64)
			case "attempt":
				attempt = "../bad"
			case "operation":
				operation = "register"
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "no-deadline":
				ctx = context.Background()
			}
			confirm := func(context.Context, Envelope) (serviceObservation, error) {
				calls++
				if kind == "transport" {
					return serviceObservation{}, errors.New("private-service-diagnostic")
				}
				return serviceObservation{nil, "unknown", "boot"}, nil
			}
			got, err := n.recoverExact(ctx, attempt, current, operation, nativeReceiptOperations(), confirm, receiptTime)
			if err == nil || got.Confirmed || got.Receipt != nil {
				t.Fatal("unconfirmed cleanup certified")
			}
			if kind != "transport" && kind != "invalid-result" && calls != 0 {
				t.Fatal("invalid input sent recovery request")
			}
			if _, err := os.Lstat(receiptPath(b, e)); !os.IsNotExist(err) {
				t.Fatal("unconfirmed call wrote receipt")
			}
			if strings.Contains(fmt.Sprintf("%+v", err), "private-service") {
				t.Fatal("raw service diagnostic escaped")
			}
			assertAuthorityUnchanged(t, b, e, before)
		})
	}
}

// Unsafe final/temp records are never replaced. Cleanup failure leaves only
// one bounded per-attempt residue and subsequent recovery refuses its reuse.
func TestRecoverReceiptCustodyAndResidue(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "mode", "malformed", "foreign", "existing-temp", "cleanup-failure"} {
		t.Run(kind, func(t *testing.T) {
			b, n, e, before := preparedReceiptFixture(t)
			final := receiptPath(b, e)
			temp := filepath.Join(b.Directory(), "receipts", ".receipt-"+e.AttemptID+".tmp")
			sentinel := filepath.Join(b.base, "sentinel")
			if err := os.WriteFile(sentinel, []byte("unchanged"), 0600); err != nil {
				t.Fatal(err)
			}
			ops := nativeReceiptOperations()
			switch kind {
			case "symlink":
				if os.Symlink(sentinel, final) != nil {
					t.Fatal("fixture failed")
				}
			case "hardlink":
				if os.Link(sentinel, final) != nil {
					t.Fatal("fixture failed")
				}
			case "mode":
				if os.WriteFile(final, []byte("unsafe"), 0644) != nil {
					t.Fatal("fixture failed")
				}
			case "malformed":
				if os.WriteFile(final, []byte("{}"), 0600) != nil {
					t.Fatal("fixture failed")
				}
			case "foreign":
				r := receiptFixture()
				r.AttemptID = strings.Repeat("e", 64)
				r.ProfileDigest = b.profile
				raw, _ := json.Marshal(r)
				if os.WriteFile(final, raw, 0600) != nil {
					t.Fatal("fixture failed")
				}
			case "existing-temp":
				if os.WriteFile(temp, []byte("foreign-temp"), 0600) != nil {
					t.Fatal("fixture failed")
				}
			case "cleanup-failure":
				ops.write = func(int, []byte) (int, error) { return 0, errors.New("fault") }
				ops.unlink = func(int, string) error { return errors.New("fault") }
			}
			got, err := n.recoverExact(bounded(t), e.AttemptID, b, "recover", ops, confirmedFixture, receiptTime)
			if !errors.Is(err, ErrLocalEvidence) || !got.Confirmed || got.Receipt != nil {
				t.Fatal("unsafe local evidence accepted")
			}
			if kind == "cleanup-failure" {
				info, err := os.Stat(temp)
				if err != nil || info.Size() != 0 {
					t.Fatal("bounded residue not retained")
				}
				retry, err := n.recoverExact(bounded(t), e.AttemptID, b, "recover", nativeReceiptOperations(), confirmedFixture, receiptTime)
				if !errors.Is(err, ErrLocalEvidence) || retry.Receipt != nil {
					t.Fatal("unknown temp overwritten")
				}
			}
			raw, _ := os.ReadFile(sentinel)
			if string(raw) != "unchanged" {
				t.Fatal("unrelated sentinel modified")
			}
			assertAuthorityUnchanged(t, b, e, before)
		})
	}
}

// Same-attempt recoveries serialize and re-sync one immutable receipt; separate
// attempts retain independent entries and cannot cross their outcomes.
func TestRecoverConcurrentExactAttempts(t *testing.T) {
	b, n, e, before := preparedReceiptFixture(t)
	second := e
	second.AttemptID = strings.Repeat("e", 64)
	second.RecoveryProof = strings.Repeat("f", 64)
	if _, err := n.Prepare(bounded(t), second); err != nil {
		t.Fatal("second prepare failed")
	}
	var running atomic.Int32
	var wg sync.WaitGroup
	confirm := func(ctx context.Context, got Envelope) (serviceObservation, error) {
		if running.Add(1) != 1 {
			t.Error("namespace lock failed serialization")
		}
		defer running.Add(-1)
		return confirmedFixture(ctx, got)
	}
	for _, attempt := range []string{e.AttemptID, e.AttemptID, second.AttemptID} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := n.recoverExact(bounded(t), attempt, b, "recover", nativeReceiptOperations(), confirm, receiptTime)
			if err != nil || got.Receipt == nil || got.Receipt.AttemptID() != attempt {
				t.Error("concurrent exact completion failed")
			}
		}()
	}
	wg.Wait()
	assertEnvelopeRetained(t, b, e, before)
}

// Production recovery sends only health and the original exact E to its Unix
// endpoint. Changed authority/malformed/foreign replies yield no local success.
func TestRecoverRealUnixWire(t *testing.T) {
	for _, kind := range []string{"valid", "not-ready", "cancelled-before-issue", "service-mismatch", "not-ready-mismatch", "clock-unavailable", "false-durable", "foreign", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			b := fixtureBinding(t)
			short, err := os.MkdirTemp("", "rr-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(short) })
			short, err = filepath.EvalSymlinks(short)
			if err != nil {
				t.Fatal(err)
			}
			b.socket = filepath.Join(short, "control.sock")
			fixtureLayout(t, b)
			n, err := OpenExisting(b)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = n.Close() })
			e := fixtureEnvelope()
			if _, err := n.Prepare(bounded(t), e); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(finalPath(b, e))
			listener, err := net.Listen("unix", b.socket)
			if err != nil {
				t.Fatal("fixture socket failed")
			}
			var health, recoveries atomic.Int32
			server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set(netproxy.VersionHeader, netproxy.RecoveryAPI)
				if r.Method == "GET" && r.URL.Path == "/v2/health" {
					health.Add(1)
					if kind == "clock-unavailable" {
						w.WriteHeader(http.StatusServiceUnavailable)
						_, _ = io.WriteString(w, `{"api":"netproxy-control/2","error":"clock_unavailable"}`)
						return
					}
					registry := e.RegistryID
					if kind == "service-mismatch" || kind == "not-ready-mismatch" {
						registry = strings.Repeat("f", 64)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"api": netproxy.RecoveryAPI, "ready": !strings.HasPrefix(kind, "not-ready"), "rule_set": "fixture", "key_mode": "fixture", "registry_id": registry, "boot": "observed-boot", "now_ns": "100", "recovery": netproxy.RecoveryProtocol})
					return
				}
				recoveries.Add(1)
				raw, _ := io.ReadAll(io.LimitReader(r.Body, netproxy.RecoveryMaxBody+1))
				var got Envelope
				if r.Method != "POST" || r.URL.Path != "/v2/recover" || json.Unmarshal(raw, &got) != nil || got != e {
					t.Error("request changed original issuance envelope")
				}
				f := map[string]any{"api": netproxy.RecoveryAPI, "attempt_id": e.AttemptID, "launch_id": strings.Repeat("d", 64), "outcome": "revoked", "durable": true}
				switch kind {
				case "cancelled-before-issue":
					f["launch_id"] = nil
					f["outcome"] = "cancelled_before_issue"
				case "false-durable":
					f["durable"] = false
				case "foreign":
					f["attempt_id"] = strings.Repeat("f", 64)
				case "malformed":
					_, _ = io.WriteString(w, "{}")
					return
				}
				_ = json.NewEncoder(w).Encode(f)
			})}
			done := make(chan struct{})
			go func() { defer close(done); _ = server.Serve(listener) }()
			t.Cleanup(func() { _ = server.Close(); <-done })
			got, err := n.RecoverExact(bounded(t), e.AttemptID, b, "recover")
			if kind == "valid" || kind == "not-ready" || kind == "cancelled-before-issue" {
				if err != nil || !got.Confirmed || got.Receipt == nil {
					t.Fatal("valid authenticated receipt refused")
				}
			} else if err == nil || got.Confirmed || got.Receipt != nil {
				t.Fatal("RECOVERY RECEIPT ALARM: unconfirmed service certified")
			}
			want := int32(1)
			if kind == "service-mismatch" || kind == "not-ready-mismatch" || kind == "clock-unavailable" {
				want = 0
			}
			if health.Load() != 1 || recoveries.Load() != want {
				t.Fatal("unexpected service request count")
			}
			if kind == "valid" || kind == "not-ready" || kind == "cancelled-before-issue" {
				assertEnvelopeRetained(t, b, e, before)
			} else {
				assertAuthorityUnchanged(t, b, e, before)
			}
		})
	}
}
