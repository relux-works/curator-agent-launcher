package managerschema

import (
	"encoding/json"
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
			if _, err := compilePattern(pattern); err != nil {
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

// Closed validation retains unknown members for rejection; CCJ-1 signature
// omission belongs only to canonical hashing, not protocol/schema acceptance.
func TestValidateRetainsUnknownMembers(t *testing.T) {
	if err := Validate("describe-request.schema.json", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	for _, member := range []string{"sig", "extra"} {
		if err := Validate("describe-request.schema.json", []byte(`{"`+member+`":"sentinel"}`)); err == nil {
			t.Fatalf("closed schema accepted %s", member)
		}
	}
	b, err := CanonicalJSON([]byte(`{"sig":"sentinel"}`))
	if err != nil || string(b) != "{}" {
		t.Fatal("canonical hashing contract changed", string(b), err)
	}
}

// TestCompilePatternEndAnchor proves that only the exact terminal
// `$(?![\s\S])` whose `$` is a metacharacter becomes RE2's end of text, and
// that every other form fails closed instead of compiling to a different,
// weaker pattern. It covers the translation, not the published schemas,
// which TestSchemaCatalog compiles.
func TestCompilePatternEndAnchor(t *testing.T) {
	for _, tc := range []struct {
		name, pattern string
		compiles      bool
		match, miss   []string
	}{
		{name: "terminal-anchor", pattern: `^[a-z]{1,3}$(?![\s\S])`, compiles: true, match: []string{"abc"}, miss: []string{"abc\n", "abcd", ""}},
		{name: "even-backslashes-before-anchor", pattern: `^a\\$(?![\s\S])`, compiles: true, match: []string{`a\`}, miss: []string{"a\\\n", "a"}},
		{name: "plain-dollar-unchanged", pattern: `^a$`, compiles: true, match: []string{"a"}, miss: []string{"ab"}},
		{name: "escaped-dollar-refused", pattern: `^a\$(?![\s\S])`},
		{name: "three-backslashes-refused", pattern: `^a\\\$(?![\s\S])`},
		{name: "literal-block-refused", pattern: `^\Qa$(?![\s\S])`},
		{name: "literal-block-closed-refused", pattern: `^\Qa\E$(?![\s\S])`},
		{name: "non-terminal-suffix-refused", pattern: `^a$(?![\s\S])b`},
		{name: "doubled-suffix-refused", pattern: `^a$(?![\s\S])$(?![\s\S])`},
		{name: "other-lookahead-refused", pattern: `^a(?=b)`},
		{name: "negative-lookbehind-refused", pattern: `^(?<!x)a$(?![\s\S])`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := compilePattern(tc.pattern)
			if !tc.compiles {
				if err == nil {
					t.Fatalf("compilePattern(%q) = %q, want a refusal", tc.pattern, r)
				}
				return
			}
			if err != nil {
				t.Fatalf("compilePattern(%q): %v", tc.pattern, err)
			}
			for _, s := range tc.match {
				if !r.MatchString(s) {
					t.Fatalf("%q does not match %q", r, s)
				}
			}
			for _, s := range tc.miss {
				if r.MatchString(s) {
					t.Fatalf("%q matches %q", r, s)
				}
			}
		})
	}
}

// endAnchoredPatterns returns every `pattern` in node and the ones that do
// not end with the engine-independent end anchor `$(?![\s\S])`.
func endAnchoredPatterns(node any) (all, loose []string) {
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			if p, ok := child.(string); ok && key == "pattern" {
				all = append(all, p)
				if !strings.HasSuffix(p, `$(?![\s\S])`) {
					loose = append(loose, p)
				}
				continue
			}
			a, l := endAnchoredPatterns(child)
			all, loose = append(all, a...), append(loose, l...)
		}
	case []any:
		for _, child := range v {
			a, l := endAnchoredPatterns(child)
			all, loose = append(all, a...), append(loose, l...)
		}
	}
	return all, loose
}

// TestHomeLeasePatternsEndAnchored proves that every pattern of the two lease
// schemas, and of the process tuple inside the session and session-record
// schemas, ends with `$(?![\s\S])`. Go's RE2 treats a plain `$` as the end of
// text, so no vector can tell the two apart in Go; this test is the witness
// that the published grammar also holds in engines where `$` matches before a
// final newline. The in-test control shows that one reverted pattern is
// reported. It does not check patterns outside these subtrees.
func TestHomeLeasePatternsEndAnchored(t *testing.T) {
	load := func(name string) map[string]any {
		t.Helper()
		data, err := Documents.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		return doc
	}
	process := func(doc map[string]any) any {
		t.Helper()
		branches, _ := doc["oneOf"].([]any)
		if len(branches) == 0 {
			t.Fatal("no oneOf branch")
		}
		first, _ := branches[0].(map[string]any)
		props, _ := first["properties"].(map[string]any)
		p, ok := props["process"]
		if !ok {
			t.Fatal("no process member in the first branch")
		}
		return p
	}
	subtrees := map[string]any{
		"lease-holder.schema.json":           load("lease-holder.schema.json"),
		"lease-result.schema.json":           load("lease-result.schema.json"),
		"session.schema.json process":        process(load("session.schema.json")),
		"session-record.schema.json process": process(load("session-record.schema.json")),
	}
	for name, node := range subtrees {
		all, loose := endAnchoredPatterns(node)
		if len(all) == 0 {
			t.Fatalf("%s: no pattern found", name)
		}
		if len(loose) != 0 {
			t.Fatalf("%s: patterns without the end anchor: %q", name, loose)
		}
	}
	// Control: the same check reports a pattern reverted to a plain `$`.
	reverted := map[string]any{"pattern": `^[0-9]{1,20}$`, "properties": map[string]any{"x": map[string]any{"pattern": `^a$(?![\s\S])`}}}
	if _, loose := endAnchoredPatterns(reverted); len(loose) != 1 || loose[0] != `^[0-9]{1,20}$` {
		t.Fatalf("control: reverted pattern not reported: %q", loose)
	}
}
