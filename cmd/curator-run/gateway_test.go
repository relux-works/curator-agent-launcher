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
	qualified := "schema = \"curator-network-qualified-v1\"\n[[builds]]\nharness = \"claude-code\"\nentrypoint = \"exec\"\nbinary_sha256 = \"" + sha +
		"\"\nplatform = \"" + runtime.GOOS + "\"\nevidence = \"test fixture\"\nqualified_on = \"2026-10-30\"\n"
	for name, data := range map[string]string{"network.toml": doc, "network.confirmations.json": string(ledger), "network-qualified.toml": qualified} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
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
