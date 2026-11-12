package defaults_test

import (
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/defaults"
)

// Exact admitted identifiers keep their model, effort and unique runtime.
// An unknown explicit identifier refuses at the model/defaults boundary with
// or without an explicit effort, rather than forwarding an empty runtime.
func TestExplicitCodexModelRuntimeBoundary(t *testing.T) {
	files, err := defaults.Load(paths(t))
	if err != nil {
		t.Fatal(err)
	}
	reg := mustRegistry(t)
	for _, model := range []string{"gpt-6-luna", "gpt-6.1-sol", "luna", "sol"} {
		t.Run(model, func(t *testing.T) {
			got, err := files.Complete("codex_cli", defaults.Pair{Model: member(model), Effort: member("max")}, reg)
			if err != nil || got.Runtime != "codex" || got.Model != resolved(model, defaults.OriginFlag) || got.Effort != resolved("max", defaults.OriginFlag) {
				t.Fatalf("exact model binding: %+v, %v", got, err)
			}
		})
	}
	for _, explicitEffort := range []bool{false, true} {
		name := "model-only"
		pair := defaults.Pair{Model: member("gpt-6.1-luna")}
		if explicitEffort {
			name, pair.Effort = "model-and-effort", member("max")
		}
		t.Run(name, func(t *testing.T) {
			got, err := files.Complete("codex_cli", pair, reg)
			code(t, err, defaults.CodeUnresolvable)
			if got != (defaults.Resolved{}) || !strings.Contains(err.Error(), `"gpt-6.1-luna"`) || !strings.Contains(err.Error(), `"codex_cli"`) {
				t.Fatalf("unknown model refusal: %+v, %v", got, err)
			}
		})
	}
}
