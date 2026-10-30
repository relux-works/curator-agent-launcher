package main

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
	"github.com/relux-works/curator-network-profiles/pkg/netprofile"
)

const e2eToken = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

// e2eGateway serves netproxy-control/1 on a unix socket and records the
// order of calls.
type e2eGateway struct {
	socket string
	mu     sync.Mutex
	paths  []string
	// persistFailures makes that many unregister calls answer 503
	// persist_failed before the usual 204.
	persistFailures int
}

func newE2EGateway(t *testing.T) *e2eGateway {
	t.Helper()
	dir, err := os.MkdirTemp("", "gw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	g := &e2eGateway{socket: filepath.Join(dir, "control.sock")}
	ln, err := net.Listen("unix", g.socket)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.paths = append(g.paths, r.URL.Path)
		g.mu.Unlock()
		w.Header().Set("Netproxy-Control-Api", "netproxy-control/1")
		switch r.URL.Path {
		case "/v1/health":
			io.WriteString(w, `{"api":"netproxy-control/1","ready":true}`)
		case "/v1/register":
			io.WriteString(w, `{"api":"netproxy-control/1","token":"`+e2eToken+`","token_header":"X-Netproxy-Token","anthropic_base":"http://127.0.0.1:47811/anthropic","openai_base":"http://127.0.0.1:47811/openai/v1","chatgpt_base":"http://127.0.0.1:47811/chatgpt/backend-api/codex","activation_seconds":120}`)
		case "/v1/activate", "/v1/renew":
			io.WriteString(w, `{"api":"netproxy-control/1","active":true}`)
		case "/v1/unregister":
			g.mu.Lock()
			fail := g.persistFailures != 0
			if g.persistFailures > 0 {
				g.persistFailures--
			}
			g.mu.Unlock()
			if fail {
				w.WriteHeader(503)
				io.WriteString(w, `{"api":"netproxy-control/1","error":"persist_failed"}`)
				return
			}
			w.WriteHeader(204)
		}
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return g
}

func (g *e2eGateway) calls() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return strings.Join(g.paths, " ")
}

// writeGatewayCatalog confirms a filtering-gateway profile egress-a under
// home and lists the fixture's provider binary as a qualified build.
func writeGatewayCatalog(t *testing.T, f *pipelineFixture, socket string) {
	t.Helper()
	dir := filepath.Join(f.dir, ".curator")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	doc := "schema = \"relux-network-profiles-v1\"\n[networks.egress-a]\nkind = \"filtering-gateway\"\ncontrol = \"" + socket + "\"\nsensitive_egress = true\n"
	file, err := netprofile.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	prof, _ := file.Lookup("egress-a")
	ledger, _ := json.Marshal(map[string]any{"schema": "relux-network-confirmations-v1",
		"confirmed": map[string]any{"egress-a": map[string]any{"digest": netprofile.Digest(prof), "confirmed_at": "2026-10-30T00:00:00Z"}}})
	sha := strings.TrimPrefix(artifactBuild(t, f), "sha256-")
	qualified := "schema = \"curator-network-qualified-v1\"\nproject_roots = [\"" + f.dir + "\"]\n[[builds]]\nharness = \"claude-code\"\nentrypoint = \"exec\"\nbinary_sha256 = \"" + sha +
		"\"\nplatform = \"" + runtime.GOOS + "\"\nevidence = \"test fixture\"\nqualified_on = \"2026-10-30\"\n"
	for name, data := range map[string]string{"network.toml": doc, "network.confirmations.json": string(ledger), "network-qualified.toml": qualified} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Admission contract A: the managed home holds a read-only settings.json.
	if err := os.WriteFile(filepath.Join(f.home, "settings.json"), []byte("{}"), 0o400); err != nil {
		t.Fatal(err)
	}
}

// Through the real run path, a filtering-gateway launch checks health and
// registers before the child starts, activates after it, and unregisters
// after it exits. The child sees only the gateway's base URL and the token
// in its header; the token appears in no argv and no launcher output.
func TestGatewayLaunchLifecycleThroughRun(t *testing.T) {
	f := printNetworkFixture(t)
	g := newE2EGateway(t)
	writeGatewayCatalog(t, f, g.socket)
	f.args = withNetworkFlag(f.args, "egress-a")
	code, out, stderr := f.run()
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if calls := g.calls(); calls != "/v1/health /v1/register /v1/activate /v1/unregister" {
		t.Fatalf("gateway calls = %q", calls)
	}
	var child childCapture
	if err := json.Unmarshal(out, &child); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	for _, e := range child.Env {
		name, value, _ := strings.Cut(e, "=")
		env[name] = value
	}
	if env["ANTHROPIC_BASE_URL"] != "http://127.0.0.1:47811/anthropic" || env["ANTHROPIC_CUSTOM_HEADERS"] != "X-Netproxy-Token: "+e2eToken ||
		env["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"] != "1" {
		t.Fatalf("child env lacks the gateway settings: %v", env)
	}
	for _, name := range []string{"HTTPS_PROXY", "HTTP_PROXY", "https_proxy", "ALL_PROXY"} {
		if _, ok := env[name]; ok {
			t.Fatalf("child env carries %s", name)
		}
	}
	if strings.Contains(strings.Join(child.Argv, " "), e2eToken) || strings.Contains(string(stderr), e2eToken) {
		t.Fatal("the token reached argv or the launcher's output")
	}
	if !strings.Contains(string(stderr), "curator-run: network: profile=egress-a origin=explicit") {
		t.Fatalf("stderr lacks the provenance line: %s", stderr)
	}
}

// A refusal after registration and before spawn still ends the
// registration: no child runs, and the gateway sees unregister, never
// activate. The lifecycle test above is the positive control.
func TestGatewayRefusalAfterRegisterUnregisters(t *testing.T) {
	f := printNetworkFixture(t)
	g := newE2EGateway(t)
	writeGatewayCatalog(t, f, g.socket)
	f.deps.networkEmit = func(io.Writer, *adapterprobe.Provenance) error { return errors.New("emission failed") }
	f.args = withNetworkFlag(f.args, "egress-a")
	code, out, stderr := f.run()
	if code == 0 || len(out) != 0 {
		t.Fatalf("exit=%d out=%q stderr=%s", code, out, stderr)
	}
	if calls := g.calls(); calls != "/v1/health /v1/register /v1/unregister" {
		t.Fatalf("gateway calls = %q", calls)
	}
	f.assertNoChild(t)
}

// Without a qualified-builds entry the launch refuses before any gateway
// call (G-A), so no registration exists to leak.
func TestGatewayUnqualifiedBuildRefusesBeforeGateway(t *testing.T) {
	f := printNetworkFixture(t)
	g := newE2EGateway(t)
	writeGatewayCatalog(t, f, g.socket)
	if err := os.Remove(filepath.Join(f.dir, ".curator", "network-qualified.toml")); err != nil {
		t.Fatal(err)
	}
	f.args = withNetworkFlag(f.args, "egress-a")
	code, _, stderr := f.run()
	if code == 0 || !strings.Contains(string(stderr), "strict_miss") {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if calls := g.calls(); calls != "" {
		t.Fatalf("gateway calls = %q", calls)
	}
	f.assertNoChild(t)
}

// G-C through the real run path: with no --network, a filtering-gateway
// operator default binds the launch with origin operator-default; a
// default of another kind keeps today's unmanaged launch; a broken
// network.toml refuses instead of launching unfiltered.
func TestGatewayOperatorDefault(t *testing.T) {
	t.Run("gateway default binds", func(t *testing.T) {
		f := printNetworkFixture(t)
		g := newE2EGateway(t)
		writeGatewayCatalog(t, f, g.socket)
		path := filepath.Join(f.dir, ".curator", "network.toml")
		doc, _ := os.ReadFile(path)
		if err := os.WriteFile(path, []byte(strings.Replace(string(doc), "\n[networks.egress-a]", "\ndefault = \"egress-a\"\n[networks.egress-a]", 1)), 0o600); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := f.run()
		if code != 0 || !strings.Contains(string(stderr), "curator-run: network: profile=egress-a origin=operator-default") {
			t.Fatalf("exit=%d stderr=%s", code, stderr)
		}
		if calls := g.calls(); calls != "/v1/health /v1/register /v1/activate /v1/unregister" {
			t.Fatalf("gateway calls = %q", calls)
		}
	})
	t.Run("other default stays unmanaged", func(t *testing.T) {
		f := printNetworkFixture(t)
		writeNetworkCatalog(t, f.dir, true)
		path := filepath.Join(f.dir, ".curator", "network.toml")
		doc, _ := os.ReadFile(path)
		if err := os.WriteFile(path, []byte(strings.Replace(string(doc), "\n[networks.egress-a]", "\ndefault = \"egress-a\"\n[networks.egress-a]", 1)), 0o600); err != nil {
			t.Fatal(err)
		}
		f.deps.prober = &scriptedNetworkProber{}
		code, _, stderr := f.run()
		if code != 0 || strings.Contains(string(stderr), "curator-run: network:") {
			t.Fatalf("exit=%d stderr=%s", code, stderr)
		}
	})
	t.Run("broken file refuses", func(t *testing.T) {
		f := printNetworkFixture(t)
		writeBrokenNetworkCatalog(t, f.dir, "schema = [\n")
		code, _, stderr := f.run()
		if code == 0 || !strings.Contains(string(stderr), "network_file_unreadable") {
			t.Fatalf("exit=%d stderr=%s", code, stderr)
		}
		f.assertNoChild(t)
	})
}

// Through the real run path, a project settings file that sets the gateway's
// base URL refuses before any gateway call and starts no child; the
// lifecycle test is the valid control.
func TestGatewayProjectSettingsEnvRefuses(t *testing.T) {
	f := printNetworkFixture(t)
	g := newE2EGateway(t)
	writeGatewayCatalog(t, f, g.socket)
	if err := os.MkdirAll(filepath.Join(f.dir, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.dir, ".claude", "settings.json"), []byte(`{"env":{"ANTHROPIC_BASE_URL":"https://elsewhere.example"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	f.args = withNetworkFlag(f.args, "egress-a")
	code, _, stderr := f.run()
	if code == 0 || !strings.Contains(string(stderr), "network_configuration_conflict") {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if calls := g.calls(); calls != "" {
		t.Fatalf("gateway calls = %q", calls)
	}
	f.assertNoChild(t)
}

// Through the real run path, an interactive Claude launch (no print
// selector) binds the gateway when the qualified-builds file lists that
// exact build for the interactive entrypoint, and refuses before any
// gateway call when it lists the build for exec only.
func TestGatewayInteractiveClaudeThroughRun(t *testing.T) {
	for _, entry := range []string{"interactive", "exec"} {
		t.Run(entry, func(t *testing.T) {
			f := entryFixture(t, "claude_code")
			f.args = []string{"claude_code", "--permissions", "native"}
			g := newE2EGateway(t)
			writeGatewayCatalog(t, f, g.socket)
			path := filepath.Join(f.dir, ".curator", "network-qualified.toml")
			doc, _ := os.ReadFile(path)
			if err := os.WriteFile(path, []byte(strings.Replace(string(doc), `entrypoint = "exec"`, `entrypoint = "`+entry+`"`, 1)), 0o600); err != nil {
				t.Fatal(err)
			}
			f.args = withNetworkFlag(f.args, "egress-a")
			code, _, stderr := f.run()
			if entry == "exec" {
				if code == 0 || !strings.Contains(string(stderr), "strict_miss") || g.calls() != "" {
					t.Fatalf("exit=%d calls=%q stderr=%s", code, g.calls(), stderr)
				}
				f.assertNoChild(t)
				return
			}
			if code != 0 || g.calls() != "/v1/health /v1/register /v1/activate /v1/unregister" {
				t.Fatalf("exit=%d calls=%q stderr=%s", code, g.calls(), stderr)
			}
			if !strings.Contains(string(stderr), "entrypoint=interactive") {
				t.Fatalf("provenance lacks the interactive entrypoint: %s", stderr)
			}
		})
	}
}

// With the operator's default a filtering gateway and no --network, every
// hosted route refuses network_scope_unsupported before any session
// manager call and without a native start: explicit hosted, a hosted
// resume, and the auto route with a selected manager. A catalog that
// exists but cannot be read refuses every route alike. Real built binary;
// the hosted front-door tests without a gateway default are the controls.
func TestGatewayDefaultRefusesHostedRoutes(t *testing.T) {
	bin := frontBuild(t)
	gateway := "schema = \"relux-network-profiles-v1\"\ndefault = \"gw\"\n[networks.gw]\nkind = \"filtering-gateway\"\ncontrol = \"/tmp/absent-gw.sock\"\nsensitive_egress = true\n"
	for _, tc := range []struct {
		name, doc, diag string
		args            []string
		code            int
	}{
		{"hosted", gateway, "network_scope_unsupported", []string{"--hosted"}, 16},
		{"hosted resume", gateway, "network_scope_unsupported", []string{"--hosted", "resume"}, 16},
		{"auto with a selected manager", gateway, "network_scope_unsupported", nil, 16},
		{"broken catalog", "schema = [\n", "network_file_unreadable", []string{"--hosted", "resume"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFront(t, bin)
			f.policy("auto", true)
			f.registry("selected")
			if err := os.MkdirAll(filepath.Join(f.root, ".curator"), 0o700); err != nil {
				t.Fatal(err)
			}
			writeFixture(t, filepath.Join(f.root, ".curator", "network.toml"), []byte(tc.doc), 0o600)
			code, _, stderr := f.invoke(true, false, tc.args...)
			f.require(t, code, tc.code, stderr, tc.diag, nil, 0, false)
		})
	}
}

// A dangling ~/.curator/network.toml symlink is a broken catalog, not an
// absent one: the launch refuses with no child, instead of running without
// the operator's default filter. Genuine absence is the control in every
// other fixture here.
func TestGatewayDanglingCatalogRefusesThroughRun(t *testing.T) {
	f := printNetworkFixture(t)
	if err := os.MkdirAll(filepath.Join(f.dir, ".curator"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(f.dir, "gone.toml"), filepath.Join(f.dir, ".curator", "network.toml")); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := f.run()
	if code == 0 || !strings.Contains(string(stderr), "network_file_unreadable") {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	f.assertNoChild(t)
}

// A signal that arrives after registration and before the child starts
// aborts the launch at the boundary: no child, no activation, and the
// registration is ended.
func TestGatewayInterruptBeforeStartUnregisters(t *testing.T) {
	f := printNetworkFixture(t)
	g := newE2EGateway(t)
	writeGatewayCatalog(t, f, g.socket)
	f.deps.leaseNotify = func(c chan<- os.Signal, _ ...os.Signal) { c <- os.Interrupt }
	f.args = withNetworkFlag(f.args, "egress-a")
	code, _, stderr := f.run()
	if code == 0 || !strings.Contains(string(stderr), "interrupted before the runtime started") {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if calls := g.calls(); calls != "/v1/health /v1/register /v1/unregister" {
		t.Fatalf("gateway calls = %q", calls)
	}
	f.assertNoChild(t)
}

// The settings check repeats at the pre-spawn boundary: a project settings
// file that appears after preparation (written here from the provenance
// emission hook, which runs after registration) refuses the launch there,
// ends the registration and starts no child.
func TestGatewayBoundaryRechecksSettings(t *testing.T) {
	f := printNetworkFixture(t)
	g := newE2EGateway(t)
	writeGatewayCatalog(t, f, g.socket)
	f.deps.networkEmit = func(w io.Writer, p *adapterprobe.Provenance) error {
		if err := os.MkdirAll(filepath.Join(f.dir, ".claude"), 0o700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(f.dir, ".claude", "settings.local.json"), []byte(`{"env":{"ANTHROPIC_BASE_URL":"https://late.example"}}`), 0o600)
	}
	f.args = withNetworkFlag(f.args, "egress-a")
	code, _, stderr := f.run()
	if code == 0 || !strings.Contains(string(stderr), "network_configuration_conflict") {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if calls := g.calls(); calls != "/v1/health /v1/register /v1/unregister" {
		t.Fatalf("gateway calls = %q", calls)
	}
	f.assertNoChild(t)
}
