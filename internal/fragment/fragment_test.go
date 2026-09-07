package fragment

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The parser is driven against two authoritative corpora:
//
//   - testdata/schema-cases: the launch-env-fragment-v1 rows of curator-spec
//     conformance/v1/schema-cases/index.json at 87a0d00, copied verbatim with
//     their recorded valid/invalid verdicts. One violated rule per negative
//     case, so each is a narrowing probe of exactly one gate.
//   - testdata/a0: the three fragments the installed curator
//     (v0.14.1-0.20260907213730-04550e282705) printed for the `default`
//     profile on 2026-09-07 (A0, TASK-260908-qblycn), with the sha256 values
//     recorded beside them.

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestConformanceCorpus(t *testing.T) {
	var index struct {
		Cases []struct {
			Instance string `json:"instance"`
			Valid    bool   `json:"valid"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(readFile(t, "testdata/schema-cases/index.json"), &index); err != nil {
		t.Fatal(err)
	}
	// Independently derived from curator-spec 87a0d0060bad64ab883d007dcdf35df7485368bf:
	// select launch-env-fragment-v1.schema.json rows in upstream index order;
	// hash each verbatim fixture, then hash "basename\tvalid\tsha256\n" rows.
	// This pins identities, verdicts, order and bytes, not parser enumeration.
	const wantManifest = "540f9d32f14490c9293454f466c9e139cc112971a1147d038369702bd7984e60"
	var manifest strings.Builder
	for _, c := range index.Cases {
		data := readFile(t, filepath.Join("testdata/schema-cases", c.Instance))
		fmt.Fprintf(&manifest, "%s\t%t\t%x\n", c.Instance, c.Valid, sha256.Sum256(data))
	}
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(manifest.String()))); got != wantManifest || len(index.Cases) != 49 {
		t.Fatalf("pinned corpus correspondence: got %d rows digest %s; want 49 rows digest %s", len(index.Cases), got, wantManifest)
	}
	valid, invalid := 0, 0
	for _, c := range index.Cases {
		data := readFile(t, filepath.Join("testdata/schema-cases", c.Instance))
		f, err := Parse(data)
		if c.Valid {
			valid++
			if err != nil {
				t.Errorf("%s: expected valid, got %v", c.Instance, err)
				continue
			}
			if !strings.HasPrefix(f.Digest, DigestPrefix) || len(f.Digest) != len(DigestPrefix)+64 {
				t.Errorf("%s: malformed digest %q", c.Instance, f.Digest)
			}
			// The corpus is pretty-printed; the canonical form must be the
			// same object with whitespace removed and keys sorted, i.e. the
			// parse of the canonical bytes must yield the same digest.
			g, err := Parse(f.Canonical)
			if err != nil || g.Digest != f.Digest {
				t.Errorf("%s: canonical bytes do not round-trip: %v", c.Instance, err)
			}
		} else {
			invalid++
			if err == nil {
				t.Errorf("%s: expected rejection, parsed with digest %s", c.Instance, f.Digest)
			} else if !IsInvalid(err) {
				t.Errorf("%s: rejection is not a validity error: %v", c.Instance, err)
			}
		}
	}
	t.Logf("corpus: %d valid, %d invalid cases driven", valid, invalid)
}

// a0Digests reads the recorded "<env> exit=0 sha256:<hex>" lines.
func a0Digests(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(string(readFile(t, "testdata/a0/digests.txt"))))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 3 {
			continue
		}
		if fields[1] != "exit=0" {
			t.Fatalf("unexpected digest line %q", sc.Text())
		}
		out[fields[0]] = fields[2]
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 recorded digests, got %d", len(out))
	}
	return out
}

func TestA0FragmentsParseAndDigestMatch(t *testing.T) {
	want := a0Digests(t)
	homes := map[string]string{
		EnvClaudeCode: "/Users/iv/.curator/environments/default/claude_code",
		EnvCodexCLI:   "/Users/iv/.curator/environments/default/codex_cli",
		EnvPi:         "/Users/iv/.curator/environments/default/pi",
	}
	for env, digest := range want {
		raw := readFile(t, filepath.Join("testdata/a0", env+".json"))
		f, err := Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", env, err)
		}
		if f.Digest != digest {
			t.Errorf("%s: digest %s, recorded %s", env, f.Digest, digest)
		}
		// environments.md §10.1: the printed line is the CCJ-1 bytes plus
		// exactly one LF, so the canonical form equals the line without it.
		if string(f.Canonical) != strings.TrimSuffix(string(raw), "\n") {
			t.Errorf("%s: canonical bytes differ from the printed line", env)
		}
		if f.Environment != env || f.Home() != homes[env] || f.Env[HomeVariable(env)] != homes[env] {
			t.Errorf("%s: environment/home = %q/%q", env, f.Environment, f.Home())
		}
		if f.Profile.Name != "default" || f.Profile.LockSHA256 != "726310f80f442428a9a640d2d49ad9b635f22857fb4ad22167832c7c6ff30e19" {
			t.Errorf("%s: profile %+v", env, f.Profile)
		}
		if f.Precedence != (Precedence{Winner: "higher-weight", Placement: "winner-last"}) {
			t.Errorf("%s: precedence %+v", env, f.Precedence)
		}
		if f.SystemPrompt != nil || f.MCP != nil || f.PathPrepend != "" {
			t.Errorf("%s: optional sections must stay absent", env)
		}
	}
}

// mutate applies a JSON-level edit to a valid fragment and returns the bytes.
func mutate(t *testing.T, base []byte, edit func(m map[string]any)) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(base, &m); err != nil {
		t.Fatal(err)
	}
	edit(m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDigestInvariantUnderPrintingAndSensitiveToBytes(t *testing.T) {
	raw := readFile(t, "testdata/a0/claude_code.json")
	f, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	// Pretty-printing, whitespace and key order changes: same digest.
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	pretty, _ := json.MarshalIndent(m, "", "    ")
	reordered := `{"profile":{"name":"default","lock_sha256":"726310f80f442428a9a640d2d49ad9b635f22857fb4ad22167832c7c6ff30e19"},
	  "precedence":{"winner":"higher-weight","placement":"winner-last"},
	  "fragment":"launch-env-fragment-v1", "environment":"claude_code",
	  "env":{"CLAUDE_CONFIG_DIR":"\/Users\/iv\/.curator\/environments\/default\/claude_code"}}` + "\r\n"
	for name, in := range map[string][]byte{"pretty": pretty, "reordered+escaped-slash+CRLF": []byte(reordered)} {
		g, err := Parse(in)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if g.Digest != f.Digest {
			t.Errorf("%s: digest changed to %s", name, g.Digest)
		}
	}
	// One byte in a consumed member: a different digest.
	changed := mutate(t, raw, func(m map[string]any) {
		m["profile"].(map[string]any)["lock_sha256"] = "726310f80f442428a9a640d2d49ad9b635f22857fb4ad22167832c7c6ff30e1a"
	})
	g, err := Parse(changed)
	if err != nil {
		t.Fatal(err)
	}
	if g.Digest == f.Digest {
		t.Error("lock hash change left the digest unchanged")
	}
}

// valid fixtures for the optional sections, taken from the schema example
// shapes (environments.md §10.2) and the conformance corpus.
const (
	claudeFull = `{"fragment":"launch-env-fragment-v1","environment":"claude_code",
	 "profile":{"name":"companyA","lock_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
	 "precedence":{"winner":"higher-weight","placement":"winner-last"},
	 "env":{"CLAUDE_CONFIG_DIR":"/manager/environments/companyA/claude_code"},
	 "system_prompt":{"path":"/manager/environments/companyA/claude_code/.agent-context/system-prompt.md",
	   "channels":[{"kind":"flag","semantics":"append","flag":"--append-system-prompt-file","argument":"path"},
	               {"kind":"flag","semantics":"replace","flag":"--system-prompt-file","argument":"path"}]},
	 "mcp":{"path":"/manager/environments/companyA/claude_code/.agent-context/mcp/claude_code.json",
	   "env_names":["FIGMA_API_KEY"],
	   "channels":[{"kind":"flag","flag":"--mcp-config","argument":"path","with":["--strict-mcp-config"]}]}}`
	codexFull = `{"fragment":"launch-env-fragment-v1","environment":"codex_cli",
	 "profile":{"name":"companyA","lock_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
	 "precedence":{"winner":"lower-weight","placement":"winner-first"},
	 "env":{"CODEX_HOME":"/manager/environments/companyA/codex_cli"},
	 "system_prompt":{"path":"/manager/environments/companyA/codex_cli/.agent-context/system-prompt.md",
	   "channels":[{"kind":"config-key","semantics":"replace","key":"model_instructions_file"}]},
	 "mcp":{"path":"/manager/environments/companyA/codex_cli/curator-mcp.config.toml","env_names":[],
	   "channels":[{"kind":"flag","flag":"-p","argument":"name","name":"curator-mcp"}]}}`
)

func TestOptionalSectionsParsed(t *testing.T) {
	f, err := Parse([]byte(claudeFull))
	if err != nil {
		t.Fatal(err)
	}
	if f.SystemPrompt == nil || len(f.SystemPrompt.Channels) != 2 || f.SystemPrompt.Channels[1].Semantics != SemanticsReplace {
		t.Errorf("system_prompt = %+v", f.SystemPrompt)
	}
	if f.MCP == nil || len(f.MCP.EnvNames) != 1 || len(f.MCP.Channels) != 1 || f.MCP.Channels[0].With[0] != "--strict-mcp-config" || f.MCP.Channels[0].Semantics != "" {
		t.Errorf("mcp = %+v", f.MCP)
	}
	c, err := Parse([]byte(codexFull))
	if err != nil {
		t.Fatal(err)
	}
	if c.MCP == nil || c.MCP.Channels[0].Name != "curator-mcp" || c.MCP.Channels[0].Argument != ArgumentName || len(c.MCP.EnvNames) != 0 {
		t.Errorf("codex mcp = %+v", c.MCP)
	}
	if c.Precedence != (Precedence{Winner: "lower-weight", Placement: "winner-first"}) {
		t.Errorf("precedence = %+v", c.Precedence)
	}
}

// TestRejections drives one rule per case beyond what the vendored corpus
// covers: the CCJ-1 reader rules and the fragment-specific closures.
func TestRejections(t *testing.T) {
	a0 := string(readFile(t, "testdata/a0/pi.json"))
	cases := map[string]string{
		"empty input":            ``,
		"not an object":          `[]`,
		"trailing content":       a0 + `{}`,
		"trailing garbage":       strings.TrimSpace(a0) + ` x`,
		"duplicate top-level":    strings.Replace(a0, `"environment":"pi",`, `"environment":"pi","environment":"pi",`, 1),
		"duplicate nested":       strings.Replace(a0, `"name":"default"`, `"name":"default","name":"default"`, 1),
		"invalid utf-8":          strings.Replace(a0, `"default"`, "\"def\xffault\"", 1),
		"lone high surrogate":    strings.Replace(a0, `"default"`, `"\ud800x"`, 1),
		"lone low surrogate":     strings.Replace(a0, `"default"`, `"\udc00"`, 1),
		"high then non-low":      strings.Replace(a0, `"default"`, `"\ud800A"`, 1),
		"raw control in string":  strings.Replace(a0, `"default"`, "\"def\x01ault\"", 1),
		"bad escape":             strings.Replace(a0, `"default"`, `"def\qault"`, 1),
		"non-integer number":     strings.Replace(a0, `"placement":"winner-last"`, `"placement":1.5`, 1),
		"exponent number":        strings.Replace(a0, `"placement":"winner-last"`, `"placement":1e2`, 1),
		"negative zero":          strings.Replace(a0, `"placement":"winner-last"`, `"placement":-0`, 1),
		"integer out of range":   strings.Replace(a0, `"placement":"winner-last"`, `"placement":9007199254740992`, 1),
		"leading zero":           strings.Replace(a0, `"placement":"winner-last"`, `"placement":01`, 1),
		"composition withdrawn":  strings.Replace(a0, `"environment":"pi",`, `"environment":"pi","composition":[],`, 1),
		"sig member":             strings.Replace(a0, `"environment":"pi",`, `"environment":"pi","sig":"x",`, 1),
		"wrong identity":         strings.Replace(a0, `launch-env-fragment-v1`, `launch-env-fragment-v2`, 1),
		"environment mismatch":   strings.Replace(a0, `"environment":"pi"`, `"environment":"codex_cli"`, 1),
		"env value not string":   strings.Replace(a0, `"/Users/iv/.curator/environments/default/pi"`, `1`, 1),
		"env relative":           strings.Replace(a0, `"/Users/iv/.curator/environments/default/pi"`, `"Users/pi"`, 1),
		"env dotdot":             strings.Replace(a0, `"/Users/iv/.curator/environments/default/pi"`, `"/Users/iv/../pi"`, 1),
		"env NUL":                strings.Replace(a0, `"/Users/iv/.curator/environments/default/pi"`, "\"/Users/\x00pi\"", 1),
		"lock with prefix":       strings.Replace(a0, `"lock_sha256":"7`, `"lock_sha256":"sha256:7`, 1),
		"lock uppercase":         strings.Replace(a0, `"lock_sha256":"726310f8`, `"lock_sha256":"726310F8`, 1),
		"profile name empty":     strings.Replace(a0, `"name":"default"`, `"name":""`, 1),
		"profile name reserved":  strings.Replace(a0, `"name":"default"`, `"name":"CON"`, 1),
		"profile extra member":   strings.Replace(a0, `"name":"default"`, `"name":"default","commit":"abc"`, 1),
		"precedence string":      strings.Replace(a0, `{"placement":"winner-last","winner":"higher-weight"}`, `"later-overrides-earlier"`, 1),
		"precedence unknown":     strings.Replace(a0, `"winner-last"`, `"winner-middle"`, 1),
		"precedence missing key": strings.Replace(a0, `"placement":"winner-last",`, ``, 1),
		"mcp on pi":              strings.Replace(a0, `"environment":"pi",`, `"environment":"pi","mcp":{"path":"/a/b","env_names":[],"channels":[{"kind":"variable","variable":"X"}]},`, 1),
		"path_prepend relative":  strings.Replace(a0, `"environment":"pi",`, `"environment":"pi","path_prepend":"bin",`, 1),
		"path_prepend outside":   strings.Replace(a0, `"environment":"pi",`, `"environment":"pi","path_prepend":"/usr/local/bin",`, 1),
		"path_prepend root self": strings.Replace(a0, `"environment":"pi",`, `"environment":"pi","path_prepend":"/Users/iv/.curator/environments",`, 1),
		"path_prepend not str":   strings.Replace(a0, `"environment":"pi",`, `"environment":"pi","path_prepend":7,`, 1),
		"system_prompt not obj":  strings.Replace(a0, `"environment":"pi",`, `"environment":"pi","system_prompt":"x",`, 1),
		"sp channels not array":  strings.Replace(a0, `"environment":"pi",`, `"environment":"pi","system_prompt":{"path":"/p/x.md","channels":{}},`, 1),
		"sp channel not object":  strings.Replace(a0, `"environment":"pi",`, `"environment":"pi","system_prompt":{"path":"/p/x.md","channels":[1]},`, 1),
		"sp channels subset":     strings.Replace(a0, `"environment":"pi",`, `"environment":"pi","system_prompt":{"path":"/p/x.md","channels":[{"kind":"flag","semantics":"append","flag":"--append-system-prompt","argument":"path"}]},`, 1),
		"sp channels reordered":  strings.Replace(a0, `"environment":"pi",`, `"environment":"pi","system_prompt":{"path":"/p/x.md","channels":[{"kind":"file","semantics":"append","filename":"APPEND_SYSTEM.md"},{"kind":"flag","semantics":"append","flag":"--append-system-prompt","argument":"path"},{"kind":"file","semantics":"replace","filename":"SYSTEM.md"}]},`, 1),
		"sp file with slash":     strings.Replace(a0, `"environment":"pi",`, `"environment":"pi","system_prompt":{"path":"/p/x.md","channels":[{"kind":"flag","semantics":"append","flag":"--append-system-prompt","argument":"path"},{"kind":"file","semantics":"append","filename":"a/APPEND_SYSTEM.md"},{"kind":"file","semantics":"replace","filename":"SYSTEM.md"}]},`, 1),
		"sp path dotdot":         strings.Replace(a0, `"environment":"pi",`, `"environment":"pi","system_prompt":{"path":"/p/../x.md","channels":[]},`, 1),
		"deep nesting":           strings.Repeat("[", 100) + strings.Repeat("]", 100),
		// mutations of the full claude fixture
		"mcp env_names unsorted":     strings.Replace(claudeFull, `["FIGMA_API_KEY"]`, `["FIGMA_API_KEY","DOCS_TOKEN"]`, 1),
		"mcp env_names duplicate":    strings.Replace(claudeFull, `["FIGMA_API_KEY"]`, `["FIGMA_API_KEY","FIGMA_API_KEY"]`, 1),
		"mcp env_names reserved":     strings.Replace(claudeFull, `["FIGMA_API_KEY"]`, `["DYLD_LIBRARY_PATH"]`, 1),
		"mcp env_names not string":   strings.Replace(claudeFull, `["FIGMA_API_KEY"]`, `[1]`, 1),
		"mcp env_names not array":    strings.Replace(claudeFull, `["FIGMA_API_KEY"]`, `"FIGMA_API_KEY"`, 1),
		"mcp semantics present":      strings.Replace(claudeFull, `{"kind":"flag","flag":"--mcp-config"`, `{"kind":"flag","semantics":"append","flag":"--mcp-config"`, 1),
		"mcp with missing":           strings.Replace(claudeFull, `,"with":["--strict-mcp-config"]`, ``, 1),
		"mcp with empty":             strings.Replace(claudeFull, `["--strict-mcp-config"]`, `[]`, 1),
		"mcp with duplicate":         strings.Replace(claudeFull, `["--strict-mcp-config"]`, `["--strict-mcp-config","--strict-mcp-config"]`, 1),
		"mcp with not flag token":    strings.Replace(claudeFull, `["--strict-mcp-config"]`, `["strict"]`, 1),
		"mcp with not array":         strings.Replace(claudeFull, `["--strict-mcp-config"]`, `"--strict-mcp-config"`, 1),
		"mcp two channels":           strings.Replace(claudeFull, `"channels":[{"kind":"flag","flag":"--mcp-config"`, `"channels":[{"kind":"flag","flag":"--mcp-config","argument":"path","with":["--strict-mcp-config"]},{"kind":"flag","flag":"--mcp-config"`, 1),
		"mcp unknown member":         strings.Replace(claudeFull, `"env_names":`, `"servers":[],"env_names":`, 1),
		"mcp missing env_names":      strings.Replace(claudeFull, `"env_names":["FIGMA_API_KEY"],`, ``, 1),
		"channel unknown kind":       strings.Replace(claudeFull, `{"kind":"flag","semantics":"append"`, `{"kind":"env","semantics":"append"`, 1),
		"channel unknown semantics":  strings.Replace(claudeFull, `"semantics":"append"`, `"semantics":"prepend"`, 1),
		"channel missing semantics":  strings.Replace(claudeFull, `"semantics":"append",`, ``, 1),
		"channel unknown member":     strings.Replace(claudeFull, `"argument":"path"},`, `"argument":"path","extra":1},`, 1),
		"channel unknown argument":   strings.Replace(claudeFull, `"argument":"path"},`, `"argument":"file"},`, 1),
		"channel missing argument":   strings.Replace(claudeFull, `,"argument":"path"},`, `},`, 1),
		"channel flag not token":     strings.Replace(claudeFull, `"--append-system-prompt-file"`, `"append-system-prompt-file"`, 1),
		"channel flag with space":    strings.Replace(claudeFull, `"--append-system-prompt-file"`, `"--append system-prompt-file"`, 1),
		"channel name without arg":   strings.Replace(claudeFull, `"argument":"path"},`, `"argument":"path","name":"x"},`, 1),
		"channel name missing":       strings.Replace(codexFull, `,"name":"curator-mcp"`, ``, 1),
		"channel name not string":    strings.Replace(codexFull, `"name":"curator-mcp"`, `"name":1`, 1),
		"channel flag with filename": strings.Replace(claudeFull, `"argument":"path"},`, `"argument":"path","filename":"x"},`, 1),
		"config-key missing key":     strings.Replace(codexFull, `,"key":"model_instructions_file"`, ``, 1),
		"config-key not identifier":  strings.Replace(codexFull, `"model_instructions_file"`, `"model instructions"`, 1),
		"sp path relative":           strings.Replace(claudeFull, `"path":"/manager/environments/companyA/claude_code/.agent-context/system-prompt.md"`, `"path":"system-prompt.md"`, 1),
		"sp unknown member":          strings.Replace(claudeFull, `"system_prompt":{"path"`, `"system_prompt":{"mode":"x","path"`, 1),
		"sp missing channels": strings.Replace(claudeFull, `"channels":[{"kind":"flag","semantics":"append","flag":"--append-system-prompt-file","argument":"path"},
	               {"kind":"flag","semantics":"replace","flag":"--system-prompt-file","argument":"path"}]},`, `},`, 1),
		"env two variables":      strings.Replace(claudeFull, `"env":{"CLAUDE_CONFIG_DIR"`, `"env":{"CODEX_HOME":"/a/b","CLAUDE_CONFIG_DIR"`, 1),
		"env wrong adapter var":  strings.Replace(claudeFull, `"CLAUDE_CONFIG_DIR"`, `"CODEX_HOME"`, 1),
		"env lowercase var":      strings.Replace(claudeFull, `"CLAUDE_CONFIG_DIR"`, `"claude_config_dir"`, 1),
		"env empty":              strings.Replace(claudeFull, `"env":{"CLAUDE_CONFIG_DIR":"/manager/environments/companyA/claude_code"}`, `"env":{}`, 1),
		"env not object":         strings.Replace(claudeFull, `"env":{"CLAUDE_CONFIG_DIR":"/manager/environments/companyA/claude_code"}`, `"env":[]`, 1),
		"unknown environment":    strings.Replace(claudeFull, `"environment":"claude_code"`, `"environment":"cursor"`, 1),
		"environment not string": strings.Replace(claudeFull, `"environment":"claude_code"`, `"environment":1`, 1),
		"missing fragment":       strings.Replace(claudeFull, `"fragment":"launch-env-fragment-v1",`, ``, 1),
		"missing env":            strings.Replace(claudeFull, `"env":{"CLAUDE_CONFIG_DIR":"/manager/environments/companyA/claude_code"},`, ``, 1),
		"missing profile":        strings.Replace(claudeFull, `"profile":{"name":"companyA","lock_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},`, ``, 1),
		"profile not object":     strings.Replace(claudeFull, `"profile":{"name":"companyA","lock_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},`, `"profile":"companyA",`, 1),
		"lock not string":        strings.Replace(claudeFull, `"lock_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"`, `"lock_sha256":null`, 1),
		"unknown top-level":      strings.Replace(claudeFull, `"fragment":`, `"version":1,"fragment":`, 1),
	}
	for name, in := range cases {
		// Every case must differ from the fixture it was derived from, or the
		// replacement did not apply and the case tests nothing.
		if in == a0 || in == claudeFull || in == codexFull {
			t.Fatalf("%s: mutation did not apply", name)
		}
		f, err := Parse([]byte(in))
		if err == nil {
			t.Errorf("%s: accepted (digest %s)", name, f.Digest)
			continue
		}
		if !IsInvalid(err) {
			t.Errorf("%s: not a validity error: %v", name, err)
		}
	}
}

func TestOptionalAbsenceAndPathPrepend(t *testing.T) {
	// The reserved member, when inside the environments root derived from
	// the home, is accepted; the digest covers it.
	a0 := string(readFile(t, "testdata/a0/pi.json"))
	with := strings.Replace(a0, `"environment":"pi",`, `"environment":"pi","path_prepend":"/Users/iv/.curator/environments/default/bin",`, 1)
	f, err := Parse([]byte(with))
	if err != nil {
		t.Fatal(err)
	}
	if f.PathPrepend != "/Users/iv/.curator/environments/default/bin" {
		t.Errorf("PathPrepend = %q", f.PathPrepend)
	}
	base, _ := Parse([]byte(a0))
	if f.Digest == base.Digest {
		t.Error("path_prepend must change the digest")
	}
	// Absent optional sections are absent, not defaulted, in the canonical
	// bytes: no "system_prompt", "mcp" or "path_prepend" key appears.
	for _, key := range []string{`"system_prompt"`, `"mcp"`, `"path_prepend"`} {
		if strings.Contains(string(base.Canonical), key) {
			t.Errorf("canonical bytes carry absent member %s", key)
		}
	}
}

func TestInvalidErrorShape(t *testing.T) {
	_, err := Parse([]byte(strings.Replace(claudeFull, `["--strict-mcp-config"]`, `[]`, 1)))
	var inv *InvalidError
	if !asInvalid(err, &inv) || inv.Path != "/mcp/channels/0/with" {
		t.Fatalf("err = %v", err)
	}
	if !strings.HasPrefix(err.Error(), "fragment: /mcp/channels/0/with: ") {
		t.Errorf("message %q", err.Error())
	}
}
