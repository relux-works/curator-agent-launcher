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
	"strings"
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

// A real Go import of the launcher internal path must trip the same guard.
// A tiny local launcher replacement isolates this dependency-control plant
// from network availability and the actual launcher's provider dependencies.
func TestDependencyGuardRejectsLauncherImport(t *testing.T) {
	fixture := copyModule(t)
	if err := checkDependencies(t, fixture); err != nil {
		t.Fatalf("unplanted control: %v", err)
	}
	launcher := filepath.Join(t.TempDir(), "launcher")
	if err := os.MkdirAll(filepath.Join(launcher, "internal", "fragment"), 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(name, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(launcher, "go.mod"), "module github.com/relux-works/curator-agent-launcher\n\ngo 1.25.5\n")
	write(filepath.Join(launcher, "internal", "fragment", "fragment.go"), "package fragment\n")
	write(filepath.Join(fixture, "dependency_plant.go"), "package managerschema\nimport _ \"github.com/relux-works/curator-agent-launcher/internal/fragment\"\n")
	data, err := os.ReadFile(filepath.Join(fixture, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(fixture, "go.mod"), string(data)+"\nrequire github.com/relux-works/curator-agent-launcher v0.0.0\nreplace github.com/relux-works/curator-agent-launcher => "+fmt.Sprintf("%q", filepath.ToSlash(launcher))+"\n")
	err = checkDependencies(t, fixture)
	if err == nil || err.Error() != "non-standard dependency: github.com/relux-works/curator-agent-launcher/internal/fragment" {
		t.Fatalf("plant must reach the dependency violation, got %v", err)
	}
}

func copyModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	err := filepath.WalkDir(".", func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if name != "." && name != "vectors" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dir, name), 0700)
		}
		if name != "go.mod" && !strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, ".json") {
			return nil
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, name), data, 0600)
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
