package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/plan"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/providerlimits"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

var pipelineHelper string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "curator-entry-helper-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	pipelineHelper = filepath.Join(dir, "provider")
	cmd := exec.Command("go", "build", "-o", pipelineHelper, "./testdata/provider")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		fmt.Fprintln(os.Stderr, "helper build:", err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type pipelineFixture struct {
	deps                             launchDeps
	dir, home, binary, layer, prompt string
	resolver                         *scriptedRunner
	args                             []string
	builds, verdicts                 int
}

func writeFixture(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}
func entryFixture(t *testing.T, environment string, tracked bool) *pipelineFixture {
	t.Helper()
	f := &pipelineFixture{dir: t.TempDir()}
	canonical, err := filepath.EvalSymlinks(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	f.dir = canonical
	f.home = filepath.Join(f.dir, "managed")
	if err := os.Mkdir(f.home, 0700); err != nil {
		t.Fatal(err)
	}
	bin := map[string]string{"claude_code": "claude", "codex_cli": "codex", "pi": "pi"}[environment]
	f.binary = filepath.Join(f.dir, bin)
	data, err := os.ReadFile(pipelineHelper)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, f.binary, data, 0700)
	f.layer, f.prompt = filepath.Join(f.home, "mcp.toml"), filepath.Join(f.home, "prompt.md")
	writeFixture(t, f.layer, []byte("[profiles.curator-mcp]\n"), 0600)
	writeFixture(t, f.prompt, []byte("managed prompt\n"), 0600)
	obj := map[string]any{"fragment": fragment.Identity, "environment": environment, "profile": map[string]string{"name": "default", "lock_sha256": strings.Repeat("a", 64)}, "precedence": map[string]string{"winner": "higher-weight", "placement": "winner-last"}, "env": map[string]string{fragment.HomeVariable(environment): f.home}}
	promptCh := map[string]any{"kind": "flag", "flag": "--append-system-prompt-file", "argument": "path", "semantics": "append"}
	if environment == "codex_cli" {
		promptCh = map[string]any{"kind": "config-key", "key": "model_instructions_file", "semantics": "replace"}
	}
	if environment == "pi" {
		promptCh["flag"] = "--append-system-prompt"
	}
	channels := []any{promptCh}
	if environment == "claude_code" {
		channels = append(channels, map[string]any{"kind": "flag", "flag": "--system-prompt-file", "argument": "path", "semantics": "replace"})
	}
	if environment == "pi" {
		channels = append(channels, map[string]any{"kind": "file", "filename": "APPEND_SYSTEM.md", "semantics": "append"}, map[string]any{"kind": "file", "filename": "SYSTEM.md", "semantics": "replace"})
	}
	obj["system_prompt"] = map[string]any{"path": f.prompt, "channels": channels}
	if environment != "pi" {
		ch := map[string]any{"kind": "flag", "flag": "--mcp-config", "argument": "path", "with": []string{"--strict-mcp-config"}}
		if environment == "codex_cli" {
			ch = map[string]any{"kind": "flag", "flag": "-p", "argument": "name", "name": "curator-mcp"}
		}
		obj["mcp"] = map[string]any{"path": f.layer, "env_names": []string{"FIGMA_API_KEY"}, "channels": []any{ch}}
	}
	raw, _ := json.Marshal(obj)
	// Validate fixtures with the real closed fragment parser before dispatch.
	if _, err := fragment.Parse(raw); err != nil {
		t.Fatal(err)
	}
	f.resolver = &scriptedRunner{stdout: string(raw) + "\n", stderr: "curator warning\n\x00\xff"}
	f.deps = testDeps(t, fragment.NewWithRunner("curator", f.resolver))
	f.deps.defaults = defaults.Paths{Machine: filepath.Join(f.dir, "machine", "defaults.json"), Operator: filepath.Join(f.dir, "operator", "defaults.json")}
	if err := os.Mkdir(filepath.Dir(f.deps.defaults.Operator), 0700); err != nil {
		t.Fatal(err)
	}
	if tracked {
		writeFixture(t, filepath.Join(f.dir, "operator", "ax.json"), []byte(`{"schema":"curator-run-ax-v1","enabled":true}`), 0600)
	}
	f.deps.workdir = func() (string, error) { return f.dir, nil }
	f.deps.environ = func() []string {
		return []string{"PATH=" + f.dir, "HOME=" + f.dir, "PARENT=direct-only", "FIGMA_API_KEY=source-secret", "CLAUDECODE=nested", "CODEX_THREAD_ID=nested", "TASK_BOARD_RUN_ID=nested"}
	}
	store, err := providerlimits.NewStore(providerlimits.Options{Layout: providerlimits.LayoutAt(f.dir)})
	if err != nil {
		t.Fatal(err)
	}
	f.deps.availability = func() (plan.AvailabilityFunc, error) {
		return func(q providerlimits.VerdictQuery) (vendorplugin.Availability, error) {
			f.verdicts++
			if q.Home != f.home {
				t.Fatalf("wrong managed home %q", q.Home)
			}
			return store.AvailabilityFor(q)
		}, nil
	}
	f.deps.build = func(ctx context.Context, r *vendorplugin.Registry, req vendorplugin.SpawnRequest, mode agentic.LaunchMode) (agentic.PlanWithEnvironment, error) {
		f.builds++
		if mode != agentic.LaunchModeInteractive || req.Home != f.home || req.WorkDir != f.dir || !req.Composition.IsZero() || !req.Run.IsZero() || req.Goal != nil || req.Budget != nil || req.ServiceTier != "" || req.PromptPath != "" || len(req.Prompt) > 0 {
			t.Fatalf("wrong request: %+v mode=%v", req, mode)
		}
		return vendorplugin.BuildLaunchWithEnvironment(ctx, r, req, mode)
	}
	f.deps.axBinary = pipelineHelper
	f.deps.stdin = strings.NewReader("parent stdin\n\x00\xff")
	f.deps.now = func() time.Time { return time.Date(2026, 9, 16, 1, 2, 3, 0, time.UTC) }
	model, effort, opt := "claude-opus-5", "medium", "append"
	if environment == "codex_cli" {
		model, effort, opt = "gpt-6-astra", "medium", "replace"
	}
	if environment == "pi" {
		effort = "high"
	}
	f.args = []string{environment, "--model", model, "--effort", effort, "--system-prompt", opt, "--", "", "--model=native", "a\nb"}
	return f
}
func (f *pipelineFixture) run() (int, []byte, []byte) {
	var out, stderr bytes.Buffer
	code := run(context.Background(), f.args, &out, &stderr, f.deps)
	return code, out.Bytes(), stderr.Bytes()
}
func (f *pipelineFixture) assertNoChild(t *testing.T) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(f.dir, "started")); !os.IsNotExist(err) {
		t.Fatalf("child side effect: %v", err)
	}
}

type childCapture struct {
	Argv, Env []string
	Stdin     []byte
	WorkDir   string
}

func golden(t *testing.T, name string, value any, root string) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append([]byte(strings.ReplaceAll(string(raw), root, "<ROOT>")), '\n')
	path := filepath.Join("testdata", name+".golden")
	if os.Getenv("UPDATE_PIPELINE_GOLDENS") == "1" {
		writeFixture(t, path, raw, 0600)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, want) {
		t.Fatalf("%s mismatch\ngot %s\nwant %s", name, raw, want)
	}
}
func TestProductionPipelineGoldens(t *testing.T) {
	for _, env := range []string{"claude_code", "codex_cli", "pi"} {
		for _, tracked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/tracked=%v", env, tracked), func(t *testing.T) {
				f := entryFixture(t, env, tracked)
				t.Setenv("PARENT", "ax-only")
				code, out, stderr := f.run()
				if code != 0 {
					t.Fatalf("exit=%d stderr=%s", code, stderr)
				}
				if !bytes.HasPrefix(stderr, []byte(f.resolver.stderr)) {
					t.Fatal("Curator stderr bytes changed")
				}
				if f.builds != 1 || f.verdicts != 1 || f.resolver.calls != 1 {
					t.Fatalf("calls=%d/%d/%d", f.builds, f.verdicts, f.resolver.calls)
				}
				if !reflect.DeepEqual(f.resolver.argv, []string{"env", "resolve", env, "--repair", "--format", "json"}) {
					t.Fatalf("resolve args %q", f.resolver.argv)
				}
				var child childCapture
				if err := json.Unmarshal(out, &child); err != nil {
					t.Fatal(err)
				}
				var payload any
				if tracked {
					if !strings.Contains("\n"+strings.Join(child.Env, "\n")+"\n", "\nPARENT=ax-only\n") {
						t.Fatal("ax did not inherit the launcher environment")
					}
					var doc map[string]any
					if err := json.Unmarshal(child.Stdin, &doc); err != nil {
						t.Fatal(err)
					}
					if bytes.Contains(child.Stdin, []byte("source-secret")) || bytes.Contains(child.Stdin, []byte("direct-only")) {
						t.Fatal("inherited environment serialized")
					}
					parsed, err := fragment.Parse([]byte(f.resolver.stdout))
					if err != nil {
						t.Fatal(err)
					}
					extensions := doc["extensions"].(map[string]any)
					if extensions["works.relux.curator.fragment-digest"] != parsed.Digest {
						t.Fatal("fragment digest changed")
					}
					extensions["works.relux.curator.fragment-digest"] = "<DIGEST>"
					payload = struct {
						Argv     []string
						WorkDir  string
						Document any
						Stderr   string
					}{child.Argv, child.WorkDir, doc, string(stderr)}
				} else {
					sort.Strings(child.Env)
					payload = struct {
						Child  childCapture
						Stderr string
					}{child, string(stderr)}
				}
				golden(t, fmt.Sprintf("pipeline-%s-%v", env, tracked), payload, f.dir)
				if _, err := os.Stat(filepath.Join(f.dir, "started")); err != nil {
					t.Fatal("missing fake process side effect", err)
				}
			})
		}
	}
}

// Mutations happen after composition, at the clock boundary consumed by
// execution.Prepare. They therefore cannot be caught by an earlier admission.
func TestProductionLateChecks(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		for _, tc := range []struct{ name, env, code string }{
			{"binary-removed", "claude_code", "exec_provider_missing"},
			{"binary-not-executable", "codex_cli", "exec_provider_missing"},
			{"binary-directory", "pi", "exec_provider_missing"},
			{"mcp-removed", "codex_cli", "mcp_layer_missing"},
			{"mcp-directory", "codex_cli", "mcp_layer_unreadable"},
			{"mcp-dangling", "codex_cli", "mcp_layer_unreadable"},
			{"prompt-directory", "pi", "sysprompt_file_unreadable"},
			{"prompt-dangling", "pi", "sysprompt_file_unreadable"},
			{"discovery-directory", "pi", "sysprompt_file_unreadable"},
			{"discovery-dangling", "pi", "sysprompt_file_unreadable"},
		} {
			t.Run(fmt.Sprintf("%s/tracked=%v", tc.name, tracked), func(t *testing.T) {
				f := entryFixture(t, tc.env, tracked)
				if strings.HasPrefix(tc.name, "discovery") {
					f.args = f.args[:5]
				}
				original := f.deps.now
				f.deps.now = func() time.Time {
					path := f.binary
					if strings.HasPrefix(tc.name, "mcp-") {
						path = f.layer
					}
					if strings.HasPrefix(tc.name, "prompt-") {
						path = f.prompt
					}
					if strings.HasPrefix(tc.name, "discovery-") {
						path = filepath.Join(f.home, "SYSTEM.md")
					} else if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
					switch {
					case strings.HasSuffix(tc.name, "directory"):
						if err := os.Mkdir(path, 0700); err != nil {
							t.Fatal(err)
						}
					case strings.HasSuffix(tc.name, "dangling"):
						if err := os.Symlink(path+"-absent", path); err != nil {
							t.Fatal(err)
						}
					case strings.HasSuffix(tc.name, "not-executable"):
						writeFixture(t, path, []byte("no execute bit"), 0600)
					}
					return original()
				}
				code, out, stderr := f.run()
				if code != 1 || len(out) != 0 || !bytes.Contains(stderr, []byte("curator-run: "+tc.code+": ")) {
					t.Fatalf("exit=%d out=%q stderr=%s", code, out, stderr)
				}
				if f.builds != 1 || f.verdicts != 1 {
					t.Fatal("late check was not reached after admission")
				}
				f.assertNoChild(t)
			})
		}
	}
}

func TestProductionModeSelection(t *testing.T) {
	for _, tc := range []struct {
		name, machine, operator string
		tracked                 bool
		code                    int
	}{
		{"absent", "", "", false, 0},
		{"disabled", "", `{"schema":"curator-run-ax-v1","enabled":false}`, false, 0},
		{"operator-enabled", "", `{"schema":"curator-run-ax-v1","enabled":true}`, true, 0},
		{"machine-disabled", `{"schema":"curator-run-ax-v1","enabled":false}`, `malformed`, false, 0},
		{"machine-enabled", `{"schema":"curator-run-ax-v1","enabled":true}`, `malformed`, true, 0},
		{"malformed", "", `malformed`, false, 1},
		{"dangling", "", "dangling", false, 1},
		{"directory", "", "directory", false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := entryFixture(t, "pi", false)
			for path, data := range map[string]string{filepath.Join(f.dir, "machine", "ax.json"): tc.machine, filepath.Join(f.dir, "operator", "ax.json"): tc.operator} {
				if data == "" {
					continue
				}
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				switch data {
				case "dangling":
					if err := os.Symlink(path+"-absent", path); err != nil {
						t.Fatal(err)
					}
				case "directory":
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				default:
					writeFixture(t, path, []byte(data), 0600)
				}
			}
			if tc.code != 0 {
				f.args = []string{"--forbidden"}
			}
			code, out, stderr := f.run()
			if code != tc.code {
				t.Fatalf("exit=%d stderr=%s", code, stderr)
			}
			if tc.code != 0 {
				if !bytes.HasPrefix(stderr, []byte("curator-run: defaults_config_invalid:")) || f.resolver.calls != 0 {
					t.Fatalf("config did not precede argv: %s", stderr)
				}
				f.assertNoChild(t)
				return
			}
			var child childCapture
			if err := json.Unmarshal(out, &child); err != nil {
				t.Fatal(err)
			}
			if got := len(child.Argv) > 0 && child.Argv[0] == "start"; got != tc.tracked {
				t.Fatalf("tracked=%v argv=%q", got, child.Argv)
			}
		})
	}
}

func TestProductionForbiddenFlags(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		for _, flag := range []string{"--yolo", "--dangerously-skip-permissions", "--sandbox", "--goal", "--budget", "--service-tier", "--assignment", "--engine", "--tracked"} {
			t.Run(fmt.Sprintf("%s/%v", flag, tracked), func(t *testing.T) {
				f := entryFixture(t, "pi", tracked)
				f.args = []string{"pi", flag}
				code, out, stderr := f.run()
				if code != 2 || len(out) != 0 || f.resolver.calls != 0 || f.builds != 0 {
					t.Fatalf("forbidden flag admitted: exit=%d", code)
				}
				golden(t, "forbidden-"+strings.TrimPrefix(flag, "--"), string(stderr), f.dir)
				f.assertNoChild(t)
			})
		}
	}
}

func TestProductionAdmissionRefusals(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		for _, shape := range []string{"unknown-model", "bad-effort", "limited", "unknown", "read-failed", "module-bytes", "prompt-unavailable"} {
			t.Run(fmt.Sprintf("%s/%v", shape, tracked), func(t *testing.T) {
				f := entryFixture(t, "pi", tracked)
				expected := "plan_refused"
				evidence := "foreign\r\ncurator-run: usage: forged\n\x00\xff"
				switch shape {
				case "unknown-model":
					f.args[2] = "nonexistent-model"
				case "bad-effort":
					f.args[4] = "unsupported-effort"
				case "module-bytes":
					f.deps.build = func(context.Context, *vendorplugin.Registry, vendorplugin.SpawnRequest, agentic.LaunchMode) (agentic.PlanWithEnvironment, error) {
						return agentic.PlanWithEnvironment{}, fmt.Errorf("%s", evidence)
					}
				case "limited", "unknown", "read-failed":
					expected = "plan_provider_limited"
					if shape == "read-failed" {
						expected = "plan_refused"
					}
					f.deps.availability = func() (plan.AvailabilityFunc, error) {
						return func(providerlimits.VerdictQuery) (vendorplugin.Availability, error) {
							if shape == "read-failed" {
								return vendorplugin.Availability{}, fmt.Errorf("%s", evidence)
							}
							state := vendorplugin.AvailabilityLimited
							if shape == "unknown" {
								state = vendorplugin.AvailabilityUnknown
							}
							return vendorplugin.Availability{State: state, Observed: []vendorplugin.Observation{{Source: "fixture", Detail: evidence}}}, nil
						}, nil
					}
				case "prompt-unavailable":
					expected = "sysprompt_channel_unavailable"
					f.args[6] = "replace"
				}
				code, out, stderr := f.run()
				if code != 1 || len(out) != 0 || !bytes.Contains(stderr, []byte("curator-run: "+expected+": ")) {
					t.Fatalf("exit=%d stderr=%s", code, stderr)
				}
				if shape == "limited" || shape == "unknown" || shape == "read-failed" || shape == "module-bytes" {
					if !bytes.Contains(stderr, []byte(evidence)) {
						t.Fatalf("foreign bytes altered: %q", stderr)
					}
				}
				f.assertNoChild(t)
			})
		}
	}
}

func TestProductionExitAndStdin(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		for _, shape := range []string{"exit:37", "signal", "attached-binary", "attached-empty"} {
			t.Run(fmt.Sprintf("%s/%v", shape, tracked), func(t *testing.T) {
				f := entryFixture(t, "pi", tracked)
				expected := 0
				if shape == "exit:37" || shape == "signal" {
					writeFixture(t, filepath.Join(f.dir, "behavior"), []byte(shape), 0600)
					expected = 37
					if shape == "signal" {
						expected = 143
					}
					if tracked {
						expected = 1
					}
				}
				var input []byte
				if strings.HasPrefix(shape, "attached") {
					if shape == "attached-binary" {
						input = []byte{0xff, 0, 0xfe}
					}
					realBuild := f.deps.build
					f.deps.build = func(ctx context.Context, r *vendorplugin.Registry, req vendorplugin.SpawnRequest, mode agentic.LaunchMode) (agentic.PlanWithEnvironment, error) {
						admitted, err := realBuild(ctx, r, req, mode)
						// Interactive shipped plugins attach no stdin. Augment only the payload
						// at this seam to exercise main's forwarding of future admitted payloads.
						admitted.Plan.Stdin = agentic.StdinPayload{Attached: true, Bytes: input}
						return admitted, err
					}
				}
				code, out, stderr := f.run()
				if code != expected {
					t.Fatalf("exit=%d want=%d stderr=%q", code, expected, stderr)
				}
				if shape == "exit:37" && !bytes.HasSuffix(stderr, []byte("{\"code\":\"launch_plan_invalid\"}\n\x00raw\xff")) {
					t.Fatal("child/ax bytes altered")
				}
				if strings.HasPrefix(shape, "attached") {
					var child childCapture
					if err := json.Unmarshal(out, &child); err != nil {
						t.Fatal(err)
					}
					if tracked {
						var doc map[string]any
						if err := json.Unmarshal(child.Stdin, &doc); err != nil {
							t.Fatal(err)
						}
						golden(t, "stdin-"+shape, doc["stdin"], f.dir)
					} else if !bytes.Equal(child.Stdin, input) {
						t.Fatalf("stdin=%v want=%v", child.Stdin, input)
					}
				}
			})
		}
	}
}

func TestProductionDefaultBindingsAndLineup(t *testing.T) {
	for _, env := range []string{"claude_code", "codex_cli", "pi"} {
		for _, tracked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", env, tracked), func(t *testing.T) {
				f := entryFixture(t, env, tracked)
				t.Setenv("XDG_STATE_HOME", f.dir)
				f.deps.build = nil // The real production default, without the recording wrapper.
				f.deps.availability = processAvailability
				f.args = []string{env}
				code, out, stderr := f.run()
				if code != 0 || len(out) == 0 || !bytes.Contains(stderr, []byte("(lineup)")) {
					t.Fatalf("exit=%d stderr=%s", code, stderr)
				}
				if bytes.Contains(stderr, []byte("applied")) {
					t.Fatal("unselected prompt applied")
				}
			})
		}
	}
}

func TestProductionAxProfileAndConfigDiscovery(t *testing.T) {
	f := entryFixture(t, "pi", true)
	f.deps.axPaths = func() (defaults.Paths, error) { return f.deps.defaults, nil }
	f.args = []string{"pi", "--name", "explicit-session", "--ax-profile", "yolo"}
	code, out, stderr := f.run()
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	var child childCapture
	if err := json.Unmarshal(out, &child); err != nil {
		t.Fatal(err)
	}
	want := []string{"start", "explicit-session", "--provider", "pi", "--launch-plan", "-", "--profile", "yolo", "--workspace", f.dir}
	if !reflect.DeepEqual(child.Argv, want) {
		t.Fatalf("argv=%q want=%q", child.Argv, want)
	}
	// A discovery failure must precede help, not only launch/usage validation.
	f.deps.axPaths = func() (defaults.Paths, error) {
		return defaults.Paths{}, fmt.Errorf("discovery\ncurator-run: usage: injected")
	}
	f.args = []string{"--help"}
	code, out, stderr = f.run()
	if code != 1 || len(out) != 0 || string(stderr) != "curator-run: defaults_config_invalid: discovery\n  curator-run: usage: injected\n" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
}

func TestProductionPromptWarningsWithoutSection(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		t.Run(fmt.Sprint(tracked), func(t *testing.T) {
			f := entryFixture(t, "pi", tracked)
			var obj map[string]any
			if err := json.Unmarshal([]byte(f.resolver.stdout), &obj); err != nil {
				t.Fatal(err)
			}
			delete(obj, "system_prompt")
			raw, err := json.Marshal(obj)
			if err != nil {
				t.Fatal(err)
			}
			f.resolver.stdout = string(raw)
			f.args = []string{"pi"}
			writeFixture(t, filepath.Join(f.home, "APPEND_SYSTEM.md"), []byte("append"), 0600)
			writeFixture(t, filepath.Join(f.home, "SYSTEM.md"), []byte("replace"), 0600)
			code, _, stderr := f.run()
			if code != 0 || !bytes.Contains(stderr, []byte(`observed file/append "APPEND_SYSTEM.md"`)) || !bytes.Contains(stderr, []byte(`observed file/replace "SYSTEM.md"`)) || bytes.Contains(stderr, []byte("applied flag")) {
				t.Fatalf("exit=%d stderr=%s", code, stderr)
			}
		})
	}
}
