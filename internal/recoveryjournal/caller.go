package recoveryjournal

import "strings"

// ResolveCaller projects the original parent environment and the already
// admitted network profile into a journal binding. selectedDigest is the
// network library's canonical sha256:<64 lowercase hex>, not a Curator role,
// fragment or lock digest. The internal journal representation is bare hex.
//
// The caller owns admission and provenance: this function neither loads a
// catalog nor authenticates arbitrary strings. Pass the captured ParentEnv,
// before child overlays, and the same resolved profile used by the gateway.
// It reads no ambient environment, child configuration or filesystem.
func ResolveCaller(parentEnv []string, selectedDigest, controlSocket string) (Binding, error) {
	profile, ok := strings.CutPrefix(selectedDigest, "sha256:")
	if !ok || !digest(profile) {
		return Binding{}, ErrBinding
	}
	in, err := callerState(parentEnv)
	if err != nil {
		return Binding{}, err
	}
	in.ProfileDigest, in.ControlSocket = profile, controlSocket
	return Resolve(in)
}

func callerState(parentEnv []string) (Inputs, error) {
	var in Inputs
	seen := make(map[string]bool, 2)
	for _, pair := range parentEnv {
		name, value, hasValue := strings.Cut(pair, "=")
		if name != "HOME" && name != "XDG_STATE_HOME" {
			continue
		}
		// Ambiguous routing never depends on first/last environment lookup
		// semantics. An absent/empty XDG value uses the documented fallback.
		if !hasValue || seen[name] {
			return Inputs{}, ErrBinding
		}
		seen[name] = true
		if name == "HOME" {
			in.Home = value
		} else {
			in.XDGStateHome = value
		}
	}
	return in, nil
}
