package defaults_test

import (
	"errors"
	"testing"

	"github.com/relux-works/skill-agents-management/pkg/runtimecatalog"

	"github.com/relux-works/curator-agent-launcher/internal/defaults"
)

const launcherCatalog = `schema_version = 1

[vendors.openai.models."gpt-6-9-test"]
systems = ["codex"]
effort  = { support = "required", accepts = ["low", "high"], recommended = "low" }
`

func catalogRegistry(t *testing.T, text string) error {
	t.Helper()
	catalog, err := runtimecatalog.ParseModelCatalog([]byte(text))
	if err != nil {
		return err
	}
	_, err = defaults.NewRegistryWithCatalog(catalog)
	return err
}

// A catalog model is used when named explicitly (its runtime binds and its own
// recommended effort fills the gap), while the unconfigured lineup tops stay
// exactly what the module ranks: adding a model never changes the default.
func TestCatalogModelIsExplicitOnly(t *testing.T) {
	catalog, err := runtimecatalog.ParseModelCatalog([]byte(launcherCatalog))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := defaults.NewRegistryWithCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	files, err := defaults.Load(paths(t))
	if err != nil {
		t.Fatal(err)
	}
	got, err := files.Complete("codex_cli", defaults.Pair{Model: member("gpt-6-9-test")}, reg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Runtime != "codex" || got.Effort.Value != "low" || got.Effort.Origin != defaults.OriginLineup {
		t.Fatalf("explicit catalog model: runtime %q effort %+v", got.Runtime, got.Effort)
	}
	plain := mustRegistry(t)
	for _, env := range []string{"claude_code", "codex_cli", "pi"} {
		with, err := files.Complete(env, defaults.Pair{}, reg)
		if err != nil {
			t.Fatal(err)
		}
		without, err := files.Complete(env, defaults.Pair{}, plain)
		if err != nil {
			t.Fatal(err)
		}
		if with.Model != without.Model || with.Effort != without.Effort || with.Runtime != without.Runtime {
			t.Fatalf("%s: lineup top changed with the catalog: %+v, without %+v", env, with, without)
		}
	}
}

// A catalog the module refuses fails the registry with its typed kind, so no
// launch runs on a partially applied catalog; the valid catalog is the control.
func TestRefusedCatalogFailsTheRegistry(t *testing.T) {
	if err := catalogRegistry(t, launcherCatalog); err != nil {
		t.Fatalf("valid catalog: %v", err)
	}
	for name, text := range map[string]string{
		"unsupported effort word": `schema_version = 1
[vendors.openai.models.x]
systems = ["codex"]
effort  = { support = "required", accepts = ["hgih"], recommended = "hgih" }
`,
		"unknown vendor": `schema_version = 1
[vendors.nobody.models.x]
systems = ["codex"]
effort  = { support = "none" }
`,
		"newer schema": "schema_version = 2\n",
	} {
		err := catalogRegistry(t, text)
		var refusal *runtimecatalog.Refusal
		if !errors.As(err, &refusal) {
			t.Fatalf("%s: err = %v, want a typed runtimecatalog refusal", name, err)
		}
	}
}
