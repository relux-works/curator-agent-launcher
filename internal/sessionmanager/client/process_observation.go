package client

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
)

// ProcessObservationCapability is the describe token of the observation-v2
// contract. It admits the session-record 1.3.0 members process and
// termination and the outcome 1.1.0 member no_start.
const ProcessObservationCapability = "process-observation/1"

// Admission is what a caller learned outside the response being decoded. The
// zero value is the legacy rule: no 1.3.0 or 1.1.0 member is admitted and no
// frozen owner is compared.
type Admission struct {
	// ProcessObservation is ProcessObservation(describe) for the manager that
	// produced the response. Without it, process, termination and no_start are
	// a protocol error even though the pinned schemas validate them.
	ProcessObservation bool
	// FrozenGeneration, when nonzero, is the generation frozen for the session
	// before submission. A direct session status must then name this manager,
	// the requested handle and exactly this generation; anything else is
	// session_generation_mismatch rather than a protocol error.
	FrozenGeneration int64
}

// ProcessObservation reports whether a describe advertises the token. The
// token counts only together with status/1, and any other spelling confers
// nothing.
func ProcessObservation(d Describe) bool {
	return slices.Contains(d.Capabilities, "status/1") && slices.Contains(d.Capabilities, ProcessObservationCapability)
}

// observationV2Members are the members that only a manager advertising
// process-observation/1 may send.
var observationV2Members = []string{"process", "termination", "no_start"}

// carriesObservationV2Members reports whether a schema-valid response body
// contains a negotiated member at the top level (an enveloped session or an
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

// SessionRecordV1 projects an already validated session record to 1.2.0 by
// removing exactly process and termination. Every other member keeps its
// position and its bytes, and a record carrying neither is returned unchanged,
// byte for byte. It never validates and must not be used to make an invalid
// record pass.
func SessionRecordV1(record []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(record))
	if open, err := decoder.Token(); err != nil || open != json.Delim('{') {
		return nil, protocolError()
	}
	var projected bytes.Buffer
	projected.WriteByte('{')
	removed := false
	for decoder.More() {
		token, err := decoder.Token()
		name, ok := token.(string)
		var value json.RawMessage
		if err != nil || !ok || decoder.Decode(&value) != nil {
			return nil, protocolError()
		}
		if name == "process" || name == "termination" {
			removed = true
			continue
		}
		key, err := json.Marshal(name)
		if err != nil {
			return nil, protocolError()
		}
		if projected.Len() > 1 {
			projected.WriteByte(',')
		}
		projected.Write(key)
		projected.WriteByte(':')
		projected.Write(value)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, protocolError()
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, protocolError()
	}
	if !removed {
		return record, nil
	}
	projected.WriteByte('}')
	return projected.Bytes(), nil
}
