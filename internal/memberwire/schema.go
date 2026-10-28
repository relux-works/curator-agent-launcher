package memberwire

// These explicit predicates mirror only request/authorization and their recursive
// definitions in records.schema.json. They are not a general schema evaluator.
func closed(o map[string]any, keys ...string) bool {
	if len(o) != len(keys) {
		return false
	}
	for _, k := range keys {
		if _, ok := o[k]; !ok {
			return false
		}
	}
	return true
}
func object(v any) map[string]any { o, _ := v.(map[string]any); return o }
func is(v any, choices ...string) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	for _, c := range choices {
		if s == c {
			return true
		}
	}
	return false
}
func integer(v any, low, high uint64) bool { n, ok := v.(uint64); return ok && n >= low && n <= high }
func lowerHex(v any, length int) bool {
	s, ok := v.(string)
	if !ok || len(s) != length {
		return false
	}
	for i := range len(s) {
		if !(s[i] >= 'a' && s[i] <= 'f' || s[i] >= '0' && s[i] <= '9') {
			return false
		}
	}
	return true
}
func id(v any) bool     { return lowerHex(v, 32) }
func digest(v any) bool { return lowerHex(v, 64) }
func name(v any) bool {
	s, ok := v.(string)
	if !ok || len(s) < 1 || len(s) > 64 {
		return false
	}
	for i := range len(s) {
		c := s[i]
		alpha := c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
		if !alpha && (i == 0 || c != '.' && c != '_' && c != '-') {
			return false
		}
	}
	return true
}
func reference(v any) bool {
	o := object(v)
	return closed(o, "name", "length", "sha256") && name(o["name"]) && integer(o["length"], 0, 4194304) && digest(o["sha256"])
}
func identity(v any) bool {
	o := object(v)
	if !closed(o, "controller_id", "assignment_id", "invocation_id", "member_id", "consumer_id", "role", "producer_member_id", "predecessor_invocation_id") {
		return false
	}
	for _, k := range []string{"controller_id", "assignment_id", "invocation_id", "member_id", "consumer_id"} {
		if !id(o[k]) {
			return false
		}
	}
	return is(o["role"], "development", "source_review", "recipient_assessment") && (o["producer_member_id"] == nil || id(o["producer_member_id"])) && (o["predecessor_invocation_id"] == nil || id(o["predecessor_invocation_id"]))
}
func artifacts(v any) bool {
	a, ok := v.([]any)
	if !ok || len(a) < 1 || len(a) > 128 {
		return false
	}
	for _, v := range a {
		o := object(v)
		if !closed(o, "object", "role") || !reference(o["object"]) || !is(o["role"], "instruction", "source", "tuple", "owner_input", "profile_input", "receipt", "assignment") {
			return false
		}
	}
	return true
}
func names(v any, maximum int) bool {
	a, ok := v.([]any)
	if !ok || len(a) > maximum {
		return false
	}
	seen := make(map[string]bool, len(a))
	for _, v := range a {
		if !name(v) {
			return false
		}
		s := v.(string)
		if seen[s] {
			return false
		}
		seen[s] = true
	}
	return true
}
func effects(v any) bool {
	o := object(v)
	if !closed(o, "tools", "read_objects", "output_names", "network", "provider_binding_sha256", "max_processes", "max_provider_requests", "max_provider_tokens") || !names(o["tools"], 32) || !names(o["read_objects"], 128) || !names(o["output_names"], 128) || !integer(o["max_processes"], 1, 64) {
		return false
	}
	switch o["network"] {
	case "disabled":
		return o["provider_binding_sha256"] == nil && integer(o["max_provider_requests"], 0, 0) && integer(o["max_provider_tokens"], 0, 0)
	case "admitted_provider":
		return digest(o["provider_binding_sha256"]) && integer(o["max_provider_requests"], 1, 64) && integer(o["max_provider_tokens"], 1, 65536)
	default:
		return false
	}
}
func deadlines(v any) bool {
	o := object(v)
	return closed(o, "startup_ms", "runtime_ms", "term_ms", "kill_observation_ms", "result_ms", "state_budget_bytes") && integer(o["startup_ms"], 1, 60000) && integer(o["runtime_ms"], 1, 3600000) && integer(o["term_ms"], 1, 6000) && integer(o["kill_observation_ms"], 1, 6000) && integer(o["result_ms"], 1, 6000) && integer(o["state_budget_bytes"], 1, 67108864)
}
func requestValid(o map[string]any) bool {
	if !is(o["kind"], "curator-run member-request/1") || !id(o["request_id"]) {
		return false
	}
	switch o["operation"] {
	case "submit":
		return closed(o, "kind", "request_id", "operation", "identity", "tuple", "artifacts", "effects", "deadlines", "assignment") && identity(o["identity"]) && reference(o["tuple"]) && artifacts(o["artifacts"]) && effects(o["effects"]) && deadlines(o["deadlines"]) && reference(o["assignment"])
	case "inspect", "reconcile":
		return closed(o, "kind", "request_id", "operation", "invocation_id") && id(o["invocation_id"])
	case "cancel":
		return closed(o, "kind", "request_id", "operation", "invocation_id", "cancel_id", "reason") && id(o["invocation_id"]) && id(o["cancel_id"]) && is(o["reason"], "controller_request", "deadline")
	case "consume":
		keys := []string{"kind", "request_id", "operation", "action", "invocation_id", "consumer_id", "effect_id", "result_sha256"}
		if is(o["action"], "ack") {
			keys = append(keys, "receipt")
			if !reference(o["receipt"]) {
				return false
			}
		} else if !is(o["action"], "reserve") {
			return false
		}
		return closed(o, keys...) && id(o["invocation_id"]) && id(o["consumer_id"]) && id(o["effect_id"]) && digest(o["result_sha256"])
	default:
		return false
	}
}
func authorizationValid(o map[string]any) bool {
	return closed(o, "kind", "controller_id", "store_id", "request_length", "request_sha256", "expires_at_ms") && is(o["kind"], "curator-run member-authorization/1") && id(o["controller_id"]) && id(o["store_id"]) && integer(o["request_length"], 1, 65536) && digest(o["request_sha256"]) && integer(o["expires_at_ms"], 1, 9007199254740991)
}
