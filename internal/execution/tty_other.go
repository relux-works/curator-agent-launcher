//go:build !linux && !darwin

package execution

func isTerminalFD(uintptr) bool { return false }
