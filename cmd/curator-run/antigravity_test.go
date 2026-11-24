package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
	"github.com/relux-works/curator-agent-launcher/internal/plan"
	"github.com/relux-works/skill-agents-management/pkg/providerlimits"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

type scriptedResolver struct {
	frag   *fragment.Fragment
	lastID string
}

func (s *scriptedResolver) Resolve(_ context.Context, req fragment.Request) (*fragment.Fragment, error) {
	s.lastID = req.EnvID
	return s.frag, nil
}

func TestAntigravityEndToEndNativeLaunch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub test requires unix")
	}
	binDir := t.TempDir()
	outFile := filepath.Join(t.TempDir(), "captured.txt")
	fakeAgy := filepath.Join(binDir, "agy")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "1.5.16"
  exit 0
fi
printf 'ARGV:' > %q
for arg in "$@"; do
  printf '%%s|' "$arg" >> %q
done
printf '\nFORCE=%%s\nNOUPDATE=%%s\nLEAK=%%s\n' "$AGY_CLI_FORCE_FILE_TOKEN_STORAGE" "$AGY_CLI_DISABLE_AUTO_UPDATE" "$UNEXPECTED_AMBIENT_SECRET" >> %q
`, outFile, outFile, outFile)
	if err := os.WriteFile(fakeAgy, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	home := filepath.Join(t.TempDir(), "environments", "default", "antigravity")
	rawFrag, err := json.Marshal(map[string]any{
		"fragment":    fragment.IdentityV7,
		"environment": fragment.EnvAntigravity,
		"profile":     map[string]any{"name": "default", "lock_sha256": strings.Repeat("a", 64)},
		"precedence":  map[string]any{"winner": "higher-weight", "placement": "winner-last"},
		"permissions": map[string]any{"mode": "native", "locked": false, "source": "default"},
		"home":        map[string]any{"path": home, "transport": "flag"},
		"env": map[string]any{
			fragment.AntigravityForceFileTokenVariable: fragment.AntigravityForceFileTokenTrue,
		},
		"credential_store": map[string]any{"selection": "file"},
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := fragment.Parse(rawFrag)
	if err != nil {
		t.Fatalf("fragment.Parse: %v", err)
	}

	reg, err := defaults.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	cfgDir := t.TempDir()
	resolver := &scriptedResolver{frag: parsed}
	env := []string{
		"PATH=" + binDir + string(os.PathListSeparator) + "/usr/bin:/bin",
		"UNEXPECTED_AMBIENT_SECRET=must-not-leak",
	}
	deps := launchDeps{
		resolver: resolver,
		defaults: defaults.Paths{
			Machine:  filepath.Join(cfgDir, "sys", "defaults.json"),
			Operator: filepath.Join(cfgDir, "usr", "defaults.json"),
		},
		readSelectedConfig: func(context.Context) (launcherconfig.Selected, error) {
			return launcherconfig.Selected{}, nil
		},
		workdir: func() (string, error) { return workDir, nil },
		environ: func() []string { return env },
		availability: func() (plan.AvailabilityFunc, error) {
			return func(providerlimits.VerdictQuery) (vendorplugin.Availability, error) {
				return vendorplugin.Availability{State: vendorplugin.AvailabilityHealthy}, nil
			}, nil
		},
		registry:     reg,
		providerPath: func() string { return "/usr/local/bin/curator-run" },
		isTerminal:   func() bool { return true },
	}

	// Launch via `agy` alias with --native --yolo -- --continue.
	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{"agy", "--native", "--yolo", "--", "--continue"}, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("run(agy --native --yolo) = %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}
	if resolver.lastID != fragment.EnvAntigravity {
		t.Fatalf("resolver received EnvID %q, want %q", resolver.lastID, fragment.EnvAntigravity)
	}
	captured, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read captured launch: %v", err)
	}
	text := string(captured)
	for _, want := range []string{
		"--gemini_dir|" + home + "|",
		"--model|",
		"--dangerously-skip-permissions|--continue|",
		"FORCE=true",
		"NOUPDATE=true",
		"LEAK=\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("captured launch missing %q:\n%s", want, text)
		}
	}

	// Conflicting native --gemini_dir must be refused before spawn.
	var out2, err2 strings.Builder
	if err := os.Remove(outFile); err != nil {
		t.Fatal(err)
	}
	code2 := run(context.Background(), []string{"antigravity", "--native", "--", "--gemini_dir", "/tmp/evil"}, &out2, &err2, deps)
	if code2 == 0 {
		t.Fatalf("conflicting native --gemini_dir unexpectedly succeeded\nstderr: %s", err2.String())
	}
	if _, err := os.Stat(outFile); !os.IsNotExist(err) {
		t.Fatal("child executed despite conflicting --gemini_dir")
	}
}
