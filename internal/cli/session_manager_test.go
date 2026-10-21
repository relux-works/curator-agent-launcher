package cli

import (
	"strings"
	"testing"
)

// Every reserved command has a closed grammar, with JSON only on list/show;
// launch flags, duplicate flags and extra/native tails cannot change it.
func TestSessionManagerGrammar(t *testing.T) {
	for _, args := range [][]string{{"list"}, {"list", "--json"}, {"show", "board"}, {"show", "board", "--json"}, {"select", "board"}, {"unregister", "board"}, {"register", "--manifest", "-"}, {"register", "--select", "--manifest", "file"}, {"register", "--manifest", "file", "--no-select"}} {
		got, err := Parse(append([]string{"session-manager"}, args...), Options{AxConfigured: true})
		if err != nil || got.SessionManager == nil || got.SessionManager.Operation != args[0] || got.EnvID != "" || got.Tracked {
			t.Fatal(args, got, err)
		}
	}
	for _, args := range [][]string{{}, {"unknown"}, {"list", "other"}, {"list", "--json", "--json"}, {"show"}, {"show", "Bad"}, {"show", "id", "extra"}, {"select", "id", "--json"}, {"unregister", "id", "--"}, {"register"}, {"register", "--manifest"}, {"register", "--manifest", "--select"}, {"register", "--manifest", "-", "--select", "--no-select"}, {"register", "--manifest", "-", "--manifest", "file"}, {"register", "--manifest", "-", "--select", "--select"}, {"register", "--manifest", "-", "--json"}} {
		got, err := Parse(append([]string{"session-manager"}, args...), Options{})
		if !IsUsage(err) || got.SessionManager != nil {
			t.Fatal(args, got, err)
		}
	}
	if _, err := Parse([]string{"--model", "m", "session-manager", "list"}, Options{}); !IsUsage(err) {
		t.Fatal(err)
	}
	got, err := Parse([]string{"pi", "--", "session-manager"}, Options{})
	if err != nil || got.SessionManager != nil || len(got.Native) != 1 {
		t.Fatal(got, err)
	}
}

// Hosted switches have a closed boolean grammar and remain opaque after --.
func TestHostedSwitches(t *testing.T) {
	inv, err := Parse([]string{"claude_code", "--hosted", "--board-goal", "--detach"}, Options{})
	if err != nil || !inv.BoardGoal || !inv.Detach {
		t.Fatalf("%+v %v", inv, err)
	}
	for _, args := range [][]string{{"claude_code", "--board-goal=true"}, {"claude_code", "--detach=false"}, {"claude_code", "--board-goal", "--board-goal"}, {"claude_code", "--detach", "--detach"}} {
		if _, err := Parse(args, Options{}); !IsUsage(err) {
			t.Fatalf("invalid switches admitted: %v %v", args, err)
		}
	}
	inv, err = Parse([]string{"claude_code", "--", "--board-goal", "--detach"}, Options{})
	if err != nil || inv.BoardGoal || inv.Detach || len(inv.Native) != 2 {
		t.Fatalf("native tail changed: %+v %v", inv, err)
	}
}

// Registry grammar refusals name the offending token or rule and the smallest
// help command (CLI review F11); valid neighbours still parse.
func TestSessionManagerDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"session-manager"}, []string{"needs an operation", "curator run session-manager --help"}},
		{[]string{"session-manager", "use", "x"}, []string{"unknown session-manager operation", "curator run session-manager --help"}},
		{[]string{"session-manager", "list", "extra"}, []string{"unexpected argument", "curator run session-manager list --help"}},
		{[]string{"session-manager", "show"}, []string{"manager id is required", "curator run session-manager show --help"}},
		{[]string{"session-manager", "select", "Bad_ID"}, []string{"invalid manager id"}},
		{[]string{"session-manager", "register"}, []string{"--manifest is required", "curator run session-manager register --help"}},
		{[]string{"session-manager", "register", "--manifest"}, []string{"--manifest needs a path or -"}},
		{[]string{"session-manager", "register", "--manifest", "-", "--select", "--no-select"}, []string{"mutually exclusive"}},
		{[]string{"session-manager", "register", "--manifest", "-", "--select", "--select"}, []string{"--select given twice"}},
		{[]string{"session-manager", "register", "--manifest", "-", "--force"}, []string{"unknown argument"}},
	} {
		_, err := Parse(tc.args, Options{})
		if err == nil || !IsUsage(err) {
			t.Fatalf("%v: %v", tc.args, err)
		}
		for _, w := range tc.want {
			if !strings.Contains(err.Error(), w) {
				t.Fatalf("%v: %q lacks %q", tc.args, err, w)
			}
		}
	}
	for _, args := range [][]string{{"session-manager", "list", "--json"}, {"session-manager", "register", "--manifest", "-", "--no-select"}, {"session-manager", "select", "task-board"}} {
		if _, err := Parse(args, Options{}); err != nil {
			t.Fatal(args, err)
		}
	}
}

// A registry refusal never echoes an id, a value or an unknown token; the
// same positions with valid input parse (review B1 on launcher #57).
func TestSessionManagerDiagnosticsDiscloseNoInput(t *testing.T) {
	const secret = "SENTINEL_SECRET_57"
	for _, args := range [][]string{
		{"session-manager", secret},
		{"session-manager", "list", "--token=" + secret},
		{"session-manager", "show", "x" + secret + "!"},
		{"session-manager", "show", "board", secret},
		{"session-manager", "register", "--manifest", "-", "--token=" + secret},
		{"session-manager", "register", "--manifest", "-", "--token=" + secret, "--token=" + secret},
	} {
		_, err := Parse(args, Options{})
		if err == nil || !IsUsage(err) {
			t.Fatalf("%v: %v", args, err)
		}
		if strings.Contains(err.Error(), "SENTINEL") {
			t.Fatalf("%v disclosed input: %q", args, err)
		}
	}
	for _, args := range [][]string{{"session-manager", "show", "board"}, {"session-manager", "register", "--manifest", "-"}} {
		if _, err := Parse(args, Options{}); err != nil {
			t.Fatal(args, err)
		}
	}
}

// Topic help is chosen only when -h or --help appears in an administrative
// grammar; a known verb selects its own text, anything else the topic text.
// Launch arguments never match, so a native tail cannot ask for topic help.
func TestTopicHelp(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		first string
	}{
		{[]string{"werkstatt", "--help"}, "usage: curator run werkstatt up <id>"},
		{[]string{"werkstatt", "down", "x", "-h"}, "usage: curator run werkstatt down"},
		{[]string{"werkstatt", "bogus", "--help"}, "usage: curator run werkstatt up <id>"},
		{[]string{"session-manager", "unregister", "--help"}, "usage: curator run session-manager unregister"},
		{[]string{"migrate", "-h"}, "usage: curator run migrate"},
	} {
		text, ok := TopicHelp(tc.args)
		if !ok || !strings.HasPrefix(text, tc.first) {
			t.Fatalf("%v: %v %q", tc.args, ok, text)
		}
	}
	for _, args := range [][]string{nil, {"werkstatt", "status"}, {"claude_code", "--help"}, {"claude_code", "--", "werkstatt", "--help"}, {"--help"}} {
		if _, ok := TopicHelp(args); ok {
			t.Fatalf("%v selected topic help", args)
		}
	}
}
