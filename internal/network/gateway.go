package network

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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

// gatewayRefusedName reports whether a variable name, compared without case,
// is one an overlay or settings source must not set under the gateway: the
// gateway's own names, a provider route or endpoint, or the proxy family.
func gatewayRefusedName(name string) bool {
	for _, set := range [][]string{gatewayClaudeNames, claudeRouteNames, envpatch.UnsetNames()} {
		for _, n := range set {
			if strings.EqualFold(name, n) {
				return true
			}
		}
	}
	return false
}

// refusedSettingsFlags change which Claude settings sources load or where
// they are read from; under the gateway they refuse rather than move the
// effective configuration outside the check.
var refusedSettingsFlags = []string{"--setting-sources", "--managed-settings", "--project-config-root"}

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
	requested := time.Now()
	reg, err := client.Register(ctx, register)
	if err != nil {
		if netproxy.IsCode(err, http.StatusUnprocessableEntity, "config_invalid") {
			return Prepared{}, refusal.New(refusal.CodeProxyUnreachable, name, "the filtering gateway refused registration: the project's gateway configuration is invalid; its owner fixes it")
		}
		return Prepared{}, refusal.New(refusal.CodeProxyUnreachable, name, "filtering gateway registration: "+gatewayDetail(err))
	}
	unset := append(envpatch.UnsetNames(), "ANTHROPIC_BASE_URL", "ANTHROPIC_CUSTOM_HEADERS")
	unset = append(unset, claudeRouteNames...)
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
	recheck := func() error { return settingsConflict(name, req) }
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

// maxSettingsBytes bounds each settings document the gateway check reads.
const maxSettingsBytes = 1 << 20

// managedSettingsDir is where Claude Code reads machine-managed settings on
// this platform (managed-settings.json and the managed-settings.d drop-ins),
// as named in the 2.1.312 and 2.1.314 bundles.
func managedSettingsDir() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/ClaudeCode"
	case "linux":
		return "/etc/claude-code"
	}
	return ""
}

// settingsConflict refuses a gateway launch when a Claude Code settings
// source the launcher can read could move the child off the gateway: an
// `env` entry naming a gateway, provider-route or proxy variable (compared
// without case), or a flag that changes which sources load. The sources are
// an explicit --settings value or file, the child's user settings
// (CLAUDE_CONFIG_DIR, else HOME/.claude), every .claude/settings.json and
// settings.local.json from the project root up to the filesystem root, and
// the managed-settings file and drop-ins. A source that exists but cannot
// be read or decoded refuses too. MDM profiles and server-managed settings
// cannot be read here; the check runs at preparation and again at the
// pre-spawn boundary.
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
			if gatewayRefusedName(key) {
				return conflict(source, "set "+key+", which the filtering gateway refuses")
			}
		}
		return nil
	}
	readFile := func(source, path string) error {
		f, err := os.Open(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if _, lerr := os.Lstat(path); lerr == nil {
					return conflict(source, "is present but cannot be read for the filtering-gateway check")
				}
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
	for dir := req.ProjectRoot; ; dir = filepath.Dir(dir) {
		for _, file := range []string{"settings.json", "settings.local.json"} {
			if err := readFile("project", filepath.Join(dir, ".claude", file)); err != nil {
				return err
			}
		}
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
	}
	managed := req.ManagedSettingsDir
	if managed == "" {
		managed = managedSettingsDir()
	}
	if managed != "" {
		if err := readFile("managed", filepath.Join(managed, "managed-settings.json")); err != nil {
			return err
		}
		drop := filepath.Join(managed, "managed-settings.d")
		entries, err := os.ReadDir(drop)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return conflict("managed", "drop-ins cannot be listed for the filtering-gateway check")
		}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".json") {
				if err := readFile("managed", filepath.Join(drop, entry.Name())); err != nil {
					return err
				}
			}
		}
	}
	for i := 0; i < len(req.NativeArgs); i++ {
		arg := req.NativeArgs[i]
		if arg == "--" {
			break
		}
		for _, flag := range refusedSettingsFlags {
			if arg == flag || strings.HasPrefix(arg, flag+"=") {
				return conflict(flag, "changes which settings sources load, which the filtering gateway refuses")
			}
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
