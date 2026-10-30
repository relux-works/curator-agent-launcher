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
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/network"
	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
	"github.com/relux-works/curator-network-profiles/pkg/netprofile"
	"github.com/relux-works/curator-network-profiles/pkg/probe"
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
	return "schema = \"curator-network-qualified-v1\"\nproject_roots = [\"/\"]\n\n[[builds]]\nharness = \"claude-code\"\nentrypoint = \"exec\"\nbinary_sha256 = \"" + sha +
		"\"\nplatform = \"" + runtime.GOOS + "\"\nevidence = \"unit fixture\"\nqualified_on = \"2026-10-30\"\n"
}

func writeTestFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

// readOnly makes a settings file read-only, as admission contract A requires.
func readOnly(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
}

// gatewayRequest satisfies admission contract A: a managed home with a
// read-only settings.json, an existing project directory, an empty managed
// preferences directory and a fixed user name.
func gatewayRequest(home string) network.Request {
	req := baseRequest(home)
	managed := filepath.Join(home, "managed-claude")
	project := filepath.Join(home, "project")
	os.MkdirAll(managed, 0o700)
	os.MkdirAll(project, 0o700)
	if err := os.WriteFile(filepath.Join(managed, "settings.json"), []byte("{}"), 0o400); err != nil && !os.IsPermission(err) {
		panic(err)
	}
	req.ChildEnv = []string{"CLAUDE_CONFIG_DIR=" + managed}
	req.ManagedPreferencesDir = filepath.Join(home, "managed-preferences")
	req.Username = "tester"
	req.ProjectRoot = project
	req.SessionID = "env-20261030T160000Z-42"
	req.GatewayTimeout = 300 * time.Millisecond
	req.ManagedSettingsDir = filepath.Join(home, "managed-settings-absent")
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
	if g.bodies[1] != `{"root":"`+req.ProjectRoot+`","session":"env-20261030T160000Z-42","runtime":"claude-code"}` {
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
			readOnly(t, filepath.Join(dir, "settings.json"))
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

// Under a filtering gateway, Claude Code's interactive entrypoint binds
// only through an exact qualified build for that entrypoint: an exec entry
// for the same binary does not qualify it, and without the gateway the
// interactive shape stays outside the ceiling (TestNetworkInteractive*).
func TestPrepareGatewayInteractiveNeedsItsOwnEntry(t *testing.T) {
	sum := sha256.Sum256(unitArtifact)
	sha := hex.EncodeToString(sum[:])
	for _, tc := range []struct {
		name       string
		entrypoint string
		bind       bool
	}{
		{"interactive entry", "interactive", true},
		{"exec entry only", "exec", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			home := gatewayHome(t, g, false)
			writeTestFile(t, filepath.Join(home, ".curator", network.QualifiedFileName), strings.Replace(qualifiedDoc(sha), `entrypoint = "exec"`, `entrypoint = "`+tc.entrypoint+`"`, 1))
			req := gatewayRequest(home)
			req.Entrypoint = network.EntrypointInteractive
			req.HostIdentity.Entrypoint = network.EntrypointInteractive
			req.Prober = forbiddenProber{t}
			got, err := network.Prepare(context.Background(), req)
			if !tc.bind {
				if err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported || len(g.calls()) != 0 {
					t.Fatalf("err = %v calls = %v", err, g.calls())
				}
				return
			}
			if err != nil || got.Lease == nil {
				t.Fatalf("err = %v", err)
			}
			if err := got.Lease.Finish(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Round 2 of the settings check: managed settings and their drop-ins,
// ancestor .claude discovery, provider-route and proxy names, the flags that
// move settings sources, and a dangling settings link all refuse before any
// gateway call; an absent managed directory is the valid control (every
// case of TestPrepareGatewaySettingsEnvConflict passes it).
func TestPrepareGatewaySettingsScopeRound2(t *testing.T) {
	route := `{"env":{"CLAUDE_CODE_USE_BEDROCK":"1"}}`
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, req *network.Request)
	}{
		{"managed settings file", func(t *testing.T, req *network.Request) {
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, "managed-settings.json"), `{"env":{"ANTHROPIC_BASE_URL":"https://x.example"}}`)
			req.ManagedSettingsDir = dir
		}},
		{"managed drop-in", func(t *testing.T, req *network.Request) {
			dir := t.TempDir()
			os.MkdirAll(filepath.Join(dir, "managed-settings.d"), 0o700)
			writeTestFile(t, filepath.Join(dir, "managed-settings.d", "10-route.json"), route)
			req.ManagedSettingsDir = dir
		}},
		{"ancestor project settings", func(t *testing.T, req *network.Request) {
			root := t.TempDir()
			os.MkdirAll(filepath.Join(root, ".claude"), 0o700)
			os.MkdirAll(filepath.Join(root, "a", "b"), 0o700)
			writeTestFile(t, filepath.Join(root, ".claude", "settings.json"), route)
			req.ProjectRoot = filepath.Join(root, "a", "b")
		}},
		{"provider route in user settings", func(t *testing.T, req *network.Request) {
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, "settings.json"), `{"env":{"anthropic_vertex_base_url":"https://v.example"}}`)
			req.ChildEnv = []string{"CLAUDE_CONFIG_DIR=" + dir}
		}},
		{"proxy in user settings", func(t *testing.T, req *network.Request) {
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, "settings.json"), `{"env":{"HTTPS_PROXY":"http://127.0.0.1:9"}}`)
			req.ChildEnv = []string{"CLAUDE_CONFIG_DIR=" + dir}
		}},
		{"dangling user settings link", func(t *testing.T, req *network.Request) {
			dir := t.TempDir()
			if err := os.Symlink(filepath.Join(dir, "gone.json"), filepath.Join(dir, "settings.json")); err != nil {
				t.Fatal(err)
			}
			req.ChildEnv = []string{"CLAUDE_CONFIG_DIR=" + dir}
		}},
		{"--setting-sources", func(t *testing.T, req *network.Request) {
			req.NativeArgs = []string{"--setting-sources", "user", "-p", "x"}
		}},
		{"--managed-settings=", func(t *testing.T, req *network.Request) { req.NativeArgs = []string{"--managed-settings=/tmp/m.json"} }},
		{"--project-config-root", func(t *testing.T, req *network.Request) { req.NativeArgs = []string{"--project-config-root", "/tmp"} }},
		{"overlay route selector", func(t *testing.T, req *network.Request) {
			req.FragEnv = map[string]string{"CLAUDE_CODE_USE_VERTEX": "1"}
		}},
		{"MCP lookup of a route endpoint", func(t *testing.T, req *network.Request) {
			req.MCPEnvNames = []string{"AWS_ENDPOINT_URL_BEDROCK_RUNTIME"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			tc.setup(t, &req)
			got, err := network.Prepare(context.Background(), req)
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || got.Lease != nil || len(g.calls()) != 0 {
				t.Fatalf("err = %v calls = %v", err, g.calls())
			}
		})
	}
}

// Inherited provider-route selectors and endpoints are removed from the
// child, so a parent environment cannot route the runtime around the
// gateway; the gateway's own three names are set.
func TestPrepareGatewayUnsetsRouteNames(t *testing.T) {
	g := newGateway(t)
	req := gatewayRequest(gatewayHome(t, g, true))
	req.Prober = forbiddenProber{t}
	got, err := network.Prepare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Lease.Finish(context.Background())
	unset := " " + strings.Join(got.Patch.Unset, " ") + " "
	for _, n := range []string{"CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY", "CLAUDE_CODE_USE_GATEWAY", "ANTHROPIC_API_URL", "ANTHROPIC_BEDROCK_BASE_URL", "AWS_ENDPOINT_URL_BEDROCK_RUNTIME"} {
		if !strings.Contains(unset, " "+n+" ") {
			t.Fatalf("patch does not unset %s", n)
		}
	}
	if got.Recheck == nil || got.Recheck() != nil {
		t.Fatal("recheck missing or refusing a clean launch")
	}
}

// The pre-spawn recheck sees a settings file written after preparation.
func TestPrepareGatewayRecheckSeesLateSettings(t *testing.T) {
	g := newGateway(t)
	req := gatewayRequest(gatewayHome(t, g, true))
	req.Prober = forbiddenProber{t}
	root := t.TempDir()
	req.ProjectRoot = root
	got, err := network.Prepare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Lease.Finish(context.Background())
	os.MkdirAll(filepath.Join(root, ".claude"), 0o700)
	writeTestFile(t, filepath.Join(root, ".claude", "settings.local.json"), `{"env":{"ANTHROPIC_CUSTOM_HEADERS":"X: y"}}`)
	if err := got.Recheck(); err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict {
		t.Fatalf("recheck = %v", err)
	}
}

// A network.toml entry that is present but unusable (a dangling symlink)
// refuses instead of reading as absent; genuine absence and a valid default
// are the controls in TestDefaultGatewaySelected.
func TestDefaultGatewayDanglingCatalogRefuses(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".curator"), 0o700)
	if err := os.Symlink(filepath.Join(home, "gone.toml"), filepath.Join(home, ".curator", "network.toml")); err != nil {
		t.Fatal(err)
	}
	got, err := network.DefaultGatewaySelected([]string{"HOME=" + home})
	if got || err == nil || refusalCode(t, err) != refusal.CodeFileUnreadable {
		t.Fatalf("got %v, %v", got, err)
	}
}

// Explicit strict policy on a non-sensitive profile consults the operator's
// qualified builds: a matching entry admits, another build refuses
// strict_miss, an unreadable list refuses; optimistic policy never reads the
// list (an unreadable one does not matter).
func TestStrictPolicyReadsQualifiedOnNonSensitive(t *testing.T) {
	sum := sha256.Sum256(unitArtifact)
	match := qualifiedDoc(hex.EncodeToString(sum[:]))
	other := qualifiedDoc(strings.Repeat("b", 64))
	for _, tc := range []struct {
		name   string
		mode   adapterprobe.Mode
		list   string // "" none; "dir" makes it unreadable
		refuse bool
	}{
		{"strict matching entry", adapterprobe.Strict, match, false},
		{"strict other build", adapterprobe.Strict, other, true},
		{"strict unreadable list", adapterprobe.Strict, "dir", true},
		{"optimistic ignores the list", adapterprobe.Optimistic, "dir", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			endpoint := writeCatalogEndpoint(t, home)
			path := filepath.Join(home, ".curator", network.QualifiedFileName)
			switch tc.list {
			case "dir":
				os.MkdirAll(path, 0o700)
			case "":
			default:
				writeTestFile(t, path, tc.list)
			}
			req := baseRequest(home)
			req.Mode = tc.mode
			req.Prober = &scriptedProber{res: okResult(endpoint)}
			_, err := network.Prepare(context.Background(), req)
			if (err != nil) != tc.refuse {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// writeCatalogEndpoint stores the confirmed external-proxy fixture catalog
// and returns its endpoint.
func writeCatalogEndpoint(t *testing.T, home string) string {
	t.Helper()
	prof, _ := writeCatalog(t, home, true)
	return prof.Endpoint
}

func okResult(endpoint string) probe.Result {
	return probe.Result{Endpoint: endpoint, TCP: probe.StatusOK, Connect: probe.StatusSkipped, TLS: probe.StatusSkipped, CheckedAt: time.Now()}
}

// The activation window counts from when the register request started: a
// register reply that arrives after a one-second window leaves a lease that
// ends the token without any activate attempt.
func TestPrepareGatewayWindowStartsAtRegisterRequest(t *testing.T) {
	g := newGateway(t)
	g.answer["/v1/register"] = func(w http.ResponseWriter) {
		time.Sleep(1200 * time.Millisecond)
		ok(200, strings.Replace(gwRegister, `"activation_seconds":120`, `"activation_seconds":1`, 1))(w)
	}
	req := gatewayRequest(gatewayHome(t, g, true))
	req.GatewayTimeout = 5 * time.Second
	req.Prober = forbiddenProber{t}
	got, err := network.Prepare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	got.Lease.Started(context.Background())
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(strings.Join(g.calls(), " "), "/v1/unregister") {
		if time.Now().After(deadline) {
			t.Fatalf("calls = %v", g.calls())
		}
		time.Sleep(10 * time.Millisecond)
	}
	got.Lease.Finish(context.Background())
	if strings.Contains(strings.Join(g.calls(), " "), "/v1/activate") {
		t.Fatalf("activate attempted after the window: %v", g.calls())
	}
}

// Round 3, F1–F4 and F6: every source and selector the round-2 review named
// refuses before any gateway call, and each refusal has a nearby admitted
// control in the same table (the rows marked admit).
func TestPrepareGatewayRound3Sources(t *testing.T) {
	harmless := `{"env":{"DISABLE_AUTOUPDATER":"1","ANTHROPIC_MODEL":"claude-x","ANTHROPIC_API_KEY_HELPER_TTL_MS":"1"}}`
	route := func(name string) string { return `{"env":{"` + name + `":"x"}}` }
	for _, tc := range []struct {
		name  string
		admit bool
		setup func(t *testing.T, req *network.Request)
	}{
		{"harmless user settings", true, func(t *testing.T, req *network.Request) {
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, "settings.json"), harmless)
			readOnly(t, filepath.Join(dir, "settings.json"))
			req.ChildEnv = []string{"CLAUDE_CONFIG_DIR=" + dir}
		}},
		{"F1 unix socket in user settings", false, func(t *testing.T, req *network.Request) {
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, "settings.json"), route("ANTHROPIC_UNIX_SOCKET"))
			req.ChildEnv = []string{"CLAUDE_CONFIG_DIR=" + dir}
		}},
		{"F1 api base in an overlay", false, func(t *testing.T, req *network.Request) {
			req.FragEnv = map[string]string{"CLAUDE_CODE_API_BASE_URL": "http://x"}
		}},
		{"F1 bridge url in project settings", false, func(t *testing.T, req *network.Request) {
			root := t.TempDir()
			os.MkdirAll(filepath.Join(root, ".claude"), 0o700)
			writeTestFile(t, filepath.Join(root, ".claude", "settings.json"), route("CLAUDE_BRIDGE_BASE_URL"))
			req.ProjectRoot = root
		}},
		{"F2 cowork settings", false, func(t *testing.T, req *network.Request) {
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, "settings.json"), harmless)
			writeTestFile(t, filepath.Join(dir, "cowork_settings.json"), route("ANTHROPIC_BASE_URL"))
			req.ChildEnv = []string{"CLAUDE_CONFIG_DIR=" + dir}
		}},
		{"F2 cowork selector in an overlay", false, func(t *testing.T, req *network.Request) {
			req.PromptEnv = map[string]string{"CLAUDE_CODE_USE_COWORK_PLUGINS": "1"}
		}},
		{"F2 legacy .config.json", false, func(t *testing.T, req *network.Request) {
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, ".config.json"), route("ANTHROPIC_CUSTOM_HEADERS"))
			req.ChildEnv = []string{"CLAUDE_CONFIG_DIR=" + dir}
		}},
		{"F2 legacy HOME/.claude.json", false, func(t *testing.T, req *network.Request) {
			home := t.TempDir()
			writeTestFile(t, filepath.Join(home, ".claude.json"), `{"projects":{},"env":{"ANTHROPIC_BASE_URL":"https://x"}}`)
			req.ChildEnv = []string{"HOME=" + home}
		}},
		{"F2 large harmless .claude.json", true, func(t *testing.T, req *network.Request) {
			managed := strings.TrimPrefix(req.ChildEnv[0], "CLAUDE_CONFIG_DIR=")
			writeTestFile(t, filepath.Join(managed, ".claude.json"), `{"projects":{"pad":"`+strings.Repeat("x", 2<<20)+`"}}`)
		}},
		{"F3 managed policy helpers", false, func(t *testing.T, req *network.Request) {
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, "managed-settings.json"), `{"policyHelpers":{"default":{"defaultSettings":{"env":{"ANTHROPIC_BASE_URL":"https://x"}}}}}`)
			req.ManagedSettingsDir = dir
		}},
		{"F3 managed policy helper in a drop-in", false, func(t *testing.T, req *network.Request) {
			dir := t.TempDir()
			os.MkdirAll(filepath.Join(dir, "managed-settings.d"), 0o700)
			writeTestFile(t, filepath.Join(dir, "managed-settings.d", "10.json"), `{"policyHelper":{"command":"/bin/true"}}`)
			req.ManagedSettingsDir = dir
		}},
		{"F3 harmless managed settings", true, func(t *testing.T, req *network.Request) {
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, "managed-settings.json"), harmless)
			req.ManagedSettingsDir = dir
		}},
		{"F4 physical ancestor behind a symlink", false, func(t *testing.T, req *network.Request) {
			real := t.TempDir()
			os.MkdirAll(filepath.Join(real, "repo", ".claude"), 0o700)
			os.MkdirAll(filepath.Join(real, "repo", "sub"), 0o700)
			writeTestFile(t, filepath.Join(real, "repo", ".claude", "settings.json"), route("ANTHROPIC_BASE_URL"))
			link := filepath.Join(t.TempDir(), "shortcut")
			if err := os.Symlink(filepath.Join(real, "repo", "sub"), link); err != nil {
				t.Fatal(err)
			}
			req.ProjectRoot = link
		}},
		{"F4 worktree", false, func(t *testing.T, req *network.Request) { req.NativeArgs = []string{"-w", "feature"} }},
		{"F4 --worktree=", false, func(t *testing.T, req *network.Request) { req.NativeArgs = []string{"--worktree=feature"} }},
		{"F4 resume", false, func(t *testing.T, req *network.Request) { req.NativeArgs = []string{"--resume", "abc"} }},
		{"F4 -r in a short cluster", false, func(t *testing.T, req *network.Request) { req.NativeArgs = []string{"-pr", "abc"} }},
		{"F4 continue", false, func(t *testing.T, req *network.Request) { req.NativeArgs = []string{"-c"} }},
		{"F4 remote", false, func(t *testing.T, req *network.Request) { req.NativeArgs = []string{"--remote"} }},
		{"print flag alone", true, func(t *testing.T, req *network.Request) { req.NativeArgs = []string{"-p", "hello"} }},
		{"F6 FIFO settings", false, func(t *testing.T, req *network.Request) {
			root := t.TempDir()
			os.MkdirAll(filepath.Join(root, ".claude"), 0o700)
			if err := syscall.Mkfifo(filepath.Join(root, ".claude", "settings.json"), 0o600); err != nil {
				t.Fatal(err)
			}
			req.ProjectRoot = root
		}},
		{"F6 directory as settings", false, func(t *testing.T, req *network.Request) {
			root := t.TempDir()
			os.MkdirAll(filepath.Join(root, ".claude", "settings.local.json"), 0o700)
			req.ProjectRoot = root
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			tc.setup(t, &req)
			done := make(chan struct{})
			var got network.Prepared
			var err error
			go func() { got, err = network.Prepare(context.Background(), req); close(done) }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("Prepare blocked")
			}
			if tc.admit {
				if err != nil || got.Lease == nil {
					t.Fatalf("valid control refused: %v", err)
				}
				got.Lease.Finish(context.Background())
				return
			}
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || got.Lease != nil || len(g.calls()) != 0 {
				t.Fatalf("err = %v calls = %v", err, g.calls())
			}
		})
	}
}

// F1: an inherited endpoint selector (here the Unix socket transport and the
// API base) is removed from the child, by name as inherited.
func TestPrepareGatewayUnsetsInheritedEndpointNames(t *testing.T) {
	g := newGateway(t)
	req := gatewayRequest(gatewayHome(t, g, true))
	req.Prober = forbiddenProber{t}
	req.ParentEnv = append(req.ParentEnv, "ANTHROPIC_UNIX_SOCKET=/tmp/x.sock", "CLAUDE_CODE_API_BASE_URL=http://x")
	req.ChildEnv = append(req.ChildEnv, "anthropic_unix_socket=/tmp/y.sock")
	got, err := network.Prepare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Lease.Finish(context.Background())
	unset := " " + strings.Join(got.Patch.Unset, " ") + " "
	for _, n := range []string{"ANTHROPIC_UNIX_SOCKET", "CLAUDE_CODE_API_BASE_URL", "anthropic_unix_socket"} {
		if !strings.Contains(unset, " "+n+" ") {
			t.Fatalf("patch does not unset %s: %s", n, unset)
		}
	}
	env := got.Patch.Apply([]string{"ANTHROPIC_UNIX_SOCKET=/tmp/x.sock", "PATH=/bin"})
	if strings.Contains(strings.Join(env, " "), "UNIX_SOCKET") {
		t.Fatalf("applied env keeps the socket: %v", env)
	}
}

// F6: a network.toml that is a FIFO refuses promptly instead of blocking
// the default lookup on a read.
func TestDefaultGatewayFIFOCatalogRefuses(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".curator"), 0o700)
	if err := syscall.Mkfifo(filepath.Join(home, ".curator", "network.toml"), 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := network.DefaultGatewaySelected([]string{"HOME=" + home}); done <- err }()
	select {
	case err := <-done:
		if err == nil || refusalCode(t, err) != refusal.CodeFileUnreadable {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the default lookup blocked on a FIFO")
	}
}

// P2-6 follow-up: an explicit pinned policy consults the qualified list too,
// so an unreadable list refuses a pinned launch (optimistic is the control in
// TestStrictPolicyReadsQualifiedOnNonSensitive).
func TestPinnedPolicyReadsQualified(t *testing.T) {
	home := t.TempDir()
	endpoint := writeCatalogEndpoint(t, home)
	os.MkdirAll(filepath.Join(home, ".curator", network.QualifiedFileName), 0o700)
	req := baseRequest(home)
	req.Mode = adapterprobe.Pinned
	req.PinnedBuild = unitBuild()
	req.Prober = &scriptedProber{res: okResult(endpoint)}
	if _, err := network.Prepare(context.Background(), req); err == nil {
		t.Fatal("pinned launch ignored an unreadable qualified list")
	}
}
