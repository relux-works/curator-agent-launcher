package cli

import (
	"testing"
)

// The reserved operand accepts exactly the bare and dry-run forms and never
// treats migration flags or extra operands as a launch/native argument tail.
func TestMigrateGrammar(t *testing.T) {
	for _, args := range [][]string{{"migrate"}, {"migrate", "--dry-run"}} {
		got, err := Parse(args)
		if err != nil || !got.Migrate || got.DryRun != (len(args) == 2) || got.EnvID != "" {
			t.Fatal(args, got, err)
		}
	}
	for _, args := range [][]string{{"migrate", "--dry-run", "--dry-run"}, {"migrate", "--hosted"}, {"migrate", "--"}, {"migrate", "extra"}, {"migrate", "--dry-run=true"}, {"pi", "--dry-run"}, {"--profile", "p", "migrate"}, {"--model", "m", "migrate"}} {
		got, err := Parse(args)
		if !IsUsage(err) || got.Migrate {
			t.Fatal(args, got, err)
		}
	}
	got, err := Parse([]string{"pi", "--model", "migrate"})
	if err != nil || got.Migrate || got.Model != "migrate" {
		t.Fatal("model value", got, err)
	}
	got, err = Parse([]string{"pi", "--", "migrate"})
	if err != nil || got.Migrate || len(got.Native) != 1 || got.Native[0] != "migrate" {
		t.Fatal("native operand", got, err)
	}
}
