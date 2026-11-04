package cli

import "testing"

// Member selection is explicit, closed to existing record IDs, and never
// consumes a native-tail flag or changes a resume selector.
func TestMemberSelection(t *testing.T) {
	for _, args := range [][]string{{"codex_cli", "--member", "example"}, {"codex_cli", "resume", "SES-example", "--member=example"}} {
		inv, err := Parse(args)
		if err != nil {
			t.Fatalf("valid explicit member refused: %v", err)
		}
		if inv.MemberID != "example" {
			t.Fatal("member selection lost")
		}
	}
	for _, args := range [][]string{{"codex_cli", "--member"}, {"codex_cli", "--member=../example"}, {"codex_cli", "--member="}, {"codex_cli", "--member", "example", "--member", "example"}} {
		if _, err := Parse(args); !IsUsage(err) {
			t.Fatalf("invalid member accepted: %v", args)
		}
	}
	inv, err := Parse([]string{"codex_cli", "--", "--member", "example"})
	if err != nil || inv.MemberID != "" || len(inv.Native) != 2 {
		t.Fatal("native tail became member selection")
	}
}
