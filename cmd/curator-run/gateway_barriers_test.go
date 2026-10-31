package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// R4-5: real SIGTERM, sent by the operating system to a curator-run built
// with the gatewayphases tag, while the launcher waits at an exact phase of
// its own code (the barrier is a no-op in a normal build):
//   - during the final settings recheck: the check stops as interrupted (the
//     recheck's own signal watch, not the boundary's later poll), no child
//     starts, and the registration is ended;
//   - right after the child starts: the signal reaches the child, which ends
//     by SIGTERM (exit 143), and the registration is ended;
//   - after the child exits, before the registration is ended: the signal is
//     drained and unregister still completes, with the child's exit code;
//   - in the cleanup after a boundary refusal (a route written into project
//     settings while the recheck is held): unregister still completes, no
//     child starts.
//
// The same build with every barrier released and no signal is the control.
// Each case checks that no owned child survives before any fallback kill.
// The cleanup of a refusal between preparation and execution (a deferred
// finish) has no externally triggerable refusal and is not covered here.
func TestGatewayRealSignalBarriers(t *testing.T) {
	bin := frontBuild(t)
	tagged := filepath.Join(t.TempDir(), "curator-run-phases")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	build := exec.CommandContext(ctx, "go", "build", "-tags", "gatewayphases", "-o", tagged,
		"-ldflags", "-X main.systemConfigDirectory="+bin.system+" -X main.legacySystemConfigDirectory="+filepath.Join(filepath.Dir(bin.system), "legacy-system"), ".")
	out, err := build.CombinedOutput()
	cancel()
	if err != nil {
		t.Fatalf("tagged build: %v %s", err, out)
	}
	bin.launcher = tagged
	for _, tc := range []struct {
		name     string
		phase    string
		wait     bool
		late     bool
		code     int
		calls    string
		child    bool
		stderr   string
		noStderr string
	}{
		{"control: barriers released, no signal", "", false, false, 0, "/v1/health /v1/register /v1/activate /v1/unregister", true, "", ""},
		{"during the recheck", "recheck", false, false, -1, "/v1/health /v1/register /v1/unregister", false, "settings check was interrupted", ""},
		{"right after the start", "started", true, false, 128 + int(syscall.SIGTERM), "/v1/health /v1/register /v1/activate /v1/unregister", true, "", ""},
		{"after the child exits", "exited", false, false, 0, "/v1/health /v1/register /v1/activate /v1/unregister", true, "", "abandoned"},
		{"cleanup after a boundary refusal", "exited", false, true, -1, "/v1/health /v1/register /v1/unregister", false, "network_configuration_conflict", "abandoned"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFront(t, bin)
			g := newE2EGateway(t)
			writeFrontGateway(t, f.root, g.socket)
			if tc.wait {
				writeFixture(t, filepath.Join(f.root, "native-wait"), []byte("wait"), 0o600)
			}
			phases := t.TempDir()
			cmd := exec.Command(bin.launcher, "claude_code", "--native", "--model", "claude-opus-5", "--effort", "medium",
				"--permissions", "native", "--network", "egress-a", "--", "-p", "hello")
			cmd.Dir, cmd.Env = f.root, append(append([]string{}, f.env...), "CURATOR_TEST_PHASE_DIR="+phases)
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			var stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &bytes.Buffer{}, &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			release := func(name string) { writeFixture(t, filepath.Join(phases, name+".release"), nil, 0o600) }
			if tc.late {
				// Hold the recheck, make the project settings route
				// elsewhere, and let the recheck refuse.
				waitForFile(t, filepath.Join(phases, "recheck.reached"))
				os.MkdirAll(filepath.Join(f.root, ".claude"), 0o700)
				writeFixture(t, filepath.Join(f.root, ".claude", "settings.local.json"), []byte(`{"env":{"ANTHROPIC_BASE_URL":"https://late.example"}}`), 0o600)
				release("recheck")
			} else {
				// Release every phase before the one under test.
				for _, p := range []string{"recheck", "started"} {
					if p == tc.phase || tc.phase == "" {
						break
					}
					release(p)
				}
			}
			if tc.phase != "" {
				waitForFile(t, filepath.Join(phases, tc.phase+".reached"))
				if tc.wait {
					waitForFile(t, filepath.Join(f.root, "native-pid"))
				}
				_ = syscall.Kill(cmd.Process.Pid, syscall.SIGTERM)
				time.Sleep(300 * time.Millisecond)
			}
			for _, p := range []string{"recheck", "started", "exited", "cleanup"} {
				release(p)
			}
			var runErr error
			select {
			case runErr = <-done:
			case <-time.After(30 * time.Second):
				killOwned(f.root, cmd)
				<-done
				t.Fatalf("the launcher did not exit; stderr=%s", stderr.String())
			}
			if raw, err := os.ReadFile(filepath.Join(f.root, "native-pid")); err == nil {
				pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
				if pid > 1 && !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
					killOwned(f.root, cmd)
					t.Fatalf("the child %d survived the launcher", pid)
				}
			}
			killOwned(f.root, cmd)
			code := 0
			var exit *exec.ExitError
			if errors.As(runErr, &exit) {
				code = exit.ExitCode()
			}
			if tc.code >= 0 && code != tc.code || tc.code < 0 && code == 0 {
				t.Fatalf("exit=%d stderr=%s", code, stderr.String())
			}
			if calls := g.calls(); calls != tc.calls {
				t.Fatalf("gateway calls = %q stderr=%s", calls, stderr.String())
			}
			if _, err := os.Stat(filepath.Join(f.root, "native-called")); (err == nil) != tc.child {
				t.Fatalf("child started = %v, want %v", err == nil, tc.child)
			}
			if tc.stderr != "" && !strings.Contains(stderr.String(), tc.stderr) {
				t.Fatalf("stderr = %s", stderr.String())
			}
			if tc.noStderr != "" && strings.Contains(stderr.String(), tc.noStderr) {
				t.Fatalf("stderr = %s", stderr.String())
			}
		})
	}
}
