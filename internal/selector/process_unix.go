//go:build !windows

package selector

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// boundedOutput caps stdout; a bound breach kills the process group
// immediately, including inherited pipe writers. Callers discard stderr.
type boundedOutput struct {
	data     []byte
	overflow bool
	cancel   context.CancelFunc
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := Limit - len(b.data)
	if n > remaining {
		b.overflow = true
		b.data = append(b.data, p[:remaining]...)
		if b.cancel != nil {
			b.cancel()
		}
	} else {
		b.data = append(b.data, p...)
	}
	return n, nil
}

// Enforce owns a process group and its pipes until completion or cancellation.
// Child stderr never becomes launcher diagnostics, including failed starts.
func Enforce(ctx context.Context, c Config, request Request) (Decision, error) {
	data, _, err := Encode(request)
	if err != nil {
		return Decision{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.TimeoutSeconds)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Executable, c.ArgvPrefix...)
	cmd.Dir = request.CWD
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 100 * time.Millisecond
	output := &boundedOutput{cancel: cancel}
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	err = cmd.Run()
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	if output.overflow {
		return Decision{}, unavailable("invalid")
	}
	if ctx.Err() != nil {
		return Decision{}, unavailable("timeout")
	}
	if err != nil {
		return Decision{}, unavailable("exit")
	}
	return ParseResponse(output.data, request.Candidates)
}

// WorkerArgument is a private re-exec entry point, not a launch command. Its
// only input is an inherited, unlinked descriptor created by StartShadow.
const WorkerArgument = "--internal-selector-shadow"

type ShadowJob struct {
	Config       Config
	Request      Request
	Observations string
}
type Child struct {
	PID  int
	Done <-chan error
}

// StartShadow does not wait for the worker or selector. There are no pipe-copy
// goroutines or terminal descriptors keeping the launching process alive.
// worker defaults to this executable; argv permits the test binary entry point.
func StartShadow(job ShadowJob, worker string, argv []string) (*Child, error) {
	if _, _, err := Encode(job.Request); err != nil {
		return nil, err
	}
	data, err := json.Marshal(job)
	if err != nil {
		return nil, err
	}
	if worker == "" {
		worker, err = os.Executable()
		if err != nil {
			return nil, err
		}
		argv = []string{WorkerArgument}
	}
	if err = os.MkdirAll(filepath.Dir(job.Observations), 0700); err != nil {
		return nil, err
	}
	input, err := os.CreateTemp(filepath.Dir(job.Observations), ".selector-request-*")
	if err != nil {
		return nil, err
	}
	defer input.Close()
	defer os.Remove(input.Name())
	if _, err = input.Write(data); err != nil {
		return nil, err
	}
	if _, err = input.Seek(0, 0); err != nil {
		return nil, err
	}
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer null.Close()
	cmd := exec.Command(worker, argv...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = null, null, null
	cmd.ExtraFiles = []*os.File{input}
	// A new session is also a new process group, with no controlling terminal.
	// Worker cancellation terminates and reaps its separately grouped selector.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait(); close(done) }()
	return &Child{PID: cmd.Process.Pid, Done: done}, nil
}

// RunWorker runs only in the detached process. Shadow has no selector deadline;
// timeout_seconds is exclusively an enforce policy. The caller never joins it.
func RunWorker(input io.Reader) int {
	data, err := io.ReadAll(io.LimitReader(input, 8*Limit+1))
	if closer, ok := input.(io.Closer); ok {
		_ = closer.Close()
	}
	if err != nil || len(data) > 8*Limit {
		return 1
	}
	var job ShadowJob
	if json.Unmarshal(data, &job) != nil {
		return 1
	}
	request, digest, err := Encode(job.Request)
	if err != nil {
		return 1
	}
	// The worker can be stopped by its recorded PID without leaving selector
	// children behind. No deadline is imposed on shadow policy.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGHUP, os.Interrupt)
	defer stop()
	cmd := exec.CommandContext(ctx, job.Config.Executable, job.Config.ArgvPrefix...)
	cmd.Dir = job.Request.CWD
	cmd.Stdin = bytes.NewReader(request)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	kill := func() {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}
	cmd.Cancel = func() error { kill(); return nil }
	output := &boundedOutput{cancel: kill}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 100 * time.Millisecond
	err = cmd.Run()
	kill()
	record := Observation{Kind: "decision", RequestDigest: digest}
	if output.overflow {
		record.Error = "invalid"
	} else if err != nil {
		record.Error = "exit"
	} else if decision, e := ParseResponse(output.data, job.Request.Candidates); e != nil {
		if failure, ok := e.(*Failure); ok && failure.Code == CodeAbstained {
			record.Outcome = "refused:" + failure.RefusalCode
		} else {
			record.Error = "invalid"
		}
	} else {
		record.Decision = &decision
	}
	if AppendWait(job.Observations, record) != nil {
		return 1
	}
	return 0
}
