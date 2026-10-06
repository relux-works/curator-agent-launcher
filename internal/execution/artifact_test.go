package execution_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/execution"
)

func TestArtifactSnapshotStagesExactBytesAndCleansUp(t *testing.T) {
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "provider")
	if err := os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := execution.SnapshotArtifact("provider", dir, []string{"PATH=" + dir})
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	if snapshot.BuildID() != fmt.Sprintf("sha256-%x", sha256.Sum256(data)) || !bytes.Equal(snapshot.Bytes(), data) {
		t.Fatal("snapshot identity does not name input bytes")
	}
	if err := os.WriteFile(path, []byte("replacement"), 0700); err != nil {
		t.Fatal(err)
	}
	staged, err := os.ReadFile(snapshot.Path())
	if err != nil || !bytes.Equal(staged, data) {
		t.Fatal("source replacement changed staged bytes")
	}
	info, err := os.Stat(snapshot.Path())
	if err != nil || info.Mode().Perm() != 0500 {
		t.Fatal("staged artifact is writable")
	}
	info, err = os.Stat(filepath.Dir(snapshot.Path()))
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("stage is not private")
	}
	snapshot.Close()
	if _, err := os.Stat(snapshot.Path()); !os.IsNotExist(err) {
		t.Fatal("stage was retained after close")
	}
}
func TestArtifactSnapshotMissingRefuses(t *testing.T) {
	if _, err := execution.SnapshotArtifact(filepath.Join(t.TempDir(), "missing"), t.TempDir(), nil); err == nil {
		t.Fatal("missing artifact was accepted")
	}
}
