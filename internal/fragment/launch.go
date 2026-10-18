package fragment

import "fmt"

const CodeVersionUnsupported = "fragment_version_unsupported"

// VersionError is an upgrade refusal, not a retry or a v4 policy inference.
type VersionError struct{ Environment, Required string }

func (e *VersionError) Error() string {
	return fmt.Sprintf("%s: environment=%s required=%s; upgrade Curator", CodeVersionUnsupported, e.Environment, e.Required)
}

// CheckLaunchRevision leaves the v1-v3 reader contract unchanged while closing
// the managed Muse launch boundary to fragments without v4 policy.
func CheckLaunchRevision(f *Fragment) error {
	if f.Environment == EnvMuse && (f.Revision == Identity || f.Revision == IdentityV2 || f.Revision == IdentityV3) {
		return &VersionError{Environment: EnvMuse, Required: IdentityV4}
	}
	return nil
}

// legacyMuseUpgrade recognises old Muse tokens only after strict JSON parsing.
// v1/v2 never admitted Muse in their reader schemas, but a managed launch must
// still report the required upgrade rather than retrying or inventing policy.
func legacyMuseUpgrade(raw []byte, requested string) error {
	root, err := ParseJSON(raw)
	if err != nil {
		return nil
	}
	env, _ := root.Get("environment")
	rev, _ := root.Get("fragment")
	if env.Kind != KindString || env.Str != requested || rev.Kind != KindString {
		return nil
	}
	return CheckLaunchRevision(&Fragment{Environment: env.Str, Revision: rev.Str})
}
