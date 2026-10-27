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
	museSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/muse"
	"github.com/relux-works/skill-agents-management/pkg/providerlimits"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/hosted"
	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
	"github.com/relux-works/curator-agent-launcher/internal/mapping"
	"github.com/relux-works/curator-agent-launcher/internal/migrate"
	"github.com/relux-works/curator-agent-launcher/internal/network"
	"github.com/relux-works/curator-agent-launcher/internal/plan"
	"github.com/relux-works/curator-agent-launcher/internal/selector"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	"github.com/relux-works/curator-agent-launcher/internal/systemprompt"
	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
	"github.com/relux-works/curator-network-profiles/pkg/binding"
	"github.com/relux-works/curator-network-profiles/pkg/probe"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
)

const (
	name        = cli.Name
	specVersion = "0.5.1-draft"
)

// buildVersion is the launcher's own version; the specification version is
// reported beside it (SPEC §8). It is a variable so that `make
// release-binary` can set it with -ldflags -X (FRONT-DOOR §10).
var buildVersion = "0.6.0"

func main() {
	if len(os.Args) == 2 && os.Args[1] == selector.WorkerArgument {
		input := os.NewFile(3, "selector-request")
		if input == nil {
			os.Exit(1)
		}
		os.Exit(selector.RunWorker(input))
	}
	registry, err := defaults.NewRegistry()
	if err != nil {
		_ = diagnostics.Emit(os.Stderr, diagnostics.CodePlanRefused, fmt.Sprintf("cannot build the spawn-plane registry: %v", err))
		os.Exit(diagnostics.ExitForCode(diagnostics.CodePlanRefused))
	}
	deps := launchDeps{
		resolver:        fragment.New(),
		configLocations: processConfigLocations,
		workdir:         os.Getwd,
		environ:         os.Environ,
		availability:    processAvailability,
		registry:        registry,
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
// Configuration directories are discovered before reading launch policy;
// defaults file contents are read only after a supported fragment is mapped.
// Configuration roots are fixed for distributed builds. Binary tests relocate
// only these roots at link time, preserving real parsing, resolution and exec.
var systemConfigDirectory = "/etc/swarma/launcher"
var legacySystemConfigDirectory = "/etc/curator-run"

func processConfigLocations() (configfile.Locations, error) {
	home, err := os.UserHomeDir()
	if err != nil && os.Getenv("XDG_CONFIG_HOME") == "" {
		return configfile.Locations{}, err
	}
	locations := configfile.ConfigLocations(os.Getenv("XDG_CONFIG_HOME"), home)
	locations.System, locations.LegacySystem = systemConfigDirectory, legacySystemConfigDirectory
	return locations, nil
}

// Process boundaries are injectable; production uses the tagged builder and real execution.
type launchDeps struct {
	configLocations func() (configfile.Locations, error)
	// launcherConfig is validated machine policy for host resolution.
	launcherConfig  launcherconfig.Config
	configResolver  configfile.Resolver
	hostedSelection *hostSelection
	hostedSession   func(string) // validated start result, including reconciliation
	managerService  *sessionmanager.Service
	selectorConfig  selector.Config
	selectorRequest *selector.Request
	shadowStart     func(selector.ShadowJob) (*selector.Child, error)
	observationPath func() string

	workdir       func() (string, error)
	environ       func() []string
	availability  func() (plan.AvailabilityFunc, error)
	build         plan.BuildLaunchFunc
	moduleVersion func() string // Tests supply metadata omitted by Go test binaries.
	stdin         io.Reader
	now           func() time.Time
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
	// networkScopeCheck is a test seam for the independent child-scope ceiling.
	networkScopeCheck     func(adapterprobe.Identity) bool
	networkEvaluate       func(context.Context, adapterprobe.Request, adapterprobe.Policy) (adapterprobe.Decision, error)
	networkEmit           func(io.Writer, *adapterprobe.Provenance) error
	networkSessionAllowed func(hosted bool) bool
	networkMatchIdentity  func(binding.AdapterIdentity, binding.AdapterIdentity) bool
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
	// receiverTerminal opens the controlling terminal before manager contact.
	// Native execution never calls it.
	receiverTerminal func() (*os.File, error)
	// stdinTerminal reports whether the launcher stdin is a terminal.
	// Nil checks deps.stdin. Hosted launches refuse a non-terminal stdin:
	// piped operator bytes have no forwarding path to the managed child.
	stdinTerminal func() bool
}

// run is the production entry point. Flags parse before configuration is read
// (FRONT-DOOR §3.2), so a usage error wins over a broken configuration;
// every admitted launch flows through composition and all late execution checks.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, deps launchDeps) int {
	// Help and version are static (CLI review F2/F3): answer them before any
	// configuration, registry or station path is resolved, so they work even
	// when that state is missing or broken.
	if text, ok := cli.TopicHelp(args); ok {
		fmt.Fprint(stdout, text)
		return 0
	}
	if inv, err := cli.Parse(args); err == nil {
		switch inv.Info {
		case cli.InfoHelp:
			fmt.Fprintf(stdout, "%s %s (specification %s)\n\n%s", name, buildVersion, specVersion, cli.Usage)
			return 0
		case cli.InfoVersion:
			fmt.Fprintf(stdout, "%s %s (specification %s)\n", name, buildVersion, specVersion)
			return 0
		}
	}
	paths := deps.defaults
	var configResolver configfile.Resolver
	if deps.configLocations != nil {
		loc, err := deps.configLocations()
		if err != nil {
			return emitFailure(stderr, diagnostics.CodeDefaultsInvalid, err)
		}
		configResolver = configfile.Resolver{Locations: loc, Stderr: stderr}
	} else {
		configResolver = configfile.Resolver{Locations: configfile.Locations{System: filepath.Dir(paths.Machine), User: filepath.Dir(paths.Operator)}, Stderr: stderr}
	}
	if len(args) > 0 && args[0] == "werkstatt" {
		inv, err := cli.Parse(args)
		if err != nil {
			return emitFailure(stderr, diagnostics.CodeUsage, err)
		}
		return runWerkstatt(ctx, *inv.Werkstatt, stdout, stderr, configResolver, deps)
	}
	// Registry administration and observation are independent of launch policy
	// and fragments. Only observe contacts the registered manager.
	if len(args) > 0 && args[0] == "session-manager" {
		inv, err := cli.Parse(args)
		if err != nil {
			return emitFailure(stderr, diagnostics.CodeUsage, err)
		}
		if inv.SessionManager.Operation == "observe" {
			return runManagerObservation(ctx, *inv.SessionManager, stdout, stderr, configResolver, deps)
		}
		service := sessionmanager.Service{}
		if deps.managerService != nil {
			service = *deps.managerService
		}
		service.Resolver = configResolver
		service.Stdin = deps.stdin
		service.Stdout = stdout
		service.Stderr = stderr
		if service.Now == nil {
			service.Now = deps.now
		}
		if service.IsTerminal == nil {
			service.IsTerminal = deps.stdinTerminal
			if service.IsTerminal == nil {
				service.IsTerminal = func() bool {
					in := deps.stdin
					if in == nil {
						in = os.Stdin
					}
					return execution.StdinIsTerminal(in)
				}
			}
		}
		if err := service.Run(*inv.SessionManager); err != nil {
			code := sessionmanager.CodeInvalid
			var managerError *sessionmanager.Error
			if errors.As(err, &managerError) {
				code = managerError.Code
			}
			return emitFailure(stderr, code, err)
		}
		return 0
	}
	// Migration must remain available when the legacy host member prevents a
	// normal launch. Parse its closed grammar before loading the configs.
	if len(args) > 0 && args[0] == "migrate" {
		inv, err := cli.Parse(args)
		if err != nil {
			return emitFailure(stderr, diagnostics.CodeUsage, err)
		}
		if err := migrate.Run(configResolver.Locations, inv.DryRun, stdout, migrate.Options{}); err != nil {
			code := diagnostics.CodeDefaultsInvalid
			var migrationError *migrate.Error
			if errors.As(err, &migrationError) {
				code = migrationError.Code
			}
			return emitFailure(stderr, code, err)
		}
		return 0
	}
	// Validate flag grammar before reading launch policy.
	if _, parseErr := cli.Parse(args); parseErr != nil {
		detail := parseErr.Error()
		var usage *cli.UsageError
		if errors.As(parseErr, &usage) {
			detail = usage.Detail
		}
		_ = diagnostics.Emit(stderr, diagnostics.CodeUsage, detail)
		fmt.Fprintf(stderr, "\n%s", cli.Usage)
		return diagnostics.ExitUsage
	}
	if path, err := configfile.RemovedAX(configResolver.Locations); err != nil || path != "" {
		if path != "" {
			return emitFailure(stderr, diagnostics.CodeAXTrackingRemoved, fmt.Errorf("delete this file: path=%q", path))
		}
		return emitFailure(stderr, diagnostics.CodeDefaultsInvalid, err)
	}
	policy, err := launcherconfig.Load(configResolver)
	if err != nil {
		return emitFailure(stderr, diagnostics.CodeDefaultsInvalid, err)
	}
	deps.launcherConfig = policy
	deps.configResolver = configResolver
	deps.selectorConfig, err = selector.Load(configResolver)
	if err != nil {
		return emitFailure(stderr, diagnostics.CodeDefaultsInvalid, err)
	}

	inv, _ := cli.Parse(args) // Grammar was checked above.
	if inv.HostFlag == cli.HostFlagUntracked {
		fmt.Fprintln(stderr, "curator-run: --untracked is deprecated; use --native (alias retained for two releases)")
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
	// again at this boundary so the resolve argv always carries the canonical
	// id even if the parser is ever bypassed. Aliases never persist beyond this point.
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
	if err == nil {
		err = fragment.CheckLaunchRevision(frag)
	}
	if err != nil {
		var version *fragment.VersionError
		if errors.As(err, &version) {
			return emitHostFailure(stderr, err)
		}
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
	if deps.configLocations == nil && deps.configPaths != nil {
		paths, err := deps.configPaths()
		if err != nil {
			_ = diagnostics.Emit(stderr, defaults.CodeInvalid, fmt.Sprintf("cannot locate defaults configuration: %v", err))
			return diagnostics.ExitForCode(defaults.CodeInvalid)
		}
		deps.defaults = paths
	}
	var files defaults.Files
	if deps.configLocations != nil {
		files, err = defaults.LoadResolved(configResolver)
	} else {
		files, err = defaults.Load(deps.defaults)
	}
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
		// Preserve existing locked-flag refusals before invoking external policy.
		if deps.selectorConfig.Mode == "enforce" {
			if _, resolveErr := files.Resolve(frag.Environment, flags); resolveErr != nil {
				var derr *defaults.Error
				if errors.As(resolveErr, &derr) {
					return emitFailure(stderr, derr.Code, derr.Err)
				}
				return emitFailure(stderr, diagnostics.CodeDefaultsInvalid, resolveErr)
			}
		}
		var resolved defaults.Resolved
		if deps.selectorConfig.Mode != "" {
			var request selector.Request
			request, err = selectionRequest(inv, frag, deps)
			if err != nil && deps.selectorConfig.Mode == "enforce" {
				return emitFailure(stderr, diagnostics.CodeSelectorUnavailable, errors.New("selector unavailable reason=invalid"))
			}
			if err == nil {
				deps.selectorRequest = &request
			}
		}
		if deps.selectorConfig.Mode == "enforce" {
			decision, selectionError := selector.Enforce(ctx, deps.selectorConfig, *deps.selectorRequest)
			if selectionError != nil {
				failure := selectionError.(*selector.Failure)
				return emitFailure(stderr, failure.Code, failure)
			}
			selected := defaults.Pair{Model: defaults.Member{Value: decision.Model, Present: true}, Effort: defaults.Member{Value: selectedEffort(decision), Present: true}}
			resolved, err = files.CompleteWithSelection(frag.Environment, flags, selected, deps.registry)
			resolved.SelectorDecisionID = decision.DecisionID
			resolved.SelectorRuntime = decision.Runtime
		} else {
			resolved, err = files.Complete(frag.Environment, flags, deps.registry)
		}
		if err == nil {
			provider := defaults.ResolveProviderPath()
			if deps.providerPath != nil {
				provider = deps.providerPath()
			}
			_ = defaults.EmitGroupWithProvider(stderr, resolved, provider)
			if deps.selectorConfig.Mode == "shadow" {
				capture := &selectorOutcomeWriter{Writer: stderr}
				deps.hostedSession = func(handle string) { capture.hosted = handle }
				code := launch(ctx, inv, frag, target, resolved, files, stdout, capture, deps)
				observeSelection(deps, resolved, capture.outcome())
				return code
			}
			return launch(ctx, inv, frag, target, resolved, files, stdout, stderr, deps)
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

func launch(ctx context.Context, inv cli.Invocation, frag *fragment.Fragment, target mapping.Target, resolved defaults.Resolved, files defaults.Files, stdout, stderr io.Writer, deps launchDeps) int {
	selected, err := resolveHost(ctx, inv, frag, stderr, deps)
	if err != nil {
		return emitHostFailure(stderr, err)
	}
	if selected.terminal != nil {
		defer selected.terminal.Close()
	}
	deps.hostedSelection = &selected
	isHosted := selected.route.Kind != "native"
	if !isHosted && inv.A2A != "" {
		return emitFailure(stderr, diagnostics.CodeUsage, errors.New("--a2a requires hosted execution"))
	}
	if !isHosted && (inv.BoardGoal || inv.Detach) {
		return emitFailure(stderr, diagnostics.CodeUsage, errors.New("--board-goal and --detach require hosted execution"))
	}
	if !isHosted && (inv.ResumeRequested || inv.ResumeIDSet) {
		return emitFailure(stderr, diagnostics.CodeUsage, errors.New("resume selectors require hosted execution; use --hosted or a hosted default"))
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
	tail := inv.Native
	intent := agentic.ResumeIntent{Kind: agentic.ResumeNew}
	if isHosted {
		tail, intent = selected.tail, selected.intent
	}
	if isHosted && intent.Kind != agentic.ResumeNew {
		startShadowSelection(deps)
		return runDelegated(ctx, inv, frag, resolved, decision, wd, tail, intent, stdout, stderr, deps)
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
	admitted, err := plan.Build(ctx, plan.Deps{Registry: deps.registry, BuildLaunch: build, Availability: availability, ModuleVersion: deps.moduleVersion}, plan.Request{
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
	// Muse's no-evidence mapping deliberately refuses yolo queries. Only the
	// admitted plan can establish the selected binary's bounded help evidence;
	// report that authority without guessing or parsing a provider flag.
	if !isHosted && decision.Mode == agentic.PermissionModeYolo && system.ID() == "muse" {
		fmt.Fprintf(stderr, "curator-run: permissions=%s source=%s mapped=module-attested\n", decision.Mode, decision.Source)
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
	sessionAllowed := deps.networkSessionAllowed
	if sessionAllowed == nil {
		sessionAllowed = func(hosted bool) bool { return !hosted }
	}
	var snapshot *execution.ArtifactSnapshot
	if inv.NetworkSet && sessionAllowed(isHosted) {
		snapshot, err = execution.SnapshotArtifact(admitted.Plan.Binary, admitted.Plan.WorkDir, admitted.Plan.Env)
		if err != nil {
			return emitNetworkFailure(stderr, err)
		}
		defer snapshot.Close()
	}
	var artifact []byte
	var hostIdentity binding.AdapterIdentity
	if snapshot != nil {
		artifact = snapshot.Bytes()
		hostIdentity = binding.AdapterIdentity{Adapter: "generic-env-v1", Harness: target.System, Build: snapshot.BuildID(), Entrypoint: entrypoint}
	}
	preparedNet, err := network.Prepare(ctx, network.Request{
		Explicit: inv.Network, ExplicitSet: inv.NetworkSet,
		Hosted: !sessionAllowed(isHosted), Harness: target.System,
		Entrypoint: entrypoint,
		ParentEnv:  parentEnv, Prober: deps.prober,
		Artifact: artifact, HostIdentity: hostIdentity, Mode: adapterprobe.Mode(inv.NetworkMode),
		PinnedBuild: inv.NetworkPin, KnownBadPath: inv.NetworkKnownBad, ScopeCheck: deps.networkScopeCheck, Evaluate: deps.networkEvaluate, MatchIdentity: deps.networkMatchIdentity,
		FragEnv: frag.Env, PromptEnv: prompt.Env, MCPEnvNames: mcpNames,
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
	startShadowSelection(deps)
	if snapshot != nil {
		value.Binary = snapshot.Path()
	}
	emitAdapter := deps.networkEmit
	if emitAdapter == nil {
		emitAdapter = network.EmitAdapterProvenance
	}
	if err := emitAdapter(stderr, preparedNet.Provenance); err != nil {
		return emitNetworkFailure(stderr, refusal.New(refusal.CodeScopeUnsupported, "adapter", "provenance emission failed"))
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
		if selected.route.Kind == "delegated" {
			for _, warning := range value.Warnings {
				fmt.Fprintln(stderr, name+": "+warning)
			}
			if err := boundary(); err != nil {
				return emitFailure(stderr, diagnostics.CodePlanRefused, err)
			}
			return runDelegated(ctx, inv, frag, resolved, decision, wd, tail, intent, stdout, stderr, deps)
		}
		return runHosted(inv, frag, target, value, intent, inspection, admitted.Plan, decision, parentEnv, now(), stdout, stderr, deps, boundary, selected.terminal)
	}
	return runNative(value, decision, inspection, stdout, stderr, deps, boundary)
}

// runNative reports inspected policy and runs the composed value directly.
// It performs zero manager contact; hosted failures never reach it.
func runNative(value composition.Value, decision execution.PermissionDecision, inspection agentic.StoredPolicyInspection, stdout, stderr io.Writer, deps launchDeps, boundary func() error) int {
	if decision.Mode == agentic.PermissionModeNative {
		execution.ReportEffectiveNativePolicy(stderr, inspection)
	}
	prepared := execution.Prepare(value)
	return prepared.Run(execution.Options{IO: execution.IO{Stdin: deps.stdin, Stdout: stdout, Stderr: stderr}, Boundary: boundary})
}

// runHosted sends the sealed exact plan through the selected plugin. The caller
// owns preopenedTTY and closes it after the terminal exchange.
func runHosted(inv cli.Invocation, frag *fragment.Fragment, target mapping.Target, value composition.Value, intent agentic.ResumeIntent, inspection agentic.StoredPolicyInspection, base agentic.Plan, decision execution.PermissionDecision, parentEnv []string, at time.Time, stdout, stderr io.Writer, deps launchDeps, boundary func() error, preopenedTTY *os.File) int {
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
	hostName := inv.SessionLabel
	if inv.SessionLabel == "" {
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
	result, err := deps.hostedSelection.client.Terminal(context.Background(), "launch-exact", wire, terminalStreams(preopenedTTY, stdout, stderr, deps.stdin))
	if err != nil {
		return emitHostFailure(stderr, err)
	}
	if result.Refusal != nil {
		return emitHostFailure(stderr, result.Refusal)
	}
	return result.Exit
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
	headless := execution.HasNonInteractiveMarker(parentEnv) || !terminal

	toolRelease := ""
	var probeErr error
	decision, err := execution.ResolvePermission(execution.PermissionRequest{
		Flag: inv.PermissionMode, FlagPresent: inv.PermissionSet, Profile: frag.Permissions,
		Global: global, Headless: headless,
		Transport: frag.Revision == fragment.IdentityV2 || frag.Revision == fragment.IdentityV3 || frag.Revision == fragment.IdentityV4,
	})
	if err != nil {
		return execution.PermissionDecision{}, "", nil, err
	}
	if toolRelease == "" {
		toolRelease, probeErr = agentic.ProbeToolRelease(ctx, system, parentEnv)
	}
	permissionMapping, mappingErr := agentic.Default.PermissionMapping(agentic.SystemID(target.System), toolRelease, decision.Mode)
	deferredMapping := errors.Is(mappingErr, museSystem.ErrMuseHelpEvidenceRequired)
	if deferredMapping {
		// v0.5.62 requires live help evidence for Muse yolo. BuildLaunch owns
		// that probe and refusal; a no-evidence query is not an admission.
		mappingErr = nil
	}
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
	if !hosted && !deferredMapping {
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
