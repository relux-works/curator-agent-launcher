package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
)

// Administrative commands use the shared config resolver without resolving a
// fragment or reading unrelated invalid launch policy; diagnostics retain exits.
func TestRunSessionManagerCommands(t *testing.T) {
	deps, runner, loc := configHomeFixture(t)
	deps.managerService = &sessionmanager.Service{CanWriteSystem: func(string) bool { return false }}
	writeConfig(t, loc.System, "launcher.json", "invalid")
	writeConfig(t, loc.User, "ax.json", "invalid")
	executable := filepath.Join(t.TempDir(), "manager")
	os.WriteFile(executable, []byte("never executed"), 0700)
	manifest, _ := json.Marshal(map[string]any{"id": "board", "executable": executable, "argv_prefix": []string{"plugin"}, "protocol": sessionmanager.Protocol, "registered_by": "test installer"})
	deps.stdin = strings.NewReader(string(manifest))
	deps.stdinTerminal = func() bool { return false }
	var out, stderr strings.Builder
	invoke := func(args ...string) int {
		t.Helper()
		out.Reset()
		stderr.Reset()
		return run(context.Background(), append([]string{"session-manager"}, args...), &out, &stderr, deps)
	}
	if code := invoke("register", "--manifest", "-"); code != 0 || runner.calls != 0 || stderr.Len() != 0 {
		t.Fatalf("%d %s", code, &stderr)
	}
	if code := invoke("list", "--json"); code != 0 || !strings.Contains(out.String(), `"selected":"board"`) {
		t.Fatalf("%d %s %s", code, &out, &stderr)
	}
	if code := invoke("show", "board", "--json"); code != 0 || !strings.Contains(out.String(), `"id":"board"`) {
		t.Fatalf("%d %s", code, &out)
	}
	if code := invoke("select", "unknown"); code != 2 || !strings.Contains(stderr.String(), "session_manager_not_registered") || out.Len() != 0 {
		t.Fatalf("%d %s", code, &stderr)
	}
	if code := invoke("register", "--manifest", "-", "--select", "--no-select"); code != 2 || !strings.Contains(stderr.String(), "usage") {
		t.Fatalf("%d %s", code, &stderr)
	}
	if code := invoke("unregister", "board"); code != 0 {
		t.Fatalf("%d %s", code, &stderr)
	}
	writeConfig(t, loc.User, sessionmanager.Filename, "{")
	if code := invoke("list"); code != 2 || !strings.Contains(stderr.String(), "session_manager_registry_invalid") || !strings.Contains(stderr.String(), `reason="parse"`) {
		t.Fatalf("%d %s", code, &stderr)
	}
	if runner.calls != 0 {
		t.Fatal("administrative operation resolved a fragment")
	}
}

// A malformed registry cannot affect ordinary launches before S4; a manager
// binary on PATH alone also cannot make an unknown administrative id registered.
func TestRunSessionManagerDoesNotWireLaunch(t *testing.T) {
	deps, runner, loc := configHomeFixture(t)
	writeConfig(t, loc.System, sessionmanager.Filename, "{")
	var out, stderr strings.Builder
	code := run(context.Background(), []string{"pi"}, &out, &stderr, deps)
	if code != 1 || runner.calls != 1 || !strings.Contains(stderr.String(), "plan_refused: early-stage fixture") || strings.Contains(stderr.String(), sessionmanager.CodeInvalid) {
		t.Fatalf("%d %s", code, &stderr)
	}
	os.Remove(filepath.Join(loc.System, sessionmanager.Filename))
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "board"), []byte("not invoked"), 0700)
	t.Setenv("PATH", dir)
	deps.managerService = &sessionmanager.Service{CanWriteSystem: func(string) bool { return false }}
	out.Reset()
	stderr.Reset()
	code = run(context.Background(), []string{"session-manager", "select", "board"}, &out, &stderr, deps)
	if code != 2 || !strings.Contains(stderr.String(), sessionmanager.CodeNotRegistered) || runner.calls != 1 {
		t.Fatalf("%d %s", code, &stderr)
	}
}
