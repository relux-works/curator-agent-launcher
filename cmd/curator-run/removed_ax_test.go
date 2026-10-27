//go:build unix

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
)

// Every retired location and file kind refuses before resolve, admission or a
// child, even with an explicit host selection; absence is a real native control.
func TestRemovedAXBeforeLaunch(t *testing.T) {
	for _, layer := range []string{"system", "user", "legacy-system", "legacy-user"} {
		for _, kind := range []string{"enabled", "disabled", "empty", "malformed", "symlink", "directory", "fifo", "unreadable"} {
			for _, host := range []string{"", "--native", "--hosted", "--untracked"} {
				t.Run(layer+"/"+kind+"/"+host, func(t *testing.T) {
					f := entryFixture(t, "pi")
					loc := configfile.Locations{System: filepath.Join(f.dir, "system"), User: filepath.Join(f.dir, "user"), LegacySystem: filepath.Join(f.dir, "legacy-system"), LegacyUser: filepath.Join(f.dir, "legacy-user")}
					for _, dir := range []string{loc.System, loc.User, loc.LegacySystem, loc.LegacyUser} {
						if err := os.MkdirAll(dir, 0700); err != nil {
							t.Fatal(err)
						}
					}
					f.deps.configLocations = func() (configfile.Locations, error) { return loc, nil }
					path := filepath.Join(f.dir, layer, "ax.json")
					switch kind {
					case "symlink":
						if err := os.Symlink(path+"-missing", path); err != nil {
							t.Fatal(err)
						}
					case "directory":
						if err := os.Mkdir(path, 0700); err != nil {
							t.Fatal(err)
						}
					case "fifo":
						if err := syscall.Mkfifo(path, 0600); err != nil {
							t.Fatal(err)
						}
					default:
						data := map[string]string{"enabled": `{"schema":"curator-run-ax-v1","enabled":true}`, "disabled": `{"schema":"curator-run-ax-v1","enabled":false}`, "malformed": "{invalid"}[kind]
						writeFixture(t, path, []byte(data), 0600)
						if kind == "unreadable" {
							if err := os.Chmod(path, 0); err != nil {
								t.Fatal(err)
							}
							t.Cleanup(func() { _ = os.Chmod(path, 0600) })
						}
					}
					f.args = []string{"pi"}
					if host != "" {
						f.args = append(f.args, host)
					}
					code, out, stderr := f.run()
					want := fmt.Sprintf("curator-run: ax_tracking_removed: delete this file: path=%q\n", path)
					if code != 2 || len(out) != 0 || string(stderr) != want || f.resolver.calls != 0 || f.builds != 0 || f.verdicts != 0 {
						t.Fatalf("retired ax policy reached launch: exit=%d resolve=%d build=%d verdict=%d stdout=%q stderr=%q want=%q", code, f.resolver.calls, f.builds, f.verdicts, out, stderr, want)
					}
					f.assertNoChild(t)
				})
			}
		}
	}
	t.Run("absent-positive", func(t *testing.T) {
		f := entryFixture(t, "pi")
		insertLauncherArgs(f, "--permissions", "native")
		code, out, stderr := f.run()
		if code != 0 || len(out) == 0 || f.resolver.calls != 1 || f.builds != 1 {
			t.Fatalf("absence must launch: exit=%d out=%s stderr=%s", code, out, stderr)
		}
	})
}

// Removed options reject all launcher-position forms with the versioned remedy
// and no effect; the same tokens after -- remain opaque provider arguments.
func TestRemovedAXOptions(t *testing.T) {
	for _, flag := range []string{"--name", "--ax-profile"} {
		for _, suffix := range [][]string{{flag}, {flag, "value"}, {flag + "=value"}, {flag, "--help"}} {
			t.Run(strings.Join(suffix, " "), func(t *testing.T) {
				f := entryFixture(t, "pi")
				f.args = append([]string{"pi"}, suffix...)
				code, out, stderr := f.run()
				if code != 2 || len(out) != 0 || !bytes.HasPrefix(stderr, []byte("curator-run: usage: "+flag+" was removed in 0.6.0\n")) || f.resolver.calls != 0 || f.builds != 0 {
					t.Fatalf("removed option: exit=%d stderr=%s", code, stderr)
				}
				f.assertNoChild(t)
			})
		}
		t.Run(flag+"-native-tail", func(t *testing.T) {
			f := entryFixture(t, "pi")
			f.args = []string{"pi", "--permissions", "native", "--", flag, "value"}
			code, out, stderr := f.run()
			if code != 0 || !bytes.Contains(out, []byte(flag)) {
				t.Fatalf("tail inspected: %d %s %s", code, out, stderr)
			}
		})
	}
}

// The retained alias has identical request, plan, child argv/stdin and environment;
// its only stderr difference is exactly one fixed deprecation line.
func TestUntrackedAliasIsNative(t *testing.T) {
	f := entryFixture(t, "pi")
	var previousOut, previousErr []byte
	canonicalChild := func(out []byte) []byte {
		var child childCapture
		if err := json.Unmarshal(out, &child); err != nil {
			t.Fatal(err)
		}
		sort.Strings(child.Env)
		data, err := json.Marshal(child)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	var previousRequest, previousPlan []byte
	for i, flag := range []string{"--native", "--untracked"} {
		f.args = []string{"pi", flag, "--permissions", "native"}
		f.deps.stdin = strings.NewReader("parent stdin\n\x00\xff")
		code, out, stderr := f.run()
		if code != 0 {
			t.Fatalf("%s: %d %s", flag, code, stderr)
		}
		requestBytes, err := json.Marshal(f.request)
		if err != nil {
			t.Fatal(err)
		}
		planBytes, err := json.Marshal(f.plan)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			previousOut = canonicalChild(out)
			previousErr = stderr
			previousRequest = requestBytes
			previousPlan = planBytes
			continue
		}
		warning := []byte("curator-run: --untracked is deprecated; use --native (alias retained for two releases)\n")
		if bytes.Count(stderr, warning) != 1 || !bytes.Equal(bytes.Replace(stderr, warning, nil, 1), previousErr) || !bytes.Equal(canonicalChild(out), previousOut) || !bytes.Equal(requestBytes, previousRequest) || !bytes.Equal(planBytes, previousPlan) {
			t.Fatalf("alias comparison warning=%d stderr=%t child=%t request=%t plan=%t", bytes.Count(stderr, warning), bytes.Equal(bytes.Replace(stderr, warning, nil, 1), previousErr), bytes.Equal(canonicalChild(out), previousOut), bytes.Equal(requestBytes, previousRequest), bytes.Equal(planBytes, previousPlan))
		}
	}
}
