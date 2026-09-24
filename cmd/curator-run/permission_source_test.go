package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModuleSourcesDoNotSpellProviderPermissionBypassFlags(t *testing.T) {
	var sourcePaths []string
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path == ".git" || path == ".temp" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".go" && !strings.HasSuffix(path, "_test.go") {
			sourcePaths = append(sourcePaths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{"--dangerously", "skip-permissions", "bypassPermissions", "--full-auto", "--approval", "--sandbox"}
	var findings []string
	yoloOutsideCLI := []string{}
	for _, path := range sourcePaths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, spelling := range forbidden {
			if strings.Contains(text, spelling) {
				findings = append(findings, path+": "+spelling)
			}
		}
		if strings.Contains(text, "--yolo") && filepath.Clean(path) != filepath.Join("internal", "cli", "cli.go") {
			yoloOutsideCLI = append(yoloOutsideCLI, path)
		}
	}
	if len(findings) != 0 {
		t.Fatalf("provider bypass spelling found in launcher source: %s", strings.Join(findings, ", "))
	}
	if len(yoloOutsideCLI) != 0 {
		t.Fatalf("launcher alias spelling escaped the CLI parser: %s", strings.Join(yoloOutsideCLI, ", "))
	}
}
