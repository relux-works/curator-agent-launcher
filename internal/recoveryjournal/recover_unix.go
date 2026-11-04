//go:build darwin || linux

package recoveryjournal

import (
	"context"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/netproxy"
	"golang.org/x/sys/unix"
)

type serviceObservation struct {
	launch        *string
	outcome, boot string
}
type confirmCall func(context.Context, Envelope) (serviceObservation, error)

// RecoverExact never registers, activates or discovers another namespace. Its
// only effect authority is the proof in the exact original private entry. It
// retains the original E in terminal secret/2 after success; exact pruning is
// a separate operation.
func (n *Namespace) RecoverExact(ctx context.Context, attempt string, current Binding, operation string) (CleanupObservation, error) {
	return n.recoverExact(ctx, attempt, current, operation, nativeReceiptOperations(), n.confirmRecovery, time.Now)
}

func (n *Namespace) confirmRecovery(ctx context.Context, e Envelope) (serviceObservation, error) {
	c, err := netproxy.NewRecovery(n.binding.socket)
	if err != nil {
		return serviceObservation{}, ErrRecoveryUnknown
	}
	// This supplies an observed service boot for the receipt, never a replacement
	// envelope. Recover still authenticates the original E and registry identity.
	h, err := c.Health(ctx)
	if err != nil || h.RegistryID != e.RegistryID {
		return serviceObservation{}, ErrRecoveryUnknown
	}
	r, err := c.Recover(ctx, e)
	if err != nil || r == nil || r.AttemptID() != e.AttemptID {
		return serviceObservation{}, ErrRecoveryUnknown
	}
	var launch *string
	if id, ok := r.LaunchID(); ok {
		launch = &id
	}
	return serviceObservation{launch, r.Outcome(), h.Boot}, nil
}

func (n *Namespace) recoverExact(ctx context.Context, attempt string, current Binding, operation string, ops receiptOperations, confirm confirmCall, now func() time.Time) (observation CleanupObservation, failure error) {
	return n.recoverRetained(ctx, attempt, current, operation, ops, confirm, now, nativeTerminalOperations(), nativeAwake)
}

func (n *Namespace) recoverRetained(ctx context.Context, attempt string, current Binding, operation string, ops receiptOperations, confirm confirmCall, now func() time.Time, terminalOps terminalOperations, awake func() (awakeObservation, error)) (observation CleanupObservation, failure error) {
	if !digest(attempt) || !validOperation(operation) {
		return observation, ErrRead
	}
	if err := n.binding.Match(current); err != nil {
		return observation, err
	}
	if _, ok := ctx.Deadline(); !ok || ctx.Err() != nil {
		return observation, ErrLock
	}
	n.use.RLock()
	defer n.use.RUnlock()
	if n.closed || !n.sameLocation() {
		return observation, ErrUnsafeLayout
	}
	lock, err := n.lock(ctx)
	if err != nil {
		return observation, err
	}
	defer func() {
		if unix.Close(lock) != nil {
			observation.Receipt = nil
			failure = ErrLocalEvidence
		}
	}()
	if ctx.Err() != nil || !n.sameLocation() || !lockStillBound(n.directory, lock) {
		return observation, ErrRead
	}
	entry, err := n.readLocked(ctx, attempt, nativeReadOperations(), lock)
	if err != nil {
		return observation, err
	}
	result, err := confirm(ctx, entry.envelope)
	if err != nil || ctx.Err() != nil {
		return observation, ErrRecoveryUnknown
	}
	if result.boot == "" ||
		(result.outcome == "revoked" && (result.launch == nil || !digest(*result.launch) || *result.launch == entry.envelope.RecoveryProof)) ||
		(result.outcome == "cancelled_before_issue" && result.launch != nil) ||
		(result.outcome != "revoked" && result.outcome != "cancelled_before_issue") {
		return observation, ErrRecoveryUnknown
	}
	if entry.retention != nil && (entry.outcome != result.outcome || !sameLaunch(entry.launch, result.launch)) {
		return observation, ErrRecoveryUnknown
	}
	observation.Confirmed = true
	observation.Outcome = result.outcome
	receipt := newReceipt(entry.envelope, n.binding, result.launch, result.outcome, result.boot, operation, now())
	// Validate even the internal confirmation projection. Only the native
	// closed client supplies it in production; faults cannot certify a receipt.
	raw, ok := encodeReceipt(receipt)
	if !ok {
		return observation, ErrLocalEvidence
	}
	committed, err := n.commitReceipt(ctx, receipt, raw, lock, ops)
	if err != nil {
		return observation, ErrLocalEvidence
	}
	// The awake observation MUST follow checked receipt durability. Public UTC
	// and the original issuance clock never supply the retention anchor.
	clock, err := awake()
	if err != nil {
		return observation, ErrLocalEvidence
	}
	anchor, err := nextRetention(entry.retention, committed, clock)
	if err != nil {
		return observation, ErrLocalEvidence
	}
	if err := n.commitTerminal(ctx, entry, receipt, anchor, lock, terminalOps); err != nil {
		return observation, ErrLocalEvidence
	}
	observation.Receipt = &ReceiptCommit{attempt, result.outcome}
	return observation, nil
}
