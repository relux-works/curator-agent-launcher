//go:build darwin || linux

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/fakemanager"
)

// Continue refuses a known changed profile before resolve, hooks, journals or
// starts; same/legacy profiles resume, while explicit new generations may change
// profile. These are real launcher calls with local Curator/manager doubles.
func TestWerkstattOneProfile(t *testing.T) {
	bin := frontBuild(t)
	for _, mode := range []string{"changed", "stale-owner", "latest-target", "same", "legacy-same", "legacy-changed", "new", "fresh", "recycle"} {
		t.Run(mode, func(t *testing.T) {
			w := newWS(t, bin)
			w.record["doorbell"] = map[string]any{"heartbeat": "1m", "max_wakes_per_hour": 6}
			w.write()
			raw, err := os.ReadFile(filepath.Join(w.root, "fragment.json"))
			if err != nil {
				t.Fatal(err)
			}
			var changed map[string]any
			if json.Unmarshal(raw, &changed) != nil {
				t.Fatal("fragment fixture")
			}
			changed["profile"].(map[string]any)["name"] = "different"
			changedRaw := frontJSON(changed)
			f, err := fragment.Parse(changedRaw)
			if err != nil {
				t.Fatal(err)
			}
			w.managerConfig.Profiles["different"] = fakemanager.Profile{LockSHA256: f.Profile.LockSHA256, FragmentDigest: f.Digest}
			w.reset()
			w.seedLegacySession(t)
			if w.live().Profile != "sample" {
				t.Fatal("new generation did not record submitted profile")
			}
			first := w.live()
			if mode == "stale-owner" {
				code, out, stderr := w.command(bin.manager, []string{"stop"}, frontJSON(map[string]any{"session": first.Session, "expected_generation": first.Generation, "mode": "force", "precondition": "none"}), false, false, 10*time.Second)
				if code != 0 {
					t.Fatalf("fixture stop %d %s %s", code, out, stderr)
				}
			} else {
				w.runWS("down", 0, "")
			}
			// Down, dispatch, status and operator rebind preserve generation evidence.
			if w.live().Profile != "sample" {
				t.Fatal("down lost profile")
			}
			if mode != "stale-owner" {
				w.runWS("update", 0, "", "--queue-depth", "2")
				w.runWS("status", 0, "", "--json")
				w.runWS("recover", 0, "", "--rebind-manager")
				if w.live().Profile != "sample" {
					t.Fatal("refresh/edit changed profile")
				}
			}
			if mode == "latest-target" {
				// Liveness has no matching handle, so the newest down candidate wins.
				live := w.live()
				live.Session = "SES-older"
				writeFixture(t, w.path(".json"), frontJSON(live), 0600)
			}
			if strings.HasPrefix(mode, "legacy-") {
				var row map[string]any
				if json.Unmarshal(readWSFile(t, w.path(".json")), &row) != nil {
					t.Fatal("liveness fixture")
				}
				delete(row, "profile")
				writeFixture(t, w.path(".json"), frontJSON(row), 0600)
			}
			changing := mode != "same" && mode != "legacy-same"
			if changing {
				w.record["profile"] = "different"
				writeFixture(t, filepath.Join(w.root, "fragment.json"), changedRaw, 0600)
			}
			if mode == "fresh" {
				w.record["context_mode"] = "fresh"
			}
			w.write()
			hooks := filepath.Join(w.root, ".claude", "settings.local.json")
			if err := os.Remove(hooks); err != nil {
				t.Fatal(err)
			}
			resolveLog := filepath.Join(w.root, "resolve-called")
			w.env = append(w.env, "FRONTDOOR_RESOLVE_LOG="+resolveLog)
			before := readWSFile(t, w.path(".json"))
			calls := len(w.calls())
			starts := w.observe().Starts
			action := "up"
			args := []string{}
			if mode == "new" {
				args = append(args, "--new")
			}
			if mode == "recycle" {
				action = "recycle"
			}
			code, out, stderr := w.invokeWS(action, args...)
			gotCalls := w.calls()[calls:]
			if mode == "changed" || mode == "stale-owner" || mode == "latest-target" {
				want := "curator-run: change profile with curator-run werkstatt recycle example or curator-run werkstatt up example --new\ncurator-run: werkstatt_profile_changed: Werkstatt profile changed. id=example\n"
				if code != 1 || out != "" || stderr != want {
					t.Errorf("profile refusal exit=%d out=%q stderr=%q", code, out, stderr)
				}
				reads := []string{"describe", "list"}
				if mode == "stale-owner" {
					reads = []string{"describe", "status", "list"}
				}
				if !slices.Equal(gotCalls, reads) {
					t.Errorf("profile refusal reached launch/resume: %v", gotCalls)
				}
				w.expectStarts(starts)
				w.noOperation()
				for _, path := range []string{hooks, resolveLog} {
					if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
						t.Errorf("profile refusal wrote %s: %v", filepath.Base(path), err)
					}
				}
				if !bytes.Equal(before, readWSFile(t, w.path(".json"))) {
					t.Error("profile refusal changed liveness")
				}
				if strings.Contains(stderr, "sample") || strings.Contains(stderr, "different") {
					t.Error("profile name disclosed")
				}
				return
			}
			if _, err := os.Stat(resolveLog); err != nil {
				t.Fatal("positive control did not resolve fragment", err)
			}
			if mode == "legacy-changed" {
				if code != 1 || !strings.Contains(stderr, "session_resume_identity_mismatch") {
					t.Fatalf("legacy guard changed: %d %s %s", code, out, stderr)
				}
				if !slices.Contains(gotCalls, "resume") {
					t.Fatal("legacy record bypassed manager guard")
				}
				if w.live().Profile != "" {
					t.Fatal("legacy profile inferred on refusal")
				}
				return
			}
			if code != 1 || !strings.Contains(stderr, "process_incarnation_unavailable") {
				t.Fatalf("valid transition exit=%d out=%s stderr=%s", code, out, stderr)
			}
			newGeneration := mode == "new" || mode == "fresh" || mode == "recycle"
			operation := "resume"
			wantProfile := "sample"
			if newGeneration {
				operation = "launch"
				wantProfile = "different"
			}
			if gotCalls[len(gotCalls)-1] != operation || w.live().Profile != wantProfile || w.live().Generation != first.Generation+1 {
				t.Fatalf("transition calls=%v live=%+v", gotCalls, w.live())
			}
			if (w.live().Session != first.Session) != newGeneration {
				t.Fatal("conversation identity changed incorrectly")
			}
			w.expectStarts(starts + 1)
			w.protectedOperation()
			if _, err := os.Stat(hooks); err != nil {
				t.Fatal("positive control did not install hooks", err)
			}
		})
	}
}

func readWSFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Malformed present profile evidence takes the existing read failure path,
// preserves its bytes and never silently falls through to manager or prepare.
func TestWerkstattMalformedLivenessProfile(t *testing.T) {
	bin := frontBuild(t)
	w := newWS(t, bin)
	w.seedLegacySession(t)
	w.runWS("down", 0, "")
	good := readWSFile(t, w.path(".json"))
	for _, tc := range []struct {
		name  string
		value any
	}{{"null", nil}, {"empty", ""}, {"path", "../profile"}, {"reserved", "CON.txt"}, {"overflow", strings.Repeat("p", 129)}, {"number", 7}, {"boolean", true}, {"object", map[string]any{}}, {"array", []string{}}} {
		t.Run(tc.name, func(t *testing.T) {
			var row map[string]any
			if json.Unmarshal(good, &row) != nil {
				t.Fatal("fixture")
			}
			row["profile"] = tc.value
			bad := frontJSON(row)
			writeFixture(t, w.path(".json"), bad, 0600)
			calls := len(w.calls())
			code, out, stderr := w.invokeWS("up")
			if code != 2 || out != "" || stderr != "curator-run: werkstatt_record_invalid: Werkstatt record invalid. id=example\n" {
				t.Errorf("malformed profile silently bypassed: %d %q %q", code, out, stderr)
			}
			if len(w.calls()) != calls {
				t.Error("malformed liveness reached manager")
			}
			if !bytes.Equal(bad, readWSFile(t, w.path(".json"))) {
				t.Error("malformed liveness overwritten")
			}
			w.noOperation()
		})
	}
	writeFixture(t, w.path(".json"), good, 0600)
	w.seedLegacySession(t)
}
