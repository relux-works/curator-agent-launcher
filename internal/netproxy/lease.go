package netproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// RenewInterval is the contract's keepalive period for an active session.
const RenewInterval = time.Hour

// Lease is one registered launch, owned by the process that waits for the
// runtime. It carries the token for the launcher duties after spawn
// (activate, renew, unregister) and never renders it. All timing uses Go's
// monotonic clock, so a wall-clock change neither ends a window nor
// shortens a retry.
type Lease struct {
	client *Client
	token  string
	window time.Duration
	// registered is the monotonic time of the register reply.
	registered time.Time
	alert      io.Writer

	// Retry is the pause between attempts of activate and unregister.
	Retry time.Duration
	// Renew is the renewal period; RenewInterval unless a test shortens it.
	Renew time.Duration

	mu       sync.Mutex
	stopped  chan struct{}
	done     chan struct{}
	finished bool
	// completion belongs to an explicitly selected recovery-capable caller.
	// It authenticates the original attempt and commits its local evidence;
	// a legacy unregister observation cannot replace its result.
	completion   func(context.Context) error
	completeOnce sync.Once
	completeErr  error
}

// RecoverySelected reports the caller's explicit completion protocol. It does
// not expose the runtime token or grant recovery authority.
func (l *Lease) RecoverySelected() bool { return l.completion != nil }

const RecoveryCompletionBudget = 30 * time.Second

// NewRecoveryLease preserves activation/renewal for a v2-issued runtime token,
// but gives terminal completion to the owning original-attempt journal. The
// callback never receives a token. Missing completion refuses construction;
// callers cannot accidentally get a legacy cleanup fallback.
func NewRecoveryLease(c *Client, reg Registration, alert io.Writer, requested time.Time, complete func(context.Context) error) (*Lease, error) {
	if c == nil || complete == nil {
		return nil, errors.New("netproxy recovery: completion unavailable")
	}
	l := NewLease(c, reg, alert, requested)
	l.completion = complete
	return l, nil
}

// NewLease binds a registration to the client that made it. requested is
// when the register request was started: the service's window began no
// earlier, so the lease's window is never later than the service's. Alerts
// go to alert, one line each, naming the operation and outcome only.
func NewLease(c *Client, reg Registration, alert io.Writer, requested time.Time) *Lease {
	if alert == nil {
		alert = io.Discard
	}
	return &Lease{
		client: c, token: reg.Token, window: time.Duration(reg.ActivationSeconds) * time.Second,
		registered: requested, alert: alert, Retry: 2 * time.Second, Renew: RenewInterval,
	}
}

// String names the lease without its token.
func (l *Lease) String() string { return "netproxy lease" }

// Started runs the post-spawn duties in the background: activate within
// the window (duty 4), then renew every period until Finish (duty 5).
func (l *Lease) Started(ctx context.Context) {
	l.mu.Lock()
	if l.stopped != nil || l.finished {
		l.mu.Unlock()
		return
	}
	l.stopped, l.done = make(chan struct{}), make(chan struct{})
	stopped, done := l.stopped, l.done
	l.mu.Unlock()
	go func() {
		defer close(done)
		if !l.activate(ctx, stopped) {
			// Duty 4: a failed activation ends the token now, not at
			// exit; Finish repeats it, and 404 there counts as done.
			l.end(ctx, stopped)
			return
		}
		l.renew(ctx, stopped)
	}()
}

// activate retries until 200 inside the activation window. A 404 ends at
// once; persistent failure until the window passes ends too. Either way
// the runtime fails closed on its ended token and is never rerouted, and
// Finish unregisters. It reports whether the session became active.
func (l *Lease) activate(ctx context.Context, stopped <-chan struct{}) bool {
	deadline := l.registered.Add(l.window)
	last := "no attempt"
	for {
		// No attempt starts once the window has passed, and every attempt
		// ends with it, so no reply can arrive outside the window.
		if !time.Now().Before(deadline) {
			l.alertf("activate did not succeed within the activation window (%s); requests will fail until the runtime exits", last)
			return false
		}
		actx, cancel := context.WithDeadline(ctx, deadline)
		err := l.client.Activate(actx, l.token)
		cancel()
		if err == nil {
			return true
		}
		if IsCode(err, http.StatusNotFound, "unknown_token") {
			l.alertf("activate refused: the session ended before activation; requests will fail until the runtime exits")
			return false
		}
		last = errClass(err)
		if remaining := time.Until(deadline); remaining > 0 {
			if !l.pause(ctx, stopped, minDuration(l.Retry, remaining)) {
				return false
			}
		}
	}
}

// renew keeps an active session alive. 404 means the session is over and
// is alerted once; any other failure is retried at the next period.
func (l *Lease) renew(ctx context.Context, stopped <-chan struct{}) {
	for l.pause(ctx, stopped, l.Renew) {
		err := l.client.Renew(ctx, l.token)
		switch {
		case err == nil:
		case IsCode(err, http.StatusNotFound, "unknown_token"):
			l.alertf("renew refused: the session has ended; requests will fail until the runtime exits")
			return
		case IsCode(err, http.StatusConflict, "not_active"):
			err := l.client.Activate(ctx, l.token)
			if IsCode(err, http.StatusNotFound, "unknown_token") {
				l.alertf("renew found the session pending and activate was refused; the session has ended")
				l.end(ctx, stopped)
				return
			}
			if err != nil {
				l.alertf("renew found the session pending and activate failed (%v); retrying at the next renewal", errClass(err))
			}
		default:
			l.alertf("renew failed (%v); retrying at the next renewal", errClass(err))
		}
	}
}

// Finish stops the background duties (duty 6). A recovery-capable lease runs
// its owner's completion once with a 30-second budget and retains that exact
// verdict. A legacy lease unregisters until the service answers 204 or 404,
// alerting on each failure, or ctx ends with the token's state unknown.
func (l *Lease) Finish(ctx context.Context) error {
	l.mu.Lock()
	if l.stopped != nil && !l.finished {
		close(l.stopped)
	}
	l.finished = true
	done := l.done
	l.mu.Unlock()
	if done != nil {
		<-done
	}
	if l.completion != nil {
		return l.complete(ctx)
	}
	for attempt := 1; ; attempt++ {
		err := l.client.Unregister(ctx, l.token)
		if err == nil {
			return nil
		}
		l.alertf("unregister attempt %d failed (%v); the session may still be active; retrying", attempt, errClass(err))
		if !l.pause(ctx, nil, l.Retry) {
			l.alertf("unregister abandoned; the session ends at the gateway's idle bound at the latest")
			return errors.New("netproxy unregister abandoned")
		}
	}
}

// complete is shared by background failure and foreground Finish. It must not
// wait for the background duty (which may itself be completing).
func (l *Lease) complete(ctx context.Context) error {
	l.completeOnce.Do(func() {
		bounded, cancel := context.WithTimeout(ctx, RecoveryCompletionBudget)
		defer cancel()
		l.completeErr = l.completion(bounded)
	})
	return l.completeErr
}

// end unregisters in the background until it answers or Finish takes over.
func (l *Lease) end(ctx context.Context, stopped <-chan struct{}) {
	if l.completion != nil {
		_ = l.complete(ctx)
		return
	}
	for {
		err := l.client.Unregister(ctx, l.token)
		if err == nil {
			return
		}
		l.alertf("unregister after a failed activation failed (%v); retrying", errClass(err))
		if !l.pause(ctx, stopped, l.Retry) {
			return
		}
	}
}

func (l *Lease) pause(ctx context.Context, stopped <-chan struct{}, d time.Duration) bool {
	if d <= 0 {
		d = time.Millisecond
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-stopped:
		return false
	case <-ctx.Done():
		return false
	}
}

func (l *Lease) alertf(format string, args ...any) {
	fmt.Fprintf(l.alert, "curator-run: network gateway: "+format+"\n", args...)
}

// errClass renders an error's class and code without any request content.
func errClass(err error) string {
	var e *Error
	if !errors.As(err, &e) {
		return "error"
	}
	if e.Kind == Refused {
		return fmt.Sprintf("status %d %s", e.Status, e.Code)
	}
	return "service unreachable"
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
