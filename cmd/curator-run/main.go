// Command curator-run is the Curator agent launcher: the execution plane
// that composes the spawn plane (agents-management), the context plane
// (curator env resolve), and the session plane (ax) into one exec.
//
// This build implements SPEC.md §3, the closed CLI surface, through
// internal/cli, §4.1, the fragment resolution, through
// internal/fragment, §4.2 through internal/mapping, and §4.3, the model
// and effort defaults, through internal/defaults against the real tagged
// agents-management module. The later composition steps (§4.4-§4.6),
// system-prompt application (§5), and the ax configuration file (§4.7)
// are not delivered yet: a completed launch is refused after the origin
// line-group with exit 1 and launches nothing.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/mapping"
)

const (
	name        = cli.Name
	specVersion = "0.3.0-draft"
	// buildVersion is the launcher's own version; the specification
	// version is reported beside it (SPEC §8).
	buildVersion = "0.0.0-dev"
)

func main() {
	registry, err := defaults.NewRegistry()
	if err != nil {
		_ = diagnostics.Emit(os.Stderr, diagnostics.CodePlanRefused, fmt.Sprintf("cannot build the spawn-plane registry: %v", err))
		os.Exit(diagnostics.ExitForCode(diagnostics.CodePlanRefused))
	}
	deps := launchDeps{
		resolver:    fragment.New(),
		configPaths: processDefaultsPaths,
		registry:    registry,
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
// processDefaultsPaths is evaluated only for a mapped launch. Informational
// flags and earlier refusals never depend on configuration path discovery.
func processDefaultsPaths() (defaults.Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil && os.Getenv("XDG_CONFIG_HOME") == "" {
		return defaults.Paths{}, err
	}
	return defaults.ConfigPaths(os.Getenv("XDG_CONFIG_HOME"), home), nil
}

type launchDeps struct {
	configPaths func() (defaults.Paths, error)
	resolver    fragmentResolver
	defaults    defaults.Paths
	registry    *vendorplugin.Registry
}

// run is the production entry point behind main. It reads the ax
// configuration fact (not yet: §4.6 ax.json is owned by a later section,
// so this build parses against an unconfigured integration), parses argv
// under §3, prints informational output, and reports usage errors as the
// §6 usage family on stderr with exit 2. A launch invocation that parses
// goes to §4.1: the fragment is resolved through the resolver — always
// with --repair, Curator's stderr forwarded verbatim — and a resolve
// failure is printed as its §6 resolve-family code line with exit 1. A
// resolved fragment is mapped under §4.2 (unsupported IDs refuse
// env_unsupported), then completed under §4.3: the launcher-owned files
// are read (a locked flag is a usage refusal, an unreadable file is
// defaults_config_invalid), the tagged module lineup supplies only the
// members left unset (nothing admittable is defaults_unresolvable), and
// the resolved pair with its per-member origins is printed on stderr
// before anything downstream. The plan request of §4.4 belongs to a later
// stage, so a completed launch is refused with exit 1; nothing is
// launched.
//
// Every failure renders through internal/diagnostics: one deterministic
// `curator-run: <code>: <detail>` line and the SPEC §6 exit for that code
// (2 for usage, 1 for every operational failure). The rendering is
// byte-identical to the stage-owned formats it replaces.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, deps launchDeps) int {
	opts := cli.Options{AxConfigured: false}
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
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return diagnostics.ExitOperational
	}
	switch inv.Info {
	case cli.InfoHelp:
		fmt.Fprintf(stdout, "%s %s (specification %s)\n\n%s", name, buildVersion, specVersion, cli.Usage)
		return 0
	case cli.InfoVersion:
		fmt.Fprintf(stdout, "%s %s (specification %s)\n", name, buildVersion, specVersion)
		return 0
	}

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
	// every one terminal with nothing launched. The resolved pair and
	// its per-member origins print on stderr before the plan request.
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
			_ = defaults.EmitGroup(stderr, resolved)
			fmt.Fprintf(stderr, "%s: not_implemented: resolved environment %q (profile %q, home %s, fragment digest %s), mapped system %q / provider %q, %s; the plan request beyond SPEC §4.3 is not delivered in this build; nothing was launched\n",
				name, frag.Environment, frag.Profile.Name, frag.Home(), frag.Digest, target.System, target.Provider, resolved.Describe())
			return 1
		}
	}
	if derr := (&defaults.Error{}); errors.As(err, &derr) {
		_ = diagnostics.Emit(stderr, derr.Code, derr.Err.Error())
		return diagnostics.ExitForCode(derr.Code)
	}
	_ = diagnostics.Emit(stderr, mapping.CodeUnsupported, err.Error())
	return diagnostics.ExitForCode(mapping.CodeUnsupported)
}
