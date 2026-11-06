//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/netproxy"
	"github.com/relux-works/curator-agent-launcher/internal/recoveryjournal"
	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
)

// This isolated service asserts the independent contract's original-E and
// private-prepare ordering at the actual normal-launch HTTP boundary.
type launchRecoveryGateway struct {
	mu                          sync.Mutex
	paths                       []string
	envelope                    recoveryjournal.Envelope
	directory                   string
	registerFault, recoverFault string
	healthFault                 string
	socket                      string
	restart                     func()
	ended                       bool
	healthNotReady              bool
	foreign                     bool
	onRegister                  func()
}

func recoveryLaunchFixture(t *testing.T, fixtures ...*pipelineFixture) (*pipelineFixture, *launchRecoveryGateway) {
	t.Helper()
	var f *pipelineFixture
	if len(fixtures) == 0 {
		f = printNetworkFixture(t)
	} else {
		f = fixtures[0]
	}
	dir, err := os.MkdirTemp("", "r2gw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "s")
	writeGatewayCatalog(t, f, socket)
	state := filepath.Join(f.dir, "state")
	g := &launchRecoveryGateway{socket: socket, directory: filepath.Join(state, "swarma", "launcher", "netproxy-recovery", "v1")}
	for _, p := range []string{filepath.Join(g.directory, "secrets"), filepath.Join(g.directory, "receipts")} {
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	parent := f.deps.environ
	f.deps.environ = func() []string {
		env := append([]string(nil), parent()...)
		for _, e := range env {
			if strings.HasPrefix(e, "XDG_STATE_HOME=") {
				t.Fatal("fixture already selected state")
			}
		}
		return append(env, "XDG_STATE_HOME="+state)
	}
	f.args = append([]string{"--gateway-recovery"}, withNetworkFlag(f.args, "egress-a")...)
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.paths = append(g.paths, r.URL.Path)
		w.Header().Set(netproxy.VersionHeader, netproxy.RecoveryAPI)
		switch r.URL.Path {
		case "/v2/health":
			if g.healthFault != "" {
				if g.healthFault == "legacy" {
					w.Header().Set(netproxy.VersionHeader, netproxy.APIVersion)
				}
				_, _ = io.WriteString(w, `{"api":"netproxy-control/2","ready":true}`)
				return
			}
			registry := strings.Repeat("c", 64)
			if g.foreign {
				registry = strings.Repeat("e", 64)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"api": netproxy.RecoveryAPI, "ready": !g.healthNotReady, "rule_set": "fixture", "key_mode": "fixture", "registry_id": registry, "boot": "fixture-boot", "now_ns": "1000", "recovery": netproxy.RecoveryProtocol})
		case "/v2/register":
			var e recoveryjournal.Envelope
			if json.NewDecoder(r.Body).Decode(&e) != nil || e.Validate() != nil {
				t.Error("invalid issuance envelope")
				w.WriteHeader(400)
				return
			}
			raw, err := os.ReadFile(filepath.Join(g.directory, "secrets", e.AttemptID+".json"))
			var saved struct {
				Envelope recoveryjournal.Envelope `json:"envelope"`
			}
			if err != nil || json.Unmarshal(raw, &saved) != nil || saved.Envelope != e {
				t.Error("PREPARE ORDER ALARM: issuance before original private record")
				w.WriteHeader(400)
				return
			}
			if g.envelope.AttemptID != "" {
				t.Error("DUPLICATE ISSUE ALARM: initial register replayed")
				w.WriteHeader(400)
				return
			}
			g.envelope = e
			if g.onRegister != nil {
				g.onRegister()
			}
			switch g.registerFault {
			case "lost":
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					_ = conn.Close()
				}
				return
			case "malformed":
				_, _ = io.WriteString(w, `{"api":"netproxy-control/2"}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"api": netproxy.RecoveryAPI, "attempt_id": e.AttemptID, "launch_id": strings.Repeat("d", 64), "token": e2eToken, "token_header": "X-Netproxy-Token", "anthropic_base": "http://127.0.0.1:47811/anthropic", "openai_base": "http://127.0.0.1:47811/openai/v1", "chatgpt_base": "http://127.0.0.1:47811/chatgpt/backend-api/codex", "activation_seconds": 120})
		case "/v2/recover":
			var e recoveryjournal.Envelope
			if json.NewDecoder(r.Body).Decode(&e) != nil || e != g.envelope {
				t.Error("EXACT RECOVERY ALARM: original envelope substituted")
				w.WriteHeader(400)
				return
			}
			if g.recoverFault == "malformed" {
				_, _ = io.WriteString(w, `{"api":"netproxy-control/2"}`)
				return
			}
			g.ended = true
			_ = json.NewEncoder(w).Encode(map[string]any{"api": netproxy.RecoveryAPI, "attempt_id": e.AttemptID, "launch_id": strings.Repeat("d", 64), "outcome": "revoked", "durable": true})
		case "/v1/activate", "/v1/renew":
			w.Header().Set(netproxy.VersionHeader, netproxy.APIVersion)
			if g.ended {
				w.WriteHeader(404)
				_, _ = io.WriteString(w, `{"api":"netproxy-control/1","error":"unknown_token"}`)
				return
			}
			_, _ = io.WriteString(w, `{"api":"netproxy-control/1","active":true}`)
		default:
			t.Error("DOWNGRADE ALARM: unexpected service operation")
			w.WriteHeader(400)
		}
	})}
	go srv.Serve(ln)
	g.restart = func() {
		handler := srv.Handler
		if err := srv.Close(); err != nil {
			t.Fatal(err)
		}
		listener, err := net.Listen("unix", socket)
		if err != nil {
			t.Fatal(err)
		}
		srv = &http.Server{Handler: handler}
		go srv.Serve(listener)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return f, g
}

func (g *launchRecoveryGateway) calls() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return strings.Join(g.paths, " ")
}

func (g *launchRecoveryGateway) retained(t *testing.T, committed bool) {
	t.Helper()
	g.mu.Lock()
	e := g.envelope
	g.mu.Unlock()
	if e.AttemptID == "" {
		t.Fatal("original issuance missing")
	}
	raw, err := os.ReadFile(filepath.Join(g.directory, "secrets", e.AttemptID+".json"))
	var saved struct {
		Envelope recoveryjournal.Envelope `json:"envelope"`
	}
	if err != nil || json.Unmarshal(raw, &saved) != nil || saved.Envelope != e || bytes.Contains(raw, []byte(e2eToken)) {
		t.Fatal("original private E lost or token persisted")
	}
	receipt, err := os.ReadFile(filepath.Join(g.directory, "receipts", e.AttemptID+".json"))
	if !committed {
		if err == nil {
			t.Fatal("FALSE RECEIPT ALARM: unconfirmed cleanup published receipt")
		}
		return
	}
	if err != nil || bytes.Contains(receipt, []byte(e.RecoveryProof)) || bytes.Contains(receipt, []byte(e2eToken)) || !bytes.Contains(receipt, []byte(`"service_result":"revoked"`)) {
		t.Fatal("committed sanitized receipt missing or private value leaked")
	}
}

// Actual launcher routing preserves v1 activation, reconciles the same E at
// finish, and keeps the proof out of child/provenance/public receipt.
func TestGatewayRecoveryLaunchLifecycle(t *testing.T) {
	f, g := recoveryLaunchFixture(t)
	code, out, stderr := f.run()
	if code != 0 {
		t.Fatalf("native positive refused: code=%d stderr=%s", code, stderr)
	}
	if g.calls() != "/v2/health /v2/register /v1/activate /v2/health /v2/recover" {
		t.Fatalf("native lifecycle calls=%s", g.calls())
	}
	var child childCapture
	if json.Unmarshal(out, &child) != nil {
		t.Fatal("child did not run")
	}
	if strings.Contains(string(out), g.envelope.RecoveryProof) || strings.Contains(string(stderr), g.envelope.RecoveryProof) || strings.Contains(string(stderr), e2eToken) || strings.Contains(strings.Join(child.Argv, " "), e2eToken) {
		t.Fatal("private recovery value escaped child/output boundary")
	}
	if !strings.Contains(strings.Join(child.Env, " "), "ANTHROPIC_CUSTOM_HEADERS=X-Netproxy-Token: "+e2eToken) || !strings.Contains(string(stderr), "receipt=committed") {
		t.Fatal("native route missing token or committed verdict")
	}
	if strings.Contains(string(stderr), "proposed_directories=") {
		t.Fatal("valid existing layout received a creation proposal")
	}
	g.retained(t, true)
}

// Admission readiness is required before any new envelope, private preparation
// or register. The same authority still permits cleanup of an existing attempt.
func TestGatewayRecoveryLaunchAdmissionAndCleanupReadiness(t *testing.T) {
	t.Run("admission", func(t *testing.T) {
		f, g := recoveryLaunchFixture(t)
		g.healthNotReady = true
		before := recoveryStateSnapshot(t, g.directory)
		code, out, diagnostic := f.run()
		if code == 0 || len(out) != 0 || g.calls() != "/v2/health" || strings.Contains(string(diagnostic), "attempt=") {
			t.Fatal("RECOVERY ADMISSION ALARM: unready service admitted a new attempt")
		}
		f.assertNoChild(t)
		if !reflect.DeepEqual(before, recoveryStateSnapshot(t, g.directory)) {
			t.Fatal("RECOVERY ADMISSION ALARM: unready service caused private preparation")
		}
	})
	t.Run("cleanup", func(t *testing.T) {
		f, g := recoveryLaunchFixture(t)
		f.deps.networkEmit = func(io.Writer, *adapterprobe.Provenance) error {
			g.mu.Lock()
			g.healthNotReady = true
			g.mu.Unlock()
			return nil
		}
		code, _, diagnostic := f.run()
		if code != 0 || !strings.Contains(string(diagnostic), "receipt=committed") || g.calls() != "/v2/health /v2/register /v1/activate /v2/health /v2/recover" {
			t.Fatal("RECOVERY CLEANUP ALARM: admission readiness blocked original cleanup")
		}
		attempt := publicRecoveryAttempt(t, diagnostic)
		g.retained(t, true)
		var out, errOut bytes.Buffer
		if run(context.Background(), []string{"gateway", "recover", "--attempt", attempt}, &out, &errOut, f.deps) != 0 {
			t.Fatal("RECOVERY CLEANUP ALARM: unready authority blocked exact retry")
		}
		if strings.Count(g.calls(), "/v2/register") != 1 {
			t.Fatal("RECOVERY CLEANUP ALARM: cleanup reissued authority")
		}
		g.retained(t, true)
	})
}

// Post-issue refusal, interruption and uncertain replies use the original E,
// create no child/second issuance and keep clean recovery independently retryable.
func TestGatewayRecoveryLaunchPostIssueRefusals(t *testing.T) {
	for _, fault := range []string{"lost", "malformed", "lost-unconfirmed", "malformed-unconfirmed", "late-refusal", "interrupted"} {
		t.Run(fault, func(t *testing.T) {
			f, g := recoveryLaunchFixture(t)
			switch fault {
			case "lost", "malformed":
				g.registerFault = fault
			case "lost-unconfirmed", "malformed-unconfirmed":
				g.registerFault = strings.TrimSuffix(fault, "-unconfirmed")
				g.recoverFault = "malformed"
			case "late-refusal":
				f.deps.networkEmit = func(io.Writer, *adapterprobe.Provenance) error { return errors.New("fixture refusal") }
			case "interrupted":
				f.deps.leaseNotify = func(c chan<- os.Signal, _ ...os.Signal) { g.onRegister = func() { c <- syscall.SIGTERM } }
			}
			code, out, stderr := f.run()
			committed := !strings.HasSuffix(fault, "-unconfirmed")
			if code == 0 || len(out) != 0 || strings.Contains(string(stderr), "receipt=committed") != committed {
				t.Fatalf("post-issue refusal did not reconcile: code=%d calls=%s", code, g.calls())
			}
			f.assertNoChild(t)
			if g.calls() != "/v2/health /v2/register /v2/health /v2/recover" {
				t.Fatalf("post-issue refusal calls=%s", g.calls())
			}
			g.retained(t, committed)
			var stdout, errout bytes.Buffer
			attempt := publicRecoveryAttempt(t, stderr)
			if strings.Contains(string(stderr), g.envelope.RecoveryProof) || strings.Contains(string(stderr), e2eToken) {
				t.Fatal("public refusal leaked a private proof or runtime token")
			}
			g.mu.Lock()
			g.recoverFault = ""
			g.mu.Unlock()
			if code := run(context.Background(), []string{"gateway", "recover", "--attempt", attempt}, &stdout, &errout, f.deps); code != 0 {
				t.Fatal("exact original standalone retry refused")
			}
			g.retained(t, true)
		})
	}
}

// Missing/unsafe layout and failed private prepare refuse without registration
// or child, preserving the caller's existing state rather than repairing it.
func TestGatewayRecoveryLaunchPreparationRefusals(t *testing.T) {
	for _, fault := range []string{"missing", "unsafe", "prepare", "mixed"} {
		t.Run(fault, func(t *testing.T) {
			f, g := recoveryLaunchFixture(t)
			switch fault {
			case "missing":
				if err := os.Remove(filepath.Join(g.directory, "receipts")); err != nil {
					t.Fatal(err)
				}
			case "unsafe":
				if err := os.Chmod(filepath.Join(g.directory, "secrets"), 0755); err != nil {
					t.Fatal(err)
				}
			case "prepare":
				if err := os.WriteFile(filepath.Join(g.directory, "lock"), []byte("untrusted"), 0644); err != nil {
					t.Fatal(err)
				}
			case "mixed":
				if err := os.Remove(filepath.Join(g.directory, "receipts")); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(filepath.Join(g.directory, "secrets"), 0755); err != nil {
					t.Fatal(err)
				}
			}
			code, out, stderr := f.run()
			if code == 0 || len(out) != 0 {
				t.Fatal("prepare refusal launched child")
			}
			f.assertNoChild(t)
			calls := g.calls()
			want := ""
			if fault == "prepare" {
				want = "/v2/health"
			}
			if calls != want {
				t.Fatalf("PREPARE NO EFFECT ALARM: calls=%s", calls)
			}
			if fault == "prepare" {
				// Once an envelope exists, any Issue/Prepare refusal must keep its
				// public selector available even if no request reached the service.
				_ = publicRecoveryAttempt(t, stderr)
			} else if strings.Contains(string(stderr), "attempt=") {
				t.Fatal("pre-envelope refusal invented an attempt")
			}
			if fault == "missing" {
				if _, err := os.Stat(filepath.Join(g.directory, "receipts")); !os.IsNotExist(err) {
					t.Fatal("missing layout silently repaired")
				}
			}
			if fault == "unsafe" || fault == "prepare" || fault == "mixed" {
				// An unsafe existing object needs disposition, never mkdir/chmod.
				if strings.Contains(string(stderr), "proposed_directories=") {
					t.Fatal("unsafe existing state received a creation proposal")
				}
			}
		})
	}
}

// Real normal-launch preflight exposes exactly the missing directory closure,
// without private absolute paths, service calls, namespace writes or a child.
// The existing-layout lifecycle above proves this is not an always-refuse path.
func TestGatewayRecoveryLaunchMissingPreflight(t *testing.T) {
	chain := []string{"STATE", "STATE/swarma", "STATE/swarma/launcher", "STATE/swarma/launcher/netproxy-recovery", "STATE/swarma/launcher/netproxy-recovery/v1", "STATE/swarma/launcher/netproxy-recovery/v1/secrets", "STATE/swarma/launcher/netproxy-recovery/v1/receipts"}
	for _, variant := range []string{"state", "swarma", "version", "secrets", "receipts", "both-children", "ancestor"} {
		t.Run(variant, func(t *testing.T) {
			f, g := recoveryLaunchFixture(t)
			state := filepath.Join(f.dir, "state")
			var missing []string
			var want []string
			switch variant {
			case "state":
				missing = []string{state}
				want = chain
			case "swarma":
				missing = []string{filepath.Join(state, "swarma")}
				want = chain[1:]
			case "version":
				missing = []string{g.directory}
				want = chain[4:]
			case "secrets":
				missing = []string{filepath.Join(g.directory, "secrets")}
				want = chain[5:6]
			case "receipts":
				missing = []string{filepath.Join(g.directory, "receipts")}
				want = chain[6:]
			case "both-children":
				missing = []string{filepath.Join(g.directory, "secrets"), filepath.Join(g.directory, "receipts")}
				want = chain[5:]
			case "ancestor":
				missing = []string{filepath.Join(f.dir, "missing-parent")}
				parent := f.deps.environ
				f.deps.environ = func() []string {
					env := append([]string(nil), parent()...)
					for i, e := range env {
						if strings.HasPrefix(e, "XDG_STATE_HOME=") {
							env[i] = "XDG_STATE_HOME=" + filepath.Join(missing[0], "state")
						}
					}
					return env
				}
				want = append([]string{"STATE/.."}, chain...)
			}
			for _, path := range missing {
				if err := os.RemoveAll(path); err != nil {
					t.Fatal(err)
				}
			}
			before := recoveryStateSnapshot(t, state)
			code, out, stderr := f.run()
			if code == 0 || len(out) != 0 || g.calls() != "" {
				t.Fatal("PREFLIGHT NO EFFECT ALARM: missing layout contacted service or launched child")
			}
			f.assertNoChild(t)
			var proposal string
			for _, line := range strings.Split(string(stderr), "\n") {
				if strings.Contains(line, "proposed_directories=") {
					if proposal != "" {
						t.Fatal("duplicate preflight proposal")
					}
					proposal = line
				}
			}
			if !strings.Contains(proposal, "proposed_directories=["+strings.Join(want, ",")+"]; creation_mode=0700;") || !strings.Contains(proposal, "proposal only, no creation performed") || strings.Contains(proposal, f.dir) {
				t.Fatal("PREFLIGHT LIST ALARM: missing, incorrect or private write closure")
			}
			if !reflect.DeepEqual(before, recoveryStateSnapshot(t, state)) {
				t.Fatal("preflight changed existing state")
			}
			for _, path := range missing {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatal("preflight repaired missing state")
				}
			}
		})
	}
}

// Only this fixture's selected state is inspected, including directory modes;
// process evidence and unrelated homes are outside this no-write assertion.
func recoveryStateSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return out
	}
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		value := info.Mode().String()
		if info.Mode().IsRegular() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += "|" + string(raw)
		}
		out[rel] = value
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

// Cleanup uncertainty/local evidence failure never replaces the child's exit
// status with success evidence. The proof survives for exact later recovery.
func TestGatewayRecoveryLaunchCompletionFailures(t *testing.T) {
	for _, fault := range []string{"wrong-registry", "malformed", "local-evidence", "child-exit"} {
		t.Run(fault, func(t *testing.T) {
			f, g := recoveryLaunchFixture(t)
			expectedCode := 1
			if fault == "child-exit" {
				writeFixture(t, filepath.Join(f.dir, "behavior"), []byte("exit:37"), 0600)
				expectedCode = 37
				g.recoverFault = "malformed"
			}
			f.deps.networkEmit = func(io.Writer, *adapterprobe.Provenance) error {
				g.mu.Lock()
				defer g.mu.Unlock()
				switch fault {
				case "wrong-registry":
					g.foreign = true
				case "malformed":
					g.recoverFault = "malformed"
				case "local-evidence":
					if err := os.WriteFile(filepath.Join(g.directory, "receipts", g.envelope.AttemptID+".json"), []byte("untrusted"), 0600); err != nil {
						return err
					}
				}
				return nil
			}
			code, _, stderr := f.run()
			if code != expectedCode || strings.Contains(string(stderr), "receipt=committed") {
				t.Fatal("CLEANUP VERDICT ALARM: cleanup failure became success, child error lost or false evidence emitted")
			}
			attempt := publicRecoveryAttempt(t, stderr)
			if strings.Contains(string(stderr), g.envelope.RecoveryProof) || strings.Contains(string(stderr), e2eToken) {
				t.Fatal("public completion failure leaked a private proof or runtime token")
			}
			if fault == "local-evidence" {
				if !strings.Contains(string(stderr), "local_evidence_failed") {
					t.Fatal("local failure conflated with service uncertainty")
				}
				if err := os.Remove(filepath.Join(g.directory, "receipts", attempt+".json")); err != nil {
					t.Fatal(err)
				}
			} else {
				g.retained(t, false)
			}
			if fault == "wrong-registry" && strings.Contains(g.calls(), "/v2/recover") {
				t.Fatal("replacement registry used original proof")
			}
			g.mu.Lock()
			g.foreign = false
			g.recoverFault = ""
			before := g.envelope
			g.mu.Unlock()
			var stdout, errout bytes.Buffer
			if run(context.Background(), []string{"gateway", "recover", "--attempt", attempt}, &stdout, &errout, f.deps) != 0 {
				t.Fatal("original recovery not retryable")
			}
			g.retained(t, true)
			g.mu.Lock()
			after := g.envelope
			g.mu.Unlock()
			if !reflect.DeepEqual(before, after) || strings.Count(g.calls(), "/v2/register") != 1 {
				t.Fatal("cleanup retry regenerated authority")
			}
		})
	}
}
