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
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/netproxy"
	"golang.org/x/sys/unix"
)

// Every failed publication boundary sends zero requests and yields no runtime
// capability. The valid neighbor observes the committed original E first.
func TestIssueRequiresDurablePrepare(t *testing.T) {
	for _, stage := range []string{"valid", "write", "short-write", "file-sync", "close", "rename", "directory-sync"} {
		t.Run(stage, func(t *testing.T) {
			b, n := prepareFixture(t)
			e := fixtureEnvelope()
			ops := nativePrepareOperations()
			fault := errors.New("fixture-private-fault")
			switch stage {
			case "write":
				ops.write = func(int, []byte) (int, error) { return 0, fault }
			case "short-write":
				ops.write = func(fd int, raw []byte) (int, error) { return unix.Write(fd, raw[:len(raw)/2]) }
			case "file-sync", "directory-sync":
				ops.sync = func(fd int) error {
					if (fd == n.secrets) == (stage == "directory-sync") {
						return fault
					}
					return unix.Fsync(fd)
				}
			case "close":
				ops.close = func(fd int) error { _ = unix.Close(fd); return fault }
			case "rename":
				ops.renameNew = func(int, string, string) error { return fault }
			}
			calls := 0
			register := func(_ context.Context, got Envelope) (netproxy.RecoveryRegistration, error) {
				calls++
				raw, err := os.ReadFile(finalPath(b, e))
				var saved secretRecord
				if err != nil || json.Unmarshal(raw, &saved) != nil || got != e || saved.Envelope != e {
					t.Error("ISSUANCE ORDER ALARM: request before private prepare")
				}
				return netproxy.RecoveryRegistration{AttemptID: e.AttemptID}, nil
			}
			result, err := n.issue(bounded(t), e, b, ops, register)
			if stage == "valid" {
				if err != nil || result.AttemptID != e.AttemptID || calls != 1 {
					t.Fatal("valid prepared issuance refused")
				}
			} else if err == nil || result.AttemptID != "" || calls != 0 {
				t.Fatal("ISSUANCE ORDER ALARM: failed prepare authorized request")
			}
		})
	}
}

// Lost/refused registration responses keep byte-identical recovery authority.
// Retrying Issue refuses locally instead of repeating issuance or minting E.
func TestIssueUncertaintyKeepsOriginalAuthority(t *testing.T) {
	b, n := prepareFixture(t)
	e := fixtureEnvelope()
	calls := 0
	var before []byte
	register := func(context.Context, Envelope) (netproxy.RecoveryRegistration, error) {
		calls++
		before, _ = os.ReadFile(finalPath(b, e))
		return netproxy.RecoveryRegistration{AttemptID: e.AttemptID}, errors.New("fixture-secret-transport-payload")
	}
	result, err := n.issue(bounded(t), e, b, nativePrepareOperations(), register)
	if !errors.Is(err, ErrIssueUnknown) || result.AttemptID != "" || calls != 1 || strings.Contains(fmt.Sprintf("%+v %#v", err, err), "fixture-secret") {
		t.Fatal("registration uncertainty leaked or returned capability")
	}
	after, _ := os.ReadFile(finalPath(b, e))
	if len(before) == 0 || !bytes.Equal(before, after) {
		t.Fatal("original recovery authority changed")
	}
	if result, err = n.issue(bounded(t), e, b, nativePrepareOperations(), register); !errors.Is(err, ErrExisting) || result.AttemptID != "" || calls != 1 {
		t.Fatal("ISSUANCE REPLAY ALARM: original attempt registered twice")
	}
	entry, err := n.ReadExact(bounded(t), e.AttemptID, b)
	if err != nil || entry.envelope != e {
		t.Fatal("lost-reply entry cannot be recovered exactly")
	}
}

// Binding changes, cancelled/deadline-free calls and invalid E produce no
// journal/lock write or request. A fresh call in another fixture remains valid.
func TestIssueRejectedInputsHaveNoEffects(t *testing.T) {
	for _, kind := range []string{"binding", "cancel", "deadline", "envelope", "closed"} {
		t.Run(kind, func(t *testing.T) {
			b, n := prepareFixture(t)
			e := fixtureEnvelope()
			current := b
			ctx := bounded(t)
			switch kind {
			case "binding":
				current.profile = strings.Repeat("d", 64)
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "deadline":
				ctx = context.Background()
			case "envelope":
				e.RecoveryProof = "invalid"
			case "closed":
				if n.Close() != nil {
					t.Fatal("close failed")
				}
			}
			calls := 0
			register := func(context.Context, Envelope) (netproxy.RecoveryRegistration, error) {
				calls++
				return netproxy.RecoveryRegistration{}, nil
			}
			if result, err := n.issue(ctx, e, current, nativePrepareOperations(), register); err == nil || result.AttemptID != "" || calls != 0 {
				t.Fatal("rejected input had effect")
			}
			if _, err := os.Lstat(filepath.Join(b.Directory(), "lock")); !os.IsNotExist(err) {
				t.Fatal("rejected input created lock")
			}
			files, err := os.ReadDir(filepath.Join(b.Directory(), "secrets"))
			if err != nil || len(files) != 0 {
				t.Fatal("rejected input created secret")
			}
		})
	}
}

// The production method uses only the original Unix endpoint and validated
// wire reply. Returned token stays out of the unchanged journal and diagnostics.
func TestIssueRealUnixTransport(t *testing.T) {
	b := fixtureBinding(t)
	short, err := os.MkdirTemp("", "rj-")
	if err != nil {
		t.Fatal("fixture socket directory unavailable")
	}
	t.Cleanup(func() { _ = os.RemoveAll(short) })
	short, err = filepath.EvalSymlinks(short)
	if err != nil {
		t.Fatal("fixture socket path unavailable")
	}
	b.socket = filepath.Join(short, "control.sock")
	fixtureLayout(t, b)
	n, err := OpenExisting(b)
	if err != nil {
		t.Fatal("fixture namespace unavailable")
	}
	t.Cleanup(func() {
		if n.Close() != nil {
			t.Error("fixture namespace close failed")
		}
	})
	e := fixtureEnvelope()
	listener, err := net.Listen("unix", b.socket)
	if err != nil {
		t.Fatal("fixture socket unavailable")
	}
	var calls atomic.Int32
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		raw, _ := io.ReadAll(io.LimitReader(r.Body, netproxy.RecoveryMaxBody+1))
		var got Envelope
		saved, readErr := os.ReadFile(finalPath(b, e))
		var record secretRecord
		if r.Method != "POST" || r.URL.Path != "/v2/register" || json.Unmarshal(raw, &got) != nil || got != e || readErr != nil || json.Unmarshal(saved, &record) != nil || record.Envelope != e {
			t.Error("ISSUANCE ORDER ALARM: production request without exact prepare")
		}
		w.Header().Set(netproxy.VersionHeader, netproxy.RecoveryAPI)
		_ = json.NewEncoder(w).Encode(map[string]any{"api": netproxy.RecoveryAPI, "attempt_id": e.AttemptID, "launch_id": strings.Repeat("d", 64), "token": strings.Repeat("e", 64), "token_header": "X-Netproxy-Token", "anthropic_base": "http://127.0.0.1:1234/anthropic", "openai_base": "http://127.0.0.1:1234/openai/v1", "chatgpt_base": "http://127.0.0.1:1234/chatgpt", "activation_seconds": 120})
	})}
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close(); <-done })
	reg, err := n.Issue(bounded(t), e, b)
	if err != nil || calls.Load() != 1 || reg.Registration.Token != strings.Repeat("e", 64) {
		t.Fatal("real prepared issuance failed")
	}
	raw, err := os.ReadFile(finalPath(b, e))
	if err != nil || bytes.Contains(raw, []byte(reg.Registration.Token)) || strings.Contains(fmt.Sprintf("%+v %#v", reg, reg), reg.Registration.Token) {
		t.Fatal("runtime capability leaked into journal/diagnostic")
	}
}

// Same-attempt callers produce only one registration. A distinct prepared
// attempt can register independently; no global reject-all implementation passes.
func TestIssueConcurrentAttempts(t *testing.T) {
	for _, same := range []bool{true, false} {
		t.Run(fmt.Sprint(same), func(t *testing.T) {
			b, n := prepareFixture(t)
			envelopes := []Envelope{fixtureEnvelope(), fixtureEnvelope()}
			if !same {
				envelopes[1].AttemptID = strings.Repeat("d", 64)
				envelopes[1].RecoveryProof = strings.Repeat("e", 64)
			}
			var calls, successes, duplicates atomic.Int32
			register := func(_ context.Context, e Envelope) (netproxy.RecoveryRegistration, error) {
				calls.Add(1)
				return netproxy.RecoveryRegistration{AttemptID: e.AttemptID}, nil
			}
			var wg sync.WaitGroup
			start := make(chan struct{})
			for _, e := range envelopes {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					_, err := n.issue(bounded(t), e, b, nativePrepareOperations(), register)
					if err == nil {
						successes.Add(1)
					} else if errors.Is(err, ErrExisting) {
						duplicates.Add(1)
					} else {
						t.Error("unexpected concurrent issuance refusal")
					}
				}()
			}
			close(start)
			wg.Wait()
			want := int32(2)
			if same {
				want = 1
			}
			if calls.Load() != want || successes.Load() != want || duplicates.Load() != 2-want {
				t.Fatal("ISSUANCE REPLAY ALARM: duplicate or missing registration")
			}
		})
	}
}

// Cancellation at the last prepare sync retains recoverable E but sends no
// register. A post-prepare namespace displacement is also rejected, not repaired.
func TestIssuePrepareBoundaryChanges(t *testing.T) {
	for _, kind := range []string{"cancel", "displaced"} {
		t.Run(kind, func(t *testing.T) {
			b, n := prepareFixture(t)
			e := fixtureEnvelope()
			ctx, cancel := context.WithCancel(bounded(t))
			defer cancel()
			ops := nativePrepareOperations()
			ops.close = func(fd int) error {
				err := unix.Close(fd)
				if kind == "displaced" {
					if err := os.Rename(b.Directory(), b.Directory()+"-moved"); err != nil {
						t.Error("fixture displacement failed")
					}
				}
				return err
			}
			ops.sync = func(fd int) error {
				err := unix.Fsync(fd)
				if fd == n.secrets && kind == "cancel" {
					cancel()
				}
				return err
			}
			calls := 0
			register := func(context.Context, Envelope) (netproxy.RecoveryRegistration, error) {
				calls++
				return netproxy.RecoveryRegistration{}, nil
			}
			if result, err := n.issue(ctx, e, b, ops, register); err == nil || result.AttemptID != "" || calls != 0 {
				t.Fatal("ISSUANCE ORDER ALARM: invalidated prepare used")
			}
			if kind == "cancel" {
				entry, err := n.ReadExact(bounded(t), e.AttemptID, b)
				if err != nil || entry.envelope != e {
					t.Fatal("cancelled prepare lost recovery authority")
				}
			} else if _, err := os.Lstat(b.Directory()); !os.IsNotExist(err) {
				t.Fatal("displaced namespace repaired")
			}
		})
	}
}
