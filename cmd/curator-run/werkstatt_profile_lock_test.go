//go:build darwin || linux

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// Profile-lock admission preserves matching/absent launches and refuses unsafe locks without start effects.
// Curator and the manager are local doubles, not real provider integrations.
func TestWerkstattProfileLock(t *testing.T) {
	bin := frontBuild(t)
	for _, mode := range []string{"new", "resume", "stale-owner"} {
		for _, lock := range []string{"absent", "matching", "mismatch"} {
			t.Run(mode+"/"+lock, func(t *testing.T) {
				w := newWS(t, bin)
				w.record["doorbell"] = map[string]any{"heartbeat": "1m", "max_wakes_per_hour": 6}
				w.write()
				if mode != "new" {
					w.runWS("up", 0, "")
					if mode == "resume" {
						w.runWS("down", 0, "")
					} else {
						// The manager observes down while local liveness is still idle.
						live := w.live()
						code, out, stderr := w.command(bin.manager, []string{"stop"}, frontJSON(map[string]any{"session": live.Session, "expected_generation": live.Generation, "mode": "force", "precondition": "none"}), false, false, 10*time.Second)
						if code != 0 {
							t.Fatalf("fixture stop exit=%d stdout=%s stderr=%s", code, out, stderr)
						}
					}
				}
				if mode != "new" {
					if err := os.Remove(filepath.Join(w.root, ".claude", "settings.local.json")); err != nil {
						t.Fatal(err)
					}
				}
				before, readErr := os.ReadFile(w.path(".json"))
				if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
					t.Fatal(readErr)
				}
				calls := len(w.calls())
				starts := w.observe().Starts
				resolved, required := strings.Repeat("1", 64), strings.Repeat("1", 64)
				if lock == "mismatch" {
					required = strings.Repeat("2", 64)
				}
				if lock != "absent" {
					w.record["profile_lock_sha256"] = required
				}
				w.record["doorbell"] = map[string]any{"heartbeat": "1m", "max_wakes_per_hour": 6}
				w.write()
				code, out, stderr := w.invokeWS("up")
				gotCalls := w.calls()[calls:]
				reads := []string{"describe", "list"}
				if mode == "stale-owner" {
					reads = []string{"describe", "status", "list"}
				}
				hooks := filepath.Join(w.root, ".claude", "settings.local.json")
				if lock != "mismatch" {
					operation := "launch"
					if mode != "new" {
						operation = "resume"
					}
					if code != 0 || !slices.Equal(gotCalls, append(reads, operation)) {
						t.Fatalf("valid lock exit=%d calls=%v stdout=%s stderr=%s", code, gotCalls, out, stderr)
					}
					w.expectStarts(starts + 1)
					w.noOperation()
					if _, err := os.Stat(hooks); err != nil {
						t.Fatal("matching/absent lock did not install hooks", err)
					}
					return
				}
				if code != 1 || !strings.HasSuffix(stderr, "werkstatt_profile_lock_mismatch: Werkstatt profile lock mismatch. id=example\n") || out != "" {
					t.Errorf("mismatch diagnostic exit=%d stdout=%s stderr=%s", code, out, stderr)
				}
				if !slices.Equal(gotCalls, reads) {
					t.Errorf("mismatch reached start call: %v", gotCalls)
				}
				w.expectStarts(starts)
				w.noOperation()
				if _, err := os.Stat(hooks); !errors.Is(err, os.ErrNotExist) {
					t.Error("mismatch wrote transitional hooks", err)
				}
				after, err := os.ReadFile(w.path(".json"))
				if !bytes.Equal(before, after) || errors.Is(readErr, os.ErrNotExist) != errors.Is(err, os.ErrNotExist) || err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Error("mismatch changed liveness")
				}
				if strings.Contains(stderr, resolved) || strings.Contains(stderr, required) {
					t.Error("mismatch disclosed a hash")
				}
			})
		}
	}
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"null", nil}, {"empty", ""}, {"uppercase", strings.Repeat("A", 64)},
		{"prefixed", "sha256:" + strings.Repeat("1", 64)}, {"short", strings.Repeat("1", 63)},
		{"long", strings.Repeat("1", 65)}, {"number", 123}, {"boolean", true},
	} {
		t.Run("malformed/"+tc.name, func(t *testing.T) {
			w := newWS(t, bin)
			w.record["doorbell"] = map[string]any{"heartbeat": "1m", "max_wakes_per_hour": 6}
			w.record["profile_lock_sha256"] = tc.value
			w.write()
			code, out, stderr := w.invokeWS("up")
			if code != 2 || stderr != "curator-run: werkstatt_record_invalid: Werkstatt record invalid. field=profile_lock_sha256 id=example\n" || out != "" {
				t.Errorf("malformed lock diagnostic exit=%d stdout=%s stderr=%s", code, out, stderr)
			}
			if calls := w.calls(); len(calls) != 0 {
				t.Errorf("malformed lock reached manager: %v", calls)
			}
			w.expectStarts(0)
			w.noOperation()
			if _, err := os.Stat(filepath.Join(w.root, ".claude", "settings.local.json")); !errors.Is(err, os.ErrNotExist) {
				t.Error("malformed lock wrote transitional hooks", err)
			}
			if _, err := os.Stat(w.path(".json")); !errors.Is(err, os.ErrNotExist) {
				t.Error("malformed lock wrote liveness", err)
			}
		})
	}
}
