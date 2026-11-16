package client

import (
	"testing"

	managerschema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
)

func liveProcess() map[string]any {
	return map[string]any{
		"pid":        4242,
		"uid":        1001,
		"boot_id":    "6f1c0e2a-8b44-4d1e-9a51-2f0c3d7e9b10",
		"start_time": map[string]any{"kind": "linux-stat-starttime", "ticks": "123456789", "hz": 100},
	}
}

// Proves the deferred-capability gate: every response shape that the pinned
// 1.3.0/1.1.0 schemas now admit with an observation-v2 member (an enveloped
// status or stop session with process, a down session with termination, a
// list row with process, a started outcome whose session has process, and a
// failed or cancelled outcome with no_start) is refused as
// session_host_protocol_error. Each schema-valid body is first checked against
// the pinned schema, so the refusal comes from the gate, not from validation.
// Beside each, the same body without the member decodes, so the gate does not
// refuse legacy 1.2.0 traffic. Limit: this covers the client decode path; the
// capability-aware admission is a later unit.
func TestObservationV2MembersRefusedUntilNegotiated(t *testing.T) {
	statusReq := []byte(`{"session":"SES-example"}`)
	stopReq := []byte(`{"session":"SES-example","expected_generation":1,"mode":"graceful","precondition":"none"}`)

	envelope := func(row map[string]any) map[string]any {
		row["protocol"], row["ok"] = Protocol, true
		return row
	}
	live := func() map[string]any {
		row := sessionRow()
		row["state"] = "idle"
		return row
	}
	down := func() map[string]any {
		row := sessionRow()
		row["state"] = "down"
		return row
	}

	type tc struct {
		name, op, schema string
		request          []byte
		with, without    map[string]any
	}
	withProcess := envelope(live())
	withProcess["process"] = liveProcess()
	withTermination := envelope(down())
	withTermination["termination"] = map[string]any{"scope": "linux-cgroup", "witness": "populated-0"}
	listRow := live()
	listRow["process"] = liveProcess()
	startedRow := live()
	startedRow["process"] = liveProcess()
	cases := []tc{
		{"status-process", "status", "session", statusReq, withProcess, envelope(live())},
		{"stop-process", "stop", "session", stopReq, withProcess, envelope(live())},
		{"status-termination", "status", "session", statusReq, withTermination, envelope(down())},
		{"list-row-process", "list", "list-page", []byte(`{}`),
			map[string]any{"protocol": Protocol, "ok": true, "sessions": []map[string]any{listRow}, "next_cursor": nil},
			map[string]any{"protocol": Protocol, "ok": true, "sessions": []map[string]any{live()}, "next_cursor": nil}},
		{"started-outcome-process", "status", "outcome", statusRequest(),
			map[string]any{"protocol": Protocol, "ok": true, "outcome": "started", "session": startedRow},
			map[string]any{"protocol": Protocol, "ok": true, "outcome": "started", "session": live()}},
		{"failed-no-start", "cancel", "outcome", statusRequest(),
			map[string]any{"protocol": Protocol, "ok": true, "outcome": "failed", "code": "policy_refused", "no_start": "proved"},
			map[string]any{"protocol": Protocol, "ok": true, "outcome": "failed", "code": "policy_refused"}},
		{"cancelled-no-start", "cancel", "outcome", statusRequest(),
			map[string]any{"protocol": Protocol, "ok": true, "outcome": "cancelled", "no_start": "proved"},
			map[string]any{"protocol": Protocol, "ok": true, "outcome": "cancelled"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			with, without := encoded(c.with), encoded(c.without)
			if err := managerschema.Validate(c.schema+".schema.json", with); err != nil {
				t.Fatalf("fixture is not schema-valid, so the gate would be untested: %v", err)
			}
			r, err := DecodeResponse(c.op, c.request, with, 0, "demo")
			requireCode(t, err, "session_host_protocol_error")
			if r.Session != nil || r.SessionRecord != nil || r.Outcome != nil || r.Page != nil {
				t.Fatal("partial response returned with a refused observation-v2 member")
			}
			if _, err := DecodeResponse(c.op, c.request, without, 0, "demo"); err != nil {
				t.Fatalf("legacy body refused: %v", err)
			}
		})
	}
}
