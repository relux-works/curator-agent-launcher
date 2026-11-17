//go:build unix

package werkstatt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	schema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
)

const admStamp = "2026-01-01T00:00:00Z"

type admStep struct {
	Operation string
	Body      string
}

func admBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "scripted")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binary, "../sessionmanager/client/testdata/scripted")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 100 * time.Millisecond
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build scripted manager: %v %s", err, out)
	}
	return binary
}

func admVector(t *testing.T, name string) map[string]any {
	t.Helper()
	b, err := schema.Documents.ReadFile("vectors/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct{ Document map[string]any }
	if json.Unmarshal(b, &rows) != nil || len(rows) == 0 {
		t.Fatal("fixture unavailable")
	}
	return rows[0].Document
}

func admDescribe(t *testing.T, id string, capable bool) string {
	t.Helper()
	d := admVector(t, "describe")
	caps := []string{"launch-delegated/1", "attach/1", "resume/1", "admission/1", "labels/1", "list/1", "status/1", "stop/1"}
	if capable {
		caps = append(caps, client.ProcessObservationCapability)
	}
	d["id"], d["capabilities"] = id, caps
	d["runtimes"] = map[string]any{"claude_code": []string{"launch", "attach", "resume", "cancel", "list", "status", "stop"}}
	body := string(encoded(d))
	if err := schema.Validate("describe.schema.json", []byte(body)); err != nil {
		t.Fatalf("describe fixture invalid: %v", err)
	}
	return body
}

func admProcess() map[string]any {
	return map[string]any{
		"pid":        4242,
		"uid":        1001,
		"boot_id":    "6f1c0e2a-8b44-4d1e-9a51-2f0c3d7e9b10",
		"start_time": map[string]any{"kind": "linux-stat-starttime", "ticks": "123456789", "hz": 100},
	}
}

// admSession is a schema-valid enveloped session reply from manager in state,
// optionally carrying one negotiated member ("process" or "termination").
func admSession(t *testing.T, manager, state, member string) string {
	t.Helper()
	s := admVector(t, "session")
	s["manager"], s["session"], s["state"] = manager, "SES-example", state
	s["labels"] = map[string]any{"werkstatt": "example"}
	switch member {
	case "process":
		s["process"] = admProcess()
	case "termination":
		s["termination"] = map[string]any{"scope": "linux-cgroup", "witness": "populated-0"}
	case "":
	default:
		t.Fatalf("unknown member %q", member)
	}
	body := string(encoded(s))
	if err := schema.Validate("session.schema.json", []byte(body)); err != nil {
		t.Fatalf("session fixture invalid (%s/%s): %v", state, member, err)
	}
	return body
}

// admList is one list page from manager: a down row carrying termination
// followed by a live row carrying process, both labelled for the fixture.
func admList(t *testing.T, manager string) string {
	t.Helper()
	row := func(session, state, member string) map[string]any {
		r := admVector(t, "session")
		delete(r, "protocol")
		delete(r, "ok")
		r["manager"], r["session"], r["state"] = manager, session, state
		r["labels"] = map[string]any{"werkstatt": "example"}
		if member == "process" {
			r["process"] = admProcess()
		}
		if member == "termination" {
			r["termination"] = map[string]any{"scope": "dedicated-uid", "witness": "zero-process-proof"}
		}
		return r
	}
	page := map[string]any{"protocol": client.Protocol, "ok": true, "sessions": []map[string]any{row("SES-previous", "down", "termination"), row("SES-example", "idle", "process")}, "next_cursor": nil}
	body := string(encoded(page))
	if err := schema.Validate("list-page.schema.json", []byte(body)); err != nil {
		t.Fatalf("list fixture invalid: %v", err)
	}
	return body
}

func admTrace(t *testing.T, script string) []string {
	t.Helper()
	b, err := os.ReadFile(script + ".trace")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	for _, line := range bytes.Split(bytes.TrimSpace(b), []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var row struct{ Operation string }
		if json.Unmarshal(line, &row) != nil {
			t.Fatal("invalid trace")
		}
		calls = append(calls, row.Operation)
	}
	return calls
}

func admCleanup(t *testing.T, script string) {
	t.Helper()
	t.Cleanup(func() {
		b, err := os.ReadFile(script + ".pids")
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
				t.Error("invalid scripted pid")
				continue
			}
			if !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
				t.Errorf("scripted manager survived until cleanup: %d", pid)
			}
		}
	})
}

// admFixture wires a Service whose Client returns the concrete transport
// (client.Client) bound to per-manager scripts, so wire decoding and the
// admission copy in admittedRemote engage exactly as in production.
func admFixture(t *testing.T, binary string, scripts map[string][]admStep, liveness *client.Liveness) (Service, map[string]string) {
	t.Helper()
	root := t.TempDir()
	config := filepath.Join(root, "records")
	state := filepath.Join(root, "state")
	for _, dir := range []string{config, state} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(config, "example.json"), encoded(recordValue(root)), 0600); err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	managers := map[string]sessionmanager.Entry{}
	for _, id := range []string{"demo", "other"} {
		script := filepath.Join(root, "script-"+id+".json")
		steps := scripts[id]
		if steps == nil {
			steps = []admStep{}
		}
		if err := os.WriteFile(script, encoded(steps), 0600); err != nil {
			t.Fatal(err)
		}
		admCleanup(t, script)
		paths[id] = script
		managers[id] = sessionmanager.Entry{Executable: binary, ArgvPrefix: []string{script}, Protocol: client.Protocol, RegisteredAt: admStamp}
	}
	if liveness != nil {
		if err := os.WriteFile(filepath.Join(state, "example.json"), encoded(*liveness), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s := Service{
		Loader: Loader{Directory: config},
		Store:  client.Store{Directory: state},
		Now:    func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) },
		Registry: func() (sessionmanager.Registry, error) {
			return sessionmanager.Registry{Selected: "demo", Managers: managers}, nil
		},
		Trial:  func() (bool, error) { return false, nil },
		Client: func(id string, e sessionmanager.Entry) Remote { return client.Client{ID: id, Entry: e} },
		Prepare: func(context.Context, *Record, Command, string) (Prepared, error) {
			return Prepared{}, fmt.Errorf("prepare reached past the decode under test")
		},
	}
	return s, paths
}

func admLiveness(binary, owner, state string, stale bool) *client.Liveness {
	entry := sessionmanager.Entry{Executable: binary, RegisteredAt: admStamp}
	op := "11111111-1111-4111-8111-111111111111"
	l := &client.Liveness{ID: "example", State: state, StateSource: "manager", Manager: binding(owner, entry), Session: "SES-example", OperationID: &op, Generation: 1, StartedAt: admStamp, LastHeartbeatAt: admStamp, Doorbell: "off", AbandonedOperations: []client.Abandoned{}}
	if stale {
		l.Manager.Executable = "/previous"
	}
	return l
}

// Claim: Werkstatt up admits process/termination only under the selected manager's own capable describe (inventory list and same-manager owner read), refuses 6 without the token or across managers, and down/status/recycle/rebind keep refusing 6.
func TestServiceProcessObservationAdmission(t *testing.T) {
	binary := admBinary(t)
	describeCapable := admDescribe(t, "demo", true)
	describeLegacy := admDescribe(t, "demo", false)
	listMembers := admList(t, "demo")
	idleProcessDemo := admSession(t, "demo", "idle", "process")
	idleProcessOther := admSession(t, "other", "idle", "process")
	idlePlain := admSession(t, "demo", "idle", "")
	downTermination := admSession(t, "demo", "down", "termination")
	downPlain := admSession(t, "demo", "down", "")
	drainingPlain := admSession(t, "demo", "draining", "")
	drainingProcess := admSession(t, "demo", "draining", "process")
	unknownPlain := admSession(t, "demo", "unknown", "")
	const protocolErr = "session_host_protocol_error"
	type admCase struct {
		name      string
		command   Command
		owner     string // liveness owner; "" writes no liveness
		liveState string
		stale     bool
		demo      []admStep
		other     []admStep
		want      string // refusal code; "" expects success
		demoCalls []string
		otherCall []string
	}
	cases := []admCase{
		{name: "up-list-process-termination-admitted", command: Command{Action: "up", ID: "example"},
			demo: []admStep{{"describe", describeCapable}, {"list", listMembers}}, want: "werkstatt_already_up", demoCalls: []string{"describe", "list"}},
		{name: "up-list-no-token-process-termination", command: Command{Action: "up", ID: "example"},
			demo: []admStep{{"describe", describeLegacy}, {"list", listMembers}}, want: protocolErr, demoCalls: []string{"describe", "list"}},
		{name: "up-owner-same-manager-process", command: Command{Action: "up", ID: "example"}, owner: "demo", liveState: "idle",
			demo: []admStep{{"describe", describeCapable}, {"status", idleProcessDemo}}, want: "werkstatt_already_up", demoCalls: []string{"describe", "status"}},
		{name: "up-owner-same-manager-no-token", command: Command{Action: "up", ID: "example"}, owner: "demo", liveState: "idle",
			demo: []admStep{{"describe", describeLegacy}, {"status", idleProcessDemo}}, want: protocolErr, demoCalls: []string{"describe", "status"}},
		{name: "up-owner-other-manager-process", command: Command{Action: "up", ID: "example"}, owner: "other", liveState: "idle",
			demo: []admStep{{"describe", describeCapable}}, other: []admStep{{"status", idleProcessOther}}, want: protocolErr, demoCalls: []string{"describe"}, otherCall: []string{"status"}},
		{name: "down-stop-termination", command: Command{Action: "down", ID: "example"}, owner: "demo", liveState: "idle",
			demo: []admStep{{"status", idlePlain}, {"stop", downTermination}}, want: protocolErr, demoCalls: []string{"status", "stop"}},
		{name: "down-stop-plain-control", command: Command{Action: "down", ID: "example"}, owner: "demo", liveState: "idle",
			demo: []admStep{{"status", idlePlain}, {"stop", downPlain}}, demoCalls: []string{"status", "stop"}},
		{name: "status-owner-process", command: Command{Action: "status", ID: "example", JSON: true}, owner: "demo", liveState: "idle",
			demo: []admStep{{"status", idleProcessDemo}}, want: protocolErr, demoCalls: []string{"status"}},
		{name: "status-owner-plain-control", command: Command{Action: "status", ID: "example", JSON: true}, owner: "demo", liveState: "idle",
			demo: []admStep{{"status", idlePlain}}, demoCalls: []string{"status"}},
		{name: "recycle-poll-process", command: Command{Action: "recycle", ID: "example"}, owner: "demo", liveState: "idle",
			demo: []admStep{{"status", drainingPlain}, {"stop", drainingPlain}, {"status", drainingProcess}}, want: protocolErr, demoCalls: []string{"status", "stop", "status"}},
		{name: "recycle-poll-plain-control", command: Command{Action: "recycle", ID: "example"}, owner: "demo", liveState: "idle",
			demo: []admStep{{"status", drainingPlain}, {"stop", drainingPlain}, {"status", unknownPlain}}, want: "session_liveness_unknown", demoCalls: []string{"status", "stop", "status"}},
		{name: "rebind-process", command: Command{Action: "recover", ID: "example", RebindManager: true, JSON: true}, owner: "demo", liveState: "idle", stale: true,
			demo: []admStep{{"status", idleProcessDemo}}, want: protocolErr, demoCalls: []string{"status"}},
		{name: "rebind-plain-control", command: Command{Action: "recover", ID: "example", RebindManager: true, JSON: true}, owner: "demo", liveState: "idle", stale: true,
			demo: []admStep{{"status", idlePlain}, {"status", idlePlain}}, demoCalls: []string{"status", "status"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var liveness *client.Liveness
			if tc.owner != "" {
				liveness = admLiveness(binary, tc.owner, tc.liveState, tc.stale)
			}
			s, paths := admFixture(t, binary, map[string][]admStep{"demo": tc.demo, "other": tc.other}, liveness)
			var out, stderr bytes.Buffer
			s.Stdout, s.Stderr = &out, &stderr
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			exit, err := s.Run(ctx, tc.command)
			if tc.want == "" {
				if err != nil || exit != 0 {
					t.Fatalf("valid control refused: exit=%d err=%v stderr=%s", exit, err, stderr.String())
				}
			} else {
				code(t, err, tc.want)
				if tc.want == protocolErr {
					var ce *client.Error
					if !errors.As(err, &ce) || ce.ExitCode() != 6 {
						t.Fatalf("refusal is not exit 6: %v", err)
					}
				}
			}
			if got := admTrace(t, paths["demo"]); !slices.Equal(got, tc.demoCalls) {
				t.Fatalf("selected manager calls=%v want=%v", got, tc.demoCalls)
			}
			if got := admTrace(t, paths["other"]); !slices.Equal(got, tc.otherCall) {
				t.Fatalf("other manager calls=%v want=%v", got, tc.otherCall)
			}
			if tc.name == "down-stop-plain-control" && out.String() != "down\n" {
				t.Fatalf("down control stdout=%q", out.String())
			}
			if tc.name == "rebind-plain-control" {
				got, snapErr := s.Store.Snapshot("example")
				if snapErr != nil || got == nil || got.Manager.Executable != binary {
					t.Fatalf("rebind control did not refresh the binding: %v %v", got, snapErr)
				}
			}
		})
	}
}
