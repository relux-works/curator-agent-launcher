package execution

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/composition"
)

// A SIGTERM held by the caller before the child exists (queued on the
// interrupts channel between its last check and the start) reaches the child
// once it starts: the child ends by that signal instead of running on. A
// child with no queued signal runs to its own exit (the control).
func TestRunForwardsInterruptsQueuedBeforeStart(t *testing.T) {
	const sleep = "/bin/sleep"
	if _, err := os.Stat(sleep); err != nil {
		t.Fatalf("test needs %s: %v", sleep, err)
	}
	queued := make(chan os.Signal, 1)
	queued <- syscall.SIGTERM
	start := time.Now()
	err := run(exec.Command(sleep, "5"), nil, queued)
	var exit *processExit
	if !errors.As(err, &exit) || !exit.status.Signaled() || exit.status.Signal() != syscall.SIGTERM {
		t.Fatalf("err = %v, want the child ended by SIGTERM", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("the queued signal was not forwarded at the start")
	}

	empty := make(chan os.Signal, 1)
	if err := run(exec.Command(sleep, "0.2"), nil, empty); err != nil {
		t.Fatalf("control child: %v", err)
	}
}

// Through the public Run, the caller's Options.Interrupts is the channel the
// child's signals come from: a SIGTERM queued there before the launch ends
// the child (exit 128+15). The same launch with an empty channel runs to
// its own exit (the control).
func TestLaunchRunForwardsCallerInterrupts(t *testing.T) {
	launch := func(seconds string, interrupts <-chan os.Signal) int {
		l := Prepare(composition.Value{Binary: "/bin/sleep", Argv: []string{seconds}, WorkDir: t.TempDir(), Env: []string{"PATH=/bin"}})
		var out, stderr bytes.Buffer
		return l.Run(Options{Boundary: func() error { return nil }, IO: IO{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &stderr}, Interrupts: interrupts})
	}
	queued := make(chan os.Signal, 1)
	queued <- syscall.SIGTERM
	start := time.Now()
	if code := launch("5", queued); code != 128+int(syscall.SIGTERM) {
		t.Fatalf("exit = %d, want the child ended by SIGTERM", code)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("the queued signal was not forwarded at the start")
	}
	if code := launch("0.2", make(chan os.Signal, 1)); code != 0 {
		t.Fatalf("control exit = %d", code)
	}
}
