// Package mapping implements the closed SPEC §4.2 system mapping.
// Adapter identity remains owned by fragment; launch support is separate.
package mapping

import (
	"fmt"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

const CodeUnsupported = "env_unsupported"

// Target names the spawn-plane system. Unsupported environments refuse.
type Target struct {
	System string
}

// Resolve maps only explicitly supported environment identifiers. It neither
// interprets native argv nor infers a provider from an environment name.
// Use fragment.HomeVariable for registry membership in configuration readers:
// opencode is known there even though this revision cannot launch it.
func Resolve(environment string) (Target, error) {
	switch environment {
	case fragment.EnvClaudeCode:
		return Target{System: "claude-code"}, nil
	case fragment.EnvCodexCLI:
		return Target{System: "codex"}, nil
	case fragment.EnvMuse:
		return Target{System: "muse"}, nil
	case fragment.EnvPi:
		return Target{System: "pi-native"}, nil
	default:
		return Target{}, fmt.Errorf("environment %q has no supported system mapping", environment)
	}
}
