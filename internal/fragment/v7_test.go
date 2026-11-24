package fragment

import (
	"encoding/json"
	"strings"
	"testing"
)

func v7AntigravityFixture(t *testing.T, withMCP bool) []byte {
	t.Helper()
	home := "/manager/environments/default/antigravity"
	m := map[string]any{
		"fragment":    IdentityV7,
		"environment": EnvAntigravity,
		"profile":     map[string]any{"name": "default", "lock_sha256": strings.Repeat("a", 64)},
		"precedence":  map[string]any{"winner": "higher-weight", "placement": "winner-last"},
		"permissions": map[string]any{"mode": "native", "locked": false, "source": "default"},
		"home":        map[string]any{"path": home, "transport": "flag"},
		"env": map[string]any{
			AntigravityForceFileTokenVariable: AntigravityForceFileTokenTrue,
		},
		"credential_store": map[string]any{"selection": "file"},
	}
	if withMCP {
		m["mcp"] = map[string]any{
			"path":      home + "/" + AntigravityMCPFile,
			"env_names": []any{"FIGMA_API_KEY"},
			"channels":  []any{map[string]any{"kind": "file", "filename": AntigravityMCPFile}},
		}
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestV7AntigravityValid(t *testing.T) {
	if !IsKnownEnvironment(EnvAntigravity) {
		t.Fatal("antigravity must be known")
	}
	if HomeVariable(EnvAntigravity) != "" {
		t.Fatalf("HomeVariable(antigravity) = %q, want empty", HomeVariable(EnvAntigravity))
	}
	for _, withMCP := range []bool{false, true} {
		raw := v7AntigravityFixture(t, withMCP)
		f, err := Parse(raw)
		if err != nil {
			t.Fatalf("Parse(withMCP=%v): %v", withMCP, err)
		}
		if f.Revision != IdentityV7 || f.Environment != EnvAntigravity {
			t.Fatalf("unexpected header: %+v", f)
		}
		if f.HomeBinding == nil || f.HomeBinding.Transport != "flag" || f.Home() != "/manager/environments/default/antigravity" {
			t.Fatalf("unexpected home binding: %+v", f.HomeBinding)
		}
		if f.Env[AntigravityForceFileTokenVariable] != AntigravityForceFileTokenTrue || len(f.Env) != 1 {
			t.Fatalf("unexpected env: %+v", f.Env)
		}
		if f.CredentialStore == nil || f.CredentialStore.Selection != "file" {
			t.Fatalf("unexpected credential_store: %+v", f.CredentialStore)
		}
		if withMCP && (f.MCP == nil || f.MCP.Path != f.Home()+"/"+AntigravityMCPFile) {
			t.Fatalf("unexpected mcp: %+v", f.MCP)
		}
		round, err := Parse(f.Canonical)
		if err != nil || round.Digest != f.Digest {
			t.Fatalf("canonical roundtrip failed: %v", err)
		}
		if err := CheckLaunchRevision(f); err != nil {
			t.Fatalf("CheckLaunchRevision: %v", err)
		}
	}
}

func TestV7AntigravityInvalid(t *testing.T) {
	for name, edit := range map[string]func(map[string]any){
		"missing home": func(m map[string]any) { delete(m, "home") },
		"dot-gemini basename": func(m map[string]any) {
			m["home"] = map[string]any{"path": "/manager/environments/default/.gemini", "transport": "flag"}
		},
		"unclean home path": func(m map[string]any) {
			m["home"] = map[string]any{"path": "/manager/environments/default/antigravity/", "transport": "flag"}
		},
		"relative home path": func(m map[string]any) {
			m["home"] = map[string]any{"path": "relative/antigravity", "transport": "flag"}
		},
		"wrong home transport": func(m map[string]any) {
			m["home"] = map[string]any{"path": "/manager/environments/default/antigravity", "transport": "env"}
		},
		"extra home field":       func(m map[string]any) { m["home"].(map[string]any)["extra"] = true },
		"missing file token env": func(m map[string]any) { m["env"] = map[string]any{} },
		"false file token env":   func(m map[string]any) { m["env"] = map[string]any{AntigravityForceFileTokenVariable: "false"} },
		"extra env var":          func(m map[string]any) { m["env"].(map[string]any)["HOME"] = "/tmp/other" },
		"missing credential":     func(m map[string]any) { delete(m, "credential_store") },
		"native credential":      func(m map[string]any) { m["credential_store"] = map[string]any{"selection": "native"} },
		"keychain credential":    func(m map[string]any) { m["credential_store"] = map[string]any{"selection": "keychain"} },
		"credential overrides": func(m map[string]any) {
			m["credential_store"] = map[string]any{"selection": "file", "config_overrides": map[string]any{"x": "y"}}
		},
		"root_context forbidden":  func(m map[string]any) { m["root_context"] = map[string]any{"exclusive": true} },
		"system_prompt forbidden": func(m map[string]any) { m["system_prompt"] = map[string]any{"path": "/tmp/sp.md", "channels": []any{}} },
		"wrong mcp path": func(m map[string]any) {
			m["mcp"] = map[string]any{"path": "/tmp/other/mcp.json", "env_names": []any{}, "channels": []any{map[string]any{"kind": "file", "filename": AntigravityMCPFile}}}
		},
		"wrong mcp filename": func(m map[string]any) {
			m["mcp"] = map[string]any{"path": "/manager/environments/default/antigravity/config/mcp_config.json", "env_names": []any{}, "channels": []any{map[string]any{"kind": "file", "filename": "mcp.json"}}}
		},
		"forbidden mcp AGY_ADC_AUTH": func(m map[string]any) {
			m["mcp"] = map[string]any{"path": "/manager/environments/default/antigravity/config/mcp_config.json", "env_names": []any{"AGY_ADC_AUTH"}, "channels": []any{map[string]any{"kind": "file", "filename": AntigravityMCPFile}}}
		},
		"forbidden mcp GEMINI_API_KEY": func(m map[string]any) {
			m["mcp"] = map[string]any{"path": "/manager/environments/default/antigravity/config/mcp_config.json", "env_names": []any{"GEMINI_API_KEY"}, "channels": []any{map[string]any{"kind": "file", "filename": AntigravityMCPFile}}}
		},
		"forbidden mcp force file var": func(m map[string]any) {
			m["mcp"] = map[string]any{"path": "/manager/environments/default/antigravity/config/mcp_config.json", "env_names": []any{AntigravityForceFileTokenVariable}, "channels": []any{map[string]any{"kind": "file", "filename": AntigravityMCPFile}}}
		},
		"v4 carrying antigravity": func(m map[string]any) { m["fragment"] = IdentityV4; delete(m, "home") },
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(mutate(t, v7AntigravityFixture(t, true), edit)); err == nil {
				t.Fatalf("invalid v7 antigravity (%s) was admitted", name)
			}
		})
	}

	// Non-antigravity v7 must reject `home`.
	raw := mutate(t, v4Fixture(t, EnvCodexCLI), func(m map[string]any) {
		m["fragment"] = IdentityV7
		m["home"] = map[string]any{"path": "/manager/environments/default/codex_cli", "transport": "flag"}
	})
	if _, err := Parse(raw); err == nil {
		t.Fatal("non-antigravity v7 with home was admitted")
	}
}
