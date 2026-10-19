package managerschema

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// Every published vector has the required verdict through filename and URN,
// including nullable combinations and valid/invalid cross-field and bound cases.
func TestPublishedVectors(t *testing.T) {
	files, err := Documents.ReadDir("vectors")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		name := file.Name()
		t.Run(name, func(t *testing.T) {
			data, err := Documents.ReadFile("vectors/" + name)
			if err != nil {
				t.Fatal(err)
			}
			var vectors []struct {
				Name     string          `json:"name"`
				Valid    bool            `json:"valid"`
				Document json.RawMessage `json:"document"`
			}
			if err := json.Unmarshal(data, &vectors); err != nil {
				t.Fatal(err)
			}
			schema := name[:len(name)-5] + ".schema.json"
			identities := []string{schema}
			for id, file := range catalog {
				if file == schema {
					identities = append(identities, id)
				}
			}
			if len(identities) != 2 {
				t.Fatal("vector schema requires one published identity")
			}
			for _, v := range vectors {
				t.Run(v.Name, func(t *testing.T) {
					for _, identity := range identities {
						err := Validate(identity, v.Document)
						if (err == nil) != v.Valid {
							t.Fatalf("valid=%v, error=%v (identity=%s)", v.Valid, err, identity)
						}
					}
				})
			}
		})
	}
}

// The strict JSON envelope rejects duplicate keys, unsafe integers, trailing
// documents and oversize bodies; a minimal valid request remains accepted.
func TestStrictDocuments(t *testing.T) {
	if err := Validate("describe-request.schema.json", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := Validate("describe-request.schema.json", []byte(`{}`+strings.Repeat(" ", Limit-2))); err != nil {
		t.Fatal("valid byte bound", err)
	}
	for _, data := range []string{`{"x":1,"x":2}`, `{} {}`, `{"x":9007199254740992}`, `{}` + strings.Repeat(" ", Limit-1)} {
		if Validate("describe-request.schema.json", []byte(data)) == nil {
			t.Fatal("invalid document accepted")
		}
	}
}

// Every published branch uses supported keywords and regexes, and absolute URN
// references resolve within the catalog. This also covers branches that no
// particular positive vector traverses.
func TestSchemaCatalog(t *testing.T) {
	if catalogError != nil {
		t.Fatal(catalogError)
	}
	files, err := Documents.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var visit func(map[string]any)
	visit = func(rule map[string]any) {
		for key := range rule {
			if !keywords[key] {
				t.Fatalf("unsupported schema keyword %s", key)
			}
		}
		if ref, ok := rule["$ref"].(string); ok {
			if _, ok := catalog[ref]; !ok {
				t.Fatalf("unresolved reference %s", ref)
			}
		}
		if pattern, ok := rule["pattern"].(string); ok {
			if _, err := regexp.Compile(pattern); err != nil {
				t.Fatal("invalid schema regex", err)
			}
		}
		for _, key := range []string{"anyOf", "oneOf", "allOf"} {
			if branches, ok := rule[key].([]any); ok {
				for _, branch := range branches {
					visit(branch.(map[string]any))
				}
			}
		}
		for _, key := range []string{"if", "then", "else", "not", "items", "additionalProperties", "propertyNames"} {
			if child, ok := rule[key].(map[string]any); ok {
				visit(child)
			}
		}
		if properties, ok := rule["properties"].(map[string]any); ok {
			for _, child := range properties {
				visit(child.(map[string]any))
			}
		}
	}
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".schema.json") {
			continue
		}
		b, err := Documents.ReadFile(file.Name())
		if err != nil {
			t.Fatal(err)
		}
		var rule map[string]any
		if err := json.Unmarshal(b, &rule); err != nil {
			t.Fatal(err)
		}
		visit(rule)
	}
	if err := Validate("urn:relux:curator-run:session", []byte(`{}`)); err == nil {
		t.Fatal("URN bypassed session validation")
	}
	if err := Validate("urn:relux:curator-run:unpublished", []byte(`{}`)); err == nil {
		t.Fatal("unknown URN accepted")
	}
}

// Filename and published URN apply the same UTF-8 byte bound. Native tail
// strings, including selectors, are structurally valid; provider semantics
// belong to the caller and are covered by launcher client regression tests.
func TestValidationIdentity(t *testing.T) {
	data, err := Documents.ReadFile("vectors/launch.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct{ Document json.RawMessage }
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		env   string
		tail  []string
		valid bool
	}{
		{"byte-bound", strings.Repeat("é", 2048), []string{}, true},
		{"byte-overflow", strings.Repeat("é", 2049), []string{}, false},
		{"native-value", "ok", []string{"--model", "resume"}, true},
		{"native-selector-shape", "ok", []string{"--resume", "latest"}, true},
	} {
		var request map[string]any
		if err := json.Unmarshal(vectors[0].Document, &request); err != nil {
			t.Fatal(err)
		}
		request["child_env"] = map[string]string{"LANGUAGE": tc.env}
		request["native_tail"] = tc.tail
		body, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		for _, identity := range []string{"launch.schema.json", "urn:relux:curator-run:delegated-launch"} {
			t.Run(tc.name+"/"+identity, func(t *testing.T) {
				err := Validate(identity, body)
				if (err == nil) != tc.valid {
					t.Fatalf("semantic verdict valid=%t, error=%v", tc.valid, err)
				}
			})
		}
	}
}
