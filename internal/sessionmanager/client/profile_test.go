//go:build unix

package client

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
)

// Validated launch and resume requests supply journal profile evidence before
// the call; caller-supplied metadata cannot forge it, including after lost replies.
func TestSubmittedProfileEvidence(t *testing.T) {
	for _, kind := range []string{"launch", "resume"} {
		t.Run(kind, func(t *testing.T) {
			tx := transaction(t)
			p := Pending{Profile: "forged", OperationID: operationID, Kind: kind, SubmittedAt: stamp, AdmissionKey: "werkstatt:example", Manager: Binding{ID: "demo", Executable: "/protocol", RegisteredAt: stamp}}
			var request map[string]any
			if json.Unmarshal(document(t, kind), &request) != nil {
				t.Fatal("fixture")
			}
			request["admission_key"] = "werkstatt:example"
			calls := 0
			r := Reconciler{Registry: map[string]sessionmanager.Entry{"demo": {Executable: "/protocol", RegisteredAt: stamp}}, Caller: func(string, sessionmanager.Entry) Caller {
				return callerFunc(func(_ context.Context, op string, _ []byte) (Response, error) {
					calls++
					saved, err := tx.Pending()
					if err != nil || saved == nil || saved.Profile != "sample" {
						t.Fatal("submitted profile not durable before call", saved, err)
					}
					if calls == 1 {
						if op != kind {
							t.Fatal(op)
						}
						return Response{}, unavailable("unreachable")
					}
					if op != "status" {
						t.Fatal("recovery repeated start", op)
					}
					s := session()
					return Response{Outcome: &Outcome{Outcome: "started", Session: &s}}, nil
				})
			}}
			if _, err := r.Submit(bounded(t), tx, p, encoded(request)); err != nil {
				t.Fatal(err)
			}
			l, err := tx.Liveness()
			if err != nil || l == nil || l.Profile != "sample" || calls != 2 {
				t.Fatal("settlement lost request profile", l, err, calls)
			}
			assertPending(t, tx, false)
		})
	}
}

// Same-generation refreshes and operator edits cannot change known or unknown
// profile evidence; a new generation installs its own value.
func TestLivenessProfilePreservation(t *testing.T) {
	for _, profile := range []string{"sample", ""} {
		t.Run("previous-"+profile, func(t *testing.T) {
			tx := transaction(t)
			first := liveFor(operationID, 1)
			first.Profile = profile
			if ok, err := tx.CompareAndSwap(Expected{}, first); err != nil || !ok {
				t.Fatal(ok, err)
			}
			refresh := first
			refresh.Profile = "forged"
			if ok, err := tx.RefreshSession(first.Expected(), refresh); err != nil || !ok {
				t.Fatal(ok, err)
			}
			if ok, err := tx.CompareAndSwap(first.Expected(), refresh); err != nil || !ok {
				t.Fatal(ok, err)
			}
			if ok, err := tx.EditDispatch(first.Expected(), false, nil, func() *int64 { v := int64(3); return &v }()); err != nil || !ok {
				t.Fatal(ok, err)
			}
			l, err := tx.Liveness()
			if err != nil || l.Profile != profile {
				t.Fatal("ordinary write changed profile", l, err)
			}
			if err := tx.Abandon(reconciler(Client{}).now()); err != nil {
				t.Fatal(err)
			}
			l, err = tx.Liveness()
			if err != nil || l.Profile != profile {
				t.Fatal("override changed profile", l, err)
			}
			next := liveFor(secondID, 2)
			next.Profile = "different"
			if ok, err := tx.CompareAndSwap(l.Expected(), next); err != nil || !ok {
				t.Fatal(ok, err)
			}
			l, err = tx.Liveness()
			if err != nil || l.Profile != "different" {
				t.Fatal("new generation profile missing", l, err)
			}
		})
	}
}
