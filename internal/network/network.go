// Package network implements SPEC §4.4b, the network-profile stage of the
// launcher: an explicit --network selection is resolved and validated
// against the operator catalog after plan admission, a bounded preflight
// probes the proxy for a real supported direct launch, and the binding's
// patch is returned for final application in Compose while its Record
// becomes stderr provenance. Tracked mode refuses: the source host cannot
// validate a destination.
//
// The package consumes curator-network-profiles v0.1.0 by tag (a normal
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

	"github.com/relux-works/curator-network-profiles/pkg/binding"
	"github.com/relux-works/curator-network-profiles/pkg/catalog"
	"github.com/relux-works/curator-network-profiles/pkg/envpatch"
	"github.com/relux-works/curator-network-profiles/pkg/probe"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
	"github.com/relux-works/curator-network-profiles/pkg/resolve"
)

// EntrypointExec is the entrypoint member of every launcher direct-launch
// adapter identity: the launcher execs the tool.
const EntrypointExec = "exec"

// verifiedAdapters is the production support policy: the exact
// (adapter, harness, build, entrypoint) tuples a --network launch may
// probe and bind. Membership is exact on all four members; the build is
// the probed tool release as normalized by the agents-management tool
// probe, compared verbatim — no prefix, range, or "non-empty" rule.
//
// The list holds exactly one verified tuple: the network-profiles
// verification of claude-code build 2.1.287 under the generic-env-v1
// adapter. Every other tuple — codex, muse, any other build — refuses
// network_scope_unsupported. Tests exercise other tuples with an
// injected test-only list (Request.Allowlist), never by editing this
// list. Adding a tuple requires its pinned compatibility evidence
// beside the edit.
var verifiedAdapters = []binding.AdapterIdentity{{
	Adapter:    envpatch.AdapterGeneric,
	Harness:    "claude-code",
	Build:      "2.1.287",
	Entrypoint: EntrypointExec,
}}

// VerifiedAdapters returns a copy of the production support policy: the
// exact adapter identities a --network launch may bind. It holds exactly
// the network-profiles verified tuple; see verifiedAdapters.
func VerifiedAdapters() []binding.AdapterIdentity {
	return append([]binding.AdapterIdentity(nil), verifiedAdapters...)
}

// Request carries the inputs of one network stage. Explicit is the
// --network value; ExplicitSet reports presence. Tracked, Harness and
// Build describe the admitted launch shape: the ax mode, the mapped
// agentic system id, and the probed tool release. ParentEnv is the
// operator's original environment the catalog is read through. Prober
// runs the bounded preflight; nil selects the production dialer.
// Allowlist overrides the production support policy when non-nil; nil
// selects verifiedAdapters, so production passes nil and tests inject a
// test-only list. FragEnv, PromptEnv and MCPEnvNames are the composed
// overlay names the patch follows; a proxy-family name among them is a
// managed-only conflict, refused before the preflight. EngineHosts are
// the loopback engine hosts of this launch; each must be covered by the
// profile's bypass_hosts.
type Request struct {
	Explicit    string
	ExplicitSet bool
	Tracked     bool
	Harness     string
	Build       string
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
//  5. the launch shape is verified against the support policy: the exact
//     (adapter, harness, build, entrypoint) tuple must be listed;
//  6. a proxy-family name in a composed overlay refuses without probing:
//     the conflict is pure, so it never costs a network probe;
//  7. a bounded preflight probes the resolved endpoint;
//  8. the generic patch binds and the Record is built from the probe
//     statuses and time only — never the endpoint or environment.
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
		allow = verifiedAdapters
	}
	identity, err := IdentifyWith(req.Harness, req.Build, allow)
	if err != nil {
		return Prepared{}, err
	}
	if err := CheckOverlays(req.FragEnv, req.PromptEnv, req.MCPEnvNames); err != nil {
		return Prepared{}, err
	}
	prober := req.Prober
	if prober == nil {
		prober = DefaultProber()
	}
	res, err := prober.Probe(ctx, probe.Request{
		Subject:  result.Profile.Name,
		Endpoint: result.Profile.Endpoint,
		Target:   result.Profile.ProbeTarget,
		Timeout:  probe.DefaultTimeout,
	})
	if err != nil {
		return Prepared{}, err
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

// Identify verifies the launch shape against the production support
// policy and returns the adapter identity the binding carries. Only the
// verified claude-code tuple is admitted; see verifiedAdapters.
func Identify(harness, build string) (binding.AdapterIdentity, error) {
	return IdentifyWith(harness, build, verifiedAdapters)
}

// DefaultProber is the production prober Prepare selects when the
// request carries none: the library's dialer with its production
// transport. Tests assert this wiring by type without performing I/O;
// no test dials through it.
func DefaultProber() probe.Prober { return &probe.Dialer{} }

// IdentifyWith verifies the launch shape against allow: the exact
// (generic-env-v1, harness, build, exec) tuple must be listed, compared
// member-wise with no prefix, range, or non-emptiness rule. Anything
// else — an unknown harness, an unknown or unparsable version, a missing
// build — refuses typed network_scope_unsupported before any probe or
// workload. The entrypoint is always EntrypointExec.
func IdentifyWith(harness, build string, allow []binding.AdapterIdentity) (binding.AdapterIdentity, error) {
	id := binding.AdapterIdentity{
		Adapter:    envpatch.AdapterGeneric,
		Harness:    harness,
		Build:      build,
		Entrypoint: EntrypointExec,
	}
	for _, v := range allow {
		if v == id {
			return id, nil
		}
	}
	return binding.AdapterIdentity{}, refusal.New(refusal.CodeScopeUnsupported, harness, "no verified network adapter tuple for this harness and build")
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
