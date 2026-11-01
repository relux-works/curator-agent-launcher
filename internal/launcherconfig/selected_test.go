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
