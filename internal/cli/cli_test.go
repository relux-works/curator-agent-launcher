package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/cases.golden from the current parser")

// shape is one command-line shape driven through Parse, the production
// call site (cmd/curator-run main -> run -> cli.Parse).
type shape struct {
	name string
	args []string
}

// accepted shapes; each is checked for a nil error and rendered to the golden.
var acceptedShapes = []shape{
	{"env only", []string{"codex_cli"}},
	{"env with empty native tail", []string{"codex_cli", "--"}},
	{"native tail verbatim", []string{"codex_cli", "--profile", "companyA", "--", "resume", "--last"}},
	{"native tail keeps empty operand", []string{"pi", "--", "", "hello", ""}},
	{"native tail keeps colliding launcher flags", []string{"claude_code", "--", "--profile", "x", "--model", "m", "--help", "-h", "--version"}},
	{"native tail keeps second double dash", []string{"claude_code", "--", "--", "a", "--"}},
	{"native tail keeps stray operands", []string{"codex_cli", "--", "one", "two", "three"}},
	{"native tail keeps unknown flags", []string{"codex_cli", "--", "--no-such-flag", "--dangerously-bypass"}},
	{"flags before env", []string{"--profile", "companyA", "--model", "gpt-5.3-codex", "codex_cli"}},
	{"flags after env", []string{"codex_cli", "--model", "gpt-5.3-codex", "--effort", "high"}},
	{"equals form", []string{"codex_cli", "--profile=companyA", "--effort=medium"}},
	{"equals form value containing equals", []string{"codex_cli", "--profile=a=b"}},
	{"permissions native separate", []string{"codex_cli", "--permissions", "native"}},
	{"permissions yolo separate", []string{"codex_cli", "--permissions", "yolo"}},
	{"permissions native equals", []string{"--permissions=native", "codex_cli"}},
	{"permissions yolo equals", []string{"--permissions=yolo", "codex_cli"}},
	{"yolo alias", []string{"--yolo", "codex_cli"}},
	{"native tail keeps permission spellings", []string{"pi", "--", "--permissions", "yolo", "--yolo", "-d", "--danger"}},
	{"system prompt append", []string{"pi", "--system-prompt", "append"}},
	{"system prompt replace", []string{"pi", "--system-prompt", "replace"}},
	{"network separated", []string{"codex_cli", "--network", "egress-a"}},
	{"network equals", []string{"codex_cli", "--network=egress-a"}},
	{"network before env", []string{"--network", "egress-a", "codex_cli"}},
	{"native tail keeps network flag", []string{"codex_cli", "--", "--network", "egress-a"}},
	{"help alone", []string{"--help"}},
	{"short help alone", []string{"-h"}},
	{"version alone", []string{"--version"}},
	{"help after valid flags wins", []string{"codex_cli", "--profile", "p", "--help"}},
	{"version after env wins", []string{"codex_cli", "--version", "--unknown-never-read"}},
	{"help before version: first wins", []string{"--help", "--version"}},
	{"version before help: first wins", []string{"--version", "--help"}},
	{"help without env-id", []string{"--model", "m", "-h"}},
	{"env-id lookalike is not validated here", []string{"not_registered"}},
	{"alias claude normalizes to claude_code", []string{"claude"}},
	{"alias codex normalizes to codex_cli", []string{"codex"}},
	{"alias claude with flags and native tail", []string{"claude", "--profile", "p", "--", "resume", "--last"}},
	{"alias codex with flags before env", []string{"--model", "m", "codex"}},
	{"native tail alias spellings untouched", []string{"pi", "--", "claude", "codex"}},
	{"hosted flag", []string{"claude_code", "--hosted"}},
	{"native flag", []string{"claude_code", "--native"}},
	{"untracked flag", []string{"claude_code", "--untracked"}},
	{"host flag before env", []string{"--hosted", "claude_code", "--", "x"}},
	{"resume latest selector", []string{"claude_code", "resume"}},
	{"resume handle selector", []string{"claude_code", "resume", "SES-abc123"}},
	{"resume selector with flags interleaved", []string{"claude_code", "--hosted", "resume", "--model", "m", "SES-1", "--", "x"}},
	{"resume flag", []string{"claude_code", "--hosted", "--resume", "01234567-89ab-cdef-0123-456789abcdef"}},
	{"resume flag equals form", []string{"claude_code", "--resume=abc"}},
	{"network flag", []string{"claude_code", "--network", "office"}},
	{"network flag equals form", []string{"claude_code", "--network=office"}},
	{"native tail keeps host and resume spellings", []string{"pi", "--", "--hosted", "--native", "--untracked", "resume", "--resume", "x", "--network", "y"}},
}

// rejected shapes; each must yield a *UsageError. want is a substring of
// the detail, so a mutant that admits the shape or misnames the fault fails.
var rejectedShapes = []struct {
	shape
	want string
}{
	{shape{"no arguments", []string{}}, "missing <env-id>"},
	{shape{"only double dash", []string{"--"}}, "missing <env-id>"},
	{shape{"flags only no env", []string{"--profile", "p"}}, "missing <env-id>"},
	{shape{"native tail no env", []string{"--", "codex_cli"}}, "missing <env-id>"},
	{shape{"empty env operand", []string{""}}, "empty operand"},
	{shape{"stray operand", []string{"codex_cli", "frobnicate"}}, `stray operand "frobnicate"`},
	{shape{"stray operand then double dash", []string{"codex_cli", "frobnicate", "--", "x"}}, `stray operand "frobnicate"`},
	{shape{"second operand after resume handle", []string{"codex_cli", "resume", "SES-1", "extra"}}, `stray operand "extra"`},
	{shape{"second resume token is a handle", []string{"codex_cli", "resume", "resume", "extra"}}, `stray operand "extra"`},
	{shape{"unknown flag", []string{"codex_cli", "--nope"}}, `unknown flag "--nope"`},
	{shape{"unknown flag equals form", []string{"codex_cli", "--nope=1"}}, `unknown flag "--nope=1"`},
	{shape{"unknown single dash flag", []string{"codex_cli", "-p", "x"}}, `unknown flag "-p"`},
	{shape{"lone dash", []string{"codex_cli", "-"}}, `unknown flag "-"`},
	{shape{"unknown flag before help", []string{"--nope", "--help"}}, `unknown flag "--nope"`},
	{shape{"triple dash", []string{"codex_cli", "---"}}, `unknown flag "---"`},
	{shape{"repeated profile", []string{"codex_cli", "--profile", "a", "--profile", "b"}}, "--profile given more than once"},
	{shape{"repeated model mixed forms", []string{"codex_cli", "--model=a", "--model", "b"}}, "--model given more than once"},
	{shape{"repeated effort", []string{"codex_cli", "--effort", "a", "--effort", "a"}}, "--effort given more than once"},
	{shape{"repeated permissions", []string{"codex_cli", "--permissions=native", "--permissions", "yolo"}}, "cannot be repeated or combined"},
	{shape{"permissions and yolo alias", []string{"codex_cli", "--permissions", "native", "--yolo"}}, "cannot be repeated or combined"},
	{shape{"yolo alias and permissions", []string{"codex_cli", "--yolo", "--permissions=yolo"}}, "cannot be repeated or combined"},
	{shape{"yolo alias equals value", []string{"codex_cli", "--yolo=true"}}, "alias without a value"},
	{shape{"unknown permissions mode", []string{"codex_cli", "--permissions=automatic"}}, "accepts native or yolo"},
	{shape{"unknown permissions mode separate", []string{"codex_cli", "--permissions", "auto"}}, "accepts native or yolo"},
	{shape{"legacy short danger", []string{"codex_cli", "-d"}}, "is rejected"},
	{shape{"legacy short danger equals", []string{"codex_cli", "-d=true"}}, "is rejected"},
	{shape{"legacy danger", []string{"codex_cli", "--danger"}}, "is rejected"},
	{shape{"legacy danger equals", []string{"codex_cli", "--danger=true"}}, "is rejected"},
	{shape{"repeated system prompt", []string{"pi", "--system-prompt", "append", "--system-prompt", "replace"}}, "--system-prompt given more than once"},
	{shape{"repeated name", []string{"codex_cli", "--name", "a", "--name", "b"}}, "was removed in 0.6.0"},
	{shape{"repeated ax profile", []string{"codex_cli", "--ax-profile", "yolo", "--ax-profile", "standard"}}, "was removed in 0.6.0"},
	{shape{"repeated network", []string{"codex_cli", "--network", "a", "--network", "b"}}, "--network given more than once"},
	{shape{"repeated network mixed forms", []string{"codex_cli", "--network=a", "--network", "b"}}, "--network given more than once"},
	{shape{"network missing value at end", []string{"codex_cli", "--network"}}, "--network requires a value"},
	{shape{"network missing value before double dash", []string{"codex_cli", "--network", "--", "x"}}, "--network requires a value"},
	{shape{"network missing value equals empty", []string{"codex_cli", "--network="}}, "--network requires a value"},
	{shape{"network value spelled as flag", []string{"codex_cli", "--network", "--model", "m"}}, "--network requires a value"},
	{shape{"missing value at end", []string{"codex_cli", "--profile"}}, "--profile requires a value"},
	{shape{"missing value before double dash", []string{"codex_cli", "--model", "--", "x"}}, "--model requires a value"},
	{shape{"missing value equals empty", []string{"codex_cli", "--profile="}}, "--profile requires a value"},
	{shape{"missing value empty string", []string{"codex_cli", "--effort", ""}}, "--effort requires a value"},
	{shape{"value spelled as flag", []string{"codex_cli", "--profile", "--model", "m"}}, "--profile requires a value"},
	{shape{"value is help flag", []string{"codex_cli", "--name", "--help"}}, "was removed in 0.6.0"},
	{shape{"system prompt bad value", []string{"pi", "--system-prompt", "prepend"}}, "--system-prompt accepts append or replace"},
	{shape{"system prompt case sensitive", []string{"pi", "--system-prompt", "Append"}}, "--system-prompt accepts append or replace"},
	{shape{"system prompt without value", []string{"pi", "--system-prompt"}}, "--system-prompt requires a value"},
	{shape{"ax profile bad value tracked", []string{"codex_cli", "--ax-profile", "unsafe"}}, "was removed in 0.6.0"},
	{shape{"ax profile bad value untracked", []string{"codex_cli", "--ax-profile", "unsafe"}}, "was removed in 0.6.0"},
	{shape{"ax profile case sensitive", []string{"codex_cli", "--ax-profile", "YOLO"}}, "was removed in 0.6.0"},
	{shape{"ax profile on untracked machine", []string{"codex_cli", "--ax-profile", "standard"}}, "was removed in 0.6.0"},
	{shape{"ax profile yolo on untracked machine", []string{"codex_cli", "--ax-profile", "yolo"}}, "was removed in 0.6.0"},
	{shape{"name 65 chars", []string{"codex_cli", "--name", "a" + strings.Repeat("b", 64)}}, "was removed in 0.6.0"},
	{shape{"name leading dot", []string{"codex_cli", "--name", ".hidden"}}, "was removed in 0.6.0"},
	{shape{"name leading underscore", []string{"codex_cli", "--name", "_x"}}, "was removed in 0.6.0"},
	{shape{"name with slash", []string{"codex_cli", "--name", "a/b"}}, "was removed in 0.6.0"},
	{shape{"name with space", []string{"codex_cli", "--name", "a b"}}, "was removed in 0.6.0"},
	{shape{"name with unicode", []string{"codex_cli", "--name", "сессия"}}, "was removed in 0.6.0"},
	{shape{"name with newline", []string{"codex_cli", "--name", "a\nb"}}, "was removed in 0.6.0"},
	{shape{"name invalid on untracked machine still rejected", []string{"codex_cli", "--name", "bad/name"}}, "was removed in 0.6.0"},
	{shape{"repeated hosted flag", []string{"claude_code", "--hosted", "--hosted"}}, "one host request and cannot be repeated or combined"},
	{shape{"repeated native flag", []string{"claude_code", "--native", "--native"}}, "one host request and cannot be repeated or combined"},
	{shape{"hosted native combination", []string{"claude_code", "--hosted", "--native"}}, "one host request and cannot be repeated or combined"},
	{shape{"native untracked combination", []string{"claude_code", "--native", "--untracked"}}, "one host request and cannot be repeated or combined"},
	{shape{"untracked hosted combination", []string{"claude_code", "--untracked", "--hosted"}}, "one host request and cannot be repeated or combined"},
	{shape{"hosted flag with value", []string{"claude_code", "--hosted=yes"}}, "--hosted takes no value"},
	{shape{"native flag with value", []string{"claude_code", "--native=no"}}, "--native takes no value"},
	{shape{"repeated resume flag", []string{"claude_code", "--resume", "a", "--resume", "b"}}, "--resume given more than once"},
	{shape{"resume flag missing value", []string{"claude_code", "--resume"}}, "--resume requires a value"},
	{shape{"resume flag empty value", []string{"claude_code", "--resume", ""}}, "--resume requires a value"},
	{shape{"repeated network flag", []string{"claude_code", "--network", "a", "--network", "b"}}, "--network given more than once"},
	{shape{"network flag missing value", []string{"claude_code", "--network"}}, "--network requires a value"},
	{shape{"ax profile with native flag", []string{"codex_cli", "--ax-profile", "standard", "--native"}}, "was removed in 0.6.0"},
	{shape{"ax profile with untracked flag", []string{"codex_cli", "--untracked", "--ax-profile", "yolo"}}, "was removed in 0.6.0"},
	{shape{"ax profile with native flag untracked", []string{"codex_cli", "--native", "--ax-profile", "standard"}}, "was removed in 0.6.0"},
}

// Each accepted command shape parses successfully with its exact native tail.
func TestParseAccepted(t *testing.T) {
	for _, s := range acceptedShapes {
		t.Run(s.name, func(t *testing.T) {
			inv, err := Parse(s.args)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v, want nil", s.args, err)
			}
			if inv.Info == InfoNone && inv.Native == nil {
				t.Fatalf("Parse(%q).Native is nil on a launch invocation", s.args)
			}
		})
	}
}

// Accepted permission spellings resolve to one explicit mode member.
func TestPermissionModeFormsResolveToOneMember(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want PermissionMode
	}{
		{"separate native", []string{"pi", "--permissions", "native"}, PermissionNative},
		{"equals native", []string{"--permissions=native", "pi"}, PermissionNative},
		{"separate yolo", []string{"pi", "--permissions", "yolo"}, PermissionYolo},
		{"equals yolo", []string{"--permissions=yolo", "pi"}, PermissionYolo},
		{"alias", []string{"--yolo", "pi"}, PermissionYolo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv, err := Parse(tc.args)
			if err != nil || !inv.PermissionSet || inv.PermissionMode != tc.want {
				t.Fatalf("Parse(%q) = %+v, %v; want permission %q", tc.args, inv, err, tc.want)
			}
		})
	}
}

// Invalid command shapes produce typed usage errors naming the intended fault.
func TestParseRejected(t *testing.T) {
	for _, s := range rejectedShapes {
		t.Run(s.name, func(t *testing.T) {
			inv, err := Parse(s.args)
			if err == nil {
				t.Fatalf("Parse(%q) accepted %+v, want usage error containing %q", s.args, inv, s.want)
			}
			if !IsUsage(err) {
				t.Fatalf("Parse(%q) error %T, want *UsageError", s.args, err)
			}
			if !strings.Contains(err.Error(), s.want) {
				t.Fatalf("Parse(%q) error %q does not contain %q", s.args, err.Error(), s.want)
			}
			if err.(*UsageError).Code() != "usage" {
				t.Fatalf("Parse(%q) code = %q, want usage", s.args, err.(*UsageError).Code())
			}
		})
	}
}

// TestNormalizeEnvID pins the closed alias table: the two aliases map to
// their canonical ids, and every other spelling (including near-misses,
// case variants, and whitespace) passes through for the later stages to
// refuse as today.
func TestNormalizeEnvID(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"claude", "claude_code"},
		{"codex", "codex_cli"},
		{"claude_code", "claude_code"},
		{"codex_cli", "codex_cli"},
		{"pi", "pi"},
		{"opencode", "opencode"},
		{"not_registered", "not_registered"},
		{"Claude", "Claude"},
		{"CODEX", "CODEX"},
		{"claude_code ", "claude_code "},
		{" claude", " claude"},
		{"claudes", "claudes"},
		{"codexx", "codexx"},
		{"", ""},
	} {
		if got := NormalizeEnvID(tc.in); got != tc.want {
			t.Errorf("NormalizeEnvID(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if got := NormalizeEnvID(NormalizeEnvID(tc.in)); got != tc.want {
			t.Errorf("NormalizeEnvID is not idempotent for %q: %q", tc.in, got)
		}
	}
}

// TestParseNormalizesAliases drives both spellings through Parse, the
// production call site, and requires the canonical id in the result. The
// native tail is never normalized.
func TestParseNormalizesAliases(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		wantEnv  string
		wantTail []string
	}{
		{[]string{"claude"}, "claude_code", []string{}},
		{[]string{"codex"}, "codex_cli", []string{}},
		{[]string{"claude", "--profile", "p", "--", "resume", "--last"}, "claude_code", []string{"resume", "--last"}},
		{[]string{"--model", "m", "codex"}, "codex_cli", []string{}},
		{[]string{"pi", "--", "claude", "codex"}, "pi", []string{"claude", "codex"}},
		{[]string{"claude", "--", "claude", "codex"}, "claude_code", []string{"claude", "codex"}},
	} {
		inv, err := Parse(tc.args)
		if err != nil {
			t.Errorf("Parse(%q) error = %v", tc.args, err)
			continue
		}
		if inv.EnvID != tc.wantEnv {
			t.Errorf("Parse(%q).EnvID = %q, want %q", tc.args, inv.EnvID, tc.wantEnv)
		}
		if fmt.Sprintf("%q", inv.Native) != fmt.Sprintf("%q", tc.wantTail) {
			t.Errorf("Parse(%q).Native = %q, want %q", tc.args, inv.Native, tc.wantTail)
		}
	}
	// Near-miss spellings are not aliases: they pass through for the later
	// stages to refuse, and never normalize to a canonical id.
	for _, alias := range []string{"Claude", "CODEX", "claudes", "codexx"} {
		inv, err := Parse([]string{alias})
		if err != nil {
			t.Errorf("Parse(%q) error = %v, want nil (refusal is later)", alias, err)
			continue
		}
		if inv.EnvID != alias {
			t.Errorf("Parse(%q).EnvID = %q, want verbatim %q", alias, inv.EnvID, alias)
		}
	}
}

// TestNativeTailVerbatim proves the post-"--" contract element by element:
// the tail is a copy, not an alias, and nothing after "--" is interpreted.
func TestNativeTailVerbatim(t *testing.T) {
	tail := []string{"", "--", "--help", "-h", "--version", "--profile", "", "--ax-profile", "yolo", "--network", "egress-a", "resume", "--last"}
	args := append([]string{"codex_cli", "--profile", "p", "--"}, tail...)
	inv, err := Parse(args)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if len(inv.Native) != len(tail) {
		t.Fatalf("Native = %q, want %q", inv.Native, tail)
	}
	for i := range tail {
		if inv.Native[i] != tail[i] {
			t.Fatalf("Native[%d] = %q, want %q", i, inv.Native[i], tail[i])
		}
	}
	if inv.Info != InfoNone || inv.NetworkSet {
		t.Fatalf("post-- tokens were interpreted: %+v", inv)
	}
	args[len(args)-1] = "mutated"
	if inv.Native[len(inv.Native)-1] != "--last" {
		t.Fatal("Native aliases the caller's argv")
	}
}

// TestInformationalCarriesNothingElse: an informational result is only the
// Info member, so no caller can mistake it for a launch.
func TestInformationalCarriesNothingElse(t *testing.T) {
	for _, args := range [][]string{{"codex_cli", "--profile", "p", "--help"}, {"--version"}, {"-h", "--", "x"}} {
		inv, err := Parse(args)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", args, err)
		}
		want := Invocation{Info: inv.Info}
		if inv.Info == InfoNone {
			t.Fatalf("Parse(%q).Info = none", args)
		}
		if fmt.Sprintf("%+v", inv) != fmt.Sprintf("%+v", want) {
			t.Fatalf("Parse(%q) = %+v, want %+v", args, inv, want)
		}
	}
}

// TestExplicitNative pins the --untracked synonym: both native spellings
// select the explicit native path, while --hosted and no flag do not.
func TestExplicitNative(t *testing.T) {
	for flag, want := range map[string]bool{
		"--native": true, "--untracked": true, "--hosted": false, "": false, "--hostedx": false,
	} {
		if got := ExplicitNative(flag); got != want {
			t.Errorf("ExplicitNative(%q) = %v, want %v", flag, got, want)
		}
	}
}

func TestUsageErrorShape(t *testing.T) {
	_, err := Parse(nil)
	var u *UsageError
	if !IsUsage(err) {
		t.Fatalf("err %T is not usage", err)
	}
	u = err.(*UsageError)
	if u.Code() != "usage" || !strings.HasPrefix(err.Error(), "usage: ") || ExitCode != 2 {
		t.Fatalf("usage error shape wrong: code=%q msg=%q exit=%d", u.Code(), err.Error(), ExitCode)
	}
}

// TestGolden renders every shape through Parse and compares against
// testdata/cases.golden, so an accepted or rejected shape cannot change
// silently. Regenerate with: go test ./internal/cli -run TestGolden -update
func TestGolden(t *testing.T) {
	var b strings.Builder
	render := func(s shape) {
		fmt.Fprintf(&b, "# %s\nargs: %q\n", s.name, s.args)
		inv, err := Parse(s.args)
		if err != nil {
			fmt.Fprintf(&b, "error: %s\n\n", err.Error())
			return
		}
		fmt.Fprintf(&b, "result: %s\n\n", renderInvocation(inv))
	}
	for _, s := range acceptedShapes {
		render(s)
	}
	for _, s := range rejectedShapes {
		render(s.shape)
	}
	got := b.String()
	path := filepath.Join("testdata", "cases.golden")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run with -update to create)", err)
	}
	if string(want) != got {
		t.Fatalf("golden mismatch; run `go test ./internal/cli -run TestGolden -update` and review the diff\n--- got ---\n%s", got)
	}
}

func renderInvocation(inv Invocation) string {
	switch inv.Info {
	case InfoHelp:
		return "info=help"
	case InfoVersion:
		return "info=version"
	}
	parts := []string{fmt.Sprintf("env=%q", inv.EnvID)}
	if inv.HostFlag != "" {
		parts = append(parts, fmt.Sprintf("host=%s", inv.HostFlag))
	}
	if inv.ResumeRequested {
		parts = append(parts, "resume")
	}
	if inv.ResumeHandleSet {
		parts = append(parts, fmt.Sprintf("handle=%q", inv.ResumeHandle))
	}
	if inv.ResumeIDSet {
		parts = append(parts, fmt.Sprintf("resume-id=%q", inv.ResumeID))
	}
	if inv.ProfileSet {
		parts = append(parts, fmt.Sprintf("profile=%q", inv.Profile))
	}
	if inv.SystemPrompt != SystemPromptNone {
		parts = append(parts, fmt.Sprintf("system-prompt=%s", inv.SystemPrompt))
	}
	if inv.ModelSet {
		parts = append(parts, fmt.Sprintf("model=%q", inv.Model))
	}
	if inv.EffortSet {
		parts = append(parts, fmt.Sprintf("effort=%q", inv.Effort))
	}
	if inv.PermissionSet {
		parts = append(parts, fmt.Sprintf("permissions=%q", inv.PermissionMode))
	}
	if inv.NetworkSet {
		parts = append(parts, fmt.Sprintf("network=%q", inv.Network))
	}
	parts = append(parts, fmt.Sprintf("native=%q", inv.Native))
	return strings.Join(parts, " ")
}
