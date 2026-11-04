package network_test

import (
	"context"
	"errors"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/network"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
)

// Opt-in never upgrades an absent, external-proxy or hosted selection into a
// filtering gateway. Existing legacy and explicit-native lifecycle tests are
// the nearby valid controls; every rejected form must stop before probing.
func TestPrepareGatewayRecoverySelectionRefusals(t *testing.T) {
	for _, variant := range []string{"unselected", "external", "hosted"} {
		t.Run(variant, func(t *testing.T) {
			home := t.TempDir()
			writeCatalog(t, home, true)
			req := baseRequest(home)
			req.GatewayRecovery = true
			req.Prober = forbiddenProber{t}
			if variant == "unselected" {
				req.ExplicitSet = false
				req.Explicit = ""
			}
			if variant == "hosted" {
				req.Hosted = true
			}
			got, err := network.Prepare(context.Background(), req)
			var rejected *refusal.Refusal
			if !errors.As(err, &rejected) || rejected.Code != refusal.CodeScopeUnsupported || got.Lease != nil || got.Record != nil || len(got.Patch.Set) != 0 {
				t.Fatal("RECOVERY SCOPE ALARM: unsupported selection admitted")
			}
		})
	}
}
