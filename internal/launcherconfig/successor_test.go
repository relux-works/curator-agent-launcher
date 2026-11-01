package launcherconfig

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Host policy retains its v1 encoding. Round trips preserve explicit policy;
// neither layer is an alternative authority for Core launcher selections.
func TestHostPolicyRoundTripAndSelectionAuthority(t *testing.T) {
	original, err := Parse([]byte(`{"schema":"curator-run-launcher-v1","host_mode":"native","delegated_trial":false}`))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(encoded)
	if err != nil || !reflect.DeepEqual(original, again) || !bytes.Contains(encoded, []byte(`"schema":"curator-run-launcher-v1"`)) {
		t.Fatal("host policy changed")
	}
	for _, layer := range []bool{false, true} {
		for _, member := range []string{`"proxy":null`, `"proxy":{"profile":"filtered"}`, `"claude_auth":null`, `"claude_auth":{"type":"oauth","store":{"type":"keychain","service":"s","account":"a"}}`} {
			r := fixture(t)
			put(t, r.Locations.System, `{"schema":"curator-run-launcher-v1","host_mode":"native"}`)
			put(t, r.Locations.User, `{"schema":"curator-run-launcher-v1"}`)
			if got, err := Load(r); err != nil || got.HostMode.Value != "native" {
				t.Fatal("valid policy refused")
			}
			path := r.Locations.System
			if layer {
				path = r.Locations.User
			}
			put(t, path, `{"schema":"curator-run-launcher-v1",`+member+`}`)
			if got, err := Load(r); err == nil || got != (Config{}) {
				t.Fatal("launcher.json supplied Core selection")
			}
		}
	}
}

// Typed OAuth validation preserves the selected file reference and rejects
// mixed store metadata without accessing the path.
func TestTypedClaudeSelectionValidation(t *testing.T) {
	selected := ClaudeAuth{Type: "oauth", Store: AuthStore{Type: "file", Path: filepath.Join(t.TempDir(), "absent-token")}}
	if err := ValidateClaudeAuth(selected); err != nil {
		t.Fatal(err)
	}
	selected.Store.Service = "forbidden-mixed-store"
	if err := ValidateClaudeAuth(selected); err == nil {
		t.Fatal("mixed store accepted")
	}
}

// Invalid selected sections return no partial policy. Nearby valid inputs are
// covered above; neither legacy syntax nor an unknown member enables auth.
func TestSharedSuccessorClosedInvalid(t *testing.T) {
	for _, member := range []string{
		`"proxy":null`, `"proxy":{}`, `"proxy":{"profile":""}`,
		`"proxy":{"profile":"ok","extra":true}`,
		`"proxy":{"profile":" bad "}`,
		`"claude_auth":null`, `"claude_auth":{}`,
		`"claude_auth":{"type":"native","store":{"type":"file","path":"/token"}}`,
		`"claude_auth":{"type":"oauth","token":"synthetic-secret"}`,
		`"claude_auth":{"type":"oauth","store":null}`,
		`"claude_auth":{"type":"oauth","store":{"type":"file","path":"relative"}}`,
		`"claude_auth":{"type":"oauth","store":{"type":"file","path":"/a/../token"}}`,
		`"claude_auth":{"type":"oauth","store":{"type":"file","path":"/token","service":"mixed"}}`,
		`"claude_auth":{"type":"oauth","store":{"type":"keychain","service":"s"}}`,
		`"claude_auth":{"type":"oauth","store":{"type":"keychain","service":"s","account":"a","path":"/mixed"}}`,
		`"claude_auth":{"type":"oauth","store":{"type":"keychain","service":"s\n","account":"a"}}`,
		`"claude_auth":{"type":"oauth","store":{"type":"node","source_ref":"node"}}`,
	} {
		doc := `{"schema":"curator.launcher-config/1","config_schema":7,"config_path":"selected.json","extensions":{` + member + `}}`
		got, err := ParseSelected([]byte(doc))
		if err == nil || got != (Selected{}) || strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatalf("invalid metadata was accepted or disclosed: %+v %v", got, err)
		}
	}
	for _, member := range []string{`"proxy":{"profile":"secret-filter"}`, `"claude_auth":{"type":"oauth","store":{"type":"file","path":"/token"}}`} {
		if _, err := Parse([]byte(`{"schema":"curator-run-launcher-v1",` + member + `}`)); err == nil {
			t.Fatal("legacy schema admitted successor member")
		}
	}
}

// Invalid JSON and unknown/duplicate keys may themselves contain sensitive
// bytes. The parser reports only a static error, not an input key or value.
func TestSharedSuccessorSanitizedSyntax(t *testing.T) {
	secret := "synthetic-secret-privacy-plant"
	for _, doc := range []string{
		`{"schema":"curator-run-launcher-v1","` + secret + `":1}`,
		`{"schema":"curator-run-launcher-v1","claude_auth":{"` + secret + `":1,"` + secret + `":2}}`,
	} {
		_, err := Parse([]byte(doc))
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("invalid input disclosed a key: %v", err)
		}
	}
}
