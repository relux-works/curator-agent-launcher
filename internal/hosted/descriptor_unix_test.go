//go:build unix

package hosted_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/relux-works/curator-agent-launcher/internal/hosted"
)

// clearCloseOnExec clears the close-on-exec flag so the test proves the
// launcher's explicit mark, not the runtime's open-time mark.
func clearCloseOnExec(t *testing.T, f *os.File) {
	t.Helper()
	if _, err := unix.FcntlInt(f.Fd(), unix.F_SETFD, 0); err != nil {
		t.Fatal(err)
	}
}

// TestValidateTerminal pins the r6 §4 terminal classes: a read-write
// regular file (test double) or character device passes; a directory,
// a pipe, a read-only file, a closed file, and an unmarked descriptor
// refuse. Production /dev/tty is a read-write character device owned
// by root, which the same predicate admits.
func TestValidateTerminal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tty.txt")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	good, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer good.Close()
	if err := hosted.ValidateTerminal(good); err != nil {
		t.Fatalf("valid terminal refused: %v", err)
	}
	readonly, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	if err := hosted.ValidateTerminal(readonly); err == nil {
		t.Fatal("read-only terminal admitted")
	}
	d, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := hosted.ValidateTerminal(d); err == nil {
		t.Fatal("directory terminal admitted")
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	defer pw.Close()
	if err := hosted.ValidateTerminal(pr); err == nil {
		t.Fatal("pipe terminal admitted")
	}
	unmarked, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unmarked.Close()
	clearCloseOnExec(t, unmarked)
	if err := hosted.ValidateTerminal(unmarked); err == nil {
		t.Fatal("unmarked terminal admitted")
	}
	hosted.MarkCloseOnExec(unmarked)
	if err := hosted.ValidateTerminal(unmarked); err != nil {
		t.Fatalf("marked terminal refused: %v", err)
	}
	closed, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	if err := hosted.ValidateTerminal(closed); err == nil {
		t.Fatal("closed terminal admitted")
	}
	if err := hosted.ValidateTerminal(nil); err == nil {
		t.Fatal("nil terminal admitted")
	}
}

// TestTransportTerminalValidationRefuses proves a wrong terminal
// descriptor refuses session_host_terminal_required before the receiver
// spawns: the lookup resolves a path that does not exist, so only the
// validation refusal (exit 6) — never a spawn failure (exit 1) — is
// possible.
func TestTransportTerminalValidationRefuses(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		open func() *os.File
	}{
		{"directory", func() *os.File {
			f, err := os.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			return f
		}},
		{"read-only", func() *os.File {
			p := filepath.Join(dir, "ro.txt")
			if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(p)
			if err != nil {
				t.Fatal(err)
			}
			return f
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			transport := hosted.Transport{
				Lookup:   func() (string, error) { return filepath.Join(dir, "no-such-receiver"), nil },
				Terminal: func() (*os.File, error) { return tc.open(), nil },
			}
			if code := transport.Run([]byte("{}"), &stdout, &stderr); code != 6 {
				t.Fatalf("exit = %d, want 6 (stderr %q)", code, stderr.String())
			}
			if first, _, _ := bytes.Cut(stderr.Bytes(), []byte("\n")); string(first) != "curator-run: session_host_terminal_required: a hosted launch requires a controlling terminal" {
				t.Fatalf("diagnostic line = %q", first)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

// TestTransportMarksCloseOnExec proves the launcher's explicit mark: a
// terminal that arrives without close-on-exec still launches, because
// the transport marks it before validation. Without the mark the same
// descriptor refuses.
func TestTransportMarksCloseOnExec(t *testing.T) {
	dir := t.TempDir()
	ttyPath := filepath.Join(dir, "tty.txt")
	if err := os.WriteFile(ttyPath, []byte("terminal-marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	tty, err := os.OpenFile(ttyPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	clearCloseOnExec(t, tty)
	var stdout, stderr bytes.Buffer
	transport := hosted.Transport{
		Lookup:   func() (string, error) { return testFDContract, nil },
		Terminal: func() (*os.File, error) { return tty, nil },
	}
	if code := transport.Run([]byte(`{}`), &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, stderr.String())
	}
}
