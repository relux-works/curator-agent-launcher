//go:build linux

package m0adapter

import (
	"bytes"
	"encoding/json"
	"io"
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
	default:
		if runnerCommandShape(os.Args[1]) {
			fakeRunner()
		}
	}
}

// childAdapter runs the real pipeline with synthetic literals and fake seams:
// args are E, H, the first and the second qualified clock reading. Exec is
// the real syscall.Exec.
func childAdapter(args []string) {
	if len(args) != 5 {
		Exit(ReasonInternal)
	}
	now1, err1 := strconv.ParseInt(args[2], 10, 64)
	now2, err2 := strconv.ParseInt(args[3], 10, 64)
	if err1 != nil || err2 != nil {
		Exit(ReasonInternal)
	}
	readings := []int64{now1, now2}
	next := 0
	lit := testLiterals(args[0], args[1])
	ops := nativePromptSyscalls()
	nativeWrite := ops.write
	closeFD := syscall.Close
	var readEnv func() ([]string, error)
	execRunner := syscall.Exec
	switch args[4] {
	case "unknown-runner":
		lit.RunnerCommand = "run-other"
	case "claude-unpinned":
		lit.RunnerCommand = "run-claude"
	case "origin-missing":
		lit.OriginSocketMarker = ""
	case "keymaker-missing":
		lit.KeymakerMarker = ""
	case "prefix-missing":
		lit.DeniedPathPrefix = ""
	case "env-add-home", "env-add-model", "env-add-endpoint", "env-rewrite-path":
		execRunner = func(path string, argv, env []string) error {
			changed := append([]string(nil), env...)
			switch args[4] {
			case "env-add-home":
				changed = append(changed, "CODEX_HOME=x")
			case "env-add-model":
				changed = append(changed, "MODEL=x")
			case "env-add-endpoint":
				changed = append(changed, "ENDPOINT=x")
			case "env-rewrite-path":
				for i, kv := range changed {
					if strings.HasPrefix(kv, "PATH=") {
						changed[i] = "PATH=/rewritten"
					}
				}
			}
			return syscall.Exec(path, argv, changed)
		}
	case "native-properties", "native-blocking":
		nativePipe := ops.pipe2
		capacity := 0
		ops.pipe2 = func(p []int, flags int) error {
			if err := nativePipe(p, flags); err != nil {
				return err
			}
			var err error
			capacity, err = fcntl(p[1], fGetPipeSz, 0)
			return err
		}
		ops.write = func(fd int, p []byte) (int, error) {
			if args[4] == "native-blocking" {
				_ = syscall.SetNonblock(fd, false)
			}
			flags, err := fcntl(fd, syscall.F_GETFL, 0)
			size, sizeErr := fcntl(fd, fGetPipeSz, 0)
			if err != nil || sizeErr != nil || flags&syscall.O_NONBLOCK == 0 || size != capacity {
				return 0, syscall.EINVAL
			}
			return nativeWrite(fd, p)
		}
		execRunner = func(path string, argv, env []string) error {
			size, err := fcntl(0, fGetPipeSz, 0)
			if err != nil || size != capacity {
				return syscall.EINVAL
			}
			return syscall.Exec(path, argv, env)
		}
	case "environment-unreadable":
		readEnv = func() ([]string, error) {
			return readEnvironmentWith(func() (io.ReadCloser, error) { return nil, syscall.EACCES })
		}
	case "timeout-marker":
		lit.KeymakerMarker = "9"
		lit.Home, lit.Executable = testHome, "/example/broker"
		execRunner = func(_ string, argv, env []string) error { return syscall.Exec(args[0], argv, env) }
	case "stdout-closed":
		_ = syscall.Close(1)
	case "stdout-cloexec":
		ops.dup = func(old, new, flags int) error {
			syscall.CloseOnExec(1)
			return syscall.Dup3(old, new, flags)
		}
	case "stdout-replaced":
		ops.dup = func(old, new, flags int) error {
			fd, err := syscall.Open(os.DevNull, syscall.O_WRONLY|syscall.O_CLOEXEC, 0)
			if err != nil {
				return err
			}
			defer syscall.Close(fd)
			if err := syscall.Dup3(fd, 1, 0); err != nil {
				return err
			}
			return syscall.Dup3(old, new, flags)
		}
	case "home-input", "prompt-twice":
		ops.write = func(fd int, p []byte) (int, error) {
			var data []byte
			if args[4] == "home-input" {
				var err error
				data, err = os.ReadFile(lit.Home + "/prompt.txt")
				if err != nil {
					return 0, err
				}
			} else {
				data = bytes.Repeat(p, 2)
			}
			_, err := nativeWrite(fd, data)
			if err != nil {
				return 0, err
			}
			return len(p), nil
		}
	case "plan-close-fails":
		closeFD = func(int) error { return syscall.EIO }
	case "pipe2-fails":
		ops.pipe2 = func([]int, int) error { return syscall.EIO }
	case "getsize-fails":
		ops.fcntl = func(int, int, int) (int, error) { return -1, syscall.EIO }
	case "setfl-fails":
		ops.setNonblock = func(int, bool) error { return syscall.EIO }
	case "write-close-fails":
		writeFD := -1
		ops.write = func(fd int, p []byte) (int, error) { writeFD = fd; return nativeWrite(fd, p) }
		ops.close = func(fd int) error {
			if fd == writeFD {
				return syscall.EIO
			}
			return syscall.Close(fd)
		}
	case "dup-fails":
		ops.dup = func(int, int, int) error { return syscall.EIO }
	case "writer-inherited":
		writeFD := -1
		ops.write = func(fd int, p []byte) (int, error) { writeFD = fd; return nativeWrite(fd, p) }
		ops.close = func(fd int) error {
			if fd == writeFD {
				// F_DUPFD (command 0) returns the lowest free descriptor at or
				// above 10 without FD_CLOEXEC. A fixed target would fail with
				// EINVAL whenever the writer already has that number.
				if _, err := fcntl(fd, 0, 10); err != nil {
					return err
				}
			}
			return syscall.Close(fd)
		}
	case "second-read-fd1":
		ops.dup = func(old, new, flags int) error {
			if err := syscall.Dup3(old, 1, 0); err != nil {
				return err
			}
			return syscall.Dup3(old, new, flags)
		}
	case "stdin-rdwr":
		ops.dup = func(old, new, flags int) error {
			fd, err := syscall.Open("/proc/self/fd/"+strconv.Itoa(old), syscall.O_RDWR|syscall.O_CLOEXEC, 0)
			if err != nil {
				return err
			}
			defer syscall.Close(fd)
			return syscall.Dup3(fd, new, flags)
		}
	}
	if args[4] == "guard-pipe" || strings.HasSuffix(args[4], "-missing") || args[4] == "unknown-runner" || args[4] == "claude-unpinned" {
		ops.pipe2 = func([]int, int) error {
			_, _ = syscall.Write(2, []byte("unexpected_pipe_creation\n"))
			return syscall.EIO
		}
	}
	Exit(runWithSyscalls(Config{
		Literals: lit,
		WaitForC: func() error { return nil },
		NowMs: func(string) (int64, error) {
			v := readings[next]
			if next < len(readings)-1 {
				next++
			}
			return v, nil
		},
		readEnviron: readEnv,
		Exec:        execRunner,
	}, ops, closeFD))
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
	Capacity  int      `json:"capacity"`
}

// fakeRunner stands in for the broker run-codex. It records its pid, argv,
// environment, inherited descriptors (FD_CLOEXEC clear) and all of stdin up
// to EOF, prints that report on stdout and exits with the status named by an
// account selector "status-N" (default 0). The observation code is
// independent of the adapter's own descriptor helpers.
func fakeRunner() {
	rep := runnerReport{PID: os.Getpid(), Args: os.Args}
	// Independent raw observation: do not reuse the adapter reader or os.Environ.
	block, envErr := os.ReadFile("/proc/self/environ")
	if envErr != nil || len(block) == 0 || block[len(block)-1] != 0 {
		rep.Error = "environment"
	} else {
		rep.Env = strings.Split(string(block[:len(block)-1]), "\x00")
	}
	size, _, errno := syscall.Syscall(syscall.SYS_FCNTL, 0, 1032, 0)
	if errno != 0 {
		rep.Error = "capacity"
	} else {
		rep.Capacity = int(size)
	}
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

// isolateDescriptors marks every open descriptor above 2 close-on-exec in the
// parent test process. Go's own descriptors already are; this covers those
// inherited from outside (CI runner, make, go test), which would otherwise
// leak into every child and make the adapter refuse fd_unexpected.
func isolateDescriptors() {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return
	}
	for _, e := range entries {
		fd, err := strconv.Atoi(e.Name())
		if err != nil || fd <= 2 {
			continue
		}
		syscall.CloseOnExec(fd)
	}
}
