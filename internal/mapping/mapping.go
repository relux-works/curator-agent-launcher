// Package mapping implements the closed SPEC §4.2 system mapping.
// Adapter identity remains owned by fragment; launch support is separate.
package mapping

import (
	"fmt"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

const CodeUnsupported = "env_unsupported"

// EnvAntigravity is Curator's harness id. SAM's system id is antigravity;
// its model rows bind the agy runtime.
const EnvAntigravity = fragment.EnvAntigravity

// Target names the spawn-plane system. Unsupported environments refuse.
type Target struct {
	System string
}

// KnownEnvironment is the closed launcher-side set of recognized Curator
// environments. opencode is recognized for configuration validation, but
// this revision has no launch mapping for it.
func KnownEnvironment(environment string) bool {
	switch environment {
	case fragment.EnvClaudeCode, fragment.EnvCodexCLI, fragment.EnvMuse, fragment.EnvPi, fragment.EnvOpenCode, EnvAntigravity:
		return true
	default:
		return false
	}
}

// Resolve maps only explicitly supported environment identifiers. It neither
// interprets native argv nor infers a provider from an environment name.
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
	case EnvAntigravity:
		return Target{System: "antigravity"}, nil
	default:
		return Target{}, fmt.Errorf("environment %q has no supported system mapping", environment)
	}
}
