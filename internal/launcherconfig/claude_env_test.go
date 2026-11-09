package launcherconfig

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

const claudeEnvCorpus = "testdata/machine-claude-env-v1"

// Blob IDs of the vendored corpus at curator-spec 32a8bf14 (see the README).
var claudeEnvCorpusBlobs = map[string]string{
	"cases.json":            "db4019f89cffd3726dc3f281dd57d22c08f02d66",
	"plants.json":           "e321757deed379795135a798499561d23d295250",
	"containing-cases.json": "51837cb9950c8d441f4e7fc14f3eed0a48b2a3a6",
}

type claudeEnvCase struct {
	ID          string `json:"id"`
	Claim       string `json:"claim"`
	Entry       string `json:"entry"`
	InputJSON   string `json:"input_json"`
	Want        string `json:"want"`
	SchemaValid bool   `json:"schema_valid"`
}

type claudeEnvPlant struct {
	ID          string `json:"id"`
	Claim       string `json:"claim"`
	Entry       string `json:"entry"`
	ValidJSON   string `json:"valid_json"`
	InvalidJSON string `json:"invalid_json"`
	Want        string `json:"want"`
}

type containingCase struct {
	ID          string `json:"id"`
	Claim       string `json:"claim"`
	Entry       string `json:"entry"`
	InputJSON   string `json:"input_json"`
	SchemaValid bool   `json:"schema_valid"`
}

func withLauncherConfigV3(t *testing.T, admit bool) {
	t.Helper()
	old := admitLauncherConfigV3
	admitLauncherConfigV3 = admit
	t.Cleanup(func() { admitLauncherConfigV3 = old })
}

func loadClaudeEnvCorpus(t *testing.T, name string, into any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(claudeEnvCorpus, name))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		t.Fatalf("%s: corpus setup failure", name)
	}
}

// claudeEnvCarrier embeds a declaration vector in an otherwise valid /3
// projection, the containing file the launcher actually reads.
func claudeEnvCarrier(declaration string) []byte {
	return []byte(`{"schema":"curator.launcher-config/3","config_schema":9,"config_path":"/example/config.json","extensions":{"claude_env":` + declaration + `}}`)
}

// claudeEnvVerdict runs one corpus input through the production receiver and
// returns "accept", a refusal code, or a "setup:" marker for an outcome that
// is neither. A strict-file refusal (the containing file itself is not strict
// JSON) is the file-level form of claude_env_invalid only when the carrier
// really fails strict decoding; any other generic refusal is a setup failure.
func claudeEnvVerdict(entry, input string) string {
	var err error
	switch entry {
	case "declaration":
		doc := claudeEnvCarrier(input)
		var selected Selected
		selected, err = ParseSelected(doc)
		if err == nil {
			if selected.ClaudeEnv == nil || selected.ConfigSchema != 9 {
				return "setup:no-selection"
			}
			return "accept"
		}
		if errors.Is(err, ErrSelected) && !errors.As(err, new(*ClaudeEnvError)) {
			if _, parseErr := fragment.ParseJSON(doc); parseErr != nil {
				return CodeClaudeEnvInvalid
			}
			return "setup:generic-refusal"
		}
	case "projection":
		if err = ValidateContribution([]byte(input)); err == nil {
			return "accept"
		}
	default:
		return "setup:entry"
	}
	var refusal *ClaudeEnvError
	if errors.As(err, &refusal) {
		return refusal.Code
	}
	return "setup:untyped"
}

func claudeEnvMismatches(cases []claudeEnvCase, verdict func(entry, input string) string) []string {
	var out []string
	for _, c := range cases {
		if got := verdict(c.Entry, c.InputJSON); got != c.Want {
			out = append(out, c.ID)
		}
	}
	return out
}

// The vendored corpus is the exact landed spec copy; an edited fixture fails.
func TestClaudeEnvCorpusProvenance(t *testing.T) {
	for name, want := range claudeEnvCorpusBlobs {
		data, err := os.ReadFile(filepath.Join(claudeEnvCorpus, name))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha1.Sum(append([]byte(fmt.Sprintf("blob %d\x00", len(data))), data...))
		if got := fmt.Sprintf("%x", sum); got != want {
			t.Fatalf("%s: blob %s, want %s", name, got, want)
		}
	}
}

// Every corpus case reaches its stated verdict through the real /3 receiver:
// declarations embedded in a projection through ParseSelected, contributions
// through ValidateContribution. Refusals carry only the code and fixed path,
// never a supplied value or name; accepted declarations keep the exact string.
// Limit: this proves parser verdicts, not child-launch effects.
func TestClaudeEnvCorpusCases(t *testing.T) {
	withLauncherConfigV3(t, true)
	var cases []claudeEnvCase
	loadClaudeEnvCorpus(t, "cases.json", &cases)
	if len(cases) == 0 {
		t.Fatal("empty corpus")
	}
	// Carrier setup controls: the carrier admits a valid member, and an
	// unrelated carrier defect is a generic refusal, never a member code.
	if claudeEnvVerdict("declaration", `{"BASH_MAX_OUTPUT_LENGTH":"8000"}`) != "accept" {
		t.Fatal("carrier rejects a valid member")
	}
	unrelated := bytes.Replace(claudeEnvCarrier(`{}`), []byte(`"extensions"`), []byte(`"extra":1,"extensions"`), 1)
	if _, err := ParseSelected(unrelated); !errors.Is(err, ErrSelected) || errors.As(err, new(*ClaudeEnvError)) {
		t.Fatal("unrelated carrier defect reported as a member refusal")
	}
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			if got := claudeEnvVerdict(c.Entry, c.InputJSON); got != c.Want {
				t.Fatalf("verdict %q, want %q", got, c.Want)
			}
			if c.Entry != "declaration" {
				return
			}
			selected, err := ParseSelected(claudeEnvCarrier(c.InputJSON))
			if c.Want == "accept" {
				var declared map[string]string
				if json.Unmarshal([]byte(c.InputJSON), &declared) != nil || selected.ClaudeEnv.BashMaxOutputLength != declared[ClaudeEnvVariable] {
					t.Fatal("accepted value changed")
				}
				return
			}
			if text := err.Error(); text != ErrSelected.Error() && text != c.Want+": "+ClaudeEnvPath {
				t.Fatal("refusal text is not the fixed form")
			}
		})
	}
}

// The checker itself detects a wrong expectation and a permissive receiver:
// one flipped expectation yields exactly that mismatch, and a receiver that
// accepts everything fails every refusal case.
func TestClaudeEnvCorpusCheckerControls(t *testing.T) {
	withLauncherConfigV3(t, true)
	var cases []claudeEnvCase
	loadClaudeEnvCorpus(t, "cases.json", &cases)
	if got := claudeEnvMismatches(cases, claudeEnvVerdict); len(got) != 0 {
		t.Fatalf("baseline mismatches %v", got)
	}
	planted := slices.Clone(cases)
	planted[0].Want = CodeClaudeEnvValueOverBound
	if got := claudeEnvMismatches(planted, claudeEnvVerdict); !slices.Equal(got, []string{cases[0].ID}) {
		t.Fatalf("wrong expectation detected as %v", got)
	}
	refusals := 0
	for _, c := range cases {
		if c.Want != "accept" {
			refusals++
		}
	}
	permissive := func(string, string) string { return "accept" }
	if got := claudeEnvMismatches(cases, permissive); len(got) != refusals || refusals == 0 {
		t.Fatalf("permissive receiver missed refusals: %d of %d", len(got), refusals)
	}
}

// Each single-defect plant leaves its valid control accepted and reaches its
// own named refusal, not some other refusal.
func TestClaudeEnvCorpusPlants(t *testing.T) {
	withLauncherConfigV3(t, true)
	var plants []claudeEnvPlant
	loadClaudeEnvCorpus(t, "plants.json", &plants)
	if len(plants) != 6 {
		t.Fatalf("want the six required plants, got %d", len(plants))
	}
	for _, p := range plants {
		t.Run(p.ID, func(t *testing.T) {
			if got := claudeEnvVerdict(p.Entry, p.ValidJSON); got != "accept" {
				t.Fatalf("valid control %q", got)
			}
			if got := claudeEnvVerdict(p.Entry, p.InvalidJSON); got != p.Want {
				t.Fatalf("plant %q, want %q", got, p.Want)
			}
		})
	}
}

// The complete /3 and /2 projection cases reach the structural verdicts: all
// predecessor selections retained under /3, strict unknown-member rejection,
// the clean absolute config_path, and /2 refusing claude_env.
func TestClaudeEnvContainingProjections(t *testing.T) {
	withLauncherConfigV3(t, true)
	var cases []containingCase
	loadClaudeEnvCorpus(t, "containing-cases.json", &cases)
	ran := 0
	for _, c := range cases {
		if c.Entry != "launcher" && c.Entry != "launcher-v2" {
			continue
		}
		ran++
		t.Run(c.ID, func(t *testing.T) {
			_, err := ParseSelected([]byte(c.InputJSON))
			if (err == nil) != c.SchemaValid {
				t.Fatalf("accepted=%v, want %v", err == nil, c.SchemaValid)
			}
			if err != nil && strings.Contains(err.Error(), "example") {
				t.Fatal("refusal reflects input")
			}
		})
	}
	if ran < 20 {
		t.Fatalf("only %d launcher cases ran", ran)
	}
}

// Production readers keep refusing /3 until the candidate gate opens; /2
// is unaffected by the gate. With the gate open the same bytes are accepted.
func TestLauncherConfigV3CandidateGate(t *testing.T) {
	v3 := []byte(`{"schema":"curator.launcher-config/3","config_schema":9,"config_path":"/example/config.json","extensions":{"claude_env":{"BASH_MAX_OUTPUT_LENGTH":"8000"}}}`)
	v2 := []byte(`{"schema":"curator.launcher-config/2","config_schema":8,"config_path":"/example/config.json","extensions":{"notification":{"backend":"off"}}}`)
	if admitLauncherConfigV3 {
		t.Fatal("production gate is open")
	}
	if _, err := ParseSelected(v3); err != ErrSelected {
		t.Fatalf("closed gate admitted /3: %v", err)
	}
	if _, err := ParseSelected(v2); err != nil {
		t.Fatal("closed gate broke /2")
	}
	withLauncherConfigV3(t, true)
	got, err := ParseSelected(v3)
	if err != nil || !got.ClaudeEnv.Selected() || got.ClaudeEnv.BashMaxOutputLength != "8000" {
		t.Fatalf("open gate refused /3: %v", err)
	}
	if _, err := ParseSelected(v2); err != nil {
		t.Fatal("open gate broke /2")
	}
}

// Absence and {} both select nothing; names are checked before the value.
func TestClaudeEnvSelectionShapes(t *testing.T) {
	withLauncherConfigV3(t, true)
	absent, err := ParseSelected([]byte(`{"schema":"curator.launcher-config/3","config_schema":9,"config_path":"/example/config.json","extensions":{}}`))
	if err != nil || absent.ClaudeEnv != nil || absent.ClaudeEnv.Selected() {
		t.Fatal("absent member selected something")
	}
	empty, err := ParseSelected(claudeEnvCarrier(`{}`))
	if err != nil || empty.ClaudeEnv == nil || empty.ClaudeEnv.Selected() {
		t.Fatal("empty member selected something")
	}
	if got := claudeEnvVerdict("declaration", `{"BASH_MAX_OUTPUT_LENGTH":"0","OTHER":"1"}`); got != CodeClaudeEnvNameNotAllowed {
		t.Fatalf("name precedence %q", got)
	}
	if got := claudeEnvVerdict("declaration", `{"BASH_MAX_OUTPUT_LENGTH":"65536"}`); got != "accept" {
		t.Fatal("bound itself refused")
	}
}

// The selected value replaces every inherited entry of exactly that name
// and nothing else; the caller's slice is untouched. An empty selection is
// an unchanged copy. Contribution agrees with ValidateContribution for every
// admitted runtime.
func TestApplyClaudeEnv(t *testing.T) {
	inherited := []string{"PATH=/bin", ClaudeEnvVariable + "=12", "BASH_MAX_OUTPUT_LENGTH_EXTRA=1", ClaudeEnvVariable + "=13", "HOME=/h"}
	before := slices.Clone(inherited)
	selected := &ClaudeEnv{BashMaxOutputLength: "8000"}
	got := ApplyClaudeEnv(inherited, selected)
	want := []string{"PATH=/bin", "BASH_MAX_OUTPUT_LENGTH_EXTRA=1", "HOME=/h", ClaudeEnvVariable + "=8000"}
	if !slices.Equal(got, want) || !slices.Equal(inherited, before) {
		t.Fatalf("applied %v", got)
	}
	for _, none := range []*ClaudeEnv{nil, {}} {
		if got := ApplyClaudeEnv(inherited, none); !slices.Equal(got, inherited) {
			t.Fatal("empty selection changed the environment")
		}
	}
	for _, runtime := range claudeEnvRuntimes {
		contribution := selected.Contribution(runtime)
		if (len(contribution) == 1) != (runtime == "claude_code") {
			t.Fatalf("%s contribution %v", runtime, contribution)
		}
		raw, _ := json.Marshal(map[string]any{"runtime": runtime, "env": contribution})
		if err := ValidateContribution(raw); err != nil {
			t.Fatalf("%s contribution invalid: %v", runtime, err)
		}
	}
}
