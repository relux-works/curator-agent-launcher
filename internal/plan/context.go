package plan

import (
	"maps"
	"slices"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

// Context projects validated Claude/Codex fragment channels without constructing
// argv. The system plugin owns channel selection, encoding and conflict checks.
// Later fragment revisions retain their permission transport in the launcher;
// their context members are the v1-compatible subset supported by the carrier.
// Pi prompt discovery and Muse's XDG environment stay on their existing paths.
// Reserved fragment members remain in the original fragment's digest and
// transport only; forwarding them would activate unsupported carrier semantics.
func Context(f *fragment.Fragment, intent fragment.Semantics) *agentic.CuratorContext {
	if f.Environment != fragment.EnvClaudeCode && f.Environment != fragment.EnvCodexCLI {
		return nil
	}
	value := &agentic.CuratorContext{
		Revision: agentic.CuratorLaunchFragmentV1, Environment: f.Environment,
		Profile:    agentic.CuratorProfilePin{Name: f.Profile.Name, LockSHA256: f.Profile.LockSHA256},
		Precedence: agentic.CuratorPrecedence{Winner: f.Precedence.Winner, Placement: f.Precedence.Placement},
		Env:        maps.Clone(f.Env),
	}
	channels := func(source []fragment.Channel) []agentic.CuratorChannelDescriptor {
		out := make([]agentic.CuratorChannelDescriptor, 0, len(source))
		for _, c := range source {
			out = append(out, agentic.CuratorChannelDescriptor{
				Kind: agentic.CuratorDescriptorKind(c.Kind), Semantics: agentic.CuratorSystemPromptIntent(c.Semantics),
				Flag: c.Flag, Argument: agentic.CuratorFlagArgument(c.Argument), Name: c.Name, With: slices.Clone(c.With),
				Key: c.Key, Variable: c.Variable, Filename: c.Filename,
			})
		}
		return out
	}
	if f.SystemPrompt != nil && intent != "" {
		value.SystemPrompt = &agentic.CuratorSystemPromptContext{Path: f.SystemPrompt.Path,
			Intent: agentic.CuratorSystemPromptIntent(intent), Channels: channels(f.SystemPrompt.Channels)}
	}
	if f.MCP != nil {
		value.MCP = &agentic.CuratorMCPContext{Path: f.MCP.Path,
			Channels: channels(f.MCP.Channels), EnvNames: slices.Clone(f.MCP.EnvNames)}
	}
	return value
}
