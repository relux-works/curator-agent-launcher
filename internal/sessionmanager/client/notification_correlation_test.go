package client

import (
	"encoding/json"
	"strings"
	"testing"
)

// The decoder compares the selected workflow with every launch/resume phase;
// independently valid foreign values, omission and off cannot erase intent.
// This isolates response correlation using the accepted request shape;
// it does not qualify a manager's independently retained provider admission.
func TestNotificationRequestResponseCorrelation(t *testing.T) {
	for _, operation := range []string{"launch", "resume"} {
		for _, state := range []string{"preparing", "ready", "unresolved"} {
			for _, kind := range []string{"valid", "other-valid-workflow", "foreign-reference", "foreign-digest", "missing-result", "off"} {
				t.Run(operation+"/"+state+"/"+kind, func(t *testing.T) {
					request := notificationCorrelationRequest(t, operation)
					n := notificationStatus(state)
					workflow := n
					if state == "ready" {
						workflow = n["binding"].(map[string]any)
					}
					row := notificationSession(t, n)
					switch kind {
					case "other-valid-workflow":
						request["notification"].(map[string]any)["workflow"] = map[string]any{
							"reference": "independent-work-2", "sha256": strings.Repeat("c", 64),
						}
						workflow["workflow_reference"] = "independent-work-2"
						workflow["workflow_sha256"] = strings.Repeat("c", 64)
					case "foreign-reference":
						workflow["workflow_reference"] = "other-valid-workflow"
					case "foreign-digest":
						workflow["workflow_sha256"] = strings.Repeat("b", 64)
					case "missing-result":
						delete(row, "notification")
					case "off":
						row["notification"] = notificationStatus("off")
					}
					// Keep ordinary operation/runtime correlation independently valid.
					row["operation_id"], row["runtime"] = request["operation_id"], request["runtime"]
					result, err := DecodeResponse(operation, encoded(request), encoded(row), 0, "demo")
					if kind == "valid" || kind == "other-valid-workflow" {
						if err != nil || result.Session == nil || result.Session.Notification == nil {
							t.Fatalf("matching workflow refused: %v", err)
						}
						return
					}
					requireCode(t, err, "session_host_protocol_error")
					if result.Session != nil || result.SessionRecord != nil || result.Outcome != nil || result.Page != nil || result.Describe != nil {
						t.Fatal("uncorrelated notification result escaped")
					}
				})
			}
		}
	}
}

// Legacy requests retain both omitted and off results. Status uses the stored
// session lifecycle, so it does not infer launch intent from a request field.
func TestNotificationCorrelationPreservesLegacyAndStatus(t *testing.T) {
	for _, operation := range []string{"launch", "resume", "status"} {
		for _, state := range []string{"omitted", "off", "ready"} {
			t.Run(operation+"/"+state, func(t *testing.T) {
				request := notificationCorrelationRequest(t, "launch")
				if operation != "status" {
					delete(request, "notification")
				}
				row := notificationSession(t, notificationStatus("off"))
				if state == "omitted" {
					delete(row, "notification")
				} else if state == "ready" {
					row["notification"] = notificationStatus("ready")
				}
				row["operation_id"], row["runtime"] = request["operation_id"], request["runtime"]
				if operation == "status" {
					delete(request, "operation_id")
				}
				result, err := DecodeResponse(operation, encoded(request), encoded(row), 0, "demo")
				if err != nil || result.Session == nil {
					t.Fatalf("legacy/status response refused: %v", err)
				}
			})
		}
	}
}

func notificationCorrelationRequest(t *testing.T, operation string) map[string]any {
	t.Helper()
	var request map[string]any
	if err := json.Unmarshal(document(t, operation), &request); err != nil {
		t.Fatal(err)
	}
	request["notification"] = map[string]any{
		"contract": "curator-host-notification/1", "backend": "host",
		"workflow": map[string]any{"reference": "work-1", "sha256": strings.Repeat("a", 64)},
		"member": map[string]any{
			"mxid": "@fixture-agent:example.test", "agent_selector": "fixture-agent",
			"connection": "fixture-connection", "configuration_root": "/fixture/source",
		},
		"admission_reference": "fixture-record", "call_timeout_ms": 1,
	}
	return request
}
