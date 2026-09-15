// Lineup fallback for SPEC §4.3 level 3.
//
// Files.Resolve (defaults.go) covers levels 1-2: flags, then the machine
// and operator files with presence, origins, and machine locks. Complete
// below supplies only the members those levels left unset, from the real
// tagged agents-management module: the highest-ranked model of
// vendorplugin.Lineup among the models the registry admits for the mapped
// system, with that model's declared Effort.Recommended as the effort, or
// no effort when its EffortSupport is EffortSupportNone.
//
// The module is the authority for ranking, recommendation, and admission:
// the launcher never invents a model name, never substitutes one row's
// effort for another's, and never retries past a refusal. A configured
// model with no configured effort takes the lineup's effort for THAT
// model; an unknown configured model leaves the effort unset so the spawn
// plane's admission (a later stage) refuses it. When the lineup admits no
// model for the system, the launch fails with defaults_unresolvable.
//
// Native Pi selects the first driven runtime in the SPEC preference before
// ranking that runtime's vendor-local lineup. Configured models instead bind
// their exact contributor across the mapped system's declarations. Scores
// from different vendors are never compared.
//
// Only absence reaches the lineup: Complete propagates a Resolve failure
// (locked flags, unreadable files) without consulting the registry, so
// the fallback never fires past a read failure.
package defaults

import (
	"fmt"
	"io"
	"strings"

	"github.com/relux-works/skill-agents-management/pkg/agentic"
	agySystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/agy"
	claudeSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/claude"
	codexSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/codex"
	geminiSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/gemini"
	pinativeSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/pinative"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin/vendors/anthropic"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin/vendors/google"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin/vendors/openai"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/mapping"
)

// CodeUnresolvable is the SPEC §6 defaults diagnostic for a system the
// lineup admits no model for. It is owned here; diagnostics transcribes it.
const CodeUnresolvable = "defaults_unresolvable"

// OriginLineup marks a member the level-3 fallback supplied.
const OriginLineup Origin = "lineup"

// piRuntimePreference is the SPEC §4.3 ordered convention, not a score ranking.
const piRuntimePreference = "pi-anthropic pi-openai pi-google"

// Resolved is the §4.3 resolved pair plus the module runtime binding the
// lineup completed it against. Runtime is empty exactly when the registry
// admitted no runtime for the mapped system: the pair is then fully
// file-supplied and the plan stage refuses the unknown runtime.
type Resolved struct {
	Model, Effort ResolvedMember
	Runtime       string
}

// NewRegistry builds the production compatibility registry from the real
// tagged module: the claude-code, codex, pi-native, gemini-cli, and
// antigravity system plugins, the frozen runtime declarations, and the
// anthropic, openai, and google vendor plugins.
//
// The legacy pi system plugin is deliberately not registered: it drives
// the agents-infra wrapper, which replaces the managed home, and it backs
// no declared runtime. Mapping never yields the legacy id, so nothing
// here can admit a Pi launch through it; native Pi arrives only through
// the pi-native plugin and the three frozen pi-* runtime rows.
//
// The gemini-cli and antigravity systems are registered only because the
// google vendor's own registration contract requires every agentic system
// its rows mention; the launcher maps nothing to them and launches
// nothing through them.
func NewRegistry() (*vendorplugin.Registry, error) {
	systems := agentic.NewRegistry()
	if err := systems.Register(claudeSystem.New()); err != nil {
		return nil, fmt.Errorf("register claude-code system: %w", err)
	}
	if err := systems.Register(codexSystem.New()); err != nil {
		return nil, fmt.Errorf("register codex system: %w", err)
	}
	if err := systems.Register(pinativeSystem.New()); err != nil {
		return nil, fmt.Errorf("register pi-native system: %w", err)
	}
	if err := systems.Register(geminiSystem.New()); err != nil {
		return nil, fmt.Errorf("register gemini-cli system: %w", err)
	}
	if err := systems.Register(agySystem.New()); err != nil {
		return nil, fmt.Errorf("register antigravity system: %w", err)
	}
	registry := vendorplugin.NewRegistry(systems)
	if err := vendorplugin.SeedFrozenRuntimes(registry); err != nil {
		return nil, fmt.Errorf("seed frozen runtimes: %w", err)
	}
	if err := registry.Register(anthropic.New()); err != nil {
		return nil, fmt.Errorf("register anthropic vendor: %w", err)
	}
	if err := registry.Register(openai.New()); err != nil {
		return nil, fmt.Errorf("register openai vendor: %w", err)
	}
	if err := registry.Register(google.New()); err != nil {
		return nil, fmt.Errorf("register google vendor: %w", err)
	}
	return registry, nil
}

// Complete applies levels 1-2 through Resolve, then level 3 for each
// member left unset. A Resolve error is returned unchanged and the lineup
// is not consulted. An environment mapping.Resolve refuses (for example
// opencode, which has no launchable system/provider pair) is returned
// unchanged: configuration for it never reaches a launch.
func (f Files) Complete(environment string, flags Pair, reg *vendorplugin.Registry) (Resolved, error) {
	target, err := mapping.Resolve(environment)
	if err != nil {
		return Resolved{}, err
	}
	partial, err := f.Resolve(environment, flags)
	if err != nil {
		return Resolved{}, err
	}
	if reg == nil {
		return Resolved{}, &Error{CodeUnresolvable, fmt.Errorf("no compatibility registry completes %s defaults", environment)}
	}
	out := Resolved{Model: partial.Model, Effort: partial.Effort}
	cands, err := systemCandidates(target.System, reg)
	if err != nil {
		return Resolved{}, &Error{CodeUnresolvable, fmt.Errorf("the lineup admits no model for environment %q (system %q): %v", environment, target.System, err)}
	}
	if target.System == "pi-native" && !partial.Model.Present {
		var preferred []candidate
		for _, id := range strings.Fields(piRuntimePreference) {
			for _, cand := range cands {
				if string(cand.runtime) == id {
					preferred = append(preferred, cand)
				}
			}
			if len(preferred) > 0 {
				break
			}
		}
		if len(preferred) == 0 {
			return Resolved{}, &Error{CodeUnresolvable, fmt.Errorf("no preferred Pi runtime (%s) declares a driven model for system %q", piRuntimePreference, target.System)}
		}
		cands = preferred
	}
	if len(cands) == 0 {
		return Resolved{}, &Error{CodeUnresolvable, fmt.Errorf("the lineup admits no model for environment %q (system %q)", environment, target.System)}
	}
	if partial.Model.Present && partial.Effort.Present {
		// Nothing missing: the lineup supplies nothing. Record the
		// binding when exactly one of the system's runtimes drives the
		// configured model; otherwise an empty Runtime leaves the
		// unknown runtime to the plan stage.
		if matches := contributorsFor(cands, partial.Model.Value); len(matches) == 1 {
			out.Runtime = string(matches[0].runtime)
		}
		return out, nil
	}
	if partial.Model.Present {
		// The effort is missing here: a full pair returned above, so
		// this fill cannot overwrite an explicit value.
		matches := contributorsFor(cands, partial.Model.Value)
		if len(matches) > 1 {
			return Resolved{}, &Error{CodeUnresolvable, fmt.Errorf("model %q for environment %q is driven by several runtimes; the launcher binds none", partial.Model.Value, environment)}
		}
		if len(matches) == 1 {
			out.Runtime = string(matches[0].runtime)
			if effort, ok := effortForRow(matches[0].model); ok {
				out.Effort = ResolvedMember{Member: effort, Origin: OriginLineup}
			}
		}
		return out, nil
	}
	models := make([]vendorplugin.Model, 0, len(cands))
	for _, cand := range cands {
		models = append(models, cand.model)
	}
	// Candidates are non-empty here, so the lineup is non-empty too:
	// the only empty gate is the candidate check above, and it stays
	// in exactly that one place.
	top := vendorplugin.Lineup(models)[0].Model
	matches := contributorsFor(cands, string(top.ID))
	if len(matches) != 1 {
		return Resolved{}, &Error{CodeUnresolvable, fmt.Errorf("the lineup top model %q for environment %q is driven by several runtimes; the launcher binds none", string(top.ID), environment)}
	}
	out.Runtime = string(matches[0].runtime)
	out.Model = ResolvedMember{Member: Member{Value: string(top.ID), Present: true}, Origin: OriginLineup}
	if partial.Effort.Present {
		return out, nil
	}
	if effort, ok := effortForRow(top); ok {
		out.Effort = ResolvedMember{Member: effort, Origin: OriginLineup}
	}
	return out, nil
}

// candidate is one vendor row the mapped system can drive, tagged with
// the declared runtime that contributed it.
type candidate struct {
	runtime vendorplugin.RuntimeID
	model   vendorplugin.Model
}

// systemCandidates gathers the driven rows across every runtime declared
// for a mapped system, in the registry's own sorted declaration order.
// Rows for other systems are not candidates: a launch on an undeclared
// harness is a run nobody has evidence works. A declared runtime that
// fails to materialize fails the whole system: a binding the registry
// names but cannot resolve is a failure, never a hint to look elsewhere.
func systemCandidates(system string, reg *vendorplugin.Registry) ([]candidate, error) {
	var cands []candidate
	declared := false
	for _, decl := range reg.RuntimeDeclarations() {
		if string(decl.System) != system {
			continue
		}
		declared = true
		runtime, err := reg.ResolveRuntime(decl.ID)
		if err != nil {
			return nil, err
		}
		for _, model := range runtime.Vendor.Models() {
			if model.DrivenBy(agentic.SystemID(system)) {
				cands = append(cands, candidate{runtime: decl.ID, model: model})
			}
		}
	}
	if !declared {
		return nil, fmt.Errorf("no declared runtime serves the system")
	}
	return cands, nil
}

// contributorsFor looks a model id up among the candidates by exact id.
// Names are never normalized, prefixed, or otherwise derived: a spelling
// no vendor issued matches nothing, and the plan stage refuses it.
func contributorsFor(cands []candidate, id string) []candidate {
	var matches []candidate
	for _, cand := range cands {
		if string(cand.model.ID) == id {
			matches = append(matches, cand)
		}
	}
	return matches
}

// effortForRow renders a row's effort declaration as a member: the
// vendor's Recommended word for a required-effort model, nothing for a
// model with no effort axis. The display Recommended flag on the row is
// never consulted: it steers listings, not launches.
func effortForRow(row vendorplugin.Model) (Member, bool) {
	if row.Effort.Support != agentic.EffortSupportRequired {
		return Member{}, false
	}
	return Member{Value: row.Effort.Recommended, Present: true}, true
}

// Describe renders the resolved pair with the level that produced each
// member, for the stderr origin line-group and the refusal detail. Values
// are folded the way diagnostics.Line frames detail, so an embedded
// newline stays a whitespace-led continuation and never a second line.
func (r Resolved) Describe() string {
	effort := "effort unset"
	if r.Effort.Present {
		effort = "effort=" + foldValue(r.Effort.Value) + " (" + string(r.Effort.Origin) + ")"
	}
	return "model=" + foldValue(r.Model.Value) + " (" + string(r.Model.Origin) + ") " + effort
}

// Line is the stderr origin line-group body: one line naming each
// resolved value and its origin.
func (r Resolved) Line() string {
	return cli.Name + ": defaults: " + r.Describe()
}

// EmitGroup writes the origin line-group to stderr. It runs at every
// launch after resolution and before the plan request, so an operator
// always sees which model is about to run and why.
func EmitGroup(stderr io.Writer, r Resolved) error {
	_, err := fmt.Fprintln(stderr, r.Line())
	return err
}

// foldValue keeps one value to one logical line: the same framing rule
// diagnostics.Line applies to detail, so a hostile value never splits
// the line-group into a second parseable line.
func foldValue(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.ReplaceAll(value, "\n", "\n  ")
}
