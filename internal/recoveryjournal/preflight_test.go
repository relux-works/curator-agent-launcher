package recoveryjournal

import (
	"reflect"
	"strings"
	"testing"
)

// Every emitted namespace form and missing state ancestor has a public label;
// unknown, aliased or duplicate targets never become proposed writes. This is
// a pure projection, not a filesystem permission or creation check.
func TestMissingDirectoryPreflightLabels(t *testing.T) {
	b, err := Resolve(Inputs{XDGStateHome: "/private-fixture/state", ProfileDigest: strings.Repeat("a", 64), ControlSocket: "/fixture.sock"})
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"/", "/private-fixture", "/private-fixture/state", "/private-fixture/state/swarma", "/private-fixture/state/swarma/launcher", "/private-fixture/state/swarma/launcher/netproxy-recovery", "/private-fixture/state/swarma/launcher/netproxy-recovery/v1", "/private-fixture/state/swarma/launcher/netproxy-recovery/v1/secrets", "/private-fixture/state/swarma/launcher/netproxy-recovery/v1/receipts"}
	want := []string{"STATE/../..", "STATE/..", "STATE", "STATE/swarma", "STATE/swarma/launcher", "STATE/swarma/launcher/netproxy-recovery", "STATE/swarma/launcher/netproxy-recovery/v1", "STATE/swarma/launcher/netproxy-recovery/v1/secrets", "STATE/swarma/launcher/netproxy-recovery/v1/receipts"}
	for i, p := range paths {
		got, err := (&MissingDirectories{[]string{p}}).PreflightLabels(b)
		if err != nil || !reflect.DeepEqual(got, []string{want[i]}) {
			t.Fatal("legitimate preflight label refused or incorrect")
		}
	}
	got, err := (&MissingDirectories{paths[2:]}).PreflightLabels(b)
	if err != nil || !reflect.DeepEqual(got, want[2:]) {
		t.Fatal("complete write closure reordered or incomplete")
	}
	for _, targets := range [][]string{nil, {"/private-fixture/state/other"}, {"/private-fixture/state/swarma/../swarma"}, {paths[7] + "/extra"}, {paths[7], paths[7]}, {"/foreign/state"}, {"/private-fixture/state\nforged"}} {
		if out, err := (&MissingDirectories{targets}).PreflightLabels(b); err == nil || len(out) != 0 {
			t.Fatal("PREFLIGHT TARGET ALARM: forbidden write target projected")
		}
	}
	if out, err := (&MissingDirectories{paths[2:]}).PreflightLabels(Binding{}); err == nil || len(out) != 0 {
		t.Fatal("unbound preflight admitted")
	}
	var absent *MissingDirectories
	if _, err := absent.PreflightLabels(b); err == nil {
		t.Fatal("missing closure admitted")
	}
}
