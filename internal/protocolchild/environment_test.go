package protocolchild

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func baselineFixture() []string {
	return []string{"HOME=/fixture/operator", "PATH=/fixture/bin:/usr/bin:/bin"}
}

// The exact admitted baseline is preserved in an independent snapshot. No
// ambient environment, home, profile, credential or process is consulted here.
func TestEntryEnvironmentAllowlist(t *testing.T) {
	all := append(baselineFixture(), "TMPDIR=/fixture/tmp", "LANG=en_US.UTF-8", "LC_ALL=", "LC_CTYPE=UTF-8", "TZ=UTC", "USER=fixture", "LOGNAME=fixture", "SHELL=/bin/zsh", "CURATOR_HOME=/fixture/curator")
	for _, env := range [][]string{baselineFixture(), all} {
		got, err := EntryEnvironment(env)
		if err != nil || !reflect.DeepEqual(got, env) {
			t.Fatalf("valid baseline: %v", err)
		}
		original := got[0]
		env[0] = "HOME=/changed"
		if got[0] != original {
			t.Fatal("snapshot aliases caller input")
		}
	}
}

// Every extra name refuses with the intended entry code and no partial result,
// including synthetic secrets/foreign markers, alone or together. The nearby
// valid baseline prevents reject-all success. This does not exercise child IO.
func TestEntryEnvironmentRejectsEveryUnknownName(t *testing.T) {
	for _, name := range []string{"A2A_AGENT", "A2A_CONFIG_DIR", "A2A_UNKNOWN", "CODEX_HOME", "CODEX_THREAD_ID", "CLAUDE_CONFIG_DIR", "CLAUDECODE", "GH_TOKEN", "OPENAI_API_KEY", "HTTP_PROXY", "DYLD_INSERT_LIBRARIES", "UNRECOGNIZED"} {
		t.Run(name, func(t *testing.T) {
			assertEnvironmentRefusal(t, append(baselineFixture(), name+"=synthetic-forbidden"))
		})
	}
	assertEnvironmentRefusal(t, append(baselineFixture(), "A2A_AGENT=synthetic-other", "CODEX_HOME=/fixture/foreign", "CODEX_THREAD_ID=synthetic-session", "GH_TOKEN=synthetic-forbidden"))
	if _, err := EntryEnvironment(baselineFixture()); err != nil {
		t.Fatal(err)
	}
}

// Malformed, duplicate and incomplete baseline entries cannot be scrubbed into
// a partial success; diagnostics never include an injected synthetic value.
func TestEntryEnvironmentInvalidShape(t *testing.T) {
	for name, env := range map[string][]string{
		"missing home": {"PATH=/fixture/bin"}, "missing path": {"HOME=/fixture/operator"},
		"empty home": {"HOME=", "PATH=/fixture/bin"}, "empty path": {"HOME=/fixture/operator", "PATH="},
		"no equals": append(baselineFixture(), "LANG"), "empty name": append(baselineFixture(), "=synthetic-forbidden"),
		"duplicate home":     append(baselineFixture(), "HOME=/fixture/other"),
		"duplicate optional": append(baselineFixture(), "LANG=en", "LANG=en"),
		"control":            append(baselineFixture(), "LANG=synthetic-forbidden\nvalue"),
		"invalid utf8":       append(baselineFixture(), "LANG=\xff"),
	} {
		t.Run(name, func(t *testing.T) { assertEnvironmentRefusal(t, env) })
	}
}

func assertEnvironmentRefusal(t *testing.T, env []string) {
	t.Helper()
	got, err := EntryEnvironment(env)
	var refusal *Error
	if got != nil || !errors.As(err, &refusal) || refusal.Code != CodeEnvironmentRefused || refusal.ExitCode() != 78 {
		t.Fatalf("got %v/%v; want environment refusal78/no result", got, err)
	}
	if strings.Contains(err.Error(), "synthetic-forbidden") || strings.Contains(err.Error(), "/fixture/other") {
		t.Fatal("refusal leaked injected value")
	}
}

// Only the two explicitly admitted member selectors are produced, byte-exact;
// a null pair yields neither. No arbitrary child environment map is accepted.
func TestMemberEnvironmentExactOrAbsent(t *testing.T) {
	args := validArgs()
	args[17] = `--member-a2a-agent-json="fixture.member"`
	args[18] = `--member-a2a-config-dir-json="/fixture/member root"`
	selected, err := Parse(args)
	if err != nil {
		t.Fatal(err)
	}
	got, err := selected.MemberEnvironment()
	if err != nil || !reflect.DeepEqual(got, []string{"A2A_AGENT=fixture.member", "A2A_CONFIG_DIR=/fixture/member root"}) {
		t.Fatalf("identity changed: %v %v", got, err)
	}
	absent, err := Parse(validArgs())
	if err != nil {
		t.Fatal(err)
	}
	if got, err := absent.MemberEnvironment(); err != nil || len(got) != 0 {
		t.Fatalf("null identity exported: %v %v", got, err)
	}
	// A post-parse invalid mutation cannot turn a previously checked pair into
	// an arbitrary export; actual retained-record rechecks remain Host-owned.
	*selected.MemberAgent = "../other"
	got, err = selected.MemberEnvironment()
	var refusal *Error
	if got != nil || !errors.As(err, &refusal) || refusal.ExitCode() != 78 {
		t.Fatalf("invalid identity mutation: %v %v", got, err)
	}
}

// The provider/composer result must preserve exactly the admitted member pair,
// or neither selector for a null pair. A missing, changed, duplicate or extra
// messaging selector refuses; this checks bytes, not actual Host membership.
func TestMemberEnvironmentComposedBoundary(t *testing.T) {
	args := validArgs()
	args[17] = `--member-a2a-agent-json="fixture.member"`
	args[18] = `--member-a2a-config-dir-json="/fixture/member root"`
	member, err := Parse(args)
	if err != nil {
		t.Fatal(err)
	}
	absent, err := Parse(validArgs())
	if err != nil {
		t.Fatal(err)
	}
	baseline := append(baselineFixture(), "CODEX_HOME=/fixture/managed")
	pair := []string{"A2A_AGENT=fixture.member", "A2A_CONFIG_DIR=/fixture/member root"}
	for _, input := range []struct {
		selected *Selectors
		env      []string
	}{
		{member, append(append([]string{}, baseline...), pair...)}, {absent, baseline},
	} {
		before := append([]string{}, input.env...)
		if err := input.selected.CheckMemberEnvironment(input.env); err != nil || !reflect.DeepEqual(before, input.env) {
			t.Fatalf("valid final environment refused or changed: %v", err)
		}
	}
	for name, env := range map[string][]string{
		"missing both": baseline,
		"missing root": append(append([]string{}, baseline...), pair[0]),
		"wrong agent":  append(append([]string{}, baseline...), "A2A_AGENT=fixture.other", pair[1]),
		"wrong root":   append(append([]string{}, baseline...), pair[0], "A2A_CONFIG_DIR=/fixture/other"),
		"duplicate":    append(append(append([]string{}, baseline...), pair...), pair[0]),
		"extra name":   append(append(append([]string{}, baseline...), pair...), "A2A_CONNECTION=synthetic-foreign"),
		"empty extra":  append(append(append([]string{}, baseline...), pair...), "A2A_UNKNOWN="),
	} {
		t.Run(name, func(t *testing.T) {
			before := append([]string{}, env...)
			err := member.CheckMemberEnvironment(env)
			var refusal *Error
			if !errors.As(err, &refusal) || refusal.Code != CodeEnvironmentRefused || refusal.ExitCode() != 78 || !reflect.DeepEqual(before, env) {
				t.Fatalf("final environment did not refuse unchanged: %v", err)
			}
			if strings.Contains(err.Error(), "fixture.other") || strings.Contains(err.Error(), "synthetic-foreign") {
				t.Fatal("final environment refusal disclosed a value")
			}
		})
	}
	for _, env := range [][]string{
		append(append([]string{}, baseline...), pair...),
		append(append([]string{}, baseline...), "A2A_AGENT="),
		append(append([]string{}, baseline...), "A2A_UNKNOWN=synthetic-foreign"),
	} {
		if err := absent.CheckMemberEnvironment(env); err == nil {
			t.Fatal("null member acquired messaging selectors")
		}
	}
}
