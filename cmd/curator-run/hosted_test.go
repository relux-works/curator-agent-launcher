package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/relux-works/skill-agents-management/pkg/agentic"
	claudeSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/claude"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/hosted"
)

// hostedHarness extends the pipeline fixture with the hosted doubles: a
// system registry for elevation, a spying receiver lookup, a terminal file,
// and the fake receiver control surface.
type hostedHarness struct {
	fix             *pipelineFixture
	lookupCalls     int
	terminalCalls   int
	lookupErr       error
	lookupPath      string
	receiverExit    string
	receiverCorrupt string
	receiverRecords []string
	captureDir      string
	ttyPath         string
}

func newHostedHarness(t *testing.T, environment string, tracked bool) *hostedHarness {
	t.Helper()
	return wrapHostedHarness(t, entryFixture(t, environment, tracked))
}

func wrapHostedHarness(t *testing.T, f *pipelineFixture) *hostedHarness {
	t.Helper()
	h := &hostedHarness{fix: f}
	systems := agentic.NewRegistry()
	if err := systems.Register(claudeSystem.New()); err != nil {
		t.Fatal(err)
	}
	h.fix.deps.systems = systems
	h.fix.deps.stdinTerminal = func() bool { return true }
	h.lookupPath = hostedReceiver
	h.fix.deps.receiverLookup = func() (string, error) {
		h.lookupCalls++
		if h.lookupErr != nil {
			return "", h.lookupErr
		}
		return h.lookupPath, nil
	}
	h.captureDir = t.TempDir()
	h.ttyPath = filepath.Join(h.captureDir, "tty.txt")
	if err := os.WriteFile(h.ttyPath, []byte("entry-terminal"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.fix.deps.receiverTerminal = func() (*os.File, error) {
		h.terminalCalls++
		// Read-write like production /dev/tty: the r6 §4 terminal
		// validation requires O_RDWR.
		return os.OpenFile(h.ttyPath, os.O_RDWR, 0)
	}
	t.Setenv("RECEIVER_CAPTURE_DIR", h.captureDir)
	return h
}

func (h *hostedHarness) run(t *testing.T, args []string) (int, string, string) {
	t.Helper()
	h.fix.args = args
	if len(h.receiverRecords) > 0 {
		path := filepath.Join(h.captureDir, "records.jsonl")
		if err := os.WriteFile(path, []byte(strings.Join(h.receiverRecords, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("RECEIVER_RECORDS", path)
	}
	if h.receiverExit != "" {
		t.Setenv("RECEIVER_EXIT", h.receiverExit)
	}
	if h.receiverCorrupt != "" {
		t.Setenv("RECEIVER_CORRUPT", h.receiverCorrupt)
	}
	code, out, stderr := h.fix.run()
	return code, string(out), string(stderr)
}

func (h *hostedHarness) capture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(h.captureDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func (h *hostedHarness) payload(t *testing.T) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(h.capture(t, "stdin.bin"), &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func writeDefaults(t *testing.T, path, doc string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, path, []byte(doc), 0o600)
}

func lastDiagnostic(stderr string) string {
	lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
	return lines[len(lines)-1]
}

func containsLine(stderr, want string) bool {
	for _, line := range strings.Split(stderr, "\n") {
		if line == want {
			return true
		}
	}
	return false
}

// TestHostedRefusalsBeforeContact drives every pre-contact hosted refusal
// through the production entry point and proves each fires before any
// receiver contact: lookup and terminal spies must stay at zero.
func TestHostedRefusalsBeforeContact(t *testing.T) {
	uuid := "01234567-89ab-cdef-0123-456789abcdef"
	for _, tc := range []struct {
		name      string
		env       string
		tracked   bool
		useMuse   bool
		setup     func(t *testing.T, h *hostedHarness)
		args      []string
		wantCode  int
		wantLine  string
		wantUsage bool
	}{
		{
			name: "host-ax-conflict", env: "claude_code", tracked: true,
			args:     []string{"claude_code", "--hosted"},
			wantCode: 2, wantLine: "curator-run: host_configuration_conflict: hosted execution conflicts with the enabled ax integration on this machine",
		},
		// Q-D1a (2026-10-05) = yes: an operator hosted default is
		// admitted (TestOperatorHostedDefaultAdmitted). Only machine
		// hosted defaults stay gated, locked or not.
		{
			name: "machine-hosted-default-not-ready", env: "claude_code",
			setup: func(t *testing.T, h *hostedHarness) {
				writeDefaults(t, h.fix.deps.defaults.Machine, `{"schema":"curator-run-defaults-v3","defaults":{"claude_code":{"host":"hosted"}}}`)
			},
			args:     []string{"claude_code"},
			wantCode: 16, wantLine: "curator-run: session_host_default_not_ready: the machine hosted default for claude_code is not ready until upgrade-without-hangup; use explicit --hosted or an operator default",
		},
		{
			name: "locked-machine-hosted-default-not-ready", env: "claude_code",
			setup: func(t *testing.T, h *hostedHarness) {
				writeDefaults(t, h.fix.deps.defaults.Machine, `{"schema":"curator-run-defaults-v3","locked":true,"defaults":{"claude_code":{"host":"hosted"}}}`)
				writeDefaults(t, h.fix.deps.defaults.Operator, `{"schema":"curator-run-defaults-v3","defaults":{"claude_code":{"host":"native"}}}`)
			},
			args:     []string{"claude_code"},
			wantCode: 16, wantLine: "curator-run: session_host_default_not_ready: the machine hosted default for claude_code is not ready until upgrade-without-hangup; use explicit --hosted or an operator default",
		},
		{
			name: "network-hosted", env: "claude_code",
			args:     []string{"claude_code", "--hosted", "--network", "office"},
			wantCode: 16, wantLine: "curator-run: network_scope_unsupported: managed network selections are not supported by hosted execution",
		},
		{
			name: "network-direct", env: "claude_code",
			args:     []string{"claude_code", "--hosted", "--network", "direct"},
			wantCode: 16, wantLine: "curator-run: network_scope_unsupported: managed network selections are not supported by hosted execution",
		},
		{
			// The native --network path is SPEC §4.4b (network_test.go);
			// only a hosted launch, including one chosen by the operator
			// default, refuses with the hosted policy exit.
			name: "network-hosted-operator-default", env: "claude_code",
			setup: func(t *testing.T, h *hostedHarness) {
				writeDefaults(t, h.fix.deps.defaults.Operator, `{"schema":"curator-run-defaults-v3","defaults":{"claude_code":{"host":"hosted"}}}`)
			},
			args:     []string{"claude_code", "--network", "office"},
			wantCode: 16, wantLine: "curator-run: network_scope_unsupported: managed network selections are not supported by hosted execution",
		},
		{
			name: "resume-positional-native", env: "claude_code",
			args: []string{"claude_code", "resume"}, wantCode: 2, wantUsage: true,
			wantLine: "curator-run: usage: resume selectors require hosted execution; use --hosted or a hosted default",
		},
		{
			name: "resume-handle-native", env: "claude_code",
			args: []string{"claude_code", "resume", "SES-1"}, wantCode: 2, wantUsage: true,
			wantLine: "curator-run: usage: resume selectors require hosted execution; use --hosted or a hosted default",
		},
		{
			name: "resume-flag-native", env: "claude_code",
			args: []string{"claude_code", "--resume", uuid}, wantCode: 2, wantUsage: true,
			wantLine: "curator-run: usage: resume selectors require hosted execution; use --hosted or a hosted default",
		},
		// Q-D3 (2026-10-05): hosted yolo from any level is admitted
		// (TestHostedYoloAdmitted); the tracked refusal stays native-only
		// (TestChoice5PermissionRowsThroughRealCuratorRun/tracked-yolo).
		{
			name: "hosted-yolo-without-transport", env: "claude_code",
			args:     []string{"claude_code", "--hosted", "--permissions", "yolo"},
			wantCode: 1, wantLine: "curator-run: permission_policy_unsupported: the fragment does not establish permission-mode transport",
		},
		{
			// Q-D3 literal: the unconfigured default is yolo, so a
			// legacy v1 fragment (no permission transport) refuses
			// even in silence, on every stdio shape.
			name: "hosted-unconfigured-without-transport", env: "claude_code",
			args:     []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium"},
			wantCode: 1, wantLine: "curator-run: permission_policy_unsupported: the fragment does not establish permission-mode transport",
		},
		{
			name: "provider-codex", env: "codex_cli",
			args:     []string{"codex_cli", "--hosted", "--model", "gpt-6-astra", "--effort", "medium", "--permissions", "native"},
			wantCode: 6, wantLine: "curator-run: session_host_provider_unsupported: hosted execution in Phase 1 admits claude_code only; codex_cli is not admitted",
		},
		{
			name: "provider-pi", env: "pi",
			args:     []string{"pi", "--hosted", "--effort", "high", "--permissions", "native"},
			wantCode: 6, wantLine: "curator-run: session_host_provider_unsupported: hosted execution in Phase 1 admits claude_code only; pi is not admitted",
		},
		{
			name: "provider-muse", env: "muse", useMuse: true,
			args:     []string{"muse", "--hosted", "--permissions=native"},
			wantCode: 6, wantLine: "curator-run: session_host_provider_unsupported: hosted execution in Phase 1 admits claude_code only; muse is not admitted",
		},
		{
			name: "resume-bad-handle", env: "claude_code",
			args:     []string{"claude_code", "--hosted", "--permissions", "native", "resume", "BADHANDLE"},
			wantCode: 2, wantLine: "curator-run: session_resume_invalid: invalid identity or provider kind",
		},
		{
			name: "resume-both-selectors", env: "claude_code",
			args:     []string{"claude_code", "--hosted", "--permissions", "native", "resume", "SES-1", "--resume", uuid},
			wantCode: 2, wantLine: "curator-run: session_resume_invalid: conflicting selectors",
		},
		{
			name: "resume-tail-conflict", env: "claude_code",
			args:     []string{"claude_code", "--hosted", "--permissions", "native", "--", "--resume", uuid, "--resume", uuid},
			wantCode: 2, wantLine: "curator-run: session_resume_invalid: conflicting selectors",
		},
		{
			name: "resume-tail-ambiguous", env: "claude_code",
			args:     []string{"claude_code", "--hosted", "--permissions", "native", "--", "--session-id", "zzz"},
			wantCode: 2, wantLine: "curator-run: session_resume_invalid: ambiguous native identity",
		},
		{
			name: "stdin-attached", env: "claude_code",
			setup: func(t *testing.T, h *hostedHarness) {
				inner := h.fix.deps.build
				h.fix.deps.build = func(ctx context.Context, r *vendorplugin.Registry, req vendorplugin.SpawnRequest, mode agentic.LaunchMode) (agentic.PlanWithEnvironment, error) {
					built, err := inner(ctx, r, req, mode)
					if err == nil {
						built.Plan.Stdin = agentic.StdinPayload{Attached: true, Bytes: []byte("prompt-bytes")}
					}
					return built, err
				}
			},
			args:     []string{"claude_code", "--hosted", "--permissions", "native"},
			wantCode: 6, wantLine: "curator-run: session_host_stdin_unsupported: hosted launches in Phase 1 do not support attached stdin",
		},
		{
			name: "stdin-piped", env: "claude_code",
			setup: func(t *testing.T, h *hostedHarness) {
				h.fix.deps.stdinTerminal = func() bool { return false }
			},
			args:     []string{"claude_code", "--hosted", "--permissions", "native"},
			wantCode: 6, wantLine: "curator-run: session_host_stdin_unsupported: hosted launches in Phase 1 do not support piped stdin",
		},
		{
			name: "seal-unexportable", env: "claude_code",
			setup: func(t *testing.T, h *hostedHarness) {
				h.fix.deps.build = func(ctx context.Context, r *vendorplugin.Registry, req vendorplugin.SpawnRequest, mode agentic.LaunchMode) (agentic.PlanWithEnvironment, error) {
					return agentic.PlanWithEnvironment{
						Plan: agentic.Plan{System: "claude-code", Binary: req.WorkDir + "/claude", Home: req.Home, WorkDir: req.WorkDir},
					}, nil
				}
			},
			args:     []string{"claude_code", "--hosted", "--permissions", "native"},
			wantCode: 2, wantLine: "curator-run: launch_plan_invalid: process.exec_guard: plan exports no hosted-admissible guard",
		},
		{
			name: "receiver-missing", env: "claude_code",
			setup: func(t *testing.T, h *hostedHarness) {
				h.lookupErr = errors.New("no receiver on PATH")
				// The provider binary stays: a silent native fallback would
				// launch it and exit 0 instead of refusing session_host_missing.
			},
			args:     []string{"claude_code", "--hosted", "--permissions", "native"},
			wantCode: 1, wantLine: "curator-run: session_host_missing: the task-board receiver is not available on PATH",
		},
		{
			name: "terminal-missing", env: "claude_code",
			setup: func(t *testing.T, h *hostedHarness) {
				h.fix.deps.receiverTerminal = func() (*os.File, error) {
					h.terminalCalls++
					return nil, errors.New("no terminal")
				}
			},
			args:     []string{"claude_code", "--hosted", "--permissions", "native"},
			wantCode: 6, wantLine: "curator-run: session_host_terminal_required: a hosted launch requires a controlling terminal",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var h *hostedHarness
			if tc.useMuse {
				f, _ := museFixture(t)
				h = wrapHostedHarness(t, f)
			} else {
				h = newHostedHarness(t, tc.env, tc.tracked)
			}
			if tc.setup != nil {
				tc.setup(t, h)
			}
			code, _, stderr := h.run(t, tc.args)
			if code != tc.wantCode {
				t.Fatalf("exit = %d, want %d (stderr %q)", code, tc.wantCode, stderr)
			}
			if !containsLine(stderr, tc.wantLine) {
				t.Fatalf("stderr %q lacks the line %q", stderr, tc.wantLine)
			}
			if tc.wantUsage && !strings.HasSuffix(stderr, cli.Usage) {
				t.Fatalf("stderr %q lacks the usage suffix", stderr)
			}
			// The entry point opens the terminal before the plan build so a
			// known terminal failure refuses with zero builds and zero
			// lookup; the transport reuses that descriptor. Rows that fail
			// before the preopen never probe; rows that fail after the
			// build hold the preopened terminal once; only
			// receiver-missing reaches receiver lookup.
			wantLookup, wantTerminal := 0, 0
			switch tc.name {
			case "receiver-missing":
				wantLookup, wantTerminal = 1, 1
			case "terminal-missing":
				wantLookup, wantTerminal = 0, 1
			case "stdin-attached", "seal-unexportable":
				wantLookup, wantTerminal = 0, 1
			}
			if h.lookupCalls != wantLookup || h.terminalCalls != wantTerminal {
				t.Fatalf("lookup calls = %d, terminal calls = %d; want %d and %d",
					h.lookupCalls, h.terminalCalls, wantLookup, wantTerminal)
			}
			// The known terminal failure refuses before any build,
			// composition, export, or receiver lookup.
			if tc.name == "terminal-missing" && h.fix.builds != 0 {
				t.Fatalf("builds = %d, want zero builds before the known-terminal refusal", h.fix.builds)
			}
		})
	}
}

// TestOperatorHostedDefaultAdmitted proves operator decision Q-D1a
// (2026-10-05) = yes: an operator-scope hosted default with no flag
// completes the hosted handoff — the payload reaches the receiver and
// the launch exits 0 — while a machine hosted default still refuses
// (TestHostedRefusalsBeforeContact/machine-hosted-default-not-ready).
func TestOperatorHostedDefaultAdmitted(t *testing.T) {
	h := newHostedHarness(t, "claude_code", false)
	writeDefaults(t, h.fix.deps.defaults.Operator, `{"schema":"curator-run-defaults-v3","defaults":{"claude_code":{"host":"hosted"}}}`)
	code, _, stderr := h.run(t, []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"})
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, stderr)
	}
	payload := h.payload(t)
	if payload["schema"] != "urn:relux:task-board:session-launch-plan" || payload["schema_version"] != "1.0.0" {
		t.Fatalf("payload identity = %v/%v", payload["schema"], payload["schema_version"])
	}
	if h.lookupCalls != 1 || h.terminalCalls != 1 {
		t.Fatalf("lookup calls = %d, terminal calls = %d; want one contact each", h.lookupCalls, h.terminalCalls)
	}
}

// TestHostedRefusalOrder proves the §4.8 order with combined triggers: ax
// routing beats the defaults gate, the gate beats the network selection,
// the network beats permissions, permissions beat provider admission, and
// admission beats resume elevation.
func TestHostedRefusalOrder(t *testing.T) {
	uuid := "01234567-89ab-cdef-0123-456789abcdef"
	for _, tc := range []struct {
		name         string
		env          string
		tracked      bool
		setup        func(t *testing.T, h *hostedHarness)
		args         []string
		wantCode     int
		wantCodeName string
	}{
		{
			name: "ax-beats-network", env: "claude_code", tracked: true,
			args:     []string{"claude_code", "--hosted", "--network", "office"},
			wantCode: 2, wantCodeName: "host_configuration_conflict",
		},
		{
			// Q-D1a: only a machine hosted default is still gated,
			// so the order proof uses the machine file.
			name: "gate-beats-network", env: "claude_code",
			setup: func(t *testing.T, h *hostedHarness) {
				writeDefaults(t, h.fix.deps.defaults.Machine, `{"schema":"curator-run-defaults-v3","defaults":{"claude_code":{"host":"hosted"}}}`)
			},
			args:     []string{"claude_code", "--network", "office"},
			wantCode: 16, wantCodeName: "session_host_default_not_ready",
		},
		{
			name: "network-beats-permission", env: "claude_code",
			args:     []string{"claude_code", "--hosted", "--network", "office", "--permissions", "yolo"},
			wantCode: 16, wantCodeName: "network_scope_unsupported",
		},
		{
			name: "network-beats-provider", env: "codex_cli",
			args:     []string{"codex_cli", "--hosted", "--network", "office", "--model", "gpt-6-astra", "--effort", "medium"},
			wantCode: 16, wantCodeName: "network_scope_unsupported",
		},
		{
			// Q-D3 admits hosted yolo, so the permission-before-provider
			// order proof uses the transport gate: the default v1
			// fixture fragment cannot carry yolo on any path.
			name: "permission-beats-provider", env: "codex_cli",
			args:     []string{"codex_cli", "--hosted", "--permissions", "yolo", "--model", "gpt-6-astra", "--effort", "medium"},
			wantCode: 1, wantCodeName: "permission_policy_unsupported",
		},
		{
			name: "provider-beats-resume", env: "codex_cli",
			args:     []string{"codex_cli", "--hosted", "--permissions", "native", "resume", "BADHANDLE", "--model", "gpt-6-astra", "--effort", "medium"},
			wantCode: 6, wantCodeName: "session_host_provider_unsupported",
		},
		{
			name: "network-beats-resume", env: "claude_code",
			args:     []string{"claude_code", "--hosted", "--network", "office", "--", "--resume", uuid, "--resume", uuid},
			wantCode: 16, wantCodeName: "network_scope_unsupported",
		},
		{
			name: "resume-beats-stdin", env: "claude_code",
			setup: func(t *testing.T, h *hostedHarness) {
				h.fix.deps.stdinTerminal = func() bool { return false }
			},
			args:     []string{"claude_code", "--hosted", "--permissions", "native", "resume", "BADHANDLE"},
			wantCode: 2, wantCodeName: "session_resume_invalid",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHostedHarness(t, tc.env, tc.tracked)
			if tc.setup != nil {
				tc.setup(t, h)
			}
			code, _, stderr := h.run(t, tc.args)
			if code != tc.wantCode {
				t.Fatalf("exit = %d, want %d (stderr %q)", code, tc.wantCode, stderr)
			}
			line := lastDiagnostic(stderr)
			if !strings.HasPrefix(line, "curator-run: "+tc.wantCodeName+":") {
				t.Fatalf("diagnostic = %q, want the %s line", line, tc.wantCodeName)
			}
			if h.lookupCalls != 0 || h.terminalCalls != 0 {
				t.Fatalf("lookup calls = %d, terminal calls = %d; want zero contact", h.lookupCalls, h.terminalCalls)
			}
		})
	}
}

// TestQd3UnconfiguredDefaultIsYolo pins operator decision Q-D3
// (2026-10-05) literally through the production entry point: with no
// permission configuration (no flag, no per-env or global default),
// the requested mode is yolo for native and hosted launches alike,
// on interactive, non-interactive, CI, and CI-present-empty stdio.
// Headless detection must not change the permission default.
func TestQd3UnconfiguredDefaultIsYolo(t *testing.T) {
	for _, hosted := range []bool{false, true} {
		for _, tc := range []struct {
			name  string
			setup func(t *testing.T, h *hostedHarness)
		}{
			{"interactive", func(t *testing.T, h *hostedHarness) {
				h.fix.usePermissionsFragment(t, "native", false, "default")
				h.fix.deps.isTerminal = func() bool { return true }
			}},
			{"nonterminal", func(t *testing.T, h *hostedHarness) {
				h.fix.usePermissionsFragment(t, "native", false, "default")
				h.fix.deps.isTerminal = func() bool { return false }
			}},
			{"ci-true", func(t *testing.T, h *hostedHarness) {
				h.fix.usePermissionsFragment(t, "native", false, "default")
				h.fix.deps.isTerminal = func() bool { return true }
				h.fix.addEnv("CI", "true")
			}},
			{"ci-present-empty", func(t *testing.T, h *hostedHarness) {
				h.fix.usePermissionsFragment(t, "native", false, "default")
				h.fix.deps.isTerminal = func() bool { return true }
				h.fix.addEnv("CI", "")
			}},
			{"github-actions", func(t *testing.T, h *hostedHarness) {
				h.fix.usePermissionsFragment(t, "native", false, "default")
				h.fix.deps.isTerminal = func() bool { return true }
				h.fix.addEnv("GITHUB_ACTIONS", "true")
			}},
		} {
			name := "native/" + tc.name
			if hosted {
				name = "hosted/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				h := newHostedHarness(t, "claude_code", false)
				tc.setup(t, h)
				args := []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium"}
				if hosted {
					args = []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium"}
				}
				code, _, stderr := h.run(t, args)
				if code != 0 {
					t.Fatalf("exit = %d, want 0 (stderr %q)", code, stderr)
				}
				if h.fix.request.PermissionMode != agentic.PermissionModeYolo {
					t.Fatalf("spawn request mode = %q, want yolo", h.fix.request.PermissionMode)
				}
				if hosted {
					policy := h.payload(t)["policy"].(map[string]any)
					if policy["permission_source"] != "default-interactive" || policy["execution_profile"] != "yolo" {
						t.Fatalf("hosted policy = %v, want default-interactive/yolo", policy)
					}
				} else if !strings.Contains(stderr, "curator-run: permissions=yolo source=default-interactive mapped=") {
					t.Fatalf("stderr %q lacks the yolo provenance line", stderr)
				}
			})
		}
	}
}

// TestTrackedNativeUnconfiguredStaysNative pins the preserved tracked
// term beside Q-D3: a tracked native launch cannot use yolo at all, so
// tracked silence keeps the native built-in instead of refusing.
func TestTrackedNativeUnconfiguredStaysNative(t *testing.T) {
	h := newHostedHarness(t, "claude_code", true)
	h.fix.usePermissionsFragment(t, "native", false, "default")
	h.fix.deps.isTerminal = func() bool { return false }
	code, _, stderr := h.run(t, []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium"})
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, stderr)
	}
	if h.fix.request.PermissionMode != agentic.PermissionModeNative {
		t.Fatalf("spawn request mode = %q, want native", h.fix.request.PermissionMode)
	}
	if h.lookupCalls != 0 || h.terminalCalls != 0 {
		t.Fatalf("lookup calls = %d, terminal calls = %d; want zero", h.lookupCalls, h.terminalCalls)
	}
}

// TestHostedPipedStdinRefusesBeforeBuild proves the known-input gate
// order: a piped launcher stdin is knowable before composition, so it
// refuses before the plan build and before any receiver contact. A
// native launch with the same piped stdin still launches: the gate is
// hosted-only.
func TestHostedPipedStdinRefusesBeforeBuild(t *testing.T) {
	t.Run("hosted-refuses-with-zero-builds", func(t *testing.T) {
		h := newHostedHarness(t, "claude_code", false)
		h.fix.deps.stdinTerminal = func() bool { return false }
		code, _, stderr := h.run(t, []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"})
		if code != 6 {
			t.Fatalf("exit = %d, want 6 (stderr %q)", code, stderr)
		}
		if got := lastDiagnostic(stderr); got != "curator-run: session_host_stdin_unsupported: hosted launches in Phase 1 do not support piped stdin" {
			t.Fatalf("diagnostic = %q", got)
		}
		if h.fix.builds != 0 {
			t.Fatalf("builds = %d, want zero builds before the known-input refusal", h.fix.builds)
		}
		if h.lookupCalls != 0 || h.terminalCalls != 0 {
			t.Fatalf("lookup calls = %d, terminal calls = %d; want zero contact", h.lookupCalls, h.terminalCalls)
		}
	})
	t.Run("piped-valid-resume-still-refuses", func(t *testing.T) {
		h := newHostedHarness(t, "claude_code", false)
		h.fix.deps.stdinTerminal = func() bool { return false }
		code, _, stderr := h.run(t, []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "resume", "SES-42"})
		if code != 6 {
			t.Fatalf("exit = %d, want 6 (stderr %q)", code, stderr)
		}
		if h.fix.builds != 0 || h.lookupCalls != 0 {
			t.Fatalf("builds = %d, lookups = %d; want zero", h.fix.builds, h.lookupCalls)
		}
	})
	t.Run("native-launches-piped", func(t *testing.T) {
		h := newHostedHarness(t, "claude_code", false)
		h.fix.deps.stdinTerminal = func() bool { return false }
		code, _, stderr := h.run(t, []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"})
		if code != 0 {
			t.Fatalf("exit = %d, want 0 (stderr %q)", code, stderr)
		}
		if h.fix.builds != 1 || h.lookupCalls != 0 {
			t.Fatalf("builds = %d, lookups = %d; want one build and zero contact", h.fix.builds, h.lookupCalls)
		}
	})
	t.Run("default-probe-consults-deps-stdin", func(t *testing.T) {
		h := newHostedHarness(t, "claude_code", false)
		h.fix.deps.stdinTerminal = nil
		code, _, stderr := h.run(t, []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"})
		if code != 6 {
			t.Fatalf("exit = %d, want 6 (stderr %q)", code, stderr)
		}
		if h.fix.builds != 0 {
			t.Fatalf("builds = %d, want zero", h.fix.builds)
		}
	})
}

// TestHostedTerminalMissingRefusesBeforeBuild proves the known
// controlling-terminal failure refuses before the plan build,
// composition, export, and receiver lookup: builds and lookups stay
// zero while the terminal opens exactly once. The transport reuses
// that descriptor on success (TestOperatorHostedDefaultAdmitted).
func TestHostedTerminalMissingRefusesBeforeBuild(t *testing.T) {
	h := newHostedHarness(t, "claude_code", false)
	h.fix.deps.receiverTerminal = func() (*os.File, error) {
		h.terminalCalls++
		return nil, errors.New("no controlling terminal")
	}
	code, _, stderr := h.run(t, []string{"claude_code", "--hosted", "--permissions", "native", "--model", "claude-opus-5", "--effort", "medium"})
	if code != 6 {
		t.Fatalf("exit = %d, want 6 (stderr %q)", code, stderr)
	}
	if got := lastDiagnostic(stderr); got != "curator-run: session_host_terminal_required: a hosted launch requires a controlling terminal" {
		t.Fatalf("diagnostic = %q", got)
	}
	if h.fix.builds != 0 {
		t.Fatalf("builds = %d, want zero builds before the known-terminal refusal", h.fix.builds)
	}
	if h.lookupCalls != 0 {
		t.Fatalf("lookups = %d, want zero receiver lookup before the known-terminal refusal", h.lookupCalls)
	}
	if h.terminalCalls != 1 {
		t.Fatalf("terminal calls = %d, want exactly one terminal probe", h.terminalCalls)
	}
}

// TestHostedTerminalValidationRefusesBeforeBuild proves a wrong terminal
// descriptor (here a directory) refuses session_host_terminal_required
// before the plan build, composition, export, and receiver lookup,
// like a missing terminal. The diagnostic carries no descriptor values.
func TestHostedTerminalValidationRefusesBeforeBuild(t *testing.T) {
	h := newHostedHarness(t, "claude_code", false)
	h.fix.deps.receiverTerminal = func() (*os.File, error) {
		h.terminalCalls++
		return os.Open(h.captureDir)
	}
	code, _, stderr := h.run(t, []string{"claude_code", "--hosted", "--permissions", "native", "--model", "claude-opus-5", "--effort", "medium"})
	if code != 6 {
		t.Fatalf("exit = %d, want 6 (stderr %q)", code, stderr)
	}
	if got := lastDiagnostic(stderr); got != "curator-run: session_host_terminal_required: a hosted launch requires a controlling terminal" {
		t.Fatalf("diagnostic = %q", got)
	}
	if h.fix.builds != 0 {
		t.Fatalf("builds = %d, want zero builds before the terminal validation refusal", h.fix.builds)
	}
	if h.lookupCalls != 0 {
		t.Fatalf("lookups = %d, want zero receiver lookup before the terminal validation refusal", h.lookupCalls)
	}
	if h.terminalCalls != 1 {
		t.Fatalf("terminal calls = %d, want exactly one terminal probe", h.terminalCalls)
	}
}

// TestHostedConflictDiagnosticCarriesNoValues proves hosted build
// refusals never copy native argument values into stderr: a conflicting
// native value and an unknown policy mode value both refuse with the
// conflict channel or the fixed sentinel, and the secret-shaped markers
// stay out of every stderr byte. The native legacy shape is pinned
// separately by TestProductionContextCarrierRefusesNativeConflicts.
func TestHostedConflictDiagnosticCarriesNoValues(t *testing.T) {
	t.Run("context-conflict", func(t *testing.T) {
		h := newHostedHarness(t, "claude_code", false)
		marker := "SYNTHETIC_PRIVATE_VALUE"
		code, _, stderr := h.run(t, []string{"claude_code", "--hosted", "--permissions", "native", "--model", "claude-opus-5", "--effort", "medium", "--system-prompt", "append", "--", "--append-system-prompt", marker})
		if code != 1 {
			t.Fatalf("exit = %d, want 1 (stderr %q)", code, stderr)
		}
		if strings.Contains(stderr, marker) {
			t.Fatalf("stderr copies the native value: %q", stderr)
		}
		if got := lastDiagnostic(stderr); !strings.HasPrefix(got, "curator-run: plan_refused: native arguments conflict with fragment channel system-prompt: ") {
			t.Fatalf("diagnostic = %q, want the channel-only conflict", got)
		}
		// The build failure holds the terminal preopened before the
		// build; the receiver is never looked up.
		if h.lookupCalls != 0 || h.terminalCalls != 1 {
			t.Fatalf("lookup calls = %d, terminal calls = %d; want zero lookup and the preopened terminal", h.lookupCalls, h.terminalCalls)
		}
	})
	t.Run("unknown-policy-mode", func(t *testing.T) {
		h := newHostedHarness(t, "claude_code", false)
		h.fix.usePermissionsFragment(t, "native", false, "default")
		marker := "SYNTHETIC_SECRET_MODE"
		code, _, stderr := h.run(t, []string{"claude_code", "--hosted", "--permissions", "yolo", "--model", "claude-opus-5", "--effort", "medium", "--", "--permission-mode", marker})
		if code != 2 {
			t.Fatalf("exit = %d, want 2 (stderr %q)", code, stderr)
		}
		if strings.Contains(stderr, marker) {
			t.Fatalf("stderr copies the native value: %q", stderr)
		}
		if !strings.Contains(stderr, "curator-run: usage: agentic: unknown native policy form") {
			t.Fatalf("stderr %q lacks the fixed sentinel", stderr)
		}
		if h.lookupCalls != 0 {
			t.Fatalf("lookup calls = %d, want zero contact", h.lookupCalls)
		}
	})
}

// TestNativePerformsZeroReceiverLookup proves native routing — implicit,
// flagged, configured, locked, tracked, and bypassed — never resolves the
// receiver or opens the terminal. The spies fail the test on any call by
// counting; the count must stay zero on every native path.
func TestNativePerformsZeroReceiverLookup(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tracked bool
		setup   func(t *testing.T, h *hostedHarness)
		args    []string
	}{
		{"implicit-native", false, nil, []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"}},
		{"explicit-native", false, nil, []string{"claude_code", "--native", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"}},
		{"explicit-untracked", false, nil, []string{"claude_code", "--untracked", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"}},
		{"native-tail", false, nil, []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--", "--resume", "01234567-89ab-cdef-0123-456789abcdef"}},
		{
			name: "operator-native-default", tracked: false,
			setup: func(t *testing.T, h *hostedHarness) {
				writeDefaults(t, h.fix.deps.defaults.Operator, `{"schema":"curator-run-defaults-v3","defaults":{"claude_code":{"host":"native"}}}`)
			},
			args: []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"},
		},
		{
			name: "locked-ignores-operator-hosted", tracked: false,
			setup: func(t *testing.T, h *hostedHarness) {
				writeDefaults(t, h.fix.deps.defaults.Machine, `{"schema":"curator-run-defaults-v3","locked":true,"defaults":{"claude_code":{"model":"claude-opus-5","effort":"medium"}}}`)
				writeDefaults(t, h.fix.deps.defaults.Operator, `{"schema":"curator-run-defaults-v3","defaults":{"claude_code":{"host":"hosted"}}}`)
			},
			args: []string{"claude_code", "--permissions", "native"},
		},
		{"tracked-implicit-native", true, nil, []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"}},
		{
			name: "tracked-configured-native", tracked: true,
			setup: func(t *testing.T, h *hostedHarness) {
				writeDefaults(t, h.fix.deps.defaults.Operator, `{"schema":"curator-run-defaults-v3","defaults":{"claude_code":{"host":"native"}}}`)
			},
			args: []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"},
		},
		{"tracked-explicit-native-bypass", true, nil, []string{"claude_code", "--native", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"}},
		{"tracked-explicit-untracked-bypass", true, nil, []string{"claude_code", "--untracked", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHostedHarness(t, "claude_code", tc.tracked)
			if tc.setup != nil {
				tc.setup(t, h)
			}
			code, _, stderr := h.run(t, tc.args)
			if code != 0 {
				t.Fatalf("exit = %d, want 0 (stderr %q)", code, stderr)
			}
			if h.lookupCalls != 0 || h.terminalCalls != 0 {
				t.Fatalf("lookup calls = %d, terminal calls = %d; native must not probe", h.lookupCalls, h.terminalCalls)
			}
		})
	}
}

// TestExplicitNativeBypass proves the legacy ax table: an explicit native
// flag on an ax-configured machine takes the untracked direct path (the ax
// binary is never consulted), while implicit and configured native take
// the existing ax path (a missing ax binary fails the launch).
func TestExplicitNativeBypass(t *testing.T) {
	for _, tc := range []struct {
		name     string
		setup    func(t *testing.T, h *hostedHarness)
		args     []string
		wantCode int
	}{
		{
			name: "explicit-native-bypasses-ax", setup: nil,
			args: []string{"claude_code", "--native", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"}, wantCode: 0,
		},
		{
			name: "explicit-untracked-bypasses-ax", setup: nil,
			args: []string{"claude_code", "--untracked", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"}, wantCode: 0,
		},
		{
			name: "implicit-native-keeps-ax", setup: nil,
			args: []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"}, wantCode: 1,
		},
		{
			name: "configured-native-keeps-ax",
			setup: func(t *testing.T, h *hostedHarness) {
				writeDefaults(t, h.fix.deps.defaults.Operator, `{"schema":"curator-run-defaults-v3","defaults":{"claude_code":{"host":"native"}}}`)
			},
			args: []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"}, wantCode: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHostedHarness(t, "claude_code", true)
			h.fix.deps.axBinary = filepath.Join(h.captureDir, "no-such-ax")
			if tc.setup != nil {
				tc.setup(t, h)
			}
			code, _, stderr := h.run(t, tc.args)
			if code != tc.wantCode {
				t.Fatalf("exit = %d, want %d (stderr %q)", code, tc.wantCode, stderr)
			}
			if tc.wantCode == 1 {
				if got := lastDiagnostic(stderr); got != "curator-run: ax_handoff_failed: ax could not take the launch" {
					t.Fatalf("diagnostic = %q", got)
				}
			}
			if h.lookupCalls != 0 || h.terminalCalls != 0 {
				t.Fatalf("lookup calls = %d, terminal calls = %d; want zero", h.lookupCalls, h.terminalCalls)
			}
		})
	}
}

// launchDocument is the fake provider's stdout record of a native exec.
type launchDocument struct {
	Argv    []string
	Env     []string
	Stdin   []byte
	WorkDir string
}

func parseLaunchDocument(t *testing.T, stdout string) launchDocument {
	t.Helper()
	var doc launchDocument
	dec := json.NewDecoder(strings.NewReader(stdout))
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("decode native launch document: %v\nstdout: %q", err, stdout)
	}
	return doc
}

// TestHostedParityWithFakeReceiver proves the §6 parity claim end to end:
// for each composition and Claude RC/name row, the hosted payload process
// (binary, ordered argv, full env, cwd, home, stdin) captured by the fake
// receiver equals the native exec plan observed by the fake provider,
// including the --disallowedTools=AskUserQuestion denial in both. The
// yolo rows prove the same equality under Q-D3 hosted yolo: both sides
// carry the module's yolo mapping identically.
func TestHostedParityWithFakeReceiver(t *testing.T) {
	type sessionExpectation struct {
		name    string
		rc      bool
		indices []int
	}
	sessionWant := map[string]sessionExpectation{
		"rc-alone":              {name: "RCNAME", rc: true, indices: []int{8}},
		"name-alone":            {name: "NATNAME"},
		"name-and-rc":           {name: "A", rc: true, indices: []int{10}},
		"rc-prefix":             {rc: true, indices: []int{8}},
		"rc-bare":               {rc: true, indices: []int{8}},
		"contract-example-tail": {name: "NAME", rc: true, indices: []int{10}},
		"yolo-rc-name":          {name: "NAME", rc: true, indices: []int{11}},
	}
	tail := []string{"", "--model=native", "a\nb"}
	v2 := func(t *testing.T, h *hostedHarness) { h.fix.usePermissionsFragment(t, "native", false, "default") }
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, h *hostedHarness)
		args  []string
	}{
		{"minimal", nil, []string{"claude_code", "--permissions", "native"}},
		{"model-effort-prompt-tail", nil, append([]string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--system-prompt", "append", "--"}, tail...)},
		{"inline-json-prompt", nil, []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--", `{"json":"prompt\"with quotes"}`, "line\nbreak"}},
		{"rc-alone", nil, []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--", "--remote-control", "RCNAME"}},
		{"name-alone", nil, []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--", "-n", "NATNAME"}},
		{"rc-prefix", nil, []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--", "--remote-control-session-name-prefix", "P"}},
		{"rc-bare", nil, []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--", "--remote-control"}},
		{"contract-example-tail", nil, []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--", "-n", "NAME", "--remote-control", "NAME"}},
		{"yolo-flag", v2, []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "yolo"}},
		{"yolo-rc-name", v2, []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--yolo", "--", "-n", "NAME", "--remote-control", "NAME"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			native := newHostedHarness(t, "claude_code", false)
			if tc.setup != nil {
				tc.setup(t, native)
			}
			nativeCode, nativeOut, nativeErr := native.run(t, tc.args)
			if nativeCode != 0 {
				t.Fatalf("native exit = %d (stderr %q)", nativeCode, nativeErr)
			}
			doc := parseLaunchDocument(t, nativeOut)
			hostedH := newHostedHarness(t, "claude_code", false)
			if tc.setup != nil {
				tc.setup(t, hostedH)
			}
			hostedArgs := append([]string{tc.args[0], "--hosted"}, tc.args[1:]...)
			hostedCode, _, hostedErr := hostedH.run(t, hostedArgs)
			if hostedCode != 0 {
				t.Fatalf("hosted exit = %d (stderr %q)", hostedCode, hostedErr)
			}
			rawPayload := hostedH.capture(t, "stdin.bin")
			payload := hostedH.payload(t)
			process := payload["process"].(map[string]any)
			// Each mode runs in its own fixture root; normalize both sides
			// to <ROOT> like the pipeline goldens before comparing.
			normNative := func(s string) string { return strings.ReplaceAll(s, native.fix.dir, "<ROOT>") }
			normHosted := func(s string) string { return strings.ReplaceAll(s, hostedH.fix.dir, "<ROOT>") }
			// Binary: the payload carries the exact native plan binary,
			// absolute, which direct exec resolves without PATH.
			if normHosted(process["binary"].(string)) != normNative(native.fix.plan.Binary) {
				t.Fatalf("payload binary = %v, native plan binary = %q", process["binary"], native.fix.plan.Binary)
			}
			if !filepath.IsAbs(process["binary"].(string)) {
				t.Fatalf("payload binary %q is not absolute", process["binary"])
			}
			// Ordered argv: identical, including the module denial.
			var payloadArgv []string
			for _, arg := range process["argv"].([]any) {
				payloadArgv = append(payloadArgv, normHosted(arg.(string)))
			}
			var nativeArgv []string
			for _, arg := range doc.Argv {
				nativeArgv = append(nativeArgv, normNative(arg))
			}
			if !reflect.DeepEqual(payloadArgv, nativeArgv) {
				t.Fatalf("payload argv = %q, native argv = %q", payloadArgv, nativeArgv)
			}
			if !contains(payloadArgv, "--disallowedTools=AskUserQuestion") || !contains(nativeArgv, "--disallowedTools=AskUserQuestion") {
				t.Fatalf("denial missing: payload %q native %q", payloadArgv, nativeArgv)
			}
			// Yolo rows prove the Q-D3 export shape: identical argv on
			// both sides and execution_profile yolo in the payload.
			if strings.HasPrefix(tc.name, "yolo") {
				policy := payload["policy"].(map[string]any)
				if policy["permission_mode"] != "native" || policy["execution_profile"] != "yolo" {
					t.Fatalf("yolo policy = %v, want native mode with yolo profile", policy)
				}
				if native.fix.request.PermissionMode != agentic.PermissionModeYolo || hostedH.fix.request.PermissionMode != agentic.PermissionModeYolo {
					t.Fatalf("spawn modes = %q/%q, want yolo on both sides", native.fix.request.PermissionMode, hostedH.fix.request.PermissionMode)
				}
			}
			// Full env: identical up to the helper's CURATOR_TEST_RELEASE
			// filter, which never reaches the child env record.
			var payloadEnv []string
			for _, entry := range process["env"].([]any) {
				payloadEnv = append(payloadEnv, entry.(string))
			}
			var wantEnv []string
			for _, entry := range payloadEnv {
				if !strings.HasPrefix(entry, "CURATOR_TEST_RELEASE=") {
					wantEnv = append(wantEnv, normHosted(entry))
				}
			}
			sort.Strings(wantEnv)
			var gotEnv []string
			for _, entry := range doc.Env {
				gotEnv = append(gotEnv, normNative(entry))
			}
			sort.Strings(gotEnv)
			if !reflect.DeepEqual(gotEnv, wantEnv) {
				t.Fatalf("native env = %q, payload env = %q", gotEnv, wantEnv)
			}
			// Cwd and home.
			if normHosted(process["cwd"].(string)) != normNative(doc.WorkDir) || normHosted(process["cwd"].(string)) != "<ROOT>" {
				t.Fatalf("payload cwd = %v, native workdir = %q", process["cwd"], doc.WorkDir)
			}
			managed := payload["managed_home"].(map[string]any)
			if normHosted(managed["path"].(string)) != "<ROOT>/managed" {
				t.Fatalf("managed home = %v", managed["path"])
			}
			if !contains(payloadEnv, "CLAUDE_CONFIG_DIR="+hostedH.fix.home) {
				t.Fatal("final env lacks the managed home entry")
			}
			// Stdin: no attached plan stdin on either side, null payload.
			if process["stdin"] != nil {
				t.Fatalf("payload stdin = %v, want null", process["stdin"])
			}
			if native.fix.plan.Stdin.Attached || hostedH.fix.plan.Stdin.Attached {
				t.Fatal("plan stdin attached on a parity row")
			}
			// Restart: the template argv equals the process argv, and the
			// module's own transformation check accepts the new-process shape.
			// Both use raw hosted paths: normalization is only for the
			// cross-mode comparison above.
			var rawArgv []string
			for _, arg := range process["argv"].([]any) {
				rawArgv = append(rawArgv, arg.(string))
			}
			restart := payload["restart"].(map[string]any)
			restartData := restart["data"].(map[string]any)
			var restartArgv []string
			for _, arg := range restartData["argv"].([]any) {
				restartArgv = append(restartArgv, arg.(string))
			}
			if !reflect.DeepEqual(restartArgv, rawArgv) {
				t.Fatalf("restart argv = %q, process argv = %q", restartArgv, rawArgv)
			}
			var template claudeSystem.RestartTemplate
			restartRaw, _ := json.Marshal(restart)
			if err := json.Unmarshal(restartRaw, &template); err != nil {
				t.Fatal(err)
			}
			system, ok := hostedH.fix.deps.systems.Lookup("claude-code")
			if !ok {
				t.Fatal("claude system not registered")
			}
			exporter, ok := system.(interface {
				ValidateRestartTransformation(claudeSystem.RestartTemplate, []string, *string) error
			})
			if !ok {
				t.Fatal("claude system validates no restart transformation")
			}
			if err := exporter.ValidateRestartTransformation(template, rawArgv, nil); err != nil {
				t.Fatalf("module rejected the emitted template: %v", err)
			}
			// Digest recomputes over the captured wire bytes.
			recomputed, err := hosted.ContentDigest(rawPayload)
			if err != nil || recomputed != payload["content_digest"] {
				t.Fatalf("digest recomputation = %q, %v", recomputed, err)
			}
			// F2: the payload carries the module-filled native session
			// name and RC intent of the SAME plan. The expectations are
			// authored per row; the RC indices must be exactly the
			// positions of the RC-family tokens in the composed argv, in
			// order.
			want := sessionWant[tc.name]
			session := payload["session_name"].(map[string]any)
			remote := payload["remote_control"].(map[string]any)
			if want.name == "" {
				if session["native"] != nil {
					t.Fatalf("session_name.native = %v, want null", session["native"])
				}
			} else if session["native"] != want.name {
				t.Fatalf("session_name.native = %v, want %q", session["native"], want.name)
			}
			if remote["enabled"] != want.rc {
				t.Fatalf("remote_control.enabled = %v, want %v", remote["enabled"], want.rc)
			}
			var wantIndices []int
			for i, arg := range rawArgv {
				if strings.HasPrefix(arg, "--remote-control") {
					wantIndices = append(wantIndices, i)
				}
			}
			gotIndices := []int{}
			for _, index := range remote["argv_indices"].([]any) {
				gotIndices = append(gotIndices, int(index.(float64)))
			}
			if (len(wantIndices) != 0 || len(gotIndices) != 0) && !reflect.DeepEqual(gotIndices, wantIndices) {
				t.Fatalf("remote_control.argv_indices = %v, RC tokens sit at %v in %q", gotIndices, wantIndices, rawArgv)
			}
			if want.rc && len(want.indices) != 0 && !reflect.DeepEqual(gotIndices, want.indices) {
				t.Fatalf("remote_control.argv_indices = %v, want %v", gotIndices, want.indices)
			}
			// Warnings match across modes.
			if nativeWarnings, hostedWarnings := warningLines(nativeErr), warningLines(hostedErr); !reflect.DeepEqual(nativeWarnings, hostedWarnings) {
				t.Fatalf("native warnings = %q, hosted warnings = %q", nativeWarnings, hostedWarnings)
			}
		})
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func warningLines(stderr string) []string {
	var out []string
	for _, line := range strings.Split(stderr, "\n") {
		if strings.Contains(line, "environment override:") || strings.Contains(line, "environment literal replaces lookup:") {
			out = append(out, line)
		}
	}
	return out
}

// TestHostedEnvNamesDivergence proves the specified native/hosted split: an
// env lookup name that is absent from the caller env, or empty, behaves per
// mode. Native direct execution ignores lookup names and launches; hosted
// validates presence (absent refuses required_env_missing, empty passes).
func TestHostedEnvNamesDivergence(t *testing.T) {
	t.Run("absent-native-launches-hosted-refuses", func(t *testing.T) {
		native := newHostedHarness(t, "claude_code", false)
		native.fix.deps.environ = func() []string {
			var env []string
			for _, entry := range []string{"PATH=" + native.fix.dir, "HOME=" + native.fix.dir, "PARENT=direct-only", "CURATOR_TEST_RELEASE=2.1.261"} {
				env = append(env, entry)
			}
			return env
		}
		code, _, stderr := native.run(t, []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"})
		if code != 0 {
			t.Fatalf("native exit = %d, want 0 (stderr %q)", code, stderr)
		}
		hostedH := newHostedHarness(t, "claude_code", false)
		hostedH.fix.deps.environ = native.fix.deps.environ
		code, _, stderr = hostedH.run(t, []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"})
		if code != 2 {
			t.Fatalf("hosted exit = %d, want 2 (stderr %q)", code, stderr)
		}
		if got := lastDiagnostic(stderr); got != "curator-run: launch_plan_invalid: env_names: required_env_missing FIGMA_API_KEY" {
			t.Fatalf("diagnostic = %q", got)
		}
		if hostedH.lookupCalls != 0 {
			t.Fatalf("lookup calls = %d, want zero contact", hostedH.lookupCalls)
		}
	})
	t.Run("present-empty-passes-both", func(t *testing.T) {
		for _, mode := range []string{"native", "hosted"} {
			h := newHostedHarness(t, "claude_code", false)
			dir := h.fix.dir
			h.fix.deps.environ = func() []string {
				return []string{"PATH=" + dir, "HOME=" + dir, "PARENT=direct-only", "FIGMA_API_KEY=", "CURATOR_TEST_RELEASE=2.1.261"}
			}
			args := []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"}
			if mode == "hosted" {
				args = []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"}
			}
			if code, _, stderr := h.run(t, args); code != 0 {
				t.Fatalf("%s exit = %d (stderr %q)", mode, code, stderr)
			}
		}
	})
}

// TestHostedEnvNamesMaxItems proves the closed schema bound through the
// production entry point: 64 lookup names launch and project, while 65
// refuse launch_plan_invalid before any receiver contact.
func TestHostedEnvNamesMaxItems(t *testing.T) {
	for _, count := range []int{64, 65} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			h := newHostedHarness(t, "claude_code", false)
			var obj map[string]any
			if err := json.Unmarshal([]byte(h.fix.resolver.stdout), &obj); err != nil {
				t.Fatal(err)
			}
			names := []string{"FIGMA_API_KEY"}
			var extra []string
			for i := 1; i < count; i++ {
				name := fmt.Sprintf("HOSTED_ENV_%02d", i)
				names = append(names, name)
				extra = append(extra, name+"=fixture")
			}
			sort.Strings(names)
			obj["mcp"].(map[string]any)["env_names"] = names
			raw, err := json.Marshal(obj)
			if err != nil {
				t.Fatal(err)
			}
			h.fix.resolver.stdout = string(raw) + "\n"
			original := h.fix.deps.environ
			h.fix.deps.environ = func() []string { return append(original(), extra...) }
			code, _, stderr := h.run(t, []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"})
			if count == 64 {
				if code != 0 {
					t.Fatalf("exit = %d, want 0 (stderr %q)", code, stderr)
				}
				if got := len(h.payload(t)["env_names"].([]any)); got != 64 {
					t.Fatalf("payload env_names = %d, want 64", got)
				}
				if h.lookupCalls != 1 {
					t.Fatalf("lookup calls = %d, want one contact", h.lookupCalls)
				}
				return
			}
			if code != 2 {
				t.Fatalf("exit = %d, want 2 (stderr %q)", code, stderr)
			}
			if got := lastDiagnostic(stderr); !strings.HasPrefix(got, "curator-run: launch_plan_invalid: env_names:") {
				t.Fatalf("diagnostic = %q, want the env_names refusal", got)
			}
			// The export-time refusal holds the terminal preopened
			// before the build; the receiver is never looked up.
			if h.lookupCalls != 0 || h.terminalCalls != 1 {
				t.Fatalf("lookup calls = %d, terminal calls = %d; want zero lookup and the preopened terminal", h.lookupCalls, h.terminalCalls)
			}
		})
	}
}

// TestHostedResumePayload proves hosted resume inputs land in the payload:
// wrapper selectors elevate to typed intent, the process argv excludes
// original selectors, and native mode keeps the tail verbatim.
func TestHostedResumePayload(t *testing.T) {
	uuid := "01234567-89ab-cdef-0123-456789abcdef"
	for _, tc := range []struct {
		name         string
		args         []string
		wantKind     string
		wantIdentity any
	}{
		{"wrapper-latest", []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "resume"}, "latest", nil},
		{"wrapper-handle", []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "resume", "SES-42"}, "handle", "SES-42"},
		{"wrapper-uuid", []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--resume", uuid}, "claude_uuid", uuid},
		{"tail-uuid", []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--", "--resume", uuid, "--model", "tail"}, "claude_uuid", uuid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHostedHarness(t, "claude_code", false)
			code, _, stderr := h.run(t, tc.args)
			if code != 0 {
				t.Fatalf("exit = %d (stderr %q)", code, stderr)
			}
			payload := h.payload(t)
			resume := payload["resume"].(map[string]any)
			if resume["kind"] != tc.wantKind || resume["identity"] != tc.wantIdentity {
				t.Fatalf("resume = %v, want %s/%v", resume, tc.wantKind, tc.wantIdentity)
			}
			process := payload["process"].(map[string]any)
			var argv []string
			for _, arg := range process["argv"].([]any) {
				argv = append(argv, arg.(string))
			}
			for _, token := range argv {
				if token == "--resume" || token == uuid || token == "SES-42" {
					t.Fatalf("process argv %q retains a resume selector", argv)
				}
			}
			if tc.name == "tail-uuid" && !contains(argv, "--model") {
				t.Fatalf("process argv %q lost the non-selector tail", argv)
			}
		})
	}
	t.Run("native-keeps-tail-verbatim", func(t *testing.T) {
		h := newHostedHarness(t, "claude_code", false)
		code, out, stderr := h.run(t, []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--", "--resume", uuid})
		if code != 0 {
			t.Fatalf("exit = %d (stderr %q)", code, stderr)
		}
		doc := parseLaunchDocument(t, out)
		if !contains(doc.Argv, "--resume") || !contains(doc.Argv, uuid) {
			t.Fatalf("native argv %q lost the tail selector", doc.Argv)
		}
		if h.lookupCalls != 0 {
			t.Fatalf("lookup calls = %d", h.lookupCalls)
		}
	})
}

// TestHostedSuccessEndToEnd proves the full hosted handoff: the receiver
// gets the private argv, the terminal, and the payload; permission sources
// project; names resolve; and receiver refusals map through the entry point.
func TestHostedSuccessEndToEnd(t *testing.T) {
	t.Run("private-contract-and-names", func(t *testing.T) {
		h := newHostedHarness(t, "claude_code", false)
		code, _, stderr := h.run(t, []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"})
		if code != 0 {
			t.Fatalf("exit = %d (stderr %q)", code, stderr)
		}
		var argv []string
		if err := json.Unmarshal(h.capture(t, "argv.json"), &argv); err != nil {
			t.Fatal(err)
		}
		want := []string{"session", "launch-plan", "--plan", "-", "--terminal-fd", "3", "--status-fd", "4"}
		if !reflect.DeepEqual(argv, want) {
			t.Fatalf("receiver argv = %q", argv)
		}
		if got := string(h.capture(t, "fd3.txt")); got != "entry-terminal" {
			t.Fatalf("fd3 = %q", got)
		}
		payload := h.payload(t)
		session := payload["session_name"].(map[string]any)
		if session["host"] != "claude_code-20260916T010203Z" {
			t.Fatalf("default host name = %v", session["host"])
		}
		if h.lookupCalls != 1 || h.terminalCalls != 1 {
			t.Fatalf("lookup calls = %d, terminal calls = %d; want one contact each", h.lookupCalls, h.terminalCalls)
		}
	})
	t.Run("explicit-name", func(t *testing.T) {
		h := newHostedHarness(t, "claude_code", false)
		code, _, stderr := h.run(t, []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--name", "myhost"})
		if code != 0 {
			t.Fatalf("exit = %d (stderr %q)", code, stderr)
		}
		if got := h.payload(t)["session_name"].(map[string]any)["host"]; got != "myhost" {
			t.Fatalf("host name = %v", got)
		}
	})
	// Q-D3 (2026-10-05), literal: hosted permission resolution is
	// flag > profile > launcher-global > built-in, and the built-in
	// default is yolo on every stdio shape, exactly as on the native
	// path. Hosted yolo exports with execution_profile yolo while
	// permission_mode stays native per the frozen 1.0.0 const.
	t.Run("permission-sources", func(t *testing.T) {
		v2 := func(t *testing.T, h *hostedHarness) { h.fix.usePermissionsFragment(t, "native", false, "default") }
		interactive := func(t *testing.T, h *hostedHarness) {
			v2(t, h)
			h.fix.deps.isTerminal = func() bool { return true }
		}
		headlessSignal := func(t *testing.T, h *hostedHarness) {
			v2(t, h)
			h.fix.deps.isTerminal = func() bool { return false }
		}
		for _, tc := range []struct {
			name        string
			setup       func(t *testing.T, h *hostedHarness)
			args        []string
			wantSource  string
			wantMode    agentic.PermissionMode
			wantProfile string
		}{
			{"flag", v2, []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"}, "flag", agentic.PermissionModeNative, "standard"},
			{
				"profile",
				func(t *testing.T, h *hostedHarness) { h.fix.usePermissionsFragment(t, "native", false, "profile") },
				[]string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium"}, "profile", agentic.PermissionModeNative, "standard",
			},
			{
				"global",
				func(t *testing.T, h *hostedHarness) {
					v2(t, h)
					writeDefaults(t, h.fix.deps.defaults.Operator, `{"schema":"curator-run-defaults-v2","defaults":{"claude_code":{"permissions":"native"}}}`)
				},
				[]string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium"}, "global", agentic.PermissionModeNative, "standard",
			},
			{"default-interactive-yolo", interactive, []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium"}, "default-interactive", agentic.PermissionModeYolo, "yolo"},
			{"default-headless-signal-yolo", headlessSignal, []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium"}, "default-interactive", agentic.PermissionModeYolo, "yolo"},
			{"flag-yolo", v2, []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "yolo"}, "flag", agentic.PermissionModeYolo, "yolo"},
			{"yolo-alias", v2, []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--yolo"}, "flag", agentic.PermissionModeYolo, "yolo"},
			{
				"profile-yolo",
				func(t *testing.T, h *hostedHarness) { h.fix.usePermissionsFragment(t, "yolo", false, "profile") },
				[]string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium"}, "profile", agentic.PermissionModeYolo, "yolo",
			},
			{
				"global-yolo",
				func(t *testing.T, h *hostedHarness) {
					v2(t, h)
					writeDefaults(t, h.fix.deps.defaults.Operator, `{"schema":"curator-run-defaults-v2","defaults":{"claude_code":{"permissions":"yolo"}}}`)
				},
				[]string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium"}, "global", agentic.PermissionModeYolo, "yolo",
			},
			{
				"flag-beats-profile-and-global",
				func(t *testing.T, h *hostedHarness) {
					h.fix.usePermissionsFragment(t, "yolo", false, "profile")
					writeDefaults(t, h.fix.deps.defaults.Operator, `{"schema":"curator-run-defaults-v2","defaults":{"claude_code":{"permissions":"yolo"}}}`)
				},
				[]string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"}, "flag", agentic.PermissionModeNative, "standard",
			},
			{
				"profile-beats-global",
				func(t *testing.T, h *hostedHarness) {
					h.fix.usePermissionsFragment(t, "yolo", false, "profile")
					writeDefaults(t, h.fix.deps.defaults.Operator, `{"schema":"curator-run-defaults-v2","defaults":{"claude_code":{"permissions":"native"}}}`)
				},
				[]string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium"}, "profile", agentic.PermissionModeYolo, "yolo",
			},
			{
				"global-beats-interactive-default",
				func(t *testing.T, h *hostedHarness) {
					interactive(t, h)
					writeDefaults(t, h.fix.deps.defaults.Operator, `{"schema":"curator-run-defaults-v2","defaults":{"claude_code":{"permissions":"native"}}}`)
				},
				[]string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium"}, "global", agentic.PermissionModeNative, "standard",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				h := newHostedHarness(t, "claude_code", false)
				if tc.setup != nil {
					tc.setup(t, h)
				}
				code, _, stderr := h.run(t, tc.args)
				if code != 0 {
					t.Fatalf("exit = %d (stderr %q)", code, stderr)
				}
				// The resolved mode reaches both the spawn request and
				// the exported payload policy.
				if h.fix.request.PermissionMode != tc.wantMode {
					t.Fatalf("spawn request mode = %q, want %q", h.fix.request.PermissionMode, tc.wantMode)
				}
				policy := h.payload(t)["policy"].(map[string]any)
				if policy["permission_mode"] != "native" || policy["permission_source"] != tc.wantSource || policy["execution_profile"] != tc.wantProfile {
					t.Fatalf("policy = %v, want native/%s/%s", policy, tc.wantSource, tc.wantProfile)
				}
			})
		}
	})
	t.Run("receiver-refusal-maps", func(t *testing.T) {
		h := newHostedHarness(t, "claude_code", false)
		h.receiverRecords = []string{`{"schema":"urn:relux:task-board:session-launch-status","schema_version":"1.0.0","type":"refusal","code":"session_host_unavailable","message":"Session host is unavailable.","details":{}}`}
		h.receiverExit = "2"
		code, _, stderr := h.run(t, []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"})
		if code != 1 {
			t.Fatalf("exit = %d, want 1 (stderr %q)", code, stderr)
		}
		if got := lastDiagnostic(stderr); got != "curator-run: session_host_unavailable: Session host is unavailable." {
			t.Fatalf("diagnostic = %q", got)
		}
		// The payload still reached the receiver before the refusal.
		if len(h.capture(t, "stdin.bin")) == 0 {
			t.Fatal("receiver captured no payload")
		}
	})
	// A non-contract refusal record normalizes to protocol_error/6 with
	// the constant message: unknown codes, mismatched messages, and bad
	// details never reflect receiver bytes to the operator.
	t.Run("receiver-noncontract-maps", func(t *testing.T) {
		const prefix = `{"schema":"urn:relux:task-board:session-launch-status","schema_version":"1.0.0","type":"refusal",`
		for _, tc := range []struct {
			name   string
			record string
		}{
			{"unknown-code", prefix + `"code":"session_host_future_x","message":"CANARY-new","details":{}}`},
			{"wrong-message", prefix + `"code":"launch_plan_invalid","message":"CANARY-wrong","details":{}}`},
			{"bad-detail", prefix + `"code":"policy_refused","message":"Policy refused.","details":{"field":"CANARY-field"}}`},
			{"missing-envelope", `{"ses":"SES-1"}`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				h := newHostedHarness(t, "claude_code", false)
				h.receiverRecords = []string{tc.record}
				h.receiverExit = "0"
				code, _, stderr := h.run(t, []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"})
				if code != 6 {
					t.Fatalf("exit = %d, want 6 (stderr %q)", code, stderr)
				}
				if got := lastDiagnostic(stderr); got != "curator-run: session_host_protocol_error: Invalid session host response." {
					t.Fatalf("diagnostic = %q", got)
				}
				if strings.Contains(stderr, "CANARY") {
					t.Fatalf("stderr reflects receiver bytes: %q", stderr)
				}
			})
		}
	})
	t.Run("receiver-corrupt-maps", func(t *testing.T) {
		h := newHostedHarness(t, "claude_code", false)
		h.receiverCorrupt = "badjson"
		code, _, stderr := h.run(t, []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"})
		if code != 6 {
			t.Fatalf("exit = %d, want 6 (stderr %q)", code, stderr)
		}
		if got := lastDiagnostic(stderr); got != "curator-run: session_host_protocol_error: Invalid session host response." {
			t.Fatalf("diagnostic = %q", got)
		}
	})
	t.Run("fragment-record-pinned", func(t *testing.T) {
		h := newHostedHarness(t, "claude_code", false)
		code, _, stderr := h.run(t, []string{"claude_code", "--hosted", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--system-prompt", "append"})
		if code != 0 {
			t.Fatalf("exit = %d (stderr %q)", code, stderr)
		}
		payload := h.payload(t)
		record := payload["fragment"].(map[string]any)
		if record["profile_name"] != "default" || record["system_modules"] != true {
			t.Fatalf("fragment record = %v", record)
		}
		if record["pin"] != "sha256:"+strings.Repeat("a", 64) {
			t.Fatalf("pin = %v", record["pin"])
		}
		digest, _ := record["fragment_digest"].(string)
		if !strings.HasPrefix(digest, "sha256:") || len(digest) != 7+64 {
			t.Fatalf("fragment_digest = %q", digest)
		}
	})
}

// TestHostedConflictingNativeSessionNamesRefuse pins the module-owned
// refusal for an authored tail whose -n and --remote-control values name
// different sessions. The module records session metadata while it builds
// the plan, so the conflict refuses plan_refused before composition in both
// modes; hosted never reaches the receiver and never copies the values.
func TestHostedConflictingNativeSessionNamesRefuse(t *testing.T) {
	args := []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--", "-n", "SECRET-A", "--remote-control", "SECRET-B"}
	for _, mode := range []string{"", "--hosted"} {
		h := newHostedHarness(t, "claude_code", false)
		runArgs := args
		if mode != "" {
			runArgs = append([]string{args[0], mode}, args[1:]...)
		}
		code, _, stderr := h.run(t, runArgs)
		if code != diagnostics.ExitForCode(diagnostics.CodePlanRefused) {
			t.Fatalf("mode %q: exit = %d (stderr %q)", mode, code, stderr)
		}
		if got := lastDiagnostic(stderr); !strings.HasPrefix(got, "curator-run: plan_refused") && !strings.Contains(stderr, "conflicting native session names") {
			t.Fatalf("mode %q: diagnostic = %q", mode, stderr)
		}
		if mode != "" {
			if strings.Contains(stderr, "SECRET") {
				t.Fatalf("hosted refusal copied native argument values: %q", stderr)
			}
			// The controlling terminal is opened before the build (F8);
			// the receiver is never looked up.
			if h.lookupCalls != 0 {
				t.Fatalf("hosted refusal looked up the receiver: lookup=%d", h.lookupCalls)
			}
		}
	}
}
