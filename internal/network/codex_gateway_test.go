package network

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	toml "github.com/pelletier/go-toml/v2"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
	"github.com/relux-works/curator-network-profiles/pkg/binding"
	"github.com/relux-works/curator-network-profiles/pkg/netprofile"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
	"github.com/relux-works/curator-network-profiles/pkg/resolve"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	codexpolicy "github.com/relux-works/skill-agents-management/pkg/agentic/systems/codex"
	"github.com/relux-works/skill-agents-management/pkg/gatewayadmission"
	"github.com/relux-works/skill-agents-management/pkg/providerlimits"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
	_ "github.com/relux-works/skill-agents-management/pkg/vendorplugin/vendors/openai"
)

const codexSyntheticToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type codexControl struct {
	mu         sync.Mutex
	paths      []string
	base       string
	onRegister func()
}

func codexFixture(t *testing.T) (Request, *resolve.Result, *codexControl) {
	t.Helper()
	dir, err := os.MkdirTemp("", "cgw-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s")
	control := &codexControl{base: "http://127.0.0.1:47811/chatgpt/backend-api/codex"}
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		control.mu.Lock()
		control.paths = append(control.paths, r.URL.Path)
		control.mu.Unlock()
		w.Header().Set("Netproxy-Control-Api", "netproxy-control/1")
		switch r.URL.Path {
		case "/v1/health":
			io.WriteString(w, `{"api":"netproxy-control/1","ready":true}`)
		case "/v1/register":
			if control.onRegister != nil {
				control.onRegister()
			}
			json.NewEncoder(w).Encode(map[string]any{"api": "netproxy-control/1", "token": codexSyntheticToken, "token_header": "X-Netproxy-Token", "anthropic_base": "http://127.0.0.1:47811/anthropic", "openai_base": "http://127.0.0.1:47811/openai/v1", "chatgpt_base": control.base, "activation_seconds": 120})
		case "/v1/unregister":
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected control call %s", r.URL.Path)
			w.WriteHeader(500)
		}
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(t.TempDir(), "codex")
	data := []byte("\x7fELFcodex-policy-fixture")
	if err := os.WriteFile(bin, data, 0700); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(stage, data, 0500); err != nil {
		t.Fatal(err)
	}
	store, err := providerlimits.NewStore(providerlimits.Options{Layout: providerlimits.LayoutAt(t.TempDir())})
	if err != nil {
		t.Fatal(err)
	}
	built, err := gatewayadmission.Build(context.Background(), vendorplugin.Default, store, vendorplugin.SpawnRequest{Runtime: "codex", Model: "sol", Effort: "high", ToolRelease: "0.164.0", Home: t.TempDir(), WorkDir: t.TempDir(), Env: []string{"PATH=" + filepath.Dir(bin), "HOME=" + t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Harness: "codex-cli", Entrypoint: EntrypointInteractive, CodexPlan: &built, StagedExecutable: stage, ToolRelease: "0.164.0", ProjectRoot: built.Plan.WorkDir, SessionID: "unit-launch", Artifact: data, GatewayTimeout: time.Second}
	result := &resolve.Result{Profile: netprofile.Profile{Name: "gateway", Control: socket}}
	return req, result, control
}
func (c *codexControl) calls() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.paths)
}

// These component tests use a real SAM policy basis and a synthetic control
// service, without executing a runtime. They do not qualify an adapter recipe;
// the outer real Evaluate is exercised separately with its actual policy.
func TestCodexGatewayPrivateTokenAndFinalProjection(t *testing.T) {
	req, result, control := codexFixture(t)
	p, err := prepareCodexGateway(context.Background(), req, result, binding.AdapterIdentity{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Lease.Finish(context.Background())
	final, ok := p.CodexPlan()
	if !ok {
		t.Fatal("missing module plan")
	}
	raw, _ := json.Marshal(struct {
		Patch  any
		Record any
		Plan   any
	}{p.Patch, p.Record, final})
	if strings.Contains(string(raw), codexSyntheticToken) || !p.Patch.Empty() {
		t.Fatal("token entered serializable composition")
	}
	var decoded map[string]any
	if len(final.Plan.Argv) < 4 || toml.Unmarshal([]byte(final.Plan.Argv[1]+"\n"+final.Plan.Argv[3]), &decoded) != nil {
		t.Fatal("invalid module provider table")
	}
	providers, ok := decoded["model_providers"].(map[string]any)
	if !ok || len(providers) != 1 || decoded["model_provider"] != "curator-gateway" {
		t.Fatal("provider selection widened")
	}
	provider, ok := providers["curator-gateway"].(map[string]any)
	if !ok || provider["base_url"] != control.base || provider["wire_api"] != "responses" || provider["requires_openai_auth"] != true || provider["supports_websockets"] != false {
		t.Fatal("SAM route/auth/transport contract changed")
	}
	headers, ok := provider["env_http_headers"].(map[string]any)
	if !ok || len(headers) != 1 || headers["X-Netproxy-Token"] != codexTokenEnv {
		t.Fatal("token entered provider table or wrong header selected")
	}

	env := slices.Clone(final.Plan.Env)
	if err := p.BindProcess(req.StagedExecutable, final.Plan.Argv, env, nil, nil); err != nil {
		t.Fatal(err)
	}
	child, err := p.ChildEnvironment(env)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(child, codexTokenEnv+"="+codexSyntheticToken) {
		t.Fatal("child token missing")
	}
	if err := p.VerifyProcess(req.StagedExecutable, final.Plan.Argv, child); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func([]string) []string{
		func(e []string) []string { return append(e, "CURATOR_CODEX_GATEWAY_TOKEN=other") },
		func(e []string) []string {
			e[len(e)-1] = "curator_codex_gateway_token=" + codexSyntheticToken
			return e
		},
		func(e []string) []string { e[len(e)-1] = "CURATOR_CODEX_GATEWAY_TOKEN=wrong"; return e },
		func(e []string) []string { return e[:len(e)-1] },
	} {
		if err := p.VerifyProcess(req.StagedExecutable, final.Plan.Argv, mutate(slices.Clone(child))); !errors.Is(err, agentic.ErrCodexGatewayRefused) {
			t.Fatal("late token drift admitted", err)
		}
	}
	argv := append(slices.Clone(final.Plan.Argv), "--remote=ws://127.0.0.1:1")
	if p.VerifyProcess(req.StagedExecutable, argv, child) == nil {
		t.Fatal("late argv admitted")
	}
	if p.VerifyProcess(final.Plan.Binary, final.Plan.Argv, child) == nil {
		t.Fatal("original path substituted for staged path")
	}
	if err := os.Chmod(req.StagedExecutable, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(req.StagedExecutable, []byte("changed"), 0700); err != nil {
		t.Fatal(err)
	}
	if p.VerifyProcess(req.StagedExecutable, final.Plan.Argv, child) == nil {
		t.Fatal("staged bytes drift admitted")
	}
	if got := control.calls(); !slices.Equal(got, []string{"/v1/health", "/v1/register"}) {
		t.Fatal(got)
	}
}

// Invalid authority/selector inputs refuse before health or registration;
// failed finalization after registration and cancellation both unregister.
func TestCodexGatewayRefusalCleanup(t *testing.T) {
	cases := []struct {
		name              string
		change            func(*Request, *codexControl)
		afterRegistration bool
	}{
		{"missing-basis", func(r *Request, c *codexControl) { r.CodexPlan.GatewayBasis = nil }, false},
		{"native-provider", func(r *Request, c *codexControl) { r.NativeArgs = []string{"-c", "model_provider=other"} }, false},
		{"token-alias-overlay", func(r *Request, c *codexControl) {
			r.FragEnv = map[string]string{"curator_codex_gateway_token": "alias"}
		}, false},
		{"token-mcp-lookup", func(r *Request, c *codexControl) { r.MCPEnvNames = []string{codexTokenEnv} }, false},
		{"token-mcp-alias", func(r *Request, c *codexControl) { r.MCPEnvNames = []string{strings.ToLower(codexTokenEnv)} }, false},
		{"provider-mcp-lookup", func(r *Request, c *codexControl) { r.MCPEnvNames = []string{"OPENAI_BASE_URL"} }, false},
		{"remote-mcp-lookup", func(r *Request, c *codexControl) { r.MCPEnvNames = []string{"CODEX_REMOTE_URL"} }, false},
		{"different-plan", func(r *Request, c *codexControl) { r.CodexPlan.Plan.Argv = append(r.CodexPlan.Plan.Argv, "--remote=x") }, false},
		{"bad-base-path", func(r *Request, c *codexControl) { c.base = "http://127.0.0.1:47811/wrong" }, true},
		{"original-drift-during-register", func(r *Request, c *codexControl) {
			c.onRegister = func() { os.WriteFile(r.CodexPlan.Plan.Binary, []byte("changed"), 0700) }
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, result, c := codexFixture(t)
			tc.change(&req, c)
			p, err := prepareCodexGateway(context.Background(), req, result, binding.AdapterIdentity{}, nil)
			if err == nil || p.Lease != nil || p.codex != nil {
				t.Fatal("refusal returned usable launch")
			}
			expected := []string(nil)
			if tc.afterRegistration {
				expected = []string{"/v1/health", "/v1/register", "/v1/unregister"}
			}
			if got := c.calls(); !slices.Equal(got, expected) {
				t.Fatalf("cleanup calls %v, want %v", got, expected)
			}
		})
	}
	t.Run("cancel-after-registration", func(t *testing.T) {
		req, result, c := codexFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c.onRegister = cancel
		p, err := prepareCodexGateway(ctx, req, result, binding.AdapterIdentity{}, nil)
		if err == nil || p.Lease != nil {
			t.Fatal("cancelled registration admitted")
		}
		if got := c.calls(); !slices.Equal(got, []string{"/v1/health", "/v1/register", "/v1/unregister"}) {
			t.Fatal(got)
		}
	})
}

// Real Evaluate preserves explicit-false optimistic warning/provenance, strict
// misses and exact qualification. Synthetic container bytes are never executed;
// these are policy/docking controls, not native adapter qualification.
func TestCodexGatewayRealEvaluateDocking(t *testing.T) {
	for _, kind := range []string{"optimistic", "strict-exact", "strict-missing", "sensitive-missing", "wrong-harness", "wrong-entrypoint", "wrong-digest", "default-false", "machine-false", "hosted", "exec"} {
		t.Run(kind, func(t *testing.T) {
			req, result, control := codexFixture(t)
			req.Harness = "codex-cli"
			req.Explicit = "egress-a"
			req.ExplicitSet = true
			req.ParentEnv = slices.Clone(req.CodexPlan.Plan.Env)
			req.Mode = adapterprobe.Strict
			if kind == "optimistic" {
				req.Mode = adapterprobe.Optimistic
			}
			home := ""
			for _, v := range req.ParentEnv {
				if h, ok := strings.CutPrefix(v, "HOME="); ok {
					home = h
				}
			}
			dir := filepath.Join(home, ".curator")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			sensitive := kind == "sensitive-missing"
			doc := fmt.Sprintf("schema = \"relux-network-profiles-v1\"\n[networks.egress-a]\nkind = \"filtering-gateway\"\ncontrol = %q\nsensitive_egress = %t\n", result.Profile.Control, sensitive)
			if kind == "default-false" {
				doc = strings.Replace(doc, "[networks.egress-a]", "default = \"egress-a\"\n[networks.egress-a]", 1)
				if err := os.WriteFile(filepath.Join(dir, "network.toml"), []byte(doc), 0600); err != nil {
					t.Fatal(err)
				}
				req.ExplicitSet, req.Explicit, req.DefaultGateway = false, "", true
				p, err := Prepare(context.Background(), req)
				var typed *refusal.Refusal
				if !errors.As(err, &typed) || !strings.Contains(err.Error(), "a non-sensitive filtering-gateway cannot be the operator default") || p.Lease != nil || len(control.calls()) != 0 {
					t.Fatal("default false did not receive its intended refusal", err)
				}
				return
			}
			file, err := netprofile.Parse([]byte(doc))
			if err != nil {
				t.Fatal(err)
			}
			profile, _ := file.Lookup("egress-a")
			ledger, _ := json.Marshal(map[string]any{"schema": "relux-network-confirmations-v1", "confirmed": map[string]any{"egress-a": map[string]any{"digest": netprofile.Digest(profile), "confirmed_at": "2026-11-03T00:00:00Z"}}})
			if err := os.WriteFile(filepath.Join(dir, "network.toml"), []byte(doc), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "network.confirmations.json"), ledger, 0600); err != nil {
				t.Fatal(err)
			}
			digest := fmt.Sprintf("%x", sha256.Sum256(req.Artifact))
			harness, entry := "codex-cli", "interactive"
			req.HostIdentity = binding.AdapterIdentity{Adapter: "generic-env-v1", Harness: harness, Build: "sha256-" + digest, Entrypoint: entry}
			switch kind {
			case "wrong-harness":
				harness = "codex"
			case "wrong-entrypoint":
				entry = "exec"
			case "wrong-digest":
				digest = strings.Repeat("a", 64)
			case "default-false":
				req.ExplicitSet = false
				req.Explicit = ""
				req.DefaultGateway = true
			case "machine-false":
				req.ExplicitSet = false
				req.Explicit = ""
				req.LauncherProfile = "egress-a"
			case "hosted":
				req.Hosted = true
			case "exec":
				req.Entrypoint = EntrypointExec
			}
			if kind != "strict-missing" && kind != "sensitive-missing" && kind != "optimistic" {
				qualified := fmt.Sprintf("schema = \"curator-network-qualified-v1\"\n[[builds]]\nharness = %q\nentrypoint = %q\nbinary_sha256 = %q\nplatform = %q\nevidence = \"synthetic policy fixture only\"\nqualified_on = \"2026-11-03\"\n", harness, entry, digest, runtime.GOOS)
				if err := os.WriteFile(filepath.Join(dir, QualifiedFileName), []byte(qualified), 0600); err != nil {
					t.Fatal(err)
				}
			}
			prepared, err := Prepare(context.Background(), req)
			if kind == "optimistic" || kind == "strict-exact" {
				if err != nil || prepared.Lease == nil {
					t.Fatalf("real Evaluate failed: %v", err)
				}
				if kind == "optimistic" {
					if prepared.Provenance == nil || prepared.Provenance.Outcome != "unqualified_build" || prepared.Provenance.OperatorText() == "" {
						t.Fatal("missing required warning/provenance")
					}
					var warning strings.Builder
					if err := EmitAdapterProvenance(&warning, prepared.Provenance); err != nil || !strings.Contains(warning.String(), prepared.Provenance.OperatorText()) || !strings.Contains(warning.String(), "adapter-provenance:") {
						t.Fatal("unqualified warning/record not emitted", err)
					}
				} else if prepared.Provenance != nil {
					t.Fatal("qualified row carried unqualified provenance")
				}
				if err := prepared.Lease.Finish(context.Background()); err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(control.calls(), []string{"/v1/health", "/v1/register", "/v1/unregister"}) {
					t.Fatal(control.calls())
				}
				return
			}
			var typed *refusal.Refusal
			if !errors.As(err, &typed) || prepared.Lease != nil || len(control.calls()) != 0 {
				t.Fatalf("refusal reached control or lost typed error: %v %v", err, control.calls())
			}
			if kind == "strict-missing" || kind == "sensitive-missing" || strings.HasPrefix(kind, "wrong-") {
				if !strings.Contains(err.Error(), "strict_miss") && kind != "wrong-harness" && kind != "wrong-entrypoint" {
					t.Fatal("wrong strict refusal", err)
				}
			}
		})
	}
}

// Exercise both unchanged SAM predicates directly: admission and the binding
// renderer. A permission row alone cannot admit a release at either gate.
func TestCodexGatewayBothReleasePredicates(t *testing.T) {
	system := codexpolicy.New()
	for _, release := range []string{"0.164.0", "0.166.0", "9.999.0"} {
		t.Run(release, func(t *testing.T) {
			request := agentic.LaunchRequest{ToolRelease: release, PermissionMode: agentic.PermissionModeNative}
			policy := system.CodexGatewayBasis(request, agentic.LaunchModeInteractive)
			err := policy.Admit(agentic.CodexGatewayInputs{}, nil)
			if release == "0.164.0" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, agentic.ErrCodexGatewayRefused) {
				t.Fatal("admission release predicate widened")
			}
			// Hold the admitted request at 164 and vary only the binding release so
			// the second predicate cannot be masked by first-stage release refusal.
			request.ToolRelease = "0.164.0"
			original, err := codexpolicy.Args(request, agentic.LaunchModeInteractive)
			if err != nil {
				t.Fatal(err)
			}
			policy = system.CodexGatewayBasis(request, agentic.LaunchModeInteractive)
			b := agentic.RuntimeGatewayBindingV1{ProviderID: "curator-gateway", BaseURL: "http://127.0.0.1:47811/chatgpt/backend-api/codex", HeaderName: "X-Netproxy-Token", TokenEnv: codexTokenEnv, WireAPI: "responses", RequiresOpenAIAuth: true, ToolRelease: release, ExecutablePath: "/synthetic/codex", ExecutableSHA256: strings.Repeat("a", 64)}
			_, err = policy.Render(b, original)
			if release == "0.164.0" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, agentic.ErrCodexGatewayRefused) {
				t.Fatal("binding release predicate widened")
			}
		})
	}
}
