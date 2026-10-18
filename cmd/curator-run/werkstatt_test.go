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
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/selector"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/fakemanager"
	managerschema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
)

type wsFixture struct {
	*frontFixture
	record map[string]any
	id     string
}

func newWS(t *testing.T, bin frontBinaries) *wsFixture {
	f := newFront(t, bin)
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range f.env {
		if strings.HasPrefix(v, "PATH=") {
			f.env[i] = v + string(os.PathListSeparator) + filepath.Dir(gitPath)
		}
	}
	f.managerConfig.InitialState = "idle"
	f.reset()
	f.policy("auto", true)
	f.env = append(f.env, "A2A_CONFIG_DIR="+filepath.Join(f.root, "a2a"))
	if err := os.MkdirAll(filepath.Join(f.root, "a2a", "agents"), 0700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(f.root, "a2a", "agents", "sample.dev.json"), []byte("PRESENCE_ONLY_NOT_A_CREDENTIAL"), 0600)
	b, err := os.ReadFile(bin.provider)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(f.root, "bin", "a2a"), b, 0700)
	w := &wsFixture{frontFixture: f, id: "example", record: map[string]any{
		"schema": "werkstatt-record-v1", "id": "example", "project": "sample", "role": "developer", "runtime": "claude_code",
		"model": "claude-opus-5", "effort": "medium", "profile": "sample", "clone": f.root, "identity": "sample.dev",
		"room": "#sample", "orchestrator": "sample.orch", "sandbox": "none", "trust_class": "standard", "doorbell": nil,
	}}
	w.write()
	t.Cleanup(func() {
		history := f.control(fakemanager.FixtureRequest{Operation: "observe_calls"}).CallHistory
		if history == nil {
			t.Error("missing PID telemetry")
			return
		}
		for _, p := range history.CallProcesses {
			for _, pid := range []int{p.PID, -p.Group} {
				if !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
					_ = syscall.Kill(pid, syscall.SIGKILL)
					t.Errorf("manager survived: %d", pid)
				}
			}
		}
	})
	return w
}
func (w *wsFixture) write() {
	w.t.Helper()
	dir := filepath.Join(w.config, "werkstatt")
	if err := os.MkdirAll(dir, 0700); err != nil {
		w.t.Fatal(err)
	}
	writeFixture(w.t, filepath.Join(dir, w.id+".json"), frontJSON(w.record), 0600)
}
func (w *wsFixture) invokeWS(action string, args ...string) (int, string, string) {
	w.t.Helper()
	base := []string{"werkstatt", action, w.id}
	return w.command(w.bin.launcher, append(base, args...), nil, false, false, 35*time.Second)
}
func (w *wsFixture) runWS(action string, want int, diagnostic string, args ...string) string {
	w.t.Helper()
	code, out, stderr := w.invokeWS(action, args...)
	if code != want || diagnostic != "" && !strings.Contains(stderr, diagnostic) {
		w.t.Fatalf("%s exit=%d want=%d diagnostic=%s stdout=%s stderr=%s", action, code, want, diagnostic, out, stderr)
	}
	return out
}
func (w *wsFixture) path(suffix string) string {
	return filepath.Join(w.state, "swarma", "launcher", "werkstatt", w.id+suffix)
}
func (w *wsFixture) live() *client.Liveness {
	w.t.Helper()
	b, err := os.ReadFile(w.path(".json"))
	if err != nil {
		w.t.Fatal(err)
	}
	var v client.Liveness
	if json.Unmarshal(b, &v) != nil {
		w.t.Fatal("invalid liveness")
	}
	return &v
}
func (w *wsFixture) managerState() fakemanager.State {
	w.t.Helper()
	o := w.observe()
	var state fakemanager.State
	if json.Unmarshal(o.Records, &state) != nil {
		w.t.Fatal("invalid state")
	}
	return state
}
func (w *wsFixture) rows(out string) []map[string]json.RawMessage {
	w.t.Helper()
	var rows []map[string]json.RawMessage
	if err := managerschema.Validate("werkstatt-status.schema.json", []byte(out)); err != nil {
		w.t.Fatalf("invalid status: %v %s", err, out)
	}
	if json.Unmarshal([]byte(out), &rows) != nil {
		w.t.Fatal(out)
	}
	for _, row := range rows {
		if len(row) != 5 {
			w.t.Fatal("status row shape", row)
		}
	}
	return rows
}
func (w *wsFixture) noOperation() {
	w.t.Helper()
	if _, err := os.Stat(w.path(".op.json")); !errors.Is(err, os.ErrNotExist) {
		w.t.Fatal("pending operation", err)
	}
}
func (w *wsFixture) activity(activity string) {
	w.t.Helper()
	w.control(fakemanager.FixtureRequest{Operation: "set_activity", Session: w.live().Session, Activity: activity})
}
func (w *wsFixture) expectStarts(n int) {
	w.t.Helper()
	if o := w.observe(); o.Starts != n {
		w.t.Fatalf("starts=%d want=%d", o.Starts, n)
	}
}
func (w *wsFixture) calls() []string {
	return w.control(fakemanager.FixtureRequest{Operation: "observe_calls"}).CallHistory.Calls
}

// Real CLI invocations exercise every pre-start stage with nearby valid controls,
// typed refusal checks and no admitted operation on rejection. Fixture processes
// are bounded, reaped and checked by recorded PID/group; no real services run.
func TestWerkstattBinary(t *testing.T) {
	bin := frontBuild(t)
	t.Run("admission", func(t *testing.T) {
		for _, name := range []string{"valid", "missing", "invalid", "trial-off", "native-mode-trial", "native-mode-no-trial", "hosted-mode-trial", "hosted-mode-no-trial", "child-env", "identity-equals", "doorbell", "effort", "attach-capability", "credential-node", "unselected", "empty", "capability", "clone", "identity", "model", "fragment", "secret", "reserved", "weights", "git-missing", "git-valid", "git-mismatch", "CLAUDE.md", "CLAUDE.local.md", "AGENTS.md", "AGENTS.override.md", "physical-leak", "lexical-leak", "symlink-clean", "unreadable"} {
			t.Run(name, func(t *testing.T) {
				w := newWS(t, bin)
				want, diagnostic := 0, ""
				calls := []string{"describe", "list", "launch"}
				switch name {
				case "missing":
					_ = os.Remove(filepath.Join(w.config, "werkstatt", w.id+".json"))
					want, diagnostic, calls = 1, "werkstatt_not_found", nil
				case "invalid":
					w.record["extra"] = true
					w.write()
					want, diagnostic, calls = 2, "werkstatt_record_invalid", nil
				case "trial-off":
					w.policy("auto", false)
					want, diagnostic, calls = 16, "session_host_delegated_unauthorized", []string{"describe"}
				case "native-mode-trial", "hosted-mode-trial", "native-mode-no-trial", "hosted-mode-no-trial":
					mode := "native"
					if strings.HasPrefix(name, "hosted") {
						mode = "hosted"
					}
					trial := strings.HasSuffix(name, "-trial") && !strings.HasSuffix(name, "no-trial")
					w.policy(mode, trial)
					if !trial {
						want, diagnostic, calls = 16, "session_host_delegated_unauthorized", []string{"describe"}
					}
				case "identity-equals":
					w.record["identity"] = "example=dev"
					writeFixture(t, filepath.Join(w.root, "a2a", "agents", "example=dev.json"), []byte("PRESENCE_ONLY"), 0600)
					w.write()
				case "child-env":
					w.record["child_env"] = map[string]string{"GIT_AUTHOR_NAME": "Example", "GOCACHE": filepath.Join(w.root, "cache"), "SSH_AUTH_SOCK": filepath.Join(w.root, "socket")}
					w.write()
				case "doorbell":
					w.record["doorbell"] = map[string]any{"heartbeat": "1m", "max_wakes_per_hour": 6}
					w.write()
					// Default fixture already advertises doorbell-claude/1.
					w.reset()
				case "effort":
					w.record["effort"] = "not-an-effort"
					w.write()
					want, diagnostic, calls = 1, "plan_refused", []string{"describe", "list"}
				case "unselected", "empty":
					w.registry(name)
					want, diagnostic, calls = 1, "werkstatt_session_manager_required", nil
				case "capability":
					w.managerConfig.Capabilities = slices.DeleteFunc(w.managerConfig.Capabilities, func(c string) bool { return c == "admission/1" })
					w.reset()
					want, diagnostic, calls = 6, "capability=admission/1", []string{"describe"}
				case "clone":
					w.record["clone"] = filepath.Join(w.root, "absent")
					w.write()
					want, diagnostic, calls = 1, "werkstatt_clone_missing", []string{"describe", "list"}
				case "identity":
					_ = os.Remove(filepath.Join(w.root, "a2a", "agents", "sample.dev.json"))
					want, diagnostic, calls = 1, "werkstatt_identity_missing", []string{"describe", "list"}
				case "attach-capability":
					w.managerConfig.Capabilities = slices.DeleteFunc(w.managerConfig.Capabilities, func(c string) bool { return c == "attach/1" })
					w.reset()
					want, diagnostic, calls = 6, "capability=attach/1", []string{"describe"}
				case "credential-node":
					raw, _ := os.ReadFile(filepath.Join(w.root, "fragment.json"))
					var v map[string]any
					_ = json.Unmarshal(raw, &v)
					v["fragment"] = "launch-env-fragment-v6"
					v["credential_source"] = map[string]any{"source": map[string]string{"mode": "node"}}
					writeFixture(t, filepath.Join(w.root, "fragment.json"), frontJSON(v), 0600)
					want, diagnostic, calls = 1, "resolve_fragment_invalid", []string{"describe", "list"}
				case "model":
					w.record["model"] = "not-a-model"
					w.write()
					want, diagnostic, calls = 1, "plan_refused", []string{"describe", "list"}
				case "fragment":
					writeFixture(t, filepath.Join(w.root, "fragment.json"), []byte("{"), 0600)
					want, diagnostic, calls = 1, "resolve_fragment_invalid", []string{"describe", "list"}
				case "secret":
					w.record["child_env"] = map[string]string{"API_TOKEN": "sentinel"}
					w.write()
					want, diagnostic, calls = 16, "secret_policy_violation", nil
				case "reserved":
					w.record["child_env"] = map[string]string{"LD_PRELOAD": "sentinel"}
					w.write()
					want, diagnostic, calls = 2, "credential_reserved_name", nil
				case "weights":
					w.record["weights_id"] = "gguf-sha256:" + strings.Repeat("a", 64)
					w.write()
					want, diagnostic = 6, "weights-pin"
				case "git-missing":
					w.record["git_dir"] = filepath.Join(w.root, "absent")
					w.write()
					want, diagnostic, calls = 2, "werkstatt_git_layout_invalid", []string{"describe", "list"}
				case "git-valid", "git-mismatch":
					clone := filepath.Join(w.root, "clone")
					_ = os.Mkdir(clone, 0700)
					frag, _ := os.ReadFile(filepath.Join(w.root, "fragment.json"))
					writeFixture(t, filepath.Join(clone, "fragment.json"), frag, 0600)
					gitdir := filepath.Join(w.root, "git-state")
					_ = os.Mkdir(gitdir, 0700)
					target := clone
					if name == "git-mismatch" {
						target = w.root
						want, diagnostic, calls = 2, "worktree_mismatch", []string{"describe", "list"}
					}
					writeFixture(t, filepath.Join(gitdir, "config"), []byte("[core]\n worktree = "+target+"\n"), 0600)
					w.record["clone"], w.record["git_dir"] = clone, gitdir
					w.write()
				case "CLAUDE.md", "CLAUDE.local.md", "AGENTS.md", "AGENTS.override.md", "physical-leak", "lexical-leak", "symlink-clean", "unreadable":
					physical := filepath.Join(w.root, "physical")
					lexical := filepath.Join(w.root, "lexical")
					clone := filepath.Join(physical, "clone")
					_ = os.MkdirAll(clone, 0700)
					_ = os.Mkdir(lexical, 0700)
					frag, _ := os.ReadFile(filepath.Join(w.root, "fragment.json"))
					writeFixture(t, filepath.Join(clone, "fragment.json"), frag, 0600)
					w.record["clone"] = clone
					if name == "physical-leak" || name == "lexical-leak" || name == "symlink-clean" {
						alias := filepath.Join(lexical, "clone")
						if err := os.Symlink(clone, alias); err != nil {
							t.Fatal(err)
						}
						w.record["clone"] = alias
					}
					if name == "unreadable" {
						_ = os.Chmod(physical, 0000)
						t.Cleanup(func() { _ = os.Chmod(physical, 0700) })
						want, diagnostic, calls = 2, "werkstatt_context_check_failed", []string{"describe", "list"}
					} else if name != "symlink-clean" {
						p := physical
						file := name
						if name == "physical-leak" {
							file = "CLAUDE.md"
						}
						if name == "lexical-leak" {
							p = lexical
							file = "AGENTS.override.md"
						}
						writeFixture(t, filepath.Join(p, file), []byte("inert"), 0600)
						want, diagnostic, calls = 2, "werkstatt_context_leak", []string{"describe", "list"}
					}
					w.write()
				}
				args := []string{}
				if name == "attach-capability" {
					args = []string{"--attach"}
				}
				w.runWS("up", want, diagnostic, args...)
				if !slices.Equal(w.calls(), calls) {
					t.Fatalf("calls=%v want=%v", w.calls(), calls)
				}
				starts := 0
				if want == 0 {
					starts = 1
				}
				w.expectStarts(starts)
				if _, err := os.Stat(filepath.Join(w.root, "native-called")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("native execution escaped manager")
				}
				w.noOperation()
				if want == 0 {
					l := w.live()
					if l.State != "idle" || l.Manager.ID != "fake" || l.Generation != 1 {
						t.Fatal(l)
					}
					stored := frontStored(t, w.observe())
					var request map[string]any
					_ = json.Unmarshal(stored.Plan, &request)
					child := request["child_env"].(map[string]any)
					if child["A2A_AGENT"] != w.record["identity"] || len(request["labels"].(map[string]any)) != 2 || request["labels"].(map[string]any)["identity"] != w.record["identity"] || request["admission_key"] != "werkstatt:example" || request["detach"] != true || request["board_goal"] != "none" || request["harness_memory"] != true {
						t.Fatal(request)
					}
					if name == "child-env" && (child["GIT_AUTHOR_NAME"] != "Example" || child["GOCACHE"] != filepath.Join(w.root, "cache") || child["SSH_AUTH_SOCK"] != filepath.Join(w.root, "socket") || len(child) != 4) {
						t.Fatal(child)
					}
					if name == "doorbell" && request["doorbell"].(map[string]any)["identity"] != "sample.dev" {
						t.Fatal(request)
					}
					if name == "git-valid" && (child["GIT_DIR"] != w.record["git_dir"] || child["GIT_WORK_TREE"] != w.record["clone"]) {
						t.Fatal(child)
					}
				}
			})
		}
	})
	t.Run("lifecycle", func(t *testing.T) {
		w := newWS(t, bin)
		w.runWS("up", 0, "")
		first := *w.live()
		w.runWS("up", 1, "werkstatt_already_up")
		w.expectStarts(1)
		w.runWS("status", 0, "", "--set-task", "TASK-example", "--queue-depth", "2", "--json")
		if l := w.live(); l.CurrentTask == nil || *l.CurrentTask != "TASK-example" || l.QueueDepth != 2 {
			t.Fatal(l)
		}
		before, _ := os.ReadFile(w.path(".json"))
		w.activity("busy")
		w.runWS("down", 1, "state=busy")
		after, _ := os.ReadFile(w.path(".json"))
		if !bytes.Equal(before, after) {
			t.Fatal("busy refusal wrote liveness")
		}
		if !w.observe().Gates[first.Session] {
			t.Fatal("busy refusal closed gate")
		}
		w.activity("idle")
		w.runWS("down", 0, "")
		if w.live().State != "down" {
			t.Fatal("down not persisted")
		}
		w.runWS("up", 0, "")
		second := w.live()
		if second.Session != first.Session || second.Generation != 2 || second.QueueDepth != 2 || second.CurrentTask == nil {
			t.Fatal("continue lost identity or metadata", second)
		}
		w.runWS("recycle", 0, "")
		third := w.live()
		if third.Session == first.Session || third.Generation != 3 {
			t.Fatal("recycle did not start new thread", third)
		}
		rows := w.rows(w.runWS("status", 0, "", "--clear-task", "--json"))
		if len(rows) != 1 || w.live().CurrentTask != nil {
			t.Fatal(rows)
		}
		code, out, stderr := w.command(bin.launcher, []string{"werkstatt", "list", "--json"}, nil, false, false, 12*time.Second)
		if code != 0 || len(w.rows(out)) != 1 {
			t.Fatal(code, out, stderr)
		}
	})
	t.Run("attach", func(t *testing.T) {
		w := newWS(t, bin)
		code, _, stderr := w.command(bin.launcher, []string{"werkstatt", "up", w.id, "--attach"}, nil, true, false, 12*time.Second)
		if code != 0 {
			t.Fatal(code, stderr)
		}
		w.expectStarts(1)
		if !slices.Equal(w.calls(), []string{"describe", "list", "launch", "attach"}) {
			t.Fatal(w.calls())
		}
	})
	t.Run("lost-answer", func(t *testing.T) {
		for _, fault := range []string{"after-admission", "before-admission", "crash-after-start"} {
			t.Run(fault, func(t *testing.T) {
				w := newWS(t, bin)
				w.control(fakemanager.FixtureRequest{Operation: "inject_fault", Name: fault})
				switch fault {
				case "after-admission":
					w.runWS("up", 0, "")
					w.expectStarts(1)
					w.noOperation()
				case "before-admission":
					w.runWS("up", 1, "reason=cancelled")
					w.expectStarts(0)
					w.noOperation()
				case "crash-after-start":
					w.runWS("up", 1, "session_liveness_unknown")
					before, e := os.ReadFile(w.path(".op.json"))
					if e != nil {
						t.Fatal(e)
					}
					w.runWS("up", 1, "session_liveness_unknown")
					after, _ := os.ReadFile(w.path(".op.json"))
					if !bytes.Equal(before, after) {
						t.Fatal("pending op changed")
					}
					w.expectStarts(1)
					w.runWS("status", 0, "", "--abandon-operation", "--json")
					l := w.live()
					if l.State != "down" || l.StateSource != "operator_override" || l.Session != "" || l.Generation != 0 || len(l.AbandonedOperations) != 1 {
						t.Fatal(l)
					}
					w.noOperation()
				}
			})
		}
	})
	// A local write failure after admission still prints the admitted handle,
	// retains correlation, and is recovered by status without another launch.
	t.Run("liveness-write-recovery", func(t *testing.T) {
		w := newWS(t, bin)
		wrapper := filepath.Join(w.root, "manager-wrapper")
		quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
		script := "#!/bin/sh\nreply=$(" + quote(bin.manager) + " \"$@\")\nresult=$?\nif [ \"$1\" = launch ] && [ \"$result\" = 0 ]; then /bin/mkdir " + quote(w.path(".json")) + " || exit 2; fi\nprintf '%s' \"$reply\"\nexit \"$result\"\n"
		writeFixture(t, wrapper, []byte(script), 0700)
		registry, _, err := sessionmanager.Load(configfile.Resolver{Locations: configfile.Locations{System: bin.system, User: w.config}})
		if err != nil {
			t.Fatal(err)
		}
		entry := registry.Managers["fake"]
		entry.Executable = wrapper
		registry.Managers["fake"] = entry
		b, _ := sessionmanager.Encode(registry)
		writeFixture(t, filepath.Join(w.config, sessionmanager.Filename), b, 0600)
		out := w.runWS("up", 1, "werkstatt_liveness_write_failed")
		stored := frontStored(t, w.observe())
		if strings.TrimSpace(out) != stored.Handle {
			t.Fatal("admitted handle lost", out)
		}
		pending, err := os.ReadFile(w.path(".op.json"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(pending), wrapper) {
			t.Fatal("owner correlation lost")
		}
		if err := os.Remove(w.path(".json")); err != nil {
			t.Fatal(err)
		}
		w.runWS("status", 0, "", "--json")
		w.noOperation()
		w.expectStarts(1)
		if w.live().Session != stored.Handle {
			t.Fatal(w.live())
		}
		if !slices.Equal(w.calls(), []string{"describe", "list", "launch", "status", "status"}) {
			t.Fatal(w.calls())
		}
	})

	t.Run("concurrent-up", func(t *testing.T) {
		w := newWS(t, bin)
		type result struct {
			code int
			err  string
		}
		results := make(chan result, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() { defer wg.Done(); code, _, err := w.invokeWS("up"); results <- result{code, err} }()
		}
		wg.Wait()
		close(results)
		wins, refused := 0, 0
		for r := range results {
			if r.code == 0 {
				wins++
			} else if r.code == 1 && (strings.Contains(r.err, "werkstatt_busy") || strings.Contains(r.err, "werkstatt_already_up")) {
				refused++
			} else {
				t.Fatal(r)
			}
		}
		if wins != 1 || refused != 1 {
			t.Fatal(wins, refused)
		}
		w.expectStarts(1)
	})
	t.Run("owner-rebind", func(t *testing.T) {
		w := newWS(t, bin)
		w.runWS("up", 0, "")
		l := w.live()
		original := l.Manager
		r, _, e := sessionmanager.Load(configfile.Resolver{Locations: configfile.Locations{System: bin.system, User: w.config}})
		if e != nil {
			t.Fatal(e)
		}
		entry := r.Managers["fake"]
		entry.RegisteredAt = "2026-02-01T00:00:00Z"
		r.Managers["fake"] = entry
		data, _ := sessionmanager.Encode(r)
		writeFixture(t, filepath.Join(w.config, sessionmanager.Filename), data, 0600)
		w.runWS("up", 1, "reason=manager_changed")
		w.runWS("down", 1, "reason=manager_changed")
		if w.live().Manager != original {
			t.Fatal("implicit rebind")
		}
		w.runWS("status", 0, "", "--rebind-manager", "--json")
		if w.live().Manager.RegisteredAt != entry.RegisteredAt {
			t.Fatal("rebind failed")
		}
		w.runWS("down", 0, "")
	})
	t.Run("unreadable-status", func(t *testing.T) {
		w := newWS(t, bin)
		w.runWS("up", 0, "")
		before := *w.live()
		w.control(fakemanager.FixtureRequest{Operation: "corrupt_index", Kind: "unreadable", Session: before.Session})
		w.runWS("up", 1, "session_liveness_unknown")
		w.runWS("down", 1, "session_liveness_unknown")
		rows := w.rows(w.runWS("status", 0, "", "--json"))
		var m map[string]any
		_ = json.Unmarshal(rows[0]["manager_session"], &m)
		if m["readable"] != false {
			t.Fatal(m)
		}
		after := w.live()
		if after.State != "unknown" || after.Generation != before.Generation || after.Session != before.Session || after.StartedAt != before.StartedAt || after.Manager != before.Manager {
			t.Fatal("projection destroyed evidence", after)
		}
	})
	// a2a presence is shown separately and cannot alter the manager's state.
	t.Run("a2a-separate", func(t *testing.T) {
		for _, mode := range []string{"default", "heartbeat", "unavailable"} {
			t.Run(mode, func(t *testing.T) {
				w := newWS(t, bin)
				want := "3m0s"
				if mode == "heartbeat" {
					w.record["doorbell"] = map[string]any{"heartbeat": "30s", "max_wakes_per_hour": 6}
					w.write()
					want = "1m30s"
				}
				w.runWS("up", 0, "")
				writeFixture(t, filepath.Join(w.root, "a2a-who.json"), []byte(`[{"agent":"sample.dev","state":"busy"}]`), 0600)
				if mode == "unavailable" {
					writeFixture(t, filepath.Join(w.root, "a2a-who-fail"), []byte("inert"), 0600)
				}
				rows := w.rows(w.runWS("status", 0, "", "--json"))
				var presence map[string]any
				if json.Unmarshal(rows[0]["a2a"], &presence) != nil || w.live().State != "idle" {
					t.Fatal(rows, w.live())
				}
				if mode == "unavailable" {
					if string(rows[0]["a2a"]) != "null" {
						t.Fatal("failed who fabricated presence")
					}
				} else if presence["state"] != "busy" {
					t.Fatal(rows)
				}
				b, e := os.ReadFile(filepath.Join(w.root, "a2a-argv.json"))
				if e != nil {
					t.Fatal(e)
				}
				var args []string
				_ = json.Unmarshal(b, &args)
				if !slices.Equal(args, []string{"who", "--stale-after", want, "--json"}) {
					t.Fatal(args)
				}
			})
		}
	})

	t.Run("stale-without-liveness", func(t *testing.T) {
		w := newWS(t, bin)
		tx, e := (client.Store{Directory: filepath.Dir(w.path(".json"))}).Lock(context.Background(), w.id)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Close()
		rows := w.rows(w.runWS("status", 0, "", "--json"))
		if string(rows[0]["liveness"]) != "null" || string(rows[0]["stale"]) != "true" {
			t.Fatal(rows)
		}
		if _, e = os.Stat(w.path(".json")); !errors.Is(e, os.ErrNotExist) {
			t.Fatal("stale status wrote liveness")
		}
	})

	// Continue uses recorded ownership first, then a unique newest down session.
	// Fresh/new controls never resume, and ambiguous or unreadable candidates
	// refuse without admitting another operation.
	t.Run("resume-choice", func(t *testing.T) {
		for _, mode := range []string{"newest", "tie", "unreadable", "new", "fresh", "resume-capability"} {
			t.Run(mode, func(t *testing.T) {
				w := newWS(t, bin)
				w.control(fakemanager.FixtureRequest{Operation: "set_clock", Now: "2026-01-01T00:00:00Z"})
				w.runWS("up", 0, "")
				first := w.live().Session
				w.runWS("down", 0, "")
				if mode == "resume-capability" {
					w.managerConfig.Capabilities = slices.DeleteFunc(w.managerConfig.Capabilities, func(c string) bool { return c == "resume/1" })
					// Changing fixture configuration must preserve the sessions under test.
					writeFixture(t, filepath.Join(w.state, "curator-run", "fake-manager", "config.json"), frontJSON(w.managerConfig), 0600)
					w.runWS("up", 6, "capability=resume/1")
					w.expectStarts(1)
					w.noOperation()
					return
				}
				if mode == "newest" {
					w.control(fakemanager.FixtureRequest{Operation: "set_clock", Now: "2026-01-01T00:00:01Z"})
				}
				w.runWS("up", 0, "", "--new")
				second := w.live().Session
				w.runWS("down", 0, "")
				if first == second {
					t.Fatal("--new resumed")
				}
				if err := os.Remove(w.path(".json")); err != nil {
					t.Fatal(err)
				}
				if mode == "unreadable" {
					w.control(fakemanager.FixtureRequest{Operation: "corrupt_index", Kind: "unreadable", Session: second})
				}
				switch mode {
				case "tie", "unreadable":
					w.runWS("up", 1, "session_resume_ambiguous")
					w.expectStarts(2)
				case "new", "fresh":
					args := []string{"--new"}
					if mode == "fresh" {
						w.record["context_mode"] = "fresh"
						w.write()
						args = nil
					}
					w.runWS("up", 0, "", args...)
					if l := w.live(); l.Session == first || l.Session == second {
						t.Fatal("fresh launch resumed", l)
					}
					w.expectStarts(3)
				default:
					w.runWS("up", 0, "")
					if w.live().Session != second {
						t.Fatal("newest not resumed")
					}
					w.expectStarts(3)
				}
				w.noOperation()
			})
		}
	})
	// Down reads the manager's current generation rather than a stale local
	// generation; force bypasses only the busy precondition. Draining retains
	// admission until the provider is gone, including recycle's bounded refusal.
	t.Run("stopping", func(t *testing.T) {
		for _, mode := range []string{"force", "draining", "recycle-draining", "recycle-timeout"} {
			t.Run(mode, func(t *testing.T) {
				w := newWS(t, bin)
				w.runWS("up", 0, "")
				first := w.live().Session
				if mode == "force" {
					w.activity("busy")
					w.runWS("down", 0, "", "--force")
					if w.live().State != "down" || w.observe().Alive[first] {
						t.Fatal("force did not stop")
					}
					return
				}
				w.control(fakemanager.FixtureRequest{Operation: "set_activity", Session: first, Activity: "graceful-hang", Enabled: true})
				if mode == "draining" {
					w.runWS("down", 0, "")
					if w.live().State != "draining" {
						t.Fatal(w.live())
					}
					w.runWS("up", 1, "werkstatt_already_up")
					w.expectStarts(1)
					w.control(fakemanager.FixtureRequest{Operation: "set_activity", Session: first, Activity: "provider-gone", Enabled: true})
					w.runWS("status", 0, "", "--json")
					if w.live().State != "down" {
						t.Fatal(w.live())
					}
					return
				}
				if mode == "recycle-timeout" {
					w.runWS("recycle", 1, "state=draining")
					w.expectStarts(1)
					if w.live().State != "draining" {
						t.Fatal(w.live())
					}
					w.noOperation()
					return
				}
				done := make(chan struct{})
				type answer struct {
					code int
					err  string
				}
				result := make(chan answer, 1)
				go func() {
					r := answer{code: -1}
					defer func() { result <- r; close(done) }()
					r.code, _, r.err = w.invokeWS("recycle")
				}()
				t.Cleanup(func() { <-done })
				deadline := time.NewTimer(10 * time.Second)
				defer deadline.Stop()
				for {
					if strings.Contains(string(w.managerState().Sessions[first].Record), `"state":"draining"`) {
						break
					}
					select {
					case <-done:
						t.Fatal("recycle ended before draining")
					case <-deadline.C:
						t.Fatal("no draining state")
					case <-time.After(20 * time.Millisecond):
					}
				}
				w.control(fakemanager.FixtureRequest{Operation: "set_activity", Session: first, Activity: "provider-gone", Enabled: true})
				<-done
				r := <-result
				if r.code != 0 {
					t.Fatal(r)
				}
				if l := w.live(); l.Session == first || l.Generation != 2 {
					t.Fatal(l)
				}
				w.expectStarts(2)
				w.noOperation()
			})
		}
	})
	// The owner status, not stale liveness, supplies stop's generation.
	t.Run("current-stop-generation", func(t *testing.T) {
		w := newWS(t, bin)
		w.runWS("up", 0, "")
		old, err := os.ReadFile(w.path(".json"))
		if err != nil {
			t.Fatal(err)
		}
		w.runWS("down", 0, "")
		w.runWS("up", 0, "")
		if w.live().Generation != 2 {
			t.Fatal(w.live())
		}
		writeFixture(t, w.path(".json"), old, 0600)
		w.runWS("down", 0, "")
		if l := w.live(); l.Generation != 2 || l.State != "down" {
			t.Fatal(l)
		}
	})

	t.Run("override-and-dispatch", func(t *testing.T) {
		w := newWS(t, bin)
		w.runWS("status", 1, "werkstatt_not_up", "--set-task", "example")
		if _, err := os.Stat(w.path(".json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("invented liveness")
		}
		w.runWS("up", 0, "")
		first := w.live().Session
		w.runWS("status", 0, "", "--abandon-operation", "--json")
		if l := w.live(); l.State != "down" || l.StateSource != "operator_override" || len(l.AbandonedOperations) != 1 {
			t.Fatal(l)
		}
		if !w.observe().Alive[first] {
			t.Fatal("abandon stopped provider")
		}
		w.runWS("status", 0, "", "--json")
		if l := w.live(); l.State != "idle" || l.StateSource != "manager" || l.Session != first {
			t.Fatal("owner evidence not restored", l)
		}
		w.runWS("up", 1, "werkstatt_already_up")
		w.expectStarts(1)
	})
	// A different selected manager cannot supersede an active owner. Once down
	// is established, the new manager's own generation and binding are installed;
	// later status/down still route to it after selection changes again.
	t.Run("manager-switch", func(t *testing.T) {
		w := newWS(t, bin)
		w.runWS("up", 0, "")
		old := *w.live()
		other := filepath.Join(w.root, "other-manager")
		otherState := filepath.Join(w.root, "other-state")
		quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
		writeFixture(t, other, []byte("#!/bin/sh\nexec "+quote(bin.manager)+" --state "+quote(otherState)+" \"$@\"\n"), 0700)
		otherControl := func(r fakemanager.FixtureRequest) fakemanager.FixtureResponse {
			code, out, stderr := w.command(bin.adapter, []string{"--state", otherState}, frontJSON(r), false, false, 10*time.Second)
			var v fakemanager.FixtureResponse
			if code != 0 || json.Unmarshal([]byte(out), &v) != nil || !v.OK {
				t.Fatal(code, out, stderr)
			}
			return v
		}
		cfg := w.managerConfig
		cfg.ID = "other"
		otherControl(fakemanager.FixtureRequest{Operation: "reset", Config: &cfg})
		t.Cleanup(func() {
			h := otherControl(fakemanager.FixtureRequest{Operation: "observe_calls"}).CallHistory
			for _, p := range h.CallProcesses {
				for _, pid := range []int{p.PID, -p.Group} {
					if !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
						_ = syscall.Kill(pid, syscall.SIGKILL)
						t.Errorf("manager survived: %d", pid)
					}
				}
			}
		})
		r, _, err := sessionmanager.Load(configfile.Resolver{Locations: configfile.Locations{System: bin.system, User: w.config}})
		if err != nil {
			t.Fatal(err)
		}
		r.Managers["other"] = sessionmanager.Entry{RegisteredBy: "fixture", Protocol: client.Protocol, Executable: other, ArgvPrefix: []string{}, RegisteredAt: "2026-01-02T00:00:00Z"}
		selected := "other"
		r.Selected = selected
		save := func() {
			b, e := sessionmanager.Encode(r)
			if e != nil {
				t.Fatal(e)
			}
			writeFixture(t, filepath.Join(w.config, sessionmanager.Filename), b, 0600)
		}
		save()
		code, _, stderr := w.invokeWS("up")
		if otherControl(fakemanager.FixtureRequest{Operation: "observe"}).Observation.Starts != 0 {
			t.Fatal("cross-manager duplicate")
		}
		if code != 1 || !strings.Contains(stderr, "werkstatt_already_up") {
			t.Fatal(code, stderr)
		}
		if w.live().Manager != old.Manager {
			t.Fatal("owner bypassed")
		}
		if otherControl(fakemanager.FixtureRequest{Operation: "observe"}).Observation.Starts != 0 {
			t.Fatal("cross-manager duplicate")
		}
		w.runWS("down", 0, "")
		w.runWS("up", 0, "")
		current := *w.live()
		if current.Manager.ID != "other" || current.Generation != 1 || current.Session == old.Session {
			t.Fatal(current)
		}
		r.Selected = "fake"
		save()
		rows := w.rows(w.runWS("status", 0, "", "--json"))
		if string(rows[0]["manager_session"]) == "null" || w.live().Manager.ID != "other" {
			t.Fatal(rows)
		}
		w.runWS("down", 0, "")
		if w.live().State != "down" {
			t.Fatal(w.live())
		}
		if w.observe().Starts != 1 || otherControl(fakemanager.FixtureRequest{Operation: "observe"}).Observation.Starts != 1 {
			t.Fatal("unexpected starts")
		}
	})

	t.Run("selector", func(t *testing.T) {
		w := newWS(t, bin)
		writeFixture(t, filepath.Join(bin.system, "selector.json"), frontJSON(map[string]any{"schema": "curator-run-selector-v1", "mode": "enforce", "executable": bin.selector, "argv_prefix": []string{"valid"}, "timeout_seconds": 5}), 0600)
		writeFixture(t, filepath.Join(filepath.Dir(bin.selector), "response.json"), []byte(`{"decision_id":"dispatch/@+1","runtime":"claude","model":"claude-opus-5","effort":"high","weights_id":null,"refusal":null}`), 0600)
		cleanupSelectorFake(t, bin.selector)
		w.runWS("up", 0, "", "--task-class", "code.implement", "--difficulty", "high")
		b, e := os.ReadFile(filepath.Join(filepath.Dir(bin.selector), "request.json"))
		if e != nil {
			t.Fatal(e)
		}
		var req selector.Request
		if json.Unmarshal(b, &req) != nil || req.Werkstatt == nil || *req.Werkstatt != w.id || req.Role == nil || *req.Role != "developer" || req.Task == nil || req.Task.Class != "code.implement" || req.Task.Difficulty == nil || *req.Task.Difficulty != "high" {
			t.Fatal(string(b))
		}
		var request map[string]any
		_ = json.Unmarshal(frontStored(t, w.observe()).Plan, &request)
		if request["model"] != "claude-opus-5" || request["effort"] != "high" || request["labels"].(map[string]any)["selector"] != "dispatch/@+1" {
			t.Fatal(request)
		}
	})
}
