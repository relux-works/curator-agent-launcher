package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

func configHomeFixture(t *testing.T) (launchDeps, *scriptedRunner, configfile.Locations) {
	t.Helper()
	root := t.TempDir()
	loc := configfile.Locations{System: filepath.Join(root, "system"), User: filepath.Join(root, "user"), LegacySystem: filepath.Join(root, "old-system"), LegacyUser: filepath.Join(root, "old-user")}
	for _, dir := range []string{loc.System, loc.User, loc.LegacySystem, loc.LegacyUser} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	runner := &scriptedRunner{stdout: piFragmentLine}
	deps := testDeps(t, fragment.NewWithRunner("curator", runner))
	deps.configLocations = func() (configfile.Locations, error) { return loc, nil }
	deps.configPaths = nil
	deps.axPaths = nil
	return deps, runner, loc
}
func writeConfig(t *testing.T, dir, name, data string) {
	t.Helper()
	writeFixture(t, filepath.Join(dir, name), []byte(data), 0600)
}

// Production dispatch uses one resolver for all three files, uses machine
// policy for native routing, and never reads shadowed legacy files.
func TestRunSharedConfigHome(t *testing.T) {
	for _, shape := range []string{"new", "fallback", "both"} {
		t.Run(shape, func(t *testing.T) {
			deps, runner, loc := configHomeFixture(t)
			machine, user := loc.System, loc.User
			if shape == "fallback" {
				machine, user = loc.LegacySystem, loc.LegacyUser
			}
			writeConfig(t, machine, "launcher.json", `{"schema":"curator-run-launcher-v1","host_mode":"native"}`)
			writeConfig(t, user, "ax.json", `{"schema":"curator-run-ax-v1","enabled":false}`)
			writeConfig(t, user, "defaults.json", `{"schema":"curator-run-defaults-v3","defaults":{"pi":{"model":"claude-fable-5","effort":"high","permissions":"native"}}}`)
			if shape == "both" {
				for _, row := range []struct{ dir, name string }{{loc.LegacySystem, "launcher.json"}, {loc.LegacyUser, "ax.json"}, {loc.LegacyUser, "defaults.json"}} {
					if err := os.Symlink(filepath.Join(row.dir, "missing"), filepath.Join(row.dir, row.name)); err != nil {
						t.Fatal(err)
					}
				}
			}
			var out, stderr strings.Builder
			code := run(context.Background(), []string{"pi"}, &out, &stderr, deps)
			if code != 1 || runner.calls != 1 || !strings.Contains(stderr.String(), "plan_refused: early-stage fixture") || !strings.Contains(stderr.String(), "model=claude-fable-5 (operator) effort=high (operator)") {
				t.Fatalf("exit=%d calls=%d stderr=%s", code, runner.calls, &stderr)
			}
			want := 0
			if shape == "fallback" {
				want = 3
			}
			if strings.Count(stderr.String(), "deprecated config path") != want {
				t.Fatal(stderr.String())
			}
		})
	}
}

// Machine policy errors refuse a launch before resolver contact, while version
// stays static and answers on the same broken policy (CLI review F3);
// removing the forbidden user member lets the launch reach its next stage.
func TestRunLauncherPolicyBeforeVersion(t *testing.T) {
	deps, runner, loc := configHomeFixture(t)
	writeConfig(t, loc.User, "launcher.json", `{"schema":"curator-run-launcher-v1","host_mode":"native"}`)
	var out, stderr strings.Builder
	if code := run(context.Background(), []string{"--version"}, &out, &stderr, deps); code != 0 || runner.calls != 0 || stderr.Len() != 0 || !strings.Contains(out.String(), buildVersion) {
		t.Fatalf("version on broken policy: exit=%d stderr=%s", code, &stderr)
	}
	out.Reset()
	code := run(context.Background(), []string{"claude_code"}, &out, &stderr, deps)
	if code != 1 || runner.calls != 0 || out.Len() != 0 || !strings.Contains(stderr.String(), "defaults_config_invalid") || !strings.Contains(stderr.String(), filepath.Join(loc.System, "launcher.json")) {
		t.Fatalf("exit=%d stderr=%s", code, &stderr)
	}
	writeConfig(t, loc.User, "launcher.json", `{"schema":"curator-run-launcher-v1"}`)
	stderr.Reset()
	if code := run(context.Background(), []string{"--version"}, &out, &stderr, deps); code != 0 || runner.calls != 0 || stderr.Len() != 0 || !strings.Contains(out.String(), buildVersion) {
		t.Fatalf("exit=%d stderr=%s", code, &stderr)
	}
}

// Migration bypasses normal config parsing, so withdrawn host and malformed ax
// cannot prevent repair; destination conflicts still exit 2 before any writes.
func TestRunMigrateReservedDispatch(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "copy", true: "conflict"}[conflict], func(t *testing.T) {
			deps, runner, loc := configHomeFixture(t)
			raw := `{"schema":"curator-run-defaults-v3","defaults":{"pi":{"host":"native"}}}`
			writeConfig(t, loc.LegacyUser, "defaults.json", raw)
			writeConfig(t, loc.LegacyUser, "ax.json", `invalid`)
			if conflict {
				writeConfig(t, loc.User, "defaults.json", "different")
			}
			var out, stderr strings.Builder
			code := run(context.Background(), []string{"migrate", "--dry-run"}, &out, &stderr, deps)
			want := 0
			if conflict {
				want = 2
			}
			if code != want || runner.calls != 0 {
				t.Fatalf("exit=%d stderr=%s", code, &stderr)
			}
			if _, err := os.Stat(filepath.Join(loc.User, "ax.json")); !os.IsNotExist(err) {
				t.Fatal("dry run wrote", err)
			}
			if conflict {
				if !strings.Contains(stderr.String(), "migrate_destination_exists") {
					t.Fatal(stderr.String())
				}
				return
			}
			code = run(context.Background(), []string{"migrate"}, &out, &stderr, deps)
			if code != 0 || runner.calls != 0 {
				t.Fatalf("exit=%d stderr=%s", code, &stderr)
			}
			data, err := os.ReadFile(filepath.Join(loc.User, "defaults.json"))
			if err != nil || strings.Contains(string(data), `"host"`) {
				t.Fatal(string(data), err)
			}
			backup, err := os.ReadFile(filepath.Join(loc.User, "defaults.json.bak"))
			if err != nil || string(backup) != raw {
				t.Fatal("backup mismatch", err)
			}
		})
	}
}
