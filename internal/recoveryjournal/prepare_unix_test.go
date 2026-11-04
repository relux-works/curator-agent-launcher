//go:build darwin || linux

package recoveryjournal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func prepareFixture(t *testing.T) (Binding, *Namespace) {
	t.Helper()
	b := fixtureBinding(t)
	fixtureLayout(t, b)
	n, err := OpenExisting(b)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if n.Close() != nil {
			t.Error("namespace close failed")
		}
	})
	return b, n
}

func bounded(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func finalPath(b Binding, e Envelope) string {
	return filepath.Join(b.Directory(), "secrets", e.AttemptID+".json")
}

// Prepared becomes observable only after full checked write, file sync/close,
// exclusive rename and directory sync. It persists E and original routing only.
func TestPrivatePrepareDurabilityOrder(t *testing.T) {
	b, n := prepareFixture(t)
	e := fixtureEnvelope()
	ops := nativePrepareOperations()
	var order []string
	ops.write = func(fd int, p []byte) (int, error) { order = append(order, "write"); return unix.Write(fd, p) }
	ops.sync = func(fd int) error {
		if fd == n.secrets {
			order = append(order, "directory-sync")
		} else {
			order = append(order, "file-sync")
		}
		return unix.Fsync(fd)
	}
	ops.close = func(fd int) error { order = append(order, "close"); return unix.Close(fd) }
	ops.renameNew = func(dir int, old, next string) error {
		order = append(order, "rename")
		return renameNew(dir, old, next)
	}
	p, err := n.prepare(bounded(t), e, ops)
	if err != nil || p == nil || p.AttemptID() != e.AttemptID {
		t.Fatal("valid private prepare did not commit")
	}
	if !reflect.DeepEqual(order, []string{"write", "file-sync", "close", "rename", "directory-sync"}) {
		t.Fatal("private prepare returned out of durability order")
	}
	raw, err := os.ReadFile(finalPath(b, e))
	if err != nil {
		t.Fatal(err)
	}
	var record secretRecord
	if json.Unmarshal(raw, &record) != nil || record.Version != SecretVersion || record.Envelope != e || record.StateBase != b.base || record.ProfileDigest != b.profile || record.ControlSocket != b.socket || record.LaunchID != nil || record.State != "prepared" || record.LastOutcome != "not_observed" {
		t.Fatal("frozen private record projection wrong")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 8 {
		t.Fatal("unexpected private record member")
	}
	before := append([]byte(nil), raw...)
	if p, err := n.Prepare(bounded(t), e); p != nil || !errors.Is(err, ErrExisting) {
		t.Fatal("attempt replay overwrote authority")
	}
	after, _ := os.ReadFile(finalPath(b, e))
	if !bytes.Equal(before, after) {
		t.Fatal("original envelope changed on replay")
	}
}

// Every private publication fault returns no Prepared value. Evidence is kept;
// a directory-sync failure may leave the final record but cannot certify it.
func TestPrivatePreparePublicationFailures(t *testing.T) {
	for _, stage := range []string{"write", "short-write", "file-sync", "close", "rename", "directory-sync"} {
		t.Run(stage, func(t *testing.T) {
			b, n := prepareFixture(t)
			e := fixtureEnvelope()
			ops := nativePrepareOperations()
			injected := false
			failure := errors.New("synthetic-private-diagnostic")
			switch stage {
			case "write":
				ops.write = func(int, []byte) (int, error) { injected = true; return 0, failure }
			case "short-write":
				ops.write = func(fd int, p []byte) (int, error) { injected = true; return unix.Write(fd, p[:len(p)/2]) }
			case "file-sync":
				ops.sync = func(fd int) error {
					if fd != n.secrets {
						injected = true
						return failure
					}
					return unix.Fsync(fd)
				}
			case "close":
				ops.close = func(fd int) error { unix.Close(fd); injected = true; return failure }
			case "rename":
				ops.renameNew = func(int, string, string) error { injected = true; return failure }
			case "directory-sync":
				ops.sync = func(fd int) error {
					if fd == n.secrets {
						injected = true
						return failure
					}
					return unix.Fsync(fd)
				}
			}
			p, err := n.prepare(bounded(t), e, ops)
			if !injected || p != nil || !errors.Is(err, ErrPrepare) || strings.Contains(err.Error(), "synthetic-private") {
				t.Fatal("PRIVATE PREPARE ALARM: failed publication certified or wrong failure")
			}
			entries, err := os.ReadDir(filepath.Join(b.Directory(), "secrets"))
			if err != nil || len(entries) != 1 {
				t.Fatal("incomplete private evidence lost")
			}
			_, err = os.Stat(finalPath(b, e))
			if (err == nil) != (stage == "directory-sync") {
				t.Fatal("private publication stage misreported")
			}
		})
	}
}

// An interrupted writer cannot overwrite an occupied final name or publish a
// swapped temporary inode; unrelated sentinel bytes remain unchanged.
func TestPrivatePrepareNamespaceRaces(t *testing.T) {
	for _, plant := range []string{"occupied-final", "swapped-temp", "symlink-temp", "cancel-write", "moved-namespace"} {
		t.Run(plant, func(t *testing.T) {
			b, n := prepareFixture(t)
			e := fixtureEnvelope()
			ops := nativePrepareOperations()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			sentinel := []byte("untouched sentinel")
			if plant == "occupied-final" {
				ops.renameNew = func(dir int, old, next string) error {
					if err := os.WriteFile(finalPath(b, e), sentinel, 0600); err != nil {
						t.Fatal(err)
					}
					return renameNew(dir, old, next)
				}
			}
			if plant == "cancel-write" {
				ops.write = func(fd int, p []byte) (int, error) { count, err := unix.Write(fd, p); cancel(); return count, err }
			}
			if plant == "swapped-temp" || plant == "symlink-temp" {
				ops.close = func(fd int) error {
					if err := unix.Close(fd); err != nil {
						return err
					}
					entries, err := os.ReadDir(filepath.Join(b.Directory(), "secrets"))
					if err != nil || len(entries) != 1 {
						t.Fatal("missing synthetic temp")
					}
					path := filepath.Join(b.Directory(), "secrets", entries[0].Name())
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
					if plant == "swapped-temp" {
						return os.WriteFile(path, sentinel, 0600)
					}
					target := filepath.Join(b.StateBase(), "sentinel")
					if err := os.WriteFile(target, sentinel, 0600); err != nil {
						return err
					}
					return os.Symlink(target, path)
				}
			}
			if plant == "moved-namespace" {
				if err := os.Rename(b.Directory(), b.Directory()+"-original"); err != nil {
					t.Fatal(err)
				}
				fixtureLayout(t, b)
			}
			p, err := n.prepare(ctx, e, ops)
			want := ErrPrepare
			if plant == "moved-namespace" {
				want = ErrUnsafeLayout
			}
			if p != nil || !errors.Is(err, want) {
				t.Fatal("PRIVATE PREPARE ALARM: namespace/cancellation violation certified")
			}
			if plant == "occupied-final" {
				raw, _ := os.ReadFile(finalPath(b, e))
				if !bytes.Equal(raw, sentinel) {
					t.Fatal("occupied final overwritten")
				}
			}
			if plant == "symlink-temp" {
				raw, _ := os.ReadFile(filepath.Join(b.StateBase(), "sentinel"))
				if !bytes.Equal(raw, sentinel) {
					t.Fatal("linked sentinel modified")
				}
			}
		})
	}
}

// Lock authority is a private regular single-link inode, not a path spelling;
// cancellation is bounded and does not replace a held or unsafe lock.
func TestPrivatePrepareLock(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "permissive", "fifo", "held"} {
		t.Run(kind, func(t *testing.T) {
			b, n := prepareFixture(t)
			path := filepath.Join(b.Directory(), "lock")
			switch kind {
			case "symlink":
				if err := os.Symlink(filepath.Join(b.StateBase(), "unrelated"), path); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := unix.Mkfifo(path, 0600); err != nil {
					t.Fatal(err)
				}
			default:
				if err := os.WriteFile(path, nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "hardlink" {
				if err := os.Link(path, path+"-other"); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "permissive" {
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			}
			want := ErrUnsafeLayout
			if kind == "held" {
				fd, err := unix.Open(path, unix.O_RDWR, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer unix.Close(fd)
				if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
					t.Fatal(err)
				}
				want = ErrLock
			}
			before, _ := os.Lstat(path)
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancel()
			if p, err := n.Prepare(ctx, fixtureEnvelope()); p != nil || !errors.Is(err, want) {
				t.Fatal("unsafe/held lock accepted")
			}
			after, _ := os.Lstat(path)
			if !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatal("lock replaced or repaired")
			}
			entries, err := os.ReadDir(filepath.Join(b.Directory(), "secrets"))
			if err != nil || len(entries) != 0 {
				t.Fatal("refused lock wrote authority")
			}
		})
	}
	_, n := prepareFixture(t)
	if p, err := n.Prepare(context.Background(), fixtureEnvelope()); p != nil || !errors.Is(err, ErrLock) {
		t.Fatal("unbounded lock accepted")
	}
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}
	if p, err := n.Prepare(bounded(t), fixtureEnvelope()); p != nil || !errors.Is(err, ErrUnsafeLayout) {
		t.Fatal("closed namespace reused")
	}
}

// Independent descriptor holders serialize through flock. One original attempt
// commits once; a separate attempt can then commit without cross-record effects.
func TestPrivatePrepareConcurrent(t *testing.T) {
	b, n := prepareFixture(t)
	other, err := OpenExisting(b)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	e := fixtureEnvelope()
	ctx := bounded(t)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, ns := range []*Namespace{n, other} {
		wg.Add(1)
		go func(ns *Namespace) {
			defer wg.Done()
			p, err := ns.Prepare(ctx, e)
			if err == nil && p == nil {
				err = ErrPrepare
			}
			results <- err
		}(ns)
	}
	wg.Wait()
	close(results)
	success, existing := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrExisting) {
			existing++
		} else {
			t.Fatal("unexpected concurrent prepare refusal")
		}
	}
	if success != 1 || existing != 1 {
		t.Fatal("duplicate original preparation")
	}
	e.AttemptID = strings.Repeat("d", 64)
	if p, err := n.Prepare(ctx, e); err != nil || p == nil {
		t.Fatal("independent attempt blocked")
	}
}

// File metadata guards reject foreign owner/type/mode/link count/size. The
// valid control is a private ordinary single-link file, not a directory.
func TestPrivateFilePredicates(t *testing.T) {
	s := unix.Stat_t{Mode: unix.S_IFREG | 0600, Uid: uint32(os.Geteuid()), Nlink: 1, Size: 23}
	if !privateStat(s, 23) {
		t.Fatal("ordinary private file rejected")
	}
	for _, edit := range []func(*unix.Stat_t){func(s *unix.Stat_t) { s.Uid++ }, func(s *unix.Stat_t) { s.Mode = unix.S_IFREG | 0644 }, func(s *unix.Stat_t) { s.Mode = unix.S_IFDIR | 0600 }, func(s *unix.Stat_t) { s.Nlink = 2 }, func(s *unix.Stat_t) { s.Size++ }} {
		bad := s
		edit(&bad)
		if privateStat(bad, 23) {
			t.Fatal("unsafe file accepted")
		}
	}
}

// New preparation respects metadata count/byte quotas and preserves every
// unfinished file. Exact-record decoding and ended-record retention are later.
func TestPrivatePrepareCapacity(t *testing.T) {
	for _, kind := range []string{"count", "oversize-file"} {
		t.Run(kind, func(t *testing.T) {
			b, n := prepareFixture(t)
			count, size := MaxAttempts, 0
			want := ErrCapacity
			if kind == "oversize-file" {
				count, size = 1, MaxRecord+1
				want = ErrUnsafeLayout
			}
			for i := 0; i < count; i++ {
				name := fmt.Sprintf("%064x.json", i)
				if err := os.WriteFile(filepath.Join(b.Directory(), "secrets", name), make([]byte, size), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if p, err := n.Prepare(bounded(t), fixtureEnvelope()); p != nil || !errors.Is(err, want) {
				t.Fatal("new prepare exceeded private capacity")
			}
			entries, err := os.ReadDir(filepath.Join(b.Directory(), "secrets"))
			if err != nil || len(entries) != count {
				t.Fatal("unfinished files evicted")
			}
		})
	}
}

// The 4096-record/16KiB-per-record caller limit admits more than the service's
// separate 16MiB snapshot bound. Each synthetic existing record is valid JSON
// with a valid envelope; this checks admission accounting, not replay parsing.
func TestPrivatePrepareFullCallerBudget(t *testing.T) {
	b, n := prepareFixture(t)
	record := secretRecord{SecretVersion, fixtureEnvelope(), b.base, b.profile, b.socket, nil, "prepared", "not_observed"}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	record.Envelope.Boot += strings.Repeat("x", MaxRecord-len(raw))
	for i := 0; i < MaxAttempts-1; i++ {
		record.Envelope.AttemptID = fmt.Sprintf("%064x", i)
		raw, err = json.Marshal(record)
		if err != nil || len(raw) != MaxRecord || record.Envelope.Validate() != nil {
			t.Fatal("invalid full-budget positive fixture")
		}
		if err := os.WriteFile(finalPath(b, record.Envelope), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if p, err := n.Prepare(bounded(t), fixtureEnvelope()); p == nil || err != nil {
		t.Fatal("CALLER CAPACITY ALARM: legitimate caller budget rejected")
	}
	e := fixtureEnvelope()
	e.AttemptID = strings.Repeat("d", 64)
	if p, err := n.Prepare(bounded(t), e); p != nil || !errors.Is(err, ErrCapacity) {
		t.Fatal("caller record-count cap not enforced")
	}
}
