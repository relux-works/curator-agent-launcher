package managerschema

import (
	"encoding/json"
	"strings"
	"testing"
)

// A readable session without provider activity permits null last_activity_at.
// The other five evidence fields may be null only in the pre-start override.
func TestLivenessNullableEvidence(t *testing.T) {
	data, err := Documents.ReadFile("vectors/liveness.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct{ Document map[string]any }
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"last_activity_at", "session", "generation", "operation_id", "started_at", "last_heartbeat_at"} {
		t.Run(field, func(t *testing.T) {
			row := make(map[string]any)
			for key, value := range vectors[0].Document {
				row[key] = value
			}
			row[field] = nil
			body, _ := json.Marshal(row)
			want := field == "last_activity_at"
			if err := Validate("liveness.schema.json", body); (err == nil) != want {
				t.Fatalf("nullable %s valid=%t: %v", field, want, err)
			}
		})
	}
	body, _ := json.Marshal(vectors[1].Document)
	if err := Validate("liveness.schema.json", body); err != nil {
		t.Fatal("prestart override refused", err)
	}
}

// The already-published label grammar accepts all punctuation including '='
// and its exact length bound, and refuses empty, oversized and foreign values.
// Launch, resume scope and both session-record projections use the same domain.
func TestPublishedLabelGrammar(t *testing.T) {
	for _, schema := range []string{"launch", "resume", "session-record"} {
		data, err := Documents.ReadFile("vectors/" + schema + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var vectors []struct{ Document map[string]any }
		if err := json.Unmarshal(data, &vectors); err != nil {
			t.Fatal(err)
		}
		count := 1
		if schema == "session-record" {
			count = 2
		}
		for i := range count {
			for _, tc := range []struct {
				name, value string
				valid       bool
			}{
				{"punctuation", "Az09._:/@+=-", true},
				{"bound", strings.Repeat("=", 128), true},
				{"empty", "", false},
				{"overflow", strings.Repeat("=", 129), false},
				{"foreign", "bad value", false},
			} {
				t.Run(schema+"/"+[]string{"readable", "unreadable"}[i]+"/"+tc.name, func(t *testing.T) {
					row := vectors[i].Document
					target := row
					if schema == "resume" {
						target = row["scope"].(map[string]any)
					}
					target["labels"] = map[string]string{"sample": tc.value}
					body, _ := json.Marshal(row)
					if err := Validate(schema+".schema.json", body); (err == nil) != tc.valid {
						t.Fatalf("label %s valid=%t: %v", tc.name, tc.valid, err)
					}
				})
			}
		}
	}
}
