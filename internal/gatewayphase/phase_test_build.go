//go:build gatewayphases

package gatewayphase

import (
	"os"
	"path/filepath"
	"time"
)

// Reach, in a build for phase tests, records that the phase was reached in
// $CURATOR_TEST_PHASE_DIR and waits, at most ten seconds, until the test
// releases it. Without that variable it does nothing.
func Reach(name string) {
	dir := os.Getenv("CURATOR_TEST_PHASE_DIR")
	if dir == "" {
		return
	}
	if os.WriteFile(filepath.Join(dir, name+".reached"), nil, 0o600) != nil {
		return
	}
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(dir, name+".release")); err == nil {
			return
		}
	}
}
