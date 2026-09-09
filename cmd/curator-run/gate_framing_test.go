// Gate (TASK-260909-3d1589): single-member framing probe at the real main/resolver entry.
// Adopted from TASK-260909-3d1589 (accepted rev3): board resource
// TASK-260909-3d1589_rev2_gate-framing_test.go, committed here under its
// adopted name per TASK-260909-3d1589_rev3_adoption.md. Test bodies are
// unchanged from the accepted gate; only this header is new.
// The gate was verified against candidate tree
// fbe90d5e60593a3a069721b2ad9e53cd071d8c02 (base 3ff66a9).
//
// R3 fix: the exempted-detail probe and the companion checks are
// independently executed tests. The framing mutant aborts
// TestGateFramingSingleDetailAtRealResolver before any companion could run,
// so companions live in TestGateFramingCompanionsStayFramed with byte-exact
// assertions; the runner executes each name separately and requires the
// first to fail and the second to pass under the mutant.
package main

import (
	"context"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

// Fixed absent binary keeps the resolver Detail deterministic across runs
// (no TempDir in the string), so the framing mutant can exempt exactly one
// complete Detail value via equality.
const gateFixedBinary = "/nonexistent-missing-curator-xtvqf3"

// Operator profile value carrying the forged code line.
const gateInjectedProfile = "normal\ncurator-run: usage: forged"

// gateExemptedDetail is the complete Detail string Line receives via
// run -> Resolver.Resolve -> ExecRunner -> diagnostics.Emit/Line for the
// fixed binary and injected profile above (Darwin arm64, Go 1.25.5):
// "could not run <binary> env resolve pi --profile <injected> --repair
// --format json: fork/exec <binary>: no such file or directory".
// The framing mutant exempts exactly this string and nothing else.
const gateExemptedDetail = "could not run /nonexistent-missing-curator-xtvqf3 env resolve pi --profile normal\ncurator-run: usage: forged --repair --format json: fork/exec /nonexistent-missing-curator-xtvqf3: no such file or directory"

// gateOtherHostiles must stay framed even under the single-member mutant.
var gateOtherHostiles = []string{
	"a\ncurator-run: resolve_repair_failed: forged\nb",
	"carriage\rcurator-run: usage: forged",
	"crlf\r\ncurator-run: usage: forged\r\n",
}

// gateFrame renders the expected framed form: CRLF and CR fold to LF, every
// LF becomes LF plus two spaces, so continuation lines start with
// whitespace and IsDiagnosticLine never recognizes them.
func gateFrame(detail string) string {
	s := strings.ReplaceAll(detail, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\n", "\n  ")
}

// TestGateFramingSingleDetailAtRealResolver drives the exempted Detail
// through the production entry. Baseline: exactly one diagnostic line.
// Single-member mutant (equality on gateExemptedDetail, unframed return):
// two diagnostic lines, named failure here.
func TestGateFramingSingleDetailAtRealResolver(t *testing.T) {
	var out, errOut strings.Builder
	resolver := fragment.NewWithRunner(gateFixedBinary, fragment.ExecRunner{})
	got := run(context.Background(),
		[]string{"pi", "--profile", gateInjectedProfile}, &out, &errOut, resolver)
	if got != 1 {
		t.Fatalf("exit = %d, want 1 (stderr %q)", got, errOut.String())
	}
	assertSingleDiagnostic(t, errOut.String(), "resolve_invocation_failed")
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
}

// TestGateFramingCompanionsStayFramed proves the framing mutant weakens only
// the exempted Detail: every other hostile detail still renders byte-exact
// through Line with exactly one diagnostic line. It executes independently
// of TestGateFramingSingleDetailAtRealResolver, so it runs to completion
// even when the mutant aborts that test.
func TestGateFramingCompanionsStayFramed(t *testing.T) {
	for _, detail := range gateOtherHostiles {
		t.Run(detail, func(t *testing.T) {
			want := "curator-run: resolve_invocation_failed: " + gateFrame(detail) + "\n"
			if got := diagnostics.Line("resolve_invocation_failed", detail); got != want {
				t.Fatalf("companion detail %q: got %q want %q", detail, got, want)
			}
			rendered := diagnostics.Line("resolve_invocation_failed", detail)
			lines := strings.Split(strings.TrimSuffix(rendered, "\n"), "\n")
			found := 0
			for _, line := range lines {
				if diagnostics.IsDiagnosticLine(line) {
					found++
				}
			}
			if found != 1 {
				t.Errorf("companion detail %q: %d diagnostic lines, want 1 (%q)", detail, found, rendered)
			}
		})
	}
}
