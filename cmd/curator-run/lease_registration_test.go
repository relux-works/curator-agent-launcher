//go:build darwin || linux

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/fakemanager"
)

func setLeaseRegistration(t *testing.T, f *frontFixture, stamp string) {
	t.Helper()
	reg, _, err := sessionmanager.Load(configfile.Resolver{Locations: configfile.Locations{System: f.bin.system, User: f.config}})
	if err != nil {
		t.Fatal(err)
	}
	entry := reg.Managers["fake"]
	entry.RegisteredAt = stamp
	reg.Managers["fake"] = entry
	raw, err := sessionmanager.Encode(reg)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(f.config, sessionmanager.Filename), raw, 0600)
}

func requireLeaseRegistration(t *testing.T, path, stamp string) client.Pending {
	t.Helper()
	var p client.Pending
	if json.Unmarshal(readWSFile(t, path), &p) != nil || p.Phase != "bind_pending" || p.Manager.RegisteredAt != stamp || p.Lease == nil || p.Lease.Holder.ManagerRegisteredAt != stamp || p.Lease.Record == nil || p.Lease.Record.Holder.ManagerRegisteredAt != stamp || p.Lease.Record.Phase != "prepared" {
		t.Fatal("REGISTRATION-BIND ALARM: exact journal/holder identity or prepared pending bind lost")
	}
	return p
}

func requireRegistrationLiveness(t *testing.T, w *wsFixture, stamp, stderr, alarm string) string {
	t.Helper()
	data, err := os.ReadFile(w.path(".json"))
	var live client.Liveness
	if err != nil || json.Unmarshal(data, &live) != nil || live.Manager.RegisteredAt != stamp || live.Session == "" || !strings.Contains(stderr, "session="+live.Session) {
		t.Fatalf("%s: truthful liveness with exact registration missing: %v; stderr=%s", alarm, err, stderr)
	}
	p := requireLeaseRegistration(t, w.path(".op.json"), stamp)
	if live.Manager != p.Manager || live.OperationID == nil || *live.OperationID != p.OperationID || w.observe().Starts != 1 {
		t.Fatalf("%s: liveness/journal identity or single start changed", alarm)
	}
	for _, op := range w.leaseCalls() {
		if op == "bind" || op == "release" || op == "break" {
			t.Fatalf("forbidden lease effect: %s", op)
		}
	}
	return live.Session
}

// HL21/HL22 / L1/L2: real Werkstatt starts and automatic status recovery retain
// whole/1-digit/9-digit registration bytes and the same handle/pending bind.
// Plain hosted is the cross-route control; all remain local bind refusals.
func TestLeaseRegistrationBinary(t *testing.T) {
	bin := frontBuild(t)
	for _, row := range []struct{ name, stamp string }{
		{"whole", "2026-01-01T00:00:00Z"},
		{"fraction-1", "2026-01-01T00:00:00.1Z"},
		{"fraction-9", "2026-01-01T00:00:00.123456789Z"},
		{"fraction-zeroes", "2026-01-01T00:00:00.100000000Z"},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Run("werkstatt", func(t *testing.T) {
				w := newWS(t, bin)
				setLeaseRegistration(t, w.frontFixture, row.stamp)
				code, _, stderr := w.invokeWS("up")
				if code != 1 || !strings.Contains(stderr, "reason=process_incarnation_unavailable") || w.observe().Starts != 1 {
					t.Fatalf("POST-START-REGISTRATION ALARM: expected one started session at bind gate, exit=%d stderr=%s starts=%d", code, stderr, w.observe().Starts)
				}
				handle := requireRegistrationLiveness(t, w, row.stamp, stderr, "POST-START-REGISTRATION ALARM")
				code, _, stderr = w.invokeWS("status")
				if code != 1 || !strings.Contains(stderr, "reason=process_incarnation_unavailable") || requireRegistrationLiveness(t, w, row.stamp, stderr, "STATUS-REGISTRATION ALARM") != handle || !slices.Equal(w.calls(), []string{"describe", "list", "launch", "status"}) {
					t.Fatalf("STATUS-REGISTRATION ALARM: recovery changed handle, submitted or attached, exit=%d stderr=%s calls=%v", code, stderr, w.calls())
				}
				// Equal instants with distinct valid spellings are distinct registration
				// identities. Refuse before manager RPC and retain both durable records.
				if row.name == "fraction-1" {
					beforeLive, beforePending := readWSFile(t, w.path(".json")), readWSFile(t, w.path(".op.json"))
					setLeaseRegistration(t, w.frontFixture, "2026-01-01T00:00:00.100000000Z")
					code, _, stderr = w.invokeWS("status")
					if code != 1 || !strings.Contains(stderr, "werkstatt_operation_unresolved") || !strings.Contains(stderr, "reason=manager_changed") || !slices.Equal(w.calls(), []string{"describe", "list", "launch", "status"}) || !slices.Equal(beforeLive, readWSFile(t, w.path(".json"))) || !slices.Equal(beforePending, readWSFile(t, w.path(".op.json"))) || w.observe().Starts != 1 {
						t.Fatal("changed registration was normalized, adopted or rewritten", code, stderr)
					}
				}
			})
			t.Run("plain", func(t *testing.T) {
				f := newFront(t, bin)
				setLeaseRegistration(t, f, row.stamp)
				code, _, stderr := f.invoke(false, false, "--hosted", "--detach")
				if code != 1 || !strings.Contains(stderr, "reason=process_incarnation_unavailable") || f.observe().Starts != 1 {
					t.Fatalf("PLAIN-REGISTRATION ALARM: exit=%d stderr=%s starts=%d", code, stderr, f.observe().Starts)
				}
				paths, err := filepath.Glob(filepath.Join(f.state, "swarma", "launcher", "hosted", "*.op.json"))
				if err != nil || len(paths) != 1 {
					t.Fatal("plain journal missing", err)
				}
				requireLeaseRegistration(t, paths[0], row.stamp)
				for _, op := range f.leaseCalls() {
					if op == "bind" || op == "release" || op == "break" {
						t.Fatalf("forbidden lease effect: %s", op)
					}
				}
				if !slices.Equal(f.control(fakemanager.FixtureRequest{Operation: "observe_calls"}).CallHistory.Calls, []string{"describe", "launch"}) {
					t.Fatal("plain admission submitted again or attached")
				}
			})
		})
	}
}

// HL21/HL22 / L1/L2: a fixture-only blocked liveness write leaves a real start
// at possibly_submitted. Public status, after removing only that I/O blocker,
// writes exact fractional identity and bind_pending without a second start.
func TestLeaseRegistrationStatusRecoveryBinary(t *testing.T) {
	bin := frontBuild(t)
	for _, row := range []struct{ name, stamp string }{
		{"whole", "2026-01-01T00:00:00Z"},
		{"fraction-1", "2026-01-01T00:00:00.1Z"},
		{"fraction-9", "2026-01-01T00:00:00.123456789Z"},
	} {
		t.Run(row.name, func(t *testing.T) {
			w := newWS(t, bin)
			setLeaseRegistration(t, w.frontFixture, row.stamp)
			// Install the I/O blocker only after successful manager admission.
			wrapper := filepath.Join(w.root, "manager-wrapper")
			quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
			script := "#!/bin/sh\nreply=$(" + quote(bin.manager) + " \"$@\")\nresult=$?\nif [ \"$1\" = launch ] && [ \"$result\" = 0 ]; then /bin/mkdir " + quote(w.path(".json")) + " || exit 2; fi\nprintf '%s' \"$reply\"\nexit \"$result\"\n"
			writeFixture(t, wrapper, []byte(script), 0700)
			reg, _, err := sessionmanager.Load(configfile.Resolver{Locations: configfile.Locations{System: bin.system, User: w.config}})
			if err != nil {
				t.Fatal(err)
			}
			entry := reg.Managers["fake"]
			entry.Executable = wrapper
			reg.Managers["fake"] = entry
			raw, err := sessionmanager.Encode(reg)
			if err != nil {
				t.Fatal(err)
			}
			writeFixture(t, filepath.Join(w.config, sessionmanager.Filename), raw, 0600)
			code, _, stderr := w.invokeWS("up")
			var pending client.Pending
			if code != 1 || !strings.Contains(stderr, "werkstatt_liveness_write_failed") || w.observe().Starts != 1 || json.Unmarshal(readWSFile(t, w.path(".op.json")), &pending) != nil || pending.Phase != "possibly_submitted" || pending.Manager.RegisteredAt != row.stamp {
				t.Fatal("post-start I/O failure did not retain truthful recovery state", code, stderr)
			}
			if err := os.Remove(w.path(".json")); err != nil {
				t.Fatal(err)
			}
			code, _, detail := w.invokeWS("status")
			if code != 1 || !strings.Contains(detail, "reason=process_incarnation_unavailable") || w.observe().Starts != 1 {
				t.Fatalf("STATUS-RECOVERY-REGISTRATION ALARM: failed after real start, exit=%d stderr=%s starts=%d", code, detail, w.observe().Starts)
			}
			handle := requireRegistrationLiveness(t, w, row.stamp, detail, "STATUS-RECOVERY-REGISTRATION ALARM")
			if !strings.Contains(stderr, "session="+handle) || !slices.Equal(w.calls(), []string{"describe", "list", "launch", "status"}) {
				t.Fatal("status recovery changed handle, resubmitted or attached", stderr, detail, w.calls())
			}
		})
	}
}
