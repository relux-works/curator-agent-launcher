package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	agy "github.com/relux-works/skill-agents-management/pkg/agentic/systems/agy"
	claude "github.com/relux-works/skill-agents-management/pkg/agentic/systems/claude"
	codex "github.com/relux-works/skill-agents-management/pkg/agentic/systems/codex"
	gemini "github.com/relux-works/skill-agents-management/pkg/agentic/systems/gemini"
	pinative "github.com/relux-works/skill-agents-management/pkg/agentic/systems/pinative"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin/vendors/anthropic"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin/vendors/google"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin/vendors/openai"
)

func TestRunDefaultsPerMember(t *testing.T) {
	for _, tc := range []struct {
		name, env, machine, operator string
		flags                        []string
		want                         string
	}{
		{"own-row-recommendation", "claude_code", `{"model":"claude-opus-4-8"}`, "", nil, "model=claude-opus-4-8 (machine) effort=xhigh (lineup)"},
		{"no-effort", "claude_code", "", `{"model":"claude-haiku-4-5"}`, nil, "model=claude-haiku-4-5 (operator) effort unset"},
		{"pi-google-override", "pi", "", `{"model":"gemini-3.1-pro-preview"}`, nil, "model=gemini-3.1-pro-preview (operator) effort unset"},
		{"pi-openai-override", "pi", `{"model":"gpt-5.6-sol"}`, "", nil, "model=gpt-5.6-sol (machine) effort=max (lineup)"},
		{"flag-model-machine-effort", "codex_cli", `{"effort":"low"}`, "", []string{"--model", "gpt-6-astra"}, "model=gpt-6-astra (flag) effort=low (machine)"},
		{"operator-model-machine-effort", "codex_cli", `{"model":"gpt-5.6-sol","effort":"low"}`, `{"model":"gpt-6-astra"}`, nil, "model=gpt-6-astra (operator) effort=low (machine)"},
		{"flag-effort", "pi", "", "", []string{"--effort", "unsupported-word"}, "model=claude-fable-5 (lineup) effort=unsupported-word (flag)"},
		{"explicit-empty", "pi", "", `{"model":"","effort":""}`, nil, "model= (operator) effort= (operator)"},
		{"unknown-not-replaced", "pi", "", "", []string{"--model", "unknown"}, "model=unknown (flag) effort unset"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sr := &scriptedRunner{stdout: fragmentLineFor(tc.env)}
			deps := testDeps(t, fragment.NewWithRunner("curator", sr))
			for path, row := range map[string]string{deps.defaults.Machine: tc.machine, deps.defaults.Operator: tc.operator} {
				if row != "" {
					if err := os.WriteFile(path, []byte(fmt.Sprintf(`{"schema":"curator-run-defaults-v1","defaults":{%q:%s}}`, tc.env, row)), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			var out, stderr strings.Builder
			code := run(context.Background(), append([]string{tc.env}, tc.flags...), &out, &stderr, deps)
			if code != 1 || out.Len() != 0 || sr.calls != 1 || !strings.HasPrefix(stderr.String(), "curator-run: defaults: "+tc.want+"\ncurator-run: not_implemented: ") {
				t.Fatalf("exit=%d calls=%d stdout=%q stderr=%q", code, sr.calls, out.String(), stderr.String())
			}
		})
	}
}

// Vendor fixture retains the real rows and contract, but removes only native-Pi
// compatibility to exercise a declared runtime with no driven row.
type noPiVendor struct{ vendorplugin.Vendor }

func (v noPiVendor) Models() []vendorplugin.Model {
	rows := v.Vendor.Models()
	for i := range rows {
		var systems []agentic.SystemID
		for _, s := range rows[i].Systems {
			if s != "pi-native" {
				systems = append(systems, s)
			}
		}
		rows[i].Systems = systems
	}
	return rows
}

func piRegistry(t *testing.T, ids []string, emptyAnthropic bool) *vendorplugin.Registry {
	t.Helper()
	systems := agentic.NewRegistry()
	for _, s := range []agentic.System{claude.New(), codex.New(), pinative.New(), gemini.New(), agy.New()} {
		if err := systems.Register(s); err != nil {
			t.Fatal(err)
		}
	}
	reg := vendorplugin.NewRegistry(systems)
	full, err := defaults.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		for _, decl := range full.RuntimeDeclarations() {
			if string(decl.ID) == id || (id == "pi-other" && decl.ID == "pi-anthropic") {
				decl.ID = vendorplugin.RuntimeID(id)
				if err := reg.DeclareRuntime(decl); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	var av vendorplugin.Vendor = anthropic.New()
	if emptyAnthropic {
		av = noPiVendor{av}
	}
	for _, v := range []vendorplugin.Vendor{av, openai.New(), google.New()} {
		if err := reg.Register(v); err != nil {
			t.Fatal(err)
		}
	}
	return reg
}

func TestRunPiRuntimePreference(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ids   []string
		empty bool
		want  string
	}{
		{"ordered-not-declaration-or-score", []string{"pi-google", "pi-openai", "pi-anthropic"}, false, "model=claude-fable-5 (lineup) effort=high (lineup)"},
		{"missing-first", []string{"pi-google", "pi-openai"}, false, "model=gpt-5.6-sol (lineup) effort=max (lineup)"},
		{"first-with-no-driven-rows", []string{"pi-anthropic", "pi-openai", "pi-google"}, true, "model=gpt-5.6-sol (lineup) effort=max (lineup)"},
		{"third", []string{"pi-google"}, false, "model=gemini-3.1-pro-preview (lineup) effort unset"},
		{"no-preferred-driven-row", []string{"pi-anthropic"}, true, "defaults_unresolvable:"},
		{"unpreferred-only", []string{"pi-other"}, false, "defaults_unresolvable:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sr := &scriptedRunner{stdout: piFragmentLine}
			deps := testDeps(t, fragment.NewWithRunner("curator", sr))
			deps.registry = piRegistry(t, tc.ids, tc.empty)
			var out, stderr strings.Builder
			code := run(context.Background(), []string{"pi"}, &out, &stderr, deps)
			if code != 1 || sr.calls != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("exit=%d stderr=%q", code, stderr.String())
			}
			if strings.Contains(tc.want, "unresolvable") {
				if !strings.Contains(stderr.String(), `no preferred Pi runtime (pi-anthropic pi-openai pi-google) declares a driven model`) {
					t.Fatal(stderr.String())
				}
				if strings.Contains(stderr.String(), "not_implemented") || strings.Contains(stderr.String(), "curator-run: defaults:") {
					t.Fatal(stderr.String())
				}
			} else if !strings.HasPrefix(stderr.String(), "curator-run: defaults: "+tc.want+"\ncurator-run: not_implemented:") {
				t.Fatal(stderr.String())
			}
		})
	}
}

func TestRunDefaultsFailuresStopBeforeGroup(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		flags      []string
		code       int
		detail     string
	}{
		{"effort-lock", `{"schema":"curator-run-defaults-v1","locked":true,"defaults":{"pi":{"effort":"high"}}}`, []string{"--effort", "low"}, 2, "effort"},
		{"model-lock", `{"schema":"curator-run-defaults-v1","locked":true,"defaults":{"pi":{"model":"claude-opus-5"}}}`, []string{"--model", "claude-fable-5"}, 2, "model"},
		{"unknown-member", `{"schema":"curator-run-defaults-v1","defaults":{},"extra":true}`, nil, 1, "unknown member"},
		{"broken-link", "", nil, 1, "symlink"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sr := &scriptedRunner{stdout: piFragmentLine}
			deps := testDeps(t, fragment.NewWithRunner("curator", sr))
			if tc.body != "" {
				if err := os.WriteFile(deps.defaults.Machine, []byte(tc.body), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Symlink(deps.defaults.Machine+"-absent", deps.defaults.Machine); err != nil {
					t.Fatal(err)
				}
			}
			var out, stderr strings.Builder
			code := run(context.Background(), append([]string{"pi"}, tc.flags...), &out, &stderr, deps)
			if code != tc.code || sr.calls != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), tc.detail) || strings.Contains(stderr.String(), "not_implemented") || strings.Contains(stderr.String(), "curator-run: defaults:") {
				t.Fatalf("exit=%d stderr=%q", code, stderr.String())
			}
		})
	}
}

func TestRunDefaultsPathOrdering(t *testing.T) {
	for _, tc := range []struct {
		name            string
		args            []string
		env             string
		pathCalls, exit int
	}{
		{"help", []string{"--help"}, "pi", 0, 0},
		{"usage", []string{"--unknown"}, "pi", 0, 2},
		{"mapping", []string{"opencode"}, "opencode", 0, 1},
		{"path-failure", []string{"pi"}, "pi", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sr := &scriptedRunner{stdout: fragmentLineFor(tc.env)}
			deps := testDeps(t, fragment.NewWithRunner("curator", sr))
			calls := 0
			deps.configPaths = func() (defaults.Paths, error) { calls++; return defaults.Paths{}, fmt.Errorf("path discovery failed") }
			var out, stderr strings.Builder
			code := run(context.Background(), tc.args, &out, &stderr, deps)
			if code != tc.exit || calls != tc.pathCalls {
				t.Fatalf("exit=%d path calls=%d stderr=%q", code, calls, stderr.String())
			}
			if calls > 0 && (!strings.Contains(stderr.String(), "defaults_config_invalid: cannot locate defaults configuration: path discovery failed") || strings.Contains(stderr.String(), "not_implemented")) {
				t.Fatal(stderr.String())
			}
		})
	}
}
