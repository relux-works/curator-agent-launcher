package hosted

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
)

// Receiver argv of the private contract (§4). Provider, cwd, name, and intent
// travel only on the payload stdin, never here.
var receiverArgv = []string{"session", "launch-plan", "--plan", "-", "--terminal-fd", "3", "--status-fd", "4"}

// statusRecordMax bounds one fd4 status record (§4: 32-bit big-endian
// length-prefixed JSON, at most 64 KiB, never child bytes).
const statusRecordMax = 64 << 10

// Transport carries the receiver boundaries. A nil Lookup resolves
// "task-board" on PATH; a nil Terminal opens the controlling terminal.
// Tests inject fakes; production uses the defaults.
//
// PreopenedTerminal carries a terminal the caller already opened (the
// entry point opens it before the plan build so a known terminal
// failure refuses with zero builds and zero lookup). When set, Run
// uses it instead of calling Terminal and takes ownership (it closes
// the file). When nil, Run opens the terminal itself after lookup.
type Transport struct {
	Lookup            func() (string, error)
	Terminal          func() (*os.File, error)
	PreopenedTerminal *os.File
}

// LookupReceiver resolves the receiver binary on PATH.
func LookupReceiver() (string, error) {
	return exec.LookPath("task-board")
}

// OpenTerminal opens the controlling terminal read/write for fd3.
func OpenTerminal() (*os.File, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}

// Run hands the payload to the receiver and maps the outcome to the process
// exit code. The payload travels only on the private receiver stdin; the
// controlling terminal passes as fd3 and a dedicated status pipe as fd4.
// Standard output and error stay the live terminal. Only fd4 records
// classify a refusal; stderr bytes never become refusal metadata.
//
//   - receiver lookup or spawn failure refuses session_host_missing;
//   - an unavailable terminal refuses session_host_terminal_required;
//   - a truncated status channel (EOF mid-frame) refuses
//     session_host_unavailable: the transport was lost, not the envelope —
//     unless a complete malformed frame already poisoned the channel,
//     which stays session_host_protocol_error/6;
//   - any complete frame that is not a closed refusal envelope — bad
//     JSON, wrong members, unknown code, mismatched message, bad detail,
//     zero or oversize length — refuses session_host_protocol_error/6
//     with the constant message and empty details, never receiver bytes;
//   - a valid refusal record emits its registry code, constant message,
//     and validated field/reason with the registry exit (1/2/6/16); the
//     last valid refusal wins;
//   - without a record the receiver exit propagates unchanged, including
//     child statuses that coincide with refusal exits.
//
// There is no fallback: a missing receiver never degrades to native
// execution. The receiver owns the terminal (raw mode, resize, signals) and
// all deadlines; the launcher waits like a native exec and drops
// terminal-generated SIGINT/SIGQUIT, which the same foreground group
// delivers to the receiver directly.
func (t Transport) Run(payload []byte, stdout, stderr io.Writer) int {
	lookup := t.Lookup
	if lookup == nil {
		lookup = LookupReceiver
	}
	terminal := t.Terminal
	if terminal == nil {
		terminal = OpenTerminal
	}
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	receiver, err := lookup()
	if err != nil || receiver == "" {
		_ = diagnostics.Emit(stderr, CodeSessionHostMissing, "the task-board receiver is not available on PATH")
		return diagnostics.ExitForCode(CodeSessionHostMissing)
	}
	var tty *os.File
	if t.PreopenedTerminal != nil {
		tty = t.PreopenedTerminal
	} else {
		tty, err = terminal()
		if err != nil || tty == nil {
			_ = diagnostics.Emit(stderr, CodeSessionHostTerminalRequired, "a hosted launch requires a controlling terminal")
			return diagnostics.ExitForCode(CodeSessionHostTerminalRequired)
		}
	}
	defer tty.Close()
	statusR, statusW, err := os.Pipe()
	if err != nil {
		_ = diagnostics.Emit(stderr, CodeSessionHostUnavailable, "cannot open the receiver status channel")
		return diagnostics.ExitForCode(CodeSessionHostUnavailable)
	}
	// r6 §4 descriptor contract, launcher side: mark both descriptors
	// close-on-exec and validate type, access, and ownership before
	// contact. The marks cover the launcher's own copies, so no later
	// launcher child inherits them; Go's exec necessarily clears the
	// flag for the receiver's copies (fd3/fd4 must survive the first
	// exec), so the receiver MUST set close-on-exec immediately on
	// startup to protect its own descendants (SPEC §4.9). Neither
	// refusal below reflects descriptor values.
	MarkCloseOnExec(tty)
	MarkCloseOnExec(statusW)
	if err := ValidateTerminal(tty); err != nil {
		statusW.Close()
		statusR.Close()
		_ = diagnostics.Emit(stderr, CodeSessionHostTerminalRequired, "a hosted launch requires a controlling terminal")
		return diagnostics.ExitForCode(CodeSessionHostTerminalRequired)
	}
	if err := validateStatusPipe(statusW); err != nil {
		statusW.Close()
		statusR.Close()
		_ = diagnostics.Emit(stderr, CodeSessionHostUnavailable, "cannot open the receiver status channel")
		return diagnostics.ExitForCode(CodeSessionHostUnavailable)
	}
	cmd := exec.Command(receiver, receiverArgv...)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// ExtraFiles[i] becomes descriptor 3+i in the receiver: the terminal
	// on fd3, the status pipe write end on fd4. The receiver ingress
	// flags are necessarily clear (see above); the contract's descendant
	// isolation is established by the receiver's immediate close-on-exec
	// marking, proven end to end by TestTransportDescriptorContract.
	cmd.ExtraFiles = []*os.File{tty, statusW}
	// The receiver inherits the launcher environment and working directory,
	// never the composed child environment: values travel on stdin only,
	// and the receiver discovers the board from the operator cwd.
	drop := make(chan os.Signal, 4)
	signal.Notify(drop, os.Interrupt, syscall.SIGQUIT)
	defer signal.Stop(drop)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-drop:
			case <-done:
				return
			}
		}
	}()
	if err := cmd.Start(); err != nil {
		statusW.Close()
		statusR.Close()
		_ = diagnostics.Emit(stderr, CodeSessionHostMissing, "the task-board receiver could not start")
		return diagnostics.ExitForCode(CodeSessionHostMissing)
	}
	// The parent holds only the read end; EOF arrives when the receiver
	// closes fd4 or exits.
	statusW.Close()
	refusals, statusErr := readStatusRecords(statusR)
	statusR.Close()
	waitErr := cmd.Wait()
	if statusErr != nil {
		if errors.Is(statusErr, errStatusProtocol) {
			_ = diagnostics.Emit(stderr, StatusProtocolErrorCode, ProtocolErrorMessage())
			return ProtocolErrorExit()
		}
		_ = diagnostics.Emit(stderr, CodeSessionHostUnavailable, "the receiver status channel is corrupt")
		return diagnostics.ExitForCode(CodeSessionHostUnavailable)
	}
	if len(refusals) > 0 {
		last := refusals[len(refusals)-1]
		_ = diagnostics.Emit(stderr, last.Code, last.Detail())
		// Every refusal here normalized against the registry, so its
		// exit is authoritative — never the receiver status.
		if exit, ok := StatusExitForCode(last.Code); ok {
			return exit
		}
		return exitOf(waitErr)
	}
	return exitOf(waitErr)
}

// StatusRefusal is one normalized fd4 refusal: the registry code, its
// constant message, and validated field/reason tokens. The last one wins,
// so a failing launch prints exactly one diagnostic code line.
type StatusRefusal struct {
	Code    string
	Message string
	Field   string
	Reason  string
}

func (r StatusRefusal) Detail() string {
	detail := r.Message
	if r.Field != "" {
		detail += " field=" + r.Field
	}
	if r.Reason != "" {
		detail += " reason=" + r.Reason
	}
	return detail
}

// readStatusRecords drains length-prefixed JSON records until EOF. Every
// complete frame normalizes against the closed envelope: any violation
// poisons the channel (protocol_error), while valid refusals collect for
// last-wins. Truncated reads — EOF mid-frame — are transport loss, not a
// malformed envelope, and fail as unavailable; but once a complete
// malformed frame has poisoned the channel, every later fault — clean
// EOF, partial header, or partial body — still yields protocol_error.
// Only a stream that saw no malformed frame maps a transport fault to
// unavailable.
func readStatusRecords(r io.Reader) ([]StatusRefusal, error) {
	var out []StatusRefusal
	poisoned := false
	for {
		var hdr [4]byte
		_, err := io.ReadFull(r, hdr[:])
		if err == io.EOF {
			if poisoned {
				return nil, errStatusProtocol
			}
			return out, nil
		}
		if err != nil {
			if poisoned {
				return nil, errStatusProtocol
			}
			return nil, errStatusTransport
		}
		size := binary.BigEndian.Uint32(hdr[:])
		if size == 0 || size > statusRecordMax {
			return nil, errStatusProtocol
		}
		body := make([]byte, size)
		if _, err := io.ReadFull(r, body); err != nil {
			if poisoned {
				return nil, errStatusProtocol
			}
			return nil, errStatusTransport
		}
		refusal, err := NormalizeStatusRecord(body)
		if err != nil {
			poisoned = true
			continue
		}
		out = append(out, refusal)
	}
}

var errStatusProtocol = errors.New("receiver status protocol violation")
var errStatusTransport = errors.New("receiver status transport loss")

// exitOf propagates the receiver exit: 0, a status, or 128+signal. A wait
// failure that is not an exit is reported as unavailable by the caller.
func exitOf(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal())
		}
		return exitErr.ExitCode()
	}
	return diagnostics.ExitForCode(diagnostics.CodeSessionHostUnavailable)
}
