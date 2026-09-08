package main

import (
	"strings"
	"testing"
)

// run is the production dispatch site: main exits with its return value.
// These tests drive SPEC §3 shapes through it end to end: informational
// flags exit 0 on stdout, usage errors exit 2 with the "usage" code line
// and the usage text on stderr, and a parsed launch is refused with exit 1
// because this build carries no composition stage.

func TestRunInformationalFlags(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--version"}, {"codex_cli", "--profile", "p", "--help"}, {"--version", "--", "x"}} {
		var out, errOut strings.Builder
		if got := run(args, &out, &errOut); got != 0 {
			t.Errorf("run(%v) = %d, want 0 (stderr %q)", args, got, errOut.String())
		}
		if !strings.Contains(out.String(), name) || !strings.Contains(out.String(), specVersion) || !strings.Contains(out.String(), buildVersion) {
			t.Errorf("run(%v) stdout %q missing name, build or spec version", args, out.String())
		}
		if errOut.Len() != 0 {
			t.Errorf("run(%v) wrote to stderr: %q", args, errOut.String())
		}
	}
}

func TestRunUsageErrorsExit2(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{}, "missing <env-id>"},
		{[]string{"--"}, "missing <env-id>"},
		{[]string{"codex_cli", "resume"}, "stray operand"},
		{[]string{"codex_cli", "--unknown"}, "unknown flag"},
		{[]string{"codex_cli", "--profile"}, "requires a value"},
		{[]string{"codex_cli", "--profile", "a", "--profile", "b"}, "more than once"},
		{[]string{"pi", "--system-prompt", "prepend"}, "accepts append or replace"},
		{[]string{"codex_cli", "--ax-profile", "bogus"}, "accepts standard or yolo"},
		// This build reads no ax.json, so the integration is never configured:
		// --ax-profile is the §3 untracked usage error at the entry point.
		{[]string{"codex_cli", "--ax-profile", "yolo"}, "not configured"},
		{[]string{"codex_cli", "--name", "bad/name"}, "not a valid ax session name"},
		{[]string{"codex_cli", "--name", strings.Repeat("x", 65)}, "not a valid ax session name"},
		{[]string{"--nope", "--help"}, "unknown flag"},
	}
	for _, c := range cases {
		var out, errOut strings.Builder
		if got := run(c.args, &out, &errOut); got != 2 {
			t.Errorf("run(%v) = %d, want 2", c.args, got)
		}
		first, _, _ := strings.Cut(errOut.String(), "\n")
		if !strings.HasPrefix(first, name+": usage: ") {
			t.Errorf("run(%v) first stderr line %q is not the usage code line", c.args, first)
		}
		if !strings.Contains(first, c.want) {
			t.Errorf("run(%v) stderr %q missing %q", c.args, first, c.want)
		}
		if !strings.Contains(errOut.String(), "usage: curator-run <env-id>") {
			t.Errorf("run(%v) stderr missing usage text", c.args)
		}
		if out.Len() != 0 {
			t.Errorf("run(%v) wrote to stdout: %q", c.args, out.String())
		}
	}
}

// TestRunParsedLaunchRefused: a well-formed launch, including a native tail
// that collides with launcher flags, parses and is then refused with exit
// 1 — never 2 (it is not a usage error) and never 0 (nothing launched).
func TestRunParsedLaunchRefused(t *testing.T) {
	cases := [][]string{
		{"codex_cli"},
		{"claude_code", "--profile", "companyA", "--model", "m", "--effort", "high", "--", "resume", "--last"},
		{"pi", "--system-prompt", "append", "--name", "ok-name", "--", "--help", "", "--ax-profile", "yolo"},
	}
	for _, args := range cases {
		var out, errOut strings.Builder
		if got := run(args, &out, &errOut); got != 1 {
			t.Errorf("run(%v) = %d, want 1 (stderr %q)", args, got, errOut.String())
		}
		if !strings.HasPrefix(errOut.String(), name+": not_implemented: ") {
			t.Errorf("run(%v) stderr %q missing not_implemented line", args, errOut.String())
		}
		if out.Len() != 0 {
			t.Errorf("run(%v) wrote to stdout: %q", args, out.String())
		}
	}
}

// TestSpecVersionPinned fails when the reported specification version
// drifts from the version SPEC.md and README.md state; the three are one fact.
func TestSpecVersionPinned(t *testing.T) {
	const want = "0.2.1-draft"
	if specVersion != want {
		t.Fatalf("specVersion = %q, want %q", specVersion, want)
	}
	var out, errOut strings.Builder
	if got := run([]string{"--version"}, &out, &errOut); got != 0 {
		t.Fatalf("run(--version) = %d, want 0", got)
	}
	if out.String() != name+" "+buildVersion+" (specification "+want+")\n" {
		t.Fatalf("run(--version) stdout = %q", out.String())
	}
}
