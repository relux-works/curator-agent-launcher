package client

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func notificationStatus(state string) map[string]any {
	n := map[string]any{"contract": "curator-host-notification/1", "state": state}
	if state == "off" {
		n["reason"] = "selection-disabled"
		return n
	}
	n["attachment_ref"] = "attachment-1"
	if state == "ready" {
		n["binding"] = map[string]any{
			"execution": "execution-1", "controller_epoch": "18446744073709551615",
			"runner_instance": "runner-1", "native_thread": "thread-1",
			"member_ref": "member-1", "connection_ref": "connection-1",
			"configuration_ref": "configuration-1", "profile_lock_ref": "lock-1",
			"consumer_generation": "generation-1", "enrollment_ref": "enrollment-1",
			"workflow_reference": "work-1", "workflow_sha256": strings.Repeat("a", 64),
		}
		return n
	}
	n["workflow_reference"], n["workflow_sha256"] = "work-1", strings.Repeat("a", 64)
	if state == "unresolved" {
		n["reason"] = "observation-unavailable"
	}
	return n
}

func notificationSession(t *testing.T, n any) map[string]any {
	t.Helper()
	var row map[string]any
	if err := json.Unmarshal(encoded(session()), &row); err != nil {
		t.Fatal(err)
	}
	row["native_id"], row["notification"] = "thread-1", n
	return row
}

// The real status decoder retains each closed phase in both its typed session
// and raw record. This tests shape/retention, not enrollment or Trigger rights.
func TestNotificationStatusRetention(t *testing.T) {
	for _, state := range []string{"preparing", "ready", "unresolved", "off"} {
		t.Run(state, func(t *testing.T) {
			n := notificationStatus(state)
			r, err := DecodeResponse("status", []byte(`{"session":"SES-example"}`), encoded(notificationSession(t, n)), 0, "demo")
			if err != nil || r.Session == nil {
				t.Fatalf("valid notification status refused: %v", err)
			}
			for _, data := range [][]byte{encoded(r.Session), r.SessionRecord} {
				var got map[string]any
				if err := json.Unmarshal(data, &got); err != nil || !reflect.DeepEqual(got["notification"], n) {
					t.Fatal("notification projection lost or changed", err)
				}
			}
		})
	}
}

// Malformed notification or foreign session correlation poisons the entire
// response; no typed or raw partial result can escape the existing decoder.
func TestNotificationStatusRefusal(t *testing.T) {
	for _, kind := range []string{"null", "partial-ready", "foreign-field", "foreign-session", "foreign-native-thread", "missing-native-thread", "nonzero-exit"} {
		t.Run(kind, func(t *testing.T) {
			n := notificationStatus("ready")
			row := notificationSession(t, n)
			exit := 0
			switch kind {
			case "null":
				row["notification"] = nil
			case "partial-ready":
				delete(n["binding"].(map[string]any), "enrollment_ref")
			case "foreign-field":
				n["endpoint"] = "sentinel"
			case "foreign-session":
				row["session"] = "SES-other"
			case "foreign-native-thread":
				n["binding"].(map[string]any)["native_thread"] = "thread-other"
			case "missing-native-thread":
				row["native_id"] = nil
			case "nonzero-exit":
				exit = 1
			}
			r, err := DecodeResponse("status", []byte(`{"session":"SES-example"}`), encoded(row), exit, "demo")
			requireCode(t, err, "session_host_protocol_error")
			if r.Session != nil || r.SessionRecord != nil || r.Outcome != nil || r.Page != nil || r.Describe != nil {
				t.Fatal("partial notification response escaped")
			}
		})
	}
}

// List rows and started outcomes retain the same validated ready result as a
// direct status. Neither nested projection may hide a conflicting native thread.
func TestNotificationNestedResponses(t *testing.T) {
	for _, kind := range []string{"list", "started"} {
		for _, valid := range []bool{true, false} {
			t.Run(kind+map[bool]string{true: "-valid", false: "-foreign"}[valid], func(t *testing.T) {
				n := notificationStatus("ready")
				if !valid {
					n["binding"].(map[string]any)["native_thread"] = "thread-other"
				}
				row := notificationSession(t, n)
				delete(row, "protocol")
				delete(row, "ok")
				op, request := "list", []byte(`{"labels":{}}`)
				body := map[string]any{"protocol": Protocol, "ok": true, "sessions": []any{row}, "next_cursor": nil}
				if kind == "started" {
					op, request = "status", statusRequest()
					body = map[string]any{"protocol": Protocol, "ok": true, "outcome": "started", "session": row}
				}
				r, err := DecodeResponse(op, request, encoded(body), 0, "demo")
				if !valid {
					requireCode(t, err, "session_host_protocol_error")
					if r.Page != nil || r.Outcome != nil || r.Session != nil || r.SessionRecord != nil {
						t.Fatal("partial nested notification response escaped")
					}
					return
				}
				if err != nil {
					t.Fatal("valid nested notification refused", err)
				}
				var record *Session
				if kind == "started" {
					record = r.Outcome.Session
				} else {
					if r.Page == nil || len(r.Page.Sessions) != 1 {
						t.Fatal("missing list row")
					}
					record = &r.Page.Sessions[0]
				}
				var got map[string]any
				if record == nil || json.Unmarshal(encoded(record), &got) != nil || !reflect.DeepEqual(got["notification"], n) {
					t.Fatal("nested notification projection lost or changed")
				}
			})
		}
	}
}

// The selected notification capability survives a manager refusal round-trip;
// a neighboring version, invented capability or extra detail poisons the frame
// and retains no partial response.
func TestNotificationCapabilityRefusal(t *testing.T) {
	for _, tc := range []struct {
		name    string
		details map[string]string
		valid   bool
	}{
		{"selected", map[string]string{"capability": "curator-host-notification/1"}, true},
		{"next-version", map[string]string{"capability": "curator-host-notification/2"}, false},
		{"invented", map[string]string{"capability": "invented-capability"}, false},
		{"extra-detail", map[string]string{"capability": "curator-host-notification/1", "reason": "sentinel"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := encoded(map[string]any{"protocol": Protocol, "ok": false, "code": "session_host_capability_missing", "details": tc.details})
			response, err := DecodeResponse("launch", document(t, "launch"), body, 6, "demo")
			code := "session_host_protocol_error"
			if tc.valid {
				code = "session_host_capability_missing"
			}
			typed := requireCode(t, err, code)
			if tc.valid && typed.Details["capability"] != "curator-host-notification/1" {
				t.Fatal("notification capability detail lost")
			}
			if !tc.valid && len(typed.Details) != 0 {
				t.Fatal("invalid notification refusal retained details")
			}
			if response.Session != nil || response.Outcome != nil || response.Page != nil || response.Describe != nil {
				t.Fatal("partial notification refusal response")
			}
		})
	}
}
