package network

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
	"github.com/relux-works/curator-network-profiles/pkg/binding"
	"github.com/relux-works/curator-network-profiles/pkg/catalog"
	"github.com/relux-works/curator-network-profiles/pkg/envpatch"
	"github.com/relux-works/curator-network-profiles/pkg/netprofile"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
	"github.com/relux-works/curator-network-profiles/pkg/resolve"

	"github.com/relux-works/curator-agent-launcher/internal/netproxy"
)

// Claude Code settings the gateway patch owns. A composed overlay naming
// one of them refuses: the gateway's base URL and token header must be the
// only ones the child sees.
var gatewayClaudeNames = []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_CUSTOM_HEADERS", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"}

// claudeRouteNames select another provider route or endpoint in Claude Code
// 2.1.312 and 2.1.314 (read from their bundles): any of them could send the
// conversation somewhere other than the gateway's base. Inherited values are
// unset in the child; an overlay or settings source naming one refuses.
var claudeRouteNames = []string{
	"ANTHROPIC_API_URL",
	"CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY", "CLAUDE_CODE_USE_MANTLE",
	"CLAUDE_CODE_USE_ANTHROPIC_AWS", "CLAUDE_CODE_USE_ANTHROPIC_GOOGLE_CLOUD", "CLAUDE_CODE_USE_GATEWAY",
	"ANTHROPIC_BEDROCK_BASE_URL", "ANTHROPIC_BEDROCK_MANTLE_BASE_URL", "ANTHROPIC_VERTEX_BASE_URL",
	"ANTHROPIC_FOUNDRY_BASE_URL", "ANTHROPIC_FOUNDRY_RESOURCE", "ANTHROPIC_AWS_BASE_URL", "ANTHROPIC_GOOGLE_CLOUD_BASE_URL",
	"AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_BEDROCK", "AWS_ENDPOINT_URL_BEDROCK_RUNTIME",
}

// prepareGateway binds a filtering-gateway profile for a native launch:
// the gateway's health and a registration come before any spawn, and every
// failure refuses with no unfiltered fallback. The returned lease carries
// the post-spawn duties; the Record and errors never carry the token.
func prepareGateway(ctx context.Context, req Request, result *resolve.Result, identity binding.AdapterIdentity, provenance *adapterprobe.Provenance) (Prepared, error) {
	name := result.Profile.Name
	runtime := ""
	switch req.Harness {
	case claudeCodeHarness:
		runtime = "claude-code"
	default:
		// Codex needs its launch-private provider layer and the deny base;
		// until both exist here, a Codex launch through the gateway refuses.
		return Prepared{}, refusal.New(refusal.CodeScopeUnsupported, name, "the filtering gateway supports Claude Code launches only in this revision")
	}
	for _, layer := range []map[string]string{req.FragEnv, req.PromptEnv} {
		for n := range layer {
			if gatewayRefusedName(n) {
				return Prepared{}, refusal.New(refusal.CodeConfigurationConflict, name, "a composed overlay sets "+n+", which the filtering gateway refuses")
			}
		}
	}
	for _, mcp := range req.MCPEnvNames {
		if gatewayRefusedName(mcp) {
			return Prepared{}, refusal.New(refusal.CodeConfigurationConflict, name, "an MCP lookup names "+mcp+", which the filtering gateway refuses")
		}
	}
	curatorDir := ""
	for _, e := range req.ParentEnv {
		if home, ok := strings.CutPrefix(e, "HOME="); ok {
			curatorDir = filepath.Join(home, ".curator")
		}
	}
	roots, err := loadGatewayRoots(adapterprobe.SafeKnownBadRead, curatorDir)
	if err != nil {
		return Prepared{}, err
	}
	mdmDir := req.ManagedPreferencesDir
	if mdmDir == "" && goruntime.GOOS == "darwin" {
		mdmDir = "/Library/Managed Preferences"
	}
	username := req.Username
	if username == "" {
		if u, err := user.Current(); err == nil {
			username = u.Username
		}
	}
	guard := func(ctx context.Context) error {
		clock := newGuardClock(ctx, req.GuardBudget)
		if _, err := managedHome(name, req); err != nil {
			return err
		}
		if err := settingsConflict(clock, name, req); err != nil {
			return err
		}
		if err := admissionContract(clock, name, req, roots, mdmDir, username); err != nil {
			return err
		}
		// The last inspection may have run past the budget or been
		// interrupted: a check that did not finish in time never admits.
		return clock.step(name)
	}
	if err := guard(ctx); err != nil {
		return Prepared{}, err
	}
	register := netproxy.RegisterRequest{Root: req.ProjectRoot, Session: req.SessionID, Runtime: runtime}
	if err := register.Validate(); err != nil {
		return Prepared{}, refusal.New(refusal.CodeScopeUnsupported, name, "the launch cannot register with the filtering gateway: "+err.Error())
	}
	client := netproxy.New(result.Profile.Control)
	if req.GatewayTimeout > 0 {
		client.Timeout = req.GatewayTimeout
	}
	if err := client.Health(ctx); err != nil {
		return Prepared{}, refusal.New(refusal.CodeProxyUnreachable, name, "filtering gateway health: "+gatewayDetail(err))
	}
	// Registration is not cut short by the caller's cancellation: once sent,
	// its outcome must be known so that it is ended. The client's own
	// timeout bounds it.
	requested := time.Now()
	reg, err := client.Register(context.WithoutCancel(ctx), register)
	if err != nil {
		if netproxy.IsCode(err, http.StatusUnprocessableEntity, "config_invalid") {
			return Prepared{}, refusal.New(refusal.CodeProxyUnreachable, name, "the filtering gateway refused registration: the project's gateway configuration is invalid; its owner fixes it")
		}
		return Prepared{}, refusal.New(refusal.CodeProxyUnreachable, name, "filtering gateway registration: "+gatewayDetail(err))
	}
	unset := append(envpatch.UnsetNames(), "ANTHROPIC_BASE_URL", "ANTHROPIC_CUSTOM_HEADERS")
	unset = append(unset, claudeRouteNames...)
	unset = append(unset, inheritedRouteNames(req.ParentEnv, req.ChildEnv)...)
	patch := envpatch.Patch{
		Unset: unset,
		Set: []envpatch.Pair{
			{Name: "ANTHROPIC_BASE_URL", Value: reg.AnthropicBase},
			{Name: "ANTHROPIC_CUSTOM_HEADERS", Value: reg.TokenHeader + ": " + reg.Token},
			{Name: "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", Value: "1"},
		},
	}
	bound := binding.Binding{
		ProfileRef:       result.Selection.ProfileRef,
		ProfileDigest:    result.Digest,
		AdapterIdentity:  identity,
		Assurance:        result.Assurance,
		ResolvedEndpoint: reg.AnthropicBase,
		EnvPatch:         patch,
	}
	// No CONNECT probe runs for a gateway: health and registration are its
	// preflight, so the probe facts stay skipped.
	record := bound.Record(string(result.Selection.Origin), &binding.ProbeRecord{
		TCP: "skipped", Connect: "skipped", TLS: "skipped", CheckedAt: time.Now().UTC(),
	})
	lease := netproxy.NewLease(client, reg, req.Alert, requested)
	recheck := guard
	return Prepared{Patch: patch, Record: &record, Provenance: provenance, Lease: lease, Recheck: recheck}, nil
}

// gatewayDetail names the failure class and code, never a body or token.
func gatewayDetail(err error) string {
	var e *netproxy.Error
	if errors.As(err, &e) {
		if e.Kind == netproxy.Refused {
			return "refused " + e.Code
		}
		return "unreachable"
	}
	return "failed"
}

// DefaultGatewaySelected reports whether a launch without an explicit
// selection falls under the operator's default filtering gateway (spec
// N4, operator default). With no HOME or no network.toml it is false, as
// before; defaults of other kinds keep today's explicit-only behaviour.
// A network.toml that exists but cannot be read or validated refuses, so
// a broken file never drops the filter silently.
func DefaultGatewaySelected(parentEnv []string) (bool, error) {
	home := false
	for _, e := range parentEnv {
		if strings.HasPrefix(e, "HOME=") {
			home = true
		}
	}
	if !home {
		return false, nil
	}
	// Absence is decided on the directory entry, never on a failed read: a
	// dangling symlink or a file that vanishes after inspection is a broken
	// catalog, and a broken catalog refuses instead of dropping the filter.
	path := ""
	for _, e := range parentEnv {
		if h, ok := strings.CutPrefix(e, "HOME="); ok {
			path = filepath.Join(h, ".curator", netprofile.FileName)
		}
	}
	_, statErr := os.Lstat(path)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return false, refusal.New(refusal.CodeFileUnreadable, netprofile.FileName, "cannot inspect the operator network catalog")
	}
	present := statErr == nil
	// The library reads the catalog with a blocking read; a present entry
	// that is not a regular file (a FIFO, a device) refuses before that.
	if present {
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			return false, refusal.New(refusal.CodeFileUnreadable, netprofile.FileName, "the operator network catalog is present but is not a regular file")
		}
	}
	cat, err := catalog.Load(parentEnv)
	if err != nil {
		return false, err
	}
	if present && (cat == nil || cat.File == nil) {
		return false, refusal.New(refusal.CodeFileUnreadable, netprofile.FileName, "the operator network catalog is present but cannot be read")
	}
	if cat == nil || cat.File == nil || cat.File.Default == "" {
		return false, nil
	}
	p, ok := cat.File.Lookup(cat.File.Default)
	return ok && p.Kind == netprofile.KindFilteringGateway, nil
}
