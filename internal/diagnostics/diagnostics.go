// Package diagnostics implements the stable SPEC §6 diagnostic contract:
// the closed code families, the exit-code rule, and the deterministic
// stderr rendering.
//
// This package produces no diagnostics of its own. Every code is owned by
// the pipeline stage named below; the constants here transcribe the SPEC
// §6 table verbatim, and TestOwnedCodesMatchOwners pins each owned literal
// to its owner's constant or method, so the owner stays the single source
// of truth. New behavior must reuse those owners' error contracts rather
// than duplicating them here.
//
// Closed families (code → owner → production call site):
//
//	usage                          cli.UsageError              cli.Parse (exit 2)
//	resolve_invocation_failed      fragment.ResolveError       fragment.Resolver.Resolve (exit 1)
//	resolve_environment_unknown    fragment.ResolveError       fragment.Resolver.Resolve (exit 1)
//	resolve_profile_unknown        fragment.ResolveError       fragment.Resolver.Resolve (exit 1)
//	resolve_repair_failed          fragment.ResolveError       fragment.Resolver.Resolve (exit 1)
//	resolve_lock_unavailable       fragment.ResolveError       fragment.Resolver.Resolve (exit 1)
//	resolve_fragment_invalid       fragment.ResolveError       fragment.Resolver.Resolve (exit 1)
//	defaults_config_invalid        axconfig.Error              axconfig.Load (exit 1)
//	defaults_unresolvable          (no producer in this build) separately owned (exit 1)
//	plan_refused                   (no producer in this build) separately owned (exit 1)
//	plan_provider_limited          (no producer in this build) separately owned (exit 1)
//	env_unsupported                mapping (constant)          mapping.Resolve (exit 1)
//	exec_provider_missing          execution (sentinel text)   execution.Launch.Run (exit 1)
//	ax_handoff_failed              execution (sentinel text)   execution.Launch.Run (exit 1)
//	mcp_layer_missing              composition.LayerError      composition.Value.CheckLaunchBoundary (exit 1)
//	mcp_layer_unreadable           composition.LayerError      composition.Value.CheckLaunchBoundary (exit 1)
//	sysprompt_channel_unavailable  systemprompt.Refusal        systemprompt.Select via PrepareLaunch (exit 1)
//	sysprompt_file_unreadable      systemprompt.Refusal        systemprompt.ProbeFiles via PrepareLaunch (exit 1)
//
// Two invariants hold across every family (SPEC §6). First, an absence and
// a failure to read are different facts: a fallback defined for absence
// never fires on a failed or malformed read, and where absence is itself
// the failure (mcp_layer_missing) it carries its own code rather than
// sharing the read failure's. Second, no diagnostic downgrades the launch:
// every failure is terminal for that invocation. ExitForCode encodes the
// second invariant: any error code — including one outside the closed set,
// which signals a caller bug rather than a new contract — exits 1, never
// 0. CodeOf encodes the first half of the first: an error this layer does
// not recognize yields no code at all rather than an invented one.
//
// Warnings (`warning: <code>: <detail>`) and child/subprocess stderr are
// transport, not diagnostics: IsDiagnosticLine distinguishes a launcher
// code line from them so a warning is never mistaken for the contract.
//
// Byte rule: Line frames launcher-owned detail (see Line). Forwarded
// bytes — Curator's stderr, provider verdict evidence, ax Structured
// Error payloads — are written verbatim by their owners and never pass
// through Line, so this package transforms no foreign bytes.
//
// Bound: CodeOf has no production caller in this build; main classifies
// through the owners' own predicates (cli.IsUsage, fragment.IsResolve)
// and chooses the mapping/execution codes at their call sites. CodeOf is
// the API-only statement of the same closed contract for the later
// pipeline families.
package diagnostics

import (
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/relux-works/curator-agent-launcher/internal/axconfig"
	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/systemprompt"
)

// Process exit statuses of SPEC §6: usage errors exit 2; every operational
// failure exits 1. ExitUsage equals cli.ExitCode by contract (pinned).
const (
	ExitUsage       = 2
	ExitOperational = 1
)

// Stable diagnostic codes of SPEC §6, in table order. Owned literals are
// pinned to their owners in tests; the three separately-owned codes have
// no producer in this build and name their pending call site in
// RemainingObligations.
const (
	CodeUsage = "usage"

	CodeResolveInvocationFailed   = "resolve_invocation_failed"
	CodeResolveEnvironmentUnknown = "resolve_environment_unknown"
	CodeResolveProfileUnknown     = "resolve_profile_unknown"
	CodeResolveRepairFailed       = "resolve_repair_failed"
	CodeResolveLockUnavailable    = "resolve_lock_unavailable"
	CodeResolveFragmentInvalid    = "resolve_fragment_invalid"

	CodeDefaultsInvalid      = "defaults_config_invalid"
	CodeDefaultsUnresolvable = "defaults_unresolvable"

	CodePlanRefused         = "plan_refused"
	CodePlanProviderLimited = "plan_provider_limited"

	CodeEnvUnsupported = "env_unsupported"

	CodeExecMissing = "exec_provider_missing"

	CodeAxHandoffFailed = "ax_handoff_failed"

	CodeMCPLayerMissing    = "mcp_layer_missing"
	CodeMCPLayerUnreadable = "mcp_layer_unreadable"

	CodeSyspromptUnavailable = "sysprompt_channel_unavailable"
	CodeSyspromptUnreadable  = "sysprompt_file_unreadable"
)

// Codes returns the closed code set in SPEC §6 table order.
func Codes() []string {
	return []string{
		CodeUsage,
		CodeResolveInvocationFailed,
		CodeResolveEnvironmentUnknown,
		CodeResolveProfileUnknown,
		CodeResolveRepairFailed,
		CodeResolveLockUnavailable,
		CodeResolveFragmentInvalid,
		CodeDefaultsInvalid,
		CodeDefaultsUnresolvable,
		CodePlanRefused,
		CodePlanProviderLimited,
		CodeEnvUnsupported,
		CodeExecMissing,
		CodeAxHandoffFailed,
		CodeMCPLayerMissing,
		CodeMCPLayerUnreadable,
		CodeSyspromptUnavailable,
		CodeSyspromptUnreadable,
	}
}

var valid = func() map[string]bool {
	m := map[string]bool{}
	for _, c := range Codes() {
		m[c] = true
	}
	return m
}()

// Valid reports whether code is a member of the closed set. Codes outside
// the set — including Curator's environment_home_stale, which SPEC §4.1
// declares unreachable from a --repair invocation — are not launcher
// diagnostics.
func Valid(code string) bool { return valid[code] }

// ExitForCode maps a diagnostic code to its process exit status: usage
// exits 2, everything else exits 1. An unknown code still exits 1: a bug
// in code selection stays terminal and visible, never silent success and
// never a usage report.
func ExitForCode(code string) int {
	if code == CodeUsage {
		return ExitUsage
	}
	return ExitOperational
}

// resolveFamily, layerFamily, and refusalFamily are the closed code sets
// each typed owner may speak. They transcribe the owner constants, so the
// owner stays the single source of truth; a typed error carrying any
// other code — another family's member, a Curator-internal code, or an
// invented one — is a caller bug and yields no code.
var resolveFamily = map[string]bool{
	fragment.CodeInvocationFailed:   true,
	fragment.CodeEnvironmentUnknown: true,
	fragment.CodeProfileUnknown:     true,
	fragment.CodeRepairFailed:       true,
	fragment.CodeLockUnavailable:    true,
	fragment.CodeFragmentInvalid:    true,
}

var layerFamily = map[string]bool{
	composition.CodeMCPLayerMissing:    true,
	composition.CodeMCPLayerUnreadable: true,
}

var refusalFamily = map[string]bool{
	systemprompt.CodeUnavailable: true,
	systemprompt.CodeUnreadable:  true,
}

// chain walks err breadth-first through Unwrap() error and
// Unwrap() []error links, calling fn at every level until it reports a
// code, so wrapping (including errors.Join) preserves the classification.
// A typed-nil link ends its branch with no match: several owners'
// Unwrap methods dereference the receiver, so the stdlib errors.As walk
// panics on them, while this guarded walk fails closed instead.
func chain(err error, fn func(error) (string, bool)) (string, bool) {
	queue := []error{err}
	for len(queue) > 0 {
		e := queue[0]
		queue = queue[1:]
		if e == nil || nilValue(e) {
			continue
		}
		if code, ok := fn(e); ok {
			return code, true
		}
		switch u := e.(type) {
		case interface{ Unwrap() []error }:
			queue = append(queue, u.Unwrap()...)
		case interface{ Unwrap() error }:
			queue = append(queue, u.Unwrap())
		}
	}
	return "", false
}

// nilValue reports whether e is a typed nil: a non-nil error interface
// holding a nil pointer, map, slice, channel, function, or interface.
func nilValue(e error) bool {
	switch v := reflect.ValueOf(e); v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// CodeOf extracts the stable diagnostic code from a production error
// without inventing one. It recognizes the concrete error types of the
// owning stages at every level of their wrap chains, and accepts only
// the owner's own closed family. Mapping failures (plain errors under
// mapping.CodeUnsupported) and execution sentinels
// (exec_provider_missing, ax_handoff_failed) are chosen explicitly at
// their call sites and are not classified here. An error no owner claims
// — including a bare fragment parse error, which only becomes a
// diagnostic when the resolver wraps it — yields ok=false. Typed-nil
// owner values (a non-nil error interface holding a nil pointer, direct
// or wrapped) also yield ok=false rather than panicking.
func CodeOf(err error) (code string, ok bool) {
	return chain(err, func(e error) (string, bool) {
		if ue, ok := e.(*cli.UsageError); ok && ue != nil && ue.Code() == CodeUsage {
			return CodeUsage, true
		}
		if re, ok := e.(*fragment.ResolveError); ok && re != nil && resolveFamily[re.Code] {
			return re.Code, true
		}
		if le, ok := e.(*composition.LayerError); ok && le != nil && layerFamily[le.Code] {
			return le.Code, true
		}
		if sr, ok := e.(*systemprompt.Refusal); ok && sr != nil && refusalFamily[sr.Code] {
			return sr.Code, true
		}
		if ae, ok := e.(*axconfig.Error); ok && ae != nil {
			return CodeDefaultsInvalid, true
		}
		return "", false
	})
}

// Line renders one deterministic diagnostic code line: the executable
// name, the stable code, and the human-oriented detail. The code, not the
// prose, is the contract.
//
// Framing rule: detail is launcher-owned text and may carry operator or
// subprocess bytes (a profile name, an argv rendering, a start error),
// so every CR and LF inside it is folded to a LF followed by two spaces.
// The first line of the result is therefore always the only code line;
// each continuation line starts with whitespace, so IsDiagnosticLine
// never recognizes it — even when the embedded bytes spell a code line.
// Only detail is framed: code is chosen by the call site from the closed
// set, and forwarded child/provider/ax bytes never pass through Line.
func Line(code, detail string) string {
	detail = strings.ReplaceAll(detail, "\r\n", "\n")
	detail = strings.ReplaceAll(detail, "\r", "\n")
	detail = strings.ReplaceAll(detail, "\n", "\n  ")
	return cli.Name + ": " + code + ": " + detail + "\n"
}

// Emit writes Line to w. It validates nothing: the call site owns the
// family-correct code, and even a caller bug stays terminal through
// ExitForCode. The returned error is the write error, if any.
func Emit(w io.Writer, code, detail string) error {
	_, err := fmt.Fprint(w, Line(code, detail))
	return err
}

// IsDiagnosticLine reports whether line is a single launcher diagnostic
// code line: anchored at the executable name, carrying a closed-set code,
// followed by the detail separator. Warning lines, Curator's own
// `curator: <code>: <detail>` lines, and child stderr never satisfy it,
// even when they embed a launcher code token.
func IsDiagnosticLine(line string) bool {
	if strings.Contains(line, "\n") {
		return false
	}
	rest, ok := strings.CutPrefix(line, cli.Name+": ")
	if !ok {
		return false
	}
	code, _, found := strings.Cut(rest, ":")
	code = strings.TrimSuffix(code, " ")
	if !found || code == "" {
		return false
	}
	return Valid(code)
}

// RemainingObligations names the production call sites this contract
// covers but this build does not yet wire into the launcher entry point.
// Each is owned by the task in parentheses; wiring one means choosing the
// named family code at that boundary and exiting through ExitForCode, with
// no fallback to a weaker launch shape.
func RemainingObligations() []string {
	return []string{
		"axconfig.Load before cli.Parse, so a present-but-unreadable ax.json reports defaults_config_invalid even when argv is also a usage error (TASK-260908-1o7i8y)",
		"defaults.json read, locked-member usage refusal, lineup fallback, defaults_unresolvable, and the per-member stderr line-group (TASK-260909-2vy977)",
		"vendorplugin.BuildLaunch admission and providerlimits.Store.AvailabilityFor verdict enforcement as plan_refused / plan_provider_limited with verbatim verdict evidence (TASK-260908-2so46q)",
		"composition.Value.CheckLaunchBoundary immediately before both ax handoff and direct exec, with the binary check and systemprompt.PrepareLaunch as the execution Boundary (TASK-260908-1o7i8y)",
		"systemprompt.PrepareLaunch selection, file-kind probe, and warning emission on every launch (TASK-260908-1o7i8y)",
	}
}
