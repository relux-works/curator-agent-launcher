package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
	"github.com/relux-works/curator-agent-launcher/internal/protocolchild"
)

// A fresh nonrepair profile/config/defaults read must agree with the retained
// inputs. This is a final-boundary fixture, not an actual exec/cancellation or
// arbitrary-descendant cleanup witness. The valid control cannot rebuild a plan.
func TestProtocolBoundaryReread(t *testing.T) {
	for _, kind := range []string{"unchanged", "baseline-drift", "profile-stale", "defaults-drift", "launcher-policy-drift", "selected-drift", "selected-failure", "cancel-before", "cancel-during-read"} {
		t.Run(kind, func(t *testing.T) {
			f, args := protocolPlanFixture(t)
			policy := protocolPolicyFixture(t, f, args)
			prepared, err := composeProtocolPlan(context.Background(), policy)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			deps := f.deps
			calls := 0
			original := deps.resolver
			deps.resolver = protocolMetadataResolver{resolve: func(ctx context.Context, req fragment.Request) (*fragment.Fragment, error) {
				calls++
				if kind == "profile-stale" {
					return nil, &fragment.ResolveError{Code: fragment.CodeHomeStale, Detail: "private-profile-fixture"}
				}
				if kind == "cancel-during-read" {
					cancel()
				}
				return original.Resolve(ctx, req)
			}}
			switch kind {
			case "baseline-drift":
				deps.environ = func() []string { return append(policy.metadata.baseline, "LANG=C") }
			case "defaults-drift":
				f.writeDefaults(t, f.deps.defaults.Machine, `{"schema":"curator-run-defaults-v3","defaults":{"codex_cli":{"model":"gpt-6-astra","effort":"high"}}}`)
			case "launcher-policy-drift":
				f.writeDefaults(t, policy.ordinary.deps.configResolver.Locations.System+"/launcher.json", `{"schema":"curator-run-launcher-v1","host_mode":"hosted"}`)
			case "selected-drift", "selected-failure":
				deps.readSelectedConfig = func(context.Context) (launcherconfig.Selected, error) {
					if kind == "selected-failure" {
						return launcherconfig.Selected{}, errors.New("private-selected-fixture")
					}
					return launcherconfig.Selected{ConfigSchema: 7, ConfigPath: "different-config.json"}, nil
				}
			case "cancel-before":
				cancel()
			}
			// Fresh policy/metadata are allowed; a second admission/version plan
			// is not part of this boundary and cannot quietly replace the first.
			deps.build = nil
			deps.availability = nil
			beforeArgs := append([]string{}, prepared.value.Argv...)
			beforeEnv := append([]string{}, prepared.value.Env...)
			err = recheckProtocolPolicy(ctx, prepared, deps)
			if kind == "unchanged" {
				if err != nil || calls != 1 {
					t.Fatalf("unchanged boundary refused: %v calls=%d", err, calls)
				}
			} else {
				code := "protocol_child_stale"
				if strings.HasPrefix(kind, "cancel-") {
					code = "protocol_child_cancelled"
				}
				var refusal *protocolchild.Error
				if !errors.As(err, &refusal) || refusal.Code != code || strings.Contains(err.Error(), "private-") {
					t.Fatalf("wrong drift/cancel refusal: %v want=%s", err, code)
				}
				if (kind == "baseline-drift" || kind == "cancel-before") && calls != 0 {
					t.Fatal("changed/cancelled entry reached fresh Core metadata")
				}
			}
			if !reflect.DeepEqual(beforeArgs, prepared.value.Argv) || !reflect.DeepEqual(beforeEnv, prepared.value.Env) || f.builds != 1 || f.verdicts != 1 {
				t.Fatal("boundary mutated/replaced the prepared process")
			}
			f.assertNoChild(t)
		})
	}
}
