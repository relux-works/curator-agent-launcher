package diagnostics_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/axconfig"
	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	sp "github.com/relux-works/curator-agent-launcher/internal/systemprompt"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

// stubEnv stands in for the spawn-plane module at the ChildEnv boundary:
// the composition and probe code under test is real, only the external
// module's owned literals are stubbed.
type stubEnv struct{ env []string }

func (s stubEnv) ChildEnv([]string, agentic.LaunchRequest) ([]string, error) {
	return s.env, nil
}

func codexFragment(t *testing.T, home, layer string) fragment.Fragment {
	t.Helper()
	f := map[string]any{
		"fragment":    fragment.Identity,
		"environment": fragment.EnvCodexCLI,
		"profile":     map[string]any{"name": "default", "lock_sha256": strings.Repeat("a", 64)},
		"precedence":  map[string]string{"winner": "higher-weight", "placement": "winner-last"},
		"env":         map[string]string{fragment.HomeVariable(fragment.EnvCodexCLI): home},
		"mcp": map[string]any{
			"path":      layer,
			"env_names": []string{},
			"channels":  []any{map[string]any{"kind": "flag", "flag": "-p", "argument": "name", "name": "curator-mcp"}},
		},
	}
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	frag, err := fragment.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return *frag
}

// codexBoundary composes a real codex_cli value through production
// composition.Compose and returns the real CheckLaunchBoundary failures
// for a missing layer file and for a directory at the layer path.
func codexBoundary(t *testing.T) (missing, unreadable error) {
	t.Helper()
	home := t.TempDir()
	plan := agentic.Plan{Binary: "codex", WorkDir: home, Argv: []string{}, Env: []string{"PATH=/usr/bin"}}
	req := agentic.LaunchRequest{}
	mk := func(layer string) composition.Value {
		t.Helper()
		v, err := composition.Compose(plan, stubEnv{env: []string{"PATH=/usr/bin"}}, req, codexFragment(t, home, layer), composition.PromptApplication{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	missing = mk(filepath.Join(home, "absent-curator-mcp.config.toml")).CheckLaunchBoundary()
	if missing == nil {
		t.Fatal("absent layer must fail")
	}
	dirLayer := filepath.Join(home, "dir-layer.toml")
	if err := os.Mkdir(dirLayer, 0o755); err != nil {
		t.Fatal(err)
	}
	unreadable = mk(dirLayer).CheckLaunchBoundary()
	if unreadable == nil {
		t.Fatal("directory layer must fail")
	}
	// A readable regular layer launches: absence and failure stay distinct.
	fileLayer := filepath.Join(home, "curator-mcp.config.toml")
	if err := os.WriteFile(fileLayer, []byte("# mcp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := mk(fileLayer).CheckLaunchBoundary(); err != nil {
		t.Fatalf("readable layer must pass: %v", err)
	}
	return missing, unreadable
}

func minimalFragment(t *testing.T, env, home string, section bool) *fragment.Fragment {
	t.Helper()
	f := map[string]any{
		"fragment":    fragment.Identity,
		"environment": env,
		"profile":     map[string]any{"name": "default", "lock_sha256": strings.Repeat("b", 64)},
		"precedence":  map[string]string{"winner": "higher-weight", "placement": "winner-last"},
		"env":         map[string]string{fragment.HomeVariable(env): home},
	}
	if section {
		f["system_prompt"] = map[string]any{"path": filepath.Join(home, "prompt.md"), "channels": []any{}}
	}
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	frag, err := fragment.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return frag
}

// syspromptSelection drives the real Select refusal: opt-in over a
// fragment with no system_prompt section.
func syspromptSelection(t *testing.T) error {
	t.Helper()
	_, err := sp.Select(minimalFragment(t, fragment.EnvClaudeCode, t.TempDir(), false), fragment.SemanticsAppend)
	if err == nil {
		t.Fatal("opt-in without a section must refuse")
	}
	return err
}

// syspromptProbe drives the real ProbeFiles refusal: a directory where the
// Pi registry filename must be a readable regular file.
func syspromptProbe(t *testing.T) error {
	t.Helper()
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "SYSTEM.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, refusal := sp.ProbeFiles(minimalFragment(t, fragment.EnvPi, home, false))
	if refusal == nil {
		t.Fatal("directory at the registry filename must refuse")
	}
	// Absence stays absence: no observation, no error, no diagnostic.
	empty := t.TempDir()
	obs, err := sp.ProbeFiles(minimalFragment(t, fragment.EnvPi, empty, false))
	if err != nil || len(obs) != 0 {
		t.Fatalf("absent files: %v %v", obs, err)
	}
	return refusal
}

// axconfigFailure drives the real Load read failure: a present but broken
// ax.json is defaults_config_invalid, never "not configured".
func axconfigFailure(t *testing.T) error {
	t.Helper()
	root := t.TempDir()
	machine := filepath.Join(root, "machine")
	if err := os.MkdirAll(machine, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(machine, "ax.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := axconfig.Load(machine, filepath.Join(root, "absent-operator"))
	if err == nil {
		t.Fatal("broken ax.json must fail")
	}
	// Absence stays absence: both dirs missing means not configured.
	if enabled, err := axconfig.Load(filepath.Join(root, "no-machine"), filepath.Join(root, "no-operator")); err != nil || enabled {
		t.Fatalf("absent files: %v %v", enabled, err)
	}
	return err
}
