//go:build !linux

package m0adapter

// helperMain has no process modes off Linux; the process tests are Linux-only.
func helperMain() {}

// isolateDescriptors is a no-op off Linux, where no process test runs.
func isolateDescriptors() {}
