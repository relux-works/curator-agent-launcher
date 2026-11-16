package client

import (
	"strings"
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

func envelopedRow(state string) map[string]any {
	row := sessionRow()
	row["state"] = state
	row["protocol"], row["ok"] = Protocol, true
	return row
}

// Claim: the token is process-observation/1 together with status/1. The token
// alone, status/1 alone, an empty list and near spellings confer nothing;
// beside them the exact pair is recognised in either order.
func TestProcessObservationToken(t *testing.T) {
	for _, capabilities := range [][]string{
		{"status/1", "process-observation/1"},
		{"process-observation/1", "launch-delegated/1", "status/1"},
	} {
		if !ProcessObservation(Describe{Capabilities: capabilities}) {
			t.Fatalf("valid control refused: %v", capabilities)
		}
	}
	for _, capabilities := range [][]string{
		nil,
		{"status/1"},
		{"process-observation/1"},
		{"status/1", "process-observation/2"},
		{"status/1", "process-observation"},
		{"status/1", "Process-Observation/1"},
		{"status/1", "process-observation/1 "},
		{"status/2", "process-observation/1"},
	} {
		if ProcessObservation(Describe{Capabilities: capabilities}) {
			t.Fatalf("token admitted from %v", capabilities)
		}
	}
}

// Claim: without the token, every response shape that the pinned 1.3.0/1.1.0
// schemas admit with a negotiated member (launch, resume, status, stop, list
// rows, a started outcome's session, and failed or cancelled outcomes with
// no_start on cancel and on operation status)
// is refused as session_host_protocol_error with no partial result. Each body
// is first checked against the pinned schema, so the refusal comes from the
// capability rule and not from validation. Beside each: the same body is
// admitted with the token, and the same body without the member decodes with
// and without the token.
func TestProcessObservationMembersNeedToken(t *testing.T) {
	statusReq := []byte(`{"session":"SES-example"}`)
	stopReq := []byte(`{"session":"SES-example","expected_generation":1,"mode":"graceful","precondition":"none"}`)
	launchReq := document(t, "launch")
	resumeReq := document(t, "resume")

	withProcess := envelopedRow("idle")
	withProcess["process"] = liveProcess()
	withTermination := envelopedRow("down")
	withTermination["termination"] = map[string]any{"scope": "linux-cgroup", "witness": "populated-0"}
	plainRow := func() map[string]any {
		row := sessionRow()
		row["state"] = "idle"
		return row
	}
	processRow := plainRow()
	processRow["process"] = liveProcess()
	downRow := func() map[string]any {
		row := sessionRow()
		row["state"] = "down"
		return row
	}
	terminationRow := downRow()
	terminationRow["termination"] = map[string]any{"scope": "linux-cgroup", "witness": "populated-0"}

	cases := []struct {
		name, op, schema string
		request          []byte
		with, without    map[string]any
	}{
		{"launch-process", "launch", "session", launchReq, withProcess, envelopedRow("idle")},
		{"resume-process", "resume", "session", resumeReq, withProcess, envelopedRow("idle")},
		{"status-process", "status", "session", statusReq, withProcess, envelopedRow("idle")},
		{"stop-process", "stop", "session", stopReq, withProcess, envelopedRow("idle")},
		{"stop-termination", "stop", "session", stopReq, withTermination, envelopedRow("down")},
		{"status-termination", "status", "session", statusReq, withTermination, envelopedRow("down")},
		{"list-row-process", "list", "list-page", []byte(`{}`),
			map[string]any{"protocol": Protocol, "ok": true, "sessions": []map[string]any{processRow}, "next_cursor": nil},
			map[string]any{"protocol": Protocol, "ok": true, "sessions": []map[string]any{plainRow()}, "next_cursor": nil}},
		{"list-row-termination", "list", "list-page", []byte(`{}`),
			map[string]any{"protocol": Protocol, "ok": true, "sessions": []map[string]any{terminationRow}, "next_cursor": nil},
			map[string]any{"protocol": Protocol, "ok": true, "sessions": []map[string]any{downRow()}, "next_cursor": nil}},
		{"started-outcome-process", "status", "outcome", statusRequest(),
			map[string]any{"protocol": Protocol, "ok": true, "outcome": "started", "session": processRow},
			map[string]any{"protocol": Protocol, "ok": true, "outcome": "started", "session": plainRow()}},
		{"cancel-failed-no-start", "cancel", "outcome", statusRequest(),
			map[string]any{"protocol": Protocol, "ok": true, "outcome": "failed", "code": "policy_refused", "no_start": "proved"},
			map[string]any{"protocol": Protocol, "ok": true, "outcome": "failed", "code": "policy_refused"}},
		{"cancel-cancelled-no-start", "cancel", "outcome", statusRequest(),
			map[string]any{"protocol": Protocol, "ok": true, "outcome": "cancelled", "no_start": "proved"},
			map[string]any{"protocol": Protocol, "ok": true, "outcome": "cancelled"}},
		{"opstatus-failed-no-start", "status", "outcome", statusRequest(),
			map[string]any{"protocol": Protocol, "ok": true, "outcome": "failed", "code": "policy_refused", "no_start": "proved"},
			map[string]any{"protocol": Protocol, "ok": true, "outcome": "failed", "code": "policy_refused"}},
		{"opstatus-cancelled-no-start", "status", "outcome", statusRequest(),
			map[string]any{"protocol": Protocol, "ok": true, "outcome": "cancelled", "no_start": "proved"},
			map[string]any{"protocol": Protocol, "ok": true, "outcome": "cancelled"}},
	}
	token := Admission{ProcessObservation: true}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			with, without := encoded(c.with), encoded(c.without)
			if err := managerschema.Validate(c.schema+".schema.json", with); err != nil {
				t.Fatalf("fixture is not schema-valid, so the rule would be untested: %v", err)
			}
			r, err := DecodeResponse(c.op, c.request, with, 0, "demo")
			requireCode(t, err, "session_host_protocol_error")
			if r.Session != nil || r.SessionRecord != nil || r.Outcome != nil || r.Page != nil {
				t.Fatal("partial response returned with a refused member")
			}
			r, err = DecodeAdmitted(c.op, c.request, with, 0, "demo", Admission{})
			requireCode(t, err, "session_host_protocol_error")
			if r.Session != nil || r.SessionRecord != nil || r.Outcome != nil || r.Page != nil {
				t.Fatal("partial response returned with a refused member")
			}
			if _, err := DecodeAdmitted(c.op, c.request, with, 0, "demo", token); err != nil {
				t.Fatalf("member refused from a manager with the token: %v", err)
			}
			if _, err := DecodeResponse(c.op, c.request, without, 0, "demo"); err != nil {
				t.Fatalf("legacy body refused without the token: %v", err)
			}
			if _, err := DecodeAdmitted(c.op, c.request, without, 0, "demo", token); err != nil {
				t.Fatalf("legacy body refused with the token: %v", err)
			}
		})
	}
}

// Claim: the token admits the members only as the pinned schemas define them.
// A malformed process, an unknown process member, process on a down record,
// termination on a live record and no_start on started or any other
// non-failed/cancelled outcome stay
// session_host_protocol_error with the token; beside them the well-formed
// record is admitted and its status record keeps process unchanged.
func TestProcessObservationTokenDoesNotWidenSchemas(t *testing.T) {
	token := Admission{ProcessObservation: true}
	statusReq := []byte(`{"session":"SES-example"}`)
	good := envelopedRow("idle")
	good["process"] = liveProcess()
	r, err := DecodeAdmitted("status", statusReq, encoded(good), 0, "demo", token)
	if err != nil || !strings.Contains(string(r.SessionRecord), `"process":`+string(encoded(liveProcess()))) {
		t.Fatalf("valid control: %s %v", r.SessionRecord, err)
	}
	if err := managerschema.Validate("session-record.schema.json", r.SessionRecord); err != nil {
		t.Fatal(err)
	}

	pidZero := envelopedRow("idle")
	process := liveProcess()
	process["pid"] = 0
	pidZero["process"] = process
	extra := envelopedRow("idle")
	process = liveProcess()
	process["started_at"] = stamp
	extra["process"] = process
	onDown := envelopedRow("down")
	onDown["process"] = liveProcess()
	liveTermination := envelopedRow("idle")
	liveTermination["termination"] = map[string]any{"scope": "linux-cgroup", "witness": "populated-0"}
	for name, body := range map[string][]byte{
		"pid-zero":            encoded(pidZero),
		"extra-member":        encoded(extra),
		"process-on-down":     encoded(onDown),
		"termination-on-live": encoded(liveTermination),
	} {
		r, err := DecodeAdmitted("status", statusReq, body, 0, "demo", token)
		requireCode(t, err, "session_host_protocol_error")
		if r.Session != nil || r.SessionRecord != nil {
			t.Fatalf("%s: partial response", name)
		}
	}
	started := sessionRow()
	started["state"] = "idle"
	_, err = DecodeAdmitted("status", statusRequest(), encoded(map[string]any{"protocol": Protocol, "ok": true, "outcome": "started", "session": started, "no_start": "proved"}), 0, "demo", token)
	requireCode(t, err, "session_host_protocol_error")
	for _, other := range []string{"prepared", "admitted", "not_found", "unknown"} {
		_, err = DecodeAdmitted("status", statusRequest(), encoded(map[string]any{"protocol": Protocol, "ok": true, "outcome": other, "no_start": "proved"}), 0, "demo", token)
		requireCode(t, err, "session_host_protocol_error")
	}
}

// Claim: with a frozen generation, a direct status record must name this
// manager, the requested handle and exactly that generation. A foreign
// manager, another handle, another generation and an unreadable record (no
// generation evidence) refuse session_generation_mismatch with no partial
// result; beside them the exact match decodes. Without a frozen generation
// the same foreign records keep their existing protocol-error refusal.
func TestFrozenGenerationOwnerCheck(t *testing.T) {
	statusReq := []byte(`{"session":"SES-example"}`)
	frozen := Admission{FrozenGeneration: 1}
	good := envelopedRow("idle")
	if r, err := DecodeAdmitted("status", statusReq, encoded(good), 0, "demo", frozen); err != nil || r.SessionRecord == nil {
		t.Fatalf("valid control: %v", err)
	}
	foreign := envelopedRow("idle")
	foreign["manager"] = "other"
	handle := envelopedRow("idle")
	handle["session"] = "SES-other"
	generation := envelopedRow("idle")
	generation["generation"] = 2
	unreadable := unreadableRow()
	unreadable["protocol"], unreadable["ok"] = Protocol, true
	for name, row := range map[string]map[string]any{"foreign-manager": foreign, "other-handle": handle, "other-generation": generation, "unreadable": unreadable} {
		body := encoded(row)
		if err := managerschema.Validate("session.schema.json", body); err != nil {
			t.Fatalf("%s: fixture is not schema-valid: %v", name, err)
		}
		r, err := DecodeAdmitted("status", statusReq, body, 0, "demo", frozen)
		requireCode(t, err, "session_generation_mismatch")
		if r.Session != nil || r.SessionRecord != nil {
			t.Fatalf("%s: partial response", name)
		}
	}
	for _, row := range []map[string]any{foreign, handle} {
		_, err := DecodeResponse("status", statusReq, encoded(row), 0, "demo")
		requireCode(t, err, "session_host_protocol_error")
	}
	if _, err := DecodeResponse("status", statusReq, encoded(generation), 0, "demo"); err != nil {
		t.Fatalf("unfrozen status gained a generation check: %v", err)
	}
	// A schema-invalid record is a protocol error first, never a mismatch.
	_, err := DecodeAdmitted("status", statusReq, []byte(strings.Replace(string(encoded(foreign)), `"ok":true`, `"ok":true,"text":"x"`, 1)), 0, "demo", frozen)
	requireCode(t, err, "session_host_protocol_error")
}

// Claim: the v1 projection removes exactly process and termination from a
// valid record, yielding a valid 1.2.0 record equal to the same record sent
// without them; a record carrying neither is returned byte for byte, and a
// non-object is refused rather than passed through.
func TestSessionRecordV1Projection(t *testing.T) {
	token := Admission{ProcessObservation: true}
	statusReq := []byte(`{"session":"SES-example"}`)
	for state, member := range map[string]map[string]any{
		"idle": {"process": liveProcess()},
		"down": {"termination": map[string]any{"scope": "dedicated-uid", "witness": "zero-process-proof"}},
	} {
		plain, err := DecodeResponse("status", statusReq, encoded(envelopedRow(state)), 0, "demo")
		if err != nil {
			t.Fatal(err)
		}
		row := envelopedRow(state)
		for k, v := range member {
			row[k] = v
		}
		full, err := DecodeAdmitted("status", statusReq, encoded(row), 0, "demo", token)
		if err != nil {
			t.Fatal(err)
		}
		projected, err := SessionRecordV1(full.SessionRecord)
		if err != nil || string(projected) != string(plain.SessionRecord) {
			t.Fatalf("%s projection: %s want %s: %v", state, projected, plain.SessionRecord, err)
		}
		if string(full.SessionRecord) == string(projected) {
			t.Fatalf("%s: the complete record lost its member before projection", state)
		}
		same, err := SessionRecordV1(plain.SessionRecord)
		if err != nil || string(same) != string(plain.SessionRecord) {
			t.Fatalf("%s: a 1.2.0 record changed: %s", state, same)
		}
	}
	if _, err := SessionRecordV1([]byte(`[]`)); err == nil {
		t.Fatal("non-object projected")
	}
}

// Claim: the projection keeps the position and the bytes of every member it
// does not remove, so a record whose members are not in sorted order is not
// re-encoded. Limit: the input here is a literal, not a validated response.
func TestSessionRecordV1ProjectionKeepsMemberOrder(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"process-in-the-middle", `{"state":"idle","process":{"uid":501,"pid":7},"manager":"demo","generation":3}`, `{"state":"idle","manager":"demo","generation":3}`},
		{"termination-first", `{"termination":{"witness":"w","scope":"s"},"state":"down","manager":"demo"}`, `{"state":"down","manager":"demo"}`},
		{"both-last", `{"state":"idle","manager":"demo","process":{},"termination":{}}`, `{"state":"idle","manager":"demo"}`},
		{"nested-name-is-kept", `{"state":"idle","detail":{"process":1,"b":2,"a":3}}`, `{"state":"idle","detail":{"process":1,"b":2,"a":3}}`},
	} {
		got, err := SessionRecordV1([]byte(c.in))
		if err != nil || string(got) != c.want {
			t.Fatalf("%s: %s want %s: %v", c.name, got, c.want, err)
		}
	}
	for _, bad := range []string{`{"state":"idle"} x`, `{"state":"idle"`, `"process"`} {
		if _, err := SessionRecordV1([]byte(bad)); err == nil {
			t.Fatalf("%q projected", bad)
		}
	}
}
