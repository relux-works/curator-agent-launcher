//go:build !windows

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/relux-works/skill-agents-management/pkg/runtimecatalog"

	"github.com/relux-works/curator-agent-launcher/internal/defaults"
)

const selectorCatalog = `schema_version = 1
[[effort_scale]]
word  = "deepest"
after = "max"
[vendors.openai.models."gpt-6-9-test"]
systems = ["codex"]
effort  = { support = "required", accepts = ["low", "deepest"], recommended = "low" }
[vendors.openai.models."gpt-6-astra"]
effort = { accepts = ["deepest"] }
`

func catalogFixtureRegistry(t *testing.T, f *pipelineFixture) {
	t.Helper()
	catalog, err := runtimecatalog.ParseModelCatalog([]byte(selectorCatalog))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := defaults.NewRegistryWithCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	f.deps.registry = reg
}

// An enforcing selector may return an explicitly flagged catalog model with a
// catalog effort word, and a module model with a catalog effort word; both
// reach the plan exactly. A catalog model returned without that explicit flag
// is outside the candidate set and refuses before any plan, so an unconfigured
// selector can never auto-select a catalog row.
func TestRunSelectorEnforceWithCatalog(t *testing.T) {
	binary := selectorBinary(t)
	response := func(model, effort string) string {
		return strings.NewReplacer("gpt-5.6-sol", model, `"effort":"high"`, `"effort":"`+effort+`"`).Replace(selectedResponse)
	}
	for _, tc := range []struct {
		name, model, effort string
		flags               []string
	}{
		{"explicit-catalog-model", "gpt-6-9-test", "deepest", []string{"--model", "gpt-6-9-test", "--effort", "deepest"}},
		{"module-model-catalog-word", "gpt-6-astra", "deepest", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := entryFixture(t, "codex_cli")
			catalogFixtureRegistry(t, f)
			f.args = append([]string{"codex_cli", "--permissions", "native"}, tc.flags...)
			selectorPolicy(t, f, binary, "enforce", "valid")
			selectorResponse(t, binary, response(tc.model, tc.effort))
			cleanupSelectorFake(t, binary)
			code, _, stderr := f.run()
			if code != 0 || f.builds != 1 || string(f.request.Model) != tc.model || f.request.Effort != tc.effort {
				t.Fatalf("exit %d req %+v stderr %s", code, f.request, stderr)
			}
		})
	}
	t.Run("unflagged-catalog-model", func(t *testing.T) {
		f := entryFixture(t, "codex_cli")
		catalogFixtureRegistry(t, f)
		selectorPolicy(t, f, binary, "enforce", "valid")
		selectorResponse(t, binary, response("gpt-6-9-test", "low"))
		cleanupSelectorFake(t, binary)
		code, _, stderr := f.run()
		if code != 1 || f.builds != 0 || !bytes.Contains(stderr, []byte("selector_unavailable: selector unavailable reason=invalid")) {
			t.Fatalf("exit %d builds %d stderr %s", code, f.builds, stderr)
		}
	})
}
