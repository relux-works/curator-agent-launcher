package m0adapter

import (
	"io"
	"strings"
)

// These local admission bounds reserve finite storage for the raw environment.
// They do not select a platform exec limit or permit any name outside the six.
const maxEnvironmentBytes = 16384
const maxEnvironmentEntries = 64

// readRawEnvironment is the sole production source: the kernel's original
// environment block, never the runtime's normalized copy. Read it once, after
// durable C and before any environment mutation; preserve every entry and
// its original order. openEnviron admits only a regular file on procfs.
func readRawEnvironment() ([]string, error) {
	return readEnvironmentWith(func() (io.ReadCloser, error) {
		return openEnviron("/proc/self/environ")
	})
}

func readEnvironmentWith(open func() (io.ReadCloser, error)) ([]string, error) {
	f, err := open()
	if err != nil {
		return nil, refuse(reasonEnvRead)
	}
	b, err := io.ReadAll(io.LimitReader(f, maxEnvironmentBytes+1))
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return nil, refuse(reasonEnvRead)
	}
	if len(b) > maxEnvironmentBytes {
		return nil, refuse(reasonEnvLimit)
	}
	if len(b) == 0 {
		return nil, nil // checkEnv reports the missing six names
	}
	if b[len(b)-1] != 0 {
		return nil, refuse(ReasonEnvMalformed)
	}
	if strings.Count(string(b), "\x00") > maxEnvironmentEntries {
		return nil, refuse(reasonEnvLimit)
	}
	// Remove exactly the terminator, not empty entries preceding it.
	return strings.Split(string(b[:len(b)-1]), "\x00"), nil
}
