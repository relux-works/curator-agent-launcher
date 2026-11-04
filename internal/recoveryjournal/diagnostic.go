package recoveryjournal

import "errors"

const (
	CodeStateInvalid  = "gateway_recovery_state_invalid"
	CodeUnconfirmed   = "gateway_recovery_unconfirmed"
	CodeLocalEvidence = "local_evidence_failed"
)

// FailureCode exposes only a fixed category, never the filesystem, catalog,
// private entry, capability or raw transport error that caused the refusal.
func FailureCode(err error) string {
	if errors.Is(err, ErrLocalEvidence) {
		return CodeLocalEvidence
	}
	for _, known := range []error{ErrBinding, ErrBindingChanged, ErrUnsafeLayout, ErrLayoutIO, ErrUnsupported, ErrLock, ErrRead, ErrCapacity, ErrExisting} {
		if errors.Is(err, known) {
			return CodeStateInvalid
		}
	}
	return CodeUnconfirmed
}
