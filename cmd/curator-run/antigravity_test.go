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
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
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

	// Conflicting native flags (--gemini_dir, --cli_dir_name, --settings) must be refused before spawn (a1).
	for _, tail := range [][]string{
		{"--gemini_dir", "/tmp/evil"},
		{"--cli_dir_name", ".other"},
		{"--settings", "/tmp/settings.json"},
	} {
		_ = os.Remove(outFile)
		var out2, err2 strings.Builder
		argv := append([]string{"antigravity", "--native", "--"}, tail...)
		code2 := run(context.Background(), argv, &out2, &err2, deps)
		if code2 == 0 {
			t.Fatalf("conflicting native %v unexpectedly succeeded\nstderr: %s", tail, err2.String())
		}
		if _, err := os.Stat(outFile); !os.IsNotExist(err) {
			t.Fatalf("child executed despite conflicting %v", tail)
		}
	}

	// Non-yolo Curator-managed launches also enforce the verified agy release pin (a2).
	unverifiedDir := t.TempDir()
	unverifiedAgy := filepath.Join(unverifiedDir, "agy")
	unverifiedScript := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo \"1.6.0\"; exit 0; fi\necho RAN > %q\n", outFile)
	if err := os.WriteFile(unverifiedAgy, []byte(unverifiedScript), 0o755); err != nil {
		t.Fatal(err)
	}
	unverifiedDeps := deps
	unverifiedDeps.environ = func() []string {
		return []string{"PATH=" + unverifiedDir + string(os.PathListSeparator) + "/usr/bin:/bin"}
	}
	_ = os.Remove(outFile)
	var outPin, errPin strings.Builder
	if codePin := run(context.Background(), []string{"agy", "--native"}, &outPin, &errPin, unverifiedDeps); codePin == 0 {
		t.Fatalf("agy --native on unverified 1.6.0 unexpectedly succeeded\nstderr: %s", errPin.String())
	}
	if _, err := os.Stat(outFile); !os.IsNotExist(err) {
		t.Fatal("child executed despite unverified agy release")
	}

	// Hosted route with a selected session manager must refuse antigravity with
	// session_host_provider_unsupported before manager contact or child spawn (l2).
	mgrData, err := sessionmanager.Encode(sessionmanager.Registry{
		Schema:   sessionmanager.Schema,
		Selected: "stub",
		Managers: map[string]sessionmanager.Entry{
			"stub": {
				Executable:   fakeAgy,
				ArgvPrefix:   []string{},
				Protocol:     sessionmanager.Protocol,
				RegisteredAt: "2026-01-01T00:00:00Z",
				RegisteredBy: "test fixture",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	usrCfgDir := filepath.Join(cfgDir, "usr")
	if err := os.MkdirAll(usrCfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(usrCfgDir, sessionmanager.Filename), mgrData, 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(outFile)
	var out3, err3 strings.Builder
	code3 := run(context.Background(), []string{"agy", "--hosted", "--detach"}, &out3, &err3, deps)
	if code3 == 0 {
		t.Fatalf("agy --hosted --detach unexpectedly succeeded\nstderr: %s", err3.String())
	}
	if !strings.Contains(err3.String(), "session_host_provider_unsupported") {
		t.Fatalf("agy --hosted --detach stderr = %q, want session_host_provider_unsupported", err3.String())
	}
	if _, err := os.Stat(outFile); !os.IsNotExist(err) {
		t.Fatal("child or stub manager executed on refused hosted route")
	}
}
