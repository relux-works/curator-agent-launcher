package network_test

import (
	"context"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/network"
)

// Core-selected profiles use the existing catalog/admission path and provenance;
// explicit CLI selection wins, while an invalid machine choice never falls back.
func TestPrepareLauncherSelectionPrecedence(t *testing.T) {
	for _, mode := range []string{"machine", "explicit", "invalid-machine", "hosted"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			writeDirectCatalog(t, home, true)
			req := baseRequest(home)
			req.Prober = forbiddenProber{t}
			req.Explicit, req.ExplicitSet = "", false
			req.LauncherProfile = selectedDirectProfile
			if mode == "explicit" {
				req.Explicit, req.ExplicitSet = selectedDirectProfile, true
				req.LauncherProfile = "invalid-must-not-win"
			}
			if mode == "invalid-machine" {
				req.LauncherProfile = "missing"
			}
			if mode == "hosted" {
				req.Hosted = true
			}
			got, err := network.Prepare(context.Background(), req)
			if mode == "invalid-machine" || mode == "hosted" {
				if err == nil || got.Record != nil || !got.Patch.Empty() || got.Lease != nil {
					t.Fatal("forbidden fallback or partial binding")
				}
				return
			}
			origin := "launcher-config"
			if mode == "explicit" {
				origin = "explicit"
			}
			if err != nil || got.Record == nil || got.Record.ProfileRef != selectedDirectProfile || got.Record.Origin != origin || got.Patch.Empty() {
				t.Fatalf("selection/provenance changed: %+v %v", got.Record, err)
			}
		})
	}
}
