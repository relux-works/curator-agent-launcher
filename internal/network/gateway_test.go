package network_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/network"
	"github.com/relux-works/curator-network-profiles/pkg/netprofile"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
)

const gwToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// gateway is an in-process netproxy-control/1 service on a unix socket.
// Each path answers from its handler; every request is recorded.
type gateway struct {
	socket string
	mu     sync.Mutex
	paths  []string
	bodies []string
	answer map[string]func(http.ResponseWriter)
}

func ok(status int, body string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Netproxy-Control-Api", "netproxy-control/1")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}
}

const gwRegister = `{"api":"netproxy-control/1","token":"` + gwToken + `","token_header":"X-Netproxy-Token",` +
	`"anthropic_base":"http://127.0.0.1:47811/anthropic","openai_base":"http://127.0.0.1:47811/openai/v1",` +
	`"chatgpt_base":"http://127.0.0.1:47811/chatgpt/backend-api/codex","activation_seconds":120}`

func newGateway(t *testing.T) *gateway {
	t.Helper()
	dir, err := os.MkdirTemp("", "gw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	g := &gateway{socket: filepath.Join(dir, "control.sock"), answer: map[string]func(http.ResponseWriter){
		"/v1/health":     ok(200, `{"api":"netproxy-control/1","ready":true}`),
		"/v1/register":   ok(200, gwRegister),
		"/v1/activate":   ok(200, `{"api":"netproxy-control/1","active":true}`),
		"/v1/renew":      ok(200, `{"api":"netproxy-control/1","active":true}`),
		"/v1/unregister": ok(204, ""),
	}}
	ln, err := net.Listen("unix", g.socket)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		g.mu.Lock()
		g.paths = append(g.paths, r.URL.Path)
		g.bodies = append(g.bodies, string(body))
		answer := g.answer[r.URL.Path]
		g.mu.Unlock()
		answer(w)
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return g
}

func (g *gateway) calls() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.paths...)
}

// gatewayHome writes a confirmed filtering-gateway catalog naming the
// gateway's socket and, when qualify is set, a qualified-builds file
// listing the unit artifact.
func gatewayHome(t *testing.T, g *gateway, qualify bool) string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".curator")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	doc := "schema = \"relux-network-profiles-v1\"\n[networks.egress-a]\nkind = \"filtering-gateway\"\ncontrol = \"" + g.socket + "\"\nsensitive_egress = true\n"
	file, err := netprofile.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	prof, _ := file.Lookup("egress-a")
	ledger, _ := json.Marshal(map[string]any{"schema": "relux-network-confirmations-v1",
		"confirmed": map[string]any{"egress-a": map[string]any{"digest": netprofile.Digest(prof), "confirmed_at": "2026-10-30T00:00:00Z"}}})
	writeTestFile(t, filepath.Join(dir, "network.toml"), doc)
	writeTestFile(t, filepath.Join(dir, "network.confirmations.json"), string(ledger))
	if qualify {
		sum := sha256.Sum256(unitArtifact)
		writeTestFile(t, filepath.Join(dir, network.QualifiedFileName), qualifiedDoc(hex.EncodeToString(sum[:])))
	}
	return home
}

func qualifiedDoc(sha string) string {
	return "schema = \"curator-network-qualified-v1\"\n\n[[builds]]\nharness = \"claude-code\"\nentrypoint = \"exec\"\nbinary_sha256 = \"" + sha +
		"\"\nplatform = \"" + runtime.GOOS + "\"\nevidence = \"unit fixture\"\nqualified_on = \"2026-10-30\"\n"
}

func writeTestFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func gatewayRequest(home string) network.Request {
	req := baseRequest(home)
	req.ProjectRoot = "/Users/op/project"
	req.SessionID = "env-20261030T160000Z-42"
	req.GatewayTimeout = 300 * time.Millisecond
	return req
}

// Positive control (conformance 1, 2): a qualified Claude Code exec launch
// checks health and registers before spawn, and its patch carries exactly
// the reply's token-free base and the token in its header, in the child's
// launch-private environment only. The Record never holds the token, and
// the lease unregisters at Finish.
func TestPrepareGatewayBindsQualifiedClaude(t *testing.T) {
	g := newGateway(t)
	req := gatewayRequest(gatewayHome(t, g, true))
	req.Prober = forbiddenProber{t}
	got, err := network.Prepare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if calls := g.calls(); strings.Join(calls, " ") != "/v1/health /v1/register" {
		t.Fatalf("calls = %v", calls)
	}
	if g.bodies[1] != `{"root":"/Users/op/project","session":"env-20261030T160000Z-42","runtime":"claude-code"}` {
		t.Fatalf("register body = %s", g.bodies[1])
	}
	set := map[string]string{}
	for _, kv := range got.Patch.Set {
		set[kv.Name] = kv.Value
	}
	if len(set) != 3 || set["ANTHROPIC_BASE_URL"] != "http://127.0.0.1:47811/anthropic" ||
		set["ANTHROPIC_CUSTOM_HEADERS"] != "X-Netproxy-Token: "+gwToken || set["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"] != "1" {
		t.Fatalf("set = %v", set)
	}
	unset := strings.Join(got.Patch.Unset, " ")
	for _, name := range []string{"HTTPS_PROXY", "https_proxy", "ANTHROPIC_BASE_URL", "ANTHROPIC_CUSTOM_HEADERS"} {
		if !strings.Contains(" "+unset+" ", " "+name+" ") {
			t.Fatalf("unset lacks %s: %s", name, unset)
		}
	}
	record, _ := json.Marshal(got.Record)
	if got.Record == nil || strings.Contains(string(record), gwToken) || strings.Contains(network.ProvenanceLine(*got.Record), gwToken) {
		t.Fatalf("record = %s", record)
	}
	if got.Lease == nil || strings.Contains(got.Lease.String(), gwToken) {
		t.Fatal("lease missing or renders the token")
	}
	if err := got.Lease.Finish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls := g.calls(); calls[len(calls)-1] != "/v1/unregister" {
		t.Fatalf("calls = %v", calls)
	}
}

// Every refusal below happens before spawn, leaves no lease and never
// renders the token; the case says how far the gateway was reached. The
// positive control is TestPrepareGatewayBindsQualifiedClaude.
func TestPrepareGatewayRefusals(t *testing.T) {
	for _, tc := range []struct {
		name    string
		qualify bool
		edit    func(t *testing.T, g *gateway, home string, req *network.Request)
		code    string
		calls   string
	}{
		{"unqualified build (G-A)", false, nil, refusal.CodeScopeUnsupported, ""},
		{"qualified list for another build", false, func(t *testing.T, g *gateway, home string, req *network.Request) {
			writeTestFile(t, filepath.Join(home, ".curator", network.QualifiedFileName), qualifiedDoc(strings.Repeat("a", 64)))
		}, refusal.CodeScopeUnsupported, ""},
		{"malformed qualified list", false, func(t *testing.T, g *gateway, home string, req *network.Request) {
			writeTestFile(t, filepath.Join(home, ".curator", network.QualifiedFileName), "schema = \"curator-network-qualified-v1\"\n[[builds]]\nharness = \"claude-code\"\n")
		}, refusal.CodeProfileInvalid, ""},
		{"overlay sets the base URL", true, func(t *testing.T, g *gateway, home string, req *network.Request) {
			req.FragEnv = map[string]string{"ANTHROPIC_BASE_URL": "https://api.anthropic.com"}
		}, refusal.CodeConfigurationConflict, ""},
		{"overlay sets custom headers", true, func(t *testing.T, g *gateway, home string, req *network.Request) {
			req.PromptEnv = map[string]string{"ANTHROPIC_CUSTOM_HEADERS": "X: y"}
		}, refusal.CodeConfigurationConflict, ""},
		{"relative project root", true, func(t *testing.T, g *gateway, home string, req *network.Request) {
			req.ProjectRoot = "project"
		}, refusal.CodeScopeUnsupported, ""},
		{"service down", true, func(t *testing.T, g *gateway, home string, req *network.Request) {
			os.Remove(g.socket)
		}, refusal.CodeProxyUnreachable, ""},
		{"health 503 store_unavailable", true, func(t *testing.T, g *gateway, home string, req *network.Request) {
			g.answer["/v1/health"] = ok(503, `{"api":"netproxy-control/1","ready":false,"error":"store_unavailable"}`)
		}, refusal.CodeProxyUnreachable, "/v1/health"},
		{"health 503 persist_failed", true, func(t *testing.T, g *gateway, home string, req *network.Request) {
			g.answer["/v1/health"] = ok(503, `{"api":"netproxy-control/1","ready":false,"error":"persist_failed"}`)
		}, refusal.CodeProxyUnreachable, "/v1/health"},
		{"health without version header", true, func(t *testing.T, g *gateway, home string, req *network.Request) {
			g.answer["/v1/health"] = func(w http.ResponseWriter) { io.WriteString(w, `{"api":"netproxy-control/1","ready":true}`) }
		}, refusal.CodeProxyUnreachable, "/v1/health"},
		{"register bad_root", true, func(t *testing.T, g *gateway, home string, req *network.Request) {
			g.answer["/v1/register"] = ok(400, `{"api":"netproxy-control/1","error":"bad_root"}`)
		}, refusal.CodeProxyUnreachable, "/v1/health /v1/register"},
		{"register config_invalid", true, func(t *testing.T, g *gateway, home string, req *network.Request) {
			g.answer["/v1/register"] = ok(422, `{"api":"netproxy-control/1","error":"config_invalid"}`)
		}, refusal.CodeProxyUnreachable, "/v1/health /v1/register"},
		{"register store_unavailable", true, func(t *testing.T, g *gateway, home string, req *network.Request) {
			g.answer["/v1/register"] = ok(503, `{"api":"netproxy-control/1","error":"store_unavailable"}`)
		}, refusal.CodeProxyUnreachable, "/v1/health /v1/register"},
		{"register reply without token_header", true, func(t *testing.T, g *gateway, home string, req *network.Request) {
			g.answer["/v1/register"] = ok(200, strings.Replace(gwRegister, `"token_header":"X-Netproxy-Token",`, "", 1))
		}, refusal.CodeProxyUnreachable, "/v1/health /v1/register"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			home := gatewayHome(t, g, tc.qualify)
			req := gatewayRequest(home)
			req.Prober = forbiddenProber{t}
			if tc.edit != nil {
				tc.edit(t, g, home, &req)
			}
			got, err := network.Prepare(context.Background(), req)
			if err == nil || refusalCode(t, err) != tc.code {
				t.Fatalf("err = %v, want %s", err, tc.code)
			}
			if got.Lease != nil || !got.Patch.Empty() {
				t.Fatalf("refusal left a lease or patch: %+v", got)
			}
			if strings.Contains(err.Error(), gwToken) {
				t.Fatal("refusal renders the token")
			}
			if calls := strings.Join(g.calls(), " "); calls != tc.calls {
				t.Fatalf("gateway calls = %q, want %q", calls, tc.calls)
			}
			if tc.name == "register config_invalid" && !strings.Contains(err.Error(), "configuration is invalid") {
				t.Fatalf("config_invalid detail = %v", err)
			}
		})
	}
}

// G-C: only an operator default that names a filtering gateway selects
// without --network. No HOME, no file, no default and a default of another
// kind keep today's unmanaged launch; a file that exists but does not
// parse refuses instead of dropping the filter.
func TestDefaultGatewaySelected(t *testing.T) {
	g := newGateway(t)
	write := func(doc string) []string {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".curator"), 0o700); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(home, ".curator", "network.toml"), doc)
		return []string{"HOME=" + home}
	}
	gw := "[networks.gw]\nkind = \"filtering-gateway\"\ncontrol = \"" + g.socket + "\"\nsensitive_egress = true\n"
	ext := "[networks.ext]\nkind = \"external-http-proxy\"\nendpoint = \"http://127.0.0.1:3128\"\nbypass_hosts = [\"localhost\", \"127.0.0.1\", \"::1\"]\n"
	head := "schema = \"relux-network-profiles-v1\"\n"
	for _, tc := range []struct {
		name string
		env  []string
		want bool
		err  bool
	}{
		{"no HOME", []string{"PATH=/usr/bin"}, false, false},
		{"no file", []string{"HOME=" + t.TempDir()}, false, false},
		{"no default", write(head + gw), false, false},
		{"external default", write(head + "default = \"ext\"\n" + ext + gw), false, false},
		{"gateway default", write(head + "default = \"gw\"\n" + ext + gw), true, false},
		{"broken file", write("schema = [\n"), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := network.DefaultGatewaySelected(tc.env)
			if got != tc.want || (err != nil) != tc.err {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}
}

// A Claude Code settings source that names a gateway variable in `env`
// refuses before any gateway call: Claude applies settings env into its
// process environment, so the entry could replace the base URL or the
// token header. Harmless settings, the same names after `--`, and absent
// files are the valid controls.
func TestPrepareGatewaySettingsEnvConflict(t *testing.T) {
	bad := `{"env":{"ANTHROPIC_BASE_URL":"https://elsewhere.example"}}`
	for _, tc := range []struct {
		name   string
		setup  func(t *testing.T, req *network.Request)
		refuse bool
	}{
		{"harmless user settings", func(t *testing.T, req *network.Request) {
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, "settings.json"), `{"env":{"DISABLE_AUTOUPDATER":"1"},"model":"x"}`)
			req.ChildEnv = []string{"CLAUDE_CONFIG_DIR=" + dir}
		}, false},
		{"user settings in CLAUDE_CONFIG_DIR", func(t *testing.T, req *network.Request) {
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, "settings.json"), bad)
			req.ChildEnv = []string{"CLAUDE_CONFIG_DIR=" + dir}
		}, true},
		{"user settings under HOME", func(t *testing.T, req *network.Request) {
			home := t.TempDir()
			os.MkdirAll(filepath.Join(home, ".claude"), 0o700)
			writeTestFile(t, filepath.Join(home, ".claude", "settings.json"), `{"env":{"anthropic_custom_headers":"X: y"}}`)
			req.ChildEnv = []string{"HOME=" + home}
		}, true},
		{"project settings.json", func(t *testing.T, req *network.Request) {
			root := t.TempDir()
			os.MkdirAll(filepath.Join(root, ".claude"), 0o700)
			writeTestFile(t, filepath.Join(root, ".claude", "settings.json"), bad)
			req.ProjectRoot = root
		}, true},
		{"project settings.local.json", func(t *testing.T, req *network.Request) {
			root := t.TempDir()
			os.MkdirAll(filepath.Join(root, ".claude"), 0o700)
			writeTestFile(t, filepath.Join(root, ".claude", "settings.local.json"), `{"env":{"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC":"0"}}`)
			req.ProjectRoot = root
		}, true},
		{"undecodable project settings", func(t *testing.T, req *network.Request) {
			root := t.TempDir()
			os.MkdirAll(filepath.Join(root, ".claude"), 0o700)
			writeTestFile(t, filepath.Join(root, ".claude", "settings.json"), `{"env":`)
			req.ProjectRoot = root
		}, true},
		{"inline --settings", func(t *testing.T, req *network.Request) {
			req.NativeArgs = []string{"--settings", bad, "-p", "hi"}
		}, true},
		{"--settings= file", func(t *testing.T, req *network.Request) {
			path := filepath.Join(t.TempDir(), "s.json")
			writeTestFile(t, path, bad)
			req.NativeArgs = []string{"--settings=" + path}
		}, true},
		{"settings text after --", func(t *testing.T, req *network.Request) {
			req.NativeArgs = []string{"-p", "--", "--settings", bad}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			tc.setup(t, &req)
			got, err := network.Prepare(context.Background(), req)
			if !tc.refuse {
				if err != nil {
					t.Fatalf("valid control refused: %v", err)
				}
				if got.Lease != nil {
					got.Lease.Finish(context.Background())
				}
				return
			}
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || got.Lease != nil {
				t.Fatalf("err = %v", err)
			}
			if calls := g.calls(); len(calls) != 0 {
				t.Fatalf("gateway calls = %v", calls)
			}
		})
	}
}
