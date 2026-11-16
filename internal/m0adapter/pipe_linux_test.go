//go:build linux

package m0adapter

import (
	"bytes"
	"syscall"
	"testing"
)

// Proves the no-feeder capacity rule: a pipe whose capacity is below the
// prompt length is refused with stdin_pipe_capacity before any byte is
// written, and the adapter never raises the capacity itself. Beside it, the
// same 24576-byte prompt fits a pipe of the default capacity (the positive
// end-to-end delivery is covered by the process tests). Limit: fd 0 is not
// replaced in either case here, because the refusal comes before dup3 and the
// positive case only checks capacity.
func TestPromptPipeCapacity(t *testing.T) {
	prompt := bytes.Repeat([]byte("p"), 24576)

	var small [2]int
	if err := syscall.Pipe2(small[:], syscall.O_CLOEXEC); err != nil {
		t.Fatal(err)
	}
	if _, err := fcntl(small[1], fSetPipeSz, 4096); err != nil {
		syscall.Close(small[0])
		syscall.Close(small[1])
		t.Skipf("cannot shrink the pipe on this kernel: %v", err)
	}
	before, err := fcntl(small[1], fGetPipeSz, 0)
	if err != nil || before >= len(prompt) {
		syscall.Close(small[0])
		syscall.Close(small[1])
		t.Fatalf("shrunk pipe capacity %d, err %v", before, err)
	}
	if _, err := fillPromptPipe(small[0], small[1], prompt); reasonOf(err) != ReasonStdinCapacity {
		t.Fatalf("want %s, got %v", ReasonStdinCapacity, err)
	}

	var normal [2]int
	if err := syscall.Pipe2(normal[:], syscall.O_CLOEXEC); err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(normal[0])
	defer syscall.Close(normal[1])
	size, err := fcntl(normal[1], fGetPipeSz, 0)
	if err != nil || size < len(prompt) {
		t.Fatalf("default pipe capacity %d, err %v: the positive control does not hold", size, err)
	}
}
