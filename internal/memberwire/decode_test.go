package memberwire

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, file string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../docs/same-user-v1/fixtures/" + file)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func requestFixture(t *testing.T, file string) map[string]any {
	t.Helper()
	r, err := DecodeRequest(fixture(t, file))
	if err != nil {
		t.Fatal(err)
	}
	return r.Value()
}
func encode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func requestOK(t *testing.T, v any) *UntrustedRecord {
	t.Helper()
	r, err := DecodeRequest(encode(t, v))
	if err != nil || r == nil {
		t.Fatalf("structural positive refused: %v", err)
	}
	return r
}
func requestBad(t *testing.T, raw []byte, code string) {
	t.Helper()
	r, err := DecodeRequest(raw)
	if r != nil || err == nil || err.Stage() != "parse" || err.Code() != code {
		t.Fatalf("wanted nil, parse/%s; got record=%t error=%v", code, r != nil, err)
	}
	if err.Error() != "parse/"+code || len(err.Error()) > 256 {
		t.Fatal("error must be a bounded static tag")
	}
}

// These published inert bytes exercise every request branch and the expired A's
// structure only. Hard-coded Stage1 lengths/digests are independent raw-byte
// expectations, not a second parser or any authority/expiry/tuple witness.
// After caller mutation, both fresh and earlier snapshots must still match
// independently frozen fixture values, including existing array elements.
func TestPublishedStructuralSnapshots(t *testing.T) {
	cases := []struct {
		file, operation, digest string
		length                  int
	}{
		{"submit.json", "submit", "3a5080f47c4d8af599c8ec654ae2d926924868938a882ed1c801b0978b928cbb", 3348},
		{"request-inspect.json", "inspect", "6dd6628eb08395631cb0ac6641a954cf34e01d3a4acf9664d6e7adddf5fa3639", 178},
		{"request-cancel.json", "cancel", "88cb5be74ff869be3933466b98bf46e1b177886acbbd2d762becaa5bf181d67f", 262},
		{"request-reconcile.json", "reconcile", "21fbe4e1ef4d9ac7f92ba563df1828f97f6c0a12af1af9a3c7cce4d723a439c2", 180},
		{"request-reserve.json", "consume", "feb1b2b8c4b02d3ef42c66fa6b97619e552979f3b94c41a9b0b43bf4c741d8fa", 392},
		{"request-ack.json", "consume", "f8e2762be1df6ec69da7c2fe4f7280dc0c94a7ad903741183bcd7a96467a85d3", 530},
		{"authorization.json", "", "99965b37937380c1a38fd5e7070274f7a0b953b6b1f4b1a43e614e67584c27c4", 292},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			raw := fixture(t, c.file)
			original := bytes.Clone(raw)
			var r *UntrustedRecord
			var err *Error
			if c.operation == "" {
				r, err = DecodeAuthorization(raw)
			} else {
				r, err = DecodeRequest(raw)
			}
			if err != nil || r == nil {
				t.Fatalf("fixture structure refused: %v", err)
			}
			sum := r.SHA256()
			if r.Length() != c.length || hex.EncodeToString(sum[:]) != c.digest || !bytes.Equal(r.Raw(), original) {
				t.Fatal("original raw length/hash/bytes changed")
			}
			if c.operation != "" && r.Value()["operation"] != c.operation {
				t.Fatal("operation lost")
			}
			if c.operation == "" && r.Value()["expires_at_ms"] != uint64(1) {
				t.Fatal("expiry must be preserved, not admitted or rewritten")
			}
			// The immutable fixture hash establishes these independently published
			// valid bytes. Standard JSON is used only to compare preservation of
			// their complete values, never as the acceptance/refusal oracle.
			var expected any
			comparison := json.NewDecoder(bytes.NewReader(original))
			comparison.UseNumber()
			if err := comparison.Decode(&expected); err != nil {
				t.Fatal(err)
			}
			expectedBytes := encode(t, expected)
			if !bytes.Equal(encode(t, r.Value()), expectedBytes) {
				t.Fatal("decoded snapshot changed a published value, field, null or array")
			}
			before := r.Value()
			raw[0] = '!'
			exported := r.Raw()
			exported[0] = '?'
			changed := r.Value()
			changed["kind"] = "changed"
			if c.operation == "submit" {
				changed["identity"].(map[string]any)["role"] = "changed"
				changed["artifacts"].([]any)[0].(map[string]any)["object"].(map[string]any)["name"] = "changed"
				changed["effects"].(map[string]any)["tools"] = append(changed["effects"].(map[string]any)["tools"].([]any), "changed")
				changed["effects"].(map[string]any)["read_objects"].([]any)[0] = "changed"
			}
			if !bytes.Equal(r.Raw(), original) || r.SHA256() != sum {
				t.Fatal("caller mutation reached retained raw bytes")
			}
			if !bytes.Equal(encode(t, r.Value()), expectedBytes) {
				t.Fatal("caller mutation reached retained decoded state")
			}
			if !bytes.Equal(encode(t, before), expectedBytes) {
				t.Fatal("caller mutation reached earlier decoded snapshot")
			}
		})
	}
}

// Byte ceilings are inclusive and apply before interpretation. Whitespace is
// retained and hashed; an oversized malformed input still gets the size tag.
func TestRawCeilingsAndFraming(t *testing.T) {
	for _, c := range []struct {
		file   string
		limit  int
		decode func([]byte) (*UntrustedRecord, *Error)
	}{
		{"request-inspect.json", RequestLimit, DecodeRequest}, {"authorization.json", AuthorizationLimit, DecodeAuthorization},
	} {
		t.Run(c.file, func(t *testing.T) {
			original := fixture(t, c.file)
			for _, size := range []int{c.limit - 1, c.limit} {
				raw := append(bytes.Repeat([]byte(" "), size-len(original)), original...)
				r, e := c.decode(raw)
				if e != nil || r == nil || r.Length() != size || r.SHA256() != sha256.Sum256(raw) || !bytes.Equal(r.Raw(), raw) {
					t.Fatal("bounded raw bytes must survive unchanged", e)
				}
			}
			for _, raw := range [][]byte{bytes.Repeat([]byte(" "), c.limit+1), append(bytes.Repeat([]byte(" "), c.limit+1-len(original)), original...)} {
				r, e := c.decode(raw)
				if r != nil || e == nil || e.Code() != "bounds_exceeded" || e.Stage() != "parse" {
					t.Fatal("raw ceiling must precede parsing")
				}
			}
		})
	}
	raw := fixture(t, "request-inspect.json")
	for _, pad := range []string{"", " \t\r\n"} {
		r, e := DecodeRequest(append(append([]byte(pad), raw...), []byte(pad)...))
		if e != nil || r == nil {
			t.Fatal(e)
		}
	}
	for _, bad := range [][]byte{nil, {}, []byte("[]"), []byte("null"), []byte("true"), []byte("1"), append(bytes.Clone(raw), raw...), append(bytes.Clone(raw), 'x'), raw[:len(raw)-2], append([]byte{0xef, 0xbb, 0xbf}, raw...), append([]byte{0}, raw...), append([]byte{0xc0, 0xaf}, raw...), append([]byte("\v"), raw...)} {
		requestBad(t, bad, "encoding_invalid")
	}
}

// Valid branch controls accompany root/nested closure, explicit-null and branch
// refusals. Error text must not echo the hostile marker, even after an ID parsed.
func TestRequestClosureAndStaticErrors(t *testing.T) {
	files := []string{"submit.json", "request-inspect.json", "request-reconcile.json", "request-cancel.json", "request-reserve.json", "request-ack.json"}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			base := requestFixture(t, file)
			requestOK(t, base)
			for key := range base {
				for _, how := range []string{"missing", "null", "wrong_type"} {
					t.Run(key+"/"+how, func(t *testing.T) {
						v := requestFixture(t, file)
						if how == "missing" {
							delete(v, key)
						} else if how == "null" {
							v[key] = nil
						} else {
							v[key] = true
						}
						requestBad(t, encode(t, v), "schema_invalid")
					})
				}
			}
			for _, key := range []string{"unrecognized-private-marker", "resume", "argv", "environment", "AdmissionOK"} {
				v := requestFixture(t, file)
				v[key] = "private/path/secret-marker"
				requestBad(t, encode(t, v), "schema_invalid")
			}
			for _, value := range []any{"unknown", uint64(1), map[string]any{}} {
				v := requestFixture(t, file)
				v["kind"] = value
				requestBad(t, encode(t, v), "schema_invalid")
			}
			for _, value := range []string{"unknown", "resume", "ack"} {
				v := requestFixture(t, file)
				v["operation"] = value
				requestBad(t, encode(t, v), "schema_invalid")
			}
		})
	}
	// Every recursively referenced object has all its required fields and closure
	// exercised; null is valid ONLY for the three schema-nullable properties.
	for _, path := range []string{"identity", "tuple", "artifacts.0", "artifacts.0.object", "effects", "deadlines", "assignment"} {
		t.Run(path, func(t *testing.T) {
			selectObject := func(v map[string]any) map[string]any {
				var cur any = v
				for _, key := range strings.Split(path, ".") {
					if key == "0" {
						cur = cur.([]any)[0]
					} else {
						cur = cur.(map[string]any)[key]
					}
				}
				return cur.(map[string]any)
			}
			base := requestFixture(t, "submit.json")
			requestOK(t, base)
			for key := range selectObject(base) {
				v := requestFixture(t, "submit.json")
				delete(selectObject(v), key)
				requestBad(t, encode(t, v), "schema_invalid")
				v = requestFixture(t, "submit.json")
				selectObject(v)[key] = true
				requestBad(t, encode(t, v), "schema_invalid")
				v = requestFixture(t, "submit.json")
				selectObject(v)[key] = nil
				nullable := path == "identity" && (key == "producer_member_id" || key == "predecessor_invocation_id") || path == "effects" && key == "provider_binding_sha256"
				if nullable {
					requestOK(t, v)
				} else {
					requestBad(t, encode(t, v), "schema_invalid")
				}
			}
			v := requestFixture(t, "submit.json")
			selectObject(v)["unknown"] = "secret-marker"
			requestBad(t, encode(t, v), "schema_invalid")
		})
	}
	for _, file := range []string{"negative-resume.json", "negative-unknown-member.json", "duplicate-key.json.txt"} {
		requestBad(t, fixture(t, file), "schema_invalid")
	}
}

// Authorization is a separate closed structure; binding and expiry are NOT
// checked. Schema ranges differ from the larger global lexical integer ceiling.
func TestAuthorizationStructure(t *testing.T) {
	good, e := DecodeAuthorization(fixture(t, "authorization.json"))
	if e != nil {
		t.Fatal(e)
	}
	bad := func(v map[string]any, code string) {
		t.Helper()
		r, e := DecodeAuthorization(encode(t, v))
		if r != nil || e == nil || e.Stage() != "parse" || e.Code() != code || e.Error() != "parse/"+code {
			t.Fatal("authorization refusal", e)
		}
	}
	for key := range good.Value() {
		for _, how := range []string{"missing", "null", "wrong"} {
			v := good.Value()
			switch how {
			case "missing":
				delete(v, key)
			case "null":
				v[key] = nil
			case "wrong":
				v[key] = false
			}
			bad(v, "schema_invalid")
		}
	}
	v := good.Value()
	v["unknown"] = "secret-marker"
	bad(v, "schema_invalid")
	for _, c := range []struct {
		key            string
		valid, invalid []any
	}{
		{"kind", []any{"curator-run member-authorization/1"}, []any{"curator-run member-request/1", "unknown"}},
		{"controller_id", []any{strings.Repeat("a", 32)}, []any{strings.Repeat("a", 31), strings.Repeat("A", 32)}},
		{"store_id", []any{strings.Repeat("0", 32)}, []any{strings.Repeat("0", 33), strings.Repeat("g", 32)}},
		{"request_sha256", []any{strings.Repeat("f", 64)}, []any{strings.Repeat("f", 63), strings.Repeat("F", 64)}},
		{"request_length", []any{uint64(1), uint64(65536)}, []any{uint64(0), uint64(65537)}},
		{"expires_at_ms", []any{uint64(1), uint64(9007199254740991)}, []any{uint64(0)}},
	} {
		for _, value := range c.valid {
			v := good.Value()
			v[c.key] = value
			r, e := DecodeAuthorization(encode(t, v))
			if e != nil || r == nil {
				t.Fatal(c.key, e)
			}
		}
		for _, value := range c.invalid {
			v := good.Value()
			v[c.key] = value
			bad(v, "schema_invalid")
		}
	}
	v = good.Value()
	v["expires_at_ms"] = uint64(9007199254740992)
	bad(v, "bounds_exceeded")
	requestBad(t, fixture(t, "authorization.json"), "schema_invalid")
	r, e := DecodeAuthorization(fixture(t, "request-inspect.json"))
	if r != nil || e == nil || e.Code() != "schema_invalid" {
		t.Fatal("request cannot masquerade as A")
	}
}

// Duplicate detection precedes field interpretation on otherwise valid product
// requests as well as synthetic objects; legal ASCII escapes preserve values.
func TestProductEscapedAndNestedMembers(t *testing.T) {
	raw := string(fixture(t, "submit.json"))
	good := strings.Replace(raw, `"operation": "submit"`, `"oper\u0061tion": "submit"`, 1)
	r, e := DecodeRequest([]byte(good))
	if e != nil || r == nil || r.Value()["operation"] != "submit" {
		t.Fatal("legal escaped member lost", e)
	}
	for _, pair := range [][2]string{
		{`"role": "development"`, `"role": "development", "r\u006fle": "development"`},
		{`"name": "instruction"`, `"name": "instruction", "\u006eame": "instruction"`},
	} {
		bad := strings.Replace(raw, pair[0], pair[1], 1)
		if bad == raw {
			t.Fatal("duplicate product neighbor not constructed")
		}
		requestBad(t, []byte(bad), "schema_invalid")
	}
}
