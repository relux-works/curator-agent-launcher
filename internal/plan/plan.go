// Package plan implements SPEC §4.4, the spawn-plane step of the launcher:
// an explicitly resolved runtime/model/effort triple is admitted through
// the real tagged vendorplugin.BuildLaunch in agentic.LaunchModeInteractive,
// then gated on a separate explicit providerlimits.Store.AvailabilityFor
// read over the same runtime/model/managed home.
//
// Inputs are explicitly resolved; this package owns no defaults engine
// (defaults are TASK-260909-2vy977). The request it builds carries the
// managed fragment home, the current working directory, and the inherited
// environment in both tracked and untracked modes, with an empty
// Composition, a zero Run, and goal/budget/service-tier/assignment unset.
// The mode is requested by name (agentic.LaunchModeInteractive); no
// permission bypass or yolo is spelled, and plan building starts no child.
//
// Provider-limit admission is a separate read keyed by the exact
// runtime/model/managed home. Only a Serviceable (Healthy) verdict admits
// launch; a determinate no-record Healthy follows the module's fail-open
// contract, while an unknown or failed read never implies healthy. Refusal
// evidence (state/Until/Checked/Observed/Failures and module errors) is
// preserved verbatim; there is no retry, fallback, or model downgrade.
// Required-effort refusals carry the launcher --effort guidance in addition
// to the module's model/vocabulary/recommendation text.
//
// An empty managed home is refused before any module call: letting
// AvailabilityFor fall back to DefaultProviderHome would substitute an
// empty/native home for the managed one, which §4.4 forbids. An empty
// workdir is refused the same way. A blank runtime/model is passed through
// to BuildLaunch so the refusal evidence names the module's verdict.
//
// The native-Pi system (pi-native) is declared by the §4.2 mapping and
// provided by agents-management v0.5.11 (PR23): the pinative system plugin
// serves the pi-anthropic, pi-openai and pi-google runtimes natively on the
// managed home. No pseudo-version, replace, workspace override, or
// synthetic admission is used; the request pins the real tag.
package plan

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/providerlimits"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"

	// Production plugin registrations: the systems and vendors behind the
	// §4.2 runtimes. Each import runs the plugin's own init into the
	// package-level defaults, which is the production registration path.
	// local-models has no init registration (it needs explicit config) and
	// is intentionally absent: nothing here invents a vendor for a runtime
	// whose broker was never established.
	_ "github.com/relux-works/skill-agents-management/pkg/agentic/systems/claude"
	_ "github.com/relux-works/skill-agents-management/pkg/agentic/systems/codex"
	_ "github.com/relux-works/skill-agents-management/pkg/agentic/systems/pinative"
	_ "github.com/relux-works/skill-agents-management/pkg/vendorplugin/vendors/anthropic"
	_ "github.com/relux-works/skill-agents-management/pkg/vendorplugin/vendors/google"
	_ "github.com/relux-works/skill-agents-management/pkg/vendorplugin/vendors/openai"
)

// Stable diagnostic codes of SPEC §6 owned here. Main chooses them at the
// plan call site; diagnostics.CodeOf does not classify them (they are
// call-site-selected, like env_unsupported and the exec/ax sentinels).
const (
	CodeRefused = "plan_refused"
	CodeLimited = "plan_provider_limited"
)

// Request carries the explicitly resolved launch inputs. Runtime matches
// the §4.2 mapping, Model/Effort are the §4.3 resolved pair, Home is the
// managed fragment home, WorkDir is the launcher's current directory, and
// Env is os.Environ() in production in both modes. There is deliberately
// no Composition, Run, goal, budget, service-tier, profile, prompt, or
// engine member: those stay zero/unset by construction and no caller can
// supply them.
type Request struct {
	Runtime string
	Model   string
	Effort  string
	Home    string
	WorkDir string
	Env     []string
}

// BuildLaunchFunc is vendorplugin.BuildLaunch's shape, injectable so tests
// can capture the exact SpawnRequest while still driving the real tagged
// admission. Production passes vendorplugin.BuildLaunch.
type BuildLaunchFunc func(ctx context.Context, r *vendorplugin.Registry, req vendorplugin.SpawnRequest, mode agentic.LaunchMode) (agentic.Plan, error)

// AvailabilityFunc is providerlimits.Store.AvailabilityFor's shape.
// Production passes a real store's method; tests pass fakes or a real
// store over a temp layout.
type AvailabilityFunc func(q providerlimits.VerdictQuery) (vendorplugin.Availability, error)

// Deps wires the module boundaries. Registry is the vendorplugin registry
// (production: vendorplugin.Default, populated by the blank imports above).
type Deps struct {
	Registry     *vendorplugin.Registry
	BuildLaunch  BuildLaunchFunc
	Availability AvailabilityFunc
}

// DefaultDeps wires the real tagged contracts: the default registry, the
// real vendorplugin.BuildLaunch, and the given store's AvailabilityFor.
func DefaultDeps(store *providerlimits.Store) Deps {
	d := Deps{Registry: vendorplugin.Default, BuildLaunch: vendorplugin.BuildLaunch}
	if store != nil {
		d.Availability = store.AvailabilityFor
	}
	return d
}

// RefusedError is the plan_refused family: the spawn plane refused the
// request, the limits read produced no verdict, or the caller left a
// load-bearing input empty. Cause preserves the module error; when the
// cause is a required-effort refusal the message additionally carries the
// launcher's --effort spelling.
type RefusedError struct {
	Detail string
	Cause  error
}

func (e *RefusedError) Error() string {
	if e == nil {
		return CodeRefused + ": <nil>"
	}
	msg := CodeRefused + ": " + e.Detail
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
		if isEffortMissing(e.Cause) {
			msg += "; supply the required effort with --effort <effort>"
		}
	}
	return msg
}

// Unwrap preserves the module refusal evidence.
func (e *RefusedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Code is the stable §6 diagnostic code for this family.
func (e *RefusedError) Code() string { return CodeRefused }

func isEffortMissing(err error) bool {
	return errors.Is(err, vendorplugin.ErrEffortMissing) || errors.Is(err, agentic.ErrEffortMissing)
}

// LimitedError is the plan_provider_limited family: the explicit limits
// read produced a non-serviceable verdict. Verdict is preserved verbatim:
// state, Until, Checked, Observed, and Failures.
type LimitedError struct {
	Verdict vendorplugin.Availability
}

func (e *LimitedError) Error() string {
	if e == nil {
		return CodeLimited + ": <nil>"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: provider limits do not admit this launch (state %s)", CodeLimited, e.Verdict.State)
	if !e.Verdict.Until.IsZero() {
		fmt.Fprintf(&b, " until %s", e.Verdict.Until.Format(time.RFC3339Nano))
	}
	if len(e.Verdict.Checked) > 0 {
		fmt.Fprintf(&b, "; checked %s", strings.Join(e.Verdict.Checked, ", "))
	}
	for _, o := range e.Verdict.Observed {
		fmt.Fprintf(&b, "; observed %s: %s", o.Source, o.Detail)
		if !o.At.IsZero() {
			fmt.Fprintf(&b, " at %s", o.At.Format(time.RFC3339Nano))
		}
	}
	for _, f := range e.Verdict.Failures {
		fmt.Fprintf(&b, "; %s unreadable: %s", f.Source, f.Reason)
	}
	return b.String()
}

// Code is the stable §6 diagnostic code for this family.
func (e *LimitedError) Code() string { return CodeLimited }

// SpawnRequest builds the exact vendorplugin.SpawnRequest §4.4 requires:
// runtime/model/effort from the resolved triple, managed home, current
// workdir, inherited env, empty composition, zero run, and everything else
// unset. It calls nothing and starts no child.
func SpawnRequest(req Request) vendorplugin.SpawnRequest {
	return vendorplugin.SpawnRequest{
		Runtime: vendorplugin.RuntimeID(req.Runtime),
		Model:   vendorplugin.ModelID(req.Model),
		Effort:  req.Effort,
		Home:    req.Home,
		WorkDir: req.WorkDir,
		Env:     req.Env,
	}
}

// Build obtains the interactive plan and enforces provider-limit admission.
// Exactly one BuildLaunch call in agentic.LaunchModeInteractive (by name),
// then exactly one AvailabilityFor read over the same runtime/model/managed
// home. No retry, no fallback, no model downgrade: any refusal is terminal
// for this invocation.
//
// A nil BuildLaunch/Availability in Deps is a caller bug and is refused as
// plan_refused without touching the other boundary.
func Build(ctx context.Context, d Deps, req Request) (agentic.Plan, error) {
	if strings.TrimSpace(req.Home) == "" {
		return agentic.Plan{}, &RefusedError{Detail: "managed home is required; an empty home must not fall back to a native default"}
	}
	if strings.TrimSpace(req.WorkDir) == "" {
		return agentic.Plan{}, &RefusedError{Detail: "working directory is required"}
	}
	if d.BuildLaunch == nil {
		return agentic.Plan{}, &RefusedError{Detail: "no BuildLaunch implementation wired"}
	}
	if d.Availability == nil {
		return agentic.Plan{}, &RefusedError{Detail: "no provider-limits read wired"}
	}
	spawn := SpawnRequest(req)
	// The mode is spelled by name. Never a numeric literal, never a bare
	// agentic.BuildPlan call: admission (runtime/vendor/model/effort) owns
	// this path and must not be bypassed.
	built, err := d.BuildLaunch(ctx, d.Registry, spawn, agentic.LaunchModeInteractive)
	if err != nil {
		return agentic.Plan{}, &RefusedError{Detail: "spawn plane refused the launch", Cause: err}
	}
	verdict, err := d.Availability(providerlimits.VerdictQuery{
		Runtime: req.Runtime,
		Model:   req.Model,
		Home:    req.Home,
	})
	if err != nil {
		// Failure to produce a verdict is terminal plan_refused with the
		// module error, never healthy by inference.
		return agentic.Plan{}, &RefusedError{Detail: "provider-limits read produced no verdict", Cause: err}
	}
	if !verdict.Serviceable() {
		return agentic.Plan{}, &LimitedError{Verdict: verdict}
	}
	return built, nil
}
