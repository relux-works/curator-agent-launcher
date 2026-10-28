package memberwire

import (
	"fmt"
	"strings"
	"testing"
)

// Field boundaries below come from the closed request $defs, not a comparison
// with another decoder. Cross-reference truth, roles and authorization remain
// deliberately outside this package's structural success claim.
func TestSchemaFieldBoundsAndBranches(t *testing.T) {
	submit := func() map[string]any { return requestFixture(t, "submit.json") }
	for _, c := range []struct {
		key     string
		maximum uint64
	}{
		{"startup_ms", 60000}, {"runtime_ms", 3600000}, {"term_ms", 6000}, {"kill_observation_ms", 6000}, {"result_ms", 6000}, {"state_budget_bytes", 67108864},
	} {
		t.Run(c.key, func(t *testing.T) {
			for _, n := range []uint64{1, c.maximum} {
				v := submit()
				v["deadlines"].(map[string]any)[c.key] = n
				requestOK(t, v)
			}
			for _, n := range []uint64{0, c.maximum + 1} {
				v := submit()
				v["deadlines"].(map[string]any)[c.key] = n
				requestBad(t, encode(t, v), "schema_invalid")
			}
		})
	}
	for _, key := range []string{"tuple", "assignment"} {
		for _, c := range []struct {
			field     string
			good, bad []any
		}{
			{"name", []any{"a", "0", strings.Repeat("a", 64), "A_-.9"}, []any{"", strings.Repeat("a", 65), ".a", "/a", "a/b", "a b", "é", "a\n"}},
			{"sha256", []any{strings.Repeat("0", 64), strings.Repeat("f", 64)}, []any{strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64)}},
			{"length", []any{uint64(0), uint64(4194304)}, []any{uint64(4194305), "0", nil}},
		} {
			for _, n := range c.good {
				v := submit()
				v[key].(map[string]any)[c.field] = n
				requestOK(t, v)
			}
			for _, n := range c.bad {
				v := submit()
				v[key].(map[string]any)[c.field] = n
				requestBad(t, encode(t, v), "schema_invalid")
			}
		}
	}
	for _, key := range []string{"controller_id", "assignment_id", "invocation_id", "member_id", "consumer_id", "producer_member_id", "predecessor_invocation_id"} {
		for _, n := range []string{strings.Repeat("a", 32), strings.Repeat("0", 32)} {
			v := submit()
			v["identity"].(map[string]any)[key] = n
			requestOK(t, v)
		}
		for _, n := range []string{"", strings.Repeat("a", 31), strings.Repeat("a", 33), strings.Repeat("A", 32), strings.Repeat("g", 32), strings.Repeat("é", 16)} {
			v := submit()
			v["identity"].(map[string]any)[key] = n
			requestBad(t, encode(t, v), "schema_invalid")
		}
	}
	for _, role := range []string{"development", "source_review", "recipient_assessment"} {
		v := submit()
		v["identity"].(map[string]any)["role"] = role
		requestOK(t, v)
	}
	v := submit()
	v["identity"].(map[string]any)["role"] = "owner"
	requestBad(t, encode(t, v), "schema_invalid")
	// Repeated artifacts are permitted by structure, unlike unique effects lists.
	for _, size := range []int{1, 128} {
		v := submit()
		item := v["artifacts"].([]any)[0]
		a := make([]any, size)
		for i := range a {
			a[i] = item
		}
		v["artifacts"] = a
		requestOK(t, v)
	}
	for _, size := range []int{0, 129} {
		v := submit()
		item := v["artifacts"].([]any)[0]
		a := make([]any, size)
		for i := range a {
			a[i] = item
		}
		v["artifacts"] = a
		requestBad(t, encode(t, v), "schema_invalid")
	}
	for _, role := range []string{"instruction", "source", "tuple", "owner_input", "profile_input", "receipt", "assignment"} {
		v := submit()
		v["artifacts"].([]any)[0].(map[string]any)["role"] = role
		requestOK(t, v)
	}
	v = submit()
	v["artifacts"].([]any)[0].(map[string]any)["role"] = "unknown"
	requestBad(t, encode(t, v), "schema_invalid")
	for _, reason := range []string{"controller_request", "deadline"} {
		v := requestFixture(t, "request-cancel.json")
		v["reason"] = reason
		requestOK(t, v)
	}
	v = requestFixture(t, "request-cancel.json")
	v["reason"] = "unknown"
	requestBad(t, encode(t, v), "schema_invalid")
	for _, file := range []string{"request-reserve.json", "request-ack.json"} {
		v := requestFixture(t, file)
		requestOK(t, v)
		for _, action := range []any{"unknown", "cancel", nil, uint64(1)} {
			v := requestFixture(t, file)
			v["action"] = action
			requestBad(t, encode(t, v), "schema_invalid")
		}
		v = requestFixture(t, file)
		if file == "request-reserve.json" {
			v["receipt"] = submit()["tuple"]
		} else {
			delete(v, "receipt")
		}
		requestBad(t, encode(t, v), "schema_invalid")
		for _, key := range []string{"invocation_id", "consumer_id", "effect_id", "result_sha256"} {
			v := requestFixture(t, file)
			v[key] = "invalid"
			requestBad(t, encode(t, v), "schema_invalid")
		}
	}
	// Ack's recursive receipt is closed, with the same exact ref requirements.
	for _, key := range []string{"name", "length", "sha256"} {
		v := requestFixture(t, "request-ack.json")
		delete(v["receipt"].(map[string]any), key)
		requestBad(t, encode(t, v), "schema_invalid")
	}
	v = requestFixture(t, "request-ack.json")
	v["receipt"].(map[string]any)["unknown"] = 0
	requestBad(t, encode(t, v), "schema_invalid")
	for _, file := range []string{"request-inspect.json", "request-reconcile.json", "request-cancel.json"} {
		v := requestFixture(t, file)
		v["invocation_id"] = "invalid"
		requestBad(t, encode(t, v), "schema_invalid")
	}
}

// Required array cardinality/uniqueness and both network branches are paired
// with their inclusive valid bounds. No provider call or tool execution occurs.
func TestEffectArraysAndNetworkBranches(t *testing.T) {
	submit := func() map[string]any { return requestFixture(t, "submit.json") }
	for _, c := range []struct {
		key string
		max int
	}{{"tools", 32}, {"read_objects", 128}, {"output_names", 128}} {
		list := func(n int) []any {
			a := make([]any, n)
			for i := range a {
				a[i] = fmt.Sprintf("n%d", i)
			}
			return a
		}
		for _, size := range []int{0, c.max} {
			v := submit()
			v["effects"].(map[string]any)[c.key] = list(size)
			r := requestOK(t, v)
			if len(r.Value()["effects"].(map[string]any)[c.key].([]any)) != size {
				t.Fatal("array lost or null inserted")
			}
		}
		for _, bad := range []any{list(c.max + 1), []any{"same", "same"}, []any{"bad/name"}, []any{true}, nil, "name"} {
			v := submit()
			v["effects"].(map[string]any)[c.key] = bad
			requestBad(t, encode(t, v), "schema_invalid")
		}
	}
	for _, n := range []uint64{1, 64} {
		v := submit()
		v["effects"].(map[string]any)["max_processes"] = n
		requestOK(t, v)
	}
	for _, n := range []uint64{0, 65} {
		v := submit()
		v["effects"].(map[string]any)["max_processes"] = n
		requestBad(t, encode(t, v), "schema_invalid")
	}
	provider := func() map[string]any {
		v := submit()
		e := v["effects"].(map[string]any)
		e["network"] = "admitted_provider"
		e["provider_binding_sha256"] = strings.Repeat("a", 64)
		e["max_provider_requests"] = uint64(1)
		e["max_provider_tokens"] = uint64(1)
		return v
	}
	requestOK(t, submit())
	requestOK(t, provider())
	for _, c := range []struct {
		key string
		max uint64
	}{{"max_provider_requests", 64}, {"max_provider_tokens", 65536}} {
		for _, n := range []uint64{1, c.max} {
			v := provider()
			v["effects"].(map[string]any)[c.key] = n
			requestOK(t, v)
		}
		for _, n := range []uint64{0, c.max + 1} {
			v := provider()
			v["effects"].(map[string]any)[c.key] = n
			requestBad(t, encode(t, v), "schema_invalid")
		}
		v := submit()
		v["effects"].(map[string]any)[c.key] = uint64(1)
		requestBad(t, encode(t, v), "schema_invalid")
	}
	for _, binding := range []any{nil, "bad", strings.Repeat("A", 64)} {
		v := provider()
		v["effects"].(map[string]any)["provider_binding_sha256"] = binding
		requestBad(t, encode(t, v), "schema_invalid")
	}
	v := submit()
	v["effects"].(map[string]any)["provider_binding_sha256"] = strings.Repeat("a", 64)
	requestBad(t, encode(t, v), "schema_invalid")
	v = submit()
	v["effects"].(map[string]any)["network"] = "enabled"
	requestBad(t, encode(t, v), "schema_invalid")
	// Unicode escapes decode before uniqueItems: two spellings of one name fail.
	raw := string(encode(t, submit()))
	raw = strings.Replace(raw, `"tools":[]`, `"tools":["a","\u0061"]`, 1)
	if !strings.Contains(raw, `"tools":["a","\u0061"]`) {
		t.Fatal("unique-name negative was not constructed")
	}
	requestBad(t, []byte(raw), "schema_invalid")
}
