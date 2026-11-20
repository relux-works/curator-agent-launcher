// This fixture exposes the hook installer and read-only trust probe to the Swift contract tests.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/relux-works/curator-agent-launcher/internal/doorbell"
)

func main() {
	if len(os.Args) != 6 {
		os.Exit(2)
	}
	op, dir, home, identity, provider := os.Args[1], os.Args[2], os.Args[3], os.Args[4], os.Args[5]
	installer := doorbell.Installer{Environment: os.Environ(), InvocationDirectory: dir}
	var err error
	switch op {
	case "trust":
		err = installer.CodexTrust(context.Background(), provider, dir, home, identity)
	case "install":
		_, err = installer.EnsureCodex(context.Background(), dir, home, identity)
	default:
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
