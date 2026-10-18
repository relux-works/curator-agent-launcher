package managerschema

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// Every published vector is checked against its named schema, including all
// nullable combinations and paired valid/invalid cross-field and bound cases.
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
			for _, v := range vectors {
				t.Run(v.Name, func(t *testing.T) {
					err := Validate(name[:len(name)-5]+".schema.json", v.Document)
					if (err == nil) != v.Valid {
						t.Fatalf("valid=%v, error=%v", v.Valid, err)
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
