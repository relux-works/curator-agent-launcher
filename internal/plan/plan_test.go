package plan_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/mapping"
	"github.com/relux-works/curator-agent-launcher/internal/plan"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/providerlimits"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

// capture records one BuildLaunch call.
type capture struct {
	count int
	req   vendorplugin.SpawnRequest
	mode  agentic.LaunchMode
	reg   *vendorplugin.Registry
}

type availStub struct {
	count   int
	query   providerlimits.VerdictQuery
	verdict vendorplugin.Availability
	err     error
}

func healthyVerdict() vendorplugin.Availability {
	return vendorplugin.Availability{
		State:    vendorplugin.AvailabilityHealthy,
		Checked:  []string{"test-source"},
		Observed: []vendorplugin.Observation{{Source: "test-source", Detail: "test read"}},
	}
}

// realBuild delegates to the real tagged vendorplugin.BuildLaunch after
// recording the exact request. Production call site: plan.Build.
func (c *capture) realBuild(ctx context.Context, r *vendorplugin.Registry, req vendorplugin.SpawnRequest, mode agentic.LaunchMode) (agentic.Plan, error) {
	c.count++
	c.req = req
	c.mode = mode
	c.reg = r
	return vendorplugin.BuildLaunch(ctx, r, req, mode)
}

func (a *availStub) read(q providerlimits.VerdictQuery) (vendorplugin.Availability, error) {
	a.count++
	a.query = q
	return a.verdict, a.err
}

func testHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "managed-home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

// TestSpawnRequestShape pins the exact §4.4 request: resolved triple,
// managed home, workdir, inherited env verbatim, empty composition, zero
// run, everything else unset. Production call site: plan.SpawnRequest via
// plan.Build.
func TestSpawnRequestShape(t *testing.T) {
	req, store, _ := fixture(t, "codex")
	env := req.Env
	c := &capture{}
	a := &availStub{verdict: healthyVerdict()}
	d := plan.DefaultDeps(store)
	d.BuildLaunch = c.realBuild
	d.Availability = a.read
	if _, err := plan.Build(context.Background(), d, req); err != nil {
		t.Fatal(err)
	}
	got := c.req
	if string(got.Runtime) != "codex" || string(got.Model) != req.Model || got.Effort != req.Effort {
		t.Fatalf("resolved triple = %q/%q/%q, want the resolved input triple", got.Runtime, got.Model, got.Effort)
	}
	if got.Home != req.Home || got.WorkDir != req.WorkDir {
		t.Fatalf("home/workdir = %q/%q, want managed home and current workdir", got.Home, got.WorkDir)
	}
	if !reflect.DeepEqual(got.Env, env) {
		t.Fatalf("env = %#v, want verbatim inherited environment", got.Env)
	}
	if !got.Composition.IsZero() {
		t.Fatalf("composition = %#v, want zero (MCP belongs to the composer, §4.5)", got.Composition)
	}
	if !got.Run.IsZero() {
		t.Fatalf("run = %#v, want zero (terminal launch supplies no run context)", got.Run)
	}
	if got.Goal != nil || got.Budget != nil || got.ServiceTier != "" || got.PromptPath != "" || len(got.Prompt) != 0 {
		t.Fatalf("goal/budget/tier/assignment must stay unset: %#v", got)
	}
	if !reflect.ValueOf(got.Engine).IsZero() || got.Profile != "" {
		t.Fatalf("profile = %q, want empty (no harness profile selected)", got.Profile)
	}
}

// All behavioral tests enter plan.Build; DefaultDeps binds the tagged APIs.
func fixture(t *testing.T, runtime string) (plan.Request, *providerlimits.Store, providerlimits.Layout) {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"claude", "codex", "pi"} {
		// Any accidental process creation leaves evidence; real providers never run.
		script := fmt.Sprintf("#!/bin/sh\necho started > '%s'\nexit 91\n", filepath.Join(root, "started"))
		if err := os.WriteFile(filepath.Join(root, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	// Resolved model/effort per runtime, matching the tagged vendor rows:
	// claude/codex carry effort, pi-google rows are effort-none, and every
	// Pi word is one the installed Pi runs as requested (no clamp/drop).
	model, effort := "gpt-6-astra", "medium"
	switch runtime {
	case "claude":
		model, effort = "claude-opus-5", "medium"
	case "pi-anthropic":
		model, effort = "claude-opus-5", "high"
	case "pi-openai":
		model, effort = "gpt-5.6-sol", "max"
	case "pi-google":
		model, effort = "gemini-3.1-pro-preview", ""
	}
	req := plan.Request{Runtime: runtime, Model: model, Effort: effort, Home: testHome(t), WorkDir: root,
		Env: []string{"PATH=" + root, "HOME=" + root, "PLAN_INHERITED=kept", "CLAUDECODE=nested", "CODEX_HOME=/foreign"}}
	layout := providerlimits.LayoutAt(root)
	store, err := providerlimits.NewStore(providerlimits.Options{Layout: layout, Now: func() time.Time { return time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	return req, store, layout
}

func TestTaggedInteractivePlans(t *testing.T) {
	for _, tc := range []struct {
		env, runtime, system, binary string
		argv                         []string
	}{
		{"claude_code", "claude", "claude-code", "claude", []string{"--model", "claude-opus-5", "--effort", "medium"}},
		{"codex_cli", "codex", "codex", "codex", []string{"-m", "gpt-6-astra", "-c", `model_reasoning_effort="medium"`}},
		{"pi", "pi-anthropic", "pi-native", "pi", []string{"--model", "anthropic/claude-opus-5", "--thinking", "high"}},
	} {
		t.Run(tc.env, func(t *testing.T) {
			req, store, _ := fixture(t, tc.runtime)
			target, err := mapping.Resolve(tc.env)
			if err != nil || target.System != tc.system {
				t.Fatalf("mapping: %+v %v", target, err)
			}
			c := &capture{}
			count := 0
			d := plan.DefaultDeps(store)
			d.BuildLaunch = c.realBuild
			d.Availability = func(q providerlimits.VerdictQuery) (vendorplugin.Availability, error) {
				count++
				if q.Runtime != req.Runtime || q.Model != req.Model || q.Home != req.Home {
					t.Fatalf("query changed: %+v", q)
				}
				return store.AvailabilityFor(q)
			}
			got, err := plan.Build(context.Background(), d, req)
			if err != nil {
				t.Fatal(err)
			}
			if c.count != 1 || count != 1 || c.mode != agentic.LaunchModeInteractive || !reflect.DeepEqual(c.req.Env, req.Env) {
				t.Fatalf("calls/mode/env: %+v %d", c, count)
			}
			if got.System != agentic.SystemID(tc.system) || got.Mode != agentic.LaunchModeInteractive || got.Home != req.Home || got.WorkDir != req.WorkDir || got.Binary != filepath.Join(req.WorkDir, tc.binary) || got.Stdin.Attached {
				t.Fatalf("plan shape: %+v", got)
			}
			if !reflect.DeepEqual(got.Argv, tc.argv) {
				t.Fatalf("argv golden: %q != %q", got.Argv, tc.argv)
			}
			if !strings.Contains(strings.Join(got.Env, "\n"), "PLAN_INHERITED=kept") {
				t.Fatalf("inherited env lost: %q", got.Env)
			}
			if _, err := os.Stat(filepath.Join(req.WorkDir, "started")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("plan started child or observation failed: %v", err)
			}
		})
	}
}

func TestTaggedAdmissionRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, runtime, model, effort string
		want                         error
	}{
		{"missing_effort", "claude", "claude-opus-5", "", vendorplugin.ErrEffortMissing},
		{"invalid_effort", "codex", "gpt-6-astra", "bogus", vendorplugin.ErrEffortNotInVocabulary},
		{"unknown_model", "codex", "no-such-model", "medium", vendorplugin.ErrUnknownModel},
		{"empty_model", "codex", "", "medium", nil},
		{"unknown_runtime", "no-such-runtime", "gpt-6-astra", "medium", nil},
		{"empty_runtime", "", "gpt-6-astra", "medium", nil},
		{"nil_registry", "codex", "gpt-6-astra", "medium", nil},
		{"cancelled", "codex", "gpt-6-astra", "medium", context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, store, _ := fixture(t, "codex")
			req.Runtime, req.Model, req.Effort = tc.runtime, tc.model, tc.effort
			c := &capture{}
			a := &availStub{verdict: healthyVerdict()}
			d := plan.DefaultDeps(store)
			d.BuildLaunch = c.realBuild
			d.Availability = a.read
			if tc.name == "nil_registry" {
				d.Registry = nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.name == "cancelled" {
				cancel()
			}
			got, err := plan.Build(ctx, d, req)
			var refused *plan.RefusedError
			if !errors.As(err, &refused) || (tc.want != nil && !errors.Is(err, tc.want)) || !reflect.DeepEqual(got, agentic.Plan{}) || c.count != 1 || a.count != 0 {
				t.Fatalf("refusal: plan=%+v err=%v calls=%d/%d", got, err, c.count, a.count)
			}
			if refused.Cause == nil || !strings.Contains(err.Error(), refused.Cause.Error()) {
				t.Fatalf("module evidence lost: %v", err)
			}
			if tc.name == "missing_effort" {
				for _, word := range []string{"--effort", "claude-opus-5", "medium"} {
					if !strings.Contains(err.Error(), word) {
						t.Fatalf("guidance lost %q: %v", word, err)
					}
				}
			}
		})
	}
}

func TestRequiredInputs(t *testing.T) {
	for _, name := range []string{"home_empty", "home_whitespace", "workdir_empty", "workdir_whitespace", "build_nil", "availability_nil", "store_nil"} {
		t.Run(name, func(t *testing.T) {
			req, store, _ := fixture(t, "codex")
			c := &capture{}
			a := &availStub{verdict: healthyVerdict()}
			d := plan.DefaultDeps(store)
			d.BuildLaunch = c.realBuild
			d.Availability = a.read
			switch name {
			case "home_empty":
				req.Home = ""
			case "home_whitespace":
				req.Home = " \t"
			case "workdir_empty":
				req.WorkDir = ""
			case "workdir_whitespace":
				req.WorkDir = " \t"
			case "build_nil":
				d.BuildLaunch = nil
			case "availability_nil":
				d.Availability = nil
			case "store_nil":
				d = plan.DefaultDeps(nil)
			}
			got, err := plan.Build(context.Background(), d, req)
			var refused *plan.RefusedError
			if !errors.As(err, &refused) || !reflect.DeepEqual(got, agentic.Plan{}) || c.count != 0 || a.count != 0 {
				t.Fatalf("required input admitted: %+v %v calls=%d/%d", got, err, c.count, a.count)
			}
		})
	}
}

func TestProviderVerdicts(t *testing.T) {
	for _, state := range []vendorplugin.AvailabilityState{vendorplugin.AvailabilityUnknown, vendorplugin.AvailabilityLimited, vendorplugin.AvailabilityUnreachable, vendorplugin.AvailabilityState(99)} {
		t.Run(state.String(), func(t *testing.T) {
			req, store, _ := fixture(t, "codex")
			d := plan.DefaultDeps(store)
			c := &capture{}
			d.BuildLaunch = c.realBuild
			at := time.Date(2026, 9, 9, 10, 0, 0, 123, time.UTC)
			verdict := vendorplugin.Availability{State: state, Checked: []string{"source-a", "source-b"}, Observed: []vendorplugin.Observation{{Source: "observed-source", Detail: "quota evidence", At: at}}, Failures: []vendorplugin.ReadFailure{{Source: "failed-source", Reason: "read denied"}}}
			if state == vendorplugin.AvailabilityLimited {
				verdict.Until = at.Add(time.Hour)
			}
			a := &availStub{verdict: verdict}
			d.Availability = a.read
			got, err := plan.Build(context.Background(), d, req)
			var limited *plan.LimitedError
			if !errors.As(err, &limited) || !reflect.DeepEqual(limited.Verdict, verdict) || !reflect.DeepEqual(got, agentic.Plan{}) || c.count != 1 || a.count != 1 {
				t.Fatalf("verdict admitted/lost: %+v %v calls=%d/%d", got, err, c.count, a.count)
			}
			for _, word := range []string{state.String(), "source-a", "source-b", "observed-source", "quota evidence", at.Format(time.RFC3339Nano), "failed-source", "read denied"} {
				if !strings.Contains(err.Error(), word) {
					t.Fatalf("evidence lost %q: %v", word, err)
				}
			}
			if !verdict.Until.IsZero() && !strings.Contains(err.Error(), verdict.Until.Format(time.RFC3339Nano)) {
				t.Fatalf("until lost: %v", err)
			}
		})
	}
}

func TestProviderReadError(t *testing.T) {
	req, store, _ := fixture(t, "codex")
	d := plan.DefaultDeps(store)
	c := &capture{}
	d.BuildLaunch = c.realBuild
	sentinel := errors.New("module read failure")
	a := &availStub{verdict: healthyVerdict(), err: sentinel}
	d.Availability = a.read
	got, err := plan.Build(context.Background(), d, req)
	var refused *plan.RefusedError
	if !errors.As(err, &refused) || !errors.Is(err, sentinel) || !strings.Contains(err.Error(), sentinel.Error()) || !reflect.DeepEqual(got, agentic.Plan{}) || c.count != 1 || a.count != 1 {
		t.Fatalf("read failure admitted/retried: %+v %v calls=%d/%d", got, err, c.count, a.count)
	}
}

// Real Store reads fixture files only. Profile B and another model group
// cannot borrow A's suppression. Corruption is unknown, never absence.
func TestRealStoreIsolation(t *testing.T) {
	req, store, layout := fixture(t, "claude")
	id, err := providerlimits.IdentityFor(req.Runtime, req.Home)
	if err != nil {
		t.Fatal(err)
	}
	d := plan.DefaultDeps(store)
	if _, err = plan.Build(context.Background(), d, req); err != nil {
		t.Fatalf("determinate absent: %v", err)
	}
	if err = os.MkdirAll(layout.Root, 0700); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 9, 11, 59, 0, 0, time.UTC)
	until := at.Add(5 * time.Minute)
	record := providerlimits.GroupRecord{Provider: "claude", State: providerlimits.StateSuppressed, BackoffStep: 1, SuppressedSince: &at, NextProbeAt: &until, LastObservationAt: &at, ConsecutiveLimitObservations: 1}
	// Public module schema fixture, not a replacement state interpreter.
	raw, err := json.Marshal(map[string]any{"version": providerlimits.SchemaVersion, "identity": id.Key, "provider": id.Provider, "home_display": id.HomeDisplay, "groups": map[string]any{providerlimits.MustGroupTable().Group(req.Runtime, req.Model): record}})
	if err != nil {
		t.Fatal(err)
	}
	path := layout.StateFile(id.Key)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	_, err = plan.Build(context.Background(), d, req)
	var limited *plan.LimitedError
	if !errors.As(err, &limited) || limited.Verdict.State != vendorplugin.AvailabilityLimited || !limited.Verdict.Until.Equal(until) {
		t.Fatalf("suppression not enforced: %v", err)
	}
	profileB := req
	profileB.Home = testHome(t)
	if _, err = plan.Build(context.Background(), d, profileB); err != nil {
		t.Fatalf("profile A gated B: %v", err)
	}
	otherModel := req
	otherModel.Model = "claude-fable-5"
	otherModel.Effort = "medium"
	if _, err = plan.Build(context.Background(), d, otherModel); err != nil {
		t.Fatalf("plan group gated usage credits: %v", err)
	}
	if err = os.WriteFile(path, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = plan.Build(context.Background(), d, req)
	if !errors.As(err, &limited) || limited.Verdict.State != vendorplugin.AvailabilityUnknown || len(limited.Verdict.Failures) == 0 {
		t.Fatalf("corrupt state treated as absent: %v", err)
	}
}

// TestModelNotDrivenBySystem is the R1 rework regression for SPEC §4.4/§6
// row 15: the real v0.5.11 module refuses pi-openai / gpt-6-astra / high
// because gpt-6-astra declares only the codex harness
// (vendors/openai/models.go: Systems ["codex"]) while pi-openai runs on
// pi-native. The wrapper must propagate that refusal as RefusedError with
// the sentinel and detail preserved, a zero plan, exactly one BuildLaunch
// call in Interactive mode and zero limits reads. Production call site:
// plan.Build with the real tagged vendorplugin.BuildLaunch.
func TestModelNotDrivenBySystem(t *testing.T) {
	req, store, _ := fixture(t, "pi-openai")
	req.Model = "gpt-6-astra"
	req.Effort = "high"
	c := &capture{}
	a := &availStub{verdict: healthyVerdict()}
	d := plan.DefaultDeps(store)
	d.BuildLaunch = c.realBuild
	d.Availability = a.read
	got, err := plan.Build(context.Background(), d, req)
	var refused *plan.RefusedError
	if !errors.As(err, &refused) || !errors.Is(err, vendorplugin.ErrModelNotDrivenBySystem) {
		t.Fatalf("mismatch admitted or sentinel lost: plan=%+v err=%v", got, err)
	}
	if !reflect.DeepEqual(got, agentic.Plan{}) {
		t.Fatalf("refusal returned a plan: %+v", got)
	}
	if c.count != 1 || a.count != 0 {
		t.Fatalf("call counts = %d/%d, want exactly one launch call and zero limits calls", c.count, a.count)
	}
	if c.mode != agentic.LaunchModeInteractive {
		t.Fatalf("mode = %v, want named Interactive", c.mode)
	}
	if refused.Cause == nil || !strings.Contains(err.Error(), refused.Cause.Error()) {
		t.Fatalf("module evidence lost: %v", err)
	}
	for _, word := range []string{"pi-openai", "pi-native", "gpt-6-astra"} {
		if !strings.Contains(err.Error(), word) {
			t.Fatalf("refusal detail lost %q: %v", word, err)
		}
	}
}

// TestUnresolvedVendorScope drives the sole VendorUnresolved frozen runtime
// through the production entry point. Frozen runtime "muse" records
// VendorUnresolved (vendorplugin/runtime.go frozenRuntimes) on agentic
// system "muse", which has no plugin registered in this binary (plan.go
// imports only claude/codex/pinative systems). Registry.ResolveRuntime
// checks system registration before vendor resolution (registry.go), so the
// real tagged BuildLaunch refuses with ErrRuntimeSystemUnregistered —
// through the same plan.Build propagation gate that owns
// ErrRuntimeVendorUnresolved. ErrRuntimeVendorUnresolved itself is
// unreachable here: the system check fires first, and had the system been
// present the system-only binding (spawn.go resolveLaunchBinding) would
// absorb the unresolved vendor instead of returning it. Production call
// site: plan.Build with the real tagged vendorplugin.BuildLaunch.
func TestUnresolvedVendorScope(t *testing.T) {
	req, store, _ := fixture(t, "codex")
	req.Runtime, req.Model, req.Effort = "muse", "anything", "medium"
	c := &capture{}
	a := &availStub{verdict: healthyVerdict()}
	d := plan.DefaultDeps(store)
	d.BuildLaunch = c.realBuild
	d.Availability = a.read
	got, err := plan.Build(context.Background(), d, req)
	var refused *plan.RefusedError
	if !errors.As(err, &refused) || !errors.Is(err, vendorplugin.ErrRuntimeSystemUnregistered) {
		t.Fatalf("unresolved-vendor scope admitted or evidence lost: plan=%+v err=%v", got, err)
	}
	if !reflect.DeepEqual(got, agentic.Plan{}) || c.count != 1 || a.count != 0 {
		t.Fatalf("refusal shape: plan=%+v err=%v calls=%d/%d", got, err, c.count, a.count)
	}
}

// TestInteractiveDeclaredForMappedSystems pins the unreachable bound for
// agentic.ErrUnsupportedLaunchMode in the fixed Interactive/mapped-system
// scope. plan.Build spells agentic.LaunchModeInteractive by name
// (plan.go) and never takes another mode; every §4.2 mapped system in this
// binary — claude-code, codex, pi-native, registered by plan.go's blank
// imports — declares Interactive (claude.go, codex.go, pinative.go
// Capabilities). A refusal for an undeclared mode therefore cannot arise
// for a mapped runtime; any such refusal from an unmapped runtime would
// travel the same tested `if err != nil` propagation gate proven by
// TestModelNotDrivenBySystem and TestTaggedAdmissionRefusals.
func TestInteractiveDeclaredForMappedSystems(t *testing.T) {
	for _, id := range []agentic.SystemID{"claude-code", "codex", "pi-native"} {
		sys, ok := agentic.Default.Lookup(id)
		if !ok {
			t.Fatalf("mapped system %q not registered", id)
		}
		if !sys.Capabilities().SupportsMode(agentic.LaunchModeInteractive) {
			t.Fatalf("mapped system %q does not declare Interactive: %v", id, sys.Capabilities().LaunchModes)
		}
	}
}

// TestNativePiRuntimes drives every native-Pi runtime through the
// production entry point at the real v0.5.11 tag: the pinative system
// plugin serves provider-qualified argv on the managed home, and the
// separate limits read keys the exact Pi runtime/model/home. Production
// call site: plan.Build with plan.DefaultDeps (real tagged BuildLaunch
// and a real store whose determinate absent read admits).
func TestNativePiRuntimes(t *testing.T) {
	for _, tc := range []struct {
		runtime string
		argv    []string
	}{
		{"pi-anthropic", []string{"--model", "anthropic/claude-opus-5", "--thinking", "high"}},
		{"pi-openai", []string{"--model", "openai/gpt-5.6-sol", "--thinking", "max"}},
		{"pi-google", []string{"--model", "google/gemini-3.1-pro-preview"}},
	} {
		t.Run(tc.runtime, func(t *testing.T) {
			req, store, _ := fixture(t, tc.runtime)
			target, err := mapping.Resolve("pi")
			if err != nil || target.System != "pi-native" {
				t.Fatalf("mapping: %+v %v", target, err)
			}
			c := &capture{}
			count := 0
			d := plan.DefaultDeps(store)
			d.BuildLaunch = c.realBuild
			d.Availability = func(q providerlimits.VerdictQuery) (vendorplugin.Availability, error) {
				count++
				if q.Runtime != req.Runtime || q.Model != req.Model || q.Home != req.Home {
					t.Fatalf("query changed: %+v", q)
				}
				return store.AvailabilityFor(q)
			}
			got, err := plan.Build(context.Background(), d, req)
			if err != nil {
				t.Fatal(err)
			}
			if c.count != 1 || count != 1 || c.mode != agentic.LaunchModeInteractive {
				t.Fatalf("calls/mode: %+v %d", c, count)
			}
			if got.System != "pi-native" || got.Mode != agentic.LaunchModeInteractive || got.Home != req.Home || got.WorkDir != req.WorkDir || got.Binary != filepath.Join(req.WorkDir, "pi") || got.Stdin.Attached {
				t.Fatalf("plan shape: %+v", got)
			}
			if !reflect.DeepEqual(got.Argv, tc.argv) {
				t.Fatalf("argv golden: %q != %q", got.Argv, tc.argv)
			}
			if _, err := os.Stat(filepath.Join(req.WorkDir, "started")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("plan started child or observation failed: %v", err)
			}
		})
	}
}
