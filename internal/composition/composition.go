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
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

// ChildEnvironment is the ownership surface of agentic.System. Pass the same
// system and request used to obtain Plan. A second BuildPlan is neither needed
// nor permitted: only ChildEnv over a nil parent defines owned literals.
type ChildEnvironment interface {
	ChildEnv(parent []string, req agentic.LaunchRequest) ([]string, error)
}

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

// Compose consumes a validated fragment, an admitted single-process plan, and
// the same system/request that produced that plan. Native is appended verbatim,
// including duplicate provider flags. Inputs are not modified or retained.
func Compose(plan agentic.Plan, system ChildEnvironment, req agentic.LaunchRequest, frag fragment.Fragment, prompt PromptApplication, native []string) (Value, error) {
	ownEnv, err := system.ChildEnv(nil, req)
	if err != nil {
		return Value{}, fmt.Errorf("composition child environment: %w", err)
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
		for _, ch := range frag.MCP.Channels {
			switch ch.Kind {
			case fragment.KindFlag:
				arg := frag.MCP.Path
				if ch.Argument == fragment.ArgumentName {
					arg = ch.Name
				}
				v.Argv = append(v.Argv, ch.Flag, arg)
				v.Argv = append(v.Argv, ch.With...)
			case fragment.KindVariable:
				overlay(map[string]string{ch.Variable: frag.MCP.Path})
			}
		}
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
