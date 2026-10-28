package memberwire

const (
	SnapshotLimit    = 16 * 1024
	HeadLimit        = 16 * 1024
	CancelLimit      = 4 * 1024
	ObservationLimit = 64 * 1024
	ScopeProofLimit  = 16 * 1024
)

// DecodeSnapshot checks closed structure and the published phase conditionals.
// It neither validates a transition nor authenticates retained state or evidence.
func DecodeSnapshot(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, SnapshotLimit, snapshotValid)
}

// DecodeHead checks an untrusted pointer shape, not the referenced publication.
func DecodeHead(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, HeadLimit, headValid)
}

// DecodeCancel checks data only. It publishes nothing and cancels no work.
func DecodeCancel(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, CancelLimit, cancelValid)
}

// DecodeObservation preserves untrusted wait/argv data without observing a
// process, resolving references or asserting its source or native truth.
func DecodeObservation(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, ObservationLimit, observationValid)
}

// DecodeScopeProof accepts every boolean combination, including false. A
// structural record is not proof of matching custody, completeness or absence.
func DecodeScopeProof(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, ScopeProofLimit, scopeProofValid)
}

func lifecycleBool(v any) bool { _, ok := v.(bool); return ok }
func scopeValid(v any) bool {
	o := object(v)
	return closed(o, "adapter_id", "adapter_revision", "scope_id", "identity_sha256", "boot_sha256", "namespace_sha256", "root_incarnation_sha256") && name(o["adapter_id"]) && revision(o["adapter_revision"]) && id(o["scope_id"]) && digest(o["identity_sha256"]) && digest(o["boot_sha256"]) && digest(o["namespace_sha256"]) && (o["root_incarnation_sha256"] == nil || digest(o["root_incarnation_sha256"]))
}
func writerValid(v any) bool {
	o := object(v)
	return closed(o, "role", "id", "incarnation_sha256", "supervisor_id") && is(o["role"], "supervisor", "reconciler") && id(o["id"]) && digest(o["incarnation_sha256"]) && id(o["supervisor_id"])
}
func lifecycleSignal(o map[string]any) bool {
	s, ok := o["name"].(string)
	if !ok || len(s) < 1 || len(s) > 32 {
		return false
	}
	for _, c := range s {
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return integer(o["number"], 1, 255) && lifecycleBool(o["core_dumped"])
}
func waitValid(v any) bool {
	o := object(v)
	if !integer(o["raw_status"], 0, 4294967295) {
		return false
	}
	switch o["status"] {
	case "exit":
		return closed(o, "status", "code", "raw_status") && integer(o["code"], 0, 255)
	case "signal":
		return closed(o, "status", "number", "name", "core_dumped", "raw_status") && lifecycleSignal(o)
	default:
		return false
	}
}
func observationValid(o map[string]any) bool {
	if !closed(o, "kind", "invocation_id", "supervisor_id", "scope", "process_incarnation_sha256", "boot_sha256", "observed_monotonic_ms", "wait", "actual_tuple", "argv") || !is(o["kind"], "curator-run member-observation/1") || !id(o["invocation_id"]) || !id(o["supervisor_id"]) || !scopeValid(o["scope"]) || !digest(o["process_incarnation_sha256"]) || !digest(o["boot_sha256"]) || !integer(o["observed_monotonic_ms"], 0, safeInteger) || !waitValid(o["wait"]) || !(o["actual_tuple"] == nil || reference(o["actual_tuple"])) {
		return false
	}
	if o["argv"] == nil {
		return true
	}
	a, ok := o["argv"].([]any)
	if !ok || len(a) > 128 {
		return false
	}
	for _, arg := range a {
		if !textLength(arg, 4096) {
			return false
		}
	}
	return true
}
func scopeProofValid(o map[string]any) bool {
	return closed(o, "kind", "invocation_id", "scope", "observer", "observed_monotonic_ms", "method_sha256", "observation_complete", "scope_matches", "empty") && is(o["kind"], "curator-run member-scope-proof/1") && id(o["invocation_id"]) && scopeValid(o["scope"]) && writerValid(o["observer"]) && integer(o["observed_monotonic_ms"], 0, safeInteger) && digest(o["method_sha256"]) && lifecycleBool(o["observation_complete"]) && lifecycleBool(o["scope_matches"]) && lifecycleBool(o["empty"])
}
func sourceErrorValid(v any) bool {
	o := object(v)
	if !closed(o, "component", "namespace", "code") || !name(o["component"]) || !name(o["namespace"]) {
		return false
	}
	s, ok := o["code"].(string)
	if !ok || len(s) < 1 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == ':' || c == '-') {
			return false
		}
	}
	return true
}
func lifecycleErrorValid(v any) bool {
	o := object(v)
	if !closed(o, "stage", "code", "source", "evidence") || !(o["source"] == nil || sourceErrorValid(o["source"])) || !(o["evidence"] == nil || reference(o["evidence"])) {
		return false
	}
	switch o["stage"] {
	case "bootstrap":
		return is(o["code"], "transport_unavailable", "descriptor_invalid")
	case "parse":
		return is(o["code"], "encoding_invalid", "schema_invalid", "bounds_exceeded")
	case "admission":
		return is(o["code"], "authorization_invalid", "authorization_expired", "input_mismatch", "identity_mismatch", "invocation_conflict", "assignment_conflict")
	case "compatibility":
		return is(o["code"], "compatibility_unpublished", "compatibility_mismatch", "configuration_unknown", "presence_contamination", "history_contamination")
	case "store":
		return is(o["code"], "store_busy", "store_unreadable", "store_full", "store_interlocked")
	case "start":
		return is(o["code"], "spawn_failed")
	case "runtime":
		return is(o["code"], "supervisor_lost")
	case "result":
		return is(o["code"], "result_invalid", "result_overflow", "result_conflict")
	case "cleanup":
		return is(o["code"], "scope_unknown", "cleanup_failed")
	case "consume":
		return is(o["code"], "consumption_conflict", "effect_unknown")
	case "cancel":
		return is(o["code"], "cancel_conflict")
	default:
		return false
	}
}
func nativeValid(v any) bool {
	o := object(v)
	switch o["status"] {
	case "not_started", "running", "unknown":
		return closed(o, "status")
	case "exit":
		return closed(o, "status", "code", "evidence") && integer(o["code"], 0, 255) && reference(o["evidence"])
	case "signal":
		return closed(o, "status", "number", "name", "core_dumped", "evidence") && lifecycleSignal(o) && reference(o["evidence"])
	default:
		return false
	}
}
func startValid(v any) bool {
	o := object(v)
	switch o["status"] {
	case "not_attempted", "attempted_unknown":
		return closed(o, "status")
	case "spawned", "exec_confirmed":
		return closed(o, "status", "process_incarnation_sha256") && digest(o["process_incarnation_sha256"])
	case "spawn_failed":
		return closed(o, "status", "os_error") && sourceErrorValid(o["os_error"])
	default:
		return false
	}
}
func cleanupValid(v any) bool {
	o := object(v)
	switch o["status"] {
	case "not_started", "pending", "unknown":
		return closed(o, "status")
	case "failed":
		return closed(o, "status", "evidence") && (o["evidence"] == nil || reference(o["evidence"]))
	case "confirmed_absent":
		return closed(o, "status", "evidence") && reference(o["evidence"])
	default:
		return false
	}
}
func allocationValid(v any) bool {
	o := object(v)
	switch o["status"] {
	case "not_requested":
		return closed(o, "status")
	case "intent", "known", "unknown":
		return closed(o, "status", "allocation_id") && id(o["allocation_id"])
	default:
		return false
	}
}
func stopValid(v any) bool {
	o := object(v)
	return closed(o, "stop_id", "reason", "boot_sha256", "begun_ms", "term_deadline_ms", "kill_deadline_ms", "term_action_id", "kill_action_id", "stage") && id(o["stop_id"]) && is(o["reason"], "controller_request", "deadline", "controller_lost", "supervisor_signal", "descendants_remaining", "overflow") && digest(o["boot_sha256"]) && integer(o["begun_ms"], 0, safeInteger) && integer(o["term_deadline_ms"], 1, safeInteger) && integer(o["kill_deadline_ms"], 1, safeInteger) && id(o["term_action_id"]) && id(o["kill_action_id"]) && is(o["stage"], "term_intent", "term_observe", "kill_intent", "kill_observe", "complete", "unknown")
}
func cancelValid(o map[string]any) bool {
	return closed(o, "kind", "invocation_id", "cancel_id", "source", "requested_at_ms") && is(o["kind"], "curator-run member-cancel/1") && id(o["invocation_id"]) && id(o["cancel_id"]) && is(o["source"], "controller_request", "deadline", "controller_lost", "supervisor_signal") && integer(o["requested_at_ms"], 1, safeInteger)
}
func headValid(o map[string]any) bool {
	return closed(o, "kind", "invocation_id", "generation", "snapshot") && is(o["kind"], "curator-run member-head/1") && id(o["invocation_id"]) && integer(o["generation"], 1, 256) && reference(o["snapshot"])
}
func snapshotValid(o map[string]any) bool {
	if !closed(o, "kind", "store_id", "identity", "request_sha256", "tuple_sha256", "authorization_sha256", "generation", "phase", "writer", "scope", "boot_sha256", "deadline_monotonic_ms", "start", "native", "cleanup", "result", "error", "allocation", "gate", "stop") || !is(o["kind"], "curator-run member-snapshot/1") || !id(o["store_id"]) || !identity(o["identity"]) || !digest(o["request_sha256"]) || !digest(o["tuple_sha256"]) || !digest(o["authorization_sha256"]) || !integer(o["generation"], 1, 256) || !writerValid(o["writer"]) || !(o["scope"] == nil || scopeValid(o["scope"])) || !digest(o["boot_sha256"]) || !integer(o["deadline_monotonic_ms"], 1, safeInteger) || !startValid(o["start"]) || !nativeValid(o["native"]) || !cleanupValid(o["cleanup"]) || !(o["result"] == nil || reference(o["result"])) || !(o["error"] == nil || lifecycleErrorValid(o["error"])) || !allocationValid(o["allocation"]) || !is(o["gate"], "not_created", "held", "release_intent", "released_or_unknown", "closed") || !(o["stop"] == nil || stopValid(o["stop"])) {
		return false
	}
	start, native, cleanup, allocation := object(o["start"])["status"], object(o["native"])["status"], object(o["cleanup"])["status"], object(o["allocation"])["status"]
	// Only these published allOf/if/then relationships are structural. Unlisted
	// relationships, clock/evidence truth and cross-record transitions are not.
	switch o["phase"] {
	case "admitted":
		return start == "not_attempted" && native == "not_started" && cleanup == "not_started" && o["scope"] == nil && o["result"] == nil && allocation == "not_requested"
	case "allocating", "allocated":
		allocated := o["phase"] == "allocated"
		allocationMatches := allocation == "intent"
		if allocated {
			allocationMatches = allocation == "known"
		}
		return allocationMatches && (o["scope"] != nil) == allocated && start == "not_attempted" && native == "not_started" && cleanup == "pending" && o["result"] == nil && o["stop"] == nil && o["gate"] == "not_created"
	case "start_intent":
		return start == "attempted_unknown" && native == "unknown" && o["scope"] != nil && cleanup == "pending" && o["result"] == nil
	case "spawned":
		return start == "spawned" && native == "unknown" && o["scope"] != nil && cleanup == "pending" && o["result"] == nil
	case "running":
		return start == "exec_confirmed" && native == "running" && o["scope"] != nil && cleanup == "pending" && o["result"] == nil
	case "stopping":
		return o["stop"] != nil && o["result"] == nil
	case "terminal":
		return is(native, "not_started", "exit", "signal") && is(cleanup, "not_started", "confirmed_absent") && o["result"] != nil
	case "uncertain":
		return true
	default:
		return false
	}
}
