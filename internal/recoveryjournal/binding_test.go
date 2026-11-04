//go:build darwin || linux

package recoveryjournal

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func trustedInputs() Inputs {
	return Inputs{XDGStateHome: "/state", Home: "/home/caller", ProfileDigest: strings.Repeat("a", 64), ControlSocket: "/run/gateway.sock"}
}

// Resolution is explicit and lexical: XDG takes precedence, HOME is only its
// absent-value fallback, and no environment or child configuration is consulted.
func TestResolveOriginalBinding(t *testing.T) {
	for _, tc := range []struct{ name, xdg, home, base string }{
		{"xdg", "/state", "relative-unused", "/state"},
		{"trailing_separator", "/state///", "", "/state"},
		{"lexical_normalization", "/state/./nested/..", "", "/state"},
		{"home_fallback", "", "/home/caller/", "/home/caller/.local/state"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := trustedInputs()
			in.XDGStateHome, in.Home = tc.xdg, tc.home
			t.Setenv("XDG_STATE_HOME", "/unrelated-child-state")
			t.Setenv("HOME", "/unrelated-child-home")
			b, err := Resolve(in)
			if err != nil || b.StateBase() != tc.base || b.Directory() != filepath.Join(tc.base, "swarma/launcher/netproxy-recovery/v1") {
				t.Fatalf("original binding resolution failed: %v", err)
			}
			if err := b.Match(b); err != nil {
				t.Fatal("unchanged binding refused")
			}
		})
	}
}

// Invalid explicit inputs cannot downgrade to another state root, and errors
// do not render input paths or a profile digest.
func TestResolveRefusesInvalidBinding(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Inputs)
	}{
		{"relative_xdg", func(i *Inputs) { i.XDGStateHome = "private-relative-state" }},
		{"nul_xdg", func(i *Inputs) { i.XDGStateHome = "/private\x00state" }},
		{"missing_home", func(i *Inputs) { i.XDGStateHome, i.Home = "", "" }},
		{"relative_home", func(i *Inputs) { i.XDGStateHome, i.Home = "", "private-home" }},
		{"missing_profile", func(i *Inputs) { i.ProfileDigest = "" }},
		{"uppercase_profile", func(i *Inputs) { i.ProfileDigest = strings.Repeat("A", 64) }},
		{"short_profile", func(i *Inputs) { i.ProfileDigest = strings.Repeat("a", 63) }},
		{"relative_socket", func(i *Inputs) { i.ControlSocket = "private-socket" }},
		{"noncanonical_socket", func(i *Inputs) { i.ControlSocket = "/run/../private-socket" }},
		{"socket_root", func(i *Inputs) { i.ControlSocket = "/" }},
		{"socket_nul", func(i *Inputs) { i.ControlSocket = "/private\x00socket" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := trustedInputs()
			tc.edit(&in)
			_, err := Resolve(in)
			if !errors.Is(err, ErrBinding) || err.Error() != "recovery journal: invalid original binding" {
				t.Fatalf("wrong sanitized refusal: %v", err)
			}
		})
	}
}

// Recovery binds all original routing facts, even when the namespace remains
// unchanged; a new service registry is separately checked by the v2 protocol.
func TestBindingMatch(t *testing.T) {
	original, err := Resolve(trustedInputs())
	if err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(*Inputs){
		func(i *Inputs) { i.XDGStateHome = "/other-state" },
		func(i *Inputs) { i.ProfileDigest = strings.Repeat("b", 64) },
		func(i *Inputs) { i.ControlSocket = "/run/other.sock" },
	} {
		in := trustedInputs()
		edit(&in)
		changed, err := Resolve(in)
		if err != nil {
			t.Fatal(err)
		}
		if !errors.Is(original.Match(changed), ErrBindingChanged) {
			t.Fatal("changed original binding accepted")
		}
	}
	if !errors.Is(original.Match(Binding{}), ErrBinding) {
		t.Fatal("zero binding accepted")
	}
}
