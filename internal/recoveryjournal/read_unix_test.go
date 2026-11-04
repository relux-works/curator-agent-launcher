//go:build darwin || linux

package recoveryjournal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func writeRecordFixture(t *testing.T, b Binding, e Envelope, edit func(map[string]any)) []byte {
	t.Helper()
	raw, err := json.Marshal(secretRecord{SecretVersion, e, b.base, b.profile, b.socket, nil, "prepared", "not_observed"})
	if err != nil {
		t.Fatal(err)
	}
	if edit != nil {
		var fields map[string]any
		if json.Unmarshal(raw, &fields) != nil {
			t.Fatal("fixture decode")
		}
		edit(fields)
		raw, err = json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(finalPath(b, e), raw, 0600); err != nil {
		t.Fatal(err)
	}
	return raw
}

// Recovery loads only the named original entry. Other entries may be corrupt;
// loading is not issuance authorization or service authentication.
func TestReadExactEntry(t *testing.T) {
	b, n := prepareFixture(t)
	e := fixtureEnvelope()
	if _, err := n.Prepare(bounded(t), e); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(finalPath(b, e))
	if err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(b.Directory(), "secrets", strings.Repeat("d", 64)+".json")
	if err := os.WriteFile(sentinel, []byte("unrelated corrupt sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	entry, err := n.ReadExact(bounded(t), e.AttemptID, b)
	if err != nil || entry == nil || entry.AttemptID() != e.AttemptID || entry.envelope != e {
		t.Fatal("EXACT ENTRY ALARM: legitimate original entry refused or changed")
	}
	after, _ := os.ReadFile(finalPath(b, e))
	other, _ := os.ReadFile(sentinel)
	if !bytes.Equal(before, after) || string(other) != "unrelated corrupt sentinel" {
		t.Fatal("read changed evidence")
	}
	for _, format := range []string{"%v", "%+v", "%#v", "%s"} {
		if out := fmt.Sprintf(format, entry); strings.Contains(out, e.RecoveryProof) || strings.Contains(out, e.Root) || strings.Contains(out, b.base) {
			t.Fatal("entry diagnostic leaks authority")
		}
	}
}

// The exact 16KiB legitimate record boundary is accepted, and parallel readers
// of one original attempt retain identical authority without rewriting it.
func TestReadExactBoundaryAndConcurrency(t *testing.T) {
	b, n := prepareFixture(t)
	e := fixtureEnvelope()
	raw := writeRecordFixture(t, b, e, nil)
	e.Boot += strings.Repeat("x", MaxRecord-len(raw))
	raw = writeRecordFixture(t, b, e, nil)
	if len(raw) != MaxRecord {
		t.Fatal("boundary fixture wrong")
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			entry, err := n.ReadExact(ctx, e.AttemptID, b)
			if err != nil || entry == nil || entry.envelope != e {
				t.Error("EXACT ENTRY ALARM: valid boundary or concurrent read refused")
			}
		}()
	}
	wg.Wait()
	after, _ := os.ReadFile(finalPath(b, e))
	if !bytes.Equal(raw, after) {
		t.Fatal("parallel reads modified record")
	}
}

// Closed decoding rejects omission, duplicate/escaped/case-folded keys,
// trailing input and invalid private state; nearby canonical input stays valid.
func TestReadClosedRecord(t *testing.T) {
	for _, kind := range []string{"extra", "omitted", "duplicate", "escaped-duplicate", "case", "nested-duplicate", "nested-extra", "nested-omitted", "wrong-type", "null", "trailing", "array", "version", "state", "outcome", "launch", "envelope", "attempt", "base", "profile", "endpoint", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			b, n := prepareFixture(t)
			e := fixtureEnvelope()
			raw := writeRecordFixture(t, b, e, nil)
			switch kind {
			case "duplicate":
				raw = append([]byte(`{"version":"wrong",`), raw[1:]...)
			case "escaped-duplicate":
				raw = append([]byte(`{"\u0076ersion":"wrong",`), raw[1:]...)
			case "case":
				raw = bytes.Replace(raw, []byte(`"version"`), []byte(`"Version"`), 1)
			case "nested-duplicate":
				raw = bytes.Replace(raw, []byte(`"envelope":{`), []byte(`"envelope":{"root":"/other",`), 1)
			case "trailing":
				raw = append(raw, []byte(` {}`)...)
			case "array":
				raw = append(append([]byte("["), raw...), ']')
			case "oversize":
				raw = append(raw, bytes.Repeat([]byte(" "), MaxRecord)...)
			default:
				raw = writeRecordFixture(t, b, e, func(m map[string]any) {
					switch kind {
					case "extra":
						m["token"] = "forbidden"
					case "omitted":
						delete(m, "launch_id")
					case "nested-extra":
						m["envelope"].(map[string]any)["token"] = "forbidden"
					case "nested-omitted":
						delete(m["envelope"].(map[string]any), "boot")
					case "wrong-type":
						m["envelope"].(map[string]any)["issued_ns"] = 23
					case "null":
						m["envelope"] = nil
					case "version":
						m["version"] = "future"
					case "state":
						m["state"] = "ended"
					case "outcome":
						m["last_outcome"] = "revoked"
					case "launch":
						m["launch_id"] = strings.Repeat("e", 64)
					case "envelope":
						m["envelope"].(map[string]any)["expires_ns"] = "120000000024"
					case "attempt":
						m["envelope"].(map[string]any)["attempt_id"] = strings.Repeat("f", 64)
					case "base":
						m["state_base"] = b.base + "/other"
					case "profile":
						m["profile_digest"] = strings.Repeat("f", 64)
					case "endpoint":
						m["control_endpoint"] = "/other.sock"
					}
				})
			}
			if err := os.WriteFile(finalPath(b, e), raw, 0600); err != nil {
				t.Fatal(err)
			}
			entry, err := n.ReadExact(bounded(t), e.AttemptID, b)
			if entry != nil || !errors.Is(err, ErrRead) {
				t.Fatal("EXACT ENTRY ALARM: malformed or foreign record accepted")
			}
			after, _ := os.ReadFile(finalPath(b, e))
			if !bytes.Equal(raw, after) {
				t.Fatal("bad record repaired or discarded")
			}
			if strings.Contains(err.Error(), e.RecoveryProof) || strings.Contains(err.Error(), b.base) {
				t.Fatal("private refusal leaked")
			}
		})
	}
}

// Unsafe entries, invalid selectors and changed admitted routing refuse without
// following a link, searching alternatives, or modifying an unrelated inode.
func TestReadEntryCustody(t *testing.T) {
	for _, kind := range []string{"missing", "symlink", "hardlink", "mode", "fifo", "directory", "current-base", "current-profile", "current-endpoint", "selector", "closed", "moved", "cancelled", "no-deadline", "held-lock"} {
		t.Run(kind, func(t *testing.T) {
			b, n := prepareFixture(t)
			e := fixtureEnvelope()
			raw := writeRecordFixture(t, b, e, nil)
			path := finalPath(b, e)
			current := b
			attempt := e.AttemptID
			ctx := bounded(t)
			want := ErrRead
			sentinel := filepath.Join(b.StateBase(), "sentinel")
			if err := os.WriteFile(sentinel, raw, 0600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "missing":
				os.Remove(path)
			case "symlink":
				os.Remove(path)
				if err := os.Symlink(sentinel, path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				os.Remove(path)
				if err := os.Link(sentinel, path); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				os.Remove(path)
				if err := unix.Mkfifo(path, 0600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				os.Remove(path)
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "current-base":
				current.base += "/other"
				want = ErrBindingChanged
			case "current-profile":
				current.profile = strings.Repeat("f", 64)
				want = ErrBindingChanged
			case "current-endpoint":
				current.socket = "/other.sock"
				want = ErrBindingChanged
			case "selector":
				attempt = "../sentinel"
				want = ErrRead
			case "closed":
				if err := n.Close(); err != nil {
					t.Fatal(err)
				}
				want = ErrUnsafeLayout
			case "moved":
				if err := os.Rename(b.Directory(), b.Directory()+"-original"); err != nil {
					t.Fatal(err)
				}
				fixtureLayout(t, b)
				want = ErrUnsafeLayout
			case "cancelled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx = c
				want = ErrLock
			case "no-deadline":
				ctx = context.Background()
				want = ErrLock
			case "held-lock":
				fd, err := n.lock(bounded(t))
				if err != nil {
					t.Fatal(err)
				}
				defer unix.Close(fd)
				c, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
				defer cancel()
				ctx = c
				want = ErrLock
			}
			entry, err := n.ReadExact(ctx, attempt, current)
			if entry != nil || !errors.Is(err, want) {
				t.Fatalf("custody refusal wrong: %v", err)
			}
			after, _ := os.ReadFile(sentinel)
			if !bytes.Equal(after, raw) {
				t.Fatal("sentinel changed")
			}
		})
	}
}

// Read/close failures and a namespace/entry replacement while reading retain
// all evidence and return no recovery entry, even after a valid JSON read.
func TestReadEntryInterrupted(t *testing.T) {
	for _, kind := range []string{"read", "close", "replace", "edit", "cancel", "move"} {
		t.Run(kind, func(t *testing.T) {
			b, n := prepareFixture(t)
			e := fixtureEnvelope()
			writeRecordFixture(t, b, e, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ops := nativeReadOperations()
			if kind == "close" {
				ops.close = func(f *os.File) error { f.Close(); return errors.New("private secret") }
			} else {
				ops.read = func(r io.Reader) ([]byte, error) {
					raw, err := io.ReadAll(r)
					switch kind {
					case "read":
						return nil, errors.New("private secret")
					case "replace":
						if err := os.Rename(finalPath(b, e), finalPath(b, e)+".saved"); err != nil {
							t.Fatal(err)
						}
						writeRecordFixture(t, b, e, nil)
					case "edit":
						if err := os.WriteFile(finalPath(b, e), []byte("corrupt"), 0600); err != nil {
							t.Fatal(err)
						}
					case "cancel":
						cancel()
					case "move":
						if err := os.Rename(b.Directory(), b.Directory()+"-original"); err != nil {
							t.Fatal(err)
						}
						fixtureLayout(t, b)
					}
					return raw, err
				}
			}
			entry, err := n.readExact(ctx, e.AttemptID, b, ops)
			if entry != nil || !errors.Is(err, ErrRead) {
				t.Fatal("EXACT ENTRY ALARM: interrupted read accepted")
			}
			if strings.Contains(err.Error(), "private secret") {
				t.Fatal("raw diagnostic leaked")
			}
		})
	}
}
