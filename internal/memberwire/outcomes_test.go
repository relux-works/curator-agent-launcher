package memberwire

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

func outcomeCases() []compatibilityCase {
	return []compatibilityCase{
		{"result-exit.json", 65536, DecodeResult},
		{"response-result.json", 16384, DecodeResponse},
		{"reservation.json", 16384, DecodeReservation},
		{"ack.json", 16384, DecodeAck},
		{"conflict-result.json", 16384, DecodeConflict},
	}
}
func outcomeResult(t *testing.T) map[string]any {
	t.Helper()
	v := compatibilitySource(t, "result-exit.json")
	v["error"], v["cancel"] = lifecycleError(), lifecycleRef()
	v["identity"].(map[string]any)["producer_member_id"] = strings.Repeat("a", 32)
	v["identity"].(map[string]any)["predecessor_invocation_id"] = strings.Repeat("b", 32)
	return v
}

// This independent response table constructs schema neighbors, not actual
// correlation, stored references or effect-authorizing responses.
func outcomeResponse(t *testing.T, outcome string) map[string]any {
	t.Helper()
	v := compatibilitySource(t, "response-result.json")
	v["outcome"] = outcome
	v["snapshot"], v["result"], v["reservation"], v["ack"], v["error"] = nil, nil, nil, nil, nil
	switch outcome {
	case "refused", "conflict":
		v["error"], v["snapshot"] = lifecycleError(), lifecycleRef()
	case "snapshot":
		v["snapshot"] = lifecycleRef()
	case "result":
		v["snapshot"], v["result"] = lifecycleRef(), lifecycleRef()
	case "consumption":
		v["snapshot"], v["result"], v["reservation"], v["ack"] = lifecycleRef(), lifecycleRef(), lifecycleRef(), lifecycleRef()
	default:
		t.Fatal("unknown authored outcome")
	}
	return v
}

// Literal source hashes/verdicts predate product decoding. Replay fixtures are
// only nested response inputs, never a standalone accepted sixth API/root.
func TestOutcomePublishedBytes(t *testing.T) {
	rows := []struct {
		file   string
		length int
		digest string
		api    int
		code   string
		replay bool
	}{
		{"result-exit.json", 2612, "5f4ae5c6c2e330878d9bedac2c26d214740b0d2d890609b15d26e29ded1d1411", 0, "", false},
		{"result-signal.json", 2526, "502f5b24f654d56e15ec2dd41e300e267dc845e8c5b840d74a572d26adfe37d0", 0, "", false},
		{"result-cancel-before-allocation.json", 1570, "b9336bb393df79955d2ac9ebd16adf5eac2e0802af30a30aaec9143499c832f2", 0, "", false},
		{"response-result.json", 1001, "82e3e0a0c8c4efe1209d32746adc106a18926af8e56952eb8c49ae102be88dd5", 1, "", false},
		{"response-refusal.json", 722, "5fb80b6a174d4f130982cd5dd2203dc586989378ab2a11688e4654cf09143e26", 1, "", false},
		{"replay-identical.json", 389, "5878a260d0bdd218bf1b41fab753f0854d40e54e23585b11d3366e926841052b", 1, "", true},
		{"replay-conflict.json", 388, "ab450a73381c3f65f4180ea5b9a19661697f94b3ac6f70ea42914b0f663efcb9", 1, "", true},
		{"reservation.json", 515, "357c39b442eb8748f97d9b4fcc9c6559c8bd4a47eeae9bc63f7b8384dec123c7", 2, "", false},
		{"ack.json", 275, "10fdd5a367b06962750cb9cbdc6caf9e8ca792c402478c8936687f7f258e01cd", 3, "", false},
		{"conflict-result.json", 597, "4e8bf8540617a9fdbaa64f22586b590a4f1b3c9ee4c28f39bd3eb4d855500fcd", 4, "", false},
		{"negative-native-exit.json", 2614, "03d916bd311ccde6a310625c40c7f6515232df2df19197ea55a4a4ae7c30fa73", 0, "schema_invalid", false},
		{"truncated-response.json.txt", 58, "bd469aa1171ec6098bd85fdd0fafad84560fbba5ff171c0003bf3ad97c17b04a", 1, "encoding_invalid", false},
	}
	for _, row := range rows {
		t.Run(row.file, func(t *testing.T) {
			raw := fixture(t, row.file)
			sum := sha256.Sum256(raw)
			if len(raw) != row.length || hex.EncodeToString(sum[:]) != row.digest {
				t.Fatal("original fixture byte anchor changed")
			}
			c := outcomeCases()[row.api]
			if row.replay {
				v := outcomeResponse(t, "result")
				v["replay"] = compatibilitySource(t, row.file)
				lifecycleFrozen(t, c, encode(t, v))
			} else if row.code != "" {
				compatibilityBad(t, c, raw, row.code)
			} else {
				lifecycleFrozen(t, c, raw)
			}
		})
	}
	// The unchanged frozen-before-decode helper mutates every existing map and
	// array element, including output refs/argv, replay, source and stream maps.
	lifecycleFrozen(t, outcomeCases()[0], encode(t, outcomeResult(t)))
	lifecycleFrozen(t, outcomeCases()[1], encode(t, outcomeResponse(t, "conflict")))
	lifecycleFrozen(t, outcomeCases()[1], encode(t, outcomeResponse(t, "consumption")))
}

// The whole response cap owns nested replay too: whitespace INSIDE replay may
// fill the complete response cap, without an invented separate replay ceiling.
func TestOutcomeCapsAndRoots(t *testing.T) {
	if ResultLimit != 65536 || ResponseLimit != 16384 || ReservationLimit != 16384 || AckLimit != 16384 || ConflictLimit != 16384 {
		t.Fatal("published raw caps changed")
	}
	for _, c := range outcomeCases() {
		t.Run(c.file, func(t *testing.T) {
			raw := fixture(t, c.file)
			for _, n := range []int{c.limit - 1, c.limit} {
				padded := append(bytes.Clone(raw), bytes.Repeat([]byte(" "), n-len(raw))...)
				if len(padded) != n {
					t.Fatal("exact raw boundary not constructed")
				}
				lifecycleFrozen(t, c, padded)
			}
			for _, bad := range [][]byte{append(bytes.Clone(raw), bytes.Repeat([]byte(" "), c.limit+1-len(raw))...), bytes.Repeat([]byte("!"), c.limit+1)} {
				compatibilityBad(t, c, bad, "bounds_exceeded")
			}
			for _, kind := range []string{"curator-run member-result/1", "curator-run member-response/1", "curator-run member-reservation/1", "curator-run member-ack/1", "curator-run member-conflict/1", "curator-run member-replay/1", "curator-run member-snapshot/1", "curator-run member-request/1", "unknown"} {
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
	v := outcomeResponse(t, "consumption")
	raw := string(encode(t, v))
	for _, n := range []int{16384, 16385} {
		large := strings.Replace(raw, `"replay":{`, `"replay":{`+strings.Repeat(" ", n-len(raw)), 1)
		if large == raw || len(large) != n {
			t.Fatal("nested whitespace boundary not applied")
		}
		if n == 16384 {
			lifecycleFrozen(t, outcomeCases()[1], []byte(large))
		} else {
			compatibilityBad(t, outcomeCases()[1], []byte(large), "bounds_exceeded")
		}
	}
}

type outcomeShape struct {
	name  string
	c     compatibilityCase
	build func() map[string]any
}

func outcomeShapes(t *testing.T) []outcomeShape {
	t.Helper()
	var shapes []outcomeShape
	for i, c := range outcomeCases() {
		shapes = append(shapes, outcomeShape{c.file, c, func() map[string]any {
			if i == 0 {
				return outcomeResult(t)
			}
			if i == 1 {
				return outcomeResponse(t, "conflict")
			}
			return compatibilitySource(t, c.file)
		}})
	}
	shapes = append(shapes, outcomeShape{"consumption", outcomeCases()[1], func() map[string]any { return outcomeResponse(t, "consumption") }})
	for _, family := range []string{"native", "cleanup"} {
		for i := range lifecycleVariants(t, family) {
			shapes = append(shapes, outcomeShape{fmt.Sprintf("%s/%d", family, i), outcomeCases()[0], func() map[string]any {
				v := outcomeResult(t)
				v[family] = lifecycleVariants(t, family)[i]
				return v
			}})
		}
	}
	return shapes
}
func outcomePaths(v map[string]any) []string {
	paths := lifecycleObjectPaths(v)
	// Each authored populated result shape has exactly one output reference;
	// array count/elements at both ends are separately exercised below.
	if _, exists := v["outputs"]; exists {
		paths = append(paths, "outputs.0")
	}
	return paths
}
func outcomeNull(v map[string]any, path, key string) bool {
	if path == "" {
		switch v["kind"] {
		case "curator-run member-result/1":
			return key == "scope" || key == "actual_tuple" || key == "argv" || key == "cancel" || key == "error"
		case "curator-run member-response/1":
			switch key {
			case "request_id", "request_sha256", "operation", "ack":
				return true
			case "snapshot":
				return v["outcome"] == "refused" || v["outcome"] == "conflict"
			case "result":
				return v["outcome"] != "result" && v["outcome"] != "consumption"
			case "reservation":
				return v["outcome"] != "consumption"
			case "error":
				return v["outcome"] != "refused" && v["outcome"] != "conflict"
			}
		}
	}
	return path == "identity" && (key == "producer_member_id" || key == "predecessor_invocation_id") || path == "scope" && key == "root_incarnation_sha256" || path == "error" && (key == "source" || key == "evidence") || path == "replay" && strings.HasSuffix(key, "_sha256") || path == "cleanup" && v["cleanup"].(map[string]any)["status"] == "failed" && key == "evidence"
}

// All reachable populated object shapes (including outputs[0]) get a nearby
// valid control, required deletion, unknown member and null/incompatible types.
func TestOutcomeRecursiveClosure(t *testing.T) {
	for _, shape := range outcomeShapes(t) {
		for _, path := range outcomePaths(shape.build()) {
			t.Run(shape.name+"/"+path, func(t *testing.T) {
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
						if bad == nil && outcomeNull(positive, path, key) {
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

// Each independent then-property is violated alone with otherwise valid data.
// Unlisted snapshot/ack and all request correlation/operation axes stay legal.
func TestOutcomeResponseConditionals(t *testing.T) {
	c := outcomeCases()[1]
	rows := []struct {
		outcome string
		keys    []string
	}{
		{"refused", []string{"error", "result", "reservation", "ack"}},
		{"conflict", []string{"error", "result", "reservation", "ack"}},
		{"snapshot", []string{"snapshot", "result", "error", "reservation", "ack"}},
		{"result", []string{"result", "snapshot", "error", "reservation", "ack"}},
		{"consumption", []string{"reservation", "error", "result", "snapshot"}},
	}
	for _, row := range rows {
		t.Run(row.outcome, func(t *testing.T) {
			lifecycleFrozen(t, c, encode(t, outcomeResponse(t, row.outcome)))
			for _, key := range row.keys {
				v := outcomeResponse(t, row.outcome)
				if v[key] != nil {
					v[key] = nil
				} else if key == "error" {
					v[key] = lifecycleError()
				} else {
					v[key] = lifecycleRef()
				}
				compatibilityBad(t, c, encode(t, v), "schema_invalid")
			}
			for _, field := range []string{"snapshot", "ack"} {
				constrained := false
				for _, k := range row.keys {
					constrained = constrained || field == k
				}
				if !constrained {
					for _, value := range []any{nil, lifecycleRef()} {
						v := outcomeResponse(t, row.outcome)
						v[field] = value
						lifecycleFrozen(t, c, encode(t, v))
					}
				}
			}
			for mask := 0; mask < 4; mask++ {
				for _, operation := range []any{nil, "submit", "inspect", "cancel", "reconcile", "consume"} {
					v := outcomeResponse(t, row.outcome)
					if mask&1 != 0 {
						v["request_id"] = nil
					}
					if mask&2 != 0 {
						v["request_sha256"] = nil
					}
					v["operation"] = operation
					lifecycleFrozen(t, c, encode(t, v))
				}
			}
		})
	}
	for _, key := range []string{"outcome", "operation"} {
		v := outcomeResponse(t, "result")
		v[key] = "unknown"
		compatibilityBad(t, c, encode(t, v), "schema_invalid")
	}
}

// ALL4x4x8 key/classification/null masks are structural data, even contradictory
// ones. Equal/different hashes do not prove replay truth or consumer authority.
func TestOutcomeNestedReplay(t *testing.T) {
	c := outcomeCases()[1]
	for _, key := range []string{"invocation", "cancel", "effect", "none"} {
		for _, classification := range []string{"new", "identical", "conflict", "none"} {
			for mask := 0; mask < 8; mask++ {
				v := outcomeResponse(t, "consumption")
				r := v["replay"].(map[string]any)
				r["key"], r["classification"] = key, classification
				for i, field := range []string{"original_request_sha256", "current_request_sha256", "original_result_sha256"} {
					r[field] = strings.Repeat(string(rune('a'+i)), 64)
					if mask&(1<<i) != 0 {
						r[field] = nil
					}
				}
				lifecycleFrozen(t, c, encode(t, v))
			}
		}
	}
	for _, classification := range []string{"identical", "conflict"} {
		v := outcomeResponse(t, "refused")
		r := v["replay"].(map[string]any)
		r["classification"] = classification
		r["original_request_sha256"], r["current_request_sha256"] = strings.Repeat("a", 64), strings.Repeat("a", 64)
		lifecycleFrozen(t, c, encode(t, v))
	}
	for _, field := range []string{"kind", "key", "classification"} {
		v := outcomeResponse(t, "result")
		v["replay"].(map[string]any)[field] = "unknown"
		compatibilityBad(t, c, encode(t, v), "schema_invalid")
	}
}

// Every populated grammar/range occurrence, including output/stream/replay refs,
// is exercised through its new wrapper. Stream and ref lengths differ on purpose.
func TestOutcomeGrammarsAndRanges(t *testing.T) {
	for _, shape := range outcomeShapes(t) {
		for _, path := range outcomePaths(shape.build()) {
			for key := range compatibilityAt(shape.build(), path) {
				var good, bad []any
				switch {
				case key == "sha256" || strings.HasSuffix(key, "_sha256"):
					good = []any{strings.Repeat("0", 64), strings.Repeat("f", 64)}
					bad = []any{"", strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64)}
				case key == "adapter_revision":
					good = []any{strings.Repeat("a", 40), strings.Repeat("f", 64)}
					bad = []any{"", strings.Repeat("a", 39), strings.Repeat("a", 41), strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 40)}
				case key == "adapter_id" || key == "component" || key == "namespace" || key == "name" && path != "native":
					good = []any{"a", "0", "A_-.9", strings.Repeat("a", 64)}
					bad = []any{"", strings.Repeat("a", 65), ".a", "_a", "-a", "a/b", "a b", "a\n", "é"}
				case key == "id" || strings.HasSuffix(key, "_id"):
					good = []any{strings.Repeat("0", 32), strings.Repeat("f", 32)}
					bad = []any{"", strings.Repeat("a", 31), strings.Repeat("a", 33), strings.Repeat("A", 32), strings.Repeat("g", 32)}
				case key == "length":
					maximum := uint64(4194304)
					if path == "stdout" || path == "stderr" {
						maximum = 1048576
					}
					good, bad = []any{uint64(0), maximum}, []any{maximum + 1}
				case key == "attempt_ordinal" || key == "final_generation":
					good, bad = []any{uint64(1), uint64(256)}, []any{uint64(0), uint64(257)}
				case key == "number":
					good, bad = []any{uint64(1), uint64(255)}, []any{uint64(0), uint64(256)}
				case key == "code" && path == "native":
					good, bad = []any{uint64(0), uint64(255)}, []any{uint64(256)}
				default:
					continue
				}
				t.Run(shape.name+"/"+path+"/"+key, func(t *testing.T) {
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
}

// Data axes have no terminal/stream/output/cleanup consistency condition here.
// Every native/cleanup/worker enum combination with truncated output is positive.
func TestOutcomeIndependentDataAxes(t *testing.T) {
	c := outcomeCases()[0]
	for _, native := range lifecycleVariants(t, "native") {
		for _, cleanup := range lifecycleVariants(t, "cleanup") {
			for _, worker := range []string{"complete", "missing", "partial", "invalid", "overflow"} {
				v := outcomeResult(t)
				v["native"], v["cleanup"], v["worker_output"] = native, cleanup, worker
				v["outputs"] = []any{lifecycleRef(), lifecycleRef()}
				v["stdout"].(map[string]any)["truncated"] = true
				lifecycleFrozen(t, c, encode(t, v))
			}
		}
	}
	v := outcomeResult(t)
	v["worker_output"] = "unknown"
	compatibilityBad(t, c, encode(t, v), "schema_invalid")
	for _, family := range []string{"native", "cleanup"} {
		keys := map[string]bool{}
		for _, value := range lifecycleVariants(t, family) {
			for key := range value.(map[string]any) {
				keys[key] = true
			}
		}
		for _, variant := range lifecycleVariants(t, family) {
			for key := range keys {
				if _, exists := variant.(map[string]any)[key]; exists {
					continue
				}
				v := outcomeResult(t)
				copy := map[string]any{}
				for k, value := range variant.(map[string]any) {
					copy[k] = value
				}
				copy[key] = nil
				v[family] = copy
				compatibilityBad(t, c, encode(t, v), "schema_invalid")
			}
		}
		v := outcomeResult(t)
		v[family] = lifecycleStatus("unpublished")
		compatibilityBad(t, c, encode(t, v), "schema_invalid")
	}
	for _, stream := range []string{"stdout", "stderr"} {
		for _, flag := range []bool{false, true} {
			v := outcomeResult(t)
			v[stream].(map[string]any)["truncated"] = flag
			lifecycleFrozen(t, c, encode(t, v))
		}
	}
	for _, name := range []string{"0", "A", "Z9", strings.Repeat("A", 32), "", strings.Repeat("A", 33), "SIG_TERM", "sigterm", "Σ", "A\n"} {
		for _, core := range []bool{false, true} {
			v := outcomeResult(t)
			v["native"] = map[string]any{"status": "signal", "number": uint64(1), "name": name, "core_dumped": core, "evidence": lifecycleRef()}
			if name == "0" || name == "A" || name == "Z9" || name == strings.Repeat("A", 32) {
				compatibilityOK(t, c, v)
			} else {
				compatibilityBad(t, c, encode(t, v), "schema_invalid")
			}
		}
	}
	for _, role := range []string{"development", "source_review", "recipient_assessment", "unknown"} {
		v := outcomeResult(t)
		v["identity"].(map[string]any)["role"] = role
		if role == "unknown" {
			compatibilityBad(t, c, encode(t, v), "schema_invalid")
		} else {
			compatibilityOK(t, c, v)
		}
	}
	for _, category := range []string{"request", "result", "cancel", "consumption", "observation", "assignment", "unknown"} {
		v := compatibilitySource(t, "conflict-result.json")
		v["category"] = category
		v["conflicting_sha256"] = v["original_sha256"]
		if category == "unknown" {
			compatibilityBad(t, outcomeCases()[4], encode(t, v), "schema_invalid")
		} else {
			lifecycleFrozen(t, outcomeCases()[4], encode(t, v))
		}
	}
	for _, index := range []int{0, 4} {
		for _, role := range []string{"supervisor", "reconciler", "unknown"} {
			c := outcomeCases()[index]
			v := compatibilitySource(t, c.file)
			v["writer"].(map[string]any)["role"] = role
			if role == "unknown" {
				compatibilityBad(t, c, encode(t, v), "schema_invalid")
			} else {
				compatibilityOK(t, c, v)
			}
		}
	}
}

// Null scope/cancel/tuple/argv/error remain required data. Arrays have separate
// cardinality, exact-element, malformed-ref and scalar-vs-decoded-byte controls.
func TestOutcomeArraysAndNulls(t *testing.T) {
	c := outcomeCases()[0]
	for mask := 0; mask < 32; mask++ {
		v := outcomeResult(t)
		for i, key := range []string{"scope", "actual_tuple", "argv", "cancel", "error"} {
			if mask&(1<<i) != 0 {
				v[key] = nil
			}
		}
		lifecycleFrozen(t, c, encode(t, v))
	}
	for _, field := range []string{"argv", "outputs"} {
		for _, n := range []int{0, 1, 128, 129} {
			v := outcomeResult(t)
			a := make([]any, n)
			for i := range a {
				if field == "argv" {
					a[i] = "fixture"
				} else {
					a[i] = lifecycleRef()
				}
			}
			v[field] = a
			if n > 128 {
				compatibilityBad(t, c, encode(t, v), "schema_invalid")
			} else {
				lifecycleFrozen(t, c, encode(t, v))
			}
		}
		for _, index := range []int{0, 127} {
			for _, bad := range []any{nil, true, uint64(1), "", []any{}, map[string]any{}} {
				v := outcomeResult(t)
				a := make([]any, 128)
				for i := range a {
					if field == "argv" {
						a[i] = "fixture"
					} else {
						a[i] = lifecycleRef()
					}
				}
				a[index], v[field] = bad, a
				compatibilityBad(t, c, encode(t, v), "schema_invalid")
			}
		}
	}
	for _, value := range []any{nil, []any{}} {
		v := outcomeResult(t)
		v["argv"] = value
		r := compatibilityOK(t, c, v)
		if (r.Value()["argv"] == nil) != (value == nil) {
			t.Fatal("unknown argv normalized to known empty")
		}
	}
	for _, arg := range []string{"a", "\n", "e\u0301", strings.Repeat("a", 4096), strings.Repeat("é", 2048), strings.Repeat("𝄞", 1024)} {
		v := outcomeResult(t)
		v["argv"] = []any{arg}
		r := compatibilityOK(t, c, v)
		if r.Value()["argv"].([]any)[0] != arg {
			t.Fatal("argument normalized")
		}
		lifecycleFrozen(t, c, encode(t, v))
	}
	for _, arg := range []string{strings.Repeat("a", 4097), strings.Repeat("é", 2048) + "a", strings.Repeat("𝄞", 1025)} {
		v := outcomeResult(t)
		v["argv"] = []any{arg}
		compatibilityBad(t, c, encode(t, v), "bounds_exceeded")
	}
	for _, pair := range []struct{ escaped, literal string }{
		{strings.Repeat(`\u0061`, 4096), strings.Repeat("a", 4096)},
		{strings.Repeat(`\u00e9`, 2048), strings.Repeat("é", 2048)},
		{strings.Repeat(`\ud834\udd1e`, 1024), strings.Repeat("𝄞", 1024)},
	} {
		v := outcomeResult(t)
		v["argv"] = []any{"REPLACE"}
		raw := strings.Replace(string(encode(t, v)), `"REPLACE"`, `"`+pair.escaped+`"`, 1)
		if strings.Contains(raw, "REPLACE") {
			t.Fatal("escaped positive not applied")
		}
		r, err := c.decode([]byte(raw))
		if err != nil || r == nil || r.Value()["argv"].([]any)[0] != pair.literal {
			t.Fatal("escaped argument refused/changed", err)
		}
		lifecycleFrozen(t, c, []byte(raw))
	}
}

// Unchanged lifecycle tests cover the complete stage/code matrix. These NEW
// wrapper paths retain representative pairs, nullable source/evidence, closure
// and sourceError grammar, without claiming another local matrix campaign.
func TestOutcomeErrorRoutes(t *testing.T) {
	pairs := [][2]string{{"bootstrap", "descriptor_invalid"}, {"parse", "schema_invalid"}, {"admission", "assignment_conflict"}, {"compatibility", "presence_contamination"}, {"store", "store_interlocked"}, {"start", "spawn_failed"}, {"runtime", "supervisor_lost"}, {"result", "result_conflict"}, {"cleanup", "cleanup_failed"}, {"consume", "effect_unknown"}, {"cancel", "cancel_conflict"}}
	for _, index := range []int{0, 1} {
		c := outcomeCases()[index]
		build := func() map[string]any {
			if index == 0 {
				return outcomeResult(t)
			}
			return outcomeResponse(t, "conflict")
		}
		for _, pair := range pairs {
			for _, source := range []any{nil, lifecycleSourceError()} {
				for _, evidence := range []any{nil, lifecycleRef()} {
					v := build()
					v["error"] = map[string]any{"stage": pair[0], "code": pair[1], "source": source, "evidence": evidence}
					lifecycleFrozen(t, c, encode(t, v))
				}
			}
		}
		for _, pair := range [][2]string{{"bootstrap", "spawn_failed"}, {"unknown", "spawn_failed"}, {"start", "unknown"}} {
			v := build()
			v["error"].(map[string]any)["stage"], v["error"].(map[string]any)["code"] = pair[0], pair[1]
			compatibilityBad(t, c, encode(t, v), "schema_invalid")
		}
		for _, code := range []string{"A", ".", "_", ":", "-", strings.Repeat(":", 128), "", strings.Repeat("a", 129), "é", "a/b", "a b", "a\n"} {
			v := build()
			v["error"].(map[string]any)["source"].(map[string]any)["code"] = code
			if code == "A" || code == "." || code == "_" || code == ":" || code == "-" || code == strings.Repeat(":", 128) {
				compatibilityOK(t, c, v)
			} else {
				compatibilityBad(t, c, encode(t, v), "schema_invalid")
			}
		}
	}
}

// Each new API preserves the strict entry, exact nil/static/privacy and input
// immutability route. Conflict has no numeric field: numeric-token probes use an
// unknown member to check lexical priority, never invent a permitted property.
func TestOutcomeStrictEntryRefusals(t *testing.T) {
	for index, c := range outcomeCases() {
		t.Run(c.file, func(t *testing.T) {
			v := compatibilitySource(t, c.file)
			raw := string(encode(t, v))
			good := strings.Replace(raw, `"kind":`, `"k\u0069nd":`, 1)
			if good == raw {
				t.Fatal("escaped control not constructed")
			}
			lifecycleFrozen(t, c, []byte(good))
			duplicate := `{"kind":"` + v["kind"].(string) + `","k\u0069nd":"` + v["kind"].(string) + `",` + raw[1:]
			compatibilityBad(t, c, []byte(duplicate), "schema_invalid")
			needle, replacement := "", ""
			switch index {
			case 0:
				needle, replacement = `"identity":{`, `"identity":{"role":"development","ro\u006ce":"development",`
			case 1:
				needle, replacement = `"replay":{`, `"replay":{"key":"none","k\u0065y":"none",`
			case 3:
				needle, replacement = `"receipt":{`, `"receipt":{"name":"receipt","na\u006de":"receipt",`
			case 4:
				needle, replacement = `"writer":{`, `"writer":{"role":"supervisor","ro\u006ce":"supervisor",`
			}
			if needle != "" {
				bad := strings.Replace(raw, needle, replacement, 1)
				if bad == raw {
					t.Fatal("nested duplicate not applied")
				}
				compatibilityBad(t, c, []byte(bad), "schema_invalid")
			}
			for _, bad := range [][]byte{append([]byte{0xef, 0xbb, 0xbf}, []byte(raw)...), append([]byte{0xff}, []byte(raw)...), []byte(strings.Replace(raw, `"kind":`, "\"kind\":\x00", 1)), []byte(`{"x":"\ud800"}`), []byte(`{"x":"\udc00"}`), []byte(`{"x":"\u0000"}`)} {
				compatibilityBad(t, c, bad, "encoding_invalid")
			}
			compatibilityBad(t, c, []byte(`{"private/path/secret-marker":"private/path/secret-marker",`+raw[1:]), "schema_invalid")
			var numeric, token string
			switch index {
			case 0:
				v["final_generation"], token = uint64(1), `"final_generation":1`
			case 1:
				v["snapshot"].(map[string]any)["length"], token = uint64(1), `"length":1`
			case 2:
				v["attempt_ordinal"], token = uint64(1), `"attempt_ordinal":1`
			case 3:
				v["receipt"].(map[string]any)["length"], token = uint64(1), `"length":1`
			case 4:
				v["numeric_probe"], token = uint64(1), `"numeric_probe":1`
			}
			numeric = string(encode(t, v))
			if index != 4 {
				lifecycleFrozen(t, c, []byte(numeric))
			} else {
				compatibilityBad(t, c, []byte(numeric), "schema_invalid")
			}
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
				bad := []byte(`{"private":"` + strings.Repeat("a", n) + `",` + raw[1:])
				code := "schema_invalid"
				if n > 4096 {
					code = "bounds_exceeded"
				}
				compatibilityBad(t, c, bad, code)
			}
		})
	}
}
