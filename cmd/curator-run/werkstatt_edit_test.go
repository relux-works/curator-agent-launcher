//go:build darwin || linux

package main

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Invalid update/recover grammar exits 2 before creating a lock or liveness,
// changing configuration, or contacting the manager; valid neighbours run below.
func TestWerkstattEditGrammarNoEffects(t *testing.T) {
	bin := frontBuild(t)
	for _, tc := range []struct {
		action string
		flags  []string
		want   string
	}{
		{"update", nil, "at least one"},
		{"recover", nil, "exactly one"},
		{"recover", []string{"--rebind-manager", "--abandon-operation"}, "mutually exclusive"},
		{"update", []string{"--rebind-manager"}, "valid only with recover"},
		{"update", []string{"--abandon-operation"}, "valid only with recover"},
		{"recover", []string{"--set-task", "next"}, "valid only with update"},
		{"recover", []string{"--clear-task"}, "valid only with update"},
		{"recover", []string{"--queue-depth", "1"}, "valid only with update"},
		{"update", []string{"--set-task", "next", "--clear-task"}, "mutually exclusive"},
		{"update", []string{"--queue-depth", "-1"}, "needs an integer"},
		{"update", []string{"--queue-depth"}, "needs a value"},
		{"recover", []string{"--attach"}, "valid only with up"},
	} {
		t.Run(tc.action+"/"+strings.Join(tc.flags, " "), func(t *testing.T) {
			w := newWS(t, bin)
			before := snapshotTree(t, w.config)
			calls := w.calls()
			code, out, stderr := w.invokeWS(tc.action, tc.flags...)
			if code != 2 || out != "" || !strings.Contains(stderr, tc.want) || strings.Contains(stderr, "deprecated:") {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, out, stderr)
			}
			if !reflect.DeepEqual(before, snapshotTree(t, w.config)) || !reflect.DeepEqual(calls, w.calls()) {
				t.Fatal("usage error changed config or called manager")
			}
			for _, suffix := range []string{".lock", ".json", ".op.json"} {
				if _, err := os.Lstat(w.path(suffix)); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("usage error created state", suffix, err)
				}
			}
		})
	}
}

// The actual binary reaches both dedicated handlers; legacy stdout matches the
// new form for dispatch (ignoring clock-bearing JSON covered by fixed-clock tests).
func TestWerkstattEditBinary(t *testing.T) {
	bin := frontBuild(t)
	for _, tc := range []struct {
		action string
		flags  []string
	}{
		{"update", []string{"--set-task", "next", "--queue-depth", "2"}},
		{"update", []string{"--clear-task"}},
		{"recover", []string{"--rebind-manager"}},
		{"recover", []string{"--abandon-operation"}},
	} {
		t.Run(tc.action+"/"+strings.Join(tc.flags, " "), func(t *testing.T) {
			w := newWS(t, bin)
			w.runWS("up", 0, "")
			before, err := os.ReadFile(w.path(".json"))
			if err != nil {
				t.Fatal(err)
			}
			code, out, stderr := w.invokeWS(tc.action, tc.flags...)
			if code != 0 || strings.Contains(stderr, "deprecated:") {
				t.Fatal(code, out, stderr)
			}
			if err := os.WriteFile(w.path(".json"), before, 0600); err != nil {
				t.Fatal(err)
			}
			aliasCode, aliasOut, aliasErr := w.invokeWS("status", tc.flags...)
			want := "curator-run: deprecated: use 'werkstatt " + tc.action + " <id> …'\n"
			if aliasCode != code || !bytes.Equal([]byte(out), []byte(aliasOut)) || aliasErr != want+stderr {
				t.Fatal("binary alias mismatch", aliasCode, aliasOut, aliasErr)
			}
			w.noOperation()
		})
	}
}
