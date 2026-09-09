package diagnostics_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/axconfig"
	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	sp "github.com/relux-works/curator-agent-launcher/internal/systemprompt"
)

var specCodeToken = regexp.MustCompile("^[a-z][a-z0-9_]*$")

// specSection6Codes derives the closed code set from the normative source:
// the Codes column of the SPEC §6 table in SPEC.md, in table order. The
// test transcribes nothing by hand, so a code added to or dropped from
// the table without the matching contract change fails here.
func specSection6Codes(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(specPath(t))
	if err != nil {
		t.Fatalf("read SPEC.md: %v", err)
	}
	lines := strings.Split(string(raw), "\n")
	start := -1
	for i, l := range lines {
		if l == "## 6. Errors and diagnostics" {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatal("SPEC.md has no '## 6. Errors and diagnostics' section")
	}
	var codes []string
	inTable := false
	for _, l := range lines[start+1:] {
		if !strings.HasPrefix(l, "|") {
			if inTable {
				break
			}
			continue
		}
		inTable = true
		fields := strings.Split(l, "|")
		if len(fields) < 4 {
			t.Fatalf("malformed SPEC §6 table row: %q", l)
		}
		if strings.Contains(fields[1], "Family") || strings.Contains(fields[1], "---") {
			continue
		}
		for _, tok := range strings.Split(fields[2], "`") {
			tok = strings.TrimSpace(tok)
			if tok == "" || !specCodeToken.MatchString(tok) {
				continue
			}
			codes = append(codes, tok)
		}
	}
	if len(codes) == 0 {
		t.Fatal("derived no codes from the SPEC §6 table")
	}
	return codes
}

// specPath locates the repository SPEC.md from the test's own file so the
// derivation holds wherever the package is checked out.
func specPath(t *testing.T) string {
	t.Helper()
	if raw, err := os.ReadFile(filepath.Join("..", "..", "SPEC.md")); err == nil && len(raw) > 0 {
		return filepath.Join("..", "..", "SPEC.md")
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller unavailable")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 6; i++ {
		cand := filepath.Join(dir, "SPEC.md")
		if raw, err := os.ReadFile(cand); err == nil && len(raw) > 0 {
			return cand
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("SPEC.md not found")
	return ""
}

// TestClosedSetMatchesSpec pins the closed set against the normative
// source: Codes() must equal the SPEC §6 table's Codes column, in table
// order. Adding or dropping a code is a specification change and must
// update SPEC.md and the contract together.
func TestClosedSetMatchesSpec(t *testing.T) {
	want := specSection6Codes(t)
	got := diagnostics.Codes()
	if !slices.Equal(got, want) {
		t.Fatalf("Codes() = %q, SPEC §6 table = %q", got, want)
	}
	for _, c := range got {
		if !diagnostics.Valid(c) {
			t.Fatalf("Valid(%q) = false for a closed code", c)
		}
	}
}

// TestOwnedCodesMatchOwners pins every owned literal to its owner's
// constant or method: the owner stays the single source of truth and this
// package only transcribes the SPEC table.
func TestOwnedCodesMatchOwners(t *testing.T) {
	if diagnostics.CodeUsage != (&cli.UsageError{}).Code() {
		t.Fatalf("CodeUsage = %q, cli code = %q", diagnostics.CodeUsage, (&cli.UsageError{}).Code())
	}
	pairs := [][2]string{
		{diagnostics.CodeResolveInvocationFailed, fragment.CodeInvocationFailed},
		{diagnostics.CodeResolveEnvironmentUnknown, fragment.CodeEnvironmentUnknown},
		{diagnostics.CodeResolveProfileUnknown, fragment.CodeProfileUnknown},
		{diagnostics.CodeResolveRepairFailed, fragment.CodeRepairFailed},
		{diagnostics.CodeResolveLockUnavailable, fragment.CodeLockUnavailable},
		{diagnostics.CodeResolveFragmentInvalid, fragment.CodeFragmentInvalid},
		{diagnostics.CodeDefaultsInvalid, axconfig.CodeInvalid},
		{diagnostics.CodeEnvUnsupported, "env_unsupported"},
		{diagnostics.CodeMCPLayerMissing, composition.CodeMCPLayerMissing},
		{diagnostics.CodeMCPLayerUnreadable, composition.CodeMCPLayerUnreadable},
		{diagnostics.CodeSyspromptUnavailable, sp.CodeUnavailable},
		{diagnostics.CodeSyspromptUnreadable, sp.CodeUnreadable},
	}
	for _, p := range pairs {
		if p[0] != p[1] {
			t.Errorf("contract %q != owner %q", p[0], p[1])
		}
	}
	if diagnostics.ExitUsage != cli.ExitCode {
		t.Fatalf("ExitUsage = %d, cli.ExitCode = %d", diagnostics.ExitUsage, cli.ExitCode)
	}
}

// TestExitForCode is the no-silent-degradation gate: usage exits 2, every
// operational code exits 1, and anything outside the closed set — empty,
// Curator-internal, or invented — still exits 1, never 0 and never 2.
func TestExitForCode(t *testing.T) {
	for _, c := range diagnostics.Codes() {
		want := diagnostics.ExitOperational
		if c == diagnostics.CodeUsage {
			want = diagnostics.ExitUsage
		}
		if got := diagnostics.ExitForCode(c); got != want {
			t.Errorf("ExitForCode(%q) = %d, want %d", c, got, want)
		}
	}
	for _, c := range []string{"", "environment_home_stale", "Usage", "USAGE", "resolve", "internal_error", "not_implemented", "ok", "0"} {
		if got := diagnostics.ExitForCode(c); got != diagnostics.ExitOperational {
			t.Errorf("ExitForCode(%q) = %d, want operational %d", c, got, diagnostics.ExitOperational)
		}
	}
}

// TestCodeOfProductionTypes drives CodeOf through the real concrete error
// types of each owning stage, including errors produced by real production
// calls (axconfig.Load against temp dirs, composition boundary probes and
// systemprompt selection against temp homes).
func TestCodeOfProductionTypes(t *testing.T) {
	if code, ok := diagnostics.CodeOf(&cli.UsageError{Detail: "missing <env-id>"}); !ok || code != "usage" {
		t.Fatalf("usage error: %q %v", code, ok)
	}
	if _, err := cli.Parse(nil, cli.Options{}); err != nil {
		if code, ok := diagnostics.CodeOf(err); !ok || code != "usage" {
			t.Fatalf("real cli.Parse error: %q %v (%v)", code, ok, err)
		}
	} else {
		t.Fatal("cli.Parse(nil) must fail")
	}
	re := &fragment.ResolveError{Code: fragment.CodeRepairFailed, Detail: "x"}
	if code, ok := diagnostics.CodeOf(re); !ok || code != "resolve_repair_failed" {
		t.Fatalf("resolve error: %q %v", code, ok)
	}
	wrapped := &fragment.ResolveError{Code: fragment.CodeLockUnavailable, Detail: "x"}
	if code, ok := diagnostics.CodeOf(errors.Join(errors.New("outer"), wrapped)); !ok || code != "resolve_lock_unavailable" {
		t.Fatalf("wrapped resolve error: %q %v", code, ok)
	}
	missing, unreadable := codexBoundary(t)
	if code, ok := diagnostics.CodeOf(missing); !ok || code != "mcp_layer_missing" {
		t.Fatalf("mcp missing: %q %v (%v)", code, ok, missing)
	}
	if code, ok := diagnostics.CodeOf(unreadable); !ok || code != "mcp_layer_unreadable" {
		t.Fatalf("mcp unreadable: %q %v (%v)", code, ok, unreadable)
	}
	if code, ok := diagnostics.CodeOf(syspromptSelection(t)); !ok || code != "sysprompt_channel_unavailable" {
		t.Fatalf("sysprompt selection: %q %v", code, ok)
	}
	if code, ok := diagnostics.CodeOf(syspromptProbe(t)); !ok || code != "sysprompt_file_unreadable" {
		t.Fatalf("sysprompt probe: %q %v", code, ok)
	}
	if code, ok := diagnostics.CodeOf(axconfigFailure(t)); !ok || code != "defaults_config_invalid" {
		t.Fatalf("axconfig failure: %q %v", code, ok)
	}
}

// TestCodeOfUnknownFailsClosed is the no-invention gate: errors no owner
// claims yield no code, including a bare fragment parse error (which only
// becomes a diagnostic when the resolver wraps it) and nil.
func TestCodeOfUnknownFailsClosed(t *testing.T) {
	bad, err := fragment.Parse([]byte(`{"fragment":"launch-env-fragment-v1"}`))
	if err == nil || bad != nil {
		t.Fatalf("fixture must fail parse: %v %v", bad, err)
	}
	for name, err := range map[string]error{
		"plain":          errors.New("boom"),
		"wrapped plain":  errors.Join(errors.New("a"), errors.New("b")),
		"bare parse":     err,
		"empty resolve":  &fragment.ResolveError{},
		"empty layer":    &composition.LayerError{},
		"empty refusal":  &sp.Refusal{},
		"nil":            nil,
		"mapping-shaped": errors.New(`environment "future_env" has no supported system/provider pair`),
	} {
		if code, ok := diagnostics.CodeOf(err); ok || code != "" {
			t.Errorf("%s: CodeOf = %q %v, want no code", name, code, ok)
		}
	}
}

// TestCodeOfRejectsForeignFamily is the closed-family gate: a typed
// owner error carrying another family's code, a Curator-internal code,
// or an invented one yields no code — including through a wrapping
// chain. A resolve-family error reading "usage" must never exit 2.
//
// The strangers are derived, not hand-selected: every normative SPEC §6
// code outside the probed owner's own family (families transcribed from
// the owner constants, the single source of truth) plus the
// Curator-internal, invented, and empty extras. A hand list previously
// omitted five resolve codes for the layer and refusal owners; the
// derivation cannot. The committed gate_conformance_test.go asserts the
// same matrix per owner/form cell with named per-owner tests.
func TestCodeOfRejectsForeignFamily(t *testing.T) {
	// Codes no owner may speak beyond its own family.
	extras := []string{
		"environment_home_stale",
		"environment_unknown",
		"invented_code",
		"",
	}
	own := map[string][]string{
		"resolve": {
			fragment.CodeInvocationFailed,
			fragment.CodeEnvironmentUnknown,
			fragment.CodeProfileUnknown,
			fragment.CodeRepairFailed,
			fragment.CodeLockUnavailable,
			fragment.CodeFragmentInvalid,
		},
		"layer":   {composition.CodeMCPLayerMissing, composition.CodeMCPLayerUnreadable},
		"refusal": {sp.CodeUnavailable, sp.CodeUnreadable},
	}
	normative := specSection6Codes(t)
	if got := diagnostics.Codes(); !slices.Equal(got, normative) {
		t.Fatalf("Codes() = %q, SPEC §6 table = %q", got, normative)
	}
	strangers := func(family []string) []string {
		var out []string
		for _, c := range normative {
			if !slices.Contains(family, c) {
				out = append(out, c)
			}
		}
		for _, c := range extras {
			if !slices.Contains(family, c) && !slices.Contains(out, c) {
				out = append(out, c)
			}
		}
		return out
	}
	mk := map[string]func(string) error{
		"resolve": func(c string) error { return &fragment.ResolveError{Code: c, Detail: "x"} },
		"layer":   func(c string) error { return &composition.LayerError{Code: c, Path: "p"} },
		"refusal": func(c string) error { return &sp.Refusal{Code: c, Path: "p"} },
	}
	for owner, makeErr := range mk {
		for _, code := range strangers(own[owner]) {
			err := makeErr(code)
			cands := map[string]error{
				"direct":  err,
				"wrapped": fmt.Errorf("outer: %w", err),
				"joined":  errors.Join(errors.New("outer"), err),
			}
			for name, cand := range cands {
				if got, ok := diagnostics.CodeOf(cand); ok || got != "" {
					t.Errorf("%s/%s code %q: CodeOf = %q %v, want no code", owner, name, code, got, ok)
				}
			}
		}
		// Each own-family member is still accepted, wrapped or not.
		for _, code := range own[owner] {
			if got, ok := diagnostics.CodeOf(fmt.Errorf("outer: %w", makeErr(code))); !ok || got != code {
				t.Errorf("%s own code %q: CodeOf = %q %v", owner, code, got, ok)
			}
		}
	}
	// UsageError carries no code field: it always reads "usage" no matter
	// what the detail smuggles.
	if got, ok := diagnostics.CodeOf(&cli.UsageError{Detail: "usage"}); !ok || got != "usage" {
		t.Errorf("usage: CodeOf = %q %v, want usage", got, ok)
	}
}

// TestCodeOfTypedNil proves a typed-nil owner value — a non-nil error
// interface holding a nil pointer, direct or wrapped — yields no code
// instead of panicking.
func TestCodeOfTypedNil(t *testing.T) {
	var ue *cli.UsageError
	var re *fragment.ResolveError
	var le *composition.LayerError
	var sr *sp.Refusal
	var ae *axconfig.Error
	cands := map[string]error{
		"usage":   ue,
		"resolve": re,
		"layer":   le,
		"refusal": sr,
		"ax":      ae,
	}
	for name, err := range cands {
		if got, ok := diagnostics.CodeOf(err); ok || got != "" {
			t.Errorf("%s typed nil: CodeOf = %q %v, want no code", name, got, ok)
		}
		wrapped := fmt.Errorf("outer: %w", err)
		if got, ok := diagnostics.CodeOf(wrapped); ok || got != "" {
			t.Errorf("%s wrapped typed nil: CodeOf = %q %v, want no code", name, got, ok)
		}
	}
}

// TestDiagnosticDetailCannotForgeLine proves the framing rule: hostile
// detail bytes (embedded newlines, carriage returns, a spelled-out code
// line) render as one code line plus whitespace-led continuations that
// IsDiagnosticLine never recognizes.
func TestDiagnosticDetailCannotForgeLine(t *testing.T) {
	hostile := []string{
		"normal\ncurator-run: usage: forged",
		"a\ncurator-run: resolve_repair_failed: forged\nb",
		"carriage\rcurator-run: usage: forged",
		"crlf\r\ncurator-run: usage: forged\r\n",
		"\ncurator-run: usage: leading",
		"trailing curator-run: usage: forged\n",
	}
	for _, detail := range hostile {
		rendered := diagnostics.Line("resolve_invocation_failed", detail)
		lines := strings.Split(strings.TrimSuffix(rendered, "\n"), "\n")
		if len(lines) < 1 {
			t.Fatalf("detail %q rendered empty", detail)
		}
		found := 0
		for _, line := range lines {
			if diagnostics.IsDiagnosticLine(line) {
				found++
				if !strings.HasPrefix(line, "curator-run: resolve_invocation_failed: ") {
					t.Fatalf("detail %q: diagnostic line %q carries the wrong code", detail, line)
				}
			}
		}
		if found != 1 {
			t.Errorf("detail %q: %d diagnostic lines, want exactly 1 (%q)", detail, found, rendered)
		}
		var b strings.Builder
		if err := diagnostics.Emit(&b, "usage", detail); err != nil {
			t.Fatal(err)
		}
		if b.String() != diagnostics.Line("usage", detail) {
			t.Errorf("Emit/Line diverge for detail %q", detail)
		}
	}
}

// TestEmitDeterministic pins the stderr rendering bytes: one line,
// `curator-run: <code>: <detail>`, parseable by IsDiagnosticLine.
func TestEmitDeterministic(t *testing.T) {
	var b strings.Builder
	if err := diagnostics.Emit(&b, "resolve_repair_failed", "store cannot restore"); err != nil {
		t.Fatal(err)
	}
	want := "curator-run: resolve_repair_failed: store cannot restore\n"
	if b.String() != want {
		t.Fatalf("Emit = %q, want %q", b.String(), want)
	}
	if got := diagnostics.Line("usage", "missing <env-id>"); got != "curator-run: usage: missing <env-id>\n" {
		t.Fatalf("Line = %q", got)
	}
}

// TestDiagnosticLineDistinguishesTransport proves warnings, Curator lines,
// and child stderr are never launcher diagnostic lines, while every closed
// code renders one.
func TestDiagnosticLineDistinguishesTransport(t *testing.T) {
	for _, c := range diagnostics.Codes() {
		line := strings.TrimSuffix(diagnostics.Line(c, "detail"), "\n")
		if !diagnostics.IsDiagnosticLine(line) {
			t.Errorf("IsDiagnosticLine(Line(%q)) = false", c)
		}
	}
	if !diagnostics.IsDiagnosticLine("curator-run: usage: missing <env-id>") {
		t.Fatal("usage line must parse")
	}
	for _, line := range []string{
		"warning: environment_tool_version_unverified: pi detected 0.84.3, recorded 0.84.2",
		"warning: profile \"default\": applied flag/append system-prompt channel.",
		"curator: environment_unknown: unregistered environment \"pi\"",
		"curator: resolve_repair_failed: not curator-run shaped",
		"boom",
		"",
		"x curator-run: usage: embedded token is not anchored",
		" curator-run: usage: leading space is not the contract",
		"curator-run: environment_home_stale: unreachable codes are not launcher codes",
		"curator-run: internal_error: invented codes are not launcher codes",
		"curator-run: usage",
		"curator-run: usage: multi\nline detail is not one line",
		"curator-run:: empty code",
	} {
		if diagnostics.IsDiagnosticLine(line) {
			t.Errorf("IsDiagnosticLine(%q) = true, want false", line)
		}
	}
}

// TestRemainingObligationsAreStated bounds the contract: families without
// a producer in this build are declared, not silently dropped.
func TestRemainingObligationsAreStated(t *testing.T) {
	joined := strings.Join(diagnostics.RemainingObligations(), "\n")
	for _, need := range []string{"axconfig.Load", "defaults_unresolvable", "plan_refused", "plan_provider_limited", "CheckLaunchBoundary", "PrepareLaunch"} {
		if !strings.Contains(joined, need) {
			t.Errorf("obligations missing %q:\n%s", need, joined)
		}
	}
	if len(diagnostics.RemainingObligations()) == 0 {
		t.Fatal("no obligations stated")
	}
}
