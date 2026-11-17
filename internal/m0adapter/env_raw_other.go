//go:build !linux

package m0adapter

import "io"

// openEnviron has no procfs source off Linux; Run already refuses there.
func openEnviron(path string) (io.ReadCloser, error) {
	_ = path
	return nil, refuse(reasonEnvRead)
}
