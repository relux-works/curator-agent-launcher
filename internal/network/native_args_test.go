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

// Under a filtering gateway the native tail admits only the allowlisted
// options, with the value shapes Claude Code's parser uses, and refuses a
// first operand that names a Claude Code command. Each refusal comes before
// any gateway call, inside a fixture that otherwise satisfies contract A;
// the admitted rows are the controls, including values that only look like
// refused options and operands after "--". The parser's behaviour is read
// from the 2.1.314 bundle; no runtime is started here.
func TestPrepareGatewayNativeTailAllowlist(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		admit bool
	}{
		{"R3-1 --environment", []string{"-p", "--environment", "ccpool_x", "hi"}, false},
		{"R3-1 --environment=", []string{"-p", "--environment=ccpool_x", "hi"}, false},
		{"R3-1 --pool", []string{"--pool", "p1", "-p", "hi"}, false},
		{"R3-1 --attach-serve", []string{"--attach-serve", "s1"}, false},
		{"--correlation-id", []string{"-p", "--correlation-id", "c", "hi"}, false},
		{"--cloud", []string{"--cloud", "task"}, false},
		{"--remote", []string{"--remote"}, false},
		{"--teleport", []string{"--teleport"}, false},
		{"--remote-control", []string{"--remote-control"}, false},
		{"--rc", []string{"--rc"}, false},
		{"--sdk-url", []string{"-p", "--sdk-url", "ws://x", "hi"}, false},
		{"--settings", []string{"-p", "--settings", "{}", "hi"}, false},
		{"--add-dir", []string{"-p", "--add-dir", "/tmp", "hi"}, false},
		{"--plugin-dir", []string{"-p", "--plugin-dir", "/tmp/p", "hi"}, false},
		{"--worktree", []string{"--worktree"}, false},
		{"-w", []string{"-w", "x"}, false},
		{"-c", []string{"-c"}, false},
		{"-r", []string{"-r", "id"}, false},
		{"cluster -pc", []string{"-p", "-pc"}, false},
		{"cluster -cv", []string{"-p", "-cv"}, false},
		{"unknown option", []string{"-p", "--brand-new-option", "hi"}, false},
		{"unknown short", []string{"-p", "-x", "hi"}, false},
		{"value given to a switch", []string{"--print=yes", "hi"}, false},
		{"required value missing", []string{"-p", "--model"}, false},
		{"command as the prompt", []string{"-p", "doctor"}, false},
		{"cloud command first", []string{"ultrareview"}, false},
		{"command after --", []string{"-p", "--", "remote-control"}, false},
		{"alias command", []string{"rc"}, false},
		{"command after an inline variadic", []string{"--allowedTools=Bash", "mcp"}, false},
		{"control: -p prompt", []string{"-p", "hello"}, true},
		{"control: model and effort", []string{"-p", "--model", "claude-opus-5-5", "--effort", "high", "hi"}, true},
		{"control: inline value", []string{"--output-format=json", "-p", "hi"}, true},
		{"control: variadic then option", []string{"--allowedTools", "Bash", "Edit", "-p", "hi"}, true},
		{"control: variadic absorbs a command word", []string{"--allowedTools", "Bash", "doctor", "-p", "hi"}, true},
		{"control: required value that looks refused", []string{"-p", "--append-system-prompt", "--environment", "hi"}, true},
		{"control: optional debug value", []string{"-d", "api", "-p", "hi"}, true},
		{"control: bare -d", []string{"-d", "-p", "hi"}, true},
		{"control: operand after -- that is not a command", []string{"-p", "--", "--environment"}, true},
		{"control: prompt starting with a command word", []string{"-p", "doctor please"}, true},
		{"control: second operand is a command word", []string{"-p", "hello", "doctor"}, true},
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
			for _, a := range tc.args {
				if len(a) > 3 && strings.Contains(err.Error(), a) {
					t.Fatalf("the refusal echoes the argument %q: %v", a, err)
				}
			}
		})
	}
}

// R3-6: only route-bearing names refuse. The two audited CLAUDE_CODE_USE_
// feature selectors are admitted from a fragment and from settings and kept
// when inherited; provider routes, the cowork source selector and an unknown
// CLAUDE_CODE_USE_ name still refuse, as does the Unix socket. Global
// configuration is read only from the selected .claude.json: an unrelated or
// unselected .claude-*.json file, even an undecodable one, is not a source.
func TestPrepareGatewayRouteNamesAndSelectedGlobal(t *testing.T) {
	route := `{"env":{"ANTHROPIC_BASE_URL":"https://x"}}`
	for _, tc := range []struct {
		name  string
		admit bool
		setup func(t *testing.T, managed string, req *network.Request)
	}{
		{"control: PowerShell tool in the fragment", true, func(_ *testing.T, _ string, req *network.Request) {
			req.FragEnv = map[string]string{"CLAUDE_CODE_USE_POWERSHELL_TOOL": "1"}
		}},
		{"control: native file search in settings", true, func(t *testing.T, managed string, _ *network.Request) {
			rewriteReadOnly(t, filepath.Join(managed, "settings.json"), `{"env":{"CLAUDE_CODE_USE_NATIVE_FILE_SEARCH":"1"}}`)
		}},
		{"Bedrock route in the fragment", false, func(_ *testing.T, _ string, req *network.Request) {
			req.FragEnv = map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1"}
		}},
		{"remote session route in settings", false, func(t *testing.T, managed string, _ *network.Request) {
			rewriteReadOnly(t, filepath.Join(managed, "settings.json"), `{"env":{"CLAUDE_CODE_USE_CCR_V2":"1"}}`)
		}},
		{"cowork selector in the fragment", false, func(_ *testing.T, _ string, req *network.Request) {
			req.FragEnv = map[string]string{"CLAUDE_CODE_USE_COWORK_PLUGINS": "1"}
		}},
		{"unknown CLAUDE_CODE_USE_ name", false, func(_ *testing.T, _ string, req *network.Request) {
			req.FragEnv = map[string]string{"CLAUDE_CODE_USE_SOMETHING_NEW": "1"}
		}},
		{"Unix socket in the fragment", false, func(_ *testing.T, _ string, req *network.Request) {
			req.FragEnv = map[string]string{"ANTHROPIC_UNIX_SOCKET": "/tmp/x.sock"}
		}},
		{"control: unrelated .claude-notes.json with a route", true, func(t *testing.T, managed string, _ *network.Request) {
			writeTestFile(t, filepath.Join(managed, ".claude-notes.json"), route)
		}},
		{"control: unselected .claude-staging-oauth.json, undecodable", true, func(t *testing.T, managed string, _ *network.Request) {
			writeTestFile(t, filepath.Join(managed, ".claude-staging-oauth.json"), `{"env":`)
		}},
		{"selected .claude.json with a route", false, func(t *testing.T, managed string, _ *network.Request) {
			writeTestFile(t, filepath.Join(managed, ".claude.json"), route)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			tc.setup(t, managedHome(req), &req)
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
		})
	}
}

// R3-6: an inherited feature selector stays in the child; an inherited
// route selector is removed (the control for the same mechanism).
func TestPrepareGatewayKeepsInheritedFeatureSelectors(t *testing.T) {
	g := newGateway(t)
	req := gatewayRequest(gatewayHome(t, g, true))
	req.Prober = forbiddenProber{t}
	req.ParentEnv = append(req.ParentEnv, "CLAUDE_CODE_USE_POWERSHELL_TOOL=1", "CLAUDE_CODE_USE_VERTEX=1")
	got, err := network.Prepare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Lease.Finish(context.Background())
	unset := " " + strings.Join(got.Patch.Unset, " ") + " "
	if strings.Contains(unset, " CLAUDE_CODE_USE_POWERSHELL_TOOL ") {
		t.Fatalf("the feature selector is removed: %s", unset)
	}
	if !strings.Contains(unset, " CLAUDE_CODE_USE_VERTEX ") {
		t.Fatalf("the route selector is kept: %s", unset)
	}
}

func rewriteReadOnly(t *testing.T, path, data string) {
	t.Helper()
	os.Chmod(path, 0o600)
	writeTestFile(t, path, data)
	readOnly(t, path)
}
