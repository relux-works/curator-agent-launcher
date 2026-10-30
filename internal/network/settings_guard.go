package network

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/relux-works/curator-network-profiles/pkg/envpatch"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
)

// Settings sources are bounded: ordinary settings documents at 1 MiB, the
// global configuration (which also stores per-project history) at 64 MiB.
const (
	maxSettingsBytes = 1 << 20
	maxGlobalBytes   = 64 << 20
)

// endpointSuffixes mark Claude Code variables that choose where a request,
// a socket or a proxy goes. Claude Code 2.1.314 reads dozens of such names
// (ANTHROPIC_UNIX_SOCKET, CLAUDE_CODE_API_BASE_URL, the provider bases, the
// proxy and bridge endpoints, ...), so the gateway refuses the whole shape
// instead of a list that a new release could outgrow.
var endpointSuffixes = []string{"_URL", "_URLS", "_SOCKET", "_ENDPOINT", "_HOST", "_BASE", "_RESOURCE", "_PROXY", "_GATEWAY"}

// claudeFeatureSelectors are the CLAUDE_CODE_USE_ names in the 2.1.314
// bundle that select a local tool, not a provider, transport or settings
// source; they are admitted and kept. Every other CLAUDE_CODE_USE_ name,
// including one a later release adds, refuses: BEDROCK, VERTEX, FOUNDRY,
// MANTLE, GATEWAY, ANTHROPIC_AWS, ANTHROPIC_GOOGLE_CLOUD and CCR_V2 route the
// conversation, and COWORK_PLUGINS selects another settings document.
var claudeFeatureSelectors = map[string]bool{
	"CLAUDE_CODE_USE_POWERSHELL_TOOL":    true,
	"CLAUDE_CODE_USE_NATIVE_FILE_SEARCH": true,
}

// gatewayRefusedName reports whether a variable name, compared without case,
// is one an overlay or settings source must not set under the gateway, and
// that is removed from an inherited environment: the gateway's own names,
// any endpoint-shaped ANTHROPIC_, CLAUDE_ or CLAUDE_CODE_ name, every
// CLAUDE_CODE_USE_ route selector, the AWS endpoint names, the listed
// provider routes and the proxy family.
func gatewayRefusedName(name string) bool {
	upper := strings.ToUpper(name)
	for _, set := range [][]string{gatewayClaudeNames, claudeRouteNames, envpatch.UnsetNames()} {
		for _, n := range set {
			if upper == strings.ToUpper(n) {
				return true
			}
		}
	}
	if strings.HasPrefix(upper, "CLAUDE_CODE_USE_") {
		return !claudeFeatureSelectors[upper]
	}
	if strings.HasPrefix(upper, "AWS_ENDPOINT_URL") {
		return true
	}
	if strings.HasPrefix(upper, "ANTHROPIC_") || strings.HasPrefix(upper, "CLAUDE_") {
		for _, suffix := range endpointSuffixes {
			if strings.HasSuffix(upper, suffix) {
				return true
			}
		}
	}
	return false
}

// inheritedRouteNames are the names in env that gatewayRefusedName matches;
// the gateway removes them from the child's environment.
func inheritedRouteNames(envs ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, env := range envs {
		for _, e := range env {
			name, _, ok := strings.Cut(e, "=")
			if ok && !seen[name] && gatewayRefusedName(name) {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	return out
}

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

// errAbsent marks a source with no directory entry at all.
var errAbsent = errors.New("absent")

// readRegular reads a settings source without blocking on a special file:
// a final symbolic link is followed only to a regular file, the file is
// opened non-blocking, and the opened file must be the inspected regular
// file. A missing directory entry is errAbsent; anything else that is not a
// readable regular file within limit is an error.
func readRegular(path string, limit int64) ([]byte, error) {
	if _, err := os.Lstat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errAbsent
		}
		return nil, err
	}
	target, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !target.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(target, opened) {
		return nil, errors.New("changed while being opened")
	}
	doc, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(doc)) > limit {
		return nil, errors.New("not readable within its limit")
	}
	return doc, nil
}

// settingsDirs returns dir and its ancestors twice over, along the path as
// given and along its resolved physical path, without duplicates: Claude
// discovers project settings from its real working directory and git root,
// which a symbolic link in the given path can hide.
func settingsDirs(dir string) []string {
	var out []string
	seen := map[string]bool{}
	walk := func(d string) {
		for ; ; d = filepath.Dir(d) {
			if !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
			if filepath.Dir(d) == d {
				return
			}
		}
	}
	walk(dir)
	if physical, err := filepath.EvalSymlinks(dir); err == nil {
		walk(physical)
	}
	return out
}

// settingsConflict refuses a gateway launch when a Claude Code settings
// source the launcher can read could move the child off the gateway. The
// sources are: an explicit --settings value or file; the child's user
// settings (CLAUDE_CONFIG_DIR, else HOME/.claude) and its cowork_settings
// alternative; the legacy global configuration (.config.json in that
// directory, and the selected .claude.json in CLAUDE_CONFIG_DIR or HOME), whose `env`
// Claude applies first; every .claude/settings.json and settings.local.json
// from the project root to the filesystem root along both the given and the
// physical path; and the managed-settings file and drop-ins, which also
// refuse when they carry a policy helper, whose output cannot be checked
// here. Any refused name in an `env`, a source that exists but is not a
// readable regular file or does not decode, and any flag that changes the
// source set or the working directory refuse. MDM profiles and
// server-managed settings cannot be read here.
func settingsConflict(name string, req Request) error {
	conflict := func(source, detail string) error {
		return refusal.New(refusal.CodeConfigurationConflict, name, "Claude Code settings ("+source+") "+detail)
	}
	checkEnv := func(source string, env map[string]json.RawMessage) error {
		for key := range env {
			if gatewayRefusedName(key) {
				return conflict(source, "set "+key+", which the filtering gateway refuses")
			}
		}
		return nil
	}
	check := func(source string, doc []byte, managed bool) error {
		var settings struct {
			Env            map[string]json.RawMessage `json:"env"`
			PolicyHelper   json.RawMessage            `json:"policyHelper"`
			PolicyHelpers  json.RawMessage            `json:"policyHelpers"`
			CoworkPlugins  json.RawMessage            `json:"coworkPlugins"`
			ManagedDefault json.RawMessage            `json:"defaultSettings"`
		}
		if err := json.Unmarshal(doc, &settings); err != nil {
			return conflict(source, "cannot be decoded for the filtering-gateway check")
		}
		if managed && (settings.PolicyHelper != nil || settings.PolicyHelpers != nil || settings.ManagedDefault != nil) {
			return conflict(source, "names a policy helper or default policy payload, whose settings the filtering gateway cannot check")
		}
		return checkEnv(source, settings.Env)
	}
	read := func(source, path string, limit int64, managed bool) error {
		doc, err := readRegular(path, limit)
		if errors.Is(err, errAbsent) {
			return nil
		}
		if err != nil {
			return conflict(source, "is present but is not a readable regular file for the filtering-gateway check")
		}
		return check(source, doc, managed)
	}
	env := childEnv(req)
	userDir := env["CLAUDE_CONFIG_DIR"]
	globalDir := userDir
	if userDir == "" && env["HOME"] != "" {
		userDir = filepath.Join(env["HOME"], ".claude")
		globalDir = env["HOME"]
	}
	if userDir != "" {
		for _, file := range []string{"settings.json", "cowork_settings.json"} {
			if err := read("user", filepath.Join(userDir, file), maxSettingsBytes, false); err != nil {
				return err
			}
		}
		if err := read("global", filepath.Join(userDir, ".config.json"), maxGlobalBytes, false); err != nil {
			return err
		}
	}
	// The runtime's global configuration is <dir>/.claude.json: its suffix
	// comes only from CLAUDE_CODE_CUSTOM_OAUTH_URL, a name the gateway
	// refuses and removes, so other .claude-*.json files are never selected
	// and are not read.
	if globalDir != "" {
		if err := read("global", filepath.Join(globalDir, ".claude.json"), maxGlobalBytes, false); err != nil {
			return err
		}
	}
	for _, dir := range settingsDirs(req.ProjectRoot) {
		for _, file := range []string{"settings.json", "settings.local.json"} {
			if err := read("project", filepath.Join(dir, ".claude", file), maxSettingsBytes, false); err != nil {
				return err
			}
		}
	}
	managed := req.ManagedSettingsDir
	if managed == "" {
		managed = managedSettingsDir()
	}
	if managed != "" {
		if err := read("managed", filepath.Join(managed, "managed-settings.json"), maxSettingsBytes, true); err != nil {
			return err
		}
		drop := filepath.Join(managed, "managed-settings.d")
		entries, err := os.ReadDir(drop)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return conflict("managed", "drop-ins cannot be listed for the filtering-gateway check")
		}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".json") {
				if err := read("managed", filepath.Join(drop, entry.Name()), maxSettingsBytes, true); err != nil {
					return err
				}
			}
		}
	}
	if err := nativeArgsConflict(name, req.NativeArgs); err != nil {
		return err
	}
	return nil
}

// childEnv is the environment the child is composed with, as far as the
// settings sources are concerned: the plan's environment, then the
// fragment's and the prompt's overlays, in composition's order. The network
// patch only adds the gateway's names, which every settings source refuses.
func childEnv(req Request) map[string]string {
	env := map[string]string{}
	for _, e := range req.ChildEnv {
		if k, v, ok := strings.Cut(e, "="); ok {
			env[k] = v
		}
	}
	for _, layer := range []map[string]string{req.FragEnv, req.PromptEnv} {
		for k, v := range layer {
			env[k] = v
		}
	}
	return env
}

// mdmDomain is Claude Code's managed-preferences domain on macOS, as the
// 2.1.314 bundle names it.
const mdmDomain = "com.anthropic.claudecode"

// admissionContract applies admission contract A for Claude Code under a
// filtering gateway (the pilot contract; settings reloaded during a session
// remain the written residual):
//   - the launch runs in a managed home (CLAUDE_CONFIG_DIR, an absolute ASCII
//     path, so the runtime's NFC-normalized path is the same) whose settings.json
//     exists and, like any cowork, global or legacy settings file there, is
//     not writable;
//   - its physical working directory is inside an operator project root;
//   - no Claude Code managed preferences (MDM) and no server-managed policy
//     cache is present on the host.
//
// Worktree and resume flags are refused by settingsConflict. A same-uid
// process can still change file modes back; the contract narrows, it does
// not isolate.
func admissionContract(name string, req Request, roots []string, mdmDir string, username string) error {
	refuse := func(detail string) error {
		return refusal.New(refusal.CodeScopeUnsupported, name, "filtering-gateway admission: "+detail)
	}
	home := childEnv(req)["CLAUDE_CONFIG_DIR"]
	if home == "" || !filepath.IsAbs(home) {
		return refuse("the launch does not run in a managed Claude home (CLAUDE_CONFIG_DIR)")
	}
	// Claude Code reads user settings from the NFC form of this path but the
	// global file from the raw one; an ASCII path is its own NFC form, so
	// every check here inspects the directories the runtime uses.
	for i := 0; i < len(home); i++ {
		if home[i] >= 0x80 {
			return refuse("the managed Claude home path must be ASCII")
		}
	}
	for _, file := range []string{"settings.json", "cowork_settings.json", ".config.json"} {
		info, err := os.Lstat(filepath.Join(home, file))
		if errors.Is(err, os.ErrNotExist) {
			if file == "settings.json" {
				return refuse("the managed home has no settings.json; it must exist and be read-only")
			}
			continue
		}
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o222 != 0 {
			return refuse("the managed home's " + file + " must be a read-only regular file")
		}
	}
	physical, err := filepath.EvalSymlinks(req.ProjectRoot)
	if err != nil {
		return refuse("the working directory does not resolve")
	}
	inside := false
	for _, root := range roots {
		if rel, err := filepath.Rel(root, physical); err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
			inside = true
			break
		}
	}
	if !inside {
		return refuse("the working directory is not inside an operator project root")
	}
	candidates := []string{filepath.Join(mdmDir, mdmDomain+".plist")}
	if username != "" {
		candidates = append(candidates, filepath.Join(mdmDir, username, mdmDomain+".plist"))
	}
	candidates = append(candidates, filepath.Join(home, "remote-settings.json"), filepath.Join(home, "policy-limits.json"))
	for _, path := range candidates {
		if _, err := os.Lstat(path); err == nil {
			return refuse("managed preferences or a server-managed policy is present (" + filepath.Base(path) + ")")
		} else if !errors.Is(err, os.ErrNotExist) {
			return refuse("managed preferences cannot be inspected")
		}
	}
	return nil
}
