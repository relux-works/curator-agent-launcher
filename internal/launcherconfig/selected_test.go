package launcherconfig

import (
	"fmt"
	"strings"
	"testing"
)

// Core's selected-source projection accepts absent metadata on older supported
// envelopes and closed references on schema 7, without accessing either store.
func TestSelectedCoreProjection(t *testing.T) {
	for _, version := range []int{1, 2, 3, 5, 7} {
		doc := fmt.Sprintf(`{"schema":"curator.launcher-config/1","config_schema":%d,"config_path":"selected/config.json","extensions":{}}`, version)
		got, err := ParseSelected([]byte(doc))
		if err != nil || got.ConfigPath != "selected/config.json" || got.ConfigSchema != version || got.Proxy != nil || got.ClaudeAuth != nil {
			t.Fatalf("supported empty projection: %v", err)
		}
	}
	for _, ext := range []string{
		`"proxy":{"profile":"filtered"}`,
		`"claude_auth":{"type":"oauth","store":{"type":"keychain","service":"synthetic-service","account":"synthetic-account"}}`,
		`"proxy":{"profile":"filtered"},"claude_auth":{"type":"oauth","store":{"type":"keychain","service":"synthetic-service","account":"synthetic-account"}}`,
	} {
		doc := `{"schema":"curator.launcher-config/1","config_schema":7,"config_path":"selected/config.json","extensions":{` + ext + `}}`
		if _, err := ParseSelected([]byte(doc)); err != nil {
			t.Fatal(err)
		}
	}
}

// Invalid, ambiguous or future projections return no partial selections and no
// input diagnostics. Nearby accepted projections above prevent reject-all success.
func TestSelectedCoreProjectionClosed(t *testing.T) {
	const prefix = `{"schema":"curator.launcher-config/1","config_schema":7,"config_path":"selected/config.json","extensions":`
	for _, doc := range []string{
		`{}`, prefix + `null}`, prefix + `[]}`,
		prefix + `{"SECRET_KEY":"SECRET_VALUE"}}`,
		prefix + `{"proxy":{"profile":"filtered","SECRET_KEY":"SECRET_VALUE"}}}`,
		prefix + `{"proxy":{"profile":"a","profile":"SECRET_VALUE"}}}`,
		prefix + `{"claude_auth":{"type":"oauth","token":"SECRET_VALUE"}}}`,
		prefix + `{"claude_auth":{"type":"oauth","store":{"type":"file","path":"relative","account":"SECRET_VALUE"}}}}`,
		strings.Replace(prefix+`{}}`, `"config_schema":7`, `"config_schema":6`, 1),
		strings.Replace(prefix+`{}}`, `"config_schema":7`, `"config_schema":8`, 1),
		strings.Replace(prefix+`{"proxy":{"profile":"filtered"}}}`, `"config_schema":7`, `"config_schema":5`, 1),
		strings.Replace(prefix+`{}}`, `"config_path":"selected/config.json"`, `"config_path":""`, 1),
		prefix + `{ },"SECRET_KEY":"SECRET_VALUE"}`,
	} {
		got, err := ParseSelected([]byte(doc))
		if err == nil || got != (Selected{}) || strings.Contains(err.Error(), "SECRET") {
			t.Fatalf("unsafe projection refusal: %+v %v", got, err)
		}
	}
}

// /2 and /3 name the selected source by a clean absolute path, while /1 keeps
// its admitted reference grammar. Each refusal has an accepted neighbour that
// differs only in the path.
func TestSelectedConfigPathAbsoluteFromSchema8(t *testing.T) {
	withLauncherConfigV3(t, true)
	envelopes := map[string]string{
		"v2": `{"schema":"curator.launcher-config/2","config_schema":8,"config_path":%q,"extensions":{}}`,
		"v3": `{"schema":"curator.launcher-config/3","config_schema":9,"config_path":%q,"extensions":{}}`,
	}
	for name, envelope := range envelopes {
		if _, err := ParseSelected([]byte(fmt.Sprintf(envelope, "/selected/config.json"))); err != nil {
			t.Fatalf("%s: absolute path refused", name)
		}
		for _, path := range []string{"selected/config.json", "./config.json", "/selected/../config.json", "/selected//config.json", "/selected/"} {
			if _, err := ParseSelected([]byte(fmt.Sprintf(envelope, path))); err != ErrSelected {
				t.Fatalf("%s: path %q admitted", name, path)
			}
		}
	}
	v1 := `{"schema":"curator.launcher-config/1","config_schema":7,"config_path":"selected/config.json","extensions":{}}`
	if _, err := ParseSelected([]byte(v1)); err != nil {
		t.Fatal("/1 relative reference refused")
	}
}
