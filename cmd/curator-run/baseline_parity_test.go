package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// authorizedUnmanagedDelta is the single unmanaged-launch difference
// this revision carries against baseline fbcdbaf0: the
// agents-management v0.5.45+ module appends its unconditional Claude
// denial after the prompt channel of the direct argv. The line below is
// the exact golden bytes (six-space JSON indent, trailing comma).
const authorizedUnmanagedDelta = `      "--disallowedTools=AskUserQuestion",`

// Operator decision Q-D3 (2026-10-05) makes an unconfigured permission
// default YOLO on every stdio shape, so the pipeline fixtures that must
// keep launching natively state `--permissions native` and their
// permissions line reports its real source. That is the one authorized
// change to the untracked goldens' stderr: the line below, in the
// baseline spelling, is what the current golden is normalized back to
// before the byte comparison, so any other drift still fails.
const (
	baselinePermissionsLine = `permissions=native source=default-headless mapped=none`
	currentPermissionsLine  = `permissions=native source=flag mapped=none`
)

// normalizeAuthorizedPermissionsLine rewrites exactly one occurrence of the
// Q-D3 spelling to the baseline spelling and fails when the golden does not
// carry it exactly once: an untracked golden without the line, or with it
// twice, is itself drift.
func normalizeAuthorizedPermissionsLine(t *testing.T, name string, current []byte) []byte {
	t.Helper()
	if n := bytes.Count(current, []byte(currentPermissionsLine)); n != 1 {
		t.Fatalf("%s: carries the Q-D3 permissions line %d times, want exactly once", name, n)
	}
	return bytes.Replace(current, []byte(currentPermissionsLine), []byte(baselinePermissionsLine), 1)
}

// TestUnmanagedGoldensMatchBaselineFbcdbaf0 pins row-5 parity: every
// unmanaged launch shape the suite goldens — six direct/tracked
// pipeline shapes plus three Muse shapes, all launched WITHOUT
// --network — matches baseline fbcdbaf0, except the two Claude goldens,
// which carry exactly the one authorized denial line above and nothing
// else. Baseline bytes live in testdata/baseline-fbcdbaf0/, extracted
// with `git show fbcdbaf0:cmd/curator-run/testdata/<name>.golden`
// (provenance hashes in the task results); they are the pin, not a
// second copy of the live fixtures.
//
// The parity chain is two links: TestProductionPipelineGoldens and the
// Muse golden tests prove production output equals the live goldens,
// and this test proves the live goldens equal baseline plus the
// enumerated delta. Any other unmanaged drift — a second added line, a
// changed value, a new golden outside this list — fails here by name.
//
// Scope: unmanaged LAUNCH shapes only. help.golden also differs from
// baseline (it documents --network); that is feature documentation,
// pinned by TestRunHelpGolden, not launch behavior. The §4.4 admission
// argv pin in internal/plan/plan_test.go carries the same single token;
// it is enumerated in SPEC §4.4b, not re-checked here.
func TestUnmanagedGoldensMatchBaselineFbcdbaf0(t *testing.T) {
	identical := []string{
		"pipeline-codex_cli-false", "pipeline-codex_cli-true",
		"pipeline-pi-false", "pipeline-pi-true",
		"pipeline-muse-native", "pipeline-muse-yolo", "pipeline-muse-yolo-alias",
	}
	for _, name := range identical {
		current := readGolden(t, filepath.Join("testdata", name+".golden"))
		base := readGolden(t, filepath.Join("testdata", "baseline-fbcdbaf0", name+".golden"))
		if strings.HasSuffix(name, "-false") && (strings.HasPrefix(name, "pipeline-codex_cli") || strings.HasPrefix(name, "pipeline-pi")) {
			current = normalizeAuthorizedPermissionsLine(t, name, current)
		}
		if !bytes.Equal(current, base) {
			t.Fatalf("%s: unmanaged golden differs from baseline fbcdbaf0 (%d bytes vs %d); only the Claude denial line is authorized",
				name, len(current), len(base))
		}
	}
	for _, name := range []string{"pipeline-claude_code-false", "pipeline-claude_code-true"} {
		current := readGolden(t, filepath.Join("testdata", name+".golden"))
		base := readGolden(t, filepath.Join("testdata", "baseline-fbcdbaf0", name+".golden"))
		if strings.HasSuffix(name, "-false") {
			current = normalizeAuthorizedPermissionsLine(t, name, current)
		}
		assertSingleAddedLine(t, name, base, current, authorizedUnmanagedDelta)
	}
}

func readGolden(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// assertSingleAddedLine requires current to equal base plus exactly one
// inserted line holding want: the line count grows by one, the first
// differing line is want, and every line after it matches shifted by
// one. Any second difference fails.
func assertSingleAddedLine(t *testing.T, name string, base, current []byte, want string) {
	t.Helper()
	bl, cl := strings.Split(string(base), "\n"), strings.Split(string(current), "\n")
	if len(cl) != len(bl)+1 {
		t.Fatalf("%s: current has %d lines, baseline %d: want exactly one added line", name, len(cl), len(bl))
	}
	k := 0
	for k < len(bl) && bl[k] == cl[k] {
		k++
	}
	if k == len(bl) {
		if cl[k] != want {
			t.Fatalf("%s: trailing added line %q, want %q", name, cl[k], want)
		}
		return
	}
	if cl[k] != want {
		t.Fatalf("%s: first differing line %d is %q, want the authorized %q", name, k+1, cl[k], want)
	}
	for i := k; i < len(bl); i++ {
		if bl[i] != cl[i+1] {
			t.Fatalf("%s: second difference at baseline line %d: base=%q current=%q", name, i+1, bl[i], cl[i+1])
		}
	}
}
