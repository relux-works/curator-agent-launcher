package fragment

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func v4Fixture(t *testing.T, env string) []byte {
	t.Helper()
	return mutate(t, readFile(t, "testdata/v3/muse.json"), func(m map[string]any) {
		m["fragment"] = "launch-env-fragment-v4"
		m["environment"] = env
		m["credential_store"] = map[string]any{"selection": "native"}
		if env == EnvMuse {
			m["env"].(map[string]any)["TBH_CREDENTIAL_BACKEND"] = "file"
			m["credential_store"] = map[string]any{"selection": "file"}
			m["root_context"] = map[string]any{"exclusive": true}
		} else {
			m["env"] = map[string]any{HomeVariable(env): "/manager/environments/default/" + env}
		}
	})
}

// The v4 contract admits each adapter's native policy, Muse's explicit fact,
// and both typed Codex overrides; canonical round trips preserve identity.
func TestV4Valid(t *testing.T) {
	for _, env := range []string{EnvMuse, EnvCodexCLI, EnvClaudeCode, EnvPi, EnvOpenCode} {
		t.Run(env, func(t *testing.T) {
			raw := v4Fixture(t, env)
			f, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			if f.Revision != "launch-env-fragment-v4" || f.Environment != env || f.CredentialStore == nil {
				t.Fatalf("wrong v4: %+v", f)
			}
			if env == EnvMuse && (f.RootContext == nil || !f.RootContext.Exclusive || len(f.Env) != 5 || f.Env["TBH_CREDENTIAL_BACKEND"] != "file") {
				t.Fatalf("lost Muse policy: %+v", f)
			}
			round, err := Parse(f.Canonical)
			if err != nil || round.Digest != f.Digest {
				t.Fatalf("roundtrip: %v", err)
			}
			if env != EnvMuse {
				_, err = Parse(mutate(t, raw, func(m map[string]any) { delete(m, "credential_store") }))
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	for selection, value := range map[string]string{"file": "file", "keychain": "keyring"} {
		f, err := Parse(mutate(t, v4Fixture(t, EnvCodexCLI), func(m map[string]any) {
			m["credential_store"] = map[string]any{"selection": selection, "config_overrides": map[string]any{"cli_auth_credentials_store": value}}
		}))
		if err != nil {
			t.Fatal(err)
		}
		if f.CredentialStore.ConfigOverrides["cli_auth_credentials_store"] != value {
			t.Fatal("lost typed override")
		}
	}
	_, err := Parse(mutate(t, v4Fixture(t, EnvMuse), func(m map[string]any) { delete(m, "root_context") }))
	if err != nil {
		t.Fatal(err)
	}
}

// Nearby valid controls above prevent blanket rejection. These vectors reject
// malformed policy, cross-adapter facts, legacy-token additions and ambiguous JSON.
func TestV4Invalid(t *testing.T) {
	for _, env := range []string{EnvMuse, EnvCodexCLI, EnvClaudeCode} {
		for name, edit := range map[string]func(map[string]any){
			"missing permissions": func(m map[string]any) { delete(m, "permissions") },
			"unknown revision":    func(m map[string]any) { m["fragment"] = "launch-env-fragment-v5" },
			"unknown member":      func(m map[string]any) { m["future"] = true },
			"v3 carrying v4":      func(m map[string]any) { m["fragment"] = IdentityV3 },
			"null store":          func(m map[string]any) { m["credential_store"] = nil },
			"empty store":         func(m map[string]any) { m["credential_store"] = map[string]any{} },
			"unknown selection":   func(m map[string]any) { m["credential_store"] = map[string]any{"selection": "auto"} },
			"extra store":         func(m map[string]any) { m["credential_store"].(map[string]any)["extra"] = true },
			"empty overrides":     func(m map[string]any) { m["credential_store"].(map[string]any)["config_overrides"] = map[string]any{} },
			"null root":           func(m map[string]any) { m["root_context"] = nil },
			"empty root":          func(m map[string]any) { m["root_context"] = map[string]any{} },
			"false root":          func(m map[string]any) { m["root_context"] = map[string]any{"exclusive": false} },
			"string root":         func(m map[string]any) { m["root_context"] = "exclusive" },
			"string exclusive":    func(m map[string]any) { m["root_context"] = map[string]any{"exclusive": "true"} },
			"extra root":          func(m map[string]any) { m["root_context"] = map[string]any{"exclusive": true, "extra": true} },
		} {
			t.Run(env+"/"+name, func(t *testing.T) {
				if _, err := Parse(mutate(t, v4Fixture(t, env), edit)); err == nil {
					t.Fatal("invalid v4 admitted")
				}
			})
		}
		raw := string(v4Fixture(t, env))
		t.Run(env+"/duplicate", func(t *testing.T) {
			if _, err := Parse([]byte(strings.Replace(raw, `"selection":`, `"selection":"file","selection":`, 1))); err == nil {
				t.Fatal("duplicate admitted")
			}
		})
	}
	for name, edit := range map[string]func(map[string]any){
		"missing store":    func(m map[string]any) { delete(m, "credential_store") },
		"native store":     func(m map[string]any) { m["credential_store"] = map[string]any{"selection": "native"} },
		"missing constant": func(m map[string]any) { delete(m["env"].(map[string]any), "TBH_CREDENTIAL_BACKEND") },
		"wrong constant":   func(m map[string]any) { m["env"].(map[string]any)["TBH_CREDENTIAL_BACKEND"] = "keychain" },
		"extra HOME":       func(m map[string]any) { m["env"].(map[string]any)["HOME"] = "/foreign" },
		"foreign parent":   func(m map[string]any) { m["env"].(map[string]any)["XDG_DATA_HOME"] = "/foreign/data" },
		"relative parent":  func(m map[string]any) { m["env"].(map[string]any)["XDG_CACHE_HOME"] = "relative/cache" },
		"system prompt":    func(m map[string]any) { m["system_prompt"] = map[string]any{"path": "/prompt", "channels": []any{}} },
		"mcp": func(m map[string]any) {
			m["mcp"] = map[string]any{"path": "/mcp", "channels": []any{}, "env_names": []any{}}
		},
	} {
		t.Run("muse/"+name, func(t *testing.T) {
			if _, err := Parse(mutate(t, v4Fixture(t, EnvMuse), edit)); err == nil {
				t.Fatal("invalid Muse policy admitted")
			}
		})
	}
	for _, env := range []string{EnvCodexCLI, EnvClaudeCode, EnvPi, EnvOpenCode} {
		for _, store := range []any{
			map[string]any{"selection": "native", "config_overrides": map[string]any{"cli_auth_credentials_store": "file"}},
			map[string]any{"selection": "file"},
			map[string]any{"selection": "keychain", "config_overrides": map[string]any{"cli_auth_credentials_store": "file"}},
			map[string]any{"selection": "file", "config_overrides": map[string]any{"cli_auth_credentials_store": "file", "extra": "x"}},
			map[string]any{"selection": "file", "config_overrides": nil},
			map[string]any{"selection": "file", "config_overrides": map[string]any{"cli_auth_credentials_store": true}},
		} {
			if _, err := Parse(mutate(t, v4Fixture(t, env), func(m map[string]any) { m["credential_store"] = store })); err == nil {
				t.Fatalf("%s invalid store admitted: %#v", env, store)
			}
		}
		if _, err := Parse(mutate(t, v4Fixture(t, env), func(m map[string]any) { m["root_context"] = map[string]any{"exclusive": true} })); err == nil {
			t.Fatal("non-Muse root admitted")
		}
	}
}

// Strict duplicate detection covers both the envelope and nested v4 policy;
// the unmodified fragment is a neighboring valid control.
func TestV4DuplicateKeys(t *testing.T) {
	raw := string(v4Fixture(t, EnvMuse))
	if _, err := Parse([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"fragment", "exclusive", "TBH_CREDENTIAL_BACKEND"} {
		t.Run(key, func(t *testing.T) {
			bad := strings.Replace(raw, `"`+key+`":`, `"`+key+`":null,"`+key+`":`, 1)
			if _, err := Parse([]byte(bad)); err == nil || !strings.Contains(err.Error(), "duplicate") {
				t.Fatalf("duplicate %s: %v", key, err)
			}
		})
	}
}

// The vendored source schema remains byte-identical to the recorded source
// commit. Reader semantics are exercised separately by the vectors above.
func TestV4SchemaSourcePin(t *testing.T) {
	raw := readFile(t, "testdata/v4/launch-env-fragment-v4.schema.json")
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != "afc824151de968d1bd2c3c9a15d6bf4024ad6920f0f0a56351ee755b56aa9847" {
		t.Fatalf("schema source changed: %s", got)
	}
}
