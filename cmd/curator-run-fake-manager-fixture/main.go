//go:build unix

// curator-run-fake-manager-fixture is the out-of-band qualification adapter.
package main

import (
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/fakemanager"
	"os"
)

func main() {
	dir, args := fakemanager.StateArguments(os.Args[1:])
	if len(args) != 0 {
		os.Exit(2)
	}
	os.Exit(fakemanager.Fixture(dir, os.Stdin, os.Stdout))
}
