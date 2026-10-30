// Package execution implements the SPEC §4.6 process API over admitted inputs.
// It does not select defaults, admit plans, or implement §5 prompt policy.
package execution

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
)

// Boundary is REQUIRED on every Run, including launches without an engaged
// prompt. Integration must bind the real §5 file-kind probe here. Errors are
// terminal and emitted before any process; there is no success default.
type Boundary func() error

type IO struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// Options controls the native child streams and late validation boundary.
// Started, when set, runs once right after the child process has started,
// before the launcher waits for it; it must not block.
type Options struct {
	IO       IO
	Boundary Boundary
	Started  func()
}

// Launch is a prepared native execution snapshot. Run rechecks files.
type Launch struct{ value composition.Value }

// Prepare snapshots the admitted composition, retaining no mutable execution inputs.
func Prepare(v composition.Value) Launch {
	v.Argv = append([]string{}, v.Argv...)
	v.Env = append([]string{}, v.Env...)
	v.RawStdin.Bytes = bytes.Clone(v.RawStdin.Bytes)
	v.Warnings = append([]string{}, v.Warnings...)
	return Launch{value: v}
}

// Run waits for the actual child. Direct mode shares supplied terminal file
// descriptors (defaults: os.Stdin/out/err), never allocates a PTY or replaces the
// launcher. Attached stdin uses exactly the plan bytes. Exit status is preserved,
// with signal exits returned as 128+signal. The caller must exit with this code.
// The child owns a separate foreground process group when a controlling terminal
// is available. Parent-only signals are relayed to that group. Child stops
// suspend the launcher, and continuation restores the child terminal ownership.
func (l Launch) Run(opts Options) int {
	streams := opts.IO
	if streams.Stdin == nil {
		streams.Stdin = os.Stdin
	}
	if streams.Stdout == nil {
		streams.Stdout = os.Stdout
	}
	if streams.Stderr == nil {
		streams.Stderr = os.Stderr
	}
	fail := func(err error) int {
		code, detail, ok := strings.Cut(err.Error(), ": ")
		if !ok {
			code, detail = diagnostics.CodePlanRefused, err.Error()
		}
		_ = diagnostics.Emit(streams.Stderr, code, detail)
		return diagnostics.ExitForCode(code)
	}
	for _, warning := range l.value.Warnings {
		fmt.Fprintln(streams.Stderr, "curator-run: "+warning)
	}
	if opts.Boundary == nil {
		return fail(errors.New("launch_boundary_required: §5 boundary callback is required"))
	}
	// Build transport before the late checks to minimize the pathname race.
	cmd := &exec.Cmd{Args: append([]string{l.value.Binary}, l.value.Argv...), Env: append([]string{}, l.value.Env...), Stdin: streams.Stdin, Stderr: streams.Stderr}
	if l.value.RawStdin.Attached {
		cmd.Stdin = bytes.NewReader(l.value.RawStdin.Bytes)
	}
	cmd.Dir = l.value.WorkDir
	cmd.Stdout = streams.Stdout
	// SPEC §4.6 order: the binary check first, then the §4.5 codex layer
	// stat, then the §5 boundary (the §5.1 file-kind probe); the first
	// failure is the one reported.
	binary, err := executable(l.value.Binary, l.value.WorkDir, l.value.Env)
	if err != nil {
		return fail(fmt.Errorf("exec_provider_missing: %s: install the provider executable and make it available on the launch PATH: %w", l.value.Binary, err))
	}
	if err := l.value.CheckLaunchBoundary(); err != nil {
		return fail(err)
	}
	if err := opts.Boundary(); err != nil {
		return fail(err)
	}
	cmd.Path = binary
	err = run(cmd, opts.Started)
	if err == nil {
		return 0
	}
	var exit *processExit
	if errors.As(err, &exit) {
		if status := exit.status; status.Signaled() {
			return 128 + int(status.Signal())
		}
		return exit.ExitCode()
	}
	return fail(fmt.Errorf("exec_provider_missing: %s: cannot execute provider: %w", l.value.Binary, err))
}

// Resolve a bare name against the FULL composed environment, not an unrelated
// parent PATH. Relative entries and explicit paths are relative to WorkDir.
func executable(binary, workdir string, env []string) (string, error) {
	check := func(path string) (string, error) {
		if !filepath.IsAbs(path) {
			path = filepath.Join(workdir, path)
		}
		path, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return "", os.ErrPermission
		}
		return path, nil
	}
	if strings.ContainsRune(binary, '/') {
		return check(binary)
	}
	for _, entry := range env {
		if path, ok := strings.CutPrefix(entry, "PATH="); ok {
			for _, dir := range filepath.SplitList(path) {
				if found, err := check(filepath.Join(dir, binary)); err == nil {
					return found, nil
				}
			}
		}
	}
	return "", exec.ErrNotFound
}
