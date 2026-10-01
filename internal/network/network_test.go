package network_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/network"
	"github.com/relux-works/curator-network-profiles/pkg/binding"
	"github.com/relux-works/curator-network-profiles/pkg/envpatch"
	"github.com/relux-works/curator-network-profiles/pkg/netprofile"
	"github.com/relux-works/curator-network-profiles/pkg/probe"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
)

// scriptedProber records its request and replays one outcome. Tests never
// reach the network: every managed Prepare below injects this prober or
// an in-process Dialer; no test opens a socket.
type scriptedProber struct {
	calls int
	got   probe.Request
	res   probe.Result
	err   error
}

func (s *scriptedProber) Probe(_ context.Context, req probe.Request) (probe.Result, error) {
	s.calls++
	s.got = req
	return s.res, s.err
}

// forbiddenProber fails the test when the stage under test must not probe.
type forbiddenProber struct{ t *testing.T }

func (p forbiddenProber) Probe(_ context.Context, _ probe.Request) (probe.Result, error) {
	p.t.Fatal("probe ran for a launch that must not probe")
	return probe.Result{}, nil
}

const catalogTOML = `schema = "relux-network-profiles-v1"

[networks.egress-a]
kind = "external-http-proxy"
endpoint = "http://127.0.0.1:18080"
bypass_hosts = ["127.0.0.1", "::1", "localhost"]
`

// writeCatalog stores the catalog under home and, when confirmed, a ledger
// confirming the profile at its current digest. It returns the parsed
// profile and digest so expectations derive from the library, not copies.
func writeCatalog(t *testing.T, home string, confirmed bool) (netprofile.Profile, string) {
	t.Helper()
	dir := filepath.Join(home, ".curator")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "network.toml"), []byte(catalogTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := netprofile.Parse([]byte(catalogTOML))
	if err != nil {
		t.Fatal(err)
	}
	prof, ok := file.Lookup("egress-a")
	if !ok {
		t.Fatal("fixture profile missing after parse")
	}
	digest := netprofile.Digest(prof)
	if confirmed {
		ledger := map[string]any{
			"schema": "relux-network-confirmations-v1",
			"confirmed": map[string]any{
				"egress-a": map[string]any{"digest": digest, "confirmed_at": "2026-10-02T00:00:00Z"},
			},
		}
		raw, err := json.Marshal(ledger)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "network.confirmations.json"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return prof, digest
}

// fixtureAllowlist is the test-only support policy: exactly the
// baseRequest tuple. Production Identify uses the shipped list, which
// holds only the verified claude-code tuple, so every test that must
// reach the probe with the codex fixture injects this one.
func fixtureAllowlist() []binding.AdapterIdentity {
	return []binding.AdapterIdentity{{
		Adapter: envpatch.AdapterGeneric, Harness: "codex",
		Build: "codex-cli 0.153.2", Entrypoint: network.EntrypointExec,
	}}
}

func baseRequest(home string) network.Request {
	return network.Request{
		Explicit: selectedProfile, ExplicitSet: true,
		Tracked: false, Harness: "codex", Build: "codex-cli 0.153.2",
		ParentEnv: []string{"HOME=" + home, "PATH=/usr/bin"},
		Allowlist: fixtureAllowlist(),
	}
}

const selectedProfile = "egress-a"

func refusalCode(t *testing.T, err error) string {
	t.Helper()
	r, ok := refusal.As(err)
	if !ok || r == nil {
		t.Fatalf("error is not a *refusal.Refusal: %T %v", err, err)
	}
	return r.Code
}

// TestPrepareUnmanagedTouchesNothing: without a selection there is no
// catalog read and no probe, even when both would fail.
func TestPrepareUnmanagedTouchesNothing(t *testing.T) {
	got, err := network.Prepare(context.Background(), network.Request{
		ExplicitSet: false, ParentEnv: nil, Prober: forbiddenProber{t},
	})
	if err != nil {
		t.Fatalf("unmanaged Prepare error = %v", err)
	}
	if !got.Patch.Empty() || got.Record != nil {
		t.Fatalf("unmanaged outcome = %+v, want empty patch and nil record", got)
	}
}

// TestPrepareBlankRefuses: a blank selection fails closed without I/O
// instead of launching unmanaged under --network.
func TestPrepareBlankRefuses(t *testing.T) {
	_, err := network.Prepare(context.Background(), network.Request{
		Explicit: "  ", ExplicitSet: true, ParentEnv: nil, Prober: forbiddenProber{t},
	})
	if err == nil || refusalCode(t, err) != refusal.CodeProfileInvalid {
		t.Fatalf("blank selection error = %v, want %s", err, refusal.CodeProfileInvalid)
	}
}

// TestPrepareTrackedRefusesBeforeIO: tracked mode refuses before the
// catalog read and the probe. HOME is unset (a catalog read would fail)
// and the prober fails the test when called.
func TestPrepareTrackedRefusesBeforeIO(t *testing.T) {
	req := baseRequest(t.TempDir())
	req.Tracked = true
	req.ParentEnv = []string{"PATH=/usr/bin"}
	req.Prober = forbiddenProber{t}
	_, err := network.Prepare(context.Background(), req)
	if err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported {
		t.Fatalf("tracked error = %v, want %s", err, refusal.CodeScopeUnsupported)
	}
	if !strings.Contains(err.Error(), "egress-a") {
		t.Fatalf("tracked refusal %q does not name the selection", err.Error())
	}
}

func TestPrepareResolveRefusals(t *testing.T) {
	home := t.TempDir()
	for _, tc := range []struct {
		name     string
		setup    func(t *testing.T) []string
		explicit string
		want     string
	}{
		{"unknown", func(t *testing.T) []string {
			writeCatalog(t, home, true)
			return []string{"HOME=" + home}
		}, "no-such-profile", refusal.CodeProfileUnknown},
		{"unconfirmed", func(t *testing.T) []string {
			other := t.TempDir()
			writeCatalog(t, other, false)
			return []string{"HOME=" + other}
		}, "egress-a", refusal.CodeProfileDenied},
		{"no-home", func(t *testing.T) []string {
			return []string{"PATH=/usr/bin"}
		}, "egress-a", refusal.CodeFileUnreadable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := baseRequest(home)
			req.ParentEnv = tc.setup(t)
			req.Explicit = tc.explicit
			req.Prober = forbiddenProber{t}
			_, err := network.Prepare(context.Background(), req)
			if err == nil || refusalCode(t, err) != tc.want {
				t.Fatalf("error = %v, want %s", err, tc.want)
			}
		})
	}
}

// TestPrepareBrokenCatalogRefuses: a catalog that exists but cannot be
// used is refused, never headroom for an unmanaged launch: a syntax
// error is unreadable, a semantically invalid profile is invalid.
func TestPrepareBrokenCatalogRefuses(t *testing.T) {
	for _, tc := range []struct {
		name string
		doc  string
		want string
	}{
		{"syntax-error", "schema = [\n", refusal.CodeFileUnreadable},
		{"bad-endpoint", strings.Replace(catalogTOML, "http://127.0.0.1:18080", "http://user@127.0.0.1:18080", 1), refusal.CodeProfileInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, ".curator")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "network.toml"), []byte(tc.doc), 0o600); err != nil {
				t.Fatal(err)
			}
			req := baseRequest(home)
			req.Prober = forbiddenProber{t}
			_, err := network.Prepare(context.Background(), req)
			if err == nil || refusalCode(t, err) != tc.want {
				t.Fatalf("error = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestPrepareSupportRefusalsSkipProbe(t *testing.T) {
	home := t.TempDir()
	writeCatalog(t, home, true)
	for _, tc := range []struct {
		name    string
		harness string
		build   string
	}{
		{"muse", "muse", "1.4.2"},
		{"unknown-harness", "future-tool", "9.9"},
		{"empty-build", "codex", ""},
		{"blank-build", "pi-native", "  "},
		{"unlisted-build", "codex", "codex-cli 9.9.9"},
		{"build-prefix", "codex", "codex-cli 0.153"},
		{"build-trailing-space", "codex", "codex-cli 0.153.2 "},
		{"harness-case", "Codex", "codex-cli 0.153.2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := baseRequest(home)
			req.Harness, req.Build = tc.harness, tc.build
			req.Prober = forbiddenProber{t}
			_, err := network.Prepare(context.Background(), req)
			if err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported {
				t.Fatalf("error = %v, want %s", err, refusal.CodeScopeUnsupported)
			}
		})
	}
}

// TestPrepareProductionAllowlistRefuses: N1 regression. With a nil
// allowlist — the production shape — the codex fixture tuple refuses:
// the shipped list holds only the verified claude-code tuple.
func TestPrepareProductionAllowlistRefuses(t *testing.T) {
	home := t.TempDir()
	writeCatalog(t, home, true)
	req := baseRequest(home)
	req.Allowlist = nil
	req.Prober = forbiddenProber{t}
	_, err := network.Prepare(context.Background(), req)
	if err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported {
		t.Fatalf("production-list error = %v, want %s", err, refusal.CodeScopeUnsupported)
	}
}

// TestPrepareProductionAllowlistAdmitsVerifiedTuple: with a nil
// allowlist — the production shape — the verified claude-code tuple
// reaches the probe and binds.
func TestPrepareProductionAllowlistAdmitsVerifiedTuple(t *testing.T) {
	home := t.TempDir()
	prof, _ := writeCatalog(t, home, true)
	sp := &scriptedProber{res: probe.Result{
		Endpoint: prof.Endpoint, TCP: probe.StatusOK,
		Connect: probe.StatusSkipped, TLS: probe.StatusSkipped, CheckedAt: time.Now(),
	}}
	req := baseRequest(home)
	req.Harness, req.Build = "claude-code", "2.1.287"
	req.Allowlist = nil
	req.Prober = sp
	got, err := network.Prepare(context.Background(), req)
	if err != nil {
		t.Fatalf("production-list Prepare error = %v", err)
	}
	if sp.calls != 1 || got.Record == nil {
		t.Fatalf("calls = %d record = %+v, want one probe and a bound record", sp.calls, got.Record)
	}
	if got.Record.AdapterIdentity.Harness != "claude-code" || got.Record.AdapterIdentity.Build != "2.1.287" {
		t.Fatalf("record identity = %+v, want the verified claude-code tuple", got.Record.AdapterIdentity)
	}
}

func TestCheckOverlays(t *testing.T) {
	for _, tc := range []struct {
		name      string
		frag      map[string]string
		prompt    map[string]string
		mcp       []string
		wantCode  string
		wantSubj  string
		wantLayer string
	}{
		{"frag canonical upper", map[string]string{"HTTP_PROXY": "x"}, nil, nil, refusal.CodeConfigurationConflict, "HTTP_PROXY", "fragment"},
		{"frag canonical lower", map[string]string{"no_proxy": "x"}, nil, nil, refusal.CodeConfigurationConflict, "no_proxy", "fragment"},
		{"prompt mixed case", nil, map[string]string{"Http_Proxy": "x"}, nil, refusal.CodeConfigurationConflict, "Http_Proxy", "prompt channel"},
		{"prompt all proxy", nil, map[string]string{"ALL_PROXY": "x"}, nil, refusal.CodeConfigurationConflict, "ALL_PROXY", "prompt channel"},
		{"mcp mixed case", nil, nil, []string{"FIGMA_API_KEY", "Http_Proxy"}, refusal.CodeConfigurationConflict, "Http_Proxy", "mcp env_names"},
		{"mcp canonical", nil, nil, []string{"HTTPS_PROXY"}, refusal.CodeConfigurationConflict, "HTTPS_PROXY", "mcp env_names"},
		{"mcp alternating case", nil, nil, []string{"fTp_PrOxY"}, refusal.CodeConfigurationConflict, "fTp_PrOxY", "mcp env_names"},
		{"clean", map[string]string{"SIBLING": "x"}, map[string]string{"OTHER": "y"}, []string{"FIGMA_API_KEY"}, "", "", ""},
		{"nil overlays", nil, nil, nil, "", "", ""},
		{"proxy-like value is not a name", map[string]string{"NOTE": "HTTP_PROXY=x"}, nil, nil, "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := network.CheckOverlays(tc.frag, tc.prompt, tc.mcp)
			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("clean overlays error = %v, want nil", err)
				}
				return
			}
			if err == nil || refusalCode(t, err) != tc.wantCode {
				t.Fatalf("error = %v, want %s", err, tc.wantCode)
			}
			if !strings.Contains(err.Error(), tc.wantSubj) || !strings.Contains(err.Error(), tc.wantLayer) {
				t.Fatalf("error %q does not name %q in %q", err.Error(), tc.wantSubj, tc.wantLayer)
			}
		})
	}
}

// TestPrepareOverlayConflictSkipsProbe: the pure conflict check runs after
// support verification and before the preflight, so a conflict never
// costs a network probe.
func TestPrepareOverlayConflictSkipsProbe(t *testing.T) {
	home := t.TempDir()
	writeCatalog(t, home, true)
	for _, tc := range []struct {
		name string
		req  func(*network.Request)
	}{
		{"frag", func(r *network.Request) { r.FragEnv = map[string]string{"HTTP_PROXY": "x"} }},
		{"prompt", func(r *network.Request) { r.PromptEnv = map[string]string{"Http_Proxy": "x"} }},
		{"mcp", func(r *network.Request) { r.MCPEnvNames = []string{"FIGMA_API_KEY", "Http_Proxy"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := baseRequest(home)
			req.Prober = forbiddenProber{t}
			tc.req(&req)
			_, err := network.Prepare(context.Background(), req)
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict {
				t.Fatalf("error = %v, want %s", err, refusal.CodeConfigurationConflict)
			}
		})
	}
}

// TestPrepareEngineCoverage: EngineHosts reach the resolve check. A host
// the profile's bypass_hosts covers admits; an uncovered one refuses
// network_configuration_conflict without probing.
func TestPrepareEngineCoverage(t *testing.T) {
	home := t.TempDir()
	prof, _ := writeCatalog(t, home, true)
	t.Run("covered", func(t *testing.T) {
		sp := &scriptedProber{res: probe.Result{
			Endpoint: prof.Endpoint, TCP: probe.StatusOK,
			Connect: probe.StatusSkipped, TLS: probe.StatusSkipped, CheckedAt: time.Now(),
		}}
		req := baseRequest(home)
		req.Prober = sp
		req.EngineHosts = []string{"127.0.0.1", "LOCALHOST", "[::1]:11434"}
		if _, err := network.Prepare(context.Background(), req); err != nil {
			t.Fatalf("covered hosts error = %v, want nil", err)
		}
		if sp.calls != 1 {
			t.Fatalf("probe calls = %d, want 1", sp.calls)
		}
	})
	t.Run("uncovered", func(t *testing.T) {
		req := baseRequest(home)
		req.Prober = forbiddenProber{t}
		req.EngineHosts = []string{"127.0.0.1", "engine.local"}
		_, err := network.Prepare(context.Background(), req)
		if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict {
			t.Fatalf("error = %v, want %s", err, refusal.CodeConfigurationConflict)
		}
		if !strings.Contains(err.Error(), "engine.local") {
			t.Fatalf("error %q does not name the uncovered host", err.Error())
		}
	})
}

// TestPrepareExplicitBeatsOperatorDefault: N-B is explicit-only. A
// catalog operator default never overrides the --network selection;
// the library's N4 precedence resolves the explicit ref.
func TestPrepareExplicitBeatsOperatorDefault(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".curator")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	doc := "schema = \"relux-network-profiles-v1\"\ndefault = \"other\"\n\n" +
		"[networks.egress-a]\nkind = \"external-http-proxy\"\nendpoint = \"http://127.0.0.1:18080\"\n" +
		"bypass_hosts = [\"127.0.0.1\", \"::1\", \"localhost\"]\n\n" +
		"[networks.other]\nkind = \"external-http-proxy\"\nendpoint = \"http://127.0.0.1:19090\"\n" +
		"bypass_hosts = [\"127.0.0.1\", \"::1\", \"localhost\"]\n"
	if err := os.WriteFile(filepath.Join(dir, "network.toml"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := netprofile.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	prof, _ := file.Lookup("egress-a")
	ledger, err := json.Marshal(map[string]any{
		"schema": "relux-network-confirmations-v1",
		"confirmed": map[string]any{
			"egress-a": map[string]any{"digest": netprofile.Digest(prof), "confirmed_at": "2026-10-02T00:00:00Z"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "network.confirmations.json"), ledger, 0o600); err != nil {
		t.Fatal(err)
	}
	sp := &scriptedProber{res: probe.Result{Endpoint: prof.Endpoint, TCP: probe.StatusOK,
		Connect: probe.StatusSkipped, TLS: probe.StatusSkipped, CheckedAt: time.Now()}}
	req := baseRequest(home)
	req.Prober = sp
	got, err := network.Prepare(context.Background(), req)
	if err != nil {
		t.Fatalf("Prepare error = %v", err)
	}
	if sp.got.Endpoint != "http://127.0.0.1:18080" || got.Record.ProfileRef != "egress-a" {
		t.Fatalf("resolved %+v, want the explicit egress-a, not the operator default", sp.got)
	}
}

func TestProductionAllowlistHoldsVerifiedTuple(t *testing.T) {
	want := []binding.AdapterIdentity{{
		Adapter: envpatch.AdapterGeneric, Harness: "claude-code",
		Build: "2.1.287", Entrypoint: network.EntrypointExec,
	}}
	got := network.VerifiedAdapters()
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("production allowlist = %+v, want exactly %+v", got, want)
	}
}

func TestIdentify(t *testing.T) {
	allow := fixtureAllowlist()
	id, err := network.IdentifyWith("codex", "codex-cli 0.153.2", allow)
	if err != nil {
		t.Fatalf("IdentifyWith(listed) error = %v", err)
	}
	if want := allow[0]; id != want {
		t.Fatalf("IdentifyWith(listed) = %+v, want %+v", id, want)
	}
	for _, tc := range []struct{ harness, build string }{
		{"muse", "1.4.2"}, {"future-tool", "9.9"}, {"codex", ""}, {"", "1.0"},
		{"codex", "codex-cli 0.153"}, {"codex", "codex-cli 0.153.2 "},
		{"Codex", "codex-cli 0.153.2"},
	} {
		_, err := network.IdentifyWith(tc.harness, tc.build, allow)
		if err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported {
			t.Fatalf("IdentifyWith(%q,%q) = %v, want %s", tc.harness, tc.build, err, refusal.CodeScopeUnsupported)
		}
	}
	// Adapter-only and entrypoint-only allowlist mismatches refuse: the
	// launch tuple is fixed, so the one changed member is on the list
	// side. Each changes exactly one member from the admitted tuple.
	for _, tc := range []struct {
		name  string
		entry binding.AdapterIdentity
	}{
		{"adapter", binding.AdapterIdentity{Adapter: "other-adapter-v9", Harness: "codex", Build: "codex-cli 0.153.2", Entrypoint: network.EntrypointExec}},
		{"entrypoint", binding.AdapterIdentity{Adapter: envpatch.AdapterGeneric, Harness: "codex", Build: "codex-cli 0.153.2", Entrypoint: "spawn"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := network.IdentifyWith("codex", "codex-cli 0.153.2", []binding.AdapterIdentity{tc.entry})
			if err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported {
				t.Fatalf("IdentifyWith(%s-mismatch) = %v, want %s", tc.name, err, refusal.CodeScopeUnsupported)
			}
		})
	}
	// Production Identify uses the shipped list: the verified
	// claude-code tuple admits, the codex fixture tuple refuses.
	if _, err := network.Identify("claude-code", "2.1.287"); err != nil {
		t.Fatalf("Identify(verified) error = %v, want nil", err)
	}
	if _, err := network.Identify("codex", "codex-cli 0.153.2"); err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported {
		t.Fatalf("Identify(unlisted) = %v, want %s", err, refusal.CodeScopeUnsupported)
	}
}

func TestPrepareProbeFailuresTerminate(t *testing.T) {
	home := t.TempDir()
	writeCatalog(t, home, true)
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"unreachable", refusal.New(refusal.CodeProxyUnreachable, "egress-a", "tcp: connection refused"), refusal.CodeProxyUnreachable},
		{"auth-failed", refusal.New(refusal.CodeProxyAuthFailed, "egress-a", "connect: proxy authentication required (407)"), refusal.CodeProxyAuthFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sp := &scriptedProber{err: tc.err}
			req := baseRequest(home)
			req.Prober = sp
			_, err := network.Prepare(context.Background(), req)
			if err == nil || refusalCode(t, err) != tc.want {
				t.Fatalf("error = %v, want %s", err, tc.want)
			}
			if sp.calls != 1 {
				t.Fatalf("probe calls = %d, want 1", sp.calls)
			}
		})
	}
}

// TestPrepareSuccessBindsPatchAndRecord: the production path for a real
// supported direct launch. The request to the prober is bounded and
// targetless; the patch is the generic unset-then-set; the Record keeps
// the probe statuses and time only.
func TestPrepareSuccessBindsPatchAndRecord(t *testing.T) {
	home := t.TempDir()
	prof, digest := writeCatalog(t, home, true)
	checkedAt := time.Date(2026, 10, 2, 7, 0, 0, 0, time.UTC)
	sp := &scriptedProber{res: probe.Result{
		Endpoint: prof.Endpoint, TCP: probe.StatusOK,
		Connect: probe.StatusSkipped, TLS: probe.StatusSkipped, CheckedAt: checkedAt,
	}}
	req := baseRequest(home)
	req.Prober = sp
	got, err := network.Prepare(context.Background(), req)
	if err != nil {
		t.Fatalf("Prepare error = %v", err)
	}
	if sp.calls != 1 {
		t.Fatalf("probe calls = %d, want 1", sp.calls)
	}
	if sp.got.Subject != "egress-a" || sp.got.Endpoint != prof.Endpoint || sp.got.Target != "" || sp.got.Timeout != probe.DefaultTimeout {
		t.Fatalf("probe request = %+v, want bounded targetless probe of the resolved endpoint", sp.got)
	}
	wantPatch := envpatch.Generic{}.Patch(prof)
	if len(got.Patch.Unset) != len(wantPatch.Unset) || len(got.Patch.Set) != len(wantPatch.Set) {
		t.Fatalf("patch = %+v, want generic patch %+v", got.Patch, wantPatch)
	}
	for i, kv := range wantPatch.Set {
		if got.Patch.Set[i] != kv {
			t.Fatalf("patch set[%d] = %+v, want %+v", i, got.Patch.Set[i], kv)
		}
	}
	if got.Record == nil {
		t.Fatal("record is nil on a managed launch")
	}
	r := *got.Record
	if r.Schema != binding.SchemaRecord || r.ProfileRef != "egress-a" || r.Origin != "explicit" ||
		r.ProfileDigest != digest || r.Assurance != binding.AssuranceCooperative ||
		r.AdapterIdentity.Adapter != envpatch.AdapterGeneric || r.AdapterIdentity.Harness != "codex" ||
		r.AdapterIdentity.Build != "codex-cli 0.153.2" || r.AdapterIdentity.Entrypoint != network.EntrypointExec ||
		r.Probe.TCP != "ok" || r.Probe.Connect != "skipped" || r.Probe.TLS != "skipped" || !r.Probe.CheckedAt.Equal(checkedAt) {
		t.Fatalf("record = %+v, want bound explicit record", r)
	}
	// The Record serializes without the endpoint: raw probe output must
	// not become manifest provenance.
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), prof.Endpoint) {
		t.Fatalf("record serializes the endpoint: %s", raw)
	}
}

// TestDefaultProberIsProductionDialer: the wiring assertion without
// I/O. A nil request prober selects DefaultProber; that default is the
// production dialer type. No socket is opened.
func TestDefaultProberIsProductionDialer(t *testing.T) {
	d, ok := network.DefaultProber().(*probe.Dialer)
	if !ok || d == nil {
		t.Fatalf("DefaultProber = %T, want *probe.Dialer", network.DefaultProber())
	}
	if d.Dial != nil {
		t.Fatal("DefaultProber carries a custom Dial func, want the production transport")
	}
}

// TestPrepareProductionDialerProbesInProcess: the production Dialer type
// drives a managed Prepare through an in-memory net.Pipe transport — no
// TCP socket is opened anywhere — and the patch binds with the probe
// statuses in the Record.
func TestPrepareProductionDialerProbesInProcess(t *testing.T) {
	home := t.TempDir()
	prof, _ := writeCatalog(t, home, true)
	dialer := &probe.Dialer{Dial: func(_ context.Context, _, _ string) (net.Conn, error) {
		client, server := net.Pipe()
		t.Cleanup(func() { _ = server.Close() })
		return client, nil
	}}
	req := baseRequest(home)
	req.Prober = dialer
	got, err := network.Prepare(context.Background(), req)
	if err != nil {
		t.Fatalf("Prepare error = %v", err)
	}
	if got.Patch.Empty() {
		t.Fatal("patch is empty on a managed launch")
	}
	if got.Record == nil || got.Record.Probe.TCP != "ok" {
		t.Fatalf("record = %+v, want a bound record with a TCP ok probe", got.Record)
	}
	wantPatch := envpatch.Generic{}.Patch(prof)
	if len(got.Patch.Set) != len(wantPatch.Set) {
		t.Fatalf("patch sets = %d, want %d", len(got.Patch.Set), len(wantPatch.Set))
	}
}

func TestProvenanceLine(t *testing.T) {
	r := binding.Record{
		Schema:          binding.SchemaRecord,
		ProfileRef:      "egress-a",
		ProfileDigest:   "sha256:abc",
		AdapterIdentity: binding.AdapterIdentity{Adapter: envpatch.AdapterGeneric, Harness: "codex", Build: "codex-cli 0.153.2", Entrypoint: network.EntrypointExec},
		Assurance:       binding.AssuranceCooperative,
		Origin:          "explicit",
		Probe:           binding.ProbeRecord{TCP: "ok", Connect: "skipped", TLS: "skipped"},
	}
	want := "curator-run: network: profile=egress-a origin=explicit digest=sha256:abc adapter=generic-env-v1 harness=codex build=codex-cli 0.153.2 entrypoint=exec assurance=cooperative probe=ok/skipped/skipped"
	if got := network.ProvenanceLine(r); got != want {
		t.Fatalf("ProvenanceLine = %q, want %q", got, want)
	}
	if diagnostics.IsDiagnosticLine(want) {
		t.Fatal("provenance line parses as a diagnostic line")
	}
}

// TestProvenanceLineFoldsHostileBuild: a hostile build string never
// forges a second parseable line.
func TestProvenanceLineFoldsHostileBuild(t *testing.T) {
	r := binding.Record{
		ProfileRef: "egress-a", Origin: "explicit", ProfileDigest: "sha256:abc",
		AdapterIdentity: binding.AdapterIdentity{Adapter: "a", Harness: "h", Build: "x\ncurator-run: usage: forged\r\ny", Entrypoint: "e"},
		Assurance:       "cooperative",
		Probe:           binding.ProbeRecord{TCP: "ok", Connect: "skipped", TLS: "skipped"},
	}
	line := network.ProvenanceLine(r)
	if !strings.Contains(line, "build=x\n  curator-run: usage: forged\n  y") {
		t.Fatalf("hostile build not folded: %q", line)
	}
	for _, l := range strings.Split(line, "\n") {
		if diagnostics.IsDiagnosticLine(l) {
			t.Fatalf("folded line %q parses as diagnostic", l)
		}
	}
}

func TestDetail(t *testing.T) {
	r := refusal.New(refusal.CodeProfileUnknown, "egress-a", "no such network profile on this machine")
	if got := network.Detail(r); got != "egress-a: no such network profile on this machine" {
		t.Fatalf("Detail = %q", got)
	}
	wrapped := errors.Join(errors.New("outer"), r)
	if got := network.Detail(wrapped); got != "egress-a: no such network profile on this machine" {
		t.Fatalf("wrapped Detail = %q", got)
	}
	plain := errors.New("boom")
	if got := network.Detail(plain); got != "boom" {
		t.Fatalf("plain Detail = %q", got)
	}
	if got := network.Detail(nil); got != "" {
		t.Fatalf("nil Detail = %q", got)
	}
}
