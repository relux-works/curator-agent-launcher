// Package cli implements the closed launcher command-line surface of
// SPEC.md §3: the argv grammar, the usage errors of §6's usage family, and
// the verbatim native tail after "--".
//
// The package parses only. It resolves no fragment, reads no configuration
// file, requests no plan, and starts no process. Whether the machine's ax
// integration is configured (SPEC §4.6, ax.json) is a fact the caller
// establishes before parsing and supplies through Options, so the two
// usage rules that depend on it — "--ax-profile on an untracked machine is
// a usage error" and "--name on an untracked machine is accepted and has no
// effect" — are expressed here without this package owning the file read.
//
// Environment identifiers are not validated against a registry here: an
// unsupported environment is env_unsupported or resolve_environment_unknown
// in the later stages that own those facts (SPEC §4.1, §4.2). The two CLI
// aliases (SPEC §3) are normalized here, before any validation or lookup,
// so every later stage sees only canonical ids.
package cli

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Name is the executable name the launcher reports in every message.
const Name = "curator-run"

// Usage is the usage text printed after every usage error and by --help.
const Usage = `usage: curator-run <env-id> [--profile <name>] [--system-prompt <append|replace>]
                   [--model <model>] [--effort <effort>]
                   [--permissions <native|yolo> | --yolo]
                   [--name <session-name>] [--ax-profile <standard|yolo>]
                   [--] <native args...>
       curator-run --help | -h
       curator-run --version

Launcher flags are recognized only before "--". Everything after "--" is
forwarded to the tool verbatim, in order, uninspected.

options:
  --profile <name>                  Curator profile forwarded to env resolve
  --system-prompt <append|replace>  engage the fragment's system-prompt channel
  --model <model>                   model passed to the spawn plane as declared
  --effort <effort>                 reasoning effort passed as declared
  --permissions <native|yolo>        permission mode passed to the spawn plane
  --yolo                             alias for --permissions yolo
  -d, --danger                       rejected; use --permissions explicitly
  --name <session-name>             ax session name (tracked machines only)
  --ax-profile <standard|yolo>      ax execution profile (tracked machines only)
  --help, -h                        print this usage text and exit 0
  --version                         print the launcher name and version, exit 0

Environments: claude_code (alias claude), codex_cli (alias codex), pi.
opencode is currently unsupported. Aliases normalize to the canonical id
before validation or lookup; outputs carry the canonical id only.
Omitting --profile uses Curator's current profile; resolution always repairs.
Use curator run <env> --profile <p> -- <args> for umbrella discovery.
Install curator-run on trusted PATH (e.g. /usr/local/bin), not Curator's
~/.local/bin shim directory or another managed directory.

Defaults: flags > operator > machine > admitted lineup, per model/effort.
Files: $XDG_CONFIG_HOME/curator-run/defaults.json (default
~/.config/curator-run/defaults.json) and /etc/curator-run/defaults.json.
Machine locked:true ignores operator entries for its named environments and
rejects flags overriding members it sets. Pi fallback preference is
pi-anthropic, pi-openai, pi-google; explicit models bind their own runtime.
Pi has no MCP channel. Prompt opt-in defaults off; native prompt-file
candidates can still produce warnings without opt-in.

Tracking: sibling ax.json, schema curator-run-ax-v1, enabled:true.
An existing machine ax.json wins; otherwise use the operator file.
Missing files or enabled:false select direct execution. Invalid/unreadable
configuration refuses even --help/--version. No direct fallback on ax failure.
--name defaults to <env-id>-<YYYYMMDDTHHMMSSZ> (UTC), accepts
[A-Za-z0-9][A-Za-z0-9._-]{0,63}, and has no effect when untracked.
--ax-profile is a usage error when untracked; absent uses ax's default.
Repeated/unknown flags, missing values and extra operands before -- refuse.

Exit codes: 0 success; 2 usage; 1 operational refusal. Direct child exit codes
propagate unchanged; signal exits are 128 + signal. Stable diagnostic codes
and details go to stderr; see README.md and SPEC.md section 6.
`

// Options carries the facts the parser needs from its caller.
type Options struct {
	// AxConfigured is true when the ax integration is configured on this
	// machine (SPEC §4.6). It decides whether --ax-profile is admitted and
	// whether --name will have an effect. The caller reads ax.json; this
	// package never does.
	AxConfigured bool
}

// Info names an informational outcome: the invocation asked for text, not a
// launch.
type Info int

const (
	// InfoNone means the invocation is a launch request.
	InfoNone Info = iota
	// InfoHelp means --help or -h was given.
	InfoHelp
	// InfoVersion means --version was given.
	InfoVersion
)

// SystemPrompt is the closed vocabulary of --system-prompt (SPEC §3, §5).
type SystemPrompt string

const (
	// SystemPromptNone means the flag was absent: no opt-in.
	SystemPromptNone SystemPrompt = ""
	// SystemPromptAppend engages the channel with append semantics.
	SystemPromptAppend SystemPrompt = "append"
	// SystemPromptReplace engages the channel with replace semantics.
	SystemPromptReplace SystemPrompt = "replace"
)

// AxProfile is the closed vocabulary of --ax-profile (SPEC §3, §4.6).
type AxProfile string

const (
	// AxProfileNone means the flag was absent: ax's own default applies.
	AxProfileNone AxProfile = ""
	// AxProfileStandard forwards "--profile standard" to ax start.
	AxProfileStandard AxProfile = "standard"
	// AxProfileYolo forwards "--profile yolo" to ax start.
	AxProfileYolo AxProfile = "yolo"
)

// PermissionMode is the closed vocabulary of --permissions and its alias.
type PermissionMode string

const (
	PermissionNative PermissionMode = "native"
	PermissionYolo   PermissionMode = "yolo"
)

// sessionNamePattern is the ax §2.1 session-name grammar, anchored.
var sessionNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// NormalizeEnvID maps the two SPEC §3 CLI aliases to their canonical
// environment ids: claude -> claude_code, codex -> codex_cli. Every other
// spelling, including unknown ids, is returned unchanged so the later
// stages that own those facts refuse it as today. The mapping is exact
// and case-sensitive; it never trims, folds, or prefixes.
func NormalizeEnvID(id string) string {
	switch id {
	case "claude":
		return "claude_code"
	case "codex":
		return "codex_cli"
	default:
		return id
	}
}

// Invocation is a successfully parsed command line. Every member is as
// typed, except EnvID which carries the normalized canonical id (SPEC §3);
// nothing is resolved, defaulted, or validated beyond §3.
type Invocation struct {
	// Info is non-zero for --help / -h / --version; every other member is
	// then zero and the caller prints and exits 0.
	Info Info

	// EnvID is the required operand, normalized through NormalizeEnvID.
	EnvID string
	// Profile is the --profile value; ProfileSet tells absence from a
	// value that was never given (an empty value is a usage error, so
	// ProfileSet implies a non-empty Profile).
	Profile    string
	ProfileSet bool
	// SystemPrompt is the opt-in semantics, or SystemPromptNone.
	SystemPrompt SystemPrompt
	// Model and Effort are §4.3 level 1, verbatim and unvalidated.
	Model, Effort       string
	ModelSet, EffortSet bool
	// PermissionMode is the explicit §4.3 mode or its --yolo alias.
	PermissionMode PermissionMode
	PermissionSet  bool
	// Name is the --name value, already validated against the ax §2.1
	// grammar. NameSet reports presence. On an untracked machine
	// (Tracked false) the value is accepted and has no effect.
	Name    string
	NameSet bool
	// AxProfile is the --ax-profile value or AxProfileNone. It is never set
	// when Tracked is false: that shape is a usage error.
	AxProfile AxProfile
	// Tracked copies Options.AxConfigured so consumers of the invocation
	// see the fact the parse was made against.
	Tracked bool
	// Native is everything after "--", verbatim, in order. It is never nil
	// on a launch invocation; it is empty when "--" was absent or last.
	Native []string
}

// UsageError is the §6 usage family: exit 2, nothing resolved, nothing
// launched. Code is always "usage"; Detail is the human-oriented message.
type UsageError struct {
	Detail string
}

// Code is the stable diagnostic code of the usage family.
func (e *UsageError) Code() string { return "usage" }

func (e *UsageError) Error() string { return "usage: " + e.Detail }

// ExitCode is the process exit status for every usage error.
const ExitCode = 2

func usageErr(format string, a ...any) error {
	return &UsageError{Detail: fmt.Sprintf(format, a...)}
}

// IsUsage reports whether err is a usage error.
func IsUsage(err error) bool {
	var u *UsageError
	return errors.As(err, &u)
}

// valueFlags is the closed set of value-taking launcher flags.
var valueFlags = map[string]bool{
	"--profile":       true,
	"--system-prompt": true,
	"--model":         true,
	"--effort":        true,
	"--permissions":   true,
	"--name":          true,
	"--ax-profile":    true,
}

// Parse classifies args (os.Args[1:]) under SPEC §3. The rules, closed:
//
//   - Tokens are read left to right until "--". The first "--" ends launcher
//     parsing; every later token, including a second "--", an empty string,
//     or a token spelled like a launcher flag, is native argv verbatim.
//   - "--help", "-h", and "--version" at flag position are informational: the
//     first one met wins immediately, the rest of the line is not read, and
//     the result carries Info only. A usage error met earlier wins over a
//     later informational flag; the line is read once, in order.
//   - A value-taking flag takes exactly the next token as its value, in the
//     form "--flag value" or "--flag=value". A flag with no following token,
//     one followed by "--", or one whose value is empty or begins with "-"
//     is a missing value. A repeated flag is a usage error, never last-wins.
//   - The first token not beginning with "-" is <env-id>; a second one is a
//     stray operand. Any other token beginning with "-" is an unknown flag.
//     The operand is normalized through NormalizeEnvID before it is stored,
//     so an alias never reaches validation, lookup, or output.
//   - "--system-prompt" and "--ax-profile" accept only their vocabularies;
//     "--name" must match the ax §2.1 grammar; "--ax-profile" requires
//     opts.AxConfigured.
//   - <env-id> is required on a launch invocation.
func Parse(args []string, opts Options) (Invocation, error) {
	inv := Invocation{Tracked: opts.AxConfigured}
	seen := map[string]bool{}
	envSet := false
	permissionSeen := false

	i := 0
	for ; i < len(args); i++ {
		tok := args[i]
		if tok == "--" {
			i++
			break
		}
		switch tok {
		case "--help", "-h":
			return Invocation{Info: InfoHelp}, nil
		case "--version":
			return Invocation{Info: InfoVersion}, nil
		}
		if !strings.HasPrefix(tok, "-") {
			if envSet {
				return inv, usageErr("stray operand %q before --: native arguments must follow --", tok)
			}
			if tok == "" {
				return inv, usageErr("empty operand before --: <env-id> must not be empty")
			}
			inv.EnvID = NormalizeEnvID(tok)
			envSet = true
			continue
		}

		flag, value, hasEq := strings.Cut(tok, "=")
		if flag == "-d" || flag == "--danger" {
			return inv, usageErr("%s is rejected; use --permissions native or --permissions yolo", flag)
		}
		if flag == "--yolo" {
			if hasEq {
				return inv, usageErr("--yolo is an alias without a value")
			}
			if permissionSeen {
				return inv, usageErr("--permissions and --yolo are one permission request and cannot be repeated or combined")
			}
			permissionSeen = true
			inv.PermissionMode, inv.PermissionSet = PermissionYolo, true
			continue
		}
		if flag == "--permissions" {
			if permissionSeen {
				return inv, usageErr("--permissions and --yolo are one permission request and cannot be repeated or combined")
			}
			permissionSeen = true
		}
		if !valueFlags[flag] {
			return inv, usageErr("unknown flag %q before --", tok)
		}
		if seen[flag] {
			return inv, usageErr("flag %s given more than once", flag)
		}
		seen[flag] = true
		if !hasEq {
			if i+1 >= len(args) || args[i+1] == "--" {
				return inv, usageErr("flag %s requires a value", flag)
			}
			i++
			value = args[i]
		}
		if value == "" || strings.HasPrefix(value, "-") {
			return inv, usageErr("flag %s requires a value, got %q", flag, value)
		}
		if err := inv.set(flag, value, opts); err != nil {
			return inv, err
		}
	}

	if !envSet {
		return inv, usageErr("missing <env-id>")
	}
	inv.Native = append([]string{}, args[i:]...)
	return inv, nil
}

// set stores one validated flag value.
func (inv *Invocation) set(flag, value string, opts Options) error {
	switch flag {
	case "--profile":
		inv.Profile, inv.ProfileSet = value, true
	case "--model":
		inv.Model, inv.ModelSet = value, true
	case "--effort":
		inv.Effort, inv.EffortSet = value, true
	case "--permissions":
		switch PermissionMode(value) {
		case PermissionNative, PermissionYolo:
			inv.PermissionMode, inv.PermissionSet = PermissionMode(value), true
		default:
			return usageErr("--permissions accepts native or yolo, got %q", value)
		}
	case "--system-prompt":
		switch SystemPrompt(value) {
		case SystemPromptAppend, SystemPromptReplace:
			inv.SystemPrompt = SystemPrompt(value)
		default:
			return usageErr("--system-prompt accepts append or replace, got %q", value)
		}
	case "--name":
		if !sessionNamePattern.MatchString(value) {
			return usageErr("--name %q is not a valid ax session name: [A-Za-z0-9][A-Za-z0-9._-]{0,63}", value)
		}
		inv.Name, inv.NameSet = value, true
	case "--ax-profile":
		switch AxProfile(value) {
		case AxProfileStandard, AxProfileYolo:
		default:
			return usageErr("--ax-profile accepts standard or yolo, got %q", value)
		}
		if !opts.AxConfigured {
			return usageErr("--ax-profile %s given but the ax integration is not configured on this machine; an execution profile is ax's and would be discarded", value)
		}
		inv.AxProfile = AxProfile(value)
	default:
		return usageErr("unknown flag %q before --", flag)
	}
	return nil
}
