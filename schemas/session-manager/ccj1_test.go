package managerschema

import (
	"bytes"
	"strings"
	"testing"
)

// Independent expected bytes pin CCJ-1 escaping, sorting, sig scope and safe
// integer bounds; malformed neighbors must refuse instead of being repaired.
func TestCanonicalJSON(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{` { "z": -9007199254740991, "a": 9007199254740991 } `, `{"a":9007199254740991,"z":-9007199254740991}`},
		{`{"sig":"omit","nested":{"sig":"keep"}}`, `{"nested":{"sig":"keep"}}`},
		{`["\uD83D\uDE00","\u00e9","e\u0301","<>&/\u2028\u2029","\u0001\n\t"]`, "[\"😀\",\"é\",\"é\",\"<>&/\u2028\u2029\",\"\\u0001\\n\\t\"]"},
		{`{"é":true,"a":[false,null,0],"😀":"x"}`, `{"a":[false,null,0],"é":true,"😀":"x"}`},
		{strings.Repeat("[", 63) + "0" + strings.Repeat("]", 63), strings.Repeat("[", 63) + "0" + strings.Repeat("]", 63)},
	} {
		got, err := CanonicalJSON([]byte(tc.input))
		if err != nil || !bytes.Equal(got, []byte(tc.want)) {
			t.Fatalf("canonical bytes=%q, error=%v; want %q", got, err, tc.want)
		}
	}
	for _, input := range []string{
		`{"a":1,"\u0061":2}`, `9007199254740992`, `-9007199254740992`, `-0`, `1.0`, `1e0`, `01`,
		`"\ud800"`, `"\udfff"`, "\"\xff\"", "\"\x01\"", `{} {}`, `[1,]`, `{"a":1,}`,
		strings.Repeat("[", 64) + "0" + strings.Repeat("]", 64),
	} {
		if data, err := CanonicalJSON([]byte(input)); err == nil || data != nil {
			t.Fatalf("invalid JSON accepted/repaired: %q => %q, %v", input, data, err)
		}
	}
}
