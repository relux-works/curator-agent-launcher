//go:build unix

package client

import (
	"context"
	"encoding/json"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
)

type Caller interface {
	Call(context.Context, string, []byte) (Response, error)
}
type Reconciler struct {
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
	if entry.Executable != p.Manager.Executable {
		return nil, refusal("werkstatt_operation_unresolved", map[string]string{"reason": "manager_changed", "operation": p.OperationID})
	}
	if r.Caller != nil {
		return r.Caller(p.Manager.ID, entry), nil
	}
	return Client{ID: p.Manager.ID, Entry: entry}, nil
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
	return r.finish(tx, p, expected, Outcome{Outcome: "started", Session: response.Session})
}

// Recover consumes cancel's answer directly. Only a confirmed terminal outcome
// clears correlation; any failed status/cancel, poison or unknown retains it.
func (r Reconciler) Recover(ctx context.Context, tx *Transaction) (*Session, error) {
	p, err := tx.Pending()
	if err != nil || p == nil {
		return nil, err
	}
	remote, err := r.owner(*p)
	if err != nil {
		return nil, err
	}
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
			return nil, unresolved(p.OperationID)
		}
		outcome := *response.Outcome
		switch outcome.Outcome {
		case "started", "failed", "cancelled":
			if outcome.Outcome == "started" && (outcome.Session == nil || outcome.Session.OperationID != p.OperationID) {
				return nil, unresolved(p.OperationID)
			}
			return r.finish(tx, *p, expected, outcome)
		case "not_found", "prepared":
			response, err = remote.Call(ctx, "cancel", data)
		case "admitted":
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, unresolved(p.OperationID)
			case <-timer.C:
			}
			response, err = remote.Call(ctx, "status", data)
		default:
			return nil, unresolved(p.OperationID)
		}
	}
}
func (r Reconciler) finish(tx *Transaction, p Pending, expected Expected, outcome Outcome) (*Session, error) {
	var result *Session
	var resultErr error
	switch outcome.Outcome {
	case "started":
		result = outcome.Session
		op := p.OperationID
		next := Liveness{Profile: p.Profile, ID: tx.id, State: result.State, Manager: p.Manager, OperationID: &op, StateSource: "manager", Session: result.Session, Generation: result.Generation, StartedAt: result.StartedAt, LastHeartbeatAt: r.now().UTC().Format(time.RFC3339), LastActivityAt: result.LastActivityAt, Doorbell: result.Doorbell.State, DoorbellUncertain: result.Doorbell.Uncertain}
		written, err := tx.CompareAndSwap(expected, next)
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
	if err := tx.clearTerminal(p.OperationID, outcome.Outcome); err != nil {
		return result, err
	}
	return result, resultErr
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
