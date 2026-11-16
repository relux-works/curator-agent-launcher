package m0adapter

// The three refusing seams. Each is the single place where a mechanism that
// the spec leaves unpublished would be pinned. Until then each refuses 78, so
// the installed adapter refuses every launch. Tests substitute fakes through
// Config; production Main always passes these functions.

// waitForC is the spec §5 seam for the selected A's receiver-owned durable C,
// which must be established after root receipt/verified drop and before ANY
// target code. The owning mechanism is unpublished and unqualified, so this
// refuses c_unpublished. Known residual for review: the Go runtime and package
// initialisation run before main and therefore before this seam.
func waitForC() error {
	return refuse(ReasonCUnpublished)
}

// qualifiedNowMs is the spec §5 seam for the only admitted time source: now_ms
// in the exact qualified tuple's coordinate, selected by the immutable clock
// profile literal. No clock profile is published or qualified, so this
// refuses clock_unqualified for every profile. It must never fall back to the
// wall clock or another unqualified source.
func qualifiedNowMs(profile string) (int64, error) {
	_ = profile
	return 0, refuse(ReasonClockUnqualified)
}

// claudeRunnerArgv is the seam for a Claude runner command from a future
// broker slice (S3a). Its argv shape is unpinned, so this refuses
// runner_variant_unpinned for every command; no Claude flag is invented here.
// When the slice is pinned, the runnerCommand literal names the broker's
// Claude command and this function builds its exact argv from the same
// mapping inputs.
func claudeRunnerArgv(command string, m mapping) ([]string, error) {
	_, _ = command, m
	return nil, refuse(ReasonRunnerVariantPending)
}
