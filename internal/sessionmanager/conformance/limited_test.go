//go:build unix

package conformance

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/fakemanager"
)

var limitedBinary = flag.String("control-plugin", "", "reference-only subprocess control executable")
var limitedKind = flag.String("control-capabilities", "no-labels", "reference-only limited capability set")
var limitedPlant = flag.String("control-plant", "", "reference-only validator violation")

// This subprocess entry uses the actual qualification driver, including its
// applicability gates. The parent checks its verdict and named test results.
func TestLimitedQualificationDriver(t *testing.T) {
	if *limitedBinary == "" {
		t.Skip("subprocess entry for limited-capability controls")
	}
	cfg := fakemanager.DefaultConfig()
	switch *limitedKind {
	case "no-labels":
		cfg.Capabilities = slices.DeleteFunc(cfg.Capabilities, func(v string) bool { return v == "labels/1" })
	case "launch-only":
		cfg.Capabilities = []string{"launch-delegated/1"}
	default:
		t.Fatal("unknown control capability set")
	}
	var bad []string
	if *limitedPlant != "" {
		bad = []string{*limitedPlant}
	}
	runConformance(t, func(t *testing.T) *harness { return newHarnessWithConfig(t, *limitedBinary, bad, &cfg) })
}

// Labels-free and launch-only plugins must exercise mandatory profile/child-env
// validation. The same gated driver must reject active validator violations;
// skips, setup failures, and unrelated failures cannot satisfy either control.
func TestLimitedCapabilityValidation(t *testing.T) {
	if *plugin != "" {
		t.Skip("reference qualification control")
	}
	binary := buildFake(t)
	self, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	body, e := os.ReadFile(self)
	if e != nil {
		t.Fatal(e)
	}
	runner := filepath.Join(t.TempDir(), "qualification-control")
	if e = os.WriteFile(runner, body, 0700); e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"no-labels", "launch-only"} {
		for _, plant := range []string{"", "request-validation", "profile-digest"} {
			name := plant
			if name == "" {
				name = "valid"
			}
			t.Run(kind+"/"+name, func(t *testing.T) {
				h := newHarness(t, binary, nil)
				args := []string{"-test.run=^TestLimitedQualificationDriver$/^(A|B)$/^(profile-digest|secret-child-env|reserved-child-env)$", "-test.v", "-test.timeout=50s", "-control-plugin=" + binary, "-control-capabilities=" + kind, "-control-plant=" + plant}
				w, e := h.exec(endpoint{Path: runner, Prefix: args, Budget: 60 * time.Second}, "", nil, false)
				if e != nil || bytes.Contains(w.Body, []byte("SETUP:")) {
					t.Fatalf("qualification control setup: %v %s", e, w.Body)
				}
				for _, property := range []string{"B/profile-digest", "A/secret-child-env", "A/reserved-child-env"} {
					failed := (plant == "request-validation" && property != "B/profile-digest") || (plant == "profile-digest" && property == "B/profile-digest")
					verdict := "PASS"
					if failed {
						verdict = "FAIL"
					}
					want := []byte(verdict + ": TestLimitedQualificationDriver/" + property + " ")
					if !bytes.Contains(w.Body, want) {
						t.Fatalf("%s was not exercised with %s: exit=%d %s", property, verdict, w.Exit, w.Body)
					}
					if failed && !bytes.Contains(w.Body, []byte("VIOLATION "+property[2:]+":")) {
						t.Fatalf("wrong violation: %s", w.Body)
					}
				}
				wantExit := 0
				if plant != "" {
					wantExit = 1
				}
				if w.Exit != wantExit {
					t.Fatalf("qualification exit=%d want=%d %s", w.Exit, wantExit, w.Body)
				}
				if plant != "" {
					t.Logf("KILLED %s with %s capabilities", plant, kind)
				}
			})
		}
	}
}
