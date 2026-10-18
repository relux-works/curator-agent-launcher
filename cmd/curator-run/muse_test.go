package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
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
	f.deps.moduleVersion = func() string { return "v0.5.68" }
	data, err := os.ReadFile("../../internal/fragment/testdata/v3/muse.json")
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	object["fragment"] = fragment.IdentityV4
	object["credential_store"] = map[string]string{"selection": "file"}
	object["env"].(map[string]any)["TBH_CREDENTIAL_BACKEND"] = "file"
	data, err = json.Marshal(object)
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
	frag := &fragment.Fragment{Revision: fragment.IdentityV4, Environment: fragment.EnvMuse, Env: wire.Env,
		Profile: fragment.Profile{Name: "default", LockSHA256: strings.Repeat("a", 64)}, Precedence: fragment.Precedence{Winner: "higher-weight", Placement: "winner-last"}, Permissions: &fragment.Permissions{Mode: "native", Source: "default"}, CredentialStore: &fragment.CredentialStore{Selection: "file"}}
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
	writeFixture(t, filepath.Join(f.dir, "muse-version"), []byte("Muse Code 1.4.6 (1.4.6-R5000.1)\n"), 0600)
	writeFixture(t, filepath.Join(f.dir, "muse-help"), []byte("Options:\n  --yolo  Bypass approvals and sandbox\n"), 0600)
	t.Setenv("CURATOR_TEST_FRAGMENT", fragmentPath)
	t.Setenv("CURATOR_TEST_RESOLVE_ARGV", filepath.Join(f.dir, "resolve-argv"))
	f.deps.resolver = fragment.NewWithRunner(curator, fragment.ExecRunner{})
	f.deps.environ = func() []string {
		return []string{"PATH=" + f.dir, "HOME=" + f.dir, "XDG_CONFIG_HOME=/foreign/config"}
	}
	f.deps.isTerminal = func() bool { return false }
	f.args = []string{"muse", "--permissions=native"}
	return f, frag
}

// Production call sites: run -> Resolve -> resolvePermission -> plan.Build
// (real BuildLaunchWithEnvironment in Interactive mode) -> Compose -> fake Muse.
// These rows require admission; a plugin regression to exec-only must fail.
func TestMuseV4InteractiveThroughRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		mode agentic.PermissionMode
	}{
		{"native", []string{"muse", "--permissions", "native"}, agentic.PermissionModeNative},
		{"yolo", []string{"muse", "--permissions", "yolo"}, agentic.PermissionModeYolo},
		{"yolo-alias", []string{"muse", "--yolo"}, agentic.PermissionModeYolo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, frag := museFixture(t)
			f.args = tc.args
			code, out, stderr := f.run()
			if code != 0 || f.builds != 1 || f.verdicts != 1 {
				t.Fatalf("interactive admission: exit=%d builds=%d verdicts=%d stderr=%s", code, f.builds, f.verdicts, stderr)
			}
			argv, err := os.ReadFile(filepath.Join(f.dir, "resolve-argv"))
			if err != nil || string(argv) != "env\nresolve\nmuse\n--repair\n--format\njson" {
				t.Fatalf("resolve argv=%q err=%v", argv, err)
			}
			if f.request.Runtime != "muse" || f.request.Model != "muse-wonder-1.0-contributor" || f.request.Effort != "xhigh" || f.request.Home != frag.Home() || f.request.ToolRelease != "1.4.6" || f.request.PermissionMode != tc.mode || !reflect.DeepEqual(f.request.Env, f.deps.environ()) {
				t.Fatalf("wrong Muse request: %+v", f.request)
			}
			wantArgv := museArgv(f.dir, tc.mode)
			if !reflect.DeepEqual(f.plan.Argv, wantArgv) {
				t.Fatalf("plan argv=%q want=%q", f.plan.Argv, wantArgv)
			}
			var child childCapture
			if err := json.Unmarshal(out, &child); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(child.Argv, wantArgv) || child.WorkDir != f.dir || !reflect.DeepEqual(child.Stdin, []byte("parent stdin\n\x00\xff")) {
				t.Fatalf("interactive child=%+v want argv=%q", child, wantArgv)
			}
			assertMuseEnv(t, child.Env, f.dir, frag)
			mapped := "none"
			if tc.mode == agentic.PermissionModeYolo {
				mapped = "module-attested"
			}
			if !strings.Contains(string(stderr), "curator-run: permissions="+string(tc.mode)+" source=flag mapped="+mapped+"\n") {
				t.Fatalf("permission diagnostic: %s", stderr)
			}
			golden(t, "pipeline-muse-v4-"+tc.name, child, f.dir)
		})
	}
}

func museArgv(workdir string, mode agentic.PermissionMode) []string {
	args := []string{"--model", "muse-wonder-1.0-contributor", "--reasoning-effort", "xhigh", "--workspace", workdir}
	if mode == agentic.PermissionModeYolo {
		args = append(args, "--yolo")
	}
	return args
}

// Production call site: plan.Build with the real tagged BuildLaunch API.
func TestMuseV4PlanBuildInteractive(t *testing.T) {
	for _, mode := range []agentic.PermissionMode{agentic.PermissionModeNative, agentic.PermissionModeYolo} {
		t.Run(string(mode), func(t *testing.T) {
			f, frag := museFixture(t)
			deps := plan.DefaultDeps(nil)
			deps.Registry = f.deps.registry
			reads := 0
			deps.Availability = func(providerlimits.VerdictQuery) (vendorplugin.Availability, error) {
				reads++
				return vendorplugin.Availability{State: vendorplugin.AvailabilityHealthy}, nil
			}
			req := plan.Request{Runtime: "muse", Model: "muse-wonder-1.0-contributor", Effort: "xhigh", PermissionMode: mode, ToolRelease: "1.4.6", Home: frag.Home(), WorkDir: f.dir, Env: f.deps.environ()}
			built, err := plan.Build(context.Background(), deps, req)
			if err != nil || reads != 1 || built.Plan.Binary != f.binary || !reflect.DeepEqual(built.Plan.Argv, museArgv(f.dir, mode)) {
				t.Fatalf("interactive plan=%+v err=%v reads=%d", built.Plan, err, reads)
			}
			wantEnv := []string{"PATH=" + f.dir, "HOME=" + f.dir, "XDG_CONFIG_HOME=/foreign/config", "MUSE_NO_AUTO_UPDATE=1"}
			if !reflect.DeepEqual(built.Plan.Env, wantEnv) || !reflect.DeepEqual(built.OwnedEnv, []string{"MUSE_NO_AUTO_UPDATE=1"}) {
				t.Fatalf("plan env=%q owned=%q", built.Plan.Env, built.OwnedEnv)
			}
			f.assertNoChild(t)
		})
	}
}

func assertMuseEnv(t *testing.T, env []string, nativeHome string, frag *fragment.Fragment) {
	t.Helper()
	want := []string{"HOME=" + nativeHome, "MUSE_NO_AUTO_UPDATE=1", "PATH=" + nativeHome, "TBH_CREDENTIAL_BACKEND=file",
		"XDG_CACHE_HOME=" + frag.Env["XDG_CACHE_HOME"], "XDG_CONFIG_HOME=" + frag.Env["XDG_CONFIG_HOME"],
		"XDG_DATA_HOME=" + frag.Env["XDG_DATA_HOME"], "XDG_STATE_HOME=" + frag.Env["XDG_STATE_HOME"]}
	if !reflect.DeepEqual(env, want) {
		t.Fatalf("Muse env=%q want=%q", env, want)
	}
}

func TestMuseV4CompositionPreservesHOME(t *testing.T) {
	f, frag := museFixture(t)
	p := agentic.Plan{Binary: f.binary, WorkDir: f.dir, Argv: []string{"opaque"}, Env: []string{"HOME=" + f.dir, "PATH=" + f.dir, "MUSE_NO_AUTO_UPDATE=1"}}
	v, err := composition.Compose(p, []string{"MUSE_NO_AUTO_UPDATE=1"}, *frag, composition.PromptApplication{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertMuseEnv(t, v.Env, f.dir, frag)
	wantLiterals := map[string]string{"MUSE_NO_AUTO_UPDATE": "1"}
	for k, value := range frag.Env {
		wantLiterals[k] = value
	}
	if !reflect.DeepEqual(v.EnvLiterals, wantLiterals) || !reflect.DeepEqual(v.Argv, p.Argv) {
		t.Fatalf("XDG-only overrides (no HOME): %+v", v)
	}
	target, err := mapping.Resolve(frag.Environment)
	if err != nil || target.System != "muse" || target.Provider != "muse" {
		t.Fatalf("Muse mapping=%+v err=%v", target, err)
	}
}

func TestMuseV4MalformedReleaseRefusedThroughRun(t *testing.T) {
	for _, mode := range []string{"native", "yolo"} {
		for _, version := range []string{"", "garbage\n", "Muse Code 1.4.6 (1.4.5-R5000.1)\n"} {
			t.Run(mode+"/version="+version, func(t *testing.T) {
				f, _ := museFixture(t)
				f.args = []string{"muse", "--permissions", mode}
				writeFixture(t, filepath.Join(f.dir, "muse-version"), []byte(version), 0600)
				code, out, stderr := f.run()
				wantBuilds := 0
				if mode == "native" {
					wantBuilds = 1
				} // Native still requires a sealed, attested plan.
				if code != 1 || len(out) != 0 || f.builds != wantBuilds || f.verdicts != 0 || !strings.Contains(string(stderr), "plan_refused:") || !strings.Contains(string(stderr), agentic.ErrToolReleaseUndetected.Error()) {
					t.Fatalf("malformed release admitted: exit=%d builds=%d verdicts=%d stdout=%s stderr=%s", code, f.builds, f.verdicts, out, stderr)
				}
				f.assertNoChild(t)
			})
		}
	}
}

// run -> resolvePermission probes a claim; the real module builder must
// compare it with the selected binary's later attestation, for both postures.
func TestMuseV4ReleaseMismatchRefusedThroughRun(t *testing.T) {
	for _, mode := range []string{"native", "yolo"} {
		t.Run(mode, func(t *testing.T) {
			f, _ := museFixture(t)
			original := f.deps.build
			f.deps.build = func(ctx context.Context, r *vendorplugin.Registry, req vendorplugin.SpawnRequest, launchMode agentic.LaunchMode) (agentic.PlanWithEnvironment, error) {
				if req.ToolRelease != "1.4.6" {
					t.Fatalf("probe claim lost: %q", req.ToolRelease)
				}
				writeFixture(t, filepath.Join(f.dir, "muse-version"), []byte("Muse Code 1.4.7 (1.4.7-R5001.1)\n"), 0600)
				return original(ctx, r, req, launchMode)
			}
			f.args = []string{"muse", "--permissions", mode}
			code, out, stderr := f.run()
			if code != 1 || len(out) != 0 || f.builds != 1 || f.verdicts != 0 || !strings.Contains(string(stderr), "caller tool release does not match the attested release") {
				t.Fatalf("mismatch admitted: exit=%d builds=%d verdicts=%d stderr=%s", code, f.builds, f.verdicts, stderr)
			}
			f.assertNoChild(t)
		})
	}
}

func TestMuseV4YoloRequiresHelpEvidenceThroughRun(t *testing.T) {
	for _, help := range []string{"", "Example: muse --yolo\n", "  --yolo-extra  Not the bypass flag\n"} {
		t.Run(help, func(t *testing.T) {
			f, _ := museFixture(t)
			writeFixture(t, filepath.Join(f.dir, "muse-help"), []byte(help), 0600)
			f.args = []string{"muse", "--permissions", "yolo"}
			code, out, stderr := f.run()
			wantDiagnostic := "permission_mode_unsupported:"
			if help == "" {
				wantDiagnostic = "plan_refused:"
			}
			if code != 1 || len(out) != 0 || f.builds != 1 || f.verdicts != 0 || !strings.Contains(string(stderr), wantDiagnostic) {
				t.Fatalf("unproven yolo admitted: exit=%d builds=%d verdicts=%d stderr=%s", code, f.builds, f.verdicts, stderr)
			}
			f.assertNoChild(t)
		})
	}
}

func TestMuseV4NovelBuildAndWonderThroughRun(t *testing.T) {
	for _, release := range []string{"1.4.6", "9.9.9"} {
		for _, mode := range []string{"native", "yolo"} {
			t.Run(release+"/"+mode, func(t *testing.T) {
				f, _ := museFixture(t)
				writeFixture(t, filepath.Join(f.dir, "muse-version"), []byte("Muse Code "+release+" ("+release+"-R5000.1)\n"), 0600)
				f.args = []string{"muse", "--profile", "test-profile", "--model", "muse-wonder", "--permissions", mode}
				code, out, stderr := f.run()
				if code != 0 || f.builds != 1 || f.verdicts != 1 || f.request.Model != "muse-wonder" || f.request.Effort != "xhigh" || f.request.ToolRelease != release || f.plan.ToolRelease != release {
					t.Fatalf("wonder admission: exit=%d request=%+v stderr=%s", code, f.request, stderr)
				}
				var child childCapture
				if err := json.Unmarshal(out, &child); err != nil {
					t.Fatal(err)
				}
				if !containsArg(child.Argv, "muse-wonder-1.0-contributor") {
					t.Fatalf("alias not forwarded canonically: %q", child.Argv)
				}
				argv, err := os.ReadFile(filepath.Join(f.dir, "resolve-argv"))
				if err != nil || !strings.Contains(string(argv), "--profile\ntest-profile") {
					t.Fatalf("profile not forwarded: %q %v", argv, err)
				}
			})
		}
	}
}

// Production call site: run -> cli.Parse, before any fragment/env-specific
// stage. An unknown --permissions value must refuse with usage even when
// the operand is the muse environment; nothing may resolve or launch.
func TestMuseV4UnknownPermissionsRefusedThroughRun(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		value string
	}{
		{"equals", []string{"muse", "--permissions=automatic"}, "automatic"},
		{"separate", []string{"muse", "--permissions", "auto"}, "auto"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := museFixture(t)
			f.args = tc.args
			code, out, stderr := f.run()
			if code != 2 || len(out) != 0 || f.builds != 0 || f.verdicts != 0 {
				t.Fatalf("unknown permissions admitted: exit=%d builds=%d verdicts=%d stdout=%s stderr=%s", code, f.builds, f.verdicts, out, stderr)
			}
			if !strings.Contains(string(stderr), "curator-run: usage:") || !strings.Contains(string(stderr), "--permissions accepts native or yolo, got "+strconv.Quote(tc.value)) {
				t.Fatalf("usage refusal misnamed: stderr=%s", stderr)
			}
			if _, err := os.Stat(filepath.Join(f.dir, "resolve-argv")); !os.IsNotExist(err) {
				t.Fatalf("refused launch reached Curator resolve: err=%v", err)
			}
			f.assertNoChild(t)
		})
	}
}

func TestMuseV4DuplicateYoloRefusedThroughRun(t *testing.T) {
	for _, args := range [][]string{{"--yolo"}, {"--yolo=true"}, {"--disable-approval", "--disable-sandbox"}} {
		t.Run(strings.Join(args, ","), func(t *testing.T) {
			f, _ := museFixture(t)
			f.args = append([]string{"muse", "--permissions", "yolo", "--"}, args...)
			code, out, stderr := f.run()
			if code != 2 || len(out) != 0 || f.builds != 1 || f.verdicts != 0 || !strings.Contains(string(stderr), agentic.ErrPermissionModeDuplicate.Error()) {
				t.Fatalf("duplicate admitted: exit=%d builds=%d verdicts=%d stdout=%s stderr=%s", code, f.builds, f.verdicts, out, stderr)
			}
			f.assertNoChild(t)
		})
	}
}

// Production call sites: run -> launch -> plan.Context/plan.Build -> real
// BuildLaunchWithEnvironment -> composition -> execution. Muse v4 carries typed
// backend policy, while path_prepend remains reserved and the tail stays opaque.
func TestMuseV4InteractiveReservedContextAndNativeSuffix(t *testing.T) {
	for _, mode := range []agentic.PermissionMode{agentic.PermissionModeNative, agentic.PermissionModeYolo} {
		t.Run(string(mode), func(t *testing.T) {
			f, frag := museFixture(t)
			path := filepath.Join(f.dir, "muse-fragment.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var wire map[string]any
			if err := json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			wire["path_prepend"] = f.home + "/reserved-bin"
			raw, err = json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			writeFixture(t, path, raw, 0600)
			native := []string{"", "literal\nvalue", "--system-prompt-file=literal"}
			f.args = append([]string{"muse", "--permissions", string(mode), "--"}, native...)
			code, out, stderr := f.run()
			if code != 0 || f.builds != 1 || f.verdicts != 1 {
				t.Fatalf("interactive Muse launch: exit=%d builds=%d verdicts=%d stderr=%s", code, f.builds, f.verdicts, stderr)
			}
			if f.request.Context == nil || f.request.Context.Revision != fragment.IdentityV4 || f.request.Context.RootContext != nil || f.request.Context.CredentialStore == nil || f.request.Context.PathPrepend != "" {
				t.Fatalf("wrong explicit Muse carrier: %+v", f.request.Context)
			}
			if _, present := f.plan.CuratorContextProvenanceSnapshot(); !present {
				t.Fatal("Muse v4 plan must attest its typed context")
			}
			var child childCapture
			if err := json.Unmarshal(out, &child); err != nil {
				t.Fatal(err)
			}
			want := append(museArgv(f.dir, mode), native...)
			if !reflect.DeepEqual(child.Argv, want) || !reflect.DeepEqual(f.plan.Argv, want) {
				t.Fatalf("Muse native suffix changed: child=%q plan=%q want=%q", child.Argv, f.plan.Argv, want)
			}
			assertMuseEnv(t, child.Env, f.dir, frag)
			unchanged, err := os.ReadFile(path)
			if err != nil || !reflect.DeepEqual(unchanged, raw) {
				t.Fatalf("reserved fragment changed: %v", err)
			}
		})
	}
}

// A release admitted without a root fact must still refuse a required
// exclusion it cannot qualify. The real tagged builder owns that distinction;
// refusal occurs before the provider-limits boundary or provider execution.
func TestMuseV4UnqualifiedExclusionBuild(t *testing.T) {
	for _, root := range []bool{false, true} {
		t.Run(strconv.FormatBool(root), func(t *testing.T) {
			f, frag := museFixture(t)
			if root {
				frag.RootContext = &fragment.RootContext{Exclusive: true}
			}
			reads := 0
			deps := plan.DefaultDeps(nil)
			deps.Registry = f.deps.registry
			deps.ModuleVersion = func() string { return "v0.5.68" }
			deps.Availability = func(providerlimits.VerdictQuery) (vendorplugin.Availability, error) {
				reads++
				return vendorplugin.Availability{State: vendorplugin.AvailabilityHealthy}, nil
			}
			_, err := plan.Build(context.Background(), deps, plan.Request{
				Runtime: "muse", Model: "muse-wonder-1.0-contributor", Effort: "xhigh",
				PermissionMode: agentic.PermissionModeNative, ToolRelease: "1.4.6",
				Home: frag.Home(), WorkDir: f.dir, Env: f.deps.environ(), Context: plan.Context(frag, ""),
			})
			if root {
				if err == nil || !strings.Contains(err.Error(), "required foreign personal context exclusion is unqualified") || reads != 0 {
					t.Fatalf("unqualified exclusion: %v reads=%d", err, reads)
				}
			} else if err != nil || reads != 1 {
				t.Fatalf("absent-fact control: %v reads=%d", err, reads)
			}
			f.assertNoChild(t)
		})
	}
}
