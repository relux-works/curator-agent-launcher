//go:build !unix

package hosted

import (
	"errors"
	"os"
)

// Non-Unix platforms have no fd-passing contract (Go ExtraFiles is
// unsupported on Windows): marking is a no-op and validation checks
// only that the descriptor is open and not a directory. Hosted
// launches there keep their existing behavior; the r6 §4 descriptor
// mechanism is established and tested on Unix.
func MarkCloseOnExec(f *os.File) {}

func ValidateTerminal(f *os.File) error {
	if f == nil {
		return errors.New("terminal descriptor is missing")
	}
	st, err := f.Stat()
	if err != nil {
		return errors.New("terminal descriptor is not statable")
	}
	if st.IsDir() {
		return errors.New("terminal descriptor is a directory")
	}
	return nil
}

func validateStatusPipe(f *os.File) error {
	if f == nil {
		return errors.New("status descriptor is missing")
	}
	if _, err := f.Stat(); err != nil {
		return errors.New("status descriptor is not statable")
	}
	return nil
}
