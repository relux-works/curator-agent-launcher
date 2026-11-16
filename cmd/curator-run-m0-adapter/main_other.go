//go:build !linux

// Command curator-run-m0-adapter is Linux-only. On every other platform it
// refuses before exec with exit 78 and one sanitized reason line.
package main

import "github.com/relux-works/curator-agent-launcher/internal/m0adapter"

func main() {
	m0adapter.Exit(m0adapter.ReasonPlatformUnsupported)
}
