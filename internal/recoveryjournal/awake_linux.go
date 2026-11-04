//go:build linux

package recoveryjournal

import (
	"golang.org/x/sys/unix"
	"io"
	"os"
	"strings"
)

func nativeAwake() (awakeObservation, error) {
	fd, err := unix.Open("/proc/sys/kernel/random/boot_id", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return awakeObservation{}, ErrLocalEvidence
	}
	f := os.NewFile(uintptr(fd), "")
	raw, readErr := io.ReadAll(io.LimitReader(f, 38))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || len(raw) != 37 || raw[36] != '\n' {
		return awakeObservation{}, ErrLocalEvidence
	}
	var ts unix.Timespec
	// Linux CLOCK_MONOTONIC excludes suspend; CLOCK_BOOTTIME includes it.
	if unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts) != nil {
		return awakeObservation{}, ErrLocalEvidence
	}
	return awakeTimespec(strings.TrimSuffix(string(raw), "\n"), ts.Sec, ts.Nsec)
}
