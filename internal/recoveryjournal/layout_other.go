//go:build !darwin && !linux

package recoveryjournal

import "context"

import "github.com/relux-works/curator-agent-launcher/internal/netproxy"

type Namespace struct{}

func (*Namespace) PruneExact(context.Context, string, Binding) (PruneObservation, error) {
	return PruneObservation{}, ErrUnsupported
}

func DiscoverBinding(context.Context, []string, string) (Binding, error) {
	return Binding{}, ErrUnsupported
}
func OpenExisting(Binding) (*Namespace, error)                          { return nil, ErrUnsupported }
func (*Namespace) Close() error                                         { return nil }
func (*Namespace) Prepare(context.Context, Envelope) (*Prepared, error) { return nil, ErrUnsupported }
func (*Namespace) ReadExact(context.Context, string, Binding) (*Entry, error) {
	return nil, ErrUnsupported
}
func (*Namespace) Issue(context.Context, Envelope, Binding) (netproxy.RecoveryRegistration, error) {
	return netproxy.RecoveryRegistration{}, ErrUnsupported
}
func (*Namespace) RecoverExact(context.Context, string, Binding, string) (CleanupObservation, error) {
	return CleanupObservation{}, ErrUnsupported
}
