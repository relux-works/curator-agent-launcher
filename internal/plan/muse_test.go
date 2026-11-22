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
// any build or availability read; a tagged v4 consumer reaches both
// boundaries. So does a commit after such a release, in Go's pseudo-version
// form and judged by the release it follows, which is what a module pinned
// between two tags reports. Every other spelling with a hyphen or a plus
// refuses: a pre-release, build metadata, the two other pseudo-version forms,
// a commit after a release older than v0.5.66, and a malformed time or hash.
// A plain release is three numbers of decimal digits in canonical form: a
// sign, a leading zero, a missing or extra part and surrounding space refuse.
// The linked version is injected; no build metadata is read here.
func TestMuseModuleVersionGate(t *testing.T) {
	admittedVersions := map[string]bool{
		"v0.5.66": true, "v0.5.68": true, "v0.6.0": true, "v1.0.0": true,
		"0.5.66": true, "1.0.0": true, "v10.0.0": true, // a plain release may omit the v
		"v0.5.79-0.20261122055244-b43a71d2e645": true, // the commit after v0.5.78
		"v0.5.67-0.20260101000000-0123456789ab": true, // the commit after v0.5.66
		"v0.6.1-0.20260101000000-0123456789ab":  true,
		"v1.0.1-0.20260101000000-0123456789ab":  true,
	}
	for _, version := range []string{
		"v0.5.62", "v0.5.65", "v0.5.66", "v0.5.68", "v0.6.0", "v1.0.0", "unknown", "",
		"v0.5.79-0.20261122055244-b43a71d2e645", "v0.5.67-0.20260101000000-0123456789ab",
		"v0.6.1-0.20260101000000-0123456789ab", "v1.0.1-0.20260101000000-0123456789ab",
		"v0.5.66-0.20260101000000-0123456789ab",                     // follows v0.5.65
		"v0.5.1-0.20260101000000-0123456789ab",                      // follows v0.5.0
		"v0.5.0-0.20260101000000-0123456789ab",                      // patch 0 follows no release
		"v0.5.79-rc1", "v0.5.79-rc.1", "v0.5.79+build", "v0.5.79-0", // pre-release, metadata
		"v0.5.79-0.20261122055244-b43a71d2e645+incompatible", // metadata after a pseudo-version
		"v0.5.79-pre.0.20261122055244-b43a71d2e645",          // commit after a pre-release tag
		"v0.5.79-rc.1.0.20261122055244-b43a71d2e645",         // commit after a release candidate
		"v0.0.0-20261122055244-b43a71d2e645",                 // commit with no release before it
		"v0.5.79-1.20261122055244-b43a71d2e645",
		"v0.5.79-0.2026112205524-b43a71d2e645",   // 13 digits
		"v0.5.79-0.202611220552440-b43a71d2e645", // 15 digits
		"v0.5.79-0.20261322055244-b43a71d2e645",  // month 13
		"v0.5.79-0.20261122055244-B43A71D2E645",  // upper-case hash
		"v0.5.79-0.20261122055244-b43a71d2e64",   // 11 hex
		"v0.5.79-0.20261122055244-b43a71d2e6455", // 13 hex
		"v0.5.79-0.20261122055244-b43a71d2e64g",
		"0.5.79-0.20261122055244-b43a71d2e645", // no leading v, as only a plain release may omit it
		" v0.5.79-0.20261122055244-b43a71d2e645", "v0.5.79-0.20261122055244-b43a71d2e645 ",
		"v00.5.79-0.20261122055244-b43a71d2e645", "v0.5.079-0.20261122055244-b43a71d2e645",
		"0.5.66", "1.0.0", "v10.0.0", "0.5.65",
		"v+1.0.0", "v0.+6.0", "v1.-1.0", "+1.0.0", "v-1.0.0", "v1.0.+0", "v1.0.-0", // a sign is no digit
		"v01.0.0", "v1.00.0", "v0.5.066", "v00.5.66", // a leading zero
		"v1.0", "v1", "v1.0.0.0", "v.5.66", "v0..66", "vv1.0.0", "V1.0.0", "v1.0.0 ", " v1.0.0", "v1.0.0\n",
		"v0x1.0.0", "v1_0.0.0", "v1e1.0.0", "v\uff11.0.0", // not decimal digits
	} {
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
			admitted := admittedVersions[version]
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
