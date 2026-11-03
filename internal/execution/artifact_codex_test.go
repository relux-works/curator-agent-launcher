package execution_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
)

func codexPackage(t *testing.T, layout, resource bool) (string, string) {
	t.Helper()
	root := t.TempDir()
	entry := "codex"
	if layout {
		entry = "bin/codex"
		writePackage(t, root, "codex-package.json", []byte(`{"layoutVersion":1,"version":"0.164.0","target":"fixture","entrypoint":"bin/codex","resourcesDir":"codex-resources","pathDir":"codex-path"}`), 0600)
		writePackage(t, root, "codex-path/rg", []byte("bundled search"), 0700)
		writePackage(t, root, "codex-resources/voice/manifest.json", []byte("voice fixture"), 0600)
	}
	writePackage(t, root, entry, []byte("native entrypoint fixture"), 0700)
	host := filepath.Join(filepath.Dir(entry), "codex-code-mode-host")
	writePackage(t, root, host, []byte("sibling host"), 0700)
	if resource {
		writePackage(t, root, "codex-resources/codex-code-mode-host", []byte("preferred host"), 0700)
	}
	return root, filepath.Join(root, entry)
}

func writePackage(t *testing.T, root, rel string, data []byte, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}

func mutatePackage(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// These fixtures ground paths in the pinned InstallContext contract. They prove
// layout/content preservation, not real Codex tool execution or qualification.
func TestCodexPackageSupportedLayouts(t *testing.T) {
	for _, tc := range []struct {
		name             string
		layout, resource bool
	}{
		{"legacy-sibling", false, false},
		{"package-sibling", true, false},
		{"package-resource-precedence", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, entry := codexPackage(t, tc.layout, tc.resource)
			original, _ := os.ReadFile(entry)
			s, err := execution.SnapshotCodexArtifact(entry, root, nil, "0.164.0")
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			receipt := s.PackageReceipt()
			if receipt.MainBuild != s.BuildID() || receipt.AdmittedRelease != "0.164.0" || len(receipt.Members) == 0 {
				t.Fatal("closure receipt is not paired with the main artifact/release")
			}
			receipt.Members[0].SHA256 = "changed copy"
			if s.PackageReceipt().Members[0].SHA256 == "changed copy" {
				t.Fatal("receipt exposes mutable seals")
			}
			if s.BuildID() != fmt.Sprintf("sha256-%x", sha256.Sum256(original)) || !bytes.Equal(s.Bytes(), original) {
				t.Fatal("main adapter identity changed")
			}
			stageRoot := filepath.Dir(s.Path())
			if tc.layout {
				stageRoot = filepath.Dir(stageRoot)
				for _, rel := range []string{"codex-package.json", "codex-path/rg", "codex-resources/voice/manifest.json"} {
					want, _ := os.ReadFile(filepath.Join(root, rel))
					got, err := os.ReadFile(filepath.Join(stageRoot, rel))
					if err != nil || !bytes.Equal(want, got) {
						t.Fatalf("resource %s not preserved", rel)
					}
				}
			}
			host := filepath.Join(filepath.Dir(s.Path()), "codex-code-mode-host")
			want := "sibling host"
			if tc.resource {
				host, want = filepath.Join(stageRoot, "codex-resources/codex-code-mode-host"), "preferred host"
			}
			got, err := os.ReadFile(host)
			if err != nil || string(got) != want {
				t.Fatal("selected host changed")
			}
			// Replacing the installed source cannot alter the evaluated stage.
			if err := os.WriteFile(entry, []byte("replacement"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := s.Verify(); err != nil {
				t.Fatalf("source replacement affected sealed stage: %v", err)
			}
			got, _ = os.ReadFile(s.Path())
			if !bytes.Equal(got, original) {
				t.Fatal("replacement bytes reached stage")
			}
			s.Close()
			if _, err := os.Stat(stageRoot); !os.IsNotExist(err) {
				t.Fatal("package stage retained after close")
			}
		})
	}
}

// Each invalid source must produce the intended package refusal and clean all
// partially copied files. Nearby valid layouts above rule out reject-all.
func TestCodexPackageSourceRefusals(t *testing.T) {
	for _, name := range []string{"missing-host", "nonexecutable-host", "host-link", "resource-escape", "bin-link", "bad-metadata", "entry-traversal", "version-mismatch", "special-file", "oversized-member", "too-many-members", "deep-tree", "app-bundle"} {
		t.Run(name, func(t *testing.T) {
			stages := t.TempDir()
			t.Setenv("TMPDIR", stages)
			root, entry := codexPackage(t, true, false)
			host := filepath.Join(root, "bin/codex-code-mode-host")
			switch name {
			case "missing-host":
				os.Remove(host)
			case "nonexecutable-host":
				os.Chmod(host, 0600)
			case "host-link":
				os.Remove(host)
				if err := os.Symlink(entry, host); err != nil {
					t.Fatal(err)
				}
			case "resource-escape":
				if err := os.Symlink(t.TempDir(), filepath.Join(root, "codex-resources/escape")); err != nil {
					t.Fatal(err)
				}
			case "bin-link":
				os.Rename(filepath.Join(root, "bin"), filepath.Join(root, "real-bin"))
				if err := os.Symlink("real-bin", filepath.Join(root, "bin")); err != nil {
					t.Fatal(err)
				}
			case "bad-metadata":
				os.WriteFile(filepath.Join(root, "codex-package.json"), []byte("bad"), 0600)
			case "entry-traversal":
				os.WriteFile(filepath.Join(root, "codex-package.json"), []byte(`{"layoutVersion":1,"entrypoint":"../codex"}`), 0600)
			case "version-mismatch":
				os.WriteFile(filepath.Join(root, "codex-package.json"), []byte(`{"layoutVersion":1,"version":"0.163.0","entrypoint":"bin/codex"}`), 0600)
			case "special-file":
				os.Remove(host)
				os.Mkdir(host, 0700)
			case "oversized-member":
				f, err := os.Create(filepath.Join(root, "codex-resources/oversized"))
				if err != nil {
					t.Fatal(err)
				}
				if err := f.Truncate(adapterprobe.MaxArtifactBytes + 1); err != nil {
					t.Fatal(err)
				}
				f.Close()
			case "too-many-members":
				for i := 0; i < 4096; i++ {
					writePackage(t, root, fmt.Sprintf("codex-resources/members/%04d", i), nil, 0600)
				}
			case "deep-tree":
				writePackage(t, root, "codex-resources/"+strings.Repeat("nested/", 33)+"leaf", nil, 0600)
			case "app-bundle":
				writePackage(t, root, "Fixture.app/Contents/MacOS/codex", []byte("app entry"), 0700)
				writePackage(t, root, "Fixture.app/Contents/MacOS/codex-code-mode-host", []byte("app host"), 0700)
				entry = filepath.Join(root, "Fixture.app/Contents/MacOS/codex")
			}
			s, err := execution.SnapshotCodexArtifact(entry, root, nil, "0.164.0")
			var unsupported *adapterprobe.Unsupported
			if s != nil || !errors.As(err, &unsupported) || unsupported.Reason != "artifact_package_invalid" {
				if s != nil {
					s.Close()
				}
				t.Fatalf("expected package refusal, got %v", err)
			}
			files, err := filepath.Glob(filepath.Join(stages, "curator-adapter-*"))
			if err != nil || len(files) != 0 {
				t.Fatal("partial stage leaked")
			}
		})
	}
}

// The final guard detects changed, missing, newly selected or linked stage
// members; it never falls back to a sibling or reseals changed bytes.
func TestCodexPackageSealedMutationRefuses(t *testing.T) {
	for _, name := range []string{"changed-host", "missing-host", "changed-metadata", "added-resource-host", "host-link", "changed-mode"} {
		t.Run(name, func(t *testing.T) {
			root, entry := codexPackage(t, true, false)
			s, err := execution.SnapshotCodexArtifact(entry, root, nil, "0.164.0")
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if err := s.Verify(); err != nil {
				t.Fatal(err)
			}
			stage := filepath.Dir(filepath.Dir(s.Path()))
			host := filepath.Join(filepath.Dir(s.Path()), "codex-code-mode-host")
			switch name {
			case "changed-host":
				mutatePackage(t, host, []byte("mutated"), 0500)
			case "missing-host":
				os.Remove(host)
			case "changed-metadata":
				mutatePackage(t, filepath.Join(stage, "codex-package.json"), []byte("changed"), 0400)
			case "added-resource-host":
				writePackage(t, stage, "codex-resources/codex-code-mode-host", []byte("new selection"), 0500)
			case "host-link":
				os.Remove(host)
				if err := os.Symlink(entry, host); err != nil {
					t.Fatal(err)
				}
			case "changed-mode":
				os.Chmod(host, 0700)
			}
			if s.Verify() == nil {
				t.Fatal("mutated stage accepted")
			}
		})
	}
}

// A real child process in an isolated fixture can find and execute its staged
// sibling after the source is removed. This is not the live Codex acceptance.
func TestCodexPackageChildExecAndCleanup(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	for _, exit := range []string{"0", "7"} {
		t.Run(exit, func(t *testing.T) {
			root, entry := codexPackage(t, false, false)
			writePackage(t, root, "codex", data, 0700)
			writePackage(t, root, "codex-code-mode-host", data, 0700)
			s, err := execution.SnapshotCodexArtifact(entry, root, nil, "0.164.0")
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			os.RemoveAll(root)
			cmd := exec.Command(s.Path(), "-test.run=^TestCodexPackageProcessHelper$", "--", exit)
			cmd.Env = append(os.Environ(), "CURATOR_PACKAGE_TEST=entry")
			out, err := cmd.CombinedOutput()
			if !strings.Contains(string(out), "tool-ready") || ((err == nil) != (exit == "0")) {
				t.Fatalf("child: %s %v", out, err)
			}
			stage := filepath.Dir(s.Path())
			s.Close()
			if _, err := os.Stat(stage); !os.IsNotExist(err) {
				t.Fatal("child exit retained stage")
			}
		})
	}
}

func TestCodexPackageProcessHelper(t *testing.T) {
	switch os.Getenv("CURATOR_PACKAGE_TEST") {
	case "host":
		fmt.Print("tool-ready")
		os.Exit(0)
	case "entry":
		self, err := os.Executable()
		if err != nil {
			os.Exit(2)
		}
		cmd := exec.Command(filepath.Join(filepath.Dir(self), "codex-code-mode-host"), "-test.run=^TestCodexPackageProcessHelper$")
		cmd.Env = append(os.Environ(), "CURATOR_PACKAGE_TEST=host")
		out, err := cmd.CombinedOutput()
		fmt.Print(string(out))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				fmt.Print("missing-package-host")
			}
			os.Exit(3)
		}
		if os.Args[len(os.Args)-1] == "7" {
			os.Exit(7)
		}
		os.Exit(0)
	}
}

// The historical single-executable flattening is an applied control plant: a
// valid source package's child must fail specifically at its missing host path.
func TestCodexPackageFlatteningPlant(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	root, entry := codexPackage(t, false, false)
	writePackage(t, root, "codex", data, 0700)
	writePackage(t, root, "codex-code-mode-host", data, 0700)
	s, err := execution.SnapshotArtifact(entry, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cmd := exec.Command(s.Path(), "-test.run=^TestCodexPackageProcessHelper$", "--", "0")
	cmd.Env = append(os.Environ(), "CURATOR_PACKAGE_TEST=entry")
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 || !strings.Contains(string(out), "missing-package-host") || strings.Contains(string(out), "tool-ready") {
		t.Fatalf("plant did not fail at missing host: %s %v", out, err)
	}
}
