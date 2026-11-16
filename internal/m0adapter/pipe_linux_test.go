//go:build linux

package m0adapter

import (
	"bytes"
	"strings"
	"syscall"
	"testing"
)

// pipeHarness models only the pipe syscalls; no test mutates fd 0 or a global
// syscall hook. Native process tests separately exercise the real pipe/exec.
type pipeHarness struct {
	ops      promptSyscalls
	trace    []string
	writes   int
	capacity int
	result   []struct {
		n   int
		err error
	}
}

func newPipeHarness(t *testing.T) *pipeHarness {
	t.Helper()
	h := &pipeHarness{capacity: 8192}
	h.ops = promptSyscalls{
		pipe2: func(p []int, flags int) error {
			h.trace = append(h.trace, "pipe2")
			if flags != syscall.O_CLOEXEC {
				t.Error("pipe lacks CLOEXEC")
			}
			p[0], p[1] = 10, 11
			return nil
		},
		fcntl: func(fd, cmd, arg int) (int, error) {
			h.trace = append(h.trace, "getsize")
			if fd != 11 || cmd != fGetPipeSz || arg != 0 {
				t.Error("capacity raised or wrong query")
			}
			return h.capacity, nil
		},
		setNonblock: func(fd int, on bool) error {
			h.trace = append(h.trace, "nonblock")
			if fd != 11 || !on {
				t.Error("blocking write")
			}
			return nil
		},
		write: func(fd int, p []byte) (int, error) {
			h.trace = append(h.trace, "write")
			h.writes++
			if fd != 11 || !strings.Contains(strings.Join(h.trace, ","), "nonblock,write") && h.writes == 1 {
				t.Error("write before O_NONBLOCK")
			}
			if h.writes <= len(h.result) {
				r := h.result[h.writes-1]
				return r.n, r.err
			}
			if len(p) > h.capacity {
				return 0, syscall.EAGAIN
			}
			return len(p), nil
		},
		close: func(fd int) error {
			if fd == 10 {
				h.trace = append(h.trace, "close-r")
			} else if fd == 11 {
				h.trace = append(h.trace, "close-w")
			} else {
				t.Errorf("unexpected close %d", fd)
			}
			return nil
		},
		stat: func(int) (fileID, syscall.Stat_t, error) {
			h.trace = append(h.trace, "stat")
			return fileID{1, 2}, syscall.Stat_t{}, nil
		},
		dup: func(old, new, flags int) error {
			h.trace = append(h.trace, "dup")
			if old != 10 || new != 0 || flags != 0 {
				t.Error("wrong stdin installation")
			}
			return nil
		},
	}
	return h
}

func requirePipePositive(t *testing.T, h *pipeHarness, prompt []byte) {
	t.Helper()
	id, err := installPromptWith(prompt, h.ops)
	if err != nil || id != (fileID{1, 2}) {
		t.Fatalf("valid pipe: %v", err)
	}
	if got := strings.Join(h.trace, ","); got != "pipe2,getsize,nonblock,write,close-w,stat,dup,close-r" {
		t.Fatalf("lifecycle %s", got)
	}
}

// TestPromptPipeCapacity proves capacity equal to 8192 prompt bytes passes and 8193 refuses before any write.
func TestPromptPipeCapacity(t *testing.T) {
	requirePipePositive(t, newPipeHarness(t), bytes.Repeat([]byte("p"), 8192))
	h := newPipeHarness(t)
	_, err := installPromptWith(bytes.Repeat([]byte("p"), 8193), h.ops)
	requireReason(t, err, ReasonStdinCapacity)
	if h.writes != 0 {
		t.Fatal("write attempted below capacity")
	}
}

// TestPromptPipeEINTRBound proves one zero-byte EINTR retry succeeds but a second interruption refuses without a third write.
func TestPromptPipeEINTRBound(t *testing.T) {
	positive := newPipeHarness(t)
	positive.result = []struct {
		n   int
		err error
	}{{0, syscall.EINTR}}
	if _, err := installPromptWith([]byte("prompt"), positive.ops); err != nil || positive.writes != 2 {
		t.Fatalf("one retry: %v, writes %d", err, positive.writes)
	}
	h := newPipeHarness(t)
	h.result = []struct {
		n   int
		err error
	}{{0, syscall.EINTR}, {0, syscall.EINTR}}
	_, err := installPromptWith([]byte("prompt"), h.ops)
	requireReason(t, err, ReasonStdinWrite)
	if h.writes != 2 || strings.Contains(strings.Join(h.trace, ","), "dup") {
		t.Fatal("second EINTR retried or stdin installed")
	}
}

// TestPromptPipePartialWrite proves short writes, including partial EINTR and EAGAIN, refuse with no retry beside a whole write.
func TestPromptPipePartialWrite(t *testing.T) {
	for _, result := range []struct {
		n   int
		err error
	}{{1, nil}, {1, syscall.EINTR}, {1, syscall.EAGAIN}, {0, syscall.EAGAIN}, {0, syscall.EIO}, {-1, syscall.EINTR}} {
		requirePipePositive(t, newPipeHarness(t), []byte("prompt"))
		h := newPipeHarness(t)
		h.result = append(h.result, result)
		_, err := installPromptWith([]byte("prompt"), h.ops)
		requireReason(t, err, ReasonStdinWrite)
		if h.writes != 1 || strings.Contains(strings.Join(h.trace, ","), "dup") {
			t.Fatal("failed write retried or stdin installed")
		}
	}
}

// TestPromptPipeNonblocking proves O_NONBLOCK precedes a whole write and a failed F_SETFL refuses before writing.
func TestPromptPipeNonblocking(t *testing.T) {
	requirePipePositive(t, newPipeHarness(t), []byte("prompt"))
	h := newPipeHarness(t)
	h.ops.setNonblock = func(int, bool) error { return syscall.EPERM }
	_, err := installPromptWith([]byte("prompt"), h.ops)
	requireReason(t, err, ReasonStdinWrite)
	if h.writes != 0 {
		t.Fatal("write after failed F_SETFL")
	}
}

// TestPromptPipeNoCapacityRaise proves the only capacity operation is F_GETPIPE_SZ and a short pipe refuses rather than adapting.
func TestPromptPipeNoCapacityRaise(t *testing.T) {
	requirePipePositive(t, newPipeHarness(t), []byte("prompt"))
	h := newPipeHarness(t)
	h.capacity = 5
	_, err := installPromptWith([]byte("prompt"), h.ops)
	requireReason(t, err, ReasonStdinCapacity)
	if h.writes != 0 || strings.Count(strings.Join(h.trace, ","), "getsize") != 1 {
		t.Fatal("capacity adapted")
	}
}

// TestPromptPipeGetSizeFailure proves failed F_GETPIPE_SZ refuses with no default capacity and no write.
func TestPromptPipeGetSizeFailure(t *testing.T) {
	requirePipePositive(t, newPipeHarness(t), []byte("prompt"))
	h := newPipeHarness(t)
	h.ops.fcntl = func(int, int, int) (int, error) { return -1, syscall.EIO }
	_, err := installPromptWith([]byte("prompt"), h.ops)
	requireReason(t, err, ReasonStdinCapacity)
	if h.writes != 0 {
		t.Fatal("write after failed capacity query")
	}
}
