package hosted_test

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/hosted"
)

// TestStatusRegistryCompleteness pins the receiving registry against
// Appendix V-ERR: the exact member count (no dropped or duplicated
// code), exits inside {1,2,6,16}, literal constant messages (ASCII,
// final period, at most 512 bytes), and the synthesized normalization
// outcome.
func TestStatusRegistryCompleteness(t *testing.T) {
	if got := hosted.StatusRegistrySize(); got != 135 {
		t.Fatalf("registry size = %d, want 135 Appendix V-ERR members", got)
	}
	// Membership hash over the sorted codes: any dropped, added, or
	// renamed member changes it. Verified against Appendix V-ERR.
	sum := sha256.Sum256([]byte(strings.Join(hosted.StatusRegistryCodes(), "\n")))
	if got := fmt.Sprintf("%x", sum); got != "2b6cd35f7b0caa6df6bc11e2548e1afb259228b568b664915dafe2bfa117185e" {
		t.Fatalf("registry membership hash = %s", got)
	}
	for _, code := range hosted.StatusRegistryCodes() {
		exit, message, ok := hosted.StatusRegistryRow(code)
		if !ok {
			t.Fatalf("%s: listed but not recognized", code)
		}
		switch exit {
		case 1, 2, 6, 16:
		default:
			t.Fatalf("%s: exit = %d, want 1, 2, 6, or 16", code, exit)
		}
		if message == "" || !strings.HasSuffix(message, ".") || len(message) > 512 {
			t.Fatalf("%s: message = %q, want a literal constant with a final period", code, message)
		}
		for i := 0; i < len(message); i++ {
			if message[i] > 127 {
				t.Fatalf("%s: message = %q, want ASCII bytes", code, message)
			}
		}
	}
	// The synthesized normalization outcome is authoritative here.
	if exit, ok := hosted.StatusExitForCode("session_host_protocol_error"); !ok || exit != 6 {
		t.Fatalf("protocol_error exit = %d/%v, want 6/true", exit, ok)
	}
	if hosted.ProtocolErrorExit() != 6 {
		t.Fatalf("ProtocolErrorExit() = %d, want 6", hosted.ProtocolErrorExit())
	}
	if hosted.ProtocolErrorMessage() != "Invalid session host response." {
		t.Fatalf("ProtocolErrorMessage() = %q", hosted.ProtocolErrorMessage())
	}
	// Spot rows across every exit class and detail vocabulary.
	for code, exit := range map[string]int{
		"usage": 2, "launch_plan_invalid": 2, "session_resume_invalid": 2,
		"host_configuration_conflict": 2, "session_host_missing": 1,
		"session_host_unavailable": 1, "session_host_protocol_unsupported": 6,
		"session_host_capability_missing": 6, "session_host_provider_unsupported": 6,
		"secret_policy_violation": 16, "policy_refused": 16,
		"network_scope_unsupported": 16, "network_profile_drift": 16,
		"session_host_default_not_ready": 16, "session_resume_not_found": 1,
		"session_adoption_conflict": 1, "session_liveness_unknown": 1,
		"session_live_detached": 1, "defaults_config_invalid": 1,
		"permission_mode_tracked_unsupported": 1, "plan_refused": 1,
		"claude_goal_unavailable": 1, "claude_context_base_not_forkable": 1,
		"context_fork_preflight_failed": 1, "owner_activation_in_progress": 1,
		"session_reservation_stale": 1, "session_takeover_not_applicable": 1,
		"goal_reflection_skew": 1, "codex_context_base_not_forkable": 1,
		"codex_thread_not_found": 1, "context_not_found": 1,
		"context_validation_budget_exceeded": 1, "goal_not_found": 1,
		"goal_native_reflection_required": 1,
	} {
		if got, ok := hosted.StatusExitForCode(code); !ok || got != exit {
			t.Fatalf("%s: exit = %d/%v, want %d/true", code, got, ok, exit)
		}
	}
	if _, ok := hosted.StatusExitForCode("session_host_future_x"); ok {
		t.Fatal("unknown code recognized, want protocol_error normalization")
	}
	if _, ok := hosted.StatusExitForCode(""); ok {
		t.Fatal("empty code recognized")
	}
}

// TestNormalizeStatusRecord pins the closed envelope: exact members,
// pinned consts, registry code, literal message, and the row's detail
// vocabulary. Every violation normalizes to a protocol error; nothing
// receiver-supplied survives into the refusal.
func TestNormalizeStatusRecord(t *testing.T) {
	const prefix = `{"schema":"urn:relux:task-board:session-launch-status","schema_version":"1.0.0","type":"refusal",`
	t.Run("valid", func(t *testing.T) {
		for _, tc := range []struct {
			name, body, code, detail string
			exit                     int
		}{
			{"d0-empty-details", prefix + `"code":"session_host_unavailable","message":"Session host is unavailable.","details":{}}`,
				"session_host_unavailable", "Session host is unavailable.", 1},
			{"d1-full", prefix + `"code":"launch_plan_invalid","message":"Invalid launch plan.","details":{"field":"process","reason":"shape_invalid"}}`,
				"launch_plan_invalid", "Invalid launch plan. field=process reason=shape_invalid", 2},
			{"d1-reason-only", prefix + `"code":"secret_policy_violation","message":"Secret policy violation.","details":{"reason":"credential_metadata"}}`,
				"secret_policy_violation", "Secret policy violation. reason=credential_metadata", 16},
			{"d2-full", prefix + `"code":"policy_refused","message":"Policy refused.","details":{"field":"policy","reason":"selector_drift"}}`,
				"policy_refused", "Policy refused. field=policy reason=selector_drift", 16},
			{"retained-resume-code", prefix + `"code":"session_resume_not_found","message":"Session resume refused.","details":{}}`,
				"session_resume_not_found", "Session resume refused.", 1},
			{"member-order-irrelevant", `{"type":"refusal","details":{},"message":"Invalid invocation.","code":"usage","schema_version":"1.0.0","schema":"urn:relux:task-board:session-launch-status"}`,
				"usage", "Invalid invocation.", 2},
		} {
			t.Run(tc.name, func(t *testing.T) {
				refusal, err := hosted.NormalizeStatusRecord([]byte(tc.body))
				if err != nil {
					t.Fatalf("valid envelope refused: %v", err)
				}
				if refusal.Code != tc.code || refusal.Detail() != tc.detail {
					t.Fatalf("refusal = %v/%q, want %s/%q", refusal.Code, refusal.Detail(), tc.code, tc.detail)
				}
				if exit, _ := hosted.StatusExitForCode(refusal.Code); exit != tc.exit {
					t.Fatalf("exit = %d, want %d", exit, tc.exit)
				}
			})
		}
	})
	t.Run("invalid", func(t *testing.T) {
		for _, tc := range []struct{ name, body string }{
			{"empty", ``},
			{"not-json", `{oops`},
			{"not-object", `[]`},
			{"null", `null`},
			{"trailing-content", prefix + `"code":"usage","message":"Invalid invocation.","details":{}} trailing`},
			{"duplicate-key", `{"schema":"urn:relux:task-board:session-launch-status","schema_version":"1.0.0","type":"refusal","code":"usage","message":"Invalid invocation.","details":{},"code":"usage"}`},
			{"missing-details", prefix[:len(prefix)-1] + `}`},
			{"missing-message", `{"schema":"urn:relux:task-board:session-launch-status","schema_version":"1.0.0","type":"refusal","code":"usage","details":{}}`},
			{"extra-member", prefix + `"code":"usage","message":"Invalid invocation.","details":{},"future":1}`},
			{"wrong-schema", `{"schema":"urn:relux:task-board:other","schema_version":"1.0.0","type":"refusal","code":"usage","message":"Invalid invocation.","details":{}}`},
			{"wrong-version", `{"schema":"urn:relux:task-board:session-launch-status","schema_version":"2.0.0","type":"refusal","code":"usage","message":"Invalid invocation.","details":{}}`},
			{"wrong-type", `{"schema":"urn:relux:task-board:session-launch-status","schema_version":"1.0.0","type":"progress","code":"usage","message":"Invalid invocation.","details":{}}`},
			{"unknown-code", prefix + `"code":"session_host_future_x","message":"CANARY-reflection","details":{}}`},
			{"empty-code", prefix + `"code":"","message":"Invalid invocation.","details":{}}`},
			{"wrong-message", prefix + `"code":"launch_plan_invalid","message":"CANARY-reflection","details":{}}`},
			{"message-case", prefix + `"code":"usage","message":"invalid invocation.","details":{}}`},
			{"details-not-object", prefix + `"code":"usage","message":"Invalid invocation.","details":[]}`},
			{"details-null", prefix + `"code":"usage","message":"Invalid invocation.","details":null}`},
			{"d0-with-field", prefix + `"code":"usage","message":"Invalid invocation.","details":{"field":"process"}}`},
			{"d0-with-reason", prefix + `"code":"session_host_unavailable","message":"Session host is unavailable.","details":{"reason":"shape_invalid"}}`},
			{"d1-bad-field", prefix + `"code":"launch_plan_invalid","message":"Invalid launch plan.","details":{"field":"CANARY-reflection"}}`},
			{"d1-bad-reason", prefix + `"code":"launch_plan_invalid","message":"Invalid launch plan.","details":{"reason":"invalid"}}`},
			{"d1-unknown-key", prefix + `"code":"launch_plan_invalid","message":"Invalid launch plan.","details":{"request":"process"}}`},
			{"d2-bad-field", prefix + `"code":"policy_refused","message":"Policy refused.","details":{"field":"CANARY-reflection"}}`},
			{"d2-bad-reason", prefix + `"code":"policy_refused","message":"Policy refused.","details":{"reason":"shape_invalid"}}`},
			{"detail-value-null", prefix + `"code":"launch_plan_invalid","message":"Invalid launch plan.","details":{"field":null}}`},
			{"detail-value-number", prefix + `"code":"launch_plan_invalid","message":"Invalid launch plan.","details":{"reason":1}}`},
			{"code-not-string", prefix + `"code":2,"message":"Invalid invocation.","details":{}}`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if _, err := hosted.NormalizeStatusRecord([]byte(tc.body)); err == nil {
					t.Fatalf("non-contract record %q classified", tc.body)
				}
			})
		}
	})
}
