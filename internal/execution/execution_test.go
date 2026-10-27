package execution_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

var helper, driver, python string

func TestMain(m *testing.M) {
	python, _ = exec.LookPath("python3")
	dir, err := os.MkdirTemp("", "execution-helpers-")
	if err != nil {
		panic(err)
	}
	helper = filepath.Join(dir, "fake-provider")
	cmd := exec.Command("go", "build", "-o", helper, "./testdata/helper")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		os.RemoveAll(dir)
		panic(err)
	}
	driver = filepath.Join(dir, "api-driver")
	build := exec.Command("go", "build", "-o", driver, "./testdata/driver")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err = build.Run(); err != nil {
		os.RemoveAll(dir)
		panic(err)
	}
	// All subprocess environment assertions use synthetic data only.
	os.Clearenv()
	os.Setenv("PATH", dir)
	os.Setenv("TEST_PARENT", "synthetic-parent")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func equal(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}
func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func fixture(t *testing.T, env string, modules bool) (fragment.Fragment, string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "mcp.json")
	write(t, path, []byte("{}"))
	obj := map[string]any{"fragment": fragment.Identity, "environment": env, "profile": map[string]any{"name": "test-profile", "lock_sha256": strings.Repeat("b", 64)}, "precedence": map[string]any{"winner": "higher-weight", "placement": "winner-last"}, "env": map[string]string{fragment.HomeVariable(env): dir}}
	if env != "pi" {
		ch := map[string]any{"kind": "flag", "flag": "--mcp-config", "argument": "path", "with": []string{"--strict-mcp-config"}}
		if env == "codex_cli" {
			ch = map[string]any{"kind": "flag", "flag": "-p", "argument": "name", "name": "curator-mcp"}
		}
		obj["mcp"] = map[string]any{"path": path, "env_names": []string{"FIGMA_API_KEY"}, "channels": []any{ch}}
	}
	if modules {
		channels := []any{map[string]any{"kind": "flag", "semantics": "append", "flag": "--append-system-prompt-file", "argument": "path"}, map[string]any{"kind": "flag", "semantics": "replace", "flag": "--system-prompt-file", "argument": "path"}}
		if env == "codex_cli" {
			channels = []any{map[string]any{"kind": "config-key", "semantics": "replace", "key": "model_instructions_file"}}
		}
		if env == "pi" {
			channels = []any{map[string]any{"kind": "flag", "semantics": "append", "flag": "--append-system-prompt", "argument": "path"}, map[string]any{"kind": "file", "semantics": "append", "filename": "APPEND_SYSTEM.md"}, map[string]any{"kind": "file", "semantics": "replace", "filename": "SYSTEM.md"}}
		}
		obj["system_prompt"] = map[string]any{"path": filepath.Join(dir, "prompt.md"), "channels": channels}
	}
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	f, err := fragment.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return *f, dir
}

func prepare(t *testing.T, v composition.Value) execution.Launch {
	t.Helper()
	return execution.Prepare(v)
}

type capture struct {
	Argv, Env []string
	Stdin     []byte
	WorkDir   string
}

func run(t *testing.T, l execution.Launch, b execution.Boundary) (int, capture, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	code := l.Run(execution.Options{Boundary: b, IO: execution.IO{Stdin: strings.NewReader("terminal input"), Stdout: &out, Stderr: &stderr}})
	var c capture
	if out.Len() > 0 {
		if err := json.Unmarshal(out.Bytes(), &c); err != nil {
			t.Fatalf("child output: %q: %v", out.Bytes(), err)
		}
	}
	return code, c, stderr.String()
}
func compose(t *testing.T, f fragment.Fragment, dir string, stdin agentic.StdinPayload) composition.Value {
	t.Helper()
	// Execution consumes context argv already constructed in an admitted plan.
	args := []string{"first plugin argument", "--model", "model with spaces", "prompt channel"}
	if f.Environment == "claude_code" {
		args = append(args, "--mcp-config", f.MCP.Path, "--strict-mcp-config")
	}
	if f.Environment == "codex_cli" {
		args = append(args, "-p", "curator-mcp")
	}
	v, err := composition.Compose(agentic.Plan{Binary: helper, WorkDir: dir, Argv: args, Env: []string{"FIGMA_API_KEY=inherited-secret", "OWN=old", "PARENT=only-direct"}, Stdin: stdin}, []string{"OWN=old"}, f, composition.PromptApplication{Env: map[string]string{"OWN": "override"}}, []string{"", "-p", "native", "--dangerously-skip-permissions", "a\nb"})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// Native children receive the exact composed argv, environment and each attached/unattached stdin shape.
func TestRealProcessMatrix(t *testing.T) {
	for _, env := range []string{"claude_code", "codex_cli", "pi"} {

		for _, payload := range []struct {
			name     string
			attached bool
			data     []byte
		}{{"unattached", false, nil}, {"empty", true, []byte{}}, {"utf8", true, []byte("բարեւ\n")}, {"binary", true, []byte{0, 255, 128}}} {
			for _, modules := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/modules=%v", env, payload.name, modules), func(t *testing.T) {
					f, dir := fixture(t, env, modules)
					v := compose(t, f, dir, agentic.StdinPayload{Attached: payload.attached, Bytes: payload.data})
					l := prepare(t, v)
					calls := 0
					code, c, stderr := run(t, l, func() error { calls++; return nil })
					equal(t, code, 0)
					equal(t, calls, 1)
					equal(t, c.WorkDir, dir)
					equal(t, stderr, "curator-run: environment override: OWN\nhelper stderr\n")
					wantArgs := []string{"first plugin argument", "--model", "model with spaces", "prompt channel"}
					if env == "claude_code" {
						wantArgs = append(wantArgs, "--mcp-config", f.MCP.Path, "--strict-mcp-config")
					}
					if env == "codex_cli" {
						wantArgs = append(wantArgs, "-p", "curator-mcp")
					}
					wantArgs = append(wantArgs, "", "-p", "native", "--dangerously-skip-permissions", "a\nb")

					equal(t, c.Argv, wantArgs)
					sort.Strings(c.Env)
					wantEnv := []string{fragment.HomeVariable(env) + "=" + dir, "FIGMA_API_KEY=inherited-secret", "OWN=override", "PARENT=only-direct"}
					sort.Strings(wantEnv)
					equal(t, c.Env, wantEnv)
					input := payload.data
					if !payload.attached {
						input = []byte("terminal input")
					}
					if len(input) == 0 {
						input = []byte{}
					}
					equal(t, c.Stdin, input)

				})
			}
		}

	}
}

// Empty environments stay empty; executable lookup uses the composed PATH or explicit relative path.
func TestEmptyEnvironmentAndLaunchPATH(t *testing.T) {
	_, dir := fixture(t, "pi", false)
	for _, kind := range []string{"empty", "nil", "path", "relative"} {
		t.Run(kind, func(t *testing.T) {
			v := composition.Value{Binary: helper, WorkDir: dir, Argv: []string{}, Env: []string{}, RawStdin: agentic.StdinPayload{Attached: true}}
			if kind == "nil" {
				v.Env = nil
			}
			if kind == "path" {
				v.Binary = filepath.Base(helper)
				v.Env = []string{"PATH=" + filepath.Dir(helper)}
			}
			if kind == "relative" {
				if err := os.Symlink(helper, filepath.Join(dir, "local-provider")); err != nil {
					t.Fatal(err)
				}
				v.Binary = "./local-provider"
			}
			code, c, stderr := run(t, prepare(t, v), func() error { return nil })
			equal(t, code, 0)
			equal(t, stderr, "helper stderr\n")
			expected := v.Env
			if expected == nil {
				expected = []string{}
			}
			equal(t, c.Env, expected)
		})
	}
}

// Each missing or invalid late input refuses its specific code without starting the provider.
func TestLateChecks(t *testing.T) {

	for _, kind := range []string{"nil_boundary", "boundary_refusal", "provider_removed", "provider_nonexec", "provider_PATH_missing", "mcp_removed", "mcp_dangling", "mcp_directory", "mcp_unreadable"} {
		t.Run(kind, func(t *testing.T) {
			f, dir := fixture(t, "codex_cli", false)
			v := compose(t, f, dir, agentic.StdinPayload{})
			copyPath := filepath.Join(dir, "provider")
			data, err := os.ReadFile(helper)
			if err != nil {
				t.Fatal(err)
			}
			write(t, copyPath, data)
			os.Chmod(copyPath, 0700)
			v.Binary = copyPath
			if kind == "provider_PATH_missing" {
				v.Binary = "fake-provider-not-on-PATH"
			}
			l := prepare(t, v)
			boundary := execution.Boundary(func() error { return nil })
			want := ""
			switch kind {
			case "nil_boundary":
				boundary = nil
				want = "launch_boundary_required"
			case "boundary_refusal":
				boundary = func() error { return errors.New("prompt_file_detected: late file") }
				want = "prompt_file_detected"
			case "provider_removed":
				os.Remove(copyPath)
				want = "exec_provider_missing"
			case "provider_nonexec":
				os.Chmod(copyPath, 0600)
				want = "exec_provider_missing"
			case "provider_PATH_missing":
				want = "exec_provider_missing"
			case "mcp_removed":
				os.Remove(f.MCP.Path)
				want = "mcp_layer_missing"
			case "mcp_dangling":
				os.Remove(f.MCP.Path)
				os.Symlink(filepath.Join(dir, "missing"), f.MCP.Path)
				want = "mcp_layer_unreadable"
			case "mcp_directory":
				os.Remove(f.MCP.Path)
				os.Mkdir(f.MCP.Path, 0700)
				want = "mcp_layer_unreadable"
			case "mcp_unreadable":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permission refusal")
				}
				os.Chmod(f.MCP.Path, 0000)
				defer os.Chmod(f.MCP.Path, 0600)
				want = "mcp_layer_unreadable"
			}
			code, c, stderr := run(t, l, boundary)
			equal(t, code, 1)
			equal(t, c, capture{})
			if !strings.Contains(stderr, want) {
				t.Fatalf("missing %s: %q", want, stderr)
			}
		})
	}

}

// TestPrelaunchCheckOrder pins SPEC §4.6: the binary check runs first,
// then the §4.5 codex layer stat, then the §5 boundary (the §5.1
// file-kind probe), and the first failure is the one reported — exactly
// one diagnostic code line on stderr. All three failures provoked at
// once report the binary check.
func TestPrelaunchCheckOrder(t *testing.T) {

	for _, tc := range []struct {
		name                    string
		breakBinary, breakLayer bool
		breakBoundary           bool
		want                    string
	}{
		{"all three fail", true, true, true, "exec_provider_missing"},
		{"stat before probe", false, true, true, "mcp_layer_missing"},
		{"probe last", false, false, true, "sysprompt_file_unreadable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, dir := fixture(t, "codex_cli", false)
			v := compose(t, f, dir, agentic.StdinPayload{})
			copyPath := filepath.Join(dir, "provider")
			data, err := os.ReadFile(helper)
			if err != nil {
				t.Fatal(err)
			}
			write(t, copyPath, data)
			os.Chmod(copyPath, 0700)
			v.Binary = copyPath
			if tc.breakBinary {
				os.Remove(copyPath)
			}
			if tc.breakLayer {
				os.Remove(f.MCP.Path)
			}
			boundary := execution.Boundary(func() error { return nil })
			if tc.breakBoundary {
				boundary = func() error { return errors.New("sysprompt_file_unreadable: \"SYSTEM.md\": probe refused") }
			}
			code, c, stderr := run(t, prepare(t, v), boundary)
			equal(t, code, 1)
			equal(t, c, capture{})
			lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
			found := 0
			for _, line := range lines {
				if diagnostics.IsDiagnosticLine(line) {
					found++
					if !strings.HasPrefix(line, "curator-run: "+tc.want+": ") {
						t.Fatalf("diagnostic line %q does not carry code %q", line, tc.want)
					}
				}
			}
			if found != 1 {
				t.Fatalf("stderr has %d diagnostic lines, want exactly 1: %q", found, stderr)
			}
		})
	}

}

// Native execution preserves child exit codes and signal-derived statuses.
func TestDirectExitAndSignal(t *testing.T) {
	for _, behavior := range []string{"exit:23", "signal"} {
		t.Run(behavior, func(t *testing.T) {
			f, dir := fixture(t, "pi", false)
			write(t, filepath.Join(dir, "behavior"), []byte(behavior))
			v := compose(t, f, dir, agentic.StdinPayload{})
			code, _, stderr := run(t, prepare(t, v), func() error { return nil })
			want := 23
			if behavior == "signal" {
				want = 143
				equal(t, stderr, "curator-run: environment override: OWN\n")
			} else {
				equal(t, stderr, "curator-run: environment override: OWN\n{\"code\":\"launch_plan_invalid\"}\n\x00raw\xff")
			}
			equal(t, code, want)
		})
	}
}

// A parent-directed TERM reaches the owned child group and preserves its signal exit.
func TestForwardedSignal(t *testing.T) {

	t.Run("native", func(t *testing.T) {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "behavior"), []byte("waitsignal"))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, driver, helper, dir)
		cmd.Env = []string{}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err = cmd.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = cmd.Process.Kill() }()
		reader := bufio.NewReader(stdout)
		if _, err = reader.ReadString('\n'); err != nil {
			t.Fatal(err)
		}
		ready, err := reader.ReadString('\n')
		if err != nil || ready != "READY\n" {
			t.Fatal(ready, err)
		}
		if err = cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		err = cmd.Wait()
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatal(err)
		}
		want := 143
		message := ""

		equal(t, exit.ExitCode(), want)
		equal(t, stderr.String(), message)
	})

}

// Literal values win lookup collisions and warnings disclose names without values.
func TestLookupCollisionWarnings(t *testing.T) {

	t.Run("native", func(t *testing.T) {
		f, dir := fixture(t, "claude_code", false)
		data := bytes.Replace(f.Canonical, []byte(`"FIGMA_API_KEY"`), []byte(`"OWN"`), 1)
		parsed, err := fragment.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		f = *parsed
		v := compose(t, f, dir, agentic.StdinPayload{})
		code, _, stderr := run(t, prepare(t, v), func() error { return nil })
		equal(t, code, 0)
		equal(t, stderr, "curator-run: environment override: OWN\ncurator-run: environment literal replaces lookup: OWN\nhelper stderr\n")
	})

}

// TestRealPTYOwnership exercises the actual terminal line discipline,
// not a PID-only signal or an injected executor. Python supplies POSIX PTYs.
func TestRealPTYOwnership(t *testing.T) {
	if python == "" {
		t.Fatal("python3 required for real PTY regression")
	}
	for _, mode := range []string{"native"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(python, "testdata/terminal_probe.py", driver)
			cmd.Env = []string{"PATH=/usr/bin:/bin"}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("PTY regression: %v\n%s", err, out)
			}
			t.Log(string(out))
		})
	}
}

// TestStdinIsTerminal pins the hosted stdin gate's input check: only a
// terminal file reports true; nil, non-files, and regular files are not
// terminals.
func TestStdinIsTerminal(t *testing.T) {
	if execution.StdinIsTerminal(nil) {
		t.Fatal("nil reader is a terminal")
	}
	if execution.StdinIsTerminal(strings.NewReader("x")) {
		t.Fatal("strings reader is a terminal")
	}
	f, err := os.Create(filepath.Join(t.TempDir(), "regular"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if execution.StdinIsTerminal(f) {
		t.Fatal("regular file is a terminal")
	}
}

// Prepare freezes mutable argv, env, stdin and warnings; later caller edits do
// not reach the child, while the late boundary remains required and executes.
func TestPrepareSnapshot(t *testing.T) {
	_, dir := fixture(t, "pi", false)
	v := composition.Value{Binary: helper, WorkDir: dir, Argv: []string{"original"}, Env: []string{"OWN=original"}, RawStdin: agentic.StdinPayload{Attached: true, Bytes: []byte("original")}, Warnings: []string{"original warning"}}
	l := execution.Prepare(v)
	v.Argv[0] = "changed"
	v.Env[0] = "OWN=changed"
	v.RawStdin.Bytes[0] = 'X'
	v.Warnings[0] = "changed warning"
	calls := 0
	code, c, stderr := run(t, l, func() error { calls++; return nil })
	equal(t, code, 0)
	equal(t, calls, 1)
	equal(t, c.Argv, []string{"original"})
	equal(t, c.Env, []string{"OWN=original"})
	equal(t, c.Stdin, []byte("original"))
	equal(t, stderr, "curator-run: original warning\nhelper stderr\n")
}
