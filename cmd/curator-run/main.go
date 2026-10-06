package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/providerlimits"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"

	"github.com/relux-works/curator-agent-launcher/internal/axconfig"
	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/hosted"
	"github.com/relux-works/curator-agent-launcher/internal/mapping"
	"github.com/relux-works/curator-agent-launcher/internal/network"
	"github.com/relux-works/curator-agent-launcher/internal/plan"
	"github.com/relux-works/curator-agent-launcher/internal/systemprompt"
	"github.com/relux-works/curator-network-profiles/pkg/binding"
	"github.com/relux-works/curator-network-profiles/pkg/probe"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
)

const (
	name        = cli.Name
	specVersion = "0.5.0-draft"
	// buildVersion is the launcher's own version; the specification
	// version is reported beside it (SPEC §8).
	buildVersion = "0.2.0"
)

func main() {
	registry, err := defaults.NewRegistry()
	if err != nil {
		_ = diagnostics.Emit(os.Stderr, diagnostics.CodePlanRefused, fmt.Sprintf("cannot build the spawn-plane registry: %v", err))
		os.Exit(diagnostics.ExitForCode(diagnostics.CodePlanRefused))
	}
	deps := launchDeps{
		resolver:     fragment.New(),
		configPaths:  processDefaultsPaths,
		axPaths:      processDefaultsPaths,
		workdir:      os.Getwd,
		environ:      os.Environ,
		availability: processAvailability,
		registry:     registry,
	}
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, deps))
}

// fragmentResolver is the context-plane boundary; main uses fragment.New.
type fragmentResolver interface {
	Resolve(context.Context, fragment.Request) (*fragment.Fragment, error)
}

// launchDeps carries the production boundaries run dispatches through.
// Tests inject scripted resolvers, temporary configuration paths, and
// registries built from the real module; main builds them from process
// state.
// Configuration directories are discovered before parsing for ax policy;
// defaults file contents are read only after a supported fragment is mapped.
func processDefaultsPaths() (defaults.Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil && os.Getenv("XDG_CONFIG_HOME") == "" {
		return defaults.Paths{}, err
	}
	return defaults.ConfigPaths(os.Getenv("XDG_CONFIG_HOME"), home), nil
}

// Process boundaries are injectable; production uses the tagged builder and real execution.
type launchDeps struct {
	axPaths      func() (defaults.Paths, error)
	workdir      func() (string, error)
	environ      func() []string
	availability func() (plan.AvailabilityFunc, error)
	build        plan.BuildLaunchFunc
	axBinary     string
	stdin        io.Reader
	now          func() time.Time
	// providerPath supplies the §4.3 provider-line path. Production
	// leaves it nil so EmitGroup resolves os.Executable; tests inject
	// a deterministic path so goldens stay stable across machines.
	providerPath func() string
	// isTerminal is injectable for deterministic default-mode tests. Production
	// checks both process stdin and stdout with the platform terminal API.
	isTerminal func() bool
	// prober runs the §4.4b bounded preflight. Production leaves it nil
	// so Prepare selects the dialer; tests inject a scripted prober so
	// no test reaches the network.
	prober probe.Prober
	// networkAllowlist overrides the §4.4b STRICT support policy.
	// Production leaves it nil so Prepare uses the shipped allowlist
	// (only the verified claude-code print tuple; every other
	// --network launch refuses); tests inject a test-only list to
	// exercise admission.
	networkAllowlist []binding.AdapterIdentity
	// networkEngineHosts overrides the §4.4b engine-host set.
	// Production leaves it nil: no launchable runtime in this revision
	// carries an engine (plan.Request has no engine member), so engine
	// coverage is vacuous. Tests inject hosts to prove the resolve
	// plumbing; a future engine-capable runtime derives them here.
	networkEngineHosts []string

	configPaths func() (defaults.Paths, error)
	resolver    fragmentResolver
	defaults    defaults.Paths
	registry    *vendorplugin.Registry
	// systems is the agentic system registry for hosted resume elevation.
	// Nil selects agentic.Default, populated by the plan package's plugin
	// imports; tests inject an explicit registry.
	systems *agentic.Registry
	// receiverLookup resolves the hosted receiver binary; receiverTerminal
	// opens the controlling terminal for fd3. Nil selects the hosted
	// production defaults. Native launches never call either: the
	// zero-probe tests fail on any call.
	receiverLookup   func() (string, error)
	receiverTerminal func() (*os.File, error)
	// stdinTerminal reports whether the launcher stdin is a terminal.
	// Nil checks deps.stdin. Hosted launches refuse a non-terminal stdin:
	// piped operator bytes have no forwarding path to the managed child.
	stdinTerminal func() bool
}

// run is the production entry point. Configuration precedes argument validation;
// every admitted launch flows through composition and all late execution checks.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, deps launchDeps) int {
	paths := deps.defaults
	if deps.axPaths != nil {
		var err error
		paths, err = deps.axPaths()
		if err != nil {
			return emitFailure(stderr, diagnostics.CodeDefaultsInvalid, err)
		}
	}
	configured, err := axconfig.Load(filepath.Dir(paths.Machine), filepath.Dir(paths.Operator))
	if err != nil {
		return emitFailure(stderr, diagnostics.CodeDefaultsInvalid, err)
	}
	opts := cli.Options{AxConfigured: configured}
	inv, err := cli.Parse(args, opts)
	if err != nil {
		if cli.IsUsage(err) {
			detail := err.Error()
			var ue *cli.UsageError
			if errors.As(err, &ue) {
				detail = ue.Detail
			}
			_ = diagnostics.Emit(stderr, diagnostics.CodeUsage, detail)
			fmt.Fprintf(stderr, "\n%s", cli.Usage)
			return diagnostics.ExitForCode(diagnostics.CodeUsage)
		}
		return emitFailure(stderr, diagnostics.CodeUsage, err)
	}
	switch inv.Info {
	case cli.InfoHelp:
		fmt.Fprintf(stdout, "%s %s (specification %s)\n\n%s", name, buildVersion, specVersion, cli.Usage)
		return 0
	case cli.InfoVersion:
		fmt.Fprintf(stdout, "%s %s (specification %s)\n", name, buildVersion, specVersion)
		return 0
	}

	// SPEC §3: the operand is already normalized by cli.Parse; normalize
	// again at this boundary so the resolve argv and the default ax session
	// name (execution.Prepare) always carry the canonical id even if the
	// parser is ever bypassed. Aliases never persist beyond this point.
	inv.EnvID = cli.NormalizeEnvID(inv.EnvID)

	// SPEC §4.1: obtain the fragment. The subprocess inherits the
	// launcher's working directory and environment; Curator's stderr goes
	// to the operator unchanged.
	frag, err := deps.resolver.Resolve(ctx, fragment.Request{
		EnvID:      inv.EnvID,
		Profile:    inv.Profile,
		ProfileSet: inv.ProfileSet,
		Stderr:     stderr,
	})
	if err != nil {
		if re, ok := fragment.IsResolve(err); ok {
			_ = diagnostics.Emit(stderr, re.Code, re.Detail)
			return diagnostics.ExitForCode(re.Code)
		}
		_ = diagnostics.Emit(stderr, fragment.CodeInvocationFailed, err.Error())
		return diagnostics.ExitForCode(fragment.CodeInvocationFailed)
	}

	target, err := mapping.Resolve(frag.Environment)
	if err != nil {
		_ = diagnostics.Emit(stderr, mapping.CodeUnsupported, err.Error())
		return diagnostics.ExitForCode(mapping.CodeUnsupported)
	}

	// SPEC §4.3: resolve model and effort defaults. A locked flag is a
	// usage refusal, an unreadable file is defaults_config_invalid, and
	// a system the lineup admits nothing for is defaults_unresolvable —
	// every one terminal with nothing launched. The provider path and
	// the resolved pair with its per-member origins print on stderr as
	// one line-group before the plan request.
	if deps.configPaths != nil {
		paths, err := deps.configPaths()
		if err != nil {
			_ = diagnostics.Emit(stderr, defaults.CodeInvalid, fmt.Sprintf("cannot locate defaults configuration: %v", err))
			return diagnostics.ExitForCode(defaults.CodeInvalid)
		}
		deps.defaults = paths
	}
	files, err := defaults.Load(deps.defaults)
	if err == nil {
		var flags defaults.Pair
		if inv.ModelSet {
			flags.Model = defaults.Member{Value: inv.Model, Present: true}
		}
		if inv.EffortSet {
			flags.Effort = defaults.Member{Value: inv.Effort, Present: true}
		}
		if inv.PermissionSet {
			flags.Permissions = defaults.Member{Value: string(inv.PermissionMode), Present: true}
		}
		var resolved defaults.Resolved
		resolved, err = files.Complete(frag.Environment, flags, deps.registry)
		if err == nil {
			var host defaults.ResolvedHost
			host, err = files.ResolveHost(frag.Environment, inv.HostFlag)
			if err == nil {
				provider := defaults.ResolveProviderPath()
				if deps.providerPath != nil {
					provider = deps.providerPath()
				}
				_ = defaults.EmitGroupWithProvider(stderr, resolved, provider)
				return launch(ctx, inv, frag, target, resolved, host, files, stdout, stderr, deps)
			}
		}
	}
	if derr := (&defaults.Error{}); errors.As(err, &derr) {
		_ = diagnostics.Emit(stderr, derr.Code, derr.Err.Error())
		return diagnostics.ExitForCode(derr.Code)
	}
	_ = diagnostics.Emit(stderr, mapping.CodeUnsupported, err.Error())
	return diagnostics.ExitForCode(mapping.CodeUnsupported)
}

// emitNetworkFailure retains a typed network refusal's code and renders
// its subject and detail; a non-refusal error at these call sites is the
// composition contract failing and stays plan_refused.
func emitNetworkFailure(stderr io.Writer, err error) int {
	if r, ok := refusal.As(err); ok && r != nil {
		_ = diagnostics.Emit(stderr, r.Code, network.Detail(err))
		return diagnostics.ExitForCode(r.Code)
	}
	return emitFailure(stderr, diagnostics.CodePlanRefused, err)
}

// emitFailure frames only launcher-owned diagnostics. Foreign evidence is emitted
// separately at the plan and execution boundaries.
func emitFailure(w io.Writer, code string, err error) int {
	_ = diagnostics.Emit(w, code, strings.TrimPrefix(err.Error(), code+": "))
	return diagnostics.ExitForCode(code)
}

func processAvailability() (plan.AvailabilityFunc, error) {
	layout, err := providerlimits.DefaultLayout()
	if err != nil {
		return nil, err
	}
	store, err := providerlimits.NewStore(providerlimits.Options{Layout: layout})
	if err != nil {
		return nil, err
	}
	return store.AvailabilityFor, nil
}

func launch(ctx context.Context, inv cli.Invocation, frag *fragment.Fragment, target mapping.Target, resolved defaults.Resolved, host defaults.ResolvedHost, files defaults.Files, stdout, stderr io.Writer, deps launchDeps) int {
	// SPEC §4.8 order: host resolution is done; the legacy ax table, the
	// defaults gate, and the network gate run before permissions and the
	// plan build, and every refusal precedes any receiver contact.
	isHosted := host.Host == defaults.HostHosted
	if inv.Tracked {
		if isHosted {
			return emitFailure(stderr, diagnostics.CodeHostConflict, errors.New("host_configuration_conflict: hosted execution conflicts with the enabled ax integration on this machine"))
		}
		if cli.ExplicitNative(inv.HostFlag) {
			// An explicit native request bypasses ax: the existing
			// untracked native path. A configured native default keeps
			// the existing ax behavior.
			inv.Tracked = false
		}
	}
	// Operator decision Q-D1a (2026-10-05) = yes: an operator-scope
	// hosted default is the operator's own opt-in and passes the gate.
	// Machine defaults and locks stay gated until upgrade-without-hangup
	// (Decision 0021 section 7 amendment pending). Explicit --hosted
	// always passes.
	if isHosted && host.Origin == defaults.OriginHostMachine {
		return emitFailure(stderr, diagnostics.CodeSessionHostDefaultNotReady, fmt.Errorf("session_host_default_not_ready: the machine hosted default for %s is not ready until upgrade-without-hangup; use explicit --hosted or an operator default", frag.Environment))
	}
	if !isHosted && (inv.ResumeRequested || inv.ResumeIDSet) {
		_ = diagnostics.Emit(stderr, diagnostics.CodeUsage, "resume selectors require hosted execution; use --hosted or a hosted default")
		fmt.Fprintf(stderr, "\n%s", cli.Usage)
		return diagnostics.ExitForCode(diagnostics.CodeUsage)
	}
	if isHosted && inv.NetworkSet {
		// Hosted execution supports no managed network scope: the
		// receiver cannot enforce a source-side profile. The native
		// --network path (SPEC §4.4b) is untouched; only this refusal
		// maps to the hosted policy exit.
		_ = diagnostics.Emit(stderr, diagnostics.CodeNetworkScopeUnsupported, "managed network selections are not supported by hosted execution")
		return diagnostics.ExitPolicy
	}
	if isHosted {
		// Hosted runs tracked, but operator decision Q-D3 (2026-10-05)
		// exempts the hosted path from tracked permission semantics:
		// the configured ladder applies, silence defaults like native,
		// and yolo is admitted. resolvePermission carries the
		// exemption; nothing else in the hosted path reads Tracked.
		inv.Tracked = true
	}
	wd, err := deps.workdir()
	if err != nil {
		return emitFailure(stderr, diagnostics.CodePlanRefused, err)
	}
	parentEnv := deps.environ()
	decision, toolRelease, system, err := resolvePermission(ctx, inv, frag, target, files, parentEnv, stderr, deps, isHosted)
	if err != nil {
		return emitPermissionFailure(stderr, err)
	}
	if isHosted && frag.Environment != fragment.EnvClaudeCode {
		return emitFailure(stderr, diagnostics.CodeSessionHostProviderUnsupported, fmt.Errorf("session_host_provider_unsupported: hosted execution in Phase 1 admits claude_code only; %s is not admitted", frag.Environment))
	}
	// The native tail for the plan build. Hosted elevation runs the module
	// typed-intent API over the wrapper selectors and the verbatim tail and
	// detaches elevated selectors; native mode never interprets the tail.
	tail := inv.Native
	intent := agentic.ResumeIntent{Kind: agentic.ResumeNew}
	if isHosted {
		systems := deps.systems
		if systems == nil {
			systems = agentic.Default
		}
		wrapper, err := hosted.WrapperArgs(inv.ResumeRequested, inv.ResumeHandle, inv.ResumeHandleSet, inv.ResumeID, inv.ResumeIDSet)
		if err != nil {
			return emitFailure(stderr, diagnostics.CodeSessionResumeInvalid, err)
		}
		elevation, err := hosted.ElevateResume(systems, target.System, wrapper, inv.Native)
		if err != nil {
			return emitFailure(stderr, diagnostics.CodeSessionResumeInvalid, err)
		}
		tail, intent = elevation.NativeArgs, elevation.Intent
	}
	// The piped-launcher-stdin property is known before composition, so
	// it refuses here, after every higher-priority refusal (host, ax,
	// defaults gate, network, permission, provider, resume) and before
	// the plan build or any receiver contact. The attached-plan-stdin
	// property below is only knowable from the built plan, so that gate
	// necessarily stays after the build.
	if isHosted {
		terminal := deps.stdinTerminal
		if terminal == nil {
			stdin := deps.stdin
			if stdin == nil {
				stdin = os.Stdin
			}
			terminal = func() bool { return execution.StdinIsTerminal(stdin) }
		}
		if !terminal() {
			return emitFailure(stderr, diagnostics.CodeSessionHostStdinUnsupported, errors.New("session_host_stdin_unsupported: hosted launches in Phase 1 do not support piped stdin"))
		}
	}
	// The controlling-terminal property is known before composition
	// (opening the terminal either succeeds or fails), so it refuses
	// here, after every higher-priority refusal and before the plan
	// build, composition, export, and receiver lookup. The opened
	// descriptor is reused by the transport; it is never opened twice.
	var preopenedTTY *os.File
	if isHosted {
		openTerminal := deps.receiverTerminal
		if openTerminal == nil {
			openTerminal = hosted.OpenTerminal
		}
		tty, err := openTerminal()
		if err != nil || tty == nil {
			if tty != nil {
				tty.Close()
			}
			return emitFailure(stderr, diagnostics.CodeSessionHostTerminalRequired, errors.New("session_host_terminal_required: a hosted launch requires a controlling terminal"))
		}
		// r6 §4, before the build: the launcher's close-on-exec mark
		// plus type/access/ownership validation. A wrong descriptor
		// refuses here with zero builds and zero lookup, like a
		// missing terminal; the transport re-validates before contact.
		hosted.MarkCloseOnExec(tty)
		if err := hosted.ValidateTerminal(tty); err != nil {
			tty.Close()
			return emitFailure(stderr, diagnostics.CodeSessionHostTerminalRequired, errors.New("session_host_terminal_required: a hosted launch requires a controlling terminal"))
		}
		preopenedTTY = tty
		defer func() {
			if preopenedTTY != nil {
				preopenedTTY.Close()
			}
		}()
	}
	availability, err := deps.availability()
	if err != nil {
		return emitFailure(stderr, diagnostics.CodePlanRefused, err)
	}
	build := deps.build
	if build == nil {
		build = vendorplugin.BuildLaunchWithEnvironment
	}
	selection, err := systemprompt.Select(frag, fragment.Semantics(inv.SystemPrompt))
	if err != nil {
		code, _ := diagnostics.CodeOf(err)
		return emitFailure(stderr, code, err)
	}
	launchContext := plan.Context(frag, fragment.Semantics(inv.SystemPrompt))
	admitted, err := plan.Build(ctx, plan.Deps{Registry: deps.registry, BuildLaunch: build, Availability: availability}, plan.Request{
		Runtime: resolved.Runtime, Model: resolved.Model.Value, Effort: resolved.Effort.Value,
		PermissionMode: decision.Mode, ToolRelease: toolRelease, NativeArgs: tail,
		Home: frag.Home(), WorkDir: wd, Env: parentEnv, Context: launchContext,
	})
	if err != nil {
		code := diagnostics.CodePlanRefused
		var limited *plan.LimitedError
		if errors.As(err, &limited) {
			code = diagnostics.CodePlanProviderLimited
		} else if errors.Is(err, agentic.ErrPermissionModeUnsupported) {
			code = diagnostics.CodePermissionModeUnsupported
		} else if errors.Is(err, agentic.ErrNativePolicyUnknown) || errors.Is(err, agentic.ErrPermissionModeDuplicate) {
			code = diagnostics.CodeUsage
		}
		var conflict *agentic.ContextDescriptorConflictError
		if errors.As(err, &conflict) {
			if isHosted {
				// Hosted diagnostics never copy native argument
				// values: the channel and the module's fixed
				// reason identify the conflict. The launcher
				// cannot name the finer flag without parsing
				// provider options, which the contract forbids.
				// Native keeps the legacy shape below.
				return emitFailure(stderr, diagnostics.CodePlanRefused, fmt.Errorf("native arguments conflict with fragment channel %s: %s", conflict.Channel, conflict.Reason))
			}
			return emitFailure(stderr, diagnostics.CodePlanRefused, fmt.Errorf("native arguments %q conflict with fragment channel %s: %w", tail, conflict.Channel, err))
		}
		if code == diagnostics.CodeUsage {
			detail := err.Error()
			if isHosted && errors.Is(err, agentic.ErrNativePolicyUnknown) {
				// The unknown-mode text echoes the operator's
				// mode value; the hosted boundary reports the
				// fixed sentinel instead of the value.
				detail = agentic.ErrNativePolicyUnknown.Error()
			}
			_ = diagnostics.Emit(stderr, code, detail)
			fmt.Fprintf(stderr, "\n%s", cli.Usage)
			return diagnostics.ExitForCode(code)
		}
		_ = diagnostics.Emit(stderr, code, "spawn-plane admission refused the launch")
		// Preserve provider/module evidence bytes, including embedded newlines.
		fmt.Fprintln(stderr, err.Error())
		return diagnostics.ExitForCode(code)
	}
	// Phase 1 carries terminal stdin only: an attached plan stdin
	// refuses here. It is knowable only from the built plan, so this
	// gate necessarily follows the build; the known piped-launcher-stdin
	// gate above precedes it. Neither has a forwarding path to the
	// managed child.
	if isHosted && admitted.Plan.Stdin.Attached {
		return emitFailure(stderr, diagnostics.CodeSessionHostStdinUnsupported, errors.New("session_host_stdin_unsupported: hosted launches in Phase 1 do not support attached stdin"))
	}
	// SPEC §4.4b: resolve and validate the explicit network selection
	// after admission and before composition, reading the catalog with
	// the operator's original parentEnv. The support check runs at the
	// ACTUAL admitted entrypoint — the EFFECTIVE shape, not the
	// construction mode alone: plan.Build constructs interactive
	// launches, and an explicit native print selection (-p/--print in
	// flag position, read as the plan carries it) is the verified
	// `claude -p` entrypoint and stays admitted, while a managed
	// interactive launch without one refuses network_scope_unsupported.
	// The derivation runs only for a managed selection, so unmanaged
	// launches never meet it. A bounded preflight probes only a real
	// supported direct launch under a proxy profile; a named
	// kind="direct" profile skips every probe step. Any resolve,
	// validate or preflight error terminates without weaker routing.
	// The overlay names travel with the request so a proxy-family
	// conflict refuses before the probe, and the engine hosts travel so
	// their coverage is checked.
	prompt := composition.PromptApplication{Argv: selection.Argv(), Env: selection.Env()}
	var mcpNames []string
	if frag.MCP != nil {
		mcpNames = frag.MCP.EnvNames
	}
	entrypoint := ""
	if inv.NetworkSet {
		var merr error
		entrypoint, merr = network.EffectiveEntrypoint(admitted.Plan, target.System, inv.Native)
		if merr != nil {
			return emitNetworkFailure(stderr, merr)
		}
	}
	preparedNet, err := network.Prepare(ctx, network.Request{
		Explicit: inv.Network, ExplicitSet: inv.NetworkSet,
		Tracked: inv.Tracked, Harness: target.System,
		Build: toolRelease, Entrypoint: entrypoint,
		ParentEnv: parentEnv, Prober: deps.prober,
		Allowlist: deps.networkAllowlist,
		FragEnv:   frag.Env, PromptEnv: prompt.Env, MCPEnvNames: mcpNames,
		EngineHosts: deps.networkEngineHosts,
	})
	if err != nil {
		return emitNetworkFailure(stderr, err)
	}
	inspection := agentic.InspectStoredPolicy(system, admitted.Plan)
	value, err := composition.ComposeAdmittedPlanWithNetwork(admitted.Plan, admitted.OwnedEnv, *frag, prompt, tail, preparedNet.Patch)
	if err != nil {
		return emitNetworkFailure(stderr, err)
	}
	// The Record is provenance of a composed launch: a launch that
	// Compose refuses prints no binding Record.
	if preparedNet.Record != nil {
		fmt.Fprintln(stderr, network.ProvenanceLine(*preparedNet.Record))
	}
	now := time.Now
	if deps.now != nil {
		now = deps.now
	}
	boundary := func() error {
		prompt, err := systemprompt.PrepareLaunch(frag, fragment.Semantics(inv.SystemPrompt))
		if err != nil {
			return err
		}
		for _, warning := range prompt.Warnings {
			fmt.Fprintln(stderr, name+": "+warning)
		}
		return nil
	}
	if isHosted {
		tty := preopenedTTY
		preopenedTTY = nil
		return runHosted(inv, frag, target, value, intent, inspection, admitted.Plan, decision, parentEnv, now(), stdout, stderr, deps, boundary, tty)
	}
	return runNative(inv, frag, target, value, decision, inspection, stdout, stderr, deps, boundary, now())
}

// runNative is the existing native tail: report, prepare, and run the
// composed value directly or through ax. It performs zero receiver contact;
// hosted failures never reach it, which the fallback mutant guards.
func runNative(inv cli.Invocation, frag *fragment.Fragment, target mapping.Target, value composition.Value, decision execution.PermissionDecision, inspection agentic.StoredPolicyInspection, stdout, stderr io.Writer, deps launchDeps, boundary func() error, at time.Time) int {
	var nativePolicy *execution.EffectiveNativePolicy
	if decision.Mode == agentic.PermissionModeNative {
		nativePolicy = execution.ReportEffectiveNativePolicy(stderr, inspection, inv.Tracked)
	}
	prepared, err := execution.PrepareWithNativePolicy(value, *frag, inv, target, at, nativePolicy)
	if err != nil {
		return emitFailure(stderr, diagnostics.CodeAxHandoffFailed, err)
	}
	return prepared.Run(execution.Options{AxBinary: deps.axBinary, IO: execution.IO{Stdin: deps.stdin, Stdout: stdout, Stderr: stderr}, Boundary: boundary})
}

// runHosted serializes the once-composed plan into the versioned payload and
// hands it to the receiver. It performs the only receiver contact in the
// launcher; native execution never reaches it. There is no fallback: a
// missing receiver refuses session_host_missing, never native execution.
// preopenedTTY is the terminal the entry point opened before the build;
// runHosted owns it and hands it to the transport, closing it on any
// earlier return.
func runHosted(inv cli.Invocation, frag *fragment.Fragment, target mapping.Target, value composition.Value, intent agentic.ResumeIntent, inspection agentic.StoredPolicyInspection, base agentic.Plan, decision execution.PermissionDecision, parentEnv []string, at time.Time, stdout, stderr io.Writer, deps launchDeps, boundary func() error, preopenedTTY *os.File) int {
	pendingTTY := preopenedTTY
	defer func() {
		if pendingTTY != nil {
			pendingTTY.Close()
		}
	}()
	for _, warning := range value.Warnings {
		fmt.Fprintln(stderr, name+": "+warning)
	}
	if err := boundary(); err != nil {
		code, detail, ok := strings.Cut(err.Error(), ": ")
		if !ok || !diagnostics.Valid(code) {
			code, detail = diagnostics.CodePlanRefused, err.Error()
		}
		_ = diagnostics.Emit(stderr, code, detail)
		return diagnostics.ExitForCode(code)
	}
	seal, err := base.ExportSeal()
	if err != nil {
		_ = diagnostics.Emit(stderr, diagnostics.CodeLaunchPlanInvalid, "process.exec_guard: plan exports no hosted-admissible guard")
		fmt.Fprintln(stderr, err.Error())
		return diagnostics.ExitForCode(diagnostics.CodeLaunchPlanInvalid)
	}
	systems := deps.systems
	if systems == nil {
		systems = agentic.Default
	}
	restart, err := hosted.ExportRestart(systems, target.System, value.Argv)
	if err != nil {
		var planErr *hosted.PlanError
		if errors.As(err, &planErr) {
			return emitFailure(stderr, planErr.Code, planErr)
		}
		return emitFailure(stderr, diagnostics.CodeLaunchPlanInvalid, err)
	}
	hostName := inv.Name
	if !inv.NameSet {
		hostName = inv.EnvID + "-" + at.UTC().Format("20060102T150405Z")
	}
	// The native session name and RC intent come from Plan.Session of the
	// SAME admitted plan the value was composed from, filled by the
	// module while it built the argv; the launcher parses no provider
	// flags. The indices describe the plan argv, so a composed argv that
	// differs from it refuses instead of being re-indexed.
	session, err := hosted.SessionFromPlan(base.Session, base.Argv, value.Argv)
	if err != nil {
		var planErr *hosted.PlanError
		if errors.As(err, &planErr) {
			return emitFailure(stderr, planErr.Code, planErr)
		}
		return emitFailure(stderr, diagnostics.CodeLaunchPlanInvalid, err)
	}
	wire, err := hosted.BuildPayload(hosted.PayloadInputs{
		EnvID: frag.Environment, Value: value, Fragment: *frag, CallerEnv: parentEnv,
		PermissionMode: decision.Mode, PermissionSource: decision.Source,
		HostName: hostName, Resume: intent, Restart: restart, Seal: seal, Inspection: inspection, Session: session,
	})
	if err != nil {
		var planErr *hosted.PlanError
		if errors.As(err, &planErr) {
			return emitFailure(stderr, planErr.Code, planErr)
		}
		return emitFailure(stderr, diagnostics.CodePlanRefused, err)
	}
	pendingTTY = nil
	return hosted.Transport{Lookup: deps.receiverLookup, Terminal: deps.receiverTerminal, PreopenedTerminal: preopenedTTY}.Run(wire, stdout, stderr)
}

func resolvePermission(ctx context.Context, inv cli.Invocation, frag *fragment.Fragment, target mapping.Target, files defaults.Files, parentEnv []string, stderr io.Writer, deps launchDeps, hosted bool) (execution.PermissionDecision, string, agentic.System, error) {
	global, err := files.PermissionDefault(inv.EnvID)
	if err != nil {
		return execution.PermissionDecision{}, "", nil, err
	}
	system, ok := agentic.Default.Lookup(agentic.SystemID(target.System))
	if !ok {
		return execution.PermissionDecision{}, "", nil, fmt.Errorf("%s: permission system %q is not registered", diagnostics.CodePlanRefused, target.System)
	}

	terminal := execution.InteractiveStdio(os.Stdin, os.Stdout)
	if deps.isTerminal != nil {
		terminal = deps.isTerminal()
	}
	// Q-D3 (2026-10-05), literal: the headless signal is observed but
	// never consulted by the permission default; ResolvePermission
	// selects yolo for silence on every stdio shape. The native-argument
	// non-interactive classification served only that default, so it is
	// gone: the tool release probes once below for the mapping check.
	headless := (inv.Tracked && !hosted) || execution.HasNonInteractiveMarker(parentEnv) || !terminal

	toolRelease := ""
	var probeErr error
	decision, err := execution.ResolvePermission(execution.PermissionRequest{
		Flag: inv.PermissionMode, FlagPresent: inv.PermissionSet, Profile: frag.Permissions,
		Global: global, Headless: headless, Tracked: inv.Tracked, Hosted: hosted,
		Transport: frag.Revision == fragment.IdentityV2 || frag.Revision == fragment.IdentityV3,
	})
	if err != nil {
		return execution.PermissionDecision{}, "", nil, err
	}
	if toolRelease == "" {
		toolRelease, probeErr = agentic.ProbeToolRelease(ctx, system, parentEnv)
	}
	permissionMapping, mappingErr := agentic.Default.PermissionMapping(agentic.SystemID(target.System), toolRelease, decision.Mode)
	if mappingErr != nil {
		if errors.Is(mappingErr, agentic.ErrPermissionModeUnsupported) {
			return execution.PermissionDecision{}, "", nil, &execution.PermissionError{Code: diagnostics.CodePermissionModeUnsupported, Detail: mappingErr.Error()}
		}
		if decision.Mode != agentic.PermissionModeNative || !errors.Is(mappingErr, agentic.ErrPermissionModeUnverifiedRelease) {
			return execution.PermissionDecision{}, "", nil, fmt.Errorf("%s: permission mapping was not established: %w", diagnostics.CodePlanRefused, mappingErr)
		}
		// The module defines native as no permission override. A failed release
		// probe leaves ToolRelease empty, which remains valid for a native
		// request and makes no provider-mapping claim.
		permissionMapping.Flag = ""
	}
	if decision.Mode == agentic.PermissionModeYolo && probeErr != nil {
		return execution.PermissionDecision{}, "", nil, fmt.Errorf("%s: tool release was not established: %v: %w", diagnostics.CodePlanRefused, probeErr, agentic.ErrPermissionModeUnverifiedRelease)
	}
	if !inv.Tracked {
		mapped := permissionMapping.Flag
		if mapped == "" {
			mapped = "none"
		}
		fmt.Fprintf(stderr, "curator-run: permissions=%s source=%s mapped=%s\n", decision.Mode, decision.Source, mapped)
	}
	return decision, toolRelease, system, nil
}

func emitPermissionFailure(stderr io.Writer, err error) int {
	var permissionErr *execution.PermissionError
	if errors.As(err, &permissionErr) {
		_ = diagnostics.Emit(stderr, permissionErr.Code, permissionErr.Detail)
		if permissionErr.Code == diagnostics.CodeUsage {
			fmt.Fprintf(stderr, "\n%s", cli.Usage)
		}
		return diagnostics.ExitForCode(permissionErr.Code)
	}
	code, detail, ok := strings.Cut(err.Error(), ": ")
	if !ok || !diagnostics.Valid(code) {
		code, detail = diagnostics.CodePlanRefused, err.Error()
	}
	_ = diagnostics.Emit(stderr, code, detail)
	if code == diagnostics.CodeUsage {
		fmt.Fprintf(stderr, "\n%s", cli.Usage)
	}
	return diagnostics.ExitForCode(code)
}
