package recoveryjournal

import (
	"bytes"
	"encoding/json"
	"io"
)

// Entry retains only the exact original recovery envelope. Unlike Prepared,
// reading it never authorizes a register request. Authentication against the
// original service and durable cleanup confirmation are later operations.
type Entry struct {
	envelope  Envelope
	retention *receiptRetention
	raw       []byte
	launch    *string
	outcome   string
}

func (Entry) String() string       { return "<private recovery entry>" }
func (Entry) GoString() string     { return "<private recovery entry>" }
func (e *Entry) AttemptID() string { return e.envelope.AttemptID }

func decodeEntry(raw []byte, attempt string, b Binding) (*Entry, error) {
	fields, ok := secretFields(raw)
	if !ok {
		return nil, ErrRead
	}
	text := func(key string) string { var value string; _ = json.Unmarshal(fields[key], &value); return value }
	if !b.valid() || text("state_base") != b.base || text("profile_digest") != b.profile || text("control_endpoint") != b.socket {
		return nil, ErrRead
	}
	envelope, ok := closedObject(fields["envelope"], "root", "session", "runtime", "attempt_id", "recovery_proof", "registry_id", "boot", "issued_ns", "expires_ns")
	if !ok {
		return nil, ErrRead
	}
	// Require strings, including fields whose zero value would otherwise look
	// valid. Standard decoding alone also accepts case aliases and duplicates.
	for _, value := range envelope {
		var s string
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, &s) != nil {
			return nil, ErrRead
		}
	}
	var e Envelope
	if json.Unmarshal(fields["envelope"], &e) != nil || e.Validate() != nil || e.AttemptID != attempt {
		return nil, ErrRead
	}
	var retention *receiptRetention
	switch text("version") {
	case SecretVersion:
		if text("state") != "prepared" || text("last_outcome") != "not_observed" || !bytes.Equal(bytes.TrimSpace(fields["launch_id"]), []byte("null")) {
			return nil, ErrRead
		}
	case TerminalSecretVersion:
		if text("state") != "ended" {
			return nil, ErrRead
		}
		var valid bool
		retention, valid = decodeRetention(fields["receipt_retention"])
		if !valid {
			return nil, ErrRead
		}
		switch text("last_outcome") {
		case "revoked":
			launch := text("launch_id")
			if !digest(launch) || launch == e.RecoveryProof {
				return nil, ErrRead
			}
		case "cancelled_before_issue":
			if !bytes.Equal(bytes.TrimSpace(fields["launch_id"]), []byte("null")) {
				return nil, ErrRead
			}
		default:
			return nil, ErrRead
		}
	default:
		return nil, ErrRead
	}
	var launch *string
	if !bytes.Equal(bytes.TrimSpace(fields["launch_id"]), []byte("null")) {
		var value string
		_ = json.Unmarshal(fields["launch_id"], &value)
		launch = &value
	}
	return &Entry{envelope: e, retention: retention, raw: append([]byte(nil), raw...), launch: launch, outcome: text("last_outcome")}, nil
}

// Both formats have exact allowlists; the version cannot smuggle an extra
// retention field into secret/1 or omit it from terminal secret/2.
func secretFields(raw []byte) (map[string]json.RawMessage, bool) {
	keys := []string{"version", "envelope", "state_base", "profile_digest", "control_endpoint", "launch_id", "state", "last_outcome"}
	f, ok := closedObject(raw, keys...)
	if ok {
		var version string
		return f, json.Unmarshal(f["version"], &version) == nil && version == SecretVersion
	}
	f, ok = closedObject(raw, append(keys, "receipt_retention")...)
	if !ok {
		return nil, false
	}
	var version string
	return f, json.Unmarshal(f["version"], &version) == nil && version == TerminalSecretVersion
}

// closedObject is an exact decoded-key allowlist. It rejects duplicate keys,
// case aliases, omission, null, trailing input and nested parsing errors.
func closedObject(raw []byte, keys ...string) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 || len(raw) > MaxRecord || !json.Valid(raw) {
		return nil, false
	}
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return nil, false
	}
	fields := make(map[string]json.RawMessage, len(keys))
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || !allowed[key] {
			return nil, false
		}
		if _, exists := fields[key]; exists {
			return nil, false
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, false
		}
		fields[key] = value
	}
	last, err := decoder.Token()
	if err != nil || last != json.Delim('}') || len(fields) != len(keys) {
		return nil, false
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, false
	}
	return fields, true
}
