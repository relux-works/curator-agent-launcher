package network_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/network"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
)

// R3-5: the settings check stops on a cancelled context and on a spent time
// budget, with a refusal and no gateway call; the same request with a live
// context and the default budget is admitted (the control). The budget is
// checked between file operations: one system call that stalls in the
// kernel is not interrupted, which no test here claims.
func TestPrepareGatewayGuardStopsOnCancelAndBudget(t *testing.T) {
	slow := func(path string) (*os.File, error) {
		time.Sleep(40 * time.Millisecond)
		return network.OpenSettingsFile(path)
	}
	for _, tc := range []struct {
		name   string
		cancel bool
		budget time.Duration
		open   func(string) (*os.File, error)
		admit  bool
		detail string
	}{
		{"cancelled during the check", true, 0, nil, false, "interrupted"},
		{"budget spent by slow opens", false, 50 * time.Millisecond, slow, false, "time budget"},
		{"control: slow opens within the default budget", false, 0, slow, true, ""},
		{"control: live context", false, 0, nil, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.open != nil {
				defer network.SetOpenSettingsFile(tc.open)()
			}
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			req.GuardBudget = tc.budget
			writeTestFile(t, filepath.Join(managedHome(req), "cowork_settings.json"), "{}")
			readOnly(t, filepath.Join(managedHome(req), "cowork_settings.json"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				// The held signal arrives while the check runs: the first
				// settings open cancels, and the next step must stop.
				var once sync.Once
				defer network.SetOpenSettingsFile(func(path string) (*os.File, error) {
					once.Do(cancel)
					return network.OpenSettingsFile(path)
				})()
			}
			got, err := network.Prepare(ctx, req)
			if tc.admit {
				if err != nil || got.Lease == nil {
					t.Fatalf("valid control refused: %v", err)
				}
				got.Lease.Finish(context.Background())
				return
			}
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || !strings.Contains(err.Error(), tc.detail) || len(g.calls()) != 0 {
				t.Fatalf("err = %v calls = %v", err, g.calls())
			}
		})
	}
}

// R3-5: a source that is a device, or a FIFO, refuses from the inspection
// alone: the opener is never called for it. A source replaced between the
// inspection and the open (the opener returns another regular file) refuses
// on the descriptor check. A plain project settings file is the control; it
// is opened once.
func TestPrepareGatewayGuardNeverOpensSpecialSources(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stage  func(t *testing.T, path string)
		swap   bool
		admit  bool
		opened bool
	}{
		{"device behind a link", func(t *testing.T, path string) {
			if err := os.Symlink("/dev/zero", path); err != nil {
				t.Fatal(err)
			}
		}, false, false, false},
		{"FIFO", func(t *testing.T, path string) {
			if err := syscall.Mkfifo(path, 0o600); err != nil {
				t.Fatal(err)
			}
		}, false, false, false},
		{"replaced between inspection and open", func(t *testing.T, path string) { writeTestFile(t, path, "{}") }, true, false, true},
		{"control: regular file", func(t *testing.T, path string) { writeTestFile(t, path, "{}") }, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			os.MkdirAll(filepath.Join(req.ProjectRoot, ".claude"), 0o700)
			source := filepath.Join(req.ProjectRoot, ".claude", "settings.local.json")
			tc.stage(t, source)
			other := filepath.Join(t.TempDir(), "other.json")
			writeTestFile(t, other, "{}")
			var mu sync.Mutex
			opened := false
			defer network.SetOpenSettingsFile(func(path string) (*os.File, error) {
				if path == source {
					mu.Lock()
					opened = true
					mu.Unlock()
					if tc.swap {
						return network.OpenSettingsFile(other)
					}
				}
				return network.OpenSettingsFile(path)
			})()
			done := make(chan struct{})
			var got network.Prepared
			var err error
			go func() { got, err = network.Prepare(context.Background(), req); close(done) }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("Prepare blocked")
			}
			if opened != tc.opened {
				t.Fatalf("opened = %v, want %v", opened, tc.opened)
			}
			if tc.admit {
				if err != nil || got.Lease == nil {
					t.Fatalf("valid control refused: %v", err)
				}
				got.Lease.Finish(context.Background())
				return
			}
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || len(g.calls()) != 0 {
				t.Fatalf("err = %v calls = %v", err, g.calls())
			}
		})
	}
}

// R3-5: an explicitly selected gateway whose catalog is a FIFO refuses
// network_file_unreadable at once; the explicit path reads the catalog
// through the same non-blocking library read as the default lookup.
func TestPrepareGatewayExplicitFIFOCatalogRefuses(t *testing.T) {
	g := newGateway(t)
	home := gatewayHome(t, g, true)
	catalog := filepath.Join(home, ".curator", "network.toml")
	os.Remove(catalog)
	if err := syscall.Mkfifo(catalog, 0o600); err != nil {
		t.Fatal(err)
	}
	req := gatewayRequest(home)
	req.Prober = forbiddenProber{t}
	done := make(chan error, 1)
	go func() { _, err := network.Prepare(context.Background(), req); done <- err }()
	select {
	case err := <-done:
		if err == nil || refusalCode(t, err) != refusal.CodeFileUnreadable || len(g.calls()) != 0 {
			t.Fatalf("err = %v calls = %v", err, g.calls())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Prepare blocked on the catalog")
	}
}

// R3-5: after registration, a recheck that is slow past its budget, or
// whose context is cancelled (a held signal), refuses; the registration is
// then still ended by Finish (unregister reaches the gateway). An unchanged
// recheck with a live context is the control.
func TestPrepareGatewayRecheckStopsAfterRegister(t *testing.T) {
	for _, tc := range []struct {
		name   string
		slow   bool
		cancel bool
		admit  bool
	}{
		{"slow past the budget", true, false, false},
		{"cancelled", false, true, false},
		{"control: unchanged", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			req.GuardBudget = time.Second
			got, err := network.Prepare(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if tc.slow {
				defer network.SetOpenSettingsFile(func(path string) (*os.File, error) {
					time.Sleep(1100 * time.Millisecond)
					return network.OpenSettingsFile(path)
				})()
			}
			ctx, cancel := context.WithCancel(context.Background())
			if tc.cancel {
				cancel()
			}
			defer cancel()
			err = got.Recheck(ctx)
			got.Lease.Finish(context.Background())
			calls := strings.Join(g.calls(), " ")
			if !strings.HasSuffix(calls, "/v1/unregister") {
				t.Fatalf("calls = %s", calls)
			}
			if tc.admit {
				if err != nil {
					t.Fatalf("control recheck = %v", err)
				}
				return
			}
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict {
				t.Fatalf("recheck = %v", err)
			}
		})
	}
}

// R3-5: the managed drop-ins read by one check are bounded: more than the cap
// refuses before any gateway call, and a few harmless drop-ins are admitted
// (the control).
func TestPrepareGatewayManagedDropInCap(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int
		admit bool
	}{
		{"over the cap", 65, false},
		{"control: a few drop-ins", 3, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			dir := t.TempDir()
			os.MkdirAll(filepath.Join(dir, "managed-settings.d"), 0o700)
			for i := 0; i < tc.count; i++ {
				writeTestFile(t, filepath.Join(dir, "managed-settings.d", strings.Repeat("a", i%5+1)+"-"+strconv.Itoa(i)+".json"), `{"model":"x"}`)
			}
			req.ManagedSettingsDir = dir
			got, err := network.Prepare(context.Background(), req)
			if tc.admit {
				if err != nil || got.Lease == nil {
					t.Fatalf("valid control refused: %v", err)
				}
				got.Lease.Finish(context.Background())
				return
			}
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || len(g.calls()) != 0 {
				t.Fatalf("err = %v calls = %v", err, g.calls())
			}
		})
	}
}

// R4-2: a managed-home selector that is not an absolute ASCII path refuses
// before any settings source is opened (the recheck runs the same guard on
// the same request); the ASCII home built the same way is opened, and its
// undecodable .claude.json is then refused, showing discovery ran (the
// control).
func TestPrepareGatewayRefusesSelectorBeforeReads(t *testing.T) {
	for _, tc := range []struct {
		name  string
		leaf  string
		admit bool
	}{
		{"composed non-ASCII", "café", false},
		{"decomposed non-ASCII", "café", false},
		{"control: ASCII", "cafe", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			home := contractHome(t, tc.leaf)
			writeTestFile(t, filepath.Join(home, ".claude.json"), `{"env":`)
			req.ChildEnv = []string{"CLAUDE_CONFIG_DIR=" + home}
			var mu sync.Mutex
			opens := 0
			defer network.SetOpenSettingsFile(func(path string) (*os.File, error) {
				mu.Lock()
				opens++
				mu.Unlock()
				return network.OpenSettingsFile(path)
			})()
			got, err := network.Prepare(context.Background(), req)
			if tc.admit {
				// The undecodable .claude.json is read and refused: the
				// selector passed and discovery ran.
				if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || opens == 0 {
					t.Fatalf("err = %v opens = %d", err, opens)
				}
				return
			}
			if err == nil || refusalCode(t, err) != refusal.CodeScopeUnsupported || got.Lease != nil || len(g.calls()) != 0 || opens != 0 {
				t.Fatalf("err = %v calls = %v opens = %d", err, g.calls(), opens)
			}
		})
	}
}

// R4-3: the check stops at its last operation too. A cancellation, or a
// spent budget, that arrives during the final policy-path inspection (the
// last metadata operation of the contract) refuses instead of admitting; the
// same inspection without either is admitted (the control).
func TestPrepareGatewayGuardStopsAtTheLastOperation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		act    string
		admit  bool
		detail string
	}{
		{"cancelled during the last inspection", "cancel", false, "interrupted"},
		{"budget spent during the last inspection", "sleep", false, "time budget"},
		{"control: nothing happens", "", true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			req.GuardBudget = time.Second
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			defer network.SetInspectPath(func(path string) (os.FileInfo, error) {
				info, err := os.Lstat(path)
				if strings.HasSuffix(path, "policy-limits.json") {
					switch tc.act {
					case "cancel":
						cancel()
					case "sleep":
						time.Sleep(1100 * time.Millisecond)
					}
				}
				return info, err
			})()
			got, err := network.Prepare(ctx, req)
			if tc.admit {
				if err != nil || got.Lease == nil {
					t.Fatalf("valid control refused: %v", err)
				}
				got.Lease.Finish(context.Background())
				return
			}
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || !strings.Contains(err.Error(), tc.detail) || len(g.calls()) != 0 {
				t.Fatalf("err = %v calls = %v", err, g.calls())
			}
		})
	}
}

// R4-3: the clock is checked between the operations of one source read. A
// cancellation that arrives while a route-bearing settings file is being
// opened refuses as an interruption, before its contents are read; the same
// file without the cancellation refuses on its route (the control).
func TestPrepareGatewayReadStopsBetweenItsSteps(t *testing.T) {
	for _, cancelAtOpen := range []bool{true, false} {
		t.Run(map[bool]string{true: "cancelled at the open", false: "control: read to the end"}[cancelAtOpen], func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			os.MkdirAll(filepath.Join(req.ProjectRoot, ".claude"), 0o700)
			source := filepath.Join(req.ProjectRoot, ".claude", "settings.local.json")
			writeTestFile(t, source, `{"env":{"ANTHROPIC_BASE_URL":"https://x"}}`)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			defer network.SetOpenSettingsFile(func(path string) (*os.File, error) {
				if path == source && cancelAtOpen {
					cancel()
				}
				return network.OpenSettingsFile(path)
			})()
			_, err := network.Prepare(ctx, req)
			want := "set ANTHROPIC_BASE_URL"
			if cancelAtOpen {
				want = "interrupted"
			}
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || !strings.Contains(err.Error(), want) || len(g.calls()) != 0 {
				t.Fatalf("err = %v calls = %v", err, g.calls())
			}
		})
	}
}

// R4-3: a managed drop-in directory is listed with a bound of the cap plus
// one entry, so a huge directory is not read beyond it, and a FIFO in its
// place refuses without blocking. A few drop-ins listed with the same bound
// are admitted (the control).
func TestPrepareGatewayDropInListingIsBounded(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int
		fifo  bool
		admit bool
	}{
		{"many entries", 500, false, false},
		{"FIFO in place of the directory", 0, true, false},
		{"control: a few drop-ins", 3, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateway(t)
			req := gatewayRequest(gatewayHome(t, g, true))
			req.Prober = forbiddenProber{t}
			dir := t.TempDir()
			drop := filepath.Join(dir, "managed-settings.d")
			if tc.fifo {
				if err := syscall.Mkfifo(drop, 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				os.MkdirAll(drop, 0o700)
				for i := 0; i < tc.count; i++ {
					writeTestFile(t, filepath.Join(drop, "d"+strconv.Itoa(i)+".json"), `{"model":"x"}`)
				}
			}
			req.ManagedSettingsDir = dir
			var mu sync.Mutex
			var limits []int
			returned := 0
			defer network.SetListDropIns(func(d string, n int) ([]os.DirEntry, error) {
				entries, err := network.ListDropIns(d, n)
				mu.Lock()
				limits = append(limits, n)
				returned += len(entries)
				mu.Unlock()
				return entries, err
			})()
			done := make(chan struct{})
			var got network.Prepared
			var err error
			go func() { got, err = network.Prepare(context.Background(), req); close(done) }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("Prepare blocked")
			}
			if len(limits) != 1 || limits[0] != 65 || returned > 65 {
				t.Fatalf("listing limits = %v returned = %d", limits, returned)
			}
			if tc.admit {
				if err != nil || got.Lease == nil {
					t.Fatalf("valid control refused: %v", err)
				}
				got.Lease.Finish(context.Background())
				return
			}
			if err == nil || refusalCode(t, err) != refusal.CodeConfigurationConflict || len(g.calls()) != 0 {
				t.Fatalf("err = %v calls = %v", err, g.calls())
			}
		})
	}
}
