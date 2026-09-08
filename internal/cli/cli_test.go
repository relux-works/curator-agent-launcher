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
	ax   bool
}

var untracked = Options{AxConfigured: false}
var tracked = Options{AxConfigured: true}

// accepted shapes; each is checked for a nil error and rendered to the golden.
var acceptedShapes = []shape{
	{"env only", []string{"codex_cli"}, false},
	{"env with empty native tail", []string{"codex_cli", "--"}, false},
	{"native tail verbatim", []string{"codex_cli", "--profile", "companyA", "--", "resume", "--last"}, false},
	{"native tail keeps empty operand", []string{"pi", "--", "", "hello", ""}, false},
	{"native tail keeps colliding launcher flags", []string{"claude_code", "--", "--profile", "x", "--model", "m", "--help", "-h", "--version"}, false},
	{"native tail keeps second double dash", []string{"claude_code", "--", "--", "a", "--"}, false},
	{"native tail keeps stray operands", []string{"codex_cli", "--", "one", "two", "three"}, false},
	{"native tail keeps unknown flags", []string{"codex_cli", "--", "--no-such-flag", "--dangerously-bypass"}, false},
	{"flags before env", []string{"--profile", "companyA", "--model", "gpt-5.3-codex", "codex_cli"}, false},
	{"flags after env", []string{"codex_cli", "--model", "gpt-5.3-codex", "--effort", "high"}, false},
	{"equals form", []string{"codex_cli", "--profile=companyA", "--effort=medium"}, false},
	{"equals form value containing equals", []string{"codex_cli", "--profile=a=b"}, false},
	{"system prompt append", []string{"pi", "--system-prompt", "append"}, false},
	{"system prompt replace", []string{"pi", "--system-prompt", "replace"}, false},
	{"name on tracked machine", []string{"codex_cli", "--name", "work.1_a-b"}, true},
	{"name 64 chars on tracked machine", []string{"codex_cli", "--name", "a" + strings.Repeat("b", 63)}, true},
	{"name single char", []string{"codex_cli", "--name", "7"}, true},
	{"name on untracked machine accepted no effect", []string{"codex_cli", "--name", "ineffective"}, false},
	{"ax profile standard tracked", []string{"codex_cli", "--ax-profile", "standard"}, true},
	{"ax profile yolo tracked", []string{"codex_cli", "--ax-profile", "yolo"}, true},
	{"every flag tracked", []string{"claude_code", "--profile", "p", "--system-prompt", "append", "--model", "claude-opus-5", "--effort", "high", "--name", "s1", "--ax-profile", "standard", "--", "-p", "x"}, true},
	{"help alone", []string{"--help"}, false},
	{"short help alone", []string{"-h"}, false},
	{"version alone", []string{"--version"}, false},
	{"help after valid flags wins", []string{"codex_cli", "--profile", "p", "--help"}, false},
	{"version after env wins", []string{"codex_cli", "--version", "--unknown-never-read"}, false},
	{"help before version: first wins", []string{"--help", "--version"}, false},
	{"version before help: first wins", []string{"--version", "--help"}, false},
	{"help without env-id", []string{"--model", "m", "-h"}, false},
	{"env-id lookalike is not validated here", []string{"not_registered"}, false},
}

// rejected shapes; each must yield a *UsageError. want is a substring of
// the detail, so a mutant that admits the shape or misnames the fault fails.
var rejectedShapes = []struct {
	shape
	want string
}{
	{shape{"no arguments", []string{}, false}, "missing <env-id>"},
	{shape{"only double dash", []string{"--"}, false}, "missing <env-id>"},
	{shape{"flags only no env", []string{"--profile", "p"}, false}, "missing <env-id>"},
	{shape{"native tail no env", []string{"--", "codex_cli"}, false}, "missing <env-id>"},
	{shape{"empty env operand", []string{""}, false}, "empty operand"},
	{shape{"stray operand", []string{"codex_cli", "resume"}, false}, `stray operand "resume"`},
	{shape{"stray operand then double dash", []string{"codex_cli", "resume", "--", "x"}, false}, `stray operand "resume"`},
	{shape{"unknown flag", []string{"codex_cli", "--nope"}, false}, `unknown flag "--nope"`},
	{shape{"unknown flag equals form", []string{"codex_cli", "--nope=1"}, false}, `unknown flag "--nope=1"`},
	{shape{"unknown single dash flag", []string{"codex_cli", "-p", "x"}, false}, `unknown flag "-p"`},
	{shape{"lone dash", []string{"codex_cli", "-"}, false}, `unknown flag "-"`},
	{shape{"unknown flag before help", []string{"--nope", "--help"}, false}, `unknown flag "--nope"`},
	{shape{"triple dash", []string{"codex_cli", "---"}, false}, `unknown flag "---"`},
	{shape{"repeated profile", []string{"codex_cli", "--profile", "a", "--profile", "b"}, false}, "--profile given more than once"},
	{shape{"repeated model mixed forms", []string{"codex_cli", "--model=a", "--model", "b"}, false}, "--model given more than once"},
	{shape{"repeated effort", []string{"codex_cli", "--effort", "a", "--effort", "a"}, false}, "--effort given more than once"},
	{shape{"repeated system prompt", []string{"pi", "--system-prompt", "append", "--system-prompt", "replace"}, false}, "--system-prompt given more than once"},
	{shape{"repeated name", []string{"codex_cli", "--name", "a", "--name", "b"}, true}, "--name given more than once"},
	{shape{"repeated ax profile", []string{"codex_cli", "--ax-profile", "yolo", "--ax-profile", "standard"}, true}, "--ax-profile given more than once"},
	{shape{"missing value at end", []string{"codex_cli", "--profile"}, false}, "--profile requires a value"},
	{shape{"missing value before double dash", []string{"codex_cli", "--model", "--", "x"}, false}, "--model requires a value"},
	{shape{"missing value equals empty", []string{"codex_cli", "--profile="}, false}, "--profile requires a value"},
	{shape{"missing value empty string", []string{"codex_cli", "--effort", ""}, false}, "--effort requires a value"},
	{shape{"value spelled as flag", []string{"codex_cli", "--profile", "--model", "m"}, false}, "--profile requires a value"},
	{shape{"value is help flag", []string{"codex_cli", "--name", "--help"}, false}, "--name requires a value"},
	{shape{"system prompt bad value", []string{"pi", "--system-prompt", "prepend"}, false}, "--system-prompt accepts append or replace"},
	{shape{"system prompt case sensitive", []string{"pi", "--system-prompt", "Append"}, false}, "--system-prompt accepts append or replace"},
	{shape{"system prompt without value", []string{"pi", "--system-prompt"}, false}, "--system-prompt requires a value"},
	{shape{"ax profile bad value tracked", []string{"codex_cli", "--ax-profile", "unsafe"}, true}, "--ax-profile accepts standard or yolo"},
	{shape{"ax profile bad value untracked", []string{"codex_cli", "--ax-profile", "unsafe"}, false}, "--ax-profile accepts standard or yolo"},
	{shape{"ax profile case sensitive", []string{"codex_cli", "--ax-profile", "YOLO"}, true}, "--ax-profile accepts standard or yolo"},
	{shape{"ax profile on untracked machine", []string{"codex_cli", "--ax-profile", "standard"}, false}, "ax integration is not configured"},
	{shape{"ax profile yolo on untracked machine", []string{"codex_cli", "--ax-profile", "yolo"}, false}, "ax integration is not configured"},
	{shape{"name 65 chars", []string{"codex_cli", "--name", "a" + strings.Repeat("b", 64)}, true}, "not a valid ax session name"},
	{shape{"name leading dot", []string{"codex_cli", "--name", ".hidden"}, true}, "not a valid ax session name"},
	{shape{"name leading underscore", []string{"codex_cli", "--name", "_x"}, true}, "not a valid ax session name"},
	{shape{"name with slash", []string{"codex_cli", "--name", "a/b"}, true}, "not a valid ax session name"},
	{shape{"name with space", []string{"codex_cli", "--name", "a b"}, true}, "not a valid ax session name"},
	{shape{"name with unicode", []string{"codex_cli", "--name", "сессия"}, true}, "not a valid ax session name"},
	{shape{"name with newline", []string{"codex_cli", "--name", "a\nb"}, true}, "not a valid ax session name"},
	{shape{"name invalid on untracked machine still rejected", []string{"codex_cli", "--name", "bad/name"}, false}, "not a valid ax session name"},
}

func TestParseAccepted(t *testing.T) {
	for _, s := range acceptedShapes {
		t.Run(s.name, func(t *testing.T) {
			inv, err := Parse(s.args, Options{AxConfigured: s.ax})
			if err != nil {
				t.Fatalf("Parse(%q) error = %v, want nil", s.args, err)
			}
			if inv.Info == InfoNone && inv.Native == nil {
				t.Fatalf("Parse(%q).Native is nil on a launch invocation", s.args)
			}
			if inv.Tracked != s.ax && inv.Info == InfoNone {
				t.Fatalf("Parse(%q).Tracked = %v, want %v", s.args, inv.Tracked, s.ax)
			}
		})
	}
}

func TestParseRejected(t *testing.T) {
	for _, s := range rejectedShapes {
		t.Run(s.name, func(t *testing.T) {
			inv, err := Parse(s.args, Options{AxConfigured: s.ax})
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

// TestNativeTailVerbatim proves the post-"--" contract element by element:
// the tail is a copy, not an alias, and nothing after "--" is interpreted.
func TestNativeTailVerbatim(t *testing.T) {
	tail := []string{"", "--", "--help", "-h", "--version", "--profile", "", "--ax-profile", "yolo", "resume", "--last"}
	args := append([]string{"codex_cli", "--profile", "p", "--"}, tail...)
	inv, err := Parse(args, untracked)
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
	if inv.Info != InfoNone || inv.AxProfile != AxProfileNone {
		t.Fatalf("post-- tokens were interpreted: %+v", inv)
	}
	args[len(args)-1] = "mutated"
	if inv.Native[len(inv.Native)-1] != "--last" {
		t.Fatal("Native aliases the caller's argv")
	}
}

// TestNameIneffectiveUntracked: --name is accepted on an untracked machine,
// validated, stored, and marked ineffective through Tracked=false (SPEC §3).
func TestNameIneffectiveUntracked(t *testing.T) {
	inv, err := Parse([]string{"codex_cli", "--name", "s1"}, untracked)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if !inv.NameSet || inv.Name != "s1" || inv.Tracked {
		t.Fatalf("got %+v, want NameSet, Name=s1, Tracked=false", inv)
	}
	inv, err = Parse([]string{"codex_cli", "--name", "s1"}, tracked)
	if err != nil || !inv.Tracked {
		t.Fatalf("tracked parse: %+v, %v", inv, err)
	}
}

// TestInformationalCarriesNothingElse: an informational result is only the
// Info member, so no caller can mistake it for a launch.
func TestInformationalCarriesNothingElse(t *testing.T) {
	for _, args := range [][]string{{"codex_cli", "--profile", "p", "--help"}, {"--version"}, {"-h", "--", "x"}} {
		inv, err := Parse(args, tracked)
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

func TestUsageErrorShape(t *testing.T) {
	_, err := Parse(nil, untracked)
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
		fmt.Fprintf(&b, "# %s\nargs: %q\nax: %v\n", s.name, s.args, s.ax)
		inv, err := Parse(s.args, Options{AxConfigured: s.ax})
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
	parts := []string{fmt.Sprintf("env=%q", inv.EnvID), fmt.Sprintf("tracked=%v", inv.Tracked)}
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
	if inv.NameSet {
		parts = append(parts, fmt.Sprintf("name=%q", inv.Name))
	}
	if inv.AxProfile != AxProfileNone {
		parts = append(parts, fmt.Sprintf("ax-profile=%s", inv.AxProfile))
	}
	parts = append(parts, fmt.Sprintf("native=%q", inv.Native))
	return strings.Join(parts, " ")
}
