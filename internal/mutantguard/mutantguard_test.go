// Package mutantguard runs the required hosted narrowing mutants through
// .scripts/hosted-mutants.sh as a committed check, so an anchor that drifts
// from production can never silently turn a required mutant into a no-op.
//
// The script rewrites production sources in place, so every run happens in
// a scratch copy of the tree under t.TempDir(): the working tree is never
// mutated while sibling packages build. The package lives outside every
// MUTANT_PKGS pattern the guard passes, so a mutant's own suite run never
// re-enters this test.
package mutantguard

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// scriptTimeout bounds the one real external process each case waits for.
const scriptTimeout = 8 * time.Minute

// required names the mutants the AC makes mandatory, each with the bounded
// package and -run mask that contain the test expected to kill it.
var required = []struct {
	id, pkgs, run string
}{
	{"M-H1", "./internal/defaults", "^TestResolveHostPrecedence$"},
	{"M-H2", "./cmd/curator-run", "^TestHostedRefusalsBeforeContact$"},
	{"M-H3", "./cmd/curator-run", "^TestNativePerformsZeroReceiverLookup$"},
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".scripts", "hosted-mutants.sh")); err != nil {
		t.Fatalf("mutant harness not found from %s: %v", root, err)
	}
	return root
}

// scratchCopy copies the tree minus VCS and scratch state into a new
// temporary directory and returns it.
func scratchCopy(t *testing.T, root string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() && (rel == ".git" || rel == ".temp") {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case info.Mode().IsRegular():
			in, err := os.Open(path)
			if err != nil {
				return err
			}
			defer in.Close()
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_EXCL, info.Mode().Perm())
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, in); err != nil {
				out.Close()
				return err
			}
			return out.Close()
		default:
			return nil
		}
	})
	if err != nil {
		t.Fatalf("copy tree: %v", err)
	}
	return dst
}

// runScript runs the harness in the scratch tree for one mutant and returns
// its stdout, stderr, exit code, and the parsed summary rows keyed by id.
func runScript(t *testing.T, scratch, id, pkgs, run string) (string, string, int, map[string][]string) {
	t.Helper()
	evidence := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), scriptTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", filepath.Join(scratch, ".scripts", "hosted-mutants.sh"), evidence)
	cmd.Dir = scratch
	cmd.Env = append(os.Environ(), "MUTANTS_ONLY="+id, "MUTANT_PKGS="+pkgs, "MUTANT_RUN="+run)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run harness for %s: %v", id, err)
		}
		code = ee.ExitCode()
	}
	rows := map[string][]string{}
	raw, err := os.ReadFile(filepath.Join(evidence, "summary.tsv"))
	if err != nil {
		t.Fatalf("%s: no summary.tsv: %v\nstdout: %s\nstderr: %s", id, err, stdout.String(), stderr.String())
	}
	for i, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		if i == 0 {
			continue
		}
		fields := strings.Split(line, "\t")
		rows[fields[0]] = fields
	}
	return stdout.String(), stderr.String(), code, rows
}

// TestRequiredHostedMutantsAreAppliedAndKilled drives the real harness over
// the real production sources: each required mutant must be APPLIED (its
// anchor still matches production) and KILLED by the named test. A drifted
// anchor reports NOT_APPLIED and fails here by name.
func TestRequiredHostedMutantsAreAppliedAndKilled(t *testing.T) {
	root := repoRoot(t)
	for _, m := range required {
		t.Run(m.id, func(t *testing.T) {
			scratch := scratchCopy(t, root)
			stdout, stderr, code, rows := runScript(t, scratch, m.id, m.pkgs, m.run)
			row, ok := rows[m.id]
			if !ok || len(row) < 7 {
				t.Fatalf("%s: no summary row: %v\nstderr: %s", m.id, rows, stderr)
			}
			// Columns: mutant file expected_test applied suite_exit expected_failed verdict.
			if row[3] != "yes" || row[6] == "NOT_APPLIED" {
				t.Fatalf("%s: NOT_APPLIED (anchor drifted from production): %v\nstderr: %s", m.id, row, stderr)
			}
			if row[6] != "KILLED" || row[5] != "yes" {
				t.Fatalf("%s: not killed by %s: %v\nstdout: %s", m.id, row[2], row, stdout)
			}
			if code != 0 {
				t.Fatalf("%s: harness exit %d with an APPLIED+KILLED row\nstderr: %s", m.id, code, stderr)
			}
		})
	}
}

// TestHarnessFailsClosedOnDriftedAnchor attacks the harness itself: with
// the production call the M-H2 anchor names changed behavior-preservingly
// in the scratch copy, the anchor matches zero sources, and the script must
// exit non-zero, say NOT_APPLIED for M-H2 on stderr, and record it in the
// summary. A harness that skipped or passed a no-op mutant fails here.
func TestHarnessFailsClosedOnDriftedAnchor(t *testing.T) {
	root := repoRoot(t)
	scratch := scratchCopy(t, root)
	main := filepath.Join(scratch, "cmd", "curator-run", "main.go")
	src, err := os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	const anchor = "PreopenedTerminal: preopenedTTY"
	if n := strings.Count(string(src), anchor); n != 1 {
		t.Fatalf("production call carries %q %d times, want once", anchor, n)
	}
	// A behavior-preserving respelling: the field is still passed, only
	// the literal text the mutant anchors on moves.
	drifted := strings.Replace(string(src), anchor, "PreopenedTerminal:  preopenedTTY", 1)
	if err := os.WriteFile(main, []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code, rows := runScript(t, scratch, "M-H2", "./cmd/curator-run", "^TestHostedRefusalsBeforeContact$")
	if code == 0 {
		t.Fatalf("harness exited 0 for a mutant that matched no source")
	}
	if !strings.Contains(stderr, "NOT_APPLIED: M-H2") {
		t.Fatalf("stderr does not name NOT_APPLIED: M-H2: %q", stderr)
	}
	if row := rows["M-H2"]; len(row) < 7 || row[6] != "NOT_APPLIED" {
		t.Fatalf("summary row for M-H2 = %v, want NOT_APPLIED", row)
	}
}
