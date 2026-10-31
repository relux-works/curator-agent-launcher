package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

// An altered provider-owned snapshot can contain a selector absent from the
// plan environment inspected during preparation. Final composition refuses it
// before provenance/spawn and cleans registration. The positive MCP control
// supplies a consistent plan plus owned snapshot, as the provider contract does.
func TestGatewayFinalOwnedEnvironmentThroughRun(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		admit       bool
	}{
		{"IS_DEMO", "0", false}, {"cLaUbBiT", "true", false},
		{"MCP_SDK_GENERATION", "v2", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := printNetworkFixture(t)
			g := newE2EGateway(t)
			writeGatewayCatalog(t, f, g.socket)
			f.args = withNetworkFlag(f.args, "egress-a")
			build := f.deps.build
			f.deps.build = func(ctx context.Context, r *vendorplugin.Registry, req vendorplugin.SpawnRequest, mode agentic.LaunchMode) (agentic.PlanWithEnvironment, error) {
				built, err := build(ctx, r, req, mode)
				if err == nil {
					built.OwnedEnv = append(built.OwnedEnv, tc.name+"="+tc.value)
					if tc.admit {
						built.Plan.Env = append(built.Plan.Env, tc.name+"="+tc.value)
					}
				}
				return built, err
			}
			code, out, stderr := f.run()
			if tc.admit {
				var child childCapture
				if code != 0 || json.Unmarshal(out, &child) != nil {
					t.Fatalf("valid literal failed: code=%d stderr=%s", code, stderr)
				}
				found := false
				for _, pair := range child.Env {
					found = found || pair == tc.name+"="+tc.value
				}
				if !found || g.calls() != "/v1/health /v1/register /v1/activate /v1/unregister" {
					t.Fatal("valid literal or lifecycle lost", g.calls())
				}
				return
			}
			f.assertNoChild(t)
			if code == 0 || !strings.Contains(string(stderr), "network_configuration_conflict") ||
				strings.Contains(string(stderr), "curator-run: network:") ||
				g.calls() != "/v1/health /v1/register /v1/unregister" {
				t.Fatalf("code=%d calls=%s stderr=%s", code, g.calls(), stderr)
			}
		})
	}
}

// The inert subject reproduces the two relevant native predicates, not an
// entire Claude runtime: IS_DEMO is a nonempty string (including "0"), while
// CLAUBBIT uses the trimmed native bool vocabulary. Direct activation is the
// known-bad witness; the same inherited values cannot activate the subject
// through the built launcher. MCP v1/v2 and unknown enum strings survive
// byte-for-byte; this tests transport preservation, not native warning output.
func TestGatewayInheritedModePredicatesThroughRun(t *testing.T) {
	bin := frontBuild(t)
	for index, tc := range []struct {
		name, value, marker string
		present, active     bool
	}{
		{"IS_DEMO", "", "demo-hook-predicate", false, false},
		{"IS_DEMO", "", "demo-hook-predicate", true, false},
		{"IS_DEMO", "0", "demo-hook-predicate", true, true},
		{"IS_DEMO", "false", "demo-hook-predicate", true, true},
		{"IS_DEMO", "1", "demo-hook-predicate", true, true},
		{"CLAUBBIT", "", "claubbit-command-predicate", false, false},
		{"CLAUBBIT", "", "claubbit-command-predicate", true, false},
		{"CLAUBBIT", "0", "claubbit-command-predicate", true, false},
		{"CLAUBBIT", "false", "claubbit-command-predicate", true, false},
		{"CLAUBBIT", "1", "claubbit-command-predicate", true, true},
		{"CLAUBBIT", "true", "claubbit-command-predicate", true, true},
		{"CLAUBBIT", " On ", "claubbit-command-predicate", true, true},
	} {
		t.Run(tc.name+"/"+tc.value+"/"+map[bool]string{true: "present", false: "absent"}[tc.present], func(t *testing.T) {
			f := newFront(t, bin)
			generation := []string{"v1", "v2", "unsupported-fixture"}[index%3]
			negotiation := []string{"legacy", "auto", "unsupported-fixture"}[index%3]
			f.env = append(f.env, "MCP_SDK_GENERATION="+generation, "MCP_PROTOCOL_NEGOTIATION="+negotiation, "UNRELATED_EXTENSION_SETTING=exact-value")
			if tc.present {
				f.env = append(f.env, tc.name+"="+tc.value)
			}
			code, _, stderr := f.command(filepath.Join(f.root, "bin", "claude"), []string{"-p", "fixture"}, nil, false, false, 30*time.Second)
			_, markerErr := os.Stat(filepath.Join(f.root, tc.marker))
			if code != 0 || (markerErr == nil) != tc.active || (markerErr != nil && !os.IsNotExist(markerErr)) {
				t.Fatalf("direct predicate witness code=%d active=%v want=%v stderr=%s", code, markerErr == nil, tc.active, stderr)
			}
			for _, name := range []string{tc.marker, "native-called", "native.json"} {
				if err := os.Remove(filepath.Join(f.root, name)); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
			}
			g := newE2EGateway(t)
			writeFrontGateway(t, f.root, g.socket)
			args := []string{"claude_code", "--native", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--network", "egress-a", "--", "-p", "fixture"}
			code, _, stderr = f.command(bin.launcher, args, nil, false, false, 30*time.Second)
			if code != 0 || g.calls() != "/v1/health /v1/register /v1/activate /v1/unregister" {
				t.Fatalf("ordinary launch code=%d calls=%s stderr=%s", code, g.calls(), stderr)
			}
			if _, err := os.Stat(filepath.Join(f.root, tc.marker)); !os.IsNotExist(err) {
				t.Fatal("inherited selector reached the inert native predicate", err)
			}
			b, err := os.ReadFile(filepath.Join(f.root, "native.json"))
			if err != nil {
				t.Fatal(err)
			}
			var child childCapture
			if err := json.Unmarshal(b, &child); err != nil {
				t.Fatal(err)
			}
			joined := "\n" + strings.Join(child.Env, "\n") + "\n"
			if strings.Contains(joined, "\n"+tc.name+"=") {
				t.Fatal("selector survived in the final child environment")
			}
			for _, kept := range []string{"MCP_SDK_GENERATION=" + generation, "MCP_PROTOCOL_NEGOTIATION=" + negotiation, "UNRELATED_EXTENSION_SETTING=exact-value"} {
				if !strings.Contains(joined, "\n"+kept+"\n") {
					t.Fatal("ordinary environment value lost", kept)
				}
			}
		})
	}
}
