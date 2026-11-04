package werkstatt

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func notificationInputValue() map[string]any {
	return map[string]any{
		"member": map[string]any{
			"mxid": "@example.dev:Example.test:8448", "agent_selector": "example.dev",
			"connection": "fixture-connection", "configuration_root": "/fixture/source",
		},
		"admission_reference": "retained-record-1", "call_timeout_ms": 1,
	}
}

// The editable record carries explicit expected data without losing fields.
// Parsing does not establish independently retained manager admission.
func TestRecordNotificationExpectedData(t *testing.T) {
	for _, timeout := range []int{1, 60000} {
		v := recordValue(t.TempDir())
		input := notificationInputValue()
		input["call_timeout_ms"] = timeout
		v["notification"] = input
		r, err := Parse(encoded(v), "example")
		if err != nil {
			t.Fatalf("valid explicit notification input refused: %v", err)
		}
		var got, want map[string]any
		if json.Unmarshal(encoded(r), &got) != nil || json.Unmarshal(encoded(v), &want) != nil || !reflect.DeepEqual(got["notification"], want["notification"]) {
			t.Fatal("expected notification data was lost or changed")
		}
	}
}

// Every supplied input is closed, complete and byte bounded. Legacy omission
// remains covered by TestRecord; these verdicts concern data, not authority.
func TestRecordNotificationRefusals(t *testing.T) {
	for _, kind := range []string{"null", "missing-member", "missing-ref", "missing-timeout", "unknown", "workflow", "member-extra", "zero-timeout", "large-timeout", "fractional-timeout", "wrong-timeout-type", "ref-bytes", "root-bytes", "mxid-bytes"} {
		t.Run(kind, func(t *testing.T) {
			v := recordValue(t.TempDir())
			input := notificationInputValue()
			v["notification"] = input
			switch kind {
			case "null":
				v["notification"] = nil
			case "missing-member":
				delete(input, "member")
			case "missing-ref":
				delete(input, "admission_reference")
			case "missing-timeout":
				delete(input, "call_timeout_ms")
			case "unknown":
				input["trusted"] = true
			case "workflow":
				input["workflow"] = map[string]any{"reference": "sentinel"}
			case "member-extra":
				input["member"].(map[string]any)["owner_pid"] = 123
			case "zero-timeout":
				input["call_timeout_ms"] = 0
			case "large-timeout":
				input["call_timeout_ms"] = 60001
			case "fractional-timeout":
				input["call_timeout_ms"] = 1.5
			case "wrong-timeout-type":
				input["call_timeout_ms"] = "1"
			case "ref-bytes":
				input["admission_reference"] = strings.Repeat("é", 257)
			case "root-bytes":
				input["member"].(map[string]any)["configuration_root"] = "/" + strings.Repeat("é", 2048)
			case "mxid-bytes":
				input["member"].(map[string]any)["mxid"] = "@a:" + strings.Repeat("é", 127)
			}
			r, err := Parse(encoded(v), "example")
			code(t, err, "werkstatt_record_invalid")
			if r != nil {
				t.Fatal("partial record escaped refusal")
			}
		})
	}
}
