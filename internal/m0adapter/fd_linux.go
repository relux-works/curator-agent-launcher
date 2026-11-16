//go:build linux

package m0adapter

import (
	"syscall"
)

// planFD is the manager-delivered immutable plan descriptor (spec §1).
const planFD = 3

// maxInventory bounds one /proc/self/fd listing.
const maxInventory = 64

// Linux fcntl commands not exported by package syscall.
const (
	fSetPipeSz = 1031 // F_SETPIPE_SZ
	fGetPipeSz = 1032 // F_GETPIPE_SZ
)

// fileID is kernel object identity by (device, inode).
type fileID struct{ dev, ino uint64 }

func fstatID(fd int) (fileID, syscall.Stat_t, error) {
	var st syscall.Stat_t
	for {
		err := syscall.Fstat(fd, &st)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return fileID{}, st, err
		}
		return fileID{dev: uint64(st.Dev), ino: uint64(st.Ino)}, st, nil
	}
}

func fcntl(fd, cmd, arg int) (int, error) {
	r, _, e := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(cmd), uintptr(arg))
	if e != 0 {
		return -1, e
	}
	return int(r), nil
}

// inherited reports whether fd lacks FD_CLOEXEC. A descriptor inherited
// across execve necessarily has FD_CLOEXEC clear; one with it set was
// created in this process image (the Go runtime's poller, or this package's
// own transient descriptors) and is closed by the kernel at execve.
func inherited(fd int) (bool, error) {
	flags, err := fcntl(fd, syscall.F_GETFD, 0)
	if err != nil {
		return false, err
	}
	return flags&syscall.FD_CLOEXEC == 0, nil
}

// listFDs returns every open descriptor number except the listing's own
// directory descriptor. It is one bounded directory read, not a tree walk.
func listFDs() ([]int, error) {
	dir, err := syscall.Open("/proc/self/fd", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, refuse(ReasonFDInventory)
	}
	defer syscall.Close(dir)
	var buf [4096]byte
	var fds []int
	for {
		n, err := syscall.ReadDirent(dir, buf[:])
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return nil, refuse(ReasonFDInventory)
		}
		if n <= 0 {
			return fds, nil
		}
		_, _, names := syscall.ParseDirent(buf[:n], -1, nil)
		for _, name := range names {
			fd, ok := parseFD(name)
			if !ok {
				return nil, refuse(ReasonFDInventory)
			}
			if fd == dir {
				continue
			}
			if len(fds) == maxInventory {
				return nil, refuse(ReasonFDInventoryLimit)
			}
			fds = append(fds, fd)
		}
	}
}

func parseFD(s string) (int, bool) {
	if len(s) == 0 || len(s) > 9 {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, true
}

// isPlanDuplicate is the kernel-object identity comparison used by every
// duplicate sweep: any descriptor other than FD3 itself naming the plan's
// (dev, ino) is a duplicate, whatever its number.
func isPlanDuplicate(id, plan fileID) bool {
	return id == plan
}

// planSnapshot is the FD3 identity and size observed at entry.
type planSnapshot struct {
	id   fileID
	size int64
}

// entryInventory checks the descriptors present at entry: 0, 1, 2 and the
// plan at 3 are required; no other inherited descriptor and no duplicate of
// the plan object (including at 0-2) is admitted. FD3 must be a read-only
// regular file no larger than the encoded bound; nothing is read yet.
func entryInventory() (planSnapshot, error) {
	fds, err := listFDs()
	if err != nil {
		return planSnapshot{}, err
	}
	var have [planFD + 1]bool
	for _, fd := range fds {
		if fd <= planFD {
			have[fd] = true
		}
	}
	if !have[0] || !have[1] || !have[2] {
		return planSnapshot{}, refuse(ReasonFDStandardMissing)
	}
	if !have[planFD] {
		return planSnapshot{}, refuse(ReasonFDPlanMissing)
	}
	// Only an inherited descriptor can be the manager's plan. A descriptor at
	// 3 with close-on-exec set was created in this process image (for example
	// by the Go runtime before main), so the plan is missing, not that object.
	if inh, err := inherited(planFD); err != nil || !inh {
		return planSnapshot{}, refuse(ReasonFDPlanMissing)
	}
	plan, st, err := fstatID(planFD)
	if err != nil {
		return planSnapshot{}, refuse(ReasonFDPlanRead)
	}
	if st.Mode&syscall.S_IFMT != syscall.S_IFREG {
		return planSnapshot{}, refuse(ReasonFDPlanType)
	}
	fl, err := fcntl(planFD, syscall.F_GETFL, 0)
	if err != nil || fl&syscall.O_ACCMODE != syscall.O_RDONLY {
		return planSnapshot{}, refuse(ReasonFDPlanMode)
	}
	for _, fd := range fds {
		if fd == planFD {
			continue
		}
		id, _, err := fstatID(fd)
		if err != nil {
			return planSnapshot{}, refuse(ReasonFDInventory)
		}
		if isPlanDuplicate(id, plan) {
			return planSnapshot{}, refuse(ReasonFDPlanDuplicate)
		}
	}
	for _, fd := range fds {
		if fd <= planFD {
			continue
		}
		inh, err := inherited(fd)
		if err != nil {
			return planSnapshot{}, refuse(ReasonFDInventory)
		}
		if inh {
			return planSnapshot{}, refuse(ReasonFDUnexpected)
		}
	}
	if st.Size < 0 || st.Size > maxEncoded {
		return planSnapshot{}, refuse(ReasonEntrySizeLimit)
	}
	return planSnapshot{id: plan, size: st.Size}, nil
}

// readPlan reads FD3 from offset 0 into one buffer of maxEncoded+1 bytes, so
// excess is detected by scanning at most limit+1 bytes. The plan must not
// change identity or size while it is read.
func readPlan(snap planSnapshot) ([]byte, error) {
	buf := make([]byte, maxEncoded+1)
	n := 0
	for n < len(buf) {
		m, err := syscall.Pread(planFD, buf[n:], int64(n))
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return nil, refuse(ReasonFDPlanRead)
		}
		if m <= 0 {
			break
		}
		n += m
	}
	if n > maxEncoded {
		return nil, refuse(ReasonEntrySizeLimit)
	}
	id, st, err := fstatID(planFD)
	if err != nil {
		return nil, refuse(ReasonFDPlanRead)
	}
	if id != snap.id || st.Size != snap.size || int64(n) != snap.size {
		return nil, refuse(ReasonFDPlanUnstable)
	}
	return buf[:n], nil
}

// closePlanAndSweep closes FD3 and every other inherited descriptor above 2,
// then verifies by kernel object identity that no descriptor still names the
// plan object and that only 0-2 remain inherited. It runs before the stdin
// pipe exists.
func closePlanAndSweep(plan fileID) error {
	if err := syscall.Close(planFD); err != nil {
		return refuse(ReasonFDClose)
	}
	fds, err := listFDs()
	if err != nil {
		return err
	}
	for _, fd := range fds {
		if fd <= 2 {
			continue
		}
		inh, err := inherited(fd)
		if err != nil {
			return refuse(ReasonFDSweep)
		}
		if inh {
			if err := syscall.Close(fd); err != nil {
				return refuse(ReasonFDClose)
			}
		}
	}
	return verifyDescriptors(plan, nil)
}

// verifyDescriptors is the post-close and pre-exec check: 0-2 are present,
// nothing above 2 is inherited, no descriptor names the plan object, and
// when stdin is given, fd 0 is exactly that read-only pipe object and no
// other descriptor (such as a leaked write end) names it.
func verifyDescriptors(plan fileID, stdin *fileID) error {
	fds, err := listFDs()
	if err != nil {
		return err
	}
	have := 0
	for _, fd := range fds {
		if fd <= 2 {
			have++
		}
		id, _, err := fstatID(fd)
		if err != nil {
			return refuse(ReasonFDSweep)
		}
		if isPlanDuplicate(id, plan) {
			return refuse(ReasonFDPlanDuplicate)
		}
		if stdin != nil {
			if fd == 0 {
				fl, err := fcntl(0, syscall.F_GETFL, 0)
				if err != nil || id != *stdin || fl&syscall.O_ACCMODE != syscall.O_RDONLY {
					return refuse(ReasonFDFinal)
				}
			} else if id == *stdin {
				return refuse(ReasonFDFinal)
			}
		}
		if fd > 2 {
			inh, err := inherited(fd)
			if err != nil || inh {
				return refuse(ReasonFDSweep)
			}
		}
	}
	if have != 3 {
		return refuse(ReasonFDStandardMissing)
	}
	return nil
}

// installPrompt replaces the feeder of the spec with one bounded write: a
// fresh pipe whose capacity, read with F_GETPIPE_SZ, must already hold the
// whole prompt receives the exact prompt bytes once, without blocking; the
// write end is closed, so the reader sees EOF right after the prompt; the read
// end becomes fd 0. Nothing waits for a reader. A capacity below the prompt
// length, or a failed F_GETPIPE_SZ, refuses before any byte is written; the
// capacity is never raised (a review condition on the no-feeder design).
func installPrompt(prompt []byte) (fileID, error) {
	var p [2]int
	if err := syscall.Pipe2(p[:], syscall.O_CLOEXEC); err != nil {
		return fileID{}, refuse(ReasonStdinPipe)
	}
	return fillPromptPipe(p[0], p[1], prompt)
}

// fillPromptPipe performs installPrompt's checks and write on an already
// created pipe (r, w). It owns both descriptors: on refusal it closes them.
func fillPromptPipe(r, w int, prompt []byte) (fileID, error) {
	fail := func(reason Reason) (fileID, error) {
		syscall.Close(r)
		if w >= 0 {
			syscall.Close(w)
		}
		return fileID{}, refuse(reason)
	}
	if r <= 2 || w <= 2 {
		return fail(ReasonStdinPipe)
	}
	size, err := fcntl(w, fGetPipeSz, 0)
	if err != nil {
		return fail(ReasonStdinCapacity)
	}
	if size < len(prompt) {
		return fail(ReasonStdinCapacity)
	}
	if err := syscall.SetNonblock(w, true); err != nil {
		return fail(ReasonStdinWrite)
	}
	// One non-blocking write of the exact prompt bytes. EAGAIN, a short write
	// or any other error refuses; nothing is retried after a byte is written
	// and a blocking write is never used. EINTR before any byte is written is
	// retried, since the pipe is still empty.
	for {
		n, err := syscall.Write(w, prompt)
		if err == syscall.EINTR && n <= 0 {
			continue
		}
		if err != nil || n != len(prompt) {
			return fail(ReasonStdinWrite)
		}
		break
	}
	err = syscall.Close(w)
	w = -1
	if err != nil {
		return fail(ReasonStdinWrite)
	}
	id, _, err := fstatID(r)
	if err != nil {
		return fail(ReasonStdinPipe)
	}
	if err := syscall.Dup3(r, 0, 0); err != nil {
		return fail(ReasonStdinPipe)
	}
	if err := syscall.Close(r); err != nil {
		return fileID{}, refuse(ReasonStdinPipe)
	}
	return id, nil
}
