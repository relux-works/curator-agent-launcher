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
       curator run werkstatt update <id> [--set-task <ID> | --clear-task] [--queue-depth N] [--json]
       curator run werkstatt recover <id> (--rebind-manager | --abandon-operation) [--json]
       curator run werkstatt list [--json]

A Werkstatt is a long-lived hosted session for one project role. up starts or
continues it through the selected session manager, down retires it (graceful
unless --force), recycle retires it and starts a new generation, status and
list report the record, liveness and manager status. update edits dispatch
fields; recover applies an explicit recovery override. Mutating status flags
are deprecated aliases; use update or recover instead.
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

Reports the Werkstatt record, liveness, manager status and a2a evidence.
A zero exit is not proof of fresh liveness: check the reported source.
Legacy --set-task, --clear-task and --queue-depth are deprecated aliases for
update; --abandon-operation and --rebind-manager are deprecated aliases for
recover. These aliases print one warning on stderr.
`,
	"update": `usage: curator run werkstatt update <id> [--set-task <ID> | --clear-task] [--queue-depth N] [--json]

Edits dispatch fields of a Werkstatt with a bound session, then reports its
status row. At least one field flag is required. --set-task and --clear-task
are mutually exclusive; --queue-depth accepts an integer from 0 to 2^53-1.
`,
	"recover": `usage: curator run werkstatt recover <id> (--rebind-manager | --abandon-operation) [--json]

Applies exactly one explicit recovery override, then reports the status row.
--rebind-manager accepts a re-registered owner after a matching session read.
--abandon-operation clears the pending journal and records an operator
override and tombstone. Neither override stops or cancels anything remote.
`,
	"list": `usage: curator run werkstatt list [--json]

Reports every Werkstatt in the status row shape.
`,
}

const sessionManagerHelp = `usage: curator-run session-manager list [--json]
       curator-run session-manager show <id> [--json]
       curator-run session-manager observe <manager-id> --session <SES-handle> --json
       curator-run session-manager register --manifest <path|-> [--select | --no-select] [--scope user|system]
       curator-run session-manager use <id> [--scope user|system]
       curator-run session-manager select <id> [--scope user|system] (deprecated)
       curator-run session-manager unregister <id> [--scope user|system]

Manages the machine's session-manager registry. observe reads one session
through describe and status; it never changes registry or launcher state.
The other commands never contact a manager. list/show report the layer read
(system, user or none).
--scope chooses one write layer with no fallback. Without it, the writable
system layer wins; otherwise user. A system registry shadows user mutations.
select is deprecated in 0.6.0; removal no earlier than 0.8.0 (two releases).
`

var sessionManagerVerbHelp = map[string]string{
	"list": `usage: curator-run session-manager list [--json]

Lists the layer read and registered managers, marking the selected one.
`,
	"show": `usage: curator-run session-manager show <id> [--json]

Shows one registered manager and the layer read.
`,
	"observe": `usage: curator-run session-manager observe <manager-id> --session <SES-handle> --json

Emits one closed JSON observation of a registered manager and session.
Both flags are required. Calls describe and one status, without local writes
or reconciliation. Unknown/unreadable records remain evidence, not down.
`,
	"register": `usage: curator-run session-manager register --manifest <path|-> [--select | --no-select] [--scope user|system]

Validates a manager manifest (a file, or - for stdin) and adds it to the
registry, or updates the entry with the same id. --select selects it and
--no-select leaves the selection unchanged. Without either flag: an update
keeps the selection, a manager registered into an empty registry is
selected, and otherwise the command asks once on a terminal; without a
terminal the selection stays unchanged.
--scope system refuses if unwritable; --scope user refuses under a system
registry. Without --scope, writable system wins, otherwise user (no shadowed writes).
`,
	"use": `usage: curator-run session-manager use <id> [--scope user|system]

Selects the registered manager <id> for hosted launches.
--scope chooses one layer without fallback; system registries shadow user writes.
`,
	"select": `usage: curator-run session-manager select <id> [--scope user|system]

Deprecated alias for use since 0.6.0; removal no earlier than 0.8.0
(two releases). Same effect and stdout, with one stderr warning.
`,
	"unregister": `usage: curator-run session-manager unregister <id> [--scope user|system]

Removes <id> from the registry. Removing the selected manager leaves no
selection; no other manager is selected in its place.
--scope chooses one layer without fallback; system registries shadow user writes.
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
	prefix := "curator run"
	if topic == "session-manager" {
		prefix = "curator-run"
	}
	parts := []string{prefix, topic}
	if verb != "" {
		parts = append(parts, verb)
	}
	return "; see: " + strings.Join(append(parts, "--help"), " ")
}
