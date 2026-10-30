package main

import (
	"context"
	"os"
	"syscall"

	"github.com/relux-works/curator-agent-launcher/internal/netproxy"
)

// leaseOwner holds SIGINT and SIGTERM from before a filtering-gateway
// registration until its unregister has completed, so no signal in that span
// ends curator-run with the registration's state unknown. A signal before
// the child starts aborts the launch at the boundary; signals while the
// child runs are forwarded by execution and only drained here; a new signal
// while unregister is retried abandons the retries, with the lease's
// idle-bound alert.
type leaseOwner struct {
	sigs chan os.Signal
	stop func(chan<- os.Signal)
}

// ownLeaseSignals starts holding the signals. notify and stop are
// signal.Notify and signal.Stop in production.
func ownLeaseSignals(notify func(chan<- os.Signal, ...os.Signal), stop func(chan<- os.Signal)) *leaseOwner {
	o := &leaseOwner{sigs: make(chan os.Signal, 8), stop: stop}
	notify(o.sigs, os.Interrupt, syscall.SIGTERM)
	return o
}

// interrupted reports, without blocking, whether a signal has arrived.
func (o *leaseOwner) interrupted() bool {
	select {
	case <-o.sigs:
		return true
	default:
		return false
	}
}

// release stops holding the signals; used when no registration was made.
func (o *leaseOwner) release() { o.stop(o.sigs) }

// finish drains the signals received so far, unregisters, and abandons
// the retries only on a signal that arrives during them. It then stops
// holding the signals.
func (o *leaseOwner) finish(lease *netproxy.Lease) error {
	defer o.release()
	for drained := false; !drained; {
		select {
		case <-o.sigs:
		default:
			drained = true
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-o.sigs:
			cancel()
		case <-done:
		}
	}()
	return lease.Finish(ctx)
}
