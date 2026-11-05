package netproxy

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A selected recoverable lease keeps existing activation/renewal, stops those
// duties before exact completion, and does not substitute v1 unregister.
func TestRecoveryLeaseLifecycle(t *testing.T) {
	s := newScript()
	f := newFake(t, s.serve)
	var lease *Lease
	var calls int
	var err error
	lease, err = NewRecoveryLease(New(f.socket), leaseReg, io.Discard, time.Now(), func(ctx context.Context) error {
		calls++
		select {
		case <-lease.done:
		default:
			t.Error("RECOVERY LEASE ORDER ALARM: completion before duties stopped")
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > RecoveryCompletionBudget {
			t.Error("recovery completion is not bounded")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	lease.Renew = 5 * time.Millisecond
	lease.Started(context.Background())
	waitFor(t, func() bool { return s.calls("/v1/renew") > 0 })
	if err := lease.Finish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || s.calls("/v1/activate") != 1 || s.calls("/v1/unregister") != 0 {
		t.Fatal("RECOVERY LEASE DISPATCH ALARM: exact completion bypassed or legacy fallback used")
	}
}

// Both success and a local/service failure are terminal observations for this
// live caller. Concurrent/repeated Finish shares one callback and its exact
// verdict; it never claims an unregister as a recovery success or reissues.
func TestRecoveryLeaseTerminalOnce(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirmed", true: "unconfirmed"}[fail], func(t *testing.T) {
			s := newScript()
			f := newFake(t, s.serve)
			want := error(nil)
			if fail {
				want = errors.New("fixed unconfirmed result")
			}
			var calls atomic.Int32
			entered, release := make(chan struct{}), make(chan struct{})
			lease, err := NewRecoveryLease(New(f.socket), leaseReg, io.Discard, time.Now(), func(context.Context) error {
				calls.Add(1)
				close(entered)
				<-release
				return want
			})
			if err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if got := lease.Finish(context.Background()); got != want {
						t.Error("RECOVERY LEASE VERDICT ALARM: exact completion error lost")
					}
				}()
			}
			<-entered
			close(release)
			wg.Wait()
			lease.Started(context.Background())
			if calls.Load() != 1 || s.calls("/v1/unregister") != 0 || s.calls("/v1/activate") != 0 {
				t.Fatal("RECOVERY LEASE ONCE ALARM: duplicate completion, restart or legacy fallback")
			}
		})
	}
}

// An interrupted owner passes cancellation to recovery and receives its
// failure. A missing completion cannot construct a nominal v2 lease.
func TestRecoveryLeaseCancellationAndMissingOwner(t *testing.T) {
	s := newScript()
	f := newFake(t, s.serve)
	client := New(f.socket)
	for _, invalid := range []bool{false, true} {
		c, complete := client, func(context.Context) error { return nil }
		if invalid {
			c = nil
		} else {
			complete = nil
		}
		if l, err := NewRecoveryLease(c, leaseReg, io.Discard, time.Now(), complete); err == nil || l != nil {
			t.Fatal("missing recovery owner admitted")
		}
	}
	lease, err := NewRecoveryLease(client, leaseReg, io.Discard, time.Now(), func(ctx context.Context) error { return ctx.Err() })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(lease.Finish(ctx), context.Canceled) || s.calls("/v1/unregister") != 0 {
		t.Fatal("cancelled recovery became legacy cleanup success")
	}
}

// Background activation failure must revoke the original attempt too. Finish
// shares its result, even if failure completes before the child exits.
func TestRecoveryLeaseActivationFailureNeverUnregisters(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirmed", true: "unconfirmed"}[failed], func(t *testing.T) {
			s := newScript()
			s.then("/v1/activate", gone)
			f := newFake(t, s.serve)
			var calls atomic.Int32
			want := error(nil)
			if failed {
				want = errors.New("completion unconfirmed")
			}
			completed := make(chan struct{})
			lease, err := NewRecoveryLease(New(f.socket), leaseReg, io.Discard, time.Now(), func(ctx context.Context) error {
				calls.Add(1)
				close(completed)
				return want
			})
			if err != nil {
				t.Fatal(err)
			}
			lease.Started(context.Background())
			select {
			case <-completed:
			case <-time.After(time.Second):
				t.Fatal("background recovery did not complete")
			}
			if got := lease.Finish(context.Background()); got != want {
				t.Fatal("background verdict lost")
			}
			if calls.Load() != 1 || s.calls("/v1/unregister") != 0 {
				t.Fatal("activation failure used legacy cleanup or completed twice")
			}
		})
	}
}
