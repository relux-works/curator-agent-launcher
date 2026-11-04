//go:build darwin || linux

package recoveryjournal

import (
	"context"

	"github.com/relux-works/curator-agent-launcher/internal/netproxy"
)

type registerCall func(context.Context, Envelope) (netproxy.RecoveryRegistration, error)

// Issue admits one initial register request only after durable Prepare. It
// neither activates nor starts a child. The returned capability belongs only
// in the admitted runtime route; it is never written to this journal. Any
// failed/uncertain reply keeps the original E for Recover, never registration
// replay. The saved "prepared/not_observed" state is not a no-issuance verdict.
func (n *Namespace) Issue(ctx context.Context, e Envelope, current Binding) (netproxy.RecoveryRegistration, error) {
	client, err := netproxy.NewRecovery(n.binding.socket)
	if err != nil {
		return netproxy.RecoveryRegistration{}, ErrBinding
	}
	return n.issue(ctx, e, current, nativePrepareOperations(), client.Register)
}

func (n *Namespace) issue(ctx context.Context, e Envelope, current Binding, ops prepareOperations, register registerCall) (netproxy.RecoveryRegistration, error) {
	if err := n.binding.Match(current); err != nil {
		return netproxy.RecoveryRegistration{}, err
	}
	prepared, err := n.prepare(ctx, e, ops)
	if err != nil {
		return netproxy.RecoveryRegistration{}, err
	}
	// A cancelled or displaced caller cannot use a previously returned prepare
	// as fresh authority. Keep the record for the exact later recovery command.
	n.use.RLock()
	defer n.use.RUnlock()
	if ctx.Err() != nil || n.closed || !n.sameLocation() {
		return netproxy.RecoveryRegistration{}, ErrIssueUnknown
	}
	registration, err := register(ctx, prepared.envelope)
	if err != nil || ctx.Err() != nil {
		return netproxy.RecoveryRegistration{}, ErrIssueUnknown
	}
	return registration, nil
}
