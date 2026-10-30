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
// options, with the value shapes Claude Code's parser uses, and refuses any
// argument, in any position, equal to a word the runtime dispatches on
// before or outside that parser (R4-1: its entry path searches the raw argv,
// option values and arguments after "--" included). Each refusal comes before
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
		{"command word absorbed by a variadic list", []string{"--allowedTools", "Bash", "doctor", "-p", "hi"}, false},
		{"control: required value that looks refused", []string{"-p", "--append-system-prompt", "--environment", "hi"}, true},
		{"control: optional debug value", []string{"-d", "api", "-p", "hi"}, true},
		{"control: bare -d", []string{"-d", "-p", "hi"}, true},
		{"control: operand after -- that is not a command", []string{"-p", "--", "--environment"}, true},
		{"control: prompt starting with a command word", []string{"-p", "doctor please"}, true},
		{"command word as a later operand", []string{"-p", "hello", "doctor"}, false},
		{"R4-1 --handle-uri as a required value", []string{"-p", "--append-system-prompt", "--handle-uri", "claude-cli://open?cwd=/tmp"}, false},
		{"R4-1 --handle-uri after --", []string{"-p", "--", "--handle-uri", "claude-cli://open?cwd=/tmp"}, false},
		{"R4-1 --handle-uri= form", []string{"-p", "--", "--handle-uri=claude-cli://open"}, false},
		{"R4-1 --session-tunnel as a value", []string{"-p", "--system-prompt", "--session-tunnel", "hi"}, false},
		{"R4-1 --worktree as a value", []string{"-p", "--append-system-prompt", "--worktree", "hi"}, false},
		{"R4-1 --worktree= after --", []string{"-p", "--", "--worktree=x"}, false},
		{"R4-1 -w as a value", []string{"-p", "--system-prompt", "-w", "hi"}, false},
		{"R4-1 --tmux as a value", []string{"-p", "--system-prompt", "--tmux", "hi"}, false},
		{"R4-1 --bg as a value", []string{"-p", "--system-prompt", "--bg", "hi"}, false},
		{"R4-1 --bare as a value", []string{"-p", "--system-prompt", "--bare", "hi"}, false},
		{"R4-1 remote-control as a later operand", []string{"-p", "hi", "remote-control"}, false},
		{"R4-1 background verb as a value", []string{"-p", "--system-prompt", "logs", "hi"}, false},
		{"R4-1 mcp serve after --", []string{"-p", "--", "mcp", "serve"}, false},
		{"R4-1 plugin alias i", []string{"i"}, false},
		{"R4-1 plugin alias autoremove", []string{"-p", "autoremove"}, false},
		{"R4-1 early word sync", []string{"sync"}, false},
		{"R4-1 daemon after the permission switches", []string{"--dangerously-skip-permissions", "daemon"}, false},
		{"R4-1 dashed helper mode", []string{"--daemon-worker"}, false},
		{"R4-1 --runner-declares= form", []string{"--runner-declares=x", "-p", "hi"}, false},
		{"control: prompt containing a reserved word", []string{"-p", "please run --handle-uri later"}, true},
		{"R5-1 --safe-mode as a required value", []string{"-p", "--append-system-prompt", "--safe-mode", "hello"}, false},
		{"R5-1 --bare as a required value", []string{"-p", "--system-prompt", "--bare", "hello"}, false},
		{"R5-1 --agent-teams after --", []string{"-p", "--", "--agent-teams"}, false},
		{"R5-1 --hard-fail as a value", []string{"-p", "--system-prompt", "--hard-fail", "hi"}, false},
		{"R5-1 -c as a value", []string{"-p", "--system-prompt", "-c", "hi"}, false},
		{"R5-1 --resume= after --", []string{"-p", "--", "--resume=abc"}, false},
		{"R5-1 --from-pr as a value", []string{"-p", "--system-prompt", "--from-pr", "hi"}, false},
		{"R5-1 --gh-standin", []string{"-p", "--", "--gh-standin"}, false},
		{"R5-1 version shortcut as a value", []string{"-p", "--system-prompt", "-V", "hi"}, false},
		{"control: --safe-mode after --, which the runtime ignores there", []string{"-p", "--", "--safe-mode"}, true},
		{"control: --bare after --, which the runtime ignores there", []string{"-p", "hi", "--", "--bare"}, true},
		{"control: --safe-mode inside an inline value", []string{"-p", "--append-system-prompt=--safe-mode", "hi"}, true},
		{"control: --safe-mode inside a prompt", []string{"-p", "explain --safe-mode"}, true},
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

// R4-1, environment: names the runtime reads to choose another mode or
// session before its settings (the deep-link URL handler, feature-flag
// overrides, background kind, plugin directories, the remote-session
// families) refuse in an overlay and are removed when inherited; a harmless
// inherited name is kept (the control).
func TestPrepareGatewayDispatchEnvironment(t *testing.T) {
	for _, n := range []string{"__CFBundleIdentifier", "CLAUDE_INTERNAL_FC_OVERRIDES", "CLAUDE_CODE_SESSION_KIND", "CLAUDE_CODE_PLUGIN_DIRS", "CCR_PROXY", "CCR_AGENT_PROXY_ENABLED", "CLAUDE_CODE_REMOTE_SETTINGS_PATH",
		"CLAUDE_CODE_SAFE_MODE", "CLAUDE_CODE_SIMPLE", "CLAUDE_CODE_RESTRICTED", "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "CLAUDE_CODE_ACTION", "CLAUDE_CODE_ENTRYPOINT"} {
		t.Run("overlay "+n, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			req.FragEnv = map[string]string{n: "x"}
			got, err := network.Prepare(context.Background(), req)
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || got.Lease != nil || len(g.calls()) != 0 {
				t.Fatalf("err = %v calls = %v", err, g.calls())
			}
		})
	}
	t.Run("inherited", func(t *testing.T) {
		g := newGateway(t)
		req := gatewayRequest(gatewayHome(t, g, true))
		req.Prober = forbiddenProber{t}
		req.ParentEnv = append(req.ParentEnv, "__CFBundleIdentifier=com.anthropic.claude-code-url-handler", "CCR_PROXY=http://x", "CLAUDE_CODE_REMOTE=1", "CLAUDE_CODE_SAFE_MODE=1", "LANG=C")
		got, err := network.Prepare(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		defer got.Lease.Finish(context.Background())
		env := got.Patch.Apply([]string{"__CFBundleIdentifier=com.anthropic.claude-code-url-handler", "CCR_PROXY=http://x", "CLAUDE_CODE_REMOTE=1", "CLAUDE_CODE_SAFE_MODE=1", "LANG=C"})
		joined := " " + strings.Join(env, " ") + " "
		for _, n := range []string{"__CFBundleIdentifier=", "CCR_PROXY=", "CLAUDE_CODE_REMOTE=", "CLAUDE_CODE_SAFE_MODE="} {
			if strings.Contains(joined, " "+n) {
				t.Fatalf("%s reaches the child: %v", n, env)
			}
		}
		if !strings.Contains(joined, " LANG=C ") {
			t.Fatalf("the harmless name is removed: %v", env)
		}
	})
}

// R4-4: only the global file the runtime selects is a source. An empty
// read-only legacy .config.json is not selected, so the .claude.json beside
// it is (admitted when clean, refused when it carries a route); a non-empty
// legacy file is selected (refused when it carries a route), and the
// .claude.json beside it is then not read, even with a route in it.
func TestPrepareGatewaySelectedGlobalFile(t *testing.T) {
	route := `{"env":{"ANTHROPIC_BASE_URL":"https://x"}}`
	for _, tc := range []struct {
		name           string
		legacy, claude string
		admit          bool
	}{
		{"control: empty legacy, clean .claude.json", "", `{"projects":{}}`, true},
		{"empty legacy, route in .claude.json", "", route, false},
		{"route in the selected legacy file", route, `{"projects":{}}`, false},
		{"control: selected legacy file, route in the unselected .claude.json", `{"model":"x"}`, route, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			managed := managedHome(req)
			writeTestFile(t, filepath.Join(managed, ".config.json"), tc.legacy)
			readOnly(t, filepath.Join(managed, ".config.json"))
			writeTestFile(t, filepath.Join(managed, ".claude.json"), tc.claude)
			got, err := network.Prepare(context.Background(), req)
			if tc.admit {
				if err != nil || got.Lease == nil {
					t.Fatalf("valid control refused: %v", err)
				}
				got.Lease.Finish(context.Background())
				return
			}
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || len(g.calls()) != 0 {
				t.Fatalf("err = %v calls = %v", err, g.calls())
			}
		})
	}
}

// R6-1: in the runtime's own environment namespaces (CLAUDE_, ANTHROPIC_,
// CCR_) only the admitted names pass. The coordinator-mode selector, a model
// override and an arbitrary new name refuse in an overlay and in settings;
// the admitted names (the managed home, the harness's own values, a local-tool
// selector, a credential) and names outside the namespaces pass (the
// controls). The outside-namespace dispatch names keep refusing.
func TestPrepareGatewayNamespaceAllowlist(t *testing.T) {
	for _, tc := range []struct {
		name  string
		env   string
		admit bool
	}{
		{"coordinator mode", "CLAUDE_CODE_COORDINATOR_MODE", false},
		{"model override", "ANTHROPIC_MODEL", false},
		{"a name a later release might add", "CLAUDE_CODE_SOMETHING_NEW", false},
		{"remote-session family", "CCR_ANYTHING", false},
		{"sandbox mode outside the namespace", "IS_SANDBOX", false},
		{"telemetry exporter outside the namespace", "OTEL_EXPORTER_OTLP_ENDPOINT", false},
		{"control: compaction window the harness sets", "CLAUDE_CODE_AUTO_COMPACT_WINDOW", true},
		{"control: prompt suggestions the harness sets", "CLAUDE_CODE_ENABLE_PROMPT_SUGGESTION", true},
		{"control: credential", "ANTHROPIC_API_KEY", true},
		{"control: local-tool selector", "CLAUDE_CODE_USE_POWERSHELL_TOOL", true},
		{"control: name outside the namespaces", "DISABLE_AUTOUPDATER", true},
	} {
		for _, where := range []string{"overlay", "settings"} {
			t.Run(tc.name+" in "+where, func(t *testing.T) {
				g := newGateway(t)
				req := gatewayRequest(gatewayHome(t, g, true))
				req.Prober = forbiddenProber{t}
				if where == "overlay" {
					req.FragEnv = map[string]string{tc.env: "1"}
				} else {
					rewriteReadOnly(t, filepath.Join(managedHome(req), "settings.json"), `{"env":{"`+tc.env+`":"1"}}`)
				}
				got, err := network.Prepare(context.Background(), req)
				if tc.admit {
					if err != nil || got.Lease == nil {
						t.Fatalf("valid control refused: %v", err)
					}
					got.Lease.Finish(context.Background())
					return
				}
				if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || len(g.calls()) != 0 {
					t.Fatalf("err = %v calls = %v", err, g.calls())
				}
			})
		}
	}
}

// R6-1: inherited names in the namespaces that are not admitted are removed
// from the child (the coordinator selector among them); admitted ones and
// names outside the namespaces stay (the controls).
func TestPrepareGatewayRemovesInheritedNamespaceNames(t *testing.T) {
	g := newGateway(t)
	req := gatewayRequest(gatewayHome(t, g, true))
	req.Prober = forbiddenProber{t}
	inherited := []string{"CLAUDE_CODE_COORDINATOR_MODE=1", "ANTHROPIC_MODEL=x", "ANTHROPIC_API_KEY=k", "CLAUDE_CODE_AUTO_COMPACT_WINDOW=200000", "LANG=C"}
	req.ParentEnv = append(req.ParentEnv, inherited...)
	got, err := network.Prepare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Lease.Finish(context.Background())
	joined := " " + strings.Join(got.Patch.Apply(inherited), " ") + " "
	for _, gone := range []string{"CLAUDE_CODE_COORDINATOR_MODE=", "ANTHROPIC_MODEL="} {
		if strings.Contains(joined, " "+gone) {
			t.Fatalf("%s reaches the child: %s", gone, joined)
		}
	}
	for _, kept := range []string{"ANTHROPIC_API_KEY=k", "CLAUDE_CODE_AUTO_COMPACT_WINDOW=200000", "LANG=C"} {
		if !strings.Contains(joined, " "+kept+" ") {
			t.Fatalf("%s is removed: %s", kept, joined)
		}
	}
}
