package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

const (
	securityDefaultsJSON = `{"schema":"curator-run-defaults-v1","defaults":{"pi":{"model":"claude-fable-5"}}}`
	securityAXJSON       = `{"schema":"curator-run-ax-v1","enabled":true}`
)

type securityFiles struct {
	paths                   defaults.Paths
	machineDir, operatorDir string
}

func securityFixture(t *testing.T) (launchDeps, *scriptedRunner, securityFiles) {
	t.Helper()
	root := t.TempDir()
	machineDir := filepath.Join(root, "machine")
	operatorDir := filepath.Join(root, "operator")
	for _, dir := range []string{machineDir, operatorDir} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	files := securityFiles{
		paths: defaults.Paths{
			Machine:  filepath.Join(machineDir, "defaults.json"),
			Operator: filepath.Join(operatorDir, "defaults.json"),
		},
		machineDir:  machineDir,
		operatorDir: operatorDir,
	}
	runner := &scriptedRunner{stdout: fragmentLineFor("pi")}
	deps := testDeps(t, fragment.NewWithRunner("curator", runner))
	deps.defaults = files.paths
	deps.configPaths = func() (defaults.Paths, error) { return files.paths, nil }
	deps.axPaths = func() (defaults.Paths, error) { return files.paths, nil }
	return deps, runner, files
}

func securityPath(files securityFiles, target string) string {
	switch target {
	case "machine-defaults":
		return files.paths.Machine
	case "operator-defaults":
		return files.paths.Operator
	case "machine-ax":
		return filepath.Join(files.machineDir, "ax.json")
	case "operator-ax":
		return filepath.Join(files.operatorDir, "ax.json")
	default:
		panic("unknown security fixture target: " + target)
	}
}

func securityContents(target string) string {
	if strings.HasSuffix(target, "-ax") {
		return securityAXJSON
	}
	return securityDefaultsJSON
}

func runSecurityInvocation(t *testing.T, deps launchDeps, runner *scriptedRunner, wantCalls int, wantReason string) {
	t.Helper()
	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{"pi"}, &stdout, &stderr, deps)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q; want a terminal configuration refusal", code, stdout.String(), stderr.String())
	}
	assertSingleDiagnostic(t, stderr.String(), defaults.CodeInvalid)
	if !strings.Contains(stderr.String(), wantReason) {
		t.Fatalf("stderr %q does not name %q", stderr.String(), wantReason)
	}
	if runner.calls != wantCalls {
		t.Fatalf("fragment resolver calls=%d, want %d (stderr %q)", runner.calls, wantCalls, stderr.String())
	}
}

func TestRunConfigSecurityGoldens(t *testing.T) {
	for _, tc := range []struct {
		name, target, kind, golden, reason string
		mode                               os.FileMode
		calls                              int
	}{
		{"symlinked-operator-defaults", "operator-defaults", "symlink", "config-symlink.golden", "symlinked configuration file", 0, 1},
		{"group-writable-operator-defaults", "operator-defaults", "mode", "config-group-writable.golden", "group/world-writable configuration file", 0o620, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.kind == "mode" && runtime.GOOS == "windows" {
				t.Skip("POSIX group mode bits are not Windows DACLs")
			}
			deps, runner, files := securityFixture(t)
			path := securityPath(files, tc.target)
			switch tc.kind {
			case "symlink":
				targetPath := path + ".target"
				if err := os.WriteFile(targetPath, []byte(securityContents(tc.target)), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(targetPath, path); err != nil {
					t.Skipf("cannot create a symlink on this host: %v", err)
				}
			case "mode":
				if err := os.WriteFile(path, []byte(securityContents(tc.target)), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, tc.mode); err != nil {
					t.Fatal(err)
				}
			}

			var stdout, stderr strings.Builder
			code := run(context.Background(), []string{"pi"}, &stdout, &stderr, deps)
			if code != 1 || stdout.Len() != 0 {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			assertSingleDiagnostic(t, stderr.String(), defaults.CodeInvalid)
			if !strings.Contains(stderr.String(), tc.reason) || runner.calls != tc.calls {
				t.Fatalf("resolver calls=%d stderr=%q", runner.calls, stderr.String())
			}
			golden, err := os.ReadFile(filepath.Join("testdata", tc.golden))
			if err != nil {
				t.Fatal(err)
			}
			want := strings.ReplaceAll(string(golden), "$CONFIG_PATH", path)
			if stderr.String() != want {
				t.Fatalf("stderr differs from %s\ngot:  %q\nwant: %q", tc.golden, stderr.String(), want)
			}
		})
	}
}

func TestRunConfigSecurityRefusalRows(t *testing.T) {
	cases := []struct {
		target, kind string
		wantCalls    int
	}{
		{"machine-defaults", "symlink", 1},
		{"operator-defaults", "symlink", 1},
		{"machine-ax", "symlink", 0},
		{"operator-ax", "symlink", 0},
		{"machine-defaults", "group-write", 1},
		{"operator-defaults", "group-write", 1},
		{"machine-ax", "group-write", 0},
		{"operator-ax", "group-write", 0},
		{"machine-defaults", "world-write", 1},
		{"operator-defaults", "world-write", 1},
		{"machine-ax", "world-write", 0},
		{"operator-ax", "world-write", 0},
		{"machine-defaults", "unreadable", 1},
		{"operator-defaults", "unreadable", 1},
		{"machine-ax", "unreadable", 0},
		{"operator-ax", "unreadable", 0},
	}
	for _, tc := range cases {
		t.Run(tc.target+"/"+tc.kind, func(t *testing.T) {
			if (tc.kind == "group-write" || tc.kind == "world-write" || tc.kind == "unreadable") && runtime.GOOS == "windows" {
				t.Skip("POSIX mode bits are not Windows DACLs")
			}
			if tc.kind == "unreadable" && currentUserIsRoot() {
				t.Skip("root can read mode-000 files")
			}
			deps, runner, files := securityFixture(t)
			path := securityPath(files, tc.target)
			switch tc.kind {
			case "symlink":
				targetPath := path + ".target"
				if err := os.WriteFile(targetPath, []byte(securityContents(tc.target)), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(targetPath, path); err != nil {
					t.Skipf("cannot create a symlink on this host: %v", err)
				}
			case "group-write", "world-write":
				if err := os.WriteFile(path, []byte(securityContents(tc.target)), 0600); err != nil {
					t.Fatal(err)
				}
				mode := os.FileMode(0o620)
				if tc.kind == "world-write" {
					mode = 0o602
				}
				if err := os.Chmod(path, mode); err != nil {
					t.Fatal(err)
				}
			case "unreadable":
				if err := os.WriteFile(path, []byte(securityContents(tc.target)), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(path, 0600) })
			}
			reason := "configuration file"
			if tc.kind == "symlink" {
				reason = "symlinked configuration file"
			} else if tc.kind == "group-write" || tc.kind == "world-write" {
				reason = "group/world-writable configuration file"
			} else if tc.kind == "unreadable" {
				reason = "configuration file is unreadable"
			}
			runSecurityInvocation(t, deps, runner, tc.wantCalls, reason)
		})
	}
}

func TestRunConfigSecurityRejectsDifferentDirectoryOwner(t *testing.T) {
	path, ok := foreignOwnedConfigFile(t, securityDefaultsJSON)
	if !ok {
		t.Skip("host cannot create a file owned by a different identity than /tmp")
	}
	for _, target := range []string{"machine", "operator"} {
		t.Run(target, func(t *testing.T) {
			deps, runner, files := securityFixture(t)
			if target == "machine" {
				files.paths.Machine = path
			} else {
				files.paths.Operator = path
			}
			deps.defaults = files.paths
			deps.configPaths = func() (defaults.Paths, error) { return files.paths, nil }
			runSecurityInvocation(t, deps, runner, 1, "configuration file is owned by a different identity")
		})
	}
}

func TestRunConfigSecurityHappyPath(t *testing.T) {
	f := entryFixture(t, "pi", true)
	f.writeDefaults(t, f.deps.defaults.Machine, `{"schema":"curator-run-defaults-v1","defaults":{"pi":{"model":"claude-opus-5","effort":"high"}}}`)
	f.writeDefaults(t, f.deps.defaults.Operator, `{"schema":"curator-run-defaults-v1","defaults":{"pi":{"model":"claude-fable-5"}}}`)
	f.writeDefaults(t, filepath.Join(filepath.Dir(f.deps.defaults.Machine), "ax.json"), securityAXJSON)
	f.args = []string{"pi"}
	code, out, stderr := f.run()
	if code != 0 || len(out) == 0 || f.resolver.calls != 1 || f.builds != 1 {
		t.Fatalf("exit=%d resolver=%d builds=%d stdout=%s stderr=%s", code, f.resolver.calls, f.builds, out, stderr)
	}
	if !strings.Contains(string(stderr), "model=claude-fable-5 (operator) effort=high (machine)") {
		t.Fatalf("machine/operator defaults did not reach the production resolver: %s", stderr)
	}
}
