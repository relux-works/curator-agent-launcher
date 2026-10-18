package plan

import (
	"maps"
	"slices"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

// Context forwards validated v4 policy to the module, which owns argv and
// sealing. Older Claude/Codex carriers keep their v1-compatible projection;
// older Muse/Pi fragments retain their existing paths. Reserved members remain
// in the original fragment digest and do not activate execution semantics.
func Context(f *fragment.Fragment, intent fragment.Semantics) *agentic.CuratorContext {
	if f.Revision != fragment.IdentityV4 && f.Environment != fragment.EnvClaudeCode && f.Environment != fragment.EnvCodexCLI {
		return nil
	}
	value := &agentic.CuratorContext{
		Revision: agentic.CuratorLaunchFragmentV1, Environment: f.Environment,
		Profile:    agentic.CuratorProfilePin{Name: f.Profile.Name, LockSHA256: f.Profile.LockSHA256},
		Precedence: agentic.CuratorPrecedence{Winner: f.Precedence.Winner, Placement: f.Precedence.Placement},
		Env:        maps.Clone(f.Env),
	}
	if f.Revision == fragment.IdentityV4 {
		value.Revision = agentic.CuratorLaunchFragmentV4
		if f.Permissions != nil {
			value.Permissions = &agentic.CuratorPermissions{
				Mode: agentic.CuratorPermissionMode(f.Permissions.Mode), Locked: f.Permissions.Locked, Source: agentic.CuratorPermissionSource(f.Permissions.Source),
			}
		}
		if f.RootContext != nil {
			value.RootContext = &agentic.CuratorRootContext{Exclusive: f.RootContext.Exclusive}
		}
		if f.CredentialStore != nil {
			value.CredentialStore = &agentic.CuratorCredentialStore{Selection: agentic.CuratorCredentialSelection(f.CredentialStore.Selection), ConfigOverrides: maps.Clone(f.CredentialStore.ConfigOverrides)}
		}
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
