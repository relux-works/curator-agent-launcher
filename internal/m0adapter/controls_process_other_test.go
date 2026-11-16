//go:build !linux

package m0adapter

import "testing"

func adapterInstanceTests() map[string][]controlInstance { return nil }

func addProcessControlTests(cases map[string]func(*testing.T)) map[string]func(*testing.T) {
	for _, id := range []string{"home-prompt", "fd3-left-open", "environment", "prompt-replay", "runner-variant-selection", "status-remap"} {
		cases[id] = func(t *testing.T) { t.Skip("adapter process controls require Linux") }
	}
	return cases
}
