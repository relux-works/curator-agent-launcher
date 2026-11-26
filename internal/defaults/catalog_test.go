package defaults

import (
	"context"
	"slices"
	"testing"

	"github.com/relux-works/curator-runtimes/pkg/agentic"
	agySystem "github.com/relux-works/curator-runtimes/pkg/agentic/systems/agy"
	"github.com/relux-works/curator-runtimes/pkg/runtimecatalog"
	"github.com/relux-works/curator-runtimes/pkg/vendorplugin"
)

func TestNewRegistryWithCatalogAdmitsOperatorAntigravityModelAndEffort(t *testing.T) {
	t.Parallel()
	const tomlText = `
schema_version = 1

[vendors.google.models."gemini-4-neon-custom"]
description = "Operator catalog model on Antigravity"
systems = ["antigravity"]
[vendors.google.models."gemini-4-neon-custom".effort]
support = "required"
accepts = ["low", "medium", "high", "xhigh", "max"]
recommended = "xhigh"
`
	cat, err := runtimecatalog.ParseModelCatalog([]byte(tomlText))
	if err != nil {
		t.Fatalf("ParseModelCatalog: %v", err)
	}
	reg, err := NewRegistryWithCatalog(agySystem.Runtime{
		Executable: "/opt/agy/bin/agy",
		Version:    "1.5.18",
	}, cat)
	if err != nil {
		t.Fatalf("NewRegistryWithCatalog: %v", err)
	}
	plan, err := vendorplugin.BuildLaunch(context.Background(), reg, vendorplugin.SpawnRequest{
		Runtime:        "agy",
		Model:          "gemini-4-neon-custom",
		Effort:         "xhigh",
		ToolRelease:    "1.5.18",
		PermissionMode: agentic.PermissionModeYolo,
	}, agentic.LaunchModeInteractive)
	if err != nil {
		t.Fatalf("BuildLaunch: %v", err)
	}
	want := []string{"--model", "gemini-4-neon-custom", "--effort", "xhigh", "--dangerously-skip-permissions"}
	if !slices.Equal(plan.Argv, want) {
		t.Fatalf("argv = %v, want %v", plan.Argv, want)
	}
}
