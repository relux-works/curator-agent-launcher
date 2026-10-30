package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// R4-1 through the built launcher: a native tail that would reach Claude
// Code's early deep-link dispatcher (here an inert stand-in in the fake
// runtime, which records when it would dispatch) is refused before any
// gateway call, so the runtime never starts. The stand-in firing on the same
// tail when the runtime is run directly is the witness that the case is
// real; a tail that only contains the word inside a prompt is admitted (the
// control).
func TestGatewayEarlyDispatchRefusedThroughRun(t *testing.T) {
	bin := frontBuild(t)
	for _, tc := range []struct {
		name  string
		tail  []string
		admit bool
	}{
		{"as a required value", []string{"-p", "--append-system-prompt", "--handle-uri", "claude-cli://open?cwd=/tmp"}, false},
		{"after --", []string{"-p", "--", "--handle-uri", "claude-cli://open?cwd=/tmp"}, false},
		{"control: inside a prompt", []string{"-p", "explain --handle-uri"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFront(t, bin)
			if !tc.admit {
				witness := exec.Command(filepath.Join(f.root, "bin", "claude"), tc.tail...)
				witness.Dir = t.TempDir()
				if err := witness.Run(); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(witness.Dir, "early-dispatch")); err != nil {
					t.Fatal("the stand-in dispatcher does not fire on this tail when run directly")
				}
			}
			g := newE2EGateway(t)
			writeFrontGateway(t, f.root, g.socket)
			args := append([]string{"claude_code", "--native", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--network", "egress-a", "--"}, tc.tail...)
			cmd := exec.Command(bin.launcher, args...)
			cmd.Dir, cmd.Env = f.root, f.env
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			var stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &bytes.Buffer{}, &stderr
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
				t.Fatal("the launcher did not exit")
			}
			_, dispatched := os.Stat(filepath.Join(f.root, "early-dispatch"))
			if tc.admit {
				if err != nil || g.calls() != "/v1/health /v1/register /v1/activate /v1/unregister" || dispatched == nil {
					t.Fatalf("err=%v calls=%q dispatched=%v stderr=%s", err, g.calls(), dispatched == nil, stderr.String())
				}
				return
			}
			if err == nil || !strings.Contains(stderr.String(), "network_configuration_conflict") || g.calls() != "" || dispatched == nil {
				t.Fatalf("err=%v calls=%q dispatched=%v stderr=%s", err, g.calls(), dispatched == nil, stderr.String())
			}
			if _, err := os.Stat(filepath.Join(f.root, "native-called")); err == nil {
				t.Fatal("the runtime started")
			}
		})
	}
}
