package m0adapter

import (
	"math"
	"strconv"
	"strings"
)

// codexRunnerCommand is the one implemented runner mapping (spec §4).
const codexRunnerCommand = "run-codex"

// maxTimeoutSeconds caps T (spec §5): no fresh 300 s timeout.
const maxTimeoutSeconds = 240

// argvTimeoutIndex is the position of decimal(T) in the 19-element argv.
const argvTimeoutIndex = 18

// mapping is the complete set of values the argv is built from.
type mapping struct {
	Account, Profile, Model, ReasoningEffort string
	J, L, W, O                               string
	Timeout                                  int64
}

// Launch is the fully mapped, not yet executed launch.
type Launch struct {
	Argv    []string
	Prompt  []byte
	Timeout int64
	NowMs   int64
	entry   *Entry
	lit     Literals
}

// derivePaths is the fixed derivation of spec §4:
// J = H + "/tmp/m0/" + execution_id, L = J + "/home", W = H + "/work",
// O = J + "/last-message.txt". Each must be a clean absolute NUL-free path of
// at most 1024 bytes. No filesystem lookup happens here.
func derivePaths(home, executionID string) (j, l, w, o string, err error) {
	j = home + "/tmp/m0/" + executionID
	l = j + "/home"
	w = home + "/work"
	o = j + "/last-message.txt"
	for _, p := range []string{home, j, l, w, o} {
		if !cleanAbsolute(p) {
			return "", "", "", "", refuse(ReasonPathLimit)
		}
	}
	return j, l, w, o, nil
}

// checkedSub returns a-b or false on int64 overflow.
func checkedSub(a, b int64) (int64, bool) {
	if b > 0 && a < math.MinInt64+b || b < 0 && a > math.MaxInt64+b {
		return 0, false
	}
	return a - b, true
}

// computeTimeout is T = min(240, floor((deadline_ms-now_ms-slack_ms)/1000))
// with checked arithmetic; T < 1 refuses and nothing rounds upward.
func computeTimeout(deadlineMs, nowMs, slackMs int64) (int64, error) {
	if deadlineMs < 1 || deadlineMs > maxSafeInt || slackMs < 1 || slackMs > maxSafeInt {
		return 0, refuse(ReasonTimeoutArithmetic)
	}
	if nowMs < 0 || nowMs > maxSafeInt {
		return 0, refuse(ReasonClockRange)
	}
	rem, ok := checkedSub(deadlineMs, nowMs)
	if !ok {
		return 0, refuse(ReasonTimeoutArithmetic)
	}
	if rem, ok = checkedSub(rem, slackMs); !ok {
		return 0, refuse(ReasonTimeoutArithmetic)
	}
	t := rem / 1000
	if t < 1 {
		return 0, refuse(ReasonTimeoutInsufficient)
	}
	if t > maxTimeoutSeconds {
		t = maxTimeoutSeconds
	}
	return t, nil
}

// codexArgv is exactly the 19 elements of spec §4, in order, with separate
// option/value elements, no shell, no "=" spelling and no positional tail.
func codexArgv(lit Literals, m mapping) []string {
	return []string{
		lit.Executable, lit.RunnerCommand,
		"--account", m.Account,
		"--profile", m.Profile,
		"--launch-home", m.L,
		"--model", m.Model,
		"--reasoning-effort", m.ReasoningEffort,
		"--output-last-message", m.O,
		"--prompt-stdin",
		"--cwd", m.W,
		"--timeout", strconv.FormatInt(m.Timeout, 10),
	}
}

// runnerArgv selects the mapping by the immutable runner command literal.
// run-codex is implemented; any other command is the future Claude variant,
// which refuses through its seam.
func runnerArgv(lit Literals, m mapping) ([]string, error) {
	if lit.RunnerCommand == codexRunnerCommand {
		return codexArgv(lit, m), nil
	}
	return claudeRunnerArgv(lit.RunnerCommand, m)
}

// checkArgv enforces the storage bounds (each element <=1024 bytes; sum of
// element bytes plus one NUL each <=8192) and the build-time deny markers.
func checkArgv(argv []string, lit Literals) error {
	total := 0
	for _, a := range argv {
		if len(a) > maxArgvElement || strings.IndexByte(a, 0) >= 0 {
			return refuse(ReasonArgvLimit)
		}
		total += len(a) + 1
	}
	if total > maxArgvStorage {
		return refuse(ReasonArgvLimit)
	}
	for _, a := range argv {
		if lit.deniedMarker(a) {
			return refuse(ReasonArgvDenied)
		}
	}
	return nil
}

// prepare maps a decoded entry plus the validated literals, the unchanged
// environment and one qualified clock reading to a Launch. It is pure: no
// descriptor, file or clock access.
func prepare(lit Literals, e *Entry, env []string, nowMs int64) (*Launch, error) {
	if err := checkEnv(env, lit); err != nil {
		return nil, err
	}
	j, l, w, o, err := derivePaths(lit.Home, e.ExecutionID)
	if err != nil {
		return nil, err
	}
	t, err := computeTimeout(e.DeadlineMs, nowMs, e.SlackMs)
	if err != nil {
		return nil, err
	}
	m := mapping{
		Account: e.Account, Profile: e.Profile, Model: e.Model, ReasoningEffort: e.ReasoningEffort,
		J: j, L: l, W: w, O: o, Timeout: t,
	}
	argv, err := runnerArgv(lit, m)
	if err != nil {
		return nil, err
	}
	if err := checkArgv(argv, lit); err != nil {
		return nil, err
	}
	return &Launch{Argv: argv, Prompt: e.Prompt, Timeout: t, NowMs: nowMs, entry: e, lit: lit}, nil
}

// refreshTimeout applies the re-read of the qualified clock at exec: T is
// recomputed and only ever lowered; a clock reading earlier than the first
// one refuses, and D/S are never reset.
func (x *Launch) refreshTimeout(nowMs int64) error {
	if nowMs < x.NowMs {
		return refuse(ReasonClockRegressed)
	}
	t, err := computeTimeout(x.entry.DeadlineMs, nowMs, x.entry.SlackMs)
	if err != nil {
		return err
	}
	x.NowMs = nowMs
	if t < x.Timeout {
		x.Timeout = t
		if x.lit.RunnerCommand != codexRunnerCommand || len(x.Argv) != argvTimeoutIndex+1 {
			return refuse(ReasonInternal)
		}
		x.Argv[argvTimeoutIndex] = strconv.FormatInt(t, 10)
	}
	return nil
}
