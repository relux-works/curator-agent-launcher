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
	agySystem "github.com/relux-works/curator-runtimes/pkg/agentic/systems/agy"
	"github.com/relux-works/curator-runtimes/pkg/providerlimits"
	"github.com/relux-works/curator-runtimes/pkg/runtimecatalog"
	"github.com/relux-works/curator-runtimes/pkg/vendorplugin"
)

func TestKimiCodeEndToEndInteractiveLaunch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub test requires unix")
	}
	binDir := t.TempDir()
	outFile := filepath.Join(t.TempDir(), "captured.txt")
	fakeKimi := filepath.Join(binDir, "kimi")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "2.8.1"
  exit 0
fi
printf 'ARGV:' > %q
for arg in "$@"; do
  printf '%%s|' "$arg" >> %q
done
printf '\nKIMI_CODE_HOME=%%s\nUNEXPECTED_AMBIENT_SECRET=%%s\n' "$KIMI_CODE_HOME" "$UNEXPECTED_AMBIENT_SECRET" >> %q
`, outFile, outFile, outFile)
	if err := os.WriteFile(fakeKimi, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	home := filepath.Join(t.TempDir(), "environments", "default", "kimi_code")
	rawFrag, err := json.Marshal(map[string]any{
		"fragment":    fragment.IdentityV4,
		"environment": fragment.EnvKimiCode,
		"profile":     map[string]any{"name": "default", "lock_sha256": strings.Repeat("b", 64)},
		"precedence":  map[string]any{"winner": "higher-weight", "placement": "winner-last"},
		"permissions": map[string]any{"mode": "native", "locked": false, "source": "default"},
		"env": map[string]any{
			"KIMI_CODE_HOME": home,
		},
		"credential_store": map[string]any{"selection": "native"},
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

	// Alias `kimi` resolves to `kimi_code`, applies --model and --yolo, and sets KIMI_CODE_HOME.
	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{
		"kimi", "--native", "--permissions", "yolo", "--model", "kimi-code/k3.1-pro", "--", "--continue",
	}, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("run(kimi --permissions yolo) = %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}
	if resolver.lastID != fragment.EnvKimiCode {
		t.Fatalf("resolver received EnvID %q, want %q", resolver.lastID, fragment.EnvKimiCode)
	}
	captured, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read captured launch: %v", err)
	}
	text := string(captured)
	wantPrefix := "ARGV:--model|kimi-code/k3.1-pro|--yolo|--continue|\n"
	if !strings.HasPrefix(text, wantPrefix) {
		t.Fatalf("kimi argv mismatch:\n got: %q\nwant prefix: %q", text, wantPrefix)
	}
	if !strings.Contains(text, "KIMI_CODE_HOME="+home+"\n") {
		t.Fatalf("KIMI_CODE_HOME missing or wrong:\n%s", text)
	}

	// Out-of-range kimi release (3.0.0) must be refused before spawn.
	unverifiedDir := t.TempDir()
	unverifiedKimi := filepath.Join(unverifiedDir, "kimi")
	unverifiedScript := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo \"3.0.0\"; exit 0; fi\necho RAN > %q\n", outFile)
	if err := os.WriteFile(unverifiedKimi, []byte(unverifiedScript), 0o755); err != nil {
		t.Fatal(err)
	}
	unverifiedDeps := deps
	unverifiedDeps.environ = func() []string {
		return []string{"PATH=" + unverifiedDir + string(os.PathListSeparator) + "/usr/bin:/bin"}
	}
	_ = os.Remove(outFile)
	var outPin, errPin strings.Builder
	if codePin := run(context.Background(), []string{"kimi", "--native", "--permissions", "yolo"}, &outPin, &errPin, unverifiedDeps); codePin == 0 {
		t.Fatalf("kimi --native on unverified 3.0.0 unexpectedly succeeded\nstderr: %s", errPin.String())
	}
	if _, err := os.Stat(outFile); !os.IsNotExist(err) {
		t.Fatal("child executed despite unverified kimi release")
	}

	// Custom moonshot model from catalog.toml launches seamlessly on kimi.
	const catalogTOML = `
schema_version = 1

[vendors.moonshot.models."kimi-code/k4-preview"]
description = "Operator-configured Kimi K4 Preview"
systems = ["kimi"]
[vendors.moonshot.models."kimi-code/k4-preview".effort]
support = "none"
`
	cat, err := runtimecatalog.ParseModelCatalog([]byte(catalogTOML))
	if err != nil {
		t.Fatalf("ParseModelCatalog: %v", err)
	}
	catReg, err := defaults.NewRegistryWithCatalog(agySystem.Runtime{}, cat)
	if err != nil {
		t.Fatalf("NewRegistryWithCatalog: %v", err)
	}
	catDeps := deps
	catDeps.registry = catReg
	_ = os.Remove(outFile)
	var outCat, errCat strings.Builder
	if codeCat := run(context.Background(), []string{
		"kimi", "--native", "--permissions", "yolo", "--model", "kimi-code/k4-preview",
	}, &outCat, &errCat, catDeps); codeCat != 0 {
		t.Fatalf("kimi with catalog model failed (%d):\nstderr: %s", codeCat, errCat.String())
	}
	catCaptured, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read catalog captured launch: %v", err)
	}
	if !strings.HasPrefix(string(catCaptured), "ARGV:--model|kimi-code/k4-preview|--yolo|\n") {
		t.Fatalf("catalog kimi argv = %q", string(catCaptured))
	}
}
