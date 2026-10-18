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
