//go:build unix

package client

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
)

type leaseStep struct {
	Operation string
	Body      json.RawMessage
	Exit      int
}

func leaseReply(operation, reason string, records ...LeaseRecord) json.RawMessage {
	result := map[string]any{"leases": records}
	if operation == "acquire" {
		result = map[string]any{"changed": false, "lease": records[0]}
	}
	return encoded(map[string]any{"schema_version": 1, "operation": operation, "ok": true, "exit": 0, "code": nil, "reason": reason, "result": result, "details": nil})
}
func leaseFixture(t *testing.T, binary string, steps ...leaseStep) (LeaseCLI, func() [][]string) {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "plan"), encoded(steps))
	if err := os.Symlink(binary, filepath.Join(dir, "curator")); err != nil {
		t.Fatal(err)
	}
	read := func() [][]string {
		b, _ := os.ReadFile(filepath.Join(dir, "trace"))
		var rows [][]string
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if line == "" {
				continue
			}
			var row struct {
				PID  int
				Args []string
			}
			if json.Unmarshal([]byte(line), &row) != nil {
				t.Fatal("trace invalid")
			}
			assertGone(t, row.PID)
			rows = append(rows, row.Args)
		}
		return rows
	}
	// HL23 / L3: every fixture checks the explicit invocation counter, even on
	// error paths. Local settlements and bind refusals never authorize release.
	t.Cleanup(func() {
		counts := map[string]int{}
		for _, a := range read() {
			if len(a) > 2 {
				counts[a[2]]++
			}
		}
		if counts["release"] != 0 || counts["break"] != 0 {
			t.Fatalf("ZERO-RELEASE ALARM release=%d break=%d", counts["release"], counts["break"])
		}
	})
	return LeaseCLI{Environment: []string{"PATH=" + dir, "HOME=" + dir, "LEASE_FIXTURE=" + dir}}, read
}
func buildLeaseFixture(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "curator")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binary, "./testdata/leasecurator")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 100 * time.Millisecond
	defer func() {
		killGroup(cmd)
		if cmd.Process != nil {
			assertGone(t, cmd.Process.Pid)
			assertGone(t, -cmd.Process.Pid)
		}
	}()
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, b)
	}
	assertGone(t, cmd.Process.Pid)
	return binary
}
func leasePending(t *testing.T, tx *Transaction, b Binding) Pending {
	t.Helper()
	request := map[string]any{}
	_ = json.Unmarshal(document(t, "launch"), &request)
	request["cwd"], _ = filepath.EvalSymlinks(t.TempDir())
	request["admission_key"] = nil
	var lane *string
	if !tx.plain {
		id := tx.id
		lane = &id
		request["admission_key"] = "werkstatt:" + tx.id
	}
	d := delegatedDescription()
	caps, ops := leaseEvidence(d, "codex_cli")
	return Pending{Phase: "lease_requested", CWD: request["cwd"].(string), Request: encoded(request), Profile: "sample", OperationID: operationID, Kind: "launch", SubmittedAt: stamp, AdmissionKey: func() string {
		if lane != nil {
			return "werkstatt:" + *lane
		}
		return ""
	}(), Manager: b, Lease: &LeaseAdmission{RequestID: operationID, Route: "delegated", Environment: "codex_cli", LockSHA256: strings.Repeat("1", 64), Holder: PreparedHolder{Kind: "prepared", Launcher: "curator-run", ManagerID: b.ID, ManagerRegisteredAt: b.RegisteredAt, RequestID: operationID, Werkstatt: lane}, Capabilities: caps, Operations: ops}}
}
func leaseRecord(p Pending) LeaseRecord {
	return LeaseRecord{Kind: "home-lease/1", LeaseID: "lease-1", ManagerRootID: "root-1", HomeID: "home-1", Environment: p.Lease.Environment, Profile: p.Profile, LockSHA256: p.Lease.LockSHA256, Revision: 1, Phase: "prepared", AcquiredAt: stamp, Holder: p.Lease.Holder, Observation: struct {
		State      string  `json:"state"`
		ObservedAt *string `json:"observed_at"`
	}{State: "not_observed"}}
}
func leaseReason(t *testing.T, err error, reason string) {
	t.Helper()
	e := requireCode(t, err, "session_home_lease_unavailable")
	if e.Details["reason"] != reason || strings.Contains(e.Diagnostic(), "untrusted") {
		t.Fatalf("reason %v want %s", e, reason)
	}
}

// HL21/HL22/HL23; qualification L1/L2/L3: real executable envelopes precede
// scripted manager RPCs. Terminal replay has zero RPCs and retained settlement;
// lost prepared acquire replies replay byte-identical identity exactly once.
func TestHomeLeaseRecovery(t *testing.T) {
	binary := buildLeaseFixture(t)
	manager := buildScript(t)
	for _, phase := range []string{"prepared", "released", "broken"} {
		t.Run(phase, func(t *testing.T) {
			// A fully capable manager is present even for terminal replay: zero RPC
			// must be enforced by the consumer, not by an empty fixture script.
			steps := []step{{Operation: "describe", Body: string(encoded(wireDescription()))}, {Operation: "launch", Body: string(encoded(session()))}}
			c, traces := scripted(t, manager, steps...)
			tx := transaction(t)
			p := leasePending(t, tx, Binding{ID: c.ID, Executable: c.Entry.Executable, RegisteredAt: stamp})
			record := leaseRecord(p)
			reason := "acquired"
			if phase != "prepared" {
				record.Phase = phase
				reason = "already_" + phase
			}
			plan := []leaseStep{{"acquire", leaseReply("acquire", reason, record), 0}}
			if phase == "prepared" {
				plan = append(plan, leaseStep{"list", leaseReply("list", "listed", record), 0})
			}
			leases, calls := leaseFixture(t, binary, plan...)
			r := reconciler(c)
			r.Leases = &leases
			putPending(t, tx, p)
			started, err := r.Recover(bounded(t), tx)
			if phase != "prepared" {
				if pidLog, _ := os.ReadFile(c.Entry.ArgvPrefix[0] + ".pids"); started != nil || len(traces()) != 0 || len(pidLog) != 0 {
					t.Fatal("HL21 TERMINAL-RPC ALARM")
				}
				leaseReason(t, err, reason)
				assertPending(t, tx, false)
				if _, e := os.Stat(filepath.Join(filepath.Dir(tx.dir), "operations", "settled", operationID+".json")); e != nil {
					t.Fatal("settlement lost", e)
				}
			} else {
				leaseReason(t, err, "process_incarnation_unavailable")
				if started == nil || len(traces()) != 2 {
					t.Fatal("prepared replay did not submit once", err, traces())
				}
				p, e := tx.Pending()
				if e != nil || p.Phase != "bind_pending" || p.Lease.Record.Phase != "prepared" {
					t.Fatal("bind protection lost", p, e)
				}
			}
			if len(calls()) != len(plan) {
				t.Fatal("unexpected lease invocation")
			}
		})
	}
	t.Run("lost-acquire-reply", func(t *testing.T) {
		c, traces := scripted(t, manager, step{Operation: "describe", Body: string(encoded(wireDescription()))}, step{Operation: "launch", Body: string(encoded(session()))})
		tx := transaction(t)
		p := leasePending(t, tx, Binding{ID: c.ID, Executable: c.Entry.Executable, RegisteredAt: stamp})
		record := leaseRecord(p)
		leases, calls := leaseFixture(t, binary, leaseStep{"acquire", nil, 1}, leaseStep{"acquire", leaseReply("acquire", "acquired", record), 0}, leaseStep{"list", leaseReply("list", "listed", record), 0})
		r := reconciler(c)
		r.Leases = &leases
		putPending(t, tx, p)
		_, err := r.Recover(bounded(t), tx)
		leaseReason(t, err, "curator_lease_protocol_error")
		if len(traces()) != 0 {
			t.Fatal("manager before acquire")
		}
		_, err = r.Recover(bounded(t), tx)
		leaseReason(t, err, "process_incarnation_unavailable")
		a := calls()
		if !reflect.DeepEqual(a[0], a[1]) || len(traces()) != 2 {
			t.Fatal("identity drift or duplicate submission")
		}
	})
}

// HL24 / allocation qualification: forward home-to-lane and reverse lane-to-
// tuple checks include foreign managers, terminal transitions and ad-hoc holders.
func TestHomeLeaseAllocation(t *testing.T) {
	tx := transaction(t)
	p := leasePending(t, tx, Binding{ID: "demo", Executable: "/plugin", RegisteredAt: stamp})
	record := leaseRecord(p)
	p.Lease.Record = &record
	for _, name := range []string{"same-lane", "foreign-lane", "ad-hoc", "other-profile", "other-home", "other-digest", "terminal"} {
		t.Run(name, func(t *testing.T) {
			v := record
			v.Holder.ManagerID = "other"
			want := true
			switch name {
			case "foreign-lane":
				s := "other"
				v.Holder.Werkstatt = &s
				want = false
			case "ad-hoc":
				v.Holder.Werkstatt = nil
				want = false
			case "other-profile":
				v.Profile = "other"
				v.HomeID = "other"
				want = false
			case "other-home":
				v.HomeID = "other"
				want = false
			case "other-digest":
				v.LockSHA256 = strings.Repeat("2", 64)
				want = false
			case "terminal":
				v.Phase = "broken"
				v.Profile = "other"
			}
			err := Allocation([]LeaseRecord{v}, p)
			if want && err != nil {
				t.Fatal(err)
			}
			if !want {
				leaseReason(t, err, "allocation_conflict")
			}
		})
	}
}

// HL21 / route qualification: per-runtime recovery evidence is mandatory;
// native selection returns before this fence, and resume is independent of exact.
func TestHomeLeaseRoutes(t *testing.T) {
	d := delegatedDescription()
	n := Needs{Runtime: "codex_cli", ExplicitHosted: true, Detached: true, SignedOff: true}
	if _, e := Required(d, n); e != nil {
		t.Fatal(e)
	}
	for _, token := range []string{"admission/1", "status/1"} {
		copy := d
		copy.Capabilities = slices.DeleteFunc(slices.Clone(d.Capabilities), func(s string) bool { return s == token })
		_, e := Required(copy, n)
		leaseReason(t, e, "route_not_lease_capable")
	}
	d.Runtimes["codex_cli"] = slices.DeleteFunc(d.Runtimes["codex_cli"], func(s string) bool { return s == "cancel" })
	_, err := Required(d, n)
	leaseReason(t, err, "route_not_lease_capable")
	n.Native = true
	if r, e := Required(Describe{}, n); e != nil || r.Kind != "native" {
		t.Fatal("native crossed fence")
	}
}

var _ = sessionmanager.Protocol

func wireDescription() Describe {
	d := delegatedDescription()
	d.Protocol = Protocol
	d.OK = true
	d.ID = "demo"
	d.Version = "1.0.0"
	return d
}

// HL21/HL22 / L1/L2: a durable possible-submission marker reconciles status
// after acquire, never launch; capability drift retains the prepared reservation.
func TestHomeLeaseCrashAndDrift(t *testing.T) {
	binary := buildLeaseFixture(t)
	manager := buildScript(t)
	for _, name := range []string{"start-before-bind", "capability-drift", "operation-drift", "failed-status", "registration-changed"} {
		t.Run(name, func(t *testing.T) {
			d := wireDescription()
			steps := []step{}
			switch name {
			case "start-before-bind":
				steps = []step{{Operation: "status", Body: outcome("started")}}
			case "failed-status":
				steps = []step{{Operation: "status", Body: outcome("failed")}}
			case "operation-drift":
				d.Runtimes["codex_cli"] = slices.DeleteFunc(slices.Clone(d.Runtimes["codex_cli"]), func(op string) bool { return op == "attach" })
				steps = []step{{Operation: "describe", Body: string(encoded(d))}}
			case "capability-drift":
				d.Capabilities = append(d.Capabilities, "future/1")
				steps = []step{{Operation: "describe", Body: string(encoded(d))}}
			}
			c, traces := scripted(t, manager, steps...)
			tx := transaction(t)
			p := leasePending(t, tx, Binding{ID: c.ID, Executable: c.Entry.Executable, RegisteredAt: stamp})
			record := leaseRecord(p)
			if name == "start-before-bind" || name == "failed-status" {
				p.Phase = "possibly_submitted"
				p.Lease.Record = &record
			}
			leases, _ := leaseFixture(t, binary, leaseStep{"acquire", leaseReply("acquire", "acquired", record), 0})
			r := reconciler(c)
			r.Leases = &leases
			if name == "registration-changed" {
				entry := r.Registry[c.ID]
				entry.RegisteredAt = "2026-01-02T00:00:00Z"
				r.Registry[c.ID] = entry
			}
			putPending(t, tx, p)
			started, err := r.Recover(bounded(t), tx)
			switch name {
			case "start-before-bind":
				leaseReason(t, err, "process_incarnation_unavailable")
				if started == nil {
					t.Fatal("started handle lost")
				}
			case "capability-drift", "operation-drift":
				leaseReason(t, err, "route_not_lease_capable")
			case "failed-status":
				requireCode(t, err, "policy_refused")
				assertPending(t, tx, false)
			case "registration-changed":
				requireCode(t, err, "werkstatt_operation_unresolved")
			}
			if len(traces()) != len(steps) {
				t.Fatal("unexpected submission", traces())
			}
			if name != "failed-status" {
				assertPending(t, tx, true)
			}
		})
	}
}
func leaseRefusal(code, reason string) json.RawMessage {
	return encoded(map[string]any{"schema_version": 1, "operation": "acquire", "ok": false, "exit": 1, "code": code, "reason": reason, "result": nil, "details": map[string]any{"lease_id": nil, "revision": nil, "holders": []any{}, "count": 0}})
}

// HL20/HL21 / L1/L5: the published request-conflict refusal leaves correlation
// intact; lock busy and digest mismatch remain distinct, with closed diagnostics.
func TestHomeLeaseRefusals(t *testing.T) {
	binary := buildLeaseFixture(t)
	for _, row := range [][2]string{{"environment_lease_holder_mismatch", "request_conflict"}, {"environment_lock_unavailable", "mutation_lock_busy"}, {"environment_lease_lock_mismatch", "profile_lock_changed"}} {
		t.Run(row[1], func(t *testing.T) {
			tx := transaction(t)
			p := leasePending(t, tx, Binding{ID: "demo", Executable: "/plugin", RegisteredAt: stamp})
			putPending(t, tx, p)
			before := readFile(t, filepath.Join(tx.dir, "example.op.json"))
			leases, _ := leaseFixture(t, binary, leaseStep{"acquire", leaseRefusal(row[0], row[1]), 1})
			r := Reconciler{Leases: &leases}
			_, err := r.Recover(bounded(t), tx)
			leaseReason(t, err, row[1])
			if requireCode(t, err, "session_home_lease_unavailable").Details["curator_code"] != row[0] {
				t.Fatal("lost Curator code")
			}
			if !reflect.DeepEqual(before, readFile(t, filepath.Join(tx.dir, "example.op.json"))) {
				t.Fatal("refusal rewrote payload")
			}
		})
	}
}

// HL20 / capability probe and L5: a valid closed list is the positive. Unknown,
// duplicate, wrong-operation/exit and non-v1 shapes refuse without echoing stderr.
func TestHomeLeaseProbe(t *testing.T) {
	binary := buildLeaseFixture(t)
	valid := leaseReply("list", "listed", []LeaseRecord{}...)
	for _, name := range []string{"valid", "unknown", "duplicate", "wrong-operation", "wrong-exit", "old-curator", "unknown-reason"} {
		t.Run(name, func(t *testing.T) {
			body := slices.Clone(valid)
			exit := 0
			reason := "curator_lease_protocol_error"
			switch name {
			case "unknown":
				body = append(body[:len(body)-1], []byte(`,"extra":true}`)...)
			case "duplicate":
				body = append(body[:len(body)-1], []byte(`,"exit":0}`)...)
			case "wrong-operation":
				body = []byte(strings.Replace(string(body), `"list"`, `"release"`, 1))
			case "wrong-exit":
				exit = 1
			case "old-curator":
				body = nil
				exit = 2
				reason = "curator_lease_unsupported"
			case "unknown-reason":
				body = []byte(strings.Replace(string(body), "listed", "guessed", 1))
			}
			leases, calls := leaseFixture(t, binary, leaseStep{"list", body, exit})
			records, err := leases.List(bounded(t))
			if name == "valid" {
				if err != nil || len(records) != 0 {
					t.Fatal(records, err)
				}
			} else {
				leaseReason(t, err, reason)
			}
			if len(calls()) != 1 {
				t.Fatal("probe retried")
			}
		})
	}
}

// HL21 / L1 plain recovery selector: held locks are live, cwd is part of the
// selector, a single terminal recovery ends the run, and ambiguity never chooses.
func TestHomeLeasePlainSelection(t *testing.T) {
	binary := buildLeaseFixture(t)
	for _, name := range []string{"fresh", "held", "different-cwd", "single", "multiple"} {
		t.Run(name, func(t *testing.T) {
			store := Store{Directory: t.TempDir(), Plain: true}
			cwd, _ := filepath.EvalSymlinks(t.TempDir())
			b := Binding{ID: "demo", Executable: "/plugin", RegisteredAt: stamp}
			var tx *Transaction
			var err error
			var p Pending
			if name != "fresh" {
				tx, err = store.Lock(bounded(t), operationID)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Close()
				p = leasePending(t, tx, b)
				p.CWD = cwd
				var request map[string]any
				_ = json.Unmarshal(p.Request, &request)
				request["cwd"] = cwd
				p.Request = encoded(request)
				putPending(t, tx, p)
				if name != "held" {
					tx.Close()
				}
			}
			if name == "multiple" {
				other, err := store.Lock(bounded(t), secondID)
				if err != nil {
					t.Fatal(err)
				}
				p.OperationID = secondID
				p.Lease.RequestID = secondID
				p.Lease.Holder.RequestID = secondID
				var q map[string]any
				_ = json.Unmarshal(p.Request, &q)
				q["operation_id"] = secondID
				p.Request = encoded(q)
				putPending(t, other, p)
				other.Close()
			}
			steps := []leaseStep{}
			if name == "single" {
				record := leaseRecord(p)
				record.Phase = "broken"
				steps = []leaseStep{{"acquire", leaseReply("acquire", "already_broken", record), 0}}
			}
			leases, calls := leaseFixture(t, binary, steps...)
			r := Reconciler{Leases: &leases}
			if name == "different-cwd" {
				cwd = t.TempDir()
			}
			matched, started, err := store.RecoverMatching(bounded(t), r, "codex_cli", "sample", cwd, b)
			switch name {
			case "single":
				leaseReason(t, err, "already_broken")
				if !matched {
					t.Fatal("recovery did not end invocation")
				}
			case "multiple":
				requireCode(t, err, "session_operation_conflict")
				if !matched {
					t.Fatal("ambiguous recovery continued")
				}
			default:
				if matched || err != nil {
					t.Fatal(matched, err)
				}
			}
			if started != nil || len(calls()) != len(steps) {
				t.Fatal("fresh or unmatched operation touched")
			}
		})
	}
}

// HL23 / L4: abandon retains the closed journal, prints a current-revision
// operator command, and never invokes it; unavailable revision is explicit.
func TestHomeLeaseAbandon(t *testing.T) {
	binary := buildLeaseFixture(t)
	for _, available := range []bool{true, false} {
		t.Run(map[bool]string{true: "current", false: "unavailable"}[available], func(t *testing.T) {
			tx := transaction(t)
			p := leasePending(t, tx, Binding{ID: "demo", Executable: "/plugin", RegisteredAt: stamp})
			record := leaseRecord(p)
			p.Lease.Record = &record
			p.Phase = "lease_prepared"
			putPending(t, tx, p)
			body := leaseReply("list", "listed", record)
			if !available {
				body = nil
			}
			leases, _ := leaseFixture(t, binary, leaseStep{"list", body, 0})
			var output strings.Builder
			WriteLeaseBreakHint(bounded(t), &output, leases, p)
			if available {
				if !strings.Contains(output.String(), "--if-revision 1 --operator --reason <reason>") {
					t.Fatal("operator action missing", output.String())
				}
			} else if strings.Contains(output.String(), "--if-revision") || !strings.Contains(output.String(), "unavailable") {
				t.Fatal("invented revision")
			}
			if err := tx.Abandon(time.Now()); err != nil {
				t.Fatal(err)
			}
			assertPending(t, tx, false)
			b, err := os.ReadFile(filepath.Join(filepath.Dir(tx.dir), "operations", "settled", operationID+".json"))
			if err != nil || !strings.Contains(string(b), "lease-1") {
				t.Fatal("abandon erased lease correlation", err)
			}
		})
	}
}

// HL21/HL23 / L1/L3: terminal replay never removes pending correlation if its
// retained settlement cannot be written; a neighboring writable archive settles.
func TestHomeLeaseSettlementFailure(t *testing.T) {
	binary := buildLeaseFixture(t)
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "writable", true: "blocked"}[blocked], func(t *testing.T) {
			tx := transaction(t)
			p := leasePending(t, tx, Binding{ID: "demo", Executable: "/plugin", RegisteredAt: stamp})
			putPending(t, tx, p)
			record := leaseRecord(p)
			record.Phase = "released"
			leases, calls := leaseFixture(t, binary, leaseStep{"acquire", leaseReply("acquire", "already_released", record), 0})
			if blocked {
				mustWrite(t, filepath.Join(filepath.Dir(tx.dir), "operations"), []byte("unchanged"))
			}
			_, err := (Reconciler{Leases: &leases}).Recover(bounded(t), tx)
			if blocked {
				requireCode(t, err, "werkstatt_record_invalid")
				assertPending(t, tx, true)
			} else {
				leaseReason(t, err, "already_released")
				assertPending(t, tx, false)
			}
			if len(calls()) != 1 {
				t.Fatal("settlement invoked extra lease operation")
			}
		})
	}
}

// HL21 / L1: journal/request correlation refuses cwd, digest and registration
// drift before acquire. The unchanged prepared journal remains a valid control.
func TestHomeLeaseJournalCorrelation(t *testing.T) {
	for _, field := range []string{"unchanged", "cwd", "digest", "registration"} {
		t.Run(field, func(t *testing.T) {
			tx := transaction(t)
			p := leasePending(t, tx, Binding{ID: "demo", Executable: "/plugin", RegisteredAt: stamp})
			switch field {
			case "cwd":
				p.CWD = t.TempDir()
			case "digest":
				p.Lease.LockSHA256 = strings.Repeat("2", 64)
			case "registration":
				p.Manager.RegisteredAt = "2026-01-02T00:00:00Z"
			}
			err := tx.WritePending(p)
			if field == "unchanged" {
				if err != nil {
					t.Fatal(err)
				}
				assertPending(t, tx, true)
			} else {
				requireCode(t, err, "werkstatt_record_invalid")
				assertPending(t, tx, false)
			}
		})
	}
}
