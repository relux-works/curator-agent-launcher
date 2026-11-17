//go:build unix

package client

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	schema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
)

func darwinProcess() map[string]any {
	return map[string]any{
		"pid":        4242,
		"uid":        1001,
		"boot_id":    "6f1c0e2a-8b44-4d1e-9a51-2f0c3d7e9b10",
		"start_time": map[string]any{"kind": "darwin-pbi-start", "sec": "1731715200", "usec": 123456},
	}
}
func processBody(process map[string]any) string {
	row := envelopedRow("idle")
	row["process"] = process
	return string(encoded(row))
}
func startedBody(process map[string]any) string {
	row := sessionRow()
	row["state"] = "idle"
	row["process"] = process
	return string(encoded(map[string]any{"protocol": Protocol, "ok": true, "outcome": "started", "session": row}))
}
func bindCalls(calls [][]string) [][]string {
	var out [][]string
	for _, a := range calls {
		if len(a) > 2 && a[2] == "bind" {
			out = append(out, a)
		}
	}
	return out
}
func assertBindArgv(t *testing.T, args []string, want map[string]any) {
	t.Helper()
	// argv is env lease bind --lease <id> --if-revision <n> --holder <json> --format json.
	if len(args) != 11 || args[0] != "env" || args[1] != "lease" || args[2] != "bind" || args[3] != "--lease" || args[4] != "lease-1" || args[5] != "--if-revision" || args[6] != "1" || args[7] != "--holder" || args[9] != "--format" || args[10] != "json" {
		t.Fatalf("bind argv shape: %v", args)
	}
	holderJSON := args[8]
	if schema.Validate("lease-holder.schema.json", []byte(holderJSON)) != nil {
		t.Fatalf("bind holder invalid: %s", holderJSON)
	}
	var h PreparedHolder
	if json.Unmarshal([]byte(holderJSON), &h) != nil {
		t.Fatal("bind holder undecodable")
	}
	if h.Kind != "bound" || h.Launcher != "curator-run" || h.ManagerID != "demo" || h.ManagerRegisteredAt != stamp || h.RequestID != operationID || h.Session == nil || *h.Session != "SES-example" || h.Generation == nil || *h.Generation != 1 {
		t.Fatalf("bind holder members: %+v", h)
	}
	if h.Werkstatt == nil || *h.Werkstatt != "example" {
		t.Fatalf("bind holder werkstatt: %v", h.Werkstatt)
	}
	if !processEqual(h.Process, want) {
		t.Fatalf("bind holder process differs: %s want %v", holderJSON, want)
	}
}
func assertNoSettlement(t *testing.T, tx *Transaction) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(filepath.Dir(tx.dir), "operations", "settled", operationID+".json")); !os.IsNotExist(err) {
		t.Fatalf("unexpected lease settlement: %v", err)
	}
}
func submitBind(t *testing.T, leasesBin, manager string, d Describe, bindBody json.RawMessage, bindExit int, steps ...step) (*Session, error, *Transaction, func() [][]string, func() []trace) {
	t.Helper()
	c, traces := scripted(t, manager, steps...)
	tx := transaction(t)
	base := leasePending(t, tx, Binding{ID: c.ID, Executable: c.Entry.Executable, RegisteredAt: stamp})
	record := leaseRecord(base)
	leases, calls := leaseFixture(t, leasesBin,
		leaseStep{"list", leaseReply("list", "listed", []LeaseRecord{}...), 0},
		leaseStep{"acquire", leaseReply("acquire", "acquired", record), 0},
		leaseStep{"list", leaseReply("list", "listed", record), 0},
		leaseStep{"bind", bindBody, bindExit})
	r := reconciler(c)
	r.Leases = &leases
	p := Pending{OperationID: operationID, Kind: "launch", SubmittedAt: stamp, AdmissionKey: "werkstatt:example", Manager: base.Manager, CWD: base.CWD}
	s, err := r.SubmitLeased(bounded(t), tx, p, base.Request, d)
	return s, err, tx, calls, traces
}

// Claim: HL22 direct launch binds once with the exact bound holder for each tagged variant and settles without a settlement record.
func TestBindDirectLaunchExactHolder(t *testing.T) {
	leasesBin := buildLeaseFixture(t)
	manager := buildScript(t)
	capable := delegatedDescription()
	capable.Capabilities = capableCaps()
	for _, tc := range []struct {
		name    string
		process map[string]any
	}{
		{"linux-stat-starttime", liveProcess()},
		{"darwin-pbi-start", darwinProcess()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Build the bound fixture record from the same frozen holder the submit will freeze.
			tx0 := transaction(t)
			base0 := leasePending(t, tx0, Binding{ID: "demo", Executable: "/unused", RegisteredAt: stamp})
			bound := boundRecord(base0, tc.process)
			s, err, tx, calls, traces := submitBind(t, leasesBin, manager, capable, leaseReply("bind", "bound", bound), 0, step{Operation: "launch", Body: processBody(tc.process)})
			if err != nil || s == nil || s.Session != "SES-example" || len(traces()) != 1 {
				t.Fatalf("bound launch: %v %v traces=%d", s, err, len(traces()))
			}
			assertPending(t, tx, false)
			assertNoSettlement(t, tx)
			binds := bindCalls(calls())
			if len(binds) != 1 || len(calls()) != 4 {
				t.Fatalf("lease calls: %v", calls())
			}
			assertBindArgv(t, binds[0], tc.process)
		})
	}
	t.Run("returned-holder-differs", func(t *testing.T) {
		tx0 := transaction(t)
		base0 := leasePending(t, tx0, Binding{ID: "demo", Executable: "/unused", RegisteredAt: stamp})
		other := darwinProcess()
		bound := boundRecord(base0, other)
		s, err, tx, calls, _ := submitBind(t, leasesBin, manager, capable, leaseReply("bind", "bound", bound), 0, step{Operation: "launch", Body: processBody(liveProcess())})
		leaseReason(t, err, "curator_lease_protocol_error")
		if s == nil {
			t.Fatal("started handle lost on bind mismatch")
		}
		assertPending(t, tx, true)
		if len(bindCalls(calls())) != 1 {
			t.Fatalf("bind calls: %v", calls())
		}
	})
}

// Claim: HL22 binds from process carried only on the reconciled outcome's embedded session, never by resending launch.
func TestBindReconciledOutcomeExactHolder(t *testing.T) {
	leasesBin := buildLeaseFixture(t)
	manager := buildScript(t)
	capable := delegatedDescription()
	capable.Capabilities = capableCaps()
	t.Run("reconciled-process-binds", func(t *testing.T) {
		tx0 := transaction(t)
		base0 := leasePending(t, tx0, Binding{ID: "demo", Executable: "/unused", RegisteredAt: stamp})
		bound := boundRecord(base0, liveProcess())
		s, err, tx, calls, traces := submitBind(t, leasesBin, manager, capable, leaseReply("bind", "bound", bound), 0,
			step{Operation: "launch", Body: `{}`},
			step{Operation: "status", Body: startedBody(liveProcess())})
		if err != nil || s == nil || len(traces()) != 2 || traces()[0].Operation != "launch" || traces()[1].Operation != "status" {
			t.Fatalf("reconciled bind: %v %v traces=%v", s, err, traces())
		}
		assertPending(t, tx, false)
		assertNoSettlement(t, tx)
		binds := bindCalls(calls())
		if len(binds) != 1 {
			t.Fatalf("lease calls: %v", calls())
		}
		assertBindArgv(t, binds[0], liveProcess())
	})
	t.Run("reconciled-no-process-no-bind", func(t *testing.T) {
		tx0 := transaction(t)
		base0 := leasePending(t, tx0, Binding{ID: "demo", Executable: "/unused", RegisteredAt: stamp})
		bound := boundRecord(base0, liveProcess())
		s, err, tx, calls, traces := submitBind(t, leasesBin, manager, capable, leaseReply("bind", "bound", bound), 0,
			step{Operation: "launch", Body: `{}`},
			step{Operation: "status", Body: outcome("started")})
		leaseReason(t, err, "process_incarnation_unavailable")
		if s == nil || len(traces()) != 2 || len(bindCalls(calls())) != 0 {
			t.Fatalf("no-process reconcile bound: %v calls=%v", s, calls())
		}
		p, e := tx.Pending()
		if e != nil || p == nil || p.Phase != "bind_pending" {
			t.Fatalf("journal: %v %v", p, e)
		}
	})
}
func recoverFake(t *testing.T, leasesBin string, caps []string, started *Session, acquire LeaseRecord) (*Session, error, *Transaction, func() [][]string, *[]string) {
	t.Helper()
	tx := transaction(t)
	b := Binding{ID: "demo", Executable: "/plugin", RegisteredAt: stamp}
	p := leasePending(t, tx, b)
	p.Phase = "possibly_submitted"
	rec := leaseRecord(p)
	p.Lease.Record = &rec
	p.Lease.Capabilities = caps
	// The bound fixture matches the fake started session when it carries a tuple.
	bound := boundRecord(p, liveProcess())
	leases, calls := leaseFixture(t, leasesBin,
		leaseStep{"acquire", leaseReply("acquire", "acquired", acquire), 0},
		leaseStep{"bind", leaseReply("bind", "bound", bound), 0})
	ops := &[]string{}
	r := Reconciler{Registry: map[string]sessionmanager.Entry{"demo": {Executable: "/plugin", RegisteredAt: stamp}}, Leases: &leases}
	r.Caller = func(string, sessionmanager.Entry) Caller {
		return callerFunc(func(_ context.Context, op string, _ []byte) (Response, error) {
			*ops = append(*ops, op)
			return Response{Outcome: &Outcome{Outcome: "started", Session: started}}, nil
		})
	}
	r.Now = reconciler(Client{}).Now
	putPending(t, tx, p)
	s, err := r.Recover(bounded(t), tx)
	return s, err, tx, calls, ops
}

// Claim: without an admitted tuple no bind is sent and protection stays at bind_pending, beside the admitted control that binds.
func TestBindNoAdmittedTuple(t *testing.T) {
	leasesBin := buildLeaseFixture(t)
	withTuple := func() *Session {
		s := session()
		s.State = "idle"
		s.Process = encoded(liveProcess())
		return &s
	}
	t.Run("admitted-control-binds", func(t *testing.T) {
		tx0 := transaction(t)
		p0 := leasePending(t, tx0, Binding{ID: "demo", Executable: "/plugin", RegisteredAt: stamp})
		s, err, tx, calls, ops := recoverFake(t, leasesBin, capableCaps(), withTuple(), leaseRecord(p0))
		if err != nil || s == nil || len(bindCalls(calls())) != 1 || len(*ops) != 1 {
			t.Fatalf("control: %v %v calls=%v ops=%v", s, err, calls(), *ops)
		}
		assertPending(t, tx, false)
	})
	t.Run("capable-no-process", func(t *testing.T) {
		tx0 := transaction(t)
		p0 := leasePending(t, tx0, Binding{ID: "demo", Executable: "/plugin", RegisteredAt: stamp})
		sess := session()
		sess.State = "idle"
		s, err, tx, calls, _ := recoverFake(t, leasesBin, capableCaps(), &sess, leaseRecord(p0))
		leaseReason(t, err, "process_incarnation_unavailable")
		if s == nil || len(bindCalls(calls())) != 0 {
			t.Fatalf("no-process bound: %v calls=%v", s, calls())
		}
		p, _ := tx.Pending()
		if p == nil || p.Phase != "bind_pending" {
			t.Fatalf("journal: %v", p)
		}
	})
	t.Run("fake-process-without-token", func(t *testing.T) {
		tx0 := transaction(t)
		p0 := leasePending(t, tx0, Binding{ID: "demo", Executable: "/plugin", RegisteredAt: stamp})
		s, err, tx, calls, _ := recoverFake(t, leasesBin, delegatedDescription().Capabilities, withTuple(), leaseRecord(p0))
		leaseReason(t, err, "process_incarnation_unavailable")
		if s == nil || len(bindCalls(calls())) != 0 {
			t.Fatalf("unadmitted tuple bound: %v calls=%v", s, calls())
		}
		p, _ := tx.Pending()
		if p == nil || p.Phase != "bind_pending" {
			t.Fatalf("journal: %v", p)
		}
	})
	t.Run("fake-legacy-process", func(t *testing.T) {
		tx0 := transaction(t)
		p0 := leasePending(t, tx0, Binding{ID: "demo", Executable: "/plugin", RegisteredAt: stamp})
		sess := session()
		sess.State = "idle"
		sess.Process = []byte(`{"pid":4242,"started_at":"2026-01-01T00:00:00Z"}`)
		s, err, tx, calls, _ := recoverFake(t, leasesBin, capableCaps(), &sess, leaseRecord(p0))
		leaseReason(t, err, "process_incarnation_unavailable")
		if s == nil || len(bindCalls(calls())) != 0 {
			t.Fatalf("legacy bound: %v calls=%v", s, calls())
		}
		assertPending(t, tx, true)
	})
}

// Claim: every closed-table bind refusal maps to session_home_lease_unavailable with its Curator code and reason in one call.
func TestBindRefusalsMapClosedTable(t *testing.T) {
	binary := buildLeaseFixture(t)
	bindPending := func() (Pending, PreparedHolder) {
		lane := "example"
		frozen := PreparedHolder{Kind: "prepared", Launcher: "curator-run", ManagerID: "demo", ManagerRegisteredAt: stamp, RequestID: operationID, Werkstatt: &lane}
		rec := LeaseRecord{Kind: "home-lease/1", LeaseID: "lease-1", ManagerRootID: "root-1", HomeID: "home-1", Environment: "codex_cli", Profile: "sample", LockSHA256: "1111111111111111111111111111111111111111111111111111111111111111", Revision: 1, Phase: "prepared", AcquiredAt: stamp, Holder: frozen}
		rec.Observation.State = "not_observed"
		p := Pending{Profile: "sample", OperationID: operationID, Kind: "launch", SubmittedAt: stamp, AdmissionKey: "werkstatt:example", Manager: Binding{ID: "demo", Executable: "/plugin", RegisteredAt: stamp}, Lease: &LeaseAdmission{RequestID: operationID, Route: "delegated", Environment: "codex_cli", LockSHA256: rec.LockSHA256, Holder: frozen, Record: &rec}}
		return p, boundHolder(frozen, "SES-example", 1, encoded(liveProcess()))
	}
	t.Run("valid-bound", func(t *testing.T) {
		p, bound := bindPending()
		boundRec := *p.Lease.Record
		boundRec.Phase = "active"
		boundRec.Revision = 2
		boundRec.Holder = bound
		leases, calls := leaseFixture(t, binary, leaseStep{"bind", leaseReply("bind", "bound", boundRec), 0})
		r, err := leases.Bind(bounded(t), p, bound)
		if err != nil || r == nil || r.Revision != 2 || len(calls()) != 1 {
			t.Fatalf("valid bind: %v %v calls=%v", r, err, calls())
		}
		assertBindArgv(t, calls()[0], liveProcess())
	})
	rows := []struct {
		code, reason string
		exit         int
	}{
		{"environment_home_leased", "blocking_holders", 1},
		{"environment_lease_lock_mismatch", "profile_lock_changed", 1},
		{"environment_lock_unavailable", "mutation_lock_busy", 1},
		{"environment_lease_unreadable", "state_unreadable", 1},
		{"environment_lease_unreadable", "invalid_record", 1},
		{"environment_lease_unreadable", "ledger_missing", 1},
		{"environment_lease_recovery_required", "pending_mutation", 1},
		{"environment_lease_release_unconfirmed", "session_state_unknown", 1},
		{"environment_lease_release_unconfirmed", "termination_unconfirmed", 1},
		{"environment_lease_holder_mismatch", "manager_incarnation_changed", 1},
		{"environment_lease_holder_mismatch", "session_generation_changed", 1},
		{"environment_lease_holder_mismatch", "request_conflict", 1},
		{"environment_lease_holder_mismatch", "process_incarnation_mismatch", 1},
		{"environment_lease_revision_conflict", "revision_changed", 1},
		{"environment_lease_unknown", "unknown_lease", 1},
		{"environment_lease_authority_refused", "unsupported_lease_kind", 1},
		{"environment_lease_authority_refused", "authority_unavailable", 1},
		{"environment_lease_write_failed", "publication_failed", 1},
		{"environment_home_stale", "home_not_current", 1},
		{"profile_unknown", "profile_absent", 1},
		{"environment_unknown", "environment_absent", 1},
		{"usage", "operator_confirmation_required", 2},
		{"usage", "reason_required", 2},
		{"usage", "revision_required", 2},
		{"usage", "invalid_holder", 2},
		{"usage", "invalid_selector", 2},
		{"usage", "invalid_format", 2},
		{"usage", "invalid_flags", 2},
	}
	for _, row := range rows {
		t.Run(row.reason, func(t *testing.T) {
			p, bound := bindPending()
			body := leaseRefusalFor("bind", row.code, row.reason, row.exit)
			if row.reason == "blocking_holders" {
				body = encoded(map[string]any{"schema_version": 1, "operation": "bind", "ok": false, "exit": 1, "code": row.code, "reason": row.reason, "result": nil, "details": map[string]any{"lease_id": "lease-1", "revision": 1, "holders": []LeaseRecord{*p.Lease.Record}, "count": 1}})
			}
			leases, calls := leaseFixture(t, binary, leaseStep{"bind", body, row.exit})
			_, err := leases.Bind(bounded(t), p, bound)
			leaseReason(t, err, row.reason)
			if requireCode(t, err, "session_home_lease_unavailable").Details["curator_code"] != row.code {
				t.Fatalf("curator code lost: %v want %s", err, row.code)
			}
			if len(calls()) != 1 {
				t.Fatalf("bind retried: %v", calls())
			}
		})
	}
}

// Claim: a bind refusal after start reports the session, keeps bind_pending and is attempted once, beside the valid bind that settles.
func TestBindFinishRefusalKeepsJournal(t *testing.T) {
	leasesBin := buildLeaseFixture(t)
	manager := buildScript(t)
	capable := delegatedDescription()
	capable.Capabilities = capableCaps()
	t.Run("valid-settles", func(t *testing.T) {
		tx0 := transaction(t)
		base0 := leasePending(t, tx0, Binding{ID: "demo", Executable: "/unused", RegisteredAt: stamp})
		s, err, tx, calls, traces := submitBind(t, leasesBin, manager, capable, leaseReply("bind", "bound", boundRecord(base0, liveProcess())), 0, step{Operation: "launch", Body: processBody(liveProcess())})
		if err != nil || s == nil || len(traces()) != 1 || len(bindCalls(calls())) != 1 {
			t.Fatalf("valid: %v %v calls=%v", s, err, calls())
		}
		assertPending(t, tx, false)
	})
	for _, row := range []struct {
		code, reason string
		exit         int
	}{
		{"environment_lease_holder_mismatch", "process_incarnation_mismatch", 1},
		{"environment_lease_authority_refused", "authority_unavailable", 1},
		{"usage", "invalid_holder", 2},
	} {
		t.Run(row.reason, func(t *testing.T) {
			s, err, tx, calls, traces := submitBind(t, leasesBin, manager, capable, leaseRefusalFor("bind", row.code, row.reason, row.exit), row.exit, step{Operation: "launch", Body: processBody(liveProcess())})
			leaseReason(t, err, row.reason)
			e := requireCode(t, err, "session_home_lease_unavailable")
			if e.Details["curator_code"] != row.code || e.Details["session"] != "SES-example" {
				t.Fatalf("details: %v", e.Details)
			}
			if s == nil || len(traces()) != 1 || len(bindCalls(calls())) != 1 {
				t.Fatalf("refusal retried or relaunched: %v traces=%d calls=%v", s, len(traces()), calls())
			}
			p, e2 := tx.Pending()
			if e2 != nil || p == nil || p.Phase != "bind_pending" {
				t.Fatalf("journal: %v %v", p, e2)
			}
		})
	}
}

// Claim: an unknown bind reply keeps the lease blocking and the journal and sends nothing more, beside the valid bind that settles.
func TestBindUnknownReplyKeepsJournal(t *testing.T) {
	leasesBin := buildLeaseFixture(t)
	manager := buildScript(t)
	capable := delegatedDescription()
	capable.Capabilities = capableCaps()
	t.Run("valid-settles", func(t *testing.T) {
		tx0 := transaction(t)
		base0 := leasePending(t, tx0, Binding{ID: "demo", Executable: "/unused", RegisteredAt: stamp})
		s, err, tx, _, _ := submitBind(t, leasesBin, manager, capable, leaseReply("bind", "bound", boundRecord(base0, liveProcess())), 0, step{Operation: "launch", Body: processBody(liveProcess())})
		if err != nil || s == nil {
			t.Fatalf("valid: %v %v", s, err)
		}
		assertPending(t, tx, false)
	})
	t.Run("unknown-keeps-journal", func(t *testing.T) {
		s, err, tx, calls, traces := submitBind(t, leasesBin, manager, capable, json.RawMessage(`{"bogus":true}`), 0, step{Operation: "launch", Body: processBody(liveProcess())})
		leaseReason(t, err, "curator_lease_protocol_error")
		if s == nil || len(traces()) != 1 || len(calls()) != 4 || len(bindCalls(calls())) != 1 {
			t.Fatalf("unknown sent more: %v traces=%d calls=%v", s, len(traces()), calls())
		}
		p, e := tx.Pending()
		if e != nil || p == nil || p.Phase != "bind_pending" {
			t.Fatalf("journal: %v %v", p, e)
		}
	})
}

// Claim: HL07 retry after a lost bind reply re-binds with the same acquired revision and identical payload and then settles.
func TestBindIdempotentRetrySameRevisionAndPayload(t *testing.T) {
	leasesBin := buildLeaseFixture(t)
	manager := buildScript(t)
	c, traces := scripted(t, manager,
		step{Operation: "launch", Body: processBody(liveProcess())},
		step{Operation: "status", Body: startedBody(liveProcess())})
	tx := transaction(t)
	base := leasePending(t, tx, Binding{ID: c.ID, Executable: c.Entry.Executable, RegisteredAt: stamp})
	record := leaseRecord(base)
	bound := boundRecord(base, liveProcess())
	leases, calls := leaseFixture(t, leasesBin,
		leaseStep{"list", leaseReply("list", "listed", []LeaseRecord{}...), 0},
		leaseStep{"acquire", leaseReply("acquire", "acquired", record), 0},
		leaseStep{"list", leaseReply("list", "listed", record), 0},
		leaseStep{"bind", nil, 1},
		leaseStep{"acquire", leaseReply("acquire", "acquired", record), 0},
		leaseStep{"bind", leaseReply("bind", "bound", bound), 0})
	r := reconciler(c)
	r.Leases = &leases
	capable := delegatedDescription()
	capable.Capabilities = capableCaps()
	p := Pending{OperationID: operationID, Kind: "launch", SubmittedAt: stamp, AdmissionKey: "werkstatt:example", Manager: base.Manager, CWD: base.CWD}
	s, err := r.SubmitLeased(bounded(t), tx, p, base.Request, capable)
	leaseReason(t, err, "curator_lease_protocol_error")
	if s == nil {
		t.Fatal("started handle lost after lost bind reply")
	}
	assertPending(t, tx, true)
	s, err = r.Recover(bounded(t), tx)
	if err != nil || s == nil || len(traces()) != 2 {
		t.Fatalf("retry: %v %v traces=%d", s, err, len(traces()))
	}
	assertPending(t, tx, false)
	binds := bindCalls(calls())
	if len(binds) != 2 || !reflect.DeepEqual(binds[0], binds[1]) {
		t.Fatalf("retry payload or revision drifted: %v", binds)
	}
	assertBindArgv(t, binds[1], liveProcess())
}

// Claim: a bound replay settles only on an exactly matching started outcome; every other outcome keeps the journal, writes no settlement and sends no cancel or bind.
func TestBindLostReplySettledViaAcquireReplay(t *testing.T) {
	leasesBin := buildLeaseFixture(t)
	manager := buildScript(t)
	run := func(t *testing.T, held LeaseRecord) (*Session, error, *Transaction, [][]string, []trace) {
		t.Helper()
		c, traces := scripted(t, manager, step{Operation: "status", Body: startedBody(liveProcess())})
		tx := transaction(t)
		p := leasePending(t, tx, Binding{ID: c.ID, Executable: c.Entry.Executable, RegisteredAt: stamp})
		p.Phase = "bind_pending"
		rec := leaseRecord(p)
		p.Lease.Record = &rec
		p.Lease.Capabilities = capableCaps()
		leases, calls := leaseFixture(t, leasesBin, leaseStep{"acquire", leaseReply("acquire", "acquired", held), 0})
		r := reconciler(c)
		r.Leases = &leases
		putPending(t, tx, p)
		s, err := r.Recover(bounded(t), tx)
		return s, err, tx, calls(), traces()
	}
	runStatus := func(t *testing.T, held LeaseRecord, body string) (*Session, error, *Transaction, [][]string, []trace) {
		t.Helper()
		c, traces := scripted(t, manager, step{Operation: "status", Body: body})
		tx := transaction(t)
		p := leasePending(t, tx, Binding{ID: c.ID, Executable: c.Entry.Executable, RegisteredAt: stamp})
		p.Phase = "possibly_submitted"
		rec := leaseRecord(p)
		p.Lease.Record = &rec
		p.Lease.Capabilities = capableCaps()
		leases, calls := leaseFixture(t, leasesBin, leaseStep{"acquire", leaseReply("acquire", "acquired", held), 0})
		r := reconciler(c)
		r.Leases = &leases
		putPending(t, tx, p)
		s, err := r.Recover(bounded(t), tx)
		return s, err, tx, calls(), traces()
	}
	assertBoundKept := func(t *testing.T, err error, tx *Transaction, calls [][]string, traces []trace) {
		t.Helper()
		leaseReason(t, err, "session_generation_changed")
		e := requireCode(t, err, "session_home_lease_unavailable")
		if _, ok := e.Details["curator_code"]; ok || e.Details["source"] != "launcher" {
			t.Fatalf("local marking: %v", e.Details)
		}
		if len(traces) != 1 || traces[0].Operation != "status" || len(bindCalls(calls)) != 0 || len(calls) != 1 {
			t.Fatalf("cancel or bind sent for bound lease: traces=%v calls=%v", traces, calls)
		}
		p, e2 := tx.Pending()
		if e2 != nil || p == nil || p.Phase != "bind_pending" {
			t.Fatalf("journal not kept at bind_pending: %v %v", p, e2)
		}
		assertNoSettlement(t, tx)
	}
	basePending := func(t *testing.T) Pending {
		t.Helper()
		tx := transaction(t)
		return leasePending(t, tx, Binding{ID: "demo", Executable: "/unused", RegisteredAt: stamp})
	}
	t.Run("matching-bound-settles", func(t *testing.T) {
		s, err, tx, calls, traces := run(t, boundRecord(basePending(t), liveProcess()))
		if err != nil || s == nil || len(traces) != 1 || traces[0].Operation != "status" || len(bindCalls(calls)) != 0 || len(calls) != 1 {
			t.Fatalf("settled: %v %v traces=%v calls=%v", s, err, traces, calls)
		}
		assertPending(t, tx, false)
		assertNoSettlement(t, tx)
	})
	t.Run("process-mutated", func(t *testing.T) {
		mut := liveProcess()
		mut["start_time"] = map[string]any{"kind": "linux-stat-starttime", "ticks": "123456790", "hz": 100}
		s, err, tx, calls, traces := run(t, boundRecord(basePending(t), mut))
		leaseReason(t, err, "process_incarnation_mismatch")
		e := requireCode(t, err, "session_home_lease_unavailable")
		if _, ok := e.Details["curator_code"]; ok || e.Details["source"] != "launcher" {
			t.Fatalf("local marking: %v", e.Details)
		}
		if s == nil || len(traces) != 1 || len(bindCalls(calls)) != 0 {
			t.Fatalf("changed payload sent: %v calls=%v", s, calls)
		}
		p, _ := tx.Pending()
		if p == nil || p.Phase != "bind_pending" {
			t.Fatalf("journal: %v", p)
		}
	})
	t.Run("session-differs", func(t *testing.T) {
		held := boundRecord(basePending(t), liveProcess())
		other := "SES-other"
		held.Holder.Session = &other
		s, err, tx, calls, _ := run(t, held)
		leaseReason(t, err, "session_generation_changed")
		if s == nil || len(bindCalls(calls)) != 0 {
			t.Fatalf("changed session sent: %v calls=%v", s, calls)
		}
		assertPending(t, tx, true)
	})
	t.Run("generation-differs", func(t *testing.T) {
		held := boundRecord(basePending(t), liveProcess())
		two := int64(2)
		held.Holder.Generation = &two
		s, err, tx, calls, _ := run(t, held)
		leaseReason(t, err, "session_generation_changed")
		if s == nil || len(bindCalls(calls)) != 0 {
			t.Fatalf("changed generation sent: %v calls=%v", s, calls)
		}
		assertPending(t, tx, true)
	})
	for _, tc := range []struct {
		name, body string
	}{
		{"bound-not-found-no-cancel", outcome("not_found")},
		{"bound-prepared-no-cancel", outcome("prepared")},
		{"bound-failed-no-settle", outcome("failed")},
		{"bound-cancelled-no-settle", outcome("cancelled")},
		{"bound-unknown-keeps-journal", outcome("unknown")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err, tx, calls, traces := runStatus(t, boundRecord(basePending(t), liveProcess()), tc.body)
			if s != nil {
				t.Fatalf("non-started bound outcome returned a session: %v", s)
			}
			assertBoundKept(t, err, tx, calls, traces)
		})
	}
	t.Run("bound-started-no-process-unavailable", func(t *testing.T) {
		s, err, tx, calls, traces := runStatus(t, boundRecord(basePending(t), liveProcess()), outcome("started"))
		leaseReason(t, err, "process_incarnation_unavailable")
		if requireCode(t, err, "session_home_lease_unavailable").Details["session"] != "SES-example" {
			t.Fatalf("session detail: %v", err)
		}
		if s == nil || len(traces) != 1 || len(bindCalls(calls)) != 0 {
			t.Fatalf("no-process bound started sent: %v traces=%v calls=%v", s, traces, calls)
		}
		p, e := tx.Pending()
		if e != nil || p == nil || p.Phase != "bind_pending" {
			t.Fatalf("journal: %v %v", p, e)
		}
		assertNoSettlement(t, tx)
	})
	t.Run("bound-unknown-phase-protocol-error", func(t *testing.T) {
		held := boundRecord(basePending(t), liveProcess())
		held.Phase = "unknown"
		s, err, tx, calls, traces := runStatus(t, held, startedBody(liveProcess()))
		leaseReason(t, err, "curator_lease_protocol_error")
		if s != nil || len(traces) != 0 || len(bindCalls(calls)) != 0 {
			t.Fatalf("unknown-phase bound replay reached the manager: %v traces=%v calls=%v", s, traces, calls)
		}
		assertPending(t, tx, true)
		assertNoSettlement(t, tx)
	})
}

// Claim: no release or break command is sent on bind success, refusal, unknown or settled paths.
func TestBindNeverReleases(t *testing.T) {
	leasesBin := buildLeaseFixture(t)
	manager := buildScript(t)
	capable := delegatedDescription()
	capable.Capabilities = capableCaps()
	check := func(t *testing.T, calls [][]string) {
		t.Helper()
		for _, a := range calls {
			if len(a) > 2 && (a[2] == "release" || a[2] == "break") {
				t.Fatalf("release sent: %v", calls)
			}
		}
	}
	t.Run("success-no-release", func(t *testing.T) {
		tx0 := transaction(t)
		base0 := leasePending(t, tx0, Binding{ID: "demo", Executable: "/unused", RegisteredAt: stamp})
		s, err, _, calls, _ := submitBind(t, leasesBin, manager, capable, leaseReply("bind", "bound", boundRecord(base0, liveProcess())), 0, step{Operation: "launch", Body: processBody(liveProcess())})
		if err != nil || s == nil || len(bindCalls(calls())) != 1 {
			t.Fatalf("success control: %v %v", s, err)
		}
		check(t, calls())
	})
	t.Run("refusal-no-release", func(t *testing.T) {
		_, err, _, calls, _ := submitBind(t, leasesBin, manager, capable, leaseRefusalFor("bind", "environment_lease_holder_mismatch", "process_incarnation_mismatch", 1), 1, step{Operation: "launch", Body: processBody(liveProcess())})
		leaseReason(t, err, "process_incarnation_mismatch")
		check(t, calls())
	})
}

// Claim: a prepared replay at the journaled revision re-binds, and one at any other revision refuses locally with no bind and no manager call.
func TestBindPreparedReplayRevisionGuard(t *testing.T) {
	leasesBin := buildLeaseFixture(t)
	withTuple := func() *Session {
		s := session()
		s.State = "idle"
		s.Process = encoded(liveProcess())
		return &s
	}
	t.Run("journaled-revision-rebinds", func(t *testing.T) {
		tx0 := transaction(t)
		p0 := leasePending(t, tx0, Binding{ID: "demo", Executable: "/plugin", RegisteredAt: stamp})
		s, err, tx, calls, ops := recoverFake(t, leasesBin, capableCaps(), withTuple(), leaseRecord(p0))
		if err != nil || s == nil || len(bindCalls(calls())) != 1 || len(*ops) != 1 {
			t.Fatalf("control: %v %v calls=%v ops=%v", s, err, calls(), *ops)
		}
		assertPending(t, tx, false)
	})
	t.Run("prepared-replay-revision-differs", func(t *testing.T) {
		tx0 := transaction(t)
		p0 := leasePending(t, tx0, Binding{ID: "demo", Executable: "/plugin", RegisteredAt: stamp})
		acquire := leaseRecord(p0)
		acquire.Revision = 3
		s, err, tx, calls, ops := recoverFake(t, leasesBin, capableCaps(), withTuple(), acquire)
		leaseReason(t, err, "revision_changed")
		e := requireCode(t, err, "session_home_lease_unavailable")
		if _, ok := e.Details["curator_code"]; ok || e.Details["source"] != "launcher" {
			t.Fatalf("local marking: %v", e.Details)
		}
		if s != nil || len(*ops) != 0 || len(bindCalls(calls())) != 0 || len(calls()) != 1 {
			t.Fatalf("revision-drifted replay sent: %v ops=%v calls=%v", s, *ops, calls())
		}
		assertPending(t, tx, true)
		assertNoSettlement(t, tx)
	})
}

// Claim: invalid_holder is sent exactly once per invocation, and a later invocation re-derives and may send the identical bind once more.
func TestBindInvalidHolderOneBindPerInvocation(t *testing.T) {
	leasesBin := buildLeaseFixture(t)
	manager := buildScript(t)
	capable := delegatedDescription()
	capable.Capabilities = capableCaps()
	t.Run("valid-settles", func(t *testing.T) {
		tx0 := transaction(t)
		base0 := leasePending(t, tx0, Binding{ID: "demo", Executable: "/unused", RegisteredAt: stamp})
		s, err, tx, calls, _ := submitBind(t, leasesBin, manager, capable, leaseReply("bind", "bound", boundRecord(base0, liveProcess())), 0, step{Operation: "launch", Body: processBody(liveProcess())})
		if err != nil || s == nil || len(bindCalls(calls())) != 1 {
			t.Fatalf("valid: %v %v calls=%v", s, err, calls())
		}
		assertPending(t, tx, false)
	})
	t.Run("invalid-holder-once-per-invocation", func(t *testing.T) {
		c, traces := scripted(t, manager,
			step{Operation: "launch", Body: processBody(liveProcess())},
			step{Operation: "status", Body: startedBody(liveProcess())})
		tx := transaction(t)
		base := leasePending(t, tx, Binding{ID: c.ID, Executable: c.Entry.Executable, RegisteredAt: stamp})
		record := leaseRecord(base)
		refusal := leaseRefusalFor("bind", "usage", "invalid_holder", 2)
		leases, calls := leaseFixture(t, leasesBin,
			leaseStep{"list", leaseReply("list", "listed", []LeaseRecord{}...), 0},
			leaseStep{"acquire", leaseReply("acquire", "acquired", record), 0},
			leaseStep{"list", leaseReply("list", "listed", record), 0},
			leaseStep{"bind", refusal, 2},
			leaseStep{"acquire", leaseReply("acquire", "acquired", record), 0},
			leaseStep{"bind", refusal, 2})
		r := reconciler(c)
		r.Leases = &leases
		p := Pending{OperationID: operationID, Kind: "launch", SubmittedAt: stamp, AdmissionKey: "werkstatt:example", Manager: base.Manager, CWD: base.CWD}
		s, err := r.SubmitLeased(bounded(t), tx, p, base.Request, capable)
		leaseReason(t, err, "invalid_holder")
		if s == nil || len(bindCalls(calls())) != 1 {
			t.Fatalf("first invocation bind count: %v calls=%v", s, calls())
		}
		s, err = r.Recover(bounded(t), tx)
		leaseReason(t, err, "invalid_holder")
		binds := bindCalls(calls())
		if s == nil || len(traces()) != 2 || len(binds) != 2 || !reflect.DeepEqual(binds[0], binds[1]) {
			t.Fatalf("second invocation bind count or payload: %v traces=%d binds=%v", s, len(traces()), binds)
		}
		pending, e := tx.Pending()
		if e != nil || pending == nil || pending.Phase != "bind_pending" {
			t.Fatalf("journal: %v %v", pending, e)
		}
	})
}
