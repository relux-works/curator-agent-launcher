//go:build unix

package client

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
)

func liveFor(op string, generation int64) Liveness {
	return Liveness{ID: "example", State: "unknown", Manager: Binding{ID: "demo", Executable: "/protocol", RegisteredAt: stamp}, OperationID: &op, StateSource: "manager", Session: "SES-example", Generation: generation, StartedAt: stamp, LastHeartbeatAt: stamp, Doorbell: "off", AbandonedOperations: []Abandoned{}}
}

// CAS accepts same-operation refresh and exactly one new generation. Stale
// tuples, UUID ordering, skipped generations and abandoned ids cannot write;
// task/queue/abandonment metadata survives a valid manager refresh.
func TestLivenessCAS(t *testing.T) {
	tx := transaction(t)
	first := liveFor(operationID, 1)
	first.CurrentTask = func() *string { s := "TASK-example"; return &s }()
	first.QueueDepth = 2
	first.AbandonedOperations = []Abandoned{{OperationID: "33333333-3333-4333-8333-333333333333", AbandonedAt: stamp}}
	if ok, err := tx.CompareAndSwap(Expected{}, first); err != nil || !ok {
		t.Fatal("initial CAS", ok, err)
	}
	path := filepath.Join(tx.dir, "example.json")
	before := readFile(t, path)
	cases := []struct {
		name     string
		expected Expected
		next     Liveness
	}{
		{"stale-generation", Expected{OperationID: first.OperationID, Generation: 2, Manager: &first.Manager}, liveFor(operationID, 1)},
		{"stale-operation", Expected{OperationID: func() *string { s := secondID; return &s }(), Generation: 1, Manager: &first.Manager}, liveFor(operationID, 1)},
		{"same-op-next-generation", first.Expected(), liveFor(operationID, 2)},
		{"new-op-same-generation", first.Expected(), liveFor(secondID, 1)},
		{"new-op-skipped-generation", first.Expected(), liveFor(secondID, 3)},
		{"abandoned", first.Expected(), liveFor(first.AbandonedOperations[0].OperationID, 2)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := tx.CompareAndSwap(tc.expected, tc.next)
			if err != nil || ok {
				t.Fatal("forbidden CAS", ok, err)
			}
			if !reflect.DeepEqual(before, readFile(t, path)) {
				t.Fatal("forbidden CAS changed file")
			}
		})
	}
	changed := first
	changed.Manager.Executable = "/other"
	if ok, err := tx.CompareAndSwap(first.Expected(), changed); ok || err != nil {
		t.Fatal("same-generation binding changed")
	}
	refresh := liveFor(operationID, 1)
	refresh.State = "idle"
	if ok, err := tx.CompareAndSwap(first.Expected(), refresh); err != nil || !ok {
		t.Fatal("same-op refresh rejected", ok, err)
	}
	next := liveFor(secondID, 2)
	next.Manager.Executable = first.Manager.Executable
	if ok, err := tx.CompareAndSwap(first.Expected(), next); err != nil || !ok {
		t.Fatal("new generation rejected", ok, err)
	}
	got, err := tx.Liveness()
	if err != nil || got.Manager.Executable != first.Manager.Executable || got.QueueDepth != 2 || got.CurrentTask == nil || *got.CurrentTask != "TASK-example" || !reflect.DeepEqual(got.AbandonedOperations, first.AbandonedOperations) {
		t.Fatal("metadata lost", got, err)
	}
	// A late answer for the old generation cannot regress the now-settled pair.
	if ok, err := tx.CompareAndSwap(first.Expected(), first); ok || err != nil {
		t.Fatal("late answer overwrote newer generation")
	}
}

// Exclusive locking blocks a second writer. Closed handles, pre-existing
// journals and nonterminal clears preserve the existing bytes. Symlinked and
// permissively writable state is rejected without touching its target.
func TestStoreGuards(t *testing.T) {
	tx := transaction(t)
	p := Pending{OperationID: operationID, Kind: "launch", SubmittedAt: stamp, AdmissionKey: "werkstatt:example", Manager: Binding{ID: "demo", Executable: "/protocol", RegisteredAt: stamp}}
	putPending(t, tx, p)
	ctx, cancel := context.WithTimeout(bounded(t), 60*time.Millisecond)
	defer cancel()
	_, err := (Store{Directory: tx.dir}).Lock(ctx, "example")
	requireCode(t, err, "werkstatt_busy")
	before := readFile(t, filepath.Join(tx.dir, "example.op.json"))
	other := p
	other.OperationID = secondID
	requireCode(t, tx.WritePending(other), "werkstatt_busy")
	for _, state := range []string{"not_found", "prepared", "admitted", "unknown"} {
		requireCode(t, tx.clearTerminal(operationID, state), "werkstatt_record_invalid")
	}
	requireCode(t, tx.clearTerminal(secondID, "cancelled"), "werkstatt_record_invalid")
	if !reflect.DeepEqual(before, readFile(t, filepath.Join(tx.dir, "example.op.json"))) {
		t.Fatal("journal replaced or cleared")
	}
	if err := tx.clearTerminal(operationID, "cancelled"); err != nil {
		t.Fatal(err)
	}
	assertPending(t, tx, false)
	if err := tx.Close(); err != nil {
		t.Fatal(err)
	}
	requireCode(t, tx.WritePending(p), "werkstatt_record_invalid")
	again, err := (Store{Directory: tx.dir}).Lock(bounded(t), "example")
	if err != nil {
		t.Fatal("closed lock retained", err)
	}
	_ = again.Close()
	for _, target := range []string{"example.lock", "example.op.json", "example.json"} {
		t.Run("symlink-"+target, func(t *testing.T) {
			dir := t.TempDir()
			sentinel := filepath.Join(dir, "target")
			mustWrite(t, sentinel, []byte("sentinel"))
			if err := os.Symlink(sentinel, filepath.Join(dir, target)); err != nil {
				t.Fatal(err)
			}
			lock, err := (Store{Directory: dir}).Lock(bounded(t), "example")
			if target == "example.lock" {
				requireCode(t, err, "werkstatt_record_invalid")
			} else {
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Close()
				if target == "example.op.json" {
					_, err = lock.Pending()
				} else {
					_, err = lock.Liveness()
				}
				requireCode(t, err, "werkstatt_record_invalid")
			}
			if string(readFile(t, sentinel)) != "sentinel" {
				t.Fatal("symlink target touched")
			}
		})
	}
	t.Run("writable-pending", func(t *testing.T) {
		lock := transaction(t)
		putPending(t, lock, p)
		path := filepath.Join(lock.dir, "example.op.json")
		if err := os.Chmod(path, 0666); err != nil {
			t.Fatal(err)
		}
		_, err := lock.Pending()
		requireCode(t, err, "werkstatt_record_invalid")
	})
	t.Run("invalid-pending", func(t *testing.T) {
		lock := transaction(t)
		bad := p
		bad.AdmissionKey = "werkstatt:other"
		requireCode(t, lock.WritePending(bad), "werkstatt_record_invalid")
		assertPending(t, lock, false)
	})
}

// A terminal start remains observable even when its liveness cannot be written;
// its owner-bound journal survives a failed CAS or invalid storage unchanged.
func TestTerminalLivenessFailure(t *testing.T) {
	for _, mode := range []string{"stale", "corrupt-storage"} {
		t.Run(mode, func(t *testing.T) {
			tx := transaction(t)
			p := Pending{OperationID: operationID, Kind: "launch", SubmittedAt: stamp, AdmissionKey: "werkstatt:example", Manager: Binding{ID: "demo", Executable: "/protocol", RegisteredAt: stamp}}
			putPending(t, tx, p)
			first := liveFor(secondID, 2)
			path := filepath.Join(tx.dir, "example.json")
			data := encoded(first)
			if mode == "corrupt-storage" {
				data = []byte(`{}`)
			}
			if err := sessionmanager.WriteAtomic(path, data); err != nil {
				t.Fatal(err)
			}
			s := session()
			got, err := (Reconciler{}).finish(tx, p, Expected{}, Outcome{Outcome: "started", Session: &s})
			requireCode(t, err, "werkstatt_liveness_write_failed")
			if got == nil || got.Session != s.Session {
				t.Fatal("confirmed live handle lost")
			}
			assertPending(t, tx, true)
			if !reflect.DeepEqual(readFile(t, path), data) {
				t.Fatal("failed CAS changed liveness")
			}
		})
	}
}

// A start keeps its exact journal and live handle when the destination or writer
// is obstructed after admission. Repair allows a later invocation to recover by
// status, durably bind liveness, then clear correlation without another launch.
// The unobstructed neighbor persists liveness and clears correlation immediately.
func TestLivenessFailureRecovery(t *testing.T) {
	for _, mode := range []string{"success", "destination-directory", "unwritable-directory"} {
		t.Run(mode, func(t *testing.T) {
			tx := transaction(t)
			p := Pending{OperationID: operationID, Kind: "launch", SubmittedAt: stamp, AdmissionKey: "werkstatt:example", Manager: Binding{ID: "demo", Executable: "/protocol", RegisteredAt: stamp}}
			path := filepath.Join(tx.dir, "example.json")
			journal := filepath.Join(tx.dir, "example.op.json")
			s := session()
			s.State = "idle"
			var saved []byte
			var calls []string
			r := Reconciler{Registry: map[string]sessionmanager.Entry{"demo": {Executable: "/protocol", RegisteredAt: stamp}}}
			r.Caller = func(string, sessionmanager.Entry) Caller {
				return callerFunc(func(_ context.Context, op string, _ []byte) (Response, error) {
					calls = append(calls, op)
					stored, err := tx.Pending()
					if err != nil || stored == nil || *stored != p {
						t.Fatal("remote call lacks durable owner-bound correlation")
					}
					if op == "launch" {
						saved = readFile(t, journal)
						switch mode {
						case "destination-directory":
							if err := os.Mkdir(path, 0700); err != nil {
								t.Fatal(err)
							}
						case "unwritable-directory":
							if err := os.Chmod(tx.dir, 0500); err != nil {
								t.Fatal(err)
							}
							t.Cleanup(func() { _ = os.Chmod(tx.dir, 0700) })
						}
						return Response{Session: &s}, nil
					}
					if op != "status" {
						t.Fatalf("unexpected recovery call: %s", op)
					}
					return Response{Outcome: &Outcome{Outcome: "started", Session: &s}}, nil
				})
			}
			var request map[string]any
			if err := json.Unmarshal(document(t, "launch"), &request); err != nil {
				t.Fatal(err)
			}
			request["admission_key"] = p.AdmissionKey
			got, err := r.Submit(bounded(t), tx, p, encoded(request))
			if mode != "success" {
				e := requireCode(t, err, "werkstatt_liveness_write_failed")
				if e.ExitCode() != 1 || e.Details["session"] != s.Session {
					t.Fatal("live-handle diagnostic lost", e)
				}
				retained, readErr := os.ReadFile(journal)
				if readErr != nil || !reflect.DeepEqual(saved, retained) {
					t.Fatal("failed liveness persistence lost journal")
				}
				if mode == "destination-directory" {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				} else if err := os.Chmod(tx.dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := tx.Close(); err != nil {
					t.Fatal(err)
				}
				tx, err = (Store{Directory: tx.dir}).Lock(bounded(t), "example")
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Close()
				got, err = r.Recover(bounded(t), tx)
			}
			if err != nil || got == nil || got.Session != s.Session {
				t.Fatal("settled live handle lost", got, err)
			}
			live, err := tx.Liveness()
			if err != nil || live == nil || live.Manager != p.Manager || live.OperationID == nil || *live.OperationID != p.OperationID || live.Generation != s.Generation || live.Session != s.Session || live.State != "idle" {
				t.Fatal("settled owner-bound liveness missing", live, err)
			}
			assertPending(t, tx, false)
			want := []string{"launch"}
			if mode != "success" {
				want = append(want, "status")
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatal("start submitted again", calls)
			}
		})
	}
}
