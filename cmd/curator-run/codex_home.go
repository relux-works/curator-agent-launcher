package main

import (
	"maps"
	"os"

	"github.com/relux-works/skill-agents-management/pkg/agentic"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

const codexHomeVariable = "CODEX_HOME"

// codexGatewayFragment returns the fragment the Codex gateway route admits and
// composes. The admitted plan already owns the canonical CODEX_HOME, so the
// fragment's own spelling of the same directory (for example through a
// symlink) is dropped instead of overriding it. A fragment home that is not
// the plan's home directory refuses: it is never silently replaced.
func codexGatewayFragment(frag fragment.Fragment, planHome string) (fragment.Fragment, error) {
	declared, ok := frag.Env[codexHomeVariable]
	if !ok {
		return frag, nil
	}
	if !sameDirectory(declared, planHome) {
		return fragment.Fragment{}, agentic.ErrCodexGatewayRefused
	}
	out := frag
	out.Env = maps.Clone(frag.Env)
	delete(out.Env, codexHomeVariable)
	return out, nil
}

func sameDirectory(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ai, err := os.Stat(a)
	if err != nil || !ai.IsDir() {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil || !bi.IsDir() {
		return false
	}
	return os.SameFile(ai, bi)
}
