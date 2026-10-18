package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/skill-agents-management/pkg/agentic"
	claudeSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/claude"
)

// Native controls retain the legacy ax behavior and prove no terminal probe.
// Binary front-door tests separately assert zero registry reads and RPC calls.
type hostedHarness struct {
	fix                 *pipelineFixture
	terminalCalls       int
	captureDir, ttyPath string
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
	return h
}

func (h *hostedHarness) run(t *testing.T, args []string) (int, string, string) {
	t.Helper()
	h.fix.args = args
	code, out, stderr := h.fix.run()
	return code, string(out), string(stderr)
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
	if h.terminalCalls != 0 {
		t.Fatalf("terminal calls = %d; want zero", h.terminalCalls)
	}
}

func TestNativeDoesNotOpenHostedTerminal(t *testing.T) {
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
			name: "operator-permission-default", tracked: false,
			setup: func(t *testing.T, h *hostedHarness) {
				writeDefaults(t, h.fix.deps.defaults.Operator, `{"schema":"curator-run-defaults-v3","defaults":{"claude_code":{"permissions":"native"}}}`)
			},
			args: []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"},
		},
		{
			name: "locked-ignores-operator-permissions", tracked: false,
			setup: func(t *testing.T, h *hostedHarness) {
				writeDefaults(t, h.fix.deps.defaults.Machine, `{"schema":"curator-run-defaults-v3","locked":true,"defaults":{"claude_code":{"model":"claude-opus-5","effort":"medium"}}}`)
				writeDefaults(t, h.fix.deps.defaults.Operator, `{"schema":"curator-run-defaults-v3","defaults":{"claude_code":{"permissions":"native"}}}`)
			},
			args: []string{"claude_code", "--permissions", "native"},
		},
		{"tracked-implicit-native", true, nil, []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native"}},
		{
			name: "tracked-configured-permissions", tracked: true,
			setup: func(t *testing.T, h *hostedHarness) {
				writeDefaults(t, h.fix.deps.defaults.Operator, `{"schema":"curator-run-defaults-v3","defaults":{"claude_code":{"permissions":"native"}}}`)
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
			if h.terminalCalls != 0 {
				t.Fatalf("terminal calls = %d; native must not probe", h.terminalCalls)
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
			name: "configured-permissions-keeps-ax",
			setup: func(t *testing.T, h *hostedHarness) {
				writeDefaults(t, h.fix.deps.defaults.Operator, `{"schema":"curator-run-defaults-v3","defaults":{"claude_code":{"permissions":"native"}}}`)
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
			if h.terminalCalls != 0 {
				t.Fatalf("terminal calls = %d; want zero", h.terminalCalls)
			}
		})
	}
}
