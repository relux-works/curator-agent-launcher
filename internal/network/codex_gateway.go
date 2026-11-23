package network

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
	"github.com/relux-works/curator-network-profiles/pkg/binding"
	"github.com/relux-works/curator-network-profiles/pkg/resolve"
	"github.com/relux-works/skill-agents-management/pkg/agentic"

	"github.com/relux-works/curator-agent-launcher/internal/gatewayphase"
	"github.com/relux-works/curator-agent-launcher/internal/netproxy"
)

const codexTokenEnv = "CURATOR_CODEX_GATEWAY_TOKEN"

// A registration token stays private to this process and the final child env.
// Neither a patch, provider table, receipt nor provenance contains its value.
type codexGateway struct {
	final               agentic.PlanWithEnvironment
	receipt             *agentic.GatewayAdmissionReceiptV1
	token               string
	binary              string
	argv, env           []string
	bound               bool
	standaloneWebSearch bool
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
	req.samAttempt = admission.AttemptID()
	reg, lease, err := registerGateway(ctx, req, result, client, register)
	if err != nil {
		return Prepared{}, err
	}
	// A registration is owned even when finalization or a held signal refuses.
	defer func() {
		if err != nil {
			// Preparation still owns this registration; a held launch signal
			// must not skip cleanup, but an unreachable gateway cannot hold it
			// forever. The caller releases its signal hold after we return.
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), netproxy.RecoveryCompletionBudget)
			defer cancel()
			_ = lease.Finish(cleanup)
		}
	}()
	if ctx.Err() != nil {
		return Prepared{}, agentic.ErrCodexGatewayRefused
	}
	final, receipt, err := admission.Finalize(agentic.RuntimeGatewayBindingV1{
		AttemptID: admission.AttemptID(), ProviderID: "curator-gateway", BaseURL: reg.ChatGPTBase,
		HeaderName: reg.TokenHeader, TokenEnv: codexTokenEnv, WireAPI: "responses", RequiresOpenAIAuth: true,
		SupportsWebSockets: false, SupportsStandaloneWebSearch: req.GatewayStandaloneWebSearch,
		ToolRelease: req.ToolRelease, ExecutablePath: req.StagedExecutable,
		ExecutableSHA256: fmt.Sprintf("%x", sha256.Sum256(req.Artifact)),
	})
	if err != nil {
		return Prepared{}, err
	}
	bound := binding.Binding{ProfileRef: result.Selection.ProfileRef, ProfileDigest: result.Digest,
		AdapterIdentity: identity, Assurance: result.Assurance, ResolvedEndpoint: reg.ChatGPTBase}
	record := bound.Record(string(result.Selection.Origin), &binding.ProbeRecord{TCP: "skipped", Connect: "skipped", TLS: "skipped", CheckedAt: time.Now().UTC()})
	gateway := &codexGateway{final: final, receipt: receipt, token: reg.Token, standaloneWebSearch: req.GatewayStandaloneWebSearch}
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
	for _, entry := range env {
		n, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(n, codexTokenEnv) {
			return agentic.ErrCodexGatewayRefused
		}
	}
	// Lookup names are checked against the module's grammar at admission. The
	// process check compares the exact final environment plus the token, so
	// nothing is added for an absent lookup name.
	for _, n := range names {
		if strings.EqualFold(n, codexTokenEnv) {
			return agentic.ErrCodexGatewayRefused
		}
	}
	// The launcher's own requirement, beside the module's. Nothing is
	// executed unbound: VerifyProcess refuses a process that was not bound.
	if !codexSwitchesOff(argv, g.standaloneWebSearch) {
		return agentic.RefuseCodexGateway(agentic.CodexGatewayBindingNotCanonical)
	}
	probe := append(slices.Clone(env), codexTokenEnv+"="+g.token)
	if err := g.final.Plan.VerifyCodexGatewayProcess(binary, argv, probe); err != nil {
		return err
	}
	g.binary, g.argv, g.env, g.bound = binary, slices.Clone(argv), slices.Clone(env), true
	return nil
}

// codexThread is the identity a native `resume` carries.
var codexThread = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// codexSwitchesOff reports final Codex arguments that are, token by token and
// in order, what the pinned module renders for a gateway launch this launcher
// requests, the four owned settings among them, and the optional standalone
// search capability selected by machine policy:
//
//	-c model_providers.curator-gateway={...}    owned
//	-c model_provider="curator-gateway"         owned
//	-c features.apps=false                      owned
//	-c features.plugins=false                   owned
//	-c analytics.enabled=false                  owned
//	-c check_for_update_on_startup=false        owned
//	[-c features.standalone_web_search=true]    owned only for an explicit opt-in
//	[-c model_instructions_file="..."]          Curator system prompt
//	[-p curator-mcp]                            Curator MCP channel
//	[-m <model>] [-c model_reasoning_effort="..."]
//	[--dangerously-bypass-approvals-and-sandbox]
//	[--no-alt-screen] [resume <thread id>]      the closed native suffix
//
// This check requires the four settings in the final arguments, as these
// exact tokens in this order after the provider selection. It establishes no
// client-release behavior and no traffic effect: not that a client release
// applies the settings, and not that any connection changes. The update-check
// behavior has not been witnessed, and the `gh` status-line child is outside
// these switches. The pinned module renders the four settings; this check is
// the launcher's own, so a module that stopped rendering one cannot start a
// gateway child.
//
// Nothing else is admitted. By its source the client reads a setting from
// many spellings (`-c=key=value`, `--config`, a key it trims, `--enable`,
// legacy keys), so the launcher does not list spellings to refuse: an
// argument that is not one of these forms in its place refuses, whatever it
// would select. A pair's value is free after its opening character; a model
// does not start with `-`.
func codexSwitchesOff(argv []string, standaloneWebSearch bool) bool {
	rest := argv
	take := func(tokens ...string) bool {
		if len(rest) < len(tokens) || !slices.Equal(rest[:len(tokens)], tokens) {
			return false
		}
		rest = rest[len(tokens):]
		return true
	}
	pair := func(opening string) bool {
		if len(rest) < 2 || rest[0] != "-c" || !strings.HasPrefix(rest[1], opening) {
			return false
		}
		rest = rest[2:]
		return true
	}
	const providerTable = "model_providers.curator-gateway={"
	if len(rest) < 2 || rest[0] != "-c" || !strings.HasPrefix(rest[1], providerTable) {
		return false
	}
	if standaloneWebSearch {
		if strings.Count(rest[1], "supports_standalone_web_search") != 1 ||
			!strings.Contains(rest[1], ",supports_standalone_web_search=true,") {
			return false
		}
	} else if strings.Contains(rest[1], "supports_standalone_web_search") {
		return false
	}
	rest = rest[2:]
	if !take("-c", `model_provider="curator-gateway"`) {
		return false
	}
	for _, off := range []string{"features.apps=false", "features.plugins=false", "analytics.enabled=false", "check_for_update_on_startup=false"} {
		if !take("-c", off) {
			return false
		}
	}
	if standaloneWebSearch && !take("-c", "features.standalone_web_search=true") {
		return false
	}
	pair(`model_instructions_file="`)
	take("-p", "curator-mcp")
	if len(rest) >= 2 && rest[0] == "-m" && rest[1] != "" && !strings.HasPrefix(rest[1], "-") {
		rest = rest[2:]
	}
	pair(`model_reasoning_effort="`)
	take("--dangerously-bypass-approvals-and-sandbox")
	take("--no-alt-screen")
	if len(rest) == 2 && rest[0] == "resume" && codexThread.MatchString(rest[1]) {
		rest = nil
	}
	return len(rest) == 0
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
