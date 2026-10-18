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
	bad := func() (Command, error) {
		return Command{}, fmt.Errorf("expected werkstatt up|down|recycle|status|list with its documented arguments")
	}
	if len(args) == 0 {
		return bad()
	}
	c := Command{Action: args[0]}
	switch c.Action {
	case "up", "down", "recycle", "status", "list":
	default:
		return bad()
	}
	seen := map[string]bool{}
	for i := 1; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			if c.ID != "" || !ValidID(a) {
				return bad()
			}
			c.ID = a
			continue
		}
		if seen[a] {
			return bad()
		}
		seen[a] = true
		switch a {
		case "--attach":
			if c.Action != "up" {
				return bad()
			}
			c.Attach = true
		case "--new":
			if c.Action != "up" {
				return bad()
			}
			c.New = true
		case "--force":
			if c.Action != "down" {
				return bad()
			}
			c.Force = true
		case "--json":
			if c.Action != "status" && c.Action != "list" {
				return bad()
			}
			c.JSON = true
		case "--rebind-manager":
			if c.Action != "status" {
				return bad()
			}
			c.RebindManager = true
		case "--abandon-operation":
			if c.Action != "status" {
				return bad()
			}
			c.AbandonOperation = true
		case "--clear-task":
			if c.Action != "status" || c.SetTask {
				return bad()
			}
			c.SetTask = true
		case "--set-task", "--queue-depth", "--task-class", "--difficulty":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return bad()
			}
			i++
			v := args[i]
			switch a {
			case "--set-task":
				if c.Action != "status" || c.SetTask || v == "" {
					return bad()
				}
				c.SetTask = true
				c.Task = &v
			case "--queue-depth":
				n, e := strconv.ParseInt(v, 10, 64)
				if c.Action != "status" || e != nil || n < 0 || n > 9007199254740991 {
					return bad()
				}
				c.QueueDepth = &n
			case "--task-class":
				if c.Action != "up" || !dispatchValue(v) {
					return bad()
				}
				c.TaskClass = v
			case "--difficulty":
				if c.Action != "up" || !dispatchValue(v) {
					return bad()
				}
				c.Difficulty = v
			}
		default:
			return bad()
		}
	}
	if c.Action == "list" && c.ID != "" || c.Action != "list" && c.Action != "status" && c.ID == "" || c.Difficulty != "" && c.TaskClass == "" || c.RebindManager && c.AbandonOperation {
		return bad()
	}
	if c.ID == "" && (c.SetTask || c.QueueDepth != nil || c.RebindManager || c.AbandonOperation) {
		return bad()
	}
	return c, nil
}
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
