package execution_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
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

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/mapping"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

var helper, driver, python string

func TestMain(m *testing.M) {
	python, _ = exec.LookPath("python3")
	dir, err := os.MkdirTemp("", "execution-helpers-")
	if err != nil {
		panic(err)
	}
	helper = filepath.Join(dir, "fake-ax-provider")
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
	os.Setenv("AX_PARENT", "synthetic-parent")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type owner []string

func (o owner) ChildEnv([]string, agentic.LaunchRequest) ([]string, error) { return o, nil }
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

var at = time.Date(2026, 9, 5, 10, 32, 54, 0, time.FixedZone("UTC+3", 10800))

func prepare(t *testing.T, v composition.Value, f fragment.Fragment, tracked, explicit bool) execution.Launch {
	t.Helper()
	args := []string{f.Environment}
	if explicit {
		args = append(args, "--name", "chosen.name")
		if tracked {
			args = append(args, "--ax-profile", "yolo")
		}
	}
	inv, err := cli.Parse(args, cli.Options{AxConfigured: tracked})
	if err != nil {
		t.Fatal(err)
	}
	target, err := mapping.Resolve(f.Environment)
	if err != nil {
		t.Fatal(err)
	}
	l, err := execution.Prepare(v, f, inv, target, at)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

type capture struct {
	Argv, Env []string
	Stdin     []byte
	WorkDir   string
}

func run(t *testing.T, l execution.Launch, b execution.Boundary) (int, capture, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	code := l.Run(execution.Options{AxBinary: helper, Boundary: b, IO: execution.IO{Stdin: strings.NewReader("terminal input"), Stdout: &out, Stderr: &stderr}})
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
	v, err := composition.Compose(agentic.Plan{Binary: helper, WorkDir: dir, Argv: []string{"first plugin argument", "--model", "model with spaces"}, Env: []string{"FIGMA_API_KEY=inherited-secret", "OWN=old", "PARENT=only-direct"}, Stdin: stdin}, owner{"OWN=old"}, agentic.LaunchRequest{}, f, composition.PromptApplication{Argv: []string{"prompt channel"}, Env: map[string]string{"OWN": "override"}}, []string{"", "-p", "native", "--dangerously-skip-permissions", "a\nb"})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func TestRealProcessMatrix(t *testing.T) {
	for _, env := range []string{"claude_code", "codex_cli", "pi"} {
		for _, tracked := range []bool{false, true} {
			for _, payload := range []struct {
				name     string
				attached bool
				data     []byte
			}{{"unattached", false, nil}, {"empty", true, []byte{}}, {"utf8", true, []byte("բարեւ\n")}, {"binary", true, []byte{0, 255, 128}}} {
				for _, explicit := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/tracked=%v/%s/explicit=%v", env, tracked, payload.name, explicit), func(t *testing.T) {
						f, dir := fixture(t, env, explicit)
						v := compose(t, f, dir, agentic.StdinPayload{Attached: payload.attached, Bytes: payload.data})
						l := prepare(t, v, f, tracked, explicit)
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
						if !tracked {
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
						} else {
							name := env + "-20260905T073254Z"
							if explicit {
								name = "chosen.name"
							}
							provider := map[string]string{"claude_code": "claude", "codex_cli": "codex", "pi": "pi"}[env]
							args := []string{"start", name, "--provider", provider, "--launch-plan", "-"}
							if explicit {
								args = append(args, "--profile", "yolo")
							}
							args = append(args, "--workspace", dir)
							equal(t, c.Argv, args)
							var stdin any
							if payload.attached {
								encoding, value := "utf-8", string(payload.data)
								if payload.name == "binary" {
									encoding, value = "base64url", base64.RawURLEncoding.EncodeToString(payload.data)
								}
								stdin = map[string]string{"encoding": encoding, "bytes": value}
							}
							names := []string{}
							if env != "pi" {
								names = []string{"FIGMA_API_KEY"}
							}
							// Independent exact closed D3.2 object, not execution's document type.
							want := map[string]any{"schema": "urn:ax:schema:launch-plan-request", "schema_version": "1.0.0", "argv_suffix": wantArgs, "env_names": names, "env_literals": map[string]string{fragment.HomeVariable(env): dir, "OWN": "override"}, "stdin": stdin, "extensions": map[string]any{"works.relux.curator.profile-name": "test-profile", "works.relux.curator.profile-pin": "sha256:" + strings.Repeat("b", 64), "works.relux.curator.fragment-digest": f.Digest, "works.relux.curator.system-modules": explicit}}
							expected, _ := json.Marshal(want)
							var actual, expectedJSON any
							if err := json.Unmarshal(c.Stdin, &actual); err != nil {
								t.Fatal(err)
							}
							json.Unmarshal(expected, &expectedJSON)
							equal(t, actual, expectedJSON)
							parent := append(os.Environ(), "PWD="+dir)
							sort.Strings(parent)
							sort.Strings(c.Env)
							equal(t, c.Env, parent)
						}
					})
				}
			}
		}
	}
}
func TestEmptyEnvironmentAndLaunchPATH(t *testing.T) {
	f, dir := fixture(t, "pi", false)
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
			code, c, stderr := run(t, prepare(t, v, f, false, false), func() error { return nil })
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
func TestLateChecksBothModes(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		for _, kind := range []string{"nil_boundary", "boundary_refusal", "provider_removed", "provider_nonexec", "provider_PATH_missing", "mcp_removed", "mcp_dangling", "mcp_directory", "mcp_unreadable"} {
			t.Run(fmt.Sprintf("tracked=%v/%s", tracked, kind), func(t *testing.T) {
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
				l := prepare(t, v, f, tracked, false)
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
}
func TestAxFailureNoFallback(t *testing.T) {
	for _, kind := range []string{"nonzero", "missing", "nonexec", "bad_format"} {
		t.Run(kind, func(t *testing.T) {
			f, dir := fixture(t, "pi", false)
			v := compose(t, f, dir, agentic.StdinPayload{})
			l := prepare(t, v, f, true, false)
			ax := helper
			if kind == "nonzero" {
				write(t, filepath.Join(dir, "behavior"), []byte("exit:16"))
			} else {
				ax = filepath.Join(dir, "fake-ax")
				if kind != "missing" {
					write(t, ax, []byte("not executable content"))
					if kind == "bad_format" {
						os.Chmod(ax, 0700)
					}
				}
			}
			var out, stderr bytes.Buffer
			code := l.Run(execution.Options{AxBinary: ax, Boundary: func() error { return nil }, IO: execution.IO{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &stderr}})
			equal(t, code, 1)
			want := "curator-run: environment override: OWN\ncurator-run: ax_handoff_failed: ax could not take the launch\n"
			if kind == "nonzero" {
				want += "{\"code\":\"launch_plan_invalid\"}\n\x00raw\xff"
				var c capture
				if err := json.Unmarshal(out.Bytes(), &c); err != nil {
					t.Fatalf("fallback/multiple processes: %q", out.Bytes())
				}
				equal(t, c.Argv[0], "start")
			} else {
				equal(t, out.Len(), 0)
			}
			equal(t, stderr.String(), want)
		})
	}
}
func TestDirectExitAndSignal(t *testing.T) {
	for _, behavior := range []string{"exit:23", "signal"} {
		t.Run(behavior, func(t *testing.T) {
			f, dir := fixture(t, "pi", false)
			write(t, filepath.Join(dir, "behavior"), []byte(behavior))
			v := compose(t, f, dir, agentic.StdinPayload{})
			code, _, stderr := run(t, prepare(t, v, f, false, false), func() error { return nil })
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

func TestForwardedSignalBothModes(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		t.Run(fmt.Sprint(tracked), func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, "behavior"), []byte("waitsignal"))
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, driver, helper, dir, fmt.Sprint(tracked))
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
			if tracked {
				want = 1
				message = "curator-run: ax_handoff_failed: ax could not take the launch\n"
			}
			equal(t, exit.ExitCode(), want)
			equal(t, stderr.String(), message)
		})
	}
}

func TestStandardProfileAndSnapshot(t *testing.T) {
	f, dir := fixture(t, "claude_code", false)
	v := compose(t, f, dir, agentic.StdinPayload{Attached: true, Bytes: []byte("original")})
	inv, err := cli.Parse([]string{"claude_code", "--name", "named", "--ax-profile", "standard"}, cli.Options{AxConfigured: true})
	if err != nil {
		t.Fatal(err)
	}
	target, err := mapping.Resolve(f.Environment)
	if err != nil {
		t.Fatal(err)
	}
	l, err := execution.Prepare(v, f, inv, target, at)
	if err != nil {
		t.Fatal(err)
	}
	v.Argv[0] = "changed"
	v.Env[0] = "CHANGED=yes"
	v.EnvLiterals["OWN"] = "changed"
	v.RawStdin.Bytes[0] = 'X'
	v.Stdin.Bytes = "changed"
	v.Warnings[0] = "changed"
	code, c, stderr := run(t, l, func() error { return nil })
	equal(t, code, 0)
	equal(t, c.Argv, []string{"start", "named", "--provider", "claude", "--launch-plan", "-", "--profile", "standard", "--workspace", dir})
	equal(t, stderr, "curator-run: environment override: OWN\nhelper stderr\n")
	if bytes.Contains(c.Stdin, []byte("changed")) {
		t.Fatal("prepared document retained input mutations")
	}
}

func TestLookupCollisionWarningsBothModes(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		t.Run(fmt.Sprint(tracked), func(t *testing.T) {
			f, dir := fixture(t, "claude_code", false)
			data := bytes.Replace(f.Canonical, []byte(`"FIGMA_API_KEY"`), []byte(`"OWN"`), 1)
			parsed, err := fragment.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			f = *parsed
			v := compose(t, f, dir, agentic.StdinPayload{})
			code, _, stderr := run(t, prepare(t, v, f, tracked, false), func() error { return nil })
			equal(t, code, 0)
			equal(t, stderr, "curator-run: environment override: OWN\ncurator-run: environment literal replaces lookup: OWN\nhelper stderr\n")
		})
	}
}

// TestRealPTYOwnershipBothModes exercises the actual terminal line discipline,
// not a PID-only signal or an injected executor. Python supplies POSIX PTYs.
func TestRealPTYOwnershipBothModes(t *testing.T) {
	if python == "" {
		t.Fatal("python3 required for real PTY regression")
	}
	for _, mode := range []string{"false", "true"} {
		t.Run("tracked="+mode, func(t *testing.T) {
			cmd := exec.Command(python, "testdata/terminal_probe.py", driver, mode)
			cmd.Env = []string{"PATH=/usr/bin:/bin"}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("PTY regression: %v\n%s", err, out)
			}
			t.Log(string(out))
		})
	}
}
