// Package network implements SPEC §4.4b, the network-profile stage of the
// launcher: an explicit --network selection is resolved and validated
// against the operator catalog after plan admission, a bounded preflight
// probes the proxy for a real supported direct launch under a proxy
// profile, and the binding's patch is returned for final application in
// Compose while its Record becomes stderr provenance. A named
// kind="direct" profile skips every probe step and binds an unset-only
// patch. Tracked mode refuses: the source host cannot validate a
// destination.
//
// Support is the STRICT network policy: the launch shape is verified at
// its ACTUAL entrypoint — the EFFECTIVE shape of the admitted launch,
// never collapsed — against an exact allowlist. The network-profiles
// verification covers the one-shot print shape only, so an interactive
// launch refuses network_scope_unsupported until pinned compatibility
// evidence covers interactive mode, while an explicit print invocation
// stays admitted (see EffectiveEntrypoint).
//
// The package consumes curator-network-profiles v0.2.1 by tag (a normal
// require, no replace) and creates no processes. Every error is a
// *refusal.Refusal and terminates the launch without weaker routing: an
// error here never degrades to an unmanaged launch. Without an explicit
// selection the outcome is unmanaged — an empty patch, a nil Record — and
// this package touches nothing: no catalog read, no probe.
package network

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/relux-works/curator-network-profiles/pkg/binding"
	"github.com/relux-works/curator-network-profiles/pkg/catalog"
	"github.com/relux-works/curator-network-profiles/pkg/envpatch"
	"github.com/relux-works/curator-network-profiles/pkg/netprofile"
	"github.com/relux-works/curator-network-profiles/pkg/probe"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
	"github.com/relux-works/curator-network-profiles/pkg/resolve"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

// EntrypointExec is the one-shot entrypoint: the `claude -p` shape, the
// only shape the pinned network-profiles verification covers. A plan
// plan.Build constructs as interactive still carries this effective
// shape when its argv tail selects print (see EffectiveEntrypoint).
const EntrypointExec = "exec"

// EntrypointInteractive is the terminal-session entrypoint: the shape
// of an admitted launch whose effective command selects no print form.
// No pinned verification covers it in this revision, so a managed
// interactive launch refuses network_scope_unsupported. Collapsing it
// onto EntrypointExec would admit an unverified scope;
// EffectiveEntrypoint keeps them distinct.
const EntrypointInteractive = "interactive"

// claudeCodeHarness is the mapped agentic system id whose native print
// selectors (-p/--print) the effective-shape derivation reads. It is
// the harness member of the verified STRICT tuple below; no other
// harness has a verified print shape in this revision.
const claudeCodeHarness = "claude-code"

// strictVerifiedAdapters is the STRICT network policy: the exact
// (adapter, harness, build, entrypoint) tuples a --network launch may
// probe and bind. Membership is exact on all four members; the build is
// the probed tool release as normalized by the agents-management tool
// probe, compared verbatim — no prefix, range, or "non-empty" rule — and
// the entrypoint is the effective entrypoint of the actual admitted
// launch (see EffectiveEntrypoint), never a collapsed constant.
//
// The list holds exactly one verified tuple: the network-profiles
// verification of claude-code build 2.1.287 under the generic-env-v1
// adapter in the one-shot print shape. Every other tuple — codex, muse,
// any other build, any interactive launch — refuses
// network_scope_unsupported. Tests exercise other tuples with an
// injected test-only list (Request.Allowlist), never by editing this
// list. Adding a tuple requires its pinned compatibility evidence
// beside the edit. An optimistic policy is a future option
// (TASK-261005-yoogtw), not the default.
var strictVerifiedAdapters = []binding.AdapterIdentity{{
	Adapter:    envpatch.AdapterGeneric,
	Harness:    "claude-code",
	Build:      "2.1.287",
	Entrypoint: EntrypointExec,
}}

// VerifiedAdapters returns a copy of the STRICT network policy: the
// exact adapter identities a --network launch may bind. It holds exactly
// the network-profiles verified tuple; see strictVerifiedAdapters.
func VerifiedAdapters() []binding.AdapterIdentity {
	return append([]binding.AdapterIdentity(nil), strictVerifiedAdapters...)
}

// EntrypointForMode maps the CONSTRUCTION mode of an admitted plan to
// the policy's entrypoint vocabulary: the one-shot child process owns
// the verified entrypoint, the terminal session owns the unverified one.
// Any other mode — dry-run, managed session, or an invalid value —
// refuses typed network_scope_unsupported: an unmapped mode never
// inherits a verified entrypoint. Production derives the entrypoint
// with EffectiveEntrypoint, which refines the interactive arm to the
// effective shape; this mapping stays the vocabulary for every other
// mode.
func EntrypointForMode(mode agentic.LaunchMode) (string, error) {
	switch mode {
	case agentic.LaunchModeExec:
		return EntrypointExec, nil
	case agentic.LaunchModeInteractive:
		return EntrypointInteractive, nil
	default:
		return "", refusal.New(refusal.CodeScopeUnsupported, mode.String(), "no verified network adapter tuple for this launch mode")
	}
}

// EffectiveEntrypoint derives the policy entrypoint for the ACTUAL
// admitted launch: the construction mode refined to the effective
// shape. plan.Build constructs interactive launches only, and the
// module forwards the native tail verbatim into the interactive argv —
// so an explicit print invocation (the tail carries -p/--print in flag
// position) IS the verified `claude -p` entrypoint and classifies as
// EntrypointExec, while a tail without a print selector classifies as
// EntrypointInteractive and refuses downstream. The native tail is read
// only as the plan carries it: the derivation scans the plan argv's own
// tail and requires it to equal the admitted native request, so a
// drifted plan falls back to its construction mode instead of
// classifying a tail the effective command does not carry. Detection
// applies to claude-code only — the sole harness with a verified print
// shape; every other harness keeps its mode mapping. The module's
// typed-intent API (ClassifyNonInteractiveArgs) cannot serve this
// derivation: its release gate verifies the permission-grammar rows
// (claude 2.1.261) while the STRICT policy verifies the network build
// (2.1.287), so it errors on exactly the verified scope. The scan
// below therefore mirrors that API's pinned grammar rule instead (see
// hasPrintSelector); it admits a strict subset of what the module
// would classify as print, and refuses everything else.
func EffectiveEntrypoint(plan agentic.Plan, harness string, native []string) (string, error) {
	if plan.Mode != agentic.LaunchModeInteractive {
		return EntrypointForMode(plan.Mode)
	}
	if harness == claudeCodeHarness && hasEffectivePrintSelector(plan.Argv, native) {
		return EntrypointExec, nil
	}
	return EntrypointInteractive, nil
}

// hasEffectivePrintSelector reports whether the plan's own argv tail
// selects the print shape for the admitted native request. The tail is
// the last len(native) argv elements and must equal native exactly: a
// plan that drifted from its request (or is shorter than it) carries
// no readable selection and the construction mode stands. Stated bound:
// only the request the plan records is classified; composition refuses
// a drifted plan before any child starts.
func hasEffectivePrintSelector(argv, native []string) bool {
	if len(argv) < len(native) {
		return false
	}
	tail := argv[len(argv)-len(native):]
	for i := range tail {
		if tail[i] != native[i] {
			return false
		}
	}
	return hasPrintSelector(tail)
}

// hasPrintSelector mirrors the pinned module grammar's print rule: the
// claude classifier reports print when a flag-position token's pre-"="
// name is -p or --print, and flag parsing ends at the first standalone
// "--" (the nativeargs separator rule). A bare flag-shaped token
// consumes the next token as its value, so a print token in value
// position never admits. That consumption is a fail-closed
// approximation of the module's exact arity table: where the module
// would read a boolean flag before -p as print, this reads the
// selector as consumed and refuses — a refused real print, never an
// admitted interactive shape. Combined short clusters may likewise
// over-consume and refuse. Post-"--" prompt text, however flag-like,
// never admits.
func hasPrintSelector(tail []string) bool {
	for i := 0; i < len(tail); i++ {
		token := tail[i]
		if token == "--" {
			return false
		}
		if name, _, _ := strings.Cut(token, "="); name == "-p" || name == "--print" {
			return true
		}
		if !strings.Contains(token, "=") && len(token) > 1 && token[0] == '-' {
			i++
		}
	}
	return false
}

// Request carries the inputs of one network stage. Explicit is the
// --network value; ExplicitSet reports presence. Tracked, Harness,
// Build and Entrypoint describe the admitted launch shape: the ax mode,
// the mapped agentic system id, the probed tool release, and the
// effective entrypoint of the ACTUAL admitted launch — derived by the
// caller with EffectiveEntrypoint from the admitted plan, harness and
// native tail, never a collapsed constant. An empty entrypoint matches
// nothing and refuses.
// ParentEnv is the operator's original environment the catalog is read
// through. Prober runs the bounded preflight; nil selects the production
// dialer. Allowlist overrides the STRICT support policy when non-nil;
// nil selects strictVerifiedAdapters, so production passes nil and tests
// inject a test-only list. FragEnv, PromptEnv and MCPEnvNames are the
// composed overlay names the patch follows; a proxy-family name among
// them is a managed-only conflict, refused before the preflight.
// EngineHosts are the loopback engine hosts of this launch; each must be
// covered by the profile's bypass_hosts.
type Request struct {
	Explicit    string
	ExplicitSet bool
	Tracked     bool
	Harness     string
	Build       string
	Entrypoint  string
	ParentEnv   []string
	Prober      probe.Prober
	Allowlist   []binding.AdapterIdentity
	FragEnv     map[string]string
	PromptEnv   map[string]string
	MCPEnvNames []string
	EngineHosts []string
}

// Prepared is the outcome of one network stage: the patch for final
// application in Compose (empty when unmanaged) and the Record for
// provenance (nil when unmanaged).
type Prepared struct {
	Patch  envpatch.Patch
	Record *binding.Record
}

// Prepare resolves, validates, preflights and binds one explicit
// selection. Without an explicit selection it returns the unmanaged
// outcome and touches nothing: no catalog read, no probe. Order, closed:
//
//  1. a blank explicit selection refuses without any I/O;
//  2. a tracked selection refuses without catalog or probe I/O: the
//     source host cannot validate a destination;
//  3. the catalog loads through the operator's original parentEnv;
//  4. the explicit selection resolves (allowed set, existence,
//     assurance, confirmation, engine coverage);
//  5. the launch shape is verified against the STRICT support policy:
//     the exact (adapter, harness, build, entrypoint) tuple — at the
//     actual admitted entrypoint — must be listed;
//  6. a proxy-family name in a composed overlay refuses without probing:
//     the conflict is pure, so it never costs a network probe;
//  7. a bounded preflight probes the resolved endpoint — unless the
//     resolved profile is kind="direct", which skips every probe step:
//     the prober is not invoked and the Record carries
//     skipped/skipped/skipped;
//  8. the generic patch binds — unset-only with an empty set for direct —
//     and the Record is built from the probe statuses and time only —
//     never the endpoint or environment.
//
// Every error is a *refusal.Refusal. Admission failure in the caller
// never reaches this function, so a refused plan invokes neither the
// prober here nor the workload.
func Prepare(ctx context.Context, req Request) (Prepared, error) {
	if !req.ExplicitSet {
		return Prepared{}, nil
	}
	if strings.TrimSpace(req.Explicit) == "" {
		return Prepared{}, refusal.New(refusal.CodeProfileInvalid, req.Explicit, "profile name must not be blank")
	}
	if req.Tracked {
		return Prepared{}, refusal.New(refusal.CodeScopeUnsupported, req.Explicit, "tracked launches cannot carry a network profile in this revision; the destination does not enforce it")
	}
	cat, err := catalog.Load(req.ParentEnv)
	if err != nil {
		return Prepared{}, err
	}
	result, err := cat.Resolve(resolve.Request{Explicit: req.Explicit, EngineHosts: req.EngineHosts})
	if err != nil {
		return Prepared{}, err
	}
	if !result.Managed {
		// Unreachable through the production parser: a non-blank
		// explicit selection always selects. Fail closed rather than
		// launching unmanaged under --network.
		return Prepared{}, refusal.New(refusal.CodeScopeUnsupported, req.Explicit, "no network binding was resolved")
	}
	allow := req.Allowlist
	if allow == nil {
		allow = strictVerifiedAdapters
	}
	identity, err := IdentifyWith(req.Harness, req.Build, req.Entrypoint, allow)
	if err != nil {
		return Prepared{}, err
	}
	if err := CheckOverlays(req.FragEnv, req.PromptEnv, req.MCPEnvNames); err != nil {
		return Prepared{}, err
	}
	// A direct profile has no endpoint to probe: every probe step is
	// skipped and the prober is not invoked. The Record below carries
	// the skipped outcome with this launch's observation time.
	res := probe.Result{
		TCP: probe.StatusSkipped, Connect: probe.StatusSkipped,
		TLS: probe.StatusSkipped, CheckedAt: time.Now().UTC(),
	}
	if result.Profile.Kind != netprofile.KindDirect {
		prober := req.Prober
		if prober == nil {
			prober = DefaultProber()
		}
		var err error
		res, err = prober.Probe(ctx, probe.Request{
			Subject:  result.Profile.Name,
			Endpoint: result.Profile.Endpoint,
			Target:   result.Profile.ProbeTarget,
			Timeout:  probe.DefaultTimeout,
		})
		if err != nil {
			return Prepared{}, err
		}
	}
	patch := envpatch.Generic{}.Patch(result.Profile)
	bound := binding.Binding{
		ProfileRef:       result.Selection.ProfileRef,
		ProfileDigest:    result.Digest,
		AdapterIdentity:  identity,
		Assurance:        result.Assurance,
		ResolvedEndpoint: result.Profile.Endpoint,
		EnvPatch:         patch,
	}
	record := bound.Record(string(result.Selection.Origin), &binding.ProbeRecord{
		TCP:       string(res.TCP),
		Connect:   string(res.Connect),
		TLS:       string(res.TLS),
		CheckedAt: res.CheckedAt,
	})
	return Prepared{Patch: patch, Record: &record}, nil
}

// Identify verifies the launch shape against the STRICT support policy
// and returns the adapter identity the binding carries. Only the
// verified claude-code print tuple is admitted; see
// strictVerifiedAdapters.
func Identify(harness, build, entrypoint string) (binding.AdapterIdentity, error) {
	return IdentifyWith(harness, build, entrypoint, strictVerifiedAdapters)
}

// DefaultProber is the production prober Prepare selects when the
// request carries none: the library's dialer with its production
// transport. Tests assert this wiring by type without performing I/O;
// no test dials through it.
func DefaultProber() probe.Prober { return &probe.Dialer{} }

// IdentifyWith verifies the launch shape against allow: the exact
// (generic-env-v1, harness, build, entrypoint) tuple must be listed,
// compared member-wise with no prefix, range, or non-emptiness rule.
// Anything else — an unknown harness, an unknown or unparsable version,
// a missing build, an unverified entrypoint — refuses typed
// network_scope_unsupported before any probe or workload. The entrypoint
// is the caller's actual admitted shape (see EffectiveEntrypoint); an
// empty entrypoint matches nothing and refuses.
func IdentifyWith(harness, build, entrypoint string, allow []binding.AdapterIdentity) (binding.AdapterIdentity, error) {
	id := binding.AdapterIdentity{
		Adapter:    envpatch.AdapterGeneric,
		Harness:    harness,
		Build:      build,
		Entrypoint: entrypoint,
	}
	for _, v := range allow {
		if v == id {
			return id, nil
		}
	}
	return binding.AdapterIdentity{}, refusal.New(refusal.CodeScopeUnsupported, harness, "no verified network adapter tuple for this harness, build and entrypoint")
}

// IsProxyFamily reports whether name belongs to the reserved proxy
// family in any letter case. Composition's managed-only overlay refusal
// delegates here, so the two checks cannot drift.
func IsProxyFamily(name string) bool {
	for _, u := range envpatch.UnsetNames() {
		if strings.EqualFold(name, u) {
			return true
		}
	}
	return false
}

// CheckOverlays rejects a proxy-family name in any overlay that the §4.4b
// patch follows: the fragment env, the prompt channel env, or MCP
// env_names. Matching is case-insensitive, so mixed-case spellings the
// fragment's own reserved-name exclusion admits are still refused while
// managed. Inherited proxy values in the plan environment are not
// overlays and are replaced normally by the patch. Prepare calls this
// after support verification and before the preflight, so a conflict
// never costs a network probe; ComposeWithNetwork rechecks the same
// names at application time.
func CheckOverlays(fragEnv, promptEnv map[string]string, mcpEnvNames []string) error {
	for _, name := range sortedKeys(fragEnv) {
		if IsProxyFamily(name) {
			return refusal.New(refusal.CodeConfigurationConflict, name, "the fragment names a proxy variable while a network profile is managed")
		}
	}
	for _, name := range sortedKeys(promptEnv) {
		if IsProxyFamily(name) {
			return refusal.New(refusal.CodeConfigurationConflict, name, "the prompt channel names a proxy variable while a network profile is managed")
		}
	}
	for _, name := range mcpEnvNames {
		if IsProxyFamily(name) {
			return refusal.New(refusal.CodeConfigurationConflict, name, "mcp env_names names a proxy variable while a network profile is managed")
		}
	}
	return nil
}

func sortedKeys(m map[string]string) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ProvenanceLine renders the Record as one stderr provenance line. It
// carries the manifest-safe Record members only — never the endpoint,
// the patch, or the environment — so the line is safe to keep in logs.
// Every value is folded with the §4.3 framing rule, so a hostile build
// string never forges a second parseable line. The line is not a
// diagnostic line: "network" is not a diagnostic code. The caller prints
// it only after Compose has succeeded; a launch that Compose refuses
// prints no binding Record.
func ProvenanceLine(r binding.Record) string {
	return "curator-run: network: profile=" + fold(r.ProfileRef) +
		" origin=" + fold(r.Origin) +
		" digest=" + fold(r.ProfileDigest) +
		" adapter=" + fold(r.AdapterIdentity.Adapter) +
		" harness=" + fold(r.AdapterIdentity.Harness) +
		" build=" + fold(r.AdapterIdentity.Build) +
		" entrypoint=" + fold(r.AdapterIdentity.Entrypoint) +
		" assurance=" + fold(r.Assurance) +
		" probe=" + fold(r.Probe.TCP) + "/" + fold(r.Probe.Connect) + "/" + fold(r.Probe.TLS)
}

// Detail renders the human-oriented detail of a network refusal for the
// diagnostic line: the refusal's subject and detail without the repeated
// code. A non-refusal error renders verbatim.
func Detail(err error) string {
	if err == nil {
		return ""
	}
	r, ok := refusal.As(err)
	if !ok || r == nil {
		return err.Error()
	}
	return strings.TrimPrefix(r.Error(), r.Code+": ")
}

func fold(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\n", "\n  ")
}
