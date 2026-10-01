package fragment

import (
	"strings"
	"testing"
)

// Contract: curator-spec d373078a, environments §10.2 and v3 schema.
func TestMuseV3Fragment(t *testing.T) {
	raw := readFile(t, "testdata/v3/muse.json")
	f, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if f.Revision != IdentityV3 || f.Environment != EnvMuse || f.Home() != "/manager/environments/default/muse" || len(f.Env) != 4 || f.Permissions == nil {
		t.Fatalf("wrong v3 fragment: %+v", f)
	}
	if _, ok := f.Env["HOME"]; ok {
		t.Fatal("HOME must never be set")
	}
	g, err := Parse(f.Canonical)
	if err != nil || g.Digest != f.Digest {
		t.Fatalf("v3 canonical round trip: %v", err)
	}
}

func TestMuseV3RejectsInvalidFragments(t *testing.T) {
	raw := readFile(t, "testdata/v3/muse.json")
	for name, edit := range map[string]func(map[string]any){
		"v1 muse":     func(m map[string]any) { m["fragment"] = Identity; delete(m, "permissions") },
		"v2 muse":     func(m map[string]any) { m["fragment"] = IdentityV2 },
		"HOME":        func(m map[string]any) { m["env"].(map[string]any)["HOME"] = "/native" },
		"missing XDG": func(m map[string]any) { delete(m["env"].(map[string]any), "XDG_STATE_HOME") },
		"foreign XDG": func(m map[string]any) { m["env"].(map[string]any)["XDG_DATA_HOME"] = "/foreign/data" },
		"wrong suffix": func(m map[string]any) {
			m["env"].(map[string]any)["XDG_CONFIG_HOME"] = "/manager/environments/default/muse/wrong"
		},
		"relative": func(m map[string]any) { m["env"].(map[string]any)["XDG_CACHE_HOME"] = "relative/cache" },
		"dotdot":   func(m map[string]any) { m["env"].(map[string]any)["XDG_CACHE_HOME"] = "/manager/../cache" },
		"root parent": func(m map[string]any) {
			for _, kind := range []string{"config", "data", "state", "cache"} {
				m["env"].(map[string]any)["XDG_"+strings.ToUpper(kind)+"_HOME"] = "/" + kind
			}
		},
		"system prompt": func(m map[string]any) { m["system_prompt"] = map[string]any{"path": "/prompt", "channels": []any{}} },
		"mcp": func(m map[string]any) {
			m["mcp"] = map[string]any{"path": "/mcp", "env_names": []any{}, "channels": []any{}}
		},
		"missing permissions":   func(m map[string]any) { delete(m, "permissions") },
		"malformed permissions": func(m map[string]any) { m["permissions"].(map[string]any)["locked"] = true },
		"global yolo": func(m map[string]any) {
			m["permissions"] = map[string]any{"mode": "yolo", "locked": true, "source": "global"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(mutate(t, raw, edit)); err == nil {
				t.Fatal("invalid fragment admitted")
			}
		})
	}
}

func TestV3OtherAdaptersRetainV2Rules(t *testing.T) {
	for _, env := range []string{EnvClaudeCode, EnvCodexCLI, EnvOpenCode, EnvPi} {
		base := mutate(t, readFile(t, "testdata/schema-cases/valid-minimal.json"), func(m map[string]any) {
			m["environment"] = env
			m["env"] = map[string]any{HomeVariable(env): "/manager/environments/default/" + env}
			m["fragment"] = IdentityV2
			m["permissions"] = map[string]any{"mode": "native", "locked": false, "source": "default"}
		})
		t.Run(env, func(t *testing.T) {
			v2, err := Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			v3, err := Parse([]byte(strings.ReplaceAll(string(base), IdentityV2, IdentityV3)))
			if err != nil {
				t.Fatal(err)
			}
			if v2.Home() != v3.Home() || len(v2.Env) != 1 || len(v3.Env) != 1 || *v2.Permissions != *v3.Permissions {
				t.Fatal("legacy adapter rules changed")
			}
		})
	}
}
