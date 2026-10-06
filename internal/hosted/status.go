// Receiving boundary for fd4 status records: the closed
// urn:relux:task-board:hosted-diagnostics 1.0.0 registry (contract
// Appendix V-ERR) and its normalizer.
//
// This registry is RECEIVING-ONLY. The launcher emits only its own SPEC
// §6 diagnostics (internal/diagnostics); it never emits most of these
// codes itself. But every fd4 refusal record MUST normalize against the
// complete table: the code must be a member, the message must equal the
// row's literal constant, and details must match the row's vocabulary.
// Anything else — unknown code, mismatched message, bad detail,
// malformed envelope — becomes the fixed session_host_protocol_error/6
// with empty details. Receiver bytes are never reflected.
package hosted

import (
	"slices"

	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

// Closed fd4 refusal envelope identity (§4).
const (
	StatusSchema       = "urn:relux:task-board:session-launch-status"
	StatusSchemaVer    = "1.0.0"
	StatusTypeRefusal  = "refusal"
	StatusEnvelopeSize = 6 // schema, schema_version, type, code, message, details
)

// StatusProtocolErrorCode is the synthesized normalization outcome. It is
// owned here, not by the SPEC §6 launcher set: the launcher never emits
// it except at this receiving boundary.
const StatusProtocolErrorCode = "session_host_protocol_error"

// statusDetails is the permitted detail vocabulary of one registry row:
// D0 allows exactly {}, D1 and D2 allow any subset of their field/reason
// tokens. Values are always case-sensitive strings, never null.
type statusDetails uint8

const (
	statusD0 statusDetails = iota
	statusD1
	statusD2
)

var statusD1Fields = map[string]bool{
	"producer": true, "fragment": true, "policy": true, "process": true,
	"resume": true, "reservation": true, "auxiliary": true,
}

var statusD1Reasons = map[string]bool{
	"shape_invalid": true, "source_mismatch": true, "credential_metadata": true,
	"required_env_missing": true, "reservation_invalid": true,
}

var statusD2Fields = map[string]bool{
	"fragment": true, "policy": true, "managed_home": true,
}

var statusD2Reasons = map[string]bool{
	"environment_drift": true, "selector_drift": true, "literal_drift": true,
}

// statusRow is one Appendix V-ERR table row: every listed code receives
// exactly this exit, message, and detail vocabulary. Row order mirrors
// the contract table.
type statusRow struct {
	codes   []string
	exit    int
	message string
	details statusDetails
}

// statusRows transcribes Appendix V-ERR verbatim: code spellings, numeric
// exits, literal constant messages (exact bytes, final period), and the
// D0/D1/D2 detail class per row.
var statusRows = []statusRow{
	{[]string{"usage"}, 2, "Invalid invocation.", statusD0},
	{[]string{"launch_plan_invalid"}, 2, "Invalid launch plan.", statusD1},
	{[]string{"session_resume_invalid"}, 2, "Invalid resume selector.", statusD0},
	{[]string{"host_configuration_conflict"}, 2, "Conflicting host configuration.", statusD0},
	{[]string{"session_host_missing"}, 1, "Session host is missing.", statusD0},
	{[]string{"session_host_unavailable"}, 1, "Session host is unavailable.", statusD0},
	{[]string{"session_host_protocol_error"}, 6, "Invalid session host response.", statusD0},
	{[]string{"session_host_protocol_unsupported"}, 6, "Session host protocol is unsupported.", statusD0},
	{[]string{"session_host_capability_missing"}, 6, "Required host capability is missing.", statusD0},
	{[]string{
		"session_host_provider_unsupported", "session_host_scope_unsupported",
		"session_host_execution_profile_unsupported", "session_host_terminal_required",
		"session_host_stdin_unsupported",
	}, 6, "Hosted launch requirement is unsupported.", statusD0},
	{[]string{"secret_policy_violation"}, 16, "Secret policy violation.", statusD1},
	{[]string{"policy_refused"}, 16, "Policy refused.", statusD2},
	{[]string{"network_scope_unsupported", "network_profile_drift"}, 16, "Network policy refused.", statusD0},
	{[]string{"session_host_default_not_ready"}, 16, "Hosted default is not ready.", statusD0},
	{[]string{
		"session_resume_not_found", "session_resume_ambiguous", "session_resume_unreadable",
		"session_resume_location_mismatch", "session_resume_identity_mismatch",
	}, 1, "Session resume refused.", statusD0},
	{[]string{"session_provider_conflict", "session_adoption_conflict"}, 1, "Session ownership conflict.", statusD0},
	{[]string{"session_liveness_unknown"}, 1, "Session liveness is unknown.", statusD0},
	{[]string{"session_live_detached"}, 1, "Live session is detached.", statusD0},
	{[]string{
		"resolve_invocation_failed", "resolve_environment_unknown", "resolve_profile_unknown",
		"resolve_repair_failed", "resolve_lock_unavailable", "resolve_fragment_invalid",
		"defaults_config_invalid", "defaults_unresolvable",
	}, 1, "Profile resolution refused.", statusD0},
	{[]string{
		"plan_refused", "plan_provider_limited", "env_unsupported",
		"permission_policy_unsupported", "permission_mode_tracked_unsupported",
		"permission_mode_unsupported", "exec_provider_missing",
		"mcp_layer_missing", "mcp_layer_unreadable",
		"sysprompt_channel_unavailable", "sysprompt_file_unreadable",
	}, 1, "Provider launch refused.", statusD0},
	{[]string{
		"claude_goal_unavailable", "claude_goal_unsupported_version",
		"claude_goal_workspace_untrusted", "claude_goal_preflight_failed",
		"claude_session_resume_failed", "claude_launch_args_unsupported",
		"claude_session_adoption_invalid",
	}, 1, "Claude capability refused.", statusD0},
	{[]string{
		"claude_context_base_not_forkable", "claude_context_cwd_slug_unverified",
		"claude_context_fork_plan_invalid", "claude_context_fork_source_mutated",
		"claude_context_fork_unverified", "claude_context_materialization_failed",
		"claude_context_transcript_format_drift", "claude_context_writer_lease_unavailable",
	}, 1, "Claude context refused.", statusD0},
	{[]string{
		"context_fork_preflight_failed", "context_run_handle_unavailable",
		"context_run_transcript_unavailable",
	}, 1, "Context preparation refused.", statusD0},
	{[]string{"owner_activation_in_progress"}, 1, "Owner activation is in progress.", statusD0},
	{[]string{"session_reservation_stale", "session_generation_stale"}, 1, "Session observation is stale.", statusD0},
	{[]string{"session_takeover_not_applicable", "session_takeover_observation_changed"}, 1, "Session takeover refused.", statusD0},
	{[]string{"goal_reflection_skew"}, 1, "Goal reflection is incompatible.", statusD0},
	{[]string{
		"codex_context_base_not_forkable", "codex_context_capability_unavailable",
		"codex_context_fork_plan_invalid",
	}, 1, "Codex context refused.", statusD0},
	{[]string{
		"codex_thread_adoption_invalid", "codex_thread_not_found",
		"codex_thread_resume_failed", "codex_goal_budget_required",
	}, 1, "Codex capability refused.", statusD0},
	{[]string{
		"context_contract_invalid", "context_binding_invalid", "context_lineage_invalid",
		"context_not_found", "context_already_exists", "context_revision_conflict",
		"context_state_invalid", "context_not_ready", "context_stale",
		"context_publication_denied", "context_secret_class_denied",
		"context_provider_mismatch", "context_payload_capability_required",
		"context_materialization_divergent", "context_registry_unsupported",
		"context_persistence_failed", "context_compacted", "context_integrity_unverified",
		"context_classification_failed", "context_builder_allowlist_denied",
		"context_builder_confinement_unavailable", "context_builder_trace_unavailable",
		"context_builder_tool_isolation_unavailable", "context_assay_isolation_unavailable",
		"context_assay_tool_use_detected", "context_payload_vanished",
		"context_payload_unverified", "context_provider_incompatible",
		"context_not_materialized", "context_pointer_invalid",
		"context_assertion_unverified", "context_correctness_failed",
		"context_contaminated", "context_secret_detected",
		"context_validation_unavailable", "context_validation_budget_exceeded",
	}, 1, "Context authorization refused.", statusD0},
	{[]string{
		"goal_producer_clause_unsupported", "goal_contract_invalid",
		"goal_role_incompatible", "goal_scope_invalid", "goal_scope_empty",
		"goal_scope_blocked_only", "goal_dependency_closure_invalid",
		"goal_objective_too_long", "goal_provider_condition_too_long",
		"goal_revision_required", "goal_revision_conflict", "goal_mutation_restricted",
		"goal_actor_unauthorized", "goal_activation_failed", "goal_budget_invalid",
		"goal_manager_protocol_skew", "spawned_run_goal_plane_disabled",
		"goal_owner_invalid", "goal_not_found", "goal_persistence_failed",
		"goal_native_reflection_required",
	}, 1, "Goal operation refused.", statusD0},
}

// statusEntry is the normalized form of one recognized code.
type statusEntry struct {
	exit    int
	message string
	details statusDetails
}

// statusRegistry indexes every V-ERR code to its row. It is built once;
// duplicate codes across rows would be a transcription bug and fail
// closed at lookup time by construction (last wins is unreachable: the
// registry test pins the exact member count).
var statusRegistry = func() map[string]statusEntry {
	m := make(map[string]statusEntry, 160)
	for _, row := range statusRows {
		for _, code := range row.codes {
			m[code] = statusEntry{exit: row.exit, message: row.message, details: row.details}
		}
	}
	return m
}()

// StatusExitForCode reports the registry exit for a recognized code. It
// reports false for unknown codes, which normalize to protocol_error.
func StatusExitForCode(code string) (int, bool) {
	entry, ok := statusRegistry[code]
	if !ok {
		return 0, false
	}
	return entry.exit, true
}

// StatusRegistrySize reports the member count: the completeness test pins
// it to the Appendix V-ERR enumeration, so a dropped or duplicated code
// fails loudly.
func StatusRegistrySize() int { return len(statusRegistry) }

// StatusRegistryCodes lists every member code in sorted order for the
// completeness test's per-row shape checks.
func StatusRegistryCodes() []string {
	out := make([]string, 0, len(statusRegistry))
	for code := range statusRegistry {
		out = append(out, code)
	}
	slices.Sort(out)
	return out
}

// StatusRegistryRow reports the exit and literal message of one member
// code for the completeness test. It reports false for unknown codes.
func StatusRegistryRow(code string) (int, string, bool) {
	entry, ok := statusRegistry[code]
	if !ok {
		return 0, "", false
	}
	return entry.exit, entry.message, true
}

// NormalizeStatusRecord validates one complete fd4 frame against the
// closed refusal envelope: exactly the six required members, the pinned
// schema/version/type consts, a registry code, the row's literal message,
// and a details object inside the row's vocabulary. The returned refusal
// carries only registry constants and validated tokens. Every violation
// returns errStatusProtocol: the caller emits the fixed protocol_error/6
// with empty details and never reflects receiver bytes.
func NormalizeStatusRecord(body []byte) (StatusRefusal, error) {
	bad := func() (StatusRefusal, error) { return StatusRefusal{}, errStatusProtocol }
	v, err := fragment.ParseJSON(body)
	if err != nil {
		return bad()
	}
	if v.Kind != fragment.KindObject || len(v.Obj) != StatusEnvelopeSize {
		return bad()
	}
	str := func(key string) (string, bool) {
		member, ok := v.Get(key)
		if !ok || member.Kind != fragment.KindString {
			return "", false
		}
		return member.Str, true
	}
	schema, ok := str("schema")
	if !ok || schema != StatusSchema {
		return bad()
	}
	version, ok := str("schema_version")
	if !ok || version != StatusSchemaVer {
		return bad()
	}
	typ, ok := str("type")
	if !ok || typ != StatusTypeRefusal {
		return bad()
	}
	// The envelope carries exactly six members: unknown members fail
	// closed rather than riding along for forward compatibility.
	for _, m := range v.Obj {
		switch m.Key {
		case "schema", "schema_version", "type", "code", "message", "details":
		default:
			return bad()
		}
	}
	code, ok := str("code")
	if !ok {
		return bad()
	}
	entry, ok := statusRegistry[code]
	if !ok {
		return bad()
	}
	message, ok := str("message")
	if !ok || message != entry.message {
		return bad()
	}
	details, ok := v.Get("details")
	if !ok {
		return bad()
	}
	field, reason, err := checkStatusDetails(entry.details, details)
	if err != nil {
		return bad()
	}
	return StatusRefusal{Code: code, Message: entry.message, Field: field, Reason: reason}, nil
}

// checkStatusDetails validates the details member against the row's
// class: D0 requires exactly {}, D1/D2 allow any subset of their
// closed field/reason tokens. Keys and values are case-sensitive
// strings; null, wrong types, unknown keys, and foreign tokens refuse.
func checkStatusDetails(class statusDetails, v fragment.Value) (string, string, error) {
	if v.Kind != fragment.KindObject {
		return "", "", errStatusProtocol
	}
	if len(v.Obj) > 2 {
		return "", "", errStatusProtocol
	}
	var field, reason string
	var haveField, haveReason bool
	for _, m := range v.Obj {
		if m.Value.Kind != fragment.KindString {
			return "", "", errStatusProtocol
		}
		switch m.Key {
		case "field":
			field, haveField = m.Value.Str, true
		case "reason":
			reason, haveReason = m.Value.Str, true
		default:
			return "", "", errStatusProtocol
		}
	}
	switch class {
	case statusD0:
		if haveField || haveReason {
			return "", "", errStatusProtocol
		}
	case statusD1:
		if haveField && !statusD1Fields[field] {
			return "", "", errStatusProtocol
		}
		if haveReason && !statusD1Reasons[reason] {
			return "", "", errStatusProtocol
		}
	case statusD2:
		if haveField && !statusD2Fields[field] {
			return "", "", errStatusProtocol
		}
		if haveReason && !statusD2Reasons[reason] {
			return "", "", errStatusProtocol
		}
	default:
		return "", "", errStatusProtocol
	}
	return field, reason, nil
}

// ProtocolErrorExit is the authoritative exit of the synthesized
// normalization outcome. It equals diagnostics.ExitProtocol by contract
// (pinned); the registry — not the receiver status — owns it.
func ProtocolErrorExit() int {
	entry, ok := statusRegistry[StatusProtocolErrorCode]
	if !ok {
		return diagnostics.ExitProtocol
	}
	return entry.exit
}

// ProtocolErrorMessage is the constant detail of the synthesized
// normalization outcome: empty details, no receiver bytes.
func ProtocolErrorMessage() string {
	entry, ok := statusRegistry[StatusProtocolErrorCode]
	if !ok {
		return "Invalid session host response."
	}
	return entry.message
}
