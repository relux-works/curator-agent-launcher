//go:build darwin || linux

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/fakemanager"
)

func hookEvidence(t *testing.T, f *frontFixture) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, "a2a-hook.json"))
	if err != nil {
		t.Fatal("missing hook evidence", err)
	}
	var evidence map[string]json.RawMessage
	if json.Unmarshal(data, &evidence) != nil {
		t.Fatal("invalid hook evidence")
	}
	var pid, group int
	json.Unmarshal(evidence["pid"], &pid)
	json.Unmarshal(evidence["group"], &group)
	for _, p := range []int{pid, -group} {
		if pid < 1 || group < 1 {
			t.Fatal("missing hook PID/group")
		}
		if !errors.Is(syscall.Kill(p, 0), syscall.ESRCH) {
			_ = syscall.Kill(p, syscall.SIGKILL)
			t.Fatalf("hook helper survived: %d", p)
		}
	}
	return evidence
}
func hookAbsent(t *testing.T, f *frontFixture) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(f.root, "a2a-hook.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("forbidden hook execution", err)
	}
}
func doorbellLine(t *testing.T, text, state, reason string) {
	t.Helper()
	if strings.Count(text, "curator-run: doorbell ") != 1 || !strings.Contains(text, "doorbell state="+state) || reason != "" && !strings.Contains(text, "reason="+reason) {
		t.Fatalf("doorbell line mismatch: %s", text)
	}
}

// The real launcher installs and independently verifies the three exact Claude
// commands before any start. Failures retain no pending operation and start no
// provider; unrelated settings/hooks survive the positive control.
func TestDoorbellBinary(t *testing.T) {
	bin := frontBuild(t)
	t.Run("hooks", func(t *testing.T) {
		for _, mode := range []string{"success", "fail", "missing-stop", "wrong-agent", "wrong-command", "wrong-type", "extra-argument", "wrong-executable", "malformed", "fifo", "timeout", "post-unsafe-directory", "post-hard-link", "post-symlink"} {
			t.Run(mode, func(t *testing.T) {
				w := newWS(t, bin)
				w.record["doorbell"] = map[string]any{"heartbeat": "2m", "max_wakes_per_hour": 4}
				w.write()
				writeFixture(t, filepath.Join(w.root, "a2a-hook-mode"), []byte(mode), 0600)
				settings := filepath.Join(w.root, ".claude", "settings.local.json")
				if mode == "success" {
					if err := os.MkdirAll(filepath.Dir(settings), 0700); err != nil {
						t.Fatal(err)
					}
					writeFixture(t, settings, frontJSON(map[string]any{"unrelated": map[string]any{"kept": true}, "env": map[string]any{"KEEP": "yes"}, "hooks": map[string]any{"OtherEvent": "leave alone", "Stop": []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "user-hook"}}}}}}), 0600)
				}
				code, out, stderr := w.invokeWS("up")
				want, step := 1, "verify"
				if mode == "success" {
					want = 0
					step = ""
				} else if mode == "fail" || mode == "timeout" {
					step = "install"
				}
				if code != want {
					t.Fatalf("hook exit=%d want=%d stdout=%s stderr=%s", code, want, out, stderr)
				}
				hookEvidence(t, w.frontFixture)
				if strings.Contains(stderr, "UNTRUSTED_INSTALL_OUTPUT") {
					t.Fatal("installer output leaked")
				}
				w.noOperation()
				if mode != "success" {
					if !strings.Contains(stderr, "doorbell_setup_failed") || !strings.Contains(stderr, "step="+step) {
						t.Fatal("wrong hook failure", stderr)
					}
					w.expectStarts(0)
					if !slices.Equal(w.calls(), []string{"describe", "list"}) {
						t.Fatal("start after hook failure", w.calls())
					}
					return
				}
				w.expectStarts(1)
				doorbellLine(t, stderr, "armed", "")
				if strings.Count(stderr, "a2a executable=") != 1 {
					t.Fatal("missing frozen executable diagnostic", stderr)
				}
				var request map[string]json.RawMessage
				json.Unmarshal(frontStored(t, w.observe()).Plan, &request)
				if string(request["doorbell"]) != `{"heartbeat":"2m","identity":"sample.dev","max_wakes_per_hour":4}` {
					t.Fatal("record doorbell lost", string(request["doorbell"]))
				}
				data, _ := os.ReadFile(settings)
				if !strings.Contains(string(data), "user-hook") || !strings.Contains(string(data), "leave alone") || !strings.Contains(string(data), `"KEEP":"yes"`) || !strings.Contains(string(data), `"kept":true`) {
					t.Fatal("unrelated settings removed", string(data))
				}
				w.runWS("down", 0, "")
				w.runWS("up", 0, "")
				if !slices.Contains(w.calls(), "resume") {
					t.Fatal("missing resume", w.calls())
				}
				hookEvidence(t, w.frontFixture)
			})
		}
	})
	// Launch and text status print manager evidence exactly once; JSON stays a
	// bare array and carries all reason/uncertainty fields without extra text.
	t.Run("surface", func(t *testing.T) {
		for _, variant := range []string{"armed", "off", "unsupported_version", "previous_generation_uncertain"} {
			t.Run(variant, func(t *testing.T) {
				w := newWS(t, bin)
				if variant != "off" {
					w.record["doorbell"] = map[string]any{"heartbeat": "1m", "max_wakes_per_hour": 6}
					w.write()
				}
				if variant == "unsupported_version" || variant == "previous_generation_uncertain" {
					w.control(fakemanager.FixtureRequest{Operation: "set_doorbell", Name: variant})
				}
				code, _, stderr := w.invokeWS("up")
				if code != 0 {
					t.Fatal("up failed", stderr)
				}
				state, reason := variant, ""
				if variant == "unsupported_version" {
					state, reason = "unavailable", variant
				}
				if variant == "previous_generation_uncertain" {
					state, reason = "off", variant
				}
				doorbellLine(t, stderr, state, reason)
				if variant == "off" {
					hookAbsent(t, w.frontFixture)
				} else {
					hookEvidence(t, w.frontFixture)
				}
				code, out, stderr := w.invokeWS("status")
				if code != 0 {
					t.Fatal(stderr)
				}
				doorbellLine(t, out+stderr, state, reason)
				if variant == "previous_generation_uncertain" && !strings.Contains(out, "uncertain=true") {
					t.Fatal("uncertainty lost", out)
				}
				code, out, stderr = w.invokeWS("status", "--json")
				if code != 0 {
					t.Fatal(stderr)
				}
				rows := w.rows(out)
				if !strings.Contains(string(rows[0]["manager_session"]), `"state":"`+state+`"`) || strings.Contains(out+stderr, "curator-run: doorbell ") {
					t.Fatal("JSON surface", out, stderr)
				}
				w.expectStarts(1)
			})
		}
	})
	// Plain hosted Claude uses default doorbell values; explicit/default native
	// routes and missing runtime capabilities have no installation/start effects.
	// A leading-hyphen identity reaches the manager request unchanged.
	t.Run("plain", func(t *testing.T) {
		for _, variant := range []string{"success", "hyphen-identity", "native", "native-default", "missing-capability", "missing-executable"} {
			t.Run(variant, func(t *testing.T) {
				w := newWS(t, bin)
				identity := "sample.dev"
				args := []string{"--hosted", "--detach", "--a2a", identity}
				want, diagnostic, calls, starts := 0, "", []string{"describe", "launch"}, 1
				switch variant {
				case "hyphen-identity":
					identity = "-sample.dev"
					args[3] = identity
				case "native":
					args = []string{"--native", "--a2a", "sample.dev"}
					want, diagnostic, calls, starts = 2, "usage", nil, 0
				case "native-default":
					w.policy("native", false)
					args = []string{"--a2a", "sample.dev"}
					want, diagnostic, calls, starts = 2, "usage", nil, 0
				case "missing-capability":
					w.managerConfig.Capabilities = slices.DeleteFunc(w.managerConfig.Capabilities, func(s string) bool { return s == "doorbell-claude/1" })
					w.reset()
					want, diagnostic, calls, starts = 6, "doorbell-claude/1", []string{"describe"}, 0
				case "missing-executable":
					for n, v := range w.env {
						if strings.HasPrefix(v, "PATH=") {
							w.env[n] = "PATH=" + filepath.Join(w.root, "bin")
						}
					}
					want, diagnostic, calls, starts = 1, "doorbell_setup_failed", []string{"describe"}, 0
				}
				code, _, stderr := w.invoke(false, false, args...)
				w.require(t, code, want, stderr, diagnostic, calls, starts, false)
				if variant != "success" && variant != "hyphen-identity" {
					hookAbsent(t, w.frontFixture)
					return
				}
				hookEvidence(t, w.frontFixture)
				doorbellLine(t, stderr, "armed", "")
				var request map[string]json.RawMessage
				json.Unmarshal(frontStored(t, w.observe()).Plan, &request)
				if string(request["doorbell"]) != `{"heartbeat":"1m","identity":"`+identity+`","max_wakes_per_hour":6}` {
					t.Fatal("default doorbell lost", string(request["doorbell"]))
				}
			})
		}
	})
	// Unsafe existing settings entries refuse before the installer can modify
	// either an outside target or a contained target. A plain private directory
	// beside every plant is a nearby valid control.
	t.Run("settings-path", func(t *testing.T) {
		for _, plant := range []string{"outside-symlink", "contained-symlink", "file-symlink", "hard-link", "fifo", "unsafe-directory"} {
			t.Run(plant, func(t *testing.T) {
				for _, planted := range []bool{false, true} {
					t.Run(map[bool]string{false: "plain", true: "planted"}[planted], func(t *testing.T) {
						w := newWS(t, bin)
						w.record["doorbell"] = map[string]any{"heartbeat": "1m", "max_wakes_per_hour": 6}
						w.write()
						outside := t.TempDir()
						directory := filepath.Join(w.root, ".claude")
						target := filepath.Join(outside, "settings.local.json")
						marker := []byte("UNCHANGED_TARGET")
						writeFixture(t, target, marker, 0600)
						if planted && (plant == "outside-symlink" || plant == "contained-symlink") {
							destination := outside
							if plant == "contained-symlink" {
								destination = filepath.Join(w.root, "contained")
								if err := os.MkdirAll(destination, 0700); err != nil {
									t.Fatal(err)
								}
								target = filepath.Join(destination, "settings.local.json")
								writeFixture(t, target, marker, 0600)
							}
							if err := os.Symlink(destination, directory); err != nil {
								t.Fatal(err)
							}
						} else {
							if err := os.MkdirAll(directory, 0700); err != nil {
								t.Fatal(err)
							}
							if planted {
								switch plant {
								case "file-symlink":
									if err := os.Symlink(target, filepath.Join(directory, "settings.local.json")); err != nil {
										t.Fatal(err)
									}
								case "hard-link":
									if err := os.Link(target, filepath.Join(directory, "settings.local.json")); err != nil {
										t.Fatal(err)
									}
								case "fifo":
									if err := syscall.Mkfifo(filepath.Join(directory, "settings.local.json"), 0600); err != nil {
										t.Fatal(err)
									}
								case "unsafe-directory":
									if err := os.Chmod(directory, 0777); err != nil {
										t.Fatal(err)
									}
								}
							}
						}
						code, _, stderr := w.invokeWS("up")
						if planted {
							if code != 1 || !strings.Contains(stderr, "doorbell_setup_failed") || !strings.Contains(stderr, "step=install") {
								t.Fatalf("settings plant admitted exit=%d stderr=%s", code, stderr)
							}
							hookAbsent(t, w.frontFixture)
							w.expectStarts(0)
							w.noOperation()
						} else {
							if code != 0 {
								t.Fatal("plain directory refused", stderr)
							}
							hookEvidence(t, w.frontFixture)
							w.expectStarts(1)
						}
						data, err := os.ReadFile(target)
						if err != nil || string(data) != string(marker) {
							t.Fatal("settings target modified", err, string(data))
						}
					})
				}
			})
		}
	})
	// Real invocations must not execute helpers found in repository paths,
	// symlink targets, relative PATH entries or either git directory. A later
	// trusted candidate remains usable and is the one recorded in diagnostics.
	t.Run("path-trust", func(t *testing.T) {
		for _, variant := range []string{"repository", "symlink", "relative", "git-dir", "work-tree", "trusted-after-repo"} {
			t.Run(variant, func(t *testing.T) {
				w := newWS(t, bin)
				w.record["doorbell"] = map[string]any{"heartbeat": "1m", "max_wakes_per_hour": 6}
				w.write()
				var original string
				for _, value := range w.env {
					if strings.HasPrefix(value, "PATH=") {
						original = strings.TrimPrefix(value, "PATH=")
					}
				}
				badDir := filepath.Join(w.root, "bad-bin")
				if variant == "relative" || variant == "git-dir" || variant == "work-tree" {
					badDir = t.TempDir()
				}
				if err := os.MkdirAll(badDir, 0700); err != nil {
					t.Fatal(err)
				}
				data, _ := os.ReadFile(bin.provider)
				writeFixture(t, filepath.Join(badDir, "a2a"), data, 0700)
				candidate := badDir
				switch variant {
				case "relative":
					candidate, _ = filepath.Rel(w.root, badDir)
				case "symlink":
					candidate = t.TempDir()
					if err := os.Symlink(filepath.Join(badDir, "a2a"), filepath.Join(candidate, "a2a")); err != nil {
						t.Fatal(err)
					}
				case "git-dir":
					w.env = append(w.env, "GIT_DIR="+badDir)
				case "work-tree":
					w.env = append(w.env, "GIT_WORK_TREE="+badDir)
				}
				path := candidate + string(os.PathListSeparator) + filepath.Join(w.root, "bin")
				if variant == "trusted-after-repo" {
					path = candidate + string(os.PathListSeparator) + original
				}
				for n, value := range w.env {
					if strings.HasPrefix(value, "PATH=") {
						w.env[n] = "PATH=" + path
					}
				}
				code, _, stderr := w.invokeWS("up")
				if variant == "trusted-after-repo" {
					if code != 0 {
						t.Fatal("trusted alternative refused", stderr)
					}
					evidence := hookEvidence(t, w.frontFixture)
					var args []string
					json.Unmarshal(evidence["argv"], &args)
					if len(args) != 8 || !strings.Contains(stderr, "a2a executable=") {
						t.Fatal("trusted evidence missing", stderr, args)
					}
					w.expectStarts(1)
				} else {
					if code != 1 || !strings.Contains(stderr, "doorbell_setup_failed") {
						t.Fatalf("unsafe helper admitted exit=%d stderr=%s", code, stderr)
					}
					hookAbsent(t, w.frontFixture)
					w.expectStarts(0)
					w.noOperation()
				}
			})
		}
	})
	// Codex asks its manager to bind; the launcher never installs Claude hooks.
	t.Run("codex", func(t *testing.T) {
		w := newWS(t, bin)
		path := filepath.Join(w.root, "fragment.json")
		raw, _ := os.ReadFile(path)
		var v map[string]any
		json.Unmarshal(raw, &v)
		v["environment"] = "codex_cli"
		v["env"] = map[string]any{"CODEX_HOME": filepath.Join(w.root, "managed")}
		raw = frontJSON(v)
		writeFixture(t, path, raw, 0600)
		frag, err := fragment.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		w.managerConfig.Profiles["sample"] = fakemanager.Profile{LockSHA256: frag.Profile.LockSHA256, FragmentDigest: frag.Digest}
		w.reset()
		code, _, stderr := w.command(bin.launcher, []string{"codex_cli", "--hosted", "--detach", "--a2a", "sample.dev", "--model", "gpt-6-astra", "--effort", "medium", "--permissions", "native"}, nil, false, false, 15*time.Second)
		w.require(t, code, 0, stderr, "", []string{"describe", "launch"}, 1, false)
		hookAbsent(t, w.frontFixture)
		doorbellLine(t, stderr, "armed", "")
	})
}
