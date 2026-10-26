package werkstattcommand

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)

func ValidID(id string) bool { return idPattern.MatchString(id) }

type Command struct {
	Action, ID                                                string
	Attach, New, Force, JSON, RebindManager, AbandonOperation bool
	TaskClass, Difficulty                                     string
	SetTask                                                   bool
	Task                                                      *string
	QueueDepth                                                *int64
}

// ParseCommand admits only the operation-specific public grammar. It performs
// no reads or effects; launch flags cannot accidentally turn into station input.
func ParseCommand(args []string) (Command, error) {
	if len(args) == 0 {
		return Command{}, fmt.Errorf("werkstatt needs an action: up, down, recycle, status, update, recover or list; see: curator run werkstatt --help")
	}
	c := Command{Action: args[0]}
	switch c.Action {
	case "up", "down", "recycle", "status", "update", "recover", "list":
	default:
		return Command{}, fmt.Errorf("unknown werkstatt action; expected up, down, recycle, status, update, recover or list; see: curator run werkstatt --help")
	}
	// Each refusal names the rule and the smallest help command. It quotes only
	// tokens of the closed grammar (actions and known flags), never an operand,
	// a value or an unknown token: those can carry a misplaced credential.
	bad := func(format string, a ...any) (Command, error) {
		return Command{}, fmt.Errorf("werkstatt %s: %s; see: curator run werkstatt %s --help", c.Action, fmt.Sprintf(format, a...), c.Action)
	}
	seen := map[string]bool{}
	for i := 1; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			if c.ID != "" {
				return bad("unexpected second operand after the id")
			}
			if !ValidID(a) {
				return bad("invalid id (expected [a-z0-9][a-z0-9-]{0,47})")
			}
			c.ID = a
			continue
		}
		if seen[a] {
			if knownFlag[a] {
				return bad("%s given twice", a)
			}
			return bad("unknown flag")
		}
		seen[a] = true
		switch a {
		case "--attach":
			if c.Action != "up" {
				return bad("%s is valid only with up", a)
			}
			c.Attach = true
		case "--new":
			if c.Action != "up" {
				return bad("%s is valid only with up", a)
			}
			c.New = true
		case "--force":
			if c.Action != "down" {
				return bad("%s is valid only with down", a)
			}
			c.Force = true
		case "--json":
			if c.Action != "status" && c.Action != "list" && c.Action != "update" && c.Action != "recover" {
				return bad("%s is valid only with status, update, recover or list", a)
			}
			c.JSON = true
		case "--rebind-manager":
			if c.Action != "status" && c.Action != "recover" {
				return bad("%s is valid only with recover (status is a deprecated alias)", a)
			}
			c.RebindManager = true
		case "--abandon-operation":
			if c.Action != "status" && c.Action != "recover" {
				return bad("%s is valid only with recover (status is a deprecated alias)", a)
			}
			c.AbandonOperation = true
		case "--clear-task":
			if c.Action != "status" && c.Action != "update" {
				return bad("%s is valid only with update (status is a deprecated alias)", a)
			}
			if c.SetTask {
				return bad("--clear-task and --set-task are mutually exclusive")
			}
			c.SetTask = true
		case "--set-task", "--queue-depth", "--task-class", "--difficulty":
			if (a == "--set-task" || a == "--queue-depth") && c.Action != "status" && c.Action != "update" {
				return bad("%s is valid only with update (status is a deprecated alias)", a)
			}
			if (a == "--task-class" || a == "--difficulty") && c.Action != "up" {
				return bad("%s is valid only with up", a)
			}
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return bad("%s needs a value", a)
			}
			i++
			v := args[i]
			switch a {
			case "--set-task":
				if c.SetTask {
					return bad("--clear-task and --set-task are mutually exclusive")
				}
				if v == "" {
					return bad("%s needs a non-empty value", a)
				}
				c.SetTask = true
				c.Task = &v
			case "--queue-depth":
				n, e := strconv.ParseInt(v, 10, 64)
				if e != nil || n < 0 || n > 9007199254740991 {
					return bad("%s needs an integer from 0 to 2^53-1", a)
				}
				c.QueueDepth = &n
			case "--task-class":
				if !dispatchValue(v) {
					return bad("%s needs a dispatch value of [A-Za-z0-9._:/@+-]{1,128}", a)
				}
				c.TaskClass = v
			case "--difficulty":
				if !dispatchValue(v) {
					return bad("%s needs a dispatch value of [A-Za-z0-9._:/@+-]{1,128}", a)
				}
				c.Difficulty = v
			}
		default:
			return bad("unknown flag")
		}
	}
	switch {
	case c.Action == "list" && c.ID != "":
		return bad("list takes no id")
	case c.Action != "list" && c.Action != "status" && c.ID == "":
		return bad("an id is required")
	case c.Difficulty != "" && c.TaskClass == "":
		return bad("--difficulty needs --task-class")
	case c.RebindManager && c.AbandonOperation:
		return bad("--rebind-manager and --abandon-operation are mutually exclusive")
	case c.Action == "update" && !c.SetTask && c.QueueDepth == nil:
		return bad("at least one of --set-task, --clear-task or --queue-depth is required")
	case c.Action == "recover" && !c.RebindManager && !c.AbandonOperation:
		return bad("exactly one of --rebind-manager or --abandon-operation is required")
	case c.ID == "" && (c.SetTask || c.QueueDepth != nil || c.RebindManager || c.AbandonOperation):
		return bad("dispatch and recovery flags need an id")
	}
	return c, nil
}

// knownFlag is the closed flag set; only these names may appear in a refusal.
var knownFlag = map[string]bool{"--attach": true, "--new": true, "--force": true, "--json": true, "--rebind-manager": true, "--abandon-operation": true, "--clear-task": true, "--set-task": true, "--queue-depth": true, "--task-class": true, "--difficulty": true}

func dispatchValue(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._:/@+-", c)) {
			return false
		}
	}
	return true
}
