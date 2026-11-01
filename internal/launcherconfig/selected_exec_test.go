package launcherconfig

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The actual subprocess boundary accepts one complete projection and refuses
// partial/nonzero, oversized and malformed output without exposing its stderr.
func TestSelectedCoreCommand(t *testing.T) {
	for _, mode := range []string{"valid", "failed", "oversized", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestSelectedCoreCommandHelper$")
			cmd.Env = append(os.Environ(), "CURATOR_SELECTED_HELPER="+mode)
			got, err := readSelectedCommand(cmd)
			if mode == "valid" {
				if err != nil || got.ConfigSchema != 7 || got.Proxy == nil || got.Proxy.Profile != "filtered" {
					t.Fatalf("valid child projection: %+v %v", got, err)
				}
			} else if err == nil || got != (Selected{}) || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatalf("unsafe subprocess refusal: %+v %v", got, err)
			}
		})
	}
}

func TestSelectedCoreCommandHelper(t *testing.T) {
	mode := os.Getenv("CURATOR_SELECTED_HELPER")
	if mode == "" {
		return
	}
	if mode == "held-pipe" {
		dir := os.Getenv("CURATOR_SELECTED_PIPE_DIR")
		until := time.Now().Add(3 * time.Second)
		for time.Now().Before(until) {
			if _, err := os.Stat(filepath.Join(dir, "release")); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		_ = os.WriteFile(filepath.Join(dir, "done"), []byte("done"), 0600)
		os.Exit(0)
	}
	if mode == "retained-pipe" {
		child := exec.Command(os.Args[0], "-test.run=^TestSelectedCoreCommandHelper$")
		child.Env = append(os.Environ(), "CURATOR_SELECTED_HELPER=held-pipe")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(3)
		}
		if err := os.WriteFile(filepath.Join(os.Getenv("CURATOR_SELECTED_PIPE_DIR"), "pid"), []byte(strconv.Itoa(child.Process.Pid)), 0600); err != nil {
			os.Exit(4)
		}
	}
	if mode == "malformed" {
		fmt.Print(`{"PRIVATE_KEY":"PRIVATE_VALUE"}`)
	} else {
		fmt.Print(`{"schema":"curator.launcher-config/1","config_schema":7,"config_path":"selected/config.json","extensions":{"proxy":{"profile":"filtered"}}}`)
		if mode == "oversized" {
			fmt.Print(strings.Repeat(" ", selectedOutputLimit))
		}
	}
	fmt.Fprint(os.Stderr, "PRIVATE_BACKEND_DETAIL")
	if mode == "failed" {
		os.Exit(1)
	}
	os.Exit(0)
}

// A complete projection does not qualify a subprocess whose inherited output
// pipes outlast the drain bound. The query refuses within that bound; the owned
// fixture descendant exits cooperatively. This does not assert tree-wide kill.
func TestSelectedCoreRetainedPipeRefuses(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestSelectedCoreCommandHelper$")
	cmd.Env = append(os.Environ(), "CURATOR_SELECTED_HELPER=retained-pipe", "CURATOR_SELECTED_PIPE_DIR="+dir)
	defer func() {
		_ = os.WriteFile(filepath.Join(dir, "release"), []byte("release"), 0600)
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(filepath.Join(dir, "done")); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Error("fixture descendant did not report cooperative exit")
	}()
	start := time.Now()
	got, err := readSelectedCommand(cmd)
	elapsed := time.Since(start)
	pid, _ := os.ReadFile(filepath.Join(dir, "pid"))
	t.Logf("original=%d descendant=%s elapsed=%s", cmd.Process.Pid, pid, elapsed)
	if err == nil || got != (Selected{}) {
		t.Errorf("retained pipe admitted a projection: %+v %v", got, err)
	}
	if elapsed > 1800*time.Millisecond {
		t.Errorf("pipe drain exceeded bound: %s", elapsed)
	}
}
