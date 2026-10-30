package execution

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
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
