//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/plan"
	"github.com/relux-works/curator-agent-launcher/internal/selector"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/fakemanager"
	"github.com/relux-works/skill-agents-management/pkg/providerlimits"
)

type frontBinaries struct{ launcher, manager, adapter, provider, selector, system string }

func frontBuild(t *testing.T) frontBinaries {
	t.Helper()
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "curator-run"), filepath.Join(dir, "fake"), filepath.Join(dir, "adapter"), filepath.Join(dir, "provider"), filepath.Join(dir, "selector")}
	for i, pkg := range []string{".", "../curator-run-fake-manager", "../curator-run-fake-manager-fixture", "./testdata/frontdoor", "../../internal/selector/testdata/fake"} {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		args := []string{"build", "-o", paths[i]}
		if i == 0 {
			args = append(args, "-ldflags", "-X main.systemConfigDirectory="+filepath.Join(dir, "system")+" -X main.legacySystemConfigDirectory="+filepath.Join(dir, "legacy-system"))
		}
		cmd := exec.CommandContext(ctx, "go", append(args, pkg)...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
		out, err := cmd.CombinedOutput()
		cancel()
		frontGone(t, cmd)
		if err != nil {
			t.Fatalf("build %s: %v %s", pkg, err, out)
		}
	}
	return frontBinaries{paths[0], paths[1], paths[2], paths[3], paths[4], filepath.Join(dir, "system")}
}
func frontGone(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	// A test owns its process group. Cleanup by recorded PID also runs on failure.
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	for _, p := range []int{pid, -pid} {
		deadline := time.Now().Add(time.Second)
		for !errors.Is(syscall.Kill(p, 0), syscall.ESRCH) && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond * 5)
		}
		if !errors.Is(syscall.Kill(p, 0), syscall.ESRCH) {
			t.Fatalf("test process survived: %d", p)
		}
	}
}
func frontJSON(v any) []byte {
	b, e := json.Marshal(v)
	if e != nil {
		panic(e)
	}
	return b
}

type frontFixture struct {
	t                   *testing.T
	bin                 frontBinaries
	root, config, state string
	env                 []string
	managerConfig       fakemanager.Config
}

func newFront(t *testing.T, bin frontBinaries) *frontFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &frontFixture{t: t, bin: bin, root: root, config: filepath.Join(root, "config", "swarma", "launcher"), state: filepath.Join(root, "state")}
	for _, p := range []string{bin.system, f.config, f.state, filepath.Join(root, "bin"), filepath.Join(root, "managed")} {
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"curator", "claude", "codex", "muse"} {
		data, err := os.ReadFile(bin.provider)
		if err != nil {
			t.Fatal(err)
		}
		writeFixture(t, filepath.Join(root, "bin", name), data, 0700)
	}
	value := map[string]any{"fragment": fragment.IdentityV2, "environment": "claude_code", "profile": map[string]string{"name": "sample", "lock_sha256": strings.Repeat("1", 64)}, "precedence": map[string]string{"winner": "higher-weight", "placement": "winner-last"}, "env": map[string]string{"CLAUDE_CONFIG_DIR": filepath.Join(root, "managed")}, "permissions": map[string]any{"mode": "native", "source": "profile", "locked": false}}
	raw := frontJSON(value)
	writeFixture(t, filepath.Join(root, "fragment.json"), raw, 0600)
	frag, err := fragment.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	f.env = []string{"PATH=" + filepath.Join(root, "bin"), "HOME=" + root, "XDG_CONFIG_HOME=" + filepath.Join(root, "config"), "XDG_STATE_HOME=" + f.state}
	f.managerConfig = fakemanager.DefaultConfig()
	f.managerConfig.Profiles["sample"] = fakemanager.Profile{LockSHA256: frag.Profile.LockSHA256, FragmentDigest: frag.Digest}
	os.Remove(filepath.Join(bin.system, "selector.json"))
	f.reset()
	f.policy("auto", false)
	f.registry("selected")
	return f
}
func (f *frontFixture) policy(mode string, trial bool) {
	f.t.Helper()
	writeFixture(f.t, filepath.Join(f.bin.system, "launcher.json"), frontJSON(map[string]any{"schema": "curator-run-launcher-v1", "host_mode": mode, "delegated_trial": trial}), 0600)
}
func (f *frontFixture) registry(kind string) {
	f.t.Helper()
	path := filepath.Join(f.config, sessionmanager.Filename)
	if kind == "invalid" {
		writeFixture(f.t, path, []byte("{"), 0600)
		return
	}
	registry := sessionmanager.Empty()
	if kind != "empty" {
		registry.Managers["fake"] = sessionmanager.Entry{Executable: f.bin.manager, ArgvPrefix: []string{}, Protocol: sessionmanager.Protocol, RegisteredAt: "2026-01-01T00:00:00Z", RegisteredBy: "test fixture"}
		if kind == "selected" {
			registry.Selected = "fake"
		}
	}
	data, err := sessionmanager.Encode(registry)
	if err != nil {
		f.t.Fatal(err)
	}
	writeFixture(f.t, path, data, 0600)
}
func (f *frontFixture) command(path string, args []string, input []byte, tty, pipe bool, budget time.Duration) (int, string, string) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir, cmd.Env = f.root, f.env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if tty {
		_, slave := frontPTY(f.t)
		cmd.SysProcAttr.Setctty = true
		if pipe {
			cmd.ExtraFiles = []*os.File{slave}
			cmd.SysProcAttr.Ctty = 3
			cmd.Stdin = bytes.NewReader(input)
		} else {
			cmd.Stdin = slave
			cmd.SysProcAttr.Ctty = 0
		}
	} else {
		cmd.Stdin = bytes.NewReader(input)
	}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Millisecond * 100
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err := cmd.Run()
	frontGone(f.t, cmd)
	if ctx.Err() != nil {
		f.t.Fatalf("test command deadline: %s %s", out.String(), stderr.String())
	}
	if err != nil && cmd.ProcessState == nil {
		f.t.Fatalf("start %v", err)
	}
	return cmd.ProcessState.ExitCode(), out.String(), stderr.String()
}
func (f *frontFixture) control(r fakemanager.FixtureRequest) fakemanager.FixtureResponse {
	f.t.Helper()
	code, out, stderr := f.command(f.bin.adapter, nil, frontJSON(r), false, false, 10*time.Second)
	var response fakemanager.FixtureResponse
	if json.Unmarshal([]byte(out), &response) != nil || code != 0 || !response.OK {
		f.t.Fatalf("fixture %s: %d %s %s", r.Operation, code, out, stderr)
	}
	return response
}
func (f *frontFixture) reset() {
	f.control(fakemanager.FixtureRequest{Operation: "reset", Config: &f.managerConfig})
}
func (f *frontFixture) observe() *fakemanager.Observation {
	return f.control(fakemanager.FixtureRequest{Operation: "observe"}).Observation
}
func (f *frontFixture) invoke(tty, pipe bool, args ...string) (int, string, string) {
	f.t.Helper()
	base := []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"}
	return f.command(f.bin.launcher, append(base, args...), nil, tty, pipe, 12*time.Second)
}
func (f *frontFixture) require(t *testing.T, code, want int, stderr, diagnostic string, calls []string, starts int, native bool) {
	t.Helper()
	if code != want || (diagnostic != "" && !strings.Contains(stderr, diagnostic)) {
		t.Fatalf("exit=%d want=%d diagnostic=%s stderr=%s", code, want, diagnostic, stderr)
	}
	o := f.observe()
	history := f.control(fakemanager.FixtureRequest{Operation: "observe_calls"}).CallHistory
	if history == nil {
		t.Fatal("missing call telemetry")
	}
	for _, process := range history.CallProcesses {
		for _, pid := range []int{process.PID, -process.Group} {
			if !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
				t.Fatalf("manager process survived: %d", pid)
			}
		}
	}
	if !slices.Equal(history.Calls, calls) || o.Starts != starts {
		t.Fatalf("calls=%v starts=%d; want calls=%v starts=%d", history.Calls, o.Starts, calls, starts)
	}
	_, err := os.Stat(filepath.Join(f.root, "native-called"))
	if native != (err == nil) {
		t.Fatalf("native=%v want=%v stderr=%s", err == nil, native, stderr)
	}
	if _, err := os.Stat(filepath.Join(f.state, "curator-run", "werkstatt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("plain launch wrote Werkstatt state: %v", err)
	}
}

// Every non-Werkstatt §5.8 row in the after-sign-off column (Q1 signed off
// 2026-10-21) uses the real binary, real config files and a delegated-only
// fake. The withdrawn delegated_trial flag changes no row; the pre-sign-off
// columns remain pure client.Required tests.
func TestFrontDoorBinary(t *testing.T) {
	bin := frontBuild(t)
	t.Run("activation", func(t *testing.T) {
		for _, trial := range []bool{false, true} {
			for _, row := range []struct {
				name, mode, flag string
				code             int
				diagnostic       string
				calls            []string
				starts           int
				native           bool
			}{
				{"explicit", "auto", "--hosted", 0, "ends hosted sessions when it is upgraded", []string{"describe", "launch", "attach"}, 1, false},
				{"auto", "auto", "", 0, "ends hosted sessions when it is upgraded", []string{"describe", "launch", "attach"}, 1, false},
				{"hosted-default", "hosted", "", 0, "ends hosted sessions when it is upgraded", []string{"describe", "launch", "attach"}, 1, false},
				{"native-flag", "hosted", "--native", 0, "", nil, 0, true},
				{"native-mode", "native", "", 0, "", nil, 0, true},
			} {
				t.Run(row.name+map[bool]string{false: "-no-trial", true: "-trial"}[trial], func(t *testing.T) {
					f := newFront(t, bin)
					f.policy(row.mode, trial)
					args := []string{}
					if row.flag != "" {
						args = append(args, row.flag)
					}
					code, _, stderr := f.invoke(true, false, args...)
					f.require(t, code, row.code, stderr, row.diagnostic, row.calls, row.starts, row.native)
					if strings.Count(stderr, "curator-run: no session manager") > 1 || strings.Contains(stderr, "session_host_default_not_ready") {
						t.Fatal(stderr)
					}
				})
			}
		}
	})
	t.Run("registry-paths", func(t *testing.T) {
		for _, kind := range []string{"invalid", "empty", "unselected", "selected"} {
			for _, flag := range []string{"--native", "--untracked", "--hosted", ""} {
				t.Run(kind+"/"+flag, func(t *testing.T) {
					f := newFront(t, bin)
					f.policy("auto", true)
					f.registry(kind)
					args := []string{}
					if flag != "" {
						args = append(args, flag)
					}
					code, _, stderr := f.invoke(true, false, args...)
					if flag == "--native" || flag == "--untracked" {
						f.require(t, code, 0, stderr, "", nil, 0, true)
						return
					}
					if kind == "invalid" {
						f.require(t, code, 2, stderr, "session_manager_registry_invalid", nil, 0, false)
						return
					}
					if kind == "selected" {
						f.require(t, code, 0, stderr, "", []string{"describe", "launch", "attach"}, 1, false)
						return
					}
					detail := "no manager selected"
					notice := "curator-run: no session manager selected (1 registered); launching natively (curator-run session-manager use <id>)"
					if strings.Contains(stderr, "session-manager select") || strings.Contains(stderr, "curator run session-manager") {
						t.Fatalf("notice recommends obsolete command: %q", stderr)
					}
					if kind == "empty" {
						detail = "no manager registered"
						notice = "curator-run: no session manager registered; launching natively (curator-run session-manager list)"
					}
					if flag == "--hosted" {
						f.require(t, code, 1, stderr, detail, nil, 0, false)
					} else {
						f.require(t, code, 0, stderr, notice, nil, 0, true)
						if strings.Count(stderr, notice) != 1 {
							t.Fatal(stderr)
						}
					}
				})
			}
		}
	})
	t.Run("refusal-order", func(t *testing.T) {
		for _, flag := range []string{"--hosted", ""} {
			for _, check := range []string{"network", "terminal", "stdin"} {
				for _, kind := range []string{"invalid", "selected"} {
					t.Run(flag+"/"+check+"/"+kind, func(t *testing.T) {
						f := newFront(t, bin)
						f.policy("auto", true)
						f.registry(kind)
						args := []string{}
						if flag != "" {
							args = append(args, flag)
						}
						tty, pipe := true, false
						want, diag := 6, "session_host_stdin_unsupported"
						switch check {
						case "network":
							args = append(args, "--network", "direct")
							tty = false
							want, diag = 16, "network_scope_unsupported"
						case "terminal":
							tty = false
							diag = "session_host_terminal_required"
						case "stdin":
							pipe = true
						}
						if flag == "" && kind == "invalid" {
							want, diag = 2, "session_manager_registry_invalid"
						}
						code, _, stderr := f.invoke(tty, pipe, args...)
						f.require(t, code, want, stderr, diag, nil, 0, false)
					})
				}
			}
		}
	})
	t.Run("ax-placement", func(t *testing.T) {
		for _, kind := range []string{"invalid", "empty", "unselected", "selected"} {
			for _, flag := range []string{"--native", "--hosted", ""} {
				t.Run(kind+"/"+flag, func(t *testing.T) {
					f := newFront(t, bin)
					f.policy("auto", true)
					f.registry(kind)
					writeFixture(t, filepath.Join(f.config, "ax.json"), []byte(`{"schema":"curator-run-ax-v1","enabled":true}`), 0600)
					// The native fixture also acts as ax and records the handoff; no shell.
					data, _ := os.ReadFile(bin.provider)
					writeFixture(t, filepath.Join(f.root, "bin", "ax"), data, 0700)
					args := []string{}
					if flag != "" {
						args = append(args, flag)
					}
					code, _, stderr := f.invoke(false, false, args...)
					switch {
					case flag == "--native":
						f.require(t, code, 0, stderr, "", nil, 0, true)
					case flag == "--hosted" || kind == "selected":
						f.require(t, code, 2, stderr, "host_configuration_conflict", nil, 0, false)
					case kind == "invalid":
						f.require(t, code, 2, stderr, "session_manager_registry_invalid", nil, 0, false)
					default:
						f.require(t, code, 0, stderr, "no session manager", nil, 0, true)
					}
				})
			}
		}
	})
	t.Run("no-fallback", func(t *testing.T) {
		for _, fault := range []string{"missing-executable", "missing-capability", "unsupported-runtime", "protocol"} {
			t.Run(fault, func(t *testing.T) {
				f := newFront(t, bin)
				f.policy("auto", true)
				expected := []string{"describe"}
				want, diag := 6, "session_host_protocol_error"
				switch fault {
				case "missing-executable":
					data, _ := sessionmanager.Encode(sessionmanager.Registry{Schema: sessionmanager.Schema, Selected: "fake", Managers: map[string]sessionmanager.Entry{"fake": {Executable: filepath.Join(f.root, "missing"), ArgvPrefix: []string{}, Protocol: sessionmanager.Protocol, RegisteredAt: "2026-01-01T00:00:00Z", RegisteredBy: "test fixture"}}})
					writeFixture(t, filepath.Join(f.config, sessionmanager.Filename), data, 0600)
					want, diag, expected = 1, "registered executable missing", nil
				case "missing-capability":
					f.managerConfig.Capabilities = slices.DeleteFunc(f.managerConfig.Capabilities, func(s string) bool { return s == "attach/1" })
					f.reset()
					diag = "session_host_capability_missing"
				case "unsupported-runtime":
					delete(f.managerConfig.Runtimes, "claude_code")
					f.reset()
					diag = "session_host_provider_unsupported"
				case "protocol":
					f.control(fakemanager.FixtureRequest{Operation: "inject_fault", Name: "protocol"})
				}
				code, _, stderr := f.invoke(true, false)
				f.require(t, code, want, stderr, diag, expected, 0, false)
			})
		}
	})
	t.Run("delegated-details", func(t *testing.T) { testDelegatedDetails(t, bin) })
	t.Run("shadow-outcome", func(t *testing.T) { testHostedShadowOutcome(t, bin) })
}

func frontStored(t *testing.T, o *fakemanager.Observation) *fakemanager.Stored {
	t.Helper()
	var state fakemanager.State
	if err := json.Unmarshal(o.Records, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Sessions) != 1 {
		t.Fatalf("expected one session: %d", len(state.Sessions))
	}
	for _, s := range state.Sessions {
		return s
	}
	return nil
}
func testDelegatedDetails(t *testing.T, bin frontBinaries) {
	t.Run("request", func(t *testing.T) {
		f := newFront(t, bin)
		f.policy("auto", true)
		code, _, stderr := f.invoke(true, false, "--hosted", "--name", "example-session", "--", "hello")
		f.require(t, code, 0, stderr, "ends hosted sessions when it is upgraded", []string{"describe", "launch", "attach"}, 1, false)
		stored := frontStored(t, f.observe())
		var request map[string]any
		json.Unmarshal(stored.Plan, &request)
		expected := map[string]any{"runtime": "claude_code", "cwd": f.root, "model": "claude-opus-5", "effort": "medium", "service_tier": nil, "weights_id": nil, "permissions": map[string]any{"mode": "native", "source": "flag"}, "system_prompt": nil, "sandbox": map[string]any{"mode": "none", "writable_roots": []any{}}, "harness_memory": false, "session_name": "example-session", "labels": map[string]any{}, "child_env": map[string]any{}, "board_goal": "none", "doorbell": nil, "credential_source": nil, "native_tail": []any{"hello"}, "detach": false, "admission_key": nil}
		for k, want := range expected {
			if !reflect.DeepEqual(request[k], want) {
				t.Errorf("%s=%#v want %#v", k, request[k], want)
			}
		}
		if request["profile"].(map[string]any)["fragment_digest"] != f.managerConfig.Profiles["sample"].FragmentDigest {
			t.Fatal("wrong fragment binding")
		}
	})
	t.Run("detach", func(t *testing.T) {
		f := newFront(t, bin)
		f.policy("auto", true)
		code, _, stderr := f.invoke(false, false, "--hosted", "--detach")
		f.require(t, code, 0, stderr, "", []string{"describe", "launch"}, 1, false)
	})
	t.Run("board-goal", func(t *testing.T) {
		f := newFront(t, bin)
		f.policy("auto", true)
		code, _, stderr := f.invoke(true, false, "--hosted", "--board-goal")
		f.require(t, code, 6, stderr, "capability=board-goal/1", []string{"describe"}, 0, false)
		// Nearby positive control: goal-less request works with this same manager.
		code, _, stderr = f.invoke(true, false, "--hosted")
		f.require(t, code, 0, stderr, "", []string{"describe", "describe", "launch", "attach"}, 1, false)
	})
	t.Run("attach-exit", func(t *testing.T) {
		f := newFront(t, bin)
		f.policy("auto", true)
		f.control(fakemanager.FixtureRequest{Operation: "inject_fault", Name: "plain-attach-exit"})
		code, _, stderr := f.invoke(true, false, "--hosted")
		f.require(t, code, 7, stderr, "", []string{"describe", "launch", "attach"}, 1, false)
		if strings.Contains(stderr, "policy_refused") {
			t.Fatal("untyped exit converted to refusal")
		}
	})
	for _, fault := range []string{"after-admission", "crash-after-start", "before-admission"} {
		t.Run("lost-answer-"+fault, func(t *testing.T) {
			f := newFront(t, bin)
			f.policy("auto", true)
			f.control(fakemanager.FixtureRequest{Operation: "inject_fault", Name: fault})
			code, _, stderr := f.invoke(true, false, "--hosted")
			switch fault {
			case "after-admission":
				f.require(t, code, 0, stderr, "", []string{"describe", "launch", "status", "attach"}, 1, false)
			case "crash-after-start":
				f.require(t, code, 1, stderr, "session_liveness_unknown", []string{"describe", "launch", "status"}, 1, false)
				if !strings.Contains(stderr, "operation=") {
					t.Fatal("lost operation not disclosed")
				}
			case "before-admission":
				f.require(t, code, 1, stderr, "reason=cancelled", []string{"describe", "launch", "status", "cancel"}, 0, false)
			}
		})
	}
	for _, selector := range []string{"latest", "handle", "provider-id", "tail-id", "missing"} {
		t.Run("resume-"+selector, func(t *testing.T) {
			f := newFront(t, bin)
			f.policy("auto", true)
			code, _, stderr := f.invoke(false, false, "--hosted", "--detach")
			f.require(t, code, 0, stderr, "", []string{"describe", "launch"}, 1, false)
			stored := frontStored(t, f.observe())
			nativeID := "01234567-89ab-4cde-8123-456789abcdef"
			// The fake's generated opaque id is not a Claude UUID. Set this harmless
			// persisted provider id in the isolated fixture to exercise typed elevation.
			path := filepath.Join(f.state, "curator-run", "fake-manager", "state.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var state fakemanager.State
			json.Unmarshal(raw, &state)
			var row map[string]any
			json.Unmarshal(state.Sessions[stored.Handle].Record, &row)
			row["native_id"] = nativeID
			state.Sessions[stored.Handle].Record = frontJSON(row)
			writeFixture(t, path, frontJSON(state), 0600)
			payload := frontJSON(map[string]any{"session": stored.Handle, "expected_generation": 1, "mode": "force", "precondition": "none"})
			code, _, stderr = f.command(f.bin.manager, []string{"stop"}, payload, false, false, 10*time.Second)
			if code != 0 {
				t.Fatalf("stop %d %s", code, stderr)
			}
			args := []string{"--hosted"}
			switch selector {
			case "latest":
				args = append(args, "resume")
			case "handle":
				args = append(args, "resume", stored.Handle)
			case "provider-id":
				args = append(args, "--resume", nativeID)
			case "tail-id":
				args = append(args, "--", "--resume", nativeID)
			case "missing":
				args = append(args, "resume", "SES-missing")
			}
			code, _, stderr = f.invoke(true, false, args...)
			if selector == "missing" {
				f.require(t, code, 1, stderr, "session_resume_not_found", []string{"describe", "launch", "stop", "describe", "resume"}, 1, false)
			} else {
				f.require(t, code, 0, stderr, "", []string{"describe", "launch", "stop", "describe", "resume", "attach"}, 2, false)
				if s := frontStored(t, f.observe()); s.Handle != stored.Handle {
					t.Fatal("resume created another session")
				}
			}
		})
	}
	t.Run("prompt", func(t *testing.T) {
		f := newFront(t, bin)
		f.policy("auto", true)
		path := filepath.Join(f.root, "fragment.json")
		raw, _ := os.ReadFile(path)
		var v map[string]any
		json.Unmarshal(raw, &v)
		prompt := filepath.Join(f.root, "managed", "prompt.md")
		writeFixture(t, prompt, []byte("fixture prompt"), 0600)
		v["system_prompt"] = map[string]any{"path": prompt, "channels": []any{map[string]any{"kind": "flag", "flag": "--append-system-prompt-file", "argument": "path", "semantics": "append"}, map[string]any{"kind": "flag", "flag": "--system-prompt-file", "argument": "path", "semantics": "replace"}}}
		raw = frontJSON(v)
		writeFixture(t, path, raw, 0600)
		frag, err := fragment.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		f.managerConfig.Profiles["sample"] = fakemanager.Profile{LockSHA256: frag.Profile.LockSHA256, FragmentDigest: frag.Digest}
		f.reset()
		code, _, stderr := f.invoke(true, false, "--hosted", "--system-prompt", "append")
		f.require(t, code, 0, stderr, "", []string{"describe", "launch", "attach"}, 1, false)
		stored := frontStored(t, f.observe())
		var plan map[string]any
		json.Unmarshal(stored.Plan, &plan)
		if !reflect.DeepEqual(plan["system_prompt"], map[string]any{"semantics": "append"}) {
			t.Fatal("prompt semantics lost")
		}
		code, _, stderr = f.invoke(true, false, "--hosted", "--system-prompt", "append", "--", "--append-system-prompt", "conflict")
		f.require(t, code, 1, stderr, "plan_refused", []string{"describe", "launch", "attach", "describe"}, 1, false)
		if strings.Contains(stderr, "conflict\"") {
			t.Fatal("hosted refusal echoed native value")
		}
	})
	t.Run("codex", func(t *testing.T) {
		f := newFront(t, bin)
		f.policy("auto", true)
		path := filepath.Join(f.root, "fragment.json")
		raw, _ := os.ReadFile(path)
		var v map[string]any
		json.Unmarshal(raw, &v)
		v["environment"] = "codex_cli"
		v["env"] = map[string]any{"CODEX_HOME": filepath.Join(f.root, "managed")}
		raw = frontJSON(v)
		writeFixture(t, path, raw, 0600)
		frag, err := fragment.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		f.managerConfig.Profiles["sample"] = fakemanager.Profile{LockSHA256: frag.Profile.LockSHA256, FragmentDigest: frag.Digest}
		f.reset()
		args := []string{"codex_cli", "--hosted", "--model", "gpt-6-astra", "--effort", "medium", "--permissions", "native"}
		code, _, stderr := f.command(f.bin.launcher, args, nil, true, false, 12*time.Second)
		f.require(t, code, 0, stderr, "", []string{"describe", "launch", "attach"}, 1, false)
		if stored := frontStored(t, f.observe()); stored.Runtime != "codex_cli" {
			t.Fatal("wrong runtime")
		}
	})
	for _, decision := range []string{"decision-1", "decision/@+1"} {
		t.Run("selector/"+decision, func(t *testing.T) {
			f := newFront(t, bin)
			f.policy("auto", true)
			helper := filepath.Join(f.root, "bin", "selector")
			data, _ := os.ReadFile(bin.selector)
			writeFixture(t, helper, data, 0700)
			writeFixture(t, filepath.Join(f.root, "bin", "response.json"), frontJSON(map[string]any{"decision_id": decision, "runtime": "claude", "model": "claude-opus-5", "effort": "medium", "weights_id": nil, "refusal": nil}), 0600)
			writeFixture(t, filepath.Join(bin.system, "selector.json"), frontJSON(map[string]any{"schema": "curator-run-selector-v1", "mode": "enforce", "executable": helper, "argv_prefix": []string{}, "timeout_seconds": 10}), 0600)
			code, _, stderr := f.invoke(true, false, "--hosted")
			pidData, err := os.ReadFile(filepath.Join(f.root, "bin", "selector.pid"))
			if err != nil {
				t.Fatalf("selector never started: %v; exit=%d stderr=%s", err, code, stderr)
			}
			pid, err := strconv.Atoi(string(pidData))
			if err != nil {
				t.Fatal(err)
			}
			if !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
				syscall.Kill(-pid, syscall.SIGKILL)
				t.Fatal("selector survived invocation")
			}
			f.require(t, code, 0, stderr, "", []string{"describe", "launch", "attach"}, 1, false)
			if !reflect.DeepEqual(frontStored(t, f.observe()).Labels, map[string]string{"selector": decision}) {
				t.Fatal("selector label lost")
			}
			stored := frontStored(t, f.observe())
			before := f.observe().PolicyDigests[stored.Handle]
			payload := frontJSON(map[string]any{"session": stored.Handle, "expected_generation": 1, "mode": "force", "precondition": "none"})
			code, _, stderr = f.command(f.bin.manager, []string{"stop"}, payload, false, false, 10*time.Second)
			if code != 0 {
				t.Fatal(stderr)
			}
			writeFixture(t, filepath.Join(f.root, "bin", "response.json"), frontJSON(map[string]any{"decision_id": "new-decision", "runtime": "claude", "model": "claude-opus-5", "effort": "medium", "weights_id": nil, "refusal": nil}), 0600)
			code, _, stderr = f.invoke(true, false, "--hosted", "resume")
			f.require(t, code, 0, stderr, "", []string{"describe", "launch", "attach", "stop", "describe", "resume", "attach"}, 2, false)
			after := frontStored(t, f.observe())
			if !reflect.DeepEqual(after.Labels, stored.Labels) || f.observe().PolicyDigests[stored.Handle] != before {
				t.Fatal("resume replaced admitted labels or policy")
			}

		})
	}
	t.Run("unsupported-fragment", func(t *testing.T) {
		f := newFront(t, bin)
		f.policy("auto", true)
		raw, _ := os.ReadFile(filepath.Join(f.root, "fragment.json"))
		var v map[string]any
		json.Unmarshal(raw, &v)
		v["fragment"] = "launch-env-fragment-v6"
		v["credential_source"] = map[string]any{"source": map[string]string{"mode": "node"}}
		writeFixture(t, filepath.Join(f.root, "fragment.json"), frontJSON(v), 0600)
		code, _, stderr := f.invoke(true, false, "--hosted")
		f.require(t, code, 1, stderr, "resolve_fragment_invalid", nil, 0, false)
	})
}

// Actual shadow observations correlate ordinary and reconciled hosted starts
// with the admitted handle. A refused start never produces a hosted outcome.
// The shadow worker is stubbed here; its detached lifetime is tested separately.
func testHostedShadowOutcome(t *testing.T, bin frontBinaries) {
	for _, fault := range []string{"none", "after-admission", "before-admission"} {
		t.Run(fault, func(t *testing.T) {
			f := newFront(t, bin)
			f.policy("auto", true)
			if fault != "none" {
				f.control(fakemanager.FixtureRequest{Operation: "inject_fault", Name: fault})
			}
			raw, err := os.ReadFile(filepath.Join(f.root, "fragment.json"))
			if err != nil {
				t.Fatal(err)
			}
			deps := testDeps(t, fragment.NewWithRunner("curator", &scriptedRunner{stdout: string(raw)}))
			deps.configLocations = func() (configfile.Locations, error) {
				return configfile.Locations{System: bin.system, User: f.config}, nil
			}
			deps.workdir = func() (string, error) { return f.root, nil }
			deps.environ = func() []string { return f.env }
			store, err := providerlimits.NewStore(providerlimits.Options{Layout: providerlimits.LayoutAt(f.root)})
			if err != nil {
				t.Fatal(err)
			}
			deps.availability = func() (plan.AvailabilityFunc, error) { return store.AvailabilityFor, nil }
			path := filepath.Join(f.root, "observations.ndjson")
			deps.observationPath = func() string { return path }
			deps.shadowStart = func(selector.ShadowJob) (*selector.Child, error) { return nil, nil }
			writeFixture(t, filepath.Join(bin.system, "selector.json"), frontJSON(map[string]any{
				"schema": selector.Schema, "mode": "shadow", "executable": bin.selector,
				"argv_prefix": []string{}, "timeout_seconds": 1,
			}), 0600)
			var stdout, stderr bytes.Buffer
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			code := run(ctx, []string{"claude_code", "--hosted", "--detach", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"}, &stdout, &stderr, deps)
			calls := []string{"describe", "launch"}
			want, starts, diagnostic := 0, 1, ""
			if fault != "none" {
				calls = append(calls, "status")
			}
			if fault == "before-admission" {
				calls = append(calls, "cancel")
				want, starts, diagnostic = 1, 0, "reason=cancelled"
			}
			f.require(t, code, want, stderr.String(), diagnostic, calls, starts, false)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var record selector.Observation
			if err = json.Unmarshal(bytes.TrimSpace(data), &record); err != nil {
				t.Fatal(err)
			}
			expected := "refused:session_host_unavailable"
			if starts == 1 {
				expected = "hosted:" + frontStored(t, f.observe()).Handle
			}
			if record.Kind != "observation" || record.Outcome != expected {
				t.Fatalf("shadow outcome=%q want=%q", record.Outcome, expected)
			}
		})
	}
}
