package werkstattcommand

import "testing"

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
