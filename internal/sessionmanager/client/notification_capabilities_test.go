package client

import (
	"encoding/json"
	"slices"
	"testing"
)

// Semantic capability demand follows the validated wire intent, even when the
// caller's stale selection flag disagrees. This does not establish admission.
func TestNotificationRequestCapability(t *testing.T) {
	const capability = "curator-host-notification/1"
	for _, operation := range []string{"launch", "resume"} {
		t.Run(operation, func(t *testing.T) {
			request := notificationCorrelationRequest(t, operation)
			d := delegatedDescription()
			_, err := RequiredForRequest(d, operation, encoded(request), Needs{SignedOff: true})
			if requireCode(t, err, "session_host_capability_missing").Details["capability"] != capability {
				t.Fatal("wire intent did not require its own capability")
			}
			d.Capabilities = append(d.Capabilities, capability)
			r, err := RequiredForRequest(d, operation, encoded(request), Needs{SignedOff: true})
			if err != nil || r.Kind != "delegated" || !slices.Contains(r.Capabilities, capability) {
				t.Fatalf("valid wire intent lost capability: %+v %v", r, err)
			}
			_, err = RequiredForRequest(d, operation, encoded(request), Needs{Native: true, SignedOff: true})
			requireCode(t, err, "launch_plan_invalid")
			request["doorbell"] = map[string]any{"identity": "fixture-agent", "heartbeat": "30s", "max_wakes_per_hour": 1}
			_, err = RequiredForRequest(d, operation, encoded(request), Needs{SignedOff: true})
			requireCode(t, err, "launch_plan_invalid")
			request["doorbell"] = nil
			delete(request, "notification")
			r, err = RequiredForRequest(d, operation, encoded(request), Needs{Notification: true, SignedOff: true})
			if err != nil || slices.Contains(r.Capabilities, capability) {
				t.Fatalf("stale selection invented notification intent: %+v %v", r, err)
			}
			request["notification"] = map[string]any{"backend": "host"}
			_, err = RequiredForRequest(d, operation, encoded(request), Needs{SignedOff: true})
			requireCode(t, err, "launch_plan_invalid")
			// Null and malformed intent are not disabled selections.
			request["notification"] = json.RawMessage("null")
			_, err = RequiredForRequest(d, operation, encoded(request), Needs{SignedOff: true})
			requireCode(t, err, "launch_plan_invalid")
		})
	}
}

// Explicit Host intent requires its own advertised capability and the existing
// delegated lease route. The old doorbell capability cannot satisfy it; disabled
// selection preserves legacy routing. This tests pure selection, not admission.
func TestNotificationCapabilities(t *testing.T) {
	const capability = "curator-host-notification/1"
	for _, resume := range []bool{false, true} {
		name := "launch"
		if resume {
			name = "resume"
		}
		t.Run(name, func(t *testing.T) {
			n := Needs{Runtime: "codex_cli", Notification: true, Resume: resume, SignedOff: true}
			d := delegatedDescription()
			_, err := Required(d, n)
			if requireCode(t, err, "session_host_capability_missing").Details["capability"] != capability {
				t.Fatal("missing notification capability was misidentified")
			}
			d.Capabilities = append(d.Capabilities, capability)
			r, err := Required(d, n)
			if err != nil || r.Kind != "delegated" || !slices.Contains(r.Capabilities, capability) {
				t.Fatalf("valid Host capability refused or omitted: %+v %v", r, err)
			}
			// An unrelated manager/runtime's support cannot qualify this runtime.
			d.Runtimes = map[string][]string{"claude_code": d.Runtimes["codex_cli"]}
			_, err = Required(d, n)
			requireCode(t, err, "session_host_provider_unsupported")
		})
	}
	n := Needs{Runtime: "codex_cli", Notification: true, SignedOff: true}
	d := delegatedDescription()
	d.Capabilities = append(d.Capabilities, capability, "launch-exact/2")
	d.Runtimes["codex_cli"] = append(d.Runtimes["codex_cli"], "launch-exact")
	r, err := Required(d, n)
	if err != nil || r.Kind != "delegated" {
		t.Fatalf("Host intent chose an exact route: %+v %v", r, err)
	}
	n.Notification = false
	_, err = Required(d, n)
	leaseReason(t, err, "route_not_lease_capable")
	n.Native = true
	r, err = Required(Describe{}, n)
	if err != nil || r.Kind != "native" {
		t.Fatalf("ordinary native selection changed: %+v %v", r, err)
	}
}

// A selected Host bridge cannot escape into native or legacy terminal/hook
// ownership, including the old unsigned auto fallback. Nearby trial and disabled
// controls keep the refusal from being satisfied by rejecting every route.
func TestNotificationRoutingRefusals(t *testing.T) {
	d := delegatedDescription()
	d.Capabilities = append(d.Capabilities, "curator-host-notification/1")
	for _, row := range []struct {
		name         string
		n            Needs
		code, reason string
	}{
		{"native-flag", Needs{Native: true}, "launch_plan_invalid", "notification_requires_hosted"},
		{"native-mode", Needs{HostMode: "native"}, "launch_plan_invalid", "notification_requires_hosted"},
		{"old-doorbell", Needs{Doorbell: true}, "launch_plan_invalid", "notification_doorbell_conflict"},
		{"unsigned-auto", Needs{HostMode: "auto"}, "session_host_delegated_unauthorized", ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			n := row.n
			n.Runtime, n.Notification = "codex_cli", true
			_, err := Required(d, n)
			refusal := requireCode(t, err, row.code)
			if row.reason != "" && refusal.Details["reason"] != row.reason {
				t.Fatalf("wrong refusal reason: %+v", refusal)
			}
		})
	}
	n := Needs{Runtime: "codex_cli", Notification: true, HostMode: "auto", DelegatedTrial: true}
	r, err := Required(d, n)
	if err != nil || r.Kind != "delegated" {
		t.Fatalf("admitted trial did not use delegated route: %+v %v", r, err)
	}
	n.Notification, n.DelegatedTrial = false, false
	r, err = Required(d, n)
	if err != nil || r.Kind != "native" || r.Notice != "notice-unselected" {
		t.Fatalf("legacy auto fallback changed: %+v %v", r, err)
	}
}
