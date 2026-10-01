package defaults_test

import (
	"errors"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	museSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/muse"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

func TestMuseDefaultsUseDeclarationOwnedModels(t *testing.T) {
	reg := mustRegistry(t)
	files, err := defaults.Load(paths(t))
	if err != nil {
		t.Fatal(err)
	}
	got, err := files.Complete("muse", defaults.Pair{}, reg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Runtime != "muse" {
		t.Fatalf("runtime=%q", got.Runtime)
	}
	lineupOrigin(t, got.Model, "muse-spark-1.3-contributor")
	lineupOrigin(t, got.Effort, "max")
	// This is launch-only authority, not evidence of a resolved vendor.
	if _, err := reg.ResolveRuntime("muse"); !errors.Is(err, vendorplugin.ErrRuntimeVendorUnresolved) {
		t.Fatalf("vendor bound changed: %v", err)
	}
	got, err = files.Complete("muse", defaults.Pair{Model: defaults.Member{Value: "muse-spark-1.2-contributor", Present: true}}, reg)
	if err != nil || got.Runtime != "muse" || got.Model.Value != "muse-spark-1.2-contributor" || got.Effort.Present {
		t.Fatalf("effort-none declaration row: %+v %v", got, err)
	}
}

func TestMuseDefaultsDoNotInventMissingAuthority(t *testing.T) {
	full := mustRegistry(t)
	declaration, ok := full.RuntimeDeclarationOf("muse")
	if !ok {
		t.Fatal("missing module Muse declaration")
	}
	for _, missing := range []string{"system", "models"} {
		t.Run(missing, func(t *testing.T) {
			systems := agentic.NewRegistry()
			decl := declaration
			if missing == "models" {
				if err := systems.Register(museSystem.New()); err != nil {
					t.Fatal(err)
				}
				decl.Models = nil
			}
			reg := vendorplugin.NewRegistry(systems)
			if err := reg.DeclareRuntime(decl); err != nil {
				t.Fatal(err)
			}
			files, err := defaults.Load(paths(t))
			if err != nil {
				t.Fatal(err)
			}
			got, err := files.Complete("muse", defaults.Pair{}, reg)
			code(t, err, defaults.CodeUnresolvable)
			if got != (defaults.Resolved{}) {
				t.Fatalf("missing %s authority leaked %+v", missing, got)
			}
		})
	}
}
