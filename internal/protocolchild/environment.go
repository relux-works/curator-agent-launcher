package protocolchild

import "strings"

// baselineNames is the complete r3 entry vocabulary. Native homes, runtime
// sessions, messaging selectors, credentials, proxies and loaders are absent.
// No prefix exception or unknown-name fallback is permitted.
var baselineNames = map[string]bool{
	"HOME": true, "PATH": true, "TMPDIR": true, "LANG": true,
	"LC_ALL": true, "LC_CTYPE": true, "TZ": true, "USER": true,
	"LOGNAME": true, "SHELL": true, "CURATOR_HOME": true,
}

// EntryEnvironment checks and snapshots the complete explicitly constructed
// incoming baseline. It never removes an entry and continues. This verifies
// shape/custody only: real operator-home and install/profile identity remain
// owning resolution checks, not facts an environment string can attest.
func EntryEnvironment(env []string) ([]string, error) {
	refuse := func() ([]string, error) {
		return nil, &Error{Code: CodeEnvironmentRefused, Detail: "incoming environment is not the explicit admitted baseline"}
	}
	seen := make(map[string]bool, len(env))
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || !baselineNames[name] || seen[name] || !safeText(value) {
			return refuse()
		}
		if (name == "HOME" || name == "PATH") && value == "" {
			return refuse()
		}
		seen[name] = true
	}
	if !seen["HOME"] || !seen["PATH"] {
		return refuse()
	}
	return append([]string{}, env...), nil
}

// MemberEnvironment returns only the explicit member pair for private native
// composition. It never reads ambient messaging state, creates an identity or
// exports operation admission. Host retains the actual member/operation guard.
func (s Selectors) MemberEnvironment() ([]string, error) {
	if !validMemberPair(s.MemberAgent, s.MemberConfigDir) {
		return nil, &Error{Code: CodeEnvironmentRefused, Detail: "explicit member identity no longer matches the closed selector shape"}
	}
	if s.MemberAgent == nil {
		return nil, nil
	}
	return []string{"A2A_AGENT=" + *s.MemberAgent, "A2A_CONFIG_DIR=" + *s.MemberConfigDir}, nil
}

// CheckMemberEnvironment rechecks the exact selector pair after provider-owned
// composition. It never filters, repairs or exports an environment. Additional
// messaging inputs require an explicitly qualified extension to this contract.
func (s Selectors) CheckMemberEnvironment(env []string) error {
	refuse := func() error {
		return &Error{Code: CodeEnvironmentRefused, Detail: "composed member identity differs from the explicit admitted pair"}
	}
	pair, err := s.MemberEnvironment()
	if err != nil {
		return refuse()
	}
	expected := make(map[string]string, len(pair))
	for _, entry := range pair {
		name, value, _ := strings.Cut(entry, "=")
		expected[name] = value
	}
	seen := make(map[string]bool, len(expected))
	for _, entry := range env {
		name, value, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(name, "A2A_") {
			continue
		}
		want, ok := expected[name]
		if !ok || seen[name] || value != want {
			return refuse()
		}
		seen[name] = true
	}
	if len(seen) != len(expected) {
		return refuse()
	}
	return nil
}
