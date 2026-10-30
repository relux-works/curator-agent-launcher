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

// setRoots replaces the fixture's project_roots in the qualified-builds file;
// with no roots the key is removed.
func setRoots(t *testing.T, home string, roots ...string) {
	t.Helper()
	path := filepath.Join(home, ".curator", network.QualifiedFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	line := ""
	if len(roots) > 0 {
		line = "project_roots = [\"" + strings.Join(roots, "\", \"") + "\"]"
	}
	writeTestFile(t, path, strings.Replace(string(data), "project_roots = [\"/\"]", line, 1))
}

func managedHome(req network.Request) string {
	for _, e := range req.ChildEnv {
		if v, ok := strings.CutPrefix(e, "CLAUDE_CONFIG_DIR="); ok {
			return v
		}
	}
	return ""
}

// Admission contract A: each item refuses network_scope_unsupported before
// any gateway call, and each refusal has a nearby admitted control. Same-uid
// file modes, settings reloaded during the session and MDM or server policy
// the host cannot read are the written residual; these tests cover only what
// the launcher checks before spawn.
func TestPrepareGatewayAdmissionContract(t *testing.T) {
	for _, tc := range []struct {
		name  string
		admit bool
		setup func(t *testing.T, home string, req *network.Request)
	}{
		{"control: the contract fixture", true, func(*testing.T, string, *network.Request) {}},
		{"no managed home", false, func(t *testing.T, _ string, req *network.Request) {
			req.ChildEnv = []string{"HOME=" + t.TempDir()}
		}},
		{"relative managed home", false, func(t *testing.T, _ string, req *network.Request) {
			req.ChildEnv = []string{"CLAUDE_CONFIG_DIR=managed"}
		}},
		{"R3-4 composed non-ASCII managed home", false, func(t *testing.T, _ string, req *network.Request) {
			req.ChildEnv = []string{"CLAUDE_CONFIG_DIR=" + contractHome(t, "caf\u00e9")}
		}},
		{"R3-4 decomposed non-ASCII managed home", false, func(t *testing.T, _ string, req *network.Request) {
			req.ChildEnv = []string{"CLAUDE_CONFIG_DIR=" + contractHome(t, "cafe\u0301")}
		}},
		{"control: ASCII managed home made the same way", true, func(t *testing.T, _ string, req *network.Request) {
			req.ChildEnv = []string{"CLAUDE_CONFIG_DIR=" + contractHome(t, "cafe")}
		}},
		{"settings.json missing", false, func(t *testing.T, _ string, req *network.Request) {
			req.ChildEnv = []string{"CLAUDE_CONFIG_DIR=" + t.TempDir()}
		}},
		{"settings.json writable by its owner", false, func(t *testing.T, _ string, req *network.Request) {
			path := filepath.Join(managedHome(*req), "settings.json")
			if err := os.Chmod(path, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"settings.json writable by the group", false, func(t *testing.T, _ string, req *network.Request) {
			path := filepath.Join(managedHome(*req), "settings.json")
			if err := os.Chmod(path, 0o420); err != nil {
				t.Fatal(err)
			}
		}},
		{"settings.json a symlink to a read-only file", false, func(t *testing.T, _ string, req *network.Request) {
			dir := t.TempDir()
			target := filepath.Join(dir, "real.json")
			writeTestFile(t, target, "{}")
			readOnly(t, target)
			managed := filepath.Join(dir, "managed")
			os.MkdirAll(managed, 0o700)
			if err := os.Symlink(target, filepath.Join(managed, "settings.json")); err != nil {
				t.Fatal(err)
			}
			req.ChildEnv = []string{"CLAUDE_CONFIG_DIR=" + managed}
		}},
		{"writable cowork_settings.json", false, func(t *testing.T, _ string, req *network.Request) {
			writeTestFile(t, filepath.Join(managedHome(*req), "cowork_settings.json"), "{}")
		}},
		{"control: read-only cowork_settings.json", true, func(t *testing.T, _ string, req *network.Request) {
			path := filepath.Join(managedHome(*req), "cowork_settings.json")
			writeTestFile(t, path, "{}")
			readOnly(t, path)
		}},
		{"writable legacy .config.json", false, func(t *testing.T, _ string, req *network.Request) {
			writeTestFile(t, filepath.Join(managedHome(*req), ".config.json"), "{}")
		}},
		{"control: writable .claude.json, which Claude Code itself writes", true, func(t *testing.T, _ string, req *network.Request) {
			writeTestFile(t, filepath.Join(managedHome(*req), ".claude.json"), `{"projects":{}}`)
		}},
		{"no project roots", false, func(t *testing.T, home string, _ *network.Request) {
			setRoots(t, home)
		}},
		{"cwd outside every root", false, func(t *testing.T, home string, _ *network.Request) {
			setRoots(t, home, t.TempDir())
		}},
		{"cwd a sibling sharing the root's name prefix", false, func(t *testing.T, home string, req *network.Request) {
			parent := physical(t, t.TempDir())
			os.MkdirAll(filepath.Join(parent, "repo"), 0o700)
			os.MkdirAll(filepath.Join(parent, "repo-other"), 0o700)
			setRoots(t, home, filepath.Join(parent, "repo"))
			req.ProjectRoot = filepath.Join(parent, "repo-other")
		}},
		{"cwd lexically inside, physically outside", false, func(t *testing.T, home string, req *network.Request) {
			root := physical(t, t.TempDir())
			outside := t.TempDir()
			if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
				t.Fatal(err)
			}
			setRoots(t, home, root)
			req.ProjectRoot = filepath.Join(root, "escape")
		}},
		{"control: cwd nested inside a root", true, func(t *testing.T, home string, req *network.Request) {
			root := physical(t, t.TempDir())
			os.MkdirAll(filepath.Join(root, "a", "b"), 0o700)
			setRoots(t, home, root)
			req.ProjectRoot = filepath.Join(root, "a", "b")
		}},
		{"control: cwd a symlink resolving inside a root", true, func(t *testing.T, home string, req *network.Request) {
			root := physical(t, t.TempDir())
			os.MkdirAll(filepath.Join(root, "work"), 0o700)
			link := filepath.Join(t.TempDir(), "shortcut")
			if err := os.Symlink(filepath.Join(root, "work"), link); err != nil {
				t.Fatal(err)
			}
			setRoots(t, home, root)
			req.ProjectRoot = link
		}},
		{"machine-wide MDM preferences", false, func(t *testing.T, _ string, req *network.Request) {
			os.MkdirAll(req.ManagedPreferencesDir, 0o700)
			writeTestFile(t, filepath.Join(req.ManagedPreferencesDir, "com.anthropic.claudecode.plist"), "")
		}},
		{"per-user MDM preferences", false, func(t *testing.T, _ string, req *network.Request) {
			dir := filepath.Join(req.ManagedPreferencesDir, req.Username)
			os.MkdirAll(dir, 0o700)
			writeTestFile(t, filepath.Join(dir, "com.anthropic.claudecode.plist"), "")
		}},
		{"control: another user's MDM preferences", true, func(t *testing.T, _ string, req *network.Request) {
			dir := filepath.Join(req.ManagedPreferencesDir, "someone-else")
			os.MkdirAll(dir, 0o700)
			writeTestFile(t, filepath.Join(dir, "com.anthropic.claudecode.plist"), "")
		}},
		{"server-managed settings cache", false, func(t *testing.T, _ string, req *network.Request) {
			writeTestFile(t, filepath.Join(managedHome(*req), "remote-settings.json"), "{}")
		}},
		{"server policy-limits cache", false, func(t *testing.T, _ string, req *network.Request) {
			writeTestFile(t, filepath.Join(managedHome(*req), "policy-limits.json"), "{}")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			home := gatewayHome(t, g, true)
			req := gatewayRequest(home)
			req.Prober = forbiddenProber{t}
			tc.setup(t, home, &req)
			got, err := network.Prepare(context.Background(), req)
			if tc.admit {
				if err != nil || got.Lease == nil {
					t.Fatalf("valid control refused: %v", err)
				}
				got.Lease.Finish(context.Background())
				return
			}
			if err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported || got.Lease != nil || len(g.calls()) != 0 {
				t.Fatalf("err = %v calls = %v", err, g.calls())
			}
		})
	}
}

// A project root that is not an absolute clean path, or does not exist,
// makes the qualified-builds file invalid rather than admitting anything.
func TestPrepareGatewayInvalidProjectRoots(t *testing.T) {
	for _, root := range []string{"relative/repo", "/tmp/../tmp", "/does/not/exist/261030"} {
		t.Run(root, func(t *testing.T) {
			g := newGateway(t)
			home := gatewayHome(t, g, true)
			setRoots(t, home, root)
			req := gatewayRequest(home)
			req.Prober = forbiddenProber{t}
			got, err := network.Prepare(context.Background(), req)
			if err == nil || refusalCode(t, err) != refusal.CodeProfileInvalid || got.Lease != nil || len(g.calls()) != 0 {
				t.Fatalf("err = %v calls = %v", err, g.calls())
			}
		})
	}
}

// The pre-spawn recheck repeats the contract: a settings.json made writable
// after preparation refuses there. The unchanged recheck is the control.
func TestPrepareGatewayRecheckRepeatsContract(t *testing.T) {
	g := newGateway(t)
	req := gatewayRequest(gatewayHome(t, g, true))
	req.Prober = forbiddenProber{t}
	got, err := network.Prepare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Lease.Finish(context.Background())
	if err := got.Recheck(context.Background()); err != nil {
		t.Fatalf("unchanged recheck = %v", err)
	}
	if err := os.Chmod(filepath.Join(managedHome(req), "settings.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := got.Recheck(context.Background()); err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported {
		t.Fatalf("recheck = %v", err)
	}
}

// The guard reads the home the child is composed with: a managed home set
// only by the fragment overlay is the user settings source, and a route in
// its settings.json refuses. The same home with harmless settings is the
// control. Before the fix the guard read only the plan's environment.
func TestPrepareGatewayReadsTheComposedHome(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings string
		admit    bool
	}{
		{"route in the fragment's home", `{"env":{"ANTHROPIC_BASE_URL":"https://x"}}`, false},
		{"control: harmless fragment home", `{"model":"x"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			managed := t.TempDir()
			writeTestFile(t, filepath.Join(managed, "settings.json"), tc.settings)
			readOnly(t, filepath.Join(managed, "settings.json"))
			req.ChildEnv = nil
			req.FragEnv = map[string]string{"CLAUDE_CONFIG_DIR": managed}
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

// Inside a home that satisfies contract A, each settings source the guard
// reads still refuses a route on its own: the contract does not stand in
// for the guard. The contract fixture (above) is the admitted control.
func TestPrepareGatewayGuardInsideTheContract(t *testing.T) {
	route := `{"env":{"ANTHROPIC_BASE_URL":"https://x"}}`
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, managed string, req *network.Request)
	}{
		{"settings.json", func(t *testing.T, managed string, _ *network.Request) {
			path := filepath.Join(managed, "settings.json")
			os.Chmod(path, 0o600)
			writeTestFile(t, path, route)
			readOnly(t, path)
		}},
		{"read-only cowork_settings.json", func(t *testing.T, managed string, _ *network.Request) {
			path := filepath.Join(managed, "cowork_settings.json")
			writeTestFile(t, path, route)
			readOnly(t, path)
		}},
		{"read-only legacy .config.json", func(t *testing.T, managed string, _ *network.Request) {
			path := filepath.Join(managed, ".config.json")
			writeTestFile(t, path, route)
			readOnly(t, path)
		}},
		{"global .claude.json in the managed home", func(t *testing.T, managed string, _ *network.Request) {
			writeTestFile(t, filepath.Join(managed, ".claude.json"), `{"projects":{},"env":{"ANTHROPIC_UNIX_SOCKET":"/tmp/x.sock"}}`)
		}},
		{"project settings inside the root", func(t *testing.T, _ string, req *network.Request) {
			os.MkdirAll(filepath.Join(req.ProjectRoot, ".claude"), 0o700)
			writeTestFile(t, filepath.Join(req.ProjectRoot, ".claude", "settings.local.json"), route)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			tc.setup(t, managedHome(req), &req)
			got, err := network.Prepare(context.Background(), req)
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || got.Lease != nil || len(g.calls()) != 0 {
				t.Fatalf("err = %v calls = %v", err, g.calls())
			}
		})
	}
}

// contractHome makes a managed home named leaf that otherwise satisfies
// contract A: a read-only settings.json.
func contractHome(t *testing.T, leaf string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), leaf)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, "settings.json"), "{}")
	readOnly(t, filepath.Join(dir, "settings.json"))
	return dir
}

func physical(t *testing.T, path string) string {
	t.Helper()
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
