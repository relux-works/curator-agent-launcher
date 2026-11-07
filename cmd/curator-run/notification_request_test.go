package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
	"github.com/relux-works/curator-agent-launcher/internal/werkstatt"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

func expectedRecord(cwd string) *werkstatt.Record {
	return &werkstatt.Record{Runtime: "claude_code", Profile: "sample", ProfileLockSHA256: strings.Repeat("1", 64), Clone: cwd,
		Notification: &werkstatt.NotificationInput{Member: werkstatt.NotificationMember{MXID: "@example.dev:example.test", AgentSelector: "example.dev", Connection: "fixture-connection", ConfigurationRoot: "/fixture/source"}, AdmissionReference: "retained-record-1", CallTimeoutMS: 1234}}
}
func selectedNotification() *launcherconfig.Notification {
	return &launcherconfig.Notification{Backend: "host", Contract: "curator-host-notification/1", WorkflowReference: "work-1", WorkflowSHA256: strings.Repeat("a", 64)}
}
func notificationFragment() *fragment.Fragment {
	return &fragment.Fragment{Environment: "claude_code", Profile: fragment.Profile{Name: "sample", LockSHA256: strings.Repeat("1", 64)}, Digest: fragment.DigestPrefix + strings.Repeat("2", 64)}
}

// Both operations preserve the explicit member and Core-selected workflow,
// without acquiring station admission or copying record model/environment.
// This is wire preparation only; it proves no Host admission or attachment.
func TestNotificationRequestProjection(t *testing.T) {
	cwd := t.TempDir()
	r := expectedRecord(cwd)
	n, err := notificationFromRecord(selectedNotification(), r, notificationFragment(), cwd, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, resume := range []bool{false, true} {
		intent := agentic.ResumeIntent{Kind: agentic.ResumeNew}
		id := "thread-original"
		if resume {
			intent = agentic.ResumeIntent{Kind: agentic.ResumeCodexThread, Identity: &id}
		}
		inv := cli.Invocation{EnvID: "claude_code", MemberID: "example"}
		op, data, err := delegatedRequest(inv, notificationFragment(), defaults.Resolved{}, execution.PermissionDecision{Mode: agentic.PermissionModeNative, Source: "flag"}, cwd, nil, intent, time.Now(), n)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]json.RawMessage
		if json.Unmarshal(data, &got) != nil {
			t.Fatal("invalid projection")
		}
		want, _ := json.Marshal(n)
		if !reflect.DeepEqual(string(got["notification"]), string(want)) || string(got["admission_key"]) != "null" || string(got["doorbell"]) != "null" {
			t.Fatalf("selection changed admission or expected inputs: %s", data)
		}
		if resume {
			if op != "resume" || string(got["selector"]) != `{"id":"thread-original","kind":"provider_id"}` || got["child_env"] != nil || got["model"] != nil {
				t.Fatalf("resume changed stored facts: %s", data)
			}
		} else if op != "launch" || string(got["child_env"]) != "{}" || string(got["model"]) != "null" {
			t.Fatalf("plain member acquired station inputs: %s", data)
		}
	}
}

// Matching the expected operation is required in addition to wire validity.
// Omitted/off intent remains disabled; malformed/mismatched explicit intent
// cannot yield a partially prepared request. Actual-source checks belong to B.
func TestNotificationRecordOperationRefusals(t *testing.T) {
	cwd := t.TempDir()
	for _, kind := range []string{"valid", "missing-record", "missing-input", "runtime", "profile", "lock", "missing-lock", "workspace", "legacy-doorbell", "record-doorbell", "timeout", "selected-contract"} {
		t.Run(kind, func(t *testing.T) {
			r := expectedRecord(cwd)
			selected := selectedNotification()
			doorbell := false
			switch kind {
			case "missing-record":
				r = nil
			case "missing-input":
				r.Notification = nil
			case "runtime":
				r.Runtime = "codex_cli"
			case "profile":
				r.Profile = "foreign"
			case "lock":
				r.ProfileLockSHA256 = strings.Repeat("3", 64)
			case "missing-lock":
				r.ProfileLockSHA256 = ""
			case "workspace":
				r.Clone = t.TempDir()
			case "legacy-doorbell":
				doorbell = true
			case "record-doorbell":
				r.Doorbell = &werkstatt.Doorbell{Heartbeat: "1m", MaxWakesPerHour: 6}
			case "timeout":
				r.Notification.CallTimeoutMS = 0
			case "selected-contract":
				selected.Contract = "foreign"
			}
			n, err := notificationFromRecord(selected, r, notificationFragment(), cwd, doorbell)
			if kind == "valid" {
				if err != nil || n == nil {
					t.Fatal("valid control refused", err)
				}
			} else if err == nil || n != nil {
				t.Fatalf("%s yielded partial selection", kind)
			}
		})
	}
	for _, selected := range []*launcherconfig.Notification{nil, {Backend: "off"}} {
		if n, err := notificationFromRecord(selected, nil, nil, "", false); err != nil || n != nil {
			t.Fatal("disabled selection changed legacy behavior")
		}
	}
}

// The plain path uses the existing fixed local directory and explicit ID.
// Caller identity environment is not a fallback for absent record selection.
func TestPlainNotificationRecordSelection(t *testing.T) {
	cwd := t.TempDir()
	config := t.TempDir()
	r := expectedRecord(cwd)
	r.Schema = "werkstatt-record-v1"
	r.ID = "example"
	r.Project = "sample"
	r.Role = "developer"
	r.Identity = "example.dev"
	r.Room = "#example"
	r.Orchestrator = "example.orch"
	r.Sandbox = "none"
	r.TrustClass = "standard"
	data, _ := json.Marshal(r)
	if err := os.MkdirAll(filepath.Join(config, "werkstatt"), 0700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(config, "werkstatt", "example.json"), data, 0600)
	deps := launchDeps{configResolver: configfile.Resolver{Locations: configfile.Locations{User: config}}, selectedConfig: launcherconfig.Selected{Notification: selectedNotification()}, workdir: func() (string, error) { return cwd, nil }}
	for _, id := range []string{"example", "", "missing"} {
		n, err := plainNotification(context.Background(), cli.Invocation{MemberID: id}, notificationFragment(), deps)
		if id == "example" {
			if err != nil || n == nil {
				t.Fatal(err)
			}
		} else if err == nil || n != nil {
			t.Fatal("absent selection accepted")
		}
	}
	deps.selectedConfig.Notification = nil
	if n, err := plainNotification(context.Background(), cli.Invocation{MemberID: "example"}, notificationFragment(), deps); err == nil || n != nil {
		t.Fatal("disabled explicit selection silently ignored")
	}
}
