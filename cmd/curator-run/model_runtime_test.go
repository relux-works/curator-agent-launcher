package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
)

// The actual entry path admits the exact Luna/max tuple to an inert child.
// The unknown spelling refuses before planning, availability and child dispatch,
// naming the model rather than an empty runtime. The resolver remains scripted.
func TestExplicitModelDiagnosticBeforePlanning(t *testing.T) {
	t.Run("admitted-luna-max", func(t *testing.T) {
		f := entryFixture(t, "codex_cli")
		f.args[2], f.args[4] = "gpt-6-luna", "max"
		insertLauncherArgs(f, "--permissions", "native")
		code, out, stderr := f.run()
		if code != 0 || f.builds != 1 || f.verdicts != 1 || len(out) == 0 || f.request.Runtime != "codex" || string(f.request.Model) != "gpt-6-luna" || f.request.Effort != "max" {
			t.Fatalf("admitted tuple: exit=%d builds=%d verdicts=%d stderr=%q", code, f.builds, f.verdicts, stderr)
		}
		var child childCapture
		if err := json.Unmarshal(out, &child); err != nil {
			t.Fatal(err)
		}
		want := []string{"-c", fmt.Sprintf("model_instructions_file=%q", f.prompt), "-p", "curator-mcp", "-m", "gpt-6-luna", "-c", `model_reasoning_effort="max"`, "", "--model=native", "a\nb"}
		if !slices.Equal(child.Argv, want) {
			t.Fatalf("final Luna argv=%q want=%q", child.Argv, want)
		}
	})
	for _, withEffort := range []bool{false, true} {
		name := "unknown-model-only"
		if withEffort {
			name = "unknown-model-and-effort"
		}
		t.Run(name, func(t *testing.T) {
			f := entryFixture(t, "codex_cli")
			f.args[2], f.args[4] = "gpt-6.1-luna", "max"
			if !withEffort {
				f.args = append(f.args[:3], f.args[5:]...)
			}
			insertLauncherArgs(f, "--permissions", "native")
			code, out, stderr := f.run()
			if code != 1 || len(out) != 0 || f.builds != 0 || f.verdicts != 0 || !bytes.Contains(stderr, []byte("curator-run: defaults_unresolvable:")) || !bytes.Contains(stderr, []byte(`"gpt-6.1-luna"`)) {
				t.Fatalf("wrong model refusal: exit=%d builds=%d verdicts=%d stderr=%q", code, f.builds, f.verdicts, stderr)
			}
			f.assertNoChild(t)
		})
	}
}
