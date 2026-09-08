// Command curator-run is the Curator agent launcher: the execution plane
// that composes the spawn plane (agents-management), the context plane
// (curator env resolve), and the session plane (ax) into one exec.
//
// This build implements SPEC.md §3, the closed CLI surface, through
// internal/cli, and §4.1, the fragment resolution, through
// internal/fragment, and §4.2 through internal/mapping. The later
// composition steps (§4.3-§4.6), system-prompt
// application (§5), and the configuration file family (§4.7) are not
// delivered yet: a launch invocation whose fragment resolves is refused
// after mapping with exit 1 and launches nothing. The agents-management
// Go module is the planned spawn-plane dependency and is deliberately not
// imported yet.
package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
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
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, fragment.New()))
}

// fragmentResolver is the context-plane boundary; main uses fragment.New.
type fragmentResolver interface {
	Resolve(context.Context, fragment.Request) (*fragment.Fragment, error)
}

// run is the production entry point behind main. It reads the ax
// configuration fact (not yet: §4.6 ax.json is owned by a later section,
// so this build parses against an unconfigured integration), parses argv
// under §3, prints informational output, and reports usage errors as the
// §6 usage family on stderr with exit 2. A launch invocation that parses
// goes to §4.1: the fragment is resolved through resolver — always with
// --repair, Curator's stderr forwarded verbatim — and a resolve failure is
// printed as its §6 resolve-family code line with exit 1. A resolved
// fragment is mapped under §4.2 (unsupported IDs refuse env_unsupported),
// then refused with exit 1 because no later composition stage
// exists in this build; nothing is launched.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, resolver fragmentResolver) int {
	opts := cli.Options{AxConfigured: false}
	inv, err := cli.Parse(args, opts)
	if err != nil {
		if cli.IsUsage(err) {
			fmt.Fprintf(stderr, "%s: %s\n\n%s", name, err.Error(), cli.Usage)
			return cli.ExitCode
		}
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
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
	frag, err := resolver.Resolve(ctx, fragment.Request{
		EnvID:      inv.EnvID,
		Profile:    inv.Profile,
		ProfileSet: inv.ProfileSet,
		Stderr:     stderr,
	})
	if err != nil {
		if re, ok := fragment.IsResolve(err); ok {
			fmt.Fprintf(stderr, "%s: %s: %s\n", name, re.Code, re.Detail)
			return 1
		}
		fmt.Fprintf(stderr, "%s: %s: %v\n", name, fragment.CodeInvocationFailed, err)
		return 1
	}

	target, err := mapping.Resolve(frag.Environment)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %s: %v\n", name, mapping.CodeUnsupported, err)
		return 1
	}

	fmt.Fprintf(stderr, "%s: not_implemented: resolved environment %q (profile %q, home %s, fragment digest %s), mapped system %q / provider %q, but composition beyond SPEC §4.2 is not delivered in this build; nothing was launched\n",
		name, frag.Environment, frag.Profile.Name, frag.Home(), frag.Digest, target.System, target.Provider)
	return 1
}
