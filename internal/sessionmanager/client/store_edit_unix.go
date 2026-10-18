//go:build unix

package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	schema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
)

func sameBinding(a, b *Binding) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

// Snapshot is read-only, even while a writer holds the lock. Atomic replacement
// gives status one complete, securely read record; it grants no write authority.
func (s Store) Snapshot(id string) (*Liveness, error) {
	if !werkstattID.MatchString(id) {
		return nil, storeError("")
	}
	data, exists, err := readState(s.Directory, id, ".json")
	if err != nil || !exists {
		return nil, err
	}
	if schema.Validate("liveness.schema.json", data) != nil {
		return nil, storeError(id)
	}
	var l Liveness
	if json.Unmarshal(data, &l) != nil || l.ID != id {
		return nil, storeError(id)
	}
	return &l, nil
}
func (t *Transaction) edit(expected Expected, change func(*Liveness) error) (bool, error) {
	old, err := t.Liveness()
	if err != nil {
		return false, err
	}
	actual := old.Expected()
	if actual.Generation != expected.Generation || !sameOperation(actual.OperationID, expected.OperationID) || !sameBinding(actual.Manager, expected.Manager) {
		return false, nil
	}
	if old == nil {
		return false, storeError(t.id)
	}
	next := *old
	if err := change(&next); err != nil {
		return false, err
	}
	return true, t.writeEdited(next)
}
func (t *Transaction) writeEdited(next Liveness) error {
	if !t.valid() {
		return storeError("")
	}
	data, err := json.Marshal(next)
	if err != nil || schema.Validate("liveness.schema.json", data) != nil || next.ID != t.id {
		return storeError(t.id)
	}
	if sessionmanager.WriteAtomic(filepath.Join(t.dir, t.id+".json"), data) != nil {
		return refusal("werkstatt_liveness_write_failed", nil)
	}
	return nil
}

// EditDispatch and Rebind are explicit operator operations, separate from a
// manager refresh so the latter cannot overwrite task metadata or ownership.
func (t *Transaction) EditDispatch(expected Expected, setTask bool, task *string, queue *int64) (bool, error) {
	return t.edit(expected, func(l *Liveness) error {
		if l.Session == "" {
			return refusal("werkstatt_not_up", map[string]string{"id": t.id})
		}
		if setTask {
			l.CurrentTask = task
		}
		if queue != nil {
			l.QueueDepth = *queue
		}
		return nil
	})
}
func (t *Transaction) Rebind(expected Expected, b Binding) (bool, error) {
	return t.edit(expected, func(l *Liveness) error { l.Manager = b; return nil })
}

// Abandon records an operator decision before durably removing correlation.
// It does not call cancel or establish remote termination.
func (t *Transaction) Abandon(at time.Time) error {
	p, err := t.Pending()
	if err != nil {
		return err
	}
	l, err := t.Liveness()
	if err != nil {
		return err
	}
	var operation string
	if p != nil {
		operation = p.OperationID
	} else if l != nil && l.OperationID != nil {
		operation = *l.OperationID
	}
	if operation == "" {
		return nil
	}
	if l == nil {
		l = &Liveness{ID: t.id, Manager: p.Manager, Doorbell: "off", AbandonedOperations: []Abandoned{}}
	}
	l.State, l.StateSource = "down", "operator_override"
	l.AbandonedOperations = append(l.AbandonedOperations, Abandoned{operation, at.UTC().Format(time.RFC3339)})
	if len(l.AbandonedOperations) > 16 {
		l.AbandonedOperations = l.AbandonedOperations[len(l.AbandonedOperations)-16:]
	}
	if err = t.writeEdited(*l); err != nil {
		return err
	}
	if p != nil {
		path := filepath.Join(t.dir, t.id+".op.json")
		if err = os.Remove(path); err != nil {
			return storeError(t.id)
		}
		if err = syncDir(t.dir); err != nil {
			b, _ := json.Marshal(p)
			_ = sessionmanager.WriteAtomic(path, b)
			return storeError(t.id)
		}
	}
	return nil
}

// MarshalJSON represents the one evidence-free pre-start override without
// inventing a handle, generation or timestamps. All ordinary records retain
// their concrete values and remain subject to the published closed schema.
func (l Liveness) MarshalJSON() ([]byte, error) {
	type plain Liveness
	b, err := json.Marshal(plain(l))
	if err != nil {
		return nil, err
	}
	if l.Session != "" || l.Generation != 0 {
		return b, nil
	}
	var v map[string]any
	_ = json.Unmarshal(b, &v)
	for _, key := range []string{"session", "generation", "started_at", "last_heartbeat_at"} {
		v[key] = nil
	}
	return json.Marshal(v)
}
