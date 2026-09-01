package main

import (
	"strings"
	"testing"
)

// run is the production dispatch site: main exits with its return value.
// The stub's one gate is "anything but the two informational flags is a
// usage error, exit 2" — the negative cases below fail if the gate admits
// an argument shape it must reject.

func TestRunInformationalFlags(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--version"}} {
		var out, errOut strings.Builder
		if got := run(args, &out, &errOut); got != 0 {
			t.Errorf("run(%v) = %d, want 0", args, got)
		}
		if !strings.Contains(out.String(), name) || !strings.Contains(out.String(), specVersion) {
			t.Errorf("run(%v) stdout %q missing name or spec version", args, out.String())
		}
		if errOut.Len() != 0 {
			t.Errorf("run(%v) wrote to stderr: %q", args, errOut.String())
		}
	}
}

func TestRunRejectsEverythingElse(t *testing.T) {
	cases := [][]string{
		{},                        // no arguments: nothing to launch
		{"claude_code"},           // a real env-id must still be refused: no logic exists
		{"--version", "extra"},    // informational flag with trailing args
		{"--help", "--version"},   // two flags: not the single-flag shape
		{"--profile", "companyA"}, // spec-declared flag without implementation
		{"--", "resume", "--last"},
		{"--unknown"},
	}
	for _, args := range cases {
		var out, errOut strings.Builder
		if got := run(args, &out, &errOut); got != 2 {
			t.Errorf("run(%v) = %d, want 2", args, got)
		}
		if !strings.Contains(errOut.String(), "not implemented") {
			t.Errorf("run(%v) stderr %q missing refusal text", args, errOut.String())
		}
	}
}
