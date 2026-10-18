package client

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/hosted"
)

// A valid session/refusal/describe/page is accepted, while any poisoned member,
// envelope disagreement or foreign correlation rejects the whole response.
// Unreadable projections survive only list/direct status; started needs an
// envelope-free readable record.
func TestDecodeResponse(t *testing.T) {
	valid := session()
	good := string(encoded(valid))
	unreadable := unreadableRow()
	unreadable["protocol"], unreadable["ok"] = Protocol, true
	unreadableStatus := string(encoded(unreadable))
	unreadableList := string(encoded(map[string]any{"protocol": Protocol, "ok": true, "sessions": []map[string]any{unreadableRow()}, "next_cursor": nil}))
	refusalBody := `{"protocol":"curator-session-manager/1","ok":false,"code":"session_host_unavailable","details":{"reason":"transport"}}`
	cases := []struct {
		name, op, request, body string
		exit                    int
		code                    string
	}{
		{"unreadable-started", "status", string(statusRequest()), string(encoded(map[string]any{"protocol": Protocol, "ok": true, "outcome": "started", "session": unreadableRow()})), 0, "session_host_protocol_error"},
		{"enveloped-started", "status", string(statusRequest()), string(encoded(map[string]any{"protocol": Protocol, "ok": true, "outcome": "started", "session": valid})), 0, "session_host_protocol_error"},
		{"unreadable-launch", "launch", string(document(t, "launch")), unreadableStatus, 0, "session_host_protocol_error"},
		{"unreadable-resume", "resume", string(document(t, "resume")), unreadableStatus, 0, "session_host_protocol_error"},
		{"unreadable-stop", "stop", `{"session":"SES-example","expected_generation":1,"mode":"graceful","precondition":"none"}`, unreadableStatus, 0, "session_host_protocol_error"},
		{"unreadable-status", "status", `{"session":"SES-example"}`, unreadableStatus, 0, ""},
		{"unreadable-list", "list", `{"labels":{}}`, unreadableList, 0, ""},
		{"unreadable-filter", "list", `{"labels":{"k":"v"}}`, unreadableList, 0, "session_host_protocol_error"},
		{"row-envelope", "list", `{"labels":{}}`, string(encoded(map[string]any{"protocol": Protocol, "ok": true, "sessions": []Session{valid}, "next_cursor": nil})), 0, "session_host_protocol_error"},
		{"index-unreadable", "list", `{"labels":{}}`, `{"protocol":"curator-session-manager/1","ok":false,"code":"session_host_unavailable","details":{"reason":"index_unreadable"}}`, 1, "session_host_unavailable"},
		{"busy-draining", "describe", "{}", `{"protocol":"curator-session-manager/1","ok":false,"code":"werkstatt_busy","details":{"state":"draining"}}`, 1, "werkstatt_busy"},
		{"session", "launch", string(document(t, "launch")), good, 0, ""},
		{"resume", "resume", string(document(t, "resume")), good, 0, ""},
		{"status-handle", "status", `{"session":"SES-example"}`, good, 0, ""},
		{"stop-generation", "stop", `{"session":"SES-example","expected_generation":1,"mode":"graceful","precondition":"none"}`, good, 0, ""},
		{"refusal", "launch", string(document(t, "launch")), refusalBody, 1, "session_host_unavailable"},
		{"refusal-zero", "launch", string(document(t, "launch")), refusalBody, 0, "session_host_protocol_error"},
		{"success-nonzero", "launch", string(document(t, "launch")), good, 1, "session_host_protocol_error"},
		{"historical-failure", "status", string(statusRequest()), outcome("failed"), 0, ""},
		{"launcher-local-refusal", "describe", "{}", `{"protocol":"curator-session-manager/1","ok":false,"code":"werkstatt_identity_missing","details":{}}`, 1, "session_host_protocol_error"},
		{"launcher-local-history", "status", string(statusRequest()), `{"protocol":"curator-session-manager/1","ok":true,"outcome":"failed","code":"werkstatt_record_invalid"}`, 0, "session_host_protocol_error"},
		{"plugin-owned-refusal", "describe", "{}", `{"protocol":"curator-session-manager/1","ok":false,"code":"werkstatt_busy","details":{"state":"busy"}}`, 1, "werkstatt_busy"},
		{"started", "cancel", string(statusRequest()), outcome("started"), 0, ""},
		{"unknown-member", "launch", string(document(t, "launch")), strings.Replace(good, `"ok":true`, `"ok":true,"text":"sentinel"`, 1), 0, "session_host_protocol_error"},
		{"free-text", "describe", "{}", strings.Replace(refusalBody, `"transport"`, `"sentinel-secret"`, 1), 1, "session_host_protocol_error"},
		{"unknown-detail", "describe", "{}", strings.Replace(refusalBody, `"reason"`, `"text"`, 1), 1, "session_host_protocol_error"},
		{"missing-protocol", "launch", string(document(t, "launch")), strings.Replace(good, `"protocol":"curator-session-manager/1",`, "", 1), 0, "session_host_protocol_error"},
		{"duplicate", "launch", string(document(t, "launch")), strings.Replace(good, `"ok":true`, `"ok":true,"ok":true`, 1), 0, "session_host_protocol_error"},
		{"trailing", "launch", string(document(t, "launch")), good + "{}", 0, "session_host_protocol_error"},
		{"oversize", "launch", string(document(t, "launch")), strings.Repeat(" ", Limit+1), 0, "session_host_protocol_error"},
		{"foreign-manager", "launch", string(document(t, "launch")), strings.Replace(good, `"manager":"demo"`, `"manager":"other"`, 1), 0, "session_host_protocol_error"},
		{"foreign-operation", "launch", string(document(t, "launch")), strings.Replace(good, operationID, secondID, 1), 0, "session_host_protocol_error"},
		{"foreign-runtime", "launch", string(document(t, "launch")), strings.Replace(good, `"runtime":"codex_cli"`, `"runtime":"muse"`, 1), 0, "session_host_protocol_error"},
		{"foreign-handle", "status", `{"session":"SES-other"}`, good, 0, "session_host_protocol_error"},
		{"foreign-generation", "stop", `{"session":"SES-example","expected_generation":2}`, good, 0, "session_host_protocol_error"},
		{"describe", "describe", "{}", string(document(t, "describe")), 0, ""},
		{"foreign-describe", "describe", "{}", strings.Replace(string(document(t, "describe")), `"demo"`, `"other"`, 1), 0, "session_host_protocol_error"},
		{"page", "list", `{"labels":{}}`, string(document(t, "list-page")), 0, ""},
		{"foreign-filter", "list", `{"labels":{"werkstatt":"example"}}`, string(document(t, "list-page")), 0, "session_host_protocol_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := DecodeResponse(tc.op, []byte(tc.request), []byte(tc.body), tc.exit, "demo")
			if tc.code != "" {
				requireCode(t, err, tc.code)
				if r.Session != nil || r.Outcome != nil || r.Page != nil || r.Describe != nil {
					t.Fatal("partial poisoned response")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "unreadable-status" && (r.Session.Readable || r.Session.State != "unknown" || r.Session.Session != "SES-example") {
				t.Fatal("unreadable projection changed")
			}
			if tc.name == "unreadable-list" && (len(r.Page.Sessions) != 1 || r.Page.Sessions[0].Readable || r.Page.Sessions[0].State != "unknown") {
				t.Fatal("unreadable row lost")
			}
			if tc.name == "historical-failure" && r.Outcome.Code != "policy_refused" {
				t.Fatal("historical code lost")
			}
		})
	}
}

// Request validation rejects secrets and reserved loader inputs before exec;
// neighboring non-secret names remain allowed and errors reveal no values.
func TestRequestPolicy(t *testing.T) {
	base := document(t, "launch")
	for _, tc := range []struct{ name, value, code string }{
		{"GIT_AUTHOR_NAME", "Example", ""}, {"RELEASE_SIGNING_KEY", "/workspace/key.pub", ""},
		{"LD_PRELOAD", "/workspace/sentinel", "credential_reserved_name"}, {"PYTHONPATH", "sentinel", "credential_reserved_name"}, {"XDG_CONFIG_HOME", "/workspace/sentinel", "credential_reserved_name"},
		{"TOKEN", "sentinel-secret", "secret_policy_violation"}, {"VALUE", "ghp_sentinel-secret", "secret_policy_violation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var m map[string]any
			_ = json.Unmarshal(base, &m)
			m["child_env"] = map[string]string{tc.name: tc.value}
			err := ValidateRequest("launch", encoded(m))
			if tc.code == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			e := requireCode(t, err, tc.code)
			if strings.Contains(e.Diagnostic(), tc.value) {
				t.Fatal("request value escaped")
			}
		})
	}
}

// New diagnostics have fixed text and the documented non-default exit classes.
func TestDiagnosticClassification(t *testing.T) {
	for _, tc := range []struct {
		code string
		exit int
	}{{"session_host_protocol_error", 6}, {"session_host_capability_missing", 6}, {"session_host_delegated_unauthorized", 16}, {"credential_source_stale", 2}, {"session_operation_conflict", 1}, {"network_scope_unsupported", 16}} {
		e := refusal(tc.code, nil)
		if e.ExitCode() != tc.exit || Message(tc.code) == "" {
			t.Fatalf("classification %s: %d %s", tc.code, e.ExitCode(), Message(tc.code))
		}
	}
}

// Shared fd4 codes keep the existing receiver's constant messages and exits;
// extending the RPC registry must not invalidate a previously valid frame.
func TestExistingStatusCompatibility(t *testing.T) {
	for _, code := range hosted.StatusRegistryCodes() {
		exit, message, _ := hosted.StatusRegistryRow(code)
		if Message(code) != message || refusal(code, nil).ExitCode() != exit {
			t.Fatalf("changed existing fd4 classification: %s", code)
		}
	}
}
