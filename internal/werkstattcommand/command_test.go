package werkstattcommand

import (
	"strings"
	"testing"
)

// Operation-specific flags are accepted; conflicting, injected launch, traversal
// and incomplete dispatch arguments are usage errors before any filesystem IO.
func TestCommand(t *testing.T) {
	for _, args := range [][]string{{"up", "example"}, {"up", "example", "--new", "--attach", "--task-class", "code.implement", "--difficulty", "high"}, {"down", "example", "--force"}, {"recycle", "example"}, {"status"}, {"status", "example", "--json", "--set-task", "TASK-example", "--queue-depth", "2"}, {"status", "example", "--clear-task", "--abandon-operation"}, {"list", "--json"}} {
		if _, e := ParseCommand(args); e != nil {
			t.Fatal(args, e)
		}
	}
	for _, args := range [][]string{{}, {"up"}, {"up", "../bad"}, {"up", "example", "--native"}, {"up", "example", "--difficulty", "high"}, {"status", "--set-task", "task"}, {"status", "example", "--set-task", "task", "--clear-task"}, {"status", "example", "--rebind-manager", "--abandon-operation"}, {"list", "example"}, {"recycle", "example", "--force"}, {"down", "example", "--new"}, {"status", "example", "--queue-depth", "-1"}, {"up", "example", "--attach", "--attach"}} {
		if _, e := ParseCommand(args); e == nil {
			t.Fatal("accepted", args)
		}
	}
}

// Each refusal names the rule and the smallest help command for that action
// (CLI review F11); a valid neighbour still parses.
func TestCommandDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{nil, []string{"needs an action", "curator run werkstatt --help"}},
		{[]string{"bogus"}, []string{"unknown werkstatt action", "curator run werkstatt --help"}},
		{[]string{"up"}, []string{"an id is required", "curator run werkstatt up --help"}},
		{[]string{"up", "../bad"}, []string{"invalid id", "curator run werkstatt up --help"}},
		{[]string{"up", "example", "other"}, []string{"unexpected second operand"}},
		{[]string{"up", "example", "--native"}, []string{"unknown flag", "curator run werkstatt up --help"}},
		{[]string{"down", "example", "--new"}, []string{"--new is valid only with up", "curator run werkstatt down --help"}},
		{[]string{"up", "example", "--attach", "--attach"}, []string{"--attach given twice"}},
		{[]string{"up", "example", "--difficulty", "high"}, []string{"--difficulty needs --task-class"}},
		{[]string{"up", "example", "--task-class"}, []string{"--task-class needs a value"}},
		{[]string{"status", "example", "--queue-depth", "-1"}, []string{"--queue-depth needs an integer"}},
		{[]string{"status", "--set-task", "task"}, []string{"need an id", "curator run werkstatt status --help"}},
		{[]string{"status", "example", "--set-task", "task", "--clear-task"}, []string{"mutually exclusive"}},
		{[]string{"status", "example", "--rebind-manager", "--abandon-operation"}, []string{"mutually exclusive"}},
		{[]string{"list", "example"}, []string{"list takes no id", "curator run werkstatt list --help"}},
	} {
		_, err := ParseCommand(tc.args)
		if err == nil {
			t.Fatalf("accepted %v", tc.args)
		}
		for _, w := range tc.want {
			if !strings.Contains(err.Error(), w) {
				t.Fatalf("%v: %q lacks %q", tc.args, err, w)
			}
		}
	}
	if _, err := ParseCommand([]string{"up", "example", "--task-class", "code.implement"}); err != nil {
		t.Fatal(err)
	}
}

// A refusal never echoes an operand, a value or an unknown token, because a
// misplaced credential can sit in any of them; the same positions with valid
// input parse (review B1 on launcher #57).
func TestCommandDiagnosticsDiscloseNoInput(t *testing.T) {
	const secret = "SENTINEL_SECRET_57"
	for _, args := range [][]string{
		{secret},
		{"up", secret},
		{"up", "example", secret},
		{"up", "example", "--token=" + secret},
		{"up", "example", "--" + secret},
		{"up", "example", "--token=" + secret, "--token=" + secret},
		{"up", "example", "--task-class", secret + "!"},
		{"up", "example", "--task-class", "c", "--difficulty", secret + "!"},
		{"status", "example", "--queue-depth", secret},
		{"list", secret},
	} {
		_, err := ParseCommand(args)
		if err == nil {
			t.Fatalf("accepted %v", args)
		}
		if strings.Contains(err.Error(), "SENTINEL") {
			t.Fatalf("%v disclosed input: %q", args, err)
		}
	}
	for _, args := range [][]string{{"up", "example"}, {"up", "example", "--task-class", "c", "--difficulty", "d"}, {"status", "example", "--queue-depth", "3"}} {
		if _, err := ParseCommand(args); err != nil {
			t.Fatal(args, err)
		}
	}
}

// Dedicated edits/recovery parse beside their legacy aliases; wrong-command
// flags and incomplete requests identify the corrective command without IO.
func TestEditRecoveryGrammar(t *testing.T) {
	for _, args := range [][]string{
		{"update", "example", "--set-task", "task", "--queue-depth", "0", "--json"},
		{"update", "example", "--clear-task"},
		{"update", "example", "--queue-depth", "9007199254740991"},
		{"recover", "example", "--rebind-manager", "--json"},
		{"recover", "example", "--abandon-operation"},
		{"status", "example", "--set-task", "task", "--rebind-manager"},
	} {
		if _, err := ParseCommand(args); err != nil {
			t.Fatal(args, err)
		}
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"update", "example"}, "at least one of --set-task, --clear-task or --queue-depth"},
		{[]string{"update", "--clear-task"}, "an id is required"},
		{[]string{"recover", "--abandon-operation"}, "an id is required"},
		{[]string{"recover", "example"}, "exactly one of --rebind-manager or --abandon-operation"},
		{[]string{"recover", "example", "--rebind-manager", "--abandon-operation"}, "mutually exclusive"},
		{[]string{"recover", "example", "--set-task"}, "valid only with update"},
		{[]string{"recover", "example", "--queue-depth"}, "valid only with update"},
		{[]string{"recover", "example", "--clear-task"}, "valid only with update"},
		{[]string{"recover", "example", "--set-task", "task"}, "valid only with update"},
		{[]string{"recover", "example", "--queue-depth", "1"}, "valid only with update"},
		{[]string{"update", "example", "--rebind-manager"}, "valid only with recover"},
		{[]string{"update", "example", "--abandon-operation"}, "valid only with recover"},
		{[]string{"down", "example", "--set-task", "task"}, "valid only with update"},
		{[]string{"up", "example", "--rebind-manager"}, "valid only with recover"},
		{[]string{"update", "example", "--force"}, "valid only with down"},
		{[]string{"recover", "example", "--new"}, "valid only with up"},
		{[]string{"update", "example", "--set-task", "task", "--clear-task"}, "mutually exclusive"},
		{[]string{"update", "example", "--queue-depth", "-1"}, "needs an integer"},
	} {
		_, err := ParseCommand(tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: %v, want %q", tc.args, err, tc.want)
		}
	}
}
