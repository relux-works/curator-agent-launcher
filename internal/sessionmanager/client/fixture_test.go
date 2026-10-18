//go:build unix

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	managerschema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
)

const operationID = "11111111-1111-4111-8111-111111111111"
const secondID = "22222222-2222-4222-8222-222222222222"
const stamp = "2026-01-01T00:00:00Z"

type step struct {
	Operation    string
	Request      json.RawMessage
	Body         string
	Exit, Stderr int
	Frames       []string
	RawFrame     []byte
	SleepMS      int
	Descendant   bool
}
type trace struct {
	PID, PGID, Child int
	Operation        string
	Args, Env        []string
	Request          []byte
	FDs              map[int]string
}

func encoded(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
func document(t *testing.T, name string) []byte {
	t.Helper()
	b, err := managerschema.Documents.ReadFile("vectors/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct{ Document json.RawMessage }
	if json.Unmarshal(b, &rows) != nil || len(rows) == 0 {
		t.Fatal("fixture unavailable")
	}
	return rows[0].Document
}
func session() Session {
	return Session{Readable: true, Protocol: Protocol, OK: true, Session: "SES-example", OperationID: operationID, Manager: "demo", Runtime: "codex_cli", State: "unknown", Labels: map[string]string{}, Generation: 1, StartedAt: stamp, Doorbell: Doorbell{State: "off"}}
}
func sessionRow() map[string]any {
	var row map[string]any
	_ = json.Unmarshal(encoded(session()), &row)
	delete(row, "protocol")
	delete(row, "ok")
	return row
}
func unreadableRow() map[string]any {
	row := sessionRow()
	for k := range row {
		if k != "session" && k != "manager" && k != "labels" && k != "doorbell" {
			row[k] = nil
		}
	}
	row["readable"], row["state"] = false, "unknown"
	return row
}
func outcome(state string) string {
	m := map[string]any{"protocol": Protocol, "ok": true, "outcome": state}
	if state == "started" {
		m["session"] = sessionRow()
	}
	if state == "failed" {
		m["code"] = "policy_refused"
	}
	return string(encoded(m))
}
func statusRequest() []byte { return encoded(map[string]string{"operation_id": operationID}) }
func bounded(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func mustWrite(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
}
func buildScript(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "scripted-plugin")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binary, "./testdata/scripted")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 100 * time.Millisecond
	if b, err := cmd.CombinedOutput(); err != nil {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			assertGone(t, cmd.Process.Pid)
			assertGone(t, -cmd.Process.Pid)
		}
		t.Fatalf("build scripted protocol: %v: %s", err, b)
	}
	assertGone(t, cmd.Process.Pid)
	assertGone(t, -cmd.Process.Pid)
	return binary
}
func scripted(t *testing.T, binary string, steps ...step) (Client, func() []trace) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "script.json")
	mustWrite(t, path, encoded(steps))
	traces := func() []trace {
		b, err := os.ReadFile(path + ".trace")
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			t.Fatal(err)
		}
		var result []trace
		for _, line := range bytes.Split(bytes.TrimSpace(b), []byte{'\n'}) {
			if len(line) == 0 {
				continue
			}
			var tr trace
			if json.Unmarshal(line, &tr) != nil {
				t.Fatal("invalid trace")
			}
			result = append(result, tr)
		}
		return result
	}
	// Every spawned protocol process and optional group child is recorded, killed
	// by its exact PID/group if needed, and asserted absent at fixture cleanup.
	t.Cleanup(func() {
		for _, tr := range traces() {
			for _, pid := range []int{tr.PID, tr.Child} {
				if pid == 0 {
					continue
				}
				if syscall.Kill(pid, 0) == nil {
					_ = syscall.Kill(pid, syscall.SIGKILL)
				}
				assertGone(t, pid)
			}
			if tr.PGID == tr.PID {
				assertGone(t, -tr.PGID)
			}
		}
		// Record the PID before reading stdin too: a faulty transport cannot
		// evade cleanup by leaving the fixture blocked before its full trace.
		if data, err := os.ReadFile(path + ".pids"); err == nil {
			for _, word := range strings.Fields(string(data)) {
				pid, err := strconv.Atoi(word)
				if err != nil || pid <= 0 {
					t.Fatal("invalid fixture pid")
				}
				if steps[0].Operation != "attach" && steps[0].Operation != "launch-exact" {
					_ = syscall.Kill(-pid, syscall.SIGKILL)
				}
				if syscall.Kill(pid, 0) == nil {
					_ = syscall.Kill(pid, syscall.SIGKILL)
				}
				assertGone(t, pid)
				if steps[0].Operation != "attach" && steps[0].Operation != "launch-exact" {
					assertGone(t, -pid)
				}
			}
		}
	})
	return Client{ID: "demo", Entry: sessionmanager.Entry{Executable: binary, ArgvPrefix: []string{path}, Protocol: Protocol, RegisteredAt: stamp}}, traces
}
func assertGone(t *testing.T, pid int) {
	t.Helper()
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("protocol process %d survived cleanup", pid)
}
func requireCode(t *testing.T, err error, code string) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("wanted %s, got %v", code, err)
	}
	return e
}
func fdID(t *testing.T, f *os.File) string {
	t.Helper()
	var st syscall.Stat_t
	if err := syscall.Fstat(int(f.Fd()), &st); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%d:%d:%d", st.Dev, st.Ino, st.Mode)
}
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
