//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	"golang.org/x/sys/unix"
)

func terminalBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "terminal-plugin")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binary, "./testdata/terminal")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 100 * time.Millisecond
	out, err := cmd.CombinedOutput()
	frontGone(t, cmd)
	if err != nil {
		t.Fatalf("build terminal fixture: %v %s", err, out)
	}
	return binary
}

// Both terminal operations retain redirected file and wrapped destinations.
// Shadow observation forwards bytes but never classifies provider stderr; no
// session bytes may reach the independent TTY. Every child is bounded/reaped.
func TestTerminalStderrDestination(t *testing.T) {
	binary := terminalBinary(t)
	for _, op := range []string{"attach", "launch-exact"} {
		for _, kind := range []string{"file", "wrapped", "shadow-file", "shadow-wrapped"} {
			t.Run(op+"/"+kind, func(t *testing.T) {
				root := t.TempDir()
				master, tty := frontPTY(t)
				dst, err := os.Create(filepath.Join(root, "stderr"))
				if err != nil {
					t.Fatal(err)
				}
				defer dst.Close()
				null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer null.Close()
				var writer io.Writer = dst
				if kind == "wrapped" || kind == "shadow-wrapped" {
					writer = struct{ io.Writer }{dst}
				}
				var observer *selectorOutcomeWriter
				if kind == "shadow-file" || kind == "shadow-wrapped" {
					observer = &selectorOutcomeWriter{Writer: writer}
					writer = observer
				}
				t.Cleanup(func() {
					data, err := os.ReadFile(filepath.Join(root, "terminal.pid"))
					if err != nil {
						t.Error("missing terminal child PID", err)
						return
					}
					pid, err := strconv.Atoi(string(data))
					if err != nil || pid <= 0 {
						t.Error("invalid child PID")
						return
					}
					if !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
						_ = syscall.Kill(pid, syscall.SIGKILL)
					}
					assertSelectorGone(t, pid)
				})
				c := client.Client{ID: "test", Entry: sessionmanager.Entry{Executable: binary, Protocol: client.Protocol}, CWD: root, Environment: []string{"HOME=" + root}}
				request := []byte(`{"session":"SES-example","expected_generation":1}`)
				if op == "launch-exact" {
					request = []byte(`{"private_plan":true}`)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				result, err := c.Terminal(ctx, op, request, terminalStreams(tty, null, writer, null))
				if err != nil || result.Exit != 7 || result.Refusal != nil {
					t.Fatalf("terminal classification: %+v %v", result, err)
				}
				captured, err := os.ReadFile(dst.Name())
				if err != nil {
					t.Fatal(err)
				}
				expected := []byte("SESSION_STDERR\ncurator-run: policy_refused: provider text\n")
				if !bytes.Equal(captured, expected) {
					t.Error("session stderr bypassed redirected destination")
				}
				if err = unix.SetNonblock(int(master.Fd()), true); err != nil {
					t.Fatal(err)
				}
				var buf [256]byte
				n, err := unix.Read(int(master.Fd()), buf[:])
				if n > 0 {
					t.Error("session bytes reached TTY despite redirected stderr")
				}
				if err != nil && !errors.Is(err, unix.EAGAIN) {
					t.Fatal(err)
				}
				if observer != nil && (observer.code != "" || len(observer.pending) != 0) {
					t.Fatal("provider stderr entered launcher diagnostic observation")
				}
			})
		}
	}
}
