package network

import (
	"strconv"
	"strings"

	"github.com/relux-works/curator-network-profiles/pkg/refusal"
)

// argShape is how Claude Code's option parser consumes an option's value.
type argShape int

const (
	argNone     argShape = iota // a switch: no value, and no "=value" form
	argRequired                 // takes the next argument, whatever it is, or "=value"
	argOptional                 // takes the next argument unless it looks like an option
	argVariadic                 // takes values until an argument looks like an option
)

// gatewayNativeOptions is the allowlist of Claude Code options a native tail
// may carry under a filtering gateway, with their value shapes as the
// 2.1.314 bundle declares them. Each is local to the session it starts: none
// selects settings sources, a working directory, a stored or remote session,
// plugins, a transport or an endpoint. Every other option refuses, so an
// option the bundle adds later is refused until it is reviewed here.
var gatewayNativeOptions = map[string]argShape{
	"--print":                                  argNone,
	"--verbose":                                argNone,
	"--debug":                                  argOptional,
	"--debug-file":                             argRequired,
	"--output-format":                          argRequired,
	"--input-format":                           argRequired,
	"--json-schema":                            argRequired,
	"--include-hook-events":                    argNone,
	"--include-partial-messages":               argNone,
	"--replay-user-messages":                   argNone,
	"--dangerously-skip-permissions":           argNone,
	"--allow-dangerously-skip-permissions":     argNone,
	"--permission-mode":                        argRequired,
	"--permission-prompt-tool":                 argRequired,
	"--thinking":                               argRequired,
	"--thinking-display":                       argRequired,
	"--max-thinking-tokens":                    argRequired,
	"--max-turns":                              argRequired,
	"--max-budget-usd":                         argRequired,
	"--allowedTools":                           argVariadic,
	"--allowed-tools":                          argVariadic,
	"--disallowedTools":                        argVariadic,
	"--disallowed-tools":                       argVariadic,
	"--tools":                                  argVariadic,
	"--mcp-config":                             argVariadic,
	"--strict-mcp-config":                      argNone,
	"--system-prompt":                          argRequired,
	"--system-prompt-file":                     argRequired,
	"--append-system-prompt":                   argRequired,
	"--append-system-prompt-file":              argRequired,
	"--exclude-dynamic-system-prompt-sections": argNone,
	"--model":                                  argRequired,
	"--fallback-model":                         argRequired,
	"--effort":                                 argRequired,
	"--agent":                                  argRequired,
	"--betas":                                  argVariadic,
	"--session-id":                             argRequired,
	"--name":                                   argRequired,
	"--no-session-persistence":                 argNone,
	"--disable-slash-commands":                 argNone,
}

// gatewayNativeShortOptions are the admitted single-letter forms, each only
// on its own: a cluster such as -pc refuses.
var gatewayNativeShortOptions = map[string]argShape{
	"-p": argNone,
	"-d": argOptional,
	"-n": argRequired,
}

// claudeReservedArgs are the words the 2.1.314 runtime dispatches on outside
// its option parser, or before it runs: every command name and alias it
// registers (.command(), the background-session commands, the alias tables
// and the parser's implicit help), the words its entry path matches anywhere
// in argv (remote-control, rc, mcp serve, plugin eval, agents --json, the
// background verbs, import), the first-position modes (remote, sync, bridge,
// daemon, self-hosted-runner, update, forward-home-settings and the dashed
// helper modes, --gh-standin and the version shortcuts), and the raw tokens
// it or a later mode getter searches for anywhere in argv (--handle-uri,
// --session-tunnel, --agent-teams, --hard-fail, --bg, --background, --tmux,
// --worktree, -w, and -c, --continue, -r, --resume, --from-pr). Because
// those searches see option values and arguments after "--" too, a native
// argument equal to any of these refuses wherever it stands; a prompt that is
// exactly such a word refuses as well.
var claudeReservedArgs = map[string]bool{}

// claudeReservedBeforeTerminator are the raw tokens the runtime looks for
// only before the first standalone "--" (its mode getters for safe mode and
// bare mode): they refuse anywhere before a "--" in the native tail, values
// included, and are ordinary operands after it.
var claudeReservedBeforeTerminator = map[string]bool{"--safe-mode": true, "--bare": true}

// claudeReservedPrefixes are reserved tokens in their "=value" form.
var claudeReservedPrefixes = []string{"--handle-uri=", "--worktree=", "--tmux=", "--runner-declares=", "--session-tunnel=", "--resume=", "-r=", "--from-pr="}

func init() {
	for _, n := range strings.Fields(`help attach logs stop kill respawn rm
		edit-permission-rules edit-memory-settings edit-skill-overrides edit-sandbox-settings design-login
		edit-chrome-settings drop-worktree-registrations edit-hook project purge gateway auth login status
		logout setup-token agents ultrareview auto-mode defaults config reset critique remote-control rc
		doctor sandbox install uninstall update upgrade import import-conversations plugin plugins init
		validate tag list eval test details marketplace add remove prune enable disable configure xaa setup
		show clear mcp serve get add-json add-from-claude-desktop reset-project-choices
		i autoremove remote sync bridge daemon self-hosted-runner forward-home-settings
		--handle-uri --session-tunnel --runner-declares --eval-mock-server --claude-in-chrome-mcp
		--chrome-native-host --computer-use-mcp --daemon-worker --bg-pty-host --bg-spare --preload
		--update --upgrade --bg --background --tmux --worktree -w
		--agent-teams --hard-fail -c --continue -r --resume --from-pr --gh-standin --version -v -V`) {
		claudeReservedArgs[n] = true
	}
}

// reservedArg reports whether a native argument is a word the runtime
// dispatches on before or outside its option parser.
func reservedArg(arg string) bool {
	if claudeReservedArgs[arg] {
		return true
	}
	for _, p := range claudeReservedPrefixes {
		if strings.HasPrefix(arg, p) {
			return true
		}
	}
	return false
}

// looksLikeOption mirrors the option parser: an argument of more than one
// character that starts with a dash.
func looksLikeOption(arg string) bool { return len(arg) > 1 && arg[0] == '-' }

// nativeArgsConflict refuses a native tail that is not made only of
// admitted options and operands. The refusal names the argument's position,
// never its text.
func nativeArgsConflict(name string, args []string) error {
	refuse := func(i int, detail string) error {
		return refusal.New(refusal.CodeConfigurationConflict, name, "native argument "+strconv.Itoa(i+1)+" "+detail+", which the filtering gateway refuses")
	}
	// The runtime's entry path and mode getters search the raw argv outside
	// its option parser, so option values count, and so do arguments after
	// "--" except for the tokens it looks for only before the first one.
	terminated := false
	for i, arg := range args {
		if arg == "--" && !terminated {
			terminated = true
			continue
		}
		if reservedArg(arg) || (!terminated && claudeReservedBeforeTerminator[arg]) {
			return refuse(i, "is a word Claude Code dispatches on outside its option parser")
		}
	}
	// Words that do not look like options were all checked above, so only
	// options remain to admit. A value matters here only when it looks like
	// an option and the parser still takes it as a value: the next argument
	// after an option with a required value or a list. An optional value, and
	// the further items of a list, are taken only when they do not look like
	// options, so they never hide one.
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return nil
		}
		if !looksLikeOption(arg) {
			continue
		}
		flag, _, inline := arg, "", false
		shape, ok := gatewayNativeShortOptions[arg]
		if strings.HasPrefix(arg, "--") {
			flag, _, inline = strings.Cut(arg, "=")
			shape, ok = gatewayNativeOptions[flag]
		}
		if !ok {
			return refuse(i, "is not an admitted option")
		}
		switch shape {
		case argNone:
			if inline {
				return refuse(i, "gives a value to a switch")
			}
		case argRequired, argVariadic:
			if !inline {
				if i+1 >= len(args) {
					return refuse(i, "has no value")
				}
				i++
			}
		}
	}
	return nil
}
