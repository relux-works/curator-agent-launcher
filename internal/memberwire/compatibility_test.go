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

type compatibilityCase struct {
	file   string
	limit  int
	decode func([]byte) (*UntrustedRecord, *Error)
}

func compatibilityCases() []compatibilityCase {
	return []compatibilityCase{
		{"owner.json", 4096, DecodeOwner},
		{"catalog-empty.json", 65536, DecodeCatalog},
		{"tuple.json", 65536, DecodeTuple},
	}
}

// Standard JSON is used only to construct test inputs and independently freeze
// published values. It is never the strict acceptance/refusal oracle.
func compatibilitySource(t *testing.T, file string) map[string]any {
	t.Helper()
	var v map[string]any
	d := json.NewDecoder(bytes.NewReader(fixture(t, file)))
	d.UseNumber()
	if e := d.Decode(&v); e != nil {
		t.Fatal(e)
	}
	return v
}
func compatibilityAt(v map[string]any, path string) map[string]any {
	var cur any = v
	if path != "" {
		for _, key := range strings.Split(path, ".") {
			if key == "0" {
				cur = cur.([]any)[0]
			} else {
				cur = cur.(map[string]any)[key]
			}
		}
	}
	return cur.(map[string]any)
}
func compatibilitySet(v map[string]any, path string, value any) {
	i := strings.LastIndex(path, ".")
	if i < 0 {
		v[path] = value
	} else {
		compatibilityAt(v, path[:i])[path[i+1:]] = value
	}
}
func compatibilityRequiredManager() map[string]any {
	return map[string]any{
		"mode": "required", "id": "fixture-manager", "protocol": "fixture-protocol",
		"registration_sha256": strings.Repeat("a", 64), "transport_binding_sha256": strings.Repeat("b", 64),
		"capabilities_sha256": strings.Repeat("c", 64), "compatibility_sha256": strings.Repeat("d", 64),
	}
}
func compatibilityCatalog(t *testing.T) map[string]any {
	t.Helper()
	v := compatibilitySource(t, "catalog-empty.json")
	v["entries"] = []any{map[string]any{
		"tuple":                        map[string]any{"name": "tuple", "length": uint64(3470), "sha256": "80fdb1ade7796542a4cf5bedbc6f490e7b7f16bc2379944b11b46a98af23c25c"},
		"owner_inputs":                 compatibilitySource(t, "tuple.json")["owner_inputs"],
		"construction_contract_sha256": strings.Repeat("a", 64), "private_verification_contract_sha256": strings.Repeat("b", 64), "scope_contract_sha256": strings.Repeat("c", 64),
	}}
	return v
}
func compatibilityOK(t *testing.T, c compatibilityCase, v any) *UntrustedRecord {
	t.Helper()
	r, e := c.decode(encode(t, v))
	if e != nil || r == nil {
		t.Fatalf("structural positive refused: %v", e)
	}
	return r
}
func compatibilityBad(t *testing.T, c compatibilityCase, raw []byte, code string) {
	t.Helper()
	before := bytes.Clone(raw)
	r, e := c.decode(raw)
	if r != nil || e == nil || e.Stage() != "parse" || e.Code() != code || e.Error() != "parse/"+code || len(e.Error()) > 256 || !bytes.Equal(raw, before) {
		t.Fatalf("wanted nil/static parse/%s with unchanged input; record=%t error=%v", code, r != nil, e)
	}
}

// Expectations predate decoding and all caller mutations. Full values come from
// independently frozen input bytes, not another possibly aliased product view.
func compatibilitySnapshot(t *testing.T, c compatibilityCase, input []byte) {
	t.Helper()
	original := bytes.Clone(input)
	wantLength, wantSum := len(original), sha256.Sum256(original)
	var expected any
	d := json.NewDecoder(bytes.NewReader(original))
	d.UseNumber()
	if e := d.Decode(&expected); e != nil {
		t.Fatal(e)
	}
	wantValue := encode(t, expected)
	r, e := c.decode(input)
	if e != nil || r == nil {
		t.Fatal("snapshot positive refused", e)
	}
	earlier := r.Value()
	check := func() {
		if r.Length() != wantLength || r.SHA256() != wantSum || !bytes.Equal(r.Raw(), original) || !bytes.Equal(encode(t, r.Value()), wantValue) || !bytes.Equal(encode(t, earlier), wantValue) {
			t.Fatal("snapshot differs from independent pre-mutation raw/value expectations")
		}
	}
	check()
	input[0] = '!'
	rawView := r.Raw()
	rawView[0] = '?'
	changed := r.Value()
	changed["kind"] = "changed"
	if c.file == "tuple.json" {
		owners := changed["owner_inputs"].([]any)
		owners[0].(map[string]any)["document"].(map[string]any)["name"] = "changed"
		owners[1] = nil // Replace an EXISTING array element, not merely append.
		if owners[0].(map[string]any)["document"].(map[string]any)["name"] != "changed" || owners[1] != nil {
			t.Fatal("nested tuple mutation control did not fire")
		}
	} else if c.file == "catalog-empty.json" && len(changed["entries"].([]any)) > 0 {
		entries := changed["entries"].([]any)
		entry := entries[0].(map[string]any)
		entry["tuple"].(map[string]any)["name"] = "changed"
		owners := entry["owner_inputs"].([]any)
		owners[0].(map[string]any)["document"].(map[string]any)["name"] = "changed"
		owners[1] = nil
		entries[0] = nil
		if entry["tuple"].(map[string]any)["name"] != "changed" || owners[0].(map[string]any)["document"].(map[string]any)["name"] != "changed" || owners[1] != nil || entries[0] != nil {
			t.Fatal("nested catalog mutation control did not fire")
		}
	}
	if input[0] != '!' || rawView[0] != '?' || changed["kind"] != "changed" {
		t.Fatal("caller mutation control did not fire")
	}
	check()
}

// Fixed raw hashes/lengths pin the three inert publications independently of
// product parsing. Structural success supplies no real compatibility catalog.
func TestCompatibilityPublishedSnapshots(t *testing.T) {
	oracles := []struct {
		length int
		digest string
	}{
		{251, "26bc7289b2225398ae27fd48df20208cd691ab2317db2cdd6ff74870952046d2"},
		{167, "db1e35b69bc15a44636471febdf1f47d14905eb306a19318c64c47e87de0c7ef"},
		{3470, "80fdb1ade7796542a4cf5bedbc6f490e7b7f16bc2379944b11b46a98af23c25c"},
	}
	for i, c := range compatibilityCases() {
		t.Run(c.file, func(t *testing.T) {
			raw := fixture(t, c.file)
			sum := sha256.Sum256(raw)
			if len(raw) != oracles[i].length || hex.EncodeToString(sum[:]) != oracles[i].digest {
				t.Fatal("published bytes differ from frozen oracle")
			}
			compatibilitySnapshot(t, c, raw)
		})
	}
	// A synthetic nonempty catalog exercises existing entry and nested owner-list
	// elements. It is memory-only test data, never an admitted/native catalog.
	compatibilitySnapshot(t, compatibilityCases()[1], encode(t, compatibilityCatalog(t)))
}

// Every API has its own inclusive raw bound before encoding or schema work.
// Exact padded bytes (including trailing whitespace) remain owned and hashed.
func TestCompatibilityRawBoundsAndRoots(t *testing.T) {
	if OwnerLimit != 4096 || CatalogLimit != 65536 || TupleLimit != 65536 {
		t.Fatal("published raw bounds changed")
	}
	for _, c := range compatibilityCases() {
		t.Run(c.file, func(t *testing.T) {
			original := fixture(t, c.file)
			for _, size := range []int{c.limit - 1, c.limit} {
				raw := append(bytes.Clone(original), bytes.Repeat([]byte(" "), size-len(original))...)
				compatibilitySnapshot(t, c, raw)
			}
			for _, raw := range [][]byte{append(bytes.Clone(original), bytes.Repeat([]byte(" "), c.limit+1-len(original))...), bytes.Repeat([]byte("!"), c.limit+1)} {
				compatibilityBad(t, c, raw, "bounds_exceeded")
			}
			for _, other := range compatibilityCases() {
				if c.file != other.file {
					compatibilityBad(t, c, fixture(t, other.file), "schema_invalid")
				}
			}
			compatibilityBad(t, c, fixture(t, "request-inspect.json"), "schema_invalid")
			v := compatibilitySource(t, c.file)
			v["kind"] = "unknown-private-marker"
			compatibilityBad(t, c, encode(t, v), "schema_invalid")
			for _, raw := range [][]byte{nil, {}, []byte("[]"), []byte("null"), []byte("1"), []byte("true"), append(bytes.Clone(original), original...), append(bytes.Clone(original), 'x'), original[:len(original)-2]} {
				compatibilityBad(t, c, raw, "encoding_invalid")
			}
		})
	}
}

// Each reachable closed shape has every required field removed and every field
// replaced by null and all incompatible JSON type classes, plus an unknown key.
// Both manager/effort variants are positive before their independent neighbors.
func TestCompatibilityRecursiveClosureAndTypes(t *testing.T) {
	for _, c := range compatibilityCases() {
		variants := 1
		if c.file == "tuple.json" {
			variants = 2
		}
		for variant := 0; variant < variants; variant++ {
			base := func() map[string]any {
				if c.file == "catalog-empty.json" {
					return compatibilityCatalog(t)
				}
				v := compatibilitySource(t, c.file)
				if variant == 1 {
					v["manager"] = compatibilityRequiredManager()
					v["runtime"].(map[string]any)["effort"] = map[string]any{"mode": "not_applicable", "evidence_sha256": strings.Repeat("a", 64)}
				}
				return v
			}
			paths := []string{""}
			if c.file == "tuple.json" {
				paths = append(paths, "launcher", "curator", "agents_management", "runtime", "runtime.effort", "profile", "home", "manager", "owner_inputs.0", "owner_inputs.0.document")
			} else if c.file == "catalog-empty.json" {
				paths = append(paths, "entries.0", "entries.0.tuple", "entries.0.owner_inputs.0", "entries.0.owner_inputs.0.document")
			}
			for _, path := range paths {
				t.Run(fmt.Sprintf("%s/%d/%s", c.file, variant, path), func(t *testing.T) {
					positive := base()
					compatibilityOK(t, c, positive)
					keys := make([]string, 0)
					for key := range compatibilityAt(positive, path) {
						keys = append(keys, key)
					}
					sort.Strings(keys)
					for _, key := range keys {
						v := base()
						delete(compatibilityAt(v, path), key)
						compatibilityBad(t, c, encode(t, v), "schema_invalid")
						for _, wrong := range []any{nil, true, uint64(2), "private-marker", []any{}, map[string]any{}} {
							// Same-type value boundaries are covered separately. Here
							// every incompatible scalar/array/object/null type refuses.
							current := compatibilityAt(positive, path)[key]
							sameType := false
							switch current.(type) {
							case string:
								_, sameType = wrong.(string)
							case json.Number:
								_, sameType = wrong.(uint64)
							case uint64:
								_, sameType = wrong.(uint64)
							case []any:
								_, sameType = wrong.([]any)
							case map[string]any:
								_, sameType = wrong.(map[string]any)
							}
							if sameType {
								continue
							}
							v = base()
							compatibilityAt(v, path)[key] = wrong
							compatibilityBad(t, c, encode(t, v), "schema_invalid")
						}
					}
					v := base()
					compatibilityAt(v, path)["unknown"] = "private/path/secret-marker"
					compatibilityBad(t, c, encode(t, v), "schema_invalid")
				})
			}
		}
	}
}

type compatibilityField struct {
	path      string
	good, bad []any
}

func compatibilityGrammarFields() []compatibilityField {
	var fields []compatibilityField
	for _, path := range []string{"launcher.revision", "curator.revision", "agents_management.revision", "runtime.adapter_revision", "profile.revision", "owner_inputs.0.revision"} {
		fields = append(fields, compatibilityField{path, []any{strings.Repeat("a", 40), strings.Repeat("f", 64)}, []any{strings.Repeat("a", 39), strings.Repeat("a", 41), strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 40), strings.Repeat("g", 64), ""}})
	}
	for _, path := range []string{"launcher.repository", "curator.repository", "owner_inputs.0.repository"} {
		fields = append(fields, compatibilityField{path, []any{"https://a", "https://A0./_-", "https://" + strings.Repeat("a", 2040)}, []any{"", "https://", "http://a", "HTTPS://a", "https://a:443", "https://a?b", "https://a#b", "https://user@a", "https://é", "https://a%20", "https://a\n", "https://" + strings.Repeat("a", 2041)}})
	}
	for _, path := range []string{"runtime.id", "runtime.vendor", "runtime.adapter_id", "profile.id", "owner_inputs.0.document.name"} {
		fields = append(fields, compatibilityField{path, []any{"a", "0", "A_-.9", strings.Repeat("a", 64)}, []any{"", strings.Repeat("a", 65), ".a", "_a", "-a", "a/b", "a b", "a\n", "é"}})
	}
	for _, path := range []string{"launcher.binary_sha256", "curator.binary_sha256", "runtime.binary_sha256", "profile.source_closure_sha256", "profile.config_sha256", "profile.lock_sha256", "profile.fragment_sha256", "profile.effective_config_sha256", "profile.permission_policy_sha256", "home.provenance_sha256", "home.native_origin_sha256", "home.auth_binding_sha256", "manager.compatibility_sha256", "owner_inputs.0.document.sha256"} {
		fields = append(fields, compatibilityField{path, []any{strings.Repeat("0", 64), strings.Repeat("f", 64)}, []any{"", strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64)}})
	}
	fields = append(fields,
		compatibilityField{"owner_inputs.0.document.length", []any{uint64(0), uint64(4194304)}, []any{uint64(4194305)}},
		compatibilityField{"home.uid", []any{uint64(1), uint64(4294967295)}, []any{uint64(0), uint64(4294967296)}},
		compatibilityField{"home.mode", []any{"managed_snapshot", "native_strict"}, []any{"unavailable", "managed", ""}},
		compatibilityField{"runtime.os", []any{"linux", "darwin"}, []any{"windows", "Linux", ""}},
		compatibilityField{"runtime.architecture", []any{"amd64", "arm64"}, []any{"x86_64", "ARM64", ""}},
		compatibilityField{"agents_management.module", []any{"github.com/relux-works/skill-agents-management"}, []any{"other", "github.com/relux-works/skill-agents-management/", ""}},
		compatibilityField{"owner_inputs.0.owner", []any{"curator"}, []any{"other", "Curator", ""}},
	)
	return fields
}

// These explicit field tables come from the pinned schema, including exact
// ASCII repository grammar and 40 OR 64 revisions. No fetch/trust is implied.
func TestCompatibilityFieldGrammars(t *testing.T) {
	tuple := compatibilityCases()[2]
	for _, field := range compatibilityGrammarFields() {
		t.Run(field.path, func(t *testing.T) {
			for _, value := range field.good {
				v := compatibilitySource(t, "tuple.json")
				compatibilitySet(v, field.path, value)
				compatibilityOK(t, tuple, v)
			}
			for _, value := range field.bad {
				v := compatibilitySource(t, "tuple.json")
				compatibilitySet(v, field.path, value)
				compatibilityBad(t, tuple, encode(t, v), "schema_invalid")
			}
		})
	}
	for _, c := range compatibilityCases()[:2] {
		fields := []compatibilityField{
			{"controller_id", []any{strings.Repeat("0", 32), strings.Repeat("f", 32)}, []any{"", strings.Repeat("a", 31), strings.Repeat("a", 33), strings.Repeat("A", 32), strings.Repeat("g", 32)}},
			{"store_id", []any{strings.Repeat("0", 32)}, []any{strings.Repeat("a", 31), strings.Repeat("a", 33), strings.Repeat("A", 32), strings.Repeat("g", 32)}},
		}
		if c.file == "owner.json" {
			fields = append(fields, compatibilityField{"uid", []any{uint64(1), uint64(4294967295)}, []any{uint64(0), uint64(4294967296)}}, compatibilityField{"catalog_sha256", []any{strings.Repeat("0", 64)}, []any{strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64)}})
		} else {
			for _, path := range []string{"entries.0.construction_contract_sha256", "entries.0.private_verification_contract_sha256", "entries.0.scope_contract_sha256", "entries.0.tuple.sha256"} {
				fields = append(fields, compatibilityField{path, []any{strings.Repeat("0", 64)}, []any{strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64)}})
			}
			// The ref/ownerInput definitions are shared, but catalog entry routing
			// must apply every one of those same field constraints as well.
			for _, f := range compatibilityGrammarFields() {
				if strings.HasPrefix(f.path, "owner_inputs.") {
					fields = append(fields, compatibilityField{"entries.0." + f.path, f.good, f.bad})
					if strings.HasPrefix(f.path, "owner_inputs.0.document.") {
						fields = append(fields, compatibilityField{strings.Replace(f.path, "owner_inputs.0.document.", "entries.0.tuple.", 1), f.good, f.bad})
					}
				}
			}
		}
		for _, field := range fields {
			for _, verdict := range []bool{true, false} {
				values := field.bad
				if verdict {
					values = field.good
				}
				for _, value := range values {
					v := compatibilitySource(t, c.file)
					if c.file == "catalog-empty.json" {
						v = compatibilityCatalog(t)
					}
					compatibilitySet(v, field.path, value)
					if verdict {
						compatibilityOK(t, c, v)
					} else {
						compatibilityBad(t, c, encode(t, v), "schema_invalid")
					}
				}
			}
		}
	}
}

// Schema free-string lengths count scalar values, not bytes or UTF-16 units.
// Literal/escaped astral characters are single values; no normalization/default
// policy restricts otherwise structural model/release/fragment/permission names.
func TestCompatibilityUnicodeStringBounds(t *testing.T) {
	c := compatibilityCases()[2]
	for _, f := range []struct {
		path string
		max  int
	}{
		{"launcher.release", 128}, {"curator.release", 128}, {"agents_management.version", 128},
		{"runtime.release", 128}, {"runtime.model", 128}, {"runtime.effort.value", 64},
		{"profile.fragment_kind", 128}, {"profile.permission_posture", 64}, {"manager.protocol", 128},
	} {
		t.Run(f.path, func(t *testing.T) {
			for _, char := range []string{"a", "é", "𝄞"} {
				for _, n := range []int{1, f.max, 0, f.max + 1} {
					v := compatibilitySource(t, "tuple.json")
					v["manager"] = compatibilityRequiredManager()
					compatibilitySet(v, f.path, strings.Repeat(char, n))
					if n >= 1 && n <= f.max {
						r := compatibilityOK(t, c, v)
						i := strings.LastIndex(f.path, ".")
						if compatibilityAt(r.Value(), f.path[:i])[f.path[i+1:]] != strings.Repeat(char, n) {
							t.Fatal("bounded Unicode field changed")
						}
					} else {
						compatibilityBad(t, c, encode(t, v), "schema_invalid")
					}
				}
			}
			v := compatibilitySource(t, "tuple.json")
			v["manager"] = compatibilityRequiredManager()
			compatibilitySet(v, f.path, "𝄞")
			raw := strings.Replace(string(encode(t, v)), "𝄞", `\ud834\udd1e`, 1)
			r, e := c.decode([]byte(raw))
			if e != nil || r == nil {
				t.Fatal("valid surrogate pair refused", e)
			}
			i := strings.LastIndex(f.path, ".")
			if compatibilityAt(r.Value(), f.path[:i])[f.path[i+1:]] != "𝄞" {
				t.Fatal("Unicode scalar was normalized or lost")
			}
		})
	}
	for _, value := range []string{"floating-alias", "e\u0301", "\n", "fixture-v4-fragment"} {
		v := compatibilitySource(t, "tuple.json")
		v["runtime"].(map[string]any)["model"] = value
		r := compatibilityOK(t, c, v) // Source shape is deliberately not support policy.
		if r.Value()["runtime"].(map[string]any)["model"] != value {
			t.Fatal("structural model string normalized or lost")
		}
	}
}

// Branches are exact closed alternatives: unavailable is neither required nor
// positively attested absence, and not-applicable effort needs its own evidence.
func TestCompatibilityManagerAndEffortBranches(t *testing.T) {
	c := compatibilityCases()[2]
	for _, manager := range []map[string]any{compatibilityRequiredManager(), {"mode": "compatible_absence", "compatibility_sha256": strings.Repeat("a", 64)}} {
		for _, effort := range []map[string]any{{"mode": "named", "value": "fixture"}, {"mode": "not_applicable", "evidence_sha256": strings.Repeat("b", 64)}} {
			v := compatibilitySource(t, "tuple.json")
			v["manager"] = manager
			v["runtime"].(map[string]any)["effort"] = effort
			compatibilityOK(t, c, v)
		}
	}
	for _, branch := range []struct {
		path string
		bad  []any
	}{
		{"manager", []any{
			map[string]any{"mode": "unavailable", "compatibility_sha256": strings.Repeat("a", 64)},
			map[string]any{"mode": "compatible_absence", "compatibility_sha256": strings.Repeat("a", 64), "id": "manager"},
		}},
		{"runtime.effort", []any{
			map[string]any{"mode": "unknown", "value": "high"},
			map[string]any{"mode": "named", "value": "high", "evidence_sha256": strings.Repeat("a", 64)},
			map[string]any{"mode": "not_applicable", "evidence_sha256": strings.Repeat("a", 64), "value": "high"},
		}},
	} {
		for _, value := range branch.bad {
			v := compatibilitySource(t, "tuple.json")
			compatibilitySet(v, branch.path, value)
			compatibilityBad(t, c, encode(t, v), "schema_invalid")
		}
	}
	for _, field := range []compatibilityField{
		{"manager.id", []any{"a", strings.Repeat("a", 64)}, []any{"", "a/b", strings.Repeat("a", 65)}},
		{"manager.registration_sha256", []any{strings.Repeat("0", 64)}, []any{strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64)}},
		{"manager.transport_binding_sha256", []any{strings.Repeat("0", 64)}, []any{strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64)}},
		{"manager.capabilities_sha256", []any{strings.Repeat("0", 64)}, []any{strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64)}},
		{"manager.compatibility_sha256", []any{strings.Repeat("0", 64)}, []any{strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64)}},
		{"runtime.effort.evidence_sha256", []any{strings.Repeat("0", 64)}, []any{strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64)}},
	} {
		for _, good := range []bool{true, false} {
			values := field.bad
			if good {
				values = field.good
			}
			for _, value := range values {
				v := compatibilitySource(t, "tuple.json")
				v["manager"] = compatibilityRequiredManager()
				v["runtime"].(map[string]any)["effort"] = map[string]any{"mode": "not_applicable", "evidence_sha256": strings.Repeat("a", 64)}
				compatibilitySet(v, field.path, value)
				if good {
					compatibilityOK(t, c, v)
				} else {
					compatibilityBad(t, c, encode(t, v), "schema_invalid")
				}
			}
		}
	}
	// Required -> absence must remove ALL required-only fields, not reinterpret
	// a registration failure. A failed/unknown mode is never normalized.
	v := compatibilitySource(t, "tuple.json")
	m := compatibilityRequiredManager()
	m["mode"] = "compatible_absence"
	v["manager"] = m
	compatibilityBad(t, c, encode(t, v), "schema_invalid")
}

// Tuple alone requires all three owners. Repeated owners and tuple refs are
// structurally allowed; duplicate publications/cross-record truth are admission.
func TestCompatibilityOwnerCoverageAndCardinality(t *testing.T) {
	tuple, catalog := compatibilityCases()[2], compatibilityCases()[1]
	for _, size := range []int{2, 3, 16, 17} {
		v := compatibilitySource(t, "tuple.json")
		original := v["owner_inputs"].([]any)
		owners := make([]any, size)
		for i := range owners {
			owners[i] = original[i%3]
		}
		v["owner_inputs"] = owners
		if size == 3 || size == 16 {
			compatibilityOK(t, tuple, v)
		} else {
			compatibilityBad(t, tuple, encode(t, v), "schema_invalid")
		}
		v = compatibilityCatalog(t)
		compatibilityAt(v, "entries.0")["owner_inputs"] = owners
		if size == 3 || size == 16 {
			compatibilityOK(t, catalog, v)
		} else {
			compatibilityBad(t, catalog, encode(t, v), "schema_invalid")
		}
	}
	for _, missing := range []string{"curator", "runtime", "launcher"} {
		v := compatibilitySource(t, "tuple.json")
		owners := v["owner_inputs"].([]any)
		replacement := "curator"
		if missing == replacement {
			replacement = "runtime"
		}
		for _, item := range owners {
			if item.(map[string]any)["owner"] == missing {
				item.(map[string]any)["owner"] = replacement
			}
		}
		compatibilityBad(t, tuple, encode(t, v), "schema_invalid") // Still THREE items.
	}
	v := compatibilitySource(t, "tuple.json")
	v["owner_inputs"] = append(v["owner_inputs"].([]any), v["owner_inputs"].([]any)[0])
	compatibilityOK(t, tuple, v) // Duplicate owner + all three present is allowed.
	for _, owner := range []string{"curator", "runtime", "launcher", "unknown"} {
		v = compatibilityCatalog(t)
		for _, item := range compatibilityAt(v, "entries.0")["owner_inputs"].([]any) {
			item.(map[string]any)["owner"] = owner
		}
		if owner == "unknown" {
			compatibilityBad(t, catalog, encode(t, v), "schema_invalid")
		} else {
			compatibilityOK(t, catalog, v) // NO tuple allOf/contains on catalogEntry.
		}
	}
	for _, size := range []int{0, 1, 32, 33} {
		v = compatibilityCatalog(t)
		entry := v["entries"].([]any)[0]
		entries := make([]any, size)
		for i := range entries {
			entries[i] = entry // Exact duplicate tuple refs intentionally accepted.
		}
		v["entries"] = entries
		if size <= 32 {
			compatibilityOK(t, catalog, v)
		} else {
			compatibilityBad(t, catalog, encode(t, v), "schema_invalid")
		}
	}
}

// Existing strict lexical rules and static failure privacy are exercised through
// EACH new entrypoint. Existing depth/node/4K lexer boundary tests are reused,
// not a new parser/mutation campaign or generic-schema equivalence assertion.
func TestCompatibilityStrictEntryRefusals(t *testing.T) {
	for _, c := range compatibilityCases() {
		t.Run(c.file, func(t *testing.T) {
			v := compatibilitySource(t, c.file)
			if c.file == "catalog-empty.json" {
				v = compatibilityCatalog(t)
			}
			raw := string(encode(t, v))
			compatibilityOK(t, c, v)
			escaped := strings.Replace(raw, `"kind":`, `"k\u0069nd":`, 1)
			if escaped == raw {
				t.Fatal("escaped member control not applied")
			}
			r, e := c.decode([]byte(escaped))
			if e != nil || r == nil || r.Value()["kind"] != v["kind"] {
				t.Fatal("valid escaped member refused", e)
			}
			duplicate := strings.Replace(raw, `"kind":`, `"k\u0069nd":"private-marker","kind":`, 1)
			compatibilityBad(t, c, []byte(duplicate), "schema_invalid")
			for _, bad := range [][]byte{append([]byte{0xef, 0xbb, 0xbf}, []byte(raw)...), append([]byte{0}, []byte(raw)...), append([]byte{0xff}, []byte(raw)...), []byte(strings.Replace(raw, `"kind":`, `"unknown":"\ud800","kind":`, 1)), []byte(strings.Replace(raw, `"kind":`, `"unknown":"\udc00","kind":`, 1)), []byte(strings.Replace(raw, `"kind":`, `"unknown":"\u0000","kind":`, 1))} {
				compatibilityBad(t, c, bad, "encoding_invalid")
			}
			if c.file != "owner.json" {
				// Duplicate a field in a reachable, otherwise valid nested shape.
				needle := `"name":`
				bad := strings.Replace(raw, needle, `"n\u0061me":"private-marker","name":`, 1)
				if bad == raw {
					t.Fatal("nested duplicate not constructed")
				}
				compatibilityBad(t, c, []byte(bad), "schema_invalid")
			}
			v["private-marker"] = "private/path/secret-marker"
			compatibilityBad(t, c, encode(t, v), "schema_invalid")
			// Bound priority also applies to otherwise closed/schema-invalid data.
			v["private-marker"] = strings.Repeat("é", 2048)
			code := "schema_invalid"
			if c.file == "owner.json" {
				code = "bounds_exceeded" // Its smaller raw cap precedes lexical work.
			}
			compatibilityBad(t, c, encode(t, v), code)
			v["private-marker"] = strings.Repeat("é", 2048) + "a"
			compatibilityBad(t, c, encode(t, v), "bounds_exceeded")
			v = compatibilitySource(t, c.file)
			path := "uid"
			if c.file == "tuple.json" {
				path = "home.uid"
			} else if c.file == "catalog-empty.json" {
				v = compatibilityCatalog(t)
				path = "entries.0.tuple.length"
			}
			compatibilitySet(v, path, uint64(1234567))
			compatibilityOK(t, c, v)
			raw = string(encode(t, v))
			for _, n := range []string{"-0", "-1", "1.0", "1e0", "+1", "01", "NaN", "Infinity", "9007199254740992"} {
				bad := strings.Replace(raw, "1234567", n, 1)
				if bad == raw {
					t.Fatal("numeric neighbor not constructed")
				}
				code := "schema_invalid"
				if n == "9007199254740992" {
					code = "bounds_exceeded"
				}
				compatibilityBad(t, c, []byte(bad), code)
			}
		})
	}
}
