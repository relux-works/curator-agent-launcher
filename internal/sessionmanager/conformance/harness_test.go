//go:build unix

package conformance

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/hosted"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/fakemanager"
)

type arguments []string

func (a *arguments) String() string     { return strings.Join(*a, " ") }
func (a *arguments) Set(s string) error { *a = append(*a, s); return nil }

var plugin = flag.String("plugin", "", "absolute registered plugin executable (default: build reference fake)")
var fixture = flag.String("fixture", "", "absolute out-of-band fixture adapter executable")
var managerID = flag.String("manager", "fake", "registered manager ID")
var launchFile = flag.String("launch", "", "valid delegated launch template JSON for an external plugin")
var prefix, fixturePrefix arguments

func init() {
	flag.Var(&prefix, "prefix", "one plugin argv-prefix element; repeat")
	flag.Var(&fixturePrefix, "fixture-prefix", "one fixture argv-prefix element; repeat")
}

type endpoint struct {
	Path   string
	Prefix []string
	Budget time.Duration
}
type harness struct {
	t               *testing.T
	plugin, fixture endpoint
	root, id        string
	template        map[string]any
	bad             []string
	mu              sync.Mutex
	pids            map[int]bool
}
type violation struct {
	Property, Detail string
	Setup            bool
}

func (v *violation) Error() string {
	if v.Setup {
		return "SETUP: " + v.Detail
	}
	return "VIOLATION " + v.Property + ": " + v.Detail
}
func check(p string, ok bool, detail string) error {
	if !ok {
		return &violation{Property: p, Detail: detail}
	}
	return nil
}
func setup(err error) error {
	if err == nil {
		return nil
	}
	return &violation{Detail: err.Error(), Setup: true}
}
func jsonBytes(v any) []byte {
	b, e := json.Marshal(v)
	if e != nil {
		panic(e)
	}
	return b
}
func clone(v map[string]any) map[string]any {
	var m map[string]any
	_ = json.Unmarshal(jsonBytes(v), &m)
	return m
}
func operationID() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	s := hex.EncodeToString(b[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}
func gone(pid int) bool { return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) }
func assertGone(t *testing.T, pid int) {
	t.Helper()
	until := time.Now().Add(time.Second)
	for time.Now().Before(until) {
		if gone(pid) {
			return
		}
		time.Sleep(time.Millisecond * 5)
	}
	t.Errorf("test process survived: %d", pid)
}
func buildFake(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "fake-manager")
	for _, target := range []struct{ Suffix, Package string }{{"", "../../../cmd/curator-run-fake-manager"}, {"-fixture", "../../../cmd/curator-run-fake-manager-fixture"}} {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", "build", "-o", binary+target.Suffix, target.Package)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
		cmd.WaitDelay = time.Millisecond * 100
		b, err := cmd.CombinedOutput()
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			assertGone(t, cmd.Process.Pid)
			assertGone(t, -cmd.Process.Pid)
		}
		if err != nil {
			t.Fatalf("build fixture: %v: %s", err, b)
		}
		cancel()
	}

	return binary
}
func newHarness(t *testing.T, binary string, bad []string) *harness {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, root: root, id: *managerID, bad: bad, pids: map[int]bool{}}
	h.plugin = endpoint{Path: binary, Prefix: []string{"--state", filepath.Join(root, "manager")}}
	h.fixture = endpoint{Path: binary + "-fixture", Prefix: []string{"--state", filepath.Join(root, "manager")}}
	if *plugin != "" {
		h.plugin = endpoint{Path: *plugin, Prefix: prefix}
		h.fixture = endpoint{Path: *fixture, Prefix: fixturePrefix}
	}
	t.Cleanup(func() {
		h.mu.Lock()
		pids := []int{}
		for pid := range h.pids {
			pids = append(pids, pid)
		}
		h.mu.Unlock()
		for _, pid := range pids {
			if !gone(-pid) {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
			assertGone(t, pid)
			assertGone(t, -pid)
		}
	})
	if h.fixture.Path != "" {
		result, err := h.control(fakemanager.FixtureRequest{Operation: "reset"})
		if err != nil {
			t.Fatal(err)
		}
		if json.Unmarshal(result.Launch, &h.template) != nil {
			t.Fatal("fixture reset requires launch template")
		}
		if len(bad) > 0 {
			if _, err = h.control(fakemanager.FixtureRequest{Operation: "inject_fault", Bad: bad}); err != nil {
				t.Fatal(err)
			}
		}
	} else {
		b, err := os.ReadFile(*launchFile)
		if err != nil || json.Unmarshal(b, &h.template) != nil {
			t.Fatal("external tier A requires -launch valid-template.json")
		}
	}
	// Every case has an isolated label scope, including external runs without reset.
	labels, ok := h.template["labels"].(map[string]any)
	if !ok {
		labels = map[string]any{}
	}
	labels["conformance"] = strings.ReplaceAll(operationID(), "-", "")
	h.template["labels"] = labels
	return h
}

type wire struct {
	Body, Stderr, Frames []byte
	Leak                 bool
	Exit                 int
}

func (h *harness) exec(ep endpoint, op string, data []byte, terminal bool) (wire, error) {
	if !filepath.IsAbs(ep.Path) {
		return wire{}, errors.New("absolute executable required")
	}
	budget := ep.Budget
	if budget == 0 {
		budget = 12 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	args := append(append([]string{}, ep.Prefix...), op)
	if op == "" {
		args = append([]string{}, ep.Prefix...)
	}
	cmd := exec.CommandContext(ctx, ep.Path, args...)
	cmd.Dir = h.root
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + h.root, "XDG_CONFIG_HOME=" + h.root, "XDG_STATE_HOME=" + h.root, "CURATOR_RUN_PROTOCOL=" + client.Protocol}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Millisecond * 100
	bound := client.Limit + 1
	if op == "" {
		bound = 8 << 20
	}
	out, stderr := capture{Limit: bound}, capture{Limit: client.Limit + 1}
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	var requestR, requestW, frameR, frameW *os.File
	var framesDone chan struct{}
	frames := capture{Limit: client.Limit + 1}
	if terminal {
		// Pipes exercise fd3 request EOF and fd4 framing without a real terminal.
		var e error
		requestR, requestW, e = os.Pipe()
		if e != nil {
			return wire{}, e
		}
		defer requestR.Close()
		defer requestW.Close()
		frameR, frameW, e = os.Pipe()
		if e != nil {
			return wire{}, e
		}
		defer frameR.Close()
		defer frameW.Close()
		cmd.ExtraFiles = []*os.File{requestR, frameW}
		cmd.Stdin = strings.NewReader("")
		if op == "launch-exact" {
			cmd.Stdin = bytes.NewReader(data)
		}
	} else {
		cmd.Stdin = bytes.NewReader(data)
	}

	if err := cmd.Start(); err != nil {
		return wire{}, err
	}
	pid := cmd.Process.Pid
	h.mu.Lock()
	h.pids[pid] = true
	h.mu.Unlock()
	if terminal {
		_ = requestR.Close()
		_ = frameW.Close()
		framesDone = make(chan struct{})
		go func() { _, _ = io.Copy(&frames, frameR); close(framesDone) }()
		defer func() { _ = frameR.Close(); <-framesDone }()
		if op == "attach" {
			_, _ = requestW.Write(data)
		}
		_ = requestW.Close()
	}
	err := cmd.Wait()
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	if !gone(pid) || !gone(-pid) {
		return wire{}, errors.New("test child/group survived wait and cleanup")
	}
	h.mu.Lock()
	delete(h.pids, pid)
	h.mu.Unlock()
	result := wire{Body: out.Bytes(), Stderr: stderr.Bytes(), Leak: out.Leak || stderr.Leak}
	if terminal {
		select {
		case <-framesDone:
		case <-time.After(100 * time.Millisecond):
			_ = frameR.Close()
			<-framesDone
		}
		result.Frames = append([]byte{}, frames.Bytes()...)
	}
	if ctx.Err() != nil {
		return wire{}, errors.New("test process exceeded bounded lifetime")
	}
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return wire{}, err
		}
		result.Exit = exit.ExitCode()
	}
	return result, nil
}
func (h *harness) control(r fakemanager.FixtureRequest) (fakemanager.FixtureResponse, error) {
	w, e := h.exec(h.fixture, "", jsonBytes(r), false)
	if e != nil {
		return fakemanager.FixtureResponse{}, e
	}
	var out fakemanager.FixtureResponse
	if w.Exit != 0 || json.Unmarshal(w.Body, &out) != nil || !out.OK {
		return out, errors.New("fixture adapter refused " + r.Operation)
	}
	return out, nil
}
func (h *harness) observe() (*fakemanager.Observation, error) {
	r, e := h.control(fakemanager.FixtureRequest{Operation: "observe"})
	return r.Observation, e
}
func (h *harness) fault(name string) error {
	_, e := h.control(fakemanager.FixtureRequest{Operation: "inject_fault", Name: name})
	return setup(e)
}
func (h *harness) call(op string, r any) (client.Response, error) {
	data := jsonBytes(r)
	w, e := h.exec(h.plugin, op, data, false)
	if e != nil {
		return client.Response{}, setup(e)
	}
	return client.DecodeResponse(op, data, w.Body, w.Exit, h.id)
}
func (h *harness) launch() map[string]any {
	r := clone(h.template)
	r["operation_id"] = operationID()
	r["submitted_at"] = time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	return r
}
func (h *harness) start(r map[string]any) (*client.Session, error) {
	v, e := h.call("launch", r)
	if e != nil {
		return nil, setup(e)
	}
	if v.Session == nil {
		return nil, setup(errors.New("no session positive control"))
	}
	return v.Session, nil
}
func (h *harness) status(s *client.Session) (*client.Session, error) {
	v, e := h.call("status", map[string]any{"session": s.Session})
	return v.Session, e
}
func (h *harness) stop(s *client.Session, mode, pre string) (client.Response, error) {
	return h.call("stop", map[string]any{"session": s.Session, "expected_generation": s.Generation, "mode": mode, "precondition": pre})
}
func (h *harness) resume(s *client.Session) map[string]any {
	r := h.launch()
	r["runtime"] = s.Runtime
	scope := map[string]any{"cwd": r["cwd"], "labels": r["labels"]}
	for k := range r {
		switch k {
		case "operation_id", "submitted_at", "admission_key", "runtime", "profile", "credential_source", "doorbell", "detach":
		default:
			delete(r, k)
		}
	}
	r["scope"], r["selector"] = scope, map[string]string{"kind": "handle", "handle": s.Session}
	return r
}
func (h *harness) all(labels any, limit int) ([]client.Session, error) {
	rows := []client.Session{}
	cursor := ""
	seen := map[string]bool{}
	for pages := 0; pages < 100; pages++ {
		r := map[string]any{"labels": labels, "limit": limit}
		if cursor != "" {
			r["cursor"] = cursor
		}
		v, e := h.call("list", r)
		if e != nil {
			return nil, e
		}
		rows = append(rows, v.Page.Sessions...)
		if v.Page.NextCursor == nil {
			return rows, nil
		}
		cursor = *v.Page.NextCursor
		if seen[cursor] {
			return nil, errors.New("repeated cursor")
		}
		seen[cursor] = true
	}
	return nil, errors.New("pagination did not terminate")
}
func (h *harness) attach(s *client.Session, generation int64) (string, int, error) {
	w, e := h.exec(h.plugin, "attach", jsonBytes(map[string]any{"session": s.Session, "expected_generation": generation}), true)
	if e != nil {
		return "", 0, setup(e)
	}
	code := ""
	e = hosted.DrainStatusRecords(bytes.NewReader(w.Frames), hosted.NormalizeStatusRecord, func(r hosted.StatusRefusal) { code = r.Code })
	return code, w.Exit, e
}
func (h *harness) expectCode(p, want string, err error) error {
	return check(p, client.IsCode(err, want), fmt.Sprintf("expected %s; got %v", want, err))
}
func (h *harness) activity(s *client.Session, state string) error {
	_, e := h.control(fakemanager.FixtureRequest{Operation: "set_activity", Session: s.Session, Activity: state, Enabled: true})
	return setup(e)
}
func (h *harness) corrupt(s *client.Session, kind string) error {
	_, e := h.control(fakemanager.FixtureRequest{Operation: "corrupt_index", Session: s.Session, Kind: kind, Enabled: true})
	return setup(e)
}

// Capture keeps bounded evidence while scanning the complete stream, including
// a sentinel split across writes or beyond the retained prefix.
type capture struct {
	Buffer bytes.Buffer
	Limit  int
	Tail   []byte
	Leak   bool
}

func (c *capture) Len() int      { return c.Buffer.Len() }
func (c *capture) Bytes() []byte { return c.Buffer.Bytes() }
func (c *capture) Write(p []byte) (int, error) {
	const sentinel = "sk-conformance-sentinel"
	joined := append(append([]byte{}, c.Tail...), p...)
	c.Leak = c.Leak || bytes.Contains(joined, []byte(sentinel))
	if len(joined) > len(sentinel)-1 {
		c.Tail = append(c.Tail[:0], joined[len(joined)-len(sentinel)+1:]...)
	} else {
		c.Tail = append(c.Tail[:0], joined...)
	}
	n := len(p)
	left := c.Limit - c.Len()
	if left > n {
		left = n
	}
	if left > 0 {
		_, _ = c.Buffer.Write(p[:left])
	}
	return n, nil
}
