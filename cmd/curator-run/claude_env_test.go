package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
)

// selectClaudeEnv supplies a validated /3 selection directly. The parser and
// its candidate gate are covered in internal/launcherconfig; these tests
// cover what the launcher does with an admitted selection.
func selectClaudeEnv(f *pipelineFixture, value string) {
	f.deps.readSelectedConfig = func(context.Context) (launcherconfig.Selected, error) {
		return launcherconfig.Selected{ConfigSchema: 9, ConfigPath: "/fixture/selected.json", ClaudeEnv: &launcherconfig.ClaudeEnv{BashMaxOutputLength: value}}, nil
	}
}

func childValues(child childCapture, name string) []string {
	var out []string
	for _, entry := range child.Env {
		if v, ok := strings.CutPrefix(entry, name+"="); ok {
			out = append(out, v)
		}
	}
	return out
}

// A native Claude Code child receives exactly the selected value, which
// overrides a conflicting inherited one; other variables are kept. Codex and
// Pi children get no contribution and keep an inherited value as is, and an
// empty selection changes nothing.
func TestClaudeEnvNativeChildOnly(t *testing.T) {
	for _, tc := range []struct {
		name, runtime, selected string
		inherited               bool
		want                    []string
	}{
		{"claude-selected", "claude_code", "8000", false, []string{"8000"}},
		{"claude-overrides-inherited", "claude_code", "8000", true, []string{"8000"}},
		{"claude-empty-keeps-inherited", "claude_code", "", true, []string{"12"}},
		{"claude-empty-adds-nothing", "claude_code", "", false, nil},
		{"codex-no-contribution", "codex_cli", "8000", false, nil},
		{"codex-keeps-inherited", "codex_cli", "8000", true, []string{"12"}},
		{"pi-no-contribution", "pi", "8000", false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := entryFixture(t, tc.runtime)
			insertLauncherArgs(f, "--permissions", "native")
			if tc.inherited {
				f.addEnv(launcherconfig.ClaudeEnvVariable, "12")
			}
			selectClaudeEnv(f, tc.selected)
			code, out, stderr := f.run()
			var child childCapture
			if code != 0 || json.Unmarshal(out, &child) != nil {
				t.Fatalf("launch failed: code=%d stderr=%s", code, stderr)
			}
			got := childValues(child, launcherconfig.ClaudeEnvVariable)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("child values %v, want %v", got, tc.want)
			}
			if parent := childValues(child, "PARENT"); len(parent) != 1 || parent[0] != "direct-only" {
				t.Fatal("unrelated environment changed")
			}
		})
	}
}

// The value goes to the child only: never into argv, the built request or
// plan, or the launcher's own diagnostics.
func TestClaudeEnvStaysOutOfPlan(t *testing.T) {
	const value = "54321"
	f := entryFixture(t, "claude_code")
	insertLauncherArgs(f, "--permissions", "native")
	selectClaudeEnv(f, value)
	code, out, stderr := f.run()
	var child childCapture
	if code != 0 || json.Unmarshal(out, &child) != nil {
		t.Fatalf("launch failed: code=%d stderr=%s", code, stderr)
	}
	raw, _ := json.Marshal(struct {
		Request any
		Plan    any
	}{f.request, f.plan})
	if strings.Contains(string(raw), value) || strings.Contains(string(stderr), value) || strings.Contains(strings.Join(child.Argv, " "), value) {
		t.Fatal("selection reached an earlier surface")
	}
	if got := childValues(child, launcherconfig.ClaudeEnvVariable); len(got) != 1 || got[0] != value {
		t.Fatal("selection missing from the child")
	}
}

// The hosted route refuses a Claude selection before any plan is built or
// child started, rather than starting without it. A non-Claude runtime is
// not refused by the same machine selection; its native control launches.
func TestClaudeEnvHostedRefusesClaudeOnly(t *testing.T) {
	f := entryFixture(t, "claude_code")
	f.writeDefaults(t, filepath.Join(f.dir, "machine", "launcher.json"), `{"schema":"curator-run-launcher-v1","host_mode":"hosted"}`)
	selectClaudeEnv(f, "8000")
	code, out, stderr := f.run()
	if code == 0 || len(out) != 0 || f.builds != 0 || !strings.Contains(string(stderr), "selected claude_env requires native execution") || strings.Contains(string(stderr), "8000") {
		t.Fatalf("hosted Claude selection not refused first: code=%d builds=%d stderr=%s", code, f.builds, stderr)
	}
	f.assertNoChild(t)

	control := entryFixture(t, "claude_code")
	control.writeDefaults(t, filepath.Join(control.dir, "machine", "launcher.json"), `{"schema":"curator-run-launcher-v1","host_mode":"hosted"}`)
	selectClaudeEnv(control, "")
	_, _, stderr = control.run()
	if strings.Contains(string(stderr), "claude_env") {
		t.Fatal("empty selection refused on the hosted route")
	}
}

// A claude_env member refusal surfaces only its stable code and fixed path;
// other projection refusals keep the generic text. No child starts.
func TestClaudeEnvRefusalDiagnostic(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"member", &launcherconfig.ClaudeEnvError{Code: launcherconfig.CodeClaudeEnvValueOverBound}, "claude_env_value_over_bound: extensions.claude_env"},
		{"generic", launcherconfig.ErrSelected, launcherconfig.ErrSelected.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := entryFixture(t, "claude_code")
			f.deps.readSelectedConfig = func(context.Context) (launcherconfig.Selected, error) { return launcherconfig.Selected{}, tc.err }
			code, out, stderr := f.run()
			if code == 0 || len(out) != 0 || !strings.Contains(string(stderr), tc.want) || f.builds != 0 {
				t.Fatalf("refusal: code=%d stderr=%s", code, stderr)
			}
			if tc.name == "generic" && strings.Contains(string(stderr), "claude_env") {
				t.Fatal("generic refusal named the member")
			}
			f.assertNoChild(t)
		})
	}
}
