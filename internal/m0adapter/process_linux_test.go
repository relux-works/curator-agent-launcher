//go:build linux

package m0adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Process tests run the real pipeline in a child process of this test
// binary (descriptors are process-global). The child gets exactly the
// descriptors and environment a test chooses; the fake runner is this test
// binary again, reached through the adapter's own execve.

func skipUnderRace(t *testing.T) {
	t.Helper()
	if raceEnabled {
		// The race runtime's own descriptors and environment are outside the
		// adapter's entry contract; these cases run in the non-race test lane.
		t.Skip("process inventory tests run without -race")
	}
}

type childSpec struct {
	plan      []byte     // FD3 contents; nil with noPlan
	noPlan    bool       // deliver no FD3 at all
	planFile  *os.File   // explicit FD3 instead of plan bytes
	extra     []*os.File // descriptors 4..
	stdout    *os.File   // explicit fd 1
	env       []string   // nil: fixedEnv(home)
	exe       string     // E; empty: this test binary
	now1      int64
	now2      int64
	mode      string // empty: adapter child
	behavior  string // test-only literal/syscall mutation in the child
	useHome   string
	planPath  *string // receives the plan path
	planStats *syscall.Stat_t
}

type childResult struct {
	exit   int
	pid    int
	stdout []byte
	stderr []byte
	home   string
	exe    string
}

func testBinary(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

func writePlan(t *testing.T, dir string, b []byte) (*os.File, string) {
	t.Helper()
	path := filepath.Join(dir, "plan.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f, path
}

func runChild(t *testing.T, spec childSpec) childResult {
	t.Helper()
	skipUnderRace(t)
	home := spec.useHome
	if home == "" {
		home = t.TempDir()
	}
	exe := spec.exe
	if exe == "" {
		exe = testBinary(t)
	}
	var files []*os.File
	if !spec.noPlan {
		f := spec.planFile
		if f == nil {
			plan := spec.plan
			if plan == nil {
				plan = []byte(specimen(t))
			}
			var path string
			f, path = writePlan(t, t.TempDir(), plan)
			if spec.planPath != nil {
				*spec.planPath = path
			}
		}
		if spec.planStats != nil {
			if err := syscall.Fstat(int(f.Fd()), spec.planStats); err != nil {
				t.Fatal(err)
			}
		}
		files = append(files, f)
	}
	files = append(files, spec.extra...)
	env := spec.env
	if env == nil {
		env = fixedEnv(home)
	}
	now1, now2 := spec.now1, spec.now2
	if now1 == 0 {
		now1 = 20000
	}
	if now2 == 0 {
		now2 = now1
	}
	mode := spec.mode
	if mode == "" {
		mode = childAdapterArg
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, testBinary(t), mode, exe, home, strconv.FormatInt(now1, 10), strconv.FormatInt(now2, 10), spec.behavior)
	cmd.Env = env
	cmd.ExtraFiles = files
	var stdout, stderr bytes.Buffer
	if spec.stdout != nil {
		cmd.Stdout = spec.stdout
	} else {
		cmd.Stdout = &stdout
	}
	cmd.Stderr = &stderr
	cmd.WaitDelay = 5 * time.Second
	err := cmd.Run()
	var ee *exec.ExitError
	if err != nil && !errors.As(err, &ee) {
		t.Fatalf("child did not run: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("child timed out (stdin EOF missing or a descriptor leaked): %s", stderr.Bytes())
	}
	return childResult{exit: cmd.ProcessState.ExitCode(), pid: cmd.Process.Pid, stdout: stdout.Bytes(), stderr: stderr.Bytes(), home: home, exe: exe}
}

// requireRefusal asserts exit 78, exactly one diagnostic line with the code,
// and no runner start (the fake runner always prints a report).
func requireRefusal(t *testing.T, r childResult, want Reason) {
	t.Helper()
	line := diagnosticPrefix + string(want) + "\n"
	if r.exit != ExitRefusal || string(r.stderr) != line {
		t.Fatalf("exit %d stderr %q, want 78 %q", r.exit, r.stderr, line)
	}
	if len(r.stdout) != 0 {
		t.Fatalf("runner started after refusal: %q", r.stdout)
	}
	if strings.Contains(string(r.stderr), r.home) || strings.Contains(string(r.stderr), "Reply") {
		t.Fatalf("diagnostic echoes a path or prompt bytes")
	}
}

func runnerOutput(t *testing.T, r childResult) runnerReport {
	t.Helper()
	var rep runnerReport
	if err := json.Unmarshal(r.stdout, &rep); err != nil {
		t.Fatalf("no runner report (exit %d, stderr %q): %v", r.exit, r.stderr, err)
	}
	if rep.Error != "" {
		t.Fatalf("fake runner observation failed: %s", rep.Error)
	}
	return rep
}

func expectedArgv(exe, home, account, timeout string) []string {
	j := home + "/tmp/m0/EX-11111111111111111111111111111111"
	return []string{
		exe, "run-codex", "--account", account, "--profile", "codex-refresher-v0",
		"--launch-home", j + "/home", "--model", "catalog-model",
		"--reasoning-effort", "catalog-effort", "--output-last-message", j + "/last-message.txt",
		"--prompt-stdin", "--cwd", home + "/work", "--timeout", timeout,
	}
}

// The specimen execs the runner in the same process (same pid, no adapter
// supervisor) with the exact 19-element argv, the unchanged environment, the
// exact prompt bytes then EOF on stdin, and only descriptors 0-2 inherited,
// none of them the plan object. Status 0 passes through.
func TestProcessSpecimenExecsRunner(t *testing.T) {
	var st syscall.Stat_t
	r := runChild(t, childSpec{planStats: &st})
	rep := runnerOutput(t, r)
	if r.exit != 0 || len(r.stderr) != 0 {
		t.Fatalf("exit %d stderr %q", r.exit, r.stderr)
	}
	if rep.PID != r.pid {
		t.Errorf("runner pid %d, adapter pid %d: exec did not replace the adapter", rep.PID, r.pid)
	}
	if want := expectedArgv(r.exe, r.home, "example", "240"); strings.Join(rep.Args, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("argv:\n got %q\nwant %q", rep.Args, want)
	}
	if strings.Join(rep.Env, "\x00") != strings.Join(fixedEnv(r.home), "\x00") {
		t.Errorf("environment changed: %q", rep.Env)
	}
	if string(rep.Stdin) != "Reply with the word example." || !rep.EOF {
		t.Errorf("stdin %q eof=%v", rep.Stdin, rep.EOF)
	}
	var nums []int
	for _, f := range rep.Inherited {
		nums = append(nums, f.FD)
		if f.Dev == uint64(st.Dev) && f.Ino == uint64(st.Ino) {
			t.Errorf("fd %d in the runner is the plan object", f.FD)
		}
		if f.FD == 0 && f.Mode&syscall.S_IFMT != syscall.S_IFIFO {
			t.Errorf("runner stdin is not the prompt pipe")
		}
	}
	if len(nums) != 3 || nums[0] != 0 || nums[1] != 1 || nums[2] != 2 {
		t.Errorf("runner inherited descriptors %v, want [0 1 2]", nums)
	}
}

// The clock is re-read at exec and T only goes down: a later second reading
// lowers decimal(T) in the runner argv; an earlier one or one leaving under a
// second refuses 78 before exec.
func TestProcessTimeoutReread(t *testing.T) {
	r := runChild(t, childSpec{now1: 20000, now2: 270500})
	rep := runnerOutput(t, r)
	if got := rep.Args[len(rep.Args)-1]; got != "9" || r.exit != 0 {
		t.Fatalf("re-read T %q exit %d", got, r.exit)
	}
	requireRefusal(t, runChild(t, childSpec{now1: 20000, now2: 19999}), ReasonClockRegressed)
	requireRefusal(t, runChild(t, childSpec{now1: 20000, now2: 279100}), ReasonTimeoutInsufficient)
	requireRefusal(t, runChild(t, childSpec{now1: 279100}), ReasonTimeoutInsufficient)
}

// After a successful exec the runner's status is the process status: each
// pinned runner status passes through untouched, with no adapter line.
func TestProcessRunnerStatusPassThrough(t *testing.T) {
	for _, status := range []int{0, 1, 64, 69, 75, 124, 130} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			v := specimenInput()
			v.Account = "status-" + strconv.Itoa(status)
			r := runChild(t, childSpec{plan: []byte(variant(t, v))})
			rep := runnerOutput(t, r)
			if r.exit != status || len(r.stderr) != 0 || rep.PID != r.pid {
				t.Fatalf("exit %d stderr %q, want runner status %d", r.exit, r.stderr, status)
			}
		})
	}
}

// Pipe delivery is byte-exact: a 24576-byte prompt (multi-byte runes and
// LFs) arrives whole, then EOF; a prompt ending in LF keeps exactly that LF
// and nothing is added. No feeder exists, so nothing outlives the exec.
func TestProcessPromptPipe(t *testing.T) {
	for name, prompt := range map[string]string{
		"24576-bytes":  strings.Repeat("é\n", 8192),
		"trailing-lf":  "line one\nline two\n",
		"single-byte":  "x",
		"inner-spaces": "  a \t b  ",
	} {
		t.Run(name, func(t *testing.T) {
			v := specimenInput()
			v.Prompt = prompt
			r := runChild(t, childSpec{plan: []byte(variant(t, v))})
			rep := runnerOutput(t, r)
			if !bytes.Equal(rep.Stdin, []byte(prompt)) || !rep.EOF || r.exit != 0 {
				t.Fatalf("stdin %d bytes eof=%v exit %d, want %d bytes then EOF", len(rep.Stdin), rep.EOF, r.exit, len(prompt))
			}
		})
	}
}

// Any duplicate of the plan kernel object refuses fd_plan_duplicate before
// exec, whatever its number: the same open file at fd 4, a second open of
// the same file at fd 4, and the plan object as fd 1.
func TestProcessPlanDuplicate(t *testing.T) {
	t.Run("same-description-fd4", func(t *testing.T) {
		f, _ := writePlan(t, t.TempDir(), []byte(specimen(t)))
		requireRefusal(t, runChild(t, childSpec{planFile: f, extra: []*os.File{f}}), ReasonFDPlanDuplicate)
	})
	t.Run("second-open-fd4", func(t *testing.T) {
		f, path := writePlan(t, t.TempDir(), []byte(specimen(t)))
		g, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer g.Close()
		requireRefusal(t, runChild(t, childSpec{planFile: f, extra: []*os.File{g}}), ReasonFDPlanDuplicate)
	})
	t.Run("plan-as-stdout", func(t *testing.T) {
		f, _ := writePlan(t, t.TempDir(), []byte(specimen(t)))
		r := runChild(t, childSpec{planFile: f, stdout: f})
		if r.exit != ExitRefusal || string(r.stderr) != diagnosticPrefix+string(ReasonFDPlanDuplicate)+"\n" {
			t.Fatalf("exit %d stderr %q", r.exit, r.stderr)
		}
	})
}

// Entry inventory: an extra unrelated descriptor (file or pipe) refuses
// fd_unexpected; a missing FD3 refuses fd_plan_missing; a writable FD3
// refuses fd_plan_mode; a pipe or directory FD3 refuses fd_plan_type; an
// FD3 over 49152 bytes refuses entry_size_limit without being parsed.
func TestProcessDescriptorInventory(t *testing.T) {
	dir := t.TempDir()
	other, err := os.Create(filepath.Join(dir, "other"))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	requireRefusal(t, runChild(t, childSpec{extra: []*os.File{other}}), ReasonFDUnexpected)

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	defer pw.Close()
	requireRefusal(t, runChild(t, childSpec{extra: []*os.File{pw}}), ReasonFDUnexpected)
	requireRefusal(t, runChild(t, childSpec{noPlan: true}), ReasonFDPlanMissing)
	requireRefusal(t, runChild(t, childSpec{planFile: pr}), ReasonFDPlanType)

	d, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	requireRefusal(t, runChild(t, childSpec{planFile: d}), ReasonFDPlanType)

	rwPath := filepath.Join(dir, "rw.json")
	if err := os.WriteFile(rwPath, []byte(specimen(t)), 0o600); err != nil {
		t.Fatal(err)
	}
	rw, err := os.OpenFile(rwPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer rw.Close()
	requireRefusal(t, runChild(t, childSpec{planFile: rw}), ReasonFDPlanMode)

	s := specimen(t)
	requireRefusal(t, runChild(t, childSpec{plan: []byte(strings.Repeat(" ", 49153-len(s)) + s)}), ReasonEntrySizeLimit)
	// Positive neighbor: the 49152-byte spelling of the same entry runs.
	r := runChild(t, childSpec{plan: []byte(strings.Repeat(" ", 49152-len(s)) + s)})
	if rep := runnerOutput(t, r); r.exit != 0 || string(rep.Stdin) != "Reply with the word example." {
		t.Fatalf("49152-byte FD3 exit %d", r.exit)
	}
}

// Environment refusals happen in the real process too, before exec and
// without clearing anything: an extra variable, HOME != H and a deny marker.
func TestProcessEnvironmentRefusals(t *testing.T) {
	home := t.TempDir()
	requireRefusal(t, runChild(t, childSpec{useHome: home, env: append(fixedEnv(home), "CODEX_HOME="+home)}), ReasonEnvUnexpected)
	other := fixedEnv(home)
	other[0] = "HOME=" + home + "/other"
	requireRefusal(t, runChild(t, childSpec{useHome: home, env: other}), ReasonEnvHomeMismatch)
	marked := fixedEnv(home)
	marked[3] = "PATH=/usr/bin:/run/origin.sock"
	requireRefusal(t, runChild(t, childSpec{useHome: home, env: marked}), ReasonEnvDenied)
}

// A malformed entry refuses 78 before exec and the diagnostic names only
// the rule, never the prompt or the station path.
func TestProcessGrammarRefusal(t *testing.T) {
	doc := replaceOnce(t, specimen(t), `"input": {`, `"v": 1,`+"\n  "+`"input": {`)
	requireRefusal(t, runChild(t, childSpec{plan: []byte(doc)}), ReasonEntryDuplicate)
}

// A failed execve (absent or non-executable E) exits 78 exec_failed and no
// runner starts.
func TestProcessExecFailure(t *testing.T) {
	home := t.TempDir()
	requireRefusal(t, runChild(t, childSpec{useHome: home, exe: home + "/absent-broker"}), ReasonExecFailed)
	requireRefusal(t, runChild(t, childSpec{useHome: home, exe: home}), ReasonExecFailed)
}

// The production entry point refuses c_unpublished before any descriptor,
// FD3 or environment work, even with a valid plan and environment: the
// installed artifact cannot launch until the durable C is published.
func TestProcessProductionMainRefuses(t *testing.T) {
	requireRefusal(t, runChild(t, childSpec{mode: childProductionArg}), ReasonCUnpublished)
}
