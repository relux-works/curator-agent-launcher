// Package fragment implements SPEC.md §4.1, the context-plane step of the
// launcher: it runs `curator env resolve <env-id> [--profile <name>]
// --repair --format json` as a subprocess, parses the closed
// launch-env-fragment-v1 object (curator-spec environments.md §10.2 as
// revised by Decision 0012 D8, conformance schema
// launch-env-fragment-v1.schema.json), and computes the CCJ-1 digest
// (registry.md §1) from the parsed object.
//
// The package resolves and nothing else: it applies no channel, writes no
// managed home, composes no environment, and never falls back to a
// fragment-less launch. Every failure is one of the §6 resolve family.
package fragment

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Identity is the required value of the "fragment" member.
const Identity = "launch-env-fragment-v1"

// Environment identifiers of the closed adapter registry (environments.md
// §7.1). The launcher's own support for a launch into one of them is a §4.2
// fact, not this package's.
const (
	EnvClaudeCode = "claude_code"
	EnvCodexCLI   = "codex_cli"
	EnvOpenCode   = "opencode"
	EnvPi         = "pi"
)

// ChannelKind is the closed descriptor kind vocabulary (environments.md §7.3).
type ChannelKind string

const (
	KindFlag      ChannelKind = "flag"
	KindConfigKey ChannelKind = "config-key"
	KindVariable  ChannelKind = "variable"
	KindFile      ChannelKind = "file"
)

// Semantics is the closed system-prompt semantics vocabulary.
type Semantics string

const (
	SemanticsAppend  Semantics = "append"
	SemanticsReplace Semantics = "replace"
)

// Argument is the closed flag-argument vocabulary.
type Argument string

const (
	ArgumentPath     Argument = "path"
	ArgumentContents Argument = "contents"
	ArgumentName     Argument = "name"
)

// Channel is one channel descriptor. Exactly the members for its Kind are
// set; Semantics is set on system-prompt descriptors and empty on MCP
// descriptors, where the protocol forbids it.
type Channel struct {
	Kind      ChannelKind
	Semantics Semantics
	// flag
	Flag     string
	Argument Argument
	Name     string
	With     []string
	// config-key
	Key string
	// variable
	Variable string
	// file
	Filename string
}

// Profile is the fragment's profile object: the name and the lock hash
// (Decision 0012 D3: the lock hash is the identity).
type Profile struct {
	Name       string
	LockSHA256 string
}

// Precedence carries the two primitives (Decision 0012 D4); the launcher
// accepts and does not consume them.
type Precedence struct {
	Winner    string
	Placement string
}

// SystemPrompt is the optional system_prompt section: data about a channel,
// applied only under SPEC §5.
type SystemPrompt struct {
	Path     string
	Channels []Channel
}

// MCP is the optional mcp section (Decision 0012 D6).
type MCP struct {
	Path     string
	EnvNames []string
	Channels []Channel
}

// Fragment is a parsed, validated launch-env-fragment-v1.
type Fragment struct {
	Environment string
	Profile     Profile
	Precedence  Precedence
	// Env maps the adapter's registry-declared variable name to the managed
	// home path. Revision 1 declares exactly one variable per adapter.
	Env map[string]string
	// SystemPrompt is nil when the section is absent.
	SystemPrompt *SystemPrompt
	// MCP is nil when the section is absent.
	MCP *MCP
	// PathPrepend is the reserved optional member, empty when absent.
	PathPrepend string

	// Canonical is the CCJ-1 byte form of the parsed object and Digest is
	// "sha256:<hex>" over it (SPEC §4.1).
	Canonical []byte
	Digest    string
}

// HomeVariable returns the adapter's home variable name for env
// (environments.md §7.1), or "" for an environment outside the registry.
func HomeVariable(env string) string {
	return homeVariable[env]
}

// Home returns the managed home this launch runs in: the value of the
// adapter's home variable (SPEC §4.1, passed as LaunchRequest.Home in §4.4).
func (f *Fragment) Home() string {
	return f.Env[homeVariable[f.Environment]]
}

var homeVariable = map[string]string{
	EnvClaudeCode: "CLAUDE_CONFIG_DIR",
	EnvCodexCLI:   "CODEX_HOME",
	EnvOpenCode:   "XDG_CONFIG_HOME",
	EnvPi:         "PI_CODING_AGENT_DIR",
}

// The closed adapter channel registry (environments.md §7.3 and §7.8). A
// fragment's channels list must reproduce the adapter's descriptors exactly,
// in order; the conformance corpus rejects a descriptor the registry does
// not declare (invalid-*-channel-not-registry).
var systemPromptRegistry = map[string][]Channel{
	EnvClaudeCode: {
		{Kind: KindFlag, Semantics: SemanticsAppend, Flag: "--append-system-prompt-file", Argument: ArgumentPath},
		{Kind: KindFlag, Semantics: SemanticsReplace, Flag: "--system-prompt-file", Argument: ArgumentPath},
	},
	EnvCodexCLI: {
		{Kind: KindConfigKey, Semantics: SemanticsReplace, Key: "model_instructions_file"},
	},
	EnvOpenCode: {},
	EnvPi: {
		{Kind: KindFlag, Semantics: SemanticsAppend, Flag: "--append-system-prompt", Argument: ArgumentPath},
		{Kind: KindFile, Semantics: SemanticsAppend, Filename: "APPEND_SYSTEM.md"},
		{Kind: KindFile, Semantics: SemanticsReplace, Filename: "SYSTEM.md"},
	},
}

// mcpRegistry has no entry for pi: pi has no MCP channel and a pi fragment
// carrying an mcp section is rejected.
var mcpRegistry = map[string]Channel{
	EnvClaudeCode: {Kind: KindFlag, Flag: "--mcp-config", Argument: ArgumentPath, With: []string{"--strict-mcp-config"}},
	EnvCodexCLI:   {Kind: KindFlag, Flag: "-p", Argument: ArgumentName, Name: "curator-mcp"},
	EnvOpenCode:   {Kind: KindVariable, Variable: "OPENCODE_CONFIG"},
}

var (
	// identifierPattern is common.schema.json identifier.
	identifierPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9_-])?$`)
	windowsReserved   = regexp.MustCompile(`^(?i:con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)`)
	hex256Pattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	// reservedEnvName is agent-mcp-v1.schema.json envNames' exclusion list.
	reservedEnvName = regexp.MustCompile(`^(?:PATH|HOME|TMPDIR|TEMP|TMP|XDG_CONFIG_HOME|XDG_CACHE_HOME|XDG_DATA_HOME|XDG_STATE_HOME|HTTP_PROXY|HTTPS_PROXY|ALL_PROXY|FTP_PROXY|NO_PROXY|http_proxy|https_proxy|all_proxy|ftp_proxy|no_proxy|RES_OPTIONS|HOSTALIASES|LOCALDOMAIN|IFS|CSK_PROJECT_ROOT|USERPROFILE|APPDATA|LOCALAPPDATA|PATHEXT|COMSPEC|WINDIR|SYSTEMROOT|__PYVENV_LAUNCHER__|LD_.*|DYLD_.*|PYTHON.*|NODE_.*|NPM_CONFIG_.*)$`)
)

// InvalidError reports why bytes are not a valid closed fragment. Path is a
// JSON-pointer-like location ("/mcp/channels/0/with"); Msg says the rule.
type InvalidError struct {
	Path string
	Msg  string
}

func (e *InvalidError) Error() string {
	if e.Path == "" {
		return "fragment: " + e.Msg
	}
	return "fragment: " + e.Path + ": " + e.Msg
}

func invalid(path, format string, a ...any) error {
	return &InvalidError{Path: path, Msg: fmt.Sprintf(format, a...)}
}

// Parse validates data as one closed launch-env-fragment-v1 object and
// returns it with its CCJ-1 bytes and digest. Everything the reader rules
// of registry.md §1 reject (invalid UTF-8, duplicate keys, lone surrogates,
// non-integers, trailing content) is an error, as is every departure from
// the schema: a missing or extra member at any level, a wrong type, an
// unknown environment, kind, semantics, or argument value, a withdrawn
// `composition` member, a relative path or one with a `..` segment, a
// channel list that is not the adapter's registry list, an `mcp` section on
// `pi`, unsorted or reserved `env_names`, or a `path_prepend` outside the
// environments root derived from the managed home.
func Parse(data []byte) (*Fragment, error) {
	root, err := ParseJSON(data)
	if err != nil {
		return nil, err
	}
	f, err := fromValue(root)
	if err != nil {
		return nil, err
	}
	f.Canonical = Canonical(root)
	f.Digest = Digest(root)
	return f, nil
}

// closedObject checks v is an object whose keys are within allowed and
// include every member of required, reporting the first violation.
func closedObject(path string, v Value, required, optional []string) error {
	if v.Kind != KindObject {
		return invalid(path, "expected an object, got %s", v.Kind)
	}
	allowed := map[string]bool{}
	for _, k := range required {
		allowed[k] = true
	}
	for _, k := range optional {
		allowed[k] = true
	}
	for _, m := range v.Obj {
		if !allowed[m.Key] {
			if m.Key == "composition" {
				return invalid(path+"/"+m.Key, "member withdrawn under Decision 0012 D8")
			}
			return invalid(path+"/"+m.Key, "unknown member")
		}
	}
	for _, k := range required {
		if _, ok := v.Get(k); !ok {
			return invalid(path, "missing required member %q", k)
		}
	}
	return nil
}

func stringMember(path string, v Value, key string) (string, error) {
	m, _ := v.Get(key)
	if m.Kind != KindString {
		return "", invalid(path+"/"+key, "expected a string, got %s", m.Kind)
	}
	return m.Str, nil
}

func enumMember(path string, v Value, key string, allowed ...string) (string, error) {
	s, err := stringMember(path, v, key)
	if err != nil {
		return "", err
	}
	for _, a := range allowed {
		if s == a {
			return s, nil
		}
	}
	return "", invalid(path+"/"+key, "%q is not one of %s", s, strings.Join(allowed, ", "))
}

func checkIdentifier(path, s string) error {
	if len(s) > 128 || !identifierPattern.MatchString(s) || windowsReserved.MatchString(s) {
		return invalid(path, "%q is not an identifier", s)
	}
	return nil
}

// checkAbsolutePath is the schema's absolutePath: 2..4096 bytes, leading
// "/", no NUL, no ".." segment.
func checkAbsolutePath(path, s string) error {
	if len(s) < 2 || len(s) > 4096 || s[0] != '/' || strings.IndexByte(s, 0) >= 0 {
		return invalid(path, "%q is not an absolute path", s)
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == ".." {
			return invalid(path, "%q contains a .. segment", s)
		}
	}
	return nil
}

func checkFlagToken(path, s string) error {
	if len(s) < 2 || len(s) > 128 || s[0] != '-' || strings.ContainsAny(s, " \t\n\r\f\v") {
		return invalid(path, "%q is not a flag token", s)
	}
	return nil
}

func fromValue(root Value) (*Fragment, error) {
	if err := closedObject("", root, []string{"fragment", "environment", "profile", "precedence", "env"}, []string{"system_prompt", "mcp", "path_prepend"}); err != nil {
		return nil, err
	}
	f := &Fragment{}

	if _, err := enumMember("", root, "fragment", Identity); err != nil {
		return nil, err
	}
	env, err := enumMember("", root, "environment", EnvClaudeCode, EnvCodexCLI, EnvOpenCode, EnvPi)
	if err != nil {
		return nil, err
	}
	f.Environment = env

	// profile
	prof, _ := root.Get("profile")
	if err := closedObject("/profile", prof, []string{"name", "lock_sha256"}, nil); err != nil {
		return nil, err
	}
	if f.Profile.Name, err = stringMember("/profile", prof, "name"); err != nil {
		return nil, err
	}
	if err := checkIdentifier("/profile/name", f.Profile.Name); err != nil {
		return nil, err
	}
	if f.Profile.LockSHA256, err = stringMember("/profile", prof, "lock_sha256"); err != nil {
		return nil, err
	}
	if !hex256Pattern.MatchString(f.Profile.LockSHA256) {
		return nil, invalid("/profile/lock_sha256", "expected 64 lowercase hex digits without a prefix")
	}

	// precedence
	prec, _ := root.Get("precedence")
	if err := closedObject("/precedence", prec, []string{"winner", "placement"}, nil); err != nil {
		return nil, err
	}
	if f.Precedence.Winner, err = enumMember("/precedence", prec, "winner", "higher-weight", "lower-weight"); err != nil {
		return nil, err
	}
	if f.Precedence.Placement, err = enumMember("/precedence", prec, "placement", "winner-last", "winner-first"); err != nil {
		return nil, err
	}

	// env: exactly the adapter's one home variable, an absolute path.
	envObj, _ := root.Get("env")
	if envObj.Kind != KindObject {
		return nil, invalid("/env", "expected an object, got %s", envObj.Kind)
	}
	if len(envObj.Obj) != 1 {
		return nil, invalid("/env", "expected exactly one variable, got %d", len(envObj.Obj))
	}
	want := homeVariable[env]
	m := envObj.Obj[0]
	if m.Key != want {
		return nil, invalid("/env/"+m.Key, "not the %s home variable %s", env, want)
	}
	if m.Value.Kind != KindString {
		return nil, invalid("/env/"+m.Key, "expected a string, got %s", m.Value.Kind)
	}
	if err := checkAbsolutePath("/env/"+m.Key, m.Value.Str); err != nil {
		return nil, err
	}
	f.Env = map[string]string{m.Key: m.Value.Str}

	// system_prompt
	if sp, ok := root.Get("system_prompt"); ok {
		if err := closedObject("/system_prompt", sp, []string{"path", "channels"}, nil); err != nil {
			return nil, err
		}
		s := &SystemPrompt{}
		if s.Path, err = stringMember("/system_prompt", sp, "path"); err != nil {
			return nil, err
		}
		if err := checkAbsolutePath("/system_prompt/path", s.Path); err != nil {
			return nil, err
		}
		ch, _ := sp.Get("channels")
		if s.Channels, err = channels("/system_prompt/channels", ch, true); err != nil {
			return nil, err
		}
		if !sameChannels(s.Channels, systemPromptRegistry[env]) {
			return nil, invalid("/system_prompt/channels", "not the %s adapter's registry descriptors", env)
		}
		f.SystemPrompt = s
	}

	// mcp
	if mv, ok := root.Get("mcp"); ok {
		reg, has := mcpRegistry[env]
		if !has {
			return nil, invalid("/mcp", "%s has no MCP channel", env)
		}
		if err := closedObject("/mcp", mv, []string{"path", "env_names", "channels"}, nil); err != nil {
			return nil, err
		}
		mc := &MCP{}
		if mc.Path, err = stringMember("/mcp", mv, "path"); err != nil {
			return nil, err
		}
		if err := checkAbsolutePath("/mcp/path", mc.Path); err != nil {
			return nil, err
		}
		names, _ := mv.Get("env_names")
		if names.Kind != KindArray {
			return nil, invalid("/mcp/env_names", "expected an array, got %s", names.Kind)
		}
		mc.EnvNames = []string{}
		for i, n := range names.Arr {
			p := fmt.Sprintf("/mcp/env_names/%d", i)
			if n.Kind != KindString {
				return nil, invalid(p, "expected a string, got %s", n.Kind)
			}
			if err := checkIdentifier(p, n.Str); err != nil {
				return nil, err
			}
			if reservedEnvName.MatchString(n.Str) {
				return nil, invalid(p, "%q is a reserved variable name", n.Str)
			}
			if i > 0 && n.Str <= mc.EnvNames[i-1] {
				return nil, invalid(p, "env_names must be sorted and unique")
			}
			mc.EnvNames = append(mc.EnvNames, n.Str)
		}
		ch, _ := mv.Get("channels")
		if mc.Channels, err = channels("/mcp/channels", ch, false); err != nil {
			return nil, err
		}
		if !sameChannels(mc.Channels, []Channel{reg}) {
			return nil, invalid("/mcp/channels", "not the %s adapter's registry descriptor", env)
		}
		f.MCP = mc
	}

	// path_prepend: reserved; must lie below the environments root, which
	// is two segments above the managed home (<root>/<profile>/<env>).
	if pp, ok := root.Get("path_prepend"); ok {
		if pp.Kind != KindString {
			return nil, invalid("/path_prepend", "expected a string, got %s", pp.Kind)
		}
		if err := checkAbsolutePath("/path_prepend", pp.Str); err != nil {
			return nil, err
		}
		rootDir := environmentsRoot(f.Home())
		if rootDir == "" || !strings.HasPrefix(pp.Str, rootDir+"/") {
			return nil, invalid("/path_prepend", "%q is outside the environments root %q", pp.Str, rootDir)
		}
		f.PathPrepend = pp.Str
	}
	return f, nil
}

// environmentsRoot derives the manager-owned environments root from a
// managed-home path of the layout <root>/<profile>/<env-id>
// (environments.md §8.1), or "" when the path is too shallow.
func environmentsRoot(home string) string {
	home = strings.TrimRight(home, "/")
	i := strings.LastIndexByte(home, '/')
	if i <= 0 {
		return ""
	}
	j := strings.LastIndexByte(home[:i], '/')
	if j <= 0 {
		return ""
	}
	return home[:j]
}

// channels parses a descriptor list. withSemantics selects the
// system-prompt grammar (semantics required) over the MCP grammar
// (semantics forbidden).
func channels(path string, v Value, withSemantics bool) ([]Channel, error) {
	if v.Kind != KindArray {
		return nil, invalid(path, "expected an array, got %s", v.Kind)
	}
	out := []Channel{}
	for i, el := range v.Arr {
		c, err := channel(fmt.Sprintf("%s/%d", path, i), el, withSemantics)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func channel(path string, v Value, withSemantics bool) (Channel, error) {
	if v.Kind != KindObject {
		return Channel{}, invalid(path, "expected an object, got %s", v.Kind)
	}
	kind, err := enumMember(path, v, "kind", string(KindFlag), string(KindConfigKey), string(KindVariable), string(KindFile))
	if err != nil {
		return Channel{}, err
	}
	c := Channel{Kind: ChannelKind(kind)}
	required := []string{"kind"}
	var optional []string
	if withSemantics {
		required = append(required, "semantics")
	}
	switch c.Kind {
	case KindFlag:
		required = append(required, "flag", "argument")
		optional = []string{"name", "with"}
	case KindConfigKey:
		required = append(required, "key")
	case KindVariable:
		required = append(required, "variable")
	case KindFile:
		required = append(required, "filename")
	}
	if err := closedObject(path, v, required, optional); err != nil {
		return Channel{}, err
	}
	if withSemantics {
		s, err := enumMember(path, v, "semantics", string(SemanticsAppend), string(SemanticsReplace))
		if err != nil {
			return Channel{}, err
		}
		c.Semantics = Semantics(s)
	}
	switch c.Kind {
	case KindFlag:
		if c.Flag, err = stringMember(path, v, "flag"); err != nil {
			return Channel{}, err
		}
		if err := checkFlagToken(path+"/flag", c.Flag); err != nil {
			return Channel{}, err
		}
		arg, err := enumMember(path, v, "argument", string(ArgumentPath), string(ArgumentContents), string(ArgumentName))
		if err != nil {
			return Channel{}, err
		}
		c.Argument = Argument(arg)
		name, hasName := v.Get("name")
		if c.Argument == ArgumentName && !hasName {
			return Channel{}, invalid(path, "argument \"name\" requires a name member")
		}
		if c.Argument != ArgumentName && hasName {
			return Channel{}, invalid(path+"/name", "name is only allowed with argument \"name\"")
		}
		if hasName {
			if name.Kind != KindString {
				return Channel{}, invalid(path+"/name", "expected a string, got %s", name.Kind)
			}
			if err := checkIdentifier(path+"/name", name.Str); err != nil {
				return Channel{}, err
			}
			c.Name = name.Str
		}
		if with, ok := v.Get("with"); ok {
			if with.Kind != KindArray {
				return Channel{}, invalid(path+"/with", "expected an array, got %s", with.Kind)
			}
			if len(with.Arr) == 0 {
				return Channel{}, invalid(path+"/with", "must not be empty when present")
			}
			seen := map[string]bool{}
			for i, w := range with.Arr {
				p := fmt.Sprintf("%s/with/%d", path, i)
				if w.Kind != KindString {
					return Channel{}, invalid(p, "expected a string, got %s", w.Kind)
				}
				if err := checkFlagToken(p, w.Str); err != nil {
					return Channel{}, err
				}
				if seen[w.Str] {
					return Channel{}, invalid(p, "duplicate companion flag %q", w.Str)
				}
				seen[w.Str] = true
				c.With = append(c.With, w.Str)
			}
		}
	case KindConfigKey:
		if c.Key, err = stringMember(path, v, "key"); err != nil {
			return Channel{}, err
		}
		if err := checkIdentifier(path+"/key", c.Key); err != nil {
			return Channel{}, err
		}
	case KindVariable:
		if c.Variable, err = stringMember(path, v, "variable"); err != nil {
			return Channel{}, err
		}
		if err := checkIdentifier(path+"/variable", c.Variable); err != nil {
			return Channel{}, err
		}
	case KindFile:
		if c.Filename, err = stringMember(path, v, "filename"); err != nil {
			return Channel{}, err
		}
		if err := checkPortableFilename(path+"/filename", c.Filename); err != nil {
			return Channel{}, err
		}
	}
	return c, nil
}

// checkPortableFilename is common.schema.json portablePath narrowed to one
// segment, which is all a registry file channel names: no separators, no
// control or C1 characters, no leading "/" or trailing space or dot, not a
// dot segment or Windows reserved name.
func checkPortableFilename(path, s string) error {
	if s == "" || len(s) > 4096 || strings.ContainsAny(s, "/\\:") {
		return invalid(path, "%q is not a portable filename", s)
	}
	for _, r := range s {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return invalid(path, "%q contains a control character", s)
		}
	}
	if strings.HasSuffix(s, " ") || strings.HasSuffix(s, ".") || s == "." || s == ".." || windowsReserved.MatchString(s) {
		return invalid(path, "%q is not a portable filename", s)
	}
	return nil
}

func sameChannels(a, b []Channel) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.Kind != y.Kind || x.Semantics != y.Semantics || x.Flag != y.Flag || x.Argument != y.Argument ||
			x.Name != y.Name || x.Key != y.Key || x.Variable != y.Variable || x.Filename != y.Filename {
			return false
		}
		if len(x.With) != len(y.With) {
			return false
		}
		for j := range x.With {
			if x.With[j] != y.With[j] {
				return false
			}
		}
	}
	return true
}

// IsInvalid reports whether err is a fragment validity failure — a reader
// rule or schema rule — as opposed to a subprocess failure.
func IsInvalid(err error) bool {
	var inv *InvalidError
	var syn *SyntaxError
	return errors.As(err, &inv) || errors.As(err, &syn) || errors.Is(err, ErrInvalidUTF8)
}
