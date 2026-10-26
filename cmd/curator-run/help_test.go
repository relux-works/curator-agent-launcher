package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

// Help and version are static (CLI review F2/F3). With a malformed machine
// launcher.json and a malformed registry, every help and version form exits
// 0, writes only stdout, never reaches fragment resolution, and leaves the
// configuration tree byte-identical. The neighbouring non-help commands
// still refuse on the same broken state, so the help path is not a bypass.
func TestHelpBeforeConfig(t *testing.T) {
	broken := func(t *testing.T) (launchDeps, string) {
		deps := testDeps(t, fragment.NewWithRunner("curator", forbiddenRunner{t}))
		root := filepath.Dir(filepath.Dir(deps.defaults.Machine))
		writeFixture(t, filepath.Join(root, "machine", "launcher.json"), []byte("{bad"), 0600)
		writeFixture(t, filepath.Join(root, "machine", "session-managers.json"), []byte("{bad"), 0600)
		return deps, root
	}
	for _, tc := range []struct {
		args  []string
		first string
	}{
		{[]string{"--help"}, "curator-run "},
		{[]string{"-h"}, "curator-run "},
		{[]string{"--version"}, "curator-run "},
		{[]string{"claude_code", "--help"}, "curator-run "},
		{[]string{"session-manager", "--help"}, "usage: curator-run session-manager list"},
		{[]string{"session-manager", "use", "board", "--help"}, "usage: curator-run session-manager use"},
		{[]string{"session-manager", "select", "board", "--help"}, "usage: curator-run session-manager select"},
		{[]string{"session-manager", "register", "--help"}, "usage: curator-run session-manager register"},
		{[]string{"session-manager", "register", "--manifest", "-", "-h"}, "usage: curator-run session-manager register"},
		{[]string{"session-manager", "bogus", "--help"}, "usage: curator-run session-manager list"},
		{[]string{"werkstatt", "--help"}, "usage: curator run werkstatt up"},
		{[]string{"werkstatt", "update", "example", "--help"}, "usage: curator run werkstatt update"},
		{[]string{"werkstatt", "recover", "example", "--help"}, "usage: curator run werkstatt recover"},
		{[]string{"werkstatt", "status", "example", "--help"}, "usage: curator run werkstatt status"},
		{[]string{"werkstatt", "up", "example", "--task-class", "--help"}, "usage: curator run werkstatt up"},
		{[]string{"migrate", "--help"}, "usage: curator run migrate"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			deps, root := broken(t)
			before := snapshotTree(t, root)
			var out, errOut strings.Builder
			code := run(context.Background(), tc.args, &out, &errOut, deps)
			if code != 0 || errOut.Len() != 0 || !strings.HasPrefix(out.String(), tc.first) {
				t.Fatalf("exit=%d stderr=%q stdout=%q", code, errOut.String(), out.String())
			}
			after := snapshotTree(t, root)
			if len(after) != len(before) {
				t.Fatalf("help changed the config tree: %v", after)
			}
			for path, data := range before {
				if string(after[path]) != string(data) {
					t.Fatalf("help rewrote %s", path)
				}
			}
		})
	}
	// Controls: the same broken state still refuses real work.
	for _, tc := range []struct {
		args       []string
		code       int
		diagnostic string
	}{
		{[]string{"claude_code"}, 1, "defaults_config_invalid"},
		{[]string{"session-manager", "list"}, 2, "session_manager_registry_invalid"},
	} {
		t.Run("control "+strings.Join(tc.args, " "), func(t *testing.T) {
			deps, _ := broken(t)
			var out, errOut strings.Builder
			code := run(context.Background(), tc.args, &out, &errOut, deps)
			if code != tc.code || !strings.Contains(errOut.String(), tc.diagnostic) || out.Len() != 0 {
				t.Fatalf("exit=%d stderr=%q stdout=%q", code, errOut.String(), out.String())
			}
		})
	}
}
