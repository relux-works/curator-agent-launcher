package memberwire

const (
	AssignmentLimit          = 64 * 1024
	AttemptLimit             = 16 * 1024
	OperationIntentLimit     = 16 * 1024
	OperationCompletionLimit = 16 * 1024
)

// DecodeAssignment checks a closed untrusted binding, including effects and
// deadlines. It does not admit roles, compare manifests or bind actual inputs.
func DecodeAssignment(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, AssignmentLimit, assignmentValid)
}

// DecodeAttempt preserves shaped ordinal/predecessor data without selecting a
// latest attempt, checking history or reserving a new admission.
func DecodeAttempt(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, AttemptLimit, attemptValid)
}

// DecodeOperationIntent checks structure only, without publishing an intent,
// comparing its candidate or establishing a durable operation barrier.
func DecodeOperationIntent(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, OperationIntentLimit, operationIntentValid)
}

// DecodeOperationCompletion checks the published clear/conflict conditional.
// Success is not proof that an intent completed or any interlock was resolved.
func DecodeOperationCompletion(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, OperationCompletionLimit, operationCompletionValid)
}

func assignmentValid(o map[string]any) bool {
	return closed(o, "kind", "store_id", "controller_id", "assignment_id", "member_id", "consumer_id", "role", "producer_member_id", "tuple", "artifacts", "effects", "deadlines") && is(o["kind"], "curator-run member-assignment/1") && id(o["store_id"]) && id(o["controller_id"]) && id(o["assignment_id"]) && id(o["member_id"]) && id(o["consumer_id"]) && is(o["role"], "development", "source_review", "recipient_assessment") && (o["producer_member_id"] == nil || id(o["producer_member_id"])) && reference(o["tuple"]) && artifacts(o["artifacts"]) && effects(o["effects"]) && deadlines(o["deadlines"])
}
func attemptValid(o map[string]any) bool {
	return closed(o, "kind", "store_id", "assignment_id", "assignment_sha256", "ordinal", "invocation_id", "predecessor_invocation_id", "request_sha256") && is(o["kind"], "curator-run member-attempt/1") && id(o["store_id"]) && id(o["assignment_id"]) && digest(o["assignment_sha256"]) && integer(o["ordinal"], 1, 256) && id(o["invocation_id"]) && (o["predecessor_invocation_id"] == nil || id(o["predecessor_invocation_id"])) && digest(o["request_sha256"])
}
func operationIntentValid(o map[string]any) bool {
	return closed(o, "kind", "store_id", "controller_id", "sequence", "operation", "invocation_id", "candidate_sha256") && is(o["kind"], "curator-run member-operation-intent/1") && id(o["store_id"]) && id(o["controller_id"]) && integer(o["sequence"], 1, 4096) && is(o["operation"], "submit", "cancel", "reconcile", "reserve", "ack", "publish") && id(o["invocation_id"]) && digest(o["candidate_sha256"])
}
func operationCompletionValid(o map[string]any) bool {
	if !closed(o, "kind", "store_id", "sequence", "intent", "disposition", "conflict") || !is(o["kind"], "curator-run member-operation-completion/1") || !id(o["store_id"]) || !integer(o["sequence"], 1, 4096) || !reference(o["intent"]) {
		return false
	}
	switch o["disposition"] {
	case "clear":
		return o["conflict"] == nil
	case "conflict":
		return reference(o["conflict"])
	default:
		return false
	}
}
