//go:build unix

package client

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
)

func transaction(t *testing.T) *Transaction {
	t.Helper()
	tx, err := (Store{Directory: t.TempDir()}).Lock(bounded(t), "example")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Close() })
	return tx
}
func pending(c Client) Pending {
	return Pending{OperationID: operationID, Kind: "launch", SubmittedAt: stamp, AdmissionKey: "werkstatt:example", Manager: Binding{ID: c.ID, Executable: c.Entry.Executable, RegisteredAt: stamp}}
}
func reconciler(c Client) Reconciler {
	return Reconciler{Registry: map[string]sessionmanager.Entry{c.ID: c.Entry}, Caller: func(string, sessionmanager.Entry) Caller { return c }, Now: func() time.Time { v, _ := time.Parse(time.RFC3339, stamp); return v }}
}
func putPending(t *testing.T, tx *Transaction, p Pending) {
	t.Helper()
	if err := tx.WritePending(p); err != nil {
		t.Fatal(err)
	}
}
func assertPending(t *testing.T, tx *Transaction, want bool) {
	t.Helper()
	p, err := tx.Pending()
	if err != nil || (p != nil) != want {
		t.Fatalf("pending %v, want %t: %v", p, want, err)
	}
}

// Recovery consumes cancel's actual answer, including a start racing the fence.
// Uncertain/error answers preserve the exact journal; only terminal outcomes
// clear it. A trace verifies no later status hides the cancel result.
func TestScriptedRecovery(t *testing.T) {
	binary := buildScript(t)
	cases := []struct {
		name          string
		steps         []step
		code          string
		keep, started bool
	}{
		{"already-started", []step{{Operation: "status", Body: outcome("started")}}, "", false, true},
		{"historical-failure", []step{{Operation: "status", Body: outcome("failed")}}, "policy_refused", false, false},
		{"cancelled", []step{{Operation: "status", Body: outcome("cancelled")}}, "session_host_unavailable", false, false},
		{"not-found-fenced", []step{{Operation: "status", Body: outcome("not_found")}, {Operation: "cancel", Body: outcome("cancelled")}}, "session_host_unavailable", false, false},
		{"prepared-race-started", []step{{Operation: "status", Body: outcome("prepared")}, {Operation: "cancel", Body: outcome("started")}}, "", false, true},
		{"cancel-race-failed", []step{{Operation: "status", Body: outcome("not_found")}, {Operation: "cancel", Body: outcome("failed")}}, "policy_refused", false, false},
		{"admitted-poll", []step{{Operation: "status", Body: outcome("admitted")}, {Operation: "status", Body: outcome("started")}}, "", false, true},
		{"cancel-admitted-poll", []step{{Operation: "status", Body: outcome("prepared")}, {Operation: "cancel", Body: outcome("admitted")}, {Operation: "status", Body: outcome("started")}}, "", false, true},
		{"unknown", []step{{Operation: "status", Body: outcome("unknown")}}, "session_liveness_unknown", true, false},
		{"status-poison", []step{{Operation: "status", Body: `{"protocol":"curator-session-manager/1","ok":true,"outcome":"unknown","extra":"sentinel"}`}}, "session_liveness_unknown", true, false},
		{"cancel-poison", []step{{Operation: "status", Body: outcome("not_found")}, {Operation: "cancel", Body: `{}`}}, "session_liveness_unknown", true, false},
		{"cancel-refusal", []step{{Operation: "status", Body: outcome("prepared")}, {Operation: "cancel", Body: `{"protocol":"curator-session-manager/1","ok":false,"code":"session_host_unavailable","details":{"reason":"unreachable"}}`, Exit: 1}}, "session_liveness_unknown", true, false},
		{"foreign-start", []step{{Operation: "status", Body: string(encoded(map[string]any{"protocol": Protocol, "ok": true, "outcome": "started", "session": func() map[string]any { s := sessionRow(); s["operation_id"] = secondID; return s }()}))}}, "session_liveness_unknown", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, traces := scripted(t, binary, tc.steps...)
			tx := transaction(t)
			p := pending(c)
			putPending(t, tx, p)
			before := readFile(t, filepath.Join(tx.dir, "example.op.json"))
			s, err := reconciler(c).Recover(bounded(t), tx)
			if tc.code != "" {
				requireCode(t, err, tc.code)
			} else if err != nil {
				t.Fatal(err)
			}
			if (s != nil) != tc.started {
				t.Fatal("start result lost")
			}
			assertPending(t, tx, tc.keep)
			if tc.keep && !reflect.DeepEqual(before, readFile(t, filepath.Join(tx.dir, "example.op.json"))) {
				t.Fatal("uncertain journal changed")
			}
			live, e := tx.Liveness()
			if e != nil || (live != nil) != tc.started {
				t.Fatal("unconfirmed liveness", live, e)
			}
			if live != nil && (live.OperationID == nil || *live.OperationID != operationID || live.State != "unknown" || live.Manager != p.Manager) {
				t.Fatal("liveness inference or binding drift")
			}
			tr := traces()
			if len(tr) != len(tc.steps) {
				t.Fatalf("unexpected follow-up calls: %v", tr)
			}
			for i, v := range tr {
				if v.Operation != tc.steps[i].Operation || !reflect.DeepEqual(v.Request, statusRequest()) {
					t.Fatal("recovery call mismatch")
				}
			}
		})
	}
	t.Run("admitted-deadline-keeps-journal", func(t *testing.T) {
		c, traces := scripted(t, binary, step{Operation: "status", Body: outcome("admitted")})
		tx := transaction(t)
		putPending(t, tx, pending(c))
		ctx, cancel := context.WithTimeout(bounded(t), time.Second)
		defer cancel()
		_, err := reconciler(c).Recover(ctx, tx)
		requireCode(t, err, "session_liveness_unknown")
		assertPending(t, tx, true)
		if len(traces()) != 1 {
			t.Fatal("admitted loop ignored context")
		}
	})
}

type callerFunc func(context.Context, string, []byte) (Response, error)

func (f callerFunc) Call(ctx context.Context, op string, b []byte) (Response, error) {
	return f(ctx, op, b)
}

// The journal is durable and bound before submission; a later invocation recovers
// it without sending a second launch. Missing/changed owners are never rerouted.
// A fractional clock still writes whole-second UTC liveness timestamps.
func TestScriptedSubmissionAndBinding(t *testing.T) {
	binary := buildScript(t)
	for _, mode := range []string{"success", "refusal", "lost-answer", "fractional-clock"} {
		t.Run(mode, func(t *testing.T) {
			steps := []step{{Operation: "launch", Body: string(encoded(session()))}}
			if mode == "refusal" {
				steps[0].Body = `{"protocol":"curator-session-manager/1","ok":false,"code":"session_operation_conflict","details":{}}`
				steps[0].Exit = 1
			}
			if mode == "lost-answer" {
				steps[0].Body = `{}`
				steps = append(steps, step{Operation: "status", Body: outcome("started")})
			}
			c, traces := scripted(t, binary, steps...)
			tx := transaction(t)
			p := pending(c)
			r := reconciler(c)
			if mode == "fractional-clock" {
				base := r.now()
				r.Now = func() time.Time { return base.Add(123 * time.Millisecond) }
			}
			calls := 0
			r.Caller = func(string, sessionmanager.Entry) Caller {
				return callerFunc(func(ctx context.Context, op string, b []byte) (Response, error) {
					calls++
					if calls == 1 {
						stored, err := tx.Pending()
						if err != nil || stored == nil || *stored != p {
							t.Fatal("remote submission preceded durable correlation")
						}
						info, err := os.Stat(filepath.Join(tx.dir, "example.op.json"))
						if err != nil || info.Mode().Perm() != 0600 {
							t.Fatal("journal not private")
						}
					}
					return c.Call(ctx, op, b)
				})
			}
			var request map[string]any
			_ = json.Unmarshal(document(t, "launch"), &request)
			request["admission_key"] = "werkstatt:example"
			s, err := r.Submit(bounded(t), tx, p, encoded(request))
			if mode == "fractional-clock" {
				if err != nil {
					t.Fatal("fractional clock prevented liveness write", err)
				}
				live, readErr := tx.Liveness()
				if readErr != nil || live == nil || live.LastHeartbeatAt != stamp {
					t.Fatal("whole-second heartbeat lost", live, readErr)
				}
			}
			if mode == "refusal" {
				requireCode(t, err, "session_operation_conflict")
				if s != nil {
					t.Fatal("failed start returned session")
				}
			} else if err != nil || s == nil {
				t.Fatal(s, err)
			}
			assertPending(t, tx, false)
			if len(traces()) != len(steps) {
				t.Fatal("submission sequence")
			}
		})
	}
	t.Run("crash-before-send", func(t *testing.T) {
		c, traces := scripted(t, binary, step{Operation: "status", Body: outcome("prepared")}, step{Operation: "cancel", Body: outcome("cancelled")})
		tx := transaction(t)
		p := pending(c)
		putPending(t, tx, p)
		next := p
		next.OperationID = secondID
		_, err := reconciler(c).Submit(bounded(t), tx, next, document(t, "launch"))
		requireCode(t, err, "session_host_unavailable")
		assertPending(t, tx, false)
		tr := traces()
		if len(tr) != 2 || tr[0].Operation != "status" || tr[1].Operation != "cancel" {
			t.Fatal("overlapping request submitted")
		}
	})
	for _, mode := range []string{"missing", "changed", "registration-time-only"} {
		t.Run("owner-"+mode, func(t *testing.T) {
			c, traces := scripted(t, binary, step{Operation: "status", Body: outcome("started")})
			tx := transaction(t)
			putPending(t, tx, pending(c))
			r := reconciler(c)
			switch mode {
			case "missing":
				delete(r.Registry, c.ID)
			case "changed":
				entry := c.Entry
				entry.Executable = filepath.Join(t.TempDir(), "replacement")
				r.Registry[c.ID] = entry
			case "registration-time-only":
				entry := c.Entry
				entry.RegisteredAt = "2026-01-02T00:00:00Z"
				r.Registry[c.ID] = entry
			}
			s, err := r.Recover(bounded(t), tx)
			if mode == "registration-time-only" {
				if err != nil || s == nil {
					t.Fatal(s, err)
				}
				assertPending(t, tx, false)
			} else {
				e := requireCode(t, err, "werkstatt_operation_unresolved")
				if e.Details["reason"] != "manager_"+mode || len(traces()) != 0 {
					t.Fatal("owner binding did not fail closed")
				}
				assertPending(t, tx, true)
			}
		})
	}
}

// Plain recovery has no local journal but still consumes cancel and reports the
// operation id on uncertainty; historical codes retain their launcher exits.
func TestPlainRecovery(t *testing.T) {
	binary := buildScript(t)
	for _, state := range []string{"started", "failed", "cancelled", "unknown"} {
		t.Run(state, func(t *testing.T) {
			c, traces := scripted(t, binary, step{Operation: "status", Body: outcome("not_found")}, step{Operation: "cancel", Body: outcome(state)})
			s, err := ReconcilePlain(bounded(t), c, operationID)
			switch state {
			case "started":
				if err != nil || s == nil {
					t.Fatal(s, err)
				}
			case "failed":
				if requireCode(t, err, "policy_refused").ExitCode() != 16 {
					t.Fatal("historical refusal misclassified")
				}
			case "cancelled":
				requireCode(t, err, "session_host_unavailable")
			case "unknown":
				if requireCode(t, err, "session_liveness_unknown").Details["operation"] != operationID {
					t.Fatal("unknown operation lost")
				}
			}
			if len(traces()) != 2 {
				t.Fatal("plain cancel response ignored")
			}
		})
	}
}

// Plain Start runs reconciliation once after a poisoned/lost answer and does
// not repeat submission; a formal start refusal needs no recovery calls.
func TestPlainStart(t *testing.T) {
	binary := buildScript(t)
	for _, mode := range []string{"success", "lost", "refused"} {
		t.Run(mode, func(t *testing.T) {
			steps := []step{{Operation: "launch", Body: string(encoded(session()))}}
			if mode == "lost" {
				steps[0].Body = `{}`
				steps = append(steps, step{Operation: "status", Body: outcome("not_found")}, step{Operation: "cancel", Body: outcome("started")})
			}
			if mode == "refused" {
				steps[0].Body = `{"protocol":"curator-session-manager/1","ok":false,"code":"session_operation_expired","details":{}}`
				steps[0].Exit = 1
			}
			c, traces := scripted(t, binary, steps...)
			s, err := c.Start(bounded(t), "launch", document(t, "launch"))
			if mode == "refused" {
				requireCode(t, err, "session_operation_expired")
			} else if err != nil || s == nil {
				t.Fatal(s, err)
			}
			if len(traces()) != len(steps) {
				t.Fatal("duplicate submission/reconciliation")
			}
		})
	}
}
