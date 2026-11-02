package execution_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
)

// Large sparse inputs prove the observed-size and inclusive-limit full-byte
// snapshot/stage identity, source-replacement isolation and cleanup. The
// fixtures are synthetic native containers; none is executed or qualified.
func TestArtifactSnapshotLargeNativeBoundaries(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TMPDIR", root)
	for _, tc := range []struct {
		name   string
		size   int64
		digest string
	}{
		{"observed-size", 310114176, "9066a0e515b89d9d390693da6c689392eccbc756f9a9796aff7186faf6507530"},
		{"inclusive-limit", 512 << 20, "ab762f62190191cf7b4e1237308fcab666748de39aeaccbdfb3349f450359f4d"},
		{"above-limit", (512 << 20) + 1, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, "provider")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0700)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.Truncate(tc.size); err != nil {
				_ = f.Close()
				t.Fatal(err)
			}
			if _, err := f.WriteAt([]byte{0xcf, 0xfa, 0xed, 0xfe}, 0); err != nil {
				_ = f.Close()
				t.Fatal(err)
			}
			if _, err := f.WriteAt([]byte{0xa5}, tc.size-1); err != nil {
				_ = f.Close()
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			snapshot, err := execution.SnapshotArtifact(path, root, nil)
			if tc.digest == "" {
				// Oversize input must refuse specifically before creating a
				// stage, rather than accepting the bounded read's prefix.
				var unsupported *adapterprobe.Unsupported
				if snapshot != nil || !errors.Is(err, adapterprobe.ErrUnsupported) || !errors.As(err, &unsupported) || unsupported.Reason != "artifact_too_large" {
					t.Fatalf("oversize verdict: snapshot=%v error=%v", snapshot, err)
				}
			} else {
				if err != nil {
					t.Fatalf("valid native snapshot of %d bytes refused: %v", tc.size, err)
				}
				defer snapshot.Close()
				if int64(len(snapshot.Bytes())) != tc.size || fmt.Sprintf("%x", sha256.Sum256(snapshot.Bytes())) != tc.digest || snapshot.BuildID() != "sha256-"+tc.digest {
					t.Fatal("snapshot does not identify every source byte")
				}
				if err := os.WriteFile(path, []byte("replacement"), 0700); err != nil {
					t.Fatal(err)
				}
				stage, err := os.Open(snapshot.Path())
				if err != nil {
					t.Fatal(err)
				}
				hash := sha256.New()
				n, copyErr := io.Copy(hash, stage)
				closeErr := stage.Close()
				if copyErr != nil || closeErr != nil || n != tc.size || fmt.Sprintf("%x", hash.Sum(nil)) != tc.digest {
					t.Fatalf("staged full-byte identity changed: bytes=%d copy=%v close=%v", n, copyErr, closeErr)
				}
				stageDir := filepath.Dir(snapshot.Path())
				snapshot.Close()
				if _, err := os.Stat(stageDir); !os.IsNotExist(err) {
					t.Fatalf("stage retained after Close: %v", err)
				}
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "curator-adapter-") {
					t.Fatalf("partial stage retained: %s", entry.Name())
				}
			}
		})
	}
}
