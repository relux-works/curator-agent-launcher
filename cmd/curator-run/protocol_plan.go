package main

import (
	"context"
	"io"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/mapping"
	"github.com/relux-works/curator-agent-launcher/internal/plan"
	"github.com/relux-works/curator-agent-launcher/internal/protocolchild"
	"github.com/relux-works/curator-agent-launcher/internal/systemprompt"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

// protocolPlan stays private to this process. A prepared plan is not an
// execution permit, a serialized Host plan or ordinary-operation admission.
// The final profile/policy/executable/context boundary is still required.
type protocolPlan struct {
	policy   *protocolPolicy
	decision execution.PermissionDecision
	value    composition.Value
	sealed   agentic.Plan
}

// composeProtocolPlan uses the same permission, release, vendor admission and
// composition owners as the frontend, without its routing/interactive executor.
// It may run a bounded owning version probe, but starts no native session.
func composeProtocolPlan(ctx context.Context, policy *protocolPolicy) (*protocolPlan, error) {
	refuse := func(code string) (*protocolPlan, error) {
		return nil, &protocolchild.Error{Code: code, Detail: "protocol child plan preparation refused"}
	}
	if ctx.Err() != nil {
		return refuse("protocol_child_cancelled")
	}
	meta, ordinary := policy.metadata, policy.ordinary
	s, frag := meta.selectors, meta.fragment
	deps := ordinary.deps
	deps.environ = func() []string { return append([]string{}, meta.baseline...) }
	deps.workdir = func() (string, error) { return meta.workspace, nil }
	deps.isTerminal = func() bool { return false }
	member, err := s.MemberEnvironment()
	if err != nil {
		return refuse(protocolchild.CodeEnvironmentRefused)
	}
	if s.MemberConfigDir != nil {
		physical, err := filepath.EvalSymlinks(*s.MemberConfigDir)
		if err != nil || physical != *s.MemberConfigDir {
			return refuse(protocolchild.CodeEnvironmentRefused)
		}
	}
	// Neither a profile overlay nor a destination lookup may supply messaging
	// identity, even if it happens to match the explicit pair. Only the closed
	// selectors below own those two exported native-child names.
	noMember := protocolchild.Selectors{}
	fragmentEnv := make([]string, 0, len(frag.Env))
	for name, value := range frag.Env {
		fragmentEnv = append(fragmentEnv, name+"="+value)
	}
	if err := noMember.CheckMemberEnvironment(fragmentEnv); err != nil {
		return refuse(protocolchild.CodeEnvironmentRefused)
	}
	if frag.MCP != nil {
		for _, name := range frag.MCP.EnvNames {
			if strings.HasPrefix(name, "A2A_") {
				return refuse(protocolchild.CodeEnvironmentRefused)
			}
		}
	}
	target, err := mapping.Resolve(fragment.EnvCodexCLI)
	if err != nil {
		return refuse("protocol_child_plan_refused")
	}
	// Permission expectations stay out of Invocation flags: a match cannot
	// unlock a profile, override defaults or choose a different native policy.
	decision, release, _, err := resolvePermission(ctx, policy.inv, frag, target, ordinary.files, meta.baseline, io.Discard, deps, true)
	if ctx.Err() != nil {
		return refuse("protocol_child_cancelled")
	}
	if err != nil {
		return refuse("protocol_child_permission_refused")
	}
	if string(decision.Mode) != s.Permissions {
		return refuse("protocol_child_permission_mismatch")
	}
	if deps.availability == nil {
		return refuse("protocol_child_plan_refused")
	}
	availability, err := deps.availability()
	if err != nil {
		return refuse("protocol_child_plan_refused")
	}
	build := deps.build
	if build == nil {
		build = vendorplugin.BuildLaunchWithEnvironment
	}
	selection, err := systemprompt.Select(frag, fragment.Semantics(policy.inv.SystemPrompt))
	if err != nil {
		return refuse("protocol_child_plan_refused")
	}
	// This suffix is fixed BEFORE vendor admission and checked by composition.
	// Caller native-tail remains the closed empty array; no post-plan append.
	native := []string{"app-server"}
	admitted, err := plan.Build(ctx, plan.Deps{Registry: deps.registry, BuildLaunch: build, Availability: availability, ModuleVersion: deps.moduleVersion}, plan.Request{
		Runtime: ordinary.resolved.Runtime, Model: ordinary.resolved.Model.Value, Effort: ordinary.resolved.Effort.Value,
		PermissionMode: decision.Mode, ToolRelease: release, NativeArgs: native,
		Home: frag.Home(), WorkDir: meta.workspace, Env: append([]string{}, meta.baseline...),
		Context: plan.Context(frag, fragment.Semantics(policy.inv.SystemPrompt)),
	})
	if ctx.Err() != nil {
		return refuse("protocol_child_cancelled")
	}
	if err != nil || admitted.Plan.Stdin.Attached || admitted.Plan.WorkDir != meta.workspace {
		return refuse("protocol_child_plan_refused")
	}
	value, err := composition.ComposeAdmittedPlan(admitted.Plan, admitted.OwnedEnv, *frag,
		composition.PromptApplication{Argv: selection.Argv(), Env: selection.Env()}, native)
	if err != nil {
		return refuse("protocol_child_plan_refused")
	}
	if err := noMember.CheckMemberEnvironment(value.Env); err != nil {
		return refuse(protocolchild.CodeEnvironmentRefused)
	}
	value.Env = append(value.Env, member...)
	if err := s.CheckMemberEnvironment(value.Env); err != nil {
		return refuse(protocolchild.CodeEnvironmentRefused)
	}
	// Preserve the provider's ORIGINAL artifact basis through its finalizer.
	// app-server is already in the sealed base argv, so NativeTail stays empty.
	// The explicit member pair is a permitted environment overlay, not a Host
	// reservation/admission binding or a re-seal of current artifact contents.
	fragmentEnv = append(fragmentEnv, member...)
	var promptEnv []string
	for name, value := range selection.Env() {
		promptEnv = append(promptEnv, name+"="+value)
	}
	sealed, err := agentic.FinalizePlan(admitted.Plan, agentic.FinalizeOverlays{FragmentEnv: fragmentEnv, PromptEnv: promptEnv}, nil)
	if err != nil || sealed.Binary != value.Binary || sealed.WorkDir != value.WorkDir ||
		!reflect.DeepEqual(sealed.Argv, value.Argv) || sealed.Stdin.Attached || !sameProtocolEnvironment(sealed.Env, value.Env) {
		return refuse("protocol_child_plan_refused")
	}
	// The finalizer commits the full ORDERED environment. Use its exact order
	// after checking that composition and finalization agree by unique name.
	value.Env = append([]string{}, sealed.Env...)
	if ctx.Err() != nil {
		return refuse("protocol_child_cancelled")
	}
	return &protocolPlan{policy: policy, decision: decision, value: value, sealed: sealed}, nil
}

func sameProtocolEnvironment(a, b []string) bool {
	values := func(env []string) (map[string]string, bool) {
		out := make(map[string]string, len(env))
		for _, entry := range env {
			name, value, ok := strings.Cut(entry, "=")
			if _, seen := out[name]; !ok || name == "" || seen {
				return nil, false
			}
			out[name] = value
		}
		return out, true
	}
	left, validLeft := values(a)
	right, validRight := values(b)
	return validLeft && validRight && reflect.DeepEqual(left, right)
}
