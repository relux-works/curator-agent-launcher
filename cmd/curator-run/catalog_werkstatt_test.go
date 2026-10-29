//go:build darwin || linux

package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
)

// Werkstatt recycle stops the running session before it prepares the
// replacement, so a malformed or conflicting catalog must refuse before any
// manager call: no stop, no start, no pending operation, and byte-identical
// liveness. The absent- and valid-catalog controls reach the stop and the
// replacement start, so the refusal is the catalog's and not a recycle that
// never runs.
func TestWerkstattRecycleRefusesABadCatalogBeforeStop(t *testing.T) {
	bin := frontBuild(t)
	refused := diagnostics.ExitForCode(diagnostics.CodePlanRefused)
	for _, tc := range []struct {
		name, catalog string
		admitted      bool
	}{
		{"malformed", "schema_version = 1\nbogus-marker-key = 1\n", false},
		{"conflicting", "schema_version = 1\n[vendors.anthropic.models.\"claude-opus-5\"]\ndescription = \"secret-marker\"\n", false},
		{"absent", "", true},
		{"valid", binaryCatalog, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWS(t, bin)
			w.seedLegacySession(t)
			if tc.catalog != "" {
				writeCatalog(t, w.frontFixture, tc.catalog)
			}
			before := readWSFile(t, w.path(".json"))
			calls := len(w.calls())
			starts := w.observe().Starts
			code, out, stderr := w.invokeWS("recycle")
			got := w.calls()[calls:]
			if tc.admitted {
				if !slices.Contains(got, "stop") || w.observe().Starts != starts+1 {
					t.Fatalf("admitted-catalog recycle: calls %v starts %d (before %d) exit %d stderr %s", got, w.observe().Starts, starts, code, stderr)
				}
				return
			}
			if code != refused || out != "" || !strings.Contains(stderr, "plan_refused") || len(got) != 0 {
				t.Fatalf("bad-catalog recycle: exit %d calls %v stdout %q stderr %s", code, got, out, stderr)
			}
			if strings.Contains(stderr, "secret-marker") || strings.Contains(stderr, w.root) {
				t.Fatalf("refusal echoes the catalog or a path: %s", stderr)
			}
			w.expectStarts(starts)
			w.noOperation()
			if !bytes.Equal(before, readWSFile(t, w.path(".json"))) {
				t.Fatal("bad-catalog recycle changed liveness")
			}
		})
	}
}
