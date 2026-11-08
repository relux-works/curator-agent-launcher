package main

import (
	"maps"

	"github.com/relux-works/skill-agents-management/pkg/agentic"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

const codexHomeVariable = "CODEX_HOME"

// codexGatewayFragment returns the fragment the Codex gateway route admits and
// composes. The admitted plan already owns CODEX_HOME, so the fragment's copy
// of the same value is dropped instead of overriding it with an "environment
// override" warning. The module admits only a canonical home spelling, so any
// other value (a symlink, an unclean path, another directory) refuses here
// rather than being replaced by the plan's home.
func codexGatewayFragment(frag fragment.Fragment, planHome string) (fragment.Fragment, error) {
	declared, ok := frag.Env[codexHomeVariable]
	if !ok {
		return frag, nil
	}
	if planHome == "" || declared != planHome {
		return fragment.Fragment{}, agentic.ErrCodexGatewayRefused
	}
	out := frag
	out.Env = maps.Clone(frag.Env)
	delete(out.Env, codexHomeVariable)
	return out, nil
}
