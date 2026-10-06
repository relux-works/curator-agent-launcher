package hosted_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/relux-works/skill-agents-management/pkg/agentic"
	claudeSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/claude"
	codexSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/codex"

	"github.com/relux-works/curator-agent-launcher/internal/hosted"
)

func claudeRegistry(t *testing.T) *agentic.Registry {
	t.Helper()
	reg := agentic.NewRegistry()
	if err := reg.Register(claudeSystem.New()); err != nil {
		t.Fatal(err)
	}
	return reg
}

// TestWrapperArgs pins the CLI-to-module selector shape: at most one wrapper
// selector, and both at once conflict before the module call with no
// identity in the detail.
func TestWrapperArgs(t *testing.T) {
	uuid := "01234567-89ab-cdef-0123-456789abcdef"
	for _, tc := range []struct {
		name                     string
		requested, handleSet, id bool
		handle, resumeID         string
		want                     []string
		wantErr                  bool
	}{
		{"none", false, false, false, "", "", nil, false},
		{"resume-latest", true, false, false, "", "", []string{"resume"}, false},
		{"resume-handle", true, true, false, "SES-1", "", []string{"resume", "SES-1"}, false},
		{"resume-flag", false, false, true, "", uuid, []string{"--resume", uuid}, false},
		{"both-conflict", true, true, true, "SES-1", uuid, nil, true},
		{"latest-and-flag-conflict", true, false, true, "", uuid, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := hosted.WrapperArgs(tc.requested, tc.handle, tc.handleSet, tc.resumeID, tc.id)
			if tc.wantErr {
				resumeErr, ok := err.(*hosted.ResumeError)
				if !ok {
					t.Fatalf("error type %T, want *hosted.ResumeError", err)
				}
				if !strings.HasPrefix(resumeErr.Error(), "session_resume_invalid: ") {
					t.Fatalf("error = %q", resumeErr.Error())
				}
				for _, secret := range []string{tc.handle, tc.resumeID} {
					if secret != "" && strings.Contains(resumeErr.Error(), secret) {
						t.Fatalf("error %q carries the selector identity", resumeErr.Error())
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("wrapper = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestElevateResume drives the module typed-intent API through the launcher
// wrapper for the admitted system: wrapper selectors and native tail
// selectors elevate to typed intent, conflicts and malformed identities
// refuse session_resume_invalid, and elevated selectors leave the tail.
func TestElevateResume(t *testing.T) {
	uuid := "01234567-89ab-cdef-0123-456789abcdef"
	other := "11111111-2222-3333-4444-555555555555"
	for _, tc := range []struct {
		name          string
		wrapper       []string
		native        []string
		wantKind      agentic.ResumeKind
		wantID        string
		wantTail      []string
		wantErrSubstr string
	}{
		{"new", nil, []string{"--model", "m"}, agentic.ResumeNew, "", []string{"--model", "m"}, ""},
		{"native-empty", nil, nil, agentic.ResumeNew, "", []string{}, ""},
		{"wrapper-latest", []string{"resume"}, nil, agentic.ResumeLatest, "", []string{}, ""},
		{"wrapper-handle", []string{"resume", "SES-1"}, nil, agentic.ResumeHandle, "SES-1", []string{}, ""},
		{"wrapper-uuid", []string{"--resume", uuid}, nil, agentic.ResumeClaudeUUID, uuid, []string{}, ""},
		{"tail-uuid", nil, []string{"--resume", uuid, "--model", "m"}, agentic.ResumeClaudeUUID, uuid, []string{"--model", "m"}, ""},
		{"tail-short", nil, []string{"-r", uuid}, agentic.ResumeClaudeUUID, uuid, []string{}, ""},
		{"tail-equals", nil, []string{"--resume=" + uuid}, agentic.ResumeClaudeUUID, uuid, []string{}, ""},
		{"tail-continue", nil, []string{"--continue"}, agentic.ResumeLatest, "", []string{}, ""},
		{"tail-keeps-order", nil, []string{"--model", "m", "--resume", uuid, "--", "prompt"}, agentic.ResumeClaudeUUID, uuid, []string{"--model", "m", "--", "prompt"}, ""},
		{"post-separator-not-a-selector", nil, []string{"--", "--resume", uuid}, agentic.ResumeNew, "", []string{"--", "--resume", uuid}, ""},
		{"wrapper-tail-conflict", []string{"resume"}, []string{"--resume", uuid}, "", "", nil, "conflicting"},
		{"tail-duplicate-conflict", nil, []string{"--resume", uuid, "--resume", other}, "", "", nil, "conflicting"},
		{"tail-duplicate-identical-conflict", nil, []string{"--resume", uuid, "--resume", uuid}, "", "", nil, "conflicting"},
		{"wrapper-bad-handle", []string{"resume", "nope"}, nil, "", "", nil, "invalid"},
		{"wrapper-bad-uuid", []string{"--resume", "NOT-A-UUID"}, nil, "", "", nil, "invalid"},
		{"tail-session-id-ambiguous", nil, []string{"--session-id", "x"}, "", "", nil, "ambiguous"},
		{"tail-fork-ambiguous", nil, []string{"--fork-session"}, "", "", nil, "ambiguous"},
		{"tail-unknown-option", nil, []string{"--max-turns", "5"}, "", "", nil, "unknown option"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := hosted.ElevateResume(claudeRegistry(t), "claude-code", tc.wrapper, tc.native)
			if tc.wantErrSubstr != "" {
				resumeErr, ok := err.(*hosted.ResumeError)
				if !ok {
					t.Fatalf("error type %T, want *hosted.ResumeError", err)
				}
				if !strings.Contains(resumeErr.Error(), "session_resume_invalid: ") || !strings.Contains(resumeErr.Error(), tc.wantErrSubstr) {
					t.Fatalf("error = %q, want session_resume_invalid containing %q", resumeErr.Error(), tc.wantErrSubstr)
				}
				for _, secret := range []string{uuid, other} {
					if strings.Contains(resumeErr.Error(), secret) {
						t.Fatalf("error %q carries the selector identity", resumeErr.Error())
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Intent.Kind != tc.wantKind {
				t.Fatalf("kind = %q, want %q", got.Intent.Kind, tc.wantKind)
			}
			if tc.wantID == "" {
				if got.Intent.Identity != nil {
					t.Fatalf("identity = %q, want nil", *got.Intent.Identity)
				}
			} else if got.Intent.Identity == nil || *got.Intent.Identity != tc.wantID {
				t.Fatalf("identity = %v, want %q", got.Intent.Identity, tc.wantID)
			}
			if !reflect.DeepEqual(got.NativeArgs, tc.wantTail) {
				t.Fatalf("tail = %q, want %q", got.NativeArgs, tc.wantTail)
			}
		})
	}
}

// TestElevateResumeRegistryFailures proves wiring faults refuse as resume
// errors without reaching any provider grammar.
func TestElevateResumeRegistryFailures(t *testing.T) {
	if _, err := hosted.ElevateResume(nil, "claude-code", nil, nil); err == nil {
		t.Fatal("nil registry elevated")
	} else if _, ok := err.(*hosted.ResumeError); !ok {
		t.Fatalf("error type %T", err)
	}
	empty := agentic.NewRegistry()
	if _, err := hosted.ElevateResume(empty, "claude-code", nil, nil); err == nil {
		t.Fatal("unregistered system elevated")
	} else if _, ok := err.(*hosted.ResumeError); !ok {
		t.Fatalf("error type %T", err)
	}
}

// TestExportRestart proves the closed template export over the composed
// argv: the template carries the argv verbatim with a --resume slot inside
// it, and the module's own transformation check accepts both the new
// process shape and the one-insertion restart shape.
func TestExportRestart(t *testing.T) {
	reg := claudeRegistry(t)
	argv := []string{"--model", "m", "--disallowedTools=AskUserQuestion"}
	template, err := hosted.ExportRestart(reg, "claude-code", argv)
	if err != nil {
		t.Fatal(err)
	}
	if template.Schema != claudeSystem.RestartSchema || template.SchemaVersion != claudeSystem.RestartSchemaVersion {
		t.Fatalf("envelope = %s %s", template.Schema, template.SchemaVersion)
	}
	if !reflect.DeepEqual(template.Data.Argv, argv) {
		t.Fatalf("template argv = %q, want %q", template.Data.Argv, argv)
	}
	slot := template.Data.IdentitySlot
	if slot.Flag != "--resume" || slot.Index < 0 || slot.Index > len(argv) {
		t.Fatalf("slot = %+v", slot)
	}
	system, _ := reg.Lookup("claude-code")
	exporter, ok := system.(interface {
		ValidateRestartTransformation(claudeSystem.RestartTemplate, []string, *string) error
	})
	if !ok {
		t.Fatal("registered claude system validates no restart transformation")
	}
	if err := exporter.ValidateRestartTransformation(template, argv, nil); err != nil {
		t.Fatalf("new-process transformation rejected: %v", err)
	}
	id := "00010203-0405-4607-8809-0a0b0c0d0e0f"
	restarted := append(append(append([]string{}, argv[:slot.Index]...), "--resume", id), argv[slot.Index:]...)
	if err := exporter.ValidateRestartTransformation(template, restarted, &id); err != nil {
		t.Fatalf("one-insertion transformation rejected: %v", err)
	}
	if err := exporter.ValidateRestartTransformation(template, append(argv, "--extra"), nil); err == nil {
		t.Fatal("mutated argv accepted by the transformation check")
	}
}

// TestExportRestartFailures proves unexportable inputs refuse launch_plan_invalid.
func TestExportRestartFailures(t *testing.T) {
	reg := claudeRegistry(t)
	if _, err := hosted.ExportRestart(nil, "claude-code", []string{"--model", "m"}); err == nil {
		t.Fatal("nil registry exported")
	}
	if _, err := hosted.ExportRestart(agentic.NewRegistry(), "claude-code", []string{"--model", "m"}); err == nil {
		t.Fatal("unregistered system exported")
	}
	codexReg := agentic.NewRegistry()
	if err := codexReg.Register(codexSystem.New()); err != nil {
		t.Fatal(err)
	}
	if _, err := hosted.ExportRestart(codexReg, "codex", []string{"-m", "m"}); err == nil {
		t.Fatal("non-exporter system exported")
	} else if planErr, ok := err.(*hosted.PlanError); !ok || planErr.Code != "launch_plan_invalid" {
		t.Fatalf("error = %#v", err)
	}
	if _, err := hosted.ExportRestart(reg, "claude-code", []string{"--max-turns", "5"}); err == nil {
		t.Fatal("unknown-option argv exported")
	} else if planErr, ok := err.(*hosted.PlanError); !ok || planErr.Code != "launch_plan_invalid" {
		t.Fatalf("error = %#v", err)
	}
}
