package recoveryjournal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-network-profiles/pkg/netprofile"
)

const catalogHead = "schema = \"relux-network-profiles-v1\"\n"

func gatewayDeclaration(name, socket string, sensitive bool) string {
	return fmt.Sprintf("[networks.%s]\nkind = \"filtering-gateway\"\ncontrol = %q\nsensitive_egress = %t\n", name, socket, sensitive)
}

func callerCatalogFixture(t *testing.T, doc string, confirmed []string) ([]string, Binding, string) {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".curator")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file, err := netprofile.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]any{}
	for _, name := range confirmed {
		p, ok := file.Lookup(name)
		if !ok {
			t.Fatal("fixture confirmation missing profile")
		}
		entries[name] = map[string]string{"digest": netprofile.Digest(p), "confirmed_at": "2026-11-04T00:00:00Z"}
	}
	ledger, err := json.Marshal(map[string]any{"schema": "relux-network-confirmations-v1", "confirmed": entries})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"network.toml": []byte(doc), "network.confirmations.json": ledger, "sentinel": []byte("unchanged")} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	env := []string{"HOME=" + home, "XDG_STATE_HOME=" + filepath.Join(home, "state")}
	// Original canonical semantics are fixed independently of the catalog
	// currently under inspection, including its present default/name.
	p, err := netprofile.Normalize("original", netprofile.Input{Kind: netprofile.KindFilteringGateway, Control: "/run/original.sock", SensitiveEgress: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ResolveCaller(env, netprofile.Digest(p), p.Control)
	if err != nil {
		t.Fatal(err)
	}
	return env, b, dir
}

// Recovery follows the original admitted semantics even when the default
// changes or only a confirmed alias remains; it never rewrites the catalog.
func TestCheckCatalogOriginalProfile(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		confirmed  []string
	}{
		{"explicit_no_default", gatewayDeclaration("original", "/run/original.sock", true), []string{"original"}},
		{"changed_default", "default = \"other\"\n" + gatewayDeclaration("other", "/run/other.sock", true) + gatewayDeclaration("original", "/run/original.sock", true), []string{"original"}},
		{"confirmed_alias", gatewayDeclaration("renamed", "/run/original.sock", true), []string{"renamed"}},
		{"one_confirmed_alias", gatewayDeclaration("a", "/run/original.sock", true) + gatewayDeclaration("z", "/run/original.sock", true), []string{"z"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, b, dir := callerCatalogFixture(t, catalogHead+tc.body, tc.confirmed)
			before, _ := os.ReadFile(filepath.Join(dir, "network.toml"))
			if err := CheckCatalog(env, b); err != nil {
				t.Fatalf("original admitted profile refused: %v", err)
			}
			after, _ := os.ReadFile(filepath.Join(dir, "network.toml"))
			sentinel, _ := os.ReadFile(filepath.Join(dir, "sentinel"))
			if string(after) != string(before) || string(sentinel) != "unchanged" {
				t.Fatal("catalog or sentinel changed")
			}
		})
	}
}

// Missing, changed or unconfirmed original semantics refuse before any
// service operation. A healthy different default cannot replace them.
func TestCheckCatalogRefusesChangedProfile(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		confirmed  []string
	}{
		{"missing_original", gatewayDeclaration("other", "/run/other.sock", true), []string{"other"}},
		{"changed_socket", gatewayDeclaration("original", "/run/changed.sock", true), []string{"original"}},
		{"changed_sensitivity", gatewayDeclaration("original", "/run/original.sock", false), []string{"original"}},
		{"unconfirmed", gatewayDeclaration("original", "/run/original.sock", true), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, b, dir := callerCatalogFixture(t, catalogHead+tc.body, tc.confirmed)
			before, _ := os.ReadFile(filepath.Join(dir, "network.confirmations.json"))
			if err := CheckCatalog(env, b); !errors.Is(err, ErrBindingChanged) || err.Error() != "recovery journal: original binding changed" {
				t.Fatalf("changed or unconfirmed profile accepted: %v", err)
			}
			after, _ := os.ReadFile(filepath.Join(dir, "network.confirmations.json"))
			sentinel, _ := os.ReadFile(filepath.Join(dir, "sentinel"))
			if string(after) != string(before) || string(sentinel) != "unchanged" {
				t.Fatal("refusal mutated catalog or sentinel")
			}
		})
	}
}

// Routing changes fail before loading a different HOME; malformed or absent
// catalog/ledger inputs are fixed refusals, with no ambient fallback/repair.
func TestCheckCatalogRoutingAndReadFailures(t *testing.T) {
	for _, change := range []string{"state", "home", "broken_catalog", "missing_catalog", "broken_ledger", "missing_ledger", "stale_confirmation"} {
		t.Run(change, func(t *testing.T) {
			env, b, dir := callerCatalogFixture(t, catalogHead+gatewayDeclaration("original", "/run/original.sock", true), []string{"original"})
			switch change {
			case "state":
				env[1] += "-changed"
			case "home":
				env[0] += "-absent"
			case "broken_catalog", "broken_ledger":
				name := "network.toml"
				if change == "broken_ledger" {
					name = "network.confirmations.json"
				}
				if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture-private-invalid-content"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing_catalog", "missing_ledger":
				name := "network.toml"
				if change == "missing_ledger" {
					name = "network.confirmations.json"
				}
				if err := os.Remove(filepath.Join(dir, name)); err != nil {
					t.Fatal(err)
				}
			case "stale_confirmation":
				file := filepath.Join(dir, "network.confirmations.json")
				raw, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				raw = []byte(strings.ReplaceAll(string(raw), "sha256:"+b.profile, "sha256:"+strings.Repeat("f", 64)))
				if err := os.WriteFile(file, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := CheckCatalog(env, b); !errors.Is(err, ErrBindingChanged) || err.Error() != "recovery journal: original binding changed" {
				t.Fatalf("invalid original routing/catalog accepted: %v", err)
			}
			sentinel, err := os.ReadFile(filepath.Join(dir, "sentinel"))
			if err != nil || string(sentinel) != "unchanged" {
				t.Fatal("refusal changed sentinel")
			}
		})
	}
	if !errors.Is(CheckCatalog(nil, Binding{}), ErrBinding) {
		t.Fatal("invalid original binding accepted")
	}
}
