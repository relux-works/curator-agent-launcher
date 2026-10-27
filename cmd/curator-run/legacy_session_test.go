//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/doorbell"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	schema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
)

// seedLegacySession is setup for legacy-session read/stop/edit regressions. It
// calls the isolated fake plugin directly and stores its validated observed v1
// record. It does NOT call launcher admission or claim a successful leased up.
// Current launch/resume, bind-gate and journal persistence use separate binary
// witnesses. No lease is acquired, bound, released or erased by this setup.
func (w *wsFixture) seedLegacySession(t *testing.T) {
	t.Helper()
	if b, err := os.ReadFile(w.path(".op.json")); err == nil && strings.Contains(string(b), "lease") {
		t.Fatal("legacy setup must not erase lease-bearing correlation")
	}
	raw, err := os.ReadFile(filepath.Join(w.root, "fragment.json"))
	if err != nil {
		t.Fatal(err)
	}
	frag, err := fragment.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := (client.Store{Directory: filepath.Dir(w.path(".json"))}).Lock(context.Background(), w.id)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Close()
	old, err := tx.Liveness()
	if err != nil {
		t.Fatal(err)
	}
	vectors, _ := schema.Documents.ReadFile("vectors/launch.json")
	var cases []struct{ Document map[string]any }
	if json.Unmarshal(vectors, &cases) != nil {
		t.Fatal("legacy vector")
	}
	q := cases[0].Document
	id, err := operationID()
	if err != nil {
		t.Fatal(err)
	}
	q["operation_id"] = id
	q["submitted_at"] = time.Now().UTC().Format(time.RFC3339)
	q["runtime"] = w.record["runtime"]
	q["cwd"] = w.root
	q["admission_key"] = "werkstatt:" + w.id
	q["labels"] = map[string]string{"werkstatt": w.id}
	q["profile"] = map[string]string{"name": frag.Profile.Name, "lock_sha256": frag.Profile.LockSHA256, "fragment_digest": frag.Digest}
	q["model"], q["effort"] = w.record["model"], w.record["effort"]
	q["detach"] = true
	if d, ok := w.record["doorbell"].(map[string]any); ok {
		q["doorbell"] = map[string]any{"identity": w.record["identity"], "heartbeat": d["heartbeat"], "max_wakes_per_hour": d["max_wakes_per_hour"]}
		installer := doorbell.Installer{Environment: w.env, InvocationDirectory: w.root}
		if err = installer.Ensure(context.Background(), w.root, w.record["identity"].(string)); err != nil {
			t.Fatal(err)
		}
	}
	kind := "launch"
	if old != nil && old.State == "down" && old.Session != "" {
		kind = "resume"
		q = map[string]any{"operation_id": id, "submitted_at": q["submitted_at"], "admission_key": q["admission_key"], "runtime": q["runtime"], "profile": q["profile"], "selector": map[string]any{"kind": "handle", "handle": old.Session}, "scope": map[string]any{"cwd": w.root, "labels": map[string]string{"werkstatt": w.id}}, "credential_source": nil, "doorbell": q["doorbell"], "detach": true}
	}
	request := frontJSON(q)
	if err = client.ValidateRequest(kind, request); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := w.command(w.bin.manager, []string{kind}, request, false, false, 15*time.Second)
	response, err := client.DecodeResponse(kind, request, []byte(out), code, "fake")
	if err != nil || response.Session == nil {
		t.Fatalf("legacy plugin fixture: %v %s", err, stderr)
	}
	s := response.Session
	// Preserve metadata when a test deliberately seeds a later legacy generation.
	next := client.Liveness{ID: w.id, Profile: frag.Profile.Name, State: s.State, StateSource: "manager", Manager: client.Binding{ID: "fake", Executable: w.bin.manager, RegisteredAt: "2026-01-01T00:00:00Z"}, OperationID: &s.OperationID, Session: s.Session, Generation: s.Generation, StartedAt: s.StartedAt, LastHeartbeatAt: time.Now().UTC().Format(time.RFC3339), LastActivityAt: s.LastActivityAt, Doorbell: s.Doorbell.State, DoorbellUncertain: s.Doorbell.Uncertain, AbandonedOperations: []client.Abandoned{}}
	if old != nil {
		next.CurrentTask = old.CurrentTask
		next.QueueDepth = old.QueueDepth
		next.AbandonedOperations = old.AbandonedOperations
	}
	if written, e := tx.CompareAndSwap(old.Expected(), next); e != nil || !written {
		t.Fatal("legacy observed record", e)
	}
}

// startThenAbandon explicitly exercises today's bind refusal, followed by a
// deliberate fixture operator override and read-only refresh. It retains the
// settlement and Curator protection. Scenarios using it test subsequent session
// controls, not successful bind or automatic journal clearance.
func (w *wsFixture) startThenAbandon(t *testing.T, args ...string) {
	t.Helper()
	w.admitThenAbandon(t, "up", args...)
}
func (w *wsFixture) admitThenAbandon(t *testing.T, action string, args ...string) {
	t.Helper()
	w.runWS(action, 1, "process_incarnation_unavailable", args...)
	w.protectedOperation()
	var p client.Pending
	_ = json.Unmarshal(readWSFile(t, w.path(".op.json")), &p)
	w.runWS("recover", 0, "", "--abandon-operation")
	if _, err := os.Stat(filepath.Join(w.state, "swarma", "launcher", "operations", "settled", p.OperationID+".json")); err != nil {
		t.Fatal("operator override erased settlement", err)
	}
	w.runWS("status", 0, "")
}
