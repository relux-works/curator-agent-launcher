package execution

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"unsafe"
)

type processExit struct{ status syscall.WaitStatus }

func (e *processExit) Error() string { return fmt.Sprintf("child status: %d", e.status) }
func (e *processExit) ExitCode() int { return e.status.ExitStatus() }

func terminalGroup(fd uintptr, request uintptr, group *int32) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(unsafe.Pointer(group)))
	if errno != 0 {
		return errno
	}
	return nil
}

// One waited child group owns the terminal. The launcher never receives the
// child's terminal-generated signals, so forwarding cannot duplicate them.
func run(cmd *exec.Cmd) error {
	signals := make(chan os.Signal, 8)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGCONT)
	defer signal.Stop(signals)
	tty, _ := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	var original int32
	if tty != nil {
		defer tty.Close()
		if terminalGroup(tty.Fd(), syscall.TIOCGPGRP, &original) != nil || original != int32(syscall.Getpgrp()) {
			tty = nil
		}
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if tty != nil {
		cmd.SysProcAttr.Foreground = true
		cmd.SysProcAttr.Ctty = int(tty.Fd())
		// The launcher is in the background while restoring ownership. Ignore the
		// terminal's SIGTTOU for these ioctls, as a job-control shell does.
		signal.Ignore(syscall.SIGTTOU)
		defer signal.Reset(syscall.SIGTTOU)
		defer terminalGroup(tty.Fd(), syscall.TIOCSPGRP, &original)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	group := int32(cmd.Process.Pid)
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			select {
			case s := <-signals:
				if s == syscall.SIGCONT && tty != nil {
					_ = terminalGroup(tty.Fd(), syscall.TIOCSPGRP, &group)
				}
				_ = syscall.Kill(-int(group), s.(syscall.Signal))
			case <-done:
				return
			}
		}
	}()
	var status syscall.WaitStatus
	var err error
	for {
		_, err = syscall.Wait4(cmd.Process.Pid, &status, syscall.WUNTRACED, nil)
		if err == syscall.EINTR {
			continue
		}
		if err != nil || !status.Stopped() {
			break
		}
		if tty != nil {
			_ = terminalGroup(tty.Fd(), syscall.TIOCSPGRP, &original)
		}
		// SIGSTOP cannot be caught or inherited as ignored. The caller's shell can
		// now observe a stopped launcher and resume it with fg or SIGCONT.
		_ = syscall.Kill(os.Getpid(), syscall.SIGSTOP)
	}
	close(done)
	<-stopped
	// Wait4 owns reaping; Cmd.Wait still closes descriptors and joins its I/O
	// copying goroutines. Its ECHILD is expected after the explicit wait above.
	_ = cmd.Wait()
	if err != nil {
		return err
	}
	if status != 0 {
		return &processExit{status}
	}
	return nil
}
