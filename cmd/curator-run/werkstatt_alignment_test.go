//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/fakemanager"
)

// Real CLI witnesses pair manager/override evidence, fresh/stale output and
// absent/pre-start/settled dispatch. All children use the bounded fixture's
// PID/group cleanup and no-survivor assertions; no service is started.
func TestWerkstattAlignmentBinary(t *testing.T) {
	bin := frontBuild(t)
	for _, override := range []bool{false, true} {
		t.Run(map[bool]string{false: "unreadable-manager", true: "unreadable-override"}[override], func(t *testing.T) {
			w := newWS(t, bin)
			w.seedLegacySession(t)
			w.runWS("status", 0, "", "--set-task", "sample-task", "--queue-depth", "3", "--json")
			if override {
				w.runWS("status", 0, "", "--abandon-operation", "--json")
			}
			before := *w.live()
			before.LastHeartbeatAt = "2000-01-01T00:00:00Z"
			writeFixture(t, w.path(".json"), frontJSON(before), 0600)
			w.control(fakemanager.FixtureRequest{Operation: "corrupt_index", Kind: "unreadable", Session: before.Session})
			rows := w.rows(w.runWS("status", 0, "", "--json"))
			var manager map[string]any
			if json.Unmarshal(rows[0]["manager_session"], &manager) != nil || manager["readable"] != false {
				t.Fatal("missing unreadable projection", manager)
			}
			after := *w.live()
			if after.State != "unknown" || after.StateSource != "manager" || after.LastHeartbeatAt == before.LastHeartbeatAt {
				t.Fatal("unreadable evidence did not replace source/state/heartbeat", after)
			}
			if _, e := time.Parse(time.RFC3339, after.LastHeartbeatAt); e != nil {
				t.Fatal(e)
			}
			after.State, after.StateSource, after.LastHeartbeatAt = before.State, before.StateSource, before.LastHeartbeatAt
			if !reflect.DeepEqual(after, before) {
				t.Fatal("unreadable projection changed other evidence", before, after)
			}
		})
	}
	for _, populated := range []bool{false, true} {
		for _, action := range []string{"status", "list"} {
			for _, asJSON := range []bool{false, true} {
				name := "stale/" + map[bool]string{false: "empty", true: "populated"}[populated] + "/" + action + "/" + map[bool]string{false: "default", true: "json"}[asJSON]
				t.Run(name, func(t *testing.T) {
					w := newWS(t, bin)
					if populated {
						w.seedLegacySession(t)
					}
					before, readErr := os.ReadFile(w.path(".json"))
					if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
						t.Fatal(readErr)
					}
					// Fresh output remains usable with and without the explicit JSON flag.
					fresh := w.runWS("status", 0, "")
					if !strings.Contains(fresh, "stale=false") {
						t.Fatal("fresh text output missing", fresh)
					}
					freshRows := w.rows(w.runWS("status", 0, "", "--json"))
					if string(freshRows[0]["stale"]) != "false" {
						t.Fatal(freshRows)
					}
					// Refresh may update heartbeat; snapshot just before taking the lock.
					if populated {
						before, _ = os.ReadFile(w.path(".json"))
					}
					calls := w.calls()
					tx, e := (client.Store{Directory: filepath.Dir(w.path(".json"))}).Lock(context.Background(), w.id)
					if e != nil {
						t.Fatal(e)
					}
					defer tx.Close()
					args := []string{"werkstatt", action}
					if action == "status" {
						args = append(args, w.id)
					}
					if asJSON {
						args = append(args, "--json")
					}
					code, out, stderr := w.command(bin.launcher, args, nil, false, false, 35*time.Second)
					if code != 0 {
						t.Fatal(code, out, stderr)
					}
					rows := w.rows(out)
					if len(rows) != 1 || string(rows[0]["stale"]) != "true" || string(rows[0]["manager_session"]) != "null" || string(rows[0]["a2a"]) != "null" {
						t.Fatal("stale row shape", rows)
					}
					if populated {
						var got client.Liveness
						if json.Unmarshal(rows[0]["liveness"], &got) != nil || !reflect.DeepEqual(&got, w.live()) {
							t.Fatal("stale snapshot changed", rows)
						}
					} else if string(rows[0]["liveness"]) != "null" {
						t.Fatal("stale snapshot invented liveness", rows)
					}
					after, e := os.ReadFile(w.path(".json"))
					if populated && (e != nil || !bytes.Equal(before, after)) || !populated && !errors.Is(e, os.ErrNotExist) {
						t.Fatal("stale read changed state", e)
					}
					if !reflect.DeepEqual(calls, w.calls()) {
						t.Fatal("stale read called manager")
					}
				})
			}
		}
	}
	for _, edit := range []string{"set-task", "clear-task", "queue-depth"} {
		t.Run("prestart-dispatch/"+edit, func(t *testing.T) {
			w := newWS(t, bin)
			args := []string{"--" + edit, "--json"}
			if edit == "set-task" {
				args = []string{"--set-task", "sample-task", "--json"}
			}
			if edit == "queue-depth" {
				args = []string{"--queue-depth", "2", "--json"}
			}
			w.runWS("status", 1, "werkstatt_not_up", args...)
			if _, e := os.Stat(w.path(".json")); !errors.Is(e, os.ErrNotExist) {
				t.Fatal("dispatch invented liveness", e)
			}
			w.seedLegacySession(t)
			settled := *w.live()
			prestart := settled
			prestart.Session, prestart.Generation, prestart.OperationID = "", 0, nil
			prestart.StartedAt, prestart.LastHeartbeatAt, prestart.LastActivityAt = "", "", nil
			prestart.State, prestart.StateSource = "down", "operator_override"
			prestart.Doorbell, prestart.DoorbellUncertain = "off", false
			prestart.AbandonedOperations = []client.Abandoned{{OperationID: *settled.OperationID, AbandonedAt: settled.StartedAt}}
			body := frontJSON(prestart)
			writeFixture(t, w.path(".json"), body, 0600)
			calls := w.calls()
			w.runWS("status", 1, "werkstatt_not_up", args...)
			after, e := os.ReadFile(w.path(".json"))
			if e != nil || !bytes.Equal(body, after) || !reflect.DeepEqual(calls, w.calls()) {
				t.Fatal("prestart dispatch wrote or called manager", e)
			}
			if edit == "clear-task" {
				task := "previous-task"
				settled.CurrentTask = &task
			}
			writeFixture(t, w.path(".json"), frontJSON(settled), 0600)
			w.rows(w.runWS("status", 0, "", args...))
			got := w.live()
			if edit == "set-task" && (got.CurrentTask == nil || *got.CurrentTask != "sample-task") || edit == "clear-task" && got.CurrentTask != nil || edit == "queue-depth" && got.QueueDepth != 2 {
				t.Fatal("settled dispatch not applied", got)
			}
		})
	}
}
