//go:build unix

package conformance

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The command-line entrypoint qualifies an external executable and adapter. A
// missing adapter is SKIP/UNQUALIFIED for tier B and fails the overall verdict.
func TestQualificationCLI(t *testing.T) {
	if *plugin != "" {
		t.Skip("reference entrypoint control")
	}
	binary := buildFake(t)
	h := newHarness(t, binary, nil)
	self, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	body, e := os.ReadFile(self)
	if e != nil {
		t.Fatal(e)
	}
	runner := filepath.Join(t.TempDir(), "conformance-tests")
	if e = os.WriteFile(runner, body, 0700); e != nil {
		t.Fatal(e)
	}
	state := filepath.Join(h.root, "external-state")
	args := []string{"-test.run=^TestConformance$", "-test.v", "-plugin", binary, "-prefix=--state", "-prefix=" + state, "-fixture", binary + "-fixture", "-fixture-prefix=--state", "-fixture-prefix=" + state}
	w, e := h.exec(endpoint{Path: runner, Prefix: args, Budget: 60 * time.Second}, "", nil, false)
	if e != nil || w.Exit != 0 || !bytes.Contains(w.Body, []byte("PASS: TestConformance")) {
		t.Fatalf("external fixture qualification: %v exit=%d %s", e, w.Exit, w.Body)
	}
	args = []string{"-test.run=^TestConformance$/^B$", "-test.v", "-plugin", binary}
	w, e = h.exec(endpoint{Path: runner, Prefix: args, Budget: 30 * time.Second}, "", nil, false)
	if e != nil || w.Exit == 0 || !bytes.Contains(w.Body, []byte("not conforming (tier B unqualified)")) || !bytes.Contains(w.Body, []byte("SKIP: TestConformance/B/")) || bytes.Contains(w.Body, []byte("PASS: TestConformance/B/")) {
		t.Fatalf("unqualified tier counted as pass: %v exit=%d %s", e, w.Exit, w.Body)
	}
}

// Bounded capture still detects a credential split across writes beyond the
// retained prefix; truncation cannot make the sentinel scanner a false pass.
func TestBoundedCapture(t *testing.T) {
	c := capture{Limit: 8}
	_, _ = c.Write(bytes.Repeat([]byte("x"), 100))
	_, _ = c.Write([]byte("sk-conformance-"))
	_, _ = c.Write([]byte("sentinel"))
	if c.Len() != 8 || !c.Leak {
		t.Fatal("bounded capture lost complete-stream sentinel")
	}
}
