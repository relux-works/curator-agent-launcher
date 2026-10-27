//go:build unix

package client

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	schema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
	"golang.org/x/sys/unix"
)

func (t *Transaction) updateLease(p Pending) error {
	old, err := t.Pending()
	if err != nil {
		return err
	}
	if old == nil || old.Lease == nil || p.Lease == nil {
		return storeError(t.id)
	}
	// Immutable admission/request correlation survives all phase changes.
	a, b := *old, p
	a.Phase, b.Phase = "", ""
	la, lb := *a.Lease, *b.Lease
	la.Record, lb.Record = nil, nil
	a.Lease, b.Lease = &la, &lb
	if !reflect.DeepEqual(a, b) {
		return storeError(t.id)
	}
	phases := map[string]int{"lease_requested": 0, "lease_prepared": 1, "possibly_submitted": 2, "bind_pending": 3}
	if phases[p.Phase] < phases[old.Phase] || phases[p.Phase] > phases[old.Phase]+1 {
		return storeError(t.id)
	}
	data, err := json.Marshal(p)
	if err != nil || schema.Validate("operation.schema.json", data) != nil {
		return storeError(t.id)
	}
	if sessionmanager.WriteAtomic(filepath.Join(t.dir, t.id+".op.json"), data) != nil {
		return storeError(t.id)
	}
	return nil
}
func (t *Transaction) settleLease(p Pending, outcome, reason string, at time.Time) error {
	dir := filepath.Join(filepath.Dir(t.dir), "operations", "settled")
	if err := ownedLeaseDirectory(filepath.Dir(dir)); err != nil {
		return storeError(t.id)
	}
	if err := ownedLeaseDirectory(dir); err != nil {
		return storeError(t.id)
	}
	// Use the same ownership/no-follow reader for retained evidence.
	data, err := json.Marshal(struct {
		Operation Pending `json:"operation"`
		SettledAt string  `json:"settled_at"`
		Outcome   string  `json:"outcome"`
		Reason    string  `json:"reason"`
	}{p, at.UTC().Format(time.RFC3339), outcome, reason})
	if err != nil || schema.Validate("lease-settlement.schema.json", data) != nil {
		return storeError(t.id)
	}
	if sessionmanager.WriteAtomic(filepath.Join(dir, p.OperationID+".json"), data) != nil {
		return storeError(t.id)
	}
	return t.clearTerminal(p.OperationID, outcome)
}
func (r Reconciler) leaseCLI() LeaseCLI {
	if r.Leases != nil {
		return *r.Leases
	}
	return LeaseCLI{}
}

// SubmitLeased preserves the admitted request before acquire. A failed or lost
// acquire reply is never repaired by generating a replacement identity.
func (r Reconciler) SubmitLeased(ctx context.Context, tx *Transaction, p Pending, request []byte, d Describe) (*Session, error) {
	if old, err := tx.Pending(); err != nil {
		return nil, err
	} else if old != nil {
		return r.Recover(ctx, tx)
	}
	if err := ValidateRequest(p.Kind, request); err != nil {
		return nil, err
	}
	var q struct {
		OperationID  string  `json:"operation_id"`
		SubmittedAt  string  `json:"submitted_at"`
		AdmissionKey *string `json:"admission_key"`
		Runtime      string  `json:"runtime"`
		CWD          string  `json:"cwd"`
		Profile      struct {
			Name string `json:"name"`
			Lock string `json:"lock_sha256"`
		} `json:"profile"`
	}
	if json.Unmarshal(request, &q) != nil || q.OperationID != p.OperationID || q.SubmittedAt != p.SubmittedAt || (q.AdmissionKey == nil && p.AdmissionKey != "") || (q.AdmissionKey != nil && *q.AdmissionKey != p.AdmissionKey) {
		return nil, refusal("launch_plan_invalid", nil)
	}
	if _, err := RequiredForRequest(d, p.Kind, request, Needs{ExplicitHosted: true, Werkstatt: !tx.plain, SignedOff: Q1SignedOff}); err != nil {
		return nil, err
	}
	cwd := q.CWD
	if p.Kind == "resume" {
		var scope struct {
			Scope struct {
				CWD string `json:"cwd"`
			} `json:"scope"`
		}
		_ = json.Unmarshal(request, &scope)
		cwd = scope.Scope.CWD
	}
	if cwd == "" {
		cwd = p.CWD
	}
	physical, err := filepath.EvalSymlinks(cwd)
	if err != nil || !filepath.IsAbs(physical) || cwd != physical {
		return nil, refusal("launch_plan_invalid", nil)
	}
	p.Profile, p.CWD, p.Request, p.Phase = q.Profile.Name, physical, request, "lease_requested"
	var lane *string
	if !tx.plain {
		id := tx.id
		lane = &id
	}
	caps, ops := leaseEvidence(d, q.Runtime)
	p.Lease = &LeaseAdmission{RequestID: p.OperationID, Route: "delegated", Environment: q.Runtime, LockSHA256: q.Profile.Lock, Holder: PreparedHolder{Kind: "prepared", Launcher: "curator-run", ManagerID: p.Manager.ID, ManagerRegisteredAt: p.Manager.RegisteredAt, RequestID: p.OperationID, Werkstatt: lane}, Capabilities: caps, Operations: ops}
	c := r.leaseCLI()
	records, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	if err = Allocation(records, p); err != nil {
		return nil, err
	}
	if err = tx.WritePending(p); err != nil {
		return nil, err
	}
	return r.runLeased(ctx, tx, p, false)
}
func (r Reconciler) runLeased(ctx context.Context, tx *Transaction, p Pending, recovery bool) (*Session, error) {
	c := r.leaseCLI()
	record, reason, err := c.Acquire(ctx, p)
	if err != nil {
		return nil, err
	}
	p.Lease.Record = record
	// Terminal acquire replay is non-admitting, including zero manager describe.
	if reason == "already_released" || reason == "already_broken" {
		if err = tx.settleLease(p, "not_started", reason, r.now()); err != nil {
			return nil, err
		}
		return nil, leaseFailure(reason)
	}
	remote, err := r.owner(p)
	if err != nil {
		return nil, err
	}
	if p.Phase == "lease_requested" {
		p.Phase = "lease_prepared"
	}
	if err = tx.updateLease(p); err != nil {
		return nil, err
	}
	if p.Phase == "possibly_submitted" || p.Phase == "bind_pending" {
		return r.recoverSubmittedLease(ctx, tx, p, remote)
	}
	if recovery {
		answer, e := remote.Call(ctx, "describe", []byte(`{}`))
		if e != nil {
			return nil, e
		}
		if answer.Describe == nil {
			return nil, leaseFailure("route_not_lease_capable")
		}
		if _, e = RequiredForRequest(*answer.Describe, p.Kind, p.Request, Needs{ExplicitHosted: true, Werkstatt: !tx.plain, SignedOff: Q1SignedOff}); e != nil {
			return nil, leaseFailure("route_not_lease_capable")
		}
		caps, ops := leaseEvidence(*answer.Describe, p.Lease.Environment)
		if !reflect.DeepEqual(caps, p.Lease.Capabilities) || !reflect.DeepEqual(ops, p.Lease.Operations) {
			return nil, leaseFailure("route_not_lease_capable")
		}
	}
	records, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	if err = Allocation(records, p); err != nil {
		return nil, err
	}
	found := false
	for _, v := range records {
		if v.LeaseID == record.LeaseID && reflect.DeepEqual(v, *record) {
			found = true
		}
	}
	if !found {
		return nil, leaseFailure("curator_lease_protocol_error")
	}
	live, err := tx.Liveness()
	if err != nil {
		return nil, err
	}
	p.Phase = "possibly_submitted"
	if err = tx.updateLease(p); err != nil {
		return nil, err
	}
	answer, err := remote.Call(ctx, p.Kind, p.Request)
	if err != nil && !IsCode(err, "session_host_unavailable") && !IsCode(err, "session_host_protocol_error") {
		if e := tx.settleLease(p, "failed", "manager_refused", r.now()); e != nil {
			return nil, e
		}
		return nil, err
	}
	if err != nil || answer.Session == nil || answer.Session.OperationID != p.OperationID {
		return r.recoverSubmittedLease(ctx, tx, p, remote)
	}
	return r.finish(tx, p, live.Expected(), Outcome{Outcome: "started", Session: answer.Session})
}
func (r Reconciler) recoverSubmittedLease(ctx context.Context, tx *Transaction, p Pending, remote Caller) (*Session, error) {
	return r.recoverOwned(ctx, tx, p, remote)
}

func PlainStore(environment []string) Store {
	if environment == nil {
		environment = os.Environ()
	}
	values := map[string]string{}
	for _, v := range environment {
		if k, s, ok := strings.Cut(v, "="); ok {
			values[k] = s
		}
	}
	root := values["XDG_STATE_HOME"]
	if root == "" {
		root = filepath.Join(values["HOME"], ".local", "state")
	}
	return Store{Directory: filepath.Join(root, "swarma", "launcher", "hosted"), Plain: true}
}

// RecoverMatching never waits on a live invocation's lock and never chooses
// between two abandoned admissions. true always ends this invocation.
func (s Store) RecoverMatching(ctx context.Context, r Reconciler, env, profile, cwd string, b Binding) (bool, *Session, error) {
	physical, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return false, nil, refusal("launch_plan_invalid", nil)
	}
	entries, err := os.ReadDir(s.Directory)
	if os.IsNotExist(err) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, storeError("")
	}
	var matches []*Transaction
	defer func() {
		for _, tx := range matches {
			tx.Close()
		}
	}()
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".op.json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".op.json")
		if !s.validID(id) {
			return false, nil, storeError("")
		}
		immediate, cancel := context.WithCancel(ctx)
		cancel()
		tx, e := s.Lock(immediate, id)
		if IsCode(e, "werkstatt_busy") {
			continue
		}
		if e != nil {
			return false, nil, e
		}
		p, e := tx.Pending()
		if e != nil {
			tx.Close()
			return false, nil, e
		}
		if p != nil && p.Lease != nil && p.Lease.Environment == env && p.Profile == profile && p.CWD == physical && p.Manager == b {
			matches = append(matches, tx)
		} else {
			tx.Close()
		}
	}
	if len(matches) > 1 {
		return true, nil, refusal("session_operation_conflict", nil)
	}
	if len(matches) == 0 {
		return false, nil, nil
	}
	result, err := r.Recover(ctx, matches[0])
	return true, result, err
}

func (c Client) StartLeased(ctx context.Context, kind string, request []byte, d Describe) (*Session, error) {
	var q struct {
		OperationID string `json:"operation_id"`
		SubmittedAt string `json:"submitted_at"`
	}
	if ValidateRequest(kind, request) != nil || json.Unmarshal(request, &q) != nil {
		return nil, refusal("launch_plan_invalid", nil)
	}
	store := PlainStore(c.Environment)
	tx, err := store.Lock(ctx, q.OperationID)
	if err != nil {
		return nil, err
	}
	defer tx.Close()
	p := Pending{Kind: kind, OperationID: q.OperationID, SubmittedAt: q.SubmittedAt, CWD: c.CWD, Manager: Binding{ID: c.ID, Executable: c.Entry.Executable, RegisteredAt: c.Entry.RegisteredAt}}
	r := Reconciler{Registry: map[string]sessionmanager.Entry{c.ID: c.Entry}, Leases: &LeaseCLI{Environment: c.Environment}, Caller: func(string, sessionmanager.Entry) Caller { return c }}
	return r.SubmitLeased(ctx, tx, p, request, d)
}

func ownedLeaseDirectory(dir string) error {
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	var st unix.Stat_t
	if unix.Lstat(dir, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Uid != uint32(os.Geteuid()) || st.Mode&0022 != 0 {
		return os.ErrPermission
	}
	return nil
}
func validateLeaseCorrelation(p Pending) bool {
	if p.Lease == nil {
		return p.Phase == ""
	}
	if ValidateRequest(p.Kind, p.Request) != nil {
		return false
	}
	var q struct {
		OperationID string `json:"operation_id"`
		SubmittedAt string `json:"submitted_at"`
		Runtime     string `json:"runtime"`
		CWD         string `json:"cwd"`
		Scope       struct {
			CWD string `json:"cwd"`
		} `json:"scope"`
		AdmissionKey *string `json:"admission_key"`
		Profile      struct {
			Name string `json:"name"`
			Lock string `json:"lock_sha256"`
		} `json:"profile"`
	}
	if json.Unmarshal(p.Request, &q) != nil {
		return false
	}
	key := ""
	if q.AdmissionKey != nil {
		key = *q.AdmissionKey
	}
	requestCWD := q.CWD
	if p.Kind == "resume" {
		requestCWD = q.Scope.CWD
	}
	if requestCWD != p.CWD || !filepath.IsAbs(p.CWD) || filepath.Clean(p.CWD) != p.CWD {
		return false
	}
	l := p.Lease
	h := l.Holder
	if q.OperationID != p.OperationID || q.SubmittedAt != p.SubmittedAt || key != p.AdmissionKey || q.Runtime != l.Environment || q.Profile.Name != p.Profile || q.Profile.Lock != l.LockSHA256 || l.RequestID != p.OperationID || l.Route != "delegated" || h.Kind != "prepared" || h.Launcher != "curator-run" || h.RequestID != l.RequestID || h.ManagerID != p.Manager.ID || h.ManagerRegisteredAt != p.Manager.RegisteredAt {
		return false
	}
	if p.AdmissionKey == "" {
		if h.Werkstatt != nil {
			return false
		}
	} else if h.Werkstatt == nil || p.AdmissionKey != "werkstatt:"+*h.Werkstatt {
		return false
	}
	if p.Phase != "lease_requested" && l.Record == nil {
		return false
	}
	if l.Record != nil {
		record := l.Record
		if record.Environment != l.Environment || record.Profile != p.Profile || record.LockSHA256 != l.LockSHA256 || !reflect.DeepEqual(record.Holder, h) || record.Phase != "prepared" {
			return false
		}
	}
	return true
}
