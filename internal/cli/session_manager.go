package cli

import "github.com/relux-works/curator-agent-launcher/internal/sessionmanager"

// parseSessionManager admits the closed registry grammar. Each refusal names
// the rule and the smallest help command. It quotes only grammar tokens
// (operations and known flags), never an id, a value or an unknown token:
// those can carry a misplaced credential.
func parseSessionManager(args []string) (sessionmanager.Command, error) {
	var c sessionmanager.Command
	if len(args) == 0 {
		return c, usageErr("session-manager needs an operation: list, show, register, select or unregister%s", helpHint("session-manager", ""))
	}
	c.Operation = args[0]
	args = args[1:]
	fail := func(format string, a ...any) (sessionmanager.Command, error) {
		return sessionmanager.Command{}, usageErr("session-manager %s: "+format+"%s", append(append([]any{c.Operation}, a...), helpHint("session-manager", c.Operation))...)
	}
	switch c.Operation {
	case "list":
		if len(args) == 1 && args[0] == "--json" {
			c.JSON = true
		} else if len(args) != 0 {
			return fail("unexpected argument")
		}
	case "show", "select", "unregister":
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
				if flag == "--manifest" || flag == "--select" || flag == "--no-select" {
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
		return sessionmanager.Command{}, usageErr("unknown session-manager operation; expected list, show, register, select or unregister%s", helpHint("session-manager", ""))
	}
	return c, nil
}
