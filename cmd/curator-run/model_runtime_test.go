package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
)

// TestExplicitModelDiagnosticBeforePlanning proves admitted Codex models keep
// their model, effort, and native tail while unknown identifiers refuse
// before plan construction, availability, or child dispatch.
func TestExplicitModelDiagnosticBeforePlanning(t *testing.T) {
	t.Run("admitted-model-and-effort", func(t *testing.T) {
		f := entryFixture(t, "codex_cli")
		f.args[2], f.args[4] = "gpt-6.1-sol", "high"
		insertLauncherArgs(f, "--permissions", "native", "--native")
		code, out, stderr := f.run()
		if code != 0 || f.builds != 1 || f.verdicts != 1 || len(out) == 0 || f.request.Runtime != "codex" || string(f.request.Model) != "gpt-6.1-sol" || f.request.Effort != "high" {
			t.Fatalf("admitted tuple: exit=%d builds=%d verdicts=%d stderr=%q", code, f.builds, f.verdicts, stderr)
		}
		var child childCapture
		if err := json.Unmarshal(out, &child); err != nil {
			t.Fatal(err)
		}
		want := []string{"-c", fmt.Sprintf("model_instructions_file=%q", f.prompt), "-p", "curator-mcp", "-m", "gpt-6.1-sol", "-c", `model_reasoning_effort="high"`, "", "--model=native", "a\nb"}
		if !slices.Equal(child.Argv, want) {
			t.Fatalf("final Codex argv=%q want=%q", child.Argv, want)
		}
	})
	t.Run("model-only-keeps-catalog-recommendation", func(t *testing.T) {
		f := entryFixture(t, "codex_cli")
		f.args[2] = "gpt-6.1-sol"
		f.args = append(f.args[:3], f.args[5:]...)
		insertLauncherArgs(f, "--permissions", "native", "--native")
		code, out, stderr := f.run()
		if code != 0 || f.builds != 1 || f.verdicts != 1 || len(out) == 0 || f.request.Runtime != "codex" || string(f.request.Model) != "gpt-6.1-sol" || f.request.Effort != "medium" {
			t.Fatalf("model-only tuple: exit=%d builds=%d verdicts=%d stderr=%q", code, f.builds, f.verdicts, stderr)
		}
		var child childCapture
		if err := json.Unmarshal(out, &child); err != nil {
			t.Fatal(err)
		}
		want := []string{"-c", fmt.Sprintf("model_instructions_file=%q", f.prompt), "-p", "curator-mcp", "-m", "gpt-6.1-sol", "-c", `model_reasoning_effort="medium"`, "", "--model=native", "a\nb"}
		if !slices.Equal(child.Argv, want) {
			t.Fatalf("model-only Codex argv=%q want=%q", child.Argv, want)
		}
	})
	for _, tc := range []struct {
		name       string
		model      string
		withEffort bool
	}{
		{name: "unknown-model-only", model: "gpt-6.9-sol"},
		{name: "unknown-model-and-effort", model: "gpt-6.9-sol", withEffort: true},
		{name: "neighboring-unknown-with-effort", model: "gpt-6.9-solx", withEffort: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := entryFixture(t, "codex_cli")
			f.args[2] = tc.model
			if !tc.withEffort {
				f.args = append(f.args[:3], f.args[5:]...)
			}
			insertLauncherArgs(f, "--permissions", "native", "--native")
			code, out, stderr := f.run()
			if code != 1 || len(out) != 0 || f.builds != 0 || f.verdicts != 0 || !bytes.Contains(stderr, []byte("curator-run: defaults_unresolvable:")) || !bytes.Contains(stderr, []byte(fmt.Sprintf("%q", tc.model))) || !bytes.Contains(stderr, []byte(`"codex_cli"`)) || bytes.Contains(stderr, []byte("plan_refused")) {
				t.Fatalf("wrong model refusal: exit=%d builds=%d verdicts=%d stderr=%q", code, f.builds, f.verdicts, stderr)
			}
			f.assertNoChild(t)
		})
	}
}
