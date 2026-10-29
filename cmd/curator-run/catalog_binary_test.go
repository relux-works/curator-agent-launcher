//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
)

const binaryCatalog = `schema_version = 1
[[effort_scale]]
word  = "deepest"
after = "max"
[vendors.anthropic.models."claude-x-test"]
systems = ["claude-code"]
effort  = { support = "required", accepts = ["low", "deepest"], recommended = "low" }
[vendors.anthropic.models."claude-opus-5"]
effort = { accepts = ["deepest"] }
`

func writeCatalog(t *testing.T, f *frontFixture, text string) {
	t.Helper()
	dir := filepath.Join(f.root, ".curator")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(dir, "catalog.toml"), []byte(text), 0600)
}

func argvHas(argv []string, flag, value string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == value {
			return true
		}
	}
	return false
}

// Real binary, real module and the operator catalog under the isolated HOME:
// a bad catalog never takes away help, version, usage errors or
// session-manager administration, yet refuses every launch before any provider
// effect; each typed file refusal stops the launch without echoing the file;
// an absent or empty catalog keeps today's launch; a catalog model and a
// catalog effort word reach the exact child argv, and a typo refuses.
func TestCatalogBinary(t *testing.T) {
	bin := frontBuild(t)
	refused := diagnostics.ExitForCode(diagnostics.CodePlanRefused)
	launch := []string{"claude_code", "--model", "claude-opus-5", "--effort", "medium", "--permissions", "native", "--native"}

	t.Run("bad-catalog-keeps-static-and-admin-paths", func(t *testing.T) {
		f := newFront(t, bin)
		writeCatalog(t, f, "schema_version = 1\nbogus-marker-key = 1\n")
		for _, args := range [][]string{{"--version"}, {"--help"}, {"session-manager", "list"}, {"session-manager", "list", "--help"}} {
			code, out, stderr := f.command(bin.launcher, args, nil, false, false, 12*time.Second)
			if code != 0 || out == "" || strings.Contains(stderr, "plan_refused") {
				t.Fatalf("%v with a bad catalog: exit %d stdout %q stderr %s", args, code, out, stderr)
			}
		}
		code, _, stderr := f.command(bin.launcher, []string{"claude_code", "--no-such-flag"}, nil, false, false, 12*time.Second)
		if code != diagnostics.ExitUsage || !strings.Contains(stderr, "usage") || strings.Contains(stderr, "plan_refused") {
			t.Fatalf("usage error masked by the catalog: exit %d stderr %s", code, stderr)
		}
		code, _, stderr = f.command(bin.launcher, launch, nil, false, false, 12*time.Second)
		f.require(t, code, refused, stderr, "plan_refused", nil, 0, false)
	})

	t.Run("typed-refusals-before-provider", func(t *testing.T) {
		for name, write := range map[string]func(f *frontFixture){
			"malformed": func(f *frontFixture) { writeCatalog(t, f, "schema_version = 1\n\"secret-marker\" = 1\n") },
			"conflicting": func(f *frontFixture) {
				writeCatalog(t, f, "schema_version = 1\n[vendors.anthropic.models.\"claude-opus-5\"]\ndescription = \"secret-marker\"\n")
			},
			"unsupported": func(f *frontFixture) { writeCatalog(t, f, "schema_version = 2\n") },
			"read-failed": func(f *frontFixture) {
				if err := os.MkdirAll(filepath.Join(f.root, ".curator", "catalog.toml"), 0700); err != nil {
					t.Fatal(err)
				}
			},
		} {
			t.Run(name, func(t *testing.T) {
				f := newFront(t, bin)
				write(f)
				code, _, stderr := f.command(bin.launcher, launch, nil, false, false, 12*time.Second)
				f.require(t, code, refused, stderr, "plan_refused", nil, 0, false)
				if strings.Contains(stderr, "secret-marker") || strings.Contains(stderr, f.root) {
					t.Fatalf("refusal echoes the catalog or a path: %s", stderr)
				}
			})
		}
	})

	t.Run("absent-and-empty-catalog-keep-todays-launch", func(t *testing.T) {
		for _, text := range []string{"", "schema_version = 1\n"} {
			f := newFront(t, bin)
			if text != "" {
				writeCatalog(t, f, text)
			}
			code, _, stderr := f.command(bin.launcher, launch, nil, false, false, 12*time.Second)
			f.require(t, code, 0, stderr, "", nil, 0, true)
			if c := readNative(t, f); !argvHas(c.Argv, "--model", "claude-opus-5") || !argvHas(c.Argv, "--effort", "medium") {
				t.Fatalf("baseline argv %q", c.Argv)
			}
		}
	})

	t.Run("catalog-model-and-word-reach-argv", func(t *testing.T) {
		for _, tc := range []struct{ model, effort string }{{"claude-x-test", "deepest"}, {"claude-opus-5", "deepest"}} {
			f := newFront(t, bin)
			writeCatalog(t, f, binaryCatalog)
			args := []string{"claude_code", "--model", tc.model, "--effort", tc.effort, "--permissions", "native", "--native"}
			code, _, stderr := f.command(bin.launcher, args, nil, false, false, 12*time.Second)
			f.require(t, code, 0, stderr, "", nil, 0, true)
			c := readNative(t, f)
			if !argvHas(c.Argv, "--model", tc.model) || !argvHas(c.Argv, "--effort", tc.effort) {
				t.Fatalf("%s/%s argv %q", tc.model, tc.effort, c.Argv)
			}
		}
		f := newFront(t, bin)
		writeCatalog(t, f, binaryCatalog)
		args := []string{"claude_code", "--model", "claude-x-tset", "--effort", "deepest", "--permissions", "native", "--native"}
		code, _, stderr := f.command(bin.launcher, args, nil, false, false, 12*time.Second)
		f.require(t, code, refused, stderr, "plan_refused", nil, 0, false)
	})
}
