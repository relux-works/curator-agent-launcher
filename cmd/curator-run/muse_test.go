package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/mapping"
	"github.com/relux-works/curator-agent-launcher/internal/plan"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/providerlimits"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

func museFixture(t *testing.T) (*pipelineFixture, *fragment.Fragment) {
	t.Helper()
	f := entryFixture(t, "pi", false)
	data, err := os.ReadFile("../../internal/fragment/testdata/v3/muse.json")
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.ReplaceAll(string(data), "/manager/environments/default/muse", f.home))
	// Do not pre-validate with the reader under test: run must encounter the
	// canned bytes through the real resolver, including a rejected-v3 mutant.
	var wire struct{ Env map[string]string }
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	frag := &fragment.Fragment{Revision: fragment.IdentityV3, Environment: fragment.EnvMuse, Env: wire.Env}
	fragmentPath := filepath.Join(f.dir, "muse-fragment.json")
	writeFixture(t, fragmentPath, data, 0600)
	curator := filepath.Join(f.dir, "curator")
	helper, err := os.ReadFile(pipelineHelper)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, curator, helper, 0700)
	f.binary = filepath.Join(f.dir, "muse")
	writeFixture(t, f.binary, helper, 0700)
	t.Setenv("CURATOR_TEST_FRAGMENT", fragmentPath)
	t.Setenv("CURATOR_TEST_RESOLVE_ARGV", filepath.Join(f.dir, "resolve-argv"))
	f.deps.resolver = fragment.NewWithRunner(curator, fragment.ExecRunner{})
	f.deps.environ = func() []string {
		return []string{"PATH=" + f.dir, "HOME=" + f.dir, "CURATOR_TEST_RELEASE=1.4.1", "XDG_CONFIG_HOME=/foreign/config"}
	}
	f.deps.isTerminal = func() bool { return false }
	f.args = []string{"muse", "--permissions=native"}
	return f, frag
}

// Production call sites: run -> fragment.Resolver.Resolve -> resolvePermission.
// Today's release has no Muse permission mapping, even for native mode.
func TestMuseV3ThroughRunPermissionBound(t *testing.T) {
	f, frag := museFixture(t)
	code, out, stderr := f.run()
	argv, err := os.ReadFile(filepath.Join(f.dir, "resolve-argv"))
	if err != nil {
		t.Fatal(err)
	}
	if string(argv) != "env\nresolve\nmuse\n--repair\n--format\njson" {
		t.Fatalf("resolve argv=%q", argv)
	}
	_, mappingErr := agentic.Default.PermissionMapping("muse", "1.4.1", agentic.PermissionModeNative)
	if mappingErr != nil {
		const refusal = "curator-run: permission_mode_unsupported: agentic: system maps no permission-mode bypass flag: muse has no release-pinned permission mapping\n"
		if code != 1 || !strings.HasSuffix(string(stderr), refusal) || len(out) != 0 || f.builds != 0 || f.verdicts != 0 {
			t.Fatalf("permission bound changed: exit=%d stderr=%s", code, stderr)
		}
		if !strings.Contains(string(stderr), "model=muse-spark-1.3-contributor (lineup) effort=max (lineup)") {
			t.Fatalf("Muse declaration rows not resolved: %s", stderr)
		}
		f.assertNoChild(t)
		return
	}
	if f.builds != 1 || f.request.Runtime != "muse" || f.request.Home != frag.Home() || f.request.PermissionMode != agentic.PermissionModeNative || !reflect.DeepEqual(f.request.Env, f.deps.environ()) {
		t.Fatalf("wrong Muse request: %+v stderr=%s", f.request, stderr)
	}
	system, ok := agentic.Default.Lookup("muse")
	if !ok {
		t.Fatal("Muse plugin not registered")
	}
	if !system.Capabilities().SupportsMode(agentic.LaunchModeInteractive) {
		const refusal = "plan_refused: spawn plane refused the launch: agentic: system does not support launch mode: muse does not declare interactive"
		if code != 1 || !strings.Contains(string(stderr), refusal+"\n") || len(out) != 0 || f.verdicts != 0 {
			t.Fatalf("exact interactive bound changed: exit=%d verdicts=%d stdout=%s stderr=%s", code, f.verdicts, out, stderr)
		}
		f.assertNoChild(t)
		return
	}
	if code != 0 || f.verdicts != 1 {
		t.Fatalf("interactive-capable pin refused: exit=%d stderr=%s", code, stderr)
	}
	var child childCapture
	if err := json.Unmarshal(out, &child); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(child.Argv, f.plan.Argv) {
		t.Fatalf("argv=%q plan=%q", child.Argv, f.plan.Argv)
	}
	assertMuseEnv(t, child.Env, f.dir, frag)
}

// Production call site: plan.Build with the real tagged BuildLaunch API.
// Separate from run's earlier permission bound. This row flips to admission
// when a pinned plugin declares interactive; no mode substitution is allowed.
func TestMuseV3PlanBuildInteractiveBound(t *testing.T) {
	f, frag := museFixture(t)
	deps := plan.DefaultDeps(nil)
	deps.Registry = f.deps.registry
	reads := 0
	deps.Availability = func(providerlimits.VerdictQuery) (vendorplugin.Availability, error) {
		reads++
		return vendorplugin.Availability{State: vendorplugin.AvailabilityHealthy}, nil
	}
	req := plan.Request{Runtime: "muse", Model: "muse-spark-1.3-contributor", Effort: "max", PermissionMode: agentic.PermissionModeNative, Home: frag.Home(), WorkDir: f.dir, Env: f.deps.environ()}
	built, err := plan.Build(context.Background(), deps, req)
	system, _ := agentic.Default.Lookup("muse")
	if !system.Capabilities().SupportsMode(agentic.LaunchModeInteractive) {
		const refusal = "plan_refused: spawn plane refused the launch: agentic: system does not support launch mode: muse does not declare interactive"
		if err == nil || err.Error() != refusal || reads != 0 {
			t.Fatalf("interactive refusal=%v reads=%d", err, reads)
		}
		f.assertNoChild(t)
		return
	}
	if err != nil || reads != 1 || built.Plan.Binary != f.binary {
		t.Fatalf("interactive-capable pin: plan=%+v err=%v reads=%d", built.Plan, err, reads)
	}
	if !reflect.DeepEqual(plan.SpawnRequest(req).Env, f.deps.environ()) {
		t.Fatal("plan request changed HOME/env")
	}
}

func assertMuseEnv(t *testing.T, env []string, nativeHome string, frag *fragment.Fragment) {
	t.Helper()
	values := map[string]string{}
	for _, entry := range env {
		k, v, _ := strings.Cut(entry, "=")
		values[k] = v
	}
	if values["HOME"] != nativeHome {
		t.Fatalf("HOME replaced: %q", values["HOME"])
	}
	for k, v := range frag.Env {
		if values[k] != v {
			t.Fatalf("%s=%q want %q", k, values[k], v)
		}
	}
}

// Composition is independently reachable for admitted plans of other systems.
// Test the v3 overlay here without claiming today's Muse plugin admits a plan.
func TestMuseV3CompositionPreservesHOME(t *testing.T) {
	f, frag := museFixture(t)
	p := agentic.Plan{Binary: f.binary, WorkDir: f.dir, Argv: []string{"opaque"}, Env: f.deps.environ()}
	v, err := composition.Compose(p, nil, *frag, composition.PromptApplication{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertMuseEnv(t, v.Env, f.dir, frag)
	if _, ok := v.EnvLiterals["HOME"]; ok {
		t.Fatal("HOME serialized as an override")
	}
	if !reflect.DeepEqual(v.EnvLiterals, frag.Env) || !reflect.DeepEqual(v.Argv, p.Argv) {
		t.Fatalf("v3 composition=%+v", v)
	}
	target, err := mapping.Resolve(frag.Environment)
	if err != nil || target.System != "muse" || target.Provider != "muse" {
		t.Fatalf("Muse mapping=%+v err=%v", target, err)
	}
}

func TestMuseV3PermissionTransportThroughRun(t *testing.T) {
	f, _ := museFixture(t)
	f.args = []string{"muse", "--yolo"}
	code, _, stderr := f.run()
	// v3 carries the policy to the real module. Its current interactive Muse
	// permission capability is unsupported; do not build exec --yolo here.
	if code != 1 || !strings.Contains(string(stderr), "permission_mode_unsupported:") || strings.Contains(string(stderr), "permission_policy_unsupported:") || f.builds != 0 {
		t.Fatalf("v3 permission transport: exit=%d builds=%d stderr=%s", code, f.builds, stderr)
	}
	f.assertNoChild(t)
}
