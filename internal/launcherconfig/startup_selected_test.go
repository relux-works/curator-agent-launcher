package launcherconfig

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Machine schema 11 retains the /1 projection and the existing schema-7
// reference shapes. Parsing selects metadata without accessing either store.
func TestStartupSelectedProjection(t *testing.T) {
	proxy := &Proxy{Profile: "filtered"}
	auth := &ClaudeAuth{Type: "oauth", Store: AuthStore{Type: "keychain", Service: "synthetic-service", Account: "synthetic-account"}}
	filePath := filepath.Join(t.TempDir(), "absent-token")
	fileAuth := &ClaudeAuth{Type: "oauth", Store: AuthStore{Type: "file", Path: filePath}}
	for _, tc := range []struct {
		name       string
		extensions string
		proxy      *Proxy
		auth       *ClaudeAuth
	}{
		{"empty", `{}`, nil, nil},
		{"proxy", `{"proxy":{"profile":"filtered"}}`, proxy, nil},
		{"auth", `{"claude_auth":{"type":"oauth","store":{"type":"keychain","service":"synthetic-service","account":"synthetic-account"}}}`, nil, auth},
		{"file-auth", fmt.Sprintf(`{"claude_auth":{"type":"oauth","store":{"type":"file","path":%q}}}`, filePath), nil, fileAuth},
		{"both", `{"proxy":{"profile":"filtered"},"claude_auth":{"type":"oauth","store":{"type":"keychain","service":"synthetic-service","account":"synthetic-account"}}}`, proxy, auth},
	} {
		for _, version := range []int{7, 11} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, version), func(t *testing.T) {
				doc := fmt.Sprintf(`{"schema":"curator.launcher-config/1","config_schema":%d,"config_path":"selected/config.json","extensions":%s}`, version, tc.extensions)
				got, err := ParseSelected([]byte(doc))
				want := Selected{ConfigSchema: version, ConfigPath: "selected/config.json", Proxy: tc.proxy, ClaudeAuth: tc.auth}
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("selected metadata: got %+v, want %+v, error %v", got, want, err)
				}
			})
		}
	}
}

// Startup policy and centre configuration remain Core-owned. A malformed or
// unsupported projection refuses with the existing error and no partial result;
// the valid schema-7/11 controls above rule out accepting nothing.
func TestStartupSelectedProjectionClosed(t *testing.T) {
	const base = `{"schema":"curator.launcher-config/1","config_schema":11,"config_path":"selected/config.json","extensions":{}}`
	for _, tc := range []struct{ name, doc string }{
		{"notification-protocol", strings.Replace(base, "/1", "/2", 1)},
		{"candidate-protocol", strings.Replace(base, "/1", "/3", 1)},
		{"null-extensions", strings.Replace(base, `"extensions":{}`, `"extensions":null`, 1)},
		{"startup-policy", strings.Replace(base, `"extensions":{}`, `"extensions":{"startup_verification":{"level":3}}`, 1)},
		{"centre", strings.Replace(base, `"extensions":{}`, `"extensions":{"grant_service":{"endpoint":"https://example.invalid"}}`, 1)},
		{"notification", strings.Replace(base, `"extensions":{}`, `"extensions":{"notification":{}}`, 1)},
		{"valid-notification-wrong-carrier", strings.Replace(strings.Replace(notificationProjection(notificationHostFixture), "/2", "/1", 1), `"config_schema":8`, `"config_schema":11`, 1)},
		{"unknown-member", strings.Replace(base, `"extensions":{}`, `"extensions":{"PRIVATE_NAME":"PRIVATE_VALUE"}`, 1)},
		{"unknown-root", strings.Replace(base, `"extensions":{}`, `"extensions":{},"PRIVATE_NAME":"PRIVATE_VALUE"`, 1)},
		{"duplicate-version", strings.Replace(base, `"config_schema":11`, `"config_schema":7,"config_schema":11`, 1)},
		{"duplicate-extension", strings.Replace(base, `"extensions":{}`, `"extensions":{"proxy":{"profile":"filtered"},"proxy":{"profile":"PRIVATE_VALUE"}}`, 1)},
		{"partial-selection", strings.Replace(base, `"extensions":{}`, `"extensions":{"proxy":{"profile":"filtered"},"claude_auth":{"type":"oauth","token":"PRIVATE_VALUE"}}`, 1)},
		{"null-proxy", strings.Replace(base, `"extensions":{}`, `"extensions":{"proxy":null}`, 1)},
		{"mixed-store", strings.Replace(base, `"extensions":{}`, `"extensions":{"claude_auth":{"type":"oauth","store":{"type":"file","path":"/synthetic/token","account":"PRIVATE_VALUE"}}}`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) { assertStartupSelectedRefusal(t, tc.doc) })
	}
	for _, version := range []string{`4`, `6`, `9`, `10`, `12`, `11.0`, `11e0`, `"11"`, `null`} {
		t.Run("version/"+version, func(t *testing.T) {
			assertStartupSelectedRefusal(t, strings.Replace(base, `"config_schema":11`, `"config_schema":`+version, 1))
		})
	}
}

func assertStartupSelectedRefusal(t *testing.T, doc string) {
	t.Helper()
	got, err := ParseSelected([]byte(doc))
	if !errors.Is(err, ErrSelected) || got != (Selected{}) || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatalf("projection did not refuse atomically with sanitized error: %+v %v", got, err)
	}
}
