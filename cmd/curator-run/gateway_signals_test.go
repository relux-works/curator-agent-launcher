package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-network-profiles/pkg/netprofile"
)

// R3-7: real SIGTERM, sent by the operating system to the built curator-run
// process at an exact gateway phase, with the fake gateway's reply held as
// the phase barrier:
//   - during health: the launch stops before registering, with no child;
//   - during register: registration completes, the boundary refuses, the
//     registration is ended, and no child starts;
//   - while the child runs (at activation): the signal reaches the child,
//     which ends by SIGTERM (exit 143), and the registration is ended;
//   - during unregister retries: the retries are abandoned with the
//     idle-bound alert, and the launcher exits.
//
// The launch with no signal is the control. The final settings recheck has no
// external barrier in the real binary; its cancellation is covered by
// TestLeaseOwnerCancelOnSignal and the network package's recheck tests.
func TestGatewayRealSignalPhases(t *testing.T) {
	bin := frontBuild(t)
	for _, tc := range []struct {
		name      string
		at        string
		wait      bool
		failUnreg bool
		code      int
		calls     string
		child     bool
		stderr    string
	}{
		{"control: no signal", "", false, false, 0, "/v1/health /v1/register /v1/activate /v1/unregister", true, ""},
		{"during health", "/v1/health", false, false, -1, "/v1/health", false, ""},
		{"during register", "/v1/register", false, false, -1, "/v1/health /v1/register /v1/unregister", false, "interrupted before the runtime started"},
		{"while the child runs", "/v1/activate", true, false, 128 + int(syscall.SIGTERM), "/v1/health /v1/register /v1/activate /v1/unregister", true, ""},
		{"during unregister retries", "/v1/unregister", false, true, 0, "", true, "unregister abandoned"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFront(t, bin)
			g := newE2EGateway(t)
			writeFrontGateway(t, f.root, g.socket)
			if tc.wait {
				writeFixture(t, filepath.Join(f.root, "native-wait"), []byte("wait"), 0o600)
			}
			if tc.failUnreg {
				g.persistFailures = 1 << 20
			}
			cmd := exec.Command(bin.launcher, "claude_code", "--native", "--model", "claude-opus-5", "--effort", "medium",
				"--permissions", "native", "--network", "egress-a", "--", "-p", "hello")
			cmd.Dir, cmd.Env = f.root, f.env
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			var stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &bytes.Buffer{}, &stderr
			signalled := make(chan struct{})
			g.beforeReply = func(path string) {
				if path != tc.at {
					return
				}
				select {
				case <-signalled:
					return
				default:
					close(signalled)
				}
				if tc.wait {
					waitForFile(t, filepath.Join(f.root, "native-pid"))
				}
				_ = syscall.Kill(cmd.Process.Pid, syscall.SIGTERM)
				// Let the launcher take the signal before this call answers.
				time.Sleep(300 * time.Millisecond)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			var err error
			select {
			case err = <-done:
			case <-time.After(30 * time.Second):
				killOwned(f.root, cmd)
				<-done
				t.Fatalf("the launcher did not exit; stderr=%s", stderr.String())
			}
			killOwned(f.root, cmd)
			code := 0
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				code = exit.ExitCode()
			}
			if tc.code >= 0 && code != tc.code || tc.code < 0 && code == 0 {
				t.Fatalf("exit=%d stderr=%s", code, stderr.String())
			}
			calls := g.calls()
			if tc.calls != "" && calls != tc.calls {
				t.Fatalf("gateway calls = %q", calls)
			}
			if tc.failUnreg && !strings.Contains(calls, "/v1/unregister") {
				t.Fatalf("gateway calls = %q", calls)
			}
			if _, err := os.Stat(filepath.Join(f.root, "native-called")); (err == nil) != tc.child {
				t.Fatalf("child started = %v, want %v", err == nil, tc.child)
			}
			if tc.stderr != "" && !strings.Contains(stderr.String(), tc.stderr) {
				t.Fatalf("stderr = %s", stderr.String())
			}
			if tc.at != "" {
				select {
				case <-signalled:
				default:
					t.Fatal("the phase barrier was never reached")
				}
			}
		})
	}
}

// writeFrontGateway configures a front-door fixture for a gateway launch: a
// confirmed filtering-gateway catalog, the fake runtime as the qualified
// exec build, the fixture root as the only project root, and a read-only
// settings.json in the managed home.
func writeFrontGateway(t *testing.T, root, socket string) {
	t.Helper()
	dir := filepath.Join(root, ".curator")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	doc := "schema = \"relux-network-profiles-v1\"\n[networks.egress-a]\nkind = \"filtering-gateway\"\ncontrol = \"" + socket + "\"\nsensitive_egress = true\n"
	file, err := netprofile.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	prof, _ := file.Lookup("egress-a")
	ledger, _ := json.Marshal(map[string]any{"schema": "relux-network-confirmations-v1",
		"confirmed": map[string]any{"egress-a": map[string]any{"digest": netprofile.Digest(prof), "confirmed_at": "2026-10-30T00:00:00Z"}}})
	runtimeBytes, err := os.ReadFile(filepath.Join(root, "bin", "claude"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(runtimeBytes)
	qualified := "schema = \"curator-network-qualified-v1\"\nproject_roots = [\"" + root + "\"]\n[[builds]]\nharness = \"claude-code\"\nentrypoint = \"exec\"\nbinary_sha256 = \"" + hex.EncodeToString(sum[:]) +
		"\"\nplatform = \"" + runtime.GOOS + "\"\nevidence = \"test fixture\"\nqualified_on = \"2026-10-30\"\n"
	for name, data := range map[string]string{"network.toml": doc, "network.confirmations.json": string(ledger), "network-qualified.toml": qualified} {
		writeFixture(t, filepath.Join(dir, name), []byte(data), 0o600)
	}
	writeFixture(t, filepath.Join(root, "managed", "settings.json"), []byte("{}"), 0o400)
}

func waitForFile(t *testing.T, path string) {
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(path); err == nil {
			return
		}
	}
	t.Errorf("%s never appeared", path)
}

// killOwned ends what this test started and only that: the launcher's
// session and the waiting runtime by the PID it recorded.
func killOwned(root string, cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	if raw, err := os.ReadFile(filepath.Join(root, "native-pid")); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && pid > 1 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}
