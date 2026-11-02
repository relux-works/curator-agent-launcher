package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
	"github.com/relux-works/curator-network-profiles/pkg/netprofile"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

// This fixture uses the real parser, confirmation ledger and root declaration;
// it deliberately supplies no qualified build. All paths are test-owned.
func explicitFalseGatewayFixture(t *testing.T) (*pipelineFixture, *e2eGateway) {
	t.Helper()
	f := printNetworkFixture(t)
	g := newE2EGateway(t)
	writeGatewayCatalog(t, f, g.socket)
	dir := filepath.Join(f.dir, ".curator")
	path := filepath.Join(dir, "network.toml")
	doc, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc = bytes.Replace(doc, []byte("sensitive_egress = true"), []byte("sensitive_egress = false"), 1)
	file, err := netprofile.Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	profile, _ := file.Lookup("egress-a")
	ledger, err := json.Marshal(map[string]any{"schema": "relux-network-confirmations-v1", "confirmed": map[string]any{"egress-a": map[string]any{"digest": netprofile.Digest(profile), "confirmed_at": "2026-11-02T00:00:00Z"}}})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"network.toml": doc, "network.confirmations.json": ledger, "network-qualified.toml": []byte("schema = \"curator-network-qualified-v1\"\nproject_roots = [\"" + f.dir + "\"]\n")} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return f, g
}

// Fresh and native-resume launches, both exec and interactive, exercise the
// real admission/emitter/child/lease path. Persisting the actual emitted JSON
// proves only the caller retention contract in this fixture, not live storage
// or vendor qualification. The helper is Go, with no vendor credentials.
func TestExplicitFalseGatewayLaunchAndRetainedProvenance(t *testing.T) {
	for _, entry := range []string{"exec", "interactive"} {
		for _, resume := range []bool{false, true} {
			name := entry + map[bool]string{false: "/fresh", true: "/resume"}[resume]
			t.Run(name, func(t *testing.T) {
				f, g := explicitFalseGatewayFixture(t)
				tail := []string{}
				if resume {
					tail = append(tail, "resume", "synthetic-session")
				}
				if entry == "exec" {
					tail = append(tail, "-p", "hello")
				}
				f.args = []string{"claude_code", "--permissions", "native", "--"}
				f.args = append(f.args, tail...)
				f.args = withNetworkFlag(f.args, "egress-a")
				code, out, stderr := f.run()
				if code != 0 || g.calls() != "/v1/health /v1/register /v1/activate /v1/unregister" {
					t.Fatalf("exit=%d calls=%q stderr=%s", code, g.calls(), stderr)
				}
				var child childCapture
				if err := json.Unmarshal(out, &child); err != nil {
					t.Fatal(err)
				}
				env := map[string]string{}
				for _, e := range child.Env {
					k, v, _ := strings.Cut(e, "=")
					env[k] = v
				}
				if env["ANTHROPIC_BASE_URL"] != "http://127.0.0.1:47811/anthropic" || env["ANTHROPIC_CUSTOM_HEADERS"] != "X-Netproxy-Token: "+e2eToken {
					t.Fatal("helper lacks gateway-only transport")
				}
				for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
					if _, ok := env[k]; ok {
						t.Fatal("ambient proxy survived")
					}
				}
				if strings.Contains(strings.Join(child.Argv, " "), e2eToken) || bytes.Contains(stderr, []byte(e2eToken)) {
					t.Fatal("fixture token leaked outside child environment")
				}
				const prefix = "curator-run: adapter-provenance: "
				var raw []byte
				count := 0
				for _, line := range bytes.Split(stderr, []byte("\n")) {
					if bytes.HasPrefix(line, []byte(prefix)) {
						raw = append([]byte(nil), line[len(prefix):]...)
						count++
					}
				}
				var p adapterprobe.Provenance
				if count != 1 || json.Unmarshal(raw, &p) != nil || p.Schema != adapterprobe.ProvenanceSchema || p.Outcome != "unqualified_build" || p.BuildID != artifactBuild(t, f) || p.Adapter.Entrypoint != entry || p.Recipe != "claude-"+entry+"-v1" {
					t.Fatalf("wrong actual emitted provenance: %s", stderr)
				}
				if !bytes.Contains(stderr, []byte(p.OperatorText()+"\n")) {
					t.Fatal("exact unqualified warning missing")
				}
				retained := filepath.Join(f.dir, "caller-provenance.json")
				if err := os.WriteFile(retained, raw, 0600); err != nil {
					t.Fatal(err)
				}
				readback, err := os.ReadFile(retained)
				if err != nil {
					t.Fatal(err)
				}
				var stored adapterprobe.Provenance
				if !bytes.Equal(readback, raw) || json.Unmarshal(readback, &stored) != nil || stored != p {
					t.Fatal("actual emitted record not retained intact")
				}
			})
		}
	}
}

// A machine profile must not acquire CLI authority when Prepare resolves its
// name. Both fresh/resume refuse before service/child effects; explicit CLI is
// the nearby positive control above, and also overrides a machine selection.
func TestExplicitFalseGatewayMachineSelectionRefuses(t *testing.T) {
	for _, resume := range []bool{false, true} {
		for _, explicit := range []bool{false, true} {
			name := map[bool]string{false: "fresh", true: "resume"}[resume] + map[bool]string{false: "/machine", true: "/explicit"}[explicit]
			t.Run(name, func(t *testing.T) {
				f, g := explicitFalseGatewayFixture(t)
				if resume {
					f.args = []string{"claude_code", "--permissions", "native", "--", "resume", "synthetic-session", "-p", "hello"}
				}
				f.deps.readSelectedConfig = func(context.Context) (launcherconfig.Selected, error) {
					return launcherconfig.Selected{ConfigSchema: 7, ConfigPath: "chosen.json", Proxy: &launcherconfig.Proxy{Profile: "egress-a"}}, nil
				}
				if explicit {
					f.args = withNetworkFlag(f.args, "egress-a")
				}
				code, out, stderr := f.run()
				if explicit {
					if code != 0 || g.calls() != "/v1/health /v1/register /v1/activate /v1/unregister" {
						t.Fatalf("explicit control refused: %d %s", code, stderr)
					}
					return
				}
				if code == 0 || len(out) != 0 || !bytes.Contains(stderr, []byte("network_scope_unsupported")) || !bytes.Contains(stderr, []byte("explicit --network")) || g.calls() != "" {
					t.Fatalf("implicit false admitted: exit=%d calls=%q stderr=%s", code, g.calls(), stderr)
				}
				f.assertNoChild(t)
			})
		}
	}
}

// Omission, catalog defaults and a prior true-profile confirmation refuse
// before any gateway or child effect. Explicit false is admitted above.
func TestExplicitFalseGatewayCatalogAndConfirmationRefusals(t *testing.T) {
	for _, mode := range []string{"omitted", "default", "stale-confirmation"} {
		t.Run(mode, func(t *testing.T) {
			f, g := explicitFalseGatewayFixture(t)
			path := filepath.Join(f.dir, ".curator", "network.toml")
			doc, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want := "network_profile_invalid"
			switch mode {
			case "omitted":
				doc = bytes.Replace(doc, []byte("sensitive_egress = false\n"), nil, 1)
			case "default":
				doc = bytes.Replace(doc, []byte("[networks.egress-a]"), []byte("default = \"egress-a\"\n[networks.egress-a]"), 1)
			case "stale-confirmation":
				trueDoc := bytes.Replace(doc, []byte("sensitive_egress = false"), []byte("sensitive_egress = true"), 1)
				file, e := netprofile.Parse(trueDoc)
				if e != nil {
					t.Fatal(e)
				}
				p, _ := file.Lookup("egress-a")
				ledger, _ := json.Marshal(map[string]any{"schema": "relux-network-confirmations-v1", "confirmed": map[string]any{"egress-a": map[string]any{"digest": netprofile.Digest(p), "confirmed_at": "2026-11-02T00:00:00Z"}}})
				writeFixture(t, filepath.Join(f.dir, ".curator", "network.confirmations.json"), ledger, 0600)
				want = "network_profile_denied"
			}
			writeFixture(t, path, doc, 0600)
			f.args = withNetworkFlag(f.args, "egress-a")
			code, out, stderr := f.run()
			if code == 0 || len(out) != 0 || !bytes.Contains(stderr, []byte(want)) || g.calls() != "" {
				t.Fatalf("exit=%d calls=%q stderr=%s", code, g.calls(), stderr)
			}
			f.assertNoChild(t)
		})
	}
}

// True gateways retain strict admission under all modes. False gateways retain
// existing strict rejection and pin matching: an exact pin may run unqualified,
// while a mismatched pin refuses before gateway registration or a child.
func TestExplicitGatewayStrictAdmissionPreserved(t *testing.T) {
	for _, sensitive := range []bool{false, true} {
		for _, policy := range []string{"optimistic", "strict", "pinned", "pin-mismatch"} {
			if !sensitive && policy == "optimistic" {
				continue
			}
			t.Run(map[bool]string{false: "false/", true: "true/"}[sensitive]+policy, func(t *testing.T) {
				f, g := explicitFalseGatewayFixture(t)
				if sensitive {
					path := filepath.Join(f.dir, ".curator", "network.toml")
					doc, _ := os.ReadFile(path)
					doc = bytes.Replace(doc, []byte("sensitive_egress = false"), []byte("sensitive_egress = true"), 1)
					file, e := netprofile.Parse(doc)
					if e != nil {
						t.Fatal(e)
					}
					p, _ := file.Lookup("egress-a")
					ledger, _ := json.Marshal(map[string]any{"schema": "relux-network-confirmations-v1", "confirmed": map[string]any{"egress-a": map[string]any{"digest": netprofile.Digest(p), "confirmed_at": "2026-11-02T00:00:00Z"}}})
					writeFixture(t, path, doc, 0600)
					writeFixture(t, filepath.Join(f.dir, ".curator", "network.confirmations.json"), ledger, 0600)
				}
				f.args = withNetworkFlag(f.args, "egress-a")
				flag := policy
				if policy == "pinned" {
					flag = "pinned:" + strings.TrimPrefix(artifactBuild(t, f), "sha256-")
				}
				if policy == "pin-mismatch" {
					flag = "pinned:" + strings.Repeat("a", 64)
				}
				insertLauncherArgs(f, "--network-policy", flag)
				code, out, stderr := f.run()
				if !sensitive && policy == "pinned" {
					if code != 0 || g.calls() != "/v1/health /v1/register /v1/activate /v1/unregister" {
						t.Fatalf("matching non-sensitive pin refused: %d %s", code, stderr)
					}
					adapterRecord(t, stderr)
					return
				}
				want := "strict_miss"
				if policy == "pin-mismatch" {
					want = "pinned_miss"
				}
				if code == 0 || len(out) != 0 || !bytes.Contains(stderr, []byte(want)) || g.calls() != "" {
					t.Fatalf("strict contract weakened: exit=%d calls=%q stderr=%s", code, g.calls(), stderr)
				}
				f.assertNoChild(t)
			})
		}
	}
}

// The unqualified route still enforces settings, physical roots, known-bad
// membership and hosted refusal. A failed provenance write cleans its lease
// before spawn. The real lifecycle test is the nearby valid control.
func TestExplicitFalseGatewayGuardsPreserved(t *testing.T) {
	for _, mode := range []string{"settings", "root", "known-bad", "hosted", "provenance", "late-overlay", "service-unavailable"} {
		t.Run(mode, func(t *testing.T) {
			f, g := explicitFalseGatewayFixture(t)
			f.args = withNetworkFlag(f.args, "egress-a")
			want := "network_scope_unsupported"
			calls := ""
			switch mode {
			case "settings":
				if err := os.MkdirAll(filepath.Join(f.dir, ".claude"), 0700); err != nil {
					t.Fatal(err)
				}
				writeFixture(t, filepath.Join(f.dir, ".claude", "settings.json"), []byte(`{"env":{"ANTHROPIC_BASE_URL":"https://elsewhere.invalid"}}`), 0600)
				want = "network_configuration_conflict"
			case "root":
				other := t.TempDir()
				writeFixture(t, filepath.Join(f.dir, ".curator", "network-qualified.toml"), []byte("schema = \"curator-network-qualified-v1\"\nproject_roots = [\""+other+"\"]\n"), 0600)
			case "known-bad":
				data, err := json.Marshal(map[string]any{"schema": adapterprobe.KnownBadSchema, "sha256": []string{strings.TrimPrefix(artifactBuild(t, f), "sha256-")}})
				if err != nil {
					t.Fatal(err)
				}
				writeFixture(t, filepath.Join(f.dir, ".curator", adapterprobe.KnownBadFileName), data, 0600)
				want = "known_bad_build"
			case "service-unavailable":
				if err := os.Remove(g.socket); err != nil {
					t.Fatal(err)
				}
				want = "network_proxy_unreachable"
			case "late-overlay":
				build := f.deps.build
				f.deps.build = func(ctx context.Context, r *vendorplugin.Registry, req vendorplugin.SpawnRequest, mode agentic.LaunchMode) (agentic.PlanWithEnvironment, error) {
					built, err := build(ctx, r, req, mode)
					if err == nil {
						built.OwnedEnv = append(built.OwnedEnv, "IS_DEMO=0")
					}
					return built, err
				}
				want = "network_configuration_conflict"
				calls = "/v1/health /v1/register /v1/unregister"
			case "hosted":
				insertLauncherArgs(f, "--hosted")
			case "provenance":
				f.deps.networkEmit = func(io.Writer, *adapterprobe.Provenance) error { return errors.New("fixture emission refusal") }
				want = "provenance emission failed"
				calls = "/v1/health /v1/register /v1/unregister"
			}
			code, out, stderr := f.run()
			if code == 0 || len(out) != 0 || !bytes.Contains(stderr, []byte(want)) || g.calls() != calls {
				t.Fatalf("guard %s: exit=%d calls=%q stderr=%s", mode, code, g.calls(), stderr)
			}
			f.assertNoChild(t)
		})
	}
}
