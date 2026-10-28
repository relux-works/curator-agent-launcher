package memberwire

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

func bindingCases() []compatibilityCase {
	return []compatibilityCase{
		{"assignment.json", 65536, DecodeAssignment},
		{"attempt-first.json", 16384, DecodeAttempt},
		{"operation-submit.json", 16384, DecodeOperationIntent},
		{"operation-clear.json", 16384, DecodeOperationCompletion},
	}
}
func bindingAssignment(t *testing.T, network string) map[string]any {
	t.Helper()
	v := compatibilitySource(t, "assignment.json")
	v["producer_member_id"] = strings.Repeat("a", 32)
	v["artifacts"] = []any{map[string]any{"object": lifecycleRef(), "role": "assignment"}}
	e := v["effects"].(map[string]any)
	e["tools"], e["read_objects"], e["output_names"] = []any{"tool"}, []any{"source"}, []any{"patch"}
	e["network"] = network
	if network == "admitted_provider" {
		e["provider_binding_sha256"], e["max_provider_requests"], e["max_provider_tokens"] = strings.Repeat("b", 64), uint64(1), uint64(1)
	}
	return v
}

type bindingShape struct {
	label string
	c     compatibilityCase
	build func() map[string]any
}

func bindingShapes(t *testing.T) []bindingShape {
	t.Helper()
	cs := bindingCases()
	return []bindingShape{
		{"assignment-disabled", cs[0], func() map[string]any { return bindingAssignment(t, "disabled") }},
		{"assignment-provider", cs[0], func() map[string]any { return bindingAssignment(t, "admitted_provider") }},
		{"attempt-null", cs[1], func() map[string]any { return compatibilitySource(t, "attempt-first.json") }},
		{"attempt-predecessor", cs[1], func() map[string]any { return compatibilitySource(t, "attempt-retry.json") }},
		{"intent", cs[2], func() map[string]any { return compatibilitySource(t, "operation-submit.json") }},
		{"completion-clear", cs[3], func() map[string]any { return compatibilitySource(t, "operation-clear.json") }},
		{"completion-conflict", cs[3], func() map[string]any { return compatibilitySource(t, "operation-conflict.json") }},
	}
}
func bindingPaths(v map[string]any) []string {
	paths := lifecycleObjectPaths(v)
	if _, ok := v["artifacts"]; ok {
		paths = append(paths, "artifacts.0", "artifacts.0.object")
	}
	return paths
}
func bindingNullable(v map[string]any, path, key string) bool {
	return path == "" && (key == "producer_member_id" || key == "predecessor_invocation_id" || key == "conflict" && v["disposition"] == "clear") || path == "effects" && key == "provider_binding_sha256" && v["effects"].(map[string]any)["network"] == "disabled"
}

// Every original binding/journal fixture is a manual structural positive, even
// changed bindings and conflict publications. Literal byte anchors precede
// decoding; no digest match, filename or schema engine supplies the verdict.
func TestBindingPublishedBytes(t *testing.T) {
	rows := []struct {
		file   string
		length int
		digest string
		api    int
	}{
		{"assignment-changed.json", 2840, "0fd95c841c752878e50d2f05c45f0140d4783754838791a569ac9ed22b1ecc22", 0},
		{"assignment.json", 2840, "26f5e140a039b8d80d8a38c6f42633aedadd12b53b60734f7342adc3925799b1", 0},
		{"attempt-first.json", 437, "ae3fe9e35bd767577277d90738e72a67a46d22f791d3e160bd54e4e0dd91f250", 1},
		{"attempt-retry.json", 467, "85c216cb031c137e20ca5e297d20f42c86a5bd3d29c9e7a37a9872d17ba2d29d", 1},
		{"operation-clear.json", 322, "0753c97708c820b47c1e15a6037f001042068b3ee0a7eb0fb84c4511a707a50c", 3},
		{"operation-conflict.json", 466, "614f8e89cb01805d3a8f5cb854265ae6816d0c647c8262d14f892e5921f2f21d", 3},
		{"operation-phase-2-clear.json", 323, "76fe6592010b18128c5bdc16d0d49c9f844a28a9c7ba5bd50c15112815bc50ee", 3},
		{"operation-phase-2.json", 347, "1186a1d27dfebda0e94bce1b686cc5ab7ff17c501317951c8a618caf11ca321a", 2},
		{"operation-phase-3-clear.json", 323, "b010ab652694eab9f91abbaae919e7bce37f43258abb9f7d7428f7c6bc971dbf", 3},
		{"operation-phase-3.json", 347, "49dcb1e2d499a5fab10ec287b933354b6de84dd08a9a1ebd8c72e8347126eacd", 2},
		{"operation-phase-4-clear.json", 323, "586f507245567a10e751c46102a7da29fbbd5e73e8394f3a757cb34835b431f0", 3},
		{"operation-phase-4.json", 347, "1648b9eeca9072f72444b5deb1d8d60cf21fc886d09427b20f1fbd165c4f79f7", 2},
		{"operation-phase-5-clear.json", 323, "499735a70c35c1b987964221fb945d1a34997b11816e6acb6cbf021cd3db2cf4", 3},
		{"operation-phase-5.json", 347, "8aaff615e45576662b05aa724dba4a730c51b4d87114bff2efec428ec551ac5d", 2},
		{"operation-phase-6-clear.json", 323, "7c6163b05e4c39613d53f8ff1d9c7e06cb02b5acfc5ed53d59725b2175b9c7f1", 3},
		{"operation-phase-6.json", 347, "2eb06d911c50192911044bf6187eec49ed363a80bbbf07fc580f9340ec2efbdd", 2},
		{"operation-phase-7-clear.json", 323, "347bda5d07eae73eda3bf16d5b6bf356945946443a7205246cc83c55e1b3c5a9", 3},
		{"operation-phase-7.json", 347, "ff381e452ab15ead04c88a215e11c4cdf8ad834ace8179d3d49f40d3739a2fbe", 2},
		{"operation-phase-8-clear.json", 323, "bd524e718eb1bd3c89f1d5e308e9aa656957e5ea45a91811ee015b52cf60b405", 3},
		{"operation-phase-8.json", 347, "b0fa2224b972da26a6f2fd58385764c275be8a780baf0bdc3892c4dd5ca93c9d", 2},
		{"operation-reserve-clear.json", 323, "48d28af23148b21c8978e48e5397fe66aaa68a4338e7e8252cc96bf0139554d3", 3},
		{"operation-reserve.json", 347, "e0fbbf3493254fb0f38216248e61852a8584b806f88a64a9b4c74ec9f5b6a0a3", 2},
		{"operation-result-conflict.json", 347, "3243a519abfa7f0a2b458f12f4b4cfca924be651b53c4a8c7665d26b80891f43", 2},
		{"operation-submit.json", 346, "99d2d990c13284fe7a1f2d7c6c2a3acf6d12d6b61801f90b87465cff94c6d952", 2},
	}
	for _, row := range rows {
		t.Run(row.file, func(t *testing.T) {
			raw := fixture(t, row.file)
			sum := sha256.Sum256(raw)
			if len(raw) != row.length || hex.EncodeToString(sum[:]) != row.digest {
				t.Fatal("original fixture byte anchor changed")
			}
			lifecycleFrozen(t, bindingCases()[row.api], raw)
		})
	}
	// Populated nullable fields, BOTH network branches and an existing artifact /
	// effects-name element reach the unchanged predecode full frozen oracle.
	for _, shape := range bindingShapes(t) {
		lifecycleFrozen(t, shape.c, encode(t, shape.build()))
	}
}

// Inclusive raw caps precede copying/encoding/schema. Exact padded positives
// retain whitespace and independent raw/value/hash/length expectations.
func TestBindingCapsAndRoots(t *testing.T) {
	if AssignmentLimit != 65536 || AttemptLimit != 16384 || OperationIntentLimit != 16384 || OperationCompletionLimit != 16384 {
		t.Fatal("published raw caps changed")
	}
	for _, c := range bindingCases() {
		t.Run(c.file, func(t *testing.T) {
			raw := fixture(t, c.file)
			for _, n := range []int{c.limit - 1, c.limit} {
				padded := append(bytes.Clone(raw), bytes.Repeat([]byte(" "), n-len(raw))...)
				if len(padded) != n {
					t.Fatal("boundary not constructed")
				}
				lifecycleFrozen(t, c, padded)
			}
			for _, bad := range [][]byte{append(bytes.Clone(raw), bytes.Repeat([]byte(" "), c.limit+1-len(raw))...), bytes.Repeat([]byte("!"), c.limit+1)} {
				compatibilityBad(t, c, bad, "bounds_exceeded")
			}
			for _, kind := range []string{"curator-run member-assignment/1", "curator-run member-attempt/1", "curator-run member-operation-intent/1", "curator-run member-operation-completion/1", "curator-run member-request/1", "unknown"} {
				v := compatibilitySource(t, c.file)
				if v["kind"] == kind {
					continue
				}
				v["kind"] = kind
				compatibilityBad(t, c, encode(t, v), "schema_invalid")
			}
			for _, bad := range [][]byte{nil, {}, []byte("[]"), []byte("null"), []byte("true"), []byte("0"), []byte(`"text"`), raw[:len(raw)-2], append(bytes.Clone(raw), 'x'), append(bytes.Clone(raw), raw...)} {
				compatibilityBad(t, c, bad, "encoding_invalid")
			}
		})
	}
}

// Each required member of every reachable closed object has independent
// omission/null/type/unknown neighbors. Nullability never permits omission.
func TestBindingRecursiveClosure(t *testing.T) {
	for _, shape := range bindingShapes(t) {
		for _, path := range bindingPaths(shape.build()) {
			t.Run(shape.label+"/"+path, func(t *testing.T) {
				positive := shape.build()
				compatibilityOK(t, shape.c, positive)
				for key, current := range compatibilityAt(positive, path) {
					v := shape.build()
					delete(compatibilityAt(v, path), key)
					compatibilityBad(t, shape.c, encode(t, v), "schema_invalid")
					for _, bad := range []any{nil, true, uint64(2), "private/path/marker", []any{}, map[string]any{}} {
						if bad != nil && lifecycleType(bad) == lifecycleType(current) {
							continue
						}
						v = shape.build()
						compatibilityAt(v, path)[key] = bad
						if bad == nil && bindingNullable(positive, path, key) {
							compatibilityOK(t, shape.c, v)
						} else {
							compatibilityBad(t, shape.c, encode(t, v), "schema_invalid")
						}
					}
				}
				v := shape.build()
				compatibilityAt(v, path)["unknown"] = "private/path/secret-marker"
				compatibilityBad(t, shape.c, encode(t, v), "schema_invalid")
			})
		}
	}
}

// Structural role/predecessor combinations are data, not history/admission.
// Duplicate artifacts and assignment-role artifacts are intentionally legal.
func TestBindingIndependentDataAxes(t *testing.T) {
	c := bindingCases()[0]
	for _, role := range []string{"development", "source_review", "recipient_assessment"} {
		for _, producer := range []any{nil, strings.Repeat("a", 32), "00000000000000000000000000000005"} {
			for _, artifactRole := range []string{"instruction", "source", "tuple", "owner_input", "profile_input", "receipt", "assignment"} {
				v := bindingAssignment(t, "disabled")
				v["role"], v["producer_member_id"] = role, producer
				a := v["artifacts"].([]any)
				a[0].(map[string]any)["role"] = artifactRole
				v["artifacts"] = []any{a[0], a[0]}
				lifecycleFrozen(t, c, encode(t, v))
			}
		}
	}
	for _, key := range []string{"role", "artifacts.0.role"} {
		v := bindingAssignment(t, "disabled")
		compatibilitySet(v, key, "unknown")
		compatibilityBad(t, c, encode(t, v), "schema_invalid")
	}
	c = bindingCases()[1]
	for _, ordinal := range []uint64{1, 2, 256} {
		for _, pred := range []any{nil, strings.Repeat("b", 32), "00000000000000000000000000000004"} {
			v := compatibilitySource(t, c.file)
			v["ordinal"], v["predecessor_invocation_id"] = ordinal, pred
			lifecycleFrozen(t, c, encode(t, v))
		}
	}
	c = bindingCases()[2]
	for _, op := range []string{"submit", "cancel", "reconcile", "reserve", "ack", "publish"} {
		v := compatibilitySource(t, c.file)
		v["operation"] = op
		lifecycleFrozen(t, c, encode(t, v))
	}
	for _, op := range []string{"inspect", "consume", "unknown", ""} {
		v := compatibilitySource(t, c.file)
		v["operation"] = op
		compatibilityBad(t, c, encode(t, v), "schema_invalid")
	}
}

// Each completion implication is violated ALONE with otherwise valid data.
// No equality between intent/conflict refs or actual journal policy is added.
func TestBindingCompletionConditionals(t *testing.T) {
	c := bindingCases()[3]
	for _, disposition := range []string{"clear", "conflict"} {
		v := compatibilitySource(t, c.file)
		v["disposition"] = disposition
		if disposition == "conflict" {
			v["conflict"] = v["intent"]
		}
		lifecycleFrozen(t, c, encode(t, v))
		if disposition == "clear" {
			v["conflict"] = lifecycleRef()
		} else {
			v["conflict"] = nil
		}
		compatibilityBad(t, c, encode(t, v), "schema_invalid")
	}
	v := compatibilitySource(t, c.file)
	v["disposition"] = "unknown"
	compatibilityBad(t, c, encode(t, v), "schema_invalid")
}

// Every populated ID/digest/ref-name occurrence is reached through its actual
// wrapper, including nullable alternatives. Limits are literal source bounds.
func TestBindingGrammarsAndRanges(t *testing.T) {
	for _, shape := range bindingShapes(t) {
		for _, path := range bindingPaths(shape.build()) {
			for key := range compatibilityAt(shape.build(), path) {
				var good, bad []any
				switch {
				case strings.HasSuffix(key, "_id"):
					good = []any{strings.Repeat("0", 32), strings.Repeat("f", 32)}
					bad = []any{"", strings.Repeat("a", 31), strings.Repeat("a", 33), strings.Repeat("A", 32), strings.Repeat("g", 32), strings.Repeat("é", 16)}
				case key == "sha256" || strings.HasSuffix(key, "_sha256"):
					// Disabled binding must remain null; provider shape covers its grammar.
					if path == "effects" && shape.build()["effects"].(map[string]any)["network"] == "disabled" {
						continue
					}
					good = []any{strings.Repeat("0", 64), strings.Repeat("f", 64)}
					bad = []any{"", strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64), strings.Repeat("é", 32)}
				case key == "name":
					good = []any{"A", "0", "aZ09._-", strings.Repeat("a", 64)}
					bad = []any{"", strings.Repeat("a", 65), ".a", "_a", "-a", "a/b", "a b", "é"}
				case key == "length":
					good = []any{uint64(0), uint64(4194304)}
					bad = []any{uint64(4194305)}
				case key == "ordinal":
					good = []any{uint64(1), uint64(256)}
					bad = []any{uint64(0), uint64(257)}
				case key == "sequence":
					good = []any{uint64(1), uint64(4096)}
					bad = []any{uint64(0), uint64(4097)}
				default:
					continue
				}
				t.Run(shape.label+"/"+path+"/"+key, func(t *testing.T) {
					for _, value := range good {
						v := shape.build()
						compatibilityAt(v, path)[key] = value
						compatibilityOK(t, shape.c, v)
					}
					for _, value := range bad {
						v := shape.build()
						compatibilityAt(v, path)[key] = value
						compatibilityBad(t, shape.c, encode(t, v), "schema_invalid")
					}
				})
			}
		}
	}
	// Every positive deadline and process/provider budget gets independent zero,
	// low/high and one-over probes through the new assignment wrapper.
	c := bindingCases()[0]
	rows := []struct {
		path, key string
		high      uint64
	}{
		{"deadlines", "startup_ms", 60000}, {"deadlines", "runtime_ms", 3600000},
		{"deadlines", "term_ms", 6000}, {"deadlines", "kill_observation_ms", 6000},
		{"deadlines", "result_ms", 6000}, {"deadlines", "state_budget_bytes", 67108864},
		{"effects", "max_processes", 64}, {"effects", "max_provider_requests", 64}, {"effects", "max_provider_tokens", 65536},
	}
	for _, row := range rows {
		t.Run(row.path+"/"+row.key, func(t *testing.T) {
			for _, value := range []uint64{1, row.high} {
				v := bindingAssignment(t, "admitted_provider")
				compatibilityAt(v, row.path)[row.key] = value
				compatibilityOK(t, c, v)
			}
			for _, value := range []uint64{0, row.high + 1} {
				v := bindingAssignment(t, "admitted_provider")
				compatibilityAt(v, row.path)[row.key] = value
				compatibilityBad(t, c, encode(t, v), "schema_invalid")
			}
		})
	}
}

// Both network branches have valid controls. Each of the THREE conditioned
// properties is violated individually; budgets do not authorize network use.
func TestBindingEffectsConditionals(t *testing.T) {
	c := bindingCases()[0]
	for _, network := range []string{"disabled", "admitted_provider"} {
		lifecycleFrozen(t, c, encode(t, bindingAssignment(t, network)))
		for _, key := range []string{"provider_binding_sha256", "max_provider_requests", "max_provider_tokens"} {
			v := bindingAssignment(t, network)
			e := v["effects"].(map[string]any)
			if network == "disabled" {
				if key == "provider_binding_sha256" {
					e[key] = strings.Repeat("a", 64)
				} else {
					e[key] = uint64(1)
				}
			} else {
				if key == "provider_binding_sha256" {
					e[key] = nil
				} else {
					e[key] = uint64(0)
				}
			}
			compatibilityBad(t, c, encode(t, v), "schema_invalid")
		}
	}
	v := bindingAssignment(t, "disabled")
	v["effects"].(map[string]any)["network"] = "unknown"
	compatibilityBad(t, c, encode(t, v), "schema_invalid")
	// Process counts are independent of the disabled provider's zero budgets.
	for _, n := range []uint64{1, 64} {
		v := bindingAssignment(t, "disabled")
		v["effects"].(map[string]any)["max_processes"] = n
		compatibilityOK(t, c, v)
	}
}

// Artifacts permit duplicates and arbitrary shaped roles; effects names require
// uniqueness AFTER escape decoding. First/last invalid elements cannot hide
// behind an in-range array length. These are shapes, never manifest admission.
func TestBindingArrays(t *testing.T) {
	c := bindingCases()[0]
	for _, n := range []int{0, 1, 128, 129} {
		v := bindingAssignment(t, "disabled")
		a := make([]any, n)
		for i := range a {
			a[i] = map[string]any{"object": lifecycleRef(), "role": "assignment"}
		}
		v["artifacts"] = a
		if n == 0 || n == 129 {
			compatibilityBad(t, c, encode(t, v), "schema_invalid")
		} else {
			lifecycleFrozen(t, c, encode(t, v))
		}
	}
	for _, i := range []int{0, 127} {
		for _, mode := range []string{"map", "object", "role"} {
			v := bindingAssignment(t, "disabled")
			a := make([]any, 128)
			for j := range a {
				a[j] = map[string]any{"object": lifecycleRef(), "role": "source"}
			}
			switch mode {
			case "map":
				a[i] = true
			case "object":
				a[i].(map[string]any)["object"] = nil
			case "role":
				a[i].(map[string]any)["role"] = "unknown"
			}
			v["artifacts"] = a
			compatibilityBad(t, c, encode(t, v), "schema_invalid")
		}
	}
	for _, row := range []struct {
		key string
		max int
	}{{"tools", 32}, {"read_objects", 128}, {"output_names", 128}} {
		t.Run(row.key, func(t *testing.T) {
			for _, n := range []int{0, 1, row.max, row.max + 1} {
				v := bindingAssignment(t, "disabled")
				a := make([]any, n)
				for i := range a {
					a[i] = fmt.Sprintf("n%d", i)
				}
				v["effects"].(map[string]any)[row.key] = a
				if n > row.max {
					compatibilityBad(t, c, encode(t, v), "schema_invalid")
				} else {
					lifecycleFrozen(t, c, encode(t, v))
				}
			}
			for _, i := range []int{0, row.max - 1} {
				for _, bad := range []any{nil, true, uint64(1), map[string]any{}, []any{}, "", ".bad", "a/b", "é", strings.Repeat("a", 65)} {
					v := bindingAssignment(t, "disabled")
					a := make([]any, row.max)
					for j := range a {
						a[j] = fmt.Sprintf("n%d", j)
					}
					a[i] = bad
					v["effects"].(map[string]any)[row.key] = a
					compatibilityBad(t, c, encode(t, v), "schema_invalid")
				}
			}
			for _, name := range []string{"A", "0", "aZ09._-", strings.Repeat("a", 64)} {
				v := bindingAssignment(t, "disabled")
				v["effects"].(map[string]any)[row.key] = []any{name}
				compatibilityOK(t, c, v)
			}
			v := bindingAssignment(t, "disabled")
			v["effects"].(map[string]any)[row.key] = []any{"tool", "other"}
			raw := string(encode(t, v))
			prefix := `"` + row.key + `":`
			good := strings.Replace(raw, prefix+`["tool","other"]`, prefix+`["t\u006fol","other"]`, 1)
			bad := strings.Replace(raw, prefix+`["tool","other"]`, prefix+`["tool","t\u006fol"]`, 1)
			if good == raw || bad == raw {
				t.Fatal("decoded name neighbor not constructed")
			}
			lifecycleFrozen(t, c, []byte(good))
			compatibilityBad(t, c, []byte(bad), "schema_invalid")
			v["effects"].(map[string]any)[row.key] = []any{"tool", "tool"}
			compatibilityBad(t, c, encode(t, v), "schema_invalid")
		})
	}
}

// Every API routes unchanged strict lexical/static refusal logic. Duplicate
// decoded keys are distinct from duplicate decoded array names above. No local
// parser execution or schema-engine oracle is part of these source assertions.
func TestBindingStrictEntryRefusals(t *testing.T) {
	for index, c := range bindingCases() {
		t.Run(c.file, func(t *testing.T) {
			v := compatibilitySource(t, c.file)
			raw := string(encode(t, v))
			good := strings.Replace(raw, `"kind":`, `"k\u0069nd":`, 1)
			if good == raw {
				t.Fatal("escaped key control not applied")
			}
			lifecycleFrozen(t, c, []byte(good))
			duplicate := `{"kind":"` + v["kind"].(string) + `","k\u0069nd":"` + v["kind"].(string) + `",` + raw[1:]
			compatibilityBad(t, c, []byte(duplicate), "schema_invalid")
			// Assignment and completion contain references; attempt/intent are flat.
			needle := ""
			if index == 0 {
				needle = `"tuple":{`
			} else if index == 3 {
				needle = `"intent":{`
			}
			if needle != "" {
				bad := strings.Replace(raw, needle, needle+`"name":"x","na\u006de":"x",`, 1)
				if bad == raw {
					t.Fatal("nested duplicate not applied")
				}
				compatibilityBad(t, c, []byte(bad), "schema_invalid")
			}
			for _, bad := range [][]byte{append([]byte{0xef, 0xbb, 0xbf}, []byte(raw)...), append([]byte{0xff}, []byte(raw)...), []byte(strings.Replace(raw, `"kind":`, "\"kind\":\x00", 1)), []byte(`{"x":"\ud800"}`), []byte(`{"x":"\udc00"}`), []byte(`{"x":"\u0000"}`)} {
				compatibilityBad(t, c, bad, "encoding_invalid")
			}
			compatibilityBad(t, c, []byte(`{"private/path/secret-marker":"private/path/secret-marker",`+raw[1:]), "schema_invalid")
			token := `"sequence":1`
			switch index {
			case 0:
				v["deadlines"].(map[string]any)["startup_ms"] = uint64(1)
				token = `"startup_ms":1`
			case 1:
				token = `"ordinal":1`
			}
			numeric := string(encode(t, v))
			lifecycleFrozen(t, c, []byte(numeric))
			for _, n := range []string{"-0", "-1", "0.0", "1e0", "1E+2", "01", "+1", "NaN", "Infinity", "9007199254740992"} {
				bad := strings.Replace(numeric, token, strings.TrimSuffix(token, "1")+n, 1)
				if bad == numeric {
					t.Fatal("numeric neighbor not applied")
				}
				code := "schema_invalid"
				if n == "9007199254740992" {
					code = "bounds_exceeded"
				}
				compatibilityBad(t, c, []byte(bad), code)
			}
			for _, n := range []int{4096, 4097} {
				// Unknown fields still obey the shared decoded string/key bound first.
				for _, probe := range []string{`{"private":"` + strings.Repeat("a", n) + `",` + raw[1:], `{"` + strings.Repeat("a", n) + `":null,` + raw[1:]} {
					code := "schema_invalid"
					if n > 4096 {
						code = "bounds_exceeded"
					}
					compatibilityBad(t, c, []byte(probe), code)
				}
			}
		})
	}
}
