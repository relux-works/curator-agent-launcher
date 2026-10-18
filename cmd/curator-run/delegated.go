package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

func operationID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	id[6] = id[6]&15 | 64
	id[8] = id[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:]), nil
}
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// delegatedRequest projects resolved policy values, never the composed child
// environment or argv. The plugin resolves the same profile and composes it;
// native_tail contains only the module's selector-free elevation result.
func delegatedRequest(inv cli.Invocation, frag *fragment.Fragment, resolved defaults.Resolved, permission execution.PermissionDecision, cwd string, tail []string, intent agentic.ResumeIntent, at time.Time) (string, []byte, error) {
	id, err := operationID()
	if err != nil {
		return "", nil, err
	}
	labels := map[string]string{}
	if resolved.SelectorDecisionID != "" {
		labels["selector"] = resolved.SelectorDecisionID
	}
	request := map[string]any{
		"operation_id": id, "submitted_at": at.UTC().Format("2006-01-02T15:04:05Z"), "admission_key": nil,
		"runtime": frag.Environment, "profile": map[string]string{"name": frag.Profile.Name, "lock_sha256": frag.Profile.LockSHA256, "fragment_digest": frag.Digest},
		"credential_source": nil, "doorbell": nil, "detach": inv.Detach,
	}
	op := "launch"
	if intent.Kind != agentic.ResumeNew {
		op = "resume"
		selector := map[string]any{"kind": "latest"}
		switch intent.Kind {
		case agentic.ResumeHandle:
			selector = map[string]any{"kind": "handle", "handle": intent.Identity}
		case agentic.ResumeClaudeUUID, agentic.ResumeCodexThread:
			selector = map[string]any{"kind": "provider_id", "id": intent.Identity}
		case agentic.ResumeLatest:
		default:
			return "", nil, hostError("session_resume_invalid", nil)
		}
		request["selector"], request["scope"] = selector, map[string]any{"cwd": cwd, "labels": map[string]string{}}
	} else {
		sessionName := inv.Name
		if !inv.NameSet {
			sessionName = inv.EnvID + "-" + at.UTC().Format("20060102T150405Z")
		}
		var prompt any
		if inv.SystemPrompt != "" {
			prompt = map[string]string{"semantics": string(inv.SystemPrompt)}
		}
		goal := "none"
		if inv.BoardGoal {
			goal = "workspace"
		}
		if tail == nil {
			tail = []string{}
		}
		for k, v := range map[string]any{
			"cwd": cwd, "model": nullable(resolved.Model.Value), "effort": nullable(resolved.Effort.Value),
			"service_tier": nil, "weights_id": nil, "permissions": map[string]string{"mode": string(permission.Mode), "source": permission.Source},
			"system_prompt": prompt, "sandbox": map[string]any{"mode": "none", "writable_roots": []string{}},
			"harness_memory": false, "session_name": sessionName, "labels": labels, "child_env": map[string]string{}, "board_goal": goal, "native_tail": tail,
		} {
			request[k] = v
		}
	}
	data, err := json.Marshal(request)
	if err != nil {
		return "", nil, err
	}
	if err = client.ValidateRequest(op, data); err != nil {
		return "", nil, err
	}
	return op, data, nil
}

func runDelegated(ctx context.Context, inv cli.Invocation, frag *fragment.Fragment, resolved defaults.Resolved, permission execution.PermissionDecision, cwd string, tail []string, intent agentic.ResumeIntent, stdout, stderr io.Writer, deps launchDeps) int {
	now := time.Now
	if deps.now != nil {
		now = deps.now
	}
	op, data, err := delegatedRequest(inv, frag, resolved, permission, cwd, tail, intent, now())
	if err != nil {
		return emitHostFailure(stderr, err)
	}
	session, err := deps.hostedSelection.client.Start(ctx, op, data)
	if err != nil {
		return emitHostFailure(stderr, err)
	}
	if inv.Detach {
		return 0
	}
	data, _ = json.Marshal(map[string]any{"session": session.Session, "expected_generation": session.Generation})
	result, err := deps.hostedSelection.client.Terminal(ctx, "attach", data, terminalStreams(deps.hostedSelection.terminal, stdout, stderr, deps.stdin))
	if err != nil {
		return emitHostFailure(stderr, err)
	}
	if result.Refusal != nil {
		return emitHostFailure(stderr, result.Refusal)
	}
	return result.Exit
}
func terminalStreams(tty *os.File, stdout, stderr io.Writer, stdin io.Reader) client.TerminalIO {
	streams := client.TerminalIO{Terminal: tty}
	if stdin == nil {
		streams.Stdin = os.Stdin
	} else {
		streams.Stdin, _ = stdin.(*os.File)
	}
	streams.Stdout, _ = stdout.(*os.File)
	// Session output must not enter the launcher's diagnostic classifier.
	if observer, ok := stderr.(*selectorOutcomeWriter); ok {
		stderr = observer.Writer
	}
	streams.Stderr = stderr
	if streams.Stderr == nil {
		streams.Stderr = os.Stderr
	}
	return streams
}
