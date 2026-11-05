package protocolchild

import (
	"errors"
	"strings"
	"testing"
)

func validArgs() []string {
	return []string{Command, "--runtime=codex", "--kind=app-server", "--profile=companyA",
		"--expected-lock-sha256=" + strings.Repeat("a", 64), "--expected-fragment-sha256=" + strings.Repeat("b", 64),
		"--workspace=/fixture/work space", `--model-json="gpt-6.1-sol"`, `--effort-json="high"`, "--permissions=yolo",
		"--service-tier-json=null", "--weights-json=null", "--credential-source-json=null", "--child-env-json={}",
		"--native-tail-json=[]", "--operation-id=12345678-1234-4123-8123-123456789abc", "--lifecycle=create",
		"--member-a2a-agent-json=null", "--member-a2a-config-dir-json=null"}
}

// The actual closed invocation preserves explicit/null strings and safe path bytes;
// parsing alone grants no profile, policy, operation or process admission.
func TestParseSelectors(t *testing.T) {
	for _, model := range []string{`"gpt-6.1-sol"`, "null", `""`} {
		for _, effort := range []string{`"high"`, "null", `""`} {
			args := validArgs()
			args[7] = "--model-json=" + model
			args[8] = "--effort-json=" + effort
			got, err := Parse(args)
			if err != nil {
				t.Fatalf("model=%s effort=%s: %v", model, effort, err)
			}
			if got.Profile != "companyA" || got.Workspace != "/fixture/work space" || got.Permissions != "yolo" || got.OperationID != "12345678-1234-4123-8123-123456789abc" || got.LockSHA256 != strings.Repeat("a", 64) || got.FragmentSHA256 != strings.Repeat("b", 64) {
				t.Fatalf("changed selectors: %+v", got)
			}
			if (got.Model == nil) != (model == "null") || (got.Effort == nil) != (effort == "null") {
				t.Fatal("null collapsed")
			}
			if model == `""` && *got.Model != "" {
				t.Fatal("empty model changed")
			}
			if effort == `""` && *got.Effort != "" {
				t.Fatal("empty effort changed")
			}
		}
	}
	args := validArgs()
	args[9] = "--permissions=native"
	if _, err := Parse(args); err != nil {
		t.Fatal(err)
	}
}

// Every grammar/unsupported-semantic violation has the specific usage refusal
// and no usable selectors; adjacent TestParseSelectors prevents reject-all success.
func TestParseRefusesClosedGrammar(t *testing.T) {
	changes := map[string]struct {
		index int
		value string
	}{
		"command": {0, "codex"}, "runtime": {1, "--runtime=claude"}, "kind": {2, "--kind=resume"},
		"profile traversal": {3, "--profile=../other"}, "profile option": {3, "--profile=--native"},
		"reserved profile": {3, "--profile=CON"}, "reserved extension": {3, "--profile=lpt9.foo"}, "profile trailing dot": {3, "--profile=valid."},
		"prefixed digest":  {4, "--expected-lock-sha256=sha256:" + strings.Repeat("a", 64)},
		"uppercase digest": {5, "--expected-fragment-sha256=" + strings.Repeat("B", 64)},
		"short digest":     {4, "--expected-lock-sha256=abc"}, "relative workspace": {6, "--workspace=work"},
		"unclean workspace": {6, "--workspace=/fixture/../work"}, "model type": {7, "--model-json=[]"},
		"model malformed": {7, `--model-json="unterminated`}, "model control": {7, `--model-json="a\nb"`},
		"model trailing": {7, `--model-json="sol" {}`}, "effort type": {8, "--effort-json=1"},
		"permission": {9, "--permissions=off"}, "tier": {10, `--service-tier-json="fast"`},
		"weights": {11, "--weights-json={}"}, "source": {12, `--credential-source-json="file"`},
		"env": {13, `--child-env-json={"A":"B"}`}, "tail": {14, `--native-tail-json=["resume"]`},
		"invalid operation": {15, "--operation-id=12345678-1234-1123-8123-123456789abc"},
		"resume":            {16, "--lifecycle=resume"}, "unknown flag": {3, "--native=companyA"},
		"NUL": {6, "--workspace=/fixture/\x00work"}, "invalid UTF8": {3, "--profile=a\xff"},
	}
	for name, c := range changes {
		t.Run(name, func(t *testing.T) { args := validArgs(); args[c.index] = c.value; assertUsage(t, args) })
	}
	for name, args := range map[string][]string{
		"missing": validArgs()[:16], "extra": append(validArgs(), "--native"),
		"separator": append(validArgs(), "--"), "empty": nil,
		"duplicate": append(validArgs(), "--profile=companyA"),
	} {
		t.Run(name, func(t *testing.T) { assertUsage(t, args) })
	}
	args := validArgs()
	args[3], args[4] = args[4], args[3]
	assertUsage(t, args)
}

// Bounds are applied before JSON/profile interpretation, while a nearby valid
// invocation still parses; this proves bounded selectors, not OS argv limits.
func TestParseBounds(t *testing.T) {
	args := validArgs()
	args[7] = "--model-json=" + `"` + strings.Repeat("x", 4095) + `"`
	assertUsage(t, args)
	args = validArgs()
	for _, i := range []int{3, 6, 7, 8, 15} {
		prefix, _, _ := strings.Cut(args[i], "=")
		args[i] = prefix + "=" + strings.Repeat("x", 4096)
	}
	assertUsage(t, args)
	if _, err := Parse(validArgs()); err != nil {
		t.Fatal(err)
	}
	args = validArgs()
	args[7] = "--model-json=" + `"` + strings.Repeat("x", 4094) + `"`
	if _, err := Parse(args); err != nil {
		t.Fatalf("exact value bound refused: %v", err)
	}
}

func assertUsage(t *testing.T, args []string) {
	t.Helper()
	got, err := Parse(args)
	var refusal *Error
	if got != nil || !errors.As(err, &refusal) || refusal.Code != CodeUsage || refusal.ExitCode() != 64 {
		t.Fatalf("got %+v, %v; want specific usage refusal with no selectors", got, err)
	}
	// Invalid selector values must never be interpolated in diagnostics.
	if strings.Contains(err.Error(), "unterminated") || strings.Contains(err.Error(), "../other") {
		t.Fatal("diagnostic echoed invalid selector")
	}
}

// The paired identity selectors preserve the admitted named-member/root bytes;
// this syntax check proves neither membership nor retained-operation admission.
func TestParseMemberSelectors(t *testing.T) {
	args := validArgs()
	if len(args) != 19 {
		t.Fatalf("contract requires nineteen entries, got %d", len(args))
	}
	args[17] = `--member-a2a-agent-json="fixture.member"`
	args[18] = `--member-a2a-config-dir-json="/fixture/member root"`
	got, err := Parse(args)
	if err != nil {
		t.Fatal(err)
	}
	if got.MemberAgent == nil || *got.MemberAgent != "fixture.member" || got.MemberConfigDir == nil || *got.MemberConfigDir != "/fixture/member root" {
		t.Fatal("member selectors changed")
	}
	null, err := Parse(validArgs())
	if err != nil || null.MemberAgent != nil || null.MemberConfigDir != nil {
		t.Fatalf("null pair changed: %+v %v", null, err)
	}
	for name, change := range map[string]struct {
		index int
		value string
	}{
		"unpaired agent":  {18, `--member-a2a-config-dir-json=null`},
		"unpaired root":   {17, `--member-a2a-agent-json=null`},
		"empty agent":     {17, `--member-a2a-agent-json=""`},
		"agent traversal": {17, `--member-a2a-agent-json="../other"`},
		"root relative":   {18, `--member-a2a-config-dir-json="fixture/member"`},
		"root unclean":    {18, `--member-a2a-config-dir-json="/fixture/../other"`},
		"agent type":      {17, `--member-a2a-agent-json={}`},
		"root control":    {18, `--member-a2a-config-dir-json="/fixture/\nother"`},
	} {
		t.Run(name, func(t *testing.T) {
			bad := append([]string(nil), args...)
			bad[change.index] = change.value
			assertUsage(t, bad)
		})
	}
	// A valid-format name alone with a null root must also refuse.
	bad := validArgs()
	bad[17] = `--member-a2a-agent-json="fixture.member"`
	assertUsage(t, bad)
}
