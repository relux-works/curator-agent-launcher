//go:build darwin || linux

package sessionmanager

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"golang.org/x/sys/unix"
)

type mutationJob struct {
	Resolver configfile.Resolver
	Command  Command
	Gate     string
	Root     string
}
type mutationResult struct {
	Code, Message string
	Elapsed       time.Duration
}

// This re-exec entry runs only test-owned mutations, with a hard lifetime;
// the parent also kills/reaps its recorded process group and checks survivors.
func TestRegistryMutationProcess(t *testing.T) {
	path := os.Getenv("REGISTRY_MUTATION_JOB")
	if path == "" {
		t.Skip("test subprocess entry")
	}
	watchdog := time.AfterFunc(25*time.Second, func() { os.Exit(124) })
	defer watchdog.Stop()
	var job mutationJob
	readTestJSON(t, path, &job)
	gate := func() {
		writeTestFile(t, filepath.Join(job.Root, "ready"), nil)
		awaitTestFile(t, filepath.Join(job.Root, "release"), 20*time.Second)
	}
	s := Service{Resolver: job.Resolver, Stdin: bytes.NewBufferString("\n"), CanWriteSystem: func(string) bool { return false }}
	if job.Gate == "prompt" {
		s.IsTerminal = func() bool { gate(); return true }
	}
	if job.Gate == "write" || job.Gate == "rollback" {
		s.WriteFile = func(p string, b []byte, m os.FileMode) error {
			if job.Gate == "write" {
				gate()
				return atomicWrite(p, b, m)
			}
			return atomicWriteWithHooks(p, b, m, atomicHooks{syncDirectory: func(string) error { gate(); return errors.New("injected directory sync failure") }})
		}
	}
	writeTestFile(t, filepath.Join(job.Root, "started"), nil)
	start := time.Now()
	var err error
	if job.Gate == "hold" {
		var lock *os.File
		lock, err = acquireRegistryLock(filepath.Join(s.Resolver.Locations.User, Filename), 0600)
		if err == nil {
			gate()
			err = lock.Close()
		}
	} else {
		err = s.Run(job.Command)
	}
	result := mutationResult{Elapsed: time.Since(start)}
	if err != nil {
		var e *Error
		if errors.As(err, &e) {
			result.Code = e.Code
		} else {
			result.Code = "unexpected"
		}
		result.Message = err.Error()
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(job.Root, "result"), data)
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func readTestJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}
func awaitTestFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("waiting for %s", filepath.Base(path))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type mutationChild struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
	root string
}

func mutationBinary(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "registry-test")
	if err = os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
	return path
}
func startMutation(t *testing.T, binary string, job mutationJob) *mutationChild {
	t.Helper()
	job.Root = t.TempDir()
	data, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(job.Root, "job.json")
	writeTestFile(t, path, data)
	log, err := os.Create(filepath.Join(job.Root, "child.log"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestRegistryMutationProcess$", "-test.count=1")
	cmd.Dir = job.Root
	cmd.Env = append(os.Environ(), "REGISTRY_MUTATION_JOB="+path, "TMPDIR="+job.Root)
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &unix.SysProcAttr{Setpgid: true}
	if err = cmd.Start(); err != nil {
		log.Close()
		t.Fatal(err)
	}
	child := &mutationChild{cmd: cmd, done: make(chan struct{}), root: job.Root}
	go func() { child.err = cmd.Wait(); log.Close(); close(child.done) }()
	t.Cleanup(func() {
		select {
		case <-child.done:
		default:
			_ = unix.Kill(-cmd.Process.Pid, unix.SIGKILL)
		}
		select {
		case <-child.done:
		case <-time.After(2 * time.Second):
			t.Error("child was not reaped")
		}
		if err := unix.Kill(-cmd.Process.Pid, 0); !errors.Is(err, unix.ESRCH) {
			t.Errorf("surviving child group %d: %v", cmd.Process.Pid, err)
		}
	})
	awaitTestFile(t, filepath.Join(job.Root, "started"), 5*time.Second)
	return child
}
func (c *mutationChild) result(t *testing.T) mutationResult {
	t.Helper()
	select {
	case <-c.done:
	case <-time.After(20 * time.Second):
		t.Fatal("mutation exceeded bounded lifetime")
	}
	if c.err != nil {
		log, _ := os.ReadFile(filepath.Join(c.root, "child.log"))
		t.Fatalf("child: %v\n%s", c.err, log)
	}
	var r mutationResult
	readTestJSON(t, filepath.Join(c.root, "result"), &r)
	return r
}
func (c *mutationChild) release(t *testing.T) {
	t.Helper()
	writeTestFile(t, filepath.Join(c.root, "release"), nil)
}
func (c *mutationChild) ready(t *testing.T) {
	t.Helper()
	awaitTestFile(t, filepath.Join(c.root, "ready"), 5*time.Second)
}

// Independent processes must retain every acknowledged registration, selection
// and removal. The gated writer spans the read/prompt or post-rename rollback;
// sequential controls establish the same expected state without contention.
func TestOverlappingRegistryMutations(t *testing.T) {
	binary := mutationBinary(t)
	for _, kind := range []string{"register-register", "register-select", "select-unregister", "rollback-register"} {
		t.Run(kind, func(t *testing.T) {
			for _, overlap := range []bool{false, true} {
				t.Run(map[bool]string{false: "sequential", true: "overlap"}[overlap], func(t *testing.T) {
					s, _, _ := service(t)
					e := entry(t)
					r := Empty()
					r.Selected = "seed"
					for _, id := range []string{"seed", "other", "remove"} {
						r.Managers[id] = e
					}
					putRegistry(t, s.Resolver.Locations.User, r)
					a := Command{Operation: "register", Manifest: manifest(t, "first", e)}
					b := Command{Operation: "register", Manifest: manifest(t, "second", e), NoSelect: true}
					gate := "prompt"
					switch kind {
					case "register-select":
						b = Command{Operation: "select", ID: "other"}
					case "select-unregister":
						a = Command{Operation: "select", ID: "other"}
						b = Command{Operation: "unregister", ID: "remove"}
						gate = "write"
					case "rollback-register":
						a = Command{Operation: "select", ID: "other"}
						gate = "rollback"
					}
					// The rollback control also injects the same failure without overlap.
					job := mutationJob{Resolver: s.Resolver, Command: a}
					if overlap || kind == "rollback-register" {
						job.Gate = gate
					}
					first := startMutation(t, binary, job)
					var second *mutationChild
					early := false
					if overlap {
						first.ready(t)
						second = startMutation(t, binary, mutationJob{Resolver: s.Resolver, Command: b})
						select {
						case <-second.done:
							early = true
						case <-time.After(400 * time.Millisecond):
						}
						first.release(t)
					} else if kind == "rollback-register" {
						first.ready(t)
						first.release(t)
					}
					ar := first.result(t)
					want := ""
					if kind == "rollback-register" {
						want = CodeInvalid
					}
					if ar.Code != want {
						t.Fatalf("first result: %+v", ar)
					}
					if second == nil {
						second = startMutation(t, binary, mutationJob{Resolver: s.Resolver, Command: b})
					}
					br := second.result(t)
					if br.Code != "" {
						t.Fatalf("second result: %+v", br)
					}
					got := readTarget(t, s)
					expected := map[string]bool{"seed": true, "other": true, "remove": kind != "select-unregister", "first": kind == "register-register" || kind == "register-select", "second": kind == "register-register" || kind == "rollback-register"}
					for id, want := range expected {
						_, exists := got.Managers[id]
						if exists != want {
							t.Errorf("acknowledged update lost: %s present=%v want=%v", id, exists, want)
						}
					}
					selected := "seed"
					if kind == "register-select" || kind == "select-unregister" {
						selected = "other"
					}
					if got.Selected != selected {
						t.Errorf("acknowledged selection lost: got %s want %s", got.Selected, selected)
					}
					if early {
						t.Error("overlapping mutation completed while transaction gate was held")
					}
				})
			}
		})
	}
}

// Real OS contention observes the production ten-second deadline, emits only
// busy, and preserves bytes; releasing the holder permits the identical command.
func TestRegistryLockBusyAndRecovery(t *testing.T) {
	binary := mutationBinary(t)
	s, _, _ := service(t)
	r := Empty()
	r.Managers["seed"] = entry(t)
	r.Selected = "seed"
	putRegistry(t, s.Resolver.Locations.User, r)
	path := filepath.Join(s.Resolver.Locations.User, Filename)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	holder := startMutation(t, binary, mutationJob{Resolver: s.Resolver, Gate: "hold"})
	holder.ready(t)
	command := Command{Operation: "register", Manifest: manifest(t, "new", entry(t)), NoSelect: true}
	contender := startMutation(t, binary, mutationJob{Resolver: s.Resolver, Command: command})
	result := contender.result(t)
	if result.Code != CodeBusy || result.Message != "session-manager registry is busy" || result.Elapsed < 10*time.Second || result.Elapsed > 12*time.Second {
		t.Fatalf("busy deadline/result: %+v", result)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("busy changed registry", err)
	}
	holder.release(t)
	if result := holder.result(t); result.Code != "" {
		t.Fatal(result)
	}
	control := startMutation(t, binary, mutationJob{Resolver: s.Resolver, Command: command})
	if result := control.result(t); result.Code != "" {
		t.Fatal(result)
	}
	if _, ok := readTarget(t, s).Managers["new"]; !ok {
		t.Fatal("released lock blocked valid registration")
	}
}

// A killed holder releases an advisory lock despite the persistent lock file;
// the same mutation succeeds before and after the crash, without marker cleanup.
func TestRegistryLockCrashRecovery(t *testing.T) {
	binary := mutationBinary(t)
	s, _, _ := service(t)
	e := entry(t)
	command := Command{Operation: "register", Manifest: manifest(t, "new", e), NoSelect: true}
	if err := s.Run(command); err != nil {
		t.Fatal(err)
	}
	holder := startMutation(t, binary, mutationJob{Resolver: s.Resolver, Gate: "hold"})
	holder.ready(t)
	lockPath := filepath.Join(s.Resolver.Locations.User, Filename+".lock")
	before, err := os.Stat(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = unix.Kill(-holder.cmd.Process.Pid, unix.SIGKILL); err != nil {
		t.Fatal(err)
	}
	select {
	case <-holder.done:
	case <-time.After(2 * time.Second):
		t.Fatal("killed holder not reaped")
	}
	control := startMutation(t, binary, mutationJob{Resolver: s.Resolver, Command: command})
	if result := control.result(t); result.Code != "" {
		t.Fatal(result)
	}
	after, err := os.Stat(lockPath)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("lock inode replaced", err)
	}
	if _, ok := readTarget(t, s).Managers["new"]; !ok {
		t.Fatal("crash recovery lost registration")
	}
}

// The real descriptor reader expires silent and partial answers without writes
// or changing stdin flags; complete answers/EOF work and timeout releases the lock.
// Tests shorten only the answer timeout, while pinning both production defaults.
func TestRegistryPromptDeadline(t *testing.T) {
	if registryLockTimeout != 10*time.Second || promptAnswerTimeout != 120*time.Second {
		t.Fatal("production deadlines changed")
	}
	for _, kind := range []string{"silent", "partial", "yes", "no", "eof"} {
		t.Run(kind, func(t *testing.T) {
			s, _, _ := service(t)
			r := Empty()
			e := entry(t)
			r.Managers["seed"] = e
			r.Selected = "seed"
			putRegistry(t, s.Resolver.Locations.User, r)
			path := filepath.Join(s.Resolver.Locations.User, Filename)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			in, out, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer in.Close()
			defer out.Close()
			s.Stdin = in
			s.IsTerminal = func() bool { return true }
			s.promptTimeout = 60 * time.Millisecond
			flags := descriptorFlags(t, in)
			switch kind {
			case "partial":
				_, err = out.WriteString("y")
			case "yes":
				_, err = out.WriteString("yes\n")
			case "no":
				_, err = out.WriteString("n\n")
			case "eof":
				err = out.Close()
			}
			if err != nil {
				t.Fatal(err)
			}
			command := Command{Operation: "register", Manifest: manifest(t, "new", e)}
			start := time.Now()
			err = s.Run(command)
			if got := descriptorFlags(t, in); got != flags {
				t.Errorf("prompt changed descriptor flags: got %d want %d", got, flags)
			}
			if kind == "silent" || kind == "partial" {
				requireCode(t, err, "usage", "")
				if time.Since(start) < s.promptTimeout || time.Since(start) > time.Second {
					t.Fatal("answer deadline not observed")
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("timeout changed registry", err)
				}
				if _, err = in.Stat(); err != nil {
					t.Fatal("timeout closed stdin", err)
				}
				s.Stdin = bytes.NewBufferString("yes\n")
				if err = s.Run(command); err != nil {
					t.Fatal("retry after timeout", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			got := readTarget(t, s)
			want := "seed"
			if kind == "yes" || kind == "silent" || kind == "partial" {
				want = "new"
			}
			if got.Selected != want || len(got.Managers) != 2 {
				t.Fatal(got)
			}
		})
	}
}

// Refused lock inputs never follow a symlink or open a FIFO indefinitely;
// replacing each fixture with a regular lock enables the same valid mutation.
func TestRegistryLockFileSafety(t *testing.T) {
	for _, kind := range []string{"symlink", "fifo", "writable"} {
		t.Run(kind, func(t *testing.T) {
			s, _, _ := service(t)
			r := Empty()
			r.Managers["seed"] = entry(t)
			putRegistry(t, s.Resolver.Locations.User, r)
			path := filepath.Join(s.Resolver.Locations.User, Filename)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lockPath := path + ".lock"
			switch kind {
			case "symlink":
				err = os.Symlink(path, lockPath)
			case "fifo":
				err = unix.Mkfifo(lockPath, 0600)
			case "writable":
				err = os.WriteFile(lockPath, nil, 0666)
				if err == nil {
					err = os.Chmod(lockPath, 0666)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			command := Command{Operation: "select", ID: "seed"}
			requireCode(t, s.Run(command), CodeInvalid, "unreadable")
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("unsafe lock changed registry", err)
			}
			if err = os.Remove(lockPath); err != nil {
				t.Fatal(err)
			}
			if err = s.Run(command); err != nil {
				t.Fatal(err)
			}
			if readTarget(t, s).Selected != "seed" {
				t.Fatal("safe lock control failed")
			}
		})
	}
}

func descriptorFlags(t *testing.T, f *os.File) int {
	t.Helper()
	raw, err := f.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var flags int
	var flagErr error
	if err = raw.Control(func(fd uintptr) { flags, flagErr = unix.FcntlInt(fd, unix.F_GETFL, 0) }); err != nil {
		t.Fatal(err)
	}
	if flagErr != nil {
		t.Fatal(flagErr)
	}
	return flags
}
