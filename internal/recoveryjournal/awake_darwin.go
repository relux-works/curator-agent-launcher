//go:build darwin

package recoveryjournal

import (
	"golang.org/x/sys/unix"
	"strings"
)

func nativeAwake() (awakeObservation, error) {
	boot, err := unix.Sysctl("kern.bootsessionuuid")
	if err != nil {
		return awakeObservation{}, ErrLocalEvidence
	}
	var ts unix.Timespec
	// Darwin CLOCK_UPTIME_RAW excludes system sleep; CLOCK_MONOTONIC does not
	// provide this contract. Neither receipt UTC nor process uptime is used.
	if unix.ClockGettime(unix.CLOCK_UPTIME_RAW, &ts) != nil {
		return awakeObservation{}, ErrLocalEvidence
	}
	return awakeTimespec(strings.ToLower(boot), ts.Sec, ts.Nsec)
}
