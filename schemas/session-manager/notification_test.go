package managerschema

import (
	"encoding/json"
	"strings"
	"testing"
)

const notificationContract = "curator-host-notification/1"

func notificationBinding() map[string]any {
	return map[string]any{
		"execution": "execution-1", "controller_epoch": "1", "runner_instance": "runner-1",
		"native_thread": "thread-1", "member_ref": "member-1", "connection_ref": "connection-1",
		"configuration_ref": "configuration-1", "profile_lock_ref": "lock-1",
		"consumer_generation": "generation-1", "enrollment_ref": "enrollment-1",
		"workflow_reference": "work-1", "workflow_sha256": strings.Repeat("a", 64),
	}
}

func notificationResult(state string) map[string]any {
	v := map[string]any{"contract": notificationContract, "state": state}
	if state == "off" {
		v["reason"] = "selection-disabled"
		return v
	}
	v["attachment_ref"] = "attachment-1"
	if state == "ready" {
		v["binding"] = notificationBinding()
		return v
	}
	v["workflow_reference"], v["workflow_sha256"] = "work-1", strings.Repeat("a", 64)
	if state == "unresolved" {
		v["reason"] = "attempt-unresolved"
	}
	return v
}

func notificationDocument(t *testing.T, schema string) map[string]any {
	t.Helper()
	b, err := Documents.ReadFile("vectors/" + schema + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct{ Document map[string]any }
	if err := json.Unmarshal(b, &rows); err != nil || len(rows) == 0 {
		t.Fatal("missing positive vector", err)
	}
	return rows[0].Document
}

func notificationVerdict(t *testing.T, schema string, value any, valid bool) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	err = Validate(schema+".schema.json", b)
	if (err == nil) != valid {
		t.Fatalf("notification contract valid=%t: %v", valid, err)
	}
	if err != nil && !strings.HasPrefix(err.Error(), "schema violation at $") {
		t.Fatalf("unexpected notification refusal: %v", err)
	}
}

// Existing readable session and envelope-free records retain all four closed
// notification phases. Legacy omission stays valid; shape alone grants no
// enrollment, workflow correlation or permission to trigger a notice.
func TestSessionNotificationProjection(t *testing.T) {
	for _, schema := range []string{"session", "session-record"} {
		t.Run(schema, func(t *testing.T) {
			notificationVerdict(t, schema, notificationDocument(t, schema), true)
			for _, state := range []string{"preparing", "ready", "unresolved", "off"} {
				t.Run(state, func(t *testing.T) {
					row := notificationDocument(t, schema)
					row["notification"] = notificationResult(state)
					notificationVerdict(t, schema, row, true)
				})
			}
		})
	}
}

// Every required phase field is present and typed; unknown or cross-variant
// fields cannot turn partial readiness or uncertainty into an accepted result.
func TestNotificationResultClosedSum(t *testing.T) {
	for _, state := range []string{"preparing", "ready", "unresolved", "off"} {
		t.Run(state, func(t *testing.T) {
			valid := notificationResult(state)
			notificationVerdict(t, "notification-result", valid, true)
			for key := range valid {
				t.Run("missing-"+key, func(t *testing.T) {
					v := notificationResult(state)
					delete(v, key)
					notificationVerdict(t, "notification-result", v, false)
				})
				t.Run("null-"+key, func(t *testing.T) {
					v := notificationResult(state)
					v[key] = nil
					notificationVerdict(t, "notification-result", v, false)
				})
			}
			for _, key := range []string{"binding", "attachment_ref", "workflow_reference", "workflow_sha256", "reason", "endpoint", "native_thread"} {
				if _, present := valid[key]; present {
					continue
				}
				v := notificationResult(state)
				v[key] = "sentinel"
				notificationVerdict(t, "notification-result", v, false)
			}
			for _, bad := range []any{nil, []any{}, "ready", map[string]any{}} {
				notificationVerdict(t, "notification-result", bad, false)
			}
		})
	}
	for _, reason := range []string{"observation-unavailable", "attempt-unresolved", "selection-disabled", "unknown"} {
		v := notificationResult("unresolved")
		v["reason"] = reason
		notificationVerdict(t, "notification-result", v, reason == "observation-unavailable" || reason == "attempt-unresolved")
	}
	for _, edit := range []struct {
		key   string
		value any
	}{
		{"contract", "foreign/1"}, {"state", "armed"}, {"state", true},
		{"workflow_sha256", strings.Repeat("A", 64)}, {"workflow_reference", "bad value"},
		{"attachment_ref", ""},
	} {
		v := notificationResult("preparing")
		v[edit.key] = edit.value
		notificationVerdict(t, "notification-result", v, false)
	}
}

// A ready binding contains exactly twelve bounded primitives. Epochs are
// canonical nonzero uint64 strings, independent of JSON's safe-integer range.
func TestNotificationBindingPrimitives(t *testing.T) {
	for key := range notificationBinding() {
		for _, bad := range []any{nil, "", 1} {
			v := notificationBinding()
			v[key] = bad
			notificationVerdict(t, "notification-binding", v, false)
		}
		v := notificationBinding()
		delete(v, key)
		notificationVerdict(t, "notification-binding", v, false)
	}
	for _, epoch := range []string{"1", "9007199254740992", "18446744073709551615", "0", "01", "+1", "1e1", "18446744073709551616", "99999999999999999999"} {
		v := notificationBinding()
		v["controller_epoch"] = epoch
		notificationVerdict(t, "notification-binding", v, epoch == "1" || epoch == "9007199254740992" || epoch == "18446744073709551615")
	}
	for _, key := range []string{"execution", "runner_instance", "native_thread", "workflow_reference"} {
		for _, s := range []string{"Az09._:/@+-", strings.Repeat("x", 128), strings.Repeat("x", 129), "with space", "é", "x\n"} {
			v := notificationBinding()
			v[key] = s
			notificationVerdict(t, "notification-binding", v, s == "Az09._:/@+-" || s == strings.Repeat("x", 128))
		}
	}
	for _, key := range []string{"member_ref", "configuration_ref", "profile_lock_ref", "enrollment_ref", "connection_ref", "consumer_generation"} {
		limit := 512
		if key == "connection_ref" || key == "consumer_generation" {
			limit = 128
		}
		for _, s := range []string{strings.Repeat("é", limit/2), strings.Repeat("é", limit/2) + "x", "a\u0085b", "a\u007fb", "a\x00b", "a\tb", "space permitted"} {
			v := notificationBinding()
			v[key] = s
			notificationVerdict(t, "notification-binding", v, s == strings.Repeat("é", limit/2) || s == "space permitted")
		}
	}
	v := notificationBinding()
	v["extra"] = "sentinel"
	notificationVerdict(t, "notification-binding", v, false)
	v = notificationBinding()
	v["workflow_sha256"] = strings.Repeat("A", 64)
	notificationVerdict(t, "notification-binding", v, false)
}

// Reference byte limits apply through every nested result use, not only the
// standalone schema. An unreadable record cannot assert notification evidence.
func TestNotificationNestedRefusal(t *testing.T) {
	for _, valid := range []bool{true, false} {
		row := notificationDocument(t, "session-record")
		result := notificationResult("ready")
		if !valid {
			result["binding"].(map[string]any)["consumer_generation"] = strings.Repeat("é", 65)
		}
		row["notification"] = result
		for schema, v := range map[string]any{
			"session-record": row,
			"outcome":        map[string]any{"protocol": "curator-session-manager/1", "ok": true, "outcome": "started", "session": row},
			"list-page":      map[string]any{"protocol": "curator-session-manager/1", "ok": true, "sessions": []any{row}, "next_cursor": nil},
		} {
			notificationVerdict(t, schema, v, valid)
		}
	}
	row := notificationDocument(t, "session-record")
	row["readable"], row["state"] = false, "unknown"
	for _, key := range []string{"operation_id", "runtime", "native_id", "generation", "started_at", "last_activity_at"} {
		row[key] = nil
	}
	notificationVerdict(t, "session-record", row, true)
	row["notification"] = notificationResult("off")
	notificationVerdict(t, "session-record", row, false)
	for _, body := range []string{
		`{"contract":"curator-host-notification/1","state":"off","state":"off","reason":"selection-disabled"}`,
		`{"contract":"curator-host-notification/1","state":"off","reason":"selection-disabled"} {}`,
	} {
		if err := Validate("notification-result.schema.json", []byte(body)); err == nil || err.Error() != "invalid protocol JSON" {
			t.Fatalf("strict notification JSON refusal: %v", err)
		}
	}
}

// The new capability is an admitted refusal detail beside the existing exact
// capability. Other versions, arbitrary strings and extra detail keys refuse.
func TestNotificationCapabilityRefusalSchema(t *testing.T) {
	for _, tc := range []struct {
		name    string
		details map[string]any
		valid   bool
	}{
		{"notification", map[string]any{"capability": notificationContract}, true},
		{"existing", map[string]any{"capability": "launch-exact-detached"}, true},
		{"next-version", map[string]any{"capability": "curator-host-notification/2"}, false},
		{"invented", map[string]any{"capability": "invented-capability"}, false},
		{"null", map[string]any{"capability": nil}, false},
		{"extra-detail", map[string]any{"capability": notificationContract, "reason": "sentinel"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{"protocol": "curator-session-manager/1", "ok": false, "code": "session_host_capability_missing", "details": tc.details}
			notificationVerdict(t, "refusal", body, tc.valid)
		})
	}
}
