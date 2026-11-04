package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	managerschema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

type noNotificationSnapshot struct {
	Operation string
	Wire      []byte
	Canonical []byte
	Digest    string
}

// These bytes were captured from main before applying notification selection.
// Only the unpredictable operation UUID is replaced, in place, before comparison.
// No field is deleted and the request is never decoded and re-marshaled.
func noNotificationSnapshotOf(t *testing.T, op string, data []byte) noNotificationSnapshot {
	t.Helper()
	var envelope struct {
		ID string `json:"operation_id"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(envelope.ID) {
		t.Fatal("invalid operation UUID")
	}
	field := []byte(`"operation_id":"` + envelope.ID + `"`)
	if bytes.Count(data, field) != 1 {
		t.Fatal("operation UUID is not unique")
	}
	data = bytes.Replace(data, field, []byte(`"operation_id":"00000000-0000-4000-8000-000000000000"`), 1)
	v, err := fragment.ParseJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	canonical := fragment.Canonical(v)
	wireCanonical, err := managerschema.CanonicalJSON(data)
	if err != nil || !bytes.Equal(canonical, wireCanonical) {
		t.Fatal("protocol canonical bytes differ from fragment canonical bytes", err)
	}
	return noNotificationSnapshot{op, data, canonical, fragment.Digest(v)}
}

func TestNoNotificationRequestMainGolden(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "no-notification-main.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden map[string]noNotificationSnapshot
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	frag := &fragment.Fragment{Environment: "claude_code", Profile: fragment.Profile{Name: "sample", LockSHA256: strings.Repeat("1", 64)}, Digest: fragment.DigestPrefix + strings.Repeat("2", 64)}
	at := time.Date(2026, 1, 2, 3, 4, 5, 123456789, time.FixedZone("offset", 3600))
	id := "01234567-89ab-4cde-8123-456789abcdef"
	handle := "SES-example"
	for _, tc := range []struct {
		name     string
		kind     agentic.ResumeKind
		identity *string
	}{
		{"launch", agentic.ResumeNew, nil}, {"latest", agentic.ResumeLatest, nil}, {"handle", agentic.ResumeHandle, &handle}, {"claude", agentic.ResumeClaudeUUID, &id}, {"codex", agentic.ResumeCodexThread, &id},
	} {
		for _, doorbell := range []bool{false, true} {
			name := tc.name
			inv := cli.Invocation{EnvID: "claude_code", BoardGoal: true, SystemPrompt: cli.SystemPromptAppend, SessionLabel: "example", Detach: true}
			if doorbell {
				name += "-doorbell"
				inv.A2A = "example.dev"
			}
			t.Run(name, func(t *testing.T) {
				resolved := defaults.Resolved{SelectorDecisionID: "decision/@+1", Model: defaults.ResolvedMember{Member: defaults.Member{Value: "claude-opus-5"}}, Effort: defaults.ResolvedMember{Member: defaults.Member{Value: "medium"}}}
				op, data, err := delegatedRequest(inv, frag, resolved, execution.PermissionDecision{Mode: agentic.PermissionModeYolo, Source: "flag"}, "/workspace", []string{"hello <&é>"}, agentic.ResumeIntent{Kind: tc.kind, Identity: tc.identity}, at, nil)
				if err != nil {
					t.Fatal(err)
				}
				got := noNotificationSnapshotOf(t, op, data)
				want, exists := golden[name]
				if !exists || got.Operation != want.Operation || !bytes.Equal(got.Wire, want.Wire) || !bytes.Equal(got.Canonical, want.Canonical) || got.Digest != want.Digest {
					t.Fatalf("no-notification request differs from main: %s\nwire=%s\ncanonical=%s\ndigest=%s", name, got.Wire, got.Canonical, got.Digest)
				}
				// A stray member changes every byte/digest witness; it must not be masked
				// by the UUID normalization or by comparisons of selected fields only.
				stray := append([]byte(`{"notification":null,`), got.Wire[1:]...)
				changed := noNotificationSnapshotOf(t, op, stray)
				if bytes.Equal(changed.Wire, want.Wire) || bytes.Equal(changed.Canonical, want.Canonical) || changed.Digest == want.Digest {
					t.Fatal("stray notification escaped the main oracle")
				}
			})
		}
	}
	if len(golden) != 10 {
		t.Fatalf("expected ten main controls, got %d", len(golden))
	}
}
