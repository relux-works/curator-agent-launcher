package defaults_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/skill-agents-management/pkg/agentic"
	claudeSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/claude"
	codexSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/codex"
	pinativeSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/pinative"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin/vendors/anthropic"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin/vendors/openai"

	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
)

func mustRegistry(t *testing.T) *vendorplugin.Registry {
	t.Helper()
	reg, err := defaults.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

// errFallback is the injected resolution failure for the provider-path
// fallback table: any error must yield the fallback, never propagate.
var errFallback = errors.New("injected provider-path failure")

func lineupOrigin(t *testing.T, got defaults.ResolvedMember, value string) {
	t.Helper()
	if !got.Present || got.Value != value || got.Origin != defaults.OriginLineup {
		t.Fatalf("member = %+v, want value %q with lineup origin", got, value)
	}
}

// TestNewRegistryResolvesLaunchableSystems builds the production registry
// from the real tagged module: all five launchable runtimes resolve with
// their system and vendor, each driving at least one model row, while the
// legacy pi id and the pi-native system id (neither is a runtime id)
// resolve to nothing, so no Pi launch can slip through the wrapper.
func TestNewRegistryResolvesLaunchableSystems(t *testing.T) {
	reg := mustRegistry(t)
	for runtime, system := range map[string]string{
		"claude": "claude-code", "codex": "codex",
		"pi-anthropic": "pi-native", "pi-openai": "pi-native", "pi-google": "pi-native",
	} {
		resolved, err := reg.ResolveRuntime(vendorplugin.RuntimeID(runtime))
		if err != nil {
			t.Fatalf("ResolveRuntime(%q): %v", runtime, err)
		}
		if string(resolved.SystemID) != system || resolved.Vendor == nil {
			t.Fatalf("ResolveRuntime(%q) = system %q vendor %v", runtime, resolved.SystemID, resolved.Vendor)
		}
		if len(vendorplugin.RuntimeModels(resolved)) == 0 {
			t.Fatalf("ResolveRuntime(%q) drives no model for its system", runtime)
		}
	}
	for _, runtime := range []string{"pi", "pi-native"} {
		if _, err := reg.ResolveRuntime(vendorplugin.RuntimeID(runtime)); err == nil {
			t.Fatalf("ResolveRuntime(%q) succeeded: %q is not a declared runtime", runtime, runtime)
		}
	}
}

// TestCompleteLineupTops pins the observed v0.5.22 fallback values. The
// model is the Lineup leader by capability score, not the vendor's
// display Recommended row (claude-opus-5 for anthropic); re-verify these
// pins when the module pin changes. Pi ranks only the preferred runtime
// and binds that contributor.
func TestCompleteLineupTops(t *testing.T) {
	reg := mustRegistry(t)
	for env, want := range map[string][3]string{
		"claude_code": {"claude-opus-5-5", "high", "claude"},
		"codex_cli":   {"gpt-6-astra", "max", "codex"},
		"pi":          {"claude-fable-5", "high", "pi-anthropic"},
	} {
		t.Run(env, func(t *testing.T) {
			files, err := defaults.Load(paths(t))
			if err != nil {
				t.Fatal(err)
			}
			got, err := files.Complete(env, defaults.Pair{}, reg)
			if err != nil {
				t.Fatal(err)
			}
			lineupOrigin(t, got.Model, want[0])
			lineupOrigin(t, got.Effort, want[1])
			if got.Runtime != want[2] {
				t.Fatalf("runtime = %q, want %q", got.Runtime, want[2])
			}
		})
	}
}

// TestCompletePerMemberFill proves each level supplies only the member
// every earlier level left unset: a configured model takes the lineup's
// effort for THAT model (claude-opus-4-8 recommends xhigh, not the top
// row's high), a configured effort keeps its value unvalidated, and a
// full pair records the runtime without consulting the lineup.
func TestCompletePerMemberFill(t *testing.T) {
	reg := mustRegistry(t)
	t.Run("model-takes-own-row-effort", func(t *testing.T) {
		p := paths(t)
		write(t, p.Machine, doc(`{"claude_code":{"model":"claude-opus-4-8"}}`, false))
		files, err := defaults.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := files.Complete("claude_code", defaults.Pair{}, reg)
		if err != nil {
			t.Fatal(err)
		}
		if got.Model != resolved("claude-opus-4-8", defaults.OriginMachine) {
			t.Fatalf("model = %+v", got.Model)
		}
		lineupOrigin(t, got.Effort, "xhigh")
	})
	t.Run("effort-kept-unvalidated", func(t *testing.T) {
		files, err := defaults.Load(paths(t))
		if err != nil {
			t.Fatal(err)
		}
		got, err := files.Complete("codex_cli", defaults.Pair{Effort: member("not-a-real-effort")}, reg)
		if err != nil {
			t.Fatal(err)
		}
		lineupOrigin(t, got.Model, "gpt-6-astra")
		if got.Effort != resolved("not-a-real-effort", defaults.OriginFlag) {
			t.Fatalf("effort = %+v, want the explicit flag value untouched", got.Effort)
		}
	})
	t.Run("full-pair-records-runtime", func(t *testing.T) {
		p := paths(t)
		write(t, p.Operator, doc(`{"codex_cli":{"model":"gpt-6-astra","effort":"low"}}`, false))
		files, err := defaults.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := files.Complete("codex_cli", defaults.Pair{}, reg)
		if err != nil {
			t.Fatal(err)
		}
		if got.Model != resolved("gpt-6-astra", defaults.OriginOperator) || got.Effort != resolved("low", defaults.OriginOperator) {
			t.Fatalf("pair = %+v %+v, want the explicit values even past the row recommendation", got.Model, got.Effort)
		}
		if got.Runtime != "codex" {
			t.Fatalf("runtime = %q, want codex", got.Runtime)
		}
	})
	t.Run("pi-model-binds-contributor-runtime", func(t *testing.T) {
		files, err := defaults.Load(paths(t))
		if err != nil {
			t.Fatal(err)
		}
		got, err := files.Complete("pi", defaults.Pair{Model: member("claude-opus-5")}, reg)
		if err != nil {
			t.Fatal(err)
		}
		if got.Model != resolved("claude-opus-5", defaults.OriginFlag) {
			t.Fatalf("model = %+v", got.Model)
		}
		lineupOrigin(t, got.Effort, "high")
		if got.Runtime != "pi-anthropic" {
			t.Fatalf("runtime = %q, want the contributing pi runtime", got.Runtime)
		}
	})
}

// TestCompleteNoEffortSemantics proves a model with no effort axis gets
// no effort — never an empty or invented one — and an unknown configured
// model leaves the effort unset for the spawn plane to refuse.
func TestCompleteNoEffortSemantics(t *testing.T) {
	reg := mustRegistry(t)
	t.Run("none-axis", func(t *testing.T) {
		files, err := defaults.Load(paths(t))
		if err != nil {
			t.Fatal(err)
		}
		got, err := files.Complete("claude_code", defaults.Pair{Model: member("claude-haiku-4-5")}, reg)
		if err != nil {
			t.Fatal(err)
		}
		if got.Model != resolved("claude-haiku-4-5", defaults.OriginFlag) {
			t.Fatalf("model = %+v", got.Model)
		}
		if got.Effort != (defaults.ResolvedMember{}) {
			t.Fatalf("effort = %+v, want unset for an effortless model", got.Effort)
		}
	})
	t.Run("unknown-model", func(t *testing.T) {
		files, err := defaults.Load(paths(t))
		if err != nil {
			t.Fatal(err)
		}
		got, err := files.Complete("claude_code", defaults.Pair{Model: member("nope-not-a-model")}, reg)
		if err != nil {
			t.Fatal(err)
		}
		if got.Model != resolved("nope-not-a-model", defaults.OriginFlag) {
			t.Fatalf("model = %+v", got.Model)
		}
		if got.Effort != (defaults.ResolvedMember{}) {
			t.Fatalf("effort = %+v, want unset so admission refuses the unknown model", got.Effort)
		}
	})
	t.Run("pi-effortless-row", func(t *testing.T) {
		files, err := defaults.Load(paths(t))
		if err != nil {
			t.Fatal(err)
		}
		got, err := files.Complete("pi", defaults.Pair{Model: member("gemini-3.1-pro-preview")}, reg)
		if err != nil {
			t.Fatal(err)
		}
		if got.Model != resolved("gemini-3.1-pro-preview", defaults.OriginFlag) {
			t.Fatalf("model = %+v", got.Model)
		}
		if got.Effort != (defaults.ResolvedMember{}) {
			t.Fatalf("effort = %+v, want unset for the effortless google row", got.Effort)
		}
		if got.Runtime != "pi-google" {
			t.Fatalf("runtime = %q, want the contributing pi runtime", got.Runtime)
		}
	})
}

// TestCompleteUnresolvable proves a system with no declared runtime fails
// with the typed diagnostic and no invented model, while a fully
// file-supplied pi pair with an unknown id still resolves (with no
// runtime for the plan stage to refuse).
func TestCompleteUnresolvable(t *testing.T) {
	reg := mustRegistry(t)
	t.Run("no-declared-runtime", func(t *testing.T) {
		bare := vendorplugin.NewRegistry(agentic.NewRegistry())
		files, err := defaults.Load(paths(t))
		if err != nil {
			t.Fatal(err)
		}
		got, err := files.Complete("pi", defaults.Pair{}, bare)
		code(t, err, defaults.CodeUnresolvable)
		if got != (defaults.Resolved{}) {
			t.Fatalf("resolution leaked: %+v", got)
		}
		if !strings.Contains(err.Error(), `"pi-native"`) {
			t.Fatalf("error %q does not name the unadmitted system", err)
		}
	})
	t.Run("file-supplied-pair", func(t *testing.T) {
		p := paths(t)
		write(t, p.Operator, doc(`{"pi":{"model":"m","effort":"e"}}`, false))
		files, err := defaults.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := files.Complete("pi", defaults.Pair{}, reg)
		if err != nil {
			t.Fatal(err)
		}
		if got.Model != resolved("m", defaults.OriginOperator) || got.Effort != resolved("e", defaults.OriginOperator) {
			t.Fatalf("pair = %+v %+v", got.Model, got.Effort)
		}
		if got.Runtime != "" {
			t.Fatalf("runtime = %q, want empty: no driven row claims the unknown id", got.Runtime)
		}
	})
}

// TestCompleteNoFirePastFailure proves the lineup never supplies a member
// past a levels-1-2 refusal: locked flags stay a usage error even with a
// registry that could complete the pair.
func TestCompleteNoFirePastFailure(t *testing.T) {
	reg := mustRegistry(t)
	p := paths(t)
	write(t, p.Machine, doc(`{"pi":{"model":"machine"}}`, true))
	write(t, p.Operator, doc(`{"pi":{"model":"operator","effort":"operator"}}`, false))
	files, err := defaults.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := files.Complete("pi", defaults.Pair{Model: member("flag")}, reg)
	code(t, err, defaults.CodeUsage)
	if got != (defaults.Resolved{}) {
		t.Fatalf("resolution leaked past the lock refusal: %+v", got)
	}
}

// TestCompletePreservesLocks proves unset locked members still accept
// flags and operator entries stay ignored under a lock, through the full
// completion path.
func TestCompletePreservesLocks(t *testing.T) {
	reg := mustRegistry(t)
	p := paths(t)
	write(t, p.Machine, doc(`{"pi":{"model":"machine"}}`, true))
	write(t, p.Operator, doc(`{"pi":{"model":"operator","effort":"operator"}}`, false))
	files, err := defaults.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := files.Complete("pi", defaults.Pair{Effort: member("flag-effort")}, reg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != resolved("machine", defaults.OriginMachine) {
		t.Fatalf("model = %+v", got.Model)
	}
	if got.Effort != resolved("flag-effort", defaults.OriginFlag) {
		t.Fatalf("effort = %+v, want the flag on the unset locked member", got.Effort)
	}
}

// TestCompleteMappingRefusal proves configuration for an environment with
// no launchable mapping never reaches the lineup, even fully supplied.
func TestCompleteMappingRefusal(t *testing.T) {
	reg := mustRegistry(t)
	p := paths(t)
	write(t, p.Operator, doc(`{"opencode":{"model":"m","effort":"e"}}`, false))
	files, err := defaults.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, env := range []string{"opencode", "future"} {
		if _, err := files.Complete(env, defaults.Pair{}, reg); err == nil || !strings.Contains(err.Error(), "no supported system/provider pair") {
			t.Fatalf("%s: err = %v, want the mapping refusal", env, err)
		}
	}
}

// TestCompleteNilRegistry fails closed: no registry is not a source of models.
func TestCompleteNilRegistry(t *testing.T) {
	files, err := defaults.Load(paths(t))
	if err != nil {
		t.Fatal(err)
	}
	got, err := files.Complete("claude_code", defaults.Pair{}, nil)
	code(t, err, defaults.CodeUnresolvable)
	if got != (defaults.Resolved{}) {
		t.Fatalf("resolution leaked: %+v", got)
	}
}

// TestCompleteRuntimeResolutionFailure proves a declared-but-unmaterialized
// runtime admits nothing: the declaration names claude, but no system
// plugin is registered, so ResolveRuntime fails and the cause is refused
// as unresolvable rather than guessed past. A second declaration that
// does materialize does not rescue the first: one bad binding poisons
// the whole system, so skipping past it is refused too.
func TestCompleteRuntimeResolutionFailure(t *testing.T) {
	t.Run("all-unmaterialized", func(t *testing.T) {
		reg := vendorplugin.NewRegistry(agentic.NewRegistry())
		if err := vendorplugin.SeedFrozenRuntimes(reg); err != nil {
			t.Fatal(err)
		}
		files, err := defaults.Load(paths(t))
		if err != nil {
			t.Fatal(err)
		}
		got, err := files.Complete("claude_code", defaults.Pair{}, reg)
		code(t, err, defaults.CodeUnresolvable)
		if got != (defaults.Resolved{}) {
			t.Fatalf("resolution leaked: %+v", got)
		}
	})
	t.Run("partial-failure", func(t *testing.T) {
		systems := agentic.NewRegistry()
		if err := systems.Register(claudeSystem.New()); err != nil {
			t.Fatal(err)
		}
		if err := systems.Register(pinativeSystem.New()); err != nil {
			t.Fatal(err)
		}
		reg := vendorplugin.NewRegistry(systems)
		for _, decl := range []vendorplugin.RuntimeDeclaration{
			{
				ID:     "claude",
				System: "claude-code",
				Vendor: "anthropic",
				Broker: vendorplugin.BrokerProvenance{
					Checked: []string{"test: one good declaration"},
					Found:   "test: the frozen table binds claude to anthropic",
				},
			},
			{
				ID:     "claude-bad",
				System: "claude-code",
				Vendor: "openai",
				Broker: vendorplugin.BrokerProvenance{
					Checked: []string{"test: one unregistered vendor"},
					Found:   "test: no vendor is registered for this check",
				},
			},
		} {
			if err := reg.DeclareRuntime(decl); err != nil {
				t.Fatal(err)
			}
		}
		if err := reg.Register(anthropic.New()); err != nil {
			t.Fatal(err)
		}
		files, err := defaults.Load(paths(t))
		if err != nil {
			t.Fatal(err)
		}
		got, err := files.Complete("claude_code", defaults.Pair{}, reg)
		code(t, err, defaults.CodeUnresolvable)
		if got != (defaults.Resolved{}) {
			t.Fatalf("resolution leaked past the bad binding: %+v", got)
		}
	})
}

// TestCompleteWrongVendorModels proves rows for other systems are not
// candidates: the claude runtime is bound to the openai vendor, whose
// models all drive codex, so the lineup for claude-code is empty and the
// launch fails instead of cross-driving a model.
func TestCompleteWrongVendorModels(t *testing.T) {
	systems := agentic.NewRegistry()
	if err := systems.Register(claudeSystem.New()); err != nil {
		t.Fatal(err)
	}
	if err := systems.Register(codexSystem.New()); err != nil {
		t.Fatal(err)
	}
	if err := systems.Register(pinativeSystem.New()); err != nil {
		t.Fatal(err)
	}
	reg := vendorplugin.NewRegistry(systems)
	err := reg.DeclareRuntime(vendorplugin.RuntimeDeclaration{
		ID:     "claude",
		System: "claude-code",
		Vendor: "openai",
		Broker: vendorplugin.BrokerProvenance{
			Checked: []string{"test: a binding no vendor pair supports"},
			Found:   "test: the frozen table binds claude to anthropic",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(openai.New()); err != nil {
		t.Fatal(err)
	}
	files, err := defaults.Load(paths(t))
	if err != nil {
		t.Fatal(err)
	}
	got, err := files.Complete("claude_code", defaults.Pair{}, reg)
	code(t, err, defaults.CodeUnresolvable)
	if got != (defaults.Resolved{}) {
		t.Fatalf("resolution leaked: %+v", got)
	}
}

// TestCompleteAmbiguousRuntime proves a model driven by two runtimes on
// one system binds none: candidates union across declarations, but the
// pick must come from exactly one contributor, so the launcher refuses
// rather than guessing a vendor. The registry is built from the real
// module with a second declaration on the claude-code system, which puts
// the same rows behind two runtimes.
func TestCompleteAmbiguousRuntime(t *testing.T) {
	systems := agentic.NewRegistry()
	if err := systems.Register(claudeSystem.New()); err != nil {
		t.Fatal(err)
	}
	if err := systems.Register(pinativeSystem.New()); err != nil {
		t.Fatal(err)
	}
	reg := vendorplugin.NewRegistry(systems)
	for _, id := range []vendorplugin.RuntimeID{"claude", "claudesecond"} {
		err := reg.DeclareRuntime(vendorplugin.RuntimeDeclaration{
			ID:     id,
			System: "claude-code",
			Vendor: "anthropic",
			Broker: vendorplugin.BrokerProvenance{
				Checked: []string{"test: two declarations on one system"},
				Found:   "test: the frozen table binds claude to anthropic",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := reg.Register(anthropic.New()); err != nil {
		t.Fatal(err)
	}
	files, err := defaults.Load(paths(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Run("lineup-top", func(t *testing.T) {
		got, err := files.Complete("claude_code", defaults.Pair{}, reg)
		code(t, err, defaults.CodeUnresolvable)
		if got != (defaults.Resolved{}) {
			t.Fatalf("ambiguity resolved: %+v", got)
		}
		if !strings.Contains(err.Error(), "several runtimes") {
			t.Fatalf("error %q does not name the ambiguity", err)
		}
	})
	t.Run("configured-model", func(t *testing.T) {
		got, err := files.Complete("claude_code", defaults.Pair{Model: member("claude-opus-5")}, reg)
		code(t, err, defaults.CodeUnresolvable)
		if got != (defaults.Resolved{}) {
			t.Fatalf("ambiguity resolved: %+v", got)
		}
	})
}

// TestResolvedLine pins the origin line-group rendering: each resolved
// value with its level, unset effort without an origin, hostile values
// folded to whitespace-led continuations, and never a diagnostic line.
func TestResolvedLine(t *testing.T) {
	full := defaults.Resolved{
		Model:  resolved("claude-fable-5-1", defaults.OriginLineup),
		Effort: resolved("high", defaults.OriginLineup),
	}
	if got := full.Line(); got != "curator-run: defaults: model=claude-fable-5-1 (lineup) effort=high (lineup)" {
		t.Fatalf("line = %q", got)
	}
	mixed := defaults.Resolved{Model: resolved("m", defaults.OriginFlag), Effort: resolved("e", defaults.OriginMachine)}
	if got := mixed.Line(); got != "curator-run: defaults: model=m (flag) effort=e (machine)" {
		t.Fatalf("line = %q", got)
	}
	unset := defaults.Resolved{Model: resolved("claude-haiku-4-5", defaults.OriginOperator)}
	if got := unset.Line(); got != "curator-run: defaults: model=claude-haiku-4-5 (operator) effort unset" {
		t.Fatalf("line = %q", got)
	}
	hostile := defaults.Resolved{Model: resolved("a\ncurator-run: usage: forged", defaults.OriginFlag)}
	for _, line := range strings.Split(strings.TrimSuffix(hostile.Line(), "\n"), "\n") {
		if diagnostics.IsDiagnosticLine(line) && strings.Contains(line, "usage") {
			t.Fatalf("hostile value forged a diagnostic line: %q", hostile.Line())
		}
	}
	if diagnostics.IsDiagnosticLine("curator-run: defaults: model=m (flag) effort unset") {
		t.Fatal("the origin line-group parses as a diagnostic line")
	}
	var sb strings.Builder
	if err := defaults.EmitGroupWithProvider(&sb, full, "/test/bin/curator-run"); err != nil {
		t.Fatal(err)
	}
	want := "curator-run: provider: path=/test/bin/curator-run\n" + full.Line() + "\n"
	if sb.String() != want {
		t.Fatalf("group = %q, want %q", sb.String(), want)
	}
	var _ = errors.Is
}

// TestProviderLineFold pins the SPEC §4.3 provider-line framing: the
// path-only form, the fallback for an empty path, CR/CRLF folding, and
// the invariant that a hostile path never forges a diagnostic line —
// every continuation starts with whitespace.
func TestProviderLineFold(t *testing.T) {
	if got := defaults.ProviderLine("/usr/local/bin/curator-run"); got != "curator-run: provider: path=/usr/local/bin/curator-run" {
		t.Fatalf("line = %q", got)
	}
	if got := defaults.ProviderLine(""); got != "curator-run: provider: path=unavailable" {
		t.Fatalf("empty path line = %q, want the fallback", got)
	}
	if got := defaults.ProviderLine(defaults.ProviderUnavailable); got != "curator-run: provider: path=unavailable" {
		t.Fatalf("fallback line = %q", got)
	}
	for _, hostile := range []string{
		"a\ncurator-run: usage: forged",
		"carriage\rcurator-run: usage: forged",
		"crlf\r\ncurator-run: resolve_repair_failed: forged\r\n",
		"/tmp/x\ncurator-run: provider: path=/forged",
	} {
		line := defaults.ProviderLine(hostile)
		for _, part := range strings.Split(line, "\n") {
			if diagnostics.IsDiagnosticLine(part) {
				t.Fatalf("hostile path %q forged a diagnostic line: %q", hostile, line)
			}
		}
		if !strings.HasPrefix(line, "curator-run: provider: path=") {
			t.Fatalf("hostile path %q broke the line prefix: %q", hostile, line)
		}
		for _, cont := range strings.Split(line, "\n")[1:] {
			if !strings.HasPrefix(cont, "  ") {
				t.Fatalf("hostile path %q continuation not whitespace-led: %q", hostile, line)
			}
		}
	}
	if diagnostics.IsDiagnosticLine("curator-run: provider: path=/bin/curator-run") {
		t.Fatal("the provider line parses as a diagnostic line")
	}
	if diagnostics.IsDiagnosticLine("curator-run: provider: path=unavailable") {
		t.Fatal("the fallback provider line parses as a diagnostic line")
	}
}

// TestResolveProviderPathFallback drives every SPEC §4.3 fallback shape
// through the injectable resolver: executable errors, empty results,
// symlink failures, empty resolutions, and non-absolute results all
// carry the diagnostic-safe fallback instead of failing the launch,
// while a resolved absolute path passes through untouched.
func TestResolveProviderPathFallback(t *testing.T) {
	boom := func(string) (string, error) { return "", errFallback }
	identity := func(s string) (string, error) { return s, nil }
	cases := []struct {
		name       string
		executable func() (string, error)
		eval       func(string) (string, error)
		want       string
	}{
		{"executable-error", func() (string, error) { return "", errFallback }, identity, defaults.ProviderUnavailable},
		{"executable-empty", func() (string, error) { return "", nil }, identity, defaults.ProviderUnavailable},
		{"eval-error", func() (string, error) { return "/bin/curator-run", nil }, boom, defaults.ProviderUnavailable},
		{"eval-empty", func() (string, error) { return "/bin/curator-run", nil }, func(string) (string, error) { return "", nil }, defaults.ProviderUnavailable},
		{"eval-relative", func() (string, error) { return "bin/curator-run", nil }, identity, defaults.ProviderUnavailable},
		{"nil-executable", nil, identity, defaults.ProviderUnavailable},
		{"nil-eval", func() (string, error) { return "/bin/curator-run", nil }, nil, defaults.ProviderUnavailable},
		{"resolved-absolute", func() (string, error) { return "/tmp/link", nil }, func(string) (string, error) { return "/real/curator-run", nil }, "/real/curator-run"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := defaults.ResolveProviderPathWith(c.executable, c.eval); got != c.want {
				t.Fatalf("path = %q, want %q", got, c.want)
			}
		})
	}
}

// TestResolveProviderPathProduction proves the production resolver never
// returns an empty path: it is either an absolute path to this test
// binary or the fallback, and the provider line renders either form.
func TestResolveProviderPathProduction(t *testing.T) {
	got := defaults.ResolveProviderPath()
	if got == "" {
		t.Fatal("production resolver returned an empty path")
	}
	if got != defaults.ProviderUnavailable && !filepath.IsAbs(got) {
		t.Fatalf("production resolver returned a non-absolute path %q", got)
	}
	line := defaults.ProviderLine(got)
	if !strings.HasPrefix(line, "curator-run: provider: path=") {
		t.Fatalf("production line = %q", line)
	}
	if diagnostics.IsDiagnosticLine(strings.Split(line, "\n")[0]) {
		t.Fatalf("production line parses as a diagnostic line: %q", line)
	}
}

// This API-level check proves the binding is admitted by the real module.
// It does not claim the launcher entry point calls BuildLaunch yet.
func TestCompletedPairsAdmitTaggedBuildLaunch(t *testing.T) {
	reg := mustRegistry(t)
	bin := t.TempDir()
	for _, name := range []string{"claude", "codex", "pi"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 91\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ env, model, runtime string }{
		{"claude_code", "", "claude"}, {"codex_cli", "", "codex"}, {"pi", "", "pi-anthropic"},
		{"pi", "gpt-5.6-sol", "pi-openai"}, {"pi", "gemini-3.1-pro-preview", "pi-google"},
	} {
		t.Run(tc.env+"/"+tc.runtime, func(t *testing.T) {
			var flags defaults.Pair
			if tc.model != "" {
				flags.Model = member(tc.model)
			}
			got, err := (defaults.Files{}).Complete(tc.env, flags, reg)
			if err != nil {
				t.Fatal(err)
			}
			if got.Runtime != tc.runtime {
				t.Fatalf("runtime=%q want %q", got.Runtime, tc.runtime)
			}
			plan, err := vendorplugin.BuildLaunch(context.Background(), reg, vendorplugin.SpawnRequest{
				Runtime: vendorplugin.RuntimeID(got.Runtime), Model: vendorplugin.ModelID(got.Model.Value), Effort: got.Effort.Value, Home: t.TempDir(), WorkDir: t.TempDir(), Env: []string{"PATH=" + bin},
			}, agentic.LaunchModeInteractive)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Binary == "" {
				t.Fatal("empty plan binary")
			}
		})
	}
}
