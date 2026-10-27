package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/network"
	"github.com/relux-works/curator-agent-launcher/internal/plan"
	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
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
	request                          vendorplugin.SpawnRequest
	plan                             agentic.Plan
}

func writeFixture(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}
func entryFixture(t *testing.T, environment string) *pipelineFixture {
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
	// Deterministic §4.3 provider-line path under the fixture root, so
	// the golden normalizes it to <ROOT> like every other host path.
	provider := filepath.Join(f.dir, "curator-run")
	f.deps.providerPath = func() string { return provider }
	f.deps.defaults = defaults.Paths{Machine: filepath.Join(f.dir, "machine", "defaults.json"), Operator: filepath.Join(f.dir, "operator", "defaults.json")}
	if err := os.Mkdir(filepath.Dir(f.deps.defaults.Operator), 0700); err != nil {
		t.Fatal(err)
	}
	writeDefaults(t, filepath.Join(f.dir, "machine", "launcher.json"), `{"schema":"curator-run-launcher-v1","host_mode":"native"}`)

	f.deps.workdir = func() (string, error) { return f.dir, nil }
	releases := map[string]string{"claude_code": "2.1.261", "codex_cli": "codex-cli 0.153.2", "pi": "0.84.2"}
	f.deps.environ = func() []string {
		return []string{"PATH=" + f.dir, "HOME=" + f.dir, "PARENT=direct-only", "FIGMA_API_KEY=source-secret", "CLAUDECODE=nested", "CODEX_THREAD_ID=nested", "TASK_BOARD_RUN_ID=nested", "CURATOR_TEST_RELEASE=" + releases[environment]}
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
		f.request = req
		if mode != agentic.LaunchModeInteractive || req.Home != f.home || req.WorkDir != f.dir || !req.Composition.IsZero() || !req.Run.IsZero() || req.Goal != nil || req.Budget != nil || req.ServiceTier != "" || req.PromptPath != "" || len(req.Prompt) > 0 {
			t.Fatalf("wrong request: %+v mode=%v", req, mode)
		}
		built, err := vendorplugin.BuildLaunchWithEnvironment(ctx, r, req, mode)
		if err == nil {
			f.plan = built.Plan
		}
		return built, err
	}

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

func (f *pipelineFixture) usePermissionsFragment(t *testing.T, mode string, locked bool, source string) {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal([]byte(f.resolver.stdout), &obj); err != nil {
		t.Fatal(err)
	}
	obj["fragment"] = fragment.IdentityV2
	obj["permissions"] = map[string]any{"mode": mode, "locked": locked, "source": source}
	raw, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fragment.Parse(raw); err != nil {
		t.Fatalf("invalid v2 fixture: %v", err)
	}
	f.resolver.stdout = string(raw) + "\n"
}

func (f *pipelineFixture) addEnv(name, value string) {
	previous := f.deps.environ
	f.deps.environ = func() []string {
		return append(previous(), name+"="+value)
	}
}

func (f *pipelineFixture) writeDefaults(t *testing.T, path, raw string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, path, []byte(raw), 0600)
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

// Existing native payload goldens match with the seam off and with a
// detached crashing selector in shadow mode; no golden bytes are changed.
func TestProductionPipelineGoldens(t *testing.T) {
	for _, env := range []string{"claude_code", "codex_cli", "pi"} {

		t.Run(env+"/native", func(t *testing.T) {
			for _, seam := range []string{"off", "shadow"} {
				t.Run(seam, func(t *testing.T) {
					f := entryFixture(t, env)
					if seam == "shadow" {
						shadowFixture(t, f, selectorBinary(t), selectorWorker(t), "exit")
					}
					insertLauncherArgs(f, "--permissions", "native")
					t.Setenv("PARENT", "parent-only")
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

					sort.Strings(child.Env)
					payload = struct {
						Child  childCapture
						Stderr string
					}{child, string(stderr)}

					golden(t, fmt.Sprintf("pipeline-%s-%v", env, false), payload, f.dir)
					if _, err := os.Stat(filepath.Join(f.dir, "started")); err != nil {
						t.Fatal("missing fake process side effect", err)
					}
				})
			}
		})

	}
}

// TestProductionAliasEquivalence drives both spellings through run, the
// production entry point, with the real fragment parser, tagged admission,
// and fake provider. Each alias must resolve with the canonical argv,
// produce a payload identical to the canonical golden.
func TestProductionAliasEquivalence(t *testing.T) {
	pairs := []struct{ alias, canonical string }{
		{"claude", "claude_code"},
		{"codex", "codex_cli"},
	}
	for _, p := range pairs {

		t.Run(fmt.Sprintf("%s-as-%s/native", p.alias, p.canonical), func(t *testing.T) {
			f := entryFixture(t, p.canonical)
			f.args[0] = p.alias
			insertLauncherArgs(f, "--permissions", "native")
			t.Setenv("PARENT", "parent-only")
			code, out, stderr := f.run()
			if code != 0 {
				t.Fatalf("exit=%d stderr=%s", code, stderr)
			}
			if f.builds != 1 || f.verdicts != 1 || f.resolver.calls != 1 {
				t.Fatalf("calls=%d/%d/%d", f.builds, f.verdicts, f.resolver.calls)
			}
			if !reflect.DeepEqual(f.resolver.argv, []string{"env", "resolve", p.canonical, "--repair", "--format", "json"}) {
				t.Fatalf("alias resolve args %q, want canonical %q", f.resolver.argv, p.canonical)
			}
			var child childCapture
			if err := json.Unmarshal(out, &child); err != nil {
				t.Fatal(err)
			}
			var payload any

			sort.Strings(child.Env)
			payload = struct {
				Child  childCapture
				Stderr string
			}{child, string(stderr)}

			// The alias payload must match the canonical golden
			// byte-for-byte: curator-run <alias> behaves exactly as
			// curator-run <canonical>.
			golden(t, fmt.Sprintf("pipeline-%s-%v", p.canonical, false), payload, f.dir)
		})

	}
}

// Mutations at the post-composition provenance boundary prove the native late
// checks refuse changed artifacts after admission, before starting a child.
// Binary rows with an MCP/prompt probe also break it to prove binary refusal wins.
func TestProductionLateChecks(t *testing.T) {

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
		t.Run(tc.name+"/native", func(t *testing.T) {
			f := entryFixture(t, tc.env)
			if strings.HasPrefix(tc.name, "discovery") {
				f.args = f.args[:5]
			}
			insertLauncherArgs(f, "--permissions", "native")
			f.deps.networkEmit = func(w io.Writer, p *adapterprobe.Provenance) error {
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
				if strings.HasPrefix(tc.name, "binary-") && tc.env == "codex_cli" {
					if err := os.Remove(f.layer); err != nil {
						t.Fatal(err)
					}
				}
				if strings.HasPrefix(tc.name, "binary-") && tc.env == "pi" {
					if err := os.Remove(f.prompt); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(f.prompt, 0700); err != nil {
						t.Fatal(err)
					}
				}
				return network.EmitAdapterProvenance(w, p)
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

// Forbidden launcher flags refuse usage before resolution, admission or any child effect.
func TestProductionForbiddenFlags(t *testing.T) {

	for _, flag := range []string{"-d", "--danger", "--dangerously-skip-permissions", "--sandbox", "--goal", "--budget", "--service-tier", "--assignment", "--engine", "--untrackedx"} {
		t.Run(flag, func(t *testing.T) {
			f := entryFixture(t, "pi")
			f.args = []string{"pi", flag}
			code, out, stderr := f.run()
			if code != 2 || len(out) != 0 || f.resolver.calls != 0 || f.builds != 0 {
				t.Fatalf("forbidden flag admitted: exit=%d", code)
			}
			if !bytes.Contains(stderr, []byte("curator-run: usage:")) {
				t.Fatalf("refusal did not use the usage diagnostic: %s", stderr)
			}
			f.assertNoChild(t)
		})
	}

}

// Invalid admission inputs retain their specific refusal and start no child.
func TestProductionAdmissionRefusals(t *testing.T) {

	for _, shape := range []string{"unknown-model", "bad-effort", "limited", "unknown", "read-failed", "module-bytes", "prompt-unavailable"} {
		t.Run(shape, func(t *testing.T) {
			f := entryFixture(t, "pi")
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
			insertLauncherArgs(f, "--permissions", "native")
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

// The production native path preserves attached stdin bytes and child exit status.
func TestProductionExitAndStdin(t *testing.T) {

	for _, shape := range []string{"exit:37", "signal", "attached-binary", "attached-empty"} {
		t.Run(shape, func(t *testing.T) {
			f := entryFixture(t, "pi")
			insertLauncherArgs(f, "--permissions", "native")
			expected := 0
			if shape == "exit:37" || shape == "signal" {
				writeFixture(t, filepath.Join(f.dir, "behavior"), []byte(shape), 0600)
				expected = 37
				if shape == "signal" {
					expected = 143
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
				t.Fatal("child bytes altered")
			}
			if strings.HasPrefix(shape, "attached") {
				var child childCapture
				if err := json.Unmarshal(out, &child); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(child.Stdin, input) {
					t.Fatalf("stdin=%v want=%v", child.Stdin, input)
				}
			}
		})
	}

}

// With no selection, production requests launch using the documented lineup.
func TestProductionDefaultBindingsAndLineup(t *testing.T) {
	for _, env := range []string{"claude_code", "codex_cli", "pi"} {

		t.Run(env, func(t *testing.T) {
			f := entryFixture(t, env)
			t.Setenv("XDG_STATE_HOME", f.dir)
			f.deps.build = nil // The real production default, without the recording wrapper.
			f.deps.availability = processAvailability
			f.args = []string{env, "--permissions", "native"}
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

// Prompt-file warnings remain visible without an engaged fragment system-prompt section.
func TestProductionPromptWarningsWithoutSection(t *testing.T) {

	t.Run("native", func(t *testing.T) {
		f := entryFixture(t, "pi")
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
		f.args = []string{"pi", "--permissions", "native"}
		writeFixture(t, filepath.Join(f.home, "APPEND_SYSTEM.md"), []byte("append"), 0600)
		writeFixture(t, filepath.Join(f.home, "SYSTEM.md"), []byte("replace"), 0600)
		code, _, stderr := f.run()
		if code != 0 || !bytes.Contains(stderr, []byte(`observed file/append "APPEND_SYSTEM.md"`)) || !bytes.Contains(stderr, []byte(`observed file/replace "SYSTEM.md"`)) || bytes.Contains(stderr, []byte("applied flag")) {
			t.Fatalf("exit=%d stderr=%s", code, stderr)
		}
	})

}

// snapshotTree records every entry under root as relpath -> content: regular
// files by bytes, symlinks by target, directories by marker. It is the whole
// persisted state a launch could have touched inside the fixture.
func snapshotTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	tree := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		switch {
		case entry.IsDir():
			tree[rel] = []byte("dir")
		case entry.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			tree[rel] = []byte("link:" + target)
		case entry.Type().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			tree[rel] = data
		default:
			t.Fatalf("unexpected non-regular entry %q (%v)", rel, entry.Type())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// aliasEnvIDPatterns are the persisted shapes in which an environment id can
// appear: the JSON environment field of fragments and configuration, the
// environment= assignment shape, and the hosted session-label shape built
// by execution.Prepare as inv.EnvID + "-" + UTC timestamp. Provider
// executable names (the claude/codex binaries, --provider argv) are
// legitimate and are out of this scan by construction: file names are never
// scanned, and handoff argv is asserted in-memory by
// TestProductionAliasEquivalence, not here.
func aliasEnvIDPatterns(alias string) []*regexp.Regexp {
	quoted := regexp.QuoteMeta(alias)
	return []*regexp.Regexp{
		regexp.MustCompile(`"environment"\s*:\s*"` + quoted + `"`),
		regexp.MustCompile(`environment=` + quoted + `(["'\s]|$)`),
		regexp.MustCompile(`(^|[^A-Za-z0-9_])` + quoted + `-[0-9]{8}T[0-9]{6}Z`),
	}
}

// assertNoAliasEnvID fails when any file content in the post-launch tree
// carries alias as an environment identifier.
func assertNoAliasEnvID(t *testing.T, tree map[string][]byte, alias string) {
	t.Helper()
	patterns := aliasEnvIDPatterns(alias)
	for _, rel := range sortedKeys(tree) {
		if bytes.Equal(tree[rel], []byte("dir")) || bytes.HasPrefix(tree[rel], []byte("link:")) {
			continue
		}
		content := string(tree[rel])
		for _, pattern := range patterns {
			if at := pattern.FindStringIndex(content); at != nil {
				start := max(at[0]-40, 0)
				end := min(at[1]+40, len(content))
				t.Errorf("%q contains %q as an environment id (%s): ...%q...", rel, alias, pattern.String(), content[start:end])
			}
		}
	}
}

func sortedKeys(tree map[string][]byte) []string {
	keys := make([]string, 0, len(tree))
	for k := range tree {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestAliasEnvIDScanFlagsCanary proves the persisted-byte scan is not
// vacuous: every pattern it hunts must fire on a planted canary.
func TestAliasEnvIDScanFlagsCanary(t *testing.T) {
	for _, alias := range []string{"claude", "codex"} {
		canary := map[string][]byte{
			"fragment.json": []byte(`{"environment":"` + alias + `"}`),
			"assign.env":    []byte("environment=" + alias + "\n"),
			"session.txt":   []byte("record " + alias + "-20260916T010203Z closed"),
			"clean.txt":     []byte("provider binary " + alias + " --provider " + alias + "\n"),
		}
		patterns := aliasEnvIDPatterns(alias)
		flagged := map[string]bool{}
		for rel, data := range canary {
			for _, pattern := range patterns {
				if pattern.Match(data) {
					flagged[rel] = true
				}
			}
		}
		for _, rel := range []string{"fragment.json", "assign.env", "session.txt"} {
			if !flagged[rel] {
				t.Errorf("%s: canary %q not flagged", alias, rel)
			}
		}
		if flagged["clean.txt"] {
			t.Errorf("%s: provider-name mention flagged as an environment id", alias)
		}
	}
}

// TestProductionAliasPersistedBytesEqual is the persisted-byte regression for
// the SPEC §3 aliases: both spellings are driven through run, the production
// entry point, with the canonical spelling as control, on the native route.
// Snapshot every persisted byte after both calls; only the fake provider's
// constant started marker is an execution effect. A separate payload golden
// pins argv, environment and stdin. This does not inspect real provider state.
func TestProductionAliasPersistedBytesEqual(t *testing.T) {
	pairs := []struct{ alias, canonical string }{
		{"claude", "claude_code"},
		{"codex", "codex_cli"},
	}
	for _, p := range pairs {

		t.Run(fmt.Sprintf("%s-as-%s/native", p.alias, p.canonical), func(t *testing.T) {
			// Bounded per subtest: the timeout governs the resolve and
			// plan stages that consume the context. Child execution
			// inherits no context (execution.Run), and no sleeps or
			// polling waits are used anywhere in this test.
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			launch := func(env string) (*pipelineFixture, map[string][]byte) {
				f := entryFixture(t, p.canonical)
				f.args[0] = env
				insertLauncherArgs(f, "--permissions", "native")
				before := snapshotTree(t, f.dir)
				var out, stderr bytes.Buffer
				if code := run(ctx, f.args, &out, &stderr, f.deps); code != 0 {
					t.Fatalf("%s: exit=%d stderr=%s", env, code, stderr.String())
				}
				if ctx.Err() != nil {
					t.Fatalf("%s: launch outlived its bound: %v", env, ctx.Err())
				}
				return f, before
			}

			aliasFixture, aliasBefore := launch(p.alias)
			aliasAfter := snapshotTree(t, aliasFixture.dir)
			canonFixture, _ := launch(p.canonical)
			canonAfter := snapshotTree(t, canonFixture.dir)

			// The launch happened and the snapshot is not empty-vs-empty:
			// the fake provider started marker and managed home must be present.
			if !bytes.Equal(aliasAfter["started"], []byte("started")) {
				t.Fatalf("missing fake child side effect; trees hold %q", sortedKeys(aliasAfter))
			}
			for _, rel := range []string{
				filepath.Join("managed", "mcp.toml"),
				filepath.Join("managed", "prompt.md"),
			} {
				if _, ok := aliasAfter[rel]; !ok {
					t.Fatalf("managed home file %q absent from post-launch tree %q", rel, sortedKeys(aliasAfter))
				}
			}

			// Managed-home and configuration files that predate the
			// launch must be untouched by it: neither the launcher nor
			// the child may rewrite managed state with alias bytes.
			for rel, before := range aliasBefore {
				if !(strings.HasPrefix(rel, "managed"+string(filepath.Separator)) ||
					strings.HasSuffix(rel, "defaults.json")) {
					continue
				}
				after, ok := aliasAfter[rel]
				if !ok || !bytes.Equal(before, after) {
					t.Errorf("pre-existing %q changed during alias launch", rel)
				}
			}

			// Alias-run persisted bytes must equal canonical-run
			// persisted bytes, with the per-fixture root normalized the
			// way golden() normalizes it.
			normalize := func(tree map[string][]byte, root string) map[string][]byte {
				normalized := make(map[string][]byte, len(tree))
				for rel, data := range tree {
					if bytes.Equal(data, []byte("dir")) || bytes.HasPrefix(data, []byte("link:")) {
						normalized[rel] = data
						continue
					}
					normalized[rel] = bytes.ReplaceAll(data, []byte(root), []byte("<ROOT>"))
				}
				return normalized
			}
			aliasNorm := normalize(aliasAfter, aliasFixture.dir)
			canonNorm := normalize(canonAfter, canonFixture.dir)
			for _, rel := range sortedKeys(aliasNorm) {
				want, ok := canonNorm[rel]
				if !ok {
					t.Errorf("alias run persisted %q, canonical run did not", rel)
					continue
				}
				if !bytes.Equal(aliasNorm[rel], want) {
					t.Errorf("persisted %q differs between alias and canonical runs", rel)
				}
			}
			for _, rel := range sortedKeys(canonNorm) {
				if _, ok := aliasNorm[rel]; !ok {
					t.Errorf("canonical run persisted %q, alias run did not", rel)
				}
			}

			// No persisted environment-id field may carry the alias.
			assertNoAliasEnvID(t, aliasAfter, p.alias)
		})

	}
}
