package client

import (
	"reflect"
	"slices"
	"testing"
)

func delegatedDescription() Describe {
	return Describe{Capabilities: []string{"launch-delegated/1", "attach/1", "resume/1", "admission/1", "labels/1", "list/1", "status/1", "stop/1", "board-goal/1", "credential-node/1", "doorbell-codex/1"}, Runtimes: map[string][]string{"codex_cli": {"launch", "attach", "resume", "cancel", "list", "status", "stop"}}}
}

// These are the fifteen independent §5.8 activation cells. Capabilities are
// checked even where the activation result will be native or unauthorized.
func TestActivationMatrix(t *testing.T) {
	rows := []struct {
		name string
		n    Needs
		want [3]string
	}{
		{"explicit", Needs{ExplicitHosted: true}, [3]string{"session_host_delegated_unauthorized", "delegated", "delegated"}},
		{"werkstatt", Needs{Werkstatt: true}, [3]string{"session_host_delegated_unauthorized", "delegated", "delegated"}},
		{"auto", Needs{HostMode: "auto"}, [3]string{"native", "native", "delegated"}},
		{"hosted-default", Needs{HostMode: "hosted"}, [3]string{"session_host_missing", "session_host_missing", "delegated"}},
		{"native", Needs{Native: true}, [3]string{"native", "native", "native"}},
	}
	for _, row := range rows {
		for column, want := range row.want {
			t.Run(row.name+string(rune('0'+column)), func(t *testing.T) {
				n := row.n
				n.Runtime = "codex_cli"
				n.DelegatedTrial = column == 1
				n.SignedOff = column == 2
				r, err := Required(delegatedDescription(), n)
				if want != "native" && want != "delegated" {
					requireCode(t, err, want)
					return
				}
				if err != nil || r.Kind != want {
					t.Fatalf("activation %s: %+v %v", want, r, err)
				}
				if row.name == "auto" && column < 2 && r.Notice != "notice-unselected" {
					t.Fatal("native notice missing")
				}
			})
		}
	}
}

// Required capability checks are per runtime and precede fallback. Unknown
// tokens grant no authority; exact support on another runtime grants none here.
func TestNeededCapabilities(t *testing.T) {
	n := Needs{Runtime: "codex_cli", Werkstatt: true, Resume: true, BoardGoal: true, CredentialNode: true, Doorbell: true, DelegatedTrial: true}
	d := delegatedDescription()
	r, err := Required(d, n)
	want := []string{"admission/1", "attach/1", "board-goal/1", "credential-node/1", "doorbell-codex/1", "labels/1", "list/1", "resume/1", "status/1", "stop/1"}
	if err != nil || !reflect.DeepEqual(r.Capabilities, want) {
		t.Fatal(r, err)
	}
	for _, cap := range want {
		t.Run("missing-"+cap, func(t *testing.T) {
			copy := d
			copy.Capabilities = slices.DeleteFunc(slices.Clone(d.Capabilities), func(v string) bool { return v == cap })
			_, err := Required(copy, n)
			if cap == "admission/1" || cap == "status/1" || cap == "resume/1" {
				leaseReason(t, err, "route_not_lease_capable")
			} else if requireCode(t, err, "session_host_capability_missing").Details["capability"] != cap {
				t.Fatal("wrong capability")
			}
		})
	}
	for _, op := range []string{"resume", "status", "cancel", "attach", "list", "stop"} {
		t.Run("missing-op-"+op, func(t *testing.T) {
			copy := d
			copy.Runtimes = map[string][]string{n.Runtime: slices.DeleteFunc(slices.Clone(d.Runtimes[n.Runtime]), func(v string) bool { return v == op })}
			_, err := Required(copy, n)
			if op == "resume" || op == "status" || op == "cancel" {
				leaseReason(t, err, "route_not_lease_capable")
			} else {
				requireCode(t, err, "session_host_capability_missing")
			}
		})
	}
	// HL21 / route qualification: exact for launch cannot poison JSON resume.
	d.Capabilities = append(d.Capabilities, "launch-exact/2")
	d.Runtimes[n.Runtime] = append(d.Runtimes[n.Runtime], "launch-exact")
	n.Doorbell = false
	if r, e := Required(d, n); e != nil || r.Kind != "delegated" {
		t.Fatal("exact leaked into resume", r, e)
	}
	n.Resume = false
	_, err = Required(d, n)
	leaseReason(t, err, "route_not_lease_capable")
	n.Native = true
	if r, e := Required(Describe{}, n); e != nil || r.Kind != "native" {
		t.Fatal("native requires describe")
	}
	n.Native = false
	n.Runtime = "absent"
	_, err = Required(d, n)
	requireCode(t, err, "session_host_provider_unsupported")
}

// Semantic needs come from the validated request, overriding stale caller
// booleans; selection policy alone controls activation.
func TestRequestDerivedNeeds(t *testing.T) {
	d := delegatedDescription()
	r, err := RequiredForRequest(d, "launch", document(t, "launch"), Needs{DelegatedTrial: true, Runtime: "absent", Detached: true, BoardGoal: true, CredentialNode: true, Doorbell: true, Resume: true})
	if err != nil || !slices.Equal(r.Capabilities, []string{"admission/1", "attach/1", "launch-delegated/1", "status/1"}) {
		t.Fatal("request needs not derived", r, err)
	}
	r, err = RequiredForRequest(d, "resume", document(t, "resume"), Needs{DelegatedTrial: true})
	if err != nil || !slices.Equal(r.Capabilities, []string{"admission/1", "attach/1", "resume/1", "status/1"}) {
		t.Fatal("resume needs", r, err)
	}
	_, err = RequiredForRequest(d, "launch", []byte(`{}`), Needs{})
	requireCode(t, err, "launch_plan_invalid")
}

// HL21 / route qualification: doorbells use delegated recovery-capable wire;
// exact-only and terminal exact routes refuse before acquire or start.
func TestDoorbellRoute(t *testing.T) {
	d := delegatedDescription()
	d.Capabilities = append(d.Capabilities, "launch-exact/2")
	d.Runtimes["codex_cli"] = append(d.Runtimes["codex_cli"], "launch-exact")
	n := Needs{Runtime: "codex_cli", ExplicitHosted: true, DelegatedTrial: true, Doorbell: true}
	r, err := Required(d, n)
	if err != nil || r.Kind != "delegated" {
		t.Fatalf("doorbell route=%s err=%v", r.Kind, err)
	}
	n.Doorbell = false
	r, err = Required(d, n)
	leaseReason(t, err, "route_not_lease_capable")
	n.Doorbell = true
	n.DelegatedTrial = false
	_, err = Required(d, n)
	requireCode(t, err, "session_host_delegated_unauthorized")
	n.DelegatedTrial = true
	d.Runtimes["codex_cli"] = []string{"launch-exact", "attach", "resume"}
	_, err = Required(d, n)
	leaseReason(t, err, "route_not_lease_capable")
}
