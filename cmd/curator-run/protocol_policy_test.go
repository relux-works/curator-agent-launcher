package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
)

// Concrete selectors are comparisons, never unlocking flags. Null leaves the
// owning defaults/lineup resolution in force. These fixtures resolve policy
// only; they run no version probe, provider plan, native process or store.
func TestProtocolPolicyExpectedValues(t *testing.T) {
	for _, kind := range []string{"locked-match", "null-defaults", "model-mismatch", "effort-mismatch", "empty-model"} {
		t.Run(kind, func(t *testing.T) {
			args, baseline, f, workspace := protocolMetadataFixture(t)
			switch kind {
			case "null-defaults":
				args[7], args[8] = "--model-json=null", "--effort-json=null"
			case "model-mismatch":
				args[7] = `--model-json="gpt-5.6-sol"`
			case "effort-mismatch":
				args[8] = `--effort-json="high"`
			case "empty-model":
				args[7] = `--model-json=""`
			}
			deps := testDeps(t, protocolMetadataResolver{resolve: func(context.Context, fragment.Request) (*fragment.Fragment, error) { return f, nil }})
			deps.environ = func() []string { return baseline }
			deps.workdir = func() (string, error) { return workspace, nil }
			body := `{"schema":"curator-run-defaults-v3","locked":true,"defaults":{"codex_cli":{"model":"gpt-6-astra","effort":"low","permissions":"native"}}}`
			if err := os.WriteFile(deps.defaults.Machine, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			meta, err := resolveProtocolMetadata(context.Background(), args, deps)
			if err != nil {
				t.Fatal(err)
			}
			got, err := resolveProtocolPolicy(context.Background(), meta, deps)
			if strings.HasSuffix(kind, "mismatch") || kind == "empty-model" {
				if got != nil || err == nil || !strings.HasPrefix(err.Error(), "protocol_child_policy_mismatch:") {
					t.Fatalf("expectation changed owning policy: got=%+v err=%v", got, err)
				}
				return
			}
			if err != nil || got == nil || got.ordinary.resolved.Model.Value != "gpt-6-astra" || got.ordinary.resolved.Effort.Value != "low" || got.inv.ModelSet || got.inv.EffortSet || got.inv.PermissionSet {
				t.Fatalf("valid expectation overrode/refused locked policy: got=%+v err=%v", got, err)
			}
		})
	}
}

// An unsupported selected network and failed selected Core projection refuse
// before defaults/selector/provider work. No config-off fallback is available;
// a nearby no-network selected projection reaches normal policy resolution.
func TestProtocolPolicySelectedRefusals(t *testing.T) {
	for _, kind := range []string{"selected-proxy", "selected-failure", "valid"} {
		t.Run(kind, func(t *testing.T) {
			args, baseline, f, workspace := protocolMetadataFixture(t)
			deps := testDeps(t, protocolMetadataResolver{resolve: func(context.Context, fragment.Request) (*fragment.Fragment, error) { return f, nil }})
			deps.environ = func() []string { return baseline }
			deps.workdir = func() (string, error) { return workspace, nil }
			deps.readSelectedConfig = func(context.Context) (launcherconfig.Selected, error) {
				if kind == "selected-failure" {
					return launcherconfig.Selected{}, launcherconfig.ErrSelected
				}
				selected := launcherconfig.Selected{ConfigSchema: 7, ConfigPath: "fixture-selected.json"}
				if kind == "selected-proxy" {
					selected.Proxy = &launcherconfig.Proxy{Profile: "synthetic-network"}
				}
				return selected, nil
			}
			// Missing/broken defaults must not replace the earlier selected refusal.
			body := `{"schema":"invalid"}`
			if kind == "valid" {
				body = `{"schema":"curator-run-defaults-v3","defaults":{"codex_cli":{"model":"gpt-6-astra","effort":"low"}}}`
			}
			if err := os.WriteFile(deps.defaults.Machine, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			meta, err := resolveProtocolMetadata(context.Background(), args, deps)
			if err != nil {
				t.Fatal(err)
			}
			got, err := resolveProtocolPolicy(context.Background(), meta, deps)
			if kind == "valid" {
				if err != nil || got == nil {
					t.Fatalf("valid selected policy refused: %v", err)
				}
				return
			}
			code := "protocol_child_network_unsupported:"
			if kind == "selected-failure" {
				code = "protocol_child_policy_refused:"
			}
			if got != nil || err == nil || !strings.HasPrefix(err.Error(), code) || strings.Contains(err.Error(), "synthetic-network") {
				t.Fatalf("selected policy refusal changed: got=%+v err=%v", got, err)
			}
		})
	}
}
