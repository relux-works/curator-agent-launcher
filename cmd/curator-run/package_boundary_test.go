package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"github.com/relux-works/curator-agent-launcher/internal/network"
	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/providerlimits"
)

// The actual native-launch boundary accepts a sealed package, refuses a changed
// helper before any child output, and allows its owner to clean either result.
// This isolated native fixture does not exercise a live gateway or Codex tools.
func TestNativePackageBoundary(t *testing.T) {
	data, err := os.ReadFile(pipelineHelper)
	if err != nil {
		t.Fatal(err)
	}
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "changed-host"}[changed], func(t *testing.T) {
			root := t.TempDir()
			for _, name := range []string{"codex", "codex-code-mode-host"} {
				if err := os.WriteFile(filepath.Join(root, name), data, 0700); err != nil {
					t.Fatal(err)
				}
			}
			s, err := execution.SnapshotCodexArtifact(filepath.Join(root, "codex"), root, nil, "0.164.0")
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if changed {
				path := filepath.Join(filepath.Dir(s.Path()), "codex-code-mode-host")
				if err := os.Chmod(path, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0500); err != nil {
					t.Fatal(err)
				}
			}
			var out, diag bytes.Buffer
			value := composition.Value{Binary: s.Path(), WorkDir: root, Argv: []string{"child-started"}}
			deps := launchDeps{stdin: strings.NewReader("")}
			net := network.Prepared{}
			code := runNative(context.Background(), value, execution.PermissionDecision{}, agentic.StoredPolicyInspection{}, &out, &diag, deps, func() error { return nil }, &net, nil, s)
			if changed {
				if code == 0 || out.Len() != 0 || !strings.Contains(diag.String(), "final child process was refused") {
					t.Fatalf("changed package started: %d %s %s", code, out.String(), diag.String())
				}
			} else if code != 0 || !strings.Contains(out.String(), "child-started") {
				t.Fatalf("valid package refused: %d %s", code, diag.String())
			}
			stage := filepath.Dir(s.Path())
			s.Close()
			if _, err := os.Stat(stage); !os.IsNotExist(err) {
				t.Fatal("stage leaked")
			}
		})
	}
}

// The real pipeline owns package cleanup and the gateway lease: intact packages
// run, missing hosts refuse before health, and a stage changed at registration
// refuses without activation and still unregisters. No actual Codex is launched.
func TestCodexPackagePipelineCleanup(t *testing.T) {
	for _, scenario := range []string{"success", "child-failure", "missing-host", "changed-after-registration", "source-replaced-after-registration", "preparation-refused"} {
		t.Run(scenario, func(t *testing.T) {
			// Keep the control socket below the platform's sockaddr_un limit.
			stages, err := os.MkdirTemp("/tmp", "pkg-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(stages) })
			t.Setenv("TMPDIR", stages)
			f := entryFixture(t, "codex_cli")
			withTestRelease(f, "codex-cli 0.164.0")
			g := newE2EGateway(t)
			writeGatewayCatalog(t, f, g.socket)
			catalog := filepath.Join(f.dir, ".curator/network-qualified.toml")
			data, err := os.ReadFile(catalog)
			if err != nil {
				t.Fatal(err)
			}
			data = bytes.ReplaceAll(data, []byte(`harness = "claude-code"`), []byte(`harness = "codex-cli"`))
			data = bytes.ReplaceAll(data, []byte(`entrypoint = "exec"`), []byte(`entrypoint = "interactive"`))
			if err := os.WriteFile(catalog, data, 0600); err != nil {
				t.Fatal(err)
			}
			store, err := providerlimits.NewStore(providerlimits.Options{Layout: providerlimits.LayoutAt(t.TempDir())})
			if err != nil {
				t.Fatal(err)
			}
			f.deps.gatewayStore = func() (*providerlimits.Store, error) { return store, nil }
			f.args = []string{"codex_cli", "--permissions", "native", "--model", "gpt-6-astra", "--effort", "high", "--network", "egress-a"}
			host := filepath.Join(filepath.Dir(f.binary), "codex-code-mode-host")
			data, err = os.ReadFile(pipelineHelper)
			if err != nil {
				t.Fatal(err)
			}
			if scenario != "missing-host" {
				if err := os.WriteFile(host, data, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "child-failure" {
				if err := os.WriteFile(filepath.Join(f.dir, "behavior"), []byte("exit:7"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "changed-after-registration" {
				g.beforeReply = func(path string) {
					if path != "/v1/register" {
						return
					}
					paths, err := filepath.Glob(filepath.Join(stages, "curator-adapter-*", "codex-code-mode-host"))
					if err != nil || len(paths) != 1 {
						t.Error("missing staged host at registration")
						return
					}
					if err := os.Chmod(paths[0], 0600); err != nil {
						t.Error(err)
						return
					}
					if err := os.WriteFile(paths[0], []byte("mutated"), 0600); err != nil {
						t.Error(err)
						return
					}
					if err := os.Chmod(paths[0], 0500); err != nil {
						t.Error(err)
					}
				}
			}
			if scenario == "preparation-refused" {
				f.deps.networkEmit = func(io.Writer, *adapterprobe.Provenance) error { return errors.New("synthetic preparation refusal") }
			}
			if scenario == "source-replaced-after-registration" {
				g.beforeReply = func(path string) {
					if path != "/v1/register" {
						return
					}
					for _, source := range []string{f.binary, host} {
						if err := os.WriteFile(source, []byte("replacement source"), 0700); err != nil {
							t.Error(err)
						}
					}
				}
			}
			code, out, diag := f.run()
			switch scenario {
			case "success":
				if code != 0 || len(out) == 0 {
					t.Fatalf("positive refused %d %s", code, diag)
				}
			case "child-failure":
				if code != 7 {
					t.Fatalf("child status lost %d %s", code, diag)
				}
			case "missing-host":
				if code == 0 || len(out) != 0 || !strings.Contains(string(diag), "artifact_package_invalid") || g.calls() != "" {
					t.Fatalf("missing host reached control %d %s %s", code, diag, g.calls())
				}
				f.assertNoChild(t)
			case "changed-after-registration":
				if code == 0 || len(out) != 0 || !strings.Contains(string(diag), "final child process was refused") || g.calls() != "/v1/health /v1/register /v1/unregister" {
					t.Fatalf("mutated stage escaped %d %s %s", code, diag, g.calls())
				}
				f.assertNoChild(t)
			case "preparation-refused", "source-replaced-after-registration":
				if code == 0 || len(out) != 0 || g.calls() != "/v1/health /v1/register /v1/unregister" {
					t.Fatalf("boundary refusal escaped %d %s %s", code, diag, g.calls())
				}
				f.assertNoChild(t)
			}
			if scenario == "success" || scenario == "child-failure" {
				if g.calls() != "/v1/health /v1/register /v1/activate /v1/unregister" {
					t.Fatal(g.calls())
				}
			}
			left, err := filepath.Glob(filepath.Join(stages, "curator-adapter-*"))
			if err != nil || len(left) != 0 {
				t.Fatalf("stage leaked %v", left)
			}
		})
	}
}
