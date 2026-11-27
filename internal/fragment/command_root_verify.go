package fragment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

const (
	commandRootReceiptName   = ".curator-profile-command-root.json"
	commandRootReceiptSchema = "profile-command-root/1"
	commandRootMarkerName    = ".curator-command-generation.json"
)

type commandRootReceipt struct {
	Schema          string `json:"schema"`
	Profile         string `json:"profile"`
	LockSHA256      string `json:"lock_sha256"`
	PlanSHA256      string `json:"plan_sha256"`
	Generation      string `json:"generation"`
	Platform        string `json:"platform"`
	Root            string `json:"root"`
	InventorySHA256 string `json:"inventory_sha256"`
	Commands        []struct {
		Name   string `json:"name"`
		SHA256 string `json:"sha256"`
	} `json:"commands"`
}

// VerifyCommandRoot independently validates a launch-env-fragment-v9 profile
// command root on disk before the launcher prepends PathPrepend to child PATH.
// Fragments without CommandRoot (v1-v7) return nil immediately and keep
// PathPrepend inert.
func VerifyCommandRoot(f *Fragment) error {
	if f == nil || f.CommandRoot == nil {
		return nil
	}
	cr := f.CommandRoot
	if f.Revision != IdentityV9 || f.PathPrepend == "" {
		return fmt.Errorf("command_root requires %s and non-empty path_prepend", IdentityV9)
	}
	hostPlatform := runtime.GOOS + "-" + runtime.GOARCH
	if cr.Platform != hostPlatform {
		return fmt.Errorf("command_root platform %q does not match host %q", cr.Platform, hostPlatform)
	}
	root := filepath.Clean(f.PathPrepend)
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("stat command root %q: %w", root, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("command root %q is not a real directory", root)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("command root %q must not be group/world writable (mode %o)", root, info.Mode().Perm())
	}

	receiptPath := filepath.Join(root, commandRootReceiptName)
	receiptBytes, err := os.ReadFile(receiptPath)
	if err != nil {
		return fmt.Errorf("read command root receipt: %w", err)
	}
	rSum := sha256.Sum256(receiptBytes)
	if hex.EncodeToString(rSum[:]) != cr.ReceiptSHA256 {
		return fmt.Errorf("command root receipt sha256 mismatch")
	}
	var receipt commandRootReceipt
	if err := json.Unmarshal(receiptBytes, &receipt); err != nil {
		return fmt.Errorf("parse command root receipt: %w", err)
	}
	if receipt.Schema != commandRootReceiptSchema ||
		receipt.Profile != f.Profile.Name ||
		receipt.LockSHA256 != cr.LockSHA256 ||
		receipt.PlanSHA256 != cr.PlanSHA256 ||
		receipt.Generation != cr.Generation ||
		receipt.Platform != cr.Platform ||
		receipt.InventorySHA256 != cr.InventorySHA256 ||
		filepath.Clean(receipt.Root) != root {
		return fmt.Errorf("command root receipt fields disagree with fragment")
	}

	markerPath := filepath.Join(filepath.Dir(root), commandRootMarkerName)
	markerBytes, err := os.ReadFile(markerPath)
	if err != nil {
		return fmt.Errorf("read command generation marker: %w", err)
	}
	mSum := sha256.Sum256(markerBytes)
	if hex.EncodeToString(mSum[:]) != cr.MarkerSHA256 {
		return fmt.Errorf("command generation marker sha256 mismatch")
	}

	declared := make(map[string]string, len(receipt.Commands))
	for _, cmd := range receipt.Commands {
		declared[cmd.Name] = cmd.SHA256
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("list command root %q: %w", root, err)
	}
	verified := 0
	for _, entry := range entries {
		name := entry.Name()
		if name == commandRootReceiptName {
			continue
		}
		wantSHA, ok := declared[name]
		if !ok {
			return fmt.Errorf("undeclared entry %q in command root", name)
		}
		p := filepath.Join(root, name)
		fi, err := os.Lstat(p)
		if err != nil || !fi.Mode().IsRegular() || fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("command %q is not a regular file", name)
		}
		if runtime.GOOS != "windows" && fi.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("command %q has group/world write bits", name)
		}
		gotSHA, err := fileSHA256(p)
		if err != nil {
			return fmt.Errorf("hash command %q: %w", name, err)
		}
		if gotSHA != wantSHA {
			return fmt.Errorf("command %q sha256 mismatch", name)
		}
		verified++
	}
	if verified != len(declared) {
		return fmt.Errorf("command root missing declared commands (%d of %d present)", verified, len(declared))
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
