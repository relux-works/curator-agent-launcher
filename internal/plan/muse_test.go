package plan

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/relux-works/curator-runtimes/pkg/agentic"
	"github.com/relux-works/curator-runtimes/pkg/providerlimits"
	"github.com/relux-works/curator-runtimes/pkg/vendorplugin"
)

// Versions below the stable v0.5.66 threshold refuse before any build or
// availability read; releases and valid Go pseudo-versions at or above it pass.
func TestMuseModuleVersionGate(t *testing.T) {
	for _, tc := range []struct {
		version  string
		admitted bool
	}{
		{version: "v0.5.62"},
		{version: "v0.5.65"},
		{version: "v0.5.66", admitted: true},
		{version: "v0.5.68", admitted: true},
		{version: "v0.5.66+incompatible", admitted: true},
		{version: "v0.5.65-0.20261124231244-8efb5e8226cf"},
		{version: "v0.5.66-0.20261124231244-8efb5e8226cf"},
		{version: "v0.5.81-0.20261124231244-1fcf854d828c", admitted: true},
		{version: "v0.6.0-0.20261124231244-1fcf854d828c", admitted: true},
		{version: "v0.5.81-01.20261124231244-1fcf854d828c"},
		{version: "0.5.80"},
		{version: "unknown"},
	} {
		version := tc.version
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
			if tc.admitted {
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
