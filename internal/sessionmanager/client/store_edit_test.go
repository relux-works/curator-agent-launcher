//go:build unix

package client

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Binding is part of CAS even when op/gen match. Cross-manager starts take
// that manager's own generation only after confirmed down; metadata survives.
func TestLivenessBindingCAS(t *testing.T) {
	tx := transaction(t)
	first := liveFor(operationID, 3)
	if ok, e := tx.CompareAndSwap(Expected{}, first); e != nil || !ok {
		t.Fatal(ok, e)
	}
	stale := first.Expected()
	otherBinding := first.Manager
	otherBinding.Executable = "/other"
	stale.Manager = &otherBinding
	if ok, e := tx.CompareAndSwap(stale, first); e != nil || ok {
		t.Fatal("stale binding wrote", ok, e)
	}
	next := liveFor(secondID, 1)
	next.Manager = otherBinding
	if ok, e := tx.CompareAndSwap(first.Expected(), next); e != nil || ok {
		t.Fatal("live owner replaced", ok, e)
	}
	first.State = "down"
	if ok, e := tx.RefreshSession(first.Expected(), first); e != nil || !ok {
		t.Fatal(ok, e)
	}
	if ok, e := tx.CompareAndSwap(first.Expected(), next); e != nil || !ok {
		t.Fatal("down owner replacement refused", ok, e)
	}
	if got, e := tx.Liveness(); e != nil || got.Generation != 1 || got.Manager != otherBinding {
		t.Fatal(got, e)
	}
	before := readFile(t, filepath.Join(tx.dir, "example.json"))
	if ok, e := tx.RefreshSession(first.Expected(), first); e != nil || ok {
		t.Fatal("late owner overwrote binding", ok, e)
	}
	if !bytes.Equal(before, readFile(t, filepath.Join(tx.dir, "example.json"))) {
		t.Fatal("late owner changed bytes")
	}
}

// Dispatch/rebind require the exact expected tuple. An override keeps the
// newest sixteen tombstones and prevents late outcomes, while fresh session
// evidence can restore manager state without deleting those tombstones.
func TestLivenessEdits(t *testing.T) {
	tx := transaction(t)
	first := liveFor(operationID, 1)
	if ok, e := tx.EditDispatch(Expected{}, true, nil, nil); e == nil || ok {
		t.Fatal("dispatch invented liveness", ok, e)
	}
	if ok, e := tx.CompareAndSwap(Expected{}, first); e != nil || !ok {
		t.Fatal(ok, e)
	}
	task := "example-task"
	queue := int64(4)
	if ok, e := tx.EditDispatch(first.Expected(), true, &task, &queue); e != nil || !ok {
		t.Fatal(ok, e)
	}
	binding := first.Manager
	binding.RegisteredAt = "2026-02-01T00:00:00Z"
	if ok, e := tx.Rebind(Expected{}, binding); e != nil || ok {
		t.Fatal("stale rebind wrote", ok, e)
	}
	if ok, e := tx.Rebind(first.Expected(), binding); e != nil || !ok {
		t.Fatal(ok, e)
	}
	first.Manager = binding
	for i := range 18 {
		if e := tx.Abandon(time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC)); e != nil {
			t.Fatal(e)
		}
	}
	l, e := tx.Liveness()
	if e != nil || len(l.AbandonedOperations) != 16 || l.AbandonedOperations[0].AbandonedAt != "2026-01-01T00:00:02Z" || l.StateSource != "operator_override" {
		t.Fatal(l, e)
	}
	if ok, e := tx.CompareAndSwap(l.Expected(), first); e != nil || ok {
		t.Fatal("late abandoned outcome wrote", ok, e)
	}
	first.State = "idle"
	if ok, e := tx.RefreshSession(l.Expected(), first); e != nil || !ok {
		t.Fatal("owner evidence refused", ok, e)
	}
	l, e = tx.Liveness()
	if e != nil || l.CurrentTask == nil || *l.CurrentTask != task || l.QueueDepth != 4 || len(l.AbandonedOperations) != 16 || l.StateSource != "manager" {
		t.Fatal(l, e)
	}
}

// Abandon before a settled start writes only the documented null-evidence
// shape, removes the journal, and still fences an outcome for the abandoned id.
func TestPrestartAbandon(t *testing.T) {
	tx := transaction(t)
	p := Pending{OperationID: operationID, Kind: "launch", SubmittedAt: stamp, AdmissionKey: "werkstatt:example", Manager: liveFor(operationID, 1).Manager}
	putPending(t, tx, p)
	if e := tx.Abandon(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); e != nil {
		t.Fatal(e)
	}
	assertPending(t, tx, false)
	b, e := os.ReadFile(filepath.Join(tx.dir, "example.json"))
	if e != nil {
		t.Fatal(e)
	}
	var row map[string]any
	if json.Unmarshal(b, &row) != nil {
		t.Fatal("bad JSON")
	}
	for _, key := range []string{"session", "generation", "operation_id", "started_at", "last_heartbeat_at", "last_activity_at"} {
		if row[key] != nil {
			t.Fatal("invented evidence", row)
		}
	}
	l, e := tx.Liveness()
	if e != nil || l.Manager != p.Manager || l.State != "down" || l.StateSource != "operator_override" {
		t.Fatal(l, e)
	}
	if ok, e := tx.CompareAndSwap(l.Expected(), liveFor(operationID, 1)); e != nil || ok {
		t.Fatal("late outcome wrote", ok, e)
	}
}

// After a pre-start override, either binding can settle at the manager's own
// generation. Stale tuples, zero generations and abandoned outcomes stay fenced.
func TestPrestartSettlement(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "same-binding", true: "new-binding"}[changed], func(t *testing.T) {
			tx := transaction(t)
			p := Pending{OperationID: operationID, Kind: "launch", SubmittedAt: stamp, AdmissionKey: "werkstatt:example", Manager: liveFor(operationID, 1).Manager}
			putPending(t, tx, p)
			if e := tx.Abandon(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); e != nil {
				t.Fatal(e)
			}
			old, e := tx.Liveness()
			if e != nil {
				t.Fatal(e)
			}
			next := liveFor(secondID, 7)
			if changed {
				next.Manager.Executable = "/other"
			}
			before := readFile(t, filepath.Join(tx.dir, "example.json"))
			stale := old.Expected()
			stale.Generation = 1
			abandoned := next
			abandoned.OperationID = &p.OperationID
			zero := next
			zero.Generation = 0
			for _, probe := range []struct {
				expected Expected
				next     Liveness
			}{{stale, next}, {old.Expected(), abandoned}, {old.Expected(), zero}} {
				if ok, e := tx.CompareAndSwap(probe.expected, probe.next); e != nil || ok {
					t.Fatal("prestart fence bypassed", ok, e)
				}
				if !bytes.Equal(before, readFile(t, filepath.Join(tx.dir, "example.json"))) {
					t.Fatal("rejected outcome changed bytes")
				}
			}
			if ok, e := tx.CompareAndSwap(old.Expected(), next); e != nil || !ok {
				t.Fatal("first settled generation refused", ok, e)
			}
			got, e := tx.Liveness()
			if e != nil || got.Generation != 7 || got.Manager != next.Manager || len(got.AbandonedOperations) != 1 {
				t.Fatal("first settlement lost generation/binding/tombstone", got, e)
			}
		})
	}
}

// Dispatch edits cannot change an override-only record, but the same edits
// apply after a session settles. Rejected edits preserve the exact file bytes.
func TestPrestartDispatch(t *testing.T) {
	for _, edit := range []string{"set-task", "clear-task", "queue"} {
		t.Run(edit, func(t *testing.T) {
			tx := transaction(t)
			putPending(t, tx, Pending{OperationID: operationID, Kind: "launch", SubmittedAt: stamp, AdmissionKey: "werkstatt:example", Manager: liveFor(operationID, 1).Manager})
			if e := tx.Abandon(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); e != nil {
				t.Fatal(e)
			}
			old, e := tx.Liveness()
			if e != nil {
				t.Fatal(e)
			}
			task, depth := "sample-task", int64(2)
			var taskValue *string
			var queue *int64
			if edit == "set-task" {
				taskValue = &task
			}
			if edit == "queue" {
				queue = &depth
			}
			before := readFile(t, filepath.Join(tx.dir, "example.json"))
			ok, e := tx.EditDispatch(old.Expected(), edit != "queue", taskValue, queue)
			if ok || !IsCode(e, "werkstatt_not_up") {
				t.Fatal("prestart dispatch accepted", ok, e)
			}
			if !bytes.Equal(before, readFile(t, filepath.Join(tx.dir, "example.json"))) {
				t.Fatal("dispatch changed override")
			}
			next := liveFor(secondID, 1)
			if ok, e := tx.CompareAndSwap(old.Expected(), next); e != nil || !ok {
				t.Fatal(ok, e)
			}
			if edit == "clear-task" {
				if ok, e := tx.EditDispatch(next.Expected(), true, &task, nil); e != nil || !ok {
					t.Fatal(ok, e)
				}
			}
			if ok, e := tx.EditDispatch(next.Expected(), edit != "queue", taskValue, queue); e != nil || !ok {
				t.Fatal("settled dispatch refused", ok, e)
			}
			got, e := tx.Liveness()
			if e != nil {
				t.Fatal(e)
			}
			if edit == "set-task" && (got.CurrentTask == nil || *got.CurrentTask != task) || edit == "clear-task" && got.CurrentTask != nil || edit == "queue" && got.QueueDepth != depth {
				t.Fatal("dispatch not applied", got)
			}
		})
	}
}
