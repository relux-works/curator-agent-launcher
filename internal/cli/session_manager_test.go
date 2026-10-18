package cli

import "testing"

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
