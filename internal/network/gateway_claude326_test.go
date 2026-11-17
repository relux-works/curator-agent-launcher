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

// Claim: the 2.1.326 whole-argv token, its two new commands and the eager settings-source token refuse in every native-tail position, while near-miss prompts pass.
func TestPrepareGateway326DispatchWords(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		admit bool
	}{
		{"ignore-trust token as a required value", []string{"-p", "--append-system-prompt", "--ignore-persisted-trust", "hi"}, false},
		{"ignore-trust token after --", []string{"-p", "--", "--ignore-persisted-trust"}, false},
		{"ignore-trust token as a later operand", []string{"-p", "hi", "--ignore-persisted-trust"}, false},
		{"worktree-hook as the prompt", []string{"-p", "worktree-hook"}, false},
		{"worktree-hook as a required value", []string{"-p", "--system-prompt", "worktree-hook", "hi"}, false},
		{"workspace-trust after --", []string{"-p", "--", "workspace-trust"}, false},
		{"workspace-trust as the prompt", []string{"-p", "workspace-trust"}, false},
		{"managed-settings-in-initialize as a required value", []string{"-p", "--system-prompt", "--managed-settings-in-initialize", "hi"}, false},
		{"managed-settings-in-initialize after --", []string{"-p", "--", "--managed-settings-in-initialize"}, false},
		{"control: prompt mentioning the token", []string{"-p", "explain --ignore-persisted-trust please"}, true},
		{"control: near-miss command word", []string{"-p", "worktree-hooks"}, true},
		{"control: required value that looks refused", []string{"-p", "--append-system-prompt", "--environment", "hi"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			req.NativeArgs = tc.args
			got, err := network.Prepare(context.Background(), req)
			if tc.admit {
				if err != nil || got.Lease == nil {
					t.Fatalf("valid control refused: %v", err)
				}
				got.Lease.Finish(context.Background())
				return
			}
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || got.Lease != nil || len(g.calls()) != 0 {
				t.Fatalf("err = %v calls = %v", err, g.calls())
			}
			// The refusal must be the dispatch reservation, not the option
			// allowlist: without the reservation the later-operand case
			// still refuses, but as a non-admitted option.
			if !strings.Contains(err.Error(), "is a word Claude Code dispatches on outside its option parser") {
				t.Fatalf("refusal is not the dispatch reservation: %v", err)
			}
			for _, a := range tc.args {
				if len(a) > 3 && strings.Contains(err.Error(), a) {
					t.Fatalf("the refusal echoes the argument %q: %v", a, err)
				}
			}
		})
	}
}

// Claim: CLAUDE_CODE_IGNORE_PERSISTED_TRUST supplied in any layer refuses under the closed CLAUDE_ namespace, an inherited value is removed, and an admitted namespace value beside it passes.
func TestPrepareGatewayIgnorePersistedTrustEnv(t *testing.T) {
	const name = "CLAUDE_CODE_IGNORE_PERSISTED_TRUST"
	for _, layer := range []string{"fragment", "prompt", "settings", "mcp"} {
		t.Run("supplied in "+layer, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			switch layer {
			case "fragment":
				req.FragEnv = map[string]string{name: "1"}
			case "prompt":
				req.PromptEnv = map[string]string{name: "1"}
			case "mcp":
				req.MCPEnvNames = []string{name}
			case "settings":
				rewriteReadOnly(t, filepath.Join(managedHome(req), "settings.json"), `{"env":{"`+name+`":"1"}}`)
			}
			got, err := network.Prepare(context.Background(), req)
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || got.Lease != nil || len(g.calls()) != 0 {
				t.Fatalf("err = %v calls = %v", err, g.calls())
			}
		})
	}
	t.Run("inherited is removed, admitted control kept", func(t *testing.T) {
		g := newGateway(t)
		req := gatewayRequest(gatewayHome(t, g, true))
		req.Prober = forbiddenProber{t}
		inherited := []string{name + "=1", "CLAUDE_CODE_AUTO_COMPACT_WINDOW=200000", "LANG=C"}
		req.ParentEnv = append(req.ParentEnv, inherited...)
		got, err := network.Prepare(context.Background(), req)
		if err != nil || got.Lease == nil {
			t.Fatalf("valid control refused: %v", err)
		}
		defer got.Lease.Finish(context.Background())
		joined := " " + strings.Join(got.Patch.Apply(inherited), " ") + " "
		if strings.Contains(joined, " "+name+"=") {
			t.Fatalf("%s reaches the child: %s", name, joined)
		}
		for _, kept := range []string{"CLAUDE_CODE_AUTO_COMPACT_WINDOW=200000", "LANG=C"} {
			if !strings.Contains(joined, " "+kept+" ") {
				t.Fatalf("%s is removed: %s", kept, joined)
			}
		}
	})
}

// Claim: for every exact-name legacy/new global-file state a contract-A home can hold (legacy absent, empty read-only or non-empty read-only, against new file absent, empty, non-empty or non-file), the guard checks the env of the file both the 2.1.316 and 2.1.326 choosers select, with clean controls admitted and each refusal naming why it fired.
func TestPrepareGatewayGlobalFile326Cases(t *testing.T) {
	route := `{"env":{"ANTHROPIC_BASE_URL":"https://x"}}`
	clean := `{"projects":{}}`
	routeDetail := "ANTHROPIC_BASE_URL"
	decodeDetail := "cannot be decoded"
	unreadableDetail := "is not a readable regular file"
	for _, tc := range []struct {
		name          string
		legacy, newer string
		hasLegacy     bool
		hasNew        bool
		newIsDir      bool
		admit         bool
		detail        string
	}{
		{"control: empty legacy, clean new file selected", "", clean, true, true, false, true, ""},
		{"control: non-empty legacy selected, clean new file beside it", clean, clean, true, true, false, true, ""},
		{"control: non-empty legacy selected, route in the unselected new file", clean, route, true, true, false, true, ""},
		{"control: no legacy, no new file", "", "", false, false, false, true, ""},
		{"control: empty legacy, no new file", "", "", true, false, false, true, ""},
		{"control: clean legacy selected, no new file", clean, "", true, false, false, true, ""},
		{"control: clean legacy selected beside a non-file new path", clean, "", true, true, true, true, ""},
		{"empty legacy, empty selected new file cannot be decoded", "", "", true, true, false, false, decodeDetail},
		{"no legacy, empty selected new file cannot be decoded", "", "", false, true, false, false, decodeDetail},
		{"empty legacy, route in the selected new file", "", route, true, true, false, false, routeDetail},
		{"no legacy, route in the new file", "", route, false, true, false, false, routeDetail},
		{"route in the selected legacy file, no new file", route, "", true, false, false, false, routeDetail},
		{"route in the selected legacy file, clean new file beside it", route, clean, true, true, false, false, routeDetail},
		{"route in the selected legacy file beside a non-file new path", route, "", true, true, true, false, routeDetail},
		{"no legacy, non-file new path is not readable", "", "", false, true, true, false, unreadableDetail},
		{"empty legacy, non-file selected new path is not readable", "", "", true, true, true, false, unreadableDetail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			managed := managedHome(req)
			if tc.hasLegacy {
				writeTestFile(t, filepath.Join(managed, ".config.json"), tc.legacy)
				readOnly(t, filepath.Join(managed, ".config.json"))
			}
			if tc.hasNew && tc.newIsDir {
				if err := os.Mkdir(filepath.Join(managed, ".claude.json"), 0o755); err != nil {
					t.Fatal(err)
				}
			} else if tc.hasNew {
				writeTestFile(t, filepath.Join(managed, ".claude.json"), tc.newer)
			}
			got, err := network.Prepare(context.Background(), req)
			if tc.admit {
				if err != nil || got.Lease == nil {
					t.Fatalf("valid control refused: %v", err)
				}
				got.Lease.Finish(context.Background())
				return
			}
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || got.Lease != nil || len(g.calls()) != 0 {
				t.Fatalf("err = %v calls = %v", err, g.calls())
			}
			if tc.detail != "" && !strings.Contains(err.Error(), tc.detail) {
				t.Fatalf("refusal does not name %q: %v", tc.detail, err)
			}
		})
	}
}

// Claim: when both global files are non-empty but the legacy entry is not listed under its exact name (a case-variant on a case-insensitive filesystem), the guard refuses with the listing refusal instead of guessing which file the runtime reads — whichever file holds the route — while the exact-name control is admitted.
func TestPrepareGatewayGlobalFileCaseVariant(t *testing.T) {
	route := `{"env":{"ANTHROPIC_BASE_URL":"https://x"}}`
	clean := `{"projects":{}}`
	const listingDetail = "the legacy global file is not listed under its exact name"
	setup := func(t *testing.T, legacy, newer string) (*gateway, network.Request) {
		t.Helper()
		g := newGateway(t)
		req := gatewayRequest(gatewayHome(t, g, true))
		req.Prober = forbiddenProber{t}
		managed := managedHome(req)
		writeTestFile(t, filepath.Join(managed, ".config.json"), legacy)
		readOnly(t, filepath.Join(managed, ".config.json"))
		writeTestFile(t, filepath.Join(managed, ".claude.json"), newer)
		return g, req
	}
	// The 2.1.316/2.1.318 chooser reads the (clean) legacy file here and
	// the 2.1.326 chooser reads the (route-bearing) new file; the refusal
	// must be the listing refusal, and a guard that simply read the new
	// file would refuse with the route detail instead.
	t.Run("case-variant listing refuses", func(t *testing.T) {
		restore := network.SetExactLegacyListed(func(string, string, func() error) (bool, error) { return false, nil })
		defer restore()
		g, req := setup(t, clean, route)
		got, err := network.Prepare(context.Background(), req)
		if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || got.Lease != nil || len(g.calls()) != 0 {
			t.Fatalf("err = %v calls = %v", err, g.calls())
		}
		if !strings.Contains(err.Error(), listingDetail) {
			t.Fatalf("refusal is not the listing refusal: %v", err)
		}
	})
	// The other direction: the route sits in the legacy file the older
	// choosers read, and the new file 2.1.326 would read is clean. A
	// guard that mirrored 2.1.326 would admit; this one must still
	// refuse with the listing refusal.
	t.Run("case-variant listing refuses with the route in the legacy file", func(t *testing.T) {
		restore := network.SetExactLegacyListed(func(string, string, func() error) (bool, error) { return false, nil })
		defer restore()
		g, req := setup(t, route, clean)
		got, err := network.Prepare(context.Background(), req)
		if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || got.Lease != nil || len(g.calls()) != 0 {
			t.Fatalf("err = %v calls = %v", err, g.calls())
		}
		if !strings.Contains(err.Error(), listingDetail) {
			t.Fatalf("refusal is not the listing refusal: %v", err)
		}
	})
	t.Run("control: exact-name listing admits", func(t *testing.T) {
		restore := network.SetExactLegacyListed(func(string, string, func() error) (bool, error) { return true, nil })
		defer restore()
		_, req := setup(t, clean, route)
		got, err := network.Prepare(context.Background(), req)
		if err != nil || got.Lease == nil {
			t.Fatalf("valid control refused: %v", err)
		}
		got.Lease.Finish(context.Background())
	})
}

// Claim: CLAUDE_CODE_MODEL_CATALOG_URL supplied in any layer refuses under the closed CLAUDE_ namespace, an inherited value is removed, and the admitted control keeps the gateway-owned catalog pair set to off.
func TestPrepareGatewayModelCatalogURLEnv(t *testing.T) {
	const name = "CLAUDE_CODE_MODEL_CATALOG_URL"
	for _, layer := range []string{"fragment", "prompt", "settings", "mcp"} {
		t.Run("supplied in "+layer, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			switch layer {
			case "fragment":
				req.FragEnv = map[string]string{name: "file:///tmp/catalog.json"}
			case "prompt":
				req.PromptEnv = map[string]string{name: "file:///tmp/catalog.json"}
			case "mcp":
				req.MCPEnvNames = []string{name}
			case "settings":
				rewriteReadOnly(t, filepath.Join(managedHome(req), "settings.json"), `{"env":{"`+name+`":"file:///tmp/catalog.json"}}`)
			}
			got, err := network.Prepare(context.Background(), req)
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || got.Lease != nil || len(g.calls()) != 0 {
				t.Fatalf("err = %v calls = %v", err, g.calls())
			}
		})
	}
	t.Run("inherited is removed, admitted control kept", func(t *testing.T) {
		g := newGateway(t)
		req := gatewayRequest(gatewayHome(t, g, true))
		req.Prober = forbiddenProber{t}
		inherited := []string{name + "=file:///tmp/catalog.json", "CLAUDE_CODE_AUTO_COMPACT_WINDOW=200000", "LANG=C"}
		req.ParentEnv = append(req.ParentEnv, inherited...)
		got, err := network.Prepare(context.Background(), req)
		if err != nil || got.Lease == nil {
			t.Fatalf("valid control refused: %v", err)
		}
		defer got.Lease.Finish(context.Background())
		joined := " " + strings.Join(got.Patch.Apply(inherited), " ") + " "
		if strings.Contains(joined, " "+name+"=") {
			t.Fatalf("%s reaches the child: %s", name, joined)
		}
		for _, kept := range []string{"CLAUDE_CODE_AUTO_COMPACT_WINDOW=200000", "LANG=C", "CLAUDE_CODE_MODEL_CATALOG=off"} {
			if !strings.Contains(joined, " "+kept+" ") {
				t.Fatalf("%s is removed: %s", kept, joined)
			}
		}
	})
}
