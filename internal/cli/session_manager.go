package cli

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
)

var observationHandle = regexp.MustCompile(`^SES-[A-Za-z0-9]{1,48}$`)

// The frozen-binding values follow the published observation-v2 and
// session-record shapes: a UTC RFC 3339 registration time, and a positive
// generation within the safe-integer range.
var observationRegisteredAt = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-]00:00)$`)
var observationGeneration = regexp.MustCompile(`^[1-9][0-9]{0,15}$`)

const observationMaxGeneration = 9007199254740991

// parseSessionManager admits the closed registry grammar. Each refusal names
// the rule and the smallest help command. It quotes only grammar tokens
// (operations and known flags), never an id, a value or an unknown token:
// those can carry a misplaced credential.
func parseSessionManager(args []string) (sessionmanager.Command, error) {
	var c sessionmanager.Command
	if len(args) == 0 {
		return c, usageErr("session-manager needs an operation: list, show, observe, register, use or unregister%s", helpHint("session-manager", ""))
	}
	c.Operation = args[0]
	args = args[1:]
	fail := func(format string, a ...any) (sessionmanager.Command, error) {
		return sessionmanager.Command{}, usageErr("session-manager %s: "+format+"%s", append(append([]any{c.Operation}, a...), helpHint("session-manager", c.Operation))...)
	}
	// Selection/removal accept scope on either side of the positional id.
	// Registration consumes it below alongside its other flags and values.
	if c.Operation == "use" || c.Operation == "select" || c.Operation == "unregister" {
		filtered := make([]string, 0, len(args))
		for i := 0; i < len(args); i++ {
			if args[i] != "--scope" {
				filtered = append(filtered, args[i])
				continue
			}
			if c.Scope != "" {
				return fail("--scope given twice")
			}
			i++
			if i == len(args) || (args[i] != "user" && args[i] != "system") {
				return fail("--scope needs user or system")
			}
			c.Scope = args[i]
		}
		args = filtered
	}

	switch c.Operation {
	case "observe":
		if len(args) == 0 || !sessionmanager.ValidID(args[0]) {
			return fail("a valid manager id is required")
		}
		c.ID = args[0]
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--json":
				if c.JSON {
					return fail("--json given twice")
				}
				c.JSON = true
			case "--session":
				if c.Session != "" {
					return fail("--session given twice")
				}
				i++
				if i == len(args) || !observationHandle.MatchString(args[i]) {
					return fail("--session needs a SES handle")
				}
				c.Session = args[i]
			case "--schema":
				if c.Schema != "" {
					return fail("--schema given twice")
				}
				i++
				if i == len(args) || (args[i] != "v1" && args[i] != "v2") {
					return fail("--schema needs v1 or v2")
				}
				c.Schema = args[i]
			case "--expect-registered-at":
				if c.ExpectRegisteredAt != "" {
					return fail("--expect-registered-at given twice")
				}
				i++
				if i == len(args) || !observationRegisteredAt.MatchString(args[i]) {
					return fail("--expect-registered-at needs a UTC RFC 3339 timestamp")
				}
				c.ExpectRegisteredAt = args[i]
			case "--expect-generation":
				if c.ExpectGeneration != 0 {
					return fail("--expect-generation given twice")
				}
				i++
				if i == len(args) || !observationGeneration.MatchString(args[i]) {
					return fail("--expect-generation needs a positive integer")
				}
				n, err := strconv.ParseInt(args[i], 10, 64)
				if err != nil || n > observationMaxGeneration {
					return fail("--expect-generation needs a positive integer")
				}
				c.ExpectGeneration = n
			case "--expect-executable":
				if c.ExpectExecutable != "" {
					return fail("--expect-executable given twice")
				}
				i++
				if i == len(args) || !strings.HasPrefix(args[i], "/") || strings.ContainsRune(args[i], 0) {
					return fail("--expect-executable needs an absolute path")
				}
				c.ExpectExecutable = args[i]
			default:
				return fail("unexpected argument")
			}
		}
		if c.Session == "" || !c.JSON {
			return fail("--session and --json are required")
		}
		if c.Schema == "" {
			c.Schema = "v1"
		}
		frozen := c.ExpectRegisteredAt != "" || c.ExpectGeneration != 0 || c.ExpectExecutable != ""
		if c.Schema != "v2" && frozen {
			return fail("--expect-registered-at, --expect-generation and --expect-executable need --schema v2")
		}
		if c.Schema == "v2" && (c.ExpectRegisteredAt == "" || c.ExpectGeneration == 0) {
			return fail("--schema v2 requires --expect-registered-at and --expect-generation")
		}
	case "list":
		if len(args) == 1 && args[0] == "--json" {
			c.JSON = true
		} else if len(args) != 0 {
			return fail("unexpected argument")
		}
	case "show", "use", "select", "unregister":
		if len(args) == 0 {
			return fail("a manager id is required")
		}
		if !sessionmanager.ValidID(args[0]) {
			return fail("invalid manager id")
		}
		c.ID = args[0]
		args = args[1:]
		if c.Operation == "show" && len(args) == 1 && args[0] == "--json" {
			c.JSON = true
		} else if len(args) != 0 {
			return fail("unexpected argument")
		}
	case "register":
		seen := map[string]bool{}
		for i := 0; i < len(args); i++ {
			flag := args[i]
			if seen[flag] {
				if flag == "--manifest" || flag == "--select" || flag == "--no-select" || flag == "--scope" {
					return fail("%s given twice", flag)
				}
				return fail("unknown argument")
			}
			seen[flag] = true
			switch flag {
			case "--manifest":
				i++
				if i == len(args) || args[i] == "" || (args[i] != "-" && len(args[i]) > 1 && args[i][:2] == "--") {
					return fail("--manifest needs a path or -")
				}
				c.Manifest = args[i]
			case "--scope":
				i++
				if i == len(args) || (args[i] != "user" && args[i] != "system") {
					return fail("--scope needs user or system")
				}
				c.Scope = args[i]
			case "--select":
				c.Select = true
			case "--no-select":
				c.NoSelect = true
			default:
				return fail("unknown argument")
			}
		}
		if c.Manifest == "" {
			return fail("--manifest is required")
		}
		if c.Select && c.NoSelect {
			return fail("--select and --no-select are mutually exclusive")
		}
	default:
		return sessionmanager.Command{}, usageErr("unknown session-manager operation; expected list, show, observe, register, use or unregister%s", helpHint("session-manager", ""))
	}
	return c, nil
}
