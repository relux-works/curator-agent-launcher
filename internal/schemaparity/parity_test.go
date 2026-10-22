package schemaparity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

const schemaModule = "github.com/relux-works/curator-agent-launcher/schemas/session-manager"

// Exclude exactly schemas/session-manager/README.md: its launcher re-pin prose
// may advance independently. Every other file and directory, including other
// READMEs, go.mod, source, tests and vectors, must match the published module.
const parityExclusion = "README.md"

// The checked-out schema tree must match the selected published module without
// using Git history, so this guard also runs in depth-one CI and scratch copies.
func TestSchemaModuleParity(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-mod=readonly", "-m", "-json", schemaModule)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("schema module parity: cannot resolve pinned module (excluding only schemas/session-manager/README.md): %v: %s", err, out)
	}
	var module struct {
		Path, Version, Dir string
		Replace            *json.RawMessage
		Error              *json.RawMessage
	}
	if err := json.Unmarshal(out, &module); err != nil {
		t.Fatalf("schema module parity: cannot decode pinned module (excluding only schemas/session-manager/README.md): %v", err)
	}
	if module.Path != schemaModule || module.Version == "" || module.Dir == "" || module.Replace != nil || module.Error != nil {
		t.Fatal("schema module parity: pinned module must resolve to a downloaded version without a replacement (excluding only schemas/session-manager/README.md)")
	}
	if err := compareTrees(filepath.Join(root, "schemas/session-manager"), module.Dir); err != nil {
		t.Fatal(err)
	}
	t.Logf("schema module parity: %s@%s matches (excluding only schemas/session-manager/README.md)", module.Path, module.Version)
}

type entry struct {
	directory bool
	data      []byte
}

func readTree(root string) (map[string]entry, error) {
	entries := make(map[string]entry)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." || filepath.ToSlash(rel) == parityExclusion {
			return nil
		}
		if d.IsDir() {
			entries[filepath.ToSlash(rel)] = entry{directory: true}
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s: not a regular file", rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		entries[filepath.ToSlash(rel)] = entry{data: data}
		return nil
	})
	return entries, err
}

func compareTrees(checkout, pinned string) error {
	fail := func(format string, args ...any) error {
		return fmt.Errorf("schema module parity: %s (excluding only schemas/session-manager/README.md)", fmt.Sprintf(format, args...))
	}
	local, err := readTree(checkout)
	if err != nil {
		return fail("cannot read checkout: %v", err)
	}
	published, err := readTree(pinned)
	if err != nil {
		return fail("cannot read pinned module: %v", err)
	}
	names := make(map[string]bool)
	for name := range local {
		names[name] = true
	}
	for name := range published {
		names[name] = true
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	for _, name := range ordered {
		a, haveLocal := local[name]
		b, havePublished := published[name]
		switch {
		case !haveLocal:
			return fail("%s: missing from checkout", name)
		case !havePublished:
			return fail("%s: absent from pinned module", name)
		case a.directory != b.directory || !bytes.Equal(a.data, b.data):
			return fail("%s: differs from pinned module", name)
		}
	}
	return nil
}

// A matching tree and the one prose exclusion pass. Contract/source changes,
// additions/removals, nested README edits and missing inputs fail specifically.
func TestCompareTrees(t *testing.T) {
	for _, tc := range []struct {
		name, path, data, want string
		remove, directory      bool
	}{
		{name: "unchanged"},
		{name: "root-readme", path: "README.md", data: "new re-pin instructions"},
		{name: "vector", path: "vectors/example.json", data: "[null]", want: "vectors/example.json: differs"},
		{name: "source", path: "validate.go", data: "package changed", want: "validate.go: differs"},
		{name: "go-mod", path: "go.mod", data: "module changed", want: "go.mod: differs"},
		{name: "nested-readme", path: "vectors/README.md", data: "changed", want: "vectors/README.md: differs"},
		{name: "added", path: "extra.json", data: "{}", want: "extra.json: absent from pinned module"},
		{name: "removed", path: "validate.go", remove: true, want: "validate.go: missing from checkout"},
		{name: "empty-directory", path: "extra", directory: true, want: "extra: absent from pinned module"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local, published := t.TempDir(), t.TempDir()
			for _, root := range []string{local, published} {
				for name, data := range map[string]string{"go.mod": "module schema", "validate.go": "package schema", "vectors/example.json": "[]", "vectors/README.md": "vector notes", "README.md": "module notes"} {
					writeFixture(t, root, name, data)
				}
			}
			if tc.path != "" {
				path := filepath.Join(local, tc.path)
				switch {
				case tc.remove:
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				case tc.directory:
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				default:
					writeFixture(t, local, tc.path, tc.data)
				}
			}
			err := compareTrees(local, published)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "excluding only schemas/session-manager/README.md") {
				t.Fatalf("got %v, want %q and explicit exclusion", err, tc.want)
			}
		})
	}
	t.Run("missing-pinned-tree", func(t *testing.T) {
		err := compareTrees(t.TempDir(), filepath.Join(t.TempDir(), "absent"))
		if err == nil || !strings.Contains(err.Error(), "cannot read pinned module") {
			t.Fatalf("got %v", err)
		}
	})
}

func writeFixture(t *testing.T, root, name, data string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}
