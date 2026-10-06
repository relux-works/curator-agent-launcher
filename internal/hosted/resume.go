package hosted

import (
	"errors"

	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

// ElevateResume runs the module typed-intent elevation for the admitted
// system: wrapperArgs is the launcher-level selector (nil, ["resume"],
// ["resume", SES], or ["--resume", ID]) and nativeArgs is the verbatim
// native tail. The module owns every provider grammar; the launcher never
// parses provider flags. Any elevation failure is session_resume_invalid,
// and the returned tail excludes elevated selectors only.
func ElevateResume(reg *agentic.Registry, system string, wrapperArgs, nativeArgs []string) (agentic.ResumeElevation, error) {
	if reg == nil {
		return agentic.ResumeElevation{}, &ResumeError{Reason: "no resume registry wired"}
	}
	elevation, err := agentic.ElevateResumeIntent(reg, agentic.SystemID(system), wrapperArgs, nativeArgs)
	if err != nil {
		var invalid *agentic.ResumeInvalidError
		if errors.As(err, &invalid) {
			// The module deliberately omits caller identities from the
			// reason; carry it verbatim and add nothing.
			return agentic.ResumeElevation{}, &ResumeError{Reason: invalid.Reason}
		}
		return agentic.ResumeElevation{}, &ResumeError{Reason: "resume elevation failed"}
	}
	return elevation, nil
}

// ResumeError is the session_resume_invalid family: conflicting, ambiguous,
// or malformed resume selectors. Reason carries the module's
// identity-free reason only.
type ResumeError struct {
	Reason string
}

func (e *ResumeError) Error() string { return CodeSessionResumeInvalid + ": " + e.Reason }

// WrapperArgs builds the module wrapper selector from the parsed CLI
// resume members. Both selector forms at once conflict and refuse here,
// before the module call, with no identity in the detail.
func WrapperArgs(resumeRequested bool, handle string, handleSet bool, resumeID string, resumeIDSet bool) ([]string, error) {
	if resumeRequested && resumeIDSet {
		return nil, &ResumeError{Reason: "conflicting selectors"}
	}
	if !resumeRequested && !resumeIDSet {
		return nil, nil
	}
	if resumeIDSet {
		return []string{"--resume", resumeID}, nil
	}
	if !handleSet {
		return []string{"resume"}, nil
	}
	return []string{"resume", handle}, nil
}
