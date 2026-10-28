// Package memberwire decodes untrusted member-protocol bytes and descriptor
// argument syntax. Success establishes structure only, never authority, handle
// custody, admission, compatibility, expiry, or permission to execute anything.
package memberwire

import "crypto/sha256"

const (
	RequestLimit       = 64 * 1024
	AuthorizationLimit = 4 * 1024
)

// Error contains only source-owned static tags. It never includes caller data.
type Error struct{ stage, code string }

func (e *Error) Error() string      { return e.stage + "/" + e.code }
func (e *Error) Stage() string      { return e.stage }
func (e *Error) Code() string       { return e.code }
func parseError(code string) *Error { return &Error{"parse", code} }

// UntrustedRecord is an owned structural snapshot, not an admission capability.
// Numbers in Value are uint64; objects and arrays are map[string]any and []any.
// All listed fields (including explicit nulls) survive without normalization.
type UntrustedRecord struct {
	raw    []byte
	value  map[string]any
	digest [32]byte
}

func (r *UntrustedRecord) Raw() []byte      { return append([]byte(nil), r.raw...) }
func (r *UntrustedRecord) Length() int      { return len(r.raw) }
func (r *UntrustedRecord) SHA256() [32]byte { return r.digest }

// Value returns a deep copy. Mutating it cannot affect this or earlier snapshots.
func (r *UntrustedRecord) Value() map[string]any { return copyValue(r.value).(map[string]any) }
func copyValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			out[k] = copyValue(v)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			out[i] = copyValue(v)
		}
		return out
	default:
		return v
	}
}

// DecodeRequest accepts only the six closed request shapes. It does not compare
// references, role identities, authorization, assignment, or retained state.
func DecodeRequest(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, RequestLimit, requestValid)
}

// DecodeAuthorization checks structure, including the deliberately expired
// source example. No clock, request binding, origin or authentication is checked.
func DecodeAuthorization(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, AuthorizationLimit, authorizationValid)
}
func decode(raw []byte, limit int, valid func(map[string]any) bool) (*UntrustedRecord, *Error) {
	// The size check precedes copying, tokenizing, and field interpretation.
	if len(raw) > limit {
		return nil, parseError("bounds_exceeded")
	}
	owned := append([]byte(nil), raw...)
	value, err := parseObject(owned)
	if err != nil {
		return nil, err
	}
	if !valid(value) {
		return nil, parseError("schema_invalid")
	}
	return &UntrustedRecord{owned, value, sha256.Sum256(owned)}, nil
}
