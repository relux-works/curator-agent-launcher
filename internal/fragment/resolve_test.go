package fragment

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeRunner is the deterministic Runner injected at the subprocess
// boundary. It records what Resolve asked for and replays a scripted
// outcome. Calls counts invocations: SPEC §4.1 forbids retries, so it must
// be exactly one after any Resolve.
type fakeRunner struct {
	stdout   string
	stderr   string
	exit     int
	startErr error

	calls  int
	binary string
	argv   []string
	dir    string
	env    []string
}

func (f *fakeRunner) Run(_ context.Context, binary string, argv []string, dir string, env []string, stderr io.Writer) ([]byte, int, error) {
	f.calls++
	f.binary, f.argv, f.dir, f.env = binary, argv, dir, env
	if f.startErr != nil {
		return nil, -1, f.startErr
	}
	_, _ = io.WriteString(stderr, f.stderr)
	return []byte(f.stdout), f.exit, nil
}

func piFragment(t *testing.T) string {
	t.Helper()
	return string(readFile(t, "testdata/a0/pi.json"))
}

func resolveWith(t *testing.T, fr *fakeRunner, req Request) (*Fragment, *ResolveError, string) {
	t.Helper()
	var errOut strings.Builder
	if req.Stderr == nil {
		req.Stderr = &errOut
	}
	r := NewWithRunner("curator", fr)
	f, err := r.Resolve(context.Background(), req)
	if fr.calls != 1 {
		t.Fatalf("runner called %d times; §4.1 forbids retries", fr.calls)
	}
	if err == nil {
		return f, nil, errOut.String()
	}
	re, ok := IsResolve(err)
	if !ok {
		t.Fatalf("error is not a ResolveError: %v", err)
	}
	return nil, re, errOut.String()
}

func TestArgvExactOrderAndRepair(t *testing.T) {
	cases := []struct {
		req  Request
		want []string
	}{
		{Request{EnvID: "pi"}, []string{"env", "resolve", "pi", "--repair", "--format", "json"}},
		{Request{EnvID: "codex_cli", Profile: "companyA", ProfileSet: true}, []string{"env", "resolve", "codex_cli", "--profile", "companyA", "--repair", "--format", "json"}},
		// Operands are passed intact, never re-tokenized or re-quoted.
		{Request{EnvID: "pi", Profile: "has space --repair", ProfileSet: true}, []string{"env", "resolve", "pi", "--profile", "has space --repair", "--repair", "--format", "json"}},
		{Request{EnvID: "--weird", Profile: "-x", ProfileSet: true}, []string{"env", "resolve", "--weird", "--profile", "-x", "--repair", "--format", "json"}},
	}
	for _, c := range cases {
		if got := strings.Join(c.req.Argv(), "\x00"); got != strings.Join(c.want, "\x00") {
			t.Errorf("Argv(%+v) = %q want %q", c.req, c.req.Argv(), c.want)
		}
		fr := &fakeRunner{stdout: piFragment(t)}
		req := c.req
		req.Dir = "/tmp/work"
		req.Env = []string{"A=1", "PATH=/x"}
		resolveWith(t, fr, req)
		if fr.binary != "curator" || strings.Join(fr.argv, "\x00") != strings.Join(c.want, "\x00") {
			t.Errorf("subprocess got %s %q", fr.binary, fr.argv)
		}
		if fr.dir != "/tmp/work" || strings.Join(fr.env, ",") != "A=1,PATH=/x" {
			t.Errorf("dir/env not propagated: %q %q", fr.dir, fr.env)
		}
	}
}

func TestResolveSuccessForwardsWarnings(t *testing.T) {
	// E6: a warning on stderr with exit 0 is forwarded verbatim and does not
	// fail the resolve.
	warn := "warning: environment_tool_version_unverified: pi detected 0.84.3, recorded 0.84.2\n"
	fr := &fakeRunner{stdout: piFragment(t), stderr: warn}
	f, re, errOut := resolveWith(t, fr, Request{EnvID: "pi"})
	if re != nil {
		t.Fatalf("unexpected failure %v", re)
	}
	if errOut != warn {
		t.Errorf("stderr forwarded %q want %q", errOut, warn)
	}
	if f.Digest != "sha256:c0512f558f8dc93780288db597a1a8250c0403a175c95fe988e0e8c0991701bf" || f.Home() != "/Users/iv/.curator/environments/default/pi" {
		t.Errorf("fragment %+v", f)
	}
}

func TestInheritedEnvironmentWhenUnset(t *testing.T) {
	t.Setenv("FRAGMENT_TEST_MARKER", "present")
	fr := &fakeRunner{stdout: piFragment(t)}
	resolveWith(t, fr, Request{EnvID: "pi"})
	found := false
	for _, kv := range fr.env {
		if kv == "FRAGMENT_TEST_MARKER=present" {
			found = true
		}
	}
	if !found {
		t.Error("nil Request.Env must inherit the launcher's environment")
	}
}

func TestNonZeroExitMapping(t *testing.T) {
	cases := []struct {
		name, stderr string
		exit         int
		wantCode     string
		wantCurator  string
	}{
		{"environment_unknown", "curator: environment_unknown: unregistered environment \"nope\"\n", 1, CodeEnvironmentUnknown, "environment_unknown"},
		{"profile_unknown", "curator: profile_unknown: profile \"x\" is not installed\n", 1, CodeProfileUnknown, "profile_unknown"},
		{"profile_unknown no current", "curator: profile_unknown: no profile is current\n", 1, CodeProfileUnknown, "profile_unknown"},
		{"environment_repair_failed", "curator: environment_repair_failed: store entry missing\n", 1, CodeRepairFailed, "environment_repair_failed"},
		{"environment_lock_unavailable", "curator: environment_lock_unavailable: mutation lock held\n", 1, CodeLockUnavailable, "environment_lock_unavailable"},
		// The code line is found after a warning line.
		{"warning then code", "warning: environment_tool_version_unverified: x\ncurator: profile_unknown: y\n", 1, CodeProfileUnknown, "profile_unknown"},
		// Unreachable under --repair: falls to invocation_failed, not stale.
		{"environment_home_stale", "curator: environment_home_stale: drifted\n", 1, CodeInvocationFailed, "environment_home_stale"},
		{"unknown code", "curator: something_else: detail\n", 1, CodeInvocationFailed, "something_else"},
		{"no code line", "panic: boom\n", 2, CodeInvocationFailed, ""},
		{"empty stderr", "", 1, CodeInvocationFailed, ""},
		{"code-like text not at line start", "note curator: profile_unknown: x\n", 1, CodeInvocationFailed, ""},
		{"code token with uppercase", "curator: Profile_Unknown: x\n", 1, CodeInvocationFailed, ""},
		{"code line without detail colon", "curator: profile_unknown\n", 1, CodeInvocationFailed, ""},
		// A recognizable code line on a *successful* fragment does not matter:
		// exit status decides, and stdout that parses is a fragment.
	}
	for _, c := range cases {
		fr := &fakeRunner{stdout: "", stderr: c.stderr, exit: c.exit}
		f, re, errOut := resolveWith(t, fr, Request{EnvID: "pi"})
		if f != nil || re == nil {
			t.Errorf("%s: expected failure, got fragment", c.name)
			continue
		}
		if re.Code != c.wantCode || re.CuratorCode != c.wantCurator || re.ExitCode != c.exit {
			t.Errorf("%s: got code=%s curator=%q exit=%d; want %s %q %d", c.name, re.Code, re.CuratorCode, re.ExitCode, c.wantCode, c.wantCurator, c.exit)
		}
		if errOut != c.stderr {
			t.Errorf("%s: stderr not forwarded verbatim: %q", c.name, errOut)
		}
		if !strings.Contains(re.Error(), re.Code) {
			t.Errorf("%s: message %q lacks code", c.name, re.Error())
		}
	}
}

func TestNonZeroExitNeverYieldsFragmentEvenWithValidStdout(t *testing.T) {
	fr := &fakeRunner{stdout: piFragment(t), stderr: "curator: environment_repair_failed: x\n", exit: 1}
	f, re, _ := resolveWith(t, fr, Request{EnvID: "pi"})
	if f != nil || re == nil || re.Code != CodeRepairFailed {
		t.Fatalf("got %v %v", f, re)
	}
}

func TestStartFailureIsInvocationFailed(t *testing.T) {
	cause := errors.New("exec: \"curator\": executable file not found in $PATH")
	fr := &fakeRunner{startErr: cause}
	f, re, _ := resolveWith(t, fr, Request{EnvID: "pi"})
	if f != nil || re == nil || re.Code != CodeInvocationFailed || re.ExitCode != -1 || !errors.Is(re, cause) {
		t.Fatalf("got %v %+v", f, re)
	}
}

func TestInvalidOutputIsFragmentInvalidNeverAbsence(t *testing.T) {
	pi := piFragment(t)
	cases := map[string]string{
		"empty stdout":        "",
		"whitespace stdout":   "\n",
		"prose":               "resolved ok\n",
		"malformed json":      pi[:len(pi)-3],
		"unknown field":       strings.Replace(pi, `"environment":"pi",`, `"environment":"pi","composition":[],`, 1),
		"duplicate key":       strings.Replace(pi, `"environment":"pi",`, `"environment":"pi","environment":"pi",`, 1),
		"two fragments":       pi + pi,
		"wrong environment":   strings.Replace(pi, `"pi"`, `"codex_cli"`, 1),
		"env value for other": strings.Replace(strings.Replace(pi, `"environment":"pi"`, `"environment":"codex_cli"`, 1), `PI_CODING_AGENT_DIR`, `CODEX_HOME`, 1),
		"warning on stdout":   "warning: x\n" + pi,
		"null":                "null\n",
	}
	for name, out := range cases {
		fr := &fakeRunner{stdout: out, exit: 0}
		f, re, _ := resolveWith(t, fr, Request{EnvID: "pi"})
		if f != nil || re == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if re.Code != CodeFragmentInvalid || re.ExitCode != 0 {
			t.Errorf("%s: code=%s exit=%d", name, re.Code, re.ExitCode)
		}
	}
	// The mismatch case names the environment asked for and the one found.
	fr := &fakeRunner{stdout: strings.Replace(strings.Replace(pi, `"environment":"pi"`, `"environment":"codex_cli"`, 1), `PI_CODING_AGENT_DIR`, `CODEX_HOME`, 1)}
	_, re, _ := resolveWith(t, fr, Request{EnvID: "pi"})
	if re == nil || !strings.Contains(re.Detail, `"codex_cli"`) || !strings.Contains(re.Detail, `"pi"`) {
		t.Errorf("mismatch detail %q", re)
	}
}

func TestRequestShapeGuards(t *testing.T) {
	r := NewWithRunner("curator", &fakeRunner{stdout: piFragment(t)})
	for name, req := range map[string]Request{"empty env": {}, "profile set empty": {EnvID: "pi", ProfileSet: true}} {
		_, err := r.Resolve(context.Background(), req)
		re, ok := IsResolve(err)
		if !ok || re.Code != CodeInvocationFailed {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, ok := IsResolve(errors.New("other")); ok {
		t.Error("IsResolve matched a foreign error")
	}
}

// --- real subprocess boundary -------------------------------------------

// fakeCurator installs an executable named "curator" in a fresh directory
// that records its argv, cwd and selected environment to recordPath and
// then replays stdout/stderr/exit from files beside it. It returns the
// directory to put on PATH.
func fakeCurator(t *testing.T, stdout, stderr string, exit int) (dir, recordPath string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake curator")
	}
	dir = t.TempDir()
	recordPath = filepath.Join(dir, "record.txt")
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("stdout.bin", stdout)
	write("stderr.bin", stderr)
	// Builtins and absolute paths only: the launcher may hand the process an
	// environment whose PATH holds nothing but this directory.
	script := "#!/bin/sh\n" +
		"d=${0%/*}\n" +
		"{ printf 'cwd=%s\\n' \"$PWD\"; printf 'marker=%s\\n' \"$FRAGMENT_FAKE_MARKER\"; for a in \"$@\"; do printf 'arg=%s\\n' \"$a\"; done; } > \"$d/record.txt\"\n" +
		"/bin/cat \"$d/stdout.bin\"\n" +
		"/bin/cat \"$d/stderr.bin\" >&2\n" +
		"exit " + itoa(exit) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "curator"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, recordPath
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

// TestExecRunnerEndToEnd drives the production ExecRunner through New()'s
// binary name against a fake curator found on PATH: argv, cwd and env reach
// the process, stdout is parsed, stderr is forwarded, and the digest matches
// the recorded A0 value.
func TestExecRunnerEndToEnd(t *testing.T) {
	warn := "warning: environment_tool_version_unverified: pi detected 0.84.3, recorded 0.84.2\n"
	dir, record := fakeCurator(t, piFragment(t), warn, 0)
	work := t.TempDir()
	env := []string{"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH"), "FRAGMENT_FAKE_MARKER=m1"}
	t.Setenv("PATH", env[0][len("PATH="):]) // exec looks the binary up in the launcher's PATH

	var errOut strings.Builder
	f, err := New().Resolve(context.Background(), Request{EnvID: "pi", Profile: "default", ProfileSet: true, Stderr: &errOut, Dir: work, Env: env})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if f.Digest != "sha256:c0512f558f8dc93780288db597a1a8250c0403a175c95fe988e0e8c0991701bf" {
		t.Errorf("digest %s", f.Digest)
	}
	if errOut.String() != warn {
		t.Errorf("stderr %q", errOut.String())
	}
	rec := string(readFile(t, record))
	wantCwd, _ := filepath.EvalSymlinks(work)
	gotCwd := strings.TrimPrefix(strings.SplitN(rec, "\n", 2)[0], "cwd=")
	if got, _ := filepath.EvalSymlinks(gotCwd); got != wantCwd {
		t.Errorf("cwd %q want %q", gotCwd, wantCwd)
	}
	wantArgs := "marker=m1\narg=env\narg=resolve\narg=pi\narg=--profile\narg=default\narg=--repair\narg=--format\narg=json\n"
	if !strings.HasSuffix(rec, wantArgs) {
		t.Errorf("record %q lacks %q", rec, wantArgs)
	}
}

func TestExecRunnerNonZeroAndInvalidOutput(t *testing.T) {
	// Non-zero exit with a diagnostic line: mapped, stderr forwarded, no
	// fragment.
	dir, _ := fakeCurator(t, "", "curator: environment_unknown: unregistered environment \"pi\"\n", 1)
	t.Setenv("PATH", dir)
	var errOut strings.Builder
	f, err := New().Resolve(context.Background(), Request{EnvID: "pi", Stderr: &errOut})
	re, ok := IsResolve(err)
	if f != nil || !ok || re.Code != CodeEnvironmentUnknown || re.ExitCode != 1 || !strings.Contains(errOut.String(), "environment_unknown") {
		t.Fatalf("got %v %+v stderr=%q", f, re, errOut.String())
	}
	// Exit zero, output not a fragment: resolve_fragment_invalid, not a
	// launch with no fragment.
	dir2, _ := fakeCurator(t, "{\"fragment\":\"launch-env-fragment-v1\"}\n", "", 0)
	t.Setenv("PATH", dir2)
	f, err = New().Resolve(context.Background(), Request{EnvID: "pi"})
	re, ok = IsResolve(err)
	if f != nil || !ok || re.Code != CodeFragmentInvalid || !IsInvalid(re.Err) {
		t.Fatalf("got %v %+v", f, re)
	}
}

func TestExecRunnerMissingBinaryAndCancellation(t *testing.T) {
	// No curator on PATH: cannot be started.
	t.Setenv("PATH", t.TempDir())
	f, err := New().Resolve(context.Background(), Request{EnvID: "pi"})
	re, ok := IsResolve(err)
	if f != nil || !ok || re.Code != CodeInvocationFailed || re.ExitCode != -1 {
		t.Fatalf("got %v %+v", f, re)
	}
	// A cancelled context is a start failure, never a fragment.
	dir, _ := fakeCurator(t, piFragment(t), "", 0)
	t.Setenv("PATH", dir)
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	f, err = New().Resolve(ctx, Request{EnvID: "pi"})
	re, ok = IsResolve(err)
	if f != nil || !ok || re.Code != CodeInvocationFailed {
		t.Fatalf("cancelled: got %v %+v", f, re)
	}
}
