package client

import (
	"encoding/json"
	"slices"
	"sort"
)

// RequiredForRequest derives semantic needs from the validated delegated wire
// request. Only launcher selection/activation policy comes from selection.
func RequiredForRequest(d Describe, operation string, request []byte, selection Needs) (Route, error) {
	if operation != "launch" && operation != "resume" {
		return Route{}, refusal("launch_plan_invalid", nil)
	}
	if err := ValidateRequest(operation, request); err != nil {
		return Route{}, err
	}
	var value struct {
		Runtime          string          `json:"runtime"`
		Detach           bool            `json:"detach"`
		BoardGoal        string          `json:"board_goal"`
		CredentialSource json.RawMessage `json:"credential_source"`
		Doorbell         json.RawMessage `json:"doorbell"`
	}
	_ = json.Unmarshal(request, &value)
	selection.Runtime, selection.Detached = value.Runtime, value.Detach
	selection.Resume = operation == "resume"
	selection.BoardGoal = value.BoardGoal == "workspace"
	selection.CredentialNode = string(value.CredentialSource) != "null"
	selection.Doorbell = string(value.Doorbell) != "null"
	return Required(d, selection)
}

type Needs struct {
	Runtime, HostMode                                                                        string
	Native, ExplicitHosted, Werkstatt, Detached, Resume, BoardGoal, CredentialNode, Doorbell bool
	AttachAfter                                                                              bool
	DelegatedTrial, SignedOff                                                                bool
}
type Route struct {
	Kind         string
	Notice       string
	Capabilities []string
}

// Required is a pure capability and activation decision. Describe is scoped to
// the selected manager; exact support is never inferred across runtimes. This
// function neither resolves a profile nor authorises a credential intake.
func Required(d Describe, n Needs) (Route, error) {
	if n.Native || (!n.ExplicitHosted && !n.Werkstatt && n.HostMode == "native") {
		return Route{Kind: "native"}, nil
	}
	operations, ok := d.Runtimes[n.Runtime]
	if !ok {
		return Route{}, refusal("session_host_provider_unsupported", map[string]string{"environment": n.Runtime})
	}
	route := "delegated"
	launchCap := "launch-delegated/1"
	launchOp := "launch"
	if !n.Detached && slices.Contains(operations, "launch-exact") {
		route = "exact"
		launchCap = "launch-exact/2"
		launchOp = "launch-exact"
	}
	if n.Detached && !slices.Contains(operations, "launch") && slices.Contains(operations, "launch-exact") {
		return Route{}, refusal("session_host_capability_missing", map[string]string{"capability": "launch-exact-detached"})
	}
	required := []string{launchCap}
	neededOps := map[string]string{launchCap: launchOp}
	need := func(cap, op string) {
		required = append(required, cap)
		if op != "" {
			neededOps[cap] = op
		}
	}
	if !n.Detached || n.AttachAfter {
		need("attach/1", "attach")
	}
	if n.Resume {
		need("resume/1", "resume")
	}
	if n.BoardGoal {
		need("board-goal/1", "")
	}
	if n.CredentialNode {
		need("credential-node/1", "")
	}
	if n.Doorbell {
		switch n.Runtime {
		case "claude_code":
			need("doorbell-claude/1", "")
		case "codex_cli":
			need("doorbell-codex/1", "")
		default:
			return Route{}, refusal("session_host_provider_unsupported", map[string]string{"environment": n.Runtime})
		}
	}
	if n.Werkstatt {
		for _, pair := range [][2]string{{"admission/1", "cancel"}, {"labels/1", ""}, {"list/1", "list"}, {"status/1", "status"}, {"stop/1", "stop"}} {
			need(pair[0], pair[1])
		}
	}
	for _, cap := range required {
		if !slices.Contains(d.Capabilities, cap) || (neededOps[cap] != "" && !slices.Contains(operations, neededOps[cap])) {
			return Route{}, refusal("session_host_capability_missing", map[string]string{"capability": cap})
		}
	}
	sort.Strings(required)
	if route == "delegated" && !n.SignedOff {
		switch {
		case n.ExplicitHosted || n.Werkstatt:
			if !n.DelegatedTrial {
				return Route{}, refusal("session_host_delegated_unauthorized", nil)
			}
		case n.HostMode == "hosted":
			return Route{}, refusal("session_host_missing", map[string]string{"reason": "no manager selected"})
		default:
			return Route{Kind: "native", Notice: "notice-unselected", Capabilities: required}, nil
		}
	}
	return Route{Kind: route, Capabilities: required}, nil
}
