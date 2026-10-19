//go:build unix

package managerschema

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

const modulePath = "github.com/relux-works/curator-agent-launcher/schemas/session-manager"

// The complete production and test import closure must contain only this
// module and standard-library packages, independent of the parent workspace.
func TestStandardLibraryDependencies(t *testing.T) {
	fixture := copyModule(t)
	if err := checkDependencies(t, fixture); err != nil {
		t.Fatal(err)
	}
}

// A real Go import of the launcher internal path must trip the same guard,
// whether it sits in the root package or in a subpackage. The plant goes into
// a source tree *before* it is copied, so the subpackage case also proves the
// copy keeps every package. A copy that dropped subdirectories would lose the
// plant and let it pass. A tiny local launcher replacement isolates the plant
// from network availability and the real launcher's provider dependencies.
func TestDependencyGuardRejectsLauncherImport(t *testing.T) {
	for _, tc := range []struct{ name, dir, pkg string }{
		{"root package", ".", "managerschema"},
		{"subpackage", "extra", "extra"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := checkDependencies(t, copyModule(t)); err != nil {
				t.Fatalf("unplanted control: %v", err)
			}
			write := func(name, body string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(name, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			launcher := filepath.Join(t.TempDir(), "launcher")
			write(filepath.Join(launcher, "go.mod"), "module github.com/relux-works/curator-agent-launcher\n\ngo 1.25.5\n")
			write(filepath.Join(launcher, "internal", "fragment", "fragment.go"), "package fragment\n")
			source := copyModule(t)
			write(filepath.Join(source, tc.dir, "dependency_plant.go"), "package "+tc.pkg+"\nimport _ \"github.com/relux-works/curator-agent-launcher/internal/fragment\"\n")
			data, err := os.ReadFile(filepath.Join(source, "go.mod"))
			if err != nil {
				t.Fatal(err)
			}
			write(filepath.Join(source, "go.mod"), string(data)+"\nrequire github.com/relux-works/curator-agent-launcher v0.0.0\nreplace github.com/relux-works/curator-agent-launcher => "+fmt.Sprintf("%q", filepath.ToSlash(launcher))+"\n")
			err = checkDependencies(t, copyModuleFrom(t, source))
			if err == nil || err.Error() != "non-standard dependency: github.com/relux-works/curator-agent-launcher/internal/fragment" {
				t.Fatalf("plant must reach the dependency violation, got %v", err)
			}
		})
	}
}

// copyModule copies the whole module tree, so that `go list ./...` in the copy
// sees every package the real module has. A symlink or other special file
// fails the test instead of being skipped.
func copyModule(t *testing.T) string {
	t.Helper()
	return copyModuleFrom(t, ".")
}

// copyModuleFrom copies the module tree rooted at src.
func copyModuleFrom(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	err := filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, name)
		switch {
		case entry.IsDir():
			return os.MkdirAll(target, 0700)
		case entry.Type().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(target, data, 0600)
		default:
			return fmt.Errorf("module copy: unsupported file type at %s", name)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func checkDependencies(t *testing.T, dir string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-test", "-e", "-json", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=readonly", "GOWORK=off", "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off", "GOTELEMETRY=off")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		// Only the process group created by this test is ever addressed.
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		if err := syscall.Kill(-pid, 0); !errors.Is(err, syscall.ESRCH) {
			t.Errorf("go-list process group survived: %v", err)
		}
	})
	err := cmd.Wait()
	if err != nil || ctx.Err() != nil {
		t.Fatalf("go list did not complete: %v, %v, %s", err, ctx.Err(), stderr.String())
	}
	if !cmd.ProcessState.Exited() {
		t.Fatal("go list child did not exit")
	}
	decoder := json.NewDecoder(&stdout)
	count := 0
	for {
		var pkg struct {
			ImportPath string
			Standard   bool
			Module     *struct{ Path string }
			Error      *struct{ Err string }
		}
		err := decoder.Decode(&pkg)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		count++
		if !pkg.Standard && (pkg.Module == nil || pkg.Module.Path != modulePath) {
			return fmt.Errorf("non-standard dependency: %s", pkg.ImportPath)
		}
		if pkg.Error != nil {
			t.Fatalf("go list package error: %s", pkg.Error.Err)
		}
	}
	if count == 0 {
		t.Fatal("go list returned no packages")
	}
	return nil
}
