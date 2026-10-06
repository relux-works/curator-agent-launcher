package network_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
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
	"github.com/relux-works/skill-agents-management/pkg/agentic"
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

var unitArtifact = []byte("\xcf\xfa\xed\xfe-test-native-container")

func unitBuild() string { return fmt.Sprintf("sha256-%x", sha256.Sum256(unitArtifact)) }
func baseRequest(home string) network.Request {
	return network.Request{Explicit: selectedProfile, ExplicitSet: true,
		Harness: "claude-code", Entrypoint: network.EntrypointExec,
		ParentEnv: []string{"HOME=" + home}, Artifact: unitArtifact,
		HostIdentity: binding.AdapterIdentity{Adapter: envpatch.AdapterGeneric, Harness: "claude-code", Build: unitBuild(), Entrypoint: network.EntrypointExec}}
}

const directCatalogTOML = `schema = "relux-network-profiles-v1"

[networks.direct-a]
kind = "direct"
`

const selectedDirectProfile = "direct-a"

// writeDirectCatalog mirrors writeCatalog for a named kind="direct"
// profile: no endpoint, no bypass hosts, no probe target. Confirmation
// still applies: a direct profile binds only at its confirmed digest.
func writeDirectCatalog(t *testing.T, home string, confirmed bool) (netprofile.Profile, string) {
	t.Helper()
	dir := filepath.Join(home, ".curator")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "network.toml"), []byte(directCatalogTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := netprofile.Parse([]byte(directCatalogTOML))
	if err != nil {
		t.Fatal(err)
	}
	prof, ok := file.Lookup(selectedDirectProfile)
	if !ok {
		t.Fatal("fixture profile missing after parse")
	}
	digest := netprofile.Digest(prof)
	if confirmed {
		ledger := map[string]any{
			"schema": "relux-network-confirmations-v1",
			"confirmed": map[string]any{
				selectedDirectProfile: map[string]any{"digest": digest, "confirmed_at": "2026-10-02T00:00:00Z"},
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
		name     string
		doc      string
		explicit string
		want     string
	}{
		{"syntax-error", "schema = [\n", selectedProfile, refusal.CodeFileUnreadable},
		{"bad-endpoint", strings.Replace(catalogTOML, "http://127.0.0.1:18080", "http://user@127.0.0.1:18080", 1), selectedProfile, refusal.CodeProfileInvalid},
		{"direct-with-endpoint", strings.Replace(directCatalogTOML, "kind = \"direct\"\n", "kind = \"direct\"\nendpoint = \"http://127.0.0.1:18080\"\n", 1), selectedDirectProfile, refusal.CodeProfileInvalid},
		{"direct-with-bypass", strings.Replace(directCatalogTOML, "kind = \"direct\"\n", "kind = \"direct\"\nbypass_hosts = [\"127.0.0.1\"]\n", 1), selectedDirectProfile, refusal.CodeProfileInvalid},
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
			req.Explicit = tc.explicit
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
		name       string
		harness    string
		build      string
		entrypoint string
	}{
		{"muse", "muse", "1.4.2", network.EntrypointInteractive},
		{"unknown-harness", "future-tool", "9.9", network.EntrypointInteractive},
		{"empty-build", "codex", "", network.EntrypointInteractive},
		{"blank-build", "pi-native", "  ", network.EntrypointInteractive},
		{"unlisted-build", "codex", "codex-cli 9.9.9", network.EntrypointInteractive},
		{"build-prefix", "codex", "codex-cli 0.153", network.EntrypointInteractive},
		{"build-trailing-space", "codex", "codex-cli 0.153.2 ", network.EntrypointInteractive},
		{"harness-case", "Codex", "codex-cli 0.153.2", network.EntrypointInteractive},
		{"entrypoint-mismatch", "codex", "codex-cli 0.153.2", network.EntrypointExec},
		{"empty-entrypoint", "codex", "codex-cli 0.153.2", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := baseRequest(home)
			req.Harness, req.HostIdentity.Build, req.Entrypoint = tc.harness, tc.build, tc.entrypoint
			req.Prober = forbiddenProber{t}
			_, err := network.Prepare(context.Background(), req)
			if err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported {
				t.Fatalf("error = %v, want %s", err, refusal.CodeScopeUnsupported)
			}
		})
	}
}

// Regression tests cover library admission and the independent launch scope.
func TestPrepareProductionAllowlistRefuses(t *testing.T) {
	home := t.TempDir()
	writeCatalog(t, home, true)
	req := baseRequest(home)
	req.Mode = adapterprobe.Strict
	req.Prober = forbiddenProber{t}
	_, err := network.Prepare(context.Background(), req)
	if err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported {
		t.Fatalf("production-list error = %v, want %s", err, refusal.CodeScopeUnsupported)
	}
}

// Regression tests cover library admission and the independent launch scope.
func TestPrepareProductionAllowlistAdmitsVerifiedTuple(t *testing.T) {
	home := t.TempDir()
	prof, _ := writeCatalog(t, home, true)
	sp := &scriptedProber{res: probe.Result{
		Endpoint: prof.Endpoint, TCP: probe.StatusOK,
		Connect: probe.StatusSkipped, TLS: probe.StatusSkipped, CheckedAt: time.Now(),
	}}
	req := baseRequest(home)
	req.Harness = "claude-code"
	req.Entrypoint = network.EntrypointExec

	req.Prober = sp
	got, err := network.Prepare(context.Background(), req)
	if err != nil {
		t.Fatalf("production-list Prepare error = %v", err)
	}
	if sp.calls != 1 || got.Record == nil {
		t.Fatalf("calls = %d record = %+v, want one probe and a bound record", sp.calls, got.Record)
	}
	if got.Record.AdapterIdentity.Harness != "claude-code" || got.Record.AdapterIdentity.Build != unitBuild() ||
		got.Record.AdapterIdentity.Entrypoint != network.EntrypointExec {
		t.Fatalf("record identity = %+v, want the verified claude-code print tuple", got.Record.AdapterIdentity)
	}
}

// Regression tests cover library admission and the independent launch scope.
func TestPrepareProductionAllowlistRefusesInteractiveClaude(t *testing.T) {
	home := t.TempDir()
	writeCatalog(t, home, true)
	req := baseRequest(home)
	req.Harness = "claude-code"
	req.Entrypoint = network.EntrypointInteractive

	req.Prober = forbiddenProber{t}
	_, err := network.Prepare(context.Background(), req)
	if err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported {
		t.Fatalf("production-list error = %v, want %s", err, refusal.CodeScopeUnsupported)
	}
}

// TestEntrypointForMode: the actual admitted mode maps to the policy's
// entrypoint vocabulary — one-shot to the verified entrypoint,
// interactive to the unverified one — and every other mode refuses
// typed network_scope_unsupported instead of inheriting either.
func TestEntrypointForMode(t *testing.T) {
	for _, tc := range []struct {
		mode agentic.LaunchMode
		want string
	}{
		{agentic.LaunchModeExec, network.EntrypointExec},
		{agentic.LaunchModeInteractive, network.EntrypointInteractive},
	} {
		got, err := network.EntrypointForMode(tc.mode)
		if err != nil || got != tc.want {
			t.Fatalf("EntrypointForMode(%v) = %q, %v; want %q, nil", tc.mode, got, err, tc.want)
		}
	}
	for _, mode := range []agentic.LaunchMode{
		agentic.LaunchModeDryRun, agentic.LaunchModeManagedSession,
		agentic.LaunchMode(-1), agentic.LaunchMode(42),
	} {
		got, err := network.EntrypointForMode(mode)
		if err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported {
			t.Fatalf("EntrypointForMode(%v) = %q, %v; want %s", mode, got, err, refusal.CodeScopeUnsupported)
		}
		if got != "" {
			t.Fatalf("EntrypointForMode(%v) entrypoint = %q, want empty on refusal", mode, got)
		}
	}
}

// TestEffectiveEntrypoint: round-2 F1 derivation table. The construction
// mode refines to the effective shape: an interactive claude-code plan
// whose argv tail — read only as the plan carries the native request —
// selects print classifies as the verified one-shot entrypoint, and
// every other interactive shape keeps the unverified one. Non-
// interactive modes keep the EntrypointForMode vocabulary. Each row
// narrows one derivation branch: collapsing either direction, dropping
// the separator stop, the value consumption, the harness scope, or the
// argv-tail equality each fails exactly its rows.
func TestEffectiveEntrypoint(t *testing.T) {
	prefix := []string{"--model", "claude-opus-5-5", "--disallowedTools=AskUserQuestion"}
	withPrefix := func(native ...string) []string {
		return append(append([]string{}, prefix...), native...)
	}
	for _, tc := range []struct {
		name    string
		mode    agentic.LaunchMode
		harness string
		argv    []string
		native  []string
		want    string
	}{
		{"short print selects exec", agentic.LaunchModeInteractive, "claude-code", withPrefix("-p", "hello"), []string{"-p", "hello"}, network.EntrypointExec},
		{"long print selects exec", agentic.LaunchModeInteractive, "claude-code", withPrefix("--print", "hello"), []string{"--print", "hello"}, network.EntrypointExec},
		{"print after positional selects exec", agentic.LaunchModeInteractive, "claude-code", withPrefix("hello", "-p"), []string{"hello", "-p"}, network.EntrypointExec},
		{"equals print selects exec", agentic.LaunchModeInteractive, "claude-code", withPrefix("--print=x"), []string{"--print=x"}, network.EntrypointExec},
		{"print after valued flag selects exec", agentic.LaunchModeInteractive, "claude-code", withPrefix("--model", "other", "-p"), []string{"--model", "other", "-p"}, network.EntrypointExec},
		{"no selector stays interactive", agentic.LaunchModeInteractive, "claude-code", withPrefix("hello"), []string{"hello"}, network.EntrypointInteractive},
		{"empty tail stays interactive", agentic.LaunchModeInteractive, "claude-code", prefix, nil, network.EntrypointInteractive},
		{"post-separator print stays interactive", agentic.LaunchModeInteractive, "claude-code", withPrefix("--", "-p"), []string{"--", "-p"}, network.EntrypointInteractive},
		{"value-position print stays interactive", agentic.LaunchModeInteractive, "claude-code", withPrefix("--model", "-p"), []string{"--model", "-p"}, network.EntrypointInteractive},
		{"boolean-prefix print stays interactive", agentic.LaunchModeInteractive, "claude-code", withPrefix("--verbose", "-p"), []string{"--verbose", "-p"}, network.EntrypointInteractive},
		{"drifted argv stays interactive", agentic.LaunchModeInteractive, "claude-code", withPrefix("-p"), []string{"-p", "hello"}, network.EntrypointInteractive},
		{"short argv stays interactive", agentic.LaunchModeInteractive, "claude-code", []string{"-p"}, []string{"--model", "x", "-p"}, network.EntrypointInteractive},
		{"non-claude print stays interactive", agentic.LaunchModeInteractive, "codex", withPrefix("-p", "hello"), []string{"-p", "hello"}, network.EntrypointInteractive},
		{"exec mode stays exec", agentic.LaunchModeExec, "claude-code", []string{"-p", "--output-format", "json"}, nil, network.EntrypointExec},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := network.EffectiveEntrypoint(agentic.Plan{Mode: tc.mode, Argv: tc.argv}, tc.harness, tc.native)
			if err != nil || got != tc.want {
				t.Fatalf("EffectiveEntrypoint(%v, %q, %q) = %q, %v; want %q, nil", tc.mode, tc.harness, tc.native, got, err, tc.want)
			}
		})
	}
	for _, mode := range []agentic.LaunchMode{
		agentic.LaunchModeDryRun, agentic.LaunchModeManagedSession,
		agentic.LaunchMode(-1), agentic.LaunchMode(42),
	} {
		got, err := network.EffectiveEntrypoint(agentic.Plan{Mode: mode}, "claude-code", []string{"-p"})
		if err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported {
			t.Fatalf("EffectiveEntrypoint(%v) = %q, %v; want %s", mode, got, err, refusal.CodeScopeUnsupported)
		}
		if got != "" {
			t.Fatalf("EffectiveEntrypoint(%v) entrypoint = %q, want empty on refusal", mode, got)
		}
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

// Regression tests cover library admission and the independent launch scope.
func TestProductionAllowlistHoldsVerifiedTuple(t *testing.T) {
	// Retired labels cannot become AllowedBuild records.
	home := t.TempDir()
	writeCatalog(t, home, true)
	req := baseRequest(home)
	req.Mode = adapterprobe.Strict
	if _, err := network.Prepare(context.Background(), req); err == nil {
		t.Fatal("legacy label qualified")
	}
}
func TestIdentify(t *testing.T) {
	home := t.TempDir()
	req := baseRequest(home)
	id, p, err := network.Admit(context.Background(), req, false)
	if err != nil || id != req.HostIdentity || p == nil {
		t.Fatalf("id=%+v p=%+v err=%v", id, p, err)
	}
	req.HostIdentity.Build = "2.1.287"
	if _, _, err := network.Admit(context.Background(), req, false); err == nil {
		t.Fatal("label accepted as host identity")
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
		r.AdapterIdentity.Adapter != envpatch.AdapterGeneric || r.AdapterIdentity.Harness != "claude-code" ||
		r.AdapterIdentity.Build != unitBuild() || r.AdapterIdentity.Entrypoint != network.EntrypointExec ||
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

// Regression tests cover library admission and the independent launch scope.
func TestPrepareDirectSkipsProbe(t *testing.T) {
	home := t.TempDir()
	prof, digest := writeDirectCatalog(t, home, true)
	if prof.Kind != netprofile.KindDirect {
		t.Fatalf("fixture kind = %q, want direct", prof.Kind)
	}
	req := baseRequest(home)
	req.Explicit = selectedDirectProfile
	req.Prober = forbiddenProber{t}
	before := time.Now()
	got, err := network.Prepare(context.Background(), req)
	after := time.Now()
	if err != nil {
		t.Fatalf("Prepare error = %v", err)
	}
	wantPatch := envpatch.Generic{}.Patch(prof)
	if len(wantPatch.Set) != 0 {
		t.Fatalf("library direct patch sets %d pairs, want unset-only", len(wantPatch.Set))
	}
	if len(got.Patch.Unset) != len(wantPatch.Unset) || len(got.Patch.Set) != 0 {
		t.Fatalf("patch = %+v, want unset-only %+v", got.Patch, wantPatch)
	}
	if got.Patch.Empty() {
		t.Fatal("direct patch is empty, want a managed unset-only patch")
	}
	if got.Record == nil {
		t.Fatal("record is nil on a managed direct launch")
	}
	r := *got.Record
	if r.Schema != binding.SchemaRecord || r.ProfileRef != selectedDirectProfile || r.Origin != "explicit" ||
		r.ProfileDigest != digest || r.Assurance != binding.AssuranceCooperative ||
		r.AdapterIdentity.Adapter != envpatch.AdapterGeneric || r.AdapterIdentity.Harness != "claude-code" ||
		r.AdapterIdentity.Build != unitBuild() || r.AdapterIdentity.Entrypoint != network.EntrypointExec ||
		r.Probe.TCP != "skipped" || r.Probe.Connect != "skipped" || r.Probe.TLS != "skipped" {
		t.Fatalf("record = %+v, want bound explicit direct record", r)
	}
	if r.Probe.CheckedAt.Before(before.Add(-time.Minute)) || r.Probe.CheckedAt.After(after.Add(time.Minute)) {
		t.Fatalf("record checked_at = %v, want this launch's observation time", r.Probe.CheckedAt)
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "127.0.0.1") {
		t.Fatalf("direct record serializes endpoint bytes: %s", raw)
	}
}

// TestPrepareDirectRequiresConfirmation: a direct profile binds only at
// its confirmed digest, like every widening entry. Without the ledger
// entry the launch is denied before any probe.
func TestPrepareDirectRequiresConfirmation(t *testing.T) {
	home := t.TempDir()
	writeDirectCatalog(t, home, false)
	req := baseRequest(home)
	req.Explicit = selectedDirectProfile
	req.Prober = forbiddenProber{t}
	_, err := network.Prepare(context.Background(), req)
	if err == nil || refusalCode(t, err) != refusal.CodeProfileDenied {
		t.Fatalf("error = %v, want %s", err, refusal.CodeProfileDenied)
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
