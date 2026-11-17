//go:build unix

package client

import (
	"context"
	"encoding/json"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	schema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
)

type Caller interface {
	Call(context.Context, string, []byte) (Response, error)
}
type Reconciler struct {
	Leases   *LeaseCLI
	Registry map[string]sessionmanager.Entry
	Caller   func(string, sessionmanager.Entry) Caller
	Now      func() time.Time
}

func (r Reconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}
func (r Reconciler) owner(p Pending) (Caller, error) {
	entry, ok := r.Registry[p.Manager.ID]
	if !ok {
		return nil, refusal("werkstatt_operation_unresolved", map[string]string{"reason": "manager_missing", "operation": p.OperationID})
	}
	if entry.Executable != p.Manager.Executable || p.Lease != nil && entry.RegisteredAt != p.Manager.RegisteredAt {
		return nil, refusal("werkstatt_operation_unresolved", map[string]string{"reason": "manager_changed", "operation": p.OperationID})
	}
	remote := Caller(Client{ID: p.Manager.ID, Entry: entry})
	if r.Caller != nil {
		remote = r.Caller(p.Manager.ID, entry)
	}
	// Every decode of this operation — the submission itself and each
	// reconciliation through operation status and cancel — runs under the
	// capabilities frozen for it. On a fresh leased submission those are the
	// same capabilities this run's describe advertised, frozen into the lease
	// admission before the first call. A caller that is not this package's
	// transport decodes nothing here and keeps its own rule.
	if cl, ok := remote.(Client); ok {
		return cl.WithAdmission(frozenAdmission(p, entry)), nil
	}
	return remote, nil
}
func unresolved(operation string) *Error {
	return refusal("session_liveness_unknown", map[string]string{"operation": operation})
}

// Submit writes correlation before the first remote call. An existing pending
// operation is recovered instead; a new operation is never sent over it.
func (r Reconciler) Submit(ctx context.Context, tx *Transaction, p Pending, request []byte) (*Session, error) {
	existing, err := tx.Pending()
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return r.Recover(ctx, tx)
	}
	if err := ValidateRequest(p.Kind, request); err != nil {
		return nil, err
	}
	var correlation struct {
		OperationID  string `json:"operation_id"`
		SubmittedAt  string `json:"submitted_at"`
		AdmissionKey string `json:"admission_key"`
		Profile      struct {
			Name string `json:"name"`
		} `json:"profile"`
	}
	if json.Unmarshal(request, &correlation) != nil || correlation.OperationID != p.OperationID || correlation.SubmittedAt != p.SubmittedAt || correlation.AdmissionKey != p.AdmissionKey {
		return nil, refusal("launch_plan_invalid", nil)
	}
	// Only the validated request supplies durable profile evidence.
	p.Profile = correlation.Profile.Name
	remote, err := r.owner(p)
	if err != nil {
		return nil, err
	}
	if err := tx.WritePending(p); err != nil {
		return nil, err
	}
	live, err := tx.Liveness()
	if err != nil {
		return nil, err
	}
	expected := live.Expected()
	response, err := remote.Call(ctx, p.Kind, request)
	if err != nil {
		if IsCode(err, "session_host_unavailable") || IsCode(err, "session_host_protocol_error") {
			return r.Recover(ctx, tx)
		}
		if clearErr := tx.clearTerminal(p.OperationID, "failed"); clearErr != nil {
			return nil, clearErr
		}
		return nil, err
	}
	if response.Session == nil || response.Session.OperationID != p.OperationID {
		return r.Recover(ctx, tx)
	}
	return r.finish(ctx, tx, p, expected, Outcome{Outcome: "started", Session: response.Session})
}

// Recover consumes cancel's answer directly. Only a confirmed terminal outcome
// clears correlation; any failed status/cancel, poison or unknown retains it.
func (r Reconciler) Recover(ctx context.Context, tx *Transaction) (*Session, error) {
	p, err := tx.Pending()
	if err != nil || p == nil {
		return nil, err
	}
	if p.Lease != nil {
		return r.runLeased(ctx, tx, *p, true)
	}
	remote, err := r.owner(*p)
	if err != nil {
		return nil, err
	}
	return r.recoverOwned(ctx, tx, *p, remote)
}
func (r Reconciler) recoverOwned(ctx context.Context, tx *Transaction, p Pending, remote Caller) (*Session, error) {
	live, err := tx.Liveness()
	if err != nil {
		return nil, err
	}
	expected := live.Expected()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	data, _ := json.Marshal(map[string]string{"operation_id": p.OperationID})
	response, err := remote.Call(ctx, "status", data)
	for {
		if err != nil || ctx.Err() != nil || response.Outcome == nil {
			if boundLeaseRecord(p) {
				if _, e := ensureBindPending(tx, p); e != nil {
					return nil, e
				}
			}
			return nil, unresolved(p.OperationID)
		}
		outcome := *response.Outcome
		// A bound lease settles only on a matching started outcome; admitted may still become started.
		if boundLeaseRecord(p) && outcome.Outcome != "started" && outcome.Outcome != "admitted" {
			return r.refuseBoundNonStarted(tx, p)
		}
		switch outcome.Outcome {
		case "started", "failed", "cancelled":
			if outcome.Outcome == "started" && (outcome.Session == nil || outcome.Session.OperationID != p.OperationID) {
				return nil, unresolved(p.OperationID)
			}
			return r.finish(ctx, tx, p, expected, outcome)
		case "not_found", "prepared":
			response, err = remote.Call(ctx, "cancel", data)
		case "admitted":
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				if boundLeaseRecord(p) {
					if _, e := ensureBindPending(tx, p); e != nil {
						return nil, e
					}
				}
				return nil, unresolved(p.OperationID)
			case <-timer.C:
			}
			response, err = remote.Call(ctx, "status", data)
		default:
			return nil, unresolved(p.OperationID)
		}
	}
}
func boundLeaseRecord(p Pending) bool {
	return p.Lease != nil && p.Lease.Record != nil && isBoundHolder(p.Lease.Record.Holder)
}
func (r Reconciler) refuseBoundNonStarted(tx *Transaction, p Pending) (*Session, error) {
	if _, err := ensureBindPending(tx, p); err != nil {
		return nil, err
	}
	session := ""
	if p.Lease.Record.Holder.Session != nil {
		session = *p.Lease.Record.Holder.Session
	}
	return nil, holderMismatch("session_generation_changed", session)
}
func (r Reconciler) finish(ctx context.Context, tx *Transaction, p Pending, expected Expected, outcome Outcome) (*Session, error) {
	var result *Session
	var resultErr error
	switch outcome.Outcome {
	case "started":
		result = outcome.Session
		op := p.OperationID
		next := Liveness{Profile: p.Profile, ID: tx.id, State: result.State, Manager: p.Manager, OperationID: &op, StateSource: "manager", Session: result.Session, Generation: result.Generation, StartedAt: result.StartedAt, LastHeartbeatAt: r.now().UTC().Format(time.RFC3339), LastActivityAt: result.LastActivityAt, Doorbell: result.Doorbell.State, DoorbellUncertain: result.Doorbell.Uncertain}
		written, err := true, error(nil)
		if !tx.plain {
			written, err = tx.CompareAndSwap(expected, next)
		}
		if err != nil {
			resultErr = refusal("werkstatt_liveness_write_failed", map[string]string{"session": result.Session})
		} else if !written {
			resultErr = refusal("werkstatt_liveness_write_failed", map[string]string{"session": result.Session})
		}
		// Recovery still needs the operation and owner until liveness commits.
		if resultErr != nil {
			return result, resultErr
		}
	case "failed":
		resultErr = refusal(outcome.Code, nil)
	case "cancelled":
		resultErr = unavailable("cancelled")
	}
	if p.Lease != nil {
		if outcome.Outcome == "started" {
			return r.bindStarted(ctx, tx, p, result)
		}
		if boundLeaseRecord(p) {
			return r.refuseBoundNonStarted(tx, p)
		}
		if err := tx.settleLease(p, outcome.Outcome, outcome.Outcome, r.now()); err != nil {
			return result, err
		}
		return result, resultErr
	}
	if err := tx.clearTerminal(p.OperationID, outcome.Outcome); err != nil {
		return result, err
	}
	return result, resultErr
}

func ensureBindPending(tx *Transaction, p Pending) (Pending, error) {
	order := []string{"lease_requested", "lease_prepared", "possibly_submitted", "bind_pending"}
	idx := map[string]int{"lease_requested": 0, "lease_prepared": 1, "possibly_submitted": 2, "bind_pending": 3}
	cur, ok := idx[p.Phase]
	if !ok {
		p.Phase = "bind_pending"
		if err := tx.updateLease(p); err != nil {
			return p, err
		}
		return p, nil
	}
	if cur == 3 {
		if err := tx.updateLease(p); err != nil {
			return p, err
		}
		return p, nil
	}
	for cur < 3 {
		p.Phase = order[cur+1]
		if err := tx.updateLease(p); err != nil {
			return p, err
		}
		cur++
	}
	return p, nil
}
func (r Reconciler) bindStarted(ctx context.Context, tx *Transaction, p Pending, result *Session) (*Session, error) {
	admitted := ProcessObservation(Describe{Capabilities: p.Lease.Capabilities}) && processIsTuple(result.Process)
	if boundLeaseRecord(p) {
		held := p.Lease.Record.Holder
		reason := ""
		switch {
		case !preparedMembersEqual(p.Lease.Holder, held):
			reason = "manager_incarnation_changed"
		case held.Session == nil || *held.Session != result.Session || held.Generation == nil || *held.Generation != result.Generation:
			reason = "session_generation_changed"
		case !admitted:
			if _, err := ensureBindPending(tx, p); err != nil {
				return result, err
			}
			e := leaseFailure("process_incarnation_unavailable")
			e.Details["session"] = result.Session
			return result, e
		case !processEqual(held.Process, result.Process):
			reason = "process_incarnation_mismatch"
		}
		if reason == "" {
			if err := tx.clearTerminal(p.OperationID, "started"); err != nil {
				return result, err
			}
			return result, nil
		}
		if _, err := ensureBindPending(tx, p); err != nil {
			return result, err
		}
		return result, holderMismatch(reason, result.Session)
	}
	if !admitted || p.Lease.Record == nil {
		if _, err := ensureBindPending(tx, p); err != nil {
			return result, err
		}
		e := leaseFailure("process_incarnation_unavailable")
		e.Details["session"] = result.Session
		return result, e
	}
	bound := boundHolder(p.Lease.Holder, result.Session, result.Generation, result.Process)
	hb, err := json.Marshal(bound)
	if err != nil || schema.Validate("lease-holder.schema.json", hb) != nil {
		if _, err := ensureBindPending(tx, p); err != nil {
			return result, err
		}
		e := leaseFailure("process_incarnation_unavailable")
		e.Details["session"] = result.Session
		return result, e
	}
	p, err = ensureBindPending(tx, p)
	if err != nil {
		return result, err
	}
	if _, err := r.leaseCLI().Bind(ctx, p, bound); err != nil {
		if e, ok := err.(*Error); ok {
			if e.Details == nil {
				e.Details = map[string]string{}
			}
			e.Details["session"] = result.Session
			return result, e
		}
		return result, err
	}
	if err := tx.clearTerminal(p.OperationID, "started"); err != nil {
		return result, err
	}
	return result, nil
}

// ReconcilePlain follows the same bounded protocol without local correlation;
// the attached caller retains operation_id when reporting an unknown result.
func ReconcilePlain(ctx context.Context, remote Caller, operation string) (*Session, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	data, _ := json.Marshal(map[string]string{"operation_id": operation})
	response, err := remote.Call(ctx, "status", data)
	for {
		if err != nil || ctx.Err() != nil || response.Outcome == nil {
			return nil, unresolved(operation)
		}
		switch o := response.Outcome; o.Outcome {
		case "started":
			if o.Session == nil || o.Session.OperationID != operation {
				return nil, unresolved(operation)
			}
			return o.Session, nil
		case "failed":
			return nil, refusal(o.Code, nil)
		case "cancelled":
			return nil, unavailable("cancelled")
		case "not_found", "prepared":
			response, err = remote.Call(ctx, "cancel", data)
		case "admitted":
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, unresolved(operation)
			case <-timer.C:
			}
			response, err = remote.Call(ctx, "status", data)
		default:
			return nil, unresolved(operation)
		}
	}
}
