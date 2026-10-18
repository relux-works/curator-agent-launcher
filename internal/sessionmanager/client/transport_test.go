//go:build unix

package client

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/hosted"
	"golang.org/x/sys/unix"
)

// RPCs pass one request and only the projected environment. Oversize, timeout,
// incompatible exit/envelope and poisoned bodies return no partial response or
// plugin text. Each scripted child is bounded and checked for survival.
func TestScriptedRPC(t *testing.T) {
	binary := buildScript(t)
	good := string(encoded(session()))
	for _, tc := range []struct {
		name string
		step step
		code string
	}{
		{"success", step{Operation: "launch", Body: good}, ""},
		{"slow-success", step{Operation: "launch", Body: good, SleepMS: 1200}, ""},
		{"refusal", step{Operation: "launch", Body: `{"protocol":"curator-session-manager/1","ok":false,"code":"session_operation_conflict","details":{}}`, Exit: 4, Stderr: Limit * 3}, "session_operation_conflict"},
		{"oversize-response", step{Operation: "launch", Body: strings.Repeat("x", Limit+1)}, "session_host_protocol_error"},
		{"exit-disagreement", step{Operation: "launch", Body: good, Exit: 7}, "session_host_protocol_error"},
		{"poison", step{Operation: "launch", Body: strings.Replace(good, `"state":"unknown"`, `"state":"untrusted"`, 1)}, "session_host_protocol_error"},
		{"empty-response", step{Operation: "launch"}, "session_host_protocol_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, traces := scripted(t, binary, tc.step)
			request := document(t, "launch")
			c.Environment = []string{"PATH=/test-bin", "HOME=/test-home", "TERM=xterm", "A2A_AGENT=sentinel", "API_TOKEN=sentinel-secret", "CURATOR_RUN_PROTOCOL=wrong"}
			r, err := c.Call(bounded(t), "launch", request)
			if tc.code == "" {
				if err != nil || r.Session == nil {
					t.Fatal(r, err)
				}
			} else {
				e := requireCode(t, err, tc.code)
				if strings.Contains(e.Diagnostic(), "sentinel") {
					t.Fatal("plugin text escaped")
				}
				if tc.step.Stderr != 0 && e.StderrBytes != int64(tc.step.Stderr) {
					t.Fatalf("stderr count %d", e.StderrBytes)
				}
			}
			tr := traces()
			if len(tr) != 1 || string(tr[0].Request) != string(request) {
				t.Fatal("request changed")
			}
			if !slices.Equal(tr[0].Env, []string{"PATH=/test-bin", "HOME=/test-home", "CURATOR_RUN_PROTOCOL=" + Protocol}) {
				t.Fatalf("environment leak: %v", tr[0].Env)
			}
			if !slices.Equal(tr[0].Args, []string{"launch"}) || tr[0].PGID != tr[0].PID {
				t.Fatal("RPC argv/group mismatch")
			}
		})
	}
	t.Run("at-bound", func(t *testing.T) {
		request := document(t, "launch")
		request = append(request, []byte(strings.Repeat(" ", Limit-len(request)))...)
		body := good + strings.Repeat(" ", Limit-len(good))
		c, traces := scripted(t, binary, step{Operation: "launch", Body: body})
		r, err := c.Call(bounded(t), "launch", request)
		if err != nil || r.Session == nil {
			t.Fatal("valid exact byte bound", err)
		}
		if tr := traces(); len(tr) != 1 || len(tr[0].Request) != Limit {
			t.Fatal("bound request truncated")
		}
	})

	t.Run("request-bound-before-exec", func(t *testing.T) {
		c, traces := scripted(t, binary, step{Operation: "launch", Body: good})
		_, err := c.Call(bounded(t), "launch", append(document(t, "launch"), []byte(strings.Repeat(" ", Limit+1-len(document(t, "launch"))))...))
		requireCode(t, err, "launch_plan_invalid")
		if len(traces()) != 0 {
			t.Fatal("oversize request contacted plugin")
		}
	})
	t.Run("group-timeout", func(t *testing.T) {
		c, traces := scripted(t, binary, step{Operation: "describe", Body: string(document(t, "describe")), SleepMS: 3000, Descendant: true})
		ctx, cancel := context.WithTimeout(bounded(t), time.Second)
		defer cancel()
		start := time.Now()
		_, err := c.Call(ctx, "describe", []byte("{}"))
		if requireCode(t, err, "session_host_unavailable").Details["reason"] != "timeout" {
			t.Fatal("deadline reason lost")
		}
		if time.Since(start) > 4*time.Second {
			t.Fatal("deadline did not terminate group")
		}
		tr := traces()
		if len(tr) != 1 || tr[0].Child == 0 {
			t.Fatal("group probe did not run")
		}
		assertGone(t, tr[0].PID)
		assertGone(t, tr[0].Child)
	})
	t.Run("missing-executable", func(t *testing.T) {
		c, traces := scripted(t, binary, step{Operation: "describe"})
		c.Entry.Executable = filepath.Join(t.TempDir(), "missing")
		_, err := c.Call(bounded(t), "describe", []byte("{}"))
		e := requireCode(t, err, "session_host_unavailable")
		if e.Details["reason"] != "transport" || len(traces()) != 0 {
			t.Fatal("bad startup classification")
		}
	})
}

// The byte collectors retain at most 64 KiB, count discarded stderr, and invoke
// cancellation exactly when stdout exceeds its bound, not at the valid bound.
func TestCollectorsAndDeadlines(t *testing.T) {
	cancelled := false
	b := outputBound{cancel: func() { cancelled = true }}
	_, _ = b.Write(make([]byte, Limit))
	if cancelled || b.overflow || len(b.data) != Limit {
		t.Fatal("valid bound rejected")
	}
	_, _ = b.Write([]byte{1})
	if !cancelled || !b.overflow || len(b.data) != Limit {
		t.Fatal("response overflow retained")
	}
	var s stderrCounter
	_, _ = s.Write(make([]byte, Limit))
	_, _ = s.Write(make([]byte, Limit+1))
	if len(s.captured) != Limit || s.bytes != 2*Limit+1 {
		t.Fatal("stderr not bounded/countable")
	}
	for _, op := range []string{"describe", "status", "list", "stop", "cancel", "launch", "resume"} {
		want := 10 * time.Second
		if op == "launch" || op == "resume" {
			want = time.Minute
		}
		if Deadline(op) != want {
			t.Fatalf("wrong fixed deadline for %s", op)
		}
	}
}

// Environment projection resolves existing symlinks and missing suffixes without
// creating XDG paths; TERM is attach-only and relative XDG roots are omitted.
func TestEnvironmentProjection(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	caller := []string{"PATH=/first", "PATH=/last", "HOME=/home", "XDG_STATE_HOME=" + filepath.Join(link, "missing"), "XDG_CONFIG_HOME=relative", "TERM=xterm", "SECRET=sentinel"}
	base := []string{"PATH=/last", "HOME=/home", "XDG_STATE_HOME=" + filepath.Join(canonical, "missing")}
	for _, op := range []string{"attach", "launch-exact", "describe"} {
		env, err := PluginEnvironment(caller, op)
		want := slices.Clone(base)
		if op == "attach" {
			want = append(want, "TERM=xterm")
		}
		want = append(want, "CURATOR_RUN_PROTOCOL="+Protocol)
		if err != nil || !slices.Equal(env, want) {
			t.Fatalf("projection mismatch for %s: %v %v", op, env, err)
		}
	}
	if _, err := os.Stat(filepath.Join(real, "missing")); !os.IsNotExist(err) {
		t.Fatal("projection created state directory")
	}
	loop := filepath.Join(dir, "loop")
	if err := os.Symlink(loop, loop); err != nil {
		t.Fatal(err)
	}
	_, err = PluginEnvironment([]string{"XDG_STATE_HOME=" + loop}, "describe")
	requireCode(t, err, "session_host_unavailable")
}
func frame(code string) string {
	return string(encoded(map[string]any{"schema": hosted.StatusSchema, "schema_version": hosted.StatusSchemaVer, "type": hosted.StatusTypeRefusal, "code": code, "message": Message(code), "details": map[string]string{}}))
}

// The existing fd4 framing/classifier decides terminal refusals. Valid data
// cannot repair an earlier poison; truncation is transport, and absent status
// preserves the child exit even when terminal text resembles a refusal.
func TestScriptedTerminal(t *testing.T) {
	executable := buildScript(t)
	var short [6]byte
	binary.BigEndian.PutUint32(short[:4], 100)
	short[4] = '{'
	short[5] = '}'
	cases := []struct {
		name       string
		frames     []string
		raw        []byte
		exit, want int
		code       string
	}{
		{"child-exit", nil, nil, 7, 7, ""},
		{"attach-generation-stale", []string{frame("session_generation_stale")}, nil, 0, 1, ""},
		{"legacy-refusal", []string{frame("network_scope_unsupported")}, nil, 0, 16, ""},
		{"new-code-is-not-legacy-fd4", []string{frame("credential_source_stale")}, nil, 0, 0, "session_host_protocol_error"},
		{"last-refusal", []string{frame("launch_plan_invalid"), frame("session_host_capability_missing")}, nil, 0, 6, ""},
		{"poison-then-valid", []string{`{"secret":"sentinel"}`, frame("session_host_capability_missing")}, nil, 0, 0, "session_host_protocol_error"},
		{"free-text-message", []string{strings.Replace(frame("session_host_capability_missing"), Message("session_host_capability_missing"), "sentinel plugin text", 1)}, nil, 0, 0, "session_host_protocol_error"},
		{"zero-frame", nil, []byte{0, 0, 0, 0}, 0, 0, "session_host_protocol_error"},
		{"truncation", nil, short[:], 0, 0, "session_host_unavailable"},
	}
	for _, operation := range []string{"attach", "launch-exact"} {
		for _, tc := range cases {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				terminal, err := os.CreateTemp(t.TempDir(), "terminal")
				if err != nil {
					t.Fatal(err)
				}
				defer terminal.Close()
				c, traces := scripted(t, executable, step{Operation: operation, Frames: tc.frames, RawFrame: tc.raw, Exit: tc.exit, Body: "untyped refusal sentinel"})
				c.Environment = []string{"TERM=xterm", "SECRET=sentinel"}
				request := []byte(`{"session":"SES-example","expected_generation":1}`)
				if operation == "launch-exact" {
					request = []byte("{\n\"private_plan\": true\n}")
				}
				result, err := c.Terminal(bounded(t), operation, request, TerminalIO{Terminal: terminal})
				if tc.code != "" {
					requireCode(t, err, tc.code)
				} else if err != nil || result.Exit != tc.want {
					t.Fatalf("exit classification: %+v %v", result, err)
				}
				tr := traces()
				if len(tr) != 1 || string(tr[0].Request) != string(request) {
					t.Fatal("private request changed")
				}
				fd := 0
				wantArgs := []string{operation}
				if operation == "launch-exact" {
					fd = 3
					wantArgs = append(wantArgs, "--plan", "-", "--terminal-fd", "3", "--status-fd", "4")
				}
				if tr[0].FDs[fd] != fdID(t, terminal) || tr[0].FDs[1] != fdID(t, terminal) || tr[0].FDs[2] != fdID(t, terminal) || !slices.Equal(tr[0].Args, wantArgs) {
					t.Fatal("terminal fd/argv mapping", tr[0])
				}
				flags, err := unix.FcntlInt(terminal.Fd(), unix.F_GETFD, 0)
				if err != nil || flags&unix.FD_CLOEXEC == 0 {
					t.Fatal("launcher terminal not close-on-exec")
				}
				wantEnv := []string{"CURATOR_RUN_PROTOCOL=" + Protocol}
				if operation == "attach" {
					wantEnv = []string{"TERM=xterm", "CURATOR_RUN_PROTOCOL=" + Protocol}
				}
				if !reflect.DeepEqual(tr[0].Env, wantEnv) {
					t.Fatal("terminal environment leak", tr[0].Env)
				}
				if tr[0].PGID == tr[0].PID {
					t.Fatal("terminal moved out of foreground group")
				}
			})
		}
	}
	t.Run("terminal-required-before-contact", func(t *testing.T) {
		c, traces := scripted(t, executable, step{Operation: "attach"})
		_, err := c.Terminal(bounded(t), "attach", []byte(`{"session":"SES-example","expected_generation":1}`), TerminalIO{})
		requireCode(t, err, "session_host_terminal_required")
		if len(traces()) != 0 {
			t.Fatal("contact without terminal")
		}
	})
}

// Listing follows every cursor and fails closed, never returning partial data
// when a later page is poisoned, repeated, or belongs to another label scope.
func TestScriptedPagination(t *testing.T) {
	binary := buildScript(t)
	page := map[string]any{"protocol": Protocol, "ok": true, "sessions": []map[string]any{sessionRow()}, "next_cursor": "page-2"}
	for _, mode := range []string{"complete", "poison", "repeat"} {
		t.Run(mode, func(t *testing.T) {
			last := map[string]any{"protocol": Protocol, "ok": true, "sessions": []Session{}, "next_cursor": nil}
			if mode == "repeat" {
				last["next_cursor"] = "page-2"
			}
			if mode == "poison" {
				last["unknown"] = "sentinel"
			}
			c, traces := scripted(t, binary, step{Operation: "list", Request: encoded(map[string]any{"labels": map[string]string{}, "limit": 50}), Body: string(encoded(page))}, step{Operation: "list", Request: encoded(map[string]any{"labels": map[string]string{}, "limit": 50, "cursor": "page-2"}), Body: string(encoded(last))})
			sessions, err := c.ListAll(bounded(t), nil, 0)
			if mode == "complete" {
				if err != nil || len(sessions) != 1 {
					t.Fatal(sessions, err)
				}
			} else {
				requireCode(t, err, "session_host_protocol_error")
				if sessions != nil {
					t.Fatal("partial listing")
				}
			}
			if len(traces()) != 2 {
				t.Fatal("did not follow cursor")
			}
		})
	}
}

// An unrelated non-CLOEXEC descriptor is absent in the RPC child; the three
// stdio descriptors remain pipes, and a complete valid response still arrives.
func TestRPCDescriptorIsolation(t *testing.T) {
	binary := buildScript(t)
	path := filepath.Join(t.TempDir(), "unrelated")
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	file := os.NewFile(uintptr(fd), "unrelated")
	defer file.Close()
	unrelated := fdID(t, file)
	c, traces := scripted(t, binary, step{Operation: "describe", Body: string(document(t, "describe"))})
	response, err := c.Call(bounded(t), "describe", []byte(`{}`))
	if err != nil || response.Describe == nil {
		t.Fatal(response, err)
	}
	tr := traces()
	if len(tr) != 1 {
		t.Fatal("missing probe")
	}
	for _, identity := range tr[0].FDs {
		if identity == unrelated {
			t.Fatal("unrelated descriptor inherited")
		}
	}
}

// Exact-plan transport retains its separate 1 MiB/depth-16 contract; attach
// stays at the 64 KiB RPC request schema. Rejections occur before child contact.
func TestExactPlanBounds(t *testing.T) {
	binary := buildScript(t)
	for _, tc := range []struct {
		name    string
		request []byte
		valid   bool
	}{
		{"at-byte-bound", []byte(`{"padding":"` + strings.Repeat("x", hosted.LimitWireBytes-14) + `"}`), true},
		{"over-byte-bound", []byte(`{"padding":"` + strings.Repeat("x", hosted.LimitWireBytes-13) + `"}`), false},
		{"at-depth-bound", []byte(`{"x":` + strings.Repeat(`[`, 14) + `0` + strings.Repeat(`]`, 14) + `}`), true},
		{"over-depth-bound", []byte(`{"x":` + strings.Repeat(`[`, 15) + `0` + strings.Repeat(`]`, 15) + `}`), false},
		{"duplicate", []byte(`{"x":1,"x":2}`), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, traces := scripted(t, binary, step{Operation: "launch-exact"})
			terminal, err := os.CreateTemp(t.TempDir(), "terminal")
			if err != nil {
				t.Fatal(err)
			}
			defer terminal.Close()
			result, err := c.Terminal(bounded(t), "launch-exact", tc.request, TerminalIO{Terminal: terminal})
			if tc.valid {
				if err != nil || result.Exit != 0 {
					t.Fatal("valid plan bound", result, err)
				}
				if tr := traces(); len(tr) != 1 || string(tr[0].Request) != string(tc.request) {
					t.Fatal("exact bytes changed")
				}
			} else {
				requireCode(t, err, "launch_plan_invalid")
				if len(traces()) != 0 {
					t.Fatal("invalid plan contacted plugin")
				}
			}
		})
	}
}

// Descriptor enumeration can mark the launcher's live handles without closing
// them, including Go runtime descriptors that are already close-on-exec.
func TestDescriptorFlagging(t *testing.T) {
	if err := markDescriptorsCloseOnExec(); err != nil {
		t.Fatal(err)
	}
}

// Successful RPC completion preserves a plugin-owned descendant with closed
// stdio, unlike the nearby overrun case. The test explicitly kills that bounded
// child by its recorded PID and asserts that it cannot survive cleanup.
func TestSuccessfulRPCPreservesPluginChild(t *testing.T) {
	binary := buildScript(t)
	c, traces := scripted(t, binary, step{Operation: "describe", Body: string(document(t, "describe")), Descendant: true})
	response, err := c.Call(bounded(t), "describe", []byte(`{}`))
	if err != nil || response.Describe == nil {
		t.Fatal(response, err)
	}
	tr := traces()
	if len(tr) != 1 || tr[0].Child == 0 {
		t.Fatal("child probe not applied")
	}
	progress := c.Entry.ArgvPrefix[0] + ".child"
	before, _ := os.ReadFile(progress)
	alive := false
	until := time.Now().Add(10 * time.Second)
	for time.Now().Before(until) {
		after, _ := os.ReadFile(progress)
		if len(after) > 0 && string(after) != string(before) {
			alive = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !alive {
		t.Fatal("successful RPC killed plugin child")
	}
	_ = syscall.Kill(tr[0].Child, syscall.SIGKILL)
	assertGone(t, tr[0].Child)
}
