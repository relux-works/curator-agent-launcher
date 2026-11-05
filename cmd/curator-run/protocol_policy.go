package main

import (
	"context"
	"io"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
	"github.com/relux-works/curator-agent-launcher/internal/network"
	"github.com/relux-works/curator-agent-launcher/internal/protocolchild"
)

// protocolPolicy retains the source-owned policy inputs. It is neither an
// executable plan nor proof of the Host's retained ordinary operation.
type protocolPolicy struct {
	metadata *protocolMetadata
	ordinary *ordinaryResolution
	inv      cli.Invocation
}

func resolveProtocolPolicy(ctx context.Context, meta *protocolMetadata, deps launchDeps) (*protocolPolicy, error) {
	refuse := func(code string) (*protocolPolicy, error) {
		return nil, &protocolchild.Error{Code: code, Detail: "protocol child policy preparation refused"}
	}
	if ctx.Err() != nil {
		return refuse("protocol_child_cancelled")
	}
	// Metadata owns the frozen baseline/workspace; no mutable ambient reader
	// may replace them during policy/defaults/selector work.
	deps.workdir = func() (string, error) { return meta.workspace, nil }
	deps.environ = func() []string { return append([]string{}, meta.baseline...) }
	deps.isTerminal = func() bool { return false }
	if deps.readSelectedConfig == nil {
		deps.readSelectedConfig = func(ctx context.Context) (launcherconfig.Selected, error) {
			return launcherconfig.ReadSelected(ctx, meta.workspace, append([]string{}, meta.baseline...))
		}
	}
	resolver, err := locateOrdinaryConfig(deps, io.Discard)
	if err != nil {
		return refuse("protocol_child_policy_refused")
	}
	if path, err := configfile.RemovedAX(resolver.Locations); err != nil || path != "" {
		return refuse("protocol_child_policy_refused")
	}
	deps, err = loadOrdinaryPolicy(ctx, resolver, deps)
	if err != nil {
		return refuse("protocol_child_policy_refused")
	}
	// The initial backend owns no network lease/snapshot/signal lifetime.
	// Never bypass an explicitly selected profile or a filtering default.
	if deps.selectedConfig.Proxy != nil {
		return refuse("protocol_child_network_unsupported")
	}
	gateway, err := network.DefaultGatewaySelected(meta.baseline)
	if err != nil || gateway {
		return refuse("protocol_child_network_unsupported")
	}
	inv := cli.Invocation{EnvID: fragment.EnvCodexCLI, Profile: meta.selectors.Profile, ProfileSet: true}
	ordinary, failure := resolveOrdinary(ctx, inv, meta.fragment, resolver, deps)
	if failure != nil {
		return refuse("protocol_child_policy_refused")
	}
	if ctx.Err() != nil {
		return refuse("protocol_child_cancelled")
	}
	// Null allows only the concrete value ordinary resolution actually supplies.
	// Explicit strings, including empty, compare verbatim; none becomes a flag.
	s := meta.selectors
	if s.Model != nil && *s.Model != ordinary.resolved.Model.Value || s.Effort != nil && *s.Effort != ordinary.resolved.Effort.Value {
		return refuse("protocol_child_policy_mismatch")
	}
	return &protocolPolicy{metadata: meta, ordinary: ordinary, inv: inv}, nil
}
