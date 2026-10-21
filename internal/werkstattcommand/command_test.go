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

// Each refusal names the offending token or rule and the smallest help
// command for that action (CLI review F11); a valid neighbour still parses.
func TestCommandDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{nil, []string{"needs an action", "curator run werkstatt --help"}},
		{[]string{"bogus"}, []string{`"bogus"`, "curator run werkstatt --help"}},
		{[]string{"up"}, []string{"an id is required", "curator run werkstatt up --help"}},
		{[]string{"up", "../bad"}, []string{`"../bad"`, "curator run werkstatt up --help"}},
		{[]string{"up", "example", "other"}, []string{`"other"`, `"example"`}},
		{[]string{"up", "example", "--native"}, []string{"unknown flag --native", "curator run werkstatt up --help"}},
		{[]string{"down", "example", "--new"}, []string{"--new is valid only with up", "curator run werkstatt down --help"}},
		{[]string{"up", "example", "--attach", "--attach"}, []string{"--attach given twice"}},
		{[]string{"up", "example", "--difficulty", "high"}, []string{"--difficulty needs --task-class"}},
		{[]string{"up", "example", "--task-class"}, []string{"--task-class needs a value"}},
		{[]string{"status", "example", "--queue-depth", "-1"}, []string{`--queue-depth "-1"`}},
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
