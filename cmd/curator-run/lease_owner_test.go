package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/netproxy"
)

// fakeSignals stands in for signal.Notify/Stop: it records the channel the
// owner registers so a test can deliver a signal into it.
type fakeSignals struct {
	mu      sync.Mutex
	ch      chan<- os.Signal
	stopped bool
}

func (f *fakeSignals) notify(c chan<- os.Signal, _ ...os.Signal) {
	f.mu.Lock()
	f.ch = c
	f.mu.Unlock()
}
func (f *fakeSignals) stop(chan<- os.Signal) { f.mu.Lock(); f.stopped = true; f.mu.Unlock() }
func (f *fakeSignals) send(s os.Signal)      { f.mu.Lock(); c := f.ch; f.mu.Unlock(); c <- s }

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *lockedBuffer) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

func ownedLease(t *testing.T, g *e2eGateway, alerts *lockedBuffer) *netproxy.Lease {
	t.Helper()
	c := netproxy.New(g.socket)
	reg := netproxy.Registration{Token: e2eToken, TokenHeader: "X-Netproxy-Token", ActivationSeconds: 120}
	l := netproxy.NewLease(c, reg, alerts, time.Now())
	l.Retry = 20 * time.Millisecond
	return l
}

// A signal that arrives while the child runs (it ended the child) is
// drained, not taken as a request to abandon: the unregister is retried
// through persist_failed until the gateway confirms, and the signals are
// released afterwards.
func TestLeaseOwnerSignalBeforeFinishKeepsUnregister(t *testing.T) {
	g := newE2EGateway(t)
	g.persistFailures = 3
	alerts := &lockedBuffer{}
	sig := &fakeSignals{}
	owner := ownLeaseSignals(sig.notify, sig.stop)
	sig.send(syscall.SIGTERM)
	if err := owner.finish(ownedLease(t, g, alerts)); err != nil {
		t.Fatalf("finish = %v, alerts %q", err, alerts.String())
	}
	if n := strings.Count(g.calls(), "/v1/unregister"); n != 4 || !sig.stopped {
		t.Fatalf("unregister calls = %d, stopped = %v", n, sig.stopped)
	}
}

// A signal during the unregister retries abandons them, with the
// unknown-state, idle-bound alert; the signals are still released.
func TestLeaseOwnerSignalDuringFinishAbandons(t *testing.T) {
	g := newE2EGateway(t)
	g.persistFailures = -1
	alerts := &lockedBuffer{}
	sig := &fakeSignals{}
	owner := ownLeaseSignals(sig.notify, sig.stop)
	go func() {
		for !strings.Contains(g.calls(), "/v1/unregister") {
			time.Sleep(5 * time.Millisecond)
		}
		sig.send(os.Interrupt)
	}()
	done := make(chan error, 1)
	go func() { done <- owner.finish(ownedLease(t, g, alerts)) }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(alerts.String(), "idle bound") || !sig.stopped {
			t.Fatalf("err = %v alerts = %q stopped = %v", err, alerts.String(), sig.stopped)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a signal during cleanup did not abandon the retries")
	}
}

// A signal before the child starts is visible to the boundary, which
// refuses the launch; without one the boundary proceeds.
func TestLeaseOwnerInterruptedBeforeStart(t *testing.T) {
	sig := &fakeSignals{}
	owner := ownLeaseSignals(sig.notify, sig.stop)
	if owner.interrupted() {
		t.Fatal("interrupted without a signal")
	}
	sig.send(os.Interrupt)
	if !owner.interrupted() {
		t.Fatal("a pending signal is not seen")
	}
	owner.release()
}

// R3-5: a held signal that arrives while a check runs cancels the check's
// context and stays held, so the boundary still sees it; with no signal the
// watch ends when stopped, leaving the context cancelled only by stop and no
// signal held (the control). stop returns only after the watch goroutine
// has exited.
func TestLeaseOwnerCancelOnSignal(t *testing.T) {
	f := &fakeSignals{}
	o := ownLeaseSignals(f.notify, f.stop)
	ctx, stop := o.cancelOnSignal(context.Background())
	f.send(syscall.SIGTERM)
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the signal did not cancel the check")
	}
	stop()
	if !o.interrupted() {
		t.Fatal("the signal was not kept for the boundary")
	}

	ctx, stop = o.cancelOnSignal(context.Background())
	if ctx.Err() != nil {
		t.Fatal("cancelled with no signal")
	}
	stop()
	if o.interrupted() {
		t.Fatal("a signal appeared with none sent")
	}
	o.release()
}
