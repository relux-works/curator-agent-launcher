package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This checks the dependency pin separately from launch behavior; a
// replacement can otherwise pass every behavioral regression.
func TestConstructionDependencyPin(t *testing.T) {
	for _, name := range []string{"go.work", "go.work.sum"} {
		if _, err := os.Stat(filepath.Join("..", "..", name)); !os.IsNotExist(err) {
			t.Fatalf("release candidate must have no %s: %v", name, err)
		}
	}
	cmd := exec.Command("go", "list", "-m", "-json", "github.com/relux-works/archon-pantheon")
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GOWORK=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("read release module identity: %v", err)
	}
	var module struct {
		Version string
		Replace *json.RawMessage
	}
	if err := json.Unmarshal(out, &module); err != nil {
		t.Fatal(err)
	}
	if module.Version != "v0.8.0" || module.Replace != nil {
		t.Fatalf("module pin must be v0.8.0 without replacement: version=%s replaced=%v", module.Version, module.Replace != nil)
	}
}
