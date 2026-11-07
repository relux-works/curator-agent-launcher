//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
	"github.com/relux-works/curator-agent-launcher/internal/werkstatt"
)

// The production preparation path reads selected Core workflow once per
// operation, serializes it in launch/resume, and retains station admission.
// No manager or Source is involved; this does not prove actual admission.
func TestNotificationStationPreparation(t *testing.T) {
	for _, resume := range []bool{false, true} {
		f := entryFixture(t, "claude_code")
		f.usePermissionsFragment(t, "native", false, "profile")
		calls := 0
		f.deps.readSelectedConfig = func(context.Context) (launcherconfig.Selected, error) {
			calls++
			return launcherconfig.Selected{Notification: selectedNotification()}, nil
		}
		r := expectedRecord(f.dir)
		r.ID = "example"
		r.Profile = "default"
		r.ProfileLockSHA256 = strings.Repeat("a", 64)
		r.Model = &f.args[2]
		r.Effort = &f.args[4]
		r.Sandbox = "none"
		r.Identity = "example.dev"
		memory := false
		r.HarnessMemory = &memory
		config := configfile.Resolver{Locations: configfile.Locations{User: filepath.Join(f.dir, "config"), System: filepath.Join(f.dir, "system")}}
		target := ""
		if resume {
			target = "SES-original"
		}
		prepared, err := prepareWerkstatt(context.Background(), r, werkstatt.Command{}, target, io.Discard, config, f.deps, f.deps.environ())
		if err != nil {
			t.Fatal(err)
		}
		if calls != 1 {
			t.Fatal("Core projection was not read exactly once")
		}
		var got map[string]json.RawMessage
		if json.Unmarshal(prepared.Request, &got) != nil {
			t.Fatal("invalid prepared request")
		}
		var n notificationRequest
		if json.Unmarshal(got["notification"], &n) != nil || n.Workflow.Reference != "work-1" || n.Workflow.SHA256 != strings.Repeat("a", 64) || n.Member != r.Notification.Member || n.AdmissionReference != r.Notification.AdmissionReference || n.CallTimeoutMS != 1234 {
			t.Fatalf("station lost expected input: %s", prepared.Request)
		}
		if string(got["admission_key"]) != `"werkstatt:example"` {
			t.Fatal("station admission changed")
		}
		if resume && string(got["selector"]) != `{"handle":"SES-original","kind":"handle"}` {
			t.Fatal("resume replaced requested handle")
		}
		f.assertNoChild(t)
	}
}
