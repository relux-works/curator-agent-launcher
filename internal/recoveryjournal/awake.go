package recoveryjournal

import "math"

func awakeTimespec(boot string, sec, ns int64) (awakeObservation, error) {
	if !bootUUID(boot) || sec < 0 || ns < 0 || ns >= 1000000000 || uint64(sec) > (math.MaxUint64-uint64(ns))/1000000000 {
		return awakeObservation{}, ErrLocalEvidence
	}
	return awakeObservation{boot, uint64(sec)*1000000000 + uint64(ns)}, nil
}

func sameLaunch(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
