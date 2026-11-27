package fragment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseAndVerifyV9CommandRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission verification test")
	}
	envRoot := filepath.Join(t.TempDir(), "environments")
	lockHex := strings.Repeat("a", 64)
	planHex := strings.Repeat("b", 64)
	genHex := strings.Repeat("c", 64)
	platform := runtime.GOOS + "-" + runtime.GOARCH

	managedHome := filepath.Join(envRoot, "colab-g4-keeper", "claude_code")
	platformDir := filepath.Join(envRoot, "colab-g4-keeper", "commands", genHex, platform)
	binDir := filepath.Join(platformDir, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}

	cmdBody := []byte("#!/bin/sh\necho ok\n")
	cmdPath := filepath.Join(binDir, "colab-glm53")
	if err := os.WriteFile(cmdPath, cmdBody, 0o500); err != nil {
		t.Fatal(err)
	}
	cmdSum := sha256.Sum256(cmdBody)
	cmdHex := hex.EncodeToString(cmdSum[:])

	invBytes, _ := json.Marshal([]map[string]any{{"name": "colab-glm53", "sha256": cmdHex}})
	invSum := sha256.Sum256(invBytes)
	invHex := hex.EncodeToString(invSum[:])

	receiptBytes, _ := json.Marshal(map[string]any{
		"schema":           "profile-command-root/1",
		"profile":          "colab-g4-keeper",
		"lock_sha256":      lockHex,
		"plan_sha256":      planHex,
		"generation":       genHex,
		"platform":         platform,
		"root":             binDir,
		"inventory_sha256": invHex,
		"commands":         []map[string]any{{"name": "colab-glm53", "sha256": cmdHex}},
	})
	if err := os.WriteFile(filepath.Join(binDir, ".curator-profile-command-root.json"), receiptBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	rSum := sha256.Sum256(receiptBytes)
	rHex := hex.EncodeToString(rSum[:])

	markerBytes := []byte(`{"schema":"profile-command-generation/1"}`)
	if err := os.WriteFile(filepath.Join(platformDir, ".curator-command-generation.json"), markerBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	mSum := sha256.Sum256(markerBytes)
	mHex := hex.EncodeToString(mSum[:])

	raw, _ := json.Marshal(map[string]any{
		"fragment":    IdentityV9,
		"environment": EnvClaudeCode,
		"profile":     map[string]any{"name": "colab-g4-keeper", "lock_sha256": lockHex},
		"precedence":  map[string]any{"winner": "higher-weight", "placement": "winner-last"},
		"permissions": map[string]any{"mode": "native", "locked": false, "source": "default"},
		"env":         map[string]any{"CLAUDE_CONFIG_DIR": managedHome},
		"command_root": map[string]any{
			"lock_sha256":      lockHex,
			"plan_sha256":      planHex,
			"generation":       genHex,
			"platform":         platform,
			"receipt_sha256":   rHex,
			"marker_sha256":    mHex,
			"inventory_sha256": invHex,
		},
		"path_prepend": binDir,
	})

	frag, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse v9 failed: %v", err)
	}
	if frag.Revision != IdentityV9 || frag.CommandRoot == nil || frag.PathPrepend != binDir {
		t.Fatalf("unexpected v9 fragment: %+v", frag)
	}
	if err := VerifyCommandRoot(frag); err != nil {
		t.Fatalf("VerifyCommandRoot failed: %v", err)
	}

	// Unlisted file in binDir must fail closed.
	extra := filepath.Join(binDir, "rogue")
	if err := os.WriteFile(extra, []byte("evil"), 0o500); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCommandRoot(frag); err == nil || !strings.Contains(err.Error(), "undeclared entry") {
		t.Fatalf("expected undeclared entry refusal, got %v", err)
	}
	_ = os.Remove(extra)

	// Tampered command bytes must fail closed.
	_ = os.Chmod(cmdPath, 0o700)
	if err := os.WriteFile(cmdPath, []byte("tampered"), 0o500); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCommandRoot(frag); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("expected sha256 mismatch refusal, got %v", err)
	}
}
