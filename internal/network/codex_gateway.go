package network

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
	"github.com/relux-works/curator-network-profiles/pkg/binding"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
	"github.com/relux-works/curator-network-profiles/pkg/resolve"
	"github.com/relux-works/skill-agents-management/pkg/agentic"

	"github.com/relux-works/curator-agent-launcher/internal/gatewayphase"
	"github.com/relux-works/curator-agent-launcher/internal/netproxy"
)

const codexTokenEnv = "CURATOR_CODEX_GATEWAY_TOKEN"

// A registration token stays private to this process and the final child env.
// Neither a patch, provider table, receipt nor provenance contains its value.
type codexGateway struct {
	final     agentic.PlanWithEnvironment
	receipt   *agentic.GatewayAdmissionReceiptV1
	token     string
	binary    string
	argv, env []string
	bound     bool
}

func gatewayPairs(values map[string]string) []string {
	names := make([]string, 0, len(values))
	for n := range values {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, n+"="+values[n])
	}
	return out
}

func prepareCodexGateway(ctx context.Context, req Request, result *resolve.Result, identity binding.AdapterIdentity, provenance *adapterprobe.Provenance) (prepared Prepared, err error) {
	if req.CodexPlan == nil || req.StagedExecutable == "" || req.Entrypoint != EntrypointInteractive {
		return Prepared{}, agentic.ErrCodexGatewayRefused
	}
	admission, err := agentic.AdmitCodexGateway(req.CodexPlan.GatewayBasis, *req.CodexPlan, agentic.CodexGatewayInputs{
		FragmentEnv: gatewayPairs(req.FragEnv), PromptEnv: gatewayPairs(req.PromptEnv), NativeArgs: req.NativeArgs, LookupNames: req.MCPEnvNames,
	})
	if err != nil {
		return Prepared{}, err
	}
	register := netproxy.RegisterRequest{Root: req.ProjectRoot, Session: req.SessionID + "-" + admission.AttemptID(), Runtime: "codex"}
	if err := register.Validate(); err != nil {
		return Prepared{}, agentic.ErrCodexGatewayRefused
	}
	client := netproxy.New(result.Profile.Control)
	if req.GatewayTimeout > 0 {
		client.Timeout = req.GatewayTimeout
	}
	if err := client.Health(ctx); err != nil {
		return Prepared{}, refusal.New(refusal.CodeProxyUnreachable, result.Profile.Name, "filtering gateway health: "+gatewayDetail(err))
	}
	requested := time.Now()
	reg, err := client.Register(context.WithoutCancel(ctx), register)
	if err != nil {
		return Prepared{}, refusal.New(refusal.CodeProxyUnreachable, result.Profile.Name, "filtering gateway registration: "+gatewayDetail(err))
	}
	lease := netproxy.NewLease(client, reg, req.Alert, requested)
	// A registration is owned even when finalization or a held signal refuses.
	defer func() {
		if err != nil {
			_ = lease.Finish(context.Background())
		}
	}()
	if ctx.Err() != nil {
		return Prepared{}, agentic.ErrCodexGatewayRefused
	}
	final, receipt, err := admission.Finalize(agentic.RuntimeGatewayBindingV1{
		AttemptID: admission.AttemptID(), ProviderID: "curator-gateway", BaseURL: reg.ChatGPTBase,
		HeaderName: reg.TokenHeader, TokenEnv: codexTokenEnv, WireAPI: "responses", RequiresOpenAIAuth: true,
		SupportsWebSockets: false, ToolRelease: req.ToolRelease, ExecutablePath: req.StagedExecutable,
		ExecutableSHA256: fmt.Sprintf("%x", sha256.Sum256(req.Artifact)),
	})
	if err != nil {
		return Prepared{}, err
	}
	bound := binding.Binding{ProfileRef: result.Selection.ProfileRef, ProfileDigest: result.Digest,
		AdapterIdentity: identity, Assurance: result.Assurance, ResolvedEndpoint: reg.ChatGPTBase}
	record := bound.Record(string(result.Selection.Origin), &binding.ProbeRecord{TCP: "skipped", Connect: "skipped", TLS: "skipped", CheckedAt: time.Now().UTC()})
	gateway := &codexGateway{final: final, receipt: receipt, token: reg.Token}
	return Prepared{Record: &record, Provenance: provenance, Lease: lease, codex: gateway, Recheck: func(ctx context.Context) error {
		gatewayphase.Reach("recheck")
		if ctx.Err() != nil {
			return agentic.ErrCodexGatewayRefused
		}
		return final.Plan.VerifyBeforeExec()
	}}, nil
}

// CodexPlan returns only the module-finalized nonsecret plan. There is no
// consumer-side argument appending or plan resealing.
func (p Prepared) CodexPlan() (agentic.PlanWithEnvironment, bool) {
	if p.codex == nil {
		return agentic.PlanWithEnvironment{}, false
	}
	return p.codex.final, true
}

// BindProcess seals the complete composition before provenance. The module
// checks the native suffix and artifacts; the launcher also preserves the exact
// composed environment and keeps the token out of all serializable literals.
func (p Prepared) BindProcess(binary string, argv, env []string, literals map[string]string, names []string) error {
	g := p.codex
	if g == nil {
		return nil
	}
	if g.bound {
		return agentic.ErrCodexGatewayRefused
	}
	b, ok := g.receipt.BindingSnapshot()
	if !ok || b.ExecutablePath != binary {
		return agentic.ErrCodexGatewayRefused
	}
	for n := range literals {
		if strings.EqualFold(n, codexTokenEnv) {
			return agentic.ErrCodexGatewayRefused
		}
	}
	probe := slices.Clone(env)
	for _, entry := range env {
		n, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(n, codexTokenEnv) {
			return agentic.ErrCodexGatewayRefused
		}
	}
	for _, n := range names {
		if strings.EqualFold(n, codexTokenEnv) {
			return agentic.ErrCodexGatewayRefused
		}
		// A lookup can only select its existing inherited value. An absent name is
		// nevertheless checked against the module's reserved-selector grammar.
		found := false
		for _, entry := range probe {
			name, _, _ := strings.Cut(entry, "=")
			if name == n {
				found = true
				break
			}
		}
		if !found {
			probe = append(probe, n+"=")
		}
	}
	probe = append(probe, codexTokenEnv+"="+g.token)
	if err := g.final.Plan.VerifyCodexGatewayProcess(binary, argv, probe); err != nil {
		return err
	}
	g.binary, g.argv, g.env, g.bound = binary, slices.Clone(argv), slices.Clone(env), true
	return nil
}

func (p Prepared) ChildEnvironment(env []string) ([]string, error) {
	g := p.codex
	if g == nil {
		return env, nil
	}
	if !g.bound || !slices.Equal(env, g.env) {
		return nil, agentic.ErrCodexGatewayRefused
	}
	return append(slices.Clone(env), codexTokenEnv+"="+g.token), nil
}

func (p Prepared) VerifyProcess(binary string, argv, env []string) error {
	g := p.codex
	if g == nil {
		return nil
	}
	expected := append(slices.Clone(g.env), codexTokenEnv+"="+g.token)
	if !g.bound || binary != g.binary || !slices.Equal(argv, g.argv) || !slices.Equal(env, expected) {
		return agentic.ErrCodexGatewayRefused
	}
	return g.final.Plan.VerifyCodexGatewayProcess(binary, argv, env)
}
