package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/network"
	"github.com/relux-works/curator-network-profiles/pkg/binding"
	"github.com/relux-works/curator-network-profiles/pkg/envpatch"
	"github.com/relux-works/curator-network-profiles/pkg/netprofile"
	"github.com/relux-works/curator-network-profiles/pkg/probe"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/plugin"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

// Production call sites under test here: run -> launch -> network.Prepare
// (resolve/validate/support/overlay-check/preflight/bind after plan.Build
// admission and before ComposeAdmittedPlanWithNetwork) ->
// ComposeWithNetwork (patch last) -> provenance -> fake provider or fake
// ax. Every refusal below is driven through run; the suite asserts the
// diagnostic code, the exit status, the probe count, and the absence of
// a child side effect.
//
// The shipped STRICT policy holds exactly the verified claude-code
// print tuple, so every codex-fixture test that must reach the probe
// injects codexTestAllowlist (the interactive fixture shape); the N1
// regressions pin the production shape and exact-tuple refusal.
// TestNetworkInteractiveClaudeRefuses pins that a managed interactive
// launch refuses under the production nil allowlist, and
// TestNetworkPrintClaudeAdmitsThroughRun pins that an explicit print
// invocation stays admitted through the production run path.

// scriptedNetworkProber records its request and replays one outcome. No
// pipeline test reaches the network.
type scriptedNetworkProber struct {
	calls int
	got   probe.Request
	res   probe.Result
	err   error
}

func (s *scriptedNetworkProber) Probe(_ context.Context, req probe.Request) (probe.Result, error) {
	s.calls++
	s.got = req
	return s.res, s.err
}

// forbiddenNetworkProber fails the test when the launch must not probe.
type forbiddenNetworkProber struct{ t *testing.T }

func (p forbiddenNetworkProber) Probe(_ context.Context, _ probe.Request) (probe.Result, error) {
	p.t.Fatal("probe ran for a launch that must not probe")
	return probe.Result{}, nil
}

const networkCatalogTOML = `schema = "relux-network-profiles-v1"

[networks.egress-a]
kind = "external-http-proxy"
endpoint = "http://127.0.0.1:18080"
bypass_hosts = ["127.0.0.1", "::1", "localhost"]
`

const directNetworkCatalogTOML = `schema = "relux-network-profiles-v1"

[networks.direct-a]
kind = "direct"
`

// writeNetworkCatalog stores the catalog under home and, when confirmed,
// a ledger confirming egress-a at its current digest. It returns the
// normalized endpoint and digest so expectations derive from the library.
func writeNetworkCatalog(t *testing.T, home string, confirmed bool) (endpoint, digest string) {
	t.Helper()
	dir := filepath.Join(home, ".curator")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "network.toml"), []byte(networkCatalogTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := netprofile.Parse([]byte(networkCatalogTOML))
	if err != nil {
		t.Fatal(err)
	}
	prof, ok := file.Lookup("egress-a")
	if !ok {
		t.Fatal("fixture profile missing after parse")
	}
	if confirmed {
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
	}
	return prof.Endpoint, netprofile.Digest(prof)
}

// writeDirectNetworkCatalog stores a catalog holding only the named
// kind="direct" profile and, when confirmed, its ledger entry. It
// returns the digest so the provenance expectation derives from it.
func writeDirectNetworkCatalog(t *testing.T, home string, confirmed bool) string {
	t.Helper()
	dir := filepath.Join(home, ".curator")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "network.toml"), []byte(directNetworkCatalogTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := netprofile.Parse([]byte(directNetworkCatalogTOML))
	if err != nil {
		t.Fatal(err)
	}
	prof, ok := file.Lookup("direct-a")
	if !ok {
		t.Fatal("fixture profile missing after parse")
	}
	if confirmed {
		ledger, err := json.Marshal(map[string]any{
			"schema": "relux-network-confirmations-v1",
			"confirmed": map[string]any{
				"direct-a": map[string]any{"digest": netprofile.Digest(prof), "confirmed_at": "2026-10-02T00:00:00Z"},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "network.confirmations.json"), ledger, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return netprofile.Digest(prof)
}

// codexTestAllowlist is the test-only support policy for the codex_cli
// pipeline fixture: exactly its normalized tool release (the module
// normalizes "codex-cli 0.153.2" to "0.153.2") at the interactive
// entrypoint — the actual shape run admits. Production passes nil, the
// shipped STRICT list, which holds only the verified claude-code print
// tuple.
func codexTestAllowlist() []binding.AdapterIdentity {
	return []binding.AdapterIdentity{{
		Adapter: envpatch.AdapterGeneric, Harness: "codex",
		Build: "0.153.2", Entrypoint: network.EntrypointInteractive,
	}}
}

// writeBrokenNetworkCatalog stores a catalog doc that cannot be used.
func writeBrokenNetworkCatalog(t *testing.T, home, doc string) {
	t.Helper()
	dir := filepath.Join(home, ".curator")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "network.toml"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeBrokenNetworkLedger overwrites the ledger with unparseable bytes.
func writeBrokenNetworkLedger(t *testing.T, home string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, ".curator", "network.confirmations.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// withNetworkFlag inserts --network before the native-argument boundary.
// The fixture is a v1 fragment without a permission transport, so under
// operator decision Q-D3 (an unconfigured default is yolo) it also states
// --permissions native unless the caller already chose a mode; these tests
// exercise the network plane, not the permission default.
func withNetworkFlag(args []string, profile string) []string {
	explicit := false
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "--yolo" || a == "--permissions" || strings.HasPrefix(a, "--permissions=") {
			explicit = true
		}
	}
	extra := []string{"--network", profile}
	if !explicit {
		extra = append(extra, "--permissions", "native")
	}
	out := make([]string, 0, len(args)+len(extra))
	inserted := false
	for _, a := range args {
		if !inserted && a == "--" {
			out = append(out, extra...)
			inserted = true
		}
		out = append(out, a)
	}
	if !inserted {
		out = append(out, extra...)
	}
	return out
}

func okProbe(endpoint string) probe.Result {
	return probe.Result{Endpoint: endpoint, TCP: probe.StatusOK,
		Connect: probe.StatusSkipped, TLS: probe.StatusSkipped,
		CheckedAt: time.Date(2026, 10, 2, 7, 0, 0, 0, time.UTC)}
}

// TestNetworkDirectLaunchAppliesPatchLast: a confirmed selection on a
// supported direct launch probes once (bounded, targetless), replaces
// ambient proxies in every letter case with the patch, keeps siblings,
// and prints the Record as provenance — while the endpoint stays out of
// stderr and the digest stays out of the child environment.
func TestNetworkDirectLaunchAppliesPatchLast(t *testing.T) {
	f := entryFixture(t, "codex_cli", false)
	endpoint, digest := writeNetworkCatalog(t, f.dir, true)
	f.addEnv("HTTP_PROXY", "http://ambient:8080")
	f.addEnv("Http_Proxy", "http://mixed:8080")
	sp := &scriptedNetworkProber{res: okProbe(endpoint)}
	f.deps.prober = sp
	f.deps.networkAllowlist = codexTestAllowlist()
	f.args = withNetworkFlag(f.args, "egress-a")
	code, out, stderr := f.run()
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if f.builds != 1 || f.verdicts != 1 || f.resolver.calls != 1 || sp.calls != 1 {
		t.Fatalf("calls build=%d verdict=%d resolve=%d probe=%d, want 1/1/1/1", f.builds, f.verdicts, f.resolver.calls, sp.calls)
	}
	if sp.got.Subject != "egress-a" || sp.got.Endpoint != endpoint || sp.got.Target != "" || sp.got.Timeout != probe.DefaultTimeout {
		t.Fatalf("probe request = %+v, want bounded targetless probe", sp.got)
	}
	var child childCapture
	if err := json.Unmarshal(out, &child); err != nil {
		t.Fatal(err)
	}
	joined := "\n" + strings.Join(child.Env, "\n") + "\n"
	for _, want := range []string{
		"HTTP_PROXY=" + endpoint, "HTTPS_PROXY=" + endpoint,
		"http_proxy=" + endpoint, "https_proxy=" + endpoint,
		"NO_PROXY=127.0.0.1,::1,localhost", "no_proxy=127.0.0.1,::1,localhost",
	} {
		if !strings.Contains(joined, "\n"+want+"\n") {
			t.Fatalf("child env lacks %q in %q", want, child.Env)
		}
	}
	if strings.Contains(joined, "ambient") || strings.Contains(joined, "mixed") {
		t.Fatalf("ambient proxy survives in child env: %q", child.Env)
	}
	if strings.Contains(joined, digest) || strings.Contains(joined, "relux-network-binding-record-v1") {
		t.Fatalf("record material leaks into child env: %q", child.Env)
	}
	wantLine := "curator-run: network: profile=egress-a origin=explicit digest=" + digest +
		" adapter=generic-env-v1 harness=codex build=0.153.2 entrypoint=interactive assurance=cooperative probe=ok/skipped/skipped\n"
	if !strings.Contains(string(stderr), wantLine) {
		t.Fatalf("stderr lacks provenance line %q in %q", wantLine, stderr)
	}
	if strings.Contains(string(stderr), endpoint) {
		t.Fatalf("endpoint leaks into stderr provenance: %q", stderr)
	}
}

// TestNetworkDirectLaunchSkipsProbe: a confirmed kind="direct" selection
// on a supported direct launch binds without probing — the forbidden
// prober fails the test when called — removes ambient proxies in every
// letter case while setting none, and prints the Record as provenance
// with skipped/skipped/skipped. Production call site: run.
func TestNetworkDirectLaunchSkipsProbe(t *testing.T) {
	f := entryFixture(t, "codex_cli", false)
	digest := writeDirectNetworkCatalog(t, f.dir, true)
	f.addEnv("HTTP_PROXY", "http://ambient:8080")
	f.addEnv("Http_Proxy", "http://mixed:8080")
	f.deps.prober = forbiddenNetworkProber{t}
	f.deps.networkAllowlist = codexTestAllowlist()
	f.args = withNetworkFlag(f.args, "direct-a")
	code, out, stderr := f.run()
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if f.builds != 1 || f.verdicts != 1 || f.resolver.calls != 1 {
		t.Fatalf("calls build=%d verdict=%d resolve=%d, want 1/1/1", f.builds, f.verdicts, f.resolver.calls)
	}
	var child childCapture
	if err := json.Unmarshal(out, &child); err != nil {
		t.Fatal(err)
	}
	for _, entry := range child.Env {
		name, _, _ := strings.Cut(entry, "=")
		if network.IsProxyFamily(name) {
			t.Fatalf("direct child env carries proxy variable %q in %q", entry, child.Env)
		}
	}
	joined := "\n" + strings.Join(child.Env, "\n") + "\n"
	if strings.Contains(joined, "ambient") || strings.Contains(joined, "mixed") {
		t.Fatalf("ambient proxy survives in direct child env: %q", child.Env)
	}
	if strings.Contains(joined, digest) || strings.Contains(joined, "relux-network-binding-record-v1") {
		t.Fatalf("record material leaks into child env: %q", child.Env)
	}
	wantLine := "curator-run: network: profile=direct-a origin=explicit digest=" + digest +
		" adapter=generic-env-v1 harness=codex build=0.153.2 entrypoint=interactive assurance=cooperative probe=skipped/skipped/skipped\n"
	if !strings.Contains(string(stderr), wantLine) {
		t.Fatalf("stderr lacks provenance line %q in %q", wantLine, stderr)
	}
}

// TestNetworkReadsOperatorHomeNotManagedHome: the catalog is read with
// the operator's original parentEnv. A catalog under the managed
// fragment home alone does not resolve.
func TestNetworkReadsOperatorHomeNotManagedHome(t *testing.T) {
	f := entryFixture(t, "codex_cli", false)
	writeNetworkCatalog(t, f.home, true)
	f.deps.prober = forbiddenNetworkProber{t}
	f.args = withNetworkFlag(f.args, "egress-a")
	code, out, stderr := f.run()
	if code != 1 || len(out) != 0 {
		t.Fatalf("exit=%d out=%q stderr=%s", code, out, stderr)
	}
	if !strings.Contains(string(stderr), "curator-run: network_profile_unknown: ") {
		t.Fatalf("stderr=%s, want network_profile_unknown", stderr)
	}
	if f.builds != 1 || f.verdicts != 1 {
		t.Fatalf("builds=%d verdicts=%d, want admission before the network refusal", f.builds, f.verdicts)
	}
	f.assertNoChild(t)
}

// TestNetworkRefusalsSkipProbe: every resolve/validate/support refusal
// terminates with its own code after admission, without probing and
// without a child.
func TestNetworkRefusalsSkipProbe(t *testing.T) {
	for _, tc := range []struct {
		name    string
		profile string
		setup   func(t *testing.T, f *pipelineFixture)
		want    string
	}{
		{"unknown", "no-such-profile", func(t *testing.T, f *pipelineFixture) {
			writeNetworkCatalog(t, f.dir, true)
		}, "network_profile_unknown"},
		{"unconfirmed", "egress-a", func(t *testing.T, f *pipelineFixture) {
			writeNetworkCatalog(t, f.dir, false)
		}, "network_profile_denied"},
		{"unconfirmed-direct", "direct-a", func(t *testing.T, f *pipelineFixture) {
			writeDirectNetworkCatalog(t, f.dir, false)
		}, "network_profile_denied"},
		{"no-catalog", "egress-a", func(t *testing.T, f *pipelineFixture) {}, "network_profile_unknown"},
		{"invalid-profile", "egress-a", func(t *testing.T, f *pipelineFixture) {
			writeBrokenNetworkCatalog(t, f.dir, strings.Replace(networkCatalogTOML, "http://127.0.0.1:18080", "http://user@127.0.0.1:18080", 1))
		}, "network_profile_invalid"},
		{"blank", "  ", func(t *testing.T, f *pipelineFixture) {
			writeNetworkCatalog(t, f.dir, true)
		}, "network_profile_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := entryFixture(t, "codex_cli", false)
			tc.setup(t, f)
			f.deps.prober = forbiddenNetworkProber{t}
			f.args = withNetworkFlag(f.args, tc.profile)
			code, out, stderr := f.run()
			if code != 1 || len(out) != 0 {
				t.Fatalf("exit=%d out=%q stderr=%s", code, out, stderr)
			}
			if !strings.Contains(string(stderr), "curator-run: "+tc.want+": ") {
				t.Fatalf("stderr=%s, want %s", stderr, tc.want)
			}
			if f.builds != 1 || f.verdicts != 1 {
				t.Fatalf("builds=%d verdicts=%d, want admission before the network refusal", f.builds, f.verdicts)
			}
			f.assertNoChild(t)
		})
	}
}

// TestNetworkTrackedRefuses: tracked mode with --network refuses
// network_scope_unsupported after admission, before any catalog read,
// probe, or ax handoff. T1 regression: the fixture injects the admitted
// codex tuple, so the tracked gate — not the later unlisted-tuple gate
// (same code) — is the refusal under test. The no-catalog subtest pins
// refusal-before-catalog: a tracked exemption there would surface a
// different code (unknown), and in the confirmed case it would reach
// the forbidden prober.
func TestNetworkTrackedRefuses(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, f *pipelineFixture)
	}{
		{"confirmed-catalog", func(t *testing.T, f *pipelineFixture) {
			writeNetworkCatalog(t, f.dir, true)
		}},
		{"no-catalog", func(t *testing.T, f *pipelineFixture) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := entryFixture(t, "codex_cli", true)
			tc.setup(t, f)
			// Admitted tuple: the tracked refusal below cannot be
			// satisfied by the later support gate.
			f.deps.networkAllowlist = codexTestAllowlist()
			f.deps.prober = forbiddenNetworkProber{t}
			f.args = withNetworkFlag(f.args, "egress-a")
			code, out, stderr := f.run()
			if code != 1 || len(out) != 0 {
				t.Fatalf("exit=%d out=%q stderr=%s", code, out, stderr)
			}
			if !strings.Contains(string(stderr), "curator-run: network_scope_unsupported: ") {
				t.Fatalf("stderr=%s, want network_scope_unsupported", stderr)
			}
			if strings.Contains(string(stderr), "curator-run: network: ") {
				t.Fatalf("refused launch prints a binding Record: %s", stderr)
			}
			if f.builds != 1 || f.verdicts != 1 {
				t.Fatalf("builds=%d verdicts=%d, want admission before the tracked refusal", f.builds, f.verdicts)
			}
			f.assertNoChild(t)
		})
	}
}

// TestNetworkMuseRefuses: Muse has no verified adapter tuple — only
// the claude-code 2.1.287 print tuple is verified — and refuses
// network_scope_unsupported even direct, without probing.
func TestNetworkMuseRefuses(t *testing.T) {
	f, _ := museFixture(t)
	writeNetworkCatalog(t, f.dir, true)
	f.deps.prober = forbiddenNetworkProber{t}
	f.args = withNetworkFlag(f.args, "egress-a")
	code, out, stderr := f.run()
	if code != 1 || len(out) != 0 {
		t.Fatalf("exit=%d out=%q stderr=%s", code, out, stderr)
	}
	if !strings.Contains(string(stderr), "curator-run: network_scope_unsupported: ") {
		t.Fatalf("stderr=%s, want network_scope_unsupported", stderr)
	}
	if f.builds != 1 || f.verdicts != 1 {
		t.Fatalf("builds=%d verdicts=%d, want admission before the support refusal", f.builds, f.verdicts)
	}
	f.assertNoChild(t)
}

// TestNetworkProbeFailureTerminates: a preflight error terminates
// without weaker routing: no unmanaged child, exit 1, the probe's code.
func TestNetworkProbeFailureTerminates(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"unreachable", refusal.New(refusal.CodeProxyUnreachable, "egress-a", "tcp: connection refused"), "network_proxy_unreachable"},
		{"auth-failed", refusal.New(refusal.CodeProxyAuthFailed, "egress-a", "connect: proxy authentication required (407)"), "network_proxy_auth_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := entryFixture(t, "codex_cli", false)
			writeNetworkCatalog(t, f.dir, true)
			sp := &scriptedNetworkProber{err: tc.err}
			f.deps.prober = sp
			f.deps.networkAllowlist = codexTestAllowlist()
			f.args = withNetworkFlag(f.args, "egress-a")
			code, out, stderr := f.run()
			if code != 1 || len(out) != 0 {
				t.Fatalf("exit=%d out=%q stderr=%s", code, out, stderr)
			}
			if !strings.Contains(string(stderr), "curator-run: "+tc.want+": ") {
				t.Fatalf("stderr=%s, want %s", stderr, tc.want)
			}
			if sp.calls != 1 {
				t.Fatalf("probe calls = %d, want 1", sp.calls)
			}
			f.assertNoChild(t)
		})
	}
}

// TestNetworkAdmissionFailureSkipsProbe: a refused plan invokes neither
// the prober nor the workload, even with --network.
func TestNetworkAdmissionFailureSkipsProbe(t *testing.T) {
	f := entryFixture(t, "codex_cli", false)
	writeNetworkCatalog(t, f.dir, true)
	f.deps.prober = forbiddenNetworkProber{t}
	f.args = withNetworkFlag(f.args, "egress-a")
	for i, a := range f.args {
		if a == "--model" {
			f.args[i+1] = "no-such-model-xyz"
		}
	}
	code, out, stderr := f.run()
	if code != 1 || len(out) != 0 {
		t.Fatalf("exit=%d out=%q stderr=%s", code, out, stderr)
	}
	if !strings.Contains(string(stderr), "curator-run: plan_refused: ") {
		t.Fatalf("stderr=%s, want plan_refused, not a network code", stderr)
	}
	if strings.Contains(string(stderr), "curator-run: network") {
		t.Fatalf("network stage ran after admission failure: %s", stderr)
	}
	f.assertNoChild(t)
}

// TestNetworkOverlayConflictRefusesBeforeProbe: a proxy-family name in
// the fragment's MCP env_names refuses with
// network_configuration_conflict before the preflight: the pure check
// never costs a network probe, starts no child, and prints no binding
// Record — provenance follows a composed launch only.
func TestNetworkOverlayConflictRefusesBeforeProbe(t *testing.T) {
	f := entryFixture(t, "codex_cli", false)
	writeNetworkCatalog(t, f.dir, true)
	var obj map[string]any
	if err := json.Unmarshal([]byte(f.resolver.stdout), &obj); err != nil {
		t.Fatal(err)
	}
	obj["mcp"].(map[string]any)["env_names"] = []string{"Http_Proxy"}
	raw, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	f.resolver.stdout = string(raw) + "\n"
	f.deps.networkAllowlist = codexTestAllowlist()
	f.deps.prober = forbiddenNetworkProber{t}
	f.args = withNetworkFlag(f.args, "egress-a")
	code, out, stderr := f.run()
	if code != 1 || len(out) != 0 {
		t.Fatalf("exit=%d out=%q stderr=%s", code, out, stderr)
	}
	if !strings.Contains(string(stderr), "curator-run: network_configuration_conflict: ") {
		t.Fatalf("stderr=%s, want network_configuration_conflict", stderr)
	}
	if strings.Contains(string(stderr), "curator-run: network: ") {
		t.Fatalf("refused composition prints a binding Record: %s", stderr)
	}
	if f.builds != 1 || f.verdicts != 1 {
		t.Fatalf("builds=%d verdicts=%d, want admission before the network refusal", f.builds, f.verdicts)
	}
	f.assertNoChild(t)
}

// TestNetworkProductionAllowlistRefusesUnlistedCodex: N1 regression for
// the shipped policy (rev2 name TestNetworkProductionAllowlistEmptyRefuses;
// the policy now holds the verified claude-code print tuple instead of being
// empty). With no injected allowlist — the production shape — a confirmed
// codex selection refuses network_scope_unsupported: codex is unlisted.
func TestNetworkProductionAllowlistRefusesUnlistedCodex(t *testing.T) {
	f := entryFixture(t, "codex_cli", false)
	writeNetworkCatalog(t, f.dir, true)
	f.deps.prober = forbiddenNetworkProber{t}
	// No networkAllowlist: the production list applies, which holds
	// only the verified claude-code print tuple.
	f.args = withNetworkFlag(f.args, "egress-a")
	code, out, stderr := f.run()
	if code != 1 || len(out) != 0 {
		t.Fatalf("exit=%d out=%q stderr=%s", code, out, stderr)
	}
	if !strings.Contains(string(stderr), "curator-run: network_scope_unsupported: ") {
		t.Fatalf("stderr=%s, want network_scope_unsupported", stderr)
	}
	if strings.Contains(string(stderr), "curator-run: network: ") {
		t.Fatalf("refused launch prints a binding Record: %s", stderr)
	}
	if f.builds != 1 || f.verdicts != 1 {
		t.Fatalf("builds=%d verdicts=%d, want admission before the network refusal", f.builds, f.verdicts)
	}
	f.assertNoChild(t)
}

// TestNetworkUnlistedTupleRefuses: N1 regression for exactness. The
// injected list names only another build — entrypoint included at the
// actual interactive shape, so exactly one member differs — and this
// launch's exact tuple refuses network_scope_unsupported without
// probing.
func TestNetworkUnlistedTupleRefuses(t *testing.T) {
	f := entryFixture(t, "codex_cli", false)
	writeNetworkCatalog(t, f.dir, true)
	f.deps.networkAllowlist = []binding.AdapterIdentity{{
		Adapter: envpatch.AdapterGeneric, Harness: "codex",
		Build: "9.9.9", Entrypoint: network.EntrypointInteractive,
	}}
	f.deps.prober = forbiddenNetworkProber{t}
	f.args = withNetworkFlag(f.args, "egress-a")
	code, out, stderr := f.run()
	if code != 1 || len(out) != 0 {
		t.Fatalf("exit=%d out=%q stderr=%s", code, out, stderr)
	}
	if !strings.Contains(string(stderr), "curator-run: network_scope_unsupported: ") {
		t.Fatalf("stderr=%s, want network_scope_unsupported", stderr)
	}
	if f.builds != 1 || f.verdicts != 1 {
		t.Fatalf("builds=%d verdicts=%d, want admission before the network refusal", f.builds, f.verdicts)
	}
	f.assertNoChild(t)
}

// TestNetworkTupleIdentityDimensionsRefuse: X2 regression. The STRICT
// support policy compares the exact (adapter, harness, build,
// entrypoint) identity at the actual admitted entrypoint — interactive
// in every run test. Each subtest injects a list whose single entry
// changes exactly ONE member from this launch's admitted codex tuple,
// and each refuses network_scope_unsupported through the production run
// path: after admission, without probing, without a child, without a
// Record. A mutant that ignores any one equality fails exactly its
// subtest.
func TestNetworkTupleIdentityDimensionsRefuse(t *testing.T) {
	admitted := codexTestAllowlist()[0]
	for _, tc := range []struct {
		name  string
		entry binding.AdapterIdentity
	}{
		{"adapter", binding.AdapterIdentity{Adapter: "other-adapter-v9", Harness: admitted.Harness, Build: admitted.Build, Entrypoint: admitted.Entrypoint}},
		{"harness", binding.AdapterIdentity{Adapter: admitted.Adapter, Harness: "codex-fork", Build: admitted.Build, Entrypoint: admitted.Entrypoint}},
		{"build", binding.AdapterIdentity{Adapter: admitted.Adapter, Harness: admitted.Harness, Build: "0.153.3", Entrypoint: admitted.Entrypoint}},
		{"entrypoint", binding.AdapterIdentity{Adapter: admitted.Adapter, Harness: admitted.Harness, Build: admitted.Build, Entrypoint: "spawn"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := entryFixture(t, "codex_cli", false)
			writeNetworkCatalog(t, f.dir, true)
			f.deps.networkAllowlist = []binding.AdapterIdentity{tc.entry}
			f.deps.prober = forbiddenNetworkProber{t}
			f.args = withNetworkFlag(f.args, "egress-a")
			code, out, stderr := f.run()
			if code != 1 || len(out) != 0 {
				t.Fatalf("exit=%d out=%q stderr=%s", code, out, stderr)
			}
			if !strings.Contains(string(stderr), "curator-run: network_scope_unsupported: ") {
				t.Fatalf("stderr=%s, want network_scope_unsupported", stderr)
			}
			if strings.Contains(string(stderr), "curator-run: network: ") {
				t.Fatalf("refused launch prints a binding Record: %s", stderr)
			}
			if f.builds != 1 || f.verdicts != 1 {
				t.Fatalf("builds=%d verdicts=%d, want admission before the network refusal", f.builds, f.verdicts)
			}
			f.assertNoChild(t)
		})
	}
}

// withTestRelease replaces the fake tool-release answer the fixture's
// stub binaries print for --version. The entry fixture answers a fixed
// release per environment; the verified-tuple test needs 2.1.287.
func withTestRelease(f *pipelineFixture, release string) {
	previous := f.deps.environ
	f.deps.environ = func() []string {
		out := previous()
		for i, entry := range out {
			if strings.HasPrefix(entry, "CURATOR_TEST_RELEASE=") {
				out[i] = "CURATOR_TEST_RELEASE=" + release
			}
		}
		return out
	}
}

// TestNetworkInteractiveClaudeRefuses: F1 regression. The launcher
// constructs interactive launches, and the pinned v0.2.1 verification
// covers the one-shot `claude -p` shape while expressly excluding
// interactive mode — so a managed Claude launch without an explicit
// print selection, at the verified harness and build, refuses
// network_scope_unsupported through the production run path: after
// admission, without probing, without a child, without a Record.
// Production call site: run. The verified print shape stays admitted
// through run (see TestNetworkPrintClaudeAdmitsThroughRun); the
// product behavior is NOT switched to print mode to satisfy this gate.
func TestNetworkInteractiveClaudeRefuses(t *testing.T) {
	f := entryFixture(t, "claude_code", false)
	withTestRelease(f, "2.1.287")
	writeNetworkCatalog(t, f.dir, true)
	f.deps.prober = forbiddenNetworkProber{t}
	// No networkAllowlist: the production STRICT list applies, which
	// holds only the verified print tuple.
	f.args = withNetworkFlag(f.args, "egress-a")
	code, out, stderr := f.run()
	if code != 1 || len(out) != 0 {
		t.Fatalf("exit=%d out=%q stderr=%s", code, out, stderr)
	}
	if !strings.Contains(string(stderr), "curator-run: network_scope_unsupported: ") {
		t.Fatalf("stderr=%s, want network_scope_unsupported", stderr)
	}
	if strings.Contains(string(stderr), "curator-run: network: ") {
		t.Fatalf("refused launch prints a binding Record: %s", stderr)
	}
	if f.builds != 1 || f.verdicts != 1 {
		t.Fatalf("builds=%d verdicts=%d, want admission before the network refusal", f.builds, f.verdicts)
	}
	f.assertNoChild(t)
}

// TestNetworkPrintClaudeAdmitsThroughRun: round-2 F1 regression. An
// explicit print invocation — the native tail carries -p — is the
// verified `claude -p` entrypoint and stays ADMITTED under --network
// through the production run path: exit 0, exactly one probe, the
// workload starts carrying the native tail, and the Record prints as
// provenance with entrypoint=exec. The unmanaged subtest is the
// product-behavior control: no network stage, no provenance. The
// construction mode stays interactive in both (plan.Build admits
// interactive launches only); the gate classifies the EFFECTIVE shape.
// A Prepare-only positive is not proof of this requirement.
func TestNetworkPrintClaudeAdmitsThroughRun(t *testing.T) {
	for _, managed := range []bool{false, true} {
		name := "unmanaged"
		if managed {
			name = "managed"
		}
		t.Run(name, func(t *testing.T) {
			f := entryFixture(t, "claude_code", false)
			withTestRelease(f, "2.1.287")
			endpoint, digest := writeNetworkCatalog(t, f.dir, true)
			sp := &scriptedNetworkProber{res: okProbe(endpoint)}
			f.deps.prober = sp
			// No networkAllowlist: the production STRICT list applies,
			// which holds only the verified print tuple.
			f.args = []string{"claude_code", "--permissions", "native", "--", "-p", "hello"}
			if managed {
				f.args = withNetworkFlag(f.args, "egress-a")
			}
			code, out, stderr := f.run()
			if code != 0 {
				t.Fatalf("exit=%d stderr=%s", code, stderr)
			}
			var child childCapture
			if err := json.Unmarshal(out, &child); err != nil {
				t.Fatal(err)
			}
			tail := child.Argv[len(child.Argv)-2:]
			if tail[0] != "-p" || tail[1] != "hello" {
				t.Fatalf("child argv tail = %q, want the opaque native print tail", tail)
			}
			if f.plan.Mode != agentic.LaunchModeInteractive {
				t.Fatalf("admitted mode = %v, want the unchanged interactive construction", f.plan.Mode)
			}
			if !managed {
				if sp.calls != 0 {
					t.Fatalf("probe calls = %d, want 0 without --network", sp.calls)
				}
				if strings.Contains(string(stderr), "curator-run: network:") {
					t.Fatalf("unmanaged launch prints network provenance: %s", stderr)
				}
				return
			}
			if sp.calls != 1 {
				t.Fatalf("probe calls = %d, want exactly 1", sp.calls)
			}
			wantLine := "curator-run: network: profile=egress-a origin=explicit digest=" + digest +
				" adapter=generic-env-v1 harness=claude-code build=2.1.287 entrypoint=exec assurance=cooperative probe=ok/skipped/skipped\n"
			if !strings.Contains(string(stderr), wantLine) {
				t.Fatalf("stderr lacks provenance line %q in %q", wantLine, stderr)
			}
		})
	}
}

// TestNetworkUnreadableFilesRefuse: N2 regression. A catalog that exists
// but cannot be used, and a ledger that cannot be parsed, refuse as
// network_file_unreadable through the production run path: no unmanaged
// launch, no probe, no child.
func TestNetworkUnreadableFilesRefuse(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, f *pipelineFixture)
	}{
		{"catalog-syntax", func(t *testing.T, f *pipelineFixture) {
			writeBrokenNetworkCatalog(t, f.dir, "schema = [\n")
		}},
		{"catalog-is-directory", func(t *testing.T, f *pipelineFixture) {
			dir := filepath.Join(f.dir, ".curator")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(dir, "network.toml"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{"ledger-broken", func(t *testing.T, f *pipelineFixture) {
			writeNetworkCatalog(t, f.dir, true)
			writeBrokenNetworkLedger(t, f.dir)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := entryFixture(t, "codex_cli", false)
			tc.setup(t, f)
			f.deps.prober = forbiddenNetworkProber{t}
			f.args = withNetworkFlag(f.args, "egress-a")
			code, out, stderr := f.run()
			if code != 1 || len(out) != 0 {
				t.Fatalf("exit=%d out=%q stderr=%s", code, out, stderr)
			}
			if !strings.Contains(string(stderr), "curator-run: network_file_unreadable: ") {
				t.Fatalf("stderr=%s, want network_file_unreadable", stderr)
			}
			if f.builds != 1 || f.verdicts != 1 {
				t.Fatalf("builds=%d verdicts=%d, want admission before the network refusal", f.builds, f.verdicts)
			}
			f.assertNoChild(t)
		})
	}
}

// TestNetworkEngineHostsChecked: EngineHosts reach resolve through run. A
// covered host launches; an uncovered one refuses
// network_configuration_conflict without probing.
func TestNetworkEngineHostsChecked(t *testing.T) {
	t.Run("covered", func(t *testing.T) {
		f := entryFixture(t, "codex_cli", false)
		endpoint, _ := writeNetworkCatalog(t, f.dir, true)
		f.deps.networkAllowlist = codexTestAllowlist()
		f.deps.networkEngineHosts = []string{"[::1]:11434", "LOCALHOST"}
		sp := &scriptedNetworkProber{res: okProbe(endpoint)}
		f.deps.prober = sp
		f.args = withNetworkFlag(f.args, "egress-a")
		code, _, stderr := f.run()
		if code != 0 {
			t.Fatalf("exit=%d stderr=%s", code, stderr)
		}
		if sp.calls != 1 {
			t.Fatalf("probe calls = %d, want 1", sp.calls)
		}
	})
	t.Run("uncovered", func(t *testing.T) {
		f := entryFixture(t, "codex_cli", false)
		writeNetworkCatalog(t, f.dir, true)
		f.deps.networkAllowlist = codexTestAllowlist()
		f.deps.networkEngineHosts = []string{"engine.local"}
		f.deps.prober = forbiddenNetworkProber{t}
		f.args = withNetworkFlag(f.args, "egress-a")
		code, out, stderr := f.run()
		if code != 1 || len(out) != 0 {
			t.Fatalf("exit=%d out=%q stderr=%s", code, out, stderr)
		}
		if !strings.Contains(string(stderr), "curator-run: network_configuration_conflict: ") {
			t.Fatalf("stderr=%s, want network_configuration_conflict", stderr)
		}
		if !strings.Contains(string(stderr), "engine.local") {
			t.Fatalf("stderr=%s does not name the uncovered host", stderr)
		}
		if f.builds != 1 || f.verdicts != 1 {
			t.Fatalf("builds=%d verdicts=%d, want admission before the network refusal", f.builds, f.verdicts)
		}
		f.assertNoChild(t)
	})
}

// TestNetworkComposeRefusalPrintsNoRecord: a launch that composition
// refuses after a successful §4.4b stage prints no binding Record. The
// build boundary admits a plan whose argv drifted from the requested
// native suffix, so ComposeAdmittedPlanWithNetwork refuses; the probe
// ran (Prepare succeeded) but nothing prints and no child starts.
func TestNetworkComposeRefusalPrintsNoRecord(t *testing.T) {
	f := entryFixture(t, "codex_cli", false)
	endpoint, _ := writeNetworkCatalog(t, f.dir, true)
	f.deps.networkAllowlist = codexTestAllowlist()
	sp := &scriptedNetworkProber{res: okProbe(endpoint)}
	f.deps.prober = sp
	inner := f.deps.build
	f.deps.build = func(ctx context.Context, r *vendorplugin.Registry, req vendorplugin.SpawnRequest, mode agentic.LaunchMode) (agentic.PlanWithEnvironment, error) {
		built, err := inner(ctx, r, req, mode)
		if err == nil && len(built.Plan.Argv) > 0 {
			built.Plan.Argv = built.Plan.Argv[:len(built.Plan.Argv)-1]
		}
		return built, err
	}
	f.args = withNetworkFlag(f.args, "egress-a")
	code, out, stderr := f.run()
	if code != 1 || len(out) != 0 {
		t.Fatalf("exit=%d out=%q stderr=%s", code, out, stderr)
	}
	if !strings.Contains(string(stderr), "curator-run: plan_refused: ") {
		t.Fatalf("stderr=%s, want plan_refused for the drifted plan", stderr)
	}
	if strings.Contains(string(stderr), "curator-run: network: ") {
		t.Fatalf("refused composition prints a binding Record: %s", stderr)
	}
	if sp.calls != 1 {
		t.Fatalf("probe calls = %d, want 1 (Prepare succeeded before composition refused)", sp.calls)
	}
	f.assertNoChild(t)
}

// TestNetworkProductionCarriesNoEngine pins the engine bound: the
// admitted production plan carries no engine, so the nil production
// EngineHosts (coverage vacuous) is correct. A future engine-capable
// runtime fails here and must derive its hosts at the launch call site.
func TestNetworkProductionCarriesNoEngine(t *testing.T) {
	f := entryFixture(t, "codex_cli", false)
	endpoint, _ := writeNetworkCatalog(t, f.dir, true)
	f.deps.networkAllowlist = codexTestAllowlist()
	sp := &scriptedNetworkProber{res: okProbe(endpoint)}
	f.deps.prober = sp
	f.args = withNetworkFlag(f.args, "egress-a")
	code, _, stderr := f.run()
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if f.plan.Provenance.ResolvedEngine != (plugin.Ref{}) {
		t.Fatalf("resolved engine = %+v, want zero: engine coverage needs hosts", f.plan.Provenance.ResolvedEngine)
	}
	if f.deps.networkEngineHosts != nil {
		t.Fatalf("production engine hosts = %q, want nil", f.deps.networkEngineHosts)
	}
}

// TestNetworkFlagAfterDoubleDashIsNative: --network past -- is opaque
// native input, not a selection: the launch proceeds unmanaged with the
// tokens in the child argv, no provenance, and no probe — even with a
// confirmed catalog on disk.
func TestNetworkFlagAfterDoubleDashIsNative(t *testing.T) {
	f := entryFixture(t, "codex_cli", false)
	writeNetworkCatalog(t, f.dir, true)
	f.deps.prober = forbiddenNetworkProber{t}
	f.args = []string{"codex_cli", "--model", "gpt-6-astra", "--effort", "medium", "--system-prompt", "replace", "--permissions", "native", "--", "--network", "egress-a"}
	code, out, stderr := f.run()
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	var child childCapture
	if err := json.Unmarshal(out, &child); err != nil {
		t.Fatal(err)
	}
	tail := child.Argv[len(child.Argv)-2:]
	if tail[0] != "--network" || tail[1] != "egress-a" {
		t.Fatalf("child argv tail = %q, want the opaque --network pair", tail)
	}
	if strings.Contains(string(stderr), "curator-run: network:") {
		t.Fatalf("unmanaged launch prints network provenance: %s", stderr)
	}
}

// TestEmitNetworkFailureRetainsRefusal: a typed refusal keeps its code
// and renders subject and detail; anything else stays plan_refused.
func TestEmitNetworkFailureRetainsRefusal(t *testing.T) {
	var b strings.Builder
	code := emitNetworkFailure(&b, refusal.New(refusal.CodeProxyUnreachable, "egress-a", "tcp: connection refused"))
	if code != 1 || b.String() != "curator-run: network_proxy_unreachable: egress-a: tcp: connection refused\n" {
		t.Fatalf("code=%d stderr=%q", code, b.String())
	}
	b.Reset()
	joined := refusal.New(refusal.CodeScopeUnsupported, "muse", "no verified tuple")
	code = emitNetworkFailure(&b, joined)
	if code != 1 || !strings.HasPrefix(b.String(), "curator-run: network_scope_unsupported: muse: ") {
		t.Fatalf("code=%d stderr=%q", code, b.String())
	}
	b.Reset()
	code = emitNetworkFailure(&b, context.DeadlineExceeded)
	if code != 1 || !strings.HasPrefix(b.String(), "curator-run: plan_refused: ") {
		t.Fatalf("code=%d stderr=%q, want plan_refused", code, b.String())
	}
}

// TestNetworkDependencyReleasePin: the §4.4b library is consumed by tag
// as a normal require — v0.3.1, no replacement, no workspace — so a
// same-version substitution cannot pass the behavioral suite. It
// mirrors TestConstructionDependencyReleasePin for the release module.
func TestNetworkDependencyReleasePin(t *testing.T) {
	for _, name := range []string{"go.work", "go.work.sum"} {
		if _, err := os.Stat(filepath.Join("..", "..", name)); !os.IsNotExist(err) {
			t.Fatalf("release candidate must have no %s: %v", name, err)
		}
	}
	cmd := exec.Command("go", "list", "-m", "-json", "github.com/relux-works/curator-network-profiles")
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GOWORK=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("read network module identity: %v", err)
	}
	var module struct {
		Version string
		Replace *json.RawMessage
	}
	if err := json.Unmarshal(out, &module); err != nil {
		t.Fatal(err)
	}
	if module.Version != "v0.3.1" || module.Replace != nil {
		t.Fatalf("network module must be v0.3.1 without replacement: version=%s replaced=%v", module.Version, module.Replace != nil)
	}
}

// TestNativeHostFlagsComposeWithNetwork pins that the host-flag slot leaves
// the native --network path (SPEC §4.4b) intact: an explicit --native or its
// --untracked synonym admits a supported direct selection and prints the
// provenance Record, and on an ax-configured machine it bypasses ax and the
// tracked refusal with it. Only --hosted refuses a network selection, with
// exit 16 and zero builds (TestHostedRefusalsBeforeContact).
func TestNativeHostFlagsComposeWithNetwork(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tracked bool
		flag    string
	}{
		{"native-untracked-machine", false, "--native"},
		{"untracked-synonym", false, "--untracked"},
		{"native-bypasses-tracked-refusal", true, "--native"},
		{"untracked-synonym-bypasses-tracked-refusal", true, "--untracked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := entryFixture(t, "codex_cli", tc.tracked)
			digest := writeDirectNetworkCatalog(t, f.dir, true)
			f.deps.prober = forbiddenNetworkProber{t}
			f.deps.networkAllowlist = codexTestAllowlist()
			f.args = append([]string{f.args[0], tc.flag}, f.args[1:]...)
			f.args = withNetworkFlag(f.args, "direct-a")
			code, out, stderr := f.run()
			if code != 0 {
				t.Fatalf("exit=%d stderr=%s", code, stderr)
			}
			var child childCapture
			if err := json.Unmarshal(out, &child); err != nil {
				t.Fatalf("native launch did not exec the child directly: %v (out %q)", err, out)
			}
			wantLine := "curator-run: network: profile=direct-a origin=explicit digest=" + digest
			if !strings.Contains(string(stderr), wantLine) {
				t.Fatalf("stderr lacks provenance %q in %q", wantLine, stderr)
			}
		})
	}
}
