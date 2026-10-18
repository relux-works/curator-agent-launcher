//go:build unix

package conformance

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/fakemanager"
)

// Every declared operation requires both its capability and runtime cell;
// removing either makes that scenario not_applicable, never a passing exercise.
func TestScenarioPrerequisites(t *testing.T) {
	cfg := fakemanager.DefaultConfig()
	h := &harness{template: map[string]any{"runtime": "codex_cli"}}
	for _, c := range scenarios() {
		t.Run(c.Name, func(t *testing.T) {
			baseline := client.Describe{Capabilities: cfg.Capabilities, Runtimes: cfg.Runtimes}
			if e := c.applicable(h, &baseline); e != nil {
				t.Fatal(e)
			}
			for _, need := range strings.Fields(c.Needs) {
				cap := need
				if need == "doorbell" {
					cap = "doorbell-codex/1"
				}
				if v := operationCapability[need]; v != "" {
					cap = v
				}
				d := baseline
				d.Capabilities = slices.DeleteFunc(slices.Clone(cfg.Capabilities), func(v string) bool { return v == cap })
				var na *notApplicable
				if e := c.applicable(h, &d); !errors.As(e, &na) {
					t.Fatalf("missing %s must be not_applicable: %v", cap, e)
				}
				if operationCapability[need] != "" {
					d = baseline
					d.Runtimes = map[string][]string{"codex_cli": slices.DeleteFunc(slices.Clone(cfg.Runtimes["codex_cli"]), func(v string) bool { return v == need })}
					if e := c.applicable(h, &d); !errors.As(e, &na) {
						t.Fatalf("missing runtime %s must be not_applicable: %v", need, e)
					}
				}
			}
		})
	}
}

// Mandatory launch validation and replay cannot depend on optional lifecycle,
// labels or admission support. Optional operations keep their own refusal gates.
func TestMandatoryLaunchPrerequisites(t *testing.T) {
	h := &harness{template: map[string]any{"runtime": "codex_cli"}}
	launch := client.Describe{Capabilities: []string{"launch-delegated/1"}, Runtimes: map[string][]string{"codex_cli": {"launch"}}}
	mandatory := []string{"missing-capability", "unsupported-runtime", "profile-digest", "secret-child-env", "reserved-child-env", "idempotency", "operation-conflict", "operation-expiry"}
	optional := []string{"label-subset", "filtered-index", "resume-missing", "resume-operation-expiry", "idempotency-list", "generation", "generation-stop"}
	for _, name := range append(slices.Clone(mandatory), optional...) {
		t.Run(name, func(t *testing.T) {
			var found bool
			for _, c := range scenarios() {
				if c.Name != name {
					continue
				}
				found = true
				err := c.applicable(h, &launch)
				if slices.Contains(mandatory, name) {
					if err != nil {
						t.Fatalf("mandatory launch property hidden: %v", err)
					}
				} else {
					var na *notApplicable
					if !errors.As(err, &na) {
						t.Fatalf("absent optional operation must be not_applicable: %v", err)
					}
				}
			}
			if !found {
				t.Fatal("missing contract scenario")
			}
		})
	}
}

// A truthfully smaller capability set qualifies through the very same driver.
// No-resume exercises the review witness; individual omissions catch implicit
// lifecycle prerequisites, and launch-only exercises a minimal launch surface.
func TestAdvertisedSubsets(t *testing.T) {
	if *plugin != "" {
		t.Skip("reference configuration control")
	}
	binary := buildFake(t)
	for _, kind := range []string{"no-resume", "no-stop", "no-list", "no-attach", "no-status", "no-admission", "no-labels", "launch-only", "runtime-no-resume"} {
		t.Run(kind, func(t *testing.T) {
			cfg := fakemanager.DefaultConfig()
			switch kind {
			case "no-resume", "no-stop", "no-list", "no-attach", "no-status", "no-admission", "no-labels":
				missing := strings.TrimPrefix(kind, "no-") + "/1"
				cfg.Capabilities = slices.DeleteFunc(cfg.Capabilities, func(v string) bool { return v == missing })
			case "launch-only":
				cfg.Capabilities = []string{"launch-delegated/1"}
			case "runtime-no-resume":
				for runtime, ops := range cfg.Runtimes {
					cfg.Runtimes[runtime] = slices.DeleteFunc(slices.Clone(ops), func(v string) bool { return v == "resume" })
				}
			}
			runConformance(t, func(t *testing.T) *harness { return newHarnessWithConfig(t, binary, nil, &cfg) })
		})
	}
}

// A restrictive fixture rejects an invented model but admits its own template;
// policy preservation must exercise that template without substituting a model.
func TestResumeTemplatePolicy(t *testing.T) {
	if *plugin != "" {
		t.Skip("reference policy control")
	}
	cfg := fakemanager.DefaultConfig()
	cfg.Models = []string{"fixture-model"}
	h := newHarnessWithConfig(t, buildFake(t), nil, &cfg)
	r := h.launch()
	r["model"] = "original"
	_, e := h.call("launch", r)
	if !client.IsCode(e, "policy_refused") {
		t.Fatalf("restricted model control: %v", e)
	}
	if e = resumePolicyCase(h); e != nil {
		t.Fatal(e)
	}
}
