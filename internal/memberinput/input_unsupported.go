//go:build !darwin && !linux

package memberinput

import "os"

func readPairPlatform(_, _ *os.File) (*UntrustedPair, *Error) {
	// No syscall, stat, read, duplication or close on unsupported platforms.
	return nil, descriptorError()
}
