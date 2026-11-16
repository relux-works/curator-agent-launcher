package m0adapter

import (
	"errors"
	"math"
	"regexp"
	"strings"
	"testing"
)

const testHome = "/example/station"

func testLit() Literals { return testLiterals("/example/protected/swarma-credential-broker", testHome) }

// Environment admission: exactly the manager's six names pass unchanged; an
// extra variable refuses env_unexpected and is not cleared from the slice;
// missing, duplicate and malformed entries, HOME != H, TMPDIR outside H and
// every build-time deny marker refuse with their own code.
func TestEnvironment(t *testing.T) {
	lit := testLit()
	base := fixedEnv(testHome)
	with := func(extra ...string) []string { return append(append([]string(nil), base...), extra...) }
	set := func(name, value string) []string {
		out := append([]string(nil), base...)
		for i, kv := range out {
			if strings.HasPrefix(kv, name+"=") {
				out[i] = name + "=" + value
			}
		}
		return out
	}
	without := func(name string) []string {
		var out []string
		for _, kv := range base {
			if !strings.HasPrefix(kv, name+"=") {
				out = append(out, kv)
			}
		}
		return out
	}
	cases := []struct {
		name string
		env  []string
		want Reason
	}{
		{"fixed-six", base, ""},
		{"tmpdir-equals-home", set("TMPDIR", testHome), ""},
		{"extra", with("EXTRA=1"), ReasonEnvUnexpected},
		{"extra-codex-home", with("CODEX_HOME=/x"), ReasonEnvUnexpected},
		{"extra-godebug", with("GODEBUG=x=1"), ReasonEnvUnexpected},
		{"extra-empty-value", with("LC_ALL="), ReasonEnvUnexpected},
		{"missing-lang", without("LANG"), ReasonEnvMissing},
		{"missing-tmpdir", without("TMPDIR"), ReasonEnvMissing},
		{"duplicate-home", with("HOME=" + testHome), ReasonEnvDuplicate},
		{"malformed-no-equals", with("NOEQUALS"), ReasonEnvMalformed},
		{"malformed-empty-name", with("=x"), ReasonEnvMalformed},
		{"home-mismatch", set("HOME", "/example/other"), ReasonEnvHomeMismatch},
		{"home-trailing-slash", set("HOME", testHome+"/"), ReasonEnvHomeMismatch},
		{"tmpdir-outside", set("TMPDIR", "/tmp"), ReasonEnvTmpdir},
		{"tmpdir-sibling-prefix", set("TMPDIR", testHome+"x/tmp"), ReasonEnvTmpdir},
		{"origin-socket", set("PATH", "/usr/bin:/run/origin.sock"), ReasonEnvDenied},
		{"keymaker-marker", set("USER", "u-"+testKeymakerMarker), ReasonEnvDenied},
		{"l-owned-prefix", set("PATH", "/usr/bin:"+testDeniedPrefix+"/bin"), ReasonEnvDenied},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := strings.Join(c.env, "\x00")
			err := checkEnv(c.env, lit)
			if got := reasonFor(err); got != c.want {
				t.Fatalf("reason %q, want %q", got, c.want)
			}
			if strings.Join(c.env, "\x00") != before {
				t.Fatalf("environment was modified")
			}
		})
	}
}

// T = min(240, floor((D-now-S)/1000)) with checked arithmetic: the spec
// vectors (240; 1000 ms -> 1; 900 ms -> refuse), floor neighbors that a
// rounding-up implementation would get wrong, and overflow/range refusals.
func TestTimeoutVectors(t *testing.T) {
	cases := []struct {
		name         string
		d, now, s, t int64
		want         Reason
	}{
		{"spec-240", 290000, 20000, 10000, 240, ""},
		{"remainder-1000", 12000, 1000, 10000, 1, ""},
		{"remainder-900", 11900, 1000, 10000, 0, ReasonTimeoutInsufficient},
		{"remainder-1999-floors", 12999, 1000, 10000, 1, ""},
		{"remainder-2000", 13000, 1000, 10000, 2, ""},
		{"remainder-240999", 250999, 0, 10000, 240, ""},
		{"remainder-239999", 249999, 0, 10000, 239, ""},
		{"remainder-0", 11000, 1000, 10000, 0, ReasonTimeoutInsufficient},
		{"now-after-deadline", 1000, 5000, 1, 0, ReasonTimeoutInsufficient},
		{"max-safe-values", maxSafeInt, 0, 1, 240, ""},
		{"now-max-safe", maxSafeInt, maxSafeInt, 1, 0, ReasonTimeoutInsufficient},
		{"deadline-overflow", math.MaxInt64, 0, 1, 0, ReasonTimeoutArithmetic},
		{"slack-overflow", 1000, 0, math.MaxInt64, 0, ReasonTimeoutArithmetic},
		{"slack-zero", 290000, 0, 0, 0, ReasonTimeoutArithmetic},
		{"now-negative", 290000, -1, 10000, 0, ReasonClockRange},
		{"now-over-safe", 290000, maxSafeInt + 1, 10000, 0, ReasonClockRange},
		{"now-min-int", 290000, math.MinInt64, 10000, 0, ReasonClockRange},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := computeTimeout(c.d, c.now, c.s)
			if reasonFor(err) != c.want || got != c.t {
				t.Fatalf("T=%d reason %q, want T=%d %q", got, reasonFor(err), c.t, c.want)
			}
		})
	}
	for _, c := range []struct {
		a, b int64
		ok   bool
	}{{5, 3, true}, {math.MinInt64, 1, false}, {math.MaxInt64, -1, false}, {math.MinInt64, -1, true}, {0, math.MinInt64, false}} {
		if _, ok := checkedSub(c.a, c.b); ok != c.ok {
			t.Errorf("checkedSub(%d, %d) ok=%v, want %v", c.a, c.b, ok, c.ok)
		}
	}
}

func specimenLaunch(t *testing.T, lit Literals, now int64) *Launch {
	t.Helper()
	e, err := Decode([]byte(specimen(t)))
	if err != nil {
		t.Fatal(err)
	}
	x, err := prepare(lit, e, fixedEnv(lit.Home), now)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	return x
}

// The re-read at exec only lowers T: a later clock rewrites decimal(T), an
// equal clock keeps it, an earlier clock refuses clock_regressed and a clock
// leaving under one second refuses timeout_insufficient. D and S never reset.
func TestRefreshTimeoutOnlyLowers(t *testing.T) {
	lit := testLit()
	x := specimenLaunch(t, lit, 20000)
	if err := x.refreshTimeout(20000); err != nil || x.Timeout != 240 || x.Argv[argvTimeoutIndex] != "240" {
		t.Fatalf("equal clock: %v %d", err, x.Timeout)
	}
	if err := x.refreshTimeout(270500); err != nil || x.Timeout != 9 || x.Argv[argvTimeoutIndex] != "9" {
		t.Fatalf("later clock: %v T=%d argv=%q", err, x.Timeout, x.Argv[argvTimeoutIndex])
	}
	if err := x.refreshTimeout(270000); reasonFor(err) != ReasonClockRegressed || x.Timeout != 9 {
		t.Fatalf("earlier clock: %v T=%d", err, x.Timeout)
	}
	if err := x.refreshTimeout(279100); reasonFor(err) != ReasonTimeoutInsufficient {
		t.Fatalf("exhausted clock: %v", err)
	}
}

// The argv is exactly the 19 spec elements in order: separate option/value
// elements, no "=" spelling, no shell, no positional tail, no extra flag.
func TestArgvGolden(t *testing.T) {
	x := specimenLaunch(t, testLit(), 20000)
	want := []string{
		"/example/protected/swarma-credential-broker", "run-codex",
		"--account", "example",
		"--profile", "codex-refresher-v0",
		"--launch-home", "/example/station/tmp/m0/EX-11111111111111111111111111111111/home",
		"--model", "catalog-model",
		"--reasoning-effort", "catalog-effort",
		"--output-last-message", "/example/station/tmp/m0/EX-11111111111111111111111111111111/last-message.txt",
		"--prompt-stdin",
		"--cwd", "/example/station/work",
		"--timeout", "240",
	}
	if strings.Join(x.Argv, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("argv:\n got %q\nwant %q", x.Argv, want)
	}
	flags := 0
	for _, a := range x.Argv {
		if strings.Contains(a, "=") {
			t.Errorf("element %q uses = spelling", a)
		}
		if strings.HasPrefix(a, "-") {
			flags++
		}
	}
	if flags != 9 {
		t.Errorf("%d option elements, want 9", flags)
	}
}

// Argv bounds: each element <=1024 bytes and total storage (bytes plus one
// NUL each) <=8192 at the exact limits; deny markers in any element refuse.
// Limit: the fixed 19-element shape with 1024-byte paths cannot reach 8192,
// so the storage ceiling is exercised on synthetic argv.
func TestArgvBounds(t *testing.T) {
	lit := testLit()
	exact := make([]string, 8)
	for i := range exact {
		exact[i] = strings.Repeat("a", 1023)
	}
	if err := checkArgv(exact, lit); err != nil {
		t.Fatalf("8192 bytes refused: %v", err)
	}
	over := append(append([]string(nil), exact...), "")
	if reasonFor(checkArgv(over, lit)) != ReasonArgvLimit {
		t.Fatalf("8193 bytes admitted")
	}
	if err := checkArgv([]string{strings.Repeat("a", 1024)}, lit); err != nil {
		t.Fatalf("1024-byte element refused: %v", err)
	}
	if reasonFor(checkArgv([]string{strings.Repeat("a", 1025)}, lit)) != ReasonArgvLimit {
		t.Fatalf("1025-byte element admitted")
	}
	if reasonFor(checkArgv([]string{"a\x00b"}, lit)) != ReasonArgvLimit {
		t.Fatalf("NUL element admitted")
	}
	for _, a := range []string{"/x/origin.sock", "x" + testKeymakerMarker, testDeniedPrefix + "/y"} {
		if reasonFor(checkArgv([]string{"ok", a}, lit)) != ReasonArgvDenied {
			t.Errorf("deny marker element %q admitted", a)
		}
	}
}

// Mapping refusals: an input selector equal to a deny marker, an E naming
// the origin socket, a home whose derived paths exceed 1024 bytes, and every
// runner command other than run-codex (the Claude seam) refuse before exec.
func TestMappingRefusals(t *testing.T) {
	e, err := Decode([]byte(specimen(t)))
	if err != nil {
		t.Fatal(err)
	}
	marker := testLiterals("/opt/broker", "/srv/station")
	marker.KeymakerMarker = "example" // equals input.account only
	if _, err := prepare(marker, e, fixedEnv("/srv/station"), 20000); reasonFor(err) != ReasonArgvDenied {
		t.Errorf("deny marker in input-derived argv: %v", err)
	}
	sock := testLit()
	sock.Executable = "/opt/origin.sock/broker"
	if _, err := prepare(sock, e, fixedEnv(testHome), 20000); reasonFor(err) != ReasonArgvDenied {
		t.Errorf("deny marker in E: %v", err)
	}
	longHome := "/" + strings.Repeat("h", 1000)
	long := testLiterals("/b", longHome)
	if long.validate() != nil {
		t.Fatalf("long home literal must itself be valid")
	}
	if _, err := prepare(long, e, fixedEnv(longHome), 20000); reasonFor(err) != ReasonPathLimit {
		t.Errorf("derived path over 1024 bytes: %v", err)
	}
	claude := testLit()
	claude.RunnerCommand = "run-claude"
	if _, err := prepare(claude, e, fixedEnv(testHome), 20000); reasonFor(err) != ReasonRunnerVariantPending {
		t.Errorf("non-Codex runner command: %v", err)
	}
	if _, err := prepare(testLit(), e, fixedEnv(testHome), 280000); reasonFor(err) != ReasonTimeoutInsufficient {
		t.Errorf("exhausted deadline: %v", err)
	}
}

// Installation literals: the test binary has no -X literals, so the
// installed set refuses literal_missing; each empty field refuses
// literal_missing; non-canonical spellings refuse literal_invalid; the
// synthetic complete set is admitted.
func TestLiterals(t *testing.T) {
	if reasonFor(installedLiterals().validate()) != ReasonLiteralMissing {
		t.Fatalf("installed literals in a test build must be empty")
	}
	if err := testLit().validate(); err != nil {
		t.Fatalf("valid literals refused: %v", err)
	}
	fields := []func(*Literals) *string{
		func(l *Literals) *string { return &l.Executable },
		func(l *Literals) *string { return &l.Home },
		func(l *Literals) *string { return &l.RunnerCommand },
		func(l *Literals) *string { return &l.ClockProfile },
		func(l *Literals) *string { return &l.KeymakerMarker },
		func(l *Literals) *string { return &l.DeniedPathPrefix },
	}
	for i, f := range fields {
		l := testLit()
		*f(&l) = ""
		if reasonFor(l.validate()) != ReasonLiteralMissing {
			t.Errorf("empty field %d admitted", i)
		}
	}
	invalid := map[string]func(*Literals){
		"relative-E":        func(l *Literals) { l.Executable = "broker" },
		"root-E":            func(l *Literals) { l.Executable = "/" },
		"dotdot-H":          func(l *Literals) { l.Home = "/a/../b" },
		"trailing-H":        func(l *Literals) { l.Home = "/a/" },
		"double-slash-H":    func(l *Literals) { l.Home = "/a//b" },
		"nul-H":             func(l *Literals) { l.Home = "/a\x00b" },
		"long-E":            func(l *Literals) { l.Executable = "/" + strings.Repeat("e", 1024) },
		"relative-prefix":   func(l *Literals) { l.DeniedPathPrefix = "l-owned" },
		"runner-uppercase":  func(l *Literals) { l.RunnerCommand = "Run-codex" },
		"runner-space":      func(l *Literals) { l.RunnerCommand = "run codex" },
		"runner-33":         func(l *Literals) { l.RunnerCommand = strings.Repeat("r", 33) },
		"clock-dash-first":  func(l *Literals) { l.ClockProfile = "-x" },
		"keymaker-space":    func(l *Literals) { l.KeymakerMarker = "a b" },
		"keymaker-non-text": func(l *Literals) { l.KeymakerMarker = "a\x01" },
	}
	for name, mutate := range invalid {
		l := testLit()
		mutate(&l)
		if reasonFor(l.validate()) != ReasonLiteralInvalid {
			t.Errorf("%s admitted", name)
		}
	}
}

// Each unpublished mechanism is a refusing seam with its own code.
func TestSeamsRefuse(t *testing.T) {
	if reasonFor(waitForC()) != ReasonCUnpublished {
		t.Errorf("waitForC must refuse c_unpublished")
	}
	for _, p := range []string{"", testClockProfile, "realtime", "monotonic"} {
		if n, err := qualifiedNowMs(p); reasonFor(err) != ReasonClockUnqualified || n != 0 {
			t.Errorf("qualifiedNowMs(%q) = %d %v", p, n, err)
		}
	}
	if argv, err := claudeRunnerArgv("run-claude", mapping{}); reasonFor(err) != ReasonRunnerVariantPending || argv != nil {
		t.Errorf("claudeRunnerArgv must refuse")
	}
}

// The reason table is closed, unique and snake_case; 78 is disjoint from the
// runner-owned statuses; the diagnostic line is exactly prefix + code + LF;
// a foreign error or unlisted code is reported only as internal_error.
func TestReasonTable(t *testing.T) {
	snake := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	seen := map[Reason]bool{}
	for _, r := range reasonTable {
		if !snake.MatchString(string(r)) || seen[r] {
			t.Errorf("reason %q is not unique snake_case", r)
		}
		seen[r] = true
	}
	for _, s := range []int{0, 1, 64, 69, 75, 124, 130} {
		if s == ExitRefusal {
			t.Fatalf("78 collides with runner status %d", s)
		}
	}
	if ExitRefusal != 78 {
		t.Fatalf("ExitRefusal = %d", ExitRefusal)
	}
	if got := string(diagnosticLine(ReasonEnvUnexpected)); got != "curator-run-m0-adapter: refused env_unexpected\n" {
		t.Errorf("diagnostic %q", got)
	}
	if got := string(diagnosticLine("made_up")); got != diagnosticPrefix+"internal_error\n" {
		t.Errorf("unlisted code printed as %q", got)
	}
	if reasonOf(errors.New("foreign")) != ReasonInternal || reasonOf(refuse("made_up")) != ReasonInternal {
		t.Errorf("foreign errors must map to internal_error")
	}
	// Every code a grammar negative expects is in the closed table.
	for _, c := range grammarNegatives() {
		if !knownReason(c.want) {
			t.Errorf("case %s expects unlisted reason %q", c.name, c.want)
		}
	}
}
