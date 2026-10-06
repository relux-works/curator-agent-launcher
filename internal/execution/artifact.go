package execution

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
)

// ArtifactSnapshot owns private staging of exactly the bytes passed to Evaluate.
// The stage has no inherited writable descriptor and is removed after Run exits.
// Qualification and pin policy remain entirely in adapterprobe.
type ArtifactSnapshot struct {
	dir, path, build string
	data             []byte
}

func (s *ArtifactSnapshot) Path() string    { return s.path }
func (s *ArtifactSnapshot) BuildID() string { return s.build }
func (s *ArtifactSnapshot) Bytes() []byte   { return s.data }
func (s *ArtifactSnapshot) Close()          { _ = os.RemoveAll(s.dir) }

func SnapshotArtifact(binary, workdir string, env []string) (*ArtifactSnapshot, error) {
	path, err := executable(binary, workdir, env)
	if err != nil {
		return nil, &adapterprobe.Unsupported{Reason: "artifact_snapshot_unreadable"}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, &adapterprobe.Unsupported{Reason: "artifact_snapshot_unreadable"}
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (256<<20)+1))
	if err != nil {
		return nil, &adapterprobe.Unsupported{Reason: "artifact_snapshot_unreadable"}
	}
	if len(data) > 256<<20 {
		return nil, &adapterprobe.Unsupported{Reason: "artifact_too_large"}
	}
	dir, err := os.MkdirTemp("", "curator-adapter-")
	if err != nil {
		return nil, &adapterprobe.Unsupported{Reason: "artifact_staging_unavailable"}
	}
	stage := filepath.Join(dir, filepath.Base(path))
	if err := os.WriteFile(stage, data, 0500); err != nil {
		_ = os.RemoveAll(dir)
		return nil, &adapterprobe.Unsupported{Reason: "artifact_staging_unavailable"}
	}
	build, err := adapterprobe.BuildID(fmt.Sprintf("%x", sha256.Sum256(data)))
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return &ArtifactSnapshot{dir: dir, path: stage, build: build, data: data}, nil
}
