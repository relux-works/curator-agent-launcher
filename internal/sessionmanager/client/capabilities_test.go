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
	if err != nil || r.Kind != "delegated" {
		t.Fatal(r, err)
	}
	want := []string{"admission/1", "attach/1", "board-goal/1", "credential-node/1", "doorbell-codex/1", "labels/1", "launch-delegated/1", "list/1", "resume/1", "status/1", "stop/1"}
	if !reflect.DeepEqual(r.Capabilities, want) {
		t.Fatalf("needed set %v", r.Capabilities)
	}
	for _, cap := range want {
		t.Run("missing-"+cap, func(t *testing.T) {
			copyD := d
			copyD.Capabilities = slices.DeleteFunc(slices.Clone(d.Capabilities), func(v string) bool { return v == cap })
			copyD.Capabilities = append(copyD.Capabilities, "future/1")
			e := func() error { _, err := Required(copyD, n); return err }()
			if requireCode(t, e, "session_host_capability_missing").Details["capability"] != cap {
				t.Fatal("wrong missing capability")
			}
		})
	}
	for _, op := range d.Runtimes[n.Runtime] {
		t.Run("missing-op-"+op, func(t *testing.T) {
			copyD := d
			copyD.Runtimes = map[string][]string{n.Runtime: slices.DeleteFunc(slices.Clone(d.Runtimes[n.Runtime]), func(v string) bool { return v == op })}
			_, err := Required(copyD, n)
			requireCode(t, err, "session_host_capability_missing")
		})
	}
	n.Doorbell = false // The exact preference below is for a plain request.
	d.Capabilities = append(d.Capabilities, "launch-exact/2")
	d.Runtimes["muse"] = []string{"launch-exact"}
	r, err = Required(d, n)
	if err != nil || r.Kind != "delegated" {
		t.Fatal("foreign runtime exact leaked", r, err)
	}
	d.Runtimes[n.Runtime] = append(d.Runtimes[n.Runtime], "launch-exact")
	r, err = Required(d, n)
	if err != nil || r.Kind != "exact" {
		t.Fatal("exact preference lost", r, err)
	}
	n.DelegatedTrial = false
	r, err = Required(d, n)
	if err != nil || r.Kind != "exact" {
		t.Fatal("exact incorrectly gated", r, err)
	}
	n.Detached = true
	d.Runtimes[n.Runtime] = slices.DeleteFunc(d.Runtimes[n.Runtime], func(v string) bool { return v == "launch" })
	_, err = Required(d, n)
	if requireCode(t, err, "session_host_capability_missing").Details["capability"] != "launch-exact-detached" {
		t.Fatal("detached exact accepted")
	}
	n.Native = true
	_, err = Required(Describe{}, n)
	if err != nil {
		t.Fatal("native requires describe", err)
	}
	n.Native = false
	n.Runtime = "absent"
	_, err = Required(d, n)
	requireCode(t, err, "session_host_provider_unsupported")
	// An automatic mode must not use its native fallback after failed describe.
	_, err = Required(Describe{Runtimes: map[string][]string{"codex_cli": {"launch"}}}, Needs{Runtime: "codex_cli", HostMode: "auto"})
	requireCode(t, err, "session_host_capability_missing")
}

// Semantic needs come from the validated request, overriding stale caller
// booleans; selection policy alone controls activation.
func TestRequestDerivedNeeds(t *testing.T) {
	d := delegatedDescription()
	r, err := RequiredForRequest(d, "launch", document(t, "launch"), Needs{DelegatedTrial: true, Runtime: "absent", Detached: true, BoardGoal: true, CredentialNode: true, Doorbell: true, Resume: true})
	if err != nil || !slices.Equal(r.Capabilities, []string{"attach/1", "launch-delegated/1"}) {
		t.Fatal("request needs not derived", r, err)
	}
	r, err = RequiredForRequest(d, "resume", document(t, "resume"), Needs{DelegatedTrial: true})
	if err != nil || !slices.Equal(r.Capabilities, []string{"attach/1", "launch-delegated/1", "resume/1"}) {
		t.Fatal("resume needs", r, err)
	}
	_, err = RequiredForRequest(d, "launch", []byte(`{}`), Needs{})
	requireCode(t, err, "launch_plan_invalid")
}

// Doorbells select the delegated wire even with exact support. Exact-only
// managers refuse before start, and trial authorization still governs this path.
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
	if err != nil || r.Kind != "exact" {
		t.Fatalf("plain exact control=%s err=%v", r.Kind, err)
	}
	n.Doorbell = true
	n.DelegatedTrial = false
	_, err = Required(d, n)
	requireCode(t, err, "session_host_delegated_unauthorized")
	n.DelegatedTrial = true
	d.Runtimes["codex_cli"] = []string{"launch-exact", "attach", "resume"}
	_, err = Required(d, n)
	requireCode(t, err, "session_host_capability_missing")
	if e, ok := err.(*Error); !ok || e.Details["capability"] != "launch-exact-doorbell" {
		t.Fatal("wrong exact-only refusal", err)
	}
}
