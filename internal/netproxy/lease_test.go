package netproxy

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// script answers each control path from a queue, then from a default.
type script struct {
	mu       sync.Mutex
	queue    map[string][]reply
	fallback map[string]reply
	count    map[string]int
}

type reply struct {
	status int
	body   string
	hang   bool
}

var (
	okActive   = reply{200, `{"api":"netproxy-control/1","active":true}`, false}
	noContent  = reply{204, "", false}
	gone       = reply{404, `{"api":"netproxy-control/1","error":"unknown_token"}`, false}
	persist    = reply{503, `{"api":"netproxy-control/1","error":"persist_failed"}`, false}
	notActive  = reply{409, `{"api":"netproxy-control/1","error":"not_active"}`, false}
	noAnswer   = reply{hang: true}
	leaseReg   = Registration{Token: testToken, TokenHeader: "X-Netproxy-Token", ActivationSeconds: 120}
	shortRetry = 10 * time.Millisecond
)

func newScript() *script {
	return &script{queue: map[string][]reply{}, count: map[string]int{},
		fallback: map[string]reply{"/v1/activate": okActive, "/v1/renew": okActive, "/v1/unregister": noContent}}
}

func (s *script) then(path string, r ...reply) *script {
	s.queue[path] = append(s.queue[path], r...)
	return s
}

func (s *script) calls(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count[path]
}

func (s *script) serve(w http.ResponseWriter, r *http.Request, _ []byte) {
	s.mu.Lock()
	s.count[r.URL.Path]++
	rep := s.fallback[r.URL.Path]
	if q := s.queue[r.URL.Path]; len(q) > 0 {
		rep, s.queue[r.URL.Path] = q[0], q[1:]
	}
	s.mu.Unlock()
	if rep.hang {
		<-r.Context().Done()
		return
	}
	w.Header().Set(VersionHeader, APIVersion)
	w.WriteHeader(rep.status)
	io.WriteString(w, rep.body)
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func newLease(t *testing.T, s *script, reg Registration) (*Lease, *syncBuffer) {
	t.Helper()
	f := newFake(t, s.serve)
	c := New(f.socket)
	c.Timeout = 100 * time.Millisecond
	alerts := &syncBuffer{}
	l := NewLease(c, reg, alerts)
	l.Retry = shortRetry
	return l, alerts
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Positive control (conformance 4, 6, 19): after start the lease activates
// once, renews every period, and unregisters at Finish with no alert.
func TestLeaseLifecycle(t *testing.T) {
	s := newScript()
	l, alerts := newLease(t, s, leaseReg)
	l.Renew = 20 * time.Millisecond
	l.Started(context.Background())
	waitFor(t, func() bool { return s.calls("/v1/renew") >= 2 })
	if err := l.Finish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.calls("/v1/activate") != 1 || s.calls("/v1/unregister") != 1 || alerts.String() != "" {
		t.Fatalf("activate=%d unregister=%d alerts=%q", s.calls("/v1/activate"), s.calls("/v1/unregister"), alerts.String())
	}
}

// Conformance 20, duty 4: a lost or failed activate reply has an unknown
// outcome and is retried inside the window until 200.
func TestLeaseActivateRetriesLostReplies(t *testing.T) {
	s := newScript().then("/v1/activate", noAnswer, persist)
	l, alerts := newLease(t, s, leaseReg)
	l.Started(context.Background())
	waitFor(t, func() bool { return s.calls("/v1/activate") >= 3 })
	if err := l.Finish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.calls("/v1/activate") != 3 || alerts.String() != "" {
		t.Fatalf("activate=%d alerts=%q", s.calls("/v1/activate"), alerts.String())
	}
}

// Duty 4: activate 404 ends the token at once (unregister before Finish)
// and alerts; the runtime is never rerouted.
func TestLeaseActivateRefusedEndsTokenNow(t *testing.T) {
	s := newScript().then("/v1/activate", gone)
	l, alerts := newLease(t, s, leaseReg)
	l.Started(context.Background())
	waitFor(t, func() bool { return s.calls("/v1/unregister") >= 1 })
	if !strings.Contains(alerts.String(), "activate refused") {
		t.Fatalf("alerts = %q", alerts.String())
	}
	if err := l.Finish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.calls("/v1/activate") != 1 || s.calls("/v1/renew") != 0 {
		t.Fatalf("activate=%d renew=%d", s.calls("/v1/activate"), s.calls("/v1/renew"))
	}
}

// Duty 4: when activate keeps failing until the window ends, the lease
// alerts and unregisters without waiting for the runtime to exit.
func TestLeaseActivateWindowExpiry(t *testing.T) {
	s := newScript()
	s.fallback["/v1/activate"] = persist
	reg := leaseReg
	reg.ActivationSeconds = 1
	l, alerts := newLease(t, s, reg)
	l.Retry = 100 * time.Millisecond
	start := time.Now()
	l.Started(context.Background())
	waitFor(t, func() bool { return s.calls("/v1/unregister") >= 1 })
	if took := time.Since(start); took < 900*time.Millisecond || took > 2500*time.Millisecond {
		t.Fatalf("ended after %v, want about the 1 s window", took)
	}
	if !strings.Contains(alerts.String(), "within the activation window") {
		t.Fatalf("alerts = %q", alerts.String())
	}
	if err := l.Finish(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// Duty 5: renew 409 activates first; renew 404 alerts once and stops
// renewing; other renew failures are retried at the next period.
func TestLeaseRenewOutcomes(t *testing.T) {
	s := newScript().then("/v1/renew", notActive, persist, gone)
	l, alerts := newLease(t, s, leaseReg)
	l.Renew = 15 * time.Millisecond
	l.Started(context.Background())
	waitFor(t, func() bool { return strings.Contains(alerts.String(), "session has ended") })
	time.Sleep(60 * time.Millisecond)
	if err := l.Finish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.calls("/v1/renew") != 3 || s.calls("/v1/activate") != 2 {
		t.Fatalf("renew=%d activate=%d", s.calls("/v1/renew"), s.calls("/v1/activate"))
	}
	if strings.Count(alerts.String(), "session has ended") != 1 || !strings.Contains(alerts.String(), "renew failed") {
		t.Fatalf("alerts = %q", alerts.String())
	}
}

// Duty 6, conformance 21: unregister retries through 503 persist_failed and
// lost replies until 404, alerting on each failure, and only then returns.
func TestLeaseFinishRetriesUntilDone(t *testing.T) {
	s := newScript().then("/v1/unregister", persist, noAnswer, gone)
	l, alerts := newLease(t, s, leaseReg)
	if err := l.Finish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.calls("/v1/unregister") != 3 || strings.Count(alerts.String(), "unregister attempt") != 2 {
		t.Fatalf("unregister=%d alerts=%q", s.calls("/v1/unregister"), alerts.String())
	}
}

// An interrupted Finish (the user stops curator-run) reports the unknown
// token state instead of claiming the session ended.
func TestLeaseFinishAbandoned(t *testing.T) {
	s := newScript()
	s.fallback["/v1/unregister"] = persist
	l, alerts := newLease(t, s, leaseReg)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if err := l.Finish(ctx); err == nil {
		t.Fatal("abandoned unregister reported success")
	}
	if !strings.Contains(alerts.String(), "idle bound") {
		t.Fatalf("alerts = %q", alerts.String())
	}
}

// Conformance 23: no alert or String form renders the token.
func TestLeaseNeverRendersToken(t *testing.T) {
	s := newScript().then("/v1/activate", gone).then("/v1/unregister", persist, gone)
	l, alerts := newLease(t, s, leaseReg)
	l.Started(context.Background())
	waitFor(t, func() bool { return s.calls("/v1/unregister") >= 1 })
	if err := l.Finish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if alerts.String() == "" || strings.Contains(alerts.String(), testToken) || strings.Contains(l.String(), testToken) {
		t.Fatalf("alerts = %q", alerts.String())
	}
}
