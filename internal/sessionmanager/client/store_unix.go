//go:build unix

package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	managerschema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
	"golang.org/x/sys/unix"
)

type Binding struct {
	ID           string `json:"id"`
	Executable   string `json:"executable"`
	RegisteredAt string `json:"registered_at"`
}
type Pending struct {
	OperationID  string  `json:"operation_id"`
	Kind         string  `json:"kind"`
	SubmittedAt  string  `json:"submitted_at"`
	AdmissionKey string  `json:"admission_key"`
	Manager      Binding `json:"manager"`
}
type Abandoned struct {
	OperationID string `json:"operation_id"`
	AbandonedAt string `json:"abandoned_at"`
}
type Liveness struct {
	ID                  string      `json:"id"`
	State               string      `json:"state"`
	Manager             Binding     `json:"manager"`
	OperationID         *string     `json:"operation_id"`
	StateSource         string      `json:"state_source"`
	Session             string      `json:"session"`
	Generation          int64       `json:"generation"`
	StartedAt           string      `json:"started_at"`
	LastHeartbeatAt     string      `json:"last_heartbeat_at"`
	LastActivityAt      *string     `json:"last_activity_at"`
	Doorbell            string      `json:"doorbell"`
	DoorbellUncertain   bool        `json:"doorbell_uncertain"`
	CurrentTask         *string     `json:"current_task"`
	QueueDepth          int64       `json:"queue_depth"`
	AbandonedOperations []Abandoned `json:"abandoned_operations"`
}
type Expected struct {
	OperationID *string
	Generation  int64
}

func (l *Liveness) Expected() Expected {
	if l == nil {
		return Expected{}
	}
	return Expected{l.OperationID, l.Generation}
}
func sameOperation(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

type Store struct{ Directory string }

// Transaction is an exclusive, caller-owned handle, not shared between goroutines.
// Pending and liveness mutations cannot be performed without this handle.
type Transaction struct {
	id, dir string
	lock    *os.File
}

var werkstattID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)

func storeError(id string) *Error {
	if !werkstattID.MatchString(id) {
		return refusal("werkstatt_record_invalid", nil)
	}
	return refusal("werkstatt_record_invalid", map[string]string{"id": id})
}
func (s Store) Lock(ctx context.Context, id string) (*Transaction, error) {
	if !werkstattID.MatchString(id) || !filepath.IsAbs(s.Directory) {
		return nil, storeError("")
	}
	if err := os.MkdirAll(s.Directory, 0700); err != nil {
		return nil, storeError(id)
	}
	var parent unix.Stat_t
	if unix.Stat(s.Directory, &parent) != nil || parent.Mode&unix.S_IFMT != unix.S_IFDIR || parent.Uid != uint32(os.Geteuid()) || parent.Mode&0022 != 0 {
		return nil, storeError(id)
	}
	fd, err := unix.Open(filepath.Join(s.Directory, id+".lock"), unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, storeError(id)
	}
	file := os.NewFile(uintptr(fd), "operation lock")
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0022 != 0 || st.Uid != parent.Uid {
		file.Close()
		return nil, storeError(id)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return &Transaction{id: id, dir: s.Directory, lock: file}, nil
		}
		if err != unix.EWOULDBLOCK && err != unix.EAGAIN && err != unix.EINTR {
			file.Close()
			return nil, storeError(id)
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, refusal("werkstatt_busy", map[string]string{"operation": "in_progress"})
		case <-time.After(10 * time.Millisecond):
		}
	}
}
func (t *Transaction) Close() error {
	if t == nil || t.lock == nil {
		return nil
	}
	err := t.lock.Close()
	t.lock = nil
	return err
}
func (t *Transaction) valid() bool { return t != nil && t.lock != nil }
func (t *Transaction) read(suffix string) ([]byte, bool, error) {
	if !t.valid() {
		return nil, false, storeError("")
	}
	fd, err := unix.Open(filepath.Join(t.dir, t.id+suffix), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, storeError(t.id)
	}
	f := os.NewFile(uintptr(fd), "operation state")
	defer f.Close()
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0022 != 0 || st.Uid != uint32(os.Geteuid()) {
		return nil, true, storeError(t.id)
	}
	data, err := io.ReadAll(io.LimitReader(f, Limit+1))
	if err != nil || len(data) > Limit {
		return nil, true, storeError(t.id)
	}
	return data, true, nil
}
func (t *Transaction) Pending() (*Pending, error) {
	data, exists, err := t.read(".op.json")
	if err != nil || !exists {
		return nil, err
	}
	if managerschema.Validate("operation.schema.json", data) != nil {
		return nil, storeError(t.id)
	}
	var p Pending
	_ = json.Unmarshal(data, &p)
	if p.AdmissionKey != "werkstatt:"+t.id {
		return nil, storeError(t.id)
	}
	return &p, nil
}
func (t *Transaction) WritePending(p Pending) error {
	if !t.valid() {
		return storeError("")
	}
	current, err := t.Pending()
	if err != nil {
		return err
	}
	if current != nil {
		return refusal("werkstatt_busy", map[string]string{"operation": "in_progress"})
	}
	data, err := json.Marshal(p)
	if err != nil || managerschema.Validate("operation.schema.json", data) != nil || p.AdmissionKey != "werkstatt:"+t.id {
		return storeError(t.id)
	}
	if sessionmanager.WriteAtomic(filepath.Join(t.dir, t.id+".op.json"), data) != nil {
		return storeError(t.id)
	}
	return nil
}

// clearTerminal cannot be called for intermediate outcomes or unconfirmed cancel.
func (t *Transaction) clearTerminal(operation, outcome string) error {
	if outcome != "started" && outcome != "failed" && outcome != "cancelled" {
		return storeError(t.id)
	}
	p, err := t.Pending()
	if err != nil {
		return err
	}
	if p == nil || p.OperationID != operation {
		return storeError(t.id)
	}
	path := filepath.Join(t.dir, t.id+".op.json")
	data, _ := json.Marshal(p)
	if os.Remove(path) != nil {
		return storeError(t.id)
	}
	if syncDir(t.dir) != nil {
		_ = sessionmanager.WriteAtomic(path, data)
		return storeError(t.id)
	}
	return nil
}
func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func (t *Transaction) Liveness() (*Liveness, error) {
	data, exists, err := t.read(".json")
	if err != nil || !exists {
		return nil, err
	}
	if managerschema.Validate("liveness.schema.json", data) != nil {
		return nil, storeError(t.id)
	}
	var l Liveness
	_ = json.Unmarshal(data, &l)
	if l.ID != t.id {
		return nil, storeError(t.id)
	}
	return &l, nil
}

// CompareAndSwap never orders UUIDs. The exact observed tuple must still hold,
// and only a same-operation refresh or one-generation advance can be committed.
func (t *Transaction) CompareAndSwap(expected Expected, next Liveness) (bool, error) {
	old, err := t.Liveness()
	if err != nil {
		return false, err
	}
	actual := old.Expected()
	if actual.Generation != expected.Generation || !sameOperation(actual.OperationID, expected.OperationID) {
		return false, nil
	}
	if sameOperation(next.OperationID, actual.OperationID) {
		if next.Generation != actual.Generation {
			return false, nil
		}
		if old != nil && next.Manager != old.Manager {
			return false, nil
		}
	} else if next.Generation != actual.Generation+1 {
		return false, nil
	}
	if old != nil {
		for _, a := range old.AbandonedOperations {
			if next.OperationID != nil && a.OperationID == *next.OperationID {
				return false, nil
			}
		}
		next.CurrentTask = old.CurrentTask
		next.QueueDepth = old.QueueDepth
		next.AbandonedOperations = old.AbandonedOperations
	}
	if next.AbandonedOperations == nil {
		next.AbandonedOperations = []Abandoned{}
	}
	data, err := json.Marshal(next)
	if err != nil || next.ID != t.id || managerschema.Validate("liveness.schema.json", data) != nil {
		return false, storeError(t.id)
	}
	if sessionmanager.WriteAtomic(filepath.Join(t.dir, t.id+".json"), data) != nil {
		return false, refusal("werkstatt_liveness_write_failed", map[string]string{"session": next.Session})
	}
	return true, nil
}
