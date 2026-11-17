//go:build !race

package m0adapter

// raceEnabled is false outside -race; see skipUnderRace.
const raceEnabled = false
