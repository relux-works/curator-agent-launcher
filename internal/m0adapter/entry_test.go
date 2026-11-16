package m0adapter

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// The copied fixtures are byte-identical to swarma-dispatcher cf72502
// (testdata/README.md); an edit to them fails here rather than silently
// changing what the witness tests prove.
func TestFixtureBytesPinned(t *testing.T) {
	want := map[string]string{
		"entry.specimen.json":     "sha256:16b898164b1b4bd0568a37bf71cbdf2625dc89f45544730866793bc433217aad",
		"mapping.witness.json":    "sha256:3e12262d5828688b47a25543a87ab6e22bffa2c84b7851b0509d1e3ad61c0e21",
		"forbidden-controls.json": "sha256:71cd6e8becaac27674dd65f3dc92e7a8e88093e6051e86ce47badae12da7e262",
		"boundary.witnesses.json": "sha256:a3df6f8ec7cc983bf82cd912a4e881dff308b1458ac75519c362195d37aa3160",
	}
	for name, digest := range want {
		if got := sha(readTestdata(t, name)); got != digest {
			t.Errorf("%s: digest %s, want %s", name, got, digest)
		}
	}
}

// The test-local CCJ-1 input builder reproduces the witness CCJ-1(input)
// length and digest, so variants built with it carry a correct worker_plan
// independently of the production canonicalizer.
func TestIndependentInputCanonical(t *testing.T) {
	w := loadWitness(t)
	got := inputCCJ(specimenInput())
	if len(got) != w.InputCCJBytes || sha(got) != w.InputCCJSHA || w.InputCCJSHA != specimenWorkerPlan {
		t.Fatalf("independent CCJ-1(input): %d bytes %s; witness %d %s", len(got), sha(got), w.InputCCJBytes, w.InputCCJSHA)
	}
}

// The fixture specimen decodes and maps to exactly the witness: CCJ-1 sizes
// and digests, 173 tokens / 9 objects / 43 members, the 19-element argv of
// 392 NUL-terminated bytes, T=240 for D=290000/now=20000/S=10000, and the
// exact 28 stdin bytes with no added LF. Limit: E/H are the witness's
// synthetic examples; no file or descriptor is involved here.
func TestSpecimenMatchesWitness(t *testing.T) {
	w := loadWitness(t)
	e, err := Decode([]byte(specimen(t)))
	if err != nil {
		t.Fatalf("specimen refused: %v", err)
	}
	if e.CanonicalBytes != w.EntryCCJBytes || e.EntryDigest != w.EntryCCJSHA {
		t.Errorf("entry CCJ-1: %d %s, want %d %s", e.CanonicalBytes, e.EntryDigest, w.EntryCCJBytes, w.EntryCCJSHA)
	}
	if e.InputCanonicalBytes != w.InputCCJBytes || e.InputDigest != w.InputCCJSHA {
		t.Errorf("input CCJ-1: %d %s, want %d %s", e.InputCanonicalBytes, e.InputDigest, w.InputCCJBytes, w.InputCCJSHA)
	}
	if e.Tokens != w.EntryTokens || e.Objects != w.EntryObjects || e.Members != w.EntryMembers {
		t.Errorf("work: %d tokens %d objects %d members, want %d %d %d", e.Tokens, e.Objects, e.Members, w.EntryTokens, w.EntryObjects, w.EntryMembers)
	}
	lit := testLiterals(w.Protected.E, w.Protected.H)
	x, err := prepare(lit, e, fixedEnv(w.Protected.H), w.Protected.NowMs)
	if err != nil {
		t.Fatalf("mapping refused: %v", err)
	}
	if len(x.Argv) != 19 || strings.Join(x.Argv, "\x00") != strings.Join(w.Argv, "\x00") {
		t.Fatalf("argv:\n got %q\nwant %q", x.Argv, w.Argv)
	}
	total := 0
	for _, a := range x.Argv {
		total += len(a) + 1
	}
	if total != w.ArgvBytes {
		t.Errorf("argv storage %d, want %d", total, w.ArgvBytes)
	}
	if x.Timeout != w.TimeoutSeconds {
		t.Errorf("T=%d, want %d", x.Timeout, w.TimeoutSeconds)
	}
	if len(x.Prompt) != w.StdinBytes || sha(x.Prompt) != w.StdinSHA || !w.StdinEOF || w.AddedLF {
		t.Errorf("stdin %d bytes %s, want %d %s", len(x.Prompt), sha(x.Prompt), w.StdinBytes, w.StdinSHA)
	}
	if bytes.HasSuffix(x.Prompt, []byte("\n")) {
		t.Errorf("an LF was added to the prompt")
	}
}

type grammarCase struct {
	name    string
	control string // forbidden-controls.json id this case executes, if any
	doc     func(t *testing.T) string
	want    Reason
}

func sub(old, new string) func(t *testing.T) string {
	return func(t *testing.T) string { return replaceOnce(t, specimen(t), old, new) }
}

func raw(s string) func(t *testing.T) string { return func(*testing.T) string { return s } }

const specimenPrompt = `"prompt": "Reply with the word example."`

func nested(k int) string {
	return strings.Repeat(`{"a":`, k) + "1" + strings.Repeat("}", k)
}

// grammarNegatives is one executable negative per closed-grammar rule of
// spec §2, each asserting its exact reason. Positive neighbors live in
// TestGrammarPositiveNeighbors.
func grammarNegatives() []grammarCase {
	return []grammarCase{
		// Lexical rules.
		{"bom", "bom", func(t *testing.T) string { return "\xef\xbb\xbf" + specimen(t) }, ReasonEntryBOM},
		{"invalid-utf8", "invalid-utf8", sub("Reply", "Re\xffly"), ReasonEntryUTF8},
		{"overlong-utf8", "", sub("Reply", "Re\xc0\xafly"), ReasonEntryUTF8},
		{"encoded-surrogate-utf8", "", sub("Reply", "Re\xed\xa0\x80ly"), ReasonEntryUTF8},
		{"lone-high-surrogate", "surrogate", sub(specimenPrompt, `"prompt": "\ud800"`), ReasonEntrySurrogate},
		{"lone-low-surrogate", "", sub(specimenPrompt, `"prompt": "\udc00"`), ReasonEntrySurrogate},
		{"high-then-nonlow", "", sub(specimenPrompt, `"prompt": "`+u("d800")+u("0041")+`"`), ReasonEntrySurrogate},
		{"high-then-other-escape", "", sub(specimenPrompt, `"prompt": "\ud800\n"`), ReasonEntrySurrogate},
		{"raw-control-byte", "", sub("Reply with", "Reply\twith"), ReasonEntrySyntax},
		{"unknown-escape", "", sub("Reply", `Re\xly`), ReasonEntrySyntax},
		{"bad-unicode-escape", "", sub("Reply", `Re\u00zzly`), ReasonEntrySyntax},
		{"truncated", "", func(t *testing.T) string { s := specimen(t); return s[:len(s)-3] }, ReasonEntrySyntax},
		{"empty", "", raw(""), ReasonEntrySyntax},
		{"whitespace-only", "", raw(" \n\t "), ReasonEntrySyntax},
		{"top-level-string", "", raw(`"x"`), ReasonEntrySyntax},
		{"top-level-array", "", raw(`[{}]`), ReasonEntryArray},
		{"trailing-document", "", func(t *testing.T) string { return specimen(t) + "{}" }, ReasonEntryTrailing},
		{"trailing-garbage", "", func(t *testing.T) string { return specimen(t) + "x" }, ReasonEntryTrailing},
		{"array-member", "extra", sub(specimenPrompt, specimenPrompt+`, "argv": ["--resume","x"]`), ReasonEntryArray},
		{"null", "null", sub(specimenPrompt, `"prompt": null`), ReasonEntryLiteral},
		{"boolean", "", sub(`"op": "dispatch"`, `"op": true`), ReasonEntryLiteral},
		{"duplicate", "duplicate", sub(`"input": {`, `"v": 1,`+"\n  "+`"input": {`), ReasonEntryDuplicate},
		{"escaped-duplicate", "escaped-duplicate", sub(`"input": {`, `"`+u("0076")+`": 1,`+"\n  "+`"input": {`), ReasonEntryDuplicate},
		{"nested-escaped-duplicate", "", sub(`"revision": 1`+"\n    },\n    \"association\"", `"revision": 1, "`+u("0069")+`d": "x"`+"\n    },\n    \"association\""), ReasonEntryDuplicate},
		{"noninteger-fraction", "noninteger", sub("{\n  \"v\": 1,", "{\n  \"v\": 1.0,"), ReasonEntryNoninteger},
		{"noninteger-exponent", "noninteger", sub("{\n  \"v\": 1,", "{\n  \"v\": 1e0,"), ReasonEntryNoninteger},
		{"noninteger-upper-exponent", "", sub("{\n  \"v\": 1,", "{\n  \"v\": 1E0,"), ReasonEntryNoninteger},
		{"negative-zero", "negative-zero", sub(`"controller_epoch": 1`, `"controller_epoch": -0`), ReasonEntryNegativeZero},
		{"range", "range", sub(`"controller_epoch": 1`, `"controller_epoch": 9007199254740992`), ReasonEntryNumberRange},
		{"range-17-digits", "", sub(`"controller_epoch": 1`, `"controller_epoch": 10000000000000000`), ReasonEntryNumberRange},
		{"leading-zero", "", sub(`"controller_epoch": 1`, `"controller_epoch": 01`), ReasonEntrySyntax},
		{"plus-sign", "", sub(`"controller_epoch": 1`, `"controller_epoch": +1`), ReasonEntrySyntax},
		// Finite byte/work rules.
		{"depth", "depth", sub(specimenPrompt, `"prompt": `+nested(9)), ReasonEntryDepthLimit},
		{"size", "size", func(t *testing.T) string { s := specimen(t); return strings.Repeat(" ", 49153-len(s)) + s }, ReasonEntrySizeLimit},
		{"escape-size", "escape-size", sub(specimenPrompt, `"prompt": "`+strings.Repeat(`\u0001`, 24576)+`"`), ReasonEntrySizeLimit},
		{"name-33-bytes", "", sub(`"input": {`, `"`+strings.Repeat("n", 33)+`": 1,`+"\n  "+`"input": {`), ReasonEntryNameLength},
		{"name-32-bytes-reaches-table", "", sub(`"input": {`, `"`+strings.Repeat("n", 32)+`": 1,`+"\n  "+`"input": {`), ReasonEntryUnknown},
		{"name-non-ascii", "", sub(`"input": {`, `"vé": 1,`+"\n  "+`"input": {`), ReasonEntryNameLength},
		{"name-escaped-non-ascii", "", sub(`"input": {`, `"v`+u("00e9")+`": 1,`+"\n  "+`"input": {`), ReasonEntryNameLength},
		// Closed member table.
		{"missing", "missing", sub(",\n    "+specimenPrompt, ""), ReasonEntryMissing},
		{"missing-timing-member", "missing-timing", sub(",\n    \"slack_ms\": 10000", ""), ReasonEntryMissing},
		{"missing-timing-object", "missing-timing", sub("  \"timing\": {\n    \"deadline_ms\": 290000,\n    \"slack_ms\": 10000\n  },\n", ""), ReasonEntryMissing},
		{"extra-scalar", "extra", sub(specimenPrompt, specimenPrompt+`, "argv": "--resume"`), ReasonEntryUnknown},
		{"injected-path", "injected-member", sub(specimenPrompt, specimenPrompt+`, "path": "/x"`), ReasonEntryUnknown},
		{"injected-env", "injected-member", sub(specimenPrompt, specimenPrompt+`, "env": "X=1"`), ReasonEntryUnknown},
		{"injected-token", "injected-member", sub(specimenPrompt, specimenPrompt+`, "token": "t"`), ReasonEntryUnknown},
		{"injected-resume", "injected-member", sub(specimenPrompt, specimenPrompt+`, "resume": "r"`), ReasonEntryUnknown},
		{"injected-config", "injected-member", sub(specimenPrompt, specimenPrompt+`, "config": "c"`), ReasonEntryUnknown},
		{"injected-prompt-file", "injected-member", sub(specimenPrompt, specimenPrompt+`, "prompt_file": "/p"`), ReasonEntryUnknown},
		{"injected-nested", "", sub(`"op": "dispatch"`, `"op": "dispatch", "command": "x"`), ReasonEntryUnknown},
		{"opaque", "opaque", func(t *testing.T) string {
			s := specimen(t)
			i := strings.Index(s, `"input": {`)
			return s[:i] + `"input": {"body":"opaque"}` + "\n}\n"
		}, ReasonEntryUnknown},
		{"type-v-string", "", sub("{\n  \"v\": 1,", "{\n  \"v\": \"1\","), ReasonEntryType},
		{"type-request-string", "", func(t *testing.T) string {
			s := specimen(t)
			i, j := strings.Index(s, `"request": {`), strings.Index(s, `"execution": {`)
			return s[:i] + `"request": "x",` + "\n  " + s[j:]
		}, ReasonEntryType},
		{"type-deadline-string", "", sub(`"deadline_ms": 290000`, `"deadline_ms": "290000"`), ReasonEntryType},
		{"type-prompt-integer", "", sub(specimenPrompt, `"prompt": 1`), ReasonEntryType},
		{"type-prompt-object-depth-8", "", sub(specimenPrompt, `"prompt": `+nested(6)), ReasonEntryType},
		// Constants and exact value shapes.
		{"entry-v-2", "", sub("{\n  \"v\": 1,", "{\n  \"v\": 2,"), ReasonEntryConstant},
		{"entry-kind", "", sub(`"swarma-dispatcher m0-executor-entry/1"`, `"swarma-dispatcher m0-executor-entry/2"`), ReasonEntryConstant},
		{"request-v-2", "", sub("    \"v\": 1,", "    \"v\": 2,"), ReasonEntryConstant},
		{"request-kind", "", sub(`"swarma-dispatcher m0-request/1"`, `"swarma-dispatcher m0-request/2"`), ReasonEntryConstant},
		{"op-cancel", "", sub(`"op": "dispatch"`, `"op": "cancel"`), ReasonEntryConstant},
		{"placement", "", sub(`"station-one-shot-v1"`, `"station-one-shot-v2"`), ReasonEntryConstant},
		{"id-uppercase", "", sub(`"00000000000000000000000000000001"`, `"0000000000000000000000000000000A"`), ReasonEntryID},
		{"id-31", "", sub(`"00000000000000000000000000000001"`, `"0000000000000000000000000000001"`), ReasonEntryID},
		{"id-non-hex", "", sub(`"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"`, `"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeg"`), ReasonEntryID},
		{"hash-short", "", sub(`"sha256:5555555555555555555555555555555555555555555555555555555555555555"`, `"sha256:555555555555555555555555555555555555555555555555555555555555555"`), ReasonEntryHash},
		{"hash-prefix", "", sub(`"sha256:6666666666666666666666666666666666666666666666666666666666666666"`, `"SHA256:6666666666666666666666666666666666666666666666666666666666666666"`), ReasonEntryHash},
		{"hash-uppercase", "", sub(`"sha256:2222222222222222222222222222222222222222222222222222222222222222"`, `"sha256:222222222222222222222222222222222222222222222222222222222222222A"`), ReasonEntryHash},
		{"serial-zero", "", sub(`"revision": 1`+"\n    },", `"revision": 0`+"\n    },"), ReasonEntrySerial},
		{"serial-negative", "", sub(`"controller_epoch": 1`, `"controller_epoch": -1`), ReasonEntrySerial},
		{"deadline-zero", "", sub(`"deadline_ms": 290000`, `"deadline_ms": 0`), ReasonEntrySerial},
		{"slack-zero", "", sub(`"slack_ms": 10000`, `"slack_ms": 0`), ReasonEntrySerial},
		{"uid-low", "", sub(`"uid": 30001`, `"uid": 29999`), ReasonEntryUID},
		{"uid-high", "", sub(`"uid": 30002`, `"uid": 60000`), ReasonEntryUID},
		{"key-final-bits", "", sub(`HURo"`, `HURp"`), ReasonEntryKey},
		{"key-prefix", "", sub(`"ed25519:`, `"ed448:`), ReasonEntryKey},
		{"key-short", "", sub(`HURo"`, `HUo"`), ReasonEntryKey},
		{"key-alphabet", "", sub(`11qYAYKx`, `11qYAY+x`), ReasonEntryKey},
		{"execution-id-prefix", "", sub(`"EX-11111111111111111111111111111111"`, `"ex-11111111111111111111111111111111"`), ReasonEntryExecutionID},
		{"execution-id-uppercase", "", sub(`"EX-11111111111111111111111111111111"`, `"EX-1111111111111111111111111111111A"`), ReasonEntryExecutionID},
		{"execution-id-bare", "", sub(`"EX-11111111111111111111111111111111"`, `"11111111111111111111111111111111"`), ReasonEntryExecutionID},
		{"selector-option", "selectors", sub(`"account": "example"`, `"account": "--config"`), ReasonEntrySelector},
		{"selector-uppercase", "", sub(`"account": "example"`, `"account": "Example"`), ReasonEntrySelector},
		{"selector-empty", "", sub(`"profile": "codex-refresher-v0"`, `"profile": ""`), ReasonEntrySelector},
		{"selector-65", "", sub(`"account": "example"`, `"account": "`+strings.Repeat("a", 65)+`"`), ReasonEntrySelector},
		{"string-129", "", sub(`"account": "example"`, `"account": "`+strings.Repeat("a", 129)+`"`), ReasonEntryStringLength},
		{"catalog-option", "selectors", sub(`"model": "catalog-model"`, `"model": "--resume"`), ReasonEntryCatalogString},
		{"catalog-space", "", sub(`"model": "catalog-model"`, `"model": "catalog model"`), ReasonEntryCatalogString},
		{"catalog-empty", "", sub(`"reasoning_effort": "catalog-effort"`, `"reasoning_effort": ""`), ReasonEntryCatalogString},
		{"catalog-del", "", sub(`"reasoning_effort": "catalog-effort"`, "\"reasoning_effort\": \"catalog\x7f\""), ReasonEntryCatalogString},
		{"catalog-non-ascii", "", sub(`"reasoning_effort": "catalog-effort"`, `"reasoning_effort": "café"`), ReasonEntryCatalogString},
		{"catalog-129", "", sub(`"model": "catalog-model"`, `"model": "`+strings.Repeat("m", 129)+`"`), ReasonEntryStringLength},
		{"prompt-empty", "", sub(specimenPrompt, `"prompt": ""`), ReasonEntryPromptLength},
		{"prompt-24577", "", sub(specimenPrompt, `"prompt": "`+strings.Repeat("p", 24577)+`"`), ReasonEntryPromptLength},
		{"prompt-nul", "", sub(specimenPrompt, `"prompt": "a\u0000b"`), ReasonEntryPromptNUL},
		// Consistency of request.plan.worker_plan with CCJ-1(input).
		{"worker-plan-prompt-changed", "", sub(specimenPrompt, `"prompt": "Reply with the word example!"`), ReasonEntryWorkerPlan},
		{"worker-plan-added-lf", "", sub(specimenPrompt, `"prompt": "Reply with the word example.\n"`), ReasonEntryWorkerPlan},
		{"worker-plan-tampered", "", sub("ddda080\"", "ddda081\""), ReasonEntryWorkerPlan},
	}
}

func TestGrammarNegatives(t *testing.T) {
	for _, c := range grammarNegatives() {
		t.Run(c.name, func(t *testing.T) {
			doc := c.doc(t)
			e, err := Decode([]byte(doc))
			if got := reasonFor(err); got != c.want || e != nil {
				t.Fatalf("reason %q (entry %v), want %q", got, e != nil, c.want)
			}
			if err.Error() != string(c.want) {
				t.Fatalf("error text %q is not exactly the code", err.Error())
			}
		})
	}
}

// Positive neighbors of the negatives: the nearest admitted values still
// decode, so the gates do not reject everything.
func TestGrammarPositiveNeighbors(t *testing.T) {
	in := specimenInput
	cases := []struct {
		name string
		doc  func(t *testing.T) string
	}{
		{"specimen", func(t *testing.T) string { return specimen(t) }},
		{"max-safe-serial", sub(`"controller_epoch": 1`, `"controller_epoch": 9007199254740991`)},
		{"uid-30000", sub(`"uid": 30001`, `"uid": 30000`)},
		{"uid-59999", sub(`"uid": 30002`, `"uid": 59999`)},
		{"key-final-0", sub(`HURo"`, `HUR0"`)},
		{"encoded-49152", func(t *testing.T) string { s := specimen(t); return strings.Repeat(" ", 49152-len(s)) + s }},
		{"escaped-name-not-duplicate", sub(`"op": "dispatch"`, `"`+u("006f")+`p": "dispatch"`)},
		{"selector-64", func(t *testing.T) string { v := in(); v.Account = strings.Repeat("a", 64); return variant(t, v) }},
		{"selector-digit-first", func(t *testing.T) string { v := in(); v.Profile = "0_a-b"; return variant(t, v) }},
		{"catalog-128", func(t *testing.T) string { v := in(); v.Model = strings.Repeat("m", 128); return variant(t, v) }},
		{"catalog-punctuation", func(t *testing.T) string { v := in(); v.Effort = "x!~/{}\""; return variant(t, v) }},
		{"prompt-1", func(t *testing.T) string { v := in(); v.Prompt = "x"; return variant(t, v) }},
		{"prompt-24576", func(t *testing.T) string { v := in(); v.Prompt = strings.Repeat("p", 24576); return variant(t, v) }},
		{"surrogate-pair", func(t *testing.T) string {
			v := in()
			v.Prompt, v.RawPrompt = "\U0001F600", `"`+u("d83d")+u("de00")+`"`
			return variant(t, v)
		}},
		{"escapes-and-lf", func(t *testing.T) string {
			v := in()
			v.Prompt, v.RawPrompt = "a/b\nc\"\x01\xe2\x80\xa8", `"a\/b\nc\"`+u("0001")+u("2028")+`"`
			return variant(t, v)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Decode([]byte(c.doc(t))); err != nil {
				t.Fatalf("admitted neighbor refused: %v", err)
			}
		})
	}
}

// Shape success is not authority: identity, pin and timing changes that D
// must refuse against the protected original are shape-valid here, so the
// adapter does not pretend to perform D's protected equality (spec §3).
func TestDOwnedChangesAreShapeValid(t *testing.T) {
	cases := map[string]func(t *testing.T) string{
		"identity":   sub(`"uid": 30002`, `"uid": 30003`),
		"generation": sub(`"cccccccccccccccccccccccccccccccc"`, `"00000000000000000000000000000000"`),
		"association": sub(`"revision": 1,`+"\n      \"uid\"",
			`"revision": 2,`+"\n      \"uid\""),
		"pins":            sub(`"sha256:5555555555555555555555555555555555555555555555555555555555555555"`, `"sha256:0000000000000000000000000000000000000000000000000000000000000000"`),
		"pin-drift":       sub(`"sha256:3333333333333333333333333333333333333333333333333333333333333333"`, `"sha256:3333333333333333333333333333333333333333333333333333333333333334"`),
		"tampered-timing": sub(`"slack_ms": 10000`, `"slack_ms": 10001`),
		"catalog": func(t *testing.T) string {
			v := specimenInput()
			v.Effort = "not-in-any-catalog"
			return variant(t, v)
		},
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(doc(t))); err != nil {
				t.Fatalf("shape-valid D-owned change refused by the adapter: %v", err)
			}
		})
	}
}

// Token ceiling: exactly 512 tokens passes the lexer (and then fails only
// the closed table), 513 refuses as entry_token_limit.
func TestTokenBound(t *testing.T) {
	build := func(ints, objs int) string {
		var b strings.Builder
		for i := 0; i < ints; i++ {
			fmt.Fprintf(&b, `"a%03d":1,`, i)
		}
		for i := 0; i < objs; i++ {
			fmt.Fprintf(&b, `"b%03d":{},`, i)
		}
		s := specimen(t)
		return "{" + b.String() + s[1:]
	}
	// 173 specimen tokens + 4 per int member + 5 per empty-object member.
	at := build(81, 3)
	p, _, err := parse([]byte(at))
	if err != nil || p.tokens != 512 {
		t.Fatalf("512-token document refused by the lexer or miscounted: %v", err)
	}
	if got := reasonFor(decodeErr(at)); got != ReasonEntryUnknown {
		t.Fatalf("512 tokens: reason %q, want %q", got, ReasonEntryUnknown)
	}
	if got := reasonFor(decodeErr(build(85, 0))); got != ReasonEntryTokenLimit {
		t.Fatalf("513 tokens: reason %q, want %q", got, ReasonEntryTokenLimit)
	}
}

func decodeErr(doc string) error {
	_, err := Decode([]byte(doc))
	return err
}

// Depth ceiling: object nesting 8 passes the lexer (prompt then fails as a
// type), nesting 9 refuses as entry_depth_limit. Input is at depth 2.
func TestDepthBound(t *testing.T) {
	for k, want := range map[int]Reason{6: ReasonEntryType, 7: ReasonEntryDepthLimit} {
		doc := replaceOnce(t, specimen(t), specimenPrompt, `"prompt": `+nested(k))
		if got := reasonFor(decodeErr(doc)); got != want {
			t.Errorf("depth %d: reason %q, want %q", 2+k, got, want)
		}
	}
}

// Encoded ceiling: 49152 bytes decodes to the witness digest (whitespace is
// not identity), 49153 refuses before parsing.
func TestEncodedBound(t *testing.T) {
	w := loadWitness(t)
	s := specimen(t)
	e, err := Decode([]byte(strings.Repeat("\n", 49152-len(s)) + s))
	if err != nil || e.EncodedBytes != 49152 || e.EntryDigest != w.EntryCCJSHA {
		t.Fatalf("49152-byte document not admitted with the witness digest: %v", err)
	}
	if got := reasonFor(decodeErr(strings.Repeat("\n", 49153-len(s)) + s)); got != ReasonEntrySizeLimit {
		t.Fatalf("49153 bytes: reason %q", got)
	}
}

// Prompt ceiling counts decoded UTF-8 bytes, not runes: 24576 bytes of
// two-byte runes and LFs are admitted and preserved exactly, one more byte
// refuses as entry_prompt_length.
func TestPromptBound(t *testing.T) {
	v := specimenInput()
	v.Prompt = strings.Repeat("é\n", 8192)
	e, err := Decode([]byte(variant(t, v)))
	if err != nil || !bytes.Equal(e.Prompt, []byte(v.Prompt)) || len(e.Prompt) != 24576 {
		t.Fatalf("24576-byte prompt: %v", err)
	}
	v.Prompt += "x"
	if got := reasonFor(decodeErr(variant(t, v))); got != ReasonEntryPromptLength {
		t.Fatalf("24577-byte prompt: reason %q", got)
	}
}

// Escapes count toward the encoded bound: the largest run of \u0001 escapes
// that fits 49152 encoded bytes is admitted, one more escape refuses as
// entry_size_limit although the decoded prompt is far below 24576 bytes.
func TestEscapeExpansionBound(t *testing.T) {
	doc := func(n int) string {
		v := specimenInput()
		v.Prompt = strings.Repeat("\x01", n)
		v.RawPrompt = `"` + strings.Repeat(`\u0001`, n) + `"`
		return variant(t, v)
	}
	base := len(doc(0)) - 2 // doc(0) has an empty prompt literal ""
	n := (49152 - base - 2) / 6
	if len(doc(n)) > 49152 || len(doc(n+1)) <= 49152 {
		t.Fatalf("bound arithmetic: %d %d", len(doc(n)), len(doc(n+1)))
	}
	if err := decodeErr(doc(n)); err != nil {
		t.Fatalf("%d escapes (%d bytes) refused: %v", n, len(doc(n)), err)
	}
	if got := reasonFor(decodeErr(doc(n + 1))); got != ReasonEntrySizeLimit {
		t.Fatalf("%d escapes: reason %q", n+1, got)
	}
}

// Decoded string total excludes names: 32768 decoded value bytes reach the
// closed table (unknown member), 32769 refuse as entry_string_total.
func TestDecodedStringTotal(t *testing.T) {
	build := func(last int) string {
		var b strings.Builder
		b.WriteString("{")
		for i := 0; i < 81; i++ {
			fmt.Fprintf(&b, `"s%02d":"%s",`, i, strings.Repeat("x", 400))
		}
		fmt.Fprintf(&b, `"last":"%s"}`, strings.Repeat("y", last))
		return b.String()
	}
	if got := reasonFor(decodeErr(build(368))); got != ReasonEntryUnknown {
		t.Fatalf("32768 decoded bytes: reason %q", got)
	}
	if got := reasonFor(decodeErr(build(369))); got != ReasonEntryStringTotal {
		t.Fatalf("32769 decoded bytes: reason %q", got)
	}
}

// Member order, insignificant whitespace and escape spelling do not change
// CCJ-1 identity: a reordered, compact, escaped spelling of the specimen has
// the witness entry and input digests.
func TestCanonicalIdentityIgnoresSpelling(t *testing.T) {
	w := loadWitness(t)
	doc := `{"input":{"prompt":"Reply with the word ex` + u("0061") + `mple.","placement":"station-one-shot-v1",` +
		`"reasoning_effort":"catalog-effort","model":"catalog-model","profile":"codex-refresher-v0","` + u("0061") + `ccount":"example"},` +
		`"timing":{"slack_ms":10000,"deadline_ms":290000},` +
		`"qualification":"sha256:6666666666666666666666666666666666666666666666666666666666666666",` +
		`"catalog":"sha256:5555555555555555555555555555555555555555555555555555555555555555",` +
		`"execution":{"controller_epoch":1,"execution_id":"EX-11111111111111111111111111111111","effect_id":"ffffffffffffffffffffffffffffffff"},` +
		`"request":{"op":"dispatch","plan":{"exec_profile":"sha256:4444444444444444444444444444444444444444444444444444444444444444",` +
		`"harness":"sha256:3333333333333333333333333333333333333333333333333333333333333333",` +
		`"profile":"sha256:2222222222222222222222222222222222222222222222222222222222222222",` +
		`"worker_plan":"sha256:c02aff6b4c3f3a0a79f47d6e407472e62bf8cd632c93a6c8268a0de31ddda080"},` +
		`"worker":{"generation":"cccccccccccccccccccccccccccccccc","uid":30002},` +
		`"association":{"master_epoch":1,"registration_number":1,"key":"ed25519:11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo",` +
		`"generation":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","uid":30001,"revision":1,"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},` +
		`"policy":{"revision":1,"id":"dddddddddddddddddddddddddddddddd"},` +
		`"` + u("0072") + `eceiver":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","request_id":"00000000000000000000000000000001",` +
		`"kind":"swarma-dispatcher m0-request\/1","v":1},` +
		` "kind" : "swarma-dispatcher m0-executor-entry\/1" , "v" : 1 }`
	e, err := Decode([]byte(doc))
	if err != nil {
		t.Fatalf("respelled specimen refused: %v", err)
	}
	if e.EntryDigest != w.EntryCCJSHA || e.InputDigest != w.InputCCJSHA || e.CanonicalBytes != w.EntryCCJBytes {
		t.Fatalf("respelled identity %s / %s, want %s / %s", e.EntryDigest, e.InputDigest, w.EntryCCJSHA, w.InputCCJSHA)
	}
}

// CCJ-1 emission rules: only the prescribed escapes (lowercase \u00xx for
// other controls), "/" and non-ASCII raw, names sorted by byte order,
// shortest integers.
func TestCanonicalEmissionRules(t *testing.T) {
	cases := map[string]string{
		`{"s":"\"\\\/\b\f\n\r\t` + u("0001") + u("001F") + "\x7f" + u("00e9") + u("2028") + u("d83d") + u("de00") + `"}`: `{"s":"\"\\/\b\f\n\r\t` + u("0001") + u("001f") + "\x7f\xc3\xa9\xe2\x80\xa8\xf0\x9f\x98\x80" + `"}`,
		`{"b":1,"a":2,"B":3,"_":4}`:              `{"B":3,"_":4,"a":2,"b":1}`,
		`{ "n" : -5 , "z" : 0 }`:                 `{"n":-5,"z":0}`,
		`{"o":{"y":{},"x":"` + u("0041") + `"}}`: `{"o":{"x":"A","y":{}}}`,
	}
	for in, want := range cases {
		p, root, err := parse([]byte(in))
		if err != nil {
			t.Fatalf("parse %q: %v", in, err)
		}
		c, err := canonicalize(p, root, maxCanonical)
		if err != nil {
			t.Fatalf("CCJ-1(%q): %v", in, err)
		}
		if string(c.out) != want {
			t.Errorf("CCJ-1(%q) = %q, want %q", in, c.out, want)
		}
	}
}

// The canonical bound is enforced at the exact limit: the specimen needs
// 1494 bytes, so limit 1494 succeeds and 1493 refuses entry_canonical_limit.
// Limit: with the encoded bound at 49152 this ceiling cannot be exceeded by
// FD3 input (CCJ-1 never lengthens admitted JSON), so it is tested directly.
func TestCanonicalBound(t *testing.T) {
	p, root, err := parse([]byte(specimen(t)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := canonicalize(p, root, 1494); err != nil {
		t.Fatalf("exact limit refused: %v", err)
	}
	if _, err := canonicalize(p, root, 1493); reasonFor(err) != ReasonEntryCanonicalLimit {
		t.Fatalf("limit-1: %v", err)
	}
}

// Refusal text carries only the code: no prompt bytes or other input values
// leak into the error used for the diagnostic line.
func TestRefusalNeverEchoesInput(t *testing.T) {
	doc := replaceOnce(t, specimen(t), specimenPrompt, `"prompt": "SECRET-PROMPT", "x": "SECRET-VALUE"`)
	err := decodeErr(doc)
	line := string(diagnosticLine(reasonFor(err)))
	if strings.Contains(line, "SECRET") || line != diagnosticPrefix+string(ReasonEntryUnknown)+"\n" {
		t.Fatalf("diagnostic %q", line)
	}
}
