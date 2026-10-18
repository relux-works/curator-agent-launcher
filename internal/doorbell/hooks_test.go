//go:build unix

package doorbell

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Absolute PATH entries outside both workspace spellings and git locations are
// eligible; relative and repository candidates never execute. The first resolved
// path remains frozen even when the caller changes the environment afterwards.
func TestTrustedResolution(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	trusted := filepath.Join(root, "trusted")
	gitDir := filepath.Join(root, "git-dir")
	for _, dir := range []string{workspace, trusted, gitDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "a2a"), []byte("fixture"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(workspace)
	link := filepath.Join(root, "link")
	if err := os.Symlink(workspace, link); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		name, path, workTree, git string
		want                      bool
	}{
		{"trusted", trusted, "", "", true},
		{"lexical", workspace, "", "", false},
		{"physical", link, "", "", false},
		{"relative", "../trusted", "", "", false},
		{"empty", "", "", "", false},
		{"git-dir", gitDir, "", gitDir, false},
		{"work-tree", gitDir, gitDir, "", false},
		{"trusted-after-repo", workspace + string(os.PathListSeparator) + trusted, "", "", true},
	} {
		t.Run(row.name, func(t *testing.T) {
			var diagnostics bytes.Buffer
			installer := Installer{Environment: []string{"PATH=" + row.path, "GIT_WORK_TREE=" + row.workTree, "GIT_DIR=" + row.git}, InvocationDirectory: workspace, Diagnostics: &diagnostics}
			got, err := installer.Resolve(workspace)
			if (err == nil) != row.want {
				t.Fatalf("trust verdict=%v want=%v", err == nil, row.want)
			}
			if row.want {
				physical, _ := filepath.EvalSymlinks(filepath.Join(trusted, "a2a"))
				if got != physical {
					t.Fatal("wrong executable", got)
				}
				installer.Environment = []string{"PATH=" + workspace}
				again, err := installer.Resolve(workspace)
				if err != nil || again != got || strings.Count(diagnostics.String(), "a2a executable=") != 1 {
					t.Fatal("resolution was not frozen", again, err, diagnostics.String())
				}
				if _, err := installer.Resolve(trusted); err == nil {
					t.Fatal("frozen helper entered another workspace")
				}
			}
		})
	}
}
