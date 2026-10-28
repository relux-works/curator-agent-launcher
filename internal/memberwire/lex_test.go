package memberwire

import (
	"strings"
	"testing"
)

// These are lexical/grammar-only controls, NOT schema-valid requests. Every
// positive must also fail the public request decoder with schema_invalid.
func lexicalOK(t *testing.T, raw string) map[string]any {
	t.Helper()
	o, e := parseObject([]byte(raw))
	if e != nil || o == nil {
		t.Fatalf("syntax-only positive refused: %v", e)
	}
	requestBad(t, []byte(raw), "schema_invalid")
	return o
}
func lexicalBad(t *testing.T, raw, code string) {
	t.Helper()
	o, e := parseObject([]byte(raw))
	if o != nil || e == nil || e.Code() != code || e.Stage() != "parse" || e.Error() != "parse/"+code {
		t.Fatalf("wanted nil parse/%s, got object=%t error=%v", code, o != nil, e)
	}
	requestBad(t, []byte(raw), code)
}

// Escape-equivalent duplicate names at root, nested object and array item
// refuse. Distinct decoded names, valid Unicode and escaped punctuation survive.
func TestDecodedNamesAndUnicode(t *testing.T) {
	for _, good := range []string{`{}`, `{"a":0}`, `{"a":[]}`, `{"a":[{"b":null}]}`, `{"a":1,"b":2}`, `{"a":{"b":1,"c":2}}`, `{"a":[{"b":1},{"b":2}]}`} {
		lexicalOK(t, good)
	}
	for _, raw := range []string{`{"a":1,"a":1}`, `{"a":1,"\u0061":1}`, `{"a":{"b":1,"\u0062":1}}`, `{"a":[{"b":1,"b":1}]}`, `{"é":1,"\u00e9":2}`, `{"𝄞":1,"\uD834\uDD1E":2}`} {
		lexicalBad(t, raw, "schema_invalid")
	}
	o := lexicalOK(t, `{"é":"\uD834\uDD1E","escaped":"\"\\\/\b\f\n\r\t","literal":"𝄞"}`)
	if o["é"] != "𝄞" || o["literal"] != "𝄞" || o["escaped"] != "\"\\/\b\f\n\r\t" {
		t.Fatal("Unicode or JSON escapes were normalized/lost")
	}
	for _, raw := range []string{`{"a":"\u0000"}`, `{"\u0000":0}`, `{"a":"\ud800"}`, `{"a":"\udc00"}`, `{"a":"\ud800\u0041"}`, `{"a":"\ud800\ud800"}`, `{"a":"\uZZZZ"}`, `{"a":"\u123"}`, `{"a":"\x41"}`, `{"a":"\v"}`, "{\"a\":\"\n\"}", "{\"a\":\"\x00\"}", "{\"a\":\"\xed\xa0\x80\"}", "{\"a\":\"\xff\"}"} {
		lexicalBad(t, raw, "encoding_invalid")
	}
	for _, raw := range []string{`{/*comment*/"a":0}`, "{\"a\":0//comment\n}"} {
		lexicalBad(t, raw, "schema_invalid")
	}
}

// Integer acceptance is lexical and lossless, independent of mathematical
// JSON Schema integer permissiveness. Structural field ranges are tested apart.
func TestLexicalNumbersAndFraming(t *testing.T) {
	for _, n := range []string{"0", "1", "9007199254740991"} {
		o := lexicalOK(t, `{"n":`+n+`}`)
		if n == "9007199254740991" && o["n"] != uint64(9007199254740991) {
			t.Fatal("integer precision lost")
		}
	}
	for _, n := range []string{"-0", "-1", "0.0", "1.5", "1e0", "1E+2", "1e-2", "01", "00", "+1", ".1", "NaN", "Infinity", "-Infinity"} {
		lexicalBad(t, `{"n":`+n+`}`, "schema_invalid")
	}
	for _, n := range []string{"9007199254740992", "18446744073709551616", strings.Repeat("9", 5000)} {
		lexicalBad(t, `{"n":`+n+`}`, "bounds_exceeded")
	}
	for _, raw := range []string{`{"a":}`, `{"a":`, `{"a" 1}`, `{"a":1,}`, `{,"a":1}`, `{"a":[1,]}`, `{"a":[,1]}`, `{"a":[1 2]}`, `{"a":true false}`, `{"a":1}{}`, `{}[]`, `{`, `{"a":"unterminated}`, `{"a":tru}`, `{"a":1]`, "{}\u00a0"} {
		lexicalBad(t, raw, "encoding_invalid")
	}
	// Encoding outranks bounds/schema; bounds outrank duplicate/member errors.
	lexicalBad(t, `{"a":1,"a":"\ud800"}`, "encoding_invalid")
	lexicalBad(t, `{"a":1,"a":9007199254740992}`, "bounds_exceeded")
}

// Exact inclusive source-owner convention: root depth1; containers alone add
// levels; each value including empty containers counts once, keys do not count.
// These synthetic boundary objects remain schema-invalid on the public API.
func TestCountingAndDecodedByteBoundaries(t *testing.T) {
	for _, leaf := range []string{"null", "0", `"s"`, "true", "false"} {
		lexicalOK(t, `{"a":`+strings.Repeat("[", 15)+leaf+strings.Repeat("]", 15)+`}`)
		lexicalBad(t, `{"a":`+strings.Repeat("[", 16)+leaf+strings.Repeat("]", 16)+`}`, "bounds_exceeded")
	}
	lexicalOK(t, strings.Repeat(`{"k":`, 16)+`0`+strings.Repeat("}", 16))
	lexicalBad(t, strings.Repeat(`{"k":`, 17)+`0`+strings.Repeat("}", 17), "bounds_exceeded")
	for _, value := range []string{"0", "null", "true", "[]", "{}", `""`} {
		lexicalOK(t, `{"a":[`+strings.Repeat(value+",", 8189)+value+`]}`) // root + array +8190 values
		lexicalBad(t, `{"a":[`+strings.Repeat(value+",", 8190)+value+`]}`, "bounds_exceeded")
	}
	for _, pair := range []struct{ at, over string }{
		{strings.Repeat("a", 4096), strings.Repeat("a", 4097)},
		{strings.Repeat("é", 2048), strings.Repeat("é", 2048) + "a"},
		{strings.Repeat(`\u00e9`, 2048), strings.Repeat(`\u00e9`, 2048) + "a"},
		{strings.Repeat(`\ud834\udd1e`, 1024), strings.Repeat(`\ud834\udd1e`, 1025)},
	} {
		lexicalOK(t, `{"s":"`+pair.at+`"}`)
		lexicalBad(t, `{"s":"`+pair.over+`"}`, "bounds_exceeded")
		lexicalOK(t, `{"`+pair.at+`":0}`)
		lexicalBad(t, `{"`+pair.over+`":0}`, "bounds_exceeded")
	}
}
