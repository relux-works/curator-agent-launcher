//go:build !linux

package m0adapter

// Run refuses on every platform other than Linux: the descriptor inventory,
// pipe capacity control and exec contract are Linux-specific.
func Run(cfg Config) Reason {
	_ = cfg
	return ReasonPlatformUnsupported
}
