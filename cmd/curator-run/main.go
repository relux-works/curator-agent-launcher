// Command curator-run is the Curator agent launcher: the execution plane
// that composes the spawn plane (agents-management), the context plane
// (curator env resolve), and the session plane (ax) into one exec.
//
// This is a specification-draft stub. It implements no composition logic;
// SPEC.md is the contract. The agents-management Go module is the planned
// spawn-plane dependency and is deliberately not imported yet.
package main

import (
	"fmt"
	"io"
	"os"
)

const (
	name        = "curator-run"
	specVersion = "0.2.0-draft"
)

const usage = `usage: curator-run <env-id> [--profile <name>] [--system-prompt <append|replace>]
                   [--model <model>] [--effort <effort>]
                   [--name <session-name>] [--ax-profile <standard|yolo>]
                   [--] <native args...>

curator-run is not yet implemented; this build carries the specification
draft only. See SPEC.md in the source repository.

options:
  --help, -h       print this usage text
  --version        print the launcher name and specification version
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run classifies argv for the stub: the two informational flags succeed,
// everything else is a usage error. No argument launches anything.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 {
		switch args[0] {
		case "--help", "-h":
			fmt.Fprintf(stdout, "%s %s\n\n%s", name, specVersion, usage)
			return 0
		case "--version":
			fmt.Fprintf(stdout, "%s %s\n", name, specVersion)
			return 0
		}
	}
	fmt.Fprintf(stderr, "%s %s: not implemented; unknown or unsupported arguments\n\n%s", name, specVersion, usage)
	return 2
}
