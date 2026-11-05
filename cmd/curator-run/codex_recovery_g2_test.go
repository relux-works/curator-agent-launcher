//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"github.com/relux-works/curator-agent-launcher/internal/netproxy"
	"github.com/relux-works/curator-agent-launcher/internal/network"
	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/providerlimits"
	"time"
)

// All executables are the repository's inert capture helper. Version labels and
// qualification rows below are synthetic policy inputs, never native evidence.
func codexRecoveryFixture(t *testing.T) (*pipelineFixture, *launchRecoveryGateway, string) {
	t.Helper()
	stage, err := os.MkdirTemp("", "g2stage-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stage) })
	t.Setenv("TMPDIR", stage)
	f := entryFixture(t, "codex_cli")
	withTestRelease(f, "codex-cli 0.164.0")
	f.args = []string{"codex_cli", "--permissions", "native", "--model", "gpt-6-astra", "--effort", "high"}
	f, g := recoveryLaunchFixture(t, f)
	qual := filepath.Join(f.dir, ".curator", "network-qualified.toml")
	raw, err := os.ReadFile(qual)
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.ReplaceAll(raw, []byte(`harness = "claude-code"`), []byte(`harness = "codex-cli"`))
	raw = bytes.ReplaceAll(raw, []byte(`entrypoint = "exec"`), []byte(`entrypoint = "interactive"`))
	writeFixture(t, qual, raw, 0600)
	data, err := os.ReadFile(pipelineHelper)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(f.dir, "codex-code-mode-host"), data, 0700)
	store, err := providerlimits.NewStore(providerlimits.Options{Layout: providerlimits.LayoutAt(t.TempDir())})
	if err != nil {
		t.Fatal(err)
	}
	f.deps.gatewayStore = func() (*providerlimits.Store, error) { return store, nil }
	previous := f.deps.environ
	f.deps.environ = func() []string {
		return append(previous(), "CURATOR_TEST_PROCESS_RECEIPT="+filepath.Join(f.dir, "child-receipt.json"))
	}
	return f, g, stage
}

func TestCodexRecoveryG2Matrix(t *testing.T) {
	cases := []string{"valid164", "native166", "default166", "yolo166", "unknown-release", "provider-env", "remote-env", "native-provider", "missing-host", "stage-mutation", "missing-journal", "unsafe-journal", "prepare-fault", "not-ready", "legacy-health", "malformed-health", "lost-reply", "malformed-register", "interrupt-after-register", "child-error", "child-signal", "cleanup-registry", "cleanup-malformed", "cleanup-local", "restart-not-ready", "same-authority-restart", "forward-sigint", "forward-sigterm"}
	for _, kind := range cases {
		t.Run(kind, func(t *testing.T) {
			f, g, stages := codexRecoveryFixture(t)
			var signalDone chan struct{}
			switch kind {
			case "native166", "default166", "yolo166":
				withTestRelease(f, "codex-cli 0.166.0")
				mode := "native"
				if kind == "default166" {
					mode = "default"
				}
				if kind == "yolo166" {
					mode = "yolo"
				}
				for i := range f.args {
					if f.args[i] == "native" {
						f.args[i] = mode
					}
				}
			case "unknown-release":
				withTestRelease(f, "codex-cli 9.999.0")
			case "provider-env", "remote-env":
				name := "OPENAI_BASE_URL"
				if kind == "remote-env" {
					name = "CODEX_REMOTE_URL"
				}
				previous := f.deps.environ
				f.deps.environ = func() []string { return append(previous(), name+"=http://127.0.0.1:1") }
			case "native-provider":
				f.args = append(f.args, "--", "-c", "model_provider=other")
			case "missing-host":
				if os.Remove(filepath.Join(f.dir, "codex-code-mode-host")) != nil {
					t.Fatal("fixture host")
				}
			case "stage-mutation":
				g.onRegister = func() {
					matches, _ := filepath.Glob(filepath.Join(stages, "curator-adapter-*", "codex-code-mode-host"))
					if len(matches) != 1 {
						t.Error("missing staged host")
						return
					}
					if os.Chmod(matches[0], 0600) != nil || os.WriteFile(matches[0], []byte("changed"), 0600) != nil {
						t.Error("host mutation failed")
					}
				}
			case "missing-journal":
				if os.Remove(filepath.Join(g.directory, "receipts")) != nil {
					t.Fatal("fixture receipts")
				}
			case "unsafe-journal":
				if os.Chmod(filepath.Join(g.directory, "secrets"), 0755) != nil {
					t.Fatal("fixture journal")
				}
			case "prepare-fault":
				writeFixture(t, filepath.Join(g.directory, "lock"), []byte("unsafe"), 0644)
			case "not-ready":
				g.healthNotReady = true
			case "legacy-health":
				g.healthFault = "legacy"
			case "malformed-health":
				g.healthFault = "malformed"
			case "lost-reply":
				g.registerFault = "lost"
			case "malformed-register":
				g.registerFault = "malformed"
			case "interrupt-after-register":
				var held chan<- os.Signal
				f.deps.leaseNotify = func(ch chan<- os.Signal, _ ...os.Signal) { held = ch }
				g.onRegister = func() { held <- syscall.SIGTERM }
			case "forward-sigint", "forward-sigterm":
				ready := make(chan chan<- os.Signal, 1)
				f.deps.leaseNotify = func(ch chan<- os.Signal, _ ...os.Signal) { ready <- ch }
				writeFixture(t, filepath.Join(f.dir, "behavior"), []byte("waitsignal"), 0600)
				signalDone = make(chan struct{})
				go func() {
					defer close(signalDone)
					select {
					case held := <-ready:
						send := func() {
							if kind == "forward-sigint" {
								held <- os.Interrupt
							} else {
								held <- syscall.SIGTERM
							}
						}
						ticker := time.NewTicker(time.Millisecond)
						defer ticker.Stop()
						deadline := time.After(30 * time.Second)
						for {
							select {
							case <-ticker.C:
								if _, err := os.Stat(filepath.Join(f.dir, "child-receipt.json")); err == nil {
									send()
									return
								}
							case <-deadline:
								t.Error("child receipt absent before signal")
								// The waitsignal child only ends on a signal: fail, do not hang.
								send()
								return
							}
						}
					case <-time.After(30 * time.Second):
						t.Error("lease signals not held")
					}
				}()
			case "child-error":
				writeFixture(t, filepath.Join(f.dir, "behavior"), []byte("exit:37"), 0600)
			case "child-signal":
				writeFixture(t, filepath.Join(f.dir, "behavior"), []byte("signal"), 0600)
			case "cleanup-registry", "cleanup-malformed", "cleanup-local":
				f.deps.networkEmit = func(io.Writer, *adapterprobe.Provenance) error {
					g.mu.Lock()
					defer g.mu.Unlock()
					if kind == "cleanup-registry" {
						g.foreign = true
					}
					if kind == "cleanup-malformed" {
						g.recoverFault = "malformed"
					}
					if kind == "cleanup-local" {
						return os.Chmod(filepath.Join(g.directory, "receipts"), 0755)
					}
					return nil
				}
			case "same-authority-restart":
				f.deps.networkEmit = func(io.Writer, *adapterprobe.Provenance) error { g.restart(); return nil }
			case "restart-not-ready":
				g.onRegister = func() { g.healthNotReady = true }
			}
			code, out, diag := f.run()
			if signalDone != nil {
				<-signalDone
			}
			calls := g.calls()
			success := kind == "valid164" || kind == "restart-not-ready" || kind == "same-authority-restart"
			child := success || strings.HasPrefix(kind, "forward-") || kind == "child-error" || kind == "child-signal" || strings.HasPrefix(kind, "cleanup-")
			if success && code != 0 {
				t.Fatalf("positive refused: %d %s", code, diag)
			}
			if !success && code == 0 {
				t.Fatalf("failure became success: %s", diag)
			}
			if kind == "child-error" && code != 37 {
				t.Fatalf("child exit lost: %d", code)
			}
			if kind == "child-signal" && code != 128+int(syscall.SIGTERM) {
				t.Fatalf("child signal lost: %d", code)
			}
			if kind == "forward-sigint" && code != 130 {
				t.Fatalf("SIGINT not forwarded: %d", code)
			}
			if kind == "forward-sigterm" && code != 143 {
				t.Fatalf("SIGTERM not forwarded: %d", code)
			}
			if !child {
				f.assertNoChild(t)
				if len(out) != 0 {
					t.Fatal("refusal emitted child output")
				}
			}
			if strings.Contains(calls, "/v1/register") || strings.Contains(calls, "/v1/unregister") {
				t.Fatal("control/2 downgraded")
			}
			if strings.Count(calls, "/v2/register") > 1 || strings.Count(calls, "/v2/recover") > 1 {
				t.Fatal("duplicate issuance or completion")
			}
			issued := g.envelope.AttemptID != ""
			if !issued {
				if strings.Contains(calls, "/v2/register") {
					t.Fatal("unobserved issue")
				}
				if kind != "prepare-fault" && (strings.Contains(string(diag), "attempt=") || strings.Contains(calls, "/v2/recover")) {
					t.Fatal("pre-admission effects")
				}
				if kind == "native166" || kind == "default166" || kind == "yolo166" || kind == "unknown-release" || kind == "provider-env" || kind == "remote-env" || kind == "native-provider" || kind == "missing-host" {
					if calls != "" {
						t.Fatalf("refusal reached control: %s", calls)
					}
				}
			} else {
				if g.envelope.Runtime != "codex" {
					t.Fatal("runtime identity changed")
				}
				if strings.Contains(string(diag), g.envelope.RecoveryProof) || strings.Contains(string(diag), e2eToken) || bytes.Contains(out, []byte(g.envelope.RecoveryProof)) {
					t.Fatal("private authority leaked")
				}
				confirmed := !strings.HasPrefix(kind, "cleanup-")
				if kind != "cleanup-local" {
					g.retained(t, confirmed)
				}
				if confirmed && !strings.Contains(string(diag), "receipt=committed") {
					t.Fatal("missing completion evidence")
				}
				if !confirmed && strings.Contains(string(diag), "receipt=committed") {
					t.Fatal("false committed receipt")
				}
				if child {
					var process struct {
						PID     int
						Started string
					}
					raw, err := os.ReadFile(filepath.Join(f.dir, "child-receipt.json"))
					if err != nil || json.Unmarshal(raw, &process) != nil || process.PID <= 0 || process.Started == "" {
						t.Fatal("missing owned process receipt")
					}
					if syscall.Kill(process.PID, 0) != syscall.ESRCH {
						t.Fatal("owned child not reaped")
					}
					t.Logf("owned_pid=%d started=%s reaped=true child_exit=%d", process.PID, process.Started, code)
					var captured childCapture
					if json.NewDecoder(bytes.NewReader(out)).Decode(&captured) != nil {
						t.Fatal("missing child capture")
					}
					if !strings.Contains(strings.Join(captured.Env, " "), "CURATOR_CODEX_GATEWAY_TOKEN="+e2eToken) || strings.Contains(strings.Join(captured.Argv, " "), e2eToken) {
						t.Fatal("token boundary changed")
					}
					if !strings.Contains(string(diag), "sam_attempt=") || !strings.Contains(string(diag), "launch="+strings.Repeat("d", 64)) || !strings.Contains(string(diag), "registration_session="+g.envelope.Session) {
						t.Fatal("missing distinct correlation")
					}
					for _, env := range captured.Env {
						if strings.HasPrefix(env, "CURATOR_CODEX_GATEWAY_TOKEN=") {
							continue
						}
						if strings.Contains(env, g.envelope.RecoveryProof) {
							t.Fatal("proof in child")
						}
					}
				}
				if strings.HasPrefix(kind, "cleanup-") {
					g.mu.Lock()
					g.foreign = false
					g.recoverFault = ""
					g.mu.Unlock()
					if kind == "cleanup-local" && os.Chmod(filepath.Join(g.directory, "receipts"), 0700) != nil {
						t.Fatal("restore fixture")
					}
					original := g.envelope
					var stdout, stderr bytes.Buffer
					if run(context.Background(), []string{"gateway", "recover", "--attempt", original.AttemptID}, &stdout, &stderr, f.deps) != 0 {
						t.Fatalf("exact retry refused: %s", stderr.String())
					}
					g.retained(t, true)
					if g.envelope != original || strings.Count(g.calls(), "/v2/register") != 1 {
						t.Fatal("recovery replaced original issuance")
					}
				}
			}
			if kind == "same-authority-restart" {
				g.restart()
				client := netproxy.New(g.socket)
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if !netproxy.IsCode(client.Renew(ctx, e2eToken), 404, "unknown_token") {
					t.Fatal("ended token admitted after fixture restart")
				}
				original := g.envelope
				before, err := os.ReadFile(filepath.Join(g.directory, "receipts", original.AttemptID+".json"))
				if err != nil {
					t.Fatal(err)
				}
				var stdout, stderr bytes.Buffer
				if run(context.Background(), []string{"gateway", "recover", "--attempt", original.AttemptID}, &stdout, &stderr, f.deps) != 0 {
					t.Fatal("restart exact retry refused")
				}
				after, _ := os.ReadFile(filepath.Join(g.directory, "receipts", original.AttemptID+".json"))
				if !bytes.Equal(before, after) || g.envelope != original || strings.Count(g.calls(), "/v2/register") != 1 {
					t.Fatal("restart changed original receipt or issuance")
				}
			}
			left, _ := filepath.Glob(filepath.Join(stages, "curator-adapter-*"))
			if len(left) != 0 {
				t.Fatal("owned stage leaked")
			}
			t.Logf("scenario=%s exit=%d workload_spawn=%t controls=%s stage_removed=true", kind, code, child, calls)
		})
	}
}

// A failed OS start owns the lease just as a final-process refusal does. No
// real runtime is used, and repeated Finish shares the same completion.
func TestG2FailedSpawnCompletesOnce(t *testing.T) {
	var completions int
	lease, err := netproxy.NewRecoveryLease(netproxy.New("/unused-fixture.socket"), netproxy.Registration{}, io.Discard, time.Now(), func(context.Context) error { completions++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	owner := ownLeaseSignals(func(chan<- os.Signal, ...os.Signal) {}, func(chan<- os.Signal) {})
	prepared := network.Prepared{Lease: lease}
	var stdout, stderr bytes.Buffer
	code := runNative(context.Background(), composition.Value{Binary: filepath.Join(t.TempDir(), "missing"), WorkDir: t.TempDir()}, execution.PermissionDecision{}, agentic.StoredPolicyInspection{}, &stdout, &stderr, launchDeps{}, func() error { return nil }, &prepared, owner)
	if code == 0 || stdout.Len() != 0 || completions != 1 {
		t.Fatal("failed spawn lost original completion")
	}
	if lease.Finish(context.Background()) != nil || completions != 1 {
		t.Fatal("completion repeated")
	}
}

type rejectG2Correlation struct{}

func (rejectG2Correlation) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("gateway correlation:")) {
		return 0, io.ErrClosedPipe
	}
	return len(p), nil
}

func TestCodexRecoveryCorrelationOutputFailure(t *testing.T) {
	f, g, stages := codexRecoveryFixture(t)
	var stdout bytes.Buffer
	if run(context.Background(), f.args, &stdout, rejectG2Correlation{}, f.deps) == 0 || stdout.Len() != 0 {
		t.Fatal("lost correlation output started a child")
	}
	f.assertNoChild(t)
	if g.calls() != "/v2/health /v2/register /v2/health /v2/recover" {
		t.Fatalf("output refusal lost exact completion: %s", g.calls())
	}
	g.retained(t, true)
	left, _ := filepath.Glob(filepath.Join(stages, "curator-adapter-*"))
	if len(left) != 0 {
		t.Fatal("output refusal leaked stage")
	}
}
