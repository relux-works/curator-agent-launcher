// Package execution implements the SPEC §4.6 process API over admitted inputs.
// It does not select defaults, admit plans, or implement §5 prompt policy.
package execution

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/mapping"
)

// Boundary is REQUIRED on every Run, including launches without an engaged
// prompt. Integration must bind the real §5 file-kind probe here. Errors are
// terminal and emitted before any process; there is no success default.
type Boundary func() error

type IO struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// Options controls transport, not admission. AxBinary is an optional explicit
// executable path; empty resolves ax on the launcher's PATH. Tests always set
// it to a fake helper. Ax inherits the launcher environment, never Value.Env.
type Options struct {
	IO       IO
	AxBinary string
	Boundary Boundary
}

type document struct {
	Schema        string             `json:"schema"`
	SchemaVersion string             `json:"schema_version"`
	Argv          []string           `json:"argv_suffix"`
	EnvNames      []string           `json:"env_names"`
	EnvLiterals   map[string]string  `json:"env_literals"`
	Stdin         *composition.Stdin `json:"stdin"`
	Extensions    extensions         `json:"extensions"`
}
type extensions struct {
	ProfileName    string `json:"works.relux.curator.profile-name"`
	ProfilePin     string `json:"works.relux.curator.profile-pin"`
	FragmentDigest string `json:"works.relux.curator.fragment-digest"`
	SystemModules  bool   `json:"works.relux.curator.system-modules"`
}

// Launch is a prepared snapshot. Prepare belongs at composition time, so the
// default UTC name does not drift while the launch waits. Run rechecks files.
type Launch struct {
	value    composition.Value
	tracked  bool
	argv     []string
	document []byte
}

// Prepare consumes the actual Compose result, validated fragment and parsed
// invocation with its mapped target. These are trusted typed pipeline inputs;
// this layer does not reimplement fragment/CLI/admission validation.
func Prepare(v composition.Value, f fragment.Fragment, inv cli.Invocation, target mapping.Target, at time.Time) (Launch, error) {
	// Marshal now both snapshots the tracked transport and prevents later input
	// mutations changing it. Copy direct slices separately for the same reason.
	doc, err := json.Marshal(document{"urn:ax:schema:launch-plan-request", "1.0.0", v.Argv, v.EnvNames, v.EnvLiterals, v.Stdin,
		extensions{f.Profile.Name, "sha256:" + f.Profile.LockSHA256, f.Digest, f.SystemPrompt != nil}})
	if err != nil {
		return Launch{}, err
	}
	v.Argv = append([]string{}, v.Argv...)
	v.Env = append([]string{}, v.Env...)
	v.RawStdin.Bytes = bytes.Clone(v.RawStdin.Bytes)
	v.Warnings = append([]string{}, v.Warnings...)
	name := inv.Name
	if !inv.NameSet {
		name = inv.EnvID + "-" + at.UTC().Format("20060102T150405Z")
	}
	argv := []string{"start", name, "--provider", target.Provider, "--launch-plan", "-"}
	if inv.AxProfile != cli.AxProfileNone {
		argv = append(argv, "--profile", string(inv.AxProfile))
	}
	argv = append(argv, "--workspace", v.WorkDir)
	return Launch{v, inv.Tracked, argv, doc}, nil
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
	fail := func(err error) int { fmt.Fprintln(streams.Stderr, "curator-run: "+err.Error()); return 1 }
	for _, warning := range l.value.Warnings {
		fmt.Fprintln(streams.Stderr, "curator-run: "+warning)
	}
	if opts.Boundary == nil {
		return fail(errors.New("launch_boundary_required: §5 boundary callback is required"))
	}
	// Build transport before the late checks to minimize the pathname race.
	var cmd *exec.Cmd
	var axStderr bytes.Buffer
	if l.tracked {
		binary := opts.AxBinary
		if binary == "" {
			binary = "ax"
		}
		cmd = exec.Command(binary, l.argv...)
		cmd.Stdin = bytes.NewReader(l.document)
		cmd.Stderr = &axStderr
	} else {
		cmd = &exec.Cmd{Args: append([]string{l.value.Binary}, l.value.Argv...), Env: append([]string{}, l.value.Env...), Stdin: streams.Stdin, Stderr: streams.Stderr}
		if l.value.RawStdin.Attached {
			cmd.Stdin = bytes.NewReader(l.value.RawStdin.Bytes)
		}
	}
	cmd.Dir = l.value.WorkDir
	cmd.Stdout = streams.Stdout
	if err := opts.Boundary(); err != nil {
		return fail(err)
	}
	binary, err := executable(l.value.Binary, l.value.WorkDir, l.value.Env)
	if err != nil {
		return fail(fmt.Errorf("exec_provider_missing: %s: install the provider executable and make it available on the launch PATH: %w", l.value.Binary, err))
	}
	if err := l.value.CheckLaunchBoundary(); err != nil {
		return fail(err)
	}
	if !l.tracked {
		cmd.Path = binary
	}
	err = run(cmd)
	if l.tracked {
		if err != nil {
			fail(errors.New("ax_handoff_failed: ax could not take the launch"))
			_, _ = streams.Stderr.Write(axStderr.Bytes())
			return 1
		}
		_, _ = streams.Stderr.Write(axStderr.Bytes())
		return 0
	}
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
