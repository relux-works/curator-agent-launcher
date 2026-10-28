package memberobjects

import "golang.org/x/sys/unix"

func readableFlags(flags int) bool {
	return flags&unix.O_ACCMODE == unix.O_RDONLY && flags&unix.O_EVTONLY == 0
}
func statTimes(s *unix.Stat_t) (timestamp, timestamp) {
	return timestamp{int64(s.Mtim.Sec), int64(s.Mtim.Nsec)}, timestamp{int64(s.Ctim.Sec), int64(s.Ctim.Nsec)}
}
