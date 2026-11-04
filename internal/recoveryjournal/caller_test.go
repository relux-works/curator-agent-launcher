package recoveryjournal

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/relux-works/curator-network-profiles/pkg/netprofile"
)

// Both legitimate sensitivity forms emitted by the pinned profile library
// retain the independently specified canonical network digest and socket.
// This tests projection, not catalog admission or a live gateway.
func TestResolveCallerNetworkDigest(t *testing.T) {
	for _, sensitive := range []bool{false, true} {
		t.Run(fmt.Sprint(sensitive), func(t *testing.T) {
			p, err := netprofile.Normalize("fixture", netprofile.Input{Kind: netprofile.KindFilteringGateway, Control: "/run/gateway.sock", SensitiveEgress: sensitive, SensitiveEgressSet: true})
			if err != nil {
				t.Fatal(err)
			}
			canonical := fmt.Sprintf(`{"control":"/run/gateway.sock","credential_mode":"none","kind":"filtering-gateway","schema":"relux-network-profiles-v1","sensitive_egress":%t}`, sensitive)
			want := fmt.Sprintf("%x", sha256.Sum256([]byte(canonical)))
			b, err := ResolveCaller([]string{"HOME=/original/home", "XDG_STATE_HOME=/original/state"}, netprofile.Digest(p), p.Control)
			if err != nil || b.profile != want || b.socket != "/run/gateway.sock" || b.base != "/original/state" {
				t.Fatalf("canonical network projection failed: %v", err)
			}
		})
	}
}

// Only the supplied original routing values participate; ambient/child-like
// inputs and unrelated environment entries never select a state root.
func TestResolveCallerOriginalEnvironment(t *testing.T) {
	t.Setenv("HOME", "/ambient/home")
	t.Setenv("XDG_STATE_HOME", "/ambient/state")
	for _, tc := range []struct {
		name string
		env  []string
		base string
	}{
		{"xdg", []string{"XDG_STATE_HOME=/original/state///", "HOME=unused-relative"}, "/original/state"},
		{"xdg_without_home", []string{"XDG_STATE_HOME=/original/state"}, "/original/state"},
		{"home", []string{"HOME=/original/home"}, "/original/home/.local/state"},
		{"empty_xdg", []string{"XDG_STATE_HOME=", "HOME=/original/home/"}, "/original/home/.local/state"},
		{"unrelated", []string{"HOME=/original/home", "CHILD_HOME=/child", "CHILD_XDG_STATE_HOME=/child-state", "OTHER=fixture-private-value", "UNRELATED"}, "/original/home/.local/state"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := append([]string(nil), tc.env...)
			b, err := ResolveCaller(tc.env, "sha256:"+strings.Repeat("a", 64), "/run/gateway.sock")
			if err != nil || b.base != tc.base {
				t.Fatalf("original caller routing failed: %v", err)
			}
			if fmt.Sprint(original) != fmt.Sprint(tc.env) {
				t.Fatal("caller environment mutated")
			}
		})
	}
}

// Ambiguous/invalid original routing and malformed digest encodings refuse
// with a fixed error and a zero binding; no ambient fallback is available.
func TestResolveCallerRefusesInvalidInputs(t *testing.T) {
	goodDigest := "sha256:" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		name, digest, socket string
		env                  []string
	}{
		{"missing_original", goodDigest, "/run/gateway.sock", nil},
		{"relative_xdg", goodDigest, "/run/gateway.sock", []string{"XDG_STATE_HOME=relative", "HOME=/valid"}},
		{"duplicate_xdg", goodDigest, "/run/gateway.sock", []string{"XDG_STATE_HOME=/one", "XDG_STATE_HOME=/two"}},
		{"duplicate_home", goodDigest, "/run/gateway.sock", []string{"HOME=/one", "HOME=/one"}},
		{"missing_equals", goodDigest, "/run/gateway.sock", []string{"HOME"}},
		{"bare_digest", strings.Repeat("a", 64), "/run/gateway.sock", []string{"HOME=/original"}},
		{"other_algorithm", "sha512:" + strings.Repeat("a", 64), "/run/gateway.sock", []string{"HOME=/original"}},
		{"uppercase", "sha256:" + strings.Repeat("A", 64), "/run/gateway.sock", []string{"HOME=/original"}},
		{"trailing_data", goodDigest + " ", "/run/gateway.sock", []string{"HOME=/original"}},
		{"relative_socket", goodDigest, "relative", []string{"HOME=/original"}},
		{"noncanonical_socket", goodDigest, "/run/../gateway.sock", []string{"HOME=/original"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := ResolveCaller(tc.env, tc.digest, tc.socket)
			if !errors.Is(err, ErrBinding) || err.Error() != "recovery journal: invalid original binding" || b != (Binding{}) {
				t.Fatalf("invalid caller binding accepted or unsafe refusal: %v", err)
			}
		})
	}
}
