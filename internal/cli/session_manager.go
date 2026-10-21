package cli

import "github.com/relux-works/curator-agent-launcher/internal/sessionmanager"

// parseSessionManager admits the closed registry grammar. Each refusal names
// the offending token and the smallest help command.
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
			return fail("unexpected argument %q", args[0])
		}
	case "show", "select", "unregister":
		if len(args) == 0 {
			return fail("a manager id is required")
		}
		if !sessionmanager.ValidID(args[0]) {
			return fail("invalid manager id %q", args[0])
		}
		c.ID = args[0]
		args = args[1:]
		if c.Operation == "show" && len(args) == 1 && args[0] == "--json" {
			c.JSON = true
		} else if len(args) != 0 {
			return fail("unexpected argument %q", args[0])
		}
	case "register":
		seen := map[string]bool{}
		for i := 0; i < len(args); i++ {
			flag := args[i]
			if seen[flag] {
				return fail("%s given twice", flag)
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
				return fail("unknown argument %q", flag)
			}
		}
		if c.Manifest == "" {
			return fail("--manifest is required")
		}
		if c.Select && c.NoSelect {
			return fail("--select and --no-select are mutually exclusive")
		}
	default:
		return sessionmanager.Command{}, usageErr("unknown session-manager operation %q; expected list, show, register, select or unregister%s", c.Operation, helpHint("session-manager", ""))
	}
	return c, nil
}
