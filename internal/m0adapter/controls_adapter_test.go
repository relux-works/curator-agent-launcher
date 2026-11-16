package m0adapter

import (
	"strings"
	"testing"
)

func requireReason(t *testing.T, err error, want Reason) {
	t.Helper()
	if reasonFor(err) != want || err == nil || err.Error() != string(want) || ExitRefusal != 78 {
		t.Fatalf("reason %v, want refusal 78 %s", err, want)
	}
}

func grammarControl(id string) func(*testing.T) {
	return func(t *testing.T) {
		found := false
		for _, c := range grammarNegatives() {
			if c.control != id {
				continue
			}
			found = true
			t.Run(c.name, func(t *testing.T) {
				if _, err := Decode([]byte(specimen(t))); err != nil {
					t.Fatalf("valid neighbor: %v", err)
				}
				e, err := Decode([]byte(c.doc(t)))
				requireReason(t, err, c.want)
				if e != nil {
					t.Fatal("refusal returned an entry")
				}
			})
		}
		if !found {
			t.Fatalf("no grammar negative for %s", id)
		}
	}
}

func adapterControlTests() map[string]func(*testing.T) {
	cases := map[string]func(*testing.T){}
	for _, id := range []string{"missing", "extra", "null", "duplicate", "escaped-duplicate", "invalid-utf8", "bom", "surrogate", "noninteger", "negative-zero", "range", "depth", "size", "escape-size", "selectors", "injected-member", "missing-timing"} {
		cases[id] = grammarControl(id)
	}
	// The original cap is 240, not a fresh 300; less than one second refuses.
	cases["clock"] = func(t *testing.T) {
		x := specimenLaunch(t, testLit(), 20000)
		if x.Timeout != 240 || x.Argv[18] != "240" {
			t.Fatal("original cap lost")
		}
		_, err := computeTimeout(30999, 20000, 10000)
		requireReason(t, err, ReasonTimeoutInsufficient)
	}
	// A wrong/unpublished clock always refuses; a synthetic reading is the
	// positive arithmetic control, not qualified clock evidence.
	cases["clock-coordinate"] = func(t *testing.T) {
		if n, err := computeTimeout(290000, 20000, 10000); err != nil || n != 240 {
			t.Fatalf("synthetic neighbor: %v", err)
		}
		for _, profile := range []string{testClockProfile, "realtime", "monotonic"} {
			_, err := qualifiedNowMs(profile)
			requireReason(t, err, ReasonClockUnqualified)
		}
	}
	// The 1000/900 ms boundary floors, never rounds up.
	cases["timing-reset-rounding"] = func(t *testing.T) {
		if n, err := computeTimeout(31000, 20000, 10000); err != nil || n != 1 {
			t.Fatalf("1000 ms: %v", err)
		}
		_, err := computeTimeout(30900, 20000, 10000)
		requireReason(t, err, ReasonTimeoutInsufficient)
	}
	// Re-reading now changes neither deadline nor slack and never raises T.
	cases["timing-reset-on-reread"] = func(t *testing.T) {
		x := specimenLaunch(t, testLit(), 20000)
		d, s := x.entry.DeadlineMs, x.entry.SlackMs
		if err := x.refreshTimeout(270500); err != nil || x.Timeout != 9 || x.entry.DeadlineMs != d || x.entry.SlackMs != s {
			t.Fatalf("original bounds changed: %v", err)
		}
		requireReason(t, x.refreshTimeout(279100), ReasonTimeoutInsufficient)
	}
	// The full 53-bit member range cannot wrap in timeout arithmetic.
	cases["timing-arithmetic-wrap"] = func(t *testing.T) {
		if n, err := computeTimeout(9007199254740991, 20000, 10000); err != nil || n != 240 {
			t.Fatalf("full range: %d %v", n, err)
		}
		_, err := computeTimeout(maxSafeInt+1, 20000, 10000)
		requireReason(t, err, ReasonTimeoutArithmetic)
	}
	// Unknown environment names are refused rather than silently cleared.
	cases["adapter-env-name-outside-six"] = func(t *testing.T) {
		base := fixedEnv(testHome)
		if err := checkEnv(base, testLit()); err != nil {
			t.Fatal(err)
		}
		bad := append(append([]string(nil), base...), "ANTHROPIC_BASE_URL=x")
		before := strings.Join(bad, "\x00")
		requireReason(t, checkEnv(bad, testLit()), ReasonEnvUnexpected)
		if strings.Join(bad, "\x00") != before {
			t.Fatal("environment cleared")
		}
	}
	return addProcessControlTests(cases)
}

// TestAdapterControls proves each adapter control in the fixture has its own named positive/negative test.
func TestAdapterControls(t *testing.T) {
	for id, run := range adapterControlTests() {
		t.Run(id, run)
	}
}

// TestEnvironmentNameDenied proves a substring of an allowed environment NAME is denied beside the unchanged six-name control.
func TestEnvironmentNameDenied(t *testing.T) {
	for _, marker := range []string{"OM", "SER", "OGN", "AT", "AN", "MPD"} {
		lit := testLit()
		if err := checkEnv(fixedEnv(testHome), lit); err != nil {
			t.Fatal(err)
		}
		lit.OriginSocketMarker = marker
		requireReason(t, checkEnv(fixedEnv(testHome), lit), ReasonEnvDenied)
	}
}

// TestAdapterDeniedLiteralOccurrence proves literal substrings in every argv position and environment value, and marker substrings in closed names.
func TestAdapterDeniedLiteralOccurrence(t *testing.T) {
	for _, field := range []func(*Literals) *string{
		func(l *Literals) *string { return &l.KeymakerMarker },
		func(l *Literals) *string { return &l.DeniedPathPrefix },
		func(l *Literals) *string { return &l.OriginSocketMarker },
	} {
		lit := testLit()
		marker := *field(&lit)
		for i := 0; i < 19; i++ {
			argv := append([]string(nil), specimenLaunch(t, lit, 20000).Argv...)
			if err := checkArgv(argv, lit); err != nil {
				t.Fatal(err)
			}
			argv[i] = "prefix" + marker + "suffix"
			requireReason(t, checkArgv(argv, lit), ReasonArgvDenied)
		}
		for i := range fixedEnvNames {
			env := fixedEnv(testHome)
			if err := checkEnv(env, lit); err != nil {
				t.Fatal(err)
			}
			env[i] += "prefix" + marker + "suffix"
			requireReason(t, checkEnv(env, lit), ReasonEnvDenied)
		}
		// An absolute path prefix cannot occur in a closed environment name.
		// The two printable markers can; D must reject such collisions first.
		if marker != testDeniedPrefix {
			*field(&lit) = "OM"
			if err := lit.validate(); err != nil {
				t.Fatal(err)
			}
			requireReason(t, checkEnv(fixedEnv(testHome), lit), ReasonEnvDenied)
			*field(&lit) = "oM"
			if err := checkEnv(fixedEnv(testHome), lit); err != nil {
				t.Fatalf("case folded: %v", err)
			}
		}
	}
}

// TestOriginSocketMarkerRequired proves the third deployment literal is required and has the Keymaker marker shape, not a fixed socket name.
func TestOriginSocketMarkerRequired(t *testing.T) {
	for _, marker := range []string{"custom-origin-marker", strings.Repeat("x", 128)} {
		lit := testLit()
		lit.OriginSocketMarker = marker
		if err := lit.validate(); err != nil {
			t.Fatal(err)
		}
		lit.OriginSocketMarker = ""
		requireReason(t, lit.validate(), ReasonLiteralMissing)
	}
	for _, marker := range []string{"a b", "a\x00b", "a\x01", "é", strings.Repeat("x", 129)} {
		lit := testLit()
		if err := lit.validate(); err != nil {
			t.Fatal(err)
		}
		lit.OriginSocketMarker = marker
		requireReason(t, lit.validate(), ReasonLiteralInvalid)
	}
	if installedLiterals().OriginSocketMarker != "" {
		t.Fatal("test build unexpectedly has an origin marker")
	}
}
