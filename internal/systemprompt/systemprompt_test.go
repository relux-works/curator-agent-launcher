package systemprompt_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	sp "github.com/relux-works/curator-agent-launcher/internal/systemprompt"
)

// Fixtures come from the independently pinned conformance corpus; mutate only
// paths, keeping registry descriptors validated through the production parser.
func fixture(t *testing.T, env, home, path string, section bool) *fragment.Fragment {
	t.Helper()
	name := map[string]string{"claude_code": "valid-system-prompt-only.json", "codex_cli": "valid-codex-config-key-and-name-channel.json", "pi": "valid-pi-file-channels.json", "opencode": "valid-minimal.json"}[env]
	raw, err := os.ReadFile("../fragment/testdata/schema-cases/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err = json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	v["environment"] = env
	v["env"] = map[string]string{fragment.HomeVariable(env): home}
	delete(v, "mcp")
	if section {
		if env == "opencode" {
			v["system_prompt"] = map[string]any{"path": path, "channels": []any{}}
		} else {
			v["system_prompt"].(map[string]any)["path"] = path
		}
	} else {
		delete(v, "system_prompt")
	}
	raw, err = json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	f, err := fragment.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func refusal(t *testing.T, err error, code string) *sp.Refusal {
	t.Helper()
	var r *sp.Refusal
	if !errors.As(err, &r) || r.Code != code {
		t.Fatalf("want %s refusal, got %v", code, err)
	}
	return r
}
func write(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("unchanged prompt"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSelectExactArgv(t *testing.T) {
	path := "/tmp/a \"quote\" \\ ' $()`\n\t\x01\x7f—.md"
	for _, tc := range []struct {
		env  string
		opt  fragment.Semantics
		want []string
	}{
		{"claude_code", fragment.SemanticsAppend, []string{"--append-system-prompt-file", path}},
		{"claude_code", fragment.SemanticsReplace, []string{"--system-prompt-file", path}},
		{"pi", fragment.SemanticsAppend, []string{"--append-system-prompt", path}},
		{"codex_cli", fragment.SemanticsReplace, []string{"-c", "model_instructions_file=\"/tmp/a \\\"quote\\\" \\\\ ' $()`\\u000A\\u0009\\u0001\\u007F—.md\""}},
	} {
		t.Run(tc.env+"/"+string(tc.opt), func(t *testing.T) {
			f := fixture(t, tc.env, t.TempDir(), path, true)
			s, err := sp.Select(f, tc.opt)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(s.Argv(), tc.want) || len(s.Env()) != 0 {
				t.Fatalf("argv=%q env=%v", s.Argv(), s.Env())
			}
			args := s.Argv()
			args[0] = "mutated"
			if !reflect.DeepEqual(s.Argv(), tc.want) {
				t.Fatal("aliased argv")
			}
		})
	}
}

func TestSelectRefusalsAndNoOptIn(t *testing.T) {
	for _, env := range []string{"claude_code", "codex_cli", "pi", "opencode"} {
		for _, section := range []bool{false, true} {
			for _, opt := range []fragment.Semantics{"", fragment.SemanticsAppend, fragment.SemanticsReplace, "bogus"} {
				t.Run(env+"/"+map[bool]string{true: "section", false: "absent"}[section]+"/"+string(opt), func(t *testing.T) {
					f := fixture(t, env, t.TempDir(), "/missing/prompt.md", section)
					s, err := sp.Select(f, opt)
					supported := section && (env == "claude_code" || env == "codex_cli" && opt == fragment.SemanticsReplace || env == "pi" && opt == fragment.SemanticsAppend) && opt != "bogus"
					if opt != "" && !supported {
						refusal(t, err, sp.CodeUnavailable)
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if opt == "" && (len(s.Argv()) != 0 || len(s.Env()) != 0) {
						t.Fatal("applied without opt-in")
					}
				})
			}
		}
	}
}

func TestPrepareLaunchLateFilesAndWarnings(t *testing.T) {
	for _, section := range []bool{false, true} {
		t.Run(map[bool]string{true: "section", false: "no-section"}[section], func(t *testing.T) {
			home := t.TempDir()
			prompt := filepath.Join(home, "inert.md")
			write(t, prompt)
			f := fixture(t, "pi", home, prompt, section)
			launch, err := sp.PrepareLaunch(f, "")
			if err != nil || len(launch.Warnings) != 0 {
				t.Fatalf("absent: %+v %v", launch, err)
			}
			for _, name := range []string{"APPEND_SYSTEM.md", "SYSTEM.md"} {
				write(t, filepath.Join(home, name))
			}
			launch, err = sp.PrepareLaunch(f, "")
			if err != nil {
				t.Fatal(err)
			}
			if len(launch.Warnings) != 2 {
				t.Fatalf("warnings=%q", launch.Warnings)
			}
			for i, name := range []string{"APPEND_SYSTEM.md", "SYSTEM.md"} {
				line := launch.Warnings[i]
				for _, want := range []string{f.Profile.Name, name, "file/", "conditional native-discovery candidate", "native flags and trusted-project precedence", "caching and billing", "own cache prefix"} {
					if !strings.Contains(line, want) {
						t.Fatalf("missing %q in %q", want, line)
					}
				}
			}
			if !strings.Contains(launch.Warnings[1], "If selected, replacement discards the tool's built-in system behavior entirely.") {
				t.Fatal(launch.Warnings)
			}
			if section {
				launch, err = sp.PrepareLaunch(f, fragment.SemanticsAppend)
				if err != nil {
					t.Fatal(err)
				}
				if len(launch.Warnings) != 3 {
					t.Fatal(launch.Warnings)
				}
				want := "warning: profile \"" + f.Profile.Name + "\": observed file/append \"APPEND_SYSTEM.md\" in managed home; discovery suppressed by applied launcher flag \"--append-system-prompt\". A custom system prefix can change request caching and billing: the default may use shared prompt caching; a custom prefix forms its own cache prefix."
				if launch.Warnings[1] != want {
					t.Fatalf("got %q want %q", launch.Warnings[1], want)
				}
				if !strings.Contains(launch.Warnings[0], "applied flag/append") || !strings.Contains(launch.Warnings[2], "conditional native-discovery candidate") {
					t.Fatal(launch.Warnings)
				}
			}
			for _, name := range []string{"APPEND_SYSTEM.md", "SYSTEM.md", "inert.md"} {
				raw, err := os.ReadFile(filepath.Join(home, name))
				if err != nil || string(raw) != "unchanged prompt" {
					t.Fatalf("file changed %s", name)
				}
			}
			if err := os.Remove(filepath.Join(home, "APPEND_SYSTEM.md")); err != nil {
				t.Fatal(err)
			}
			launch, err = sp.PrepareLaunch(f, "")
			if err != nil || len(launch.Warnings) != 1 {
				t.Fatalf("late removal: %+v %v", launch, err)
			}
			if err := os.Mkdir(filepath.Join(home, "APPEND_SYSTEM.md"), 0700); err != nil {
				t.Fatal(err)
			}
			_, err = sp.PrepareLaunch(f, "")
			refusal(t, err, sp.CodeUnreadable)
		})
	}
}

func TestPrepareLaunchFileFailures(t *testing.T) {
	for _, which := range []string{"APPEND_SYSTEM.md", "SYSTEM.md", "selected"} {
		for _, kind := range []string{"missing", "directory", "dangling", "symlink", "permissions", "fifo", "loop"} {
			t.Run(which+"/"+kind, func(t *testing.T) {
				home := t.TempDir()
				prompt := filepath.Join(home, "prompt.md")
				write(t, prompt)
				path := filepath.Join(home, which)
				if which == "selected" {
					path = prompt
					if err := os.Remove(prompt); err != nil {
						t.Fatal(err)
					}
				}
				switch kind {
				case "directory":
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				case "dangling":
					if err := os.Symlink(filepath.Join(home, "missing-target"), path); err != nil {
						t.Fatal(err)
					}
				case "symlink":
					target := filepath.Join(home, "target")
					write(t, target)
					if err := os.Symlink(target, path); err != nil {
						t.Fatal(err)
					}
				case "permissions":
					write(t, path)
					if err := os.Chmod(path, 0000); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = os.Chmod(path, 0600) })
					if os.Geteuid() == 0 {
						t.Skip("root bypasses mode permissions; permission denial not proven")
					}
				case "fifo":
					if err := syscall.Mkfifo(path, 0600); err != nil {
						t.Fatal(err)
					}
				case "loop":
					if err := os.Symlink(path, path); err != nil {
						t.Fatal(err)
					}
				}
				f := fixture(t, "pi", home, prompt, true)
				// Opt-in must not bypass a suppressed APPEND_SYSTEM.md failure.
				launch, err := sp.PrepareLaunch(f, fragment.SemanticsAppend)
				good := kind == "symlink" || kind == "missing" && which != "selected"
				if good {
					if err != nil {
						t.Fatal(err)
					}
					return
				}
				r := refusal(t, err, sp.CodeUnreadable)
				if r.Path != path {
					t.Fatalf("path=%q want %q", r.Path, path)
				}
				if len(launch.Selection.Argv()) != 0 {
					t.Fatal("partial launch leaked on refusal")
				}
			})
		}
	}
}

func TestProbeIndependentAndNoWrites(t *testing.T) {
	home := t.TempDir()
	f := fixture(t, "pi", home, "/absent", false)
	for _, name := range []string{"APPEND_SYSTEM.md", "SYSTEM.md"} {
		write(t, filepath.Join(home, name))
	}
	observed, err := sp.ProbeFiles(f)
	if err != nil || len(observed) != 2 {
		t.Fatalf("%v %v", observed, err)
	}
	// A descriptor-less section cannot disable the environment registry probe.
	f.SystemPrompt = &fragment.SystemPrompt{Path: "/absent"}
	observed, err = sp.ProbeFiles(f)
	if err != nil || len(observed) != 2 {
		t.Fatalf("%v %v", observed, err)
	}
	for _, env := range []string{"claude_code", "codex_cli", "opencode"} {
		f := fixture(t, env, home, "/absent", false)
		observed, err := sp.ProbeFiles(f)
		if err != nil || len(observed) != 0 {
			t.Fatalf("non-Pi probe: %v %v", observed, err)
		}
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 2 {
		t.Fatalf("unexpected write: %v %v", entries, err)
	}
}

func TestAppliedReplacementWarning(t *testing.T) {
	for _, env := range []string{"claude_code", "codex_cli"} {
		f := fixture(t, env, t.TempDir(), "/prompt", true)
		launch, err := sp.PrepareLaunch(f, fragment.SemanticsReplace)
		if err != nil {
			t.Fatal(err)
		}
		kind := map[string]string{"claude_code": "flag", "codex_cli": "config-key"}[env]
		prefix := "warning: profile \"" + f.Profile.Name + "\": applied " + kind + "/replace system-prompt channel"
		if len(launch.Warnings) != 1 || !strings.HasPrefix(launch.Warnings[0], prefix) || !strings.Contains(launch.Warnings[0], "Replacement discards the tool's built-in system behavior entirely.") || !strings.Contains(launch.Warnings[0], "caching and billing") {
			t.Fatal(launch.Warnings)
		}
	}
}

func TestPrepareLaunchSelectedPathChangesLate(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "prompt.md")
	write(t, path)
	f := fixture(t, "pi", home, path, true)
	if _, err := sp.Select(f, fragment.SemanticsAppend); err != nil {
		t.Fatal(err)
	}
	if _, err := sp.PrepareLaunch(f, fragment.SemanticsAppend); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	_, err := sp.PrepareLaunch(f, fragment.SemanticsAppend)
	refusal(t, err, sp.CodeUnreadable)
	// Missing inert data is not a refusal when no channel was requested.
	if _, err = sp.PrepareLaunch(f, ""); err != nil {
		t.Fatal(err)
	}
}

func TestProbeParentFailures(t *testing.T) {
	for _, kind := range []string{"permissions", "non-directory"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			home := filepath.Join(root, "home")
			if kind == "permissions" {
				if os.Geteuid() == 0 {
					t.Skip("root bypasses directory permissions")
				}
				if err := os.Mkdir(home, 0000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(home, 0700) })
			} else {
				write(t, home)
			}
			f := fixture(t, "pi", home, "/prompt", false)
			_, err := sp.PrepareLaunch(f, "")
			refusal(t, err, sp.CodeUnreadable)
		})
	}
}
