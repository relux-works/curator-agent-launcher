package defaults_test

import (
	"os"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/mapping"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

// Candidate tuples exactly cover the mapped system's driven model/effort
// declarations, including no-effort rows; foreign systems/runtimes never enter.
// The expected set comes directly from registry declarations, not Complete.
func TestSelectionCandidates(t *testing.T) {
	reg := mustRegistry(t)
	for _, env := range []string{"claude_code", "codex_cli", "pi", "muse"} {
		t.Run(env, func(t *testing.T) {
			cands, err := defaults.SelectionCandidates(env, reg)
			if err != nil {
				t.Fatal(err)
			}
			target, err := mapping.Resolve(env)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]bool{}
			for _, decl := range reg.RuntimeDeclarations() {
				if string(decl.System) != target.System {
					continue
				}
				runtime, e := reg.ResolveRuntime(decl.ID)
				var models []vendorplugin.Model
				if e != nil {
					if decl.VendorResolved() || len(decl.Models) == 0 {
						t.Fatal(e)
					}
					models = decl.Models
				} else {
					models = runtime.Vendor.Models()
				}
				for _, model := range models {
					if !model.DrivenBy(agentic.SystemID(target.System)) {
						continue
					}
					efforts := model.Effort.Vocabulary
					if model.Effort.Support == agentic.EffortSupportNone {
						efforts = []string{""}
					}
					for _, effort := range efforts {
						if admitter, ok := runtime.System.(agentic.EffortAdmitter); ok {
							if _, err := admitter.AdmitEffort(string(runtime.VendorID), model.Launchable().LaunchIdentity(), effort, model.Effort.Vocabulary); err != nil {
								continue
							}
						}
						want[string(decl.ID)+"\x00"+string(model.ID)+"\x00"+effort] = true
					}
				}
			}
			if len(cands) != len(want) || len(cands) == 0 {
				t.Fatalf("candidate count %d != declared %d", len(cands), len(want))
			}
			for _, cand := range cands {
				key := cand.Runtime + "\x00" + cand.Model + "\x00"
				if cand.Effort != nil {
					key += *cand.Effort
				}
				if !want[key] {
					t.Fatalf("foreign/duplicate candidate %+v", cand)
				}
				delete(want, key)
			}
			if len(want) != 0 {
				t.Fatal(want)
			}
		})
	}
	if _, err := defaults.SelectionCandidates("codex_cli", nil); err == nil {
		t.Fatal("nil registry admitted")
	}
	if _, err := defaults.SelectionCandidates("opencode", reg); err == nil {
		t.Fatal("unsupported environment admitted")
	}
	// A pin change must update the diagnostic source's test-build fallback.
	data, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	source := defaults.AdmissionSource()
	if !strings.Contains(string(data), "github.com/relux-works/skill-agents-management "+strings.Fields(source)[1]) {
		t.Fatal("source does not name module pin", source)
	}
}

// Selector members outrank both files, explicit flags outrank the selector,
// and existing locked-flag refusals remain in force. Runtime is derived from
// the applied model rather than a separately supplied selector runtime.
func TestSelectionPrecedence(t *testing.T) {
	p := paths(t)
	write(t, p.Machine, doc(`{"codex_cli":{"model":"gpt-6-astra","effort":"low"}}`, false))
	write(t, p.Operator, doc(`{"codex_cli":{"model":"gpt-5.6-sol","effort":"medium"}}`, false))
	files, err := defaults.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	selected := defaults.Pair{Model: member("gpt-6.1-sol"), Effort: member("high")}
	reg := mustRegistry(t)
	for _, tc := range []struct {
		name          string
		flags         defaults.Pair
		model, effort defaults.ResolvedMember
	}{
		{"selected", defaults.Pair{}, resolved("gpt-6.1-sol", defaults.OriginSelector), resolved("high", defaults.OriginSelector)},
		{"model-flag", defaults.Pair{Model: member("gpt-6-astra")}, resolved("gpt-6-astra", defaults.OriginFlag), resolved("high", defaults.OriginSelector)},
		{"effort-flag", defaults.Pair{Effort: member("low")}, resolved("gpt-6.1-sol", defaults.OriginSelector), resolved("low", defaults.OriginFlag)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, e := files.CompleteWithSelection("codex_cli", tc.flags, selected, reg)
			if e != nil || got.Model != tc.model || got.Effort != tc.effort || got.Runtime != "codex" {
				t.Fatal(got, e)
			}
		})
	}
	write(t, p.Machine, doc(`{"codex_cli":{"model":"gpt-6-astra","effort":"low"}}`, true))
	files, err = defaults.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := files.CompleteWithSelection("codex_cli", defaults.Pair{Model: member("gpt-6-astra")}, selected, reg); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatal("flag bypassed lock", err)
	}
	got, err := files.CompleteWithSelection("codex_cli", defaults.Pair{}, selected, reg)
	if err != nil || got.Model.Origin != defaults.OriginSelector {
		t.Fatal("selector failed above defaults", got, err)
	}
}

// Selecting an effort-less model clears file-supplied effort, while an explicit
// effort flag remains untouched for the spawn plane to admit or refuse.
func TestSelectionNoEffort(t *testing.T) {
	p := paths(t)
	write(t, p.Operator, doc(`{"claude_code":{"effort":"high"}}`, false))
	files, e := defaults.Load(p)
	if e != nil {
		t.Fatal(e)
	}
	selected := defaults.Pair{Model: member("claude-haiku-4-5"), Effort: member("")}
	out, e := files.CompleteWithSelection("claude_code", defaults.Pair{}, selected, mustRegistry(t))
	if e != nil || out.Effort.Present || out.Model.Origin != defaults.OriginSelector {
		t.Fatal(out, e)
	}
	out, e = files.CompleteWithSelection("claude_code", defaults.Pair{Effort: member("high")}, selected, mustRegistry(t))
	if e != nil || out.Effort.Origin != defaults.OriginFlag || out.Effort.Value != "high" {
		t.Fatal(out, e)
	}
}

// Admission includes the harness's existing pure effort gate: minimal is a valid
// vendor word for this model on Codex, but Pi would clamp it. High stays admitted
// in both, so filtering cannot satisfy this test by rejecting the model.
func TestSelectionNativeEffort(t *testing.T) {
	reg := mustRegistry(t)
	for _, tc := range []struct {
		env, runtime string
		minimal      bool
	}{{"pi", "pi-openai", false}, {"codex_cli", "codex", true}} {
		cands, err := defaults.SelectionCandidates(tc.env, reg)
		if err != nil {
			t.Fatal(err)
		}
		high, minimal := false, false
		for _, cand := range cands {
			if cand.Runtime == tc.runtime && cand.Model == "gpt-5.3-codex" && cand.Effort != nil {
				if *cand.Effort == "high" {
					high = true
				}
				if *cand.Effort == "minimal" {
					minimal = true
				}
			}
		}
		if !high || minimal != tc.minimal {
			t.Fatalf("%s admission high=%v minimal=%v, want high and minimal=%v", tc.env, high, minimal, tc.minimal)
		}
	}
}
