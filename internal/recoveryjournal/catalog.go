package recoveryjournal

import (
	"github.com/relux-works/curator-network-profiles/pkg/catalog"
	"github.com/relux-works/curator-network-profiles/pkg/netprofile"
	"github.com/relux-works/curator-network-profiles/pkg/resolve"
)

// CheckCatalog checks an exact recorded binding against the current original
// caller catalog and confirmation ledger. It never selects today's default or
// changes the binding. Equivalent confirmed aliases are valid: the owning
// network digest intentionally excludes profile names. No private journal,
// home alternative, service endpoint or registry is discovered here.
//
// The caller must first read the named guarded record from its admitted state
// location, and must still authenticate its unchanged E against the original
// service. A catalog match grants neither issuance nor cleanup authority.
func CheckCatalog(parentEnv []string, original Binding) error {
	if !original.valid() {
		return ErrBinding
	}
	canonical := "sha256:" + original.profile
	currentBinding, err := ResolveCaller(parentEnv, canonical, original.socket)
	if err != nil || original.Match(currentBinding) != nil {
		return ErrBindingChanged
	}
	current, err := catalog.Load(parentEnv)
	if err != nil || current == nil || current.File == nil {
		return ErrBindingChanged
	}
	for _, name := range current.File.Names() {
		p, ok := current.File.Lookup(name)
		if !ok || p.Kind != netprofile.KindFilteringGateway || p.Control != original.socket || netprofile.Digest(p) != canonical {
			continue
		}
		// Use the owning admission route, including confirmation; a catalog
		// declaration or shaped digest alone is insufficient.
		if _, err := current.Resolve(resolve.Request{Explicit: name}); err != nil {
			continue
		}
		return nil
	}
	return ErrBindingChanged
}
