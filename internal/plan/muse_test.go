package plan

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/providerlimits"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

// Older and opaque module versions refuse with their observed version before
// any build or availability read; a tagged v4 consumer reaches both boundaries.
func TestMuseModuleVersionGate(t *testing.T) {
	for _, version := range []string{"v0.5.62", "v0.5.65", "v0.5.66", "v0.5.68", "unknown"} {
		t.Run(version, func(t *testing.T) {
			builds, reads := 0, 0
			deps := Deps{ModuleVersion: func() string { return version }, BuildLaunch: func(context.Context, *vendorplugin.Registry, vendorplugin.SpawnRequest, agentic.LaunchMode) (agentic.PlanWithEnvironment, error) {
				builds++
				return agentic.PlanWithEnvironment{}, nil
			}, Availability: func(providerlimits.VerdictQuery) (vendorplugin.Availability, error) {
				reads++
				return vendorplugin.Availability{State: vendorplugin.AvailabilityHealthy}, nil
			}}
			_, err := Build(context.Background(), deps, Request{Home: "/managed", WorkDir: "/work", Context: &agentic.CuratorContext{Environment: "muse"}})
			admitted := version == "v0.5.66" || version == "v0.5.68"
			if admitted {
				if err != nil || builds != 1 || reads != 1 {
					t.Fatalf("valid consumer: %v %d %d", err, builds, reads)
				}
			} else {
				var typed *RefusedError
				if !errors.As(err, &typed) || !strings.Contains(err.Error(), "version="+version) || builds != 0 || reads != 0 {
					t.Fatalf("old consumer: %v %d %d", err, builds, reads)
				}
			}
		})
	}
}

// The public form stays interactive. With a verified root, a command tail
// must not switch it to exec/serve; separator data and option values stay data.
func TestMuseInteractiveFormGate(t *testing.T) {
	for _, tc := range []struct {
		tail []string
		form string
	}{
		{[]string{"exec", "hello"}, "exec"}, {[]string{"serve"}, "serve"},
		{[]string{"--model", "model", "exec"}, "exec"}, {[]string{"--model=model", "serve"}, "serve"},
		{[]string{"hello"}, ""}, {[]string{"--", "exec"}, ""}, {[]string{"--workspace", "serve"}, ""},
	} {
		req := Request{Context: &agentic.CuratorContext{RootContext: &agentic.CuratorRootContext{Exclusive: true}}, NativeArgs: tc.tail}
		err := checkMuseInteractiveForm(req)
		if tc.form == "" {
			if err != nil {
				t.Fatal(err)
			}
		} else {
			var typed *FormError
			if !errors.As(err, &typed) || typed.Form != tc.form {
				t.Fatalf("tail %q: %v", tc.tail, err)
			}
		}
		req.Context.RootContext = nil
		if err := checkMuseInteractiveForm(req); err != nil {
			t.Fatalf("absent fact changed legacy tail: %v", err)
		}
	}
}
