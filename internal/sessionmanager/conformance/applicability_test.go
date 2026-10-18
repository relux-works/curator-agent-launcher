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

// A truthfully smaller capability set qualifies through the very same driver.
// No-resume exercises the review witness; launch-only also removes stop, attach,
// status, list, labels and admission to catch implicit lifecycle prerequisites.
func TestAdvertisedSubsets(t *testing.T) {
	if *plugin != "" {
		t.Skip("reference configuration control")
	}
	binary := buildFake(t)
	for _, kind := range []string{"no-resume", "launch-only", "runtime-no-resume"} {
		t.Run(kind, func(t *testing.T) {
			cfg := fakemanager.DefaultConfig()
			switch kind {
			case "no-resume":
				cfg.Capabilities = slices.DeleteFunc(cfg.Capabilities, func(v string) bool { return v == "resume/1" })
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
