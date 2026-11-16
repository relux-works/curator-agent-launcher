//go:build linux

package m0adapter

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// Test-only process modes of this test binary, selected by argv[1] so the
// adapter child keeps exactly the six-variable environment.
const (
	childAdapterArg    = "m0adapter-test-adapter-child"
	childProductionArg = "m0adapter-test-production-child"
)

func helperMain() {
	if len(os.Args) < 2 {
		return
	}
	switch os.Args[1] {
	case childAdapterArg:
		childAdapter(os.Args[2:])
	case childProductionArg:
		Main()
	case codexRunnerCommand:
		fakeRunner()
	}
}

// childAdapter runs the real pipeline with synthetic literals and fake seams:
// args are E, H, the first and the second qualified clock reading. Exec is
// the real syscall.Exec.
func childAdapter(args []string) {
	if len(args) != 4 {
		Exit(ReasonInternal)
	}
	now1, err1 := strconv.ParseInt(args[2], 10, 64)
	now2, err2 := strconv.ParseInt(args[3], 10, 64)
	if err1 != nil || err2 != nil {
		Exit(ReasonInternal)
	}
	readings := []int64{now1, now2}
	next := 0
	Exit(Run(Config{
		Literals: testLiterals(args[0], args[1]),
		WaitForC: func() error { return nil },
		NowMs: func(string) (int64, error) {
			v := readings[next]
			if next < len(readings)-1 {
				next++
			}
			return v, nil
		},
		Environ: syscall.Environ(),
		Exec:    syscall.Exec,
	}))
}

// fdInfo is one descriptor observed by the fake runner.
type fdInfo struct {
	FD   int    `json:"fd"`
	Dev  uint64 `json:"dev"`
	Ino  uint64 `json:"ino"`
	Mode uint32 `json:"mode"`
}

// runnerReport is what the fake runner observed after the adapter's exec.
type runnerReport struct {
	PID       int      `json:"pid"`
	Args      []string `json:"args"`
	Env       []string `json:"env"`
	Stdin     []byte   `json:"stdin"`
	EOF       bool     `json:"eof"`
	Inherited []fdInfo `json:"inherited"`
	Error     string   `json:"error,omitempty"`
}

// fakeRunner stands in for the broker run-codex. It records its pid, argv,
// environment, inherited descriptors (FD_CLOEXEC clear) and all of stdin up
// to EOF, prints that report on stdout and exits with the status named by an
// account selector "status-N" (default 0). The observation code is
// independent of the adapter's own descriptor helpers.
func fakeRunner() {
	rep := runnerReport{PID: os.Getpid(), Args: os.Args, Env: os.Environ()}
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		rep.Error = "readdir"
	}
	for _, e := range entries {
		fd, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_GETFD, 0)
		if errno != 0 || flags&syscall.FD_CLOEXEC != 0 {
			continue // closed meanwhile (the ReadDir handle) or not inherited
		}
		var st syscall.Stat_t
		if syscall.Fstat(fd, &st) != nil {
			continue
		}
		rep.Inherited = append(rep.Inherited, fdInfo{FD: fd, Dev: uint64(st.Dev), Ino: uint64(st.Ino), Mode: st.Mode})
	}
	buf := make([]byte, 64*1024)
	for len(rep.Stdin) <= 1<<20 {
		n, err := syscall.Read(0, buf)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			rep.Error = "read"
			break
		}
		if n == 0 {
			rep.EOF = true
			break
		}
		rep.Stdin = append(rep.Stdin, buf[:n]...)
	}
	_ = json.NewEncoder(os.Stdout).Encode(rep)
	status := 0
	for i := 0; i+1 < len(os.Args); i++ {
		if os.Args[i] == "--account" && strings.HasPrefix(os.Args[i+1], "status-") {
			status, _ = strconv.Atoi(strings.TrimPrefix(os.Args[i+1], "status-"))
		}
	}
	os.Exit(status)
}
