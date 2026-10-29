package defaults_test

import (
	"errors"
	"testing"

	"github.com/relux-works/skill-agents-management/pkg/agentic"
	codexSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/codex"
	"github.com/relux-works/skill-agents-management/pkg/runtimecatalog"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"

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

// catalogOnlyVendor declares a single operator-declared row, so the codex
// system's candidate set is non-empty while its ranked lineup is empty.
type catalogOnlyVendor struct{}

func (catalogOnlyVendor) ID() vendorplugin.VendorID { return "openai" }
func (catalogOnlyVendor) Models() []vendorplugin.Model {
	return []vendorplugin.Model{{
		ID: "gpt-catalog-only", Description: "operator-declared test row", Lifecycle: vendorplugin.LifecycleCurrent,
		Rank:             vendorplugin.CapabilityRank{Score: 1, Basis: []vendorplugin.RankEvidence{{Source: "test", Observation: "operator-declared"}}},
		Effort:           vendorplugin.EffortDeclaration{Support: agentic.EffortSupportNone},
		Systems:          []agentic.SystemID{"codex"},
		OperatorDeclared: true,
	}}
}
func (catalogOnlyVendor) Availability(vendorplugin.AvailabilityQuery) (vendorplugin.Availability, error) {
	return vendorplugin.Unchecked(), nil
}
func (catalogOnlyVendor) Spawn(sc vendorplugin.SpawnContext) (agentic.LaunchRequest, error) {
	return vendorplugin.PassthroughLaunch(sc), nil
}

// A system whose only candidates are catalog rows has no ranked lineup: an
// unconfigured completion refuses defaults_unresolvable instead of indexing an
// empty lineup, while explicit selection of that same row resolves.
func TestCatalogOnlyCandidatesNeverAutoSelect(t *testing.T) {
	systems := agentic.NewRegistry()
	if err := systems.Register(codexSystem.New()); err != nil {
		t.Fatal(err)
	}
	reg := vendorplugin.NewRegistry(systems)
	if err := reg.DeclareRuntime(vendorplugin.RuntimeDeclaration{ID: "codex", System: "codex", Vendor: "openai",
		Broker: vendorplugin.BrokerProvenance{Checked: []string{"test"}, Found: "test binds codex to openai"}}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(catalogOnlyVendor{}); err != nil {
		t.Fatal(err)
	}
	files, err := defaults.Load(paths(t))
	if err != nil {
		t.Fatal(err)
	}
	_, err = files.Complete("codex_cli", defaults.Pair{}, reg)
	var refusal *defaults.Error
	if !errors.As(err, &refusal) || refusal.Code != defaults.CodeUnresolvable {
		t.Fatalf("unconfigured completion over catalog-only rows: %v, want defaults_unresolvable", err)
	}
	got, err := files.Complete("codex_cli", defaults.Pair{Model: member("gpt-catalog-only")}, reg)
	if err != nil || got.Runtime != "codex" || got.Model.Value != "gpt-catalog-only" {
		t.Fatalf("explicit catalog row: %+v %v", got, err)
	}
}

// Selector candidates use effective admission: a catalog effort word on a
// module row is offered, and a catalog model is offered only when the explicit
// model flag names it, so an unconfigured selector never sees it.
func TestSelectionCandidatesFollowEffectiveAdmission(t *testing.T) {
	catalog, err := runtimecatalog.ParseModelCatalog([]byte(`schema_version = 1
[[effort_scale]]
word  = "deepest"
after = "max"
[vendors.openai.models."gpt-6-9-test"]
systems = ["codex"]
effort  = { support = "required", accepts = ["low", "deepest"], recommended = "low" }
[vendors.openai.models."gpt-6-astra"]
effort = { accepts = ["deepest"] }
`))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := defaults.NewRegistryWithCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	has := func(cands []defaults.SelectionCandidate, model, effort string) bool {
		for _, c := range cands {
			if c.Model == model && c.Effort != nil && *c.Effort == effort {
				return true
			}
		}
		return false
	}
	plain, err := defaults.SelectionCandidates("codex_cli", reg)
	if err != nil {
		t.Fatal(err)
	}
	if !has(plain, "gpt-6-astra", "deepest") {
		t.Fatal("a catalog effort word on a module row is not offered")
	}
	if has(plain, "gpt-6-9-test", "low") {
		t.Fatal("an unconfigured selector is offered a catalog model")
	}
	explicit, err := defaults.SelectionCandidatesFor("codex_cli", reg, "gpt-6-9-test")
	if err != nil {
		t.Fatal(err)
	}
	if !has(explicit, "gpt-6-9-test", "deepest") || !has(explicit, "gpt-6-9-test", "low") {
		t.Fatal("an explicitly named catalog model is not offered with its words")
	}
}
