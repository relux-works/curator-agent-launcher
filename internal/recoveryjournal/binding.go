// Package recoveryjournal owns caller-local recovery state. It resolves an
// explicit original binding, opens an existing private namespace without repair,
// and prepares a private envelope durably before initial control/2 issuance.
// The exact recovery command uses guarded discovery and authenticated cleanup;
// native launch issuance is integrated separately.
package recoveryjournal

import (
	"errors"
	"path/filepath"
	"strings"
)

var (
	ErrBinding         = errors.New("recovery journal: invalid original binding")
	ErrBindingChanged  = errors.New("recovery journal: original binding changed")
	ErrUnsafeLayout    = errors.New("recovery journal: unsafe directory layout")
	ErrLayoutIO        = errors.New("recovery journal: directory operation failed")
	ErrUnsupported     = errors.New("recovery journal: platform unsupported")
	ErrPrepare         = errors.New("recovery journal: private prepare failed or uncertain")
	ErrLock            = errors.New("recovery journal: bounded lock unavailable")
	ErrCapacity        = errors.New("recovery journal: private state capacity exhausted")
	ErrExisting        = errors.New("recovery journal: original attempt already present")
	ErrRead            = errors.New("recovery journal: exact private entry unavailable or invalid")
	ErrIssueUnknown    = errors.New("recovery journal: issuance unconfirmed; original recovery entry retained")
	ErrRecoveryUnknown = errors.New("recovery journal: service outcome unconfirmed; original recovery entry retained")
	ErrLocalEvidence   = errors.New("recovery journal: local_evidence_failed; original recovery entry retained")
)

// Inputs must come from the original admitted caller, before child overlays.
// Resolve cannot authenticate provenance from strings; integration owns that
// boundary. It deliberately reads no process environment or configuration.
type Inputs struct {
	XDGStateHome  string
	Home          string
	ProfileDigest string
	ControlSocket string
}

// Binding freezes normalized state routing, the admitted profile digest and
// the canonical Unix control socket path. It contains no recovery capability.
// Registry identity is an additional binding in the frozen issuance envelope.
type Binding struct {
	base, profile, socket string
}

func Resolve(in Inputs) (Binding, error) {
	loc, err := resolveLocation(in.XDGStateHome, in.Home)
	if err != nil || !digest(in.ProfileDigest) || !canonicalSocket(in.ControlSocket) {
		return Binding{}, ErrBinding
	}
	return Binding{loc.base, in.ProfileDigest, in.ControlSocket}, nil
}

func absolute(path string) bool {
	return filepath.IsAbs(path) && !strings.ContainsRune(path, 0)
}

func canonicalSocket(path string) bool {
	return absolute(path) && filepath.Clean(path) == path && path != string(filepath.Separator)
}

func digest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (b Binding) valid() bool {
	return absolute(b.base) && filepath.Clean(b.base) == b.base && digest(b.profile) && canonicalSocket(b.socket)
}

func (b Binding) StateBase() string { return b.base }
func (b Binding) Directory() string {
	return (location{b.base}).directory()
}

// Match never adopts new routing facts or searches alternative homes.
func (b Binding) Match(current Binding) error {
	if !b.valid() || !current.valid() {
		return ErrBinding
	}
	if b != current {
		return ErrBindingChanged
	}
	return nil
}

// MissingDirectories is a proposed directory write closure for preflight, not
// permission to create it. Targets are deliberately absent from Error output.
// Future lock, entry, receipt and temporary-file writes need their own closure.
type MissingDirectories struct{ targets []string }

func (*MissingDirectories) Error() string {
	return "recovery journal: directories require admitted creation"
}
func (m *MissingDirectories) Targets() []string { return append([]string(nil), m.targets...) }

// PreflightLabels projects only the fixed namespace containers and ancestors
// of the captured state base. STATE names that base without disclosing its
// absolute path. These labels propose creation, never authorize or perform it.
func (m *MissingDirectories) PreflightLabels(b Binding) ([]string, error) {
	if m == nil || !b.valid() || len(m.targets) == 0 {
		return nil, ErrBinding
	}
	allowed := map[string]string{}
	path, label := b.base, "STATE"
	for {
		allowed[path] = label
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path, label = parent, label+"/.."
	}
	path, label = b.base, "STATE"
	for _, part := range []string{"swarma", "launcher", "netproxy-recovery", "v1"} {
		path, label = filepath.Join(path, part), label+"/"+part
		allowed[path] = label
	}
	allowed[filepath.Join(path, "secrets")] = label + "/secrets"
	allowed[filepath.Join(path, "receipts")] = label + "/receipts"
	labels := make([]string, 0, len(m.targets))
	seen := map[string]bool{}
	for _, target := range m.targets {
		label, ok := allowed[target]
		if !ok || seen[target] {
			return nil, ErrBinding
		}
		seen[target] = true
		labels = append(labels, label)
	}
	return labels, nil
}
