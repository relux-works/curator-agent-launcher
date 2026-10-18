//go:build !windows

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/selector"
)

func selectorBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "selector")
	if out, err := exec.Command("go", "build", "-o", binary, "../../internal/selector/testdata/fake").CombinedOutput(); err != nil {
		t.Fatalf("build fake: %v %s", err, out)
	}
	return binary
}
func selectorPolicy(t *testing.T, f *pipelineFixture, binary, mode, behavior string) {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"schema": selector.Schema, "mode": mode, "executable": binary, "argv_prefix": []string{behavior}, "timeout_seconds": 1})
	machine := filepath.Dir(f.deps.defaults.Machine)
	if err := os.MkdirAll(machine, 0700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(machine, "selector.json"), data, 0600)
}
func selectorResponse(t *testing.T, binary string, response string) {
	t.Helper()
	writeFixture(t, filepath.Join(filepath.Dir(binary), "response.json"), []byte(response), 0600)
}

const selectedResponse = `{"decision_id":"decision/@+1","runtime":"codex","model":"gpt-5.6-sol","effort":"high","weights_id":null,"refusal":null}`

// Production calls the fake selector before building a plan, applies per-member
// flag precedence, and prints its decision id in the existing provenance group.
func TestRunSelectorEnforce(t *testing.T) {
	binary := selectorBinary(t)
	for _, tc := range []struct {
		name          string
		flags         []string
		model, effort string
	}{
		{"selector", nil, "gpt-5.6-sol", "high"}, {"model-flag", []string{"--model", "gpt-6-astra"}, "gpt-6-astra", "high"}, {"effort-flag", []string{"--effort", "low"}, "gpt-5.6-sol", "low"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := entryFixture(t, "codex_cli", false)
			f.args = append([]string{"codex_cli", "--permissions", "native"}, tc.flags...)
			selectorPolicy(t, f, binary, "enforce", "valid")
			selectorResponse(t, binary, selectedResponse)
			cleanupSelectorFake(t, binary)
			code, _, stderr := f.run()
			if code != 0 || f.builds != 1 || string(f.request.Model) != tc.model || f.request.Effort != tc.effort || f.request.Runtime != "codex" || !bytes.Contains(stderr, []byte("selector=decision/@+1")) {
				t.Fatalf("exit %d req %+v stderr %s", code, f.request, stderr)
			}
			data, err := os.ReadFile(filepath.Join(filepath.Dir(binary), "request.json"))
			if err != nil {
				t.Fatal(err)
			}
			var req selector.Request
			if json.Unmarshal(data, &req) != nil || req.CWD != f.dir || req.Profile != "default" || req.EnvID != "codex_cli" || req.Task != nil || req.Werkstatt != nil {
				t.Fatal(string(data))
			}
			if tc.name == "model-flag" {
				if req.Flags.Model == nil || *req.Flags.Model != "gpt-6-astra" {
					t.Fatal("lost model flag", req.Flags)
				}
			} else if req.Flags.Model != nil {
				t.Fatal("invented model flag", req.Flags)
			}
			if tc.name == "effort-flag" {
				if req.Flags.Effort == nil || *req.Flags.Effort != "low" {
					t.Fatal("lost effort flag", req.Flags)
				}
			} else if req.Flags.Effort != nil {
				t.Fatal("invented effort flag", req.Flags)
			}

		})
	}
}

// Enforcing failures are terminal before plan/provider contact and never fall
// back to valid defaults. Valid choices above provide the transport control.
func TestRunSelectorRefusals(t *testing.T) {
	binary := selectorBinary(t)
	for _, tc := range []struct{ name, behavior, response, diagnostic string }{
		{"outside", "valid", strings.Replace(selectedResponse, "gpt-5.6-sol", "unadmitted", 1), "selector_unavailable: selector unavailable reason=invalid"},
		{"unknown", "valid", strings.Replace(selectedResponse, `"refusal":null`, `"refusal":null,"extra":true`, 1), "selector_unavailable: selector unavailable reason=invalid"},
		{"abstained", "valid", `{"decision_id":null,"runtime":null,"model":null,"effort":null,"weights_id":null,"refusal":{"code":"no_qualified_candidate"}}`, "selector_abstained: selector abstained code=no_qualified_candidate"},
		{"timeout", "hang", "", "selector_unavailable: selector unavailable reason=timeout"}, {"exit", "exit", "", "selector_unavailable: selector unavailable reason=exit"}, {"invalid", "invalid", "", "selector_unavailable: selector unavailable reason=invalid"}, {"oversize", "oversize", "", "selector_unavailable: selector unavailable reason=invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := entryFixture(t, "codex_cli", false)
			selectorPolicy(t, f, binary, "enforce", tc.behavior)
			selectorResponse(t, binary, tc.response)
			cleanupSelectorFake(t, binary)
			code, out, stderr := f.run()
			if code != 1 || f.builds != 0 || f.verdicts != 0 || len(out) != 0 || !bytes.Contains(stderr, []byte(tc.diagnostic)) || bytes.Contains(stderr, []byte("private child failure")) {
				t.Fatalf("exit %d builds %d stderr %s", code, f.builds, stderr)
			}
		})
	}
}

// Configure a real detached production worker with a test-owned fake binary.
// Every child has a <=20s helper deadline and cleanup records/kills its group,
// waits for it, then proves neither worker nor fake survived the test.
func shadowFixture(t *testing.T, f *pipelineFixture, binary, worker, behavior string) string {
	t.Helper()
	selectorPolicy(t, f, binary, "shadow", behavior)
	path := filepath.Join(t.TempDir(), "selector", "observations.ndjson")
	f.deps.observationPath = func() string { return path }
	f.deps.shadowStart = func(job selector.ShadowJob) (*selector.Child, error) {
		child, err := selector.StartShadow(job, worker, []string{selector.WorkerArgument})
		if err != nil {
			return child, err
		}
		t.Cleanup(func() {
			_ = syscall.Kill(child.PID, syscall.SIGTERM)
			select {
			case <-child.Done:
			case <-time.After(3 * time.Second):
				t.Fatal("worker did not exit")
			}
			assertSelectorGone(t, child.PID)
			assertSelectorGone(t, -child.PID)
			data, e := os.ReadFile(filepath.Join(filepath.Dir(binary), "selector.pid"))
			if e == nil {
				var pid int
				if json.Unmarshal(data, &pid) == nil {
					_ = syscall.Kill(pid, syscall.SIGKILL)
					assertSelectorGone(t, pid)
					assertSelectorGone(t, -pid)
				}
			}
		})
		return child, nil
	}
	return path
}
func assertSelectorGone(t *testing.T, pid int) {
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
func selectorWorker(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "curator-run")
	if out, err := exec.Command("go", "build", "-o", path, ".").CombinedOutput(); err != nil {
		t.Fatalf("worker build: %v %s", err, out)
	}
	return path
}

// A real hanging/crashing selector leaves the composed launch and stdout/stderr
// unchanged and cannot delay exit. The sidecar records exact applied values,
// canonical request correlation and a launcher refusal after late-bound checks.
func TestRunSelectorShadow(t *testing.T) {
	worker := selectorWorker(t)
	for _, behavior := range []string{"hang", "exit", "valid"} {
		t.Run(behavior, func(t *testing.T) {
			binary := selectorBinary(t)
			f := entryFixture(t, "codex_cli", false)
			insertLauncherArgs(f, "--permissions", "native")
			code, offOut, offErr := f.run()
			if code != 0 {
				t.Fatal(code, string(offErr))
			}
			path := shadowFixture(t, f, binary, worker, behavior)
			f.deps.stdin = strings.NewReader("parent stdin\n\x00\xff")
			start := time.Now()
			code, out, stderr := f.run()
			if code != 0 || !bytes.Equal(out, offOut) || !bytes.Equal(stderr, offErr) || time.Since(start) > 2*time.Second {
				t.Fatalf("shadow changed launch: exit %d stderr %s", code, stderr)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
				var record selector.Observation
				if json.Unmarshal(line, &record) != nil {
					t.Fatal(string(data))
				}
				if record.Kind == "observation" {
					found = true
					if record.Outcome != "launched" || record.Runtime != "codex" || record.Model != "gpt-6-astra" || record.Effort != "medium" || record.WeightsID != nil || !strings.HasPrefix(record.RequestDigest, "sha256:") {
						t.Fatal(record)
					}
				}
			}
			if !found {
				t.Fatal("missing observation")
			}
			// Wait for the fake to actually enter its requested behavior, not a stale PID.
			deadline := time.Now().Add(3 * time.Second)
			for {
				if _, err := os.Stat(filepath.Join(filepath.Dir(binary), "selector.pid")); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("fake never started")
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

// System-only selector authority is validated before information/fragment
// resolution; removing the user policy restores the seam-off launch control.
func TestRunSelectorUserPolicy(t *testing.T) {
	deps, runner, loc := configHomeFixture(t)
	writeConfig(t, loc.User, "selector.json", `{"schema":"curator-run-selector-v1","mode":"enforce","executable":"/abs/selector","argv_prefix":[]}`)
	var out, stderr bytes.Buffer
	code := run(t.Context(), []string{"--version"}, &out, &stderr, deps)
	if code != 1 || runner.calls != 0 || !bytes.Contains(stderr.Bytes(), []byte("defaults_config_invalid")) {
		t.Fatal(code, &stderr)
	}
	if err := os.Remove(filepath.Join(loc.User, "selector.json")); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := run(t.Context(), []string{"--version"}, &out, &stderr, deps); code != 0 || stderr.Len() != 0 {
		t.Fatal(code, &stderr)
	}
}

func cleanupSelectorFake(t *testing.T, binary string) {
	t.Helper()
	_ = os.Remove(filepath.Join(filepath.Dir(binary), "selector.pid"))
	t.Cleanup(func() {
		data, e := os.ReadFile(filepath.Join(filepath.Dir(binary), "selector.pid"))
		if e != nil {
			return
		}
		var pid int
		if json.Unmarshal(data, &pid) != nil || pid <= 0 {
			t.Fatal("invalid recorded PID")
		}
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		assertSelectorGone(t, pid)
		assertSelectorGone(t, -pid)
	})
}

// A different candidate runtime is retained only as selector provenance: the
// explicit model keeps the existing Pi contributor binding and effort flag.
func TestRunSelectorRuntimeRecordedOnly(t *testing.T) {
	binary := selectorBinary(t)
	f := entryFixture(t, "pi", false)
	insertLauncherArgs(f, "--permissions", "native")
	selectorPolicy(t, f, binary, "enforce", "valid")
	selectorResponse(t, binary, strings.Replace(selectedResponse, `"runtime":"codex"`, `"runtime":"pi-openai"`, 1))
	cleanupSelectorFake(t, binary)
	code, _, stderr := f.run()
	if code != 0 || f.request.Runtime != "pi-anthropic" || string(f.request.Model) != "claude-opus-5" || f.request.Effort != "high" || !bytes.Contains(stderr, []byte("selector=decision/@+1")) {
		t.Fatalf("exit %d request %+v stderr %s", code, f.request, stderr)
	}
}

// A late launcher refusal is observable without changing its code/output or
// launching the provider; shadow remains independent of the refusal.
func TestRunSelectorShadowRefusal(t *testing.T) {
	binary := selectorBinary(t)
	worker := selectorWorker(t)
	f := entryFixture(t, "codex_cli", false)
	insertLauncherArgs(f, "--permissions", "native")
	path := shadowFixture(t, f, binary, worker, "valid")
	if err := os.Remove(f.layer); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := f.run()
	if code != 1 || len(out) != 0 || !bytes.Contains(stderr, []byte("mcp_layer_missing")) {
		t.Fatal(code, string(stderr))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var record selector.Observation
		if json.Unmarshal(line, &record) != nil {
			t.Fatal(string(data))
		}
		if record.Kind == "observation" {
			found = true
			if record.Outcome != "refused:mcp_layer_missing" {
				t.Fatal(record)
			}
		}
	}
	if !found {
		t.Fatal("missing refusal observation")
	}
}

// Locked explicit flags refuse before external policy executes; without the
// flag, a valid selector still outranks the file default in enforce mode.
func TestRunSelectorLockedFlag(t *testing.T) {
	binary := selectorBinary(t)
	f := entryFixture(t, "codex_cli", false)
	selectorPolicy(t, f, binary, "enforce", "valid")
	selectorResponse(t, binary, selectedResponse)
	cleanupSelectorFake(t, binary)
	writeFixture(t, f.deps.defaults.Machine, []byte(`{"schema":"curator-run-defaults-v1","locked":true,"defaults":{"codex_cli":{"model":"gpt-6-astra","effort":"low"}}}`), 0600)
	f.args = []string{"codex_cli", "--model", "gpt-6-astra", "--permissions", "native"}
	code, _, stderr := f.run()
	if code != 2 || f.builds != 0 || !bytes.Contains(stderr, []byte("overrides locked model")) {
		t.Fatal(code, string(stderr))
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(binary), "selector.pid")); !os.IsNotExist(err) {
		t.Fatal("selector ran before lock refusal", err)
	}
	f.args = []string{"codex_cli", "--permissions", "native"}
	code, _, stderr = f.run()
	if code != 0 || f.builds != 1 || string(f.request.Model) != "gpt-5.6-sol" || f.request.Effort != "high" {
		t.Fatal(code, string(stderr))
	}
}
