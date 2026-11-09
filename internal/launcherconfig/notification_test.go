package launcherconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const notificationHostFixture = `{"backend":"host","contract":"curator-host-notification/1","workflow_reference":"host-notification-inbox-v1","workflow_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`

func notificationProjection(selection string) string {
	extension := ""
	if selection != "" {
		extension = `"notification":` + selection
	}
	return `{"schema":"curator.launcher-config/2","config_schema":8,"config_path":"/selected/config.json","extensions":{` + extension + `}}`
}

// The real bounded subprocess reader retains Host metadata from a successful
// child, but a nonzero child cannot publish even a complete valid projection.
func TestSelectedNotificationCommand(t *testing.T) {
	for _, mode := range []string{"notification", "notification-failed"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSelectedCoreCommandHelper$")
		cmd.Env = []string{"CURATOR_SELECTED_HELPER=" + mode, "GORACE=atexit_sleep_ms=0"}
		got, err := readSelectedCommand(cmd)
		if mode == "notification" {
			if err != nil || got.Notification == nil || got.Notification.Backend != "host" || got.Notification.WorkflowReference != "host-notification-inbox-v1" || got.Notification.WorkflowSHA256 != strings.Repeat("a", 64) {
				t.Fatalf("subprocess lost Host metadata: %+v %v", got, err)
			}
		} else if err != ErrSelected || got != (Selected{}) {
			t.Fatalf("failed child published Host metadata: %+v %v", got, err)
		}
	}
}

// Every Core schema8 form uses projection2; empty, explicit off and closed Host
// selections parse without losing the selected source or existing proxy/auth.
func TestSelectedNotificationProjection(t *testing.T) {
	for i, selection := range []string{
		"", `{"backend":"off"}`, notificationHostFixture,
		strings.Replace(notificationHostFixture, "host-notification-inbox-v1", strings.Repeat("x", 128), 1),
		strings.Replace(notificationHostFixture, "host-notification-inbox-v1", "ABCxyz012._:/@+-", 1),
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			doc := notificationProjection(selection)
			got, err := ParseSelected([]byte(doc))
			if err != nil || got.ConfigSchema != 8 || got.ConfigPath != "/selected/config.json" {
				t.Fatalf("valid schema8 notification projection refused: %+v %v", got, err)
			}
			if selection == "" {
				if got.Notification != nil {
					t.Fatal("absence acquired notification metadata")
				}
				return
			}
			var want Notification
			if err := json.Unmarshal([]byte(selection), &want); err != nil {
				t.Fatal(err)
			}
			if got.Notification == nil || *got.Notification != want {
				t.Fatal("selected workflow metadata changed")
			}
		})
	}
	doc := strings.Replace(notificationProjection(notificationHostFixture), `"extensions":{`, `"extensions":{"proxy":{"profile":"filtered"},"claude_auth":{"type":"oauth","store":{"type":"keychain","service":"s","account":"a"}},`, 1)
	got, err := ParseSelected([]byte(doc))
	if err != nil || got.Proxy == nil || got.Proxy.Profile != "filtered" || got.ClaudeAuth == nil || got.ClaudeAuth.Store.Account != "a" {
		t.Fatalf("schema8 lost existing selections: %+v %v", got, err)
	}
}

// A second configuration source cannot select notification, including off;
// existing host policy remains valid. Rejections do not reflect hostile fields.
func TestNotificationNotLauncherPolicy(t *testing.T) {
	if _, err := Parse([]byte(`{"schema":"curator-run-launcher-v1","host_mode":"auto"}`)); err != nil {
		t.Fatal(err)
	}
	for _, selection := range []string{`null`, `{"backend":"off"}`, notificationHostFixture, `{"HOSTILE_SECRET_KEY":"HOSTILE_SECRET_VALUE"}`} {
		if _, err := Parse([]byte(`{"schema":"curator-run-launcher-v1","notification":` + selection + `}`)); err == nil || strings.Contains(err.Error(), "HOSTILE_SECRET") {
			t.Fatalf("launcher policy accepted or exposed notification selection: %v", err)
		}
	}
}

// Invalid selections and envelope/version combinations return no partial
// metadata or hostile diagnostics; valid neighbors are exercised above.
func TestSelectedNotificationClosedProjection(t *testing.T) {
	bad := []string{`null`, `[]`, `{}`, `{"backend":null}`, `{"backend":"native"}`,
		`{"backend":"off","contract":"curator-host-notification/1"}`,
		`{"backend":"off","backend":"host"}`, `{"backend":"host"}`,
		strings.Replace(notificationHostFixture, "curator-host-notification/1", "curator-host-notification/2", 1),
		strings.Replace(notificationHostFixture, "host-notification-inbox-v1", "", 1),
		strings.Replace(notificationHostFixture, "host-notification-inbox-v1", "bad reference", 1),
		strings.Replace(notificationHostFixture, "host-notification-inbox-v1", `bad\u0000reference`, 1),
		strings.Replace(notificationHostFixture, "host-notification-inbox-v1", `\u03b1`, 1),
		strings.Replace(notificationHostFixture, "host-notification-inbox-v1", strings.Repeat("x", 129), 1),
		strings.Replace(notificationHostFixture, strings.Repeat("a", 64), strings.Repeat("A", 64), 1),
		strings.Replace(notificationHostFixture, strings.Repeat("a", 64), strings.Repeat("a", 63), 1),
		strings.TrimSuffix(notificationHostFixture, "}") + `,"endpoint":"HOSTILE_SECRET_VALUE"}`,
		strings.TrimSuffix(notificationHostFixture, "}") + `,"HOSTILE_SECRET_KEY":"HOSTILE_SECRET_VALUE"}`,
	}
	for i, selection := range bad {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			got, err := ParseSelected([]byte(notificationProjection(selection)))
			if err != ErrSelected || got != (Selected{}) || strings.Contains(err.Error(), "HOSTILE_SECRET") {
				t.Fatalf("unsafe notification projection refusal: %+v %v", got, err)
			}
		})
	}
	for _, doc := range []string{
		strings.Replace(notificationProjection(notificationHostFixture), `"config_schema":8`, `"config_schema":7`, 1),
		strings.Replace(notificationProjection(notificationHostFixture), "curator.launcher-config/2", "curator.launcher-config/1", 1),
		strings.Replace(notificationProjection(notificationHostFixture), "curator.launcher-config/2", "curator.launcher-config/3", 1),
		strings.Replace(strings.Replace(notificationProjection(notificationHostFixture), `"config_schema":8`, `"config_schema":7`, 1), "curator.launcher-config/2", "curator.launcher-config/1", 1),
	} {
		if got, err := ParseSelected([]byte(doc)); err != ErrSelected || got != (Selected{}) {
			t.Fatalf("wrong envelope accepted notification: %+v %v", got, err)
		}
	}
}
