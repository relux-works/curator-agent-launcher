package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

// The projection preserves explicit policy and profile identity, transports no
// ambient environment, and keeps launch-only values out of a resume request.
func TestDelegatedProjection(t *testing.T) {
	frag := &fragment.Fragment{Environment: "claude_code", Profile: fragment.Profile{Name: "sample", LockSHA256: strings.Repeat("1", 64)}, Digest: fragment.DigestPrefix + strings.Repeat("2", 64)}
	inv := cli.Invocation{EnvID: "claude_code", BoardGoal: true, SystemPrompt: cli.SystemPromptAppend, Name: "example", NameSet: true}
	resolved := defaults.Resolved{SelectorDecisionID: "decision/@+1", Model: defaults.ResolvedMember{Member: defaults.Member{Value: "claude-opus-5"}}, Effort: defaults.ResolvedMember{Member: defaults.Member{Value: "medium"}}}
	permission := execution.PermissionDecision{Mode: agentic.PermissionModeYolo, Source: "flag"}
	at := time.Date(2026, 1, 2, 3, 4, 5, 123456789, time.FixedZone("offset", 3600))
	op, data, err := delegatedRequest(inv, frag, resolved, permission, "/workspace", []string{"hello"}, agentic.ResumeIntent{Kind: agentic.ResumeNew}, at)
	if err != nil || op != "launch" {
		t.Fatal(op, err)
	}
	var r map[string]any
	json.Unmarshal(data, &r)
	expected := map[string]any{"profile": map[string]any{"name": "sample", "lock_sha256": frag.Profile.LockSHA256, "fragment_digest": frag.Digest}, "board_goal": "workspace", "system_prompt": map[string]any{"semantics": "append"}, "labels": map[string]any{"selector": "decision/@+1"}, "native_tail": []any{"hello"}, "submitted_at": "2026-01-02T02:04:05Z", "permissions": map[string]any{"mode": "yolo", "source": "flag"}, "credential_source": nil, "doorbell": nil, "child_env": map[string]any{}}
	for k, want := range expected {
		if !reflect.DeepEqual(r[k], want) {
			t.Errorf("projection %s=%#v want=%#v", k, r[k], want)
		}
	}
	d := client.Describe{Capabilities: []string{"launch-delegated/1", "attach/1", "board-goal/1"}, Runtimes: map[string][]string{"claude_code": {"launch", "attach"}}}
	route, err := client.RequiredForRequest(d, op, data, client.Needs{ExplicitHosted: true, DelegatedTrial: true})
	if err != nil || route.Kind != "delegated" {
		t.Fatal(route, err)
	}
	d.Capabilities = []string{"launch-delegated/1", "attach/1"}
	if _, err := client.RequiredForRequest(d, op, data, client.Needs{ExplicitHosted: true, DelegatedTrial: true}); !client.IsCode(err, "session_host_capability_missing") {
		t.Fatal("board goal capability not required", err)
	}
	handle := "SES-example"
	op, data, err = delegatedRequest(inv, frag, resolved, permission, "/workspace", []string{"ignored"}, agentic.ResumeIntent{Kind: agentic.ResumeHandle, Identity: &handle}, at)
	if err != nil || op != "resume" {
		t.Fatal(op, err)
	}
	// Decode into a fresh object so absent launch keys cannot remain in the map.
	r = nil
	json.Unmarshal(data, &r)
	if len(r) != 10 || !reflect.DeepEqual(r["selector"], map[string]any{"kind": "handle", "handle": handle}) || !reflect.DeepEqual(r["scope"], map[string]any{"cwd": "/workspace", "labels": map[string]any{}}) {
		t.Fatalf("resume projection: %s", data)
	}
	for _, k := range []string{"model", "effort", "permissions", "system_prompt", "board_goal", "native_tail", "child_env"} {
		if _, ok := r[k]; ok {
			t.Errorf("resume replaces stored %s", k)
		}
	}
}

// A native resume selector cannot be smuggled into a delegated fresh launch;
// malformed profile bindings and unrepresentable selector labels fail closed.
func TestDelegatedProjectionRejects(t *testing.T) {
	frag := &fragment.Fragment{Environment: "claude_code", Profile: fragment.Profile{Name: "sample", LockSHA256: strings.Repeat("1", 64)}, Digest: fragment.DigestPrefix + strings.Repeat("2", 64)}
	inv := cli.Invocation{EnvID: "claude_code"}
	permission := execution.PermissionDecision{Mode: agentic.PermissionModeNative, Source: "flag"}
	for _, tc := range []struct {
		name             string
		tail             []string
		decision, digest string
	}{
		{"valid", []string{"hello"}, "decision-1", frag.Digest},
		{"tail-resume", []string{"--resume", "01234567-89ab-4cde-8123-456789abcdef"}, "", frag.Digest},
		{"label", nil, "decision;1", frag.Digest},
		{"digest", nil, "", strings.Repeat("2", 64)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copyFrag := *frag
			copyFrag.Digest = tc.digest
			_, _, err := delegatedRequest(inv, &copyFrag, defaults.Resolved{SelectorDecisionID: tc.decision}, permission, "/workspace", tc.tail, agentic.ResumeIntent{Kind: agentic.ResumeNew}, time.Now())
			if tc.name == "valid" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !client.IsCode(err, "launch_plan_invalid") {
				t.Fatal("invalid projection admitted", err)
			}
		})
	}
}
