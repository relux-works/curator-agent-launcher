//go:build unix

package doorbell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
)

// The subprocess is only a bounded fixture helper: it writes the supplied
// settings path, records its PID/group, and waits at a file barrier before write.
func TestMain(m *testing.M) {
	if os.Getenv("DOORBELL_TEST_HELPER") == "1" {
		os.Exit(containmentHelper())
	}
	os.Exit(m.Run())
}
func containmentHelper() int {
	if len(os.Args) != 9 || os.Args[7] != "--settings" {
		return 2
	}
	group, _ := syscall.Getpgid(0)
	evidence, _ := json.Marshal(map[string]any{"pid": os.Getpid(), "group": group, "settings": os.Args[8]})
	// Publish the complete barrier atomically; observing an empty/truncated
	// evidence file must never release a filesystem plant prematurely.
	if os.WriteFile("ready.tmp", evidence, 0600) != nil || os.Rename("ready.tmp", "ready") != nil {
		return 2
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat("release"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return 3
		}
		time.Sleep(5 * time.Millisecond)
	}
	if os.Getenv("HELPER_MODE") == "fail" {
		return 1
	}
	input := map[string]any{}
	if data, err := os.ReadFile(os.Args[8]); err == nil {
		_ = json.Unmarshal(data, &input)
	}
	input["env"] = map[string]any{"A2A_AGENT": os.Args[4]}
	executable, _ := os.Executable()
	executable, _ = filepath.EvalSymlinks(executable)
	hooks := map[string]any{}
	for event, sub := range map[string]string{"SessionStart": "session-start", "UserPromptSubmit": "prompt", "Stop": "stop"} {
		hooks[event] = []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": executable + " --as " + os.Args[4] + " hook " + sub + " --runtime claude"}}}}
	}
	input["hooks"] = hooks
	data, _ := json.Marshal(input)
	if os.Getenv("HELPER_MODE") == "invalid" {
		data = []byte("{")
	}
	if os.MkdirAll(filepath.Dir(os.Args[8]), 0700) != nil {
		return 2
	}
	if os.WriteFile(os.Args[8], data, 0600) != nil {
		return 2
	}
	return 0
}
func put(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}
func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
}
func noHelper(t *testing.T, root string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "ready"))
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	var e struct{ PID, Group int }
	if json.Unmarshal(data, &e) != nil || e.PID < 1 || e.Group < 1 {
		t.Fatal("missing helper PID evidence")
	}
	for _, pid := range []int{e.PID, -e.Group} {
		if !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Errorf("helper survived: %d", pid)
		}
	}
}
func waitReady(root string) ([]byte, error) {
	deadline := time.Now().Add(7 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(filepath.Join(root, "ready")); err == nil {
			return data, nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil, errors.New("helper barrier deadline")
}

// A pre-write directory swap must leave outside bytes intact; a target inode
// conflict refuses install, and even a symlink back to the pinned directory
// refuses verify. Plain/absent targets prove installation and byte equivalence.
// These barriers exercise conflicts visible before commit, not the accepted
// check-to-rename race in the single-writer contract.
func TestStagedContainment(t *testing.T) {
	for _, variant := range []string{"plain", "absent-file", "absent-directory", "swap-outside", "swap-back", "replace-file", "create-file", "helper-fail", "invalid-result"} {
		t.Run(variant, func(t *testing.T) {
			root := t.TempDir()
			workspace := filepath.Join(root, "workspace")
			trusted := filepath.Join(root, "trusted")
			state := filepath.Join(root, "state")
			mkdir(t, workspace)
			mkdir(t, trusted)
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			binary, err := os.ReadFile(self)
			if err != nil {
				t.Fatal(err)
			}
			helper := filepath.Join(trusted, "a2a")
			put(t, helper, binary, 0700)
			// The installer freezes the physical executable even when TMPDIR
			// gives this fixture a lexical path through a symlink.
			resolvedHelper, err := filepath.EvalSymlinks(helper)
			if err != nil {
				t.Fatal(err)
			}
			directory := filepath.Join(workspace, ".claude")
			settings := filepath.Join(directory, "settings.local.json")
			old := []byte(`{"unrelated":{"kept":true}}`)
			absent := variant == "absent-file" || variant == "absent-directory" || variant == "create-file"
			if variant != "absent-directory" {
				mkdir(t, directory)
			}
			if !absent {
				put(t, settings, old, 0640)
			}
			outside := filepath.Join(root, "outside")
			mkdir(t, outside)
			sentinel := []byte("outside sentinel")
			put(t, filepath.Join(outside, "settings.local.json"), sentinel, 0600)
			mode := ""
			if variant == "helper-fail" {
				mode = "fail"
			}
			if variant == "invalid-result" {
				mode = "invalid"
			}
			env := []string{"PATH=" + trusted, "HOME=" + root, "XDG_STATE_HOME=" + state, "DOORBELL_TEST_HELPER=1", "HELPER_MODE=" + mode}
			installer := Installer{Environment: env, InvocationDirectory: workspace}
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			finished := make(chan error, 1)
			go func() { finished <- installer.Ensure(ctx, workspace, "sample.dev") }()
			// Always release and wait before cleanup or assertions inspect survivors.
			var result error
			ready, barrierErr := waitReady(workspace)
			if barrierErr == nil {
				var evidence struct{ Settings string }
				if json.Unmarshal(ready, &evidence) != nil {
					t.Error("invalid barrier evidence")
				}
				if !inside(evidence.Settings, filepath.Join(state, "swarma", "launcher", "doorbell")) {
					t.Error("helper escaped staging")
				}
				info, err := os.Stat(filepath.Dir(evidence.Settings))
				if err != nil || info.Mode().Perm() != 0700 {
					t.Error("staging directory not private", err)
				}
				input, err := os.ReadFile(evidence.Settings)
				if absent {
					if !os.IsNotExist(err) {
						t.Error("absent input was staged as a file")
					}
				} else if err != nil || !bytes.Equal(input, old) {
					t.Error("staged input changed", err)
				}
				switch variant {
				case "swap-outside", "swap-back":
					moved := filepath.Join(root, "pinned")
					if err := os.Rename(directory, moved); err != nil {
						t.Error(err)
					}
					target := outside
					if variant == "swap-back" {
						target = moved
					}
					if err := os.Symlink(target, directory); err != nil {
						t.Error(err)
					}
				case "replace-file":
					replacement := filepath.Join(directory, "replacement")
					put(t, replacement, []byte("replacement sentinel"), 0600)
					if err := os.Rename(replacement, settings); err != nil {
						t.Error(err)
					}
				case "create-file":
					put(t, settings, []byte("replacement sentinel"), 0600)
				}
			}
			put(t, filepath.Join(workspace, "release"), nil, 0600)
			result = <-finished
			noHelper(t, workspace)
			if barrierErr != nil {
				t.Fatal(barrierErr, result)
			}
			wantStep := ""
			switch variant {
			case "swap-outside", "swap-back", "invalid-result":
				wantStep = "verify"
			case "replace-file", "create-file", "helper-fail":
				wantStep = "install"
			}
			if wantStep == "" {
				if result != nil {
					t.Errorf("valid install refused: %v", result)
				}
			} else {
				var typed *client.Error
				if !errors.As(result, &typed) || typed.Code != "doorbell_setup_failed" || typed.Details["step"] != wantStep {
					t.Errorf("containment verdict=%v want step=%s", result, wantStep)
				}
			}
			got, _ := os.ReadFile(filepath.Join(outside, "settings.local.json"))
			if !bytes.Equal(got, sentinel) {
				t.Error("OUTSIDE SETTINGS MODIFIED")
			}
			entries, err := os.ReadDir(filepath.Join(state, "swarma", "launcher", "doorbell"))
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Error("staging directory survived", entries)
			}
			var e struct{ Settings string }
			if json.Unmarshal(ready, &e) != nil {
				t.Fatal("invalid barrier evidence")
			}

			if variant == "replace-file" || variant == "create-file" {
				got, _ := os.ReadFile(settings)
				if string(got) != "replacement sentinel" {
					t.Error("replacement target overwritten")
				}
			}
			if variant == "helper-fail" || variant == "invalid-result" {
				got, _ := os.ReadFile(settings)
				if !bytes.Equal(got, old) {
					t.Error("failed stage changed target")
				}
			}
			if variant == "swap-outside" || variant == "swap-back" {
				got, err := os.ReadFile(filepath.Join(root, "pinned", "settings.local.json"))
				if err != nil || !verify(got, "sample.dev", resolvedHelper) {
					t.Error("commit missed pinned directory")
				}
				leftovers, err := os.ReadDir(filepath.Join(root, "pinned"))
				if err != nil || len(leftovers) != 1 {
					t.Error("commit temporary file survived", leftovers, err)
				}
			}
			if wantStep != "" {
				return
			}
			got, err = os.ReadFile(settings)
			if err != nil || !verify(got, "sample.dev", resolvedHelper) {
				t.Fatal("installed hooks missing", err)
			}
			info, _ := os.Stat(settings)
			wantMode := os.FileMode(0640)
			if absent {
				wantMode = 0600
			}
			if info.Mode().Perm() != wantMode {
				t.Error("settings mode changed", info.Mode())
			}
			// Run the same fake helper directly on identical initial bytes.
			direct := filepath.Join(root, "direct")
			mkdir(t, direct)
			directFile := filepath.Join(direct, "settings.local.json")
			if !absent {
				put(t, directFile, old, 0600)
			}
			put(t, filepath.Join(direct, "release"), nil, 0600)
			command := exec.CommandContext(ctx, helper, "hook", "install", "--agent", "sample.dev", "--runtime", "claude", "--settings", directFile)
			command.Env = env
			command.Dir = direct
			command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
			command.WaitDelay = 100 * time.Millisecond
			err = command.Run()
			noHelper(t, direct)
			if err != nil {
				t.Fatal("direct helper", err)
			}
			expected, err := os.ReadFile(directFile)
			if err != nil || !bytes.Equal(got, expected) {
				t.Error("staged bytes differ from direct helper", err)
			}
		})
	}
}
