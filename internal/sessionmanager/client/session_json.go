package client

import "encoding/json"

// MarshalJSON preserves the unreadable projection's explicit nulls when a
// manager session is surfaced by status. Zero-valued Go scalars are not evidence.
func (s Session) MarshalJSON() ([]byte, error) {
	type plain Session
	b, err := json.Marshal(plain(s))
	if err != nil || s.Readable {
		return b, err
	}
	var row map[string]any
	if err := json.Unmarshal(b, &row); err != nil {
		return nil, err
	}
	for _, key := range []string{"operation_id", "runtime", "generation", "started_at"} {
		row[key] = nil
	}
	return json.Marshal(row)
}
