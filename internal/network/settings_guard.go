package network

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

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

// claudeDispatchNames and claudeDispatchPrefixes are environment names the
// 2.1.314 runtime reads to choose another mode, session or source before or
// beside its settings: the deep-link URL handler (__CFBundleIdentifier),
// feature-flag overrides, background session kind, the environment halves
// of the safe, bare, restricted and agent-teams mode getters, the entrypoint
// selectors, plugin directories, and the remote-session families (CCR_,
// CLAUDE_CODE_REMOTE), whose names include proxies, relays and egress
// gateways. Under the filtering gateway
// they refuse like routes, and inherited values are removed.
var claudeDispatchNames = map[string]bool{
	"__CFBUNDLEIDENTIFIER":                 true,
	"CLAUDE_INTERNAL_FC_OVERRIDES":         true,
	"CLAUDE_CODE_SESSION_KIND":             true,
	"CLAUDE_CODE_SAFE_MODE":                true,
	"CLAUDE_CODE_SIMPLE":                   true,
	"CLAUDE_CODE_RESTRICTED":               true,
	"CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS": true,
	"CLAUDE_CODE_ACTION":                   true,
	"CLAUDE_CODE_ENTRYPOINT":               true,
}

var claudeDispatchPrefixes = []string{"CCR_", "CLAUDE_CODE_REMOTE", "CLAUDE_CODE_PLUGIN_"}

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
	if claudeDispatchNames[upper] {
		return true
	}
	for _, prefix := range claudeDispatchPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
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

// selectedGlobalFile mirrors the 2.1.314 runtime's choice of its global
// configuration file (y_ and XSr in the bundle) for a launch that is not
// diskless: the legacy <config dir>/.config.json when it is a regular file
// that is non-empty, or writable while <dir>/.claude.json does not exist;
// otherwise <dir>/.claude.json. Its suffix comes only from
// CLAUDE_CODE_CUSTOM_OAUTH_URL, which the gateway refuses and removes. Both
// lookups follow links, as the runtime's do. Only the selected file is a
// source; the other is not read.
func selectedGlobalFile(clock guardClock, name, configDir, globalDir string) (string, error) {
	selected := filepath.Join(globalDir, ".claude.json")
	if configDir == "" {
		return selected, nil
	}
	legacy := filepath.Join(configDir, ".config.json")
	if err := clock.step(name); err != nil {
		return "", err
	}
	info, err := os.Stat(legacy)
	if err != nil || !info.Mode().IsRegular() {
		return selected, nil
	}
	if info.Size() > 0 {
		return legacy, nil
	}
	if info.Mode().Perm()&0o222 != 0 {
		if err := clock.step(name); err != nil {
			return "", err
		}
		if _, err := os.Stat(selected); err != nil {
			return legacy, nil
		}
	}
	return selected, nil
}

// settingsChunk is the size of one read of a settings source.
const settingsChunk = 64 << 10

// readSettingsChunk performs one read of an open settings source; tests
// replace it to act between reads.
var readSettingsChunk = func(f *os.File, p []byte) (int, error) { return f.Read(p) }

// clockStop carries a check's interruption or spent budget out of a helper,
// so the refusal says that and not that the source was unreadable.
type clockStop struct{ error }

// inspectPath inspects a settings or policy path without following a final
// link; tests replace it to act at an exact inspection.
var inspectPath = os.Lstat

// listDropIns lists at most n entries of a managed drop-in directory, which
// must be a directory: it is opened as one, without blocking, so a FIFO or
// file there refuses, and a huge directory is never read beyond n entries.
// Tests replace it to observe n.
var listDropIns = func(dir string, n int, step func() error) ([]os.DirEntry, error) {
	f, err := os.OpenFile(dir, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := step(); err != nil {
		return nil, err
	}
	entries, err := f.ReadDir(n)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return entries, err
}

// openSettingsFile opens a settings source without blocking on a FIFO; tests
// replace it to stage a replacement or a slow open.
var openSettingsFile = func(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}

// readRegular reads a settings source without blocking on a special file:
// a final symbolic link is followed only to a regular file, the file is
// opened non-blocking, and the opened file must be the inspected regular
// file. A missing directory entry is errAbsent; anything else that is not a
// readable regular file within limit is an error.
func readRegular(clock guardClock, name, path string, limit int64) ([]byte, error) {
	step := func() error {
		if err := clock.step(name); err != nil {
			return clockStop{err}
		}
		return nil
	}
	if err := step(); err != nil {
		return nil, err
	}
	if _, err := inspectPath(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errAbsent
		}
		return nil, err
	}
	if err := step(); err != nil {
		return nil, err
	}
	target, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !target.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if err := step(); err != nil {
		return nil, err
	}
	f, err := openSettingsFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := step(); err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(target, opened) {
		return nil, errors.New("changed while being opened")
	}
	// Read in chunks, checking the clock before each one, so a check that is
	// interrupted or out of time stops between reads.
	var doc []byte
	buf := make([]byte, settingsChunk)
	for {
		if err := step(); err != nil {
			return nil, err
		}
		n, err := readSettingsChunk(f, buf)
		doc = append(doc, buf[:n]...)
		if int64(len(doc)) > limit {
			return nil, errors.New("not readable within its limit")
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("not readable within its limit")
		}
	}
	if err := step(); err != nil {
		return nil, err
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
// guardBudget bounds one settings and contract check, before register or at
// the pre-spawn recheck; maxManagedDropIns bounds the drop-ins it reads.
const (
	guardBudget       = 5 * time.Second
	maxManagedDropIns = 64
)

// guardClock is checked before every file operation of a check: a cancelled
// context (a held signal) or a spent budget ends the check with a refusal
// instead of letting it run on. A single system call that stalls inside the
// kernel (a hung network filesystem) is not interrupted.
type guardClock struct {
	ctx      context.Context
	deadline time.Time
}

func newGuardClock(ctx context.Context, budget time.Duration) guardClock {
	if budget <= 0 {
		budget = guardBudget
	}
	return guardClock{ctx: ctx, deadline: time.Now().Add(budget)}
}

func (c guardClock) step(name string) error {
	if c.ctx.Err() != nil {
		return refusal.New(refusal.CodeConfigurationConflict, name, "the filtering-gateway settings check was interrupted")
	}
	if time.Now().After(c.deadline) {
		return refusal.New(refusal.CodeConfigurationConflict, name, "the filtering-gateway settings check did not finish within its time budget")
	}
	return nil
}

func settingsConflict(clock guardClock, name string, req Request) error {
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
		if err := clock.step(name); err != nil {
			return err
		}
		doc, err := readRegular(clock, name, path, limit)
		var stop clockStop
		if errors.As(err, &stop) {
			return stop.error
		}
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
	}
	if globalDir != "" {
		if err := clock.step(name); err != nil {
			return err
		}
		global, err := selectedGlobalFile(clock, name, userDir, globalDir)
		if err != nil {
			return err
		}
		if err := read("global", global, maxGlobalBytes, false); err != nil {
			return err
		}
	}
	if err := clock.step(name); err != nil {
		return err
	}
	dirs := settingsDirs(req.ProjectRoot)
	if err := clock.step(name); err != nil {
		return err
	}
	for _, dir := range dirs {
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
		if err := clock.step(name); err != nil {
			return err
		}
		entries, err := listDropIns(drop, maxManagedDropIns+1, func() error {
			if err := clock.step(name); err != nil {
				return clockStop{err}
			}
			return nil
		})
		var stop clockStop
		if errors.As(err, &stop) {
			return stop.error
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return conflict("managed", "drop-ins cannot be listed for the filtering-gateway check")
		}
		if len(entries) > maxManagedDropIns {
			return conflict("managed", "has more drop-ins than the filtering-gateway check reads")
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

// managedHome returns the child's managed Claude home, refusing a selector
// that is missing, relative or not ASCII. It reads no file, so the guard runs
// it before discovering any settings source.
func managedHome(name string, req Request) (string, error) {
	refuse := func(detail string) error {
		return refusal.New(refusal.CodeScopeUnsupported, name, "filtering-gateway admission: "+detail)
	}
	home := childEnv(req)["CLAUDE_CONFIG_DIR"]
	if home == "" || !filepath.IsAbs(home) {
		return "", refuse("the launch does not run in a managed Claude home (CLAUDE_CONFIG_DIR)")
	}
	// Claude Code reads user settings from the NFC form of this path but the
	// global file from the raw one; an ASCII path is its own NFC form, so
	// every check inspects the directories the runtime uses.
	for i := 0; i < len(home); i++ {
		if home[i] >= 0x80 {
			return "", refuse("the managed Claude home path must be ASCII")
		}
	}
	return home, nil
}

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
// "Exact model" is the exact qualified build, enforced by the strict tuple;
// the LLM model is not checked. Worktree, resume and remote placement are
// refused by the native-tail allowlist. The contract narrows, it does not
// isolate; its residual (README) is a settings reload during the session,
// same-uid file modes, the writable .claude.json the runtime itself writes,
// and server policy fetched at start or refresh, of which only the local
// traces are checked.
func admissionContract(clock guardClock, name string, req Request, roots []string, mdmDir string, username string) error {
	refuse := func(detail string) error {
		return refusal.New(refusal.CodeScopeUnsupported, name, "filtering-gateway admission: "+detail)
	}
	if err := clock.step(name); err != nil {
		return err
	}
	home, err := managedHome(name, req)
	if err != nil {
		return err
	}
	for _, file := range []string{"settings.json", "cowork_settings.json", ".config.json"} {
		if err := clock.step(name); err != nil {
			return err
		}
		info, err := inspectPath(filepath.Join(home, file))
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
	if err := clock.step(name); err != nil {
		return err
	}
	physical, err := filepath.EvalSymlinks(req.ProjectRoot)
	if err != nil {
		return refuse("the working directory does not resolve")
	}
	if err := clock.step(name); err != nil {
		return err
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
		if err := clock.step(name); err != nil {
			return err
		}
		if _, err := inspectPath(path); err == nil {
			return refuse("managed preferences or a server-managed policy is present (" + filepath.Base(path) + ")")
		} else if !errors.Is(err, os.ErrNotExist) {
			return refuse("managed preferences cannot be inspected")
		}
	}
	return nil
}
