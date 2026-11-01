package claudeauth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
)

func fileSelection(path string) launcherconfig.ClaudeAuth {
	return launcherconfig.ClaudeAuth{Type: "oauth", Store: launcherconfig.AuthStore{Type: "file", Path: path}}
}
func keychainSelection() launcherconfig.ClaudeAuth {
	return launcherconfig.ClaudeAuth{Type: "oauth", Store: launcherconfig.AuthStore{Type: "keychain", Service: "fixture-service", Account: "fixture-account"}}
}
func fixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private-token")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A valid private file replaces ambient OAuth only in a new child env, while
// preserving the source bytes/mode and caller input; this does not test vendor auth.
func TestPrivateFileChildOnly(t *testing.T) {
	path := fixture(t, "synthetic-oauth-fixture\r\n")
	env := []string{"KEEP=yes", Variable + "=ambient", Variable + "=duplicate"}
	before := append([]string{}, env...)
	child, err := (Resolver{}).Inject(context.Background(), fileSelection(path), env)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(child, []string{"KEEP=yes", Variable + "=synthetic-oauth-fixture"}) || !reflect.DeepEqual(env, before) {
		t.Fatal("child selection or input preservation failed")
	}
	raw, _ := os.ReadFile(path)
	info, _ := os.Stat(path)
	if string(raw) != "synthetic-oauth-fixture\r\n" || info.Mode().Perm() != 0600 {
		t.Fatal("source changed")
	}
	child[0] = "changed"
	if !reflect.DeepEqual(env, before) {
		t.Fatal("child aliases parent")
	}
}

// Missing, broad-mode, special-mode, symlink, hardlink, directory, empty,
// oversized and multiline sources all refuse without returning partial env or fallback.
func TestFileRefusals(t *testing.T) {
	for _, kind := range []string{"missing", "mode0640", "mode0400", "sticky", "symlink", "hardlink", "directory", "empty", "oversize", "newline", "space", "nul", "invalid-utf8", "bare-cr"} {
		t.Run(kind, func(t *testing.T) {
			path := fixture(t, "synthetic-valid")
			switch kind {
			case "missing":
				os.Remove(path)
			case "mode0640":
				os.Chmod(path, 0640)
			case "mode0400":
				os.Chmod(path, 0400)
			case "sticky":
				os.Chmod(path, 0600|os.ModeSticky)
			case "symlink":
				link := path + "-link"
				if err := os.Symlink(path, link); err != nil {
					t.Fatal(err)
				}
				path = link
			case "hardlink":
				if err := os.Link(path, path+"-link"); err != nil {
					t.Fatal(err)
				}
			case "directory":
				path = filepath.Dir(path)
			case "empty":
				os.WriteFile(path, nil, 0600)
			case "oversize":
				os.WriteFile(path, []byte(strings.Repeat("x", maxBytes+1)), 0600)
			case "newline":
				os.WriteFile(path, []byte("line1\nline2"), 0600)
			case "space":
				os.WriteFile(path, []byte("token with spaces"), 0600)
			case "nul":
				os.WriteFile(path, []byte{'x', 0}, 0600)
			case "invalid-utf8":
				os.WriteFile(path, []byte{255}, 0600)
			case "bare-cr":
				os.WriteFile(path, []byte("token\r"), 0600)
			}
			child, err := (Resolver{}).Inject(context.Background(), fileSelection(path), []string{Variable + "=ambient-fallback"})
			if err != ErrRefused || child != nil {
				t.Fatal("unsafe source did not fail closed")
			}
			if strings.Contains(err.Error(), path) || strings.Contains(err.Error(), "ambient-fallback") {
				t.Fatal("diagnostic leaks source")
			}
		})
	}
}

// The synthetic Keychain seam proves explicit metadata forwarding, bounded token
// validation and sanitization of backend errors. No real Keychain helper runs.
func TestKeychainSelectionAndRefusal(t *testing.T) {
	for _, kind := range []string{"valid", "backend-error", "empty", "oversize", "cancel-after-read"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := Resolver{Keychain: func(ctx context.Context, s, a string) ([]byte, error) {
				calls++
				if s != "fixture-service" || a != "fixture-account" {
					t.Fatal("wrong lookup")
				}
				switch kind {
				case "backend-error":
					return []byte("private-backend-token"), errors.New("private-backend-token private-store-label")
				case "empty":
					return nil, nil
				case "oversize":
					return []byte(strings.Repeat("x", maxBytes+1)), nil
				case "cancel-after-read":
					cancel()
				}
				return []byte("synthetic-keychain-token\n"), nil
			}}
			child, err := r.Inject(ctx, keychainSelection(), nil)
			if calls != 1 {
				t.Fatal("lookup count")
			}
			if kind == "valid" {
				if err != nil || !reflect.DeepEqual(child, []string{Variable + "=synthetic-keychain-token"}) {
					t.Fatal("valid source refused")
				}
				return
			}
			if child != nil || err != ErrRefused {
				t.Fatal("invalid source accepted")
			}
			if strings.Contains(err.Error(), "private-") {
				t.Fatal("backend detail escaped")
			}
		})
	}
}

// Invalid metadata, cancellation and conflicting credentials refuse before lookup;
// an ordinary env is the nearby valid control. No source fallback is permitted.
func TestNoLookupBeforeAdmission(t *testing.T) {
	for _, kind := range []string{"invalid-type", "mixed-store", "cancelled", "api-key", "auth-token", "bedrock", "vertex", "foundry"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			r := Resolver{Keychain: func(context.Context, string, string) ([]byte, error) { calls++; return []byte("synthetic"), nil }}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			selected := keychainSelection()
			var env []string
			switch kind {
			case "invalid-type":
				selected.Type = "inline"
			case "mixed-store":
				selected.Store.Path = "/fixture"
			case "cancelled":
				cancel()
			case "api-key":
				env = []string{"ANTHROPIC_API_KEY=synthetic"}
			case "auth-token":
				env = []string{"ANTHROPIC_AUTH_TOKEN=synthetic"}
			case "bedrock":
				env = []string{"CLAUDE_CODE_USE_BEDROCK=1"}
			case "vertex":
				env = []string{"CLAUDE_CODE_USE_VERTEX=1"}
			case "foundry":
				env = []string{"CLAUDE_CODE_USE_FOUNDRY=1"}
			}
			child, err := r.Inject(ctx, selected, env)
			if calls != 0 || child != nil || err != ErrRefused {
				t.Fatal("precondition performed lookup or accepted")
			}
		})
	}
}
