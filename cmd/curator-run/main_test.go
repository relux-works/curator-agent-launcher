package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

// run is the production dispatch site: main exits with its return value.
// These tests drive SPEC §3 shapes through it end to end: informational
// flags exit 0 on stdout, usage errors exit 2 with the "usage" code line
// and the usage text on stderr, and a parsed launch is refused with exit 1
// because this build carries no composition stage.

func TestRunInformationalFlags(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--version"}, {"codex_cli", "--profile", "p", "--help"}, {"--version", "--", "x"}} {
		var out, errOut strings.Builder
		if got := runNoResolve(t, args, &out, &errOut); got != 0 {
			t.Errorf("run(%v) = %d, want 0 (stderr %q)", args, got, errOut.String())
		}
		if !strings.Contains(out.String(), name) || !strings.Contains(out.String(), specVersion) || !strings.Contains(out.String(), buildVersion) {
			t.Errorf("run(%v) stdout %q missing name, build or spec version", args, out.String())
		}
		if errOut.Len() != 0 {
			t.Errorf("run(%v) wrote to stderr: %q", args, errOut.String())
		}
	}
}

func TestRunUsageErrorsExit2(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{}, "missing <env-id>"},
		{[]string{"--"}, "missing <env-id>"},
		{[]string{"codex_cli", "resume"}, "stray operand"},
		{[]string{"codex_cli", "--unknown"}, "unknown flag"},
		{[]string{"codex_cli", "--profile"}, "requires a value"},
		{[]string{"codex_cli", "--profile", "a", "--profile", "b"}, "more than once"},
		{[]string{"pi", "--system-prompt", "prepend"}, "accepts append or replace"},
		{[]string{"codex_cli", "--ax-profile", "bogus"}, "accepts standard or yolo"},
		// This build reads no ax.json, so the integration is never configured:
		// --ax-profile is the §3 untracked usage error at the entry point.
		{[]string{"codex_cli", "--ax-profile", "yolo"}, "not configured"},
		{[]string{"codex_cli", "--name", "bad/name"}, "not a valid ax session name"},
		{[]string{"codex_cli", "--name", strings.Repeat("x", 65)}, "not a valid ax session name"},
		{[]string{"--nope", "--help"}, "unknown flag"},
	}
	for _, c := range cases {
		var out, errOut strings.Builder
		if got := runNoResolve(t, c.args, &out, &errOut); got != 2 {
			t.Errorf("run(%v) = %d, want 2", c.args, got)
		}
		first, _, _ := strings.Cut(errOut.String(), "\n")
		if !strings.HasPrefix(first, name+": usage: ") {
			t.Errorf("run(%v) first stderr line %q is not the usage code line", c.args, first)
		}
		if !strings.Contains(first, c.want) {
			t.Errorf("run(%v) stderr %q missing %q", c.args, first, c.want)
		}
		if !strings.Contains(errOut.String(), "usage: curator-run <env-id>") {
			t.Errorf("run(%v) stderr missing usage text", c.args)
		}
		if out.Len() != 0 {
			t.Errorf("run(%v) wrote to stdout: %q", c.args, out.String())
		}
	}
}

// scriptedRunner replays one subprocess outcome and records the argv it was
// asked to run; it is injected at the fragment package's process boundary.
type scriptedRunner struct {
	stdout, stderr string
	exit           int
	calls          int
	argv           []string
}

func (s *scriptedRunner) Run(_ context.Context, _ string, argv []string, _ string, _ []string, stderr io.Writer) ([]byte, int, error) {
	s.calls++
	s.argv = argv
	_, _ = io.WriteString(stderr, s.stderr)
	return []byte(s.stdout), s.exit, nil
}

// forbiddenRunner fails the test if §4.1 is reached: informational and
// usage invocations must resolve nothing.
type forbiddenRunner struct{ t *testing.T }

func (f forbiddenRunner) Run(context.Context, string, []string, string, []string, io.Writer) ([]byte, int, error) {
	f.t.Fatal("curator was invoked for an invocation that must resolve nothing")
	return nil, 1, nil
}

func runNoResolve(t *testing.T, args []string, stdout, stderr io.Writer) int {
	t.Helper()
	return run(context.Background(), args, stdout, stderr, fragment.NewWithRunner("curator", forbiddenRunner{t}))
}

const piFragmentLine = `{"env":{"PI_CODING_AGENT_DIR":"/Users/iv/.curator/environments/default/pi"},"environment":"pi","fragment":"launch-env-fragment-v1","precedence":{"placement":"winner-last","winner":"higher-weight"},"profile":{"lock_sha256":"726310f80f442428a9a640d2d49ad9b635f22857fb4ad22167832c7c6ff30e19","name":"default"}}` + "\n"

const piDigest = "sha256:c0512f558f8dc93780288db597a1a8250c0403a175c95fe988e0e8c0991701bf"

// TestRunParsedLaunchResolvesThenRefuses: a well-formed launch, including a
// native tail that collides with launcher flags, parses, resolves its
// fragment exactly once through SPEC §4.1 with the exact argv, and is then
// refused with exit 1 — never 2 (not a usage error) and never 0 (nothing
// launched). Curator's stderr warning is forwarded verbatim before the
// refusal line.
func TestRunParsedLaunchResolvesThenRefuses(t *testing.T) {
	warn := "warning: environment_tool_version_unverified: pi detected 0.84.3, recorded 0.84.2\n"
	cases := []struct {
		args     []string
		wantArgv []string
	}{
		{[]string{"pi"}, []string{"env", "resolve", "pi", "--repair", "--format", "json"}},
		{[]string{"pi", "--profile", "default", "--model", "m", "--effort", "high", "--", "resume", "--last"}, []string{"env", "resolve", "pi", "--profile", "default", "--repair", "--format", "json"}},
		{[]string{"pi", "--system-prompt", "append", "--name", "ok-name", "--", "--help", "", "--ax-profile", "yolo", "--profile", "x"}, []string{"env", "resolve", "pi", "--repair", "--format", "json"}},
	}
	for _, c := range cases {
		sr := &scriptedRunner{stdout: piFragmentLine, stderr: warn}
		var out, errOut strings.Builder
		if got := run(context.Background(), c.args, &out, &errOut, fragment.NewWithRunner("curator", sr)); got != 1 {
			t.Errorf("run(%v) = %d, want 1 (stderr %q)", c.args, got, errOut.String())
		}
		if sr.calls != 1 || strings.Join(sr.argv, " ") != strings.Join(c.wantArgv, " ") {
			t.Errorf("run(%v) resolved %d times with argv %q, want once with %q", c.args, sr.calls, sr.argv, c.wantArgv)
		}
		if !strings.HasPrefix(errOut.String(), warn+name+": not_implemented: ") || !strings.Contains(errOut.String(), piDigest) || !strings.Contains(errOut.String(), "/Users/iv/.curator/environments/default/pi") {
			t.Errorf("run(%v) stderr %q: want forwarded warning, then not_implemented with digest and home", c.args, errOut.String())
		}
		if out.Len() != 0 {
			t.Errorf("run(%v) wrote to stdout: %q", c.args, out.String())
		}
	}
}

// TestRunResolveFailuresExit1: every §6 resolve-family outcome reaches the
// operator as one code line with exit 1, after Curator's own stderr.
func TestRunResolveFailuresExit1(t *testing.T) {
	cases := []struct {
		name, stdout, stderr string
		exit                 int
		wantCode             string
	}{
		{"environment_unknown", "", "curator: environment_unknown: unregistered environment \"pi\"\n", 1, "resolve_environment_unknown"},
		{"profile_unknown", "", "curator: profile_unknown: no profile is current\n", 1, "resolve_profile_unknown"},
		{"repair_failed", "", "curator: environment_repair_failed: x\n", 1, "resolve_repair_failed"},
		{"lock_unavailable", "", "curator: environment_lock_unavailable: x\n", 1, "resolve_lock_unavailable"},
		{"home_stale is unexpected", "", "curator: environment_home_stale: x\n", 1, "resolve_invocation_failed"},
		{"no diagnostic", "", "boom\n", 3, "resolve_invocation_failed"},
		{"invalid fragment", "{\"fragment\":\"launch-env-fragment-v1\"}\n", "", 0, "resolve_fragment_invalid"},
		{"empty stdout is not absence", "", "", 0, "resolve_fragment_invalid"},
		{"other environment's fragment", strings.Replace(strings.Replace(piFragmentLine, `"environment":"pi"`, `"environment":"codex_cli"`, 1), "PI_CODING_AGENT_DIR", "CODEX_HOME", 1), "", 0, "resolve_fragment_invalid"},
	}
	for _, c := range cases {
		sr := &scriptedRunner{stdout: c.stdout, stderr: c.stderr, exit: c.exit}
		var out, errOut strings.Builder
		if got := run(context.Background(), []string{"pi"}, &out, &errOut, fragment.NewWithRunner("curator", sr)); got != 1 {
			t.Errorf("%s: exit %d, want 1", c.name, got)
		}
		if sr.calls != 1 {
			t.Errorf("%s: curator run %d times", c.name, sr.calls)
		}
		want := c.stderr + name + ": " + c.wantCode + ": "
		if !strings.HasPrefix(errOut.String(), want) {
			t.Errorf("%s: stderr %q, want prefix %q", c.name, errOut.String(), want)
		}
		if strings.Contains(errOut.String(), "not_implemented") || out.Len() != 0 {
			t.Errorf("%s: a failed resolve must not reach the refusal or stdout: %q %q", c.name, errOut.String(), out.String())
		}
	}
}

// TestRunProductionResolverAgainstFakeCurator drives main's own resolver
// (fragment.New: the real os/exec runner looking "curator" up on PATH)
// against a shell fake, so the whole path from argv to the subprocess and
// back is exercised as production runs it.
func TestRunProductionResolverAgainstFakeCurator(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake curator")
	}
	dir := t.TempDir()
	write := func(name, content string, mode os.FileMode) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	write("stdout.bin", piFragmentLine, 0o644)
	write("stderr.bin", "warning: environment_tool_version_unverified: pi\n", 0o644)
	write("curator", "#!/bin/sh\nd=${0%/*}\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done > \"$d/argv.txt\"\n/bin/cat \"$d/stdout.bin\"\n/bin/cat \"$d/stderr.bin\" >&2\nexit 0\n", 0o755)
	t.Setenv("PATH", dir)

	var out, errOut strings.Builder
	if got := run(context.Background(), []string{"pi", "--profile", "default", "--", "x"}, &out, &errOut, fragment.New()); got != 1 {
		t.Fatalf("exit %d, want 1 (stderr %q)", got, errOut.String())
	}
	argv, err := os.ReadFile(filepath.Join(dir, "argv.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(argv) != "env\nresolve\npi\n--profile\ndefault\n--repair\n--format\njson\n" {
		t.Errorf("subprocess argv %q", argv)
	}
	if !strings.HasPrefix(errOut.String(), "warning: environment_tool_version_unverified: pi\n"+name+": not_implemented: ") || !strings.Contains(errOut.String(), piDigest) {
		t.Errorf("stderr %q", errOut.String())
	}

	// Without curator on PATH the launch fails as resolve_invocation_failed.
	t.Setenv("PATH", t.TempDir())
	errOut.Reset()
	if got := run(context.Background(), []string{"pi"}, &out, &errOut, fragment.New()); got != 1 || !strings.HasPrefix(errOut.String(), name+": resolve_invocation_failed: ") {
		t.Errorf("missing curator: exit %d stderr %q", got, errOut.String())
	}
}

// TestSpecVersionPinned fails when the reported specification version
// drifts from the version SPEC.md and README.md state; the three are one fact.
func TestSpecVersionPinned(t *testing.T) {
	const want = "0.3.0-draft"
	if specVersion != want {
		t.Fatalf("specVersion = %q, want %q", specVersion, want)
	}
	var out, errOut strings.Builder
	if got := runNoResolve(t, []string{"--version"}, &out, &errOut); got != 0 {
		t.Fatalf("run(--version) = %d, want 0", got)
	}
	if out.String() != name+" "+buildVersion+" (specification "+want+")\n" {
		t.Fatalf("run(--version) stdout = %q", out.String())
	}
}

// TestExecutableUnicodePathBoundary uses the built production main and a real
// fake-curator subprocess; the supplied path is data and is never created.
func TestExecutableUnicodePathBoundary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake curator")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "curator-run")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	fake := "#!/bin/sh\nd=${0%/*}\n/bin/cat \"$d/fragment.json\"\n"
	if err := os.WriteFile(filepath.Join(dir, "curator"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		count int
		code  string
	}{
		{"accept4096", 4096, "not_implemented"},
		{"reject4097", 4097, "resolve_fragment_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal([]byte(piFragmentLine), &value); err != nil {
				t.Fatal(err)
			}
			value["env"] = map[string]string{"PI_CODING_AGENT_DIR": "/" + strings.Repeat("é", tc.count-1)}
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "fragment.json"), data, 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(binary, "pi")
			cmd.Env = append(os.Environ(), "PATH="+dir)
			var out, stderr strings.Builder
			cmd.Stdout, cmd.Stderr = &out, &stderr
			err = cmd.Run()
			if e, ok := err.(*exec.ExitError); !ok || e.ExitCode() != 1 {
				t.Fatalf("exit: %v", err)
			}
			if out.Len() != 0 || !strings.HasPrefix(stderr.String(), name+": "+tc.code+": ") {
				t.Fatalf("stdout %q stderr %q", out.String(), stderr.String())
			}
		})
	}
}

// TestRunMapping drives the production entry point with the real fragment
// parser. Unsupported mappings must not reach the later-stage stub.
func TestRunMapping(t *testing.T) {
	for _, tc := range []struct{ env, system, provider string }{
		{"claude_code", "claude-code", "claude"},
		{"codex_cli", "codex", "codex"},
		{"pi", "pi-native", "pi"},
		{"opencode", "", ""},
	} {
		t.Run(tc.env, func(t *testing.T) {
			line := strings.ReplaceAll(piFragmentLine, `"environment":"pi"`, `"environment":"`+tc.env+`"`)
			line = strings.ReplaceAll(line, "PI_CODING_AGENT_DIR", fragment.HomeVariable(tc.env))
			sr := &scriptedRunner{stdout: line}
			var out, stderr strings.Builder
			args := []string{tc.env, "--", "", "--help", "--profile", "native", "--"}
			before := append([]string(nil), args...)
			code := run(context.Background(), args, &out, &stderr, fragment.NewWithRunner("curator", sr))
			if code != 1 || out.Len() != 0 || sr.calls != 1 {
				t.Fatalf("exit=%d stdout=%q resolves=%d", code, out.String(), sr.calls)
			}
			if !slices.Equal(args, before) {
				t.Fatalf("native argv changed: %q", args)
			}
			if tc.system == "" {
				if !strings.HasPrefix(stderr.String(), name+": env_unsupported: ") || strings.Contains(stderr.String(), "not_implemented") {
					t.Fatalf("refusal: %q", stderr.String())
				}
			} else if !strings.HasPrefix(stderr.String(), name+": not_implemented: ") || !strings.Contains(stderr.String(), fmt.Sprintf("mapped system %q / provider %q", tc.system, tc.provider)) {
				t.Fatalf("mapping: %q", stderr.String())
			}
		})
	}
}

// A future resolver may extend its registry. Exercise run's mapping defense
// without weakening today's closed fragment parser to manufacture that state.
type resolvedUnknown struct{}

func (resolvedUnknown) Resolve(context.Context, fragment.Request) (*fragment.Fragment, error) {
	return &fragment.Fragment{Environment: "future_env"}, nil
}
func TestRunUnknownResolvedMapping(t *testing.T) {
	var out, stderr strings.Builder
	if got := run(context.Background(), []string{"future_env"}, &out, &stderr, resolvedUnknown{}); got != 1 {
		t.Fatalf("exit=%d", got)
	}
	if out.Len() != 0 || !strings.HasPrefix(stderr.String(), name+": env_unsupported: ") || strings.Contains(stderr.String(), "not_implemented") {
		t.Fatalf("stdout=%q stderr=%q", out.String(), stderr.String())
	}
}

func TestRunUnknownFragmentStillRefusesResolution(t *testing.T) {
	sr := &scriptedRunner{stdout: strings.ReplaceAll(piFragmentLine, `"environment":"pi"`, `"environment":"future_env"`)}
	var out, stderr strings.Builder
	if got := run(context.Background(), []string{"future_env"}, &out, &stderr, fragment.NewWithRunner("curator", sr)); got != 1 {
		t.Fatalf("exit=%d", got)
	}
	if sr.calls != 1 || out.Len() != 0 || !strings.HasPrefix(stderr.String(), name+": resolve_fragment_invalid: ") {
		t.Fatalf("stderr=%q calls=%d", stderr.String(), sr.calls)
	}
}

// TestRunDiagnosticsContract drives every failure this build can produce
// through the production entry point and pins the SPEC §6 contract there:
// the exit status for the code, exactly one launcher diagnostic code
// line on stderr carrying that code, Curator's stderr forwarded verbatim
// ahead of it (never parsed as a launcher diagnostic), and nothing on
// stdout. Later pipeline families (defaults, plan, exec, ax, mcp,
// sysprompt) are covered at their own production APIs in
// internal/diagnostics; their main call sites are stated obligations,
// not claimed here.
func TestRunDiagnosticsContract(t *testing.T) {
	opencodeLine := strings.ReplaceAll(piFragmentLine, `"environment":"pi"`, `"environment":"opencode"`)
	opencodeLine = strings.ReplaceAll(opencodeLine, "PI_CODING_AGENT_DIR", fragment.HomeVariable("opencode"))
	cases := []struct {
		name     string
		args     []string
		stdout   string
		stderr   string
		exit     int
		wantCode string
		wantExit int
	}{
		{"usage missing env", []string{}, "", "", 0, "usage", 2},
		{"usage stray operand", []string{"codex_cli", "resume"}, "", "", 0, "usage", 2},
		{"usage unknown flag", []string{"codex_cli", "--unknown"}, "", "", 0, "usage", 2},
		{"usage ax-profile untracked", []string{"codex_cli", "--ax-profile", "yolo"}, "", "", 0, "usage", 2},
		{"resolve environment unknown", []string{"pi"}, "", "curator: environment_unknown: unregistered\n", 1, "resolve_environment_unknown", 1},
		{"resolve profile unknown", []string{"pi"}, "", "curator: profile_unknown: none current\n", 1, "resolve_profile_unknown", 1},
		{"resolve repair failed", []string{"pi"}, "", "curator: environment_repair_failed: store\n", 1, "resolve_repair_failed", 1},
		{"resolve lock unavailable", []string{"pi"}, "", "curator: environment_lock_unavailable: busy\n", 1, "resolve_lock_unavailable", 1},
		{"resolve invocation failed", []string{"pi"}, "", "boom\n", 3, "resolve_invocation_failed", 1},
		{"resolve fragment invalid", []string{"pi"}, "{}\n", "", 0, "resolve_fragment_invalid", 1},
		{"env unsupported", []string{"opencode"}, opencodeLine, "", 0, "env_unsupported", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sr := &scriptedRunner{stdout: c.stdout, stderr: c.stderr, exit: c.exit}
			var out, errOut strings.Builder
			resolver := fragment.NewWithRunner("curator", sr)
			if diagnostics.CodeUsage == c.wantCode {
				resolver = fragment.NewWithRunner("curator", forbiddenRunner{t})
			}
			got := run(context.Background(), c.args, &out, &errOut, resolver)
			if got != c.wantExit {
				t.Fatalf("exit = %d, want %d (stderr %q)", got, c.wantExit, errOut.String())
			}
			if got != diagnostics.ExitForCode(c.wantCode) {
				t.Fatalf("exit = %d, want ExitForCode(%q)", got, c.wantCode)
			}
			if out.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", out.String())
			}
			lines := strings.Split(strings.TrimSuffix(errOut.String(), "\n"), "\n")
			found := 0
			for _, line := range lines {
				if diagnostics.IsDiagnosticLine(line) {
					found++
					if !strings.HasPrefix(line, name+": "+c.wantCode+": ") {
						t.Fatalf("diagnostic line %q does not carry code %q", line, c.wantCode)
					}
				}
			}
			if found != 1 {
				t.Fatalf("stderr has %d diagnostic lines, want exactly 1: %q", found, errOut.String())
			}
			if c.stderr != "" && !strings.HasPrefix(errOut.String(), c.stderr) {
				t.Fatalf("stderr %q does not forward curator stderr %q verbatim first", errOut.String(), c.stderr)
			}
		})
	}
}

// TestRunDetailInjectionCannotForgeLine is the one-code-line regression:
// operator bytes that spell a launcher code line still render as exactly
// one diagnostic code line at the production entry point. The resolve
// case is the framing regression: the profile value reaches the detail
// raw through the real fragment Resolver (only the binary is absent).
// The usage case pins the companion path, where the parser's %q-quoted
// echo plus the same framing keep a hostile flag token to one line.
func TestRunDetailInjectionCannotForgeLine(t *testing.T) {
	injected := "normal\ncurator-run: usage: forged"
	t.Run("resolve", func(t *testing.T) {
		var out, errOut strings.Builder
		resolver := fragment.NewWithRunner(
			filepath.Join(t.TempDir(), "missing-curator"), fragment.ExecRunner{})
		got := run(context.Background(),
			[]string{"pi", "--profile", injected}, &out, &errOut, resolver)
		if got != 1 {
			t.Fatalf("exit = %d, want 1 (stderr %q)", got, errOut.String())
		}
		assertSingleDiagnostic(t, errOut.String(), "resolve_invocation_failed")
		if out.Len() != 0 {
			t.Fatalf("stdout = %q, want empty", out.String())
		}
	})
	t.Run("usage", func(t *testing.T) {
		var out, errOut strings.Builder
		got := runNoResolve(t,
			[]string{"pi", "--bad\ncurator-run: usage: forged"}, &out, &errOut)
		if got != 2 {
			t.Fatalf("exit = %d, want 2 (stderr %q)", got, errOut.String())
		}
		assertSingleDiagnostic(t, errOut.String(), "usage")
		if out.Len() != 0 {
			t.Fatalf("stdout = %q, want empty", out.String())
		}
	})
}

// assertSingleDiagnostic requires exactly one launcher diagnostic code
// line carrying wantCode; every other stderr line — forwarded transport,
// usage text, framed continuations — must not parse as one.
func assertSingleDiagnostic(t *testing.T, stderr, wantCode string) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
	found := 0
	for _, line := range lines {
		if diagnostics.IsDiagnosticLine(line) {
			found++
			if !strings.HasPrefix(line, name+": "+wantCode+": ") {
				t.Fatalf("diagnostic line %q does not carry code %q", line, wantCode)
			}
		}
	}
	if found != 1 {
		t.Fatalf("stderr has %d diagnostic lines, want exactly 1: %q", found, stderr)
	}
}

func TestRunResolveFailurePrecedesMapping(t *testing.T) {
	sr := &scriptedRunner{exit: 1, stderr: "curator: environment_unknown: missing\n"}
	var out, stderr strings.Builder
	if got := run(context.Background(), []string{"opencode"}, &out, &stderr, fragment.NewWithRunner("curator", sr)); got != 1 {
		t.Fatalf("exit=%d", got)
	}
	if sr.calls != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), name+": resolve_environment_unknown: ") || strings.Contains(stderr.String(), "env_unsupported") || strings.Contains(stderr.String(), "not_implemented") {
		t.Fatalf("stderr=%q calls=%d", stderr.String(), sr.calls)
	}
}
