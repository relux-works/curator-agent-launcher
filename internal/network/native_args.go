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

// claudeCommandNames are every command name and alias the 2.1.314 bundle
// registers, top-level or nested, plus the parser's implicit help. The
// parser dispatches on the first operand, so a first operand equal to one of
// these runs that command instead of a session; under a filtering gateway it
// refuses. Nested names are included because the bundle does not tell its
// registrations apart reliably; a prompt that is exactly such a word refuses
// too.
var claudeCommandNames = map[string]bool{}

func init() {
	for _, n := range strings.Fields(`help attach logs stop kill respawn rm
		edit-permission-rules edit-memory-settings edit-skill-overrides edit-sandbox-settings design-login
		edit-chrome-settings drop-worktree-registrations edit-hook project purge gateway auth login status
		logout setup-token agents ultrareview auto-mode defaults config reset critique remote-control rc
		doctor sandbox install uninstall update upgrade import import-conversations plugin plugins init
		validate tag list eval test details marketplace add remove prune enable disable configure xaa setup
		show clear mcp serve get add-json add-from-claude-desktop reset-project-choices`) {
		claudeCommandNames[n] = true
	}
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
	firstOperand := true
	operand := func(i int, arg string) error {
		if firstOperand && claudeCommandNames[arg] {
			return refuse(i, "names a Claude Code command")
		}
		firstOperand = false
		return nil
	}
	variadic := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			for j := i + 1; j < len(args); j++ {
				if err := operand(j, args[j]); err != nil {
					return err
				}
			}
			return nil
		}
		if variadic && !looksLikeOption(arg) {
			continue
		}
		variadic = false
		if !looksLikeOption(arg) {
			if err := operand(i, arg); err != nil {
				return err
			}
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
				// Only the spaced form keeps taking values: the parser
				// ends a "--option=value" at its own argument.
				variadic = shape == argVariadic
			}
		case argOptional:
			if !inline && i+1 < len(args) && !looksLikeOption(args[i+1]) {
				i++
			}
		}
	}
	return nil
}
