//go:build darwin || linux

package recoveryjournal

import (
	"context"

	"golang.org/x/sys/unix"
)

// DiscoverBinding reads only the named original private record, under the
// caller's captured state location and cooperative lock. It validates complete
// closed record/binding and current catalog admission, returns no capability,
// and performs no service call, enumeration, repair or registration. The only
// possible write is creation/sync of the admitted private namespace lock.
// Future recovery must reopen and validate that exact binding/entry again.
func DiscoverBinding(ctx context.Context, parentEnv []string, attempt string) (binding Binding, failure error) {
	if !digest(attempt) {
		return Binding{}, ErrRead
	}
	if _, ok := ctx.Deadline(); !ok || ctx.Err() != nil {
		return Binding{}, ErrLock
	}
	in, err := callerState(parentEnv)
	if err != nil {
		return Binding{}, err
	}
	l, err := resolveLocation(in.XDGStateHome, in.Home)
	if err != nil {
		return Binding{}, err
	}
	n, err := openLocation(l)
	if err != nil {
		return Binding{}, err
	}
	defer func() {
		if n.Close() != nil {
			binding, failure = Binding{}, ErrRead
		}
	}()
	lock, err := n.lock(ctx)
	if err != nil {
		return Binding{}, err
	}
	defer func() {
		if unix.Close(lock) != nil {
			binding, failure = Binding{}, ErrLock
		}
	}()
	raw, err := n.readRecordLocked(ctx, attempt, nativeReadOperations(), lock)
	if err != nil {
		return Binding{}, err
	}
	b, err := recordedBinding(raw, attempt, l)
	if err != nil {
		return Binding{}, err
	}
	if err := CheckCatalog(parentEnv, b); err != nil {
		return Binding{}, err
	}
	if ctx.Err() != nil || !n.sameLocation() || !lockStillBound(n.directory, lock) {
		return Binding{}, ErrRead
	}
	return b, nil
}
