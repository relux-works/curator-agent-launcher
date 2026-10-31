package network_test

import (
	"context"
	"encoding/json"
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/network"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
)

// Known mode/authority controls refuse in every supplied layer before gateway
// I/O. Nearby ordinary tool, credential, request and MCP values still register.
func TestGatewayKnownEnvironmentContract(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		admit       bool
	}{
		{"IS_DEMO", "0", false}, {"cLaUbBiT", "1", false},
		{"CLAUDECODE", "1", false}, {"USE_STAGING_OAUTH", "1", false},
		{"_CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL", "1", false},
		{"FORCE_AUTOUPDATE_PLUGINS", "1", false},
		{"SYSTEM_REMINDER_MEMORY_CONTEXT", "1", false},
		{"SDK_NATIVE_BIN", "/alternate/claude", false},
		{"VITALS_EMITTER_BIN", "/alternate/emitter", false},
		{"ANT_OTEL_EXPORTER_OTLP_ENDPOINT", "https://alternate.invalid", false},
		{"BUGHUNTER_DEV_BUNDLE_B64", "fixture", false},
		{"NODE_TLS_REJECT_UNAUTHORIZED", "0", false},
		{"ENABLE_SESSION_BACKGROUNDING", "1", false},
		{"ULTRAPLAN_PROMPT_FILE", "/alternate/prompt", false},
		{"DISABLE_AUTOUPDATER", "1", true}, {"DISABLE_GROWTHBOOK", "1", true},
		{"DISABLE_BRIEF_MODE_STOP_HOOK", "1", true},
		{"MCP_SDK_GENERATION", "v1", true}, {"MCP_SDK_GENERATION", "v2", true},
		{"MCP_SDK_GENERATION", "unsupported-fixture", true},
		{"MCP_PROTOCOL_NEGOTIATION", "legacy", true}, {"MCP_PROTOCOL_NEGOTIATION", "auto", true},
		{"MCP_PROTOCOL_NEGOTIATION", "unsupported-fixture", true},
		{"MCP_TRUNCATION_PROMPT_OVERRIDE", "legacy", true},
		{"MCP_TRUNCATION_PROMPT_OVERRIDE", "custom-fixture", true},
		{"MCP_CLIENT_SECRET", "credential-fixture", true},
		{"AWS_CONTAINER_CREDENTIALS_FULL_URI", "http://credential.invalid", true},
		{"BROWSER", "/local/browser", true}, {"NODE_OPTIONS", "--use-system-ca", true},
		{"FALLBACK_FOR_ALL_PRIMARY_MODELS", "1", true},
		{"UNRELATED_EXTENSION_SETTING", "exact-value", true},
	} {
		for _, layer := range []string{"fragment", "prompt", "settings", "mcp"} {
			t.Run(tc.name+"/"+layer, func(t *testing.T) {
				g := newGateway(t)
				req := gatewayRequest(gatewayHome(t, g, true))
				switch layer {
				case "fragment":
					req.FragEnv = map[string]string{tc.name: tc.value}
				case "prompt":
					req.PromptEnv = map[string]string{tc.name: tc.value}
				case "mcp":
					req.MCPEnvNames = []string{tc.name}
				case "settings":
					b, err := json.Marshal(map[string]any{"env": map[string]string{tc.name: tc.value}})
					if err != nil {
						t.Fatal(err)
					}
					rewriteReadOnly(t, filepath.Join(managedHome(req), "settings.json"), string(b))
				}
				got, err := network.Prepare(context.Background(), req)
				if tc.admit {
					if err != nil || got.Lease == nil {
						t.Fatalf("ordinary control refused: %v", err)
					}
					got.Lease.Finish(context.Background())
					return
				}
				if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || len(g.calls()) != 0 {
					t.Fatalf("err=%v gateway calls=%v", err, g.calls())
				}
				if strings.Contains(err.Error(), tc.value) && tc.value != "1" && tc.value != "0" {
					t.Fatal("refusal disclosed an environment value")
				}
			})
		}
	}
}

// The final check accepts only the three exact registration-owned pairs,
// rejects surviving controls in all representations, and cannot be widened by
// mutating the public patch after registration. Generic/unmanaged is unaffected.
func TestGatewayComposedEnvironmentBoundary(t *testing.T) {
	g := newGateway(t)
	req := gatewayRequest(gatewayHome(t, g, true))
	got, err := network.Prepare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Lease.Finish(context.Background())
	owned := map[string]string{
		"ANTHROPIC_BASE_URL":                       "http://127.0.0.1:47811/anthropic",
		"ANTHROPIC_CUSTOM_HEADERS":                 "X-Netproxy-Token: " + gwToken,
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
	}
	valid := func() ([]string, map[string]string, []string) {
		env := []string{"MCP_SDK_GENERATION=v2", "UNRELATED_EXTENSION_SETTING=kept"}
		for name, value := range owned {
			env = append(env, name+"="+value)
		}
		return env, maps.Clone(owned), []string{"MCP_CLIENT_SECRET"}
	}
	if err := got.CheckComposed(valid()); err != nil {
		t.Fatalf("exact gateway patch refused: %v", err)
	}
	for _, which := range []string{"env", "literals", "lookup", "alias", "changed-token", "missing", "duplicate", "mutated-patch"} {
		t.Run(which, func(t *testing.T) {
			env, literals, names := valid()
			switch which {
			case "env":
				env = append(env, "IS_DEMO=0")
			case "literals":
				literals["CLAUBBIT"] = "1"
			case "lookup":
				names = append(names, "FORCE_AUTOUPDATE_PLUGINS")
			case "alias":
				env = append(env, "anthropic_base_url="+owned["ANTHROPIC_BASE_URL"])
			case "changed-token":
				literals["ANTHROPIC_CUSTOM_HEADERS"] = "private-canary"
			case "missing":
				delete(literals, "ANTHROPIC_BASE_URL")
			case "duplicate":
				env = append(env, "ANTHROPIC_BASE_URL="+owned["ANTHROPIC_BASE_URL"])
			case "mutated-patch":
				got.Patch.Set[0].Value = "private-canary"
				literals["ANTHROPIC_BASE_URL"] = "private-canary"
			}
			err := got.CheckComposed(env, literals, names)
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict {
				t.Fatalf("composition violation accepted: %v", err)
			}
			if strings.Contains(err.Error(), gwToken) || strings.Contains(err.Error(), "private-canary") {
				t.Fatal("composition refusal disclosed a value")
			}
		})
	}
	if err := (network.Prepared{}).CheckComposed([]string{"IS_DEMO=0"}, nil, nil); err != nil {
		t.Fatal("unmanaged behavior changed", err)
	}
}
