package cli

import "strings"

// Static help for the administrative topics (FRONT-DOOR §2). These grammars
// have no native tail, so -h or --help anywhere in them asks for help. The
// text is constant: printing it reads no configuration, registry, manager or
// station state, so it works even when that state is broken.

const werkstattHelp = `usage: curator run werkstatt up <id> [--attach] [--new] [--task-class C [--difficulty D]]
       curator run werkstatt down <id> [--force]
       curator run werkstatt recycle <id>
       curator run werkstatt status [<id>] [--json]
                   [--set-task <ID> | --clear-task] [--queue-depth N]
                   [--abandon-operation | --rebind-manager]
       curator run werkstatt list [--json]

A Werkstatt is a long-lived hosted session for one project role. up starts or
continues it through the selected session manager, down retires it (graceful
unless --force), recycle retires it and starts a new generation, status and
list report the record, liveness and manager status.
`

var werkstattVerbHelp = map[string]string{
	"up": `usage: curator run werkstatt up <id> [--attach] [--new] [--task-class C [--difficulty D]]

Starts the Werkstatt <id> through the selected session manager. In continue
mode it resumes the bound thread; --new forces a new thread. --attach attaches
after the start. --task-class and --difficulty describe the dispatched task
for model selection. up is always hosted and refuses without a selected
manager.
`,
	"down": `usage: curator run werkstatt down <id> [--force]

Retires the Werkstatt <id>. Without --force the stop is graceful and refuses
while the session is busy or its activity is unknown. --force asks the manager
to end the provider at once.
`,
	"recycle": `usage: curator run werkstatt recycle <id>

Graceful down, then up with a new generation, under one lock. A provider that
is still draining refuses and nothing new starts.
`,
	"status": `usage: curator run werkstatt status [<id>] [--json]
                   [--set-task <ID> | --clear-task] [--queue-depth N]
                   [--abandon-operation | --rebind-manager]

Reports the Werkstatt record, liveness, manager status and a2a evidence. The
dispatch flags need an <id> with a live session. --abandon-operation and
--rebind-manager are explicit recovery overrides; they stop nothing remote.
A zero exit is not proof of fresh liveness: check the reported source.
`,
	"list": `usage: curator run werkstatt list [--json]

Reports every Werkstatt in the status row shape.
`,
}

const sessionManagerHelp = `usage: curator run session-manager list [--json]
       curator run session-manager show <id> [--json]
       curator run session-manager register --manifest <path|-> [--select | --no-select]
       curator run session-manager select <id>
       curator run session-manager unregister <id>

Manages the machine's session-manager registry. These commands never launch
or contact a manager.
`

var sessionManagerVerbHelp = map[string]string{
	"list": `usage: curator run session-manager list [--json]

Lists the registered managers and marks the selected one.
`,
	"show": `usage: curator run session-manager show <id> [--json]

Shows one registered manager.
`,
	"register": `usage: curator run session-manager register --manifest <path|-> [--select | --no-select]

Validates a manager manifest (a file, or - for stdin) and adds it to the
registry, or updates the entry with the same id. --select selects it and
--no-select leaves the selection unchanged. Without either flag: an update
keeps the selection, a manager registered into an empty registry is
selected, and otherwise the command asks once on a terminal; without a
terminal the selection stays unchanged.
`,
	"select": `usage: curator run session-manager select <id>

Selects the registered manager <id> for hosted launches.
`,
	"unregister": `usage: curator run session-manager unregister <id>

Removes <id> from the registry. Removing the selected manager leaves no
selection; no other manager is selected in its place.
`,
}

const migrateHelp = `usage: curator run migrate [--dry-run]

Moves legacy launcher configuration to the current layout. --dry-run prints
the plan and writes nothing.
`

// TopicHelp returns the static help for a werkstatt, session-manager or migrate
// invocation that contains -h or --help, and whether one was asked for. A known
// verb in second position selects that verb's help.
func TopicHelp(args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	asked := false
	for _, a := range args[1:] {
		if a == "-h" || a == "--help" {
			asked = true
			break
		}
	}
	if !asked {
		return "", false
	}
	verb := ""
	if len(args) > 1 {
		verb = args[1]
	}
	switch args[0] {
	case "werkstatt":
		if text, ok := werkstattVerbHelp[verb]; ok {
			return text, true
		}
		return werkstattHelp, true
	case "session-manager":
		if text, ok := sessionManagerVerbHelp[verb]; ok {
			return text, true
		}
		return sessionManagerHelp, true
	case "migrate":
		return migrateHelp, true
	}
	return "", false
}

// helpHint names the smallest help command for a topic and verb.
func helpHint(topic, verb string) string {
	parts := []string{"curator run", topic}
	if verb != "" {
		parts = append(parts, verb)
	}
	return "; see: " + strings.Join(append(parts, "--help"), " ")
}
