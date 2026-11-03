package plan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/providerlimits"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

// The gateway path obtains authority from the real vendor/store boundary and
// retains an actual refused verdict. Callback substitutes must never run.
func TestGatewayRealPolicyAndLimitsDiagnostic(t *testing.T) {
	for _, kind := range []string{"healthy", "limited", "missing-store"} {
		t.Run(kind, func(t *testing.T) {
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "codex"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			store, err := providerlimits.NewStore(providerlimits.Options{Layout: providerlimits.LayoutAt(t.TempDir())})
			if err != nil {
				t.Fatal(err)
			}
			req := Request{Gateway: true, Runtime: "codex", Model: "sol", Effort: "high", ToolRelease: "0.164.0", Home: t.TempDir(), WorkDir: t.TempDir(), Env: []string{"PATH=" + bin, "HOME=" + t.TempDir()}}
			if kind == "limited" {
				id, err := providerlimits.IdentityFor("codex", req.Home)
				if err != nil {
					t.Fatal(err)
				}
				obs, ok := providerlimits.ClassifyManaged(providerlimits.ProviderCodex, providerlimits.ManagedSignalUsageLimited).QuotaObservation(providerlimits.Evidence{Model: req.Model})
				if !ok {
					t.Fatal("missing observation")
				}
				if err := store.Observe(id, providerlimits.MustGroupTable().Group("codex", req.Model), nil, obs); err != nil {
					t.Fatal(err)
				}
			}
			expected, err := store.AvailabilityFor(providerlimits.VerdictQuery{Runtime: req.Runtime, Model: req.Model, Home: req.Home})
			if err != nil {
				t.Fatal(err)
			}
			d := DefaultDeps(store)
			d.GatewayStore = store
			d.BuildLaunch = func(context.Context, *vendorplugin.Registry, vendorplugin.SpawnRequest, agentic.LaunchMode) (agentic.PlanWithEnvironment, error) {
				t.Fatal("callback minted gateway authority")
				return agentic.PlanWithEnvironment{}, nil
			}
			d.Availability = func(providerlimits.VerdictQuery) (vendorplugin.Availability, error) {
				t.Fatal("callback supplied gateway verdict")
				return vendorplugin.Availability{}, nil
			}
			if kind == "missing-store" {
				d.GatewayStore = nil
			}
			out, err := Build(context.Background(), d, req)
			if kind == "healthy" {
				if err != nil || out.GatewayBasis == nil || out.Plan.Binary == "" {
					t.Fatal("real policy failed", err)
				}
				return
			}
			if out.Plan.Binary != "" || out.GatewayBasis != nil {
				t.Fatal("refusal issued executable authority")
			}
			if kind == "limited" {
				var limited *LimitedError
				if !errors.As(err, &limited) || !reflect.DeepEqual(limited.Verdict, expected) {
					t.Fatal("refused limits diagnostic lost", err)
				}
			} else {
				var refused *RefusedError
				if !errors.As(err, &refused) || !errors.Is(err, agentic.ErrCodexGatewayRefused) {
					t.Fatal("missing-store refusal lost", err)
				}
			}
		})
	}
}
