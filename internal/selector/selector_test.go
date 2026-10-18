//go:build !windows

package selector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

func fake(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "selector")
	cmd := exec.Command("go", "build", "-o", binary, "./testdata/fake")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake: %v %s", err, out)
	}
	return binary
}
func request(t *testing.T) Request {
	t.Helper()
	reg, err := defaults.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	req, err := BuildRequest("codex_cli", "default", t.TempDir(), Flags{}, reg)
	if err != nil {
		t.Fatal(err)
	}
	return req
}
func failure(t *testing.T, err error, code, detail string) {
	t.Helper()
	var f *Failure
	if !errors.As(err, &f) || f.Code != code || (f.Reason != detail && f.RefusalCode != detail) {
		t.Fatalf("failure = %v, want %s/%s", err, code, detail)
	}
}
func response(c defaults.SelectionCandidate) map[string]any {
	return map[string]any{"decision_id": "d/@+1", "runtime": c.Runtime, "model": c.Model, "effort": c.Effort, "weights_id": nil, "refusal": nil}
}
func encode(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// Config is closed, timeout is a bounded integer, and every invalid variation
// below has a valid machine-policy control. User files never confer authority.
func TestConfig(t *testing.T) {
	valid := `{"schema":"curator-run-selector-v1","mode":"enforce","executable":"/bin/selector","argv_prefix":["recommend","json"]}`
	c, err := ParseConfig([]byte(valid))
	if err != nil || c.TimeoutSeconds != 5 {
		t.Fatal(c, err)
	}
	var baseline map[string]any
	_ = json.Unmarshal([]byte(valid), &baseline)
	for _, tc := range []struct {
		name, key string
		value     any
	}{
		{"mode", "mode", "off"}, {"relative", "executable", "selector"}, {"nul", "executable", "/a\x00b"}, {"schema", "schema", "v2"}, {"unknown", "extra", true}, {"timeout-zero", "timeout_seconds", 0}, {"timeout-high", "timeout_seconds", 61}, {"timeout-fraction", "timeout_seconds", 1.5}, {"timeout-null", "timeout_seconds", nil}, {"prefix-long", "argv_prefix", []string{"a", "b", "c", "d", "e"}}, {"prefix-word", "argv_prefix", []string{"--JSON"}}, {"prefix-type", "argv_prefix", "select"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := map[string]any{}
			for k, x := range baseline {
				v[k] = x
			}
			v[tc.key] = tc.value
			if _, err := ParseConfig(encode(t, v)); err == nil {
				t.Fatal("accepted invalid config")
			}
		})
	}
	for _, timeout := range []int{1, 60} {
		v := map[string]any{}
		for k, x := range baseline {
			v[k] = x
		}
		v["timeout_seconds"] = timeout
		if c, e := ParseConfig(encode(t, v)); e != nil || c.TimeoutSeconds != timeout {
			t.Fatal(c, e)
		}
	}
	for _, bad := range []string{strings.Replace(valid, `"mode":"enforce",`, "", 1), valid + "{}", strings.Replace(valid, `"schema":`, `"schema":"duplicate","schema":`, 1)} {
		if _, err := ParseConfig([]byte(bad)); err == nil {
			t.Fatal("accepted malformed config")
		}
	}
	root := t.TempDir()
	loc := configfile.Locations{System: filepath.Join(root, "system"), User: filepath.Join(root, "user"), LegacySystem: filepath.Join(root, "old")}
	for _, d := range []string{loc.System, loc.User, loc.LegacySystem} {
		if err := os.Mkdir(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	resolver := configfile.Resolver{Locations: loc}
	if c, e := Load(resolver); e != nil || c.Mode != "" {
		t.Fatal("absent is not off", c, e)
	}
	write := func(dir string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "selector.json"), []byte(valid), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(loc.System)
	if c, e := Load(resolver); e != nil || c.Mode != "enforce" {
		t.Fatal(c, e)
	}
	write(loc.User)
	if _, e := Load(resolver); e == nil || !strings.Contains(e.Error(), "machine-only") {
		t.Fatal("user authority", e)
	}
	_ = os.Remove(filepath.Join(loc.User, "selector.json"))
	_ = os.Remove(filepath.Join(loc.System, "selector.json"))
	write(loc.LegacySystem)
	var stderr bytes.Buffer
	resolver.Stderr = &stderr
	if _, e := Load(resolver); e != nil || !strings.Contains(stderr.String(), "deprecated config path") {
		t.Fatal("fallback", e, &stderr)
	}
	write(loc.System)
	_ = os.Remove(filepath.Join(loc.LegacySystem, "selector.json"))
	_ = os.Symlink("missing", filepath.Join(loc.LegacySystem, "selector.json"))
	if _, e := Load(resolver); e != nil {
		t.Fatal("inspected shadowed legacy file", e)
	}
	// New-file failure must never fall back to a valid old policy.
	_ = os.Chmod(filepath.Join(loc.System, "selector.json"), 0666)
	if _, e := Load(resolver); e == nil {
		t.Fatal("accepted writable policy")
	}
	_ = os.Remove(filepath.Join(loc.System, "selector.json"))
	_ = os.Symlink("missing", filepath.Join(loc.System, "selector.json"))
	if _, e := Load(resolver); e == nil {
		t.Fatal("accepted symlinked policy")
	}
}

// Request bytes are CCJ-1, carry null plain-launch dispatch fields, preserve
// explicit flags, and are capped before any child can receive them.
func TestRequest(t *testing.T) {
	req := request(t)
	req.Flags.Model = new(string)
	*req.Flags.Model = "explicit"
	data, digest, err := Encode(req)
	if err != nil {
		t.Fatal(err)
	}
	v, err := fragment.ParseJSON(data)
	if err != nil || !bytes.Equal(fragment.Canonical(v), data) || fragment.Digest(v) != digest {
		t.Fatal("noncanonical digest", err)
	}
	for _, key := range []string{"role", "project", "werkstatt", "task"} {
		value, ok := v.Get(key)
		if !ok || value.Kind != fragment.KindNull {
			t.Fatal("plain field", key, value)
		}
	}
	flags, _ := v.Get("flags")
	model, _ := flags.Get("model")
	effort, _ := flags.Get("effort")
	if model.Kind != fragment.KindString || model.Str != "explicit" || effort.Kind != fragment.KindNull {
		t.Fatal("flags changed", flags)
	}
	if len(req.Candidates) == 0 || !strings.Contains(req.AdmissionSource, "agents-management ") || !strings.HasSuffix(req.AdmissionSource, "lineup") {
		t.Fatal(req)
	}
	req.Profile = strings.Repeat("x", Limit)
	if _, _, err := Encode(req); err == nil {
		t.Fatal("unbounded request")
	}
}

// An admitted tuple with /@+ and valid weights passes; unknown fields, malformed
// grammars, duplicate members and non-candidates fail with the exact diagnosis.
func TestResponse(t *testing.T) {
	candidate := defaults.SelectionCandidate{Runtime: "runtime/@+", Model: "model/@+", Effort: stringPointer("effort/@+")}
	good := response(candidate)
	weight := "gguf-sha256:" + strings.Repeat("a", 64)
	good["weights_id"] = weight
	if d, e := ParseResponse(encode(t, good), []defaults.SelectionCandidate{candidate}); e != nil || d.WeightsID == nil || *d.WeightsID != weight {
		t.Fatal(d, e)
	}
	for _, tc := range []struct {
		name, key string
		value     any
	}{
		{"outside", "model", "other"}, {"runtime-outside", "runtime", "other"}, {"effort-outside", "effort", "other"}, {"unknown", "unknown", 1}, {"bad-id", "decision_id", "bad\nforged"}, {"long-id", "decision_id", strings.Repeat("a", 129)}, {"empty", "effort", ""}, {"null", "model", nil}, {"bad-weight", "weights_id", "gguf-sha256:ABC"}, {"unknown-refusal", "refusal", map[string]any{"code": "no_candidate", "extra": true}}, {"bad-code", "refusal", map[string]any{"code": "UPPER"}},
		{"empty-code", "refusal", map[string]any{"code": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := map[string]any{}
			for k, x := range good {
				v[k] = x
			}
			v[tc.key] = tc.value
			_, err := ParseResponse(encode(t, v), []defaults.SelectionCandidate{candidate})
			failure(t, err, CodeUnavailable, "invalid")
		})
	}
	refusal := response(candidate)
	refusal["refusal"] = map[string]string{"code": "no_qualified_candidate"}
	for _, k := range []string{"decision_id", "runtime", "model", "effort", "weights_id"} {
		refusal[k] = nil
	}
	_, err := ParseResponse(encode(t, refusal), nil)
	failure(t, err, CodeAbstained, "no_qualified_candidate")
	for _, data := range [][]byte{[]byte(`{}`), append(encode(t, good), []byte(`{}`)...), []byte(`{"decision_id":"a","decision_id":"b"}`), bytes.Repeat([]byte(" "), Limit+1)} {
		_, err := ParseResponse(data, []defaults.SelectionCandidate{candidate})
		failure(t, err, CodeUnavailable, "invalid")
	}
}

// The real binary transport accepts a valid response and fails closed on every
// configured failure, with a success longer than the one-second hang control. Stderr is never reflected in errors. The helper exits at
// 20 seconds regardless of transport behavior; no process survives test cleanup.
func TestEnforce(t *testing.T) {
	binary := fake(t)
	req := request(t)
	for _, mode := range []string{"valid", "slow-valid", "atbound", "hang", "exit", "invalid", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			t.Cleanup(func() { pid := pidFile(t, binary); _ = syscall.Kill(-pid, syscall.SIGKILL); assertGone(t, pid) })
			timeout := 10
			if mode == "hang" {
				timeout = 1
			}
			c := Config{Executable: binary, ArgvPrefix: []string{mode}, TimeoutSeconds: timeout}
			start := time.Now()
			d, err := Enforce(context.Background(), c, req)
			if mode == "valid" || mode == "slow-valid" || mode == "atbound" {
				if err != nil || d.DecisionID != "decision-1" {
					t.Fatal(d, err)
				}
			} else {
				reason := map[string]string{"hang": "timeout", "exit": "exit", "invalid": "invalid", "oversize": "invalid"}[mode]
				failure(t, err, CodeUnavailable, reason)
			}
			if mode == "hang" && time.Since(start) > 3*time.Second {
				t.Fatal("deadline exceeded")
			}
			if err != nil && strings.Contains(err.Error(), "private") {
				t.Fatal("stderr leaked")
			}
			assertGone(t, pidFile(t, binary))
		})
	}
	c := Config{Executable: filepath.Join(t.TempDir(), "missing"), TimeoutSeconds: 1}
	_, err := Enforce(context.Background(), c, req)
	failure(t, err, CodeUnavailable, "exit")
}
func pidFile(t *testing.T, binary string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(filepath.Dir(binary), "selector.pid"))
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	if _, err := fmt.Sscan(string(data), &pid); err != nil {
		t.Fatal(err)
	}
	return pid
}
func assertGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("selector PID %d survived cleanup", pid)
}

// The test re-exec entry point only accepts its inherited request descriptor.
func TestShadowWorkerProcess(t *testing.T) {
	if os.Getenv("SELECTOR_TEST_WORKER") == "1" {
		time.AfterFunc(25*time.Second, func() { os.Exit(98) })
		os.Exit(RunWorker(os.NewFile(3, "request")))
	}
}

// Detached workers return immediately even for a hanging/crashing selector,
// inherit no terminal, and write correlated decisions on success. All workers
// and fake children are killed by recorded group/PID and proven gone in Cleanup.
func TestShadow(t *testing.T) {
	binary := fake(t)
	req := request(t)
	for _, mode := range []string{"valid", "hang", "exit", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "selector", "observations.ndjson")
			t.Setenv("SELECTOR_TEST_WORKER", "1")
			exe, _ := os.Executable()
			job := ShadowJob{Config: Config{Executable: binary, ArgvPrefix: []string{mode}, TimeoutSeconds: 1}, Request: req, Observations: path}
			_ = os.Remove(filepath.Join(filepath.Dir(binary), "selector.pid"))
			start := time.Now()
			child, err := StartShadow(job, exe, []string{"-test.run=^TestShadowWorkerProcess$"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = syscall.Kill(child.PID, syscall.SIGTERM)
				select {
				case <-child.Done:
				case <-time.After(2 * time.Second):
					t.Fatal("worker cleanup deadline")
				}
				assertGone(t, child.PID)
				assertGone(t, -child.PID)
				fakePID := pidFile(t, binary)
				_ = syscall.Kill(fakePID, syscall.SIGKILL)
				assertGone(t, fakePID)
			})
			if time.Since(start) > time.Second {
				t.Fatal("launch waited for selector")
			}
			deadline := time.Now().Add(3 * time.Second)
			for {
				if data, e := os.ReadFile(filepath.Join(filepath.Dir(binary), "selector.pid")); e == nil && len(data) > 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("selector did not start")
				}
				time.Sleep(10 * time.Millisecond)
			}
			data, e := os.ReadFile(filepath.Join(filepath.Dir(binary), "process.json"))
			if e != nil {
				t.Fatal(e)
			}
			var meta struct {
				PGID     int  `json:"pgid"`
				SID      int  `json:"sid"`
				Terminal bool `json:"terminal"`
			}
			if json.Unmarshal(data, &meta) != nil || meta.PGID != pidFile(t, binary) || meta.SID != child.PID || meta.Terminal {
				t.Fatal("inherited terminal/session", string(data))
			}
			if mode == "hang" {
				return
			}
			select {
			case err := <-child.Done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("worker did not finish")
			}
			data, err = os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var record Observation
			if json.Unmarshal(bytes.TrimSpace(data), &record) != nil {
				t.Fatal(string(data))
			}
			_, digest, _ := Encode(req)
			if record.Kind != "decision" || record.RequestDigest != digest {
				t.Fatal(record)
			}
			if mode == "valid" && (record.Decision == nil || record.Decision.DecisionID != "decision-1") {
				t.Fatal(record)
			}
			if mode == "oversize" && record.Error != "invalid" {
				t.Fatal(record)
			}
			if mode == "exit" && record.Error != "exit" {
				t.Fatal(record)
			}
		})
	}
}

// Concurrent detached-writer appends remain complete JSON lines; symlink destinations are
// refused without touching their target. This is a local filesystem assertion.
func TestObservations(t *testing.T) {
	path := StatePath(t.TempDir(), "")
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := AppendWait(path, Observation{Kind: "observation", RequestDigest: "sha256:abc", Outcome: "launched"}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(lines) != 12 {
		t.Fatal(len(lines))
	}
	for _, line := range lines {
		var v Observation
		if json.Unmarshal(line, &v) != nil || v.Outcome != "launched" {
			t.Fatal(string(line))
		}
	}
	target := filepath.Join(t.TempDir(), "target")
	_ = os.WriteFile(target, []byte("untouched"), 0600)
	link := filepath.Join(t.TempDir(), "link")
	_ = os.Symlink(target, link)
	if Append(link, Observation{}) == nil {
		t.Fatal("wrote symlink")
	}
	data, _ = os.ReadFile(target)
	if string(data) != "untouched" {
		t.Fatal(string(data))
	}
	if StatePath("", "/home/user") != "/home/user/.local/state/swarma/launcher/selector/observations.ndjson" {
		t.Fatal("state fallback")
	}
}

func stringPointer(s string) *string { return &s }

// A missing effort axis is null only for that exact admitted tuple. Empty
// strings and null on a required axis are invalid. The same binary transport
// also accepts the published punctuation and rejects valid JSON over 64 KiB.
func TestEnforceNullAndGrammar(t *testing.T) {
	binary := fake(t)
	t.Cleanup(func() {
		pid := pidFile(t, binary)
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
		assertGone(t, pid)
	})
	req := request(t)
	candidate := defaults.SelectionCandidate{Runtime: "runtime/@+", Model: "model/@+", Effort: nil}
	req.Candidates = []defaults.SelectionCandidate{candidate}
	c := Config{Executable: binary, TimeoutSeconds: 10}
	for _, tc := range []struct {
		name   string
		effort any
		want   string
	}{
		{"null", nil, ""}, {"empty", "", "invalid"}, {"non-null", "high", "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := response(candidate)
			resp["effort"] = tc.effort
			if err := os.WriteFile(filepath.Join(filepath.Dir(binary), "response.json"), encode(t, resp), 0600); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				pid := pidFile(t, binary)
				_ = syscall.Kill(-pid, syscall.SIGKILL)
				assertGone(t, pid)
				assertGone(t, -pid)
			})
			d, e := Enforce(t.Context(), c, req)
			if tc.want == "" {
				if e != nil || d.Effort != nil || d.Model != candidate.Model {
					t.Fatal(d, e)
				}
			} else {
				failure(t, e, CodeUnavailable, tc.want)
			}
		})
	}
	req.Candidates[0].Effort = stringPointer("effort/@+")
	if err := os.WriteFile(filepath.Join(filepath.Dir(binary), "response.json"), encode(t, response(req.Candidates[0])), 0600); err != nil {
		t.Fatal(err)
	}
	d, e := Enforce(t.Context(), c, req)
	if e != nil || d.Effort == nil || *d.Effort != "effort/@+" {
		t.Fatal(d, e)
	}
	assertGone(t, pidFile(t, binary))
	bad := response(req.Candidates[0])
	bad["effort"] = nil
	_, e = ParseResponse(encode(t, bad), req.Candidates)
	failure(t, e, CodeUnavailable, "invalid")
	valid := encode(t, response(req.Candidates[0]))
	padded := append(valid, bytes.Repeat([]byte(" "), Limit-len(valid))...)
	if _, e := ParseResponse(padded, req.Candidates); e != nil {
		t.Fatal(e)
	}
	_, e = ParseResponse(append(padded, ' '), req.Candidates)
	failure(t, e, CodeUnavailable, "invalid")
}

// Dispatch enrichment remains explicit and serializable; invalid UTF-8 and a
// relative cwd are refused rather than repaired before canonical hashing.
func TestRequestEnrichment(t *testing.T) {
	req := request(t)
	req.Role = stringPointer("developer")
	req.Project = stringPointer("project")
	req.Werkstatt = stringPointer("station")
	req.Task = &Task{Class: "code.implement", Difficulty: stringPointer("high"), Language: stringPointer("go")}
	data, _, err := Encode(req)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"class":"code.implement"`)) || !bytes.Contains(data, []byte(`"id":null`)) {
		t.Fatal(string(data))
	}
	req.Flags.Model = stringPointer("bad\xff")
	if _, _, err := Encode(req); err == nil {
		t.Fatal("repaired invalid UTF-8")
	}
	req.Flags.Model = nil
	req.CWD = "relative"
	if _, _, err := Encode(req); err == nil {
		t.Fatal("relative cwd")
	}
}

// A detached writer waits at most five seconds on contention and drops its line;
// after unlocking, the same record succeeds. FIFO and symlink destinations fail
// promptly without modifying their targets, including a poisoned counter path.
func TestObservationLockBoundAndFileTypes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observations.ndjson")
	f, err := openAppend(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	record := Observation{Kind: "decision"}
	start := time.Now()
	if err := AppendWait(path, record); err == nil {
		t.Fatal("accepted contended append")
	}
	if elapsed := time.Since(start); elapsed < 5*time.Second || elapsed > 7*time.Second {
		t.Fatal("worker lock bound", elapsed)
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) != 0 {
		t.Fatal("dropped line changed observations", err)
	}
	data, err = os.ReadFile(filepath.Join(filepath.Dir(path), "dropped"))
	if err != nil || string(data) != "1\n" {
		t.Fatal("missing drop count", string(data), err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	if err := AppendWait(path, record); err != nil {
		t.Fatal("uncontended worker", err)
	}
	for _, kind := range []string{"fifo", "symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "target")
			if err := os.WriteFile(target, []byte("preserved"), 0600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "observations.ndjson")
			var err error
			switch kind {
			case "fifo":
				err = syscall.Mkfifo(path, 0600)
			case "symlink":
				err = os.Symlink(target, path)
			case "directory":
				err = os.Mkdir(path, 0700)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(dir, "dropped")); err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			if err := Append(path, record); err == nil {
				t.Fatal("accepted non-regular path")
			}
			if time.Since(start) > time.Second {
				t.Fatal("non-regular append blocked")
			}
			data, err := os.ReadFile(target)
			if err != nil || string(data) != "preserved" {
				t.Fatal("modified symlink target", err)
			}
		})
	}
}

// A locked best-effort counter is itself skipped promptly; once released,
// subsequent dropped lines increment the existing decimal count exactly.
func TestObservationDroppedCounterContention(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "observations.ndjson")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	counter := filepath.Join(dir, "dropped")
	if err := os.WriteFile(counter, []byte("8\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := openRegular(counter, os.O_RDWR)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := Append(path, Observation{}); err == nil {
		t.Fatal("accepted directory")
	}
	if time.Since(start) > time.Second {
		t.Fatal("counter blocked append")
	}
	data, err := os.ReadFile(counter)
	if err != nil || string(data) != "8\n" {
		t.Fatal("locked counter changed", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	if err := Append(path, Observation{}); err == nil {
		t.Fatal("accepted directory")
	}
	data, err = os.ReadFile(counter)
	if err != nil || string(data) != "9\n" {
		t.Fatal("counter did not increment", err, string(data))
	}
}

// A refusal requires all four choice members to be null, even if the supplied
// tuple is admitted. Null-only refusals (with or without an id) still abstain;
// an ordinary admitted choice remains the paired positive control.
func TestResponseRefusalChoice(t *testing.T) {
	candidate := defaults.SelectionCandidate{Runtime: "runtime", Model: "admitted", Effort: stringPointer("high")}
	candidates := []defaults.SelectionCandidate{candidate}
	if _, err := ParseResponse(encode(t, response(candidate)), candidates); err != nil {
		t.Fatal("valid choice", err)
	}
	refusal := map[string]any{"decision_id": nil, "runtime": nil, "model": nil, "effort": nil, "weights_id": nil, "refusal": map[string]string{"code": "no_candidate"}}
	for _, id := range []any{nil, "decision-1"} {
		refusal["decision_id"] = id
		_, err := ParseResponse(encode(t, refusal), candidates)
		failure(t, err, CodeAbstained, "no_candidate")
	}
	for _, tc := range []struct {
		name   string
		fields map[string]any
	}{
		{"runtime", map[string]any{"runtime": candidate.Runtime}},
		{"model", map[string]any{"model": candidate.Model}},
		{"effort", map[string]any{"effort": *candidate.Effort}},
		{"weights", map[string]any{"weights_id": "gguf-sha256:" + strings.Repeat("a", 64)}},
		{"admitted-tuple", map[string]any{"runtime": candidate.Runtime, "model": candidate.Model, "effort": *candidate.Effort}},
		{"outside-tuple", map[string]any{"runtime": candidate.Runtime, "model": "outside", "effort": *candidate.Effort}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := map[string]any{}
			for key, value := range refusal {
				v[key] = value
			}
			for key, value := range tc.fields {
				v[key] = value
			}
			_, err := ParseResponse(encode(t, v), candidates)
			failure(t, err, CodeUnavailable, "invalid")
		})
	}
}
