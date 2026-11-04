//go:build darwin || linux

package recoveryjournal

import (
	"golang.org/x/sys/unix"
	"io"
	"os"
	"strings"
)

// Retained/orphan public receipts count toward the same admitted 4096-attempt
// capacity. No timestamp or missing secret makes a receipt disposable here.
func (n *Namespace) receiptCapacity() (failure error) {
	fd, err := openDirectory(n.receipts, ".")
	if err != nil {
		return ErrUnsafeLayout
	}
	f := os.NewFile(uintptr(fd), "")
	defer func() {
		if f.Close() != nil && failure == nil {
			failure = ErrLayoutIO
		}
	}()
	names, err := f.Readdirnames(MaxAttempts + 1)
	if err != nil && err != io.EOF {
		return ErrUnsafeLayout
	}
	if len(names) >= MaxAttempts {
		return ErrCapacity
	}
	for _, name := range names {
		valid := strings.HasSuffix(name, ".json") && digest(strings.TrimSuffix(name, ".json"))
		if strings.HasPrefix(name, ".receipt-") && strings.HasSuffix(name, ".tmp") {
			valid = digest(strings.TrimSuffix(strings.TrimPrefix(name, ".receipt-"), ".tmp"))
		}
		var st unix.Stat_t
		if !valid || unix.Fstatat(n.receipts, name, &st, unix.AT_SYMLINK_NOFOLLOW) != nil || !privateStat(st, st.Size) || st.Size < 0 || st.Size > MaxRecord {
			return ErrUnsafeLayout
		}
	}
	// Each bounded record plus one reserved future receipt stays within
	// MaxAttempts*MaxRecord; no content reads or broad cleanup are necessary.
	return nil
}
