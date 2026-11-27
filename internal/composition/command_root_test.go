package composition

import (
	"os"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-runtimes/pkg/agentic"
)

func TestComposeV9PrependsVerifiedCommandRootPath(t *testing.T) {
	binDir := "/Users/iv/.curator/environments/colab-g4-keeper/commands/" + strings.Repeat("c", 64) + "/darwin-arm64/bin"
	frag := fragment.Fragment{
		Revision:    fragment.IdentityV9,
		Environment: fragment.EnvClaudeCode,
		Env:         map[string]string{"CLAUDE_CONFIG_DIR": "/Users/iv/.curator/environments/colab-g4-keeper/claude_code"},
		CommandRoot: &fragment.CommandRoot{
			LockSHA256: strings.Repeat("a", 64),
			Generation: strings.Repeat("c", 64),
			Platform:   "darwin-arm64",
		},
		PathPrepend: binDir,
	}
	p := agentic.Plan{
		Binary:  "/usr/local/bin/claude",
		WorkDir: "/tmp",
		Env:     []string{"PATH=/usr/bin:/bin", "HOME=/Users/iv"},
	}
	val, err := Compose(p, nil, frag, PromptApplication{}, nil)
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	wantPath := binDir + string(os.PathListSeparator) + "/usr/bin:/bin"
	found := ""
	for _, kv := range val.Env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == "PATH" {
			found = v
		}
	}
	if found != wantPath {
		t.Fatalf("PATH = %q, want %q", found, wantPath)
	}

	// Ensure v2 with PathPrepend stays reserved/inert!
	fragV2 := frag
	fragV2.Revision = fragment.IdentityV2
	fragV2.CommandRoot = nil
	valV2, err := Compose(p, nil, fragV2, PromptApplication{}, nil)
	if err != nil {
		t.Fatalf("Compose v2 failed: %v", err)
	}
	foundV2 := ""
	for _, kv := range valV2.Env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == "PATH" {
			foundV2 = v
		}
	}
	if foundV2 != "/usr/bin:/bin" {
		t.Fatalf("v2 reserved path_prepend mutated PATH to %q", foundV2)
	}
}
