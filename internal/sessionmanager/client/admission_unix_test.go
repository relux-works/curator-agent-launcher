//go:build unix

package client

import (
	"slices"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
)

func processEnvelopeBody() string {
	row := envelopedRow("idle")
	row["process"] = liveProcess()
	return string(encoded(row))
}

func terminationEnvelopeBody() string {
	row := envelopedRow("down")
	row["termination"] = map[string]any{"scope": "linux-cgroup", "witness": "populated-0"}
	return string(encoded(row))
}

func startedOutcomeProcessBody() string {
	row := sessionRow()
	row["state"] = "idle"
	row["process"] = liveProcess()
	return string(encoded(map[string]any{"protocol": Protocol, "ok": true, "outcome": "started", "session": row}))
}

func capableCaps() []string {
	return append(slices.Clone(delegatedDescription().Capabilities), ProcessObservationCapability)
}

// Claim: source (b) admission comes only from the capabilities frozen in the
// lease admission, only for the exact frozen owner. The exact binding with
// status/1 and the token admits; beside it a token without status/1, frozen
// capabilities without the token (an old durable file), no lease record, a
// re-registered entry and a changed executable each give the no-token rule,
// so the same process body then refuses session_host_protocol_error.
func TestFrozenAdmission(t *testing.T) {
	binding := Binding{ID: "demo", Executable: "/manager", RegisteredAt: stamp}
	entry := sessionmanager.Entry{Executable: "/manager", RegisteredAt: stamp}
	leased := func(caps []string) Pending {
		return Pending{Manager: binding, Lease: &LeaseAdmission{Capabilities: caps}}
	}
	exact := frozenAdmission(leased(capableCaps()), entry)
	if !exact.ProcessObservation {
		t.Fatal("exact frozen owner with the token was not admitted")
	}
	statusReq := []byte(`{"session":"SES-example"}`)
	if _, err := DecodeAdmitted("status", statusReq, []byte(processEnvelopeBody()), 0, "demo", exact); err != nil {
		t.Fatalf("valid control refused under frozen admission: %v", err)
	}
	tokenOnly := slices.DeleteFunc(capableCaps(), func(s string) bool { return s == "status/1" })
	reRegistered := entry
	reRegistered.RegisteredAt = "2026-01-02T00:00:00Z"
	changedExe := entry
	changedExe.Executable = "/replacement"
	for name, admission := range map[string]Admission{
		"token-without-status": frozenAdmission(leased(tokenOnly), entry),
		"old-file-no-token":    frozenAdmission(leased(delegatedDescription().Capabilities), entry),
		"empty-capabilities":   frozenAdmission(leased(nil), entry),
		"no-lease-record":      frozenAdmission(Pending{Manager: binding}, entry),
		"re-registered":        frozenAdmission(leased(capableCaps()), reRegistered),
		"changed-executable":   frozenAdmission(leased(capableCaps()), changedExe),
		"other-manager-entry":  frozenAdmission(leased(capableCaps()), sessionmanager.Entry{Executable: "/other", RegisteredAt: "2026-01-03T00:00:00Z"}),
	} {
		if admission.ProcessObservation {
			t.Fatalf("%s: admission granted", name)
		}
		_, err := DecodeAdmitted("status", statusReq, []byte(processEnvelopeBody()), 0, "demo", admission)
		requireCode(t, err, "session_host_protocol_error")
	}
	// WithAdmission copies: admitting one client never admits another.
	base := Client{ID: "demo"}
	if admitted := base.WithAdmission(exact); !admitted.Admission.ProcessObservation || base.Admission.ProcessObservation {
		t.Fatal("WithAdmission mutated its receiver or lost the admission")
	}
}

// Claim (the L1 witness on the plain path): a client admitted from its
// manager's describe accepts process on the launch and resume replies and on
// the started outcome its reconciliation reads, so the start reports the
// session the manager started. Beside it the same script under the zero
// admission is refused on the launch decode and again on the reconciliation
// decode, ending session_liveness_unknown, which is the pre-A3 behaviour.
func TestStartAdmitsProcessObservation(t *testing.T) {
	binary := buildScript(t)
	token := Admission{ProcessObservation: true}
	for _, op := range []string{"launch", "resume"} {
		t.Run(op+"-direct-process", func(t *testing.T) {
			c, traces := scripted(t, binary, step{Operation: op, Body: processEnvelopeBody()})
			s, err := c.WithAdmission(token).Start(bounded(t), op, document(t, op))
			if err != nil || s == nil || s.Session != "SES-example" || len(traces()) != 1 {
				t.Fatalf("admitted %s: %v %v traces=%d", op, s, err, len(traces()))
			}
		})
	}
	t.Run("launch-reconciled-outcome-process", func(t *testing.T) {
		c, traces := scripted(t, binary,
			step{Operation: "launch", Body: `{}`},
			step{Operation: "status", Body: startedOutcomeProcessBody()})
		s, err := c.WithAdmission(token).Start(bounded(t), "launch", document(t, "launch"))
		if err != nil || s == nil || len(traces()) != 2 {
			t.Fatalf("reconciled launch: %v %v traces=%d", s, err, len(traces()))
		}
	})
	t.Run("launch-process-both-no-token", func(t *testing.T) {
		c, traces := scripted(t, binary,
			step{Operation: "launch", Body: processEnvelopeBody()},
			step{Operation: "status", Body: startedOutcomeProcessBody()})
		s, err := c.Start(bounded(t), "launch", document(t, "launch"))
		requireCode(t, err, "session_liveness_unknown")
		if s != nil || len(traces()) != 2 {
			t.Fatalf("refused launch reported a session: %v traces=%d", s, len(traces()))
		}
	})
}

// Claim: no_start is admitted only under the token, on the cancel fence that
// reconciliation consumes. An admitted failed/cancelled outcome keeps its
// historical result; beside it the same body without admission is refused
// while decoding, so reconciliation can only report the operation unknown.
// The legacy failed outcome without no_start keeps its result either way.
func TestReconcilePlainNoStart(t *testing.T) {
	binary := buildScript(t)
	token := Admission{ProcessObservation: true}
	noStart := func(outcome string) string {
		m := map[string]any{"protocol": Protocol, "ok": true, "outcome": outcome, "no_start": "proved"}
		if outcome == "failed" {
			m["code"] = "policy_refused"
		}
		return string(encoded(m))
	}
	run := func(t *testing.T, admission Admission, cancelBody string) error {
		t.Helper()
		c, traces := scripted(t, binary,
			step{Operation: "status", Body: outcome("not_found")},
			step{Operation: "cancel", Body: cancelBody})
		_, err := ReconcilePlain(bounded(t), c.WithAdmission(admission), operationID)
		if len(traces()) != 2 {
			t.Fatalf("cancel answer not consumed: traces=%d", len(traces()))
		}
		return err
	}
	t.Run("admitted-failed-no-start", func(t *testing.T) {
		requireCode(t, run(t, token, noStart("failed")), "policy_refused")
	})
	t.Run("no-token-failed-no-start", func(t *testing.T) {
		requireCode(t, run(t, Admission{}, noStart("failed")), "session_liveness_unknown")
	})
	t.Run("admitted-cancelled-no-start", func(t *testing.T) {
		requireCode(t, run(t, token, noStart("cancelled")), "session_host_unavailable")
	})
	t.Run("no-token-cancelled-no-start", func(t *testing.T) {
		requireCode(t, run(t, Admission{}, noStart("cancelled")), "session_liveness_unknown")
	})
	t.Run("legacy-failed-control", func(t *testing.T) {
		requireCode(t, run(t, Admission{}, outcome("failed")), "policy_refused")
	})
}

// Claim: list pages and stop/status replies over the transport follow the
// same rule. Under admission, process and termination rows and replies
// decode; beside them the identical bodies under the zero admission refuse
// session_host_protocol_error, which is also the pinned behaviour of the
// launcher paths that still run without any admission source.
func TestTransportListStopStatusAdmission(t *testing.T) {
	binary := buildScript(t)
	token := Admission{ProcessObservation: true}
	listBody := func() string {
		live := sessionRow()
		live["state"] = "idle"
		live["process"] = liveProcess()
		down := sessionRow()
		down["session"] = "SES-other"
		down["state"] = "down"
		down["termination"] = map[string]any{"scope": "dedicated-uid", "witness": "zero-process-proof"}
		return string(encoded(map[string]any{"protocol": Protocol, "ok": true, "sessions": []map[string]any{live, down}, "next_cursor": nil}))
	}()
	t.Run("list-admitted", func(t *testing.T) {
		c, _ := scripted(t, binary, step{Operation: "list", Body: listBody})
		sessions, err := c.WithAdmission(token).ListAll(bounded(t), nil, 0)
		if err != nil || len(sessions) != 2 {
			t.Fatalf("admitted list: %v %v", sessions, err)
		}
	})
	t.Run("list-no-token", func(t *testing.T) {
		c, _ := scripted(t, binary, step{Operation: "list", Body: listBody})
		_, err := c.ListAll(bounded(t), nil, 0)
		requireCode(t, err, "session_host_protocol_error")
	})
	t.Run("stop-termination-admitted", func(t *testing.T) {
		c, _ := scripted(t, binary, step{Operation: "stop", Body: terminationEnvelopeBody()})
		r, err := c.WithAdmission(token).Call(bounded(t), "stop", document(t, "stop-request"))
		if err != nil || r.Session == nil || r.Session.State != "down" {
			t.Fatalf("admitted stop: %v %v", r.Session, err)
		}
	})
	t.Run("stop-termination-no-token", func(t *testing.T) {
		c, _ := scripted(t, binary, step{Operation: "stop", Body: terminationEnvelopeBody()})
		_, err := c.Call(bounded(t), "stop", document(t, "stop-request"))
		requireCode(t, err, "session_host_protocol_error")
	})
	t.Run("status-process-admitted", func(t *testing.T) {
		c, _ := scripted(t, binary, step{Operation: "status", Body: processEnvelopeBody()})
		r, err := c.WithAdmission(token).Call(bounded(t), "status", []byte(`{"session":"SES-example"}`))
		if err != nil || r.Session == nil {
			t.Fatalf("admitted status: %v %v", r.Session, err)
		}
	})
	t.Run("status-process-no-token", func(t *testing.T) {
		c, _ := scripted(t, binary, step{Operation: "status", Body: processEnvelopeBody()})
		_, err := c.Call(bounded(t), "status", []byte(`{"session":"SES-example"}`))
		requireCode(t, err, "session_host_protocol_error")
	})
}

// Claim (the L1 regression witness): a leased launch whose describe carries
// the token admits process on the launch reply and, after a lost answer, on
// the started outcome read back through operation status. The admitted
// tuple binds and settles the journal.
// Beside it the same script under a describe without the token stays refused
// on both decodes and the operation is reported unknown, journal retained.
func TestLeasedLaunchAdmitsProcessObservation(t *testing.T) {
	leasesBin := buildLeaseFixture(t)
	manager := buildScript(t)
	capable := delegatedDescription()
	capable.Capabilities = capableCaps()
	submit := func(t *testing.T, d Describe, steps ...step) (*Session, error, *Transaction, func() []trace) {
		t.Helper()
		c, traces := scripted(t, manager, steps...)
		tx := transaction(t)
		base := leasePending(t, tx, Binding{ID: c.ID, Executable: c.Entry.Executable, RegisteredAt: stamp})
		record := leaseRecord(base)
		leases, _ := leaseFixture(t, leasesBin,
			leaseStep{"list", leaseReply("list", "listed", []LeaseRecord{}...), 0},
			leaseStep{"acquire", leaseReply("acquire", "acquired", record), 0},
			leaseStep{"list", leaseReply("list", "listed", record), 0},
			leaseStep{"bind", leaseReply("bind", "bound", boundRecord(base, liveProcess())), 0})
		r := reconciler(c)
		r.Leases = &leases
		p := Pending{OperationID: operationID, Kind: "launch", SubmittedAt: stamp, AdmissionKey: "werkstatt:example", Manager: base.Manager, CWD: base.CWD}
		s, err := r.SubmitLeased(bounded(t), tx, p, base.Request, d)
		return s, err, tx, traces
	}
	t.Run("direct-launch-process", func(t *testing.T) {
		s, err, tx, traces := submit(t, capable, step{Operation: "launch", Body: processEnvelopeBody()})
		if err != nil || s == nil || s.Session != "SES-example" || len(traces()) != 1 {
			t.Fatalf("bound session lost: %v %v traces=%d", s, err, len(traces()))
		}
		assertPending(t, tx, false)
	})
	t.Run("reconciled-outcome-process", func(t *testing.T) {
		s, err, tx, traces := submit(t, capable,
			step{Operation: "launch", Body: `{}`},
			step{Operation: "status", Body: startedOutcomeProcessBody()})
		if err != nil || s == nil || len(traces()) != 2 {
			t.Fatalf("reconciled bound session lost: %v %v traces=%d", s, err, len(traces()))
		}
		assertPending(t, tx, false)
	})
	t.Run("no-token-process-refused", func(t *testing.T) {
		s, err, tx, traces := submit(t, delegatedDescription(),
			step{Operation: "launch", Body: processEnvelopeBody()},
			step{Operation: "status", Body: startedOutcomeProcessBody()})
		requireCode(t, err, "session_liveness_unknown")
		if s != nil || len(traces()) != 2 {
			t.Fatalf("refused launch reported a session: %v traces=%d", s, len(traces()))
		}
		assertPending(t, tx, true)
	})
	t.Run("legacy-launch-control", func(t *testing.T) {
		s, err, _, traces := submit(t, delegatedDescription(), step{Operation: "launch", Body: string(encoded(session()))})
		leaseReason(t, err, "process_incarnation_unavailable")
		if s == nil || len(traces()) != 1 {
			t.Fatalf("legacy launch broke: %v traces=%d", s, len(traces()))
		}
	})
}

// Claim: recovery of an already submitted leased operation admits process
// from the capabilities frozen at submission (source (b)), with no describe
// in the recovery run. Beside the exact-match positive, frozen capabilities
// without the token or without status/1 keep the operation unknown with its
// journal retained, and a re-registered or missing manager is never called.
func TestFrozenRecoveryAdmitsProcessObservation(t *testing.T) {
	leasesBin := buildLeaseFixture(t)
	manager := buildScript(t)
	recoverWith := func(t *testing.T, caps []string, mutate func(*Reconciler, Client), steps ...step) (*Session, error, *Transaction, func() []trace) {
		t.Helper()
		c, traces := scripted(t, manager, steps...)
		tx := transaction(t)
		p := leasePending(t, tx, Binding{ID: c.ID, Executable: c.Entry.Executable, RegisteredAt: stamp})
		p.Phase = "possibly_submitted"
		record := leaseRecord(p)
		p.Lease.Record = &record
		p.Lease.Capabilities = caps
		leases, _ := leaseFixture(t, leasesBin, leaseStep{"acquire", leaseReply("acquire", "acquired", record), 0}, leaseStep{"bind", leaseReply("bind", "bound", boundRecord(p, liveProcess())), 0})
		r := reconciler(c)
		r.Leases = &leases
		if mutate != nil {
			mutate(&r, c)
		}
		putPending(t, tx, p)
		s, err := r.Recover(bounded(t), tx)
		return s, err, tx, traces
	}
	status := step{Operation: "status", Body: startedOutcomeProcessBody()}
	t.Run("exact-match-process", func(t *testing.T) {
		s, err, tx, traces := recoverWith(t, capableCaps(), nil, status)
		if err != nil || s == nil || len(traces()) != 1 || traces()[0].Operation != "status" {
			t.Fatalf("frozen recovery lost the bound session: %v %v traces=%v", s, err, traces())
		}
		assertPending(t, tx, false)
	})
	t.Run("frozen-without-token", func(t *testing.T) {
		s, err, tx, _ := recoverWith(t, delegatedDescription().Capabilities, nil, status)
		requireCode(t, err, "session_liveness_unknown")
		if s != nil {
			t.Fatal("unadmitted recovery reported a session")
		}
		assertPending(t, tx, true)
	})
	t.Run("frozen-token-without-status", func(t *testing.T) {
		caps := slices.DeleteFunc(capableCaps(), func(v string) bool { return v == "status/1" })
		s, err, tx, _ := recoverWith(t, caps, nil, status)
		requireCode(t, err, "session_liveness_unknown")
		if s != nil {
			t.Fatal("token without status/1 admitted")
		}
		assertPending(t, tx, true)
	})
	t.Run("re-registered-manager", func(t *testing.T) {
		s, err, tx, traces := recoverWith(t, capableCaps(), func(r *Reconciler, c Client) {
			entry := r.Registry[c.ID]
			entry.RegisteredAt = "2026-01-02T00:00:00Z"
			r.Registry[c.ID] = entry
		})
		requireCode(t, err, "werkstatt_operation_unresolved")
		if s != nil || len(traces()) != 0 {
			t.Fatalf("changed incarnation was called: traces=%d", len(traces()))
		}
		assertPending(t, tx, true)
	})
	t.Run("manager-missing", func(t *testing.T) {
		s, err, tx, traces := recoverWith(t, capableCaps(), func(r *Reconciler, c Client) {
			delete(r.Registry, c.ID)
		})
		requireCode(t, err, "werkstatt_operation_unresolved")
		if s != nil || len(traces()) != 0 {
			t.Fatalf("missing manager was called: traces=%d", len(traces()))
		}
		assertPending(t, tx, true)
	})
}

// Claim: a pending operation without a lease admission holds no frozen
// capabilities, so its recovery keeps the no-token rule (a pinned (c) path):
// a started outcome carrying process stays unknown with the journal
// retained. Beside it the same recovery without the member still starts.
func TestUnleasedRecoveryKeepsNoTokenRule(t *testing.T) {
	binary := buildScript(t)
	t.Run("started-without-member", func(t *testing.T) {
		c, _ := scripted(t, binary, step{Operation: "status", Body: outcome("started")})
		tx := transaction(t)
		putPending(t, tx, pending(c))
		s, err := reconciler(c).Recover(bounded(t), tx)
		if err != nil || s == nil {
			t.Fatalf("valid control: %v %v", s, err)
		}
		assertPending(t, tx, false)
	})
	t.Run("started-with-process", func(t *testing.T) {
		c, _ := scripted(t, binary, step{Operation: "status", Body: startedOutcomeProcessBody()})
		tx := transaction(t)
		putPending(t, tx, pending(c))
		s, err := reconciler(c).Recover(bounded(t), tx)
		requireCode(t, err, "session_liveness_unknown")
		if s != nil {
			t.Fatal("unadmitted recovery reported a session")
		}
		assertPending(t, tx, true)
	})
}
