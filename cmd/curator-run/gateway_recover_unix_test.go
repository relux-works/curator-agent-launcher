//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/netproxy"
	"github.com/relux-works/curator-agent-launcher/internal/recoveryjournal"
	"github.com/relux-works/curator-network-profiles/pkg/netprofile"
)

type recoveryCommandFixture struct {
	parent                           []string
	binding                          recoveryjournal.Binding
	envelope                         recoveryjournal.Envelope
	catalog, ledger, secret, receipt string
	health, recover, unexpected      atomic.Int32
	foreign, malformed               atomic.Bool
}

func newRecoveryCommandFixture(t *testing.T) *recoveryCommandFixture {
	t.Helper()
	// The short socket and every directory belong only to this fixture. Resolve
	// the OS temporary-directory alias here, never in production discovery.
	dir, err := os.MkdirTemp("", "gateway-recover-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "s")
	f := &recoveryCommandFixture{}
	f.parent = []string{"HOME=" + dir, "XDG_STATE_HOME=" + filepath.Join(dir, "state")}
	p, err := netprofile.Normalize("original", netprofile.Input{Kind: netprofile.KindFilteringGateway, Control: socket, SensitiveEgress: true})
	if err != nil {
		t.Fatal(err)
	}
	f.binding, err = recoveryjournal.ResolveCaller(f.parent, netprofile.Digest(p), socket)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(dir, ".curator"), filepath.Join(f.binding.Directory(), "secrets"), filepath.Join(f.binding.Directory(), "receipts")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	f.catalog = filepath.Join(dir, ".curator", "network.toml")
	f.ledger = filepath.Join(dir, ".curator", "network.confirmations.json")
	// A healthy different current default must not replace the original profile.
	doc := fmt.Sprintf("schema = \"relux-network-profiles-v1\"\ndefault = \"other\"\n[networks.other]\nkind = \"filtering-gateway\"\ncontrol = \"/run/other.sock\"\nsensitive_egress = true\n[networks.original]\nkind = \"filtering-gateway\"\ncontrol = %q\nsensitive_egress = true\n", socket)
	ledger, err := json.Marshal(map[string]any{"schema": "relux-network-confirmations-v1", "confirmed": map[string]any{"original": map[string]string{"digest": netprofile.Digest(p), "confirmed_at": "2026-11-04T00:00:00Z"}}})
	if err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string][]byte{f.catalog: []byte(doc), f.ledger: ledger} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	f.envelope = recoveryjournal.Envelope{Root: "/fixture/project", Session: "fixture-session", Runtime: "codex-cli", AttemptID: strings.Repeat("a", 64), RecoveryProof: strings.Repeat("b", 64), RegistryID: strings.Repeat("c", 64), Boot: "fixture-boot", IssuedNS: "1000", ExpiresNS: "120000001000"}
	f.secret = filepath.Join(f.binding.Directory(), "secrets", f.envelope.AttemptID+".json")
	f.receipt = filepath.Join(f.binding.Directory(), "receipts", f.envelope.AttemptID+".json")
	n, err := recoveryjournal.OpenExisting(f.binding)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, prepErr := n.Prepare(ctx, f.envelope)
	if closeErr := n.Close(); prepErr != nil || closeErr != nil {
		t.Fatal("fixture private preparation failed")
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(netproxy.VersionHeader, netproxy.RecoveryAPI)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/v2/health":
			f.health.Add(1)
			registry := f.envelope.RegistryID
			if f.foreign.Load() {
				registry = strings.Repeat("e", 64)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"api": netproxy.RecoveryAPI, "ready": true, "rule_set": "fixture", "key_mode": "fixture", "registry_id": registry, "boot": f.envelope.Boot, "now_ns": "2000", "recovery": netproxy.RecoveryProtocol})
		case r.Method == "POST" && r.URL.Path == "/v2/recover":
			f.recover.Add(1)
			var e recoveryjournal.Envelope
			if json.NewDecoder(r.Body).Decode(&e) != nil || e != f.envelope {
				f.unexpected.Add(1)
				w.WriteHeader(400)
				return
			}
			if f.malformed.Load() {
				_, _ = w.Write([]byte(`{"api":"netproxy-control/2","durable":true}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"api": netproxy.RecoveryAPI, "attempt_id": e.AttemptID, "launch_id": strings.Repeat("d", 64), "outcome": "revoked", "durable": true})
		default:
			f.unexpected.Add(1)
			w.WriteHeader(404)
		}
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return f
}

func (f *recoveryCommandFixture) run(t *testing.T) (int, string, string) {
	t.Helper()
	deps := forbiddenGatewayDeps(t)
	deps.environ = func() []string { return f.parent }
	deps.gatewayRecover = nil // Exercise production discovery/journal/Unix transport.
	var out, errOut strings.Builder
	code := run(context.Background(), []string{"gateway", "recover", "--attempt", f.envelope.AttemptID}, &out, &errOut, deps)
	for _, forbidden := range []string{f.envelope.RecoveryProof, f.envelope.Root, f.binding.StateBase()} {
		if strings.Contains(out.String()+errOut.String(), forbidden) {
			t.Fatal("RECOVERY COMMAND PRIVACY ALARM: private input escaped")
		}
	}
	return code, out.String(), errOut.String()
}

// The actual command traverses guarded original-state discovery and Unix
// transport despite a different catalog default. Repeated recovery preserves E
// and the public receipt, touches no unrelated entry, and never registers.
func TestGatewayRecoveryProductionPath(t *testing.T) {
	f := newRecoveryCommandFixture(t)
	unrelated := filepath.Join(f.binding.Directory(), "secrets", strings.Repeat("f", 64)+".json")
	if err := os.WriteFile(unrelated, []byte("corrupt unrelated sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	var prior []byte
	for i := 0; i < 2; i++ {
		code, out, errOut := f.run(t)
		if code != 0 || out != "attempt="+f.envelope.AttemptID+" service_result=revoked receipt=committed\n" || errOut != "" {
			t.Fatal("RECOVERY COMMAND PATH ALARM: exact original recovery refused")
		}
		raw, err := os.ReadFile(f.receipt)
		if err != nil || (i > 0 && !bytes.Equal(prior, raw)) {
			t.Fatal("production receipt absent or rewritten")
		}
		prior = raw
		for _, private := range []string{f.envelope.RecoveryProof, f.envelope.Root, f.binding.StateBase()} {
			if bytes.Contains(raw, []byte(private)) {
				t.Fatal("receipt contains private input")
			}
		}
	}
	raw, err := os.ReadFile(f.secret)
	var secret struct {
		Envelope recoveryjournal.Envelope `json:"envelope"`
	}
	if err != nil || json.Unmarshal(raw, &secret) != nil || secret.Envelope != f.envelope {
		t.Fatal("production recovery lost original E")
	}
	sentinel, err := os.ReadFile(unrelated)
	if err != nil || string(sentinel) != "corrupt unrelated sentinel" || f.health.Load() != 2 || f.recover.Load() != 2 || f.unexpected.Load() != 0 {
		t.Fatal("recovery enumerated/replaced evidence or used another operation")
	}
}

// Invalid original catalog/state is rejected before health; replacement
// registry is rejected before recover. A malformed service response and failed
// local receipt remain nonzero and preserve the original private bytes.
func TestGatewayRecoveryProductionRefusals(t *testing.T) {
	for _, change := range []string{"unconfirmed", "changed_profile", "missing_entry", "foreign_registry", "malformed_reply", "receipt_failure"} {
		t.Run(change, func(t *testing.T) {
			f := newRecoveryCommandFixture(t)
			before, err := os.ReadFile(f.secret)
			if err != nil {
				t.Fatal(err)
			}
			wantCode, health, recover := recoveryjournal.CodeStateInvalid, int32(0), int32(0)
			switch change {
			case "unconfirmed":
				err = os.Remove(f.ledger)
			case "changed_profile":
				raw, readErr := os.ReadFile(f.catalog)
				if readErr != nil {
					t.Fatal(readErr)
				}
				err = os.WriteFile(f.catalog, bytes.ReplaceAll(raw, []byte("sensitive_egress = true"), []byte("sensitive_egress = false")), 0600)
			case "missing_entry":
				err = os.Remove(f.secret)
			case "foreign_registry":
				f.foreign.Store(true)
				wantCode, health = recoveryjournal.CodeUnconfirmed, 1
			case "malformed_reply":
				f.malformed.Store(true)
				wantCode, health, recover = recoveryjournal.CodeUnconfirmed, 1, 1
			case "receipt_failure":
				err = os.WriteFile(f.receipt, []byte("invalid prior receipt"), 0600)
				wantCode, health, recover = recoveryjournal.CodeLocalEvidence, 1, 1
			}
			if err != nil {
				t.Fatal(err)
			}
			code, out, errOut := f.run(t)
			if code != 1 || out != "" || !strings.HasPrefix(errOut, "curator-run: "+wantCode+":") || f.health.Load() != health || f.recover.Load() != recover || f.unexpected.Load() != 0 {
				t.Fatal("RECOVERY COMMAND REFUSAL ALARM: incorrect effect/result boundary")
			}
			if strings.Contains(errOut, "service confirmed revoked") != (change == "receipt_failure") {
				t.Fatal("service observation confused with local persistence")
			}
			if change != "missing_entry" {
				after, err := os.ReadFile(f.secret)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("failed recovery changed original E")
				}
			}
			if change == "receipt_failure" {
				if err := os.Remove(f.receipt); err != nil {
					t.Fatal(err)
				}
				if code, _, _ := f.run(t); code != 0 {
					t.Fatal("clean exact retry could not recover preserved E")
				}
			} else if _, err := os.Stat(f.receipt); !os.IsNotExist(err) {
				t.Fatal("refused recovery created success receipt")
			}
		})
	}
}
