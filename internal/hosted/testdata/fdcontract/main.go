// Cooperative r6 §4 descriptor receiver for the launcher-side contract
// test. Receiver mode reports the ingress close-on-exec flags of fd3
// and fd4, sets close-on-exec immediately (the receiver's contract
// obligation), spawns a grandchild while both descriptors are still
// open, and reports the grandchild's view. Grandchild mode reports
// whether fd3/fd4 survived its exec. All reports go to stdout:
//
//	ingress fd3_cloexec=false fd4_cloexec=false
//	marked fd3_cloexec=true fd4_cloexec=true
//	grandchild fd3_open=false fd4_open=false
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

func isOpen(fd uintptr) bool {
	_, err := unix.FcntlInt(fd, unix.F_GETFD, 0)
	return err == nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "fdcontract receiver:", err)
	os.Exit(99)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "grandchild" {
		fmt.Printf("grandchild fd3_open=%v fd4_open=%v\n", isOpen(3), isOpen(4))
		return
	}
	fmt.Printf("ingress fd3_cloexec=%v fd4_cloexec=%v\n", cloexec(3), cloexec(4))
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
	cmd := exec.Command(os.Args[0], "grandchild")
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
