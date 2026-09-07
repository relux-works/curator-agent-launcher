package fragment

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func asInvalid(err error, target **InvalidError) bool { return errors.As(err, target) }

func canon(t *testing.T, in string) string {
	t.Helper()
	v, err := ParseJSON([]byte(in))
	if err != nil {
		t.Fatalf("ParseJSON(%q): %v", in, err)
	}
	return string(Canonical(v))
}

// TestCanonicalRules drives registry.md §1 rules 1-8 one at a time.
func TestCanonicalRules(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"rule 1: top-level sig removed", `{"sig":"x","a":1}`, `{"a":1}`},
		{"rule 1: nested sig kept", `{"a":{"sig":"x"}}`, `{"a":{"sig":"x"}}`},
		{"rule 2: keys sorted by scalar value", `{"b":1,"a":2,"B":3,"aa":4,"é":5,"z":6}`, `{"B":3,"a":2,"aa":4,"b":1,"z":6,"é":5}`},
		{"rule 2: scalar order beats UTF-16 order", "{\"\uFF21\":1,\"\U0001F600\":2}", "{\"\uFF21\":1,\"\U0001F600\":2}"},
		{"rule 3: whitespace removed", " {\n\t\"a\" : [ 1 , 2 ] ,\r\n\"b\":{ } } ", `{"a":[1,2],"b":{}}`},
		{"rule 4: array order preserved", `[3,1,2,[2,1]]`, `[3,1,2,[2,1]]`},
		{"rule 5: named escapes", `"\"\\\b\f\n\r\t"`, `"\"\\\b\f\n\r\t"`},
		{"rule 5: other controls as lowercase \\u00xx", `"\u0000\u0001\u001f"`, `"\u0000\u0001\u001f"`},
		{"rule 5: escaped controls re-emitted lowercase", `"\u001F\u000B"`, `"\u001f\u000b"`},
		{"rule 6: slash and html not escaped", `"\/<>&"`, `"/<>&"`},
		{"rule 6: non-ASCII raw incl. U+2028/U+2029", "\"\u00e9\u2028\u2029\u00ad\U0001F600\"", "\"\u00e9\u2028\u2029\u00ad\U0001F600\""},
		{"rule 6: escaped U+2028/U+2029 emitted raw", `"\u2028\u2029\ud83d\ude00"`, "\"\u2028\u2029\U0001F600\""},
		{"rule 6: DEL not escaped", "\"\x7f\"", "\"\x7f\""},
		{"rule 7: integers shortest form", `[0,-0.0e0,7,-7,9007199254740991,-9007199254740991]`, ""},
		{"rule 8: literals", `[true,false,null]`, `[true,false,null]`},
	}
	for _, c := range cases {
		if c.want == "" {
			continue // handled below
		}
		if got := canon(t, c.in); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	// rule 7 without the rejected -0.0e0 member
	if got := canon(t, `[0,7,-7,9007199254740991,-9007199254740991]`); got != `[0,7,-7,9007199254740991,-9007199254740991]` {
		t.Errorf("rule 7: %q", got)
	}
}

func TestReaderRejections(t *testing.T) {
	cases := map[string]string{
		"duplicate key":          `{"a":1,"a":2}`,
		"duplicate key escaped":  `{"a":1,"\u0061":2}`,
		"invalid utf-8":          "\"\xc3\x28\"",
		"overlong utf-8":         "\"\xc0\xaf\"",
		"utf-8 surrogate bytes":  "\"\xed\xa0\x80\"",
		"lone high surrogate":    `"\ud800"`,
		"lone low surrogate":     `"\udfff"`,
		"high + non-low":         `"\ud800\ud800"`,
		"fraction":               `1.0`,
		"exponent":               `1e0`,
		"negative zero":          `-0`,
		"too large":              `9007199254740992`,
		"too small":              `-9007199254740992`,
		"way too large":          `123456789012345678901234567890`,
		"leading zero":           `01`,
		"plus sign":              `+1`,
		"minus alone":            `-`,
		"trailing comma object":  `{"a":1,}`,
		"trailing comma array":   `[1,]`,
		"trailing content":       `{} {}`,
		"trailing byte":          `{}x`,
		"single quotes":          `{'a':1}`,
		"unquoted key":           `{a:1}`,
		"missing colon":          `{"a" 1}`,
		"unterminated string":    `"abc`,
		"unterminated object":    `{"a":1`,
		"unterminated array":     `[1`,
		"raw newline in string":  "\"a\nb\"",
		"raw tab in string":      "\"a\tb\"",
		"bad escape":             `"\x41"`,
		"truncated unicode":      `"\u12"`,
		"bad hex":                `"\u12G4"`,
		"literal typo":           `tru`,
		"literal case":           `True`,
		"NaN":                    `NaN`,
		"empty":                  ``,
		"whitespace only":        "  \n",
		"form feed whitespace":   "\f{}",
		"BOM":                    "\ufeff{}",
		"nesting over the bound": strings.Repeat("[", 65) + strings.Repeat("]", 65),
	}
	for name, in := range cases {
		if _, err := ParseJSON([]byte(in)); err == nil {
			t.Errorf("%s: accepted %q", name, in)
		} else if !IsInvalid(err) {
			t.Errorf("%s: error is not a reader error: %v", name, err)
		}
	}
}

func TestReaderAccepts(t *testing.T) {
	cases := map[string]Value{
		`0`:                {Kind: KindInt},
		`-1`:               {Kind: KindInt, Int: -1},
		`9007199254740991`: {Kind: KindInt, Int: 9007199254740991},
		"\"\x7f\"":         {Kind: KindString, Str: "\x7f"},
		`"😀"`:              {Kind: KindString, Str: "\U0001F600"},
		`"aA\/b"`:          {Kind: KindString, Str: "aA/b"},
		`null`:             {Kind: KindNull},
		`true`:             {Kind: KindBool, Bool: true},
		` [] `:             {Kind: KindArray, Arr: []Value{}},
		`{}`:               {Kind: KindObject, Obj: []Member{}},
		`{"":0}`:           {Kind: KindObject, Obj: []Member{{Key: "", Value: Value{Kind: KindInt}}}},
		strings.Repeat("[", 64) + strings.Repeat("]", 64): {},
	}
	for in, want := range cases {
		got, err := ParseJSON([]byte(in))
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if want.Kind == KindNull && want.Arr == nil && in != `null` {
			continue // depth case: acceptance is the assertion
		}
		if got.Kind != want.Kind || got.Int != want.Int || got.Str != want.Str || got.Bool != want.Bool || len(got.Arr) != len(want.Arr) || len(got.Obj) != len(want.Obj) {
			t.Errorf("%q: got %+v want %+v", in, got, want)
		}
	}
}

func TestDigestIsSHA256OverCanonical(t *testing.T) {
	v, err := ParseJSON([]byte(`{"b":"é","a":[1,null]}`))
	if err != nil {
		t.Fatal(err)
	}
	c := Canonical(v)
	if string(c) != `{"a":[1,null],"b":"é"}` {
		t.Fatalf("canonical %q", c)
	}
	sum := sha256.Sum256(c)
	if want := DigestPrefix + hex.EncodeToString(sum[:]); Digest(v) != want {
		t.Errorf("Digest = %s want %s", Digest(v), want)
	}
	if len(Digest(v)) != 7+64 || strings.ToLower(Digest(v)) != Digest(v) {
		t.Errorf("digest shape %q", Digest(v))
	}
}

func TestKindString(t *testing.T) {
	for k, want := range map[Kind]string{KindNull: "null", KindBool: "boolean", KindInt: "integer", KindString: "string", KindArray: "array", KindObject: "object", Kind(9): "Kind(9)"} {
		if k.String() != want {
			t.Errorf("%d: %q", int(k), k.String())
		}
	}
	if (Value{Kind: KindInt}).Kind.String() != "integer" {
		t.Fatal("kind string")
	}
	var se *SyntaxError
	if _, err := ParseJSON([]byte(`{`)); !errors.As(err, &se) || !strings.Contains(err.Error(), "at byte") {
		t.Errorf("syntax error shape: %v", err)
	}
}
