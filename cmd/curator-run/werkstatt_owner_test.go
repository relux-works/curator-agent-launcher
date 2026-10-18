//go:build darwin || linux

package main

import (
	"encoding/json"
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
			w.runWS("up", 0, "")
			prior := *w.live()
			w.runWS("down", 0, "")
			w.runWS("up", 0, "")
			current := *w.live()
			if current.Generation != prior.Generation+1 {
				t.Fatal("resume did not advance generation")
			}
			w.activity(state)
			w.activity("uncertain")
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
