// Gate: single exhaustive owner-by-form registry for diagnostics CodeOf.
// Adopted from TASK-260909-3d1589 (accepted rev3): board resource
// TASK-260909-3d1589_rev2_gate-conformance_test.go, committed here under
// its adopted name per TASK-260909-3d1589_rev3_adoption.md. Test bodies
// are unchanged from the accepted gate; only this header is new.
// The gate was verified against candidate tree
// fbe90d5e60593a3a069721b2ad9e53cd071d8c02 (base 3ff66a9).
//
// ONE owner registry (gateOwnerRegistry) crossed with ONE form registry
// (gateFormRegistry: direct/wrapped/joined) derives every positive and nil
// expectation below. All five CodeOf-recognized owners are rows:
// ResolveError, LayerError and Refusal carry a mutable per-value Code;
// UsageError carries a constant code ("usage" via Code()) and
// axconfig.Error always maps to defaults_config_invalid. Both fixed-code
// owners are pinned as direct/wrapped/joined positives with named
// positive-form narrowing mutants in the runner: the rev2 reviewer showed a
// joined-UsageError rejection surviving the whole published gate, so every
// fixed owner/form combination is asserted here and counted in
// TestGateCoverageCounts. Claimed combinations cannot be silently omitted:
// the coverage test fails when the registry stops matching the measured
// matrix.
//
// Vectors are additionally derived at test runtime from SPEC section 6 and
// the owner constants, never hand-enumerated. Each mutable owner gate has
// its own named test so a single-foreign-code weakening fails exactly one
// name. The remaining normative codes (defaults_unresolvable,
// plan_refused, plan_provider_limited, env_unsupported, exec_provider_missing,
// ax_handoff_failed) are call-site-selected with no CodeOf classification in
// this tree; they appear here only as rejected foreign values and as
// Codes() members, with production call sites pinned in vectors.
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

// gateOwnerEntry is one row of the single owner registry: every
// CodeOf-recognized owner, its kind, its owned codes, the owner-side
// constant or method pinning each code, the production call site choosing
// the code, how to build an owned error for a listed code, and a typed-nil
// value of the owner's error type. kind is "mutable" (per-value Code field
// the gate can weaken with a foreign admission) or "fixed" (constant
// mapping; only positive-form narrowings apply).
type gateOwnerEntry struct {
	name     string
	kind     string
	owned    []string
	consts   []string
	callSite string
	makeErr  func(code string) error
	nilErr   error
}

// gateOwnerRegistry is the complete owner table. Mutable families reuse the
// owner constants so the owner stays the single source of truth; fixed
// owners pin their constant mapping (UsageError.Code(), axconfig.CodeInvalid).
var gateOwnerRegistry = []gateOwnerEntry{
	{
		name: "usage", kind: "fixed",
		owned:    []string{diagnostics.CodeUsage},
		consts:   []string{"cli.UsageError.Code()"},
		callSite: "cli.Parse (exit 2)",
		makeErr:  func(string) error { return &cli.UsageError{Detail: "x"} },
		nilErr:   (*cli.UsageError)(nil),
	},
	{
		name: "resolve", kind: "mutable",
		owned: gateResolveCodes,
		consts: []string{
			"fragment.CodeInvocationFailed",
			"fragment.CodeEnvironmentUnknown",
			"fragment.CodeProfileUnknown",
			"fragment.CodeRepairFailed",
			"fragment.CodeLockUnavailable",
			"fragment.CodeFragmentInvalid",
		},
		callSite: "fragment.Resolver.Resolve",
		makeErr:  func(c string) error { return &fragment.ResolveError{Code: c, Detail: "x"} },
		nilErr:   (*fragment.ResolveError)(nil),
	},
	{
		name: "layer", kind: "mutable",
		owned: gateLayerCodes,
		consts: []string{
			"composition.CodeMCPLayerMissing",
			"composition.CodeMCPLayerUnreadable",
		},
		callSite: "composition.Value.CheckLaunchBoundary",
		makeErr:  func(c string) error { return &composition.LayerError{Code: c, Path: "p"} },
		nilErr:   (*composition.LayerError)(nil),
	},
	{
		name: "refusal", kind: "mutable",
		owned: gateRefusalCodes,
		consts: []string{
			"systemprompt.CodeUnavailable",
			"systemprompt.CodeUnreadable",
		},
		callSite: "systemprompt.Select via PrepareLaunch / systemprompt.ProbeFiles via PrepareLaunch",
		makeErr:  func(c string) error { return &sp.Refusal{Code: c, Path: "p"} },
		nilErr:   (*sp.Refusal)(nil),
	},
	{
		name: "axconfig", kind: "fixed",
		owned:    []string{diagnostics.CodeDefaultsInvalid},
		consts:   []string{"axconfig.CodeInvalid"},
		callSite: "axconfig.Load (exit 1)",
		makeErr:  func(string) error { return &axconfig.Error{Path: "p", Err: errors.New("cause")} },
		nilErr:   (*axconfig.Error)(nil),
	},
}

// gateFormRegistry is the complete wrap-form table: the exact error value,
// one %w wrap layer, and an errors.Join chain holding the value beside an
// outer error.
var gateFormRegistry = []string{"direct", "wrapped", "joined"}

// gateFormErr renders one registry form around err.
func gateFormErr(form string, err error) error {
	switch form {
	case "direct":
		return err
	case "wrapped":
		return fmt.Errorf("outer: %w", err)
	case "joined":
		return errors.Join(errors.New("outer"), err)
	default:
		panic("unknown gate form " + form)
	}
}

// gateRegistryByName resolves one owner row by name; unknown names fail the
// calling test instead of silently narrowing the matrix.
func gateRegistryByName(t *testing.T, name string) gateOwnerEntry {
	t.Helper()
	for _, owner := range gateOwnerRegistry {
		if owner.name == name {
			return owner
		}
	}
	t.Fatalf("owner %q not in gateOwnerRegistry", name)
	return gateOwnerEntry{}
}

var gateSpecCodeToken = regexp.MustCompile("^[a-z][a-z0-9_]*$")

func gateSpecPath(t *testing.T) string {
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

func gateSpecSection6Codes(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(gateSpecPath(t))
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
			t.Fatalf("malformed SPEC section 6 table row: %q", l)
		}
		if strings.Contains(fields[1], "Family") || strings.Contains(fields[1], "---") {
			continue
		}
		for _, tok := range strings.Split(fields[2], "`") {
			tok = strings.TrimSpace(tok)
			if tok == "" || !gateSpecCodeToken.MatchString(tok) {
				continue
			}
			codes = append(codes, tok)
		}
	}
	if len(codes) == 0 {
		t.Fatal("derived no codes from the SPEC section 6 table")
	}
	return codes
}

// gateResolveCodes, gateLayerCodes and gateRefusalCodes are the mutable
// families transcribed from the owner constants. The registry rows below
// reuse these same slices, so the owner constants stay the single source
// of truth for both the registry and the foreign-rejection matrix.
var gateResolveCodes = []string{
	fragment.CodeInvocationFailed,
	fragment.CodeEnvironmentUnknown,
	fragment.CodeProfileUnknown,
	fragment.CodeRepairFailed,
	fragment.CodeLockUnavailable,
	fragment.CodeFragmentInvalid,
}

var gateLayerCodes = []string{
	composition.CodeMCPLayerMissing,
	composition.CodeMCPLayerUnreadable,
}

var gateRefusalCodes = []string{
	sp.CodeUnavailable,
	sp.CodeUnreadable,
}

func gateResolveFamily() []string {
	return slices.Clone(gateResolveCodes)
}

func gateLayerFamily() []string {
	return slices.Clone(gateLayerCodes)
}

func gateRefusalFamily() []string {
	return slices.Clone(gateRefusalCodes)
}

func gateExtraStrangers() []string {
	return []string{
		"environment_home_stale",
		"environment_unknown",
		"invented_code",
		"",
	}
}

func gateForeign(normative, own []string) []string {
	var out []string
	for _, c := range normative {
		if !slices.Contains(own, c) {
			out = append(out, c)
		}
	}
	for _, c := range gateExtraStrangers() {
		if !slices.Contains(own, c) && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// gateForms builds the three registry wrap forms for one owner error. The
// owned error is constructed fresh per form so no shared state leaks across
// cases.
func gateForms(makeErr func(string) error, code string) map[string]error {
	out := make(map[string]error, len(gateFormRegistry))
	for _, form := range gateFormRegistry {
		out[form] = gateFormErr(form, makeErr(code))
	}
	return out
}

func gateCheckRejects(t *testing.T, owner string, makeErr func(string) error, own []string) {
	t.Helper()
	normative := gateSpecSection6Codes(t)
	if got := diagnostics.Codes(); !slices.Equal(got, normative) {
		t.Fatalf("%s: Codes() = %q, SPEC section 6 = %q", owner, got, normative)
	}
	for _, want := range own {
		if !slices.Contains(normative, want) {
			t.Fatalf("%s: owned code %q not in SPEC section 6 table", owner, want)
		}
	}
	foreign := gateForeign(normative, own)
	if len(foreign) == 0 {
		t.Fatalf("%s: no foreign codes derived", owner)
	}
	for _, code := range foreign {
		for name, cand := range gateForms(makeErr, code) {
			if got, ok := diagnostics.CodeOf(cand); ok || got != "" {
				t.Errorf("%s accepted foreign %q as %q (%s form)", owner, code, got, name)
			}
		}
	}
	// Owned positives must hold in every form: direct, %w-wrapped, and
	// errors.Join chains. A gate that weakens joined acceptance (the rev1
	// surviving mutant) fails here as well as in TestGateJoinedOwnedAccepted.
	for _, code := range own {
		for name, cand := range gateForms(makeErr, code) {
			if got, ok := diagnostics.CodeOf(cand); !ok || got != code {
				t.Errorf("%s own code %q lost in %s form: CodeOf = %q %v, want acceptance", owner, code, name, got, ok)
			}
		}
	}
}

// TestGateResolveRejectsForeignCodes gates the resolve owner: every
// normative non-resolve code plus unknown/empty must yield no code in all
// three wrap forms; every resolve-family member stays accepted in all forms.
// Constructor and owned set come from the single owner registry row.
func TestGateResolveRejectsForeignCodes(t *testing.T) {
	owner := gateRegistryByName(t, "resolve")
	gateCheckRejects(t, "resolve", owner.makeErr, owner.owned)
}

// TestGateLayerRejectsForeignCodes gates the layer owner, including the
// five resolve codes the prior hand-selected matrix omitted.
func TestGateLayerRejectsForeignCodes(t *testing.T) {
	owner := gateRegistryByName(t, "layer")
	gateCheckRejects(t, "layer", owner.makeErr, owner.owned)
}

// TestGateRefusalRejectsForeignCodes gates the refusal owner on the same
// complete derived set.
func TestGateRefusalRejectsForeignCodes(t *testing.T) {
	owner := gateRegistryByName(t, "refusal")
	gateCheckRejects(t, "refusal", owner.makeErr, owner.owned)
}

func gateMakeOwnerErr(t *testing.T, owner, code string) error {
	t.Helper()
	return gateRegistryByName(t, owner).makeErr(code)
}

func gateOwnedByName(owner string) []string {
	switch owner {
	case "resolve":
		return gateResolveFamily()
	case "layer":
		return gateLayerFamily()
	case "refusal":
		return gateRefusalFamily()
	}
	return nil
}

// TestGateJoinedOwnedAccepted pins joined acceptance for every owned code of
// every mutable-Code owner. It is the dedicated regression for the rev1
// surviving mutant that rejected a joined chain holding a direct
// ResolveError with resolve_invocation_failed while keeping direct/wrapped
// acceptance: that mutant exits 1 here with the named loss below.
func TestGateJoinedOwnedAccepted(t *testing.T) {
	for _, owner := range []string{"resolve", "layer", "refusal"} {
		for _, code := range gateOwnedByName(owner) {
			joined := errors.Join(errors.New("outer"), gateMakeOwnerErr(t, owner, code))
			if got, ok := diagnostics.CodeOf(joined); !ok || got != code {
				t.Errorf("joined owned %s %q lost: CodeOf = %q %v, want acceptance", owner, code, got, ok)
			}
			// Joined acceptance must also survive a typed-nil sibling in the
			// same chain: the nil branch fails closed, the owned branch wins.
			var reNil *fragment.ResolveError
			mixed := errors.Join(reNil, gateMakeOwnerErr(t, owner, code))
			if got, ok := diagnostics.CodeOf(mixed); !ok || got != code {
				t.Errorf("joined owned %s %q lost beside typed nil: CodeOf = %q %v", owner, code, got, ok)
			}
		}
	}
}

// TestGateOwnerFormPositives derives one positive case per owner/code/form
// cell of the single registry: 12 owned slots (resolve 6, layer 2,
// refusal 2, usage 1, axconfig 1) x 3 forms = 36 assertions. A mutant that
// drops any single cell -- including the rev2 surviving joined-UsageError
// rejection and its axconfig/wrapped siblings -- fails here with a named
// "<owner> <form> lost" assertion the runner requires by name.
func TestGateOwnerFormPositives(t *testing.T) {
	normative := gateSpecSection6Codes(t)
	if got := diagnostics.Codes(); !slices.Equal(got, normative) {
		t.Fatalf("Codes() = %q, SPEC section 6 = %q", got, normative)
	}
	for _, owner := range gateOwnerRegistry {
		for _, code := range owner.owned {
			if !slices.Contains(normative, code) {
				t.Errorf("%s: owned code %q not in SPEC section 6 table", owner.name, code)
			}
		}
		for _, code := range owner.owned {
			for _, form := range gateFormRegistry {
				cand := gateFormErr(form, owner.makeErr(code))
				if got, ok := diagnostics.CodeOf(cand); !ok || got != code {
					t.Errorf("%s %s lost: %q %v (owned code %q)", owner.name, form, got, ok, code)
				}
			}
		}
	}
}

// TestGateOwnerFormNil derives one nil rejection per owner/form cell of the
// single registry: each owner's typed nil must fail closed direct, %w-wrapped,
// and joined in both sibling orders. The all-typed-nil join, a nil
// interface and a plain error complete the absence matrix.
func TestGateOwnerFormNil(t *testing.T) {
	for _, owner := range gateOwnerRegistry {
		nils := map[string]error{
			"direct":             owner.nilErr,
			"wrapped":            fmt.Errorf("outer: %w", owner.nilErr),
			"joined-outer-first": errors.Join(errors.New("outer"), owner.nilErr),
			"joined-nil-first":   errors.Join(owner.nilErr, errors.New("outer")),
		}
		for form, cand := range nils {
			if got, ok := diagnostics.CodeOf(cand); ok || got != "" {
				t.Errorf("%s nil %s: CodeOf = %q %v, want no code", owner.name, form, got, ok)
			}
		}
	}
	var all []error
	for _, owner := range gateOwnerRegistry {
		all = append(all, owner.nilErr)
	}
	if got, ok := diagnostics.CodeOf(errors.Join(all...)); ok || got != "" {
		t.Errorf("all-typed-nil join: CodeOf = %q %v, want no code", got, ok)
	}
	if got, ok := diagnostics.CodeOf(nil); ok || got != "" {
		t.Errorf("nil interface: CodeOf = %q %v, want no code", got, ok)
	}
	if got, ok := diagnostics.CodeOf(errors.New("boom")); ok || got != "" {
		t.Errorf("plain error: CodeOf = %q %v, want no code", got, ok)
	}
}

// TestGateCoverageCounts pins the complete coverage arithmetic derived from
// the single registry: 5 owners, 3 forms, 18 normative codes; mutable-Code
// families resolve 6 / layer 2 / refusal 2 with foreign sets 12 / 16 / 16
// (44 normative rejection pairs); 4 extra strangers; 56 codes x 3 forms =
// 168 rejection cases; 12 owned slots (10 mutable + 2 fixed) x 3 forms =
// 36 positive cases (30 mutable + 6 fixed); 5 owners x 4 nil forms + 3
// absence extras = 23 nil cases. Fixed-code owners (usage, axconfig) carry
// no mutable-Code gate by construction; their constant mappings are pinned
// here and their positives asserted in TestGateOwnerFormPositives.
// Call-site-selected codes carry no CodeOf classification; they are asserted
// as Codes() members and rejected foreign values elsewhere here.
func TestGateCoverageCounts(t *testing.T) {
	normative := gateSpecSection6Codes(t)
	if len(normative) != 18 {
		t.Fatalf("normative codes = %d, want 18: %q", len(normative), normative)
	}
	if len(gateOwnerRegistry) != 5 {
		t.Fatalf("owner registry rows = %d, want 5 (usage, resolve, layer, refusal, axconfig)", len(gateOwnerRegistry))
	}
	if !slices.Equal([]string{
		gateOwnerRegistry[0].name,
		gateOwnerRegistry[1].name,
		gateOwnerRegistry[2].name,
		gateOwnerRegistry[3].name,
		gateOwnerRegistry[4].name,
	}, []string{"usage", "resolve", "layer", "refusal", "axconfig"}) {
		t.Fatalf("owner registry names = %q, want [usage resolve layer refusal axconfig]",
			[]string{
				gateOwnerRegistry[0].name,
				gateOwnerRegistry[1].name,
				gateOwnerRegistry[2].name,
				gateOwnerRegistry[3].name,
				gateOwnerRegistry[4].name,
			})
	}
	if !slices.Equal(gateFormRegistry, []string{"direct", "wrapped", "joined"}) {
		t.Fatalf("form registry = %q, want [direct wrapped joined]", gateFormRegistry)
	}
	mutableSlots, fixedSlots := 0, 0
	for _, owner := range gateOwnerRegistry {
		switch owner.kind {
		case "mutable":
			mutableSlots += len(owner.owned)
		case "fixed":
			fixedSlots += len(owner.owned)
		default:
			t.Fatalf("%s: unknown owner kind %q, want mutable or fixed", owner.name, owner.kind)
		}
		if len(owner.owned) == 0 {
			t.Fatalf("%s: no owned codes in registry", owner.name)
		}
		if len(owner.consts) != len(owner.owned) {
			t.Fatalf("%s: %d const pins for %d owned codes", owner.name, len(owner.consts), len(owner.owned))
		}
		if owner.makeErr == nil || owner.nilErr == nil {
			t.Fatalf("%s: registry row missing constructor or typed nil", owner.name)
		}
		for _, code := range owner.owned {
			if !slices.Contains(normative, code) {
				t.Fatalf("%s: owned code %q not in SPEC section 6 table", owner.name, code)
			}
		}
	}
	if mutableSlots != 10 {
		t.Fatalf("mutable owned slots = %d, want 10 (resolve 6 + layer 2 + refusal 2)", mutableSlots)
	}
	if fixedSlots != 2 {
		t.Fatalf("fixed owned slots = %d, want 2 (usage 1 + axconfig 1)", fixedSlots)
	}
	positives := (mutableSlots + fixedSlots) * len(gateFormRegistry)
	if positives != 36 {
		t.Fatalf("positive owner/form cases = %d, want 36 (12 slots x 3 forms; 30 mutable + 6 fixed)", positives)
	}
	nils := len(gateOwnerRegistry)*4 + 3
	if nils != 23 {
		t.Fatalf("nil owner/form cases = %d, want 23 (5 owners x 4 forms + 3 absence extras)", nils)
	}
	// Normative code mapping: fixed owners pin their constant codes, and the
	// mutable families transcribe the owner constants (shared slices above).
	if diagnostics.CodeUsage != "usage" {
		t.Fatalf("diagnostics.CodeUsage = %q, want %q", diagnostics.CodeUsage, "usage")
	}
	if diagnostics.CodeDefaultsInvalid != axconfig.CodeInvalid {
		t.Fatalf("diagnostics.CodeDefaultsInvalid = %q, axconfig.CodeInvalid = %q",
			diagnostics.CodeDefaultsInvalid, axconfig.CodeInvalid)
	}
	if diagnostics.CodeDefaultsInvalid != "defaults_config_invalid" {
		t.Fatalf("defaults_config_invalid literal = %q", diagnostics.CodeDefaultsInvalid)
	}
	type fam struct {
		name      string
		own, want int
	}
	for _, f := range []fam{
		{"resolve", len(gateResolveFamily()), 6},
		{"layer", len(gateLayerFamily()), 2},
		{"refusal", len(gateRefusalFamily()), 2},
	} {
		if f.own != f.want {
			t.Fatalf("%s owned = %d, want %d", f.name, f.own, f.want)
		}
		foreign := gateForeign(normative, gateOwnedByName(f.name))
		wantForeign := 18 - f.want + len(gateExtraStrangers())
		if len(foreign) != wantForeign {
			t.Fatalf("%s foreign+extras = %d, want %d: %q", f.name, len(foreign), wantForeign, foreign)
		}
	}
	normPairs := 12 + 16 + 16
	if normPairs != 44 {
		t.Fatalf("normative rejection pairs = %d, want 44", normPairs)
	}
	withExtras := (12 + 4) + (16 + 4) + (16 + 4)
	if withExtras*3 != 168 {
		t.Fatalf("rejection cases with extras x forms = %d, want 168", withExtras*3)
	}
}

// TestGateOwnFamilyAndNilPreserved pins the acceptance side the rejection
// matrix must not break: production-typed values (direct, wrapped, joined),
// fixed-code owners in all three forms, typed-nil fails closed without panic
// in every form including errors.Join chains, and unclaimed errors invent
// nothing.
func TestGateOwnFamilyAndNilPreserved(t *testing.T) {
	if code, ok := diagnostics.CodeOf(&cli.UsageError{Detail: "missing <env-id>"}); !ok || code != "usage" {
		t.Fatalf("usage error: %q %v", code, ok)
	}
	re := &fragment.ResolveError{Code: fragment.CodeRepairFailed, Detail: "x"}
	if code, ok := diagnostics.CodeOf(re); !ok || code != "resolve_repair_failed" {
		t.Fatalf("resolve error: %q %v", code, ok)
	}
	// Every production-typed positive holds direct, %w-wrapped, and joined,
	// including both fixed-code owners in every form (the rev2 gap: only
	// usage-direct was pinned here before).
	joinedPositives := map[string]struct {
		err  error
		want string
	}{
		"usage-direct":    {&cli.UsageError{Detail: "x"}, "usage"},
		"usage-wrapped":   {fmt.Errorf("outer: %w", &cli.UsageError{Detail: "x"}), "usage"},
		"usage-joined":    {errors.Join(errors.New("outer"), &cli.UsageError{Detail: "x"}), "usage"},
		"axconfig-direct": {&axconfig.Error{Path: "p", Err: errors.New("cause")}, "defaults_config_invalid"},
		"axconfig-wrapped": {
			fmt.Errorf("outer: %w", &axconfig.Error{Path: "p", Err: errors.New("cause")}),
			"defaults_config_invalid",
		},
		"axconfig-joined": {
			errors.Join(errors.New("outer"), &axconfig.Error{Path: "p", Err: errors.New("cause")}),
			"defaults_config_invalid",
		},
		"resolve-joined":  {errors.Join(errors.New("outer"), &fragment.ResolveError{Code: fragment.CodeInvocationFailed, Detail: "x"}), "resolve_invocation_failed"},
		"resolve-wrapped": {fmt.Errorf("outer: %w", &fragment.ResolveError{Code: fragment.CodeFragmentInvalid, Detail: "x"}), "resolve_fragment_invalid"},
	}
	for name, tc := range joinedPositives {
		if code, ok := diagnostics.CodeOf(tc.err); !ok || code != tc.want {
			t.Errorf("%s: CodeOf = %q %v, want %q", name, code, ok, tc.want)
		}
	}
	missing, unreadable := codexBoundary(t)
	if code, ok := diagnostics.CodeOf(missing); !ok || code != "mcp_layer_missing" {
		t.Fatalf("mcp missing: %q %v (%v)", code, ok, missing)
	}
	if code, ok := diagnostics.CodeOf(unreadable); !ok || code != "mcp_layer_unreadable" {
		t.Fatalf("mcp unreadable: %q %v (%v)", code, ok, unreadable)
	}
	if joined := errors.Join(errors.New("outer"), missing); true {
		if code, ok := diagnostics.CodeOf(joined); !ok || code != "mcp_layer_missing" {
			t.Errorf("mcp missing joined: CodeOf = %q %v", code, ok)
		}
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
	var ue *cli.UsageError
	var reNil *fragment.ResolveError
	var leNil *composition.LayerError
	var srNil *sp.Refusal
	var aeNil *axconfig.Error
	for name, err := range map[string]error{
		"usage": ue, "resolve": reNil, "layer": leNil, "refusal": srNil, "ax": aeNil,
		"nil": nil, "plain": errors.New("boom"),
	} {
		if got, ok := diagnostics.CodeOf(err); ok || got != "" {
			t.Errorf("%s: CodeOf = %q %v, want no code", name, got, ok)
		}
		wrapped := fmt.Errorf("outer: %w", err)
		if got, ok := diagnostics.CodeOf(wrapped); ok || got != "" {
			t.Errorf("%s wrapped: CodeOf = %q %v, want no code", name, got, ok)
		}
		// Joined typed-nil behavior: a typed nil beside an outer error, a
		// typed nil leading the chain, and an all-typed-nil chain all fail
		// closed with no code and no panic.
		for joinName, joined := range map[string]error{
			"outer-first": errors.Join(errors.New("outer"), err),
			"nil-first":   errors.Join(err, errors.New("outer")),
		} {
			if err == nil && joinName == "nil-first" {
				// errors.Join(nil, outer) == Join(outer): plain outer error
				// still yields no code; assertions below hold either way.
				_ = joined
			}
			if got, ok := diagnostics.CodeOf(joined); ok || got != "" {
				t.Errorf("%s joined %s: CodeOf = %q %v, want no code", name, joinName, got, ok)
			}
		}
	}
	if got, ok := diagnostics.CodeOf(errors.Join(reNil, leNil, srNil)); ok || got != "" {
		t.Errorf("all-typed-nil join: CodeOf = %q %v, want no code", got, ok)
	}
}
