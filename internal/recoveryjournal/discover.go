package recoveryjournal

import "encoding/json"

// Decode routing only from a complete valid exact record. The record cannot
// redirect discovery to another state base; catalog admission follows this
// decode, and service authentication remains a separate required operation.
func recordedBinding(raw []byte, attempt string, l location) (Binding, error) {
	f, ok := secretFields(raw)
	if !ok || !l.valid() {
		return Binding{}, ErrRead
	}
	text := func(key string) string { var s string; _ = json.Unmarshal(f[key], &s); return s }
	if text("state_base") != l.base {
		return Binding{}, ErrRead
	}
	b, err := Resolve(Inputs{XDGStateHome: l.base, ProfileDigest: text("profile_digest"), ControlSocket: text("control_endpoint")})
	if err != nil {
		return Binding{}, ErrRead
	}
	if _, err := decodeEntry(raw, attempt, b); err != nil {
		return Binding{}, err
	}
	return b, nil
}
