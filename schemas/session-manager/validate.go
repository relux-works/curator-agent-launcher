// Package managerschema embeds the published session-manager JSON Schemas.
// Validation uses the closed subset of draft 2020-12 used by these documents;
// unsupported schema keywords fail closed rather than silently being ignored.
package managerschema

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/mapping"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	_ "github.com/relux-works/skill-agents-management/pkg/agentic/systems/claude"
	_ "github.com/relux-works/skill-agents-management/pkg/agentic/systems/codex"
	_ "github.com/relux-works/skill-agents-management/pkg/agentic/systems/muse"
)

//go:embed *.schema.json vectors/*.json
var Documents embed.FS

const Limit = 64 << 10

// Absolute URNs keep references usable by standard JSON Schema resolvers,
// including when a schema's own base URI is an opaque URN. Resolve them from
// the embedded documents' actual $id members; no network lookup is involved.
var catalog, catalogError = buildCatalog()

func buildCatalog() (map[string]string, error) {
	files, err := Documents.ReadDir(".")
	if err != nil {
		return nil, errors.New("schema catalog unavailable")
	}
	result := map[string]string{}
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".schema.json") {
			continue
		}
		data, err := Documents.ReadFile(file.Name())
		var meta struct {
			ID string `json:"$id"`
		}
		if err != nil || json.Unmarshal(data, &meta) != nil {
			return nil, errors.New("invalid schema catalog")
		}
		if meta.ID == "" {
			continue
		}
		if _, duplicate := result[meta.ID]; duplicate {
			return nil, errors.New("duplicate schema identity")
		}
		result[meta.ID] = file.Name()
	}
	return result, nil
}

// Validate reads the actual published schema, not a parallel Go shape. JSON
// also obeys the repository's strict reader (duplicates, UTF-8, safe integers).
// Errors identify schema locations only; request or plugin values never escape.
func Validate(name string, data []byte) error {
	// Resolve a published identity before both structural and semantic checks.
	if file, ok := catalog[name]; ok {
		name = file
	}
	if len(data) > Limit {
		return errors.New("protocol document exceeds byte bound")
	}
	v, err := fragment.ParseJSON(data)
	if err != nil {
		return errors.New("invalid protocol JSON")
	}
	var value any
	if json.Unmarshal(fragment.Canonical(v), &value) != nil {
		return errors.New("invalid protocol JSON")
	}
	if err := validateFile(name, value, 0); err != nil {
		return err
	}
	// Schema maxLength counts Unicode characters. These shared child values
	// additionally obey the transport's UTF-8 byte bound.
	checkEnv := func(value any) error {
		if env, ok := value.(map[string]any); ok {
			for _, v := range env {
				if text, ok := v.(string); ok && len(text) > 4096 {
					return errors.New("schema violation at $.child_env")
				}
			}
		}
		return nil
	}
	switch name {
	case "child-env.schema.json":
		if err := checkEnv(value); err != nil {
			return err
		}
	case "werkstatt-record.schema.json":
		if err := checkEnv(value.(map[string]any)["child_env"]); err != nil {
			return err
		}
	case "werkstatt-status.schema.json":
		for _, row := range value.([]any) {
			if err := checkEnv(row.(map[string]any)["record"].(map[string]any)["child_env"]); err != nil {
				return err
			}
		}
	}
	if name == "launch.schema.json" {
		object := value.(map[string]any)
		for _, v := range object["child_env"].(map[string]any) {
			if len(v.(string)) > 4096 {
				return errors.New("schema violation at $.child_env")
			}
		}
		var tail []string
		for _, v := range object["native_tail"].([]any) {
			tail = append(tail, v.(string))
		}
		target, err := mapping.Resolve(object["runtime"].(string))
		if err != nil {
			return errors.New("schema violation at $.runtime")
		}
		elevated, err := agentic.ElevateResumeIntent(agentic.Default, agentic.SystemID(target.System), nil, tail)
		if err != nil || elevated.Intent.Kind != agentic.ResumeNew || !reflect.DeepEqual(append([]string{}, elevated.NativeArgs...), append([]string{}, tail...)) {
			return errors.New("schema violation at $.native_tail")
		}
	}
	return nil
}

// ReservedChildName uses the same closed rule as the published launch schema.
func ReservedChildName(name string) bool {
	return validateFile("reserved-child-name.schema.json", name, 0) == nil
}

func validateFile(name string, value any, depth int) error {
	if catalogError != nil {
		return catalogError
	}
	if strings.HasPrefix(name, "urn:") {
		file, ok := catalog[name]
		if !ok {
			return errors.New("unknown schema identity")
		}
		name = file
	}
	if path.Base(name) != name || !strings.HasSuffix(name, ".schema.json") || depth > 64 {
		return errors.New("invalid schema reference")
	}
	data, err := Documents.ReadFile(name)
	if err != nil {
		return errors.New("schema unavailable")
	}
	var s map[string]any
	if json.Unmarshal(data, &s) != nil {
		return errors.New("invalid schema")
	}
	return check(s, value, "$", depth+1)
}

var keywords = map[string]bool{
	"$schema": true, "$id": true, "title": true, "description": true,
	"$ref": true, "type": true, "const": true, "enum": true,
	"oneOf": true, "anyOf": true, "allOf": true, "not": true,
	"if": true, "then": true, "else": true, "required": true,
	"properties": true, "additionalProperties": true, "propertyNames": true,
	"minProperties": true, "maxProperties": true, "items": true,
	"minItems": true, "maxItems": true, "uniqueItems": true,
	"minLength": true, "maxLength": true, "pattern": true, "format": true,
	"minimum": true, "maximum": true,
}

func check(s map[string]any, value any, field string, depth int) error {
	bad := func() error { return fmt.Errorf("schema violation at %s", field) }
	if depth > 64 {
		return bad()
	}
	for key := range s {
		if !keywords[key] {
			return errors.New("unsupported schema keyword")
		}
	}
	if ref, ok := s["$ref"].(string); ok {
		if err := validateFile(ref, value, depth+1); err != nil {
			return err
		}
	}
	if typ, ok := s["type"].(string); ok {
		valid := false
		switch typ {
		case "null":
			valid = value == nil
		case "boolean":
			_, valid = value.(bool)
		case "string":
			_, valid = value.(string)
		case "object":
			_, valid = value.(map[string]any)
		case "array":
			_, valid = value.([]any)
		case "integer":
			n, ok := value.(float64)
			valid = ok && math.Trunc(n) == n
		default:
			return errors.New("unsupported schema type")
		}
		if !valid {
			return bad()
		}
	}
	if c, ok := s["const"]; ok && !reflect.DeepEqual(c, value) {
		return bad()
	}
	if list, ok := s["enum"].([]any); ok {
		found := false
		for _, v := range list {
			found = found || reflect.DeepEqual(v, value)
		}
		if !found {
			return bad()
		}
	}
	for _, keyword := range []string{"oneOf", "anyOf", "allOf"} {
		if list, ok := s[keyword].([]any); ok {
			n := 0
			for _, branch := range list {
				if check(branch.(map[string]any), value, field, depth+1) == nil {
					n++
				}
			}
			if (keyword == "oneOf" && n != 1) || (keyword == "anyOf" && n == 0) || (keyword == "allOf" && n != len(list)) {
				return bad()
			}
		}
	}
	if neg, ok := s["not"].(map[string]any); ok && check(neg, value, field, depth+1) == nil {
		return bad()
	}
	if cond, ok := s["if"].(map[string]any); ok {
		key := "else"
		if check(cond, value, field, depth+1) == nil {
			key = "then"
		}
		if branch, ok := s[key].(map[string]any); ok {
			if err := check(branch, value, field, depth+1); err != nil {
				return err
			}
		}
	}
	switch v := value.(type) {
	case map[string]any:
		if !within(s, "minProperties", "maxProperties", float64(len(v))) {
			return bad()
		}
		if required, ok := s["required"].([]any); ok {
			for _, key := range required {
				if _, ok := v[key.(string)]; !ok {
					return bad()
				}
			}
		}
		props, _ := s["properties"].(map[string]any)
		for key, member := range v {
			if names, ok := s["propertyNames"].(map[string]any); ok {
				if check(names, key, field+".<name>", depth+1) != nil {
					return bad()
				}
			}
			if rule, ok := props[key].(map[string]any); ok {
				if err := check(rule, member, field+"."+key, depth+1); err != nil {
					return err
				}
				continue
			}
			if additional, ok := s["additionalProperties"].(bool); ok && !additional {
				return bad()
			}
			if additional, ok := s["additionalProperties"].(map[string]any); ok {
				if err := check(additional, member, field+".<member>", depth+1); err != nil {
					return err
				}
			}
		}
	case []any:
		if !within(s, "minItems", "maxItems", float64(len(v))) {
			return bad()
		}
		for i, member := range v {
			if item, ok := s["items"].(map[string]any); ok {
				if err := check(item, member, field+"[]", depth+1); err != nil {
					return err
				}
			}
			if unique, _ := s["uniqueItems"].(bool); unique {
				for _, prior := range v[:i] {
					if reflect.DeepEqual(member, prior) {
						return bad()
					}
				}
			}
		}
	case string:
		if !within(s, "minLength", "maxLength", float64(utf8.RuneCountInString(v))) {
			return bad()
		}
		if pattern, ok := s["pattern"].(string); ok {
			r, err := regexp.Compile(pattern)
			if err != nil || !r.MatchString(v) {
				return bad()
			}
		}
		if format, ok := s["format"].(string); ok {
			switch format {
			case "date-time":
				if _, err := time.Parse(time.RFC3339Nano, v); err != nil || !strings.HasSuffix(v, "Z") {
					return bad()
				}
			default:
				return errors.New("unsupported schema format")
			}
		}
	case float64:
		if !within(s, "minimum", "maximum", v) {
			return bad()
		}
	}
	return nil
}

func within(s map[string]any, minKey, maxKey string, n float64) bool {
	if lo, ok := s[minKey].(float64); ok && n < lo {
		return false
	}
	if hi, ok := s[maxKey].(float64); ok && n > hi {
		return false
	}
	return true
}
