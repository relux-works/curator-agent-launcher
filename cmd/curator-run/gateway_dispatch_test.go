package main

import (
	"bytes"
	"encoding/json"
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

// R5-1 through the built launcher: a native tail whose prompt value is
// --safe-mode, which the runtime's mode getter finds in the raw argv before
// any "--" (an inert stand-in in the fake runtime records it), is refused
// before any gateway call, so the runtime never starts. The same token after
// "--" is outside the getter: the stand-in does not fire on it when run
// directly, and the launcher admits it (the control).
func TestGatewayModePredicateThroughRun(t *testing.T) {
	bin := frontBuild(t)
	for _, tc := range []struct {
		name  string
		tail  []string
		admit bool
	}{
		{"--safe-mode as a required value", []string{"-p", "--append-system-prompt", "--safe-mode", "hello"}, false},
		{"control: --safe-mode after --", []string{"-p", "hello", "--", "--safe-mode"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFront(t, bin)
			witness := exec.Command(filepath.Join(f.root, "bin", "claude"), tc.tail...)
			witness.Dir = t.TempDir()
			if err := witness.Run(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(witness.Dir, "safe-mode")); (err == nil) == tc.admit {
				t.Fatalf("the stand-in mode getter fired = %v on this tail when run directly", err == nil)
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
			_, selected := os.Stat(filepath.Join(f.root, "safe-mode"))
			if tc.admit {
				if err != nil || g.calls() != "/v1/health /v1/register /v1/activate /v1/unregister" || selected == nil {
					t.Fatalf("err=%v calls=%q selected=%v stderr=%s", err, g.calls(), selected == nil, stderr.String())
				}
				return
			}
			if err == nil || !strings.Contains(stderr.String(), "network_configuration_conflict") || g.calls() != "" || selected == nil {
				t.Fatalf("err=%v calls=%q selected=%v stderr=%s", err, g.calls(), selected == nil, stderr.String())
			}
			if _, err := os.Stat(filepath.Join(f.root, "native-called")); err == nil {
				t.Fatal("the runtime started")
			}
		})
	}
}

// R6-1 through the built launcher: the coordinator-mode selector, which an
// inert stand-in in the fake runtime shows would take effect on this print
// launch, never reaches the runtime. Inherited from the launcher's own
// environment it is removed and the launch is admitted with the full
// lifecycle and no coordinator mode; set by the profile fragment it refuses
// before any gateway call (here the closed fragment schema already rejects
// the member, so the gateway's own overlay refusal is the network package's
// TestPrepareGatewayNamespaceAllowlist). The stand-in firing when the runtime is run
// directly with the variable is the witness that the case is real.
func TestGatewayCoordinatorModeNeverReachesTheRuntime(t *testing.T) {
	bin := frontBuild(t)
	for _, tc := range []struct {
		name     string
		fragment bool
		admit    bool
	}{
		{"inherited: removed", false, true},
		{"from the fragment: refused", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFront(t, bin)
			witness := exec.Command(filepath.Join(f.root, "bin", "claude"), "-p", "hello")
			witness.Dir = t.TempDir()
			witness.Env = append(os.Environ(), "CLAUDE_CODE_COORDINATOR_MODE=1")
			if err := witness.Run(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(witness.Dir, "coordinator")); err != nil {
				t.Fatal("the stand-in coordinator getter does not fire when run directly")
			}
			g := newE2EGateway(t)
			writeFrontGateway(t, f.root, g.socket)
			env := append([]string{}, f.env...)
			if tc.fragment {
				raw, err := os.ReadFile(filepath.Join(f.root, "fragment.json"))
				if err != nil {
					t.Fatal(err)
				}
				var doc map[string]any
				if err := json.Unmarshal(raw, &doc); err != nil {
					t.Fatal(err)
				}
				doc["env"].(map[string]any)["CLAUDE_CODE_COORDINATOR_MODE"] = "1"
				out, _ := json.Marshal(doc)
				writeFixture(t, filepath.Join(f.root, "fragment.json"), out, 0o600)
			} else {
				env = append(env, "CLAUDE_CODE_COORDINATOR_MODE=1")
			}
			cmd := exec.Command(bin.launcher, "claude_code", "--native", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--network", "egress-a", "--", "-p", "hello")
			cmd.Dir, cmd.Env = f.root, env
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
			if _, statErr := os.Stat(filepath.Join(f.root, "coordinator")); statErr == nil {
				t.Fatal("the runtime started in coordinator mode")
			}
			if tc.admit {
				if err != nil || g.calls() != "/v1/health /v1/register /v1/activate /v1/unregister" {
					t.Fatalf("err=%v calls=%q stderr=%s", err, g.calls(), stderr.String())
				}
				return
			}
			if err == nil || !strings.Contains(stderr.String(), "resolve_fragment_invalid") || g.calls() != "" {
				t.Fatalf("err=%v calls=%q stderr=%s", err, g.calls(), stderr.String())
			}
			if _, err := os.Stat(filepath.Join(f.root, "native-called")); err == nil {
				t.Fatal("the runtime started")
			}
		})
	}
}
