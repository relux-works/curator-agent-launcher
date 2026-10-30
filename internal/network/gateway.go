package network

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
		for _, n := range gatewayClaudeNames {
			if _, ok := layer[n]; ok {
				return Prepared{}, refusal.New(refusal.CodeConfigurationConflict, name, "a composed overlay sets "+n+", which the filtering gateway owns")
			}
		}
	}
	for _, mcp := range req.MCPEnvNames {
		for _, n := range gatewayClaudeNames {
			if mcp == n {
				return Prepared{}, refusal.New(refusal.CodeConfigurationConflict, name, "an MCP lookup names "+n+", which the filtering gateway owns")
			}
		}
	}
	if err := settingsConflict(name, req); err != nil {
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
	reg, err := client.Register(ctx, register)
	if err != nil {
		if netproxy.IsCode(err, http.StatusUnprocessableEntity, "config_invalid") {
			return Prepared{}, refusal.New(refusal.CodeProxyUnreachable, name, "the filtering gateway refused registration: the project's gateway configuration is invalid; its owner fixes it")
		}
		return Prepared{}, refusal.New(refusal.CodeProxyUnreachable, name, "filtering gateway registration: "+gatewayDetail(err))
	}
	unset := append(envpatch.UnsetNames(), "ANTHROPIC_BASE_URL", "ANTHROPIC_CUSTOM_HEADERS")
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
	lease := netproxy.NewLease(client, reg, req.Alert)
	return Prepared{Patch: patch, Record: &record, Provenance: provenance, Lease: lease}, nil
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
	cat, err := catalog.Load(parentEnv)
	if err != nil {
		return false, err
	}
	if cat == nil || cat.File == nil || cat.File.Default == "" {
		return false, nil
	}
	p, ok := cat.File.Lookup(cat.File.Default)
	return ok && p.Kind == netprofile.KindFilteringGateway, nil
}

// maxSettingsBytes bounds each settings document the gateway check reads.
const maxSettingsBytes = 1 << 20

// settingsConflict refuses a gateway launch when a Claude Code settings
// source the launcher can see sets one of the gateway's names in `env`:
// an explicit --settings value or file, the child's user settings
// (CLAUDE_CONFIG_DIR/settings.json, else HOME/.claude/settings.json), and
// the project's .claude/settings.json and settings.local.json. Claude
// applies settings env into its process environment, so such an entry could
// replace the gateway's base URL or token header. A source that exists but
// cannot be read or decoded refuses too. Managed and policy settings are
// outside this check, and the files are read before spawn only.
func settingsConflict(name string, req Request) error {
	conflict := func(source, detail string) error {
		return refusal.New(refusal.CodeConfigurationConflict, name, "Claude Code settings ("+source+") "+detail)
	}
	check := func(source string, doc []byte) error {
		var settings struct {
			Env map[string]json.RawMessage `json:"env"`
		}
		if err := json.Unmarshal(doc, &settings); err != nil {
			return conflict(source, "cannot be decoded for the filtering-gateway check")
		}
		for key := range settings.Env {
			for _, n := range gatewayClaudeNames {
				if strings.EqualFold(key, n) {
					return conflict(source, "set "+n+", which the filtering gateway owns")
				}
			}
		}
		return nil
	}
	readFile := func(source, path string) error {
		f, err := os.Open(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return conflict(source, "cannot be read for the filtering-gateway check")
		}
		defer f.Close()
		doc, err := io.ReadAll(io.LimitReader(f, maxSettingsBytes+1))
		if err != nil || len(doc) > maxSettingsBytes {
			return conflict(source, "cannot be read for the filtering-gateway check")
		}
		return check(source, doc)
	}
	env := map[string]string{}
	for _, e := range req.ChildEnv {
		if k, v, ok := strings.Cut(e, "="); ok {
			env[k] = v
		}
	}
	userDir := env["CLAUDE_CONFIG_DIR"]
	if userDir == "" && env["HOME"] != "" {
		userDir = filepath.Join(env["HOME"], ".claude")
	}
	if userDir != "" {
		if err := readFile("user", filepath.Join(userDir, "settings.json")); err != nil {
			return err
		}
	}
	for _, file := range []string{"settings.json", "settings.local.json"} {
		if err := readFile("project", filepath.Join(req.ProjectRoot, ".claude", file)); err != nil {
			return err
		}
	}
	for i := 0; i < len(req.NativeArgs); i++ {
		arg := req.NativeArgs[i]
		if arg == "--" {
			break
		}
		value, ok := "", false
		if v, cut := strings.CutPrefix(arg, "--settings="); cut {
			value, ok = v, true
		} else if arg == "--settings" && i+1 < len(req.NativeArgs) {
			value, ok = req.NativeArgs[i+1], true
			i++
		}
		if !ok {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(value), "{") {
			if err := check("--settings", []byte(value)); err != nil {
				return err
			}
			continue
		}
		path := value
		if !filepath.IsAbs(path) {
			path = filepath.Join(req.ProjectRoot, path)
		}
		if err := readFile("--settings", path); err != nil {
			return err
		}
	}
	return nil
}
