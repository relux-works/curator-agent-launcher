// Package mapping implements the closed SPEC §4.2 system/provider mapping.
// Adapter identity remains owned by fragment; launch support is separate.
package mapping

import (
	"fmt"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

const CodeUnsupported = "env_unsupported"

// Target names both consumers. A partial mapping is never launchable.
type Target struct {
	System   string
	Provider string
}

// Resolve maps only explicitly supported environment identifiers. It neither
// interprets native argv nor infers a provider from an environment name.
// Use fragment.HomeVariable for registry membership in configuration readers:
// opencode is known there even though this revision cannot launch it.
func Resolve(environment string) (Target, error) {
	switch environment {
	case fragment.EnvClaudeCode:
		return Target{System: "claude-code", Provider: "claude"}, nil
	case fragment.EnvCodexCLI:
		return Target{System: "codex", Provider: "codex"}, nil
	case fragment.EnvPi:
		return Target{System: "pi-native", Provider: "pi"}, nil
	default:
		return Target{}, fmt.Errorf("environment %q has no supported system/provider pair", environment)
	}
}
