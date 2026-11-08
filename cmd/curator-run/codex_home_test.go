package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/relux-works/skill-agents-management/pkg/agentic"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

// codexGatewayFragment drops the fragment's CODEX_HOME only when it is exactly
// the plan's canonical home, and keeps every other variable. Any other value,
// including a symlink or unclean spelling of the same directory, refuses with
// the gateway refusal and is never replaced by the plan's home. The caller's
// map is not modified.
func TestCodexGatewayFragmentHome(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	other := filepath.Join(root, "other")
	for _, dir := range []string{home, other} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(home, link); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, declared string
		drop           bool
	}{
		{"canonical", canonical, true},
		{"symlink", link, false},
		{"unclean-same", home + "/./", false},
		{"other-directory", other, false},
		{"missing", filepath.Join(root, "missing"), false},
		{"not-directory", file, false},
		{"empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{"CODEX_HOME": tc.declared, "FIGMA_API_KEY_DIR": "kept"}
			out, err := codexGatewayFragment(fragment.Fragment{Environment: fragment.EnvCodexCLI, Env: env}, canonical)
			if env["CODEX_HOME"] != tc.declared || len(env) != 2 {
				t.Fatal("caller fragment modified")
			}
			if !tc.drop {
				if !errors.Is(err, agentic.ErrCodexGatewayRefused) || out.Env != nil {
					t.Fatalf("home %q admitted: %v", tc.declared, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := out.Env["CODEX_HOME"]; ok || out.Env["FIGMA_API_KEY_DIR"] != "kept" || len(out.Env) != 1 {
				t.Fatalf("composed env %v", out.Env)
			}
		})
	}
	t.Run("absent", func(t *testing.T) {
		env := map[string]string{"FIGMA_API_KEY_DIR": "kept"}
		out, err := codexGatewayFragment(fragment.Fragment{Environment: fragment.EnvCodexCLI, Env: env}, canonical)
		if err != nil || len(out.Env) != 1 || out.Env["FIGMA_API_KEY_DIR"] != "kept" {
			t.Fatalf("absent home changed the fragment: %v %v", out.Env, err)
		}
	})
	t.Run("case-variant-kept", func(t *testing.T) {
		// Only the exact name is the plan's home; a case variant stays in the
		// fragment so the module's alias check refuses it.
		env := map[string]string{"codex_home": link}
		out, err := codexGatewayFragment(fragment.Fragment{Environment: fragment.EnvCodexCLI, Env: env}, canonical)
		if err != nil || out.Env["codex_home"] != link {
			t.Fatalf("case variant dropped: %v %v", out.Env, err)
		}
	})
}
