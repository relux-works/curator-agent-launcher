//go:build unix

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/hosted"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	"golang.org/x/sys/unix"
)

// Client is bound to one registry entry. Environment is the caller snapshot,
// never the composed harness environment; nil captures the current environment.
type Client struct {
	ID          string
	Entry       sessionmanager.Entry
	Environment []string
	CWD         string
}

func Deadline(operation string) time.Duration {
	if operation == "launch" || operation == "resume" {
		return 60 * time.Second
	}
	return 10 * time.Second
}

// PluginEnvironment projects only the protocol allowlist. Missing XDG roots
// are canonicalised through their nearest existing ancestor, without creation.
func PluginEnvironment(caller []string, operation string) ([]string, error) {
	values := map[string]string{}
	for _, s := range caller {
		if k, v, ok := strings.Cut(s, "="); ok {
			values[k] = v
		}
	}
	var result []string
	for _, k := range []string{"PATH", "HOME"} {
		if v, ok := values[k]; ok {
			result = append(result, k+"="+v)
		}
	}
	for _, k := range []string{"XDG_STATE_HOME", "XDG_CONFIG_HOME"} {
		if v, ok := values[k]; ok && filepath.IsAbs(v) {
			p, err := canonicalPath(v)
			if err != nil {
				return nil, unavailable("transport")
			}
			result = append(result, k+"="+p)
		}
	}
	if operation == "attach" {
		if v, ok := values["TERM"]; ok {
			result = append(result, "TERM="+v)
		}
	}
	return append(result, "CURATOR_RUN_PROTOCOL="+Protocol), nil
}
func canonicalPath(p string) (string, error) {
	return canonicalPathDepth(p, 0)
}
func canonicalPathDepth(p string, depth int) (string, error) {
	if depth > 40 {
		return "", errors.New("symlink cycle")
	}
	p = filepath.Clean(p)
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(p)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if st, statErr := os.Lstat(p); statErr == nil && st.Mode()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(p)
			if readErr != nil {
				return "", readErr
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(p), target)
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				target = filepath.Join(target, suffix[i])
			}
			return canonicalPathDepth(target, depth+1)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", err
		}
		suffix = append(suffix, filepath.Base(p))
		p = parent
	}
}
func (c Client) command(ctx context.Context, operation string) (*exec.Cmd, error) {
	if !filepath.IsAbs(c.Entry.Executable) || c.Entry.Protocol != Protocol || !sessionmanager.ValidID(c.ID) {
		return nil, unavailable("transport")
	}
	env := c.Environment
	if env == nil {
		env = os.Environ()
	}
	env, err := PluginEnvironment(env, operation)
	if err != nil {
		return nil, err
	}
	args := append(append([]string{}, c.Entry.ArgvPrefix...), operation)
	cmd := exec.CommandContext(ctx, c.Entry.Executable, args...)
	cmd.Env = env
	cmd.Dir = c.CWD
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 100 * time.Millisecond
	if err := markDescriptorsCloseOnExec(); err != nil {
		return nil, unavailable("transport")
	}
	return cmd, nil
}

// The launcher may itself have inherited descriptors from a shell. CLOEXEC is
// per descriptor in this process, so this preserves the caller's handles while
// preventing ambient inheritance. Go opens its new handles with CLOEXEC; exec
// clears that bit only on the explicit stdio/ExtraFiles child copies.
func markDescriptorsCloseOnExec() error {
	directory, err := os.Open("/dev/fd")
	if err != nil {
		return err
	}
	entries, err := directory.Readdirnames(-1)
	_ = directory.Close()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		fd, err := strconv.Atoi(entry)
		if err != nil || fd < 3 {
			continue
		}
		flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		if errors.Is(err, unix.EBADF) {
			continue
		} // The directory's own, now closed fd.
		if err != nil {
			return err
		}
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_SETFD, flags|unix.FD_CLOEXEC); err != nil && !errors.Is(err, unix.EBADF) {
			return err
		}
	}
	return nil
}
func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

type outputBound struct {
	data     []byte
	overflow bool
	cancel   context.CancelFunc
}

func (b *outputBound) Write(p []byte) (int, error) {
	n := len(p)
	left := Limit - len(b.data)
	if n > left {
		b.data = append(b.data, p[:left]...)
		b.overflow = true
		b.cancel()
	} else {
		b.data = append(b.data, p...)
	}
	return n, nil
}

type stderrCounter struct {
	bytes    int64
	captured []byte
}

func (s *stderrCounter) Write(p []byte) (int, error) {
	s.bytes += int64(len(p))
	left := Limit - len(s.captured)
	s.captured = append(s.captured, p[:min(len(p), left)]...)
	return len(p), nil
}

// Call enforces fixed deadlines, immediate group termination on response
// overflow, bounded drain after a child exits, and discards all stderr text.
func (c Client) Call(ctx context.Context, operation string, request []byte) (Response, error) {
	if err := ValidateRequest(operation, request); err != nil {
		return Response{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, Deadline(operation))
	defer cancel()
	cmd, err := c.command(ctx, operation)
	if err != nil {
		return Response{}, err
	}
	output := &outputBound{cancel: cancel}
	stderr := &stderrCounter{}
	cmd.Stdin = bytes.NewReader(request)
	cmd.Stdout = output
	cmd.Stderr = stderr
	err = cmd.Run()
	// A successful plugin may have started its own daemon with closed stdio.
	// Only an overrun/overflow or a retained transport pipe warrants group kill.
	if ctx.Err() != nil || output.overflow || errors.Is(err, exec.ErrWaitDelay) {
		killGroup(cmd)
	}
	fail := func(e *Error) (Response, error) { e.StderrBytes = stderr.bytes; return Response{}, e }
	if output.overflow {
		return fail(protocolError())
	}
	if ctx.Err() != nil {
		return fail(unavailable("timeout"))
	}
	if cmd.ProcessState == nil {
		return fail(unavailable("transport"))
	}
	if errors.Is(err, exec.ErrWaitDelay) {
		return fail(unavailable("transport"))
	}
	exit := cmd.ProcessState.ExitCode()
	if exit < 0 {
		return fail(unavailable("transport"))
	}
	response, decodeErr := DecodeResponse(operation, request, output.data, exit, c.ID)
	var e *Error
	if errors.As(decodeErr, &e) {
		e.StderrBytes = stderr.bytes
	}
	return response, decodeErr
}

// Start submits a plain launch/resume once. A lost or poisoned answer triggers
// one bounded reconciliation loop; a formal refusal is already terminal.
// Werkstatt callers use Reconciler.Submit instead, to persist correlation first.
func (c Client) Start(ctx context.Context, operation string, request []byte) (*Session, error) {
	if operation != "launch" && operation != "resume" {
		return nil, refusal("launch_plan_invalid", nil)
	}
	response, err := c.Call(ctx, operation, request)
	if IsCode(err, "session_host_unavailable") || IsCode(err, "session_host_protocol_error") {
		var correlation struct {
			OperationID string `json:"operation_id"`
		}
		_ = json.Unmarshal(request, &correlation)
		return ReconcilePlain(ctx, c, correlation.OperationID)
	}
	return response.Session, err
}

// TerminalIO supplies already-open terminal streams. Attach passes the request
// only on fd3; exact passes the private plan on stdin and the terminal on fd3.
// Files remain caller-owned and the launcher's copies stay close-on-exec.
// Stderr files pass through directly; other writers use os/exec's pipe copier.
// command's WaitDelay bounds draining an inherited pipe after the child exits.
type TerminalIO struct {
	Terminal      *os.File
	Stdin, Stdout *os.File
	Stderr        io.Writer
}
type TerminalResult struct {
	Exit    int
	Refusal *Error
}

func (c Client) Terminal(ctx context.Context, operation string, request []byte, streams TerminalIO) (TerminalResult, error) {
	if operation != "attach" && operation != "launch-exact" {
		return TerminalResult{}, protocolError()
	}
	if operation == "attach" {
		if err := ValidateRequest(operation, request); err != nil {
			return TerminalResult{}, err
		}
	} else {
		v, err := fragment.ParseJSON(request)
		if err != nil || v.Kind != fragment.KindObject || len(request) > hosted.LimitWireBytes || depth(v) > 16 {
			return TerminalResult{}, refusal("launch_plan_invalid", nil)
		}
	}
	hosted.MarkCloseOnExec(streams.Terminal)
	if hosted.ValidateTerminal(streams.Terminal) != nil {
		return TerminalResult{}, refusal("session_host_terminal_required", nil)
	}
	cmd, err := c.command(ctx, operation)
	if err != nil {
		return TerminalResult{}, err
	}
	// Lifetime terminal operations remain in the foreground terminal group,
	// matching the existing receiver transport. They have no protocol deadline.
	cmd.SysProcAttr = nil
	cmd.Cancel = func() error { return cmd.Process.Kill() }
	if streams.Stdin == nil {
		streams.Stdin = streams.Terminal
	}
	if streams.Stdout == nil {
		streams.Stdout = streams.Terminal
	}
	if streams.Stderr == nil {
		streams.Stderr = streams.Terminal
	}

	statusR, statusW, err := os.Pipe()
	if err != nil {
		return TerminalResult{}, unavailable("transport")
	}
	defer statusR.Close()
	defer statusW.Close()
	hosted.MarkCloseOnExec(statusR)
	hosted.MarkCloseOnExec(statusW)
	var requestR, requestW *os.File
	cmd.Stdout = streams.Stdout
	cmd.Stderr = streams.Stderr
	if operation == "attach" {
		requestR, requestW, err = os.Pipe()
		if err != nil {
			return TerminalResult{}, unavailable("transport")
		}
		defer requestR.Close()
		defer requestW.Close()
		hosted.MarkCloseOnExec(requestR)
		hosted.MarkCloseOnExec(requestW)
		cmd.Stdin = streams.Stdin
		cmd.ExtraFiles = []*os.File{requestR, statusW}
	} else {
		cmd.Args = append(cmd.Args, "--plan", "-", "--terminal-fd", "3", "--status-fd", "4")
		cmd.Stdin = bytes.NewReader(request)
		cmd.ExtraFiles = []*os.File{streams.Terminal, statusW}
	}
	// As in hosted.Transport, terminal-generated signals reach the foreground
	// receiver directly. The waiting launcher must live to classify its fd4.
	drop := make(chan os.Signal, 4)
	signal.Notify(drop, os.Interrupt, syscall.SIGQUIT)
	defer signal.Stop(drop)
	stopSignals := make(chan struct{})
	defer close(stopSignals)
	go func() {
		for {
			select {
			case <-drop:
			case <-stopSignals:
				return
			}
		}
	}()
	if err = cmd.Start(); err != nil {
		return TerminalResult{}, unavailable("transport")
	}

	_ = statusW.Close()
	var sent chan error
	if requestW != nil {
		_ = requestR.Close()
		sent = make(chan error, 1)
		go func() { _, err := requestW.Write(request); _ = requestW.Close(); sent <- err }()
	}
	// Wait owns process reaping; closing the reader after process completion also
	// prevents an inherited descendant fd4 from retaining this launcher forever.
	done := make(chan error, 1)
	go func() { err := cmd.Wait(); _ = statusW.Close(); done <- err }()
	var last *Error
	statusDone := make(chan error, 1)
	go func() {
		statusErr := hosted.DrainStatusRecords(statusR, hosted.NormalizeStatusRecord, func(normalized hosted.StatusRefusal) {
			last = refusal(normalized.Code, nil)
			if normalized.Field != "" || normalized.Reason != "" {
				last.Details = map[string]string{}
				if normalized.Field != "" {
					last.Details["field"] = normalized.Field
				}
				if normalized.Reason != "" {
					last.Details["reason"] = normalized.Reason
				}
			}
		})
		if statusErr != nil {
			_ = statusR.Close()
			_ = cmd.Process.Kill()
		}
		statusDone <- statusErr
	}()
	waitErr := <-done
	// The plugin has exited. Give the fd4 reader a bounded drain interval before
	// closing the parent pipe; this changes no lifetime-session deadline.
	var statusErr error
	select {
	case statusErr = <-statusDone:
	case <-time.After(100 * time.Millisecond):
		_ = statusR.Close()
		statusErr = <-statusDone
	}
	if sent != nil {
		_ = requestW.Close()
		if err := <-sent; err != nil && last == nil && statusErr == nil {
			return TerminalResult{}, unavailable("transport")
		}
	}
	if statusErr != nil {
		if hosted.StatusProtocolFailure(statusErr) {
			return TerminalResult{}, protocolError()
		}
		return TerminalResult{}, unavailable("transport")
	}
	if last != nil {
		return TerminalResult{Exit: last.ExitCode(), Refusal: last}, nil
	}
	if cmd.ProcessState == nil {
		return TerminalResult{}, unavailable("transport")
	}
	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		return TerminalResult{}, unavailable("transport")
	}
	if waitErr != nil && cmd.ProcessState.ExitCode() < 0 {
		if s, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && s.Signaled() {
			return TerminalResult{Exit: 128 + int(s.Signal())}, nil
		}
	}
	return TerminalResult{Exit: cmd.ProcessState.ExitCode()}, nil
}
func depth(v fragment.Value) int {
	n := 1
	for _, el := range v.Arr {
		n = max(n, 1+depth(el))
	}
	for _, el := range v.Obj {
		n = max(n, 1+depth(el.Value))
	}
	return n
}

// ListAll follows a stable snapshot to its explicit terminal cursor. A failed
// or repeated cursor never yields a partial listing that could imply absence.
func (c Client) ListAll(ctx context.Context, labels map[string]string, limit int) ([]Session, error) {
	if limit == 0 {
		limit = 50
	}
	request := map[string]any{"labels": labels, "limit": limit}
	if labels == nil {
		request["labels"] = map[string]string{}
	}
	seen := map[string]bool{}
	var sessions []Session
	for {
		data, _ := json.Marshal(request)
		response, err := c.Call(ctx, "list", data)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, response.Page.Sessions...)
		if response.Page.NextCursor == nil {
			return sessions, nil
		}
		cursor := *response.Page.NextCursor
		if seen[cursor] {
			return nil, protocolError()
		}
		seen[cursor] = true
		request["cursor"] = cursor
	}
}

var _ io.Writer = (*stderrCounter)(nil)
