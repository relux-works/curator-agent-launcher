//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
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

// g2Bypass is the flag the pinned module maps to the yolo mode for its listed
// Codex releases; g2Thread is a synthetic thread identity for a native resume.
const (
	g2Bypass = "--dangerously-bypass-approvals-and-sandbox"
	g2Thread = "01234567-89ab-cdef-0123-456789abcdef"
)

var g2PreIssueRefusalKinds = []string{
	"default166", "yolo166", "yolo-unknown-release", "native-bypass", "yolo-bypass",
	"provider-env", "remote-env", "native-provider", "native-apps", "native-enable-apps",
	"native-plugins", "native-analytics", "native-otel", "native-update-check", "missing-host",
	"symlink-home", "proxy-HTTP_PROXY", "proxy-https_proxy", "proxy-ALL_PROXY", "proxy-All_Proxy",
}

func g2PreIssueRefusalKind(kind string) bool {
	for _, refusal := range g2PreIssueRefusalKinds {
		if kind == refusal {
			return true
		}
	}
	return false
}

// g2PreIssueRefusalFailure is shared by the matrix and the deliberate plant:
// refusal rows must not issue, call control, or expose attempt/recovery effects.
func g2PreIssueRefusalFailure(kind string, issued bool, calls, diag string) string {
	if !g2PreIssueRefusalKind(kind) {
		return ""
	}
	if issued {
		return "pre-issue refusal recorded an attempt"
	}
	if calls != "" {
		return "pre-issue refusal reached control: " + calls
	}
	if strings.Contains(diag, "attempt=") {
		return "pre-issue refusal exposed attempt effects"
	}
	return ""
}

func g2ContainsIdent(node ast.Node, name string) bool {
	found := false
	ast.Inspect(node, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if ok && ident.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

func g2ContainsNode(container, child ast.Node) bool {
	return container != nil && child.Pos() >= container.Pos() && child.End() <= container.End()
}

func g2ReportsFailure(node ast.Node) bool {
	reported := false
	ast.Inspect(node, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Fatal" && selector.Sel.Name != "Fatalf" && selector.Sel.Name != "Error" && selector.Sel.Name != "Errorf" {
			return true
		}
		if receiver, ok := selector.X.(*ast.Ident); !ok || receiver.Name != "t" {
			return true
		}
		for _, arg := range call.Args {
			if ident, ok := arg.(*ast.Ident); ok && ident.Name == "failure" {
				reported = true
				return false
			}
		}
		return true
	})
	return reported
}

func g2ChecksNonEmptyFailure(expr ast.Expr) bool {
	condition, ok := expr.(*ast.BinaryExpr)
	if !ok || condition.Op != token.NEQ {
		return false
	}
	isFailure := func(expr ast.Expr) bool {
		ident, ok := expr.(*ast.Ident)
		return ok && ident.Name == "failure"
	}
	isEmpty := func(expr ast.Expr) bool {
		literal, ok := expr.(*ast.BasicLit)
		return ok && literal.Kind == token.STRING && literal.Value == `""`
	}
	return isFailure(condition.X) && isEmpty(condition.Y) || isFailure(condition.Y) && isEmpty(condition.X)
}

// The plant also inspects the matrix source because a helper-only plant cannot
// detect the helper call being moved under the matrix's !issued branch.
func g2AssertPreIssueMatrixCallSite(t *testing.T) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "codex_recovery_g2_test.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var matrix *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "TestCodexRecoveryG2Matrix" {
			matrix = fn
			break
		}
	}
	if matrix == nil {
		t.Fatal("matrix function missing")
	}
	var check *ast.CallExpr
	checks := 0
	ast.Inspect(matrix.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok {
			if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "g2PreIssueRefusalFailure" {
				check = call
				checks++
			}
		}
		return true
	})
	if checks != 1 {
		t.Fatalf("matrix has %d pre-issue refusal checks, want exactly one", checks)
	}
	checked := false
	gated := false
	ast.Inspect(matrix.Body, func(node ast.Node) bool {
		stmt, ok := node.(*ast.IfStmt)
		if !ok {
			return true
		}
		inCheck := g2ContainsNode(stmt.Init, check) || g2ContainsNode(stmt.Cond, check)
		if inCheck && g2ChecksNonEmptyFailure(stmt.Cond) && g2ReportsFailure(stmt.Body) && !g2ContainsIdent(stmt.Cond, "issued") {
			checked = true
		}
		if g2ContainsNode(stmt.Body, check) || g2ContainsNode(stmt.Else, check) {
			if !inCheck || g2ContainsIdent(stmt.Cond, "issued") {
				gated = true
			}
		}
		return true
	})
	if !checked {
		t.Fatal("matrix does not fail on the pre-issue refusal assertion")
	}
	if gated {
		t.Fatal("matrix pre-issue refusal assertion is conditional")
	}
	if len(g2PreIssueRefusalKinds) != 20 {
		t.Fatalf("plant refusal rows = %d, want 20", len(g2PreIssueRefusalKinds))
	}
	seenRows := make(map[string]bool, len(g2PreIssueRefusalKinds))
	for _, kind := range g2PreIssueRefusalKinds {
		if seenRows[kind] {
			t.Fatalf("duplicate plant refusal row %q", kind)
		}
		seenRows[kind] = true
	}
	var sharedRowsAt, matrixLoopAt token.Pos
	ast.Inspect(matrix.Body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.AssignStmt:
			if len(node.Lhs) != 1 {
				break
			}
			lhs, ok := node.Lhs[0].(*ast.Ident)
			if !ok || lhs.Name != "cases" {
				break
			}
			for _, rhs := range node.Rhs {
				call, ok := rhs.(*ast.CallExpr)
				if !ok {
					continue
				}
				fn, ok := call.Fun.(*ast.Ident)
				if ok && fn.Name == "append" && g2ContainsIdent(call, "g2PreIssueRefusalKinds") {
					sharedRowsAt = node.Pos()
				}
			}
		case *ast.RangeStmt:
			if ident, ok := node.X.(*ast.Ident); ok && ident.Name == "cases" {
				matrixLoopAt = node.Pos()
			}
		}
		return true
	})
	if sharedRowsAt == token.NoPos || matrixLoopAt == token.NoPos || sharedRowsAt >= matrixLoopAt {
		t.Fatal("matrix does not loop over the same refusal rows as the plants")
	}
}

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

// The Codex gateway route under control/2 recovery: a native-permission plan
// with an unmodified environment launches whatever its release label (0.164.0,
// the unlisted 0.166.0, 0.167.0 and an unknown one), every listed refusal
// starts no child and leaves no stage, and each refusal before admission makes
// no control call. The permission grammar stays release-bound: default and
// yolo on the unlisted 0.166.0 still refuse, and so does yolo on an unknown
// release. On the listed 0.164.0 and 0.167.0 a yolo plan launches, alone and
// before an exact `resume <thread>`: the child's arguments carry the bypass
// flag once, after the effort pair and before the native suffix, and the
// operator's line names it. Every other child carries no bypass flag and the
// line `mapped=none`. The flag typed after `--` refuses under either mode:
// natively as an argument the gateway does not admit, under yolo as a
// duplicate of the flag the mode emits. The yolo rows use a fragment that
// carries a permission mode; the fixture's own does not, and refuses yolo
// before the release is asked. Proxy, provider, remote and
// native-provider refusals name their check class in the diagnostic, and so
// does a native selection of an owned setting (`features.apps`,
// `features.plugins`, `analytics.enabled`, an `otel` exporter,
// `check_for_update_on_startup`). Every child's arguments carry the four
// owned settings (`features.apps`, `features.plugins`, `analytics.enabled`,
// `check_for_update_on_startup`, each `=false`) exactly once and no other
// selector of those settings.
// Children get exactly the plan's canonical CODEX_HOME and no override
// warning; a symlinked fragment home refuses before any control call. An
// inherited proxy selector in any letter case refuses before health.
// Executables are inert fixtures, not native Codex.
func TestCodexRecoveryG2Matrix(t *testing.T) {
	cases := []string{
		"valid164", "native166", "native167", "unknown-release", "yolo164", "yolo167", "yolo167-resume",
		"stage-mutation", "missing-journal", "unsafe-journal", "prepare-fault", "not-ready", "legacy-health",
		"malformed-health", "lost-reply", "malformed-register", "interrupt-after-register", "child-error",
		"child-signal", "cleanup-registry", "cleanup-malformed", "cleanup-local", "restart-not-ready",
		"same-authority-restart", "forward-sigint", "forward-sigterm",
	}
	cases = append(cases, g2PreIssueRefusalKinds...)
	for _, kind := range cases {
		t.Run(kind, func(t *testing.T) {
			f, g, stages := codexRecoveryFixture(t)
			var signalDone chan struct{}
			permissions := func(mode string) {
				for i := range f.args {
					if f.args[i] == "native" {
						f.args[i] = mode
					}
				}
			}
			switch kind {
			case "native166", "default166", "yolo166":
				withTestRelease(f, "codex-cli 0.166.0")
				mode := "native"
				if kind == "default166" {
					mode = "default"
				}
				if kind == "yolo166" {
					// The fixture's fragment carries no permission mode; without
					// one that does, yolo refuses before the release is asked.
					v2Default(f, t)
					mode = "yolo"
				}
				permissions(mode)
			case "yolo164":
				v2Default(f, t)
				permissions("yolo")
			case "yolo167":
				withTestRelease(f, "codex-cli 0.167.0")
				v2Default(f, t)
				permissions("yolo")
			case "yolo167-resume":
				withTestRelease(f, "codex-cli 0.167.0")
				v2Default(f, t)
				permissions("yolo")
				f.args = append(f.args, "--", "resume", g2Thread)
			case "yolo-unknown-release":
				withTestRelease(f, "codex-cli 9.999.0")
				v2Default(f, t)
				permissions("yolo")
			case "native-bypass":
				f.args = append(f.args, "--", g2Bypass)
			case "yolo-bypass":
				withTestRelease(f, "codex-cli 0.167.0")
				v2Default(f, t)
				permissions("yolo")
				f.args = append(f.args, "--", g2Bypass)
			case "native167":
				withTestRelease(f, "codex-cli 0.167.0")
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
			case "native-apps":
				f.args = append(f.args, "--", "-c", "features.apps=true")
			case "native-enable-apps":
				f.args = append(f.args, "--", "--enable", "apps")
			case "native-plugins":
				f.args = append(f.args, "--", "-c", "features.plugins=true")
			case "native-analytics":
				f.args = append(f.args, "--", "-c", "analytics.enabled=true")
			case "native-otel":
				f.args = append(f.args, "--", "-c", `otel.metrics_exporter="statsig"`)
			case "native-update-check":
				f.args = append(f.args, "--", "-c", "check_for_update_on_startup=true")
			case "symlink-home":
				link := filepath.Join(f.dir, "home-link")
				if os.Symlink(f.home, link) != nil {
					t.Fatal("fixture home link")
				}
				from, to := `"CODEX_HOME":"`+f.home+`"`, `"CODEX_HOME":"`+link+`"`
				if strings.Count(f.resolver.stdout, from) != 1 {
					t.Fatal("fixture fragment home")
				}
				f.resolver.stdout = strings.Replace(f.resolver.stdout, from, to, 1)
			case "proxy-HTTP_PROXY", "proxy-https_proxy", "proxy-ALL_PROXY", "proxy-All_Proxy":
				name := strings.TrimPrefix(kind, "proxy-")
				previous := f.deps.environ
				f.deps.environ = func() []string { return append(previous(), name+"=http://127.0.0.1:1") }
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
			yolo := kind == "yolo164" || kind == "yolo167" || kind == "yolo167-resume"
			success := yolo || kind == "valid164" || kind == "native166" || kind == "native167" || kind == "unknown-release" || kind == "restart-not-ready" || kind == "same-authority-restart"
			child := success || strings.HasPrefix(kind, "forward-") || kind == "child-error" || kind == "child-signal" || strings.HasPrefix(kind, "cleanup-")
			if success && code != 0 {
				t.Fatalf("positive refused: %d %s", code, diag)
			}
			if !success && code == 0 {
				t.Fatalf("failure became success: %s", diag)
			}
			// A refusal from the module's gateway checks names its check class in
			// the diagnostic the operator reads; the other refusals keep their text.
			reason := map[string]string{"provider-env": "gateway_provider_route_override", "remote-env": "gateway_provider_route_override", "native-provider": "gateway_native_arguments_not_admitted",
				"native-apps": "gateway_native_arguments_not_admitted", "native-enable-apps": "gateway_native_arguments_not_admitted",
				"native-plugins": "gateway_native_arguments_not_admitted", "native-analytics": "gateway_native_arguments_not_admitted",
				"native-otel": "gateway_native_arguments_not_admitted", "native-update-check": "gateway_native_arguments_not_admitted",
				"native-bypass": "gateway_native_arguments_not_admitted"}[kind]
			if strings.HasPrefix(kind, "proxy-") {
				reason = "gateway_direct_proxy_selector_present"
			}
			if reason != "" && !strings.Contains(string(diag), "curator-run: plan_refused: agentic: Codex gateway binding refused reason="+reason+"\n") {
				t.Fatalf("refusal does not name %s: %s", reason, diag)
			}
			// An unlisted or unknown release has no mapping for the bypass: the
			// refusal is the permission one, before any gateway check.
			if (kind == "yolo166" || kind == "yolo-unknown-release") && !strings.Contains(string(diag), "curator-run: plan_refused: permission mapping was not established: ") {
				t.Fatalf("yolo on an unlisted release is not the permission refusal: %s", diag)
			}
			// Under yolo the mode itself emits the flag, so the typed one is a
			// duplicate: the module refuses to emit it twice, as a usage error.
			if kind == "yolo-bypass" && (code != 2 || !strings.Contains(string(diag), "curator-run: usage: plan_refused: ") || !strings.Contains(string(diag), "bypass flag already present")) {
				t.Fatalf("typed bypass under yolo is not the duplicate refusal: %d %s", code, diag)
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
			if failure := g2PreIssueRefusalFailure(kind, issued, calls, string(diag)); failure != "" {
				t.Fatal(failure)
			}
			if !issued {
				if strings.Contains(calls, "/v2/register") {
					t.Fatal("unobserved issue")
				}
				if kind != "prepare-fault" && (strings.Contains(string(diag), "attempt=") || strings.Contains(calls, "/v2/recover")) {
					t.Fatal("pre-admission effects")
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
					switches := map[string]int{"features.apps=false": 0, "features.plugins=false": 0, "analytics.enabled=false": 0, "check_for_update_on_startup=false": 0}
					selectors := 0
					for i, arg := range captured.Argv {
						if strings.Contains(arg, "features") || strings.Contains(arg, "analytics") || strings.Contains(arg, "otel") || strings.Contains(arg, "check_for_update") || arg == "--enable" || arg == "--disable" {
							selectors++
						}
						if _, owned := switches[arg]; owned && i > 0 && captured.Argv[i-1] == "-c" {
							switches[arg]++
						}
					}
					for off, n := range switches {
						if n != 1 {
							t.Fatalf("child arguments carry -c %s %d times, want once: %q", off, n, captured.Argv)
						}
					}
					if selectors != len(switches) {
						t.Fatalf("child arguments name an owned setting %d times, want only the %d switches: %q", selectors, len(switches), captured.Argv)
					}
					// The permission mode reaches the child as the one flag the
					// module maps for it, in its place, and the operator's line
					// names the same mapping. A native child carries no such flag.
					line, tail := "curator-run: permissions=native source=flag mapped=none\n", []string{}
					if yolo {
						line, tail = "curator-run: permissions=yolo source=flag mapped="+g2Bypass+"\n", []string{g2Bypass}
					}
					if kind == "yolo167-resume" {
						tail = append(tail, "resume", g2Thread)
					}
					if strings.Count(string(diag), "curator-run: permissions=") != 1 || !strings.Contains(string(diag), line) {
						t.Fatalf("permission line is not %q: %s", line, diag)
					}
					// Counted by family, not by one spelling: the bypass flag, a
					// sandbox selector or an approval selector in any argument.
					bypasses := 0
					for _, arg := range captured.Argv {
						if strings.Contains(arg, "dangerously") || strings.Contains(arg, "sandbox") || strings.Contains(arg, "approval") {
							bypasses++
						}
					}
					if want := map[bool]int{true: 1}[yolo]; bypasses != want {
						t.Fatalf("child arguments name a permission override %d times, want %d: %q", bypasses, want, captured.Argv)
					}
					if yolo {
						before := len(captured.Argv) - len(tail)
						if before < 2 || !slices.Equal(captured.Argv[before:], tail) || captured.Argv[before-2] != "-c" || captured.Argv[before-1] != `model_reasoning_effort="high"` {
							t.Fatalf("bypass flag is not after the effort pair and before the suffix %q: %q", tail, captured.Argv)
						}
					}
					if !strings.Contains(string(diag), "sam_attempt=") || !strings.Contains(string(diag), "launch="+strings.Repeat("d", 64)) || !strings.Contains(string(diag), "registration_session="+g.envelope.Session) {
						t.Fatal("missing distinct correlation")
					}
					var homes []string
					for _, env := range captured.Env {
						if name, value, _ := strings.Cut(env, "="); strings.EqualFold(name, "CODEX_HOME") {
							homes = append(homes, name+"="+value)
						}
					}
					if len(homes) != 1 || homes[0] != "CODEX_HOME="+f.home {
						t.Fatalf("child home %v, want canonical %s", homes, f.home)
					}
					if strings.Contains(string(diag), "environment override") {
						t.Fatal("false environment override warning")
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

// These plants inspect the matrix call site and independently seed one
// pre-issue effect into every refusal row. They test the assertion boundary,
// not launch order.
func TestCodexRecoveryG2PreIssueRefusalRegisterPlant(t *testing.T) {
	t.Run("matrix-call-site", func(t *testing.T) {
		g2AssertPreIssueMatrixCallSite(t)
	})
	plants := []struct {
		name, calls, diag string
		issued            bool
	}{
		{name: "issued-attempt", issued: true},
		{name: "control-call", calls: "/v2/register"},
		{name: "attempt-diagnostic", diag: "attempt=planted"},
	}
	for _, kind := range g2PreIssueRefusalKinds {
		for _, plant := range plants {
			t.Run(kind+"/"+plant.name, func(t *testing.T) {
				if failure := g2PreIssueRefusalFailure(kind, plant.issued, plant.calls, plant.diag); failure == "" {
					t.Fatal("planted pre-issue effect passed the refusal assertion")
				}
			})
		}
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

// A failed write of the correlation line after issuance completes the original
// attempt exactly once, starts no child and leaves no stage.
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
