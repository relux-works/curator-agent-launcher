//go:build linux

package m0adapter

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestProcessRawEnvironment proves ForkExec duplicates and empty entries reach the checker, while ordered valid entries reach exec unchanged.
func TestProcessRawEnvironment(t *testing.T) {
	home := t.TempDir()
	base := fixedEnv(home)
	base[0], base[5] = base[5], base[0] // do not rely on manager or host order
	for _, c := range []struct {
		name     string
		env      []string
		behavior string
		want     Reason
	}{
		{"duplicate", append(append([]string(nil), base...), "HOME=/other"), "guard-pipe", ReasonEnvDuplicate},
		{"empty", append(append([]string(nil), base...), ""), "guard-pipe", ReasonEnvMalformed},
		{"malformed", append(append([]string(nil), base...), "broken"), "guard-pipe", ReasonEnvMalformed},
		{"missing", base[1:], "guard-pipe", ReasonEnvMissing},
		{"empty-block", []string{}, "guard-pipe", ReasonEnvMissing},
		{"over-bound", append(append([]string(nil), base...), "EXTRA="+strings.Repeat("x", maxEnvironmentBytes)), "guard-pipe", reasonEnvLimit},
		{"unreadable", base, "environment-unreadable", reasonEnvRead},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runChild(t, childSpec{useHome: home, env: base})
			rep := runnerOutput(t, r)
			if r.exit != 0 || strings.Join(rep.Env, "\x00") != strings.Join(base, "\x00") {
				t.Fatal("raw environment changed")
			}
			requireRefusal(t, runChild(t, childSpec{useHome: home, env: c.env, behavior: c.behavior}), c.want)
		})
	}
}

// TestProcessFinalArgvDenied proves a 240-to-9 rewrite refuses argv_denied_marker after writing, beside the admitted 240-to-8 rewrite.
func TestProcessFinalArgvDenied(t *testing.T) {
	positive := runChild(t, childSpec{env: fixedEnv(testHome), behavior: "timeout-marker", now2: 271500})
	if rep := runnerOutput(t, positive); positive.exit != 0 || rep.Args[18] != "8" {
		t.Fatal("nonmatching timeout refused")
	}
	requireRefusal(t, runChild(t, childSpec{env: fixedEnv(testHome), behavior: "timeout-marker", now2: 270500}), ReasonArgvDenied)
}

// TestProcessStandardDescriptors proves stdout missing at package entry, changed stdout identity or final CLOEXEC stdout refuses before exec.
func TestProcessStandardDescriptors(t *testing.T) {
	for _, c := range []struct {
		behavior string
		closed   bool
		want     Reason
	}{
		{"stdout-closed", false, ReasonFDStandardMissing},
		{"stdout-cloexec", false, ReasonFDSweep},
		{"stdout-replaced", false, ReasonFDFinal},
	} {
		requireRunnerPositive(t)
		requireRefusal(t, runChild(t, childSpec{closeStdout: c.closed, behavior: c.behavior}), c.want)
	}
}

// TestProcessNativePipeProperties proves real pipe flags are nonblocking at write and capacity is unchanged through exec, beside a real blocking-descriptor negative.
func TestProcessNativePipeProperties(t *testing.T) {
	r := runChild(t, childSpec{behavior: "native-properties"})
	if rep := runnerOutput(t, r); r.exit != 0 || rep.Capacity < len(rep.Stdin) || !rep.EOF {
		t.Fatal("native pipe properties failed")
	}
	requireRefusal(t, runChild(t, childSpec{behavior: "native-blocking"}), ReasonStdinWrite)
}

// TestInstalledMainMissingLiterals proves a real command build without linker literals exits 78 literal_missing, beside a fully bound artifact's C refusal.
func TestInstalledMainMissingLiterals(t *testing.T) {
	skipUnderRace(t)
	dir := t.TempDir()
	pkg := "github.com/relux-works/curator-agent-launcher/internal/m0adapter."
	lit := testLit()
	flags := ""
	for _, pair := range [][2]string{
		{"brokerExecutable", lit.Executable}, {"stationHome", lit.Home}, {"runnerCommand", lit.RunnerCommand},
		{"clockProfile", lit.ClockProfile}, {"keymakerMarker", lit.KeymakerMarker},
		{"deniedPathPrefix", lit.DeniedPathPrefix}, {"originSocketMarker", lit.OriginSocketMarker},
	} {
		flags += " -X " + pkg + pair[0] + "=" + pair[1]
	}
	for _, c := range []struct {
		name, flags string
		want        Reason
	}{
		{"bound", flags, ReasonCUnpublished}, {"missing", "", ReasonLiteralMissing},
	} {
		binary := filepath.Join(dir, c.name)
		// The outer test command supplies the isolated build environment.
		cmd := exec.Command("go", "build", "-ldflags", c.flags, "-o", binary, "./cmd/curator-run-m0-adapter")
		cmd.Dir = "../.."
		t.Logf("command: go build -ldflags <synthetic literals or empty> -o <scratch> ./cmd/curator-run-m0-adapter")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("build exit nonzero: %v: %s", err, out)
		}
		t.Log("build exit=0")
		cmd = exec.Command(binary)
		cmd.Env = fixedEnv(lit.Home)
		out, err = cmd.CombinedOutput()
		if e, ok := err.(*exec.ExitError); !ok || e.ExitCode() != 78 || string(out) != diagnosticPrefix+string(c.want)+"\n" {
			t.Fatalf("installed entry: %v %q", err, out)
		}
	}
}
