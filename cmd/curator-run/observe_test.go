//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/fakemanager"
	managerschema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
)

type observeFile struct {
	Mode     fs.FileMode
	Modified int64
	Data     string
}

// Audit telemetry is the sole excluded file. Directory entries and modes are
// checked, but their mtimes change when the fixture atomically replaces telemetry.
func observeSnapshot(t *testing.T, root string, exclude ...string) map[string]observeFile {
	t.Helper()
	result := map[string]observeFile{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if slices.Contains(exclude, rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		row := observeFile{Mode: info.Mode()}
		if info.Mode().IsRegular() {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			row.Data = string(b)
			row.Modified = info.ModTime().UnixNano()
		}
		if info.Mode()&os.ModeSymlink != 0 {
			row.Data, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		result[rel] = row
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func observeUnchanged(t *testing.T, root string, before map[string]observeFile, exclude ...string) {
	t.Helper()
	after := observeSnapshot(t, root, exclude...)
	if !reflect.DeepEqual(before, after) {
		for k, v := range before {
			if after[k] != v {
				t.Errorf("changed local file %s", k)
			}
		}
		for k := range after {
			if _, ok := before[k]; !ok {
				t.Errorf("new local file %s", k)
			}
		}
		t.Fatal("observation changed local state")
	}
}
func observeVector(t *testing.T, name string) map[string]any {
	t.Helper()
	b, err := managerschema.Documents.ReadFile("vectors/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct{ Document map[string]any }
	if json.Unmarshal(b, &rows) != nil || len(rows) == 0 {
		t.Fatal("invalid fixture")
	}
	return rows[0].Document
}
func observeRegistry(t *testing.T, dir, exe string, prefix []string) sessionmanager.Entry {
	t.Helper()
	entry := sessionmanager.Entry{Executable: exe, ArgvPrefix: prefix, Protocol: sessionmanager.Protocol, RegisteredAt: "2026-01-01T00:00:00.125+00:00", RegisteredBy: "test fixture"}
	r := sessionmanager.Empty()
	r.Managers["demo"] = entry
	b, err := sessionmanager.Encode(r)
	if err != nil {
		t.Fatal(err)
	}
	writeConfig(t, dir, sessionmanager.Filename, string(b))
	return entry
}
func observeNoProcesses(t *testing.T, pids []int) {
	t.Helper()
	for _, pid := range pids {
		for _, p := range []int{pid, -pid} {
			if !errors.Is(syscall.Kill(p, 0), syscall.ESRCH) {
				_ = syscall.Kill(p, syscall.SIGKILL)
				t.Errorf("observation child survived: %d", p)
			}
		}
	}
}

// Real fake-manager live/down/unknown/unreadable evidence is emitted exactly,
// without selection, reconciliation, state writes or any RPC beyond describe/status.
func TestObserveFake(t *testing.T) {
	bin := frontBuild(t)
	for _, state := range []string{"idle", "down", "unknown", "unreadable"} {
		t.Run(state, func(t *testing.T) {
			f := newFront(t, bin)
			f.registry("unselected")
			request := fakemanager.LaunchTemplate(f.root)
			p := f.managerConfig.Profiles["sample"]
			request["profile"] = map[string]string{"name": "sample", "lock_sha256": p.LockSHA256, "fragment_digest": p.FragmentDigest}
			code, out, stderr := f.command(bin.manager, []string{"launch"}, frontJSON(request), false, false, 10*time.Second)
			if code != 0 {
				t.Fatalf("seed: %d %s %s", code, out, stderr)
			}
			var record map[string]any
			if json.Unmarshal([]byte(out), &record) != nil {
				t.Fatal(out)
			}
			handle, ok := record["session"].(string)
			if !ok {
				t.Fatal("seed did not return a session", out)
			}
			switch state {
			case "down":
				code, out, stderr = f.command(bin.manager, []string{"stop"}, frontJSON(map[string]any{"session": handle, "expected_generation": record["generation"], "mode": "force", "precondition": "none"}), false, false, 10*time.Second)
				if code != 0 {
					t.Fatalf("seed down: %d %s %s", code, out, stderr)
				}
			case "unreadable":
				f.control(fakemanager.FixtureRequest{Operation: "corrupt_index", Kind: "unreadable", Session: handle})
			default:
				f.control(fakemanager.FixtureRequest{Operation: "set_activity", Activity: state, Session: handle})
			}
			code, out, stderr = f.command(bin.manager, []string{"status"}, frontJSON(map[string]string{"session": handle}), false, false, 10*time.Second)
			if code != 0 || json.Unmarshal([]byte(out), &record) != nil {
				t.Fatalf("expected record: %d %s %s", code, out, stderr)
			}
			delete(record, "protocol")
			delete(record, "ok")
			if record["state"] != state && !(state == "unreadable" && record["state"] == "unknown") {
				t.Fatal("bad positive fixture", record)
			}
			// Invalid launch policy must not affect observation. Snapshots also protect
			// unrelated liveness/journal/Werkstatt bytes, including a FIFO entry.
			for _, path := range []string{filepath.Join(bin.system, "launcher.json"), filepath.Join(f.config, "defaults.json"), filepath.Join(f.state, "curator-run", "op.json"), filepath.Join(f.state, "curator-run", "liveness", "SES-decoy.json"), filepath.Join(f.state, "curator-run", "werkstatt", "decoy", "record.json")} {
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				writeFixture(t, path, []byte("invalid unrelated state"), 0600)
			}
			fifo := filepath.Join(f.state, "curator-run", "liveness", "blocked.json")
			if err := syscall.Mkfifo(fifo, 0600); err != nil {
				t.Fatal(err)
			}
			beforeCalls := len(f.control(fakemanager.FixtureRequest{Operation: "observe_calls"}).CallHistory.Calls)
			audit := filepath.Join("state", "curator-run", "fake-manager", "call-history.json")
			before := observeSnapshot(t, f.root, audit)
			systemBefore := observeSnapshot(t, bin.system)
			loc := configfile.Locations{System: bin.system, User: f.config, LegacySystem: filepath.Join(f.root, "legacy-system"), LegacyUser: filepath.Join(f.root, "legacy-user")}
			stamp := time.Date(2026, 10, 27, 9, 8, 7, 999, time.FixedZone("fixture", 3600))
			deps := launchDeps{configLocations: func() (configfile.Locations, error) { return loc, nil }, environ: func() []string { return f.env }, now: func() time.Time { return stamp }}
			var stdout, diagnostic bytes.Buffer
			code = run(context.Background(), []string{"session-manager", "observe", "fake", "--session", handle, "--json"}, &stdout, &diagnostic, deps)
			expected := map[string]any{"schema": "curator-run-session-observation-v1", "manager": map[string]any{"id": "fake", "executable": bin.manager, "registered_at": "2026-01-01T00:00:00Z"}, "session": record, "observed_at": "2026-10-27T08:08:07Z"}
			// Compare closed canonical JSON; map key ordering is not part of the format.
			var actual any
			if code != 0 || diagnostic.Len() != 0 || json.Unmarshal(stdout.Bytes(), &actual) != nil || string(frontJSON(actual)) != string(frontJSON(expected)) || bytes.Count(stdout.Bytes(), []byte{'\n'}) != 1 {
				t.Fatalf("observation: %d %s %s; want %s", code, stdout.String(), diagnostic.String(), frontJSON(expected))
			}
			if err := managerschema.Validate("session-observation.schema.json", stdout.Bytes()); err != nil {
				t.Fatal(err)
			}
			observeUnchanged(t, f.root, before, audit)
			observeUnchanged(t, bin.system, systemBefore)
			history := f.control(fakemanager.FixtureRequest{Operation: "observe_calls"}).CallHistory
			if !slices.Equal(history.Calls[beforeCalls:], []string{"describe", "status"}) {
				t.Fatalf("observation RPCs: %v", history.Calls[beforeCalls:])
			}
			var pids []int
			for _, p := range history.CallProcesses {
				pids = append(pids, p.PID)
			}
			observeNoProcesses(t, pids)
			// A real binary witness exercises production dispatch and time generation.
			beforeCalls = len(history.Calls)
			began := time.Now().UTC().Truncate(time.Second)
			code, out, stderr = f.command(bin.launcher, []string{"session-manager", "observe", "fake", "--session", handle, "--json"}, nil, false, false, 15*time.Second)
			var real map[string]any
			if code != 0 || stderr != "" || json.Unmarshal([]byte(out), &real) != nil {
				t.Fatalf("binary: %d %s %s", code, out, stderr)
			}
			observed, err := time.Parse(time.RFC3339, real["observed_at"].(string))
			if err != nil || !strings.HasSuffix(real["observed_at"].(string), "Z") || observed.Nanosecond() != 0 || observed.Before(began) || observed.After(time.Now().UTC()) {
				t.Fatal("bad launcher clock", real)
			}
			real["observed_at"] = expected["observed_at"]
			if string(frontJSON(real)) != string(frontJSON(expected)) {
				t.Fatalf("binary changed record: %s", frontJSON(real))
			}
			observeUnchanged(t, f.root, before, audit)
			observeUnchanged(t, bin.system, systemBefore)
			history = f.control(fakemanager.FixtureRequest{Operation: "observe_calls"}).CallHistory
			if !slices.Equal(history.Calls[beforeCalls:], []string{"describe", "status"}) {
				t.Fatalf("binary observation RPCs: %v", history.Calls[beforeCalls:])
			}
			pids = nil
			for _, p := range history.CallProcesses {
				pids = append(pids, p.PID)
			}
			observeNoProcesses(t, pids)
		})
	}
}

type observeStep struct {
	Operation             string
	Request               json.RawMessage
	Body                  string
	Exit, Stderr, SleepMS int
}

func observeScriptBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "scripted")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binary, "../../internal/sessionmanager/client/testdata/scripted")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	out, err := cmd.CombinedOutput()
	frontGone(t, cmd)
	if err != nil {
		t.Fatalf("build scripted: %v %s", err, out)
	}
	return binary
}
func observeTrace(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path + ".trace")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	var pids []int
	for _, line := range bytes.Split(bytes.TrimSpace(b), []byte{'\n'}) {
		var row struct {
			Operation string
			PID, PGID int
		}
		if json.Unmarshal(line, &row) != nil {
			t.Fatal("invalid trace")
		}
		calls = append(calls, row.Operation)
		pids = append(pids, row.PID)
		if row.PGID != row.PID {
			t.Error("fixture child not isolated")
		}
	}
	observeNoProcesses(t, pids)
	return calls
}
func observeScriptCleanup(t *testing.T, path string) {
	t.Helper()
	t.Cleanup(func() {
		b, err := os.ReadFile(path + ".pids")
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		if err != nil {
			t.Error(err)
			return
		}
		for _, field := range strings.Fields(string(b)) {
			pid, err := strconv.Atoi(field)
			if err != nil || pid <= 0 {
				t.Error("invalid child pid")
				continue
			}
			if !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
				t.Errorf("child survived until cleanup: %d", pid)
			}
		}
	})
}

// Refusals and usage errors make only the permitted RPC prefix, emit no plugin
// text/stdout, and preserve local bytes/modes/timestamps, including legacy layers.
// Valid read-only registries beside them still succeed without write authority.
func TestObserveBoundaries(t *testing.T) {
	binary := observeScriptBinary(t)
	describe := observeVector(t, "describe")
	status := observeVector(t, "session")
	status["manager"] = "demo"
	status["session"] = "SES-example"
	good := []observeStep{{Operation: "describe", Request: json.RawMessage(`{}`), Body: string(frontJSON(describe))}, {Operation: "status", Request: json.RawMessage(`{"session":"SES-example"}`), Body: string(frontJSON(status))}}
	cases := []struct {
		name, registry, code string
		exit                 int
		tail                 []string
		calls                []string
	}{
		{name: "valid-user", registry: "user", calls: []string{"describe", "status"}},
		{name: "valid-system", registry: "system", calls: []string{"describe", "status"}},
		{name: "valid-legacy-user", registry: "legacy-user", calls: []string{"describe", "status"}},
		{name: "valid-legacy-system", registry: "legacy-system", calls: []string{"describe", "status"}},
		{name: "unknown-manager", registry: "other", code: "session_manager_not_registered", exit: 2},
		{name: "missing-registry", registry: "missing", code: "session_manager_not_registered", exit: 2},
		{name: "invalid-registry", registry: "invalid", code: "session_manager_registry_invalid", exit: 2},
		{name: "invalid-system-shadows-user", registry: "invalid-system", code: "session_manager_registry_invalid", exit: 2},
		{name: "unreachable", registry: "unreachable", code: "session_host_unavailable", exit: 1},
		{name: "missing-capability", registry: "user", code: "session_host_capability_missing", exit: 6, calls: []string{"describe"}},
		{name: "malformed-session", registry: "user", code: "session_host_protocol_error", exit: 6, calls: []string{"describe", "status"}},
		{name: "unreadable-malformed", registry: "user", code: "session_host_protocol_error", exit: 6, calls: []string{"describe", "status"}},
		{name: "timeout", registry: "user", code: "session_host_unavailable", exit: 1, calls: []string{"describe", "status"}},
		{name: "not-found", registry: "user", code: "session_resume_not_found", exit: 1, calls: []string{"describe", "status"}},
		{name: "liveness-unknown", registry: "user", code: "session_liveness_unknown", exit: 1, calls: []string{"describe", "status"}},
		{name: "missing-session", registry: "user", tail: []string{"--json"}, code: "usage", exit: 2},
		{name: "missing-json", registry: "user", tail: []string{"--session", "SES-example"}, code: "usage", exit: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps, runner, loc := configHomeFixture(t)
			root := filepath.Dir(loc.User)
			t.Chdir(root)
			path := filepath.Join(root, "script")
			steps := append([]observeStep(nil), good...)
			switch tc.name {
			case "missing-capability":
				d := observeVector(t, "describe")
				d["capabilities"] = []string{"launch-delegated/1"}
				steps = steps[:1]
				steps[0].Body = string(frontJSON(d))
			case "malformed-session":
				steps[1].Body = strings.Replace(good[1].Body, `"state":`, `"unknown":"SENTINEL_PLUGIN_TEXT","state":`, 1)
				steps[1].Stderr = 40
			case "unreadable-malformed":
				steps[1].Body = `{"protocol":"curator-session-manager/1","ok":true,"session":"SES-example","readable":false,"state":"unknown"}`
			case "timeout":
				steps[1].SleepMS = 12000
			case "not-found", "liveness-unknown":
				steps[1].Body = string(frontJSON(map[string]any{"protocol": sessionmanager.Protocol, "ok": false, "code": tc.code, "details": map[string]string{}}))
				steps[1].Exit = 1
			}
			writeFixture(t, path, frontJSON(steps), 0600)
			observeScriptCleanup(t, path)
			dir := loc.User
			switch tc.registry {
			case "system", "invalid-system":
				dir = loc.System
			case "legacy-user":
				dir = loc.LegacyUser
			case "legacy-system":
				dir = loc.LegacySystem
			}
			if tc.registry != "missing" {
				exe := binary
				if tc.registry == "unreachable" {
					exe = filepath.Join(root, "missing-executable")
				}
				entry := observeRegistry(t, dir, exe, []string{"script"})
				if tc.registry == "invalid" || tc.registry == "invalid-system" {
					writeConfig(t, dir, sessionmanager.Filename, "{")
				}
				if tc.registry == "invalid-system" {
					observeRegistry(t, loc.User, binary, []string{"script"})
				}
				if tc.registry == "other" {
					r := sessionmanager.Empty()
					r.Managers["other"] = entry
					r.Selected = "other"
					b, _ := sessionmanager.Encode(r)
					writeConfig(t, dir, sessionmanager.Filename, string(b))
				}
			}
			// These launch-policy bytes must never be parsed by the administrative path.
			writeConfig(t, loc.System, "launcher.json", "{")
			writeConfig(t, loc.User, "defaults.json", "{")
			deps.environ = func() []string { return []string{"HOME=" + root, "PATH=" + root} }
			deps.managerService = &sessionmanager.Service{CanWriteSystem: func(string) bool { t.Fatal("observation probed write authority"); return false }}
			if strings.HasPrefix(tc.name, "valid-") {
				if err := os.Chmod(dir, 0500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
			}
			before := observeSnapshot(t, root, "script.index", "script.trace", "script.pids")
			args := []string{"session-manager", "observe", "demo", "--session", "SES-example", "--json"}
			if tc.tail != nil {
				args = append([]string{"session-manager", "observe", "demo"}, tc.tail...)
			}
			var out, stderr bytes.Buffer
			exit := run(context.Background(), args, &out, &stderr, deps)
			if exit != tc.exit {
				t.Fatalf("exit %d want %d: %s %s", exit, tc.exit, out.String(), stderr.String())
			}
			if tc.code != "" {
				if out.Len() != 0 || !strings.HasPrefix(stderr.String(), "curator-run: "+tc.code+":") || strings.Count(stderr.String(), "\n") != 1 || strings.Contains(stderr.String(), "SENTINEL") {
					t.Fatalf("bad refusal %q %q", out.String(), stderr.String())
				}
			} else {
				if stderr.Len() != 0 || managerschema.Validate("session-observation.schema.json", out.Bytes()) != nil {
					t.Fatalf("positive control: %s %s", out.String(), stderr.String())
				}
				var document struct {
					Manager struct {
						ID, Executable string
						RegisteredAt   string `json:"registered_at"`
					}
				}
				if json.Unmarshal(out.Bytes(), &document) != nil || document.Manager.ID != "demo" || document.Manager.Executable != binary || document.Manager.RegisteredAt != "2026-01-01T00:00:00.125+00:00" {
					t.Fatalf("registration incarnation changed: %s", out.String())
				}
			}
			if tc.name == "timeout" && !strings.Contains(stderr.String(), "reason=timeout") {
				t.Fatal("wrong timeout refusal", stderr.String())
			}
			if !slices.Equal(observeTrace(t, path), tc.calls) {
				t.Fatalf("RPC trace %v want %v", observeTrace(t, path), tc.calls)
			}
			if runner.calls != 0 {
				t.Fatal("observation composed a launch")
			}
			observeUnchanged(t, root, before, "script.index", "script.trace", "script.pids")
		})
	}
}
