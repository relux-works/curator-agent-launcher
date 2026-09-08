// Command curator-run is the Curator agent launcher: the execution plane
// that composes the spawn plane (agents-management), the context plane
// (curator env resolve), and the session plane (ax) into one exec.
//
// This build implements SPEC.md §3, the closed CLI surface, through
// internal/cli. Composition (§4), system-prompt application (§5), and the
// configuration file family (§4.7) are not delivered yet: a well-formed
// launch invocation is refused after parsing with exit 1 and launches
// nothing. The agents-management Go module is the planned spawn-plane
// dependency and is deliberately not imported yet.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
)

const (
	name        = cli.Name
	specVersion = "0.2.1-draft"
	// buildVersion is the launcher's own version; the specification
	// version is reported beside it (SPEC §8).
	buildVersion = "0.0.0-dev"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the production entry point behind main. It reads the ax
// configuration fact (not yet: §4.6 ax.json is owned by a later section,
// so this build parses against an unconfigured integration), parses argv
// under §3, prints informational output, and reports usage errors as the
// §6 usage family on stderr with exit 2. A launch invocation that parses
// is refused with exit 1 because no composition stage exists in this
// build; nothing is resolved and nothing is launched.
func run(args []string, stdout, stderr io.Writer) int {
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
	fmt.Fprintf(stderr, "%s: not_implemented: parsed launch for environment %q, but composition (SPEC §4) is not delivered in this build; nothing was resolved or launched\n", name, inv.EnvID)
	return 1
}
