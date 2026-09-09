package axconfig_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/axconfig"
	"github.com/relux-works/curator-agent-launcher/internal/cli"
)

func put(t *testing.T, dir, data string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ax.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

const yes = `{"schema":"curator-run-ax-v1","enabled":true}`
const no = `{"schema":"curator-run-ax-v1","enabled":false}`

func TestLoadPrecedenceBeforeParse(t *testing.T) {
	root := t.TempDir()
	machine := filepath.Join(root, "machine")
	operator := filepath.Join(root, "operator")
	enabled, err := axconfig.Load(machine, operator)
	if err != nil || enabled {
		t.Fatalf("absent: %v %v", enabled, err)
	}
	put(t, operator, yes)
	enabled, err = axconfig.Load(machine, operator)
	if err != nil || !enabled {
		t.Fatalf("operator: %v %v", enabled, err)
	}
	inv, err := cli.Parse([]string{"codex_cli", "--ax-profile", "yolo"}, cli.Options{AxConfigured: enabled})
	if err != nil || !inv.Tracked {
		t.Fatal(inv, err)
	}
	put(t, machine, no)
	put(t, operator, "broken")
	enabled, err = axconfig.Load(machine, operator)
	if err != nil || enabled {
		t.Fatalf("machine false ignores invalid operator: %v %v", enabled, err)
	}
	if _, err = cli.Parse([]string{"codex_cli", "--ax-profile", "yolo"}, cli.Options{AxConfigured: enabled}); !cli.IsUsage(err) {
		t.Fatal(err)
	}
	// The ignored operator isn't even traversed: a dangling directory is harmless.
	ignored := filepath.Join(root, "ignored")
	if err = os.Symlink(filepath.Join(root, "missing"), ignored); err != nil {
		t.Fatal(err)
	}
	put(t, machine, yes)
	enabled, err = axconfig.Load(machine, ignored)
	if err != nil || !enabled {
		t.Fatal(enabled, err)
	}
}
func TestLoadClosedSchema(t *testing.T) {
	cases := map[string]string{
		"unknown":          `{"schema":"curator-run-ax-v1","enabled":true,"locked":false}`,
		"duplicate":        `{"schema":"curator-run-ax-v1","enabled":false,"enabled":true}`,
		"schema_duplicate": `{"schema":"bad","schema":"curator-run-ax-v1","enabled":true}`,
		"missing_enabled":  `{"schema":"curator-run-ax-v1"}`,
		"missing_schema":   `{"enabled":true}`, "bad_schema": `{"schema":"other","enabled":true}`,
		"schema_type": `{"schema":true,"enabled":true}`,
		"string":      `{"schema":"curator-run-ax-v1","enabled":"true"}`,
		"null":        `{"schema":"curator-run-ax-v1","enabled":null}`,
		"number":      `{"schema":"curator-run-ax-v1","enabled":1}`,
		"array":       `[]`, "trailing": yes + `{}`, "malformed": `{`,
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			m := filepath.Join(root, "m")
			o := filepath.Join(root, "o")
			put(t, m, data)
			put(t, o, yes)
			_, err := axconfig.Load(m, o)
			var invalid *axconfig.Error
			if !errors.As(err, &invalid) || invalid.Path != filepath.Join(m, "ax.json") {
				t.Fatalf("admitted invalid machine: %v", err)
			}
			if err = os.Remove(filepath.Join(m, "ax.json")); err != nil {
				t.Fatal(err)
			}
			put(t, o, data)
			if _, err = axconfig.Load(m, o); !errors.As(err, &invalid) {
				t.Fatalf("admitted invalid operator: %v", err)
			}
		})
	}
}
func TestLoadFilesystemFailures(t *testing.T) {
	for _, kind := range []string{"directory", "fifo", "dangling_file", "dangling_ancestor", "file_ancestor", "unreadable_file", "unreadable_ancestor"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			m := filepath.Join(root, "m")
			o := filepath.Join(root, "o")
			put(t, o, yes)
			put(t, m, yes)
			path := filepath.Join(m, "ax.json")
			switch kind {
			case "directory":
				os.Remove(path)
				os.Mkdir(path, 0700)
			case "fifo":
				os.Remove(path)
				makeFIFO(t, path)
			case "dangling_file":
				os.Remove(path)
				os.Symlink(filepath.Join(root, "missing"), path)
			case "dangling_ancestor":
				os.Remove(path)
				os.Remove(m)
				os.Symlink(filepath.Join(root, "missing"), m)
			case "file_ancestor":
				os.Remove(path)
				os.Remove(m)
				os.WriteFile(m, []byte("file"), 0600)
			case "unreadable_file":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permission refusal")
				}
				os.Chmod(path, 0000)
				defer os.Chmod(path, 0600)
			case "unreadable_ancestor":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permission refusal")
				}
				os.Chmod(m, 0000)
				defer os.Chmod(m, 0700)
			}
			_, err := axconfig.Load(m, o)
			var invalid *axconfig.Error
			if !errors.As(err, &invalid) {
				t.Fatalf("failure became fallback: %v", err)
			}
		})
	}
}
