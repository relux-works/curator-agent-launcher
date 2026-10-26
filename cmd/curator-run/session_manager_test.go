package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
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

// Auto refuses a malformed registry; a manager
// binary on PATH alone also cannot make an unknown administrative id registered.
func TestRunSessionManagerLaunchUsesRegistry(t *testing.T) {
	deps, runner, loc := configHomeFixture(t)
	writeConfig(t, loc.System, sessionmanager.Filename, "{")
	var out, stderr strings.Builder
	code := run(context.Background(), []string{"pi"}, &out, &stderr, deps)
	if code != 2 || runner.calls != 1 || !strings.Contains(stderr.String(), sessionmanager.CodeInvalid) {
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

// Busy is an operational refusal with a closed, detail-free diagnostic; usage
// remains exit 2, so contention cannot silently inherit configuration status.
func TestSessionManagerBusyDiagnostic(t *testing.T) {
	var stderr strings.Builder
	err := &sessionmanager.Error{Code: sessionmanager.CodeBusy}
	if exit := emitFailure(&stderr, err.Code, err); exit != 1 || stderr.String() != "curator-run: session_manager_registry_busy: session-manager registry is busy\n" {
		t.Fatalf("exit=%d stderr=%q", exit, stderr.String())
	}
	stderr.Reset()
	if exit := emitFailure(&stderr, "usage", &sessionmanager.Error{Code: "usage"}); exit != 2 || !strings.HasPrefix(stderr.String(), "curator-run: usage: ") {
		t.Fatalf("exit=%d stderr=%q", exit, stderr.String())
	}
}

// Invalid scope is exit 2 before any registry directory or lock is created;
// the same manifest/ID with a valid user scope registers and selects normally.
func TestSessionManagerScopeUsageNoEffects(t *testing.T) {
	deps, runner, loc := configHomeFixture(t)
	deps.managerService = &sessionmanager.Service{CanWriteSystem: func(string) bool { return false }}
	executable := filepath.Join(t.TempDir(), "manager")
	if err := os.WriteFile(executable, []byte("never executed"), 0700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"id": "board", "executable": executable, "argv_prefix": []string{"plugin"}, "protocol": sessionmanager.Protocol, "registered_by": "test installer"})
	for _, op := range []string{"register", "use", "select", "unregister"} {
		base := []string{"session-manager", op, "board"}
		if op == "register" {
			base = []string{"session-manager", op, "--manifest", "-"}
		}
		var out, stderr strings.Builder
		deps.stdin = strings.NewReader(string(data))
		code := run(context.Background(), append(base, "--scope", "SENTINEL_SECRET_SCOPE"), &out, &stderr, deps)
		if code != 2 || out.Len() != 0 || !strings.Contains(stderr.String(), "--scope needs user or system") || strings.Contains(stderr.String(), "SENTINEL") {
			t.Fatalf("exit=%d out=%q err=%q", code, out.String(), stderr.String())
		}
		for _, dir := range []string{loc.System, loc.User} {
			for _, name := range []string{sessionmanager.Filename, sessionmanager.Filename + ".lock"} {
				if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
					t.Fatalf("invalid scope touched %s: %v", name, err)
				}
			}
		}
	}
	var out, stderr strings.Builder
	deps.stdin = strings.NewReader(string(data))
	for _, args := range [][]string{{"session-manager", "register", "--manifest", "-", "--scope", "user"}, {"session-manager", "use", "board", "--scope", "user"}} {
		if code := run(context.Background(), args, &out, &stderr, deps); code != 0 || stderr.Len() != 0 {
			t.Fatalf("valid control exit=%d err=%q", code, stderr.String())
		}
	}
	if runner.calls != 0 {
		t.Fatal("registry command launched a child")
	}
}

// The explicit-scope write-authority error is a closed exit-2 diagnostic with
// a directory path and scope, distinguishable from a corrupt registry's error.
func TestSessionManagerUnwritableDiagnostic(t *testing.T) {
	for _, scope := range []string{"system", "user"} {
		var stderr strings.Builder
		path := filepath.Join(t.TempDir(), "registry-directory")
		err := &sessionmanager.Error{Code: sessionmanager.CodeUnwritable, Path: path, Scope: scope}
		want := "curator-run: session_manager_registry_unwritable: session-manager registry directory is not writable path=" + strconv.Quote(path) + " scope=" + scope + "\n"
		if exit := emitFailure(&stderr, err.Code, err); exit != 2 || stderr.String() != want {
			t.Fatalf("exit=%d stderr=%q", exit, stderr.String())
		}
	}
	var stderr strings.Builder
	err := &sessionmanager.Error{Code: sessionmanager.CodeInvalid, Reason: "parse"}
	if exit := emitFailure(&stderr, err.Code, err); exit != 2 || stderr.String() != "curator-run: session_manager_registry_invalid: invalid session-manager registry reason=\"parse\"\n" {
		t.Fatalf("corrupt-registry exit=%d stderr=%q", exit, stderr.String())
	}
}
