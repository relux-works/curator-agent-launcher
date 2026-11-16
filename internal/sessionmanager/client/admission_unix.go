//go:build unix

package client

import (
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
)

// frozenAdmission derives source (b) admission for a pending operation: the
// capabilities frozen in its lease admission at submission count only when
// entry, which the caller looked up under the frozen manager id, is still
// that frozen owner incarnation (the same executable and registration time
// as the binding written before submission). Any mismatch, a missing lease
// record, or capabilities without the token keep the no-token rule. The
// token itself is read with the one rule of ProcessObservation, so it counts
// only together with status/1. Old durable files hold no token and therefore
// decode as no-token.
func frozenAdmission(p Pending, entry sessionmanager.Entry) Admission {
	if p.Lease == nil || entry.Executable != p.Manager.Executable || entry.RegisteredAt != p.Manager.RegisteredAt {
		return Admission{}
	}
	return Admission{ProcessObservation: ProcessObservation(Describe{Capabilities: p.Lease.Capabilities})}
}
