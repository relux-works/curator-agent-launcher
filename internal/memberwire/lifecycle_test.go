package memberwire

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
)

func lifecycleCases() []compatibilityCase {
	return []compatibilityCase{
		{"state-admitted.json", 16384, DecodeSnapshot},
		{"head.json", 16384, DecodeHead},
		{"cancel.json", 4096, DecodeCancel},
		{"native-exit.json", 65536, DecodeObservation},
		{"scope-absent.json", 16384, DecodeScopeProof},
	}
}
func lifecycleRef() map[string]any {
	return map[string]any{"name": "untrusted", "length": uint64(0), "sha256": strings.Repeat("a", 64)}
}
func lifecycleSourceError() map[string]any {
	return map[string]any{"component": "fixture", "namespace": "fixture", "code": ":_.-CODE09"}
}
func lifecycleError() map[string]any {
	return map[string]any{"stage": "start", "code": "spawn_failed", "source": lifecycleSourceError(), "evidence": lifecycleRef()}
}
func lifecycleStatus(status string) map[string]any { return map[string]any{"status": status} }
func lifecycleScope(t *testing.T) any              { return compatibilitySource(t, "scope-absent.json")["scope"] }
func lifecycleStop(t *testing.T) any {
	return compatibilitySource(t, "state-stop-term-intent.json")["stop"]
}

// This independently chosen uncertain shape exposes every nullable object for
// closure/grammar neighbors. It has no semantic consistency or custody claim.
func lifecycleFull(t *testing.T) map[string]any {
	t.Helper()
	v := compatibilitySource(t, "state-final.json")
	v["phase"], v["error"], v["stop"] = "uncertain", lifecycleError(), lifecycleStop(t)
	v["identity"].(map[string]any)["producer_member_id"] = strings.Repeat("a", 32)
	v["identity"].(map[string]any)["predecessor_invocation_id"] = strings.Repeat("b", 32)
	return v
}

// Independently frozen bytes/full values predate product decoding and mutation.
// Existing nested map and observation argv elements are changed, not appended.
func lifecycleFrozen(t *testing.T, c compatibilityCase, input []byte) {
	t.Helper()
	original := bytes.Clone(input)
	wantSum, wantLength := sha256.Sum256(original), len(original)
	var expected any
	d := json.NewDecoder(bytes.NewReader(original))
	d.UseNumber()
	if err := d.Decode(&expected); err != nil {
		t.Fatal(err)
	}
	wantValue := encode(t, expected)
	r, err := c.decode(input)
	if err != nil || r == nil {
		t.Fatal("structural positive refused", err)
	}
	earlier := r.Value()
	check := func() {
		if !bytes.Equal(r.Raw(), original) || r.SHA256() != wantSum || r.Length() != wantLength || !bytes.Equal(encode(t, r.Value()), wantValue) || !bytes.Equal(encode(t, earlier), wantValue) {
			t.Fatal("retained/fresh/earlier views differ from independent frozen expectations")
		}
	}
	check()
	input[0] = '!'
	raw := r.Raw()
	raw[0] = '?'
	changed := r.Value()
	// Recursing before replacement reaches every existing nested map/array,
	// including nullable-union objects when this positive contains them.
	var mutate func(any)
	mutate = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for k, child := range v {
				mutate(child)
				v[k] = "changed"
				if v[k] != "changed" {
					t.Fatal("map mutation did not apply")
				}
			}
		case []any:
			for i, child := range v {
				mutate(child)
				v[i] = "changed"
				if v[i] != "changed" {
					t.Fatal("existing array mutation did not apply")
				}
			}
		}
	}
	mutate(changed)
	if changed["kind"] != "changed" || raw[0] != '?' || input[0] != '!' {
		t.Fatal("mutation did not apply")
	}
	check()
}

// Fixed publication hashes are byte anchors, not schema engines/native truth.
// The full list gives explicit structural verdicts, including semantic negatives.
func TestLifecyclePublishedSnapshots(t *testing.T) {
	oracles := []struct {
		file   string
		length int
		digest string
		api    int
		valid  bool
	}{
		{"state-admitted.json", 1451, "50258be0e1c5efd65395627df912d014d23bc732f7f783f96b0d2ad2529c7ade", 0, true},
		{"state-allocated.json", 1959, "26e53b3bfa2a235503579b1843110f83046ce210ce25d9a5eb2a4acf212ac43e", 0, true},
		{"state-allocating.json", 1499, "3e2225d8ed835868db087922b38c69e4f0ed046c65574fde8f8eff869e23ad45", 0, true},
		{"state-allocation-unknown.json", 1594, "699512915fedb4d532d6960adc23364a3c1935095dbf34fc335d3163696aff23", 0, true},
		{"state-cancel-after-intent.json", 2353, "24b4cdd5890e39e9281b34c9934ebc9c79128440fb7741e5f7db84df892b9384", 0, true},
		{"state-cancel-before-release.json", 2507, "6c0a72afb63fc88a7e1bae9517469910f69962ee47dcf0af410e19d444aa9ca9", 0, true},
		{"state-cancel-won.json", 1595, "4bec933ec15097669145e5462a9e08da2ae56f0ef503dd110327bd2f2d5ebfff", 0, true},
		{"state-final.json", 2596, "346db4425257bb0a2d2c0b6416ff7df46261e89eea5fc267fa0919289b1ae8e7", 0, true},
		{"state-intent.json", 1962, "f0d20eff969cfa0905e9242fe3b8c2a4638715093ed5baeeec17c7f0f3df3f0e", 0, true},
		{"state-release-intent.json", 2114, "6aa0bf15795c2d4fbbe754801f006d65a941780d519eba7c2e68e03c24ba38f3", 0, true},
		{"state-running.json", 2127, "7a7d81c788c58937e6fccda5f4309c960c1226dd1d840d8b66884626df7d80b1", 0, true},
		{"state-spawned.json", 2104, "37bf06ac94c75616de572110a1ee8704b0394d00bb743b5f6ee497d201762ad7", 0, true},
		{"state-stop-kill-intent.json", 2515, "6e93230d6ed286e3035be2b6bd45f1b6c96396108143b3005009be347a148f48", 0, true},
		{"state-stop-kill-observe.json", 2516, "1a25a1919ac4f9c7e6414979f695f8a049ac778504f15634416c7d4702d23689", 0, true},
		{"state-stop-renewed.json", 2516, "14798f4aa1a103c6a5af6b76b3c30877d922198e6e2ba7a10d77e8d364997da7", 0, true},
		{"state-stop-term-intent.json", 2514, "a21cc7486fc0c0231f49c43a7918ebfe0b2e9f372c89d40188ceda1899f9a47a", 0, true},
		{"state-stop-term-observe.json", 2515, "cf6bebe7ca3640d4b5a5493b7778efccf6cc2212165b153a2135d051c943bb74", 0, true},
		{"state-stop-unknown.json", 2608, "1eff0cc749b1d7e3fc7b320022f124bafa4a654ae93ee40674dc3e32684d38f6", 0, true},
		{"state-unknown.json", 2226, "291f93152d215540a83769b55ab94d42dcd8d65c413eaf6bac5c50dbd2c1a350", 0, true},
		{"head.json", 265, "b22a40c46ab4bcc6630b58f8f94c1bf4df6ad3f683520a64939e0b46da08f867", 1, true},
		{"cancel.json", 208, "f8411702fe5b257395e628ea055ea53bba83990f34ab67c0435364dc9bd691eb", 2, true},
		{"native-exit.json", 1199, "53b98f410b2ba168e91f6ab70fc14fc214d30bc48173fc277bf8d39abd44a868", 3, true},
		{"native-signal.json", 1254, "8199f7c6aa904a11c79048640d0b498b2e756a06441b5fc3c7211d114fff0edb", 3, true},
		{"scope-absent.json", 1082, "b1726783a25df07bf986c3ca02c29e0b525ac3a1db4e82c880d77b9ecaee3770", 4, true},
		{"negative-terminal-running.json", 2425, "1e014dbc50ae34f3d5193d44c2bfe48f96f4e1ccb8f6a4083f566ab7d8377eaf", 0, false},
	}
	for _, o := range oracles {
		t.Run(o.file, func(t *testing.T) {
			raw := fixture(t, o.file)
			sum := sha256.Sum256(raw)
			if len(raw) != o.length || hex.EncodeToString(sum[:]) != o.digest {
				t.Fatal("published byte anchor changed")
			}
			c := lifecycleCases()[o.api]
			if o.valid {
				lifecycleFrozen(t, c, raw)
			} else {
				compatibilityBad(t, c, raw, "schema_invalid")
			}
		})
	}
	lifecycleFrozen(t, lifecycleCases()[0], encode(t, lifecycleFull(t)))
	// A literal native code256 is the forbidden range witness; its source
	// fixture is a RESULT, not an observation or a sixth accepted root.
	v := lifecycleFull(t)
	v["native"] = compatibilitySource(t, "negative-native-exit.json")["native"]
	compatibilityBad(t, lifecycleCases()[0], encode(t, v), "schema_invalid")
}

// Literal per-API raw caps precede lexical/schema work; exact padded bytes are
// retained. Cross-kind tests alter a valid root's kind without hitting its cap.
func TestLifecycleCapsAndRoots(t *testing.T) {
	if SnapshotLimit != 16384 || HeadLimit != 16384 || CancelLimit != 4096 || ObservationLimit != 65536 || ScopeProofLimit != 16384 {
		t.Fatal("published raw limits changed")
	}
	for _, c := range lifecycleCases() {
		t.Run(c.file, func(t *testing.T) {
			original := fixture(t, c.file)
			for _, n := range []int{c.limit - 1, c.limit} {
				// Pad the exact remaining bytes separately, avoiding roundoff.
				padded := append(bytes.Clone(original), bytes.Repeat([]byte(" "), n-len(original))...)
				if len(padded) != n {
					t.Fatal("exact cap input not constructed")
				}
				lifecycleFrozen(t, c, padded)
			}
			for _, raw := range [][]byte{append(bytes.Clone(original), bytes.Repeat([]byte(" "), c.limit+1-len(original))...), bytes.Repeat([]byte("!"), c.limit+1)} {
				compatibilityBad(t, c, raw, "bounds_exceeded")
			}
			for _, other := range []string{"curator-run member-snapshot/1", "curator-run member-head/1", "curator-run member-cancel/1", "curator-run member-observation/1", "curator-run member-scope-proof/1", "curator-run member-request/1", "curator-run member-result/1", "curator-run member-owner/1", "unknown"} {
				v := compatibilitySource(t, c.file)
				if v["kind"] == other {
					continue
				}
				v["kind"] = other
				compatibilityBad(t, c, encode(t, v), "schema_invalid")
			}
			for _, raw := range [][]byte{nil, {}, []byte("[]"), []byte("null"), []byte("0"), []byte("true"), []byte(`"text"`), append(bytes.Clone(original), original...), append(bytes.Clone(original), 'x'), original[:len(original)-2]} {
				compatibilityBad(t, c, raw, "encoding_invalid")
			}
		})
	}
}

// Exhaustive branch inventory is chosen from the schema, not product verdicts.
// Variants are installed into uncertain to avoid unrelated phase restrictions.
func lifecycleVariants(t *testing.T, family string) []any {
	t.Helper()
	switch family {
	case "start":
		return []any{lifecycleStatus("not_attempted"), lifecycleStatus("attempted_unknown"), map[string]any{"status": "spawned", "process_incarnation_sha256": strings.Repeat("a", 64)}, map[string]any{"status": "exec_confirmed", "process_incarnation_sha256": strings.Repeat("b", 64)}, map[string]any{"status": "spawn_failed", "os_error": lifecycleSourceError()}}
	case "native":
		return []any{lifecycleStatus("not_started"), lifecycleStatus("running"), lifecycleStatus("unknown"), map[string]any{"status": "exit", "code": uint64(0), "evidence": lifecycleRef()}, map[string]any{"status": "signal", "number": uint64(1), "name": "0", "core_dumped": false, "evidence": lifecycleRef()}}
	case "cleanup":
		return []any{lifecycleStatus("not_started"), lifecycleStatus("pending"), lifecycleStatus("unknown"), map[string]any{"status": "failed", "evidence": nil}, map[string]any{"status": "failed", "evidence": lifecycleRef()}, map[string]any{"status": "confirmed_absent", "evidence": lifecycleRef()}}
	case "allocation":
		return []any{lifecycleStatus("not_requested"), map[string]any{"status": "intent", "allocation_id": strings.Repeat("a", 32)}, map[string]any{"status": "known", "allocation_id": strings.Repeat("b", 32)}, map[string]any{"status": "unknown", "allocation_id": strings.Repeat("c", 32)}}
	case "wait":
		return []any{map[string]any{"status": "exit", "code": uint64(0), "raw_status": uint64(0)}, map[string]any{"status": "signal", "number": uint64(1), "name": "0", "core_dumped": false, "raw_status": uint64(0)}}
	default:
		t.Fatal("unknown authored branch family")
		return nil
	}
}

type lifecycleShape struct {
	label string
	c     compatibilityCase
	build func() map[string]any
}

func lifecycleShapes(t *testing.T) []lifecycleShape {
	t.Helper()
	var shapes []lifecycleShape
	for _, c := range lifecycleCases() {
		shapes = append(shapes, lifecycleShape{c.file, c, func() map[string]any {
			if c.file == "state-admitted.json" {
				return lifecycleFull(t)
			}
			return compatibilitySource(t, c.file)
		}})
	}
	for _, family := range []string{"start", "native", "cleanup", "allocation"} {
		for index := range lifecycleVariants(t, family) {
			shapes = append(shapes, lifecycleShape{fmt.Sprintf("%s/%d", family, index), lifecycleCases()[0], func() map[string]any {
				v := lifecycleFull(t)
				v[family] = lifecycleVariants(t, family)[index]
				return v
			}})
		}
	}
	for index := range lifecycleVariants(t, "wait") {
		shapes = append(shapes, lifecycleShape{fmt.Sprintf("wait/%d", index), lifecycleCases()[3], func() map[string]any {
			v := compatibilitySource(t, "native-exit.json")
			v["wait"] = lifecycleVariants(t, "wait")[index]
			return v
		}})
	}
	return shapes
}
func lifecycleObjectPaths(v map[string]any) []string {
	var out []string
	var walk func(map[string]any, string)
	walk = func(v map[string]any, path string) {
		out = append(out, path)
		for k, child := range v {
			if o, ok := child.(map[string]any); ok {
				p := k
				if path != "" {
					p = path + "." + k
				}
				walk(o, p)
			}
		}
	}
	walk(v, "")
	sort.Strings(out)
	return out
}
func lifecycleNullable(root, path, key string, o map[string]any) bool {
	return path == "" && (root == "state-admitted.json" && (key == "scope" || key == "result" || key == "error" || key == "stop") || root == "native-exit.json" && (key == "actual_tuple" || key == "argv")) || path == "identity" && (key == "producer_member_id" || key == "predecessor_invocation_id") || path == "scope" && key == "root_incarnation_sha256" || path == "error" && (key == "source" || key == "evidence") || path == "cleanup" && o["status"] == "failed" && key == "evidence"
}
func lifecycleType(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "bool"
	case json.Number, uint64:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "invalid"
	}
}

// Every required field/closed reachable union shape has deletion, null and
// incompatible scalar/array/object neighbors. Nullable nulls are valid controls,
// including failed cleanup evidence; omission is always rejected.
func TestLifecycleRecursiveClosure(t *testing.T) {
	for _, shape := range lifecycleShapes(t) {
		for _, path := range lifecycleObjectPaths(shape.build()) {
			t.Run(shape.label+"/"+path, func(t *testing.T) {
				positive := shape.build()
				compatibilityOK(t, shape.c, positive)
				for key, current := range compatibilityAt(positive, path) {
					v := shape.build()
					delete(compatibilityAt(v, path), key)
					compatibilityBad(t, shape.c, encode(t, v), "schema_invalid")
					for _, wrong := range []any{nil, false, uint64(2), "private/path/marker", []any{}, map[string]any{}} {
						if wrong != nil && lifecycleType(current) == lifecycleType(wrong) {
							continue
						}
						v = shape.build()
						compatibilityAt(v, path)[key] = wrong
						if wrong == nil && lifecycleNullable(shape.c.file, path, key, compatibilityAt(positive, path)) {
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

// Every then-property gets a single otherwise-structural contrary value. The
// independent phase table mirrors source constraints, not decoder internals.
func TestLifecyclePhaseConditionals(t *testing.T) {
	c := lifecycleCases()[0]
	rows := []struct {
		phase  string
		fields map[string]any
	}{
		{"admitted", map[string]any{"start": lifecycleStatus("not_attempted"), "native": lifecycleStatus("not_started"), "cleanup": lifecycleStatus("not_started"), "scope": nil, "result": nil, "allocation": lifecycleStatus("not_requested")}},
		{"allocating", map[string]any{"allocation": map[string]any{"status": "intent", "allocation_id": strings.Repeat("a", 32)}, "scope": nil, "start": lifecycleStatus("not_attempted"), "native": lifecycleStatus("not_started"), "cleanup": lifecycleStatus("pending"), "result": nil, "stop": nil, "gate": "not_created"}},
		{"allocated", map[string]any{"allocation": map[string]any{"status": "known", "allocation_id": strings.Repeat("a", 32)}, "scope": lifecycleScope(t), "start": lifecycleStatus("not_attempted"), "native": lifecycleStatus("not_started"), "cleanup": lifecycleStatus("pending"), "result": nil, "stop": nil, "gate": "not_created"}},
		{"start_intent", map[string]any{"start": lifecycleStatus("attempted_unknown"), "native": lifecycleStatus("unknown"), "scope": lifecycleScope(t), "cleanup": lifecycleStatus("pending"), "result": nil}},
		{"spawned", map[string]any{"start": map[string]any{"status": "spawned", "process_incarnation_sha256": strings.Repeat("a", 64)}, "native": lifecycleStatus("unknown"), "scope": lifecycleScope(t), "cleanup": lifecycleStatus("pending"), "result": nil}},
		{"running", map[string]any{"start": map[string]any{"status": "exec_confirmed", "process_incarnation_sha256": strings.Repeat("a", 64)}, "native": lifecycleStatus("running"), "scope": lifecycleScope(t), "cleanup": lifecycleStatus("pending"), "result": nil}},
		{"terminal", map[string]any{"native": lifecycleStatus("not_started"), "cleanup": lifecycleStatus("not_started"), "result": lifecycleRef()}},
		{"stopping", map[string]any{"stop": lifecycleStop(t), "result": nil}},
		{"uncertain", map[string]any{}},
	}
	for _, row := range rows {
		t.Run(row.phase, func(t *testing.T) {
			build := func() map[string]any {
				v := lifecycleFull(t)
				v["phase"] = row.phase
				for k, x := range row.fields {
					v[k] = x
				}
				return v
			}
			lifecycleFrozen(t, c, encode(t, build()))
			for key, current := range row.fields {
				var contrary any
				switch key {
				case "scope":
					if current == nil {
						contrary = lifecycleScope(t)
					}
				case "result":
					if current == nil {
						contrary = lifecycleRef()
					}
				case "stop":
					if current == nil {
						contrary = lifecycleStop(t)
					}
				case "gate":
					contrary = "held"
				case "start":
					contrary = lifecycleStatus("not_attempted")
					if current.(map[string]any)["status"] == "not_attempted" {
						contrary = lifecycleStatus("attempted_unknown")
					}
				case "native":
					contrary = lifecycleStatus("unknown")
					if current.(map[string]any)["status"] == "unknown" {
						contrary = lifecycleStatus("not_started")
					}
				case "cleanup":
					contrary = lifecycleStatus("pending")
					if current.(map[string]any)["status"] == "pending" {
						contrary = lifecycleStatus("not_started")
					}
				case "allocation":
					contrary = lifecycleStatus("not_requested")
					if current.(map[string]any)["status"] == "not_requested" {
						contrary = map[string]any{"status": "intent", "allocation_id": strings.Repeat("a", 32)}
					}
				default:
					t.Fatal("unhandled authored constraint")
				}
				v := build()
				v[key] = contrary
				compatibilityBad(t, c, encode(t, v), "schema_invalid")
			}
			// All gate values remain valid in phases with no gate conditional.
			if _, constrained := row.fields["gate"]; !constrained {
				for _, gate := range []string{"not_created", "held", "release_intent", "released_or_unknown", "closed"} {
					v := build()
					v["gate"] = gate
					compatibilityOK(t, c, v)
				}
			}
			// Explicitly probe each unlisted phase property, preventing accidental
			// strengthening into transition, scope or terminal-evidence policy.
			for _, key := range []string{"start", "native", "cleanup", "allocation", "scope", "result", "stop", "error"} {
				if _, constrained := row.fields[key]; constrained {
					continue
				}
				values := []any{nil}
				switch key {
				case "start", "native", "cleanup", "allocation":
					values = lifecycleVariants(t, key)
				case "scope":
					values = append(values, lifecycleScope(t))
				case "result":
					values = append(values, lifecycleRef())
				case "stop":
					values = append(values, lifecycleStop(t))
				case "error":
					values = append(values, lifecycleError())
				}
				for _, value := range values {
					v := build()
					v[key] = value
					compatibilityOK(t, c, v)
				}
			}
		})
	}
	// Terminal permits exactly these native/cleanup alternatives. Native running
	// and unknown, or pending/unknown/failed cleanup, must not be terminal data.
	for _, n := range []any{lifecycleStatus("not_started"), map[string]any{"status": "exit", "code": uint64(255), "evidence": lifecycleRef()}, map[string]any{"status": "signal", "number": uint64(255), "name": "Z9", "core_dumped": true, "evidence": lifecycleRef()}} {
		for _, cl := range []any{lifecycleStatus("not_started"), map[string]any{"status": "confirmed_absent", "evidence": lifecycleRef()}} {
			v := lifecycleFull(t)
			v["phase"], v["native"], v["cleanup"] = "terminal", n, cl
			compatibilityOK(t, c, v)
		}
	}
	for _, key := range []string{"native", "cleanup"} {
		for _, value := range lifecycleVariants(t, key) {
			status := value.(map[string]any)["status"]
			if key == "native" && (status == "not_started" || status == "exit" || status == "signal") || key == "cleanup" && (status == "not_started" || status == "confirmed_absent") {
				continue
			}
			v := lifecycleFull(t)
			v["phase"] = "terminal"
			v[key] = value
			compatibilityBad(t, c, encode(t, v), "schema_invalid")
		}
	}
	v := lifecycleFull(t)
	v["phase"] = "unknown"
	compatibilityBad(t, c, encode(t, v), "schema_invalid")
	v = lifecycleFull(t)
	v["gate"] = "unknown"
	compatibilityBad(t, c, encode(t, v), "schema_invalid")
}

// Grammar neighbors traverse every populated occurrence, not just one shared
// helper. Expectations depend on field/path definitions, never a second decoder.
func TestLifecycleGrammarsAndRanges(t *testing.T) {
	for _, shape := range lifecycleShapes(t) {
		for _, path := range lifecycleObjectPaths(shape.build()) {
			for key := range compatibilityAt(shape.build(), path) {
				var good, bad []any
				switch {
				case strings.HasSuffix(key, "_sha256"):
					good = []any{strings.Repeat("0", 64), strings.Repeat("f", 64)}
					bad = []any{"", strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64)}
				case key == "sha256":
					good = []any{strings.Repeat("0", 64), strings.Repeat("f", 64)}
					bad = []any{"", strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64)}
				case key == "adapter_revision":
					good = []any{strings.Repeat("a", 40), strings.Repeat("f", 64)}
					bad = []any{"", strings.Repeat("a", 39), strings.Repeat("a", 41), strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 40), strings.Repeat("g", 64)}
				case key == "adapter_id" || key == "component" || key == "namespace" || key == "name" && path != "wait" && path != "native":
					good = []any{"a", "0", "A_-.9", strings.Repeat("a", 64)}
					bad = []any{"", strings.Repeat("a", 65), ".a", "-a", "_a", "a/b", "a b", "a\n", "é"}
				case strings.HasSuffix(key, "_id") || key == "id":
					good = []any{strings.Repeat("0", 32), strings.Repeat("f", 32)}
					bad = []any{"", strings.Repeat("a", 31), strings.Repeat("a", 33), strings.Repeat("A", 32), strings.Repeat("g", 32)}
				case key == "length":
					good = []any{uint64(0), uint64(4194304)}
					bad = []any{uint64(4194305)}
				case key == "generation":
					good = []any{uint64(1), uint64(256)}
					bad = []any{uint64(0), uint64(257)}
				case key == "observed_monotonic_ms" || key == "begun_ms":
					good = []any{uint64(0), uint64(9007199254740991)}
				case key == "requested_at_ms" || key == "deadline_monotonic_ms" || key == "term_deadline_ms" || key == "kill_deadline_ms":
					good = []any{uint64(1), uint64(9007199254740991)}
					bad = []any{uint64(0)}
				case key == "raw_status":
					good = []any{uint64(0), uint64(4294967295)}
					bad = []any{uint64(4294967296)}
				case key == "number":
					good = []any{uint64(1), uint64(255)}
					bad = []any{uint64(0), uint64(256)}
				case key == "code" && (path == "wait" || path == "native"):
					good = []any{uint64(0), uint64(255)}
					bad = []any{uint64(256)}
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
					if key == "observed_monotonic_ms" || key == "begun_ms" || key == "requested_at_ms" || key == "deadline_monotonic_ms" || key == "term_deadline_ms" || key == "kill_deadline_ms" {
						v := shape.build()
						compatibilityAt(v, path)[key] = uint64(9007199254740992)
						compatibilityBad(t, shape.c, encode(t, v), "bounds_exceeded")
					}
				})
			}
		}
	}
}

// Every allowed stage/code pair and every invalid cross-pair is explicit source
// data. Source/evidence are independently nullable, never optional omissions.
func TestLifecycleErrorData(t *testing.T) {
	rows := []struct {
		stage string
		codes []string
	}{
		{"bootstrap", []string{"transport_unavailable", "descriptor_invalid"}},
		{"parse", []string{"encoding_invalid", "schema_invalid", "bounds_exceeded"}},
		{"admission", []string{"authorization_invalid", "authorization_expired", "input_mismatch", "identity_mismatch", "invocation_conflict", "assignment_conflict"}},
		{"compatibility", []string{"compatibility_unpublished", "compatibility_mismatch", "configuration_unknown", "presence_contamination", "history_contamination"}},
		{"store", []string{"store_busy", "store_unreadable", "store_full", "store_interlocked"}},
		{"start", []string{"spawn_failed"}}, {"runtime", []string{"supervisor_lost"}},
		{"result", []string{"result_invalid", "result_overflow", "result_conflict"}},
		{"cleanup", []string{"scope_unknown", "cleanup_failed"}}, {"consume", []string{"consumption_conflict", "effect_unknown"}}, {"cancel", []string{"cancel_conflict"}},
	}
	c := lifecycleCases()[0]
	for _, row := range rows {
		for _, other := range rows {
			for _, code := range other.codes {
				v := lifecycleFull(t)
				e := lifecycleError()
				e["stage"], e["code"] = row.stage, code
				v["error"] = e
				if row.stage != other.stage {
					compatibilityBad(t, c, encode(t, v), "schema_invalid")
					continue
				}
				for _, source := range []any{nil, lifecycleSourceError()} {
					for _, evidence := range []any{nil, lifecycleRef()} {
						e["source"], e["evidence"] = source, evidence
						lifecycleFrozen(t, c, encode(t, v))
					}
				}
			}
		}
	}
	for _, key := range []string{"stage", "code"} {
		v := lifecycleFull(t)
		v["error"].(map[string]any)[key] = "unknown"
		compatibilityBad(t, c, encode(t, v), "schema_invalid")
	}
	for _, path := range []string{"error.source", "start.os_error"} {
		for _, value := range []string{"A", "0", ".", "_", ":", "-", "aZ09_.:-", strings.Repeat(":", 128)} {
			v := lifecycleFull(t)
			v["start"] = map[string]any{"status": "spawn_failed", "os_error": lifecycleSourceError()}
			compatibilityAt(v, path)["code"] = value
			compatibilityOK(t, c, v)
		}
		for _, value := range []string{"", strings.Repeat("a", 129), "a/b", "a b", "é", "a\n", "a@b", "a+1"} {
			v := lifecycleFull(t)
			v["start"] = map[string]any{"status": "spawn_failed", "os_error": lifecycleSourceError()}
			compatibilityAt(v, path)["code"] = value
			compatibilityBad(t, c, encode(t, v), "schema_invalid")
		}
	}
}

// Exact discriminators close each union: no unknown mode or cross-branch field
// may be interpreted as absence, an exit or successful cleanup.
func TestLifecycleUnionSelection(t *testing.T) {
	for _, family := range []string{"start", "native", "cleanup", "allocation", "wait"} {
		c := lifecycleCases()[0]
		if family == "wait" {
			c = lifecycleCases()[3]
		}
		build := func() map[string]any {
			if family == "wait" {
				return compatibilitySource(t, "native-exit.json")
			}
			return lifecycleFull(t)
		}
		for _, variant := range lifecycleVariants(t, family) {
			v := build()
			v[family] = variant
			compatibilityOK(t, c, v)
			keys := map[string]bool{}
			for _, other := range lifecycleVariants(t, family) {
				for key := range other.(map[string]any) {
					keys[key] = true
				}
			}
			for key := range keys {
				if _, exists := variant.(map[string]any)[key]; exists {
					continue
				}
				v = build()
				m := map[string]any{}
				for k, x := range variant.(map[string]any) {
					m[k] = x
				}
				m[key] = nil
				v[family] = m
				compatibilityBad(t, c, encode(t, v), "schema_invalid")
			}
			v = build()
			m := map[string]any{}
			for k, x := range variant.(map[string]any) {
				m[k] = x
			}
			m["status"] = "unknown-private-marker"
			v[family] = m
			compatibilityBad(t, c, encode(t, v), "schema_invalid")
		}
	}
	c := lifecycleCases()[0]
	for _, role := range []string{"supervisor", "reconciler"} {
		v := lifecycleFull(t)
		v["writer"].(map[string]any)["role"] = role
		compatibilityOK(t, c, v)
	}
	for _, role := range []string{"development", "source_review", "recipient_assessment"} {
		v := lifecycleFull(t)
		v["identity"].(map[string]any)["role"] = role
		compatibilityOK(t, c, v)
	}
	for _, path := range []string{"writer.role", "identity.role"} {
		v := lifecycleFull(t)
		compatibilitySet(v, path, "unknown")
		compatibilityBad(t, c, encode(t, v), "schema_invalid")
	}
	for _, role := range []string{"supervisor", "reconciler"} {
		v := compatibilitySource(t, "scope-absent.json")
		v["observer"].(map[string]any)["role"] = role
		compatibilityOK(t, lifecycleCases()[4], v)
	}
	v := compatibilitySource(t, "scope-absent.json")
	v["observer"].(map[string]any)["role"] = "unknown"
	compatibilityBad(t, lifecycleCases()[4], encode(t, v), "schema_invalid")
	for _, key := range []string{"reason", "stage"} {
		values := []string{"controller_request", "deadline", "controller_lost", "supervisor_signal", "descendants_remaining", "overflow"}
		if key == "stage" {
			values = []string{"term_intent", "term_observe", "kill_intent", "kill_observe", "complete", "unknown"}
		}
		for _, value := range values {
			v := lifecycleFull(t)
			v["stop"].(map[string]any)[key] = value
			compatibilityOK(t, c, v)
		}
		v := lifecycleFull(t)
		v["stop"].(map[string]any)[key] = "unrecognized"
		compatibilityBad(t, c, encode(t, v), "schema_invalid")
	}
	for _, value := range []string{"controller_request", "deadline", "controller_lost", "supervisor_signal"} {
		v := compatibilitySource(t, "cancel.json")
		v["source"] = value
		compatibilityOK(t, lifecycleCases()[2], v)
	}
	for _, value := range []string{"unknown", "descendants_remaining", "overflow"} {
		v := compatibilitySource(t, "cancel.json")
		v["source"] = value
		compatibilityBad(t, lifecycleCases()[2], encode(t, v), "schema_invalid")
	}
}

// Signal spelling is ASCII uppercase/digits, not an OS signal-name registry.
// False core_dumped and all eight proof-bit combinations are positive data.
func TestLifecycleSignalsAndUntrustedProofs(t *testing.T) {
	for _, family := range []string{"native", "wait"} {
		c := lifecycleCases()[0]
		if family == "wait" {
			c = lifecycleCases()[3]
		}
		build := func() map[string]any {
			if family == "wait" {
				return compatibilitySource(t, "native-signal.json")
			}
			v := lifecycleFull(t)
			v["native"] = map[string]any{"status": "signal", "number": uint64(15), "name": "SIGTERM", "core_dumped": false, "evidence": lifecycleRef()}
			return v
		}
		for _, name := range []string{"0", "A", "Z9", strings.Repeat("A", 32)} {
			for _, core := range []bool{false, true} {
				v := build()
				o := v[family].(map[string]any)
				o["name"], o["core_dumped"] = name, core
				compatibilityOK(t, c, v)
			}
		}
		for _, name := range []string{"", strings.Repeat("A", 33), "SigTERM", "sigterm", "SIG_TERM", "SIG-TERM", "Σ", "A\n"} {
			v := build()
			v[family].(map[string]any)["name"] = name
			compatibilityBad(t, c, encode(t, v), "schema_invalid")
		}
	}
	for mask := 0; mask < 8; mask++ {
		v := compatibilitySource(t, "scope-absent.json")
		for i, key := range []string{"observation_complete", "scope_matches", "empty"} {
			v[key] = mask&(1<<i) != 0
		}
		lifecycleFrozen(t, lifecycleCases()[4], encode(t, v))
	}
	// No source-level clock, identity, raw-status, evidence or semantic role
	// comparison is smuggled into these structural positives.
	v := lifecycleFull(t)
	v["phase"] = "stopping"
	v["result"] = nil
	stop := v["stop"].(map[string]any)
	stop["begun_ms"], stop["term_deadline_ms"], stop["kill_deadline_ms"] = uint64(9007199254740991), uint64(2), uint64(1)
	stop["boot_sha256"] = strings.Repeat("0", 64)
	v["deadline_monotonic_ms"] = uint64(1)
	v["identity"].(map[string]any)["producer_member_id"] = nil
	lifecycleFrozen(t, lifecycleCases()[0], encode(t, v))
	o := compatibilitySource(t, "native-exit.json")
	o["wait"].(map[string]any)["raw_status"] = uint64(4294967295)
	o["observed_monotonic_ms"] = uint64(0)
	o["scope"].(map[string]any)["boot_sha256"] = strings.Repeat("0", 64)
	o["actual_tuple"] = nil
	lifecycleFrozen(t, lifecycleCases()[3], encode(t, o))
}

// Unknown argv (null), known empty argv and bounded exact strings are distinct
// retained data. Unicode scalar bounds never override the stricter global byte
// cap; no actual process arguments are read, logged or claimed by these inputs.
func TestLifecycleObservationArguments(t *testing.T) {
	c := lifecycleCases()[3]
	for _, n := range []int{0, 1, 128, 129} {
		v := compatibilitySource(t, c.file)
		args := make([]any, n)
		for i := range args {
			args[i] = "fixture"
		}
		v["argv"] = args
		if n <= 128 {
			lifecycleFrozen(t, c, encode(t, v))
		} else {
			compatibilityBad(t, c, encode(t, v), "schema_invalid")
		}
	}
	for _, tuple := range []any{nil, lifecycleRef()} {
		for _, args := range []any{nil, []any{}} {
			v := compatibilitySource(t, c.file)
			v["actual_tuple"], v["argv"] = tuple, args
			r := compatibilityOK(t, c, v)
			if (r.Value()["argv"] == nil) != (args == nil) || (r.Value()["actual_tuple"] == nil) != (tuple == nil) {
				t.Fatal("null/empty unavailable data normalized")
			}
			lifecycleFrozen(t, c, encode(t, v))
		}
	}
	for _, good := range []string{"a", strings.Repeat("a", 4096), strings.Repeat("é", 2048), strings.Repeat("𝄞", 1024), "e\u0301", "\n"} {
		v := compatibilitySource(t, c.file)
		v["argv"] = []any{good}
		r := compatibilityOK(t, c, v)
		if r.Value()["argv"].([]any)[0] != good {
			t.Fatal("argument changed")
		}
		lifecycleFrozen(t, c, encode(t, v))
	}
	for _, bad := range []any{"", true, uint64(1), nil, []any{}, map[string]any{}} {
		v := compatibilitySource(t, c.file)
		v["argv"] = []any{bad}
		compatibilityBad(t, c, encode(t, v), "schema_invalid")
	}
	for _, bad := range []string{strings.Repeat("a", 4097), strings.Repeat("é", 2048) + "a", strings.Repeat("𝄞", 1025)} {
		v := compatibilitySource(t, c.file)
		v["argv"] = []any{bad}
		compatibilityBad(t, c, encode(t, v), "bounds_exceeded")
	}
	for _, pair := range []struct{ escaped, literal string }{
		{strings.Repeat(`\u0061`, 4096), strings.Repeat("a", 4096)},
		{strings.Repeat(`\u00e9`, 2048), strings.Repeat("é", 2048)},
		{strings.Repeat(`\ud834\udd1e`, 1024), strings.Repeat("𝄞", 1024)},
	} {
		v := compatibilitySource(t, c.file)
		v["argv"] = []any{"REPLACE"}
		raw := strings.Replace(string(encode(t, v)), `"REPLACE"`, `"`+pair.escaped+`"`, 1)
		if strings.Contains(raw, "REPLACE") {
			t.Fatal("escaped positive not constructed")
		}
		r, e := c.decode([]byte(raw))
		if e != nil || r == nil || r.Value()["argv"].([]any)[0] != pair.literal {
			t.Fatal("escaped argv changed/refused", e)
		}
		lifecycleFrozen(t, c, []byte(raw))
	}
}

// Each named API routes strict lexical/encoding failures through exact static
// nil refusals; unknown private markers are never reflected or partially echoed.
// Existing unchanged depth/node/global-byte tests provide the counting suite.
func TestLifecycleStrictEntryRefusals(t *testing.T) {
	fields := []string{"generation", "generation", "requested_at_ms", "observed_monotonic_ms", "observed_monotonic_ms"}
	for index, c := range lifecycleCases() {
		t.Run(c.file, func(t *testing.T) {
			v := compatibilitySource(t, c.file)
			if index == 0 {
				v = lifecycleFull(t)
			}
			v[fields[index]] = uint64(1)
			raw := string(encode(t, v))
			good := strings.Replace(raw, `"kind":`, `"k\u0069nd":`, 1)
			if good == raw {
				t.Fatal("escaped positive not applied")
			}
			lifecycleFrozen(t, c, []byte(good))
			kind := v["kind"].(string)
			duplicate := `{"kind":"` + kind + `","k\u0069nd":"` + kind + `",` + raw[1:]
			compatibilityBad(t, c, []byte(duplicate), "schema_invalid")
			if index != 2 {
				needle, replacement := `"scope":{`, `"scope":{"adapter_id":"fixture-scope","adap\u0074er_id":"fixture-scope",`
				if index == 1 {
					needle, replacement = `"snapshot":{`, `"snapshot":{"name":"state-final","na\u006de":"state-final",`
				}
				nested := strings.Replace(raw, needle, replacement, 1)
				if nested == raw {
					t.Fatal("nested duplicate not applied")
				}
				compatibilityBad(t, c, []byte(nested), "schema_invalid")
			}
			for _, bad := range [][]byte{append([]byte{0xef, 0xbb, 0xbf}, []byte(raw)...), append([]byte{0xff}, []byte(raw)...), []byte(strings.Replace(raw, `"kind":`, "\"kind\":\x00", 1)), []byte(`{"x":"\ud800"}`), []byte(`{"x":"\udc00"}`), []byte(`{"x":"\u0000"}`)} {
				compatibilityBad(t, c, bad, "encoding_invalid")
			}
			private := `{"private/path/secret-marker":"private/path/secret-marker",` + raw[1:]
			compatibilityBad(t, c, []byte(private), "schema_invalid")
			for _, number := range []string{"-0", "-1", "0.0", "1e0", "1E+2", "01", "+1", "NaN", "Infinity", "9007199254740992"} {
				needle := `"` + fields[index] + `":1`
				bad := strings.Replace(raw, needle, `"`+fields[index]+`":`+number, 1)
				if bad == raw {
					t.Fatal("numeric neighbor not constructed")
				}
				code := "schema_invalid"
				if number == "9007199254740992" {
					code = "bounds_exceeded"
				}
				compatibilityBad(t, c, []byte(bad), code)
			}
			for _, n := range []int{4096, 4097} {
				bad := []byte(`{"private":"` + strings.Repeat("a", n) + `",` + raw[1:])
				code := "schema_invalid"
				if n > 4096 || len(bad) > c.limit {
					code = "bounds_exceeded"
				}
				compatibilityBad(t, c, bad, code)
			}
		})
	}
}
