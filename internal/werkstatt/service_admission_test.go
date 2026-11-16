//go:build unix

package werkstatt

import (
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
)

// Claim: an admission negotiated in this run is carried only by the concrete
// session-manager transport. Admitting a copy leaves the original client at
// the no-token rule, and a Remote that answers without wire decoding is
// returned unchanged.
func TestAdmittedRemote(t *testing.T) {
	admission := client.Admission{ProcessObservation: true}
	base := client.Client{ID: "demo"}
	got, ok := admittedRemote(base, admission).(client.Client)
	if !ok || !got.Admission.ProcessObservation || base.Admission.ProcessObservation {
		t.Fatalf("transport admission: %v (ok=%t)", got, ok)
	}
	spy := &editSpy{}
	if admittedRemote(spy, admission) != Remote(spy) {
		t.Fatal("non-transport remote was replaced")
	}
}
