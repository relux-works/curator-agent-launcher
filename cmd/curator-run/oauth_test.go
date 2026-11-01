package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/claudeauth"
	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
)

func selectOAuth(t *testing.T, f *pipelineFixture, store launcherconfig.AuthStore) {
	t.Helper()
	f.deps.readSelectedConfig = func(context.Context) (launcherconfig.Selected, error) {
		raw, err := json.Marshal(map[string]any{"schema": "curator.launcher-config/1", "config_schema": 7, "config_path": "operator-selected.json", "extensions": map[string]any{"claude_auth": &launcherconfig.ClaudeAuth{Type: "oauth", Store: store}}})
		if err != nil {
			return launcherconfig.Selected{}, err
		}
		return launcherconfig.ParseSelected(raw)
	}
}
func keychainStore() launcherconfig.AuthStore {
	return launcherconfig.AuthStore{Type: "keychain", Service: "synthetic-service", Account: "synthetic-account"}
}
func assertOAuthPrivacy(t *testing.T, f *pipelineFixture, token string, child childCapture, stderr []byte) {
	t.Helper()
	// The detector must recognize a known planted leak as well as a safe neighbor.
	leaks := func(s string) bool { return strings.Contains(s, token) }
	if !leaks("plant="+token) || leaks("redacted") {
		t.Fatal("invalid privacy detector")
	}
	raw, _ := json.Marshal(struct {
		Request any
		Plan    any
	}{f.request, f.plan})
	if leaks(string(raw)) || leaks(string(stderr)) || leaks(strings.Join(child.Argv, " ")) || leaks(strings.Join(f.deps.environ(), "\n")) {
		t.Fatal("OAuth reached an earlier surface")
	}
}

// The real pipeline places synthetic file/Keychain credentials in the final
// native Claude child only. Source bytes and prepared/admitted inputs stay intact.
func TestNativeClaudeOAuthThroughRun(t *testing.T) {
	const token = "synthetic-oauth-final-child-only"
	for _, kind := range []string{"file", "keychain"} {
		t.Run(kind, func(t *testing.T) {
			f := entryFixture(t, "claude_code")
			insertLauncherArgs(f, "--permissions", "native")
			calls := 0
			store := keychainStore()
			if kind == "file" {
				path := filepath.Join(f.dir, "token")
				writeFixture(t, path, []byte(token+"\n"), 0600)
				store = launcherconfig.AuthStore{Type: "file", Path: path}
			}
			selectOAuth(t, f, store)
			f.deps.claudeAuthResolver = claudeauth.Resolver{Keychain: func(context.Context, string, string) ([]byte, error) { calls++; return []byte(token), nil }}
			code, out, stderr := f.run()
			var child childCapture
			if code != 0 || json.Unmarshal(out, &child) != nil {
				t.Fatalf("native OAuth failed: code=%d stderr=%s", code, stderr)
			}
			found := 0
			for _, e := range child.Env {
				if e == claudeauth.Variable+"="+token {
					found++
				}
			}
			want := 0
			if kind == "keychain" {
				want = 1
			}
			if found != 1 || calls != want {
				t.Fatal("wrong credential destination/lookup count")
			}
			assertOAuthPrivacy(t, f, token, child, stderr)
		})
	}
}

// Help/version and other runtimes never consult a configured Claude store.
// Other runtimes retain their normal successful launch behavior.
func TestClaudeOAuthUnrelatedPathsNoLookup(t *testing.T) {
	for _, kind := range []string{"help", "version", "codex_cli", "pi"} {
		t.Run(kind, func(t *testing.T) {
			runtime := "claude_code"
			if kind == "codex_cli" || kind == "pi" {
				runtime = kind
			}
			f := entryFixture(t, runtime)
			insertLauncherArgs(f, "--permissions", "native")
			selectOAuth(t, f, keychainStore())
			calls := 0
			f.deps.claudeAuthResolver = claudeauth.Resolver{Keychain: func(context.Context, string, string) ([]byte, error) {
				calls++
				return nil, errors.New("must not lookup")
			}}
			if kind == "help" {
				f.args = []string{"--help"}
			}
			if kind == "version" {
				f.args = []string{"--version"}
			}
			code, _, stderr := f.run()
			if code != 0 || calls != 0 {
				t.Fatalf("unrelated path changed: code=%d calls=%d stderr=%s", code, calls, stderr)
			}
		})
	}
}

// Missing/invalid explicit stores refuse without native ambient fallback. The
// gateway unregisters an acquired registration even when OAuth fails before spawn.
func TestClaudeOAuthRefusalCleansGateway(t *testing.T) {
	for _, kind := range []string{"valid", "missing", "keychain-error", "empty", "interrupt-before-lookup", "interrupt-during-lookup"} {
		t.Run(kind, func(t *testing.T) {
			f := printNetworkFixture(t)
			g := newE2EGateway(t)
			writeGatewayCatalog(t, f, g.socket)
			f.args = withNetworkFlag(f.args, "egress-a")
			store := keychainStore()
			if kind == "missing" {
				store = launcherconfig.AuthStore{Type: "file", Path: filepath.Join(f.dir, "absent-token")}
			}
			selectOAuth(t, f, store)
			calls := 0
			var held chan<- os.Signal
			if strings.HasPrefix(kind, "interrupt-") {
				f.deps.leaseNotify = func(c chan<- os.Signal, _ ...os.Signal) { held = c }
			}
			f.deps.claudeAuthResolver = claudeauth.Resolver{Keychain: func(context.Context, string, string) ([]byte, error) {
				calls++
				if kind == "interrupt-during-lookup" {
					held <- os.Interrupt
				}
				if kind == "keychain-error" {
					return nil, errors.New("synthetic-private-backend-detail")
				}
				if kind == "empty" {
					return nil, nil
				}
				return []byte("synthetic-gateway-oauth"), nil
			}}
			if kind == "interrupt-before-lookup" {
				f.deps.networkEmit = func(io.Writer, *adapterprobe.Provenance) error { held <- os.Interrupt; return nil }
			}
			code, out, stderr := f.run()
			if kind == "valid" {
				if code != 0 || len(out) == 0 || calls != 1 || g.calls() != "/v1/health /v1/register /v1/activate /v1/unregister" {
					t.Fatalf("valid gateway OAuth failed code=%d calls=%s stderr=%s", code, g.calls(), stderr)
				}
				return
			}
			if code == 0 || len(out) != 0 || g.calls() != "/v1/health /v1/register /v1/unregister" || strings.Contains(string(stderr), "synthetic-private-backend-detail") {
				t.Fatalf("OAuth refusal leaked child/registration: code=%d calls=%s stderr=%s", code, g.calls(), stderr)
			}
			want := 1
			if kind == "missing" || kind == "interrupt-before-lookup" {
				want = 0
			}
			if calls != want {
				t.Fatal("lookup happened before late checks")
			}
			f.assertNoChild(t)
		})
	}
}

// Explicit credential overlays and MCP lookups cannot bypass the selected store.
func TestClaudeOAuthOverlayRefusal(t *testing.T) {
	for _, kind := range []string{"literal", "mcp-lookup"} {
		t.Run(kind, func(t *testing.T) {
			f := entryFixture(t, "claude_code")
			selectOAuth(t, f, keychainStore())
			calls := 0
			f.deps.claudeAuthResolver = claudeauth.Resolver{Keychain: func(context.Context, string, string) ([]byte, error) { calls++; return []byte("synthetic"), nil }}
			var doc map[string]any
			if err := json.Unmarshal([]byte(f.resolver.stdout), &doc); err != nil {
				t.Fatal(err)
			}
			if kind == "literal" {
				doc["env"].(map[string]any)[claudeauth.Variable] = "synthetic-inline-forbidden"
			} else {
				doc["mcp"].(map[string]any)["env_names"] = []string{claudeauth.Variable}
			}
			raw, _ := json.Marshal(doc)
			f.resolver.stdout = string(raw) + "\n"
			code, out, _ := f.run()
			if code == 0 || len(out) != 0 || calls != 0 {
				t.Fatal("credential overlay was accepted")
			}
			f.assertNoChild(t)
		})
	}
}

// Explicit hosted/resume and selected-auto routes refuse the unsupported OAuth
// selection before any manager describe/recovery/start, without reading a real store.
func TestClaudeOAuthHostedRefusesBeforeContact(t *testing.T) {
	bin := frontBuild(t)
	for _, kind := range []string{"hosted", "resume", "auto"} {
		t.Run(kind, func(t *testing.T) {
			f := newFront(t, bin)
			f.registry("selected")
			writeFixture(t, filepath.Join(f.root, "selected-config.json"), frontJSON(map[string]any{"schema": "curator.launcher-config/1", "config_schema": 7, "config_path": "operator-selected.json", "extensions": map[string]any{"claude_auth": &launcherconfig.ClaudeAuth{Type: "oauth", Store: launcherconfig.AuthStore{Type: "file", Path: filepath.Join(f.root, "absent-token")}}}}), 0600)
			var args []string
			if kind == "hosted" {
				args = []string{"--hosted"}
			}
			if kind == "resume" {
				args = []string{"--hosted", "resume"}
			}
			code, _, stderr := f.invoke(false, false, args...)
			f.require(t, code, 1, stderr, "selected Claude OAuth source requires native execution", nil, 0, false)
		})
	}
}
