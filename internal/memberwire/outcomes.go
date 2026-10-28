package memberwire

const (
	ResultLimit      = 64 * 1024
	ResponseLimit    = 16 * 1024
	ReservationLimit = 16 * 1024
	AckLimit         = 16 * 1024
	ConflictLimit    = 16 * 1024
)

// DecodeResult checks untrusted structure, including nonterminal native states
// and independent output/cleanup axes. It imports nothing and proves no result.
func DecodeResult(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, ResultLimit, resultValid)
}

// DecodeResponse checks the complete nested replay and schema conditionals,
// not correlation, replay truth or permission to consume a referenced result.
func DecodeResponse(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, ResponseLimit, responseValid)
}

// DecodeReservation checks data only; it reserves no effect or assignment.
func DecodeReservation(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, ReservationLimit, reservationValid)
}

// DecodeAck checks a receipt reference, not receipt truth or effect completion.
func DecodeAck(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, AckLimit, ackValid)
}

// DecodeConflict accepts structurally equal hashes too. It neither establishes
// an actual conflict nor freezes, replaces or chooses any retained result.
func DecodeConflict(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, ConflictLimit, conflictValid)
}

func outcomeArgvValid(v any) bool {
	if v == nil {
		return true
	}
	a, ok := v.([]any)
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
func streamValid(v any) bool {
	o := object(v)
	return closed(o, "length", "sha256", "truncated") && integer(o["length"], 0, 1048576) && digest(o["sha256"]) && lifecycleBool(o["truncated"])
}
func resultValid(o map[string]any) bool {
	if !closed(o, "kind", "store_id", "identity", "request_sha256", "tuple_sha256", "writer", "scope", "actual_tuple", "argv", "native", "cleanup", "cancel", "worker_output", "outputs", "stdout", "stderr", "error", "final_generation") || !is(o["kind"], "curator-run member-result/1") || !id(o["store_id"]) || !identity(o["identity"]) || !digest(o["request_sha256"]) || !digest(o["tuple_sha256"]) || !writerValid(o["writer"]) || !(o["scope"] == nil || scopeValid(o["scope"])) || !(o["actual_tuple"] == nil || reference(o["actual_tuple"])) || !outcomeArgvValid(o["argv"]) || !nativeValid(o["native"]) || !cleanupValid(o["cleanup"]) || !(o["cancel"] == nil || reference(o["cancel"])) || !is(o["worker_output"], "complete", "missing", "partial", "invalid", "overflow") || !streamValid(o["stdout"]) || !streamValid(o["stderr"]) || !(o["error"] == nil || lifecycleErrorValid(o["error"])) || !integer(o["final_generation"], 1, 256) {
		return false
	}
	a, ok := o["outputs"].([]any)
	if !ok || len(a) > 128 {
		return false
	}
	for _, output := range a {
		if !reference(output) {
			return false
		}
	}
	return true
}

// Replay has no separate raw intake or invented byte ceiling. Every member is
// validated inside the bounded response; enum/hash relationships remain data.
func replayValid(v any) bool {
	o := object(v)
	return closed(o, "kind", "key", "classification", "original_request_sha256", "current_request_sha256", "original_result_sha256") && is(o["kind"], "curator-run member-replay/1") && is(o["key"], "invocation", "cancel", "effect", "none") && is(o["classification"], "new", "identical", "conflict", "none") && (o["original_request_sha256"] == nil || digest(o["original_request_sha256"])) && (o["current_request_sha256"] == nil || digest(o["current_request_sha256"])) && (o["original_result_sha256"] == nil || digest(o["original_result_sha256"]))
}
func responseValid(o map[string]any) bool {
	if !closed(o, "kind", "request_id", "request_sha256", "operation", "outcome", "snapshot", "result", "reservation", "ack", "error", "replay") || !is(o["kind"], "curator-run member-response/1") || !(o["request_id"] == nil || id(o["request_id"])) || !(o["request_sha256"] == nil || digest(o["request_sha256"])) || !(o["operation"] == nil || is(o["operation"], "submit", "inspect", "cancel", "reconcile", "consume")) || !(o["error"] == nil || lifecycleErrorValid(o["error"])) || !replayValid(o["replay"]) {
		return false
	}
	for _, key := range []string{"snapshot", "result", "reservation", "ack"} {
		if !(o[key] == nil || reference(o[key])) {
			return false
		}
	}
	// Only the published allOf/if/then properties constrain these outcomes.
	// In particular refused/conflict leave snapshot nullable, consumption ack.
	switch o["outcome"] {
	case "refused", "conflict":
		return o["error"] != nil && o["result"] == nil && o["reservation"] == nil && o["ack"] == nil
	case "snapshot":
		return o["snapshot"] != nil && o["result"] == nil && o["error"] == nil && o["reservation"] == nil && o["ack"] == nil
	case "result":
		return o["result"] != nil && o["snapshot"] != nil && o["error"] == nil && o["reservation"] == nil && o["ack"] == nil
	case "consumption":
		return o["reservation"] != nil && o["result"] != nil && o["snapshot"] != nil && o["error"] == nil
	default:
		return false
	}
}
func reservationValid(o map[string]any) bool {
	return closed(o, "kind", "store_id", "assignment_id", "invocation_id", "consumer_id", "effect_id", "result_sha256", "assignment_sha256", "attempt_ordinal") && is(o["kind"], "curator-run member-reservation/1") && id(o["store_id"]) && id(o["assignment_id"]) && id(o["invocation_id"]) && id(o["consumer_id"]) && id(o["effect_id"]) && digest(o["result_sha256"]) && digest(o["assignment_sha256"]) && integer(o["attempt_ordinal"], 1, 256)
}
func ackValid(o map[string]any) bool {
	return closed(o, "kind", "reservation_sha256", "receipt") && is(o["kind"], "curator-run member-ack/1") && digest(o["reservation_sha256"]) && reference(o["receipt"])
}
func conflictValid(o map[string]any) bool {
	return closed(o, "kind", "store_id", "invocation_id", "category", "original_sha256", "conflicting_sha256", "writer") && is(o["kind"], "curator-run member-conflict/1") && id(o["store_id"]) && id(o["invocation_id"]) && is(o["category"], "request", "result", "cancel", "consumption", "observation", "assignment") && digest(o["original_sha256"]) && digest(o["conflicting_sha256"]) && writerValid(o["writer"])
}
