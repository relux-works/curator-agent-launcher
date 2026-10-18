package cli

import "github.com/relux-works/curator-agent-launcher/internal/sessionmanager"

func parseSessionManager(args []string) (sessionmanager.Command, error) {
	var c sessionmanager.Command
	fail := func() (sessionmanager.Command, error) {
		return sessionmanager.Command{}, usageErr("expected session-manager list|show|register|select|unregister with its documented arguments")
	}
	if len(args) == 0 {
		return fail()
	}
	c.Operation = args[0]
	args = args[1:]
	switch c.Operation {
	case "list":
		if len(args) == 1 && args[0] == "--json" {
			c.JSON = true
		} else if len(args) != 0 {
			return fail()
		}
	case "show", "select", "unregister":
		if len(args) == 0 || !sessionmanager.ValidID(args[0]) {
			return fail()
		}
		c.ID = args[0]
		args = args[1:]
		if c.Operation == "show" && len(args) == 1 && args[0] == "--json" {
			c.JSON = true
		} else if len(args) != 0 {
			return fail()
		}
	case "register":
		seen := map[string]bool{}
		for i := 0; i < len(args); i++ {
			flag := args[i]
			if seen[flag] {
				return fail()
			}
			seen[flag] = true
			switch flag {
			case "--manifest":
				i++
				if i == len(args) || args[i] == "" || (args[i] != "-" && len(args[i]) > 1 && args[i][:2] == "--") {
					return fail()
				}
				c.Manifest = args[i]
			case "--select":
				c.Select = true
			case "--no-select":
				c.NoSelect = true
			default:
				return fail()
			}
		}
		if c.Manifest == "" || c.Select && c.NoSelect {
			return fail()
		}
	default:
		return fail()
	}
	return c, nil
}
