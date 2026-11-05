package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
	"github.com/relux-works/curator-agent-launcher/internal/mapping"
	"github.com/relux-works/curator-agent-launcher/internal/selector"
)

// locateOrdinaryConfig shares the frontend's actual location selection without
// reading any policy. Callers select the diagnostic sink; no contents escape.
func locateOrdinaryConfig(deps launchDeps, stderr io.Writer) (configfile.Resolver, error) {
	if deps.configLocations != nil {
		loc, err := deps.configLocations()
		if err != nil {
			return configfile.Resolver{}, err
		}
		return configfile.Resolver{Locations: loc, Stderr: stderr}, nil
	}
	return configfile.Resolver{Locations: configfile.Locations{
		System: filepath.Dir(deps.defaults.Machine), User: filepath.Dir(deps.defaults.Operator),
	}, Stderr: stderr}, nil
}

// loadOrdinaryPolicy retains the frontend's existing policy read order. It
// performs no host selection or profile repair; the selected Core projection
// remains owned by launcherconfig.ReadSelected. No partial result escapes a
// refusal, and the selected-reader's private error detail stays suppressed.
func loadOrdinaryPolicy(ctx context.Context, resolver configfile.Resolver, deps launchDeps) (launchDeps, error) {
	policy, err := launcherconfig.Load(resolver)
	if err != nil {
		return launchDeps{}, err
	}
	deps.launcherConfig = policy
	readSelected := deps.readSelectedConfig
	if readSelected == nil {
		readSelected = func(ctx context.Context) (launcherconfig.Selected, error) {
			return launcherconfig.ReadSelected(ctx, "", nil)
		}
	}
	deps.selectedConfig, err = readSelected(ctx)
	if err != nil {
		return launchDeps{}, launcherconfig.ErrSelected
	}
	deps.configResolver = resolver
	deps.selectorConfig, err = selector.Load(resolver)
	if err != nil {
		return launchDeps{}, err
	}
	return deps, nil
}

// ordinaryResolution retains the actual source files and selector request with
// their resolved pair for reuse through one owning policy path. Neither
// expected metadata nor a successful resolution grants execution.
type ordinaryResolution struct {
	files    defaults.Files
	resolved defaults.Resolved
	deps     launchDeps
}

type ordinaryFailure struct {
	code      string
	err       error
	rawDetail bool
}

// resolveOrdinary shares existing defaults/selector semantics without launching,
// emitting a provider group or selecting a session manager. Enforce mode may
// execute its admitted selector; callers retain its actual context and lifetime.
func resolveOrdinary(ctx context.Context, inv cli.Invocation, frag *fragment.Fragment, resolver configfile.Resolver, deps launchDeps) (*ordinaryResolution, *ordinaryFailure) {
	fail := func(code string, err error) (*ordinaryResolution, *ordinaryFailure) {
		return nil, &ordinaryFailure{code: code, err: err}
	}
	classify := func(err error) (*ordinaryResolution, *ordinaryFailure) {
		var derr *defaults.Error
		if errors.As(err, &derr) {
			return nil, &ordinaryFailure{code: derr.Code, err: derr.Err, rawDetail: true}
		}
		return nil, &ordinaryFailure{code: mapping.CodeUnsupported, err: err, rawDetail: true}
	}
	if deps.configLocations == nil && deps.configPaths != nil {
		paths, err := deps.configPaths()
		if err != nil {
			return fail(defaults.CodeInvalid, fmt.Errorf("cannot locate defaults configuration: %v", err))
		}
		deps.defaults = paths
	}
	var files defaults.Files
	var err error
	if deps.configLocations != nil {
		files, err = defaults.LoadResolved(resolver)
	} else {
		files, err = defaults.Load(deps.defaults)
	}
	if err != nil {
		return classify(err)
	}
	var flags defaults.Pair
	if inv.ModelSet {
		flags.Model = defaults.Member{Value: inv.Model, Present: true}
	}
	if inv.EffortSet {
		flags.Effort = defaults.Member{Value: inv.Effort, Present: true}
	}
	if inv.PermissionSet {
		flags.Permissions = defaults.Member{Value: string(inv.PermissionMode), Present: true}
	}
	// Lock refusal must precede even constructing an external selector request.
	if deps.selectorConfig.Mode == "enforce" {
		if _, err := files.Resolve(frag.Environment, flags); err != nil {
			var derr *defaults.Error
			if errors.As(err, &derr) {
				return fail(derr.Code, derr.Err)
			}
			return fail(diagnostics.CodeDefaultsInvalid, err)
		}
	}
	if deps.selectorConfig.Mode != "" {
		request, err := selectionRequest(inv, frag, deps)
		if err != nil && deps.selectorConfig.Mode == "enforce" {
			return fail(diagnostics.CodeSelectorUnavailable, errors.New("selector unavailable reason=invalid"))
		}
		if err == nil {
			deps.selectorRequest = &request
		}
	}
	var resolved defaults.Resolved
	if deps.selectorConfig.Mode == "enforce" {
		decision, selectionError := selector.Enforce(ctx, deps.selectorConfig, *deps.selectorRequest)
		if selectionError != nil {
			failure := selectionError.(*selector.Failure)
			return fail(failure.Code, failure)
		}
		selected := defaults.Pair{Model: defaults.Member{Value: decision.Model, Present: true}, Effort: defaults.Member{Value: selectedEffort(decision), Present: true}}
		resolved, err = files.CompleteWithSelection(frag.Environment, flags, selected, deps.registry)
		resolved.SelectorDecisionID = decision.DecisionID
		resolved.SelectorRuntime = decision.Runtime
	} else {
		resolved, err = files.Complete(frag.Environment, flags, deps.registry)
	}
	if err != nil {
		return classify(err)
	}
	return &ordinaryResolution{files: files, resolved: resolved, deps: deps}, nil
}
