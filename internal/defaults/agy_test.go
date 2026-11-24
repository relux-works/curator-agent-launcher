package defaults_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/defaults"
)

func antigravityEffortlessModel(t *testing.T) string {
	t.Helper()
	rows, err := defaults.SelectionCandidates("antigravity", mustRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Runtime == "agy" && row.Effort == nil && row.Model != "" {
			return row.Model
		}
	}
	t.Fatal("the pinned agy catalog has no model row with effort encoded in its model id")
	return ""
}

func TestAntigravityConfiguredModelKeepsEffortEmpty(t *testing.T) {
	model := antigravityEffortlessModel(t)
	p := paths(t)
	write(t, p.Operator, doc(fmt.Sprintf(`{"antigravity":{"model":%q}}`, model), false))
	files, err := defaults.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := files.Complete("antigravity", defaults.Pair{}, mustRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.Runtime != "agy" || got.Model != resolved(model, defaults.OriginOperator) || got.Effort.Present || got.Effort.Value != "" {
		t.Fatalf("antigravity defaults = %+v; want the agy model with empty effort", got)
	}
}

func TestAntigravityEffortRefusedFromFlagAndDefaults(t *testing.T) {
	model := antigravityEffortlessModel(t)
	t.Run("flag", func(t *testing.T) {
		files, err := defaults.Load(paths(t))
		if err != nil {
			t.Fatal(err)
		}
		got, err := files.Complete("antigravity", defaults.Pair{Model: member(model), Effort: member("high")}, mustRegistry(t))
		code(t, err, "effort_unsupported")
		if got != (defaults.Resolved{}) || !strings.Contains(err.Error(), "flag") {
			t.Fatalf("flag effort refusal = %+v, %v; want no resolution and flag source", got, err)
		}
	})
	t.Run("defaults", func(t *testing.T) {
		p := paths(t)
		write(t, p.Operator, doc(fmt.Sprintf(`{"antigravity":{"model":%q,"effort":"high"}}`, model), false))
		files, err := defaults.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := files.Complete("antigravity", defaults.Pair{}, mustRegistry(t))
		code(t, err, "effort_unsupported")
		if got != (defaults.Resolved{}) || !strings.Contains(err.Error(), "defaults") {
			t.Fatalf("defaults effort refusal = %+v, %v; want no resolution and defaults source", got, err)
		}
	})
}

func TestUnknownAntigravityModelRefusesBeforeResolution(t *testing.T) {
	files, err := defaults.Load(paths(t))
	if err != nil {
		t.Fatal(err)
	}
	got, err := files.Complete("antigravity", defaults.Pair{Model: member("unknown-model-for-antigravity-test")}, mustRegistry(t))
	code(t, err, defaults.CodeUnresolvable)
	if got != (defaults.Resolved{}) || !strings.Contains(err.Error(), "unknown-model-for-antigravity-test") {
		t.Fatalf("unknown agy model refusal = %+v, %v", got, err)
	}
}
