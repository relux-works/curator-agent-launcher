package m0adapter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain lets the test binary double as the process helpers of the Linux
// process tests (adapter child, production child, fake runner). helperMain
// returns at once in an ordinary test run. Before the tests run,
// isolateDescriptors marks every descriptor above 2 that this test process
// inherited (for example from the CI runner or make) close-on-exec, so the
// children see exactly the descriptors each test chooses. The adapter itself
// still refuses any such stray descriptor; this only keeps the harness's own
// inheritance out of the children.
func TestMain(m *testing.M) {
	helperMain()
	isolateDescriptors()
	os.Exit(m.Run())
}

// Synthetic literals used by tests; never installation values.
const (
	testKeymakerMarker = "keymaker-endpoint.invalid"
	testDeniedPrefix   = "/nonexistent-denied-prefix"
	testClockProfile   = "test-clock"
	testOriginMarker   = "origin.sock"
)

type witness struct {
	Protected struct {
		E     string `json:"E"`
		H     string `json:"H"`
		NowMs int64  `json:"now_ms"`
	} `json:"protected_example"`
	Argv              []string `json:"argv"`
	ArgvBytes         int      `json:"argv_nul_terminated_bytes"`
	StdinBytes        int      `json:"stdin_utf8_bytes"`
	StdinSHA          string   `json:"stdin_sha256"`
	StdinEOF          bool     `json:"stdin_eof"`
	AddedLF           bool     `json:"added_lf"`
	InputCCJBytes     int      `json:"input_ccj1_bytes"`
	InputCCJSHA       string   `json:"input_ccj1_sha256"`
	EntryCCJBytes     int      `json:"entry_ccj1_bytes"`
	EntryCCJSHA       string   `json:"entry_ccj1_sha256"`
	EntryObjects      int      `json:"entry_objects"`
	EntryMembers      int      `json:"entry_members"`
	EntryTokens       int      `json:"entry_lexical_tokens"`
	TimeoutSeconds    int64    `json:"timeout_seconds"`
	StatusDescription string   `json:"status"`
}

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata %s: %v", name, err)
	}
	return b
}

func specimen(t *testing.T) string { return string(readTestdata(t, "entry.specimen.json")) }

func loadWitness(t *testing.T) witness {
	t.Helper()
	var w witness
	if err := json.Unmarshal(readTestdata(t, "mapping.witness.json"), &w); err != nil {
		t.Fatalf("witness: %v", err)
	}
	return w
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(s[:])
}

// replaceOnce replaces exactly one occurrence; any other count is a broken
// fixture mutation, not a refusal.
func replaceOnce(t *testing.T, s, old, new string) string {
	t.Helper()
	if n := strings.Count(s, old); n != 1 {
		t.Fatalf("mutation anchor %q occurs %d times", old, n)
	}
	return strings.Replace(s, old, new, 1)
}

// ccjString is a test-local CCJ-1 string encoder written independently of
// canonical.go. It is used only to recompute worker_plan for input variants
// and is itself pinned to the witness by TestIndependentInputCanonical.
func ccjString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 0x20 {
				b.WriteString(`\u00`)
				b.WriteByte("0123456789abcdef"[c>>4])
				b.WriteByte("0123456789abcdef"[c&0xf])
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// inputValues are the six Input members of a variant.
type inputValues struct {
	Account, Profile, Model, Effort, Prompt string
	// RawPrompt, when set, is the exact JSON literal written for prompt in
	// the document (to exercise escapes); Prompt must be its decoded value.
	RawPrompt string
}

func specimenInput() inputValues {
	return inputValues{
		Account: "example", Profile: "codex-refresher-v0", Model: "catalog-model",
		Effort: "catalog-effort", Prompt: "Reply with the word example.",
	}
}

// inputCCJ is CCJ-1(input) assembled by hand: members sorted by name.
func inputCCJ(in inputValues) []byte {
	return []byte(`{"account":` + ccjString(in.Account) +
		`,"model":` + ccjString(in.Model) +
		`,"placement":"station-one-shot-v1"` +
		`,"profile":` + ccjString(in.Profile) +
		`,"prompt":` + ccjString(in.Prompt) +
		`,"reasoning_effort":` + ccjString(in.Effort) + `}`)
}

const specimenWorkerPlan = "sha256:c02aff6b4c3f3a0a79f47d6e407472e62bf8cd632c93a6c8268a0de31ddda080"

// variant rebuilds the specimen with new input values and a worker_plan
// recomputed by the independent inputCCJ, so the entry stays consistent.
func variant(t *testing.T, in inputValues) string {
	t.Helper()
	s := specimen(t)
	i := strings.Index(s, `"input": {`)
	if i < 0 || !strings.HasSuffix(s, "  }\n}\n") {
		t.Fatalf("specimen layout changed")
	}
	prompt := in.RawPrompt
	if prompt == "" {
		prompt = ccjString(in.Prompt)
	}
	block := `"input": {` + "\n" +
		`    "account": ` + ccjString(in.Account) + ",\n" +
		`    "profile": ` + ccjString(in.Profile) + ",\n" +
		`    "model": ` + ccjString(in.Model) + ",\n" +
		`    "reasoning_effort": ` + ccjString(in.Effort) + ",\n" +
		`    "placement": "station-one-shot-v1",` + "\n" +
		`    "prompt": ` + prompt + "\n" +
		"  }\n}\n"
	head := replaceOnce(t, s[:i], specimenWorkerPlan, sha(inputCCJ(in)))
	return head + block
}

func reasonFor(err error) Reason {
	if err == nil {
		return ""
	}
	var r *refusal
	if !errors.As(err, &r) {
		return ReasonInternal
	}
	return r.reason
}

func testLiterals(e, h string) Literals {
	return Literals{
		Executable: e, Home: h, RunnerCommand: codexRunnerCommand,
		ClockProfile: testClockProfile, KeymakerMarker: testKeymakerMarker,
		DeniedPathPrefix: testDeniedPrefix, OriginSocketMarker: testOriginMarker,
	}
}

// fixedEnv is the manager's fixed six-variable environment for home h.
func fixedEnv(h string) []string {
	return []string{
		"HOME=" + h, "USER=worker", "LOGNAME=worker", "PATH=/usr/bin:/bin",
		"LANG=C.UTF-8", "TMPDIR=" + h + "/tmp",
	}
}

// u returns the six-byte JSON escape backslash, 'u', hex. Test sources build
// every printable JSON \u-escape through it, so the escape spelling under
// test is never decoded by an editor or tool before it reaches the parser.
func u(hex string) string { return "\\" + "u" + hex }
