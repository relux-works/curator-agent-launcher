// Cooperative r6 §4 descriptor receiver for the launcher-side contract
// test. Receiver mode reports the ingress close-on-exec flags of fd3
// and fd4, sets close-on-exec immediately (the receiver's contract
// obligation), spawns a grandchild while both descriptors are still
// open, and reports the grandchild's view. Grandchild mode reports
// whether the passed terminal/pipe identities survived its exec. All reports go to stdout:
//
//	ingress fd3_cloexec=false fd4_cloexec=false
//	marked fd3_cloexec=true fd4_cloexec=true
//	grandchild fd3_inherited=false fd4_inherited=false
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"golang.org/x/sys/unix"
)

func cloexec(fd uintptr) bool {
	flags, err := unix.FcntlInt(fd, unix.F_GETFD, 0)
	if err != nil {
		return false
	}
	return flags&unix.FD_CLOEXEC != 0
}

// Identity, not descriptor number: Linux's Go runtime can reuse fd3 for epoll.
func identity(fd int) (string, bool) {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		if err == unix.EBADF {
			return "closed", false
		}
		fail(err)
	}
	return fmt.Sprintf("%d:%d", st.Dev, st.Ino), true
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "fdcontract receiver:", err)
	os.Exit(99)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "grandchild" {
		if len(os.Args) != 4 {
			fail(fmt.Errorf("missing receiver identities"))
		}
		terminal, terminalOpen := identity(3)
		pipe, pipeOpen := identity(4)
		fmt.Printf("grandchild fd3_inherited=%v fd4_inherited=%v\n", terminalOpen && terminal == os.Args[2], pipeOpen && pipe == os.Args[3])
		fmt.Printf("grandchild identities fd3=%s fd4=%s\n", terminal, pipe)
		return
	}
	fmt.Printf("ingress fd3_cloexec=%v fd4_cloexec=%v\n", cloexec(3), cloexec(4))
	terminal, _ := identity(3)
	pipe, _ := identity(4)
	fmt.Printf("receiver identities fd3=%s fd4=%s\n", terminal, pipe)
	unix.CloseOnExec(3)
	unix.CloseOnExec(4)
	fmt.Printf("marked fd3_cloexec=%v fd4_cloexec=%v\n", cloexec(3), cloexec(4))
	// Drain the payload stdin so the launcher's copy never blocks; the
	// status pipe carries no records for this shape.
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		fail(err)
	}
	// Spawn while fd3/fd4 are still open: only close-on-exec — not a
	// close — may hide them from the grandchild.
	cmd := exec.Command(os.Args[0], "grandchild", terminal, pipe)
	// Explicit negative controls prove identity detects either passed resource,
	// while an unrelated descriptor at the same number is harmless.
	switch os.Getenv("FD_CONTRACT_DESCENDANT") {
	case "terminal":
		cmd.ExtraFiles = []*os.File{os.NewFile(3, "terminal")}
	case "pipe":
		cmd.ExtraFiles = []*os.File{nil, os.NewFile(4, "status")}
	case "unrelated":
		unrelated, err := os.Open(os.Getenv("FD_CONTRACT_UNRELATED"))
		if err != nil {
			fail(err)
		}
		defer unrelated.Close()
		cmd.ExtraFiles = []*os.File{unrelated}
	}
	out, err := cmd.Output()
	if err != nil {
		fail(err)
	}
	fmt.Printf("%s", out)
	status := os.NewFile(uintptr(4), "status")
	if status != nil {
		status.Close()
	}
}
