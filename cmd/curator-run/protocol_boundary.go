package main

import (
	"context"
	"os"
	"reflect"

	"github.com/relux-works/curator-agent-launcher/internal/protocolchild"
)

// recheckProtocolPolicy rereads the actual profile/config/defaults using the
// original closed invocation immediately before the execution-owned checks.
// It never replaces a prepared plan, repairs a home or re-enters Host routing.
// Provider seal, executable, context and cancellation checks remain required at
// direct replacement; this policy check alone grants no execution permission.
func recheckProtocolPolicy(ctx context.Context, prepared *protocolPlan, deps launchDeps) error {
	refuse := func(code string) error {
		return &protocolchild.Error{Code: code, Detail: "protocol child final policy boundary refused"}
	}
	if ctx.Err() != nil {
		return refuse("protocol_child_cancelled")
	}
	meta := prepared.policy.metadata
	environ := deps.environ
	if environ == nil {
		environ = os.Environ
	}
	baseline, err := protocolchild.EntryEnvironment(environ())
	if err != nil || !reflect.DeepEqual(baseline, meta.baseline) {
		return refuse("protocol_child_stale")
	}
	// Validate current entry state before any fresh Core operation, then keep
	// that checked copy throughout the reread. Never substitute a changed
	// optional baseline value and discover the difference only afterwards.
	deps.environ = func() []string { return append([]string{}, baseline...) }
	fresh, err := resolveProtocolMetadata(ctx, append([]string{}, meta.argv...), deps)
	if ctx.Err() != nil {
		return refuse("protocol_child_cancelled")
	}
	if err != nil || !reflect.DeepEqual(fresh.baseline, meta.baseline) ||
		!reflect.DeepEqual(fresh.selectors, meta.selectors) || fresh.workspace != meta.workspace {
		return refuse("protocol_child_stale")
	}
	current, err := resolveProtocolPolicy(ctx, fresh, deps)
	if ctx.Err() != nil {
		return refuse("protocol_child_cancelled")
	}
	if err != nil {
		return refuse("protocol_child_stale")
	}
	old, now := prepared.policy.ordinary, current.ordinary
	oldResolved, nowResolved := old.resolved, now.resolved
	// A fresh selector observation may have a new correlation identifier while
	// preserving the same admitted runtime/model/effort and their provenance.
	// That identifier is not a native process input and cannot replace the
	// retained decision. All effective selection values must still match.
	oldResolved.SelectorDecisionID, nowResolved.SelectorDecisionID = "", ""
	if !reflect.DeepEqual(old.files, now.files) || !reflect.DeepEqual(oldResolved, nowResolved) ||
		!reflect.DeepEqual(old.deps.configResolver.Locations, now.deps.configResolver.Locations) ||
		!reflect.DeepEqual(old.deps.launcherConfig, now.deps.launcherConfig) ||
		!reflect.DeepEqual(old.deps.selectedConfig, now.deps.selectedConfig) ||
		!reflect.DeepEqual(old.deps.selectorConfig, now.deps.selectorConfig) {
		return refuse("protocol_child_stale")
	}
	return nil
}
