package defaults

import (
	"errors"
	"fmt"
	"runtime/debug"

	"github.com/relux-works/curator-agent-launcher/internal/mapping"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/plugin"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

// SelectionCandidate is an exact admitted tuple from the existing runtime and
// vendor declarations. An effort-less row maps the module's empty effort to JSON null.
type SelectionCandidate struct {
	Runtime string  `json:"runtime"`
	Model   string  `json:"model"`
	Effort  *string `json:"effort"`
}

const OriginSelector Origin = "selector"

func AdmissionSource() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "github.com/relux-works/skill-agents-management" {
				return "agents-management " + dep.Version + " lineup"
			}
		}
	}
	// Test executables may omit dependency build information. This fallback is
	// pinned to go.mod by TestSelectionCandidates.
	return "agents-management v0.5.74 lineup"
}
func SelectionCandidates(env string, reg *vendorplugin.Registry) ([]SelectionCandidate, error) {
	return SelectionCandidatesFor(env, reg, "")
}

// SelectionCandidatesFor is SelectionCandidates for a launch whose explicit
// model flag names explicitModel. Ranked rows offer their effective effort
// vocabulary (module words plus operator-catalog additions). An
// operator-declared catalog row is never ranked, so it joins the candidates
// only when the explicit flag names it: an unconfigured selector can never
// auto-select a catalog row, and an explicitly chosen one stays admissible.
func SelectionCandidatesFor(env string, reg *vendorplugin.Registry, explicitModel string) ([]SelectionCandidate, error) {
	target, err := mapping.Resolve(env)
	if err != nil {
		return nil, err
	}
	if reg == nil {
		return nil, &Error{CodeUnresolvable, fmt.Errorf("no compatibility registry completes %s defaults", env)}
	}
	cands, err := systemCandidates(target.System, reg)
	if err != nil || len(cands) == 0 {
		return nil, &Error{CodeUnresolvable, fmt.Errorf("the lineup admits no model for environment %q", env)}
	}
	// Preserve the module lineup ranking within each contributing runtime; scores
	// across runtimes never establish a preferred runtime.
	out := []SelectionCandidate{}
	for _, decl := range reg.RuntimeDeclarations() {
		var models []vendorplugin.Model
		for _, cand := range cands {
			if cand.runtime == decl.ID {
				models = append(models, cand.model)
			}
		}
		if len(models) == 0 {
			continue
		}
		system, vendor, err := selectionSystem(reg, decl)
		if err != nil {
			return nil, &Error{CodeUnresolvable, err}
		}
		rows := []vendorplugin.Model{}
		for _, ranked := range vendorplugin.Lineup(models) {
			rows = append(rows, ranked.Model)
		}
		for _, model := range models {
			if model.OperatorDeclared && explicitModel != "" && string(model.ID) == explicitModel {
				rows = append(rows, model)
			}
		}
		for _, row := range rows {
			vocabulary := append(append([]string(nil), row.Effort.Vocabulary...), row.Effort.OperatorAccepts...)
			efforts := vocabulary
			if row.Effort.Support == agentic.EffortSupportNone {
				efforts = []string{""}
			}
			for _, effort := range efforts {
				// Reuse BuildLaunch's pure capability gate; do not infer native effort
				// support from the vendor vocabulary (Pi admits only a subset).
				if admitter, ok := system.(agentic.EffortAdmitter); ok {
					if _, err := admitter.AdmitEffort(vendor, row.Launchable().LaunchIdentity(), effort, vocabulary); err != nil {
						continue
					}
				}
				var axis *string
				if effort != "" {
					axis = new(string)
					*axis = effort
				}
				out = append(out, SelectionCandidate{string(decl.ID), string(row.ID), axis})
			}
		}
	}
	if len(out) == 0 {
		return nil, &Error{CodeUnresolvable, fmt.Errorf("the lineup admits no model/effort tuple for %s", env)}
	}
	return out, nil
}

// CompleteWithSelection preserves explicit flag and lock validation, then puts
// a selector pair immediately above file defaults. Runtime is deliberately not
// supplied: the existing model-to-runtime binding remains authoritative.
func (f Files) CompleteWithSelection(environment string, flags Pair, selected Pair, reg *vendorplugin.Registry) (Resolved, error) {
	target, err := mapping.Resolve(environment)
	if err != nil {
		return Resolved{}, err
	}
	partial, err := f.Resolve(environment, flags)
	if err != nil {
		return Resolved{}, err
	}
	if !flags.Model.Present && selected.Model.Present {
		partial.Model = ResolvedMember{selected.Model, OriginSelector}
	}
	if !flags.Effort.Present && selected.Effort.Present {
		partial.Effort = ResolvedMember{selected.Effort, OriginSelector}
	}
	out, err := complete(environment, target, partial, reg)
	if out.Effort.Origin == OriginSelector && out.Effort.Value == "" {
		out.Effort = ResolvedMember{}
	}
	return out, err
}

// Equal compares null and non-null effort axes by value, never pointer identity.
func (c SelectionCandidate) Equal(other SelectionCandidate) bool {
	if c.Runtime != other.Runtime || c.Model != other.Model {
		return false
	}
	if c.Effort == nil || other.Effort == nil {
		return c.Effort == nil && other.Effort == nil
	}
	return *c.Effort == *other.Effort
}

// Materialize the same system capability as BuildLaunch. Its declaration-owned
// fallback has no vendor binding; the registry's own graph still owns the
// system. No alternate registry or identifier table is introduced here.
func selectionSystem(reg *vendorplugin.Registry, decl vendorplugin.RuntimeDeclaration) (agentic.System, string, error) {
	runtime, err := reg.ResolveRuntime(decl.ID)
	if err == nil {
		return runtime.System, string(runtime.VendorID), nil
	}
	if errors.Is(err, vendorplugin.ErrRuntimeVendorUnresolved) && !decl.VendorResolved() && len(decl.Models) > 0 {
		node, ok := reg.Graph().Lookup(plugin.ID(decl.System))
		if ok {
			if source, ok := node.(interface{ System() agentic.System }); ok {
				return source.System(), "", nil
			}
		}
	}
	return nil, "", fmt.Errorf("cannot materialize the admission capability for runtime %s", decl.ID)
}
