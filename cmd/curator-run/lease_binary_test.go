//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/fakemanager"
)

func (f *frontFixture) leaseCalls() []string {
	f.t.Helper()
	b, _ := os.ReadFile(filepath.Join(f.root, "lease-trace.jsonl"))
	var result []string
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var v struct{ Operation string }
		if json.Unmarshal([]byte(line), &v) != nil {
			f.t.Fatal("invalid lease trace")
		}
		result = append(result, v.Operation)
	}
	return result
}
func (w *wsFixture) protectedOperation() {
	w.t.Helper()
	var p client.Pending
	if json.Unmarshal(readWSFile(w.t, w.path(".op.json")), &p) != nil || p.Phase != "bind_pending" || p.Lease == nil || p.Lease.Record == nil || p.Lease.Record.Phase != "prepared" {
		w.t.Fatal("bind correlation or prepared protection lost")
	}
}
func (f *frontFixture) terminalLeases(phase string) {
	f.t.Helper()
	path := filepath.Join(f.root, "lease-ledger.json")
	var records []map[string]any
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &records) != nil {
		f.t.Fatal("lease ledger fixture", err)
	}
	for _, r := range records {
		r["phase"] = phase
		r["revision"] = 2
	}
	writeFixture(f.t, path, frontJSON(records), 0600)
}

// HL21/HL22/HL23 / L1/L2/L3: real launcher and fake executables retain the
// started handle and prepared journal; recovery never starts another session.
// Lost acquire replies replay the same request; terminal replay makes zero RPCs.
func TestLeaseHostedBinary(t *testing.T) {
	bin := frontBuild(t)
	for _, mode := range []string{"fresh", "lost-reply", "released", "broken"} {
		t.Run(mode, func(t *testing.T) {
			f := newFront(t, bin)
			if mode == "lost-reply" {
				writeFixture(t, filepath.Join(f.root, "lease-lost-reply"), []byte("once"), 0600)
			}
			code, _, stderr := f.invoke(false, false, "--hosted", "--detach")
			if mode == "lost-reply" {
				f.require(t, code, 1, stderr, "reason=curator_lease_protocol_error", []string{"describe"}, 0, false)
				code, _, stderr = f.invoke(false, false, "--hosted", "--detach")
				f.require(t, code, 1, stderr, "reason=process_incarnation_unavailable", []string{"describe", "describe", "launch"}, 1, false)
			} else {
				f.require(t, code, 1, stderr, "reason=process_incarnation_unavailable", []string{"describe", "launch"}, 1, false)
			}
			pending, _ := filepath.Glob(filepath.Join(f.state, "swarma", "launcher", "hosted", "*.op.json"))
			if len(pending) != 1 {
				t.Fatal("plain journal missing", pending)
			}
			var p client.Pending
			if json.Unmarshal(readWSFile(t, pending[0]), &p) != nil || p.Phase != "bind_pending" || p.Lease.Record.Phase != "prepared" || !strings.Contains(stderr, "session=") {
				t.Fatal("started outcome falsely reported", stderr)
			}
			before := f.control(fakemanager.FixtureRequest{Operation: "observe_calls"}).CallHistory.Calls
			if mode == "released" || mode == "broken" {
				f.terminalLeases(mode)
				code, _, stderr = f.invoke(false, false, "--hosted", "--detach")
				after := f.control(fakemanager.FixtureRequest{Operation: "observe_calls"}).CallHistory.Calls
				if code != 1 || !strings.Contains(stderr, "reason=already_"+mode) || !slices.Equal(before, after) {
					t.Fatal("HL21 TERMINAL-RPC ALARM", code, stderr, after)
				}
				if _, err := os.Stat(pending[0]); !os.IsNotExist(err) {
					t.Fatal("terminal pending retained", err)
				}
				if _, err := os.Stat(filepath.Join(f.state, "swarma", "launcher", "operations", "settled", p.OperationID+".json")); err != nil {
					t.Fatal("settlement missing", err)
				}
			} else {
				code, _, stderr = f.invoke(false, false, "--hosted", "--detach")
				if code != 1 || !strings.Contains(stderr, "process_incarnation_unavailable") || f.observe().Starts != 1 {
					t.Fatal("recovery resubmitted", code, stderr)
				}
			}
			for _, op := range f.leaseCalls() {
				if op == "bind" {
					t.Fatal("v1 record fabricated process evidence")
				}
			}
		})
	}
}

// HL21/L6 / route qualification: incompatible routes perform one describe and
// zero lease discovery/acquire; rich delegated is the nearby bind-gated control.
func TestLeaseRouteBinary(t *testing.T) {
	bin := frontBuild(t)
	for _, mode := range []string{"missing-admission", "missing-status-op", "exact-rich", "native", "eligible"} {
		t.Run(mode, func(t *testing.T) {
			f := newFront(t, bin)
			tty := false
			args := []string{"--hosted", "--detach"}
			switch mode {
			case "missing-admission":
				f.managerConfig.Capabilities = slices.DeleteFunc(f.managerConfig.Capabilities, func(s string) bool { return s == "admission/1" })
			case "missing-status-op":
				f.managerConfig.Runtimes["claude_code"] = slices.DeleteFunc(slices.Clone(f.managerConfig.Runtimes["claude_code"]), func(s string) bool { return s == "status" })
			case "exact-rich":
				/* Exact discovery uses a describe-only fixture: the conforming fake manager deliberately never advertises unimplemented exact. */
				tty = true
				args = []string{"--hosted"}
			case "native":
				args = []string{"--native"}
			}
			f.reset()
			if mode == "exact-rich" {
				path := filepath.Join(f.root, "bin", "advertise-manager")
				writeFixture(t, path, readWSFile(t, bin.provider), 0700)
				registry, _, err := sessionmanager.Load(configfile.Resolver{Locations: configfile.Locations{System: bin.system, User: f.config}})
				if err != nil {
					t.Fatal(err)
				}
				entry := registry.Managers["fake"]
				entry.Executable = path
				registry.Managers["fake"] = entry
				raw, err := sessionmanager.Encode(registry)
				if err != nil {
					t.Fatal(err)
				}
				writeFixture(t, filepath.Join(f.config, sessionmanager.Filename), raw, 0600)
				d := client.Describe{Protocol: sessionmanager.Protocol, OK: true, ID: "fake", Version: "1.0.0", Capabilities: append(slices.Clone(f.managerConfig.Capabilities), "launch-exact/2"), Runtimes: map[string][]string{"claude_code": append(slices.Clone(f.managerConfig.Runtimes["claude_code"]), "launch-exact")}}
				writeFixture(t, filepath.Join(f.root, "advertise-describe.json"), frontJSON(d), 0600)
			}
			code, _, stderr := f.invoke(tty, false, args...)
			switch mode {
			case "native":
				f.require(t, code, 0, stderr, "", nil, 0, true)
				if len(f.leaseCalls()) != 0 {
					t.Fatal("native invoked lease")
				}
			case "eligible":
				f.require(t, code, 1, stderr, "process_incarnation_unavailable", []string{"describe", "launch"}, 1, false)
			case "exact-rich":
				var trace struct {
					PID  int
					Args []string
				}
				_ = json.Unmarshal(readWSFile(t, filepath.Join(f.root, "advertise-trace.json")), &trace)
				if code != 1 || !strings.Contains(stderr, "route_not_lease_capable") || !slices.Equal(trace.Args, []string{"describe"}) || !errors.Is(syscall.Kill(trace.PID, 0), syscall.ESRCH) || len(f.leaseCalls()) != 0 || f.observe().Starts != 0 {
					t.Fatal("exact fence", code, stderr, trace)
				}
			default:
				f.require(t, code, 1, stderr, "reason=route_not_lease_capable", []string{"describe"}, 0, false)
				if len(f.leaseCalls()) != 0 {
					t.Fatal("incompatible route touched lease API")
				}
			}
		})
	}
}

// HL24 / allocation qualification: an old protected P/HP survives abandon and
// explicit new Q/HQ. Only authoritative terminal disposition permits transition;
// even that positive remains bind-gated and sends no release/break.
func TestLeaseWerkstattAllocationBinary(t *testing.T) {
	bin := frontBuild(t)
	w := newWS(t, bin)
	w.runWS("up", 1, "process_incarnation_unavailable")
	w.protectedOperation()
	first := w.live()
	code, out, stderr := w.command(bin.manager, []string{"stop"}, frontJSON(map[string]any{"session": first.Session, "expected_generation": first.Generation, "mode": "force", "precondition": "none"}), false, false, 10*time.Second)
	if code != 0 {
		t.Fatal(out, stderr)
	}
	_, _, stderr = w.invokeWS("recover", "--abandon-operation")
	if !strings.Contains(stderr, "curator env lease break --lease") || !strings.Contains(stderr, "--if-revision 1") {
		t.Fatal("break hint missing", stderr)
	}
	raw := readWSFile(t, filepath.Join(w.root, "fragment.json"))
	var f map[string]any
	_ = json.Unmarshal(raw, &f)
	f["profile"].(map[string]any)["name"] = "other"
	raw = frontJSON(f)
	frag, err := fragment.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(w.root, "fragment.json"), raw, 0600)
	w.record["profile"] = "other"
	w.write()
	w.managerConfig.Profiles["other"] = fakemanager.Profile{LockSHA256: frag.Profile.LockSHA256, FragmentDigest: frag.Digest}
	// Preserve manager sessions while changing the accepted profile inventory.
	writeFixture(t, filepath.Join(w.state, "curator-run", "fake-manager", "config.json"), frontJSON(w.managerConfig), 0600)
	// Select a distinct manager with independent storage while the old manager's
	// prepared allocation remains in the complete Curator inventory.
	otherState := filepath.Join(w.root, "other-state")
	otherControl := func(request fakemanager.FixtureRequest) fakemanager.FixtureResponse {
		code, out, stderr := w.command(bin.adapter, []string{"--state", otherState}, frontJSON(request), false, false, 10*time.Second)
		var result fakemanager.FixtureResponse
		if code != 0 || json.Unmarshal([]byte(out), &result) != nil || !result.OK {
			t.Fatal("other manager fixture", code, out, stderr)
		}
		return result
	}
	cfg := w.managerConfig
	cfg.ID = "other"
	otherControl(fakemanager.FixtureRequest{Operation: "reset", Config: &cfg})
	t.Cleanup(func() {
		for _, process := range otherControl(fakemanager.FixtureRequest{Operation: "observe_calls"}).CallHistory.CallProcesses {
			for _, pid := range []int{process.PID, -process.Group} {
				if !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
					_ = syscall.Kill(pid, syscall.SIGKILL)
					t.Errorf("other manager child survived: %d", pid)
				}
			}
		}
	})
	registry, _, err := sessionmanager.Load(configfile.Resolver{Locations: configfile.Locations{System: bin.system, User: w.config}})
	if err != nil {
		t.Fatal(err)
	}
	otherExecutable := filepath.Join(w.root, "other-manager")
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	writeFixture(t, otherExecutable, []byte("#!/bin/sh\nexec "+quote(bin.manager)+" --state "+quote(otherState)+" \"$@\"\n"), 0700)
	registry.Managers["other"] = sessionmanager.Entry{Executable: otherExecutable, ArgvPrefix: []string{}, Protocol: client.Protocol, RegisteredAt: "2026-01-02T00:00:00Z", RegisteredBy: "fixture"}
	registry.Selected = "other"
	registryBytes, err := sessionmanager.Encode(registry)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(w.config, sessionmanager.Filename), registryBytes, 0600)
	calls := len(w.leaseCalls())
	w.runWS("up", 1, "reason=allocation_conflict", "--new")
	if slices.Contains(w.leaseCalls()[calls:], "acquire") {
		t.Fatal("reverse conflict acquired another lease")
	}
	w.expectStarts(1)
	w.terminalLeases("broken")
	w.runWS("up", 1, "process_incarnation_unavailable", "--new")
	w.expectStarts(1)
	if otherControl(fakemanager.FixtureRequest{Operation: "observe"}).Observation.Starts != 1 {
		t.Fatal("new manager did not start exactly once after terminal disposition")
	}
	w.protectedOperation()
	if w.live().Profile != "other" || w.live().Manager.ID != "other" {
		t.Fatal("terminal transition refused fresh profile")
	}
}

// HL24 / allocation qualification: competing real launcher processes share the
// fake Curator's ordered acquire/list inventory. An ad-hoc and a Werkstatt
// holder never both submit for one home. This does not qualify a real Curator.
func TestLeaseCompetingBinary(t *testing.T) {
	bin := frontBuild(t)
	w := newWS(t, bin)
	type answer struct {
		code   int
		stderr string
	}
	ready := make(chan struct{})
	results := make(chan answer, 2)
	run := func(invoke func() (int, string, string)) {
		go func() {
			a := answer{code: -1}
			// Goexit from a fixture Fatal must still join this worker, and
			// neither worker may outlive assertions or fixture cleanup.
			defer func() { results <- a }()
			<-ready
			a.code, _, a.stderr = invoke()
		}()
	}
	run(func() (int, string, string) { return w.invoke(false, false, "--hosted", "--detach") })
	run(func() (int, string, string) { return w.invokeWS("up") })
	close(ready)
	answers := []answer{<-results, <-results}
	conflicts := 0
	for _, a := range answers {
		if a.code != 1 {
			t.Fatal("competing admission reported success", a)
		}
		if strings.Contains(a.stderr, "reason=allocation_conflict") {
			conflicts++
		} else if !strings.Contains(a.stderr, "reason=process_incarnation_unavailable") {
			t.Fatal("unrelated admission failure", a)
		}
	}
	if conflicts == 0 || w.observe().Starts > 1 {
		t.Fatal("HL24 competing holders both admitted", conflicts, w.observe().Starts)
	}
}

// HL21 / L1/L5: the public adapter and stateful fake Curator accept identical
// acquire replay but refuse changed holder/digest under that same request id.
// This tests real argv and retained fixture records, not a scripted error alone.
func TestLeaseRequestConflictBinary(t *testing.T) {
	bin := frontBuild(t)
	for _, field := range []string{"digest", "holder"} {
		t.Run(field, func(t *testing.T) {
			f := newFront(t, bin)
			id, err := operationID()
			if err != nil {
				t.Fatal(err)
			}
			binding := client.Binding{ID: "fake", Executable: bin.manager, RegisteredAt: "2026-01-01T00:00:00Z"}
			holder := client.PreparedHolder{Kind: "prepared", Launcher: "curator-run", ManagerID: binding.ID, ManagerRegisteredAt: binding.RegisteredAt, RequestID: id}
			p := client.Pending{Profile: "sample", Manager: binding, Lease: &client.LeaseAdmission{RequestID: id, Environment: "claude_code", LockSHA256: strings.Repeat("1", 64), Holder: holder}}
			leases := client.LeaseCLI{Environment: f.env}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			first, reason, err := leases.Acquire(ctx, p)
			if err != nil || reason != "acquired" {
				t.Fatal("fresh acquire", reason, err)
			}
			replay, reason, err := leases.Acquire(ctx, p)
			if err != nil || reason != "acquired" || !reflect.DeepEqual(first, replay) {
				t.Fatal("identical replay", reason, err)
			}
			if field == "digest" {
				p.Lease.LockSHA256 = strings.Repeat("2", 64)
			} else {
				lane := "other"
				p.Lease.Holder.Werkstatt = &lane
			}
			_, _, err = leases.Acquire(ctx, p)
			var refused *client.Error
			if !errors.As(err, &refused) || refused.Code != "session_home_lease_unavailable" || refused.Details["reason"] != "request_conflict" || refused.Details["curator_code"] != "environment_lease_holder_mismatch" {
				t.Fatal("changed replay admitted or misclassified", err)
			}
			var records []client.LeaseRecord
			if json.Unmarshal(readWSFile(t, filepath.Join(f.root, "lease-ledger.json")), &records) != nil || len(records) != 1 || !reflect.DeepEqual(records[0], *first) {
				t.Fatal("changed replay modified reservation")
			}
			if !slices.Equal(f.leaseCalls(), []string{"acquire", "acquire", "acquire"}) || len(f.control(fakemanager.FixtureRequest{Operation: "observe_calls"}).CallHistory.Calls) != 0 || f.observe().Starts != 0 {
				t.Fatal("changed replay had extra effects")
			}
		})
	}
}
