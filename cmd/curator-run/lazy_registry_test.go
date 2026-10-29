package main

import (
	"errors"
	"testing"

	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

// The lazy registry is built once per process and shared by every copy of
// launchDeps: a first refusal stays the result even when a second load would
// succeed, a first success is the same registry in every copy, and an
// injected registry is used without calling the loader.
func TestLazyRegistryKeepsTheFirstResult(t *testing.T) {
	refusal := errors.New("catalog refused")
	built := vendorplugin.NewRegistry(agentic.NewRegistry())

	calls := 0
	failing := launchDeps{lazyRegistry: newLazyRegistry(func() (*vendorplugin.Registry, error) {
		calls++
		if calls == 1 {
			return nil, refusal
		}
		return built, nil
	})}
	copied := failing
	for _, d := range []*launchDeps{&failing, &copied, &failing} {
		if err := d.ensureRegistry(); !errors.Is(err, refusal) || d.registry != nil {
			t.Fatalf("ensureRegistry after a failed load = %v, registry %v; want the first refusal", err, d.registry)
		}
	}
	if calls != 1 {
		t.Fatalf("loader ran %d times, want once", calls)
	}

	calls = 0
	succeeding := launchDeps{lazyRegistry: newLazyRegistry(func() (*vendorplugin.Registry, error) {
		calls++
		return built, nil
	})}
	copied = succeeding
	for _, d := range []*launchDeps{&succeeding, &copied} {
		if err := d.ensureRegistry(); err != nil || d.registry != built {
			t.Fatalf("ensureRegistry = %v, registry %p; want %p", err, d.registry, built)
		}
	}
	if calls != 1 {
		t.Fatalf("loader ran %d times, want once", calls)
	}

	calls = 0
	injected := launchDeps{registry: built, lazyRegistry: newLazyRegistry(func() (*vendorplugin.Registry, error) {
		calls++
		return nil, refusal
	})}
	if err := injected.ensureRegistry(); err != nil || injected.registry != built || calls != 0 {
		t.Fatalf("injected registry: err %v, loader calls %d", err, calls)
	}

	empty := launchDeps{lazyRegistry: newLazyRegistry(func() (*vendorplugin.Registry, error) { return nil, nil })}
	if err := empty.ensureRegistry(); err == nil {
		t.Fatal("a loader returning no registry and no error was accepted")
	}
	if err := (&launchDeps{}).ensureRegistry(); err == nil {
		t.Fatal("deps with no registry and no loader were accepted")
	}
}
