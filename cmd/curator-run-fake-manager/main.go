//go:build unix

// curator-run-fake-manager is a reference protocol fixture, not a session host.
package main

import (
	"os"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/fakemanager"
)

func main() {
	dir, args := fakemanager.StateArguments(os.Args[1:])
	var request, status *os.File
	// Do not wrap ambient fd3/fd4 on JSON operations: the Go runtime may own
	// those numbers (notably kqueue on macOS). Only terminal calls inherit them.
	if len(args) > 0 && (args[0] == "attach" || args[0] == "launch-exact") {
		request, status = os.NewFile(3, "attach request"), os.NewFile(4, "status")
	}
	os.Exit(fakemanager.Run(dir, args, os.Stdin, request, status, os.Stdout))
}
