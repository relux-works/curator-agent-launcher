package client

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/hosted"
	managerschema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
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
				if r.Session != nil || r.SessionRecord != nil || r.Outcome != nil || r.Page != nil || r.Describe != nil {
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

// A draining record keeps its schema-paired reason and remediation through
// decoding; the same member on a non-draining record rejects the response.
func TestDecodeDrainingReason(t *testing.T) {
	stop := `{"session":"SES-example","expected_generation":1,"mode":"graceful","precondition":"none"}`
	draining := session()
	draining.State = "draining"
	draining.Draining = &Draining{Reason: "stop_intent_unsignalled_after_restart", Remediation: "stop_force"}
	r, err := DecodeResponse("stop", []byte(stop), encoded(draining), 0, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if r.Session.Draining == nil || *r.Session.Draining != *draining.Draining {
		t.Fatalf("draining reason lost: %+v", r.Session.Draining)
	}
	busy := draining
	busy.State = "busy"
	r, err = DecodeResponse("stop", []byte(stop), encoded(busy), 0, "demo")
	requireCode(t, err, "session_host_protocol_error")
	if r.Session != nil {
		t.Fatal("partial poisoned response")
	}
	mismatch := draining
	mismatch.Draining = &Draining{Reason: "stop_intent_unsignalled_after_restart", Remediation: "wait"}
	_, err = DecodeResponse("stop", []byte(stop), encoded(mismatch), 0, "demo")
	requireCode(t, err, "session_host_protocol_error")
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

// Selector-bearing tails remain wire-valid but the launcher refuses them
// before transport, with the same code, empty detail and exit as before the
// module split. Value-position "resume" is an adjacent accepted control.
func TestNativeTailProviderValidation(t *testing.T) {
	for _, runtime := range []string{"codex_cli", "claude_code", "muse"} {
		for _, tc := range []struct {
			name  string
			tail  []string
			valid bool
		}{
			{"empty", []string{}, true},
			{"value", []string{"--model", "resume"}, true},
			{"selector", []string{"--resume", "latest"}, false},
			{"selector-id", []string{"--resume", operationID}, false},
			{"incomplete-selector", []string{"--resume"}, false},
		} {
			t.Run(runtime+"/"+tc.name, func(t *testing.T) {
				var request map[string]any
				if err := json.Unmarshal(document(t, "launch"), &request); err != nil {
					t.Fatal(err)
				}
				request["runtime"], request["native_tail"] = runtime, tc.tail
				body := encoded(request)
				if err := managerschema.Validate("launch.schema.json", body); err != nil {
					t.Fatalf("wire control refused: %v", err)
				}
				err := ValidateRequest("launch", body)
				if tc.valid {
					if err != nil {
						t.Fatal(err)
					}
					return
				}
				refusal := requireCode(t, err, "launch_plan_invalid")
				if len(refusal.Details) != 0 || refusal.ExitCode() != 2 || refusal.Diagnostic() != "launch_plan_invalid: Invalid launch plan." {
					t.Fatalf("selector refusal changed: %#v", refusal)
				}
			})
		}
	}
}

// The client preserves all pre-split launch-vector verdicts: the four tails
// whose wire shape is now valid still fail provider semantics at this boundary.
func TestLaunchVectorVerdicts(t *testing.T) {
	data, err := managerschema.Documents.ReadFile("vectors/launch.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Name     string          `json:"name"`
		Valid    bool            `json:"valid"`
		Document json.RawMessage `json:"document"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	semantic := map[string]bool{
		"native-tail-codex-selector-id-shape":    true,
		"native-tail-codex_cli-selector-shape":   true,
		"native-tail-claude_code-selector-shape": true,
		"native-tail-muse-selector-shape":        true,
	}
	seen := 0
	for _, vector := range vectors {
		want := vector.Valid
		if semantic[vector.Name] {
			seen++
			want = false
		}
		t.Run(vector.Name, func(t *testing.T) {
			if err := ValidateRequest("launch", vector.Document); (err == nil) != want {
				t.Fatalf("pre-split verdict valid=%t, got %v", want, err)
			}
		})
	}
	if seen != len(semantic) {
		t.Fatal("semantic regression vector missing")
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

// A local workspace path may contain newlines; it must remain one diagnostic
// value and cannot create another launcher diagnostic line.
func TestLocalPathDiagnostic(t *testing.T) {
	e := &Error{Code: "werkstatt_context_leak", Details: map[string]string{"path": "/workspace/clone\ncurator-run: injected"}}
	if strings.Contains(e.Diagnostic(), "\n") || !strings.Contains(e.Diagnostic(), `path="/workspace/clone\ncurator-run: injected"`) {
		t.Fatal("path forged diagnostic", e.Diagnostic())
	}
}

// Plugin refusals retain both specified exact-route capability details; an
// invented detail poisons the entire response instead of escaping validation.
func TestExactCapabilityRefusal(t *testing.T) {
	for _, detail := range []string{"launch-exact-doorbell", "launch-exact-detached", "launch-exact-invented"} {
		t.Run(detail, func(t *testing.T) {
			body := encoded(map[string]any{"protocol": Protocol, "ok": false, "code": "session_host_capability_missing", "details": map[string]string{"capability": detail}})
			response, err := DecodeResponse("launch", document(t, "launch"), body, 6, "demo")
			code := "session_host_capability_missing"
			if detail == "launch-exact-invented" {
				code = "session_host_protocol_error"
			}
			typed := requireCode(t, err, code)
			if code == "session_host_capability_missing" && typed.Details["capability"] != detail {
				t.Fatal("capability detail lost")
			}
			if response.Session != nil || response.Outcome != nil || response.Page != nil || response.Describe != nil {
				t.Fatal("partial refusal response")
			}
		})
	}
}

// Direct status exposes exactly the validated envelope-free members, preserving
// optional members, explicit nulls and large integers; it never projects history.
func TestStatusSessionRecord(t *testing.T) {
	readable := sessionRow()
	readable["generation"] = int64(9007199254740991)
	readable["state"] = "draining"
	readable["draining"] = map[string]string{"reason": "stop_intent_unsignalled_after_restart", "remediation": "stop_force"}
	for _, row := range []map[string]any{readable, unreadableRow()} {
		want := encoded(row)
		row["protocol"], row["ok"] = Protocol, true
		got, err := DecodeResponse("status", []byte(`{"session":"SES-example"}`), encoded(row), 0, "demo")
		if err != nil || string(got.SessionRecord) != string(want) {
			t.Fatalf("record changed: %s want %s: %v", got.SessionRecord, want, err)
		}
		if err := managerschema.Validate("session-record.schema.json", got.SessionRecord); err != nil {
			t.Fatal(err)
		}
	}
	got, err := DecodeResponse("status", statusRequest(), []byte(outcome("started")), 0, "demo")
	if err != nil || got.SessionRecord != nil {
		t.Fatal("operation outcome became a session record", got, err)
	}
}
