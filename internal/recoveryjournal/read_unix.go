//go:build darwin || linux

package recoveryjournal

import (
	"context"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

type readOperations struct {
	read  func(io.Reader) ([]byte, error)
	close func(*os.File) error
}

func nativeReadOperations() readOperations { return readOperations{io.ReadAll, (*os.File).Close} }

// ReadExact reads only one public attempt selector under the original admitted
// binding. It does not enumerate, repair, delete, rebind or send a request. It
// may create the declared private cooperative lock, just like Prepare.
func (n *Namespace) ReadExact(ctx context.Context, attempt string, current Binding) (*Entry, error) {
	return n.readExact(ctx, attempt, current, nativeReadOperations())
}

func (n *Namespace) readExact(ctx context.Context, attempt string, current Binding, ops readOperations) (entry *Entry, failure error) {
	if !digest(attempt) {
		return nil, ErrRead
	}
	if err := n.binding.Match(current); err != nil {
		return nil, err
	}
	if _, ok := ctx.Deadline(); !ok || ctx.Err() != nil {
		return nil, ErrLock
	}
	n.use.RLock()
	defer n.use.RUnlock()
	if n.closed || !n.sameLocation() {
		return nil, ErrUnsafeLayout
	}
	lock, err := n.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if unix.Close(lock) != nil {
			entry, failure = nil, ErrLock
		}
	}()
	if ctx.Err() != nil || !n.sameLocation() || !lockStillBound(n.directory, lock) {
		return nil, ErrRead
	}
	return n.readLocked(ctx, attempt, ops, lock)
}

// The caller holds n.use and the same bound namespace lock throughout this
// read and any following authenticated recovery/receipt transition.
func (n *Namespace) readLocked(ctx context.Context, attempt string, ops readOperations, lock int) (entry *Entry, failure error) {
	raw, err := n.readRecordLocked(ctx, attempt, ops, lock)
	if err != nil {
		return nil, err
	}
	return decodeEntry(raw, attempt, n.binding)
}

func (n *Namespace) readRecordLocked(ctx context.Context, attempt string, ops readOperations, lock int) (raw []byte, failure error) {
	name := attempt + ".json"
	fd, err := unix.Openat(n.secrets, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrRead
	}
	f := os.NewFile(uintptr(fd), "")
	closed := false
	defer func() {
		if !closed {
			if f.Close() != nil {
				raw, failure = nil, ErrRead
			}
		}
	}()
	var before, after unix.Stat_t
	if unix.Fstat(fd, &before) != nil || !privateStat(before, before.Size) || before.Size <= 0 || before.Size > MaxRecord || !entryMatches(n.secrets, name, before) {
		return nil, ErrRead
	}
	info, err := f.Stat()
	if err != nil {
		return nil, ErrRead
	}
	raw, err = ops.read(io.LimitReader(f, MaxRecord+1))
	if err != nil || int64(len(raw)) != before.Size || ctx.Err() != nil {
		return nil, ErrRead
	}
	updated, err := f.Stat()
	if err != nil || !updated.ModTime().Equal(info.ModTime()) || unix.Fstat(fd, &after) != nil || !privateStat(after, before.Size) || after.Dev != before.Dev || after.Ino != before.Ino {
		return nil, ErrRead
	}
	err = ops.close(f)
	closed = true // never retry an uncertain close on a reused fd
	if err != nil || ctx.Err() != nil || !n.sameLocation() || !lockStillBound(n.directory, lock) || !entryMatches(n.secrets, name, after) {
		return nil, ErrRead
	}
	return raw, nil
}
