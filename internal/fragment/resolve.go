package fragment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// CuratorBinary is the executable the production resolver starts, looked up
// on PATH.
const CuratorBinary = "curator"

// Diagnostic codes of the §6 resolve family.
const (
	CodeInvocationFailed   = "resolve_invocation_failed"
	CodeEnvironmentUnknown = "resolve_environment_unknown"
	CodeProfileUnknown     = "resolve_profile_unknown"
	CodeRepairFailed       = "resolve_repair_failed"
	CodeLockUnavailable    = "resolve_lock_unavailable"
	CodeFragmentInvalid    = "resolve_fragment_invalid"
)

// curatorCodes maps Curator's own diagnostics (environments.md §10.4) to
// the resolve family (SPEC §4.1). environment_home_stale is deliberately
// absent: it cannot arise from a --repair invocation, and a Curator that
// reports it falls under resolve_invocation_failed like any unexpected exit.
var curatorCodes = map[string]string{
	"environment_unknown":          CodeEnvironmentUnknown,
	"profile_unknown":              CodeProfileUnknown,
	"environment_repair_failed":    CodeRepairFailed,
	"environment_lock_unavailable": CodeLockUnavailable,
}

// ResolveError is one §6 resolve-family failure. Code is the stable
// diagnostic; Detail is the human-oriented text; CuratorCode is the code
// read from Curator's stderr line when one was recognized; ExitCode is the
// subprocess exit status, or -1 when it did not start; Err is the
// underlying cause when there is one (a start error or a validity error).
type ResolveError struct {
	Code        string
	Detail      string
	CuratorCode string
	ExitCode    int
	Err         error
}

func (e *ResolveError) Error() string { return e.Code + ": " + e.Detail }

// Unwrap exposes the cause to errors.Is / errors.As.
func (e *ResolveError) Unwrap() error { return e.Err }

// Request is one resolution. EnvID is passed verbatim as the operand;
// Profile, when ProfileSet, is passed verbatim as the --profile value.
// Stderr receives every byte Curator wrote to its stderr, on success and on
// failure alike (E6: warnings such as environment_tool_version_unverified
// arrive there with exit 0). Dir and Env are the subprocess working
// directory and environment; empty means inherit the launcher's own.
type Request struct {
	EnvID      string
	Profile    string
	ProfileSet bool
	Stderr     io.Writer
	Dir        string
	Env        []string
}

// Argv returns the exact subprocess argument vector of SPEC §4.1, without
// the executable: env resolve <env-id> [--profile <name>] --repair --format
// json, in that order. --repair is unconditional.
func (r Request) Argv() []string {
	argv := []string{"env", "resolve", r.EnvID}
	if r.ProfileSet {
		argv = append(argv, "--profile", r.Profile)
	}
	return append(argv, "--repair", "--format", "json")
}

// Runner starts one subprocess and reports what it produced. Production
// uses ExecRunner; tests inject a deterministic runner at exactly this
// boundary. A Runner returns startErr only when the process could not be
// started; a process that ran and exited non-zero is a normal return with
// its exit status.
type Runner interface {
	Run(ctx context.Context, binary string, argv []string, dir string, env []string, stderr io.Writer) (stdout []byte, exit int, startErr error)
}

// ExecRunner is the production Runner: os/exec with stdout captured and
// stderr streamed verbatim to the requested writer.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, binary string, argv []string, dir string, env []string, stderr io.Writer) ([]byte, int, error) {
	cmd := exec.CommandContext(ctx, binary, argv...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = nil
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = stderr
	err := cmd.Run()
	if err == nil {
		return out.Bytes(), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() >= 0 {
		return out.Bytes(), exitErr.ExitCode(), nil
	}
	if ctx.Err() != nil {
		return out.Bytes(), -1, ctx.Err()
	}
	return out.Bytes(), -1, err
}

// Resolver runs SPEC §4.1. The zero value is not usable; use New.
type Resolver struct {
	binary string
	runner Runner
}

// New returns the production resolver: it starts CuratorBinary from PATH
// through ExecRunner.
func New() *Resolver {
	return &Resolver{binary: CuratorBinary, runner: ExecRunner{}}
}

// NewWithRunner returns a resolver that starts binary through runner. It is
// the single injection point for tests; production code calls New.
func NewWithRunner(binary string, runner Runner) *Resolver {
	return &Resolver{binary: binary, runner: runner}
}

// Resolve obtains the fragment for req (SPEC §4.1). It runs the subprocess
// exactly once — no retry, no fragment-less fallback — forwards Curator's
// stderr verbatim to req.Stderr, and returns either a validated Fragment
// with its digest or a *ResolveError:
//
//   - the process could not be started (curator missing from PATH, or the
//     context was cancelled before or during the run): resolve_invocation_failed;
//   - non-zero exit whose stderr carries a recognized `curator: <code>: …`
//     line: the §4.1 mapping of that code;
//   - any other non-zero exit, environment_home_stale included:
//     resolve_invocation_failed;
//   - exit zero with stdout that is not one valid closed fragment naming
//     req.EnvID as its environment: resolve_fragment_invalid. Malformed
//     output is a read failure, never an absence.
func (r *Resolver) Resolve(ctx context.Context, req Request) (*Fragment, error) {
	if req.EnvID == "" {
		return nil, &ResolveError{Code: CodeInvocationFailed, ExitCode: -1, Detail: "no environment identifier to resolve"}
	}
	if req.ProfileSet && req.Profile == "" {
		return nil, &ResolveError{Code: CodeInvocationFailed, ExitCode: -1, Detail: "--profile given without a name"}
	}
	stderr := req.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	// Curator's stderr is both forwarded live and captured for the code line.
	var captured bytes.Buffer
	env := req.Env
	if env == nil {
		env = os.Environ()
	}
	stdout, exit, startErr := r.runner.Run(ctx, r.binary, req.Argv(), req.Dir, env, io.MultiWriter(stderr, &captured))
	if startErr != nil {
		return nil, &ResolveError{
			Code:     CodeInvocationFailed,
			ExitCode: -1,
			Detail:   fmt.Sprintf("could not run %s %s: %v", r.binary, strings.Join(req.Argv(), " "), startErr),
			Err:      startErr,
		}
	}
	if exit != 0 {
		code, detail := curatorDiagnostic(captured.Bytes())
		re := &ResolveError{Code: CodeInvocationFailed, CuratorCode: code, ExitCode: exit}
		if mapped, ok := curatorCodes[code]; ok {
			re.Code = mapped
			re.Detail = fmt.Sprintf("curator env resolve exited %d: %s: %s", exit, code, detail)
		} else if code != "" {
			re.Detail = fmt.Sprintf("curator env resolve exited %d with unmapped diagnostic %s: %s", exit, code, detail)
		} else {
			re.Detail = fmt.Sprintf("curator env resolve exited %d without a curator diagnostic line", exit)
		}
		return nil, re
	}
	f, err := Parse(stdout)
	if err != nil {
		return nil, &ResolveError{Code: CodeFragmentInvalid, ExitCode: 0, Detail: "curator env resolve exited 0 but its output is not a valid launch-env-fragment-v1: " + err.Error(), Err: err}
	}
	if f.Environment != req.EnvID {
		return nil, &ResolveError{Code: CodeFragmentInvalid, ExitCode: 0, Detail: fmt.Sprintf("fragment names environment %q, resolve was asked for %q", f.Environment, req.EnvID)}
	}
	return f, nil
}

// curatorDiagnostic finds the first stderr line of the form
// `curator: <code>: <detail>` and returns the code and detail. The code is
// an identifier of lowercase letters, digits, and underscores; any other
// line — warnings included — is skipped. Absent, both are empty.
func curatorDiagnostic(stderr []byte) (code, detail string) {
	for _, line := range strings.Split(string(stderr), "\n") {
		rest, ok := strings.CutPrefix(line, "curator: ")
		if !ok {
			continue
		}
		c, d, found := strings.Cut(rest, ":")
		if !found || c == "" || !isCodeToken(c) {
			continue
		}
		return c, strings.TrimSpace(d)
	}
	return "", ""
}

func isCodeToken(s string) bool {
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

// IsResolve returns the *ResolveError in err, if any.
func IsResolve(err error) (*ResolveError, bool) {
	var re *ResolveError
	if errors.As(err, &re) {
		return re, true
	}
	return nil, false
}
