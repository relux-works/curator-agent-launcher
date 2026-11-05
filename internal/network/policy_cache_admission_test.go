package network_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/network"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
)

const safeCachedPolicy = `{"restrictions":{"allow_connector_file_downloads":{"allowed":true},"enforce_web_search_mcp_isolation":{"allowed":true}},"defaults":{"remote_control_at_startup":false},"compliance_taints":[],"monitoring_notice":null}`

// This exercises the real Prepare route, with no native/vendor execution.
// Safe populated caches reach the fixture gateway; unsafe caches refuse the
// intended code before health/registration and never echo values/key payloads.
func TestPrepareGatewayIsolationBooleanBoundary(t *testing.T) {
	// Both bools are gateway-safe only because every input that could make
	// native policy eligible is refused or removed before the fixed patch.
	for _, allowed := range []bool{false, true} {
		for _, layer := range []string{"none", "inherited", "fragment", "prompt", "settings", "mcp"} {
			for _, name := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_UNIX_SOCKET", "_CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL", "CLAUDE_CODE_CLIENT_DATA_URL"} {
				t.Run(layer+"/"+name+"/"+map[bool]string{false: "false", true: "true"}[allowed], func(t *testing.T) {
					g := newGateway(t)
					req := gatewayRequest(gatewayHome(t, g, true))
					policy, _ := json.Marshal(map[string]any{"restrictions": map[string]any{"enforce_web_search_mcp_isolation": map[string]bool{"allowed": allowed}}})
					writeTestFile(t, filepath.Join(managedHome(req), "policy-limits.json"), string(policy))
					value := map[string]string{
						"ANTHROPIC_BASE_URL":                       "https://api.anthropic.com",
						"ANTHROPIC_UNIX_SOCKET":                    "/tmp/policy-test.sock",
						"_CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL": "1",
						"CLAUDE_CODE_CLIENT_DATA_URL":              "https://downloads.claude.ai/policy-test.json",
					}[name]
					switch layer {
					case "inherited":
						req.ParentEnv = append(req.ParentEnv, name+"="+value)
					case "fragment":
						req.FragEnv = map[string]string{name: value}
					case "prompt":
						req.PromptEnv = map[string]string{name: value}
					case "mcp":
						req.MCPEnvNames = []string{name}
					case "settings":
						doc, _ := json.Marshal(map[string]any{"env": map[string]string{name: value}})
						rewriteReadOnly(t, filepath.Join(managedHome(req), "settings.json"), string(doc))
					}
					got, err := network.Prepare(context.Background(), req)
					if layer != "none" && layer != "inherited" {
						if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || got.Lease != nil || len(g.calls()) != 0 {
							t.Fatalf("override reached gateway: %v, calls=%v", err, g.calls())
						}
						return
					}
					if err != nil || got.Lease == nil {
						t.Fatalf("valid gateway control refused: %v", err)
					}
					defer got.Lease.Finish(context.Background())
					env := got.Patch.Apply(req.ParentEnv)
					base := ""
					for _, pair := range env {
						if strings.HasPrefix(pair, "ANTHROPIC_BASE_URL=") {
							base = strings.TrimPrefix(pair, "ANTHROPIC_BASE_URL=")
						}
						if name != "ANTHROPIC_BASE_URL" && strings.HasPrefix(pair, name+"=") {
							t.Fatalf("eligibility exception survived: %s", name)
						}
					}
					if base != "http://127.0.0.1:47811/anthropic" {
						t.Fatalf("base is not registration-owned: %q", base)
					}
				})
			}
		}
	}
}

// Native client-data selectors cannot re-enable first-party policy eligibility;
// the ordinary native tail remains a paired positive control.
func TestPrepareGatewayPolicyClientDataNativeTail(t *testing.T) {
	for _, args := range [][]string{nil, {"-p", "--client-data-url", "https://downloads.claude.ai/policy-test.json", "hello"}, {"-p", "--client-data-url=https://downloads.claude.ai/policy-test.json", "hello"}} {
		g := newGateway(t)
		req := gatewayRequest(gatewayHome(t, g, true))
		req.NativeArgs = args
		writeTestFile(t, filepath.Join(managedHome(req), "policy-limits.json"), strings.Replace(safeCachedPolicy, `"enforce_web_search_mcp_isolation":{"allowed":true}`, `"enforce_web_search_mcp_isolation":{"allowed":false}`, 1))
		got, err := network.Prepare(context.Background(), req)
		if args == nil {
			if err != nil || got.Lease == nil {
				t.Fatalf("valid control: %v", err)
			}
			got.Lease.Finish(context.Background())
		} else if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || got.Lease != nil || len(g.calls()) != 0 {
			t.Fatalf("native policy-eligibility selector reached gateway: %v", err)
		}
	}
}

// One refusal covers both caches and every unsafe companion, leaves all
// fixture bytes intact, and precedes gateway I/O. Unknown keys stay private.
func TestPrepareGatewayPolicyAllFilesDiagnostics(t *testing.T) {
	g := newGateway(t)
	req := gatewayRequest(gatewayHome(t, g, true))
	home := managedHome(req)
	files := map[string]string{
		"remote-settings.json": `{"PRIVATE_KEY":"PRIVATE_VALUE"}`,
		"policy-limits.json":   `{"restrictions":{"PRIVATE_KEY":{}},"defaults":{"remote_control_at_startup":true},"monitoring_notice":false}`,
	}
	for file, doc := range files {
		writeTestFile(t, filepath.Join(home, file), doc)
	}
	companions := []string{
		"policy-limits.json.stamp.json", "policy-limits.json.signature.json", "policy-limits.json.signature-iat.json",
		"remote-settings.json.signature.json", "remote-settings.json.signature-iat.json", "remote-settings.json.synced-at",
	}
	for _, file := range companions {
		if err := os.Symlink(filepath.Join(home, "absent"), filepath.Join(home, file)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := network.Prepare(context.Background(), req)
	if err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported || got.Lease != nil || len(g.calls()) != 0 {
		t.Fatalf("unsafe controls reached gateway: %v", err)
	}
	for _, path := range []string{"remote-settings.json at $.*", "policy-limits.json at", "restrictions.*", "defaults.remote_control_at_startup", "monitoring_notice", "policy-limits.json.stamp.json at $", "remote-settings.json.synced-at at $"} {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("missing %q in %v", path, err)
		}
	}
	for _, file := range companions {
		if !strings.Contains(err.Error(), file+" at $") {
			t.Errorf("missing companion diagnostic %q in %v", file, err)
		}
		if target, readErr := os.Readlink(filepath.Join(home, file)); readErr != nil || target != filepath.Join(home, "absent") {
			t.Errorf("companion %s changed: %v", file, readErr)
		}
	}
	if strings.Contains(err.Error(), "PRIVATE_") {
		t.Fatal("cache diagnostic disclosed private content")
	}
	for file, doc := range files {
		data, readErr := os.ReadFile(filepath.Join(home, file))
		if readErr != nil || string(data) != doc {
			t.Fatalf("cache %s changed: %v", file, readErr)
		}
	}
}

// The ordinary populated-cache route still preserves its original refusals.
func TestPrepareGatewayPolicyCacheAdmission(t *testing.T) {
	for _, tc := range []struct{ name, remote, policy, path string }{
		{"safe populated", `{}`, safeCachedPolicy, ""},
		{"safe minimal", `{}`, `{"restrictions":{}}`, ""},
		{"unsafe remote", `{"env":{"ANTHROPIC_BASE_URL":"PRIVATE_VALUE"}}`, safeCachedPolicy, "remote-settings.json"},
		{"unsafe policy", `{}`, strings.Replace(safeCachedPolicy, `"remote_control_at_startup":false`, `"remote_control_at_startup":true`, 1), "defaults.remote_control_at_startup"},
		{"unknown key privacy", `{}`, `{"restrictions":{},"PRIVATE_KEY":"PRIVATE_VALUE"}`, "$.*"},
		{"malformed privacy", `{}`, `PRIVATE_VALUE`, "$"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			writeTestFile(t, filepath.Join(managedHome(req), "remote-settings.json"), tc.remote)
			writeTestFile(t, filepath.Join(managedHome(req), "policy-limits.json"), tc.policy)
			got, err := network.Prepare(context.Background(), req)
			if tc.path == "" {
				if err != nil || got.Lease == nil {
					t.Fatalf("positive: %v", err)
				}
				got.Lease.Finish(context.Background())
			} else if err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported || got.Lease != nil || len(g.calls()) != 0 || !strings.Contains(err.Error(), tc.path) || strings.Contains(err.Error(), "PRIVATE_") {
				t.Fatalf("unsafe: %v, calls=%v", err, g.calls())
			}
		})
	}
}

// A rewrite or a newly created unsafe cache/sidecar after Prepare is checked
// again at the real final boundary. Cache writes remain untouched by refusal;
// a safe atomic rewrite is a valid neighboring control.
func TestPrepareGatewayPolicyCacheRecheck(t *testing.T) {
	for _, kind := range []string{"safe-atomic", "policy", "remote", "new-policy", "sidecar-link"} {
		t.Run(kind, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			home := managedHome(req)
			path := filepath.Join(home, "policy-limits.json")
			if kind != "new-policy" {
				writeTestFile(t, path, safeCachedPolicy)
			}
			got, err := network.Prepare(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			defer got.Lease.Finish(context.Background())
			if err := got.Recheck(context.Background()); err != nil {
				t.Fatalf("unchanged control: %v", err)
			}
			doc := strings.Replace(safeCachedPolicy, `"remote_control_at_startup":false`, `"remote_control_at_startup":true`, 1)
			switch kind {
			case "safe-atomic":
				doc = safeCachedPolicy
				writeTestFile(t, path+".new", doc)
				if err := os.Rename(path+".new", path); err != nil {
					t.Fatal(err)
				}
			case "remote":
				path = filepath.Join(home, "remote-settings.json")
				doc = `{"hooks":{"PRIVATE_KEY":"PRIVATE_VALUE"}}`
				writeTestFile(t, path, doc)
			case "sidecar-link":
				path = filepath.Join(home, "policy-limits.json.stamp.json")
				if err := os.Symlink(filepath.Join(home, "absent"), path); err != nil {
					t.Fatal(err)
				}
			default:
				writeTestFile(t, path, doc)
			}
			err = got.Recheck(context.Background())
			if kind == "safe-atomic" {
				if err != nil {
					t.Fatalf("safe rewrite: %v", err)
				}
			} else if err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported || strings.Contains(err.Error(), "PRIVATE_") {
				t.Fatalf("unsafe rewrite: %v", err)
			}
			if kind == "sidecar-link" {
				if target, err := os.Readlink(path); err != nil || target != filepath.Join(home, "absent") {
					t.Fatalf("link modified: %v", err)
				}
			} else if data, err := os.ReadFile(path); err != nil || string(data) != doc {
				t.Fatalf("cache modified: %v", err)
			}
		})
	}
}
