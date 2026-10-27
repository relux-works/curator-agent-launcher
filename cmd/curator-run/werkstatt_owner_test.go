//go:build darwin || linux

package main

import (
	"bytes"
	"encoding/json"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/fakemanager"
	"os"
	"reflect"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
)

// A readable unknown row refreshes the same manager evidence as idle, including
// a newer settlement. Lifecycle commands still refuse unknown without effects.
// Children use the bounded fixture's PID/group cleanup and survivor assertions.
func TestWerkstattReadableOwnerBinary(t *testing.T) {
	bin := frontBuild(t)
	for _, state := range []string{"idle", "unknown"} {
		t.Run(state, func(t *testing.T) {
			w := newWS(t, bin)
			w.seedLegacySession(t)
			prior := *w.live()
			w.runWS("down", 0, "")
			w.seedLegacySession(t)
			current := *w.live()
			if current.Generation != prior.Generation+1 {
				t.Fatal("resume did not advance generation")
			}
			w.activity(state)
			w.activity("uncertain")
			// Distinct old timestamps make omission of either copied field visible.
			prior.StartedAt = "2000-01-01T00:00:00Z"
			oldActivity := "2000-01-01T00:00:01Z"
			prior.LastActivityAt = &oldActivity
			prior.LastHeartbeatAt = "2000-01-01T00:00:00Z"
			writeFixture(t, w.path(".json"), frontJSON(prior), 0600)
			rows := w.rows(w.runWS("status", 0, "", "--json"))
			var manager client.Session
			if err := json.Unmarshal(rows[0]["manager_session"], &manager); err != nil {
				t.Fatal(err)
			}
			got := *w.live()
			if !manager.Readable || manager.State != state || !manager.Doorbell.Uncertain || manager.Generation != current.Generation || manager.OperationID != *current.OperationID {
				t.Fatal("fixture lacks authoritative new evidence", manager)
			}
			if got.State != manager.State || got.Session != manager.Session || got.Generation != manager.Generation || got.OperationID == nil || *got.OperationID != manager.OperationID || got.StartedAt != manager.StartedAt || !reflect.DeepEqual(got.LastActivityAt, manager.LastActivityAt) || got.Doorbell != manager.Doorbell.State || got.DoorbellUncertain != manager.Doorbell.Uncertain || got.StateSource != "manager" || got.LastHeartbeatAt == prior.LastHeartbeatAt {
				t.Fatal("readable owner evidence not refreshed", got, manager)
			}
			preserved := got
			preserved.State, preserved.Session, preserved.Generation, preserved.OperationID = prior.State, prior.Session, prior.Generation, prior.OperationID
			preserved.StartedAt, preserved.LastActivityAt = prior.StartedAt, prior.LastActivityAt
			preserved.Doorbell, preserved.DoorbellUncertain = prior.Doorbell, prior.DoorbellUncertain
			preserved.StateSource, preserved.LastHeartbeatAt = prior.StateSource, prior.LastHeartbeatAt
			if !reflect.DeepEqual(preserved, prior) {
				t.Fatal("refresh changed local-only fields", preserved, prior)
			}
			if state == "unknown" {
				for _, action := range []string{"up", "down", "recycle"} {
					before, calls := *w.live(), w.calls()
					w.runWS(action, 1, "session_liveness_unknown")
					if !reflect.DeepEqual(before, *w.live()) {
						t.Fatal("unknown refusal changed liveness")
					}
					for _, call := range w.calls()[len(calls):] {
						if call == "stop" || call == "launch" || call == "resume" {
							t.Fatal("unknown refusal performed lifecycle action", call)
						}
					}
					w.noOperation()
				}
				w.expectStarts(2)
			}
		})
	}
}

// Poison and typed owner refusals retain their code on every lifecycle surface
// and leave liveness byte-for-byte unchanged. An unreachable owner still gives
// unknown evidence to status and refuses down/recycle; idle above is the control.
func TestWerkstattOwnerRefusalBinary(t *testing.T) {
	bin := frontBuild(t)
	for _, kind := range []string{"protocol", "identity", "unreachable"} {
		actions := []string{"status", "up", "down", "recycle"}
		if kind == "unreachable" {
			actions = []string{"status", "down", "recycle"}
		}
		for _, action := range actions {
			t.Run(kind+"/"+action, func(t *testing.T) {
				w := newWS(t, bin)
				w.seedLegacySession(t)
				before, err := os.ReadFile(w.path(".json"))
				if err != nil {
					t.Fatal(err)
				}
				calls := w.calls()
				code, diagnostic := 1, "session_curator_identity_stale"
				switch kind {
				case "protocol":
					w.control(fakemanager.FixtureRequest{Operation: "inject_fault", Name: "malformed"})
					code, diagnostic = 6, "session_host_protocol_error"
				case "identity":
					w.control(fakemanager.FixtureRequest{Operation: "set_identity_drift", Enabled: true})
				case "unreachable":
					if err := os.Chmod(bin.manager, 0600); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() {
						if err := os.Chmod(bin.manager, 0700); err != nil {
							t.Error(err)
						}
					})
					diagnostic = "session_liveness_unknown"
					if action == "status" {
						code, diagnostic = 0, ""
					}
				}
				w.runWS(action, code, diagnostic)
				after, err := os.ReadFile(w.path(".json"))
				if err != nil {
					t.Fatal(err)
				}
				if kind == "unreachable" && action == "status" {
					got := *w.live()
					var prior client.Liveness
					if err := json.Unmarshal(before, &prior); err != nil {
						t.Fatal(err)
					}
					if got.State != "unknown" {
						t.Fatal("unreachable owner did not yield unknown")
					}
					got.State, got.LastHeartbeatAt = prior.State, prior.LastHeartbeatAt
					if !reflect.DeepEqual(got, prior) {
						t.Fatal("unreachable owner destroyed evidence")
					}
				} else if !bytes.Equal(before, after) {
					t.Fatal("owner refusal wrote liveness")
				}
				for _, call := range w.calls()[len(calls):] {
					if call == "stop" || call == "launch" || call == "resume" {
						t.Fatal("owner refusal performed lifecycle action", call)
					}
				}
				w.expectStarts(1)
				w.noOperation()
			})
		}
	}
}
