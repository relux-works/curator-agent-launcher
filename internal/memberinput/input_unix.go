//go:build darwin || linux

package memberinput

import (
	"crypto/sha256"
	"os"

	"golang.org/x/sys/unix"
)

type timestamp struct{ seconds, nanos int64 }
type observation struct {
	device, inode     uint64
	uid, mode         uint32
	links             uint64
	size              int64
	flags             int
	modified, changed timestamp
}

type positionalRead func(int, []byte, int64) (int, error)

func readPairPlatform(n, a *os.File) (*UntrustedPair, *Error) {
	return readPair(n, a, unix.Pread)
}

// The per-call read syscall seam lets ordinary tests assert ordering and inject
// bounded read failures/real fixture changes. No global hook or alternate policy.
func readPair(n, a *os.File, readAt positionalRead) (*UntrustedPair, *Error) {
	if n == nil || a == nil {
		return nil, descriptorError()
	}
	nc, err := n.SyscallConn()
	if err != nil {
		return nil, descriptorError()
	}
	ac, err := a.SyscallConn()
	if err != nil {
		return nil, descriptorError()
	}
	var pair *UntrustedPair
	var refusal *Error
	err = nc.Control(func(nfd uintptr) {
		inner := ac.Control(func(afd uintptr) {
			pair, refusal = readDescriptors(int(nfd), int(afd), readAt)
		})
		if inner != nil {
			pair, refusal = nil, descriptorError()
		}
	})
	if err != nil {
		return nil, descriptorError()
	}
	return pair, refusal
}

func inspect(fd, limit int) (observation, *Error) {
	var s unix.Stat_t
	if err := unix.Fstat(fd, &s); err != nil {
		return observation{}, descriptorError()
	}
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if err != nil {
		return observation{}, descriptorError()
	}
	m, c := statTimes(&s)
	o := observation{uint64(s.Dev), uint64(s.Ino), s.Uid, uint32(s.Mode), uint64(s.Nlink), s.Size, flags, m, c}
	return o, validObservation(o, uint32(os.Getuid()), limit)
}

func validObservation(o observation, uid uint32, limit int) *Error {
	if o.mode != uint32(unix.S_IFREG|0600) || o.uid != uid || o.links != 1 ||
		!readableFlags(o.flags) || o.size < 0 {
		return descriptorError()
	}
	if o.size > int64(limit) {
		return boundsError()
	}
	return nil
}

func readDescriptors(n, a int, readAt positionalRead) (*UntrustedPair, *Error) {
	// Complete both descriptor/size checks BEFORE allocating or reading content.
	no, err := inspect(n, RequestLimit)
	if err != nil {
		return nil, err
	}
	ao, err := inspect(a, AuthorizationLimit)
	if err != nil {
		return nil, err
	}
	if no.device == ao.device && no.inode == ao.inode {
		return nil, descriptorError()
	}
	nr, err := readObserved(n, no.size, RequestLimit, readAt)
	if err != nil {
		return nil, err
	}
	ar, err := readObserved(a, ao.size, AuthorizationLimit, readAt)
	if err != nil {
		return nil, err
	}
	// Ignore access time, which this read itself may change. Compare actual native
	// device/inode, owner, mode, links, size, flags, mtime and ctime observations.
	nn, err := inspect(n, RequestLimit)
	if err != nil {
		return nil, err
	}
	aa, err := inspect(a, AuthorizationLimit)
	if err != nil {
		return nil, err
	}
	if nn != no || aa != ao {
		return nil, descriptorError()
	}
	return &UntrustedPair{nr, ar}, nil
}

func readObserved(fd int, size int64, limit int, readAt positionalRead) (*Snapshot, *Error) {
	// inspect bounds size first; one additional byte probes observed growth/EOF.
	buf := make([]byte, int(size)+1)
	total := 0
	for total < len(buf) {
		n, err := readAt(fd, buf[total:], int64(total))
		if n < 0 || n > len(buf)-total {
			return nil, descriptorError()
		}
		total += n
		if total > limit {
			return nil, boundsError()
		}
		if total > int(size) {
			return nil, descriptorError()
		}
		if err != nil {
			return nil, descriptorError()
		}
		if n == 0 {
			break
		}
	}
	if total != int(size) {
		return nil, descriptorError()
	}
	raw := buf[:total:total]
	return &Snapshot{raw, sha256.Sum256(raw)}, nil
}
