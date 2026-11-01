package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
	"github.com/relux-works/curator-agent-launcher/internal/network"
)

// Selected Core metadata is queried once before fragment repair. Failure cannot
// resolve a fragment, read OAuth, build a plan or start a child; help queries none.
func TestSelectedConfigBeforeRepair(t *testing.T) {
	for _, mode := range []string{"valid", "failed", "help"} {
		t.Run(mode, func(t *testing.T) {
			f := printNetworkFixture(t)
			calls := 0
			f.deps.readSelectedConfig = func(context.Context) (launcherconfig.Selected, error) {
				calls++
				if f.resolver.calls != 0 || f.builds != 0 {
					t.Fatal("selected config read after repair/admission")
				}
				if mode == "failed" {
					return launcherconfig.Selected{}, errors.New("PRIVATE_QUERY_DETAIL")
				}
				return launcherconfig.Selected{ConfigSchema: 5, ConfigPath: "chosen.json"}, nil
			}
			if mode == "help" {
				f.args = []string{"--help"}
			}
			code, out, stderr := f.run()
			if mode == "failed" {
				if code == 0 || len(out) != 0 || calls != 1 || f.resolver.calls != 0 || f.builds != 0 || strings.Contains(string(stderr), "PRIVATE") {
					t.Fatal("failed selection leaked or continued")
				}
				f.assertNoChild(t)
			} else if code != 0 || (mode == "valid" && calls != 1) || (mode == "help" && calls != 0) {
				t.Fatalf("selection call count/launch: %d %d %s", code, calls, stderr)
			}
		})
	}
}

// Native fresh and native runtime-resume tails share machine-proxy admission.
// CLI wins over a missing machine choice; the missing choice alone refuses
// rather than silently launching without a binding.
func TestSelectedConfigNativeProxyParity(t *testing.T) {
	for _, resume := range []bool{false, true} {
		for _, mode := range []string{"machine", "explicit", "invalid"} {
			t.Run(mode+map[bool]string{false: "/fresh", true: "/resume"}[resume], func(t *testing.T) {
				f := printNetworkFixture(t)
				writeDirectNetworkCatalog(t, f.dir, true)
				f.deps.prober = forbiddenNetworkProber{t}
				if resume {
					f.args = []string{"claude_code", "--permissions", "native", "--", "resume", "synthetic-session", "-p", "hello"}
				}
				profile := "direct-a"
				if mode != "machine" {
					profile = "missing-machine-profile"
				}
				if mode == "explicit" {
					f.args = withNetworkFlag(f.args, "direct-a")
				}
				f.deps.readSelectedConfig = func(context.Context) (launcherconfig.Selected, error) {
					return launcherconfig.Selected{ConfigSchema: 7, ConfigPath: "chosen.json", Proxy: &launcherconfig.Proxy{Profile: profile}}, nil
				}
				code, out, stderr := f.run()
				if mode == "invalid" {
					if code == 0 || len(out) != 0 || !strings.Contains(string(stderr), "network_profile_unknown") {
						t.Fatal("invalid machine profile did not refuse")
					}
					f.assertNoChild(t)
					return
				}
				var child childCapture
				if code != 0 || json.Unmarshal(out, &child) != nil {
					t.Fatalf("native selection failed: %d %s", code, stderr)
				}
				origin := "launcher-config"
				if mode == "explicit" {
					origin = "explicit"
				}
				if !strings.Contains(string(stderr), "profile=direct-a origin="+origin+" ") {
					t.Fatal("incorrect selected provenance")
				}
				for _, entry := range child.Env {
					name, _, _ := strings.Cut(entry, "=")
					if network.IsProxyFamily(name) {
						t.Fatal("selected direct profile left inherited proxy")
					}
				}
			})
		}
	}
}
