//go:build linux

// Command curator-run-m0-adapter is the DRAFT M0 executor adapter
// (swarma-dispatcher spec/m0-executor-entry.md). It takes no flags and no
// environment overrides; its installation literals are set at build time with
// -ldflags -X (see internal/m0adapter/literals.go). It either execve-replaces
// itself with the pinned broker runner or exits 78 before exec.
package main

import "github.com/relux-works/curator-agent-launcher/internal/m0adapter"

func main() {
	m0adapter.Main()
}
