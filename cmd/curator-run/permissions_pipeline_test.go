package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/mapping"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

type permissionRow struct {
	family, name, env string
	tracked           bool
	setup             func(*testing.T, *pipelineFixture)
	code              int
	diagnostic        string
	mode              agentic.PermissionMode
	source            string
	yoloMapped        bool
}

func insertLauncherArgs(f *pipelineFixture, args ...string) {
	for i, arg := range f.args {
		if arg == "--" {
			f.args = append(append(append([]string{}, f.args[:i]...), args...), f.args[i:]...)
			return
		}
	}
	f.args = append(f.args, args...)
}

func replaceNativeArgs(f *pipelineFixture, args ...string) {
	for i, arg := range f.args {
		if arg == "--" {
			f.args = append(append(append([]string{}, f.args[:i+1]...), args...), nil...)
			return
		}
	}
	f.args = append(f.args, append([]string{"--"}, args...)...)
}

func v2Default(f *pipelineFixture, t *testing.T) {
	t.Helper()
	f.usePermissionsFragment(t, "native", false, "default")
}

func globalPermission(t *testing.T, f *pipelineFixture, schema, value string) {
	t.Helper()
	if schema == "curator-run-defaults-v1" {
		f.writeDefaults(t, f.deps.defaults.Operator, fmt.Sprintf(`{"schema":%q,"defaults":{"claude_code":{"permissions":%q}}}`, schema, value))
		return
	}
	f.writeDefaults(t, f.deps.defaults.Operator, fmt.Sprintf(`{"schema":%q,"defaults":{"%s":{"permissions":%q}}}`, schema, f.args[0], value))
}

func TestChoice5PermissionRowsThroughRealCuratorRun(t *testing.T) {
	rows := []permissionRow{
		{family: "mapping", name: "claude-yolo-member", env: "claude_code", setup: func(t *testing.T, f *pipelineFixture) { v2Default(f, t); insertLauncherArgs(f, "--permissions=yolo") }, mode: agentic.PermissionModeYolo, source: "flag", yoloMapped: true},
		{family: "mapping", name: "codex-yolo-alias", env: "codex_cli", setup: func(t *testing.T, f *pipelineFixture) { v2Default(f, t); insertLauncherArgs(f, "--yolo") }, mode: agentic.PermissionModeYolo, source: "flag", yoloMapped: true},
		{family: "mapping", name: "claude-native-member", env: "claude_code", setup: func(t *testing.T, f *pipelineFixture) {
			v2Default(f, t)
			insertLauncherArgs(f, "--permissions", "native")
		}, mode: agentic.PermissionModeNative, source: "flag"},
		{family: "mapping", name: "codex-native-member", env: "codex_cli", setup: func(t *testing.T, f *pipelineFixture) { v2Default(f, t); insertLauncherArgs(f, "--permissions=native") }, mode: agentic.PermissionModeNative, source: "flag"},
		{family: "mapping", name: "pi-native-member", env: "pi", setup: func(t *testing.T, f *pipelineFixture) { v2Default(f, t); insertLauncherArgs(f, "--permissions=native") }, mode: agentic.PermissionModeNative, source: "flag"},
		{family: "mapping", name: "pi-yolo-unsupported", env: "pi", setup: func(t *testing.T, f *pipelineFixture) { v2Default(f, t); insertLauncherArgs(f, "--permissions=yolo") }, code: 1, diagnostic: "permission_mode_unsupported"},

		{family: "precedence", name: "flag-over-profile-and-global", env: "claude_code", setup: func(t *testing.T, f *pipelineFixture) {
			f.usePermissionsFragment(t, "yolo", false, "profile")
			globalPermission(t, f, "curator-run-defaults-v2", "native")
			insertLauncherArgs(f, "--permissions=native")
		}, mode: agentic.PermissionModeNative, source: "flag"},
		{family: "precedence", name: "profile-over-global", env: "codex_cli", setup: func(t *testing.T, f *pipelineFixture) {
			f.usePermissionsFragment(t, "yolo", false, "profile")
			globalPermission(t, f, "curator-run-defaults-v2", "native")
		}, mode: agentic.PermissionModeYolo, source: "profile", yoloMapped: true},
		{family: "precedence", name: "global-over-interactive-default", env: "claude_code", setup: func(t *testing.T, f *pipelineFixture) {
			v2Default(f, t)
			globalPermission(t, f, "curator-run-defaults-v2", "native")
			f.deps.isTerminal = func() bool { return true }
		}, mode: agentic.PermissionModeNative, source: "global"},
		{family: "precedence", name: "default-interactive", env: "claude_code", setup: func(t *testing.T, f *pipelineFixture) {
			v2Default(f, t)
			f.deps.isTerminal = func() bool { return true }
		}, mode: agentic.PermissionModeYolo, source: "default-interactive", yoloMapped: true},
		{family: "precedence", name: "default-headless", env: "codex_cli", setup: func(t *testing.T, f *pipelineFixture) {
			v2Default(f, t)
			f.deps.isTerminal = func() bool { return false }
		}, mode: agentic.PermissionModeNative, source: "default-headless"},

		{family: "invalid-configuration", name: "v1-file-with-permissions-member", env: "claude_code", setup: func(t *testing.T, f *pipelineFixture) {
			f.writeDefaults(t, f.deps.defaults.Operator, `{"schema":"curator-run-defaults-v1","defaults":{"claude_code":{"permissions":"yolo"}}}`)
		}, code: 1, diagnostic: "defaults_config_invalid"},
		{family: "invalid-configuration", name: "global-unknown-mode", env: "codex_cli", setup: func(t *testing.T, f *pipelineFixture) {
			f.writeDefaults(t, f.deps.defaults.Operator, `{"schema":"curator-run-defaults-v2","defaults":{"codex_cli":{"permissions":"automatic"}}}`)
		}, code: 1, diagnostic: "defaults_config_invalid"},
		{family: "invalid-configuration", name: "profile-unknown-mode", env: "claude_code", setup: func(t *testing.T, f *pipelineFixture) {
			f.usePermissionsFragment(t, "native", false, "default")
			var obj map[string]any
			_ = json.Unmarshal([]byte(f.resolver.stdout), &obj)
			obj["permissions"] = map[string]any{"mode": "automatic", "locked": false, "source": "profile"}
			raw, _ := json.Marshal(obj)
			f.resolver.stdout = string(raw) + "\n"
		}, code: 1, diagnostic: "resolve_fragment_invalid"},
		{family: "invalid-configuration", name: "legacy-fragment-with-member", env: "claude_code", setup: func(t *testing.T, f *pipelineFixture) {
			var obj map[string]any
			_ = json.Unmarshal([]byte(f.resolver.stdout), &obj)
			obj["permissions"] = map[string]any{"mode": "native", "locked": false, "source": "default"}
			raw, _ := json.Marshal(obj)
			f.resolver.stdout = string(raw) + "\n"
		}, code: 1, diagnostic: "resolve_fragment_invalid"},
		{family: "invalid-configuration", name: "cli-unknown-mode", env: "codex_cli", setup: func(t *testing.T, f *pipelineFixture) { insertLauncherArgs(f, "--permissions=automatic") }, code: 2, diagnostic: "usage"},
		{family: "cli-conflict", name: "permissions-and-alias", env: "claude_code", setup: func(t *testing.T, f *pipelineFixture) { insertLauncherArgs(f, "--permissions=native", "--yolo") }, code: 2, diagnostic: "usage"},

		{family: "force-native-lock", name: "flag-yolo-refused", env: "claude_code", setup: func(t *testing.T, f *pipelineFixture) {
			f.usePermissionsFragment(t, "native", true, "global")
			insertLauncherArgs(f, "--yolo")
		}, code: 2, diagnostic: "usage"},
		{family: "force-native-lock", name: "global-yolo-refused", env: "codex_cli", setup: func(t *testing.T, f *pipelineFixture) {
			f.usePermissionsFragment(t, "native", true, "global")
			globalPermission(t, f, "curator-run-defaults-v2", "yolo")
		}, code: 2, diagnostic: "usage"},
		{family: "force-native-lock", name: "silent-lock-is-native", env: "claude_code", setup: func(t *testing.T, f *pipelineFixture) { f.usePermissionsFragment(t, "native", true, "global") }, mode: agentic.PermissionModeNative, source: "default-headless"},

		{family: "tracked-yolo", name: "flag", env: "claude_code", tracked: true, setup: func(t *testing.T, f *pipelineFixture) { v2Default(f, t); insertLauncherArgs(f, "--permissions=yolo") }, code: 1, diagnostic: "permission_mode_tracked_unsupported"},
		{family: "tracked-yolo", name: "profile", env: "codex_cli", tracked: true, setup: func(t *testing.T, f *pipelineFixture) { f.usePermissionsFragment(t, "yolo", false, "profile") }, code: 1, diagnostic: "permission_mode_tracked_unsupported"},
		{family: "tracked-yolo", name: "global", env: "claude_code", tracked: true, setup: func(t *testing.T, f *pipelineFixture) {
			v2Default(f, t)
			globalPermission(t, f, "curator-run-defaults-v2", "yolo")
		}, code: 1, diagnostic: "permission_mode_tracked_unsupported"},

		{family: "headless-silence", name: "stdio-non-tty", env: "claude_code", setup: func(t *testing.T, f *pipelineFixture) {
			v2Default(f, t)
			f.deps.isTerminal = func() bool { return false }
		}, mode: agentic.PermissionModeNative, source: "default-headless"},
		{family: "headless-silence", name: "ci-marker", env: "codex_cli", setup: func(t *testing.T, f *pipelineFixture) {
			v2Default(f, t)
			f.deps.isTerminal = func() bool { return true }
			f.addEnv("CI", "true")
		}, mode: agentic.PermissionModeNative, source: "default-headless"},
		{family: "headless-silence", name: "github-actions-present-empty", env: "claude_code", setup: func(t *testing.T, f *pipelineFixture) {
			v2Default(f, t)
			f.deps.isTerminal = func() bool { return true }
			f.addEnv("GITHUB_ACTIONS", "")
		}, mode: agentic.PermissionModeNative, source: "default-headless"},
		{family: "headless-silence", name: "tracked-silence", env: "codex_cli", tracked: true, setup: func(t *testing.T, f *pipelineFixture) { v2Default(f, t) }, mode: agentic.PermissionModeNative, source: "default-headless"},
		{family: "headless-silence", name: "release-classified-codex-exec", env: "codex_cli", setup: func(t *testing.T, f *pipelineFixture) {
			v2Default(f, t)
			f.deps.isTerminal = func() bool { return true }
			replaceNativeArgs(f, "exec", "prompt")
		}, mode: agentic.PermissionModeNative, source: "default-headless"},

		{family: "legacy-yolo-transport", name: "flag", env: "claude_code", setup: func(t *testing.T, f *pipelineFixture) { insertLauncherArgs(f, "--permissions=yolo") }, code: 1, diagnostic: "permission_policy_unsupported"},
		{family: "legacy-yolo-transport", name: "global", env: "codex_cli", setup: func(t *testing.T, f *pipelineFixture) { globalPermission(t, f, "curator-run-defaults-v2", "yolo") }, code: 1, diagnostic: "permission_policy_unsupported"},
		{family: "legacy-yolo-transport", name: "default-interactive", env: "claude_code", setup: func(t *testing.T, f *pipelineFixture) { f.deps.isTerminal = func() bool { return true } }, code: 1, diagnostic: "permission_policy_unsupported"},
	}

	counts := map[string]int{}
	for _, row := range rows {
		row := row
		counts[row.family]++
		t.Run(row.family+"/"+row.name, func(t *testing.T) {
			f := entryFixture(t, row.env, row.tracked)
			row.setup(t, f)
			code, out, stderr := f.run()
			if code != row.code {
				t.Fatalf("exit=%d want=%d stderr=%s", code, row.code, stderr)
			}
			if row.diagnostic != "" {
				if len(out) != 0 || !bytes.Contains(stderr, []byte("curator-run: "+row.diagnostic+":")) {
					t.Fatalf("missing %s refusal: out=%s stderr=%s", row.diagnostic, out, stderr)
				}
				if row.family == "tracked-yolo" || row.family == "legacy-yolo-transport" || row.family == "force-native-lock" {
					f.assertNoChild(t)
				}
				if row.family == "tracked-yolo" && f.builds != 0 {
					t.Fatalf("tracked yolo reached admission: builds=%d", f.builds)
				}
				if row.family == "cli-conflict" && f.resolver.calls != 0 {
					t.Fatalf("conflicting launcher flags reached Curator resolve: calls=%d", f.resolver.calls)
				}
				return
			}
			if f.builds != 1 || f.request.PermissionMode != row.mode {
				t.Fatalf("admitted request mode=%q builds=%d, want mode=%q", f.request.PermissionMode, f.builds, row.mode)
			}
			if !row.tracked && !bytes.Contains(stderr, []byte("curator-run: permissions="+string(row.mode)+" source="+row.source+" mapped=")) {
				t.Fatalf("missing provenance source %q: %s", row.source, stderr)
			}
			if row.yoloMapped {
				target, err := mapping.Resolve(row.env)
				if err != nil {
					t.Fatal(err)
				}
				mapped, err := agentic.Default.PermissionMapping(agentic.SystemID(target.System), f.request.ToolRelease, agentic.PermissionModeYolo)
				if err != nil {
					t.Fatal(err)
				}
				if mapped.Flag == "" || !containsArg(f.plan.Argv, mapped.Flag) {
					t.Fatalf("admitted plan argv %q does not carry module mapping %q", f.plan.Argv, mapped.Flag)
				}
			}
			var child childCapture
			if err := json.Unmarshal(out, &child); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(child.Stdin), "effective-native-policy") && strings.Contains(string(stderr), "effective-native-policy") {
				t.Fatal("untracked effective policy unexpectedly persisted")
			}
		})
	}
	t.Logf("Choice-5 permission row counts: %v", counts)
}

func containsArg(argv []string, want string) bool {
	for _, arg := range argv {
		if arg == want {
			return true
		}
	}
	return false
}

func TestChoice5ProviderConflictRowsThroughRealCuratorRun(t *testing.T) {
	conflicts := []struct {
		env, name, diagnostic string
		args                  []string
		code                  int
	}{
		{"claude_code", "mode-acceptEdits", "plan_refused", []string{"--permission-mode", "acceptEdits"}, 1},
		{"claude_code", "mode-auto", "plan_refused", []string{"--permission-mode=auto"}, 1},
		{"claude_code", "mode-bypassPermissions", "plan_refused", []string{"--permission-mode", "bypassPermissions"}, 1},
		{"claude_code", "mode-manual", "plan_refused", []string{"--permission-mode=manual"}, 1},
		{"claude_code", "mode-dontAsk", "plan_refused", []string{"--permission-mode", "dontAsk"}, 1},
		{"claude_code", "mode-plan", "plan_refused", []string{"--permission-mode=plan"}, 1},
		{"claude_code", "extra-permission-selector", "plan_refused", []string{"--allow-dangerously-skip-permissions"}, 1},
		{"claude_code", "extra-permission-selector-equals", "plan_refused", []string{"--allow-dangerously-skip-permissions=true"}, 1},
		{"claude_code", "restricted-selector", "plan_refused", []string{"--restricted"}, 1},
		{"codex_cli", "short-approval", "plan_refused", []string{"-a", "never"}, 1},
		{"codex_cli", "long-approval", "plan_refused", []string{"--ask-for-approval", "on-request"}, 1},
		{"codex_cli", "short-approval-equals", "plan_refused", []string{"-a=never"}, 1},
		{"codex_cli", "long-approval-equals", "plan_refused", []string{"--ask-for-approval=on-request"}, 1},
		{"codex_cli", "short-sandbox", "plan_refused", []string{"-s", "workspace-write"}, 1},
		{"codex_cli", "long-sandbox", "plan_refused", []string{"--sandbox", "danger-full-access"}, 1},
		{"codex_cli", "short-sandbox-equals", "plan_refused", []string{"-s=read-only"}, 1},
		{"codex_cli", "long-sandbox-equals", "plan_refused", []string{"--sandbox=workspace-write"}, 1},
		{"codex_cli", "approve-for-me", "plan_refused", []string{"--approve-for-me"}, 1},
		{"codex_cli", "dangerous-bypass-family", "plan_refused", []string{"--dangerously-bypass-approvals"}, 1},
		{"codex_cli", "dangerous-bypass-hook-trust", "plan_refused", []string{"--dangerously-bypass-hook-trust"}, 1},
		{"codex_cli", "config-approval-policy", "plan_refused", []string{"-c", "approval_policy=never"}, 1},
		{"codex_cli", "config-sandbox-mode-after-exec", "plan_refused", []string{"exec", "--config", "sandbox_mode=workspace-write"}, 1},
		{"codex_cli", "config-sandbox-permissions", "plan_refused", []string{"--config=sandbox_permissions=[\"disk-full-read-access\"]"}, 1},
		{"claude_code", "unknown-permission-mode-value", "usage", []string{"--permission-mode=unknown"}, 2},
		{"codex_cli", "unknown-approval-value", "usage", []string{"-a=always"}, 2},
		{"codex_cli", "unknown-sandbox-value", "usage", []string{"--sandbox=full"}, 2},
		{"codex_cli", "unknown-config-key", "usage", []string{"--config=unknown_policy=on"}, 2},
	}
	conflictCount, unknownCount := 0, 0
	for _, row := range conflicts {
		row := row
		if row.diagnostic == "plan_refused" {
			conflictCount++
		} else {
			unknownCount++
		}
		t.Run(row.env+"/"+row.name, func(t *testing.T) {
			f := entryFixture(t, row.env, false)
			v2Default(f, t)
			insertLauncherArgs(f, "--permissions=yolo")
			replaceNativeArgs(f, row.args...)
			code, out, stderr := f.run()
			if code != row.code || len(out) != 0 || !bytes.Contains(stderr, []byte("curator-run: "+row.diagnostic+":")) {
				t.Fatalf("native-policy refusal misclassified: exit=%d want=%d out=%q stderr=%s", code, row.code, out, stderr)
			}
			if f.builds != 1 {
				t.Fatalf("module conflict check not reached: builds=%d", f.builds)
			}
			f.assertNoChild(t)
		})
	}
	t.Logf("Choice-5 provider-policy rows: conflicts=%d unknown-value refusals=%d", conflictCount, unknownCount)
}

func TestChoice5MappedBypassDuplicateRowsThroughRealCuratorRun(t *testing.T) {
	for _, tc := range []struct{ env, system, release string }{
		{"claude_code", "claude-code", "2.1.261"},
		{"codex_cli", "codex", "0.153.2"},
	} {
		t.Run(tc.env, func(t *testing.T) {
			f := entryFixture(t, tc.env, false)
			v2Default(f, t)
			mapping, err := agentic.Default.PermissionMapping(agentic.SystemID(tc.system), tc.release, agentic.PermissionModeYolo)
			if err != nil || mapping.Flag == "" {
				t.Fatalf("verified module mapping = %+v, %v", mapping, err)
			}
			insertLauncherArgs(f, "--permissions=yolo")
			replaceNativeArgs(f, mapping.Flag)
			code, out, stderr := f.run()
			if code != 2 || len(out) != 0 || !bytes.Contains(stderr, []byte("curator-run: usage:")) {
				t.Fatalf("duplicate mapped flag admitted: exit=%d out=%q stderr=%s", code, out, stderr)
			}
			if f.builds != 1 {
				t.Fatalf("module duplicate check not reached: builds=%d", f.builds)
			}
			f.assertNoChild(t)
		})
	}
}

func TestChoice5EffectiveNativePolicyLineAndTrackedRecord(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		t.Run(fmt.Sprintf("tracked=%v", tracked), func(t *testing.T) {
			f := entryFixture(t, "claude_code", tracked)
			if err := os.WriteFile(filepath.Join(f.home, "settings.json"), []byte(`{"permissions":{"defaultMode":"bypassPermissions","allow":["rule-1","rule-2","rule-3","rule-4","rule-5","rule-6","rule-7","rule-8","rule-9"],"additionalDirectories":["/tmp"]}}`), 0600); err != nil {
				t.Fatal(err)
			}
			code, out, stderr := f.run()
			if code != 0 || !bytes.Contains(stderr, []byte("curator-run: effective-native-policy: relaxation=")) || !bytes.Contains(stderr, []byte("permissions.allow (9 rules)")) {
				t.Fatalf("native-policy line missing: exit=%d stderr=%s", code, stderr)
			}
			if bytes.Contains(stderr, []byte("managed settings (system, MDM")) {
				t.Fatalf("inspection claimed a source the module did not inspect: %s", stderr)
			}
			if !bytes.Contains(stderr, []byte("source="+filepath.Join(f.home, "settings.json"))) {
				t.Fatalf("native-policy line did not identify the inspected file: %s", stderr)
			}
			var child childCapture
			if err := json.Unmarshal(out, &child); err != nil {
				t.Fatal(err)
			}
			if tracked {
				var doc struct {
					Extensions map[string]json.RawMessage `json:"extensions"`
				}
				if err := json.Unmarshal(child.Stdin, &doc); err != nil {
					t.Fatal(err)
				}
				raw := doc.Extensions["works.relux.curator.effective-native-policy"]
				var policy struct {
					Relaxations []string `json:"relaxations"`
					Source      string   `json:"source"`
				}
				if err := json.Unmarshal(raw, &policy); err != nil {
					t.Fatalf("tracked native-policy record is not a usable object: %s (%v)", raw, err)
				}
				if !reflect.DeepEqual(policy.Relaxations, []string{"permissions.additionalDirectories", "permissions.allow", "permissions.defaultMode"}) || policy.Source != filepath.Join(f.home, "settings.json") {
					t.Fatalf("tracked native-policy record = %+v", policy)
				}
			} else if strings.Contains(string(child.Stdin), "effective-native-policy") {
				t.Fatalf("untracked launch persisted policy record: %s", child.Stdin)
			}
		})
	}
}

func TestChoice5V2FragmentCarriesPermissionMember(t *testing.T) {
	f := entryFixture(t, "claude_code", false)
	f.usePermissionsFragment(t, "native", false, "default")
	parsed, err := fragment.Parse([]byte(f.resolver.stdout))
	if err != nil || parsed.Revision != fragment.IdentityV2 || parsed.Permissions == nil {
		t.Fatalf("v2 permission member not parsed: fragment=%+v err=%v", parsed, err)
	}
}
