package managerschema

import (
	"encoding/json"
	"strings"
	"testing"
)

func notificationRequest() map[string]any {
	return map[string]any{
		"contract": notificationContract, "backend": "host",
		"workflow": map[string]any{"reference": "work-1", "sha256": strings.Repeat("a", 64)},
		"member": map[string]any{
			"mxid": "@fixture-agent:example.test", "agent_selector": "fixture-agent",
			"connection": "fixture-connection", "configuration_root": "/fixture/source",
		},
		"admission_reference": "fixture-record", "call_timeout_ms": 1,
	}
}

// The accepted optional intent must cross the real launch/resume schemas,
// while legacy omission remains valid. This checks public wire acceptance,
// not the independently retained manager record or Source preparation.
func TestNotificationRequestPositive(t *testing.T) {
	for _, operation := range []string{"launch", "resume"} {
		t.Run(operation, func(t *testing.T) {
			notificationVerdict(t, operation, notificationDocument(t, operation), true)
			for _, timeout := range []int{1, 60000} {
				request := notificationDocument(t, operation)
				n := notificationRequest()
				n["call_timeout_ms"] = timeout
				request["notification"] = n
				notificationVerdict(t, operation, request, true)
			}
		})
	}
}

// Each selected intent is closed and complete; valid neighbors must still pass
// before these refusals can count as evidence for the new request contract.
func TestNotificationRequestClosed(t *testing.T) {
	for _, operation := range []string{"launch", "resume"} {
		t.Run(operation, func(t *testing.T) {
			for key := range notificationRequest() {
				for _, kind := range []string{"missing", "null"} {
					t.Run(key+"/"+kind, func(t *testing.T) {
						request := notificationDocument(t, operation)
						n := notificationRequest()
						if kind == "missing" {
							delete(n, key)
						} else {
							n[key] = nil
						}
						request["notification"] = n
						notificationVerdict(t, operation, request, false)
					})
				}
			}
			for _, kind := range []string{"null", "off", "future-contract", "unknown-field", "workflow-extra", "member-extra", "zero-timeout", "large-timeout", "fractional-timeout"} {
				t.Run(kind, func(t *testing.T) {
					request := notificationDocument(t, operation)
					n := notificationRequest()
					request["notification"] = n
					switch kind {
					case "null":
						request["notification"] = nil
					case "off":
						n["backend"] = "off"
					case "future-contract":
						n["contract"] = "curator-host-notification/2"
					case "unknown-field":
						n["trusted"] = true
					case "workflow-extra":
						n["workflow"].(map[string]any)["endpoint"] = "sentinel"
					case "member-extra":
						n["member"].(map[string]any)["owner_pid"] = 1234
					case "zero-timeout":
						n["call_timeout_ms"] = 0
					case "large-timeout":
						n["call_timeout_ms"] = 60001
					case "fractional-timeout":
						n["call_timeout_ms"] = 1.5
						// The strict protocol reader rejects fractions before schema checks.
						data, err := json.Marshal(request)
						if err != nil {
							t.Fatal(err)
						}
						err = Validate(operation+".schema.json", data)
						if err == nil || err.Error() != "invalid protocol JSON" {
							t.Fatalf("fractional timeout did not retain strict JSON refusal: %v", err)
						}
						return
					}
					notificationVerdict(t, operation, request, false)
				})
			}
		})
	}
}

// These are wire-shape and byte-bound checks, not a replacement Source server
// parser, Source-host path check or proof of independently retained admission.
func TestNotificationRequestBounds(t *testing.T) {
	for _, operation := range []string{"launch", "resume"} {
		for _, row := range []struct {
			name, field, value string
			valid              bool
		}{
			{"agent-max", "agent_selector", "a" + strings.Repeat("=", 63), true},
			{"agent-too-long", "agent_selector", strings.Repeat("a", 65), false},
			{"agent-leading-punctuation", "agent_selector", "-agent", false},
			{"connection-max", "connection", "a" + strings.Repeat("_", 63), true},
			{"connection-too-long", "connection", strings.Repeat("a", 65), false},
			{"connection-agent-only-punctuation", "connection", "a=b", false},
			{"mxid-case-port", "mxid", "@fixture-agent:Example.test:8448", true},
			{"mxid-ipv6", "mxid", "@fixture-agent:[2001:db8::1]:8448", true},
			{"mxid-max", "mxid", "@a:" + strings.Repeat("b", 252), true},
			{"mxid-too-long", "mxid", "@a:" + strings.Repeat("b", 253), false},
			{"mxid-multibyte-max", "mxid", "@a:" + strings.Repeat("é", 126), true},
			{"mxid-multibyte-too-long", "mxid", "@a:" + strings.Repeat("é", 127), false},
			{"mxid-empty-server", "mxid", "@a:", false},
			{"mxid-control", "mxid", "@a:sentinel\u0085server", false},
			{"root-max", "configuration_root", "/" + strings.Repeat("é", 2047) + "a", true},
			{"root-too-long", "configuration_root", "/" + strings.Repeat("é", 2048), false},
			{"root-empty", "configuration_root", "", false},
			{"root-control", "configuration_root", "/sentinel\nroot", false},
		} {
			t.Run(operation+"/"+row.name, func(t *testing.T) {
				request := notificationDocument(t, operation)
				n := notificationRequest()
				n["member"].(map[string]any)[row.field] = row.value
				request["notification"] = n
				notificationVerdict(t, operation, request, row.valid)
			})
		}
		for _, field := range []string{"reference", "sha256"} {
			for _, kind := range []string{"missing", "null", "malformed"} {
				t.Run(operation+"/workflow-"+field+"/"+kind, func(t *testing.T) {
					request := notificationDocument(t, operation)
					n := notificationRequest()
					workflow := n["workflow"].(map[string]any)
					switch kind {
					case "missing":
						delete(workflow, field)
					case "null":
						workflow[field] = nil
					case "malformed":
						workflow[field] = "private sentinel"
					}
					request["notification"] = n
					notificationVerdict(t, operation, request, false)
				})
			}
		}
		for _, field := range []string{"mxid", "agent_selector", "connection", "configuration_root"} {
			t.Run(operation+"/missing-member-"+field, func(t *testing.T) {
				request := notificationDocument(t, operation)
				n := notificationRequest()
				delete(n["member"].(map[string]any), field)
				request["notification"] = n
				notificationVerdict(t, operation, request, false)
			})
		}
	}
}
