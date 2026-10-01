// Package composition implements SPEC §4.5 over an already admitted plan.
// It does not build plans, select prompt policy, admit providers, or execute.
package composition

import (
	"encoding/base64"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/network"
	"github.com/relux-works/curator-network-profiles/pkg/envpatch"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

// PromptApplication is the already selected and encoded §5 channel application.
// Its Env contains only engaged variable-channel literals. File channels have
// no argv/env contribution. Selection, reading, encoding and file checks belong
// to §5; this API does not invent a policy for them.
type PromptApplication struct {
	Argv []string
	Env  map[string]string
}

// Stdin is the D4 wire representation. A nil pointer means unattached.
type Stdin struct {
	Encoding string `json:"encoding"`
	Bytes    string `json:"bytes"`
}

// Value carries direct execution data and tracked transport data separately.
// JSON contains only the composition-owned subset of a tracked document; the
// execution layer must add schema_version and extensions before handing to ax.
// Binary is never an argv element. Env is the FULL direct-execution environment
// and must never be serialized into a tracked request.
type Value struct {
	Binary      string               `json:"-"`
	WorkDir     string               `json:"-"`
	Argv        []string             `json:"argv_suffix"`
	Env         []string             `json:"-"`
	RawStdin    agentic.StdinPayload `json:"-"`
	EnvLiterals map[string]string    `json:"env_literals"`
	EnvNames    []string             `json:"env_names"`
	Stdin       *Stdin               `json:"stdin"`
	// Warnings are mode-independent and name variables only. Both execution
	// paths must print them to stderr before launch.
	Warnings []string `json:"-"`
	mcpLayer string
}

// ComposeAdmittedPlan preserves the launcher's §4.5 placement while consuming
// NativeArgs already admitted into the upstream plan. The module appends the
// opaque suffix after its own arguments; this boundary moves that exact suffix
// after the Pi prompt addition; Claude/Codex context is already constructed
// by the plugin. A mismatch means the admitted plan no longer
// reproduces the request, so composition refuses instead of guessing.
func ComposeAdmittedPlan(plan agentic.Plan, ownEnv []string, frag fragment.Fragment, prompt PromptApplication, native []string) (Value, error) {
	if len(native) > len(plan.Argv) {
		return Value{}, fmt.Errorf("admitted plan does not carry the requested native argument suffix")
	}
	cut := len(plan.Argv) - len(native)
	for i, arg := range native {
		if plan.Argv[cut+i] != arg {
			return Value{}, fmt.Errorf("admitted plan does not carry the requested native argument suffix")
		}
	}
	plan.Argv = slices.Clone(plan.Argv[:cut])
	return Compose(plan, ownEnv, frag, prompt, native)
}

// ComposeAdmittedPlanWithNetwork is ComposeAdmittedPlan with the §4.4b
// network patch applied last in Compose. Production calls this form;
// the patch is empty when the launch is unmanaged.
func ComposeAdmittedPlanWithNetwork(plan agentic.Plan, ownEnv []string, frag fragment.Fragment, prompt PromptApplication, native []string, patch envpatch.Patch) (Value, error) {
	if len(native) > len(plan.Argv) {
		return Value{}, fmt.Errorf("admitted plan does not carry the requested native argument suffix")
	}
	cut := len(plan.Argv) - len(native)
	for i, arg := range native {
		if plan.Argv[cut+i] != arg {
			return Value{}, fmt.Errorf("admitted plan does not carry the requested native argument suffix")
		}
	}
	plan.Argv = slices.Clone(plan.Argv[:cut])
	return ComposeWithNetwork(plan, ownEnv, frag, prompt, native, patch)
}

// Compose consumes the owned-environment snapshot of the admitted plan.
// Native arguments are appended verbatim. Inputs are not retained. It is
// the unmanaged form of ComposeWithNetwork: an empty patch changes
// nothing, so unmanaged behavior stays byte-identical.
func Compose(plan agentic.Plan, ownEnv []string, frag fragment.Fragment, prompt PromptApplication, native []string) (Value, error) {
	return ComposeWithNetwork(plan, ownEnv, frag, prompt, native, envpatch.Unmanaged())
}

// ComposeWithNetwork is Compose with the §4.4b network patch applied
// LAST, after frag.Env, prompt.Env and the MCP overlays and before Env
// is materialized: the patch's Unset names are removed
// case-insensitively from both env and literals, then its Set pairs are
// added to both, through the library's Patch.Apply. While managed (a
// non-empty patch), a proxy-family name in any earlier overlay —
// frag.Env, prompt.Env, or MCP env_names — is a
// network_configuration_conflict, refused before anything is composed;
// network.Prepare runs the same check before the preflight, so this
// recheck only fires when the patch arrived by another path.
// Disjointness is rechecked after the literals land: a patch literal
// wins over a destination-local lookup with the same warning the
// literal-versus-lookup rule prints. Patch application itself is silent:
// the §4.4b provenance line already announces it. Inputs are not retained.
func ComposeWithNetwork(plan agentic.Plan, ownEnv []string, frag fragment.Fragment, prompt PromptApplication, native []string, patch envpatch.Patch) (Value, error) {
	managed := !patch.Empty()
	if managed {
		if err := refuseProxyOverlays(frag, prompt); err != nil {
			return Value{}, err
		}
	}
	own := envMap(ownEnv)
	env := envMap(plan.Env)
	literals := maps.Clone(own)
	v := Value{Binary: plan.Binary, WorkDir: plan.WorkDir, Argv: append([]string{}, plan.Argv...), EnvNames: []string{}, EnvLiterals: literals,
		RawStdin: agentic.StdinPayload{Attached: plan.Stdin.Attached, Bytes: slices.Clone(plan.Stdin.Bytes)}}
	warned := map[string]bool{}
	overlay := func(layer map[string]string) {
		for _, name := range sortedKeys(layer) {
			if _, ok := own[name]; ok && !warned[name] {
				v.Warnings = append(v.Warnings, "environment override: "+name)
				warned[name] = true
			}
			env[name], literals[name] = layer[name], layer[name]
		}
	}
	overlay(frag.Env)
	v.Argv = append(v.Argv, prompt.Argv...)
	overlay(prompt.Env)
	if frag.MCP != nil {
		if frag.Environment == fragment.EnvCodexCLI {
			v.mcpLayer = frag.MCP.Path
		}
		seen := map[string]bool{}
		for _, name := range frag.MCP.EnvNames {
			if seen[name] {
				continue
			}
			seen[name] = true
			if _, collision := literals[name]; collision {
				v.Warnings = append(v.Warnings, "environment literal replaces lookup: "+name)
			} else {
				v.EnvNames = append(v.EnvNames, name)
			}
		}
	}
	sort.Strings(v.EnvNames)
	if managed {
		applyNetworkPatch(env, literals, patch)
		kept := v.EnvNames[:0]
		for _, name := range v.EnvNames {
			if _, collision := literals[name]; collision {
				v.Warnings = append(v.Warnings, "environment literal replaces lookup: "+name)
			} else {
				kept = append(kept, name)
			}
		}
		v.EnvNames = kept
	}
	v.Argv = append(v.Argv, native...)
	for _, name := range sortedKeys(env) {
		v.Env = append(v.Env, name+"="+env[name])
	}
	// A non-nil empty environment must remain distinct from os/exec's nil inherit.
	if v.Env == nil {
		v.Env = []string{}
	}
	if plan.Stdin.Attached {
		v.Stdin = &Stdin{Encoding: "utf-8", Bytes: string(plan.Stdin.Bytes)}
		if !utf8.Valid(plan.Stdin.Bytes) {
			v.Stdin.Encoding = "base64url"
			v.Stdin.Bytes = base64.RawURLEncoding.EncodeToString(plan.Stdin.Bytes)
		}
	}
	return v, nil
}

func envMap(values []string) map[string]string {
	out := map[string]string{}
	for _, entry := range values {
		if name, value, ok := strings.Cut(entry, "="); ok {
			out[name] = value
		}
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// refuseProxyOverlays rejects a proxy-family name in any overlay that the
// §4.4b patch follows: frag.Env, prompt.Env, or MCP env_names. It
// delegates to network.CheckOverlays — the same check Prepare runs
// before the preflight — so the two cannot drift.
func refuseProxyOverlays(frag fragment.Fragment, prompt PromptApplication) error {
	var mcpNames []string
	if frag.MCP != nil {
		mcpNames = frag.MCP.EnvNames
	}
	return network.CheckOverlays(frag.Env, prompt.Env, mcpNames)
}

// applyNetworkPatch applies the §4.4b patch to the composed env and
// literals through the library's Patch.Apply: every entry whose name
// matches an Unset name case-insensitively or a Set name exactly is
// removed, then the Set pairs are added to both. Calling Apply rather
// than re-implementing its matching rules keeps this site from drifting
// from the library.
func applyNetworkPatch(env, literals map[string]string, patch envpatch.Patch) {
	applyToMap(env, patch)
	applyToMap(literals, patch)
}

func applyToMap(m map[string]string, patch envpatch.Patch) {
	in := make([]string, 0, len(m)+len(patch.Set))
	for name, value := range m {
		in = append(in, name+"="+value)
	}
	out := patch.Apply(in)
	clear(m)
	for _, entry := range out {
		if name, value, ok := strings.Cut(entry, "="); ok {
			m[name] = value
		}
	}
}
