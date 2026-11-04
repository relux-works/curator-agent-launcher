//go:build darwin || linux

package recoveryjournal

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

// Namespace retains independent directory descriptors. Opening it establishes
// read-only layout observations, not a hostile same-UID writer fence. Future
// writes must recheck their entry bindings under the namespace lock.
type Namespace struct {
	base, directory, secrets, receipts int
	binding                            Binding
	location                           location
	use                                sync.RWMutex
	closed                             bool
	closeOnce                          sync.Once
	closeErr                           error
}

// OpenExisting follows no selected path component and performs no creation,
// chmod, repair, enumeration, or authority-file read. Containers through the
// state base may be system-owned; the base and every namespace container must
// be caller-owned and non-writable by group/other. v1/secrets/receipts are 0700.
func OpenExisting(b Binding) (*Namespace, error) {
	if !b.valid() {
		return nil, ErrBinding
	}
	n, err := openLocation(location{b.base})
	if err != nil {
		return nil, err
	}
	n.binding = b
	return n, nil
}

// An unbound namespace is private to exact discovery. Mutating methods still
// require a complete admitted binding, never a fabricated profile/socket.
func openLocation(l location) (*Namespace, error) {
	if !l.valid() {
		return nil, ErrBinding
	}
	uid := uint32(os.Geteuid())
	base, err := openBase(l, uid)
	if err != nil {
		return nil, err
	}
	n := &Namespace{base: base, directory: -1, secrets: -1, receipts: -1, location: l}
	fail := func(err error) (*Namespace, error) {
		if n.Close() != nil {
			return nil, ErrLayoutIO
		}
		return nil, err
	}
	parent := base
	path := l.base
	parts := []string{"swarma", "launcher", "netproxy-recovery", "v1"}
	for i, part := range parts {
		fd, e := openDirectory(parent, part)
		if parent != base && unix.Close(parent) != nil {
			if fd >= 0 {
				unix.Close(fd)
			}
			return fail(ErrLayoutIO)
		}
		path = filepath.Join(path, part)
		if errors.Is(e, unix.ENOENT) {
			return fail(missingClosure(path, parts[i+1:], true))
		}
		if e != nil {
			return fail(ErrUnsafeLayout)
		}
		if !inspectDirectory(fd, uid, i == len(parts)-1) {
			unix.Close(fd)
			return fail(ErrUnsafeLayout)
		}
		parent = fd
	}
	n.directory = parent
	var missing []string
	for _, child := range []struct {
		name string
		fd   *int
	}{{"secrets", &n.secrets}, {"receipts", &n.receipts}} {
		fd, e := openDirectory(n.directory, child.name)
		if errors.Is(e, unix.ENOENT) {
			missing = append(missing, filepath.Join(l.directory(), child.name))
			continue
		}
		if e != nil {
			return fail(ErrUnsafeLayout)
		}
		*child.fd = fd
		if !inspectDirectory(fd, uid, true) {
			return fail(ErrUnsafeLayout)
		}
	}
	if len(missing) != 0 {
		return fail(&MissingDirectories{missing})
	}
	return n, nil
}

func openDirectory(parent int, name string) (int, error) {
	return unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
}

func openBase(l location, uid uint32) (int, error) {
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, ErrLayoutIO
	}
	parts := strings.Split(strings.TrimPrefix(l.base, "/"), "/")
	if l.base == "/" {
		parts = nil
	}
	path := "/"
	for i, part := range parts {
		next, e := openDirectory(fd, part)
		if unix.Close(fd) != nil {
			if next >= 0 {
				unix.Close(next)
			}
			return -1, ErrLayoutIO
		}
		path = filepath.Join(path, part)
		if errors.Is(e, unix.ENOENT) {
			remaining := append(append([]string(nil), parts[i+1:]...), "swarma", "launcher", "netproxy-recovery", "v1")
			return -1, missingClosure(path, remaining, true)
		}
		if e != nil {
			return -1, ErrUnsafeLayout
		}
		fd = next
	}
	if !inspectDirectory(fd, uid, false) {
		unix.Close(fd)
		return -1, ErrUnsafeLayout
	}
	return fd, nil
}

func inspectDirectory(fd int, uid uint32, private bool) bool {
	var s unix.Stat_t
	return unix.Fstat(fd, &s) == nil && validDirectory(s, uid, private)
}

func validDirectory(s unix.Stat_t, uid uint32, private bool) bool {
	mode := uint32(s.Mode)
	if mode&unix.S_IFMT != unix.S_IFDIR || s.Uid != uid || mode&07000 != 0 {
		return false
	}
	if private {
		return mode&0777 == 0700
	}
	return mode&0700 == 0700 && mode&0022 == 0
}

func missingClosure(first string, remaining []string, children bool) *MissingDirectories {
	targets := []string{first}
	path := first
	for _, part := range remaining {
		path = filepath.Join(path, part)
		targets = append(targets, path)
	}
	if children {
		targets = append(targets, filepath.Join(path, "secrets"), filepath.Join(path, "receipts"))
	}
	return &MissingDirectories{targets}
}

func (n *Namespace) Close() error {
	n.use.Lock()
	defer n.use.Unlock()
	n.closeOnce.Do(func() {
		n.closed = true
		for _, fd := range []int{n.receipts, n.secrets, n.directory, n.base} {
			if fd >= 0 && unix.Close(fd) != nil {
				n.closeErr = ErrLayoutIO
			}
		}
	})
	return n.closeErr
}
