//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/netproxy"
	"github.com/relux-works/curator-agent-launcher/internal/recoveryjournal"
	"github.com/relux-works/curator-network-profiles/pkg/netprofile"
)

// The explicit service artifact runs only its supported disposable pilot CLI.
// No installed data, native home, ambient identity or real provider is used.
type dockingService struct {
	bin, data, socket, upstream string
	cmd                         *exec.Cmd
	done                        chan error
}

func dockingTemp(t *testing.T) string {
	t.Helper()
	// Select a short fixture root up front for the documented Unix path bound.
	p, err := os.MkdirTemp("/tmp", "npdock-")
	if err != nil {
		t.Fatal("disposable directory unavailable")
	}
	p, err = filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal("disposable directory not canonical")
	}
	t.Cleanup(func() { _ = os.RemoveAll(p) })
	return p
}

func newDockingService(t *testing.T, bin, upstream string) *dockingService {
	t.Helper()
	s := &dockingService{bin: bin, data: filepath.Join(dockingTemp(t), "d"), upstream: upstream}
	s.socket = filepath.Join(s.data, "control.sock")
	if len(s.socket) > 103 || os.Mkdir(s.data, 0700) != nil {
		t.Fatal("invalid disposable service directory")
	}
	t.Cleanup(func() { s.stop(t) })
	s.start(t, false)
	s.stop(t)
	key, err := os.Lstat(filepath.Join(s.data, "pilot.kek"))
	if err != nil || !key.Mode().IsRegular() || key.Mode().Perm() != 0600 || key.Size() != 32 {
		t.Fatal("disposable bootstrap key metadata invalid")
	}
	// The service made its own synthetic key; this caller never opens it.
	cmd := exec.Command(s.bin, "initialize-member-recovery", "-data", s.data, "-key-source", "pilot-file")
	s.command(t, cmd, "initialize")
	s.start(t, true)
	return s
}

func dockingEnvironment(t *testing.T) []string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal("actual HOME unavailable")
	}
	return []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
}

func (s *dockingService) command(t *testing.T, cmd *exec.Cmd, phase string) {
	t.Helper()
	cmd.Env, cmd.Stdout, cmd.Stderr = dockingEnvironment(t), io.Discard, io.Discard
	if cmd.Start() != nil {
		t.Fatal("disposable command did not start")
	}
	t.Logf("owned fixture pid=%d phase=%s", cmd.Process.Pid, phase)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("disposable command refused")
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatal("disposable command exceeded its bound")
	}
}

func (s *dockingService) start(t *testing.T, recovery bool) {
	t.Helper()
	if s.cmd != nil {
		t.Fatal("duplicate fixture service start")
	}
	args := []string{"-data", s.data, "-key-source", "pilot-file", "-listen", "127.0.0.1:0"}
	if recovery {
		args = append(args, "-member-recovery", "require-existing")
	}
	for _, name := range []string{"anthropic", "openai", "chatgpt"} {
		args = append(args, "-upstream", name+"="+s.upstream)
	}
	cmd := exec.Command(s.bin, args...)
	cmd.Env, cmd.Stdout, cmd.Stderr = dockingEnvironment(t), io.Discard, io.Discard
	if cmd.Start() != nil {
		t.Fatal("disposable service did not start")
	}
	s.cmd, s.done = cmd, make(chan error, 1)
	go func(done chan error) { done <- cmd.Wait() }(s.done)
	t.Logf("owned fixture pid=%d phase=serve recovery=%v", cmd.Process.Pid, recovery)
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		var err error
		if recovery {
			c, failure := netproxy.NewRecovery(s.socket)
			err = failure
			if err == nil {
				_, err = c.Health(ctx)
			}
		} else {
			err = netproxy.New(s.socket).Health(ctx)
		}
		cancel()
		if err == nil {
			return
		}
		select {
		case <-s.done:
			s.cmd = nil
			t.Fatal("disposable service exited before readiness")
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("disposable service readiness not observed")
}

func (s *dockingService) stop(t *testing.T) {
	t.Helper()
	if s.cmd == nil {
		return
	}
	cmd, done := s.cmd, s.done
	s.cmd = nil
	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case err := <-done:
		t.Logf("owned fixture pid=%d phase=reaped", cmd.Process.Pid)
		if err != nil {
			t.Error("disposable service normal shutdown failed")
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Error("disposable service normal shutdown exceeded bound")
	}
}

// This in-memory boundary forwards to the real service, verifies prepare
// ordering, and can discard one real reply. It never replaces service verdicts.
type dockingObserver struct {
	mu                         sync.Mutex
	socket, backend, directory string
	paths                      []string
	envelope                   recoveryjournal.Envelope
	token                      string
	dropRegister               bool
	afterRecover               func()
}

func newDockingObserver(t *testing.T, backend, directory string) *dockingObserver {
	t.Helper()
	g := &dockingObserver{socket: filepath.Join(dockingTemp(t), "s"), backend: backend, directory: directory}
	ln, err := net.Listen("unix", g.socket)
	if err != nil {
		t.Fatal("fixture observation socket unavailable")
	}
	tr := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		g.mu.Lock()
		socket := g.backend
		g.mu.Unlock()
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	server := &http.Server{ErrorLog: log.New(io.Discard, "", 0), Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.paths = append(g.paths, r.URL.Path)
		g.mu.Unlock()
		if r.URL.Path == "/v2/register" {
			raw, err := io.ReadAll(io.LimitReader(r.Body, netproxy.RecoveryMaxBody+1))
			_ = r.Body.Close()
			var e recoveryjournal.Envelope
			if err != nil || json.Unmarshal(raw, &e) != nil || e.Validate() != nil {
				t.Error("DOCK WIRE ALARM: invalid original envelope")
				w.WriteHeader(400)
				return
			}
			private, err := os.ReadFile(filepath.Join(g.directory, "secrets", e.AttemptID+".json"))
			var saved struct {
				Envelope recoveryjournal.Envelope `json:"envelope"`
			}
			if err != nil || json.Unmarshal(private, &saved) != nil || saved.Envelope != e {
				t.Error("DOCK PREPARE ALARM: real request preceded durable original entry")
				w.WriteHeader(400)
				return
			}
			g.mu.Lock()
			g.envelope = e
			g.mu.Unlock()
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
		request := r.Clone(r.Context())
		request.URL, request.RequestURI = &url.URL{Scheme: "http", Host: "control", Path: r.URL.Path}, ""
		response, err := client.Do(request)
		if err != nil {
			w.WriteHeader(502)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, netproxy.RecoveryMaxBody+1))
		closeErr := response.Body.Close()
		if err != nil || closeErr != nil || len(raw) > netproxy.RecoveryMaxBody {
			w.WriteHeader(502)
			return
		}
		g.mu.Lock()
		if strings.HasSuffix(r.URL.Path, "/register") && response.StatusCode == 200 {
			var values map[string]json.RawMessage
			if json.Unmarshal(raw, &values) == nil {
				_ = json.Unmarshal(values["token"], &g.token)
			}
		}
		drop := r.URL.Path == "/v2/register" && response.StatusCode == 200 && g.dropRegister
		if drop {
			g.dropRegister = false
		}
		after := g.afterRecover
		g.mu.Unlock()
		if drop {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		if r.URL.Path == "/v2/recover" && response.StatusCode == 200 && after != nil {
			after()
		}
		for key, values := range response.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(raw)
	})}
	go func() { _ = server.Serve(ln) }()
	t.Cleanup(func() { _ = server.Close(); tr.CloseIdleConnections() })
	return g
}

func (g *dockingObserver) calls() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return strings.Join(g.paths, " ")
}
func (g *dockingObserver) reset(backend string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.paths, g.backend = nil, backend
}

type dockingCaller struct {
	f *pipelineFixture
	g *dockingObserver
}

func newDockingCaller(t *testing.T, backend string) *dockingCaller {
	t.Helper()
	f := printNetworkFixture(t)
	state := filepath.Join(f.dir, "state")
	directory := filepath.Join(state, "swarma", "launcher", "netproxy-recovery", "v1")
	for _, part := range []string{"secrets", "receipts"} {
		if os.MkdirAll(filepath.Join(directory, part), 0700) != nil {
			t.Fatal("caller fixture layout unavailable")
		}
	}
	g := newDockingObserver(t, backend, directory)
	writeGatewayCatalog(t, f, g.socket)
	parent := f.deps.environ
	f.deps.environ = func() []string { return append(append([]string(nil), parent()...), "XDG_STATE_HOME="+state) }
	f.args = append([]string{"--gateway-recovery"}, withNetworkFlag(f.args, "egress-a")...)
	return &dockingCaller{f, g}
}

func (d *dockingCaller) recover(t *testing.T) (int, string, string) {
	t.Helper()
	deps := forbiddenGatewayDeps(t)
	deps.environ = d.f.deps.environ
	deps.gatewayRecover = nil // Use production discovery, journal and Unix transport.
	var out, diagnostic strings.Builder
	code := run(context.Background(), []string{"gateway", "recover", "--attempt", d.g.envelope.AttemptID}, &out, &diagnostic, deps)
	return code, out.String(), diagnostic.String()
}

func (d *dockingCaller) private(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(d.g.directory, "secrets", d.g.envelope.AttemptID+".json"))
	var saved struct {
		Envelope recoveryjournal.Envelope `json:"envelope"`
	}
	if err != nil || json.Unmarshal(raw, &saved) != nil || saved.Envelope != d.g.envelope {
		t.Fatal("DOCK ORIGINAL ALARM: original E was lost or replaced")
	}
	if d.g.token != "" && bytes.Contains(raw, []byte(d.g.token)) {
		t.Fatal("DOCK PRIVACY ALARM: runtime token persisted in journal")
	}
	return raw
}

func (d *dockingCaller) receipt(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(d.g.directory, "receipts", d.g.envelope.AttemptID+".json"))
	if err != nil || !bytes.Contains(raw, []byte(`"service_result":"revoked"`)) {
		t.Fatal("DOCK RECEIPT ALARM: durable result missing")
	}
	for _, private := range []string{d.g.envelope.RecoveryProof, d.g.envelope.Root, d.g.token} {
		if private != "" && bytes.Contains(raw, []byte(private)) {
			t.Fatal("DOCK PRIVACY ALARM: public receipt contains private data")
		}
	}
	return raw
}

func (d *dockingCaller) issue(t *testing.T) netproxy.RecoveryRegistration {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := netproxy.NewRecovery(d.g.socket)
	if err != nil {
		t.Fatal("fixture control client unavailable")
	}
	h, err := c.Health(ctx)
	if err != nil {
		t.Fatal("real service health refused")
	}
	e, err := recoveryjournal.NewEnvelope(recoveryjournal.IssueFacts{Root: d.f.dir, Session: "fixture-restart", Runtime: "claude-code", RegistryID: h.RegistryID, Boot: h.Boot, NowNS: h.NowNS})
	if err != nil {
		t.Fatal("fixture envelope invalid")
	}
	p, err := netprofile.Normalize("egress-a", netprofile.Input{Kind: netprofile.KindFilteringGateway, Control: d.g.socket, SensitiveEgress: true})
	if err != nil {
		t.Fatal("fixture profile invalid")
	}
	b, err := recoveryjournal.ResolveCaller(d.f.deps.environ(), netprofile.Digest(p), d.g.socket)
	if err != nil {
		t.Fatal("fixture original binding invalid")
	}
	n, err := recoveryjournal.OpenExisting(b)
	if err != nil {
		t.Fatal("fixture original layout invalid")
	}
	reg, issueErr := n.Issue(ctx, e, b)
	if closeErr := n.Close(); issueErr != nil || closeErr != nil {
		t.Fatal("DOCK ISSUANCE ALARM: real service rejected durable caller issue")
	}
	if err := netproxy.New(d.g.socket).Activate(ctx, reg.Registration.Token); err != nil {
		t.Fatal("real service activation refused")
	}
	return reg
}

// Explicit artifact qualification docks the normal caller and exact recovery
// command against the actual service CLI. Native executables remain fixtures;
// zero upstream traffic proves no inference or real-member adoption here.
func TestGatewayRecoveryServiceDocking(t *testing.T) {
	bin, digest := os.Getenv("CURATOR_TEST_RECOVERY_SERVICE"), os.Getenv("CURATOR_TEST_RECOVERY_SERVICE_SHA256")
	if bin == "" && digest == "" {
		t.Skip("explicit real service artifact required")
	}
	if !filepath.IsAbs(bin) || len(digest) != 64 {
		t.Fatal("invalid explicit service artifact")
	}
	raw, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal("selected service artifact unavailable")
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != digest {
		t.Fatal("selected service artifact bytes differ")
	}
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { upstreamCalls.Add(1); _, _ = io.WriteString(w, `{}`) }))
	t.Cleanup(upstream.Close)
	s := newDockingService(t, bin, upstream.URL)
	t.Run("normal", func(t *testing.T) {
		d := newDockingCaller(t, s.socket)
		code, out, diagnostic := d.f.run()
		if code != 0 || !strings.Contains(string(diagnostic), "receipt=committed") {
			t.Fatal("DOCK WIRE ALARM: normal caller completion refused")
		}
		if d.g.calls() != "/v2/health /v2/register /v1/activate /v2/health /v2/recover" {
			t.Fatal("DOCK WIRE ALARM: normal caller selected wrong service operations")
		}
		var child childCapture
		if json.Unmarshal(out, &child) != nil {
			t.Fatal("native fixture did not run")
		}
		if !strings.Contains(strings.Join(child.Env, " "), "X-Netproxy-Token: "+d.g.token) || d.g.token == "" {
			t.Fatal("admitted child token missing")
		}
		if bytes.Contains(out, []byte(d.g.envelope.RecoveryProof)) || strings.Contains(strings.Join(child.Argv, " "), d.g.token) || bytes.Contains(diagnostic, []byte(d.g.token)) || bytes.Contains(diagnostic, []byte(d.g.envelope.RecoveryProof)) {
			t.Fatal("DOCK PRIVACY ALARM: capability escaped allowed child placement")
		}
		d.private(t)
		receipt := d.receipt(t)
		for i := 0; i < 2; i++ {
			if code, out, diagnostic := d.recover(t); code != 0 || !strings.Contains(out, "receipt=committed") || diagnostic != "" {
				t.Fatal("exact repeated real-service recovery refused")
			}
		}
		if !bytes.Equal(receipt, d.receipt(t)) || strings.Count(d.g.calls(), "/v2/register") != 1 {
			t.Fatal("DOCK REPLAY ALARM: recovery issued again or rewrote immutable receipt")
		}
	})
	t.Run("lost-register-reply", func(t *testing.T) {
		d := newDockingCaller(t, s.socket)
		d.g.dropRegister = true
		code, out, diagnostic := d.f.run()
		if code != 1 || len(out) != 0 || !strings.Contains(string(diagnostic), "receipt=committed") {
			t.Fatal("lost real registration reply did not refuse and reconcile")
		}
		d.f.assertNoChild(t)
		d.private(t)
		d.receipt(t)
		if strings.Count(d.g.calls(), "/v2/register") != 1 || strings.Contains(d.g.calls(), "/v1/") {
			t.Fatal("DOCK REPLAY ALARM: lost reply repeated issuance or downgraded")
		}
		if code, _, _ := d.recover(t); code != 0 || strings.Count(d.g.calls(), "/v2/register") != 1 {
			t.Fatal("lost-reply exact retry could not recover without issuance")
		}
	})
	t.Run("same-authority-restart", func(t *testing.T) {
		d := newDockingCaller(t, s.socket)
		reg := d.issue(t)
		original := d.g.envelope
		s.stop(t)
		s.start(t, true)
		d.g.reset(s.socket)
		if code, _, _ := d.recover(t); code != 0 {
			t.Fatal("original active journal attempt refused after same-store restart")
		}
		if d.g.calls() != "/v2/health /v2/recover" || original != d.g.envelope {
			t.Fatal("restart replaced original E or issued a new attempt")
		}
		d.private(t)
		d.receipt(t)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := netproxy.New(d.g.socket).Renew(ctx, reg.Registration.Token); !netproxy.IsCode(err, 404, "unknown_token") {
			t.Fatal("recovered runtime token remained usable")
		}
	})
	t.Run("foreign-registry", func(t *testing.T) {
		d := newDockingCaller(t, s.socket)
		d.issue(t)
		before := d.private(t)
		foreign := newDockingService(t, bin, upstream.URL)
		d.g.reset(foreign.socket)
		if code, out, _ := d.recover(t); code != 1 || out != "" || d.g.calls() != "/v2/health" {
			t.Fatal("DOCK REGISTRY ALARM: foreign authority reached recover or success")
		}
		if !bytes.Equal(before, d.private(t)) {
			t.Fatal("foreign authority rewrote original private evidence")
		}
		if _, err := os.Lstat(filepath.Join(d.g.directory, "receipts", d.g.envelope.AttemptID+".json")); !os.IsNotExist(err) {
			t.Fatal("foreign authority created a receipt")
		}
		// The nearby original-authority control remains usable.
		d.g.reset(s.socket)
		if code, _, _ := d.recover(t); code != 0 {
			t.Fatal("original authority recovery refused after foreign negative")
		}
	})
	t.Run("missing-layout", func(t *testing.T) {
		d := newDockingCaller(t, s.socket)
		missing := filepath.Join(d.g.directory, "receipts")
		if os.Remove(missing) != nil {
			t.Fatal("missing-layout fixture unavailable")
		}
		code, out, diagnostic := d.f.run()
		if code != 1 || len(out) != 0 || d.g.calls() != "" || !strings.Contains(string(diagnostic), "proposed_directories=[STATE/swarma/launcher/netproxy-recovery/v1/receipts]") {
			t.Fatal("DOCK PREFLIGHT ALARM: missing layout reached service or lost exact proposal")
		}
		d.f.assertNoChild(t)
		if _, err := os.Lstat(missing); !os.IsNotExist(err) {
			t.Fatal("missing caller directory was repaired")
		}
	})
	t.Run("legacy-absent-flag", func(t *testing.T) {
		d := newDockingCaller(t, s.socket)
		d.f.args = d.f.args[1:]
		code, _, _ := d.f.run()
		if code != 0 || d.g.calls() != "/v1/health /v1/register /v1/activate /v1/unregister" {
			t.Fatal("legacy caller changed its real-service wire")
		}
	})
	t.Run("local-evidence-failure", func(t *testing.T) {
		d := newDockingCaller(t, s.socket)
		d.issue(t)
		d.g.afterRecover = func() {
			if os.Chmod(filepath.Join(d.g.directory, "receipts"), 0755) != nil {
				t.Error("receipt fault fixture unavailable")
			}
		}
		if code, out, diagnostic := d.recover(t); code != 1 || out != "" || !strings.Contains(diagnostic, "local_evidence_failed") || !strings.Contains(diagnostic, "service confirmed revoked") {
			t.Fatal("DOCK EVIDENCE ALARM: service confirmation became false local success")
		}
		d.private(t)
		d.g.afterRecover = nil
		if os.Chmod(filepath.Join(d.g.directory, "receipts"), 0700) != nil {
			t.Fatal("receipt fixture not restored")
		}
		if code, _, _ := d.recover(t); code != 0 || strings.Count(d.g.calls(), "/v2/register") != 1 {
			t.Fatal("exact evidence retry failed or registered again")
		}
		d.receipt(t)
	})
	if upstreamCalls.Load() != 0 {
		t.Fatal("fixture unexpectedly forwarded real runtime traffic")
	}
	final, err := os.ReadFile(bin)
	if err != nil || !bytes.Equal(raw, final) {
		t.Fatal("selected service artifact changed during docking")
	}
}
