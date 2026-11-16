package client

import "encoding/json"

// observationV2Members are the session-record 1.3.0 and outcome 1.1.0 members
// that the observation-v2 contract admits only from a manager advertising
// process-observation/1. This client does not negotiate that capability yet,
// so a response carrying any of them is a protocol error, even though the
// pinned schemas validate them. The capability-aware consumer replaces this
// gate; until then the launcher's runtime behaviour stays exactly 1.2.0.
var observationV2Members = []string{"process", "termination", "no_start"}

// carriesObservationV2Members reports whether a schema-valid response body
// contains a deferred member at the top level (an enveloped session or an
// outcome), in an outcome's embedded session, or in any list-page row.
func carriesObservationV2Members(body []byte) bool {
	var top map[string]json.RawMessage
	if json.Unmarshal(body, &top) != nil {
		return true
	}
	if hasAny(top) {
		return true
	}
	if raw, ok := top["session"]; ok {
		var embedded map[string]json.RawMessage
		if json.Unmarshal(raw, &embedded) == nil && hasAny(embedded) {
			return true
		}
	}
	if raw, ok := top["sessions"]; ok {
		var rows []map[string]json.RawMessage
		if json.Unmarshal(raw, &rows) != nil {
			return true
		}
		for _, row := range rows {
			if hasAny(row) {
				return true
			}
		}
	}
	return false
}

func hasAny(object map[string]json.RawMessage) bool {
	for _, name := range observationV2Members {
		if _, ok := object[name]; ok {
			return true
		}
	}
	return false
}
