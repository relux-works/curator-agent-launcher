package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/relux-works/skill-agents-management/pkg/providerlimits"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"

	"github.com/relux-works/curator-agent-launcher/internal/axconfig"
	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/mapping"
	"github.com/relux-works/curator-agent-launcher/internal/plan"
	"github.com/relux-works/curator-agent-launcher/internal/systemprompt"
)

const (
	name        = cli.Name
	specVersion = "0.4.0-draft"
	// buildVersion is the launcher's own version; the specification
	// version is reported beside it (SPEC §8).
	buildVersion = "0.1.0-dev"
)

func main() {
	registry, err := defaults.NewRegistry()
	if err != nil {
		_ = diagnostics.Emit(os.Stderr, diagnostics.CodePlanRefused, fmt.Sprintf("cannot build the spawn-plane registry: %v", err))
		os.Exit(diagnostics.ExitForCode(diagnostics.CodePlanRefused))
	}
	deps := launchDeps{
		resolver:     fragment.New(),
		configPaths:  processDefaultsPaths,
		axPaths:      processDefaultsPaths,
		workdir:      os.Getwd,
		environ:      os.Environ,
		availability: processAvailability,
		registry:     registry,
	}
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, deps))
}

// fragmentResolver is the context-plane boundary; main uses fragment.New.
type fragmentResolver interface {
	Resolve(context.Context, fragment.Request) (*fragment.Fragment, error)
}

// launchDeps carries the production boundaries run dispatches through.
// Tests inject scripted resolvers, temporary configuration paths, and
// registries built from the real module; main builds them from process
// state.
// Configuration directories are discovered before parsing for ax policy;
// defaults file contents are read only after a supported fragment is mapped.
func processDefaultsPaths() (defaults.Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil && os.Getenv("XDG_CONFIG_HOME") == "" {
		return defaults.Paths{}, err
	}
	return defaults.ConfigPaths(os.Getenv("XDG_CONFIG_HOME"), home), nil
}

// Process boundaries are injectable; production uses the tagged builder and real execution.
type launchDeps struct {
	axPaths      func() (defaults.Paths, error)
	workdir      func() (string, error)
	environ      func() []string
	availability func() (plan.AvailabilityFunc, error)
	build        plan.BuildLaunchFunc
	axBinary     string
	stdin        io.Reader
	now          func() time.Time
	// providerPath supplies the §4.3 provider-line path. Production
	// leaves it nil so EmitGroup resolves os.Executable; tests inject
	// a deterministic path so goldens stay stable across machines.
	providerPath func() string

	configPaths func() (defaults.Paths, error)
	resolver    fragmentResolver
	defaults    defaults.Paths
	registry    *vendorplugin.Registry
}

// run is the production entry point. Configuration precedes argument validation;
// every admitted launch flows through composition and all late execution checks.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, deps launchDeps) int {
	paths := deps.defaults
	if deps.axPaths != nil {
		var err error
		paths, err = deps.axPaths()
		if err != nil {
			return emitFailure(stderr, diagnostics.CodeDefaultsInvalid, err)
		}
	}
	configured, err := axconfig.Load(filepath.Dir(paths.Machine), filepath.Dir(paths.Operator))
	if err != nil {
		return emitFailure(stderr, diagnostics.CodeDefaultsInvalid, err)
	}
	opts := cli.Options{AxConfigured: configured}
	inv, err := cli.Parse(args, opts)
	if err != nil {
		if cli.IsUsage(err) {
			detail := err.Error()
			var ue *cli.UsageError
			if errors.As(err, &ue) {
				detail = ue.Detail
			}
			_ = diagnostics.Emit(stderr, diagnostics.CodeUsage, detail)
			fmt.Fprintf(stderr, "\n%s", cli.Usage)
			return diagnostics.ExitForCode(diagnostics.CodeUsage)
		}
		return emitFailure(stderr, diagnostics.CodeUsage, err)
	}
	switch inv.Info {
	case cli.InfoHelp:
		fmt.Fprintf(stdout, "%s %s (specification %s)\n\n%s", name, buildVersion, specVersion, cli.Usage)
		return 0
	case cli.InfoVersion:
		fmt.Fprintf(stdout, "%s %s (specification %s)\n", name, buildVersion, specVersion)
		return 0
	}

	// SPEC §3: the operand is already normalized by cli.Parse; normalize
	// again at this boundary so the resolve argv and the default ax session
	// name (execution.Prepare) always carry the canonical id even if the
	// parser is ever bypassed. Aliases never persist beyond this point.
	inv.EnvID = cli.NormalizeEnvID(inv.EnvID)

	// SPEC §4.1: obtain the fragment. The subprocess inherits the
	// launcher's working directory and environment; Curator's stderr goes
	// to the operator unchanged.
	frag, err := deps.resolver.Resolve(ctx, fragment.Request{
		EnvID:      inv.EnvID,
		Profile:    inv.Profile,
		ProfileSet: inv.ProfileSet,
		Stderr:     stderr,
	})
	if err != nil {
		if re, ok := fragment.IsResolve(err); ok {
			_ = diagnostics.Emit(stderr, re.Code, re.Detail)
			return diagnostics.ExitForCode(re.Code)
		}
		_ = diagnostics.Emit(stderr, fragment.CodeInvocationFailed, err.Error())
		return diagnostics.ExitForCode(fragment.CodeInvocationFailed)
	}

	target, err := mapping.Resolve(frag.Environment)
	if err != nil {
		_ = diagnostics.Emit(stderr, mapping.CodeUnsupported, err.Error())
		return diagnostics.ExitForCode(mapping.CodeUnsupported)
	}

	// SPEC §4.3: resolve model and effort defaults. A locked flag is a
	// usage refusal, an unreadable file is defaults_config_invalid, and
	// a system the lineup admits nothing for is defaults_unresolvable —
	// every one terminal with nothing launched. The provider path and
	// the resolved pair with its per-member origins print on stderr as
	// one line-group before the plan request.
	if deps.configPaths != nil {
		paths, err := deps.configPaths()
		if err != nil {
			_ = diagnostics.Emit(stderr, defaults.CodeInvalid, fmt.Sprintf("cannot locate defaults configuration: %v", err))
			return diagnostics.ExitForCode(defaults.CodeInvalid)
		}
		deps.defaults = paths
	}
	files, err := defaults.Load(deps.defaults)
	if err == nil {
		var flags defaults.Pair
		if inv.ModelSet {
			flags.Model = defaults.Member{Value: inv.Model, Present: true}
		}
		if inv.EffortSet {
			flags.Effort = defaults.Member{Value: inv.Effort, Present: true}
		}
		var resolved defaults.Resolved
		resolved, err = files.Complete(frag.Environment, flags, deps.registry)
		if err == nil {
			provider := defaults.ResolveProviderPath()
			if deps.providerPath != nil {
				provider = deps.providerPath()
			}
			_ = defaults.EmitGroupWithProvider(stderr, resolved, provider)
			return launch(ctx, inv, frag, target, resolved, stdout, stderr, deps)
		}
	}
	if derr := (&defaults.Error{}); errors.As(err, &derr) {
		_ = diagnostics.Emit(stderr, derr.Code, derr.Err.Error())
		return diagnostics.ExitForCode(derr.Code)
	}
	_ = diagnostics.Emit(stderr, mapping.CodeUnsupported, err.Error())
	return diagnostics.ExitForCode(mapping.CodeUnsupported)
}

// emitFailure frames only launcher-owned diagnostics. Foreign evidence is emitted
// separately at the plan and execution boundaries.
func emitFailure(w io.Writer, code string, err error) int {
	_ = diagnostics.Emit(w, code, strings.TrimPrefix(err.Error(), code+": "))
	return diagnostics.ExitForCode(code)
}

func processAvailability() (plan.AvailabilityFunc, error) {
	layout, err := providerlimits.DefaultLayout()
	if err != nil {
		return nil, err
	}
	store, err := providerlimits.NewStore(providerlimits.Options{Layout: layout})
	if err != nil {
		return nil, err
	}
	return store.AvailabilityFor, nil
}

func launch(ctx context.Context, inv cli.Invocation, frag *fragment.Fragment, target mapping.Target, resolved defaults.Resolved, stdout, stderr io.Writer, deps launchDeps) int {
	wd, err := deps.workdir()
	if err != nil {
		return emitFailure(stderr, diagnostics.CodePlanRefused, err)
	}
	availability, err := deps.availability()
	if err != nil {
		return emitFailure(stderr, diagnostics.CodePlanRefused, err)
	}
	build := deps.build
	if build == nil {
		build = vendorplugin.BuildLaunchWithEnvironment
	}
	admitted, err := plan.Build(ctx, plan.Deps{Registry: deps.registry, BuildLaunch: build, Availability: availability}, plan.Request{
		Runtime: resolved.Runtime, Model: resolved.Model.Value, Effort: resolved.Effort.Value, Home: frag.Home(), WorkDir: wd, Env: deps.environ(),
	})
	if err != nil {
		code := diagnostics.CodePlanRefused
		var limited *plan.LimitedError
		if errors.As(err, &limited) {
			code = diagnostics.CodePlanProviderLimited
		}
		_ = diagnostics.Emit(stderr, code, "spawn-plane admission refused the launch")
		// Preserve provider/module evidence bytes, including embedded newlines.
		fmt.Fprintln(stderr, err.Error())
		return diagnostics.ExitForCode(code)
	}
	selection, err := systemprompt.Select(frag, fragment.Semantics(inv.SystemPrompt))
	if err != nil {
		code, _ := diagnostics.CodeOf(err)
		return emitFailure(stderr, code, err)
	}
	value, err := composition.Compose(admitted.Plan, admitted.OwnedEnv, *frag, composition.PromptApplication{Argv: selection.Argv(), Env: selection.Env()}, inv.Native)
	if err != nil {
		return emitFailure(stderr, diagnostics.CodePlanRefused, err)
	}
	now := time.Now
	if deps.now != nil {
		now = deps.now
	}
	prepared, err := execution.Prepare(value, *frag, inv, target, now())
	if err != nil {
		return emitFailure(stderr, diagnostics.CodeAxHandoffFailed, err)
	}
	return prepared.Run(execution.Options{AxBinary: deps.axBinary, IO: execution.IO{Stdin: deps.stdin, Stdout: stdout, Stderr: stderr}, Boundary: func() error {
		prompt, err := systemprompt.PrepareLaunch(frag, fragment.Semantics(inv.SystemPrompt))
		if err != nil {
			return err
		}
		for _, warning := range prompt.Warnings {
			fmt.Fprintln(stderr, name+": "+warning)
		}
		return nil
	}})
}
