//go:build darwin || linux

package recoveryjournal

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func discoverFixture(t *testing.T) ([]string, Binding, Envelope, string) {
	t.Helper()
	env, b, catalogDir := callerCatalogFixture(t, catalogHead+"default = \"other\"\n"+gatewayDeclaration("other", "/run/other.sock", true)+gatewayDeclaration("original", "/run/original.sock", true), []string{"original"})
	// Resolve only this test-owned filesystem path. No production resolver
	// follows /var or another symlink to make a selected path pass.
	b.base = fixtureBinding(t).base
	env[1] = "XDG_STATE_HOME=" + b.base
	fixtureLayout(t, b)
	n, err := OpenExisting(b)
	if err != nil {
		t.Fatal(err)
	}
	e := fixtureEnvelope()
	if _, err := n.Prepare(bounded(t), e); err != nil {
		t.Fatal(err)
	}
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}
	return env, b, e, catalogDir
}

// Discovery takes routing from only the complete exact original record and
// current admitted catalog; corrupt unrelated records/defaults do not select
// a different attempt. It returns metadata only and preserves proof bytes.
func TestDiscoverBindingExact(t *testing.T) {
	env, b, e, _ := discoverFixture(t)
	before, err := os.ReadFile(finalPath(b, e))
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(b.Directory(), "secrets", strings.Repeat("d", 64)+".json")
	if err := os.WriteFile(other, []byte("corrupt unrelated sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverBinding(bounded(t), env, e.AttemptID)
	if err != nil || got != b {
		t.Fatalf("DISCOVERY ALARM: original exact binding refused: %v", err)
	}
	after, _ := os.ReadFile(finalPath(b, e))
	sentinel, _ := os.ReadFile(other)
	if !bytes.Equal(before, after) || string(sentinel) != "corrupt unrelated sentinel" {
		t.Fatal("exact discovery changed authority or unrelated evidence")
	}
	// The returned metadata alone cannot create a second issuance; the exact
	// namespace/entry is reopened and independently checked by later methods.
	n, err := OpenExisting(got)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	entry, err := n.ReadExact(bounded(t), e.AttemptID, got)
	if err != nil || entry.envelope != e {
		t.Fatal("discovered binding did not reopen exact original entry")
	}
}

// Both documented HOME fallback forms locate the exact existing namespace;
// normalization here resolves only the test-owned macOS temporary HOME.
func TestDiscoverBindingHomeFallback(t *testing.T) {
	for _, emptyXDG := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent_xdg", true: "empty_xdg"}[emptyXDG], func(t *testing.T) {
			env, original, _ := callerCatalogFixture(t, catalogHead+gatewayDeclaration("original", "/run/original.sock", true), []string{"original"})
			home, err := filepath.EvalSymlinks(strings.TrimPrefix(env[0], "HOME="))
			if err != nil {
				t.Fatal(err)
			}
			env = []string{"HOME=" + home}
			if emptyXDG {
				env = append(env, "XDG_STATE_HOME=")
			}
			b, err := ResolveCaller(env, "sha256:"+original.profile, original.socket)
			if err != nil {
				t.Fatal(err)
			}
			fixtureLayout(t, b)
			n, err := OpenExisting(b)
			if err != nil {
				t.Fatal(err)
			}
			e := fixtureEnvelope()
			_, prepErr := n.Prepare(bounded(t), e)
			closeErr := n.Close()
			if prepErr != nil || closeErr != nil {
				t.Fatal("fallback fixture did not prepare")
			}
			got, err := DiscoverBinding(bounded(t), env, e.AttemptID)
			if err != nil || got != b {
				t.Fatalf("valid original HOME fallback refused: %v", err)
			}
		})
	}
}

// Complete record validation precedes catalog admission: malformed envelopes,
// foreign bases, aliases and unsafe files never become usable routing facts.
func TestDiscoverBindingRecordRefusals(t *testing.T) {
	for _, change := range []string{"foreign_base", "bad_proof", "wrong_attempt", "unknown_version", "bad_state", "duplicate_key", "symlink", "hardlink", "permissive", "missing_entry"} {
		t.Run(change, func(t *testing.T) {
			env, b, e, _ := discoverFixture(t)
			path := finalPath(b, e)
			switch change {
			case "foreign_base", "unknown_version", "bad_state", "bad_proof", "wrong_attempt":
				writeRecordFixture(t, b, e, func(f map[string]any) {
					switch change {
					case "foreign_base":
						f["state_base"] = b.base + "-foreign"
					case "unknown_version":
						f["version"] = "unknown"
					case "bad_state":
						f["state"] = "ended-without-receipt"
					case "bad_proof":
						f["envelope"].(map[string]any)["recovery_proof"] = "invalid-private-proof"
					case "wrong_attempt":
						f["envelope"].(map[string]any)["attempt_id"] = strings.Repeat("d", 64)
					}
				})
			case "duplicate_key":
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				raw = append([]byte(`{"version":"netproxy-recovery-secret/1",`), raw[1:]...)
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Rename(path, path+"-target"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+"-target", path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, path+"-alias"); err != nil {
					t.Fatal(err)
				}
			case "permissive":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "missing_entry":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(path)
			got, err := DiscoverBinding(bounded(t), env, e.AttemptID)
			if !errors.Is(err, ErrRead) || got != (Binding{}) {
				t.Fatalf("DISCOVERY ALARM: invalid exact record admitted: %v", err)
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) || strings.Contains(err.Error(), e.RecoveryProof) || strings.Contains(err.Error(), b.base) {
				t.Fatal("record refusal changed evidence or disclosed private inputs")
			}
		})
	}
}

// A complete well-formed private record cannot bypass current catalog
// confirmation, and missing/changed locations never trigger a home search.
func TestDiscoverBindingAdmissionAndLocation(t *testing.T) {
	for _, change := range []string{"unconfirmed", "changed_profile", "missing_state", "invalid_state", "duplicate_state", "bad_attempt", "symlink_state"} {
		t.Run(change, func(t *testing.T) {
			env, b, e, catDir := discoverFixture(t)
			switch change {
			case "unconfirmed":
				if err := os.Remove(filepath.Join(catDir, "network.confirmations.json")); err != nil {
					t.Fatal(err)
				}
			case "changed_profile":
				raw, err := os.ReadFile(filepath.Join(catDir, "network.toml"))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(catDir, "network.toml"), bytes.ReplaceAll(raw, []byte("/run/original.sock"), []byte("/run/replaced.sock")), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing_state":
				env[1] += "-absent"
			case "invalid_state":
				env[1] = "XDG_STATE_HOME=relative"
			case "duplicate_state":
				env = append(env, env[1])
			case "bad_attempt":
				e.AttemptID = "../foreign"
			case "symlink_state":
				if err := os.Symlink(b.base, b.base+"-link"); err != nil {
					t.Fatal(err)
				}
				env[1] += "-link"
			}
			got, err := DiscoverBinding(bounded(t), env, e.AttemptID)
			if err == nil || got != (Binding{}) {
				t.Fatalf("DISCOVERY ADMISSION ALARM: invalid original binding admitted: %v", err)
			}
			if (change == "unconfirmed" || change == "changed_profile") && !errors.Is(err, ErrBindingChanged) {
				t.Fatal("wrong current-admission refusal")
			}
			if change == "missing_state" {
				if _, err := os.Lstat(b.base + "-absent"); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("discovery created alternate state")
				}
			}
		})
	}
}

// Cancellation/lock contention returns no binding and leaves the original
// record intact. A location-only namespace cannot prepare new authority.
func TestDiscoverBindingLockAndUnboundMutation(t *testing.T) {
	env, b, e, _ := discoverFixture(t)
	for _, ctx := range []context.Context{context.Background(), func() context.Context { c, cancel := context.WithCancel(context.Background()); cancel(); return c }()} {
		if got, err := DiscoverBinding(ctx, env, e.AttemptID); !errors.Is(err, ErrLock) || got != (Binding{}) {
			t.Fatal("unbounded or cancelled discovery admitted")
		}
	}
	n, err := openLocation(location{b.base})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	other := e
	other.AttemptID = strings.Repeat("e", 64)
	if result, err := n.Prepare(bounded(t), other); !errors.Is(err, ErrBinding) || result != nil {
		t.Fatal("unbound location authorized private prepare")
	}
	if _, err := os.Lstat(finalPath(b, other)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unbound location wrote authority")
	}
	lock, err := n.lock(bounded(t))
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(lock)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if got, err := DiscoverBinding(ctx, env, e.AttemptID); !errors.Is(err, ErrLock) || got != (Binding{}) {
		t.Fatal("held lock did not bound discovery")
	}
}
