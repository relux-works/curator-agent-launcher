package network_test

import (
	"context"
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
