package hosted_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/hosted"
)

// transportRun drives the production Transport against the fake receiver.
// records holds fd4 JSON record lines; exit/corrupt steer the fake. It
// returns the exit code, the captured streams, and the capture directory.
func transportRun(t *testing.T, payload []byte, records []string, exit, corrupt string) (int, string, string, string) {
	t.Helper()
	dir := t.TempDir()
	capture := filepath.Join(dir, "capture")
	if err := os.Mkdir(capture, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RECEIVER_CAPTURE_DIR", capture)
	if len(records) > 0 {
		path := filepath.Join(dir, "records.jsonl")
		if err := os.WriteFile(path, []byte(strings.Join(records, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("RECEIVER_RECORDS", path)
	}
	if exit != "" {
		t.Setenv("RECEIVER_EXIT", exit)
	}
	if corrupt != "" {
		t.Setenv("RECEIVER_CORRUPT", corrupt)
	}
	ttyPath := filepath.Join(dir, "tty.txt")
	if err := os.WriteFile(ttyPath, []byte("terminal-marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Read-write like production /dev/tty: the r6 §4 terminal
	// validation requires O_RDWR.
	tty, err := os.OpenFile(ttyPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	transport := hosted.Transport{
		Lookup:   func() (string, error) { return testReceiver, nil },
		Terminal: func() (*os.File, error) { return tty, nil },
	}
	code := transport.Run(payload, &stdout, &stderr)
	return code, stdout.String(), stderr.String(), capture
}

func readCapture(t *testing.T, capture, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(capture, name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestTransportPrivateContract drives the receiver spawn through production
// Transport: the private argv, the payload on stdin only, the terminal on
// fd3, live standard streams, and the inherited environment.
func TestTransportPrivateContract(t *testing.T) {
	t.Setenv("HOSTED_TRANSPORT_MARKER", "marker-value")
	payload := []byte(`{"schema":"probe"}`)
	code, stdout, stderr, capture := transportRun(t, payload, nil, "", "")
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q", code, stderr)
	}
	// Snapshot after the run: the helper's receiver-control variables are
	// part of the inherited environment the receiver must see exactly.
	wantEnv := append([]string{}, os.Environ()...)
	sort.Strings(wantEnv)
	var argv []string
	if err := json.Unmarshal(readCapture(t, capture, "argv.json"), &argv); err != nil {
		t.Fatal(err)
	}
	wantArgv := []string{"session", "launch-plan", "--plan", "-", "--terminal-fd", "3", "--status-fd", "4"}
	if !reflect.DeepEqual(argv, wantArgv) {
		t.Fatalf("receiver argv = %q, want %q", argv, wantArgv)
	}
	if got := readCapture(t, capture, "stdin.bin"); !bytes.Equal(got, payload) {
		t.Fatalf("receiver stdin = %q, want the payload bytes", got)
	}
	if got := string(readCapture(t, capture, "fd3.txt")); got != "terminal-marker" {
		t.Fatalf("fd3 bytes = %q, want the terminal marker", got)
	}
	var env []string
	if err := json.Unmarshal(readCapture(t, capture, "env.json"), &env); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(env, wantEnv) {
		t.Fatal("receiver env differs from the launcher env")
	}
	if stdout != "receiver stdout is live\n" {
		t.Fatalf("stdout = %q, want the live receiver line", stdout)
	}
	if stderr != "receiver stderr is live\n" {
		t.Fatalf("stderr = %q, want only the live receiver line", stderr)
	}
}

// closedRecord renders one closed refusal envelope with the registry's
// literal message for code; details carries the given JSON object.
func closedRecord(code, message, details string) string {
	return `{"schema":"urn:relux:task-board:session-launch-status","schema_version":"1.0.0","type":"refusal",` +
		`"code":` + quote(code) + `,"message":` + quote(message) + `,"details":` + details + `}`
}

// TestTransportExitMapping drives every §4 exit row: the closed refusal
// record's registry code selects the constant diagnostic and the mapped
// exit even when the receiver exit diverges, proving the channel — not
// the status — classifies.
func TestTransportExitMapping(t *testing.T) {
	for _, tc := range []struct {
		code    string
		message string
		details string
		want    int
	}{
		{"usage", "Invalid invocation.", `{}`, 2},
		{"host_configuration_conflict", "Conflicting host configuration.", `{}`, 2},
		{"launch_plan_invalid", "Invalid launch plan.", `{"field":"process","reason":"shape_invalid"}`, 2},
		{"session_resume_invalid", "Invalid resume selector.", `{}`, 2},
		{"session_host_protocol_unsupported", "Session host protocol is unsupported.", `{}`, 6},
		{"session_host_provider_unsupported", "Hosted launch requirement is unsupported.", `{}`, 6},
		{"session_host_scope_unsupported", "Hosted launch requirement is unsupported.", `{}`, 6},
		{"session_host_execution_profile_unsupported", "Hosted launch requirement is unsupported.", `{}`, 6},
		{"session_host_terminal_required", "Hosted launch requirement is unsupported.", `{}`, 6},
		{"session_host_stdin_unsupported", "Hosted launch requirement is unsupported.", `{}`, 6},
		{"session_host_capability_missing", "Required host capability is missing.", `{}`, 6},
		{"session_host_protocol_error", "Invalid session host response.", `{}`, 6},
		{"secret_policy_violation", "Secret policy violation.", `{"reason":"credential_metadata"}`, 16},
		{"policy_refused", "Policy refused.", `{"field":"policy","reason":"selector_drift"}`, 16},
		{"network_scope_unsupported", "Network policy refused.", `{}`, 16},
		{"network_profile_drift", "Network policy refused.", `{}`, 16},
		{"session_host_default_not_ready", "Hosted default is not ready.", `{}`, 16},
		{"session_host_missing", "Session host is missing.", `{}`, 1},
		{"session_host_unavailable", "Session host is unavailable.", `{}`, 1},
		{"defaults_config_invalid", "Profile resolution refused.", `{}`, 1},
		{"permission_mode_tracked_unsupported", "Provider launch refused.", `{}`, 1},
		{"plan_refused", "Provider launch refused.", `{}`, 1},
		{"session_resume_not_found", "Session resume refused.", `{}`, 1},
		{"session_adoption_conflict", "Session ownership conflict.", `{}`, 1},
		{"goal_not_found", "Goal operation refused.", `{}`, 1},
	} {
		t.Run(tc.code, func(t *testing.T) {
			record := closedRecord(tc.code, tc.message, tc.details)
			code, _, stderr, _ := transportRun(t, []byte("{}"), []string{record}, "3", "")
			if code != tc.want {
				t.Fatalf("exit = %d, want %d (stderr %q)", code, tc.want, stderr)
			}
			want := "curator-run: " + tc.code + ": " + tc.message
			var details map[string]string
			if err := json.Unmarshal([]byte(tc.details), &details); err != nil {
				t.Fatal(err)
			}
			if details["field"] != "" {
				want += " field=" + details["field"]
			}
			if details["reason"] != "" {
				want += " reason=" + details["reason"]
			}
			if last := lastLine(stderr); last != want {
				t.Fatalf("diagnostic line = %q, want %q", last, want)
			}
		})
	}
}

// TestTransportRefusalDespiteZero proves a refusal record classifies even
// when the receiver exits 0: the registry exit wins over a divergent
// status.
func TestTransportRefusalDespiteZero(t *testing.T) {
	record := closedRecord("network_scope_unsupported", "Network policy refused.", `{}`)
	code, _, stderr, _ := transportRun(t, []byte("{}"), []string{record}, "0", "")
	if code != 16 {
		t.Fatalf("exit = %d, want 16 (stderr %q)", code, stderr)
	}
	if last := lastLine(stderr); last != "curator-run: network_scope_unsupported: Network policy refused." {
		t.Fatalf("diagnostic line = %q", last)
	}
}

// TestTransportUnknownCodeIsProtocolError proves the closed contract: an
// unknown future code is not preserved — it normalizes to the fixed
// protocol_error/6 with empty details, and receiver text never reflects.
func TestTransportUnknownCodeIsProtocolError(t *testing.T) {
	record := `{"schema":"urn:relux:task-board:session-launch-status","schema_version":"1.0.0","type":"refusal","code":"session_host_future_x","message":"CANARY-reflection","details":{}}`
	code, _, stderr, _ := transportRun(t, []byte("{}"), []string{record}, "6", "")
	if code != 6 {
		t.Fatalf("exit = %d, want 6 (stderr %q)", code, stderr)
	}
	if last := lastLine(stderr); last != "curator-run: session_host_protocol_error: Invalid session host response." {
		t.Fatalf("diagnostic line = %q", last)
	}
	if strings.Contains(stderr, "CANARY-reflection") || strings.Contains(stderr, "session_host_future_x") {
		t.Fatalf("stderr reflects receiver bytes: %q", stderr)
	}
}

// TestTransportNoRecordPropagatesExit proves child-status coincidence never
// creates refusal metadata: without a record the exit propagates silently,
// even for statuses that coincide with refusal exits.
func TestTransportNoRecordPropagatesExit(t *testing.T) {
	for _, exit := range []string{"0", "1", "2", "3", "6", "16", "42"} {
		code, _, stderr, _ := transportRun(t, []byte("{}"), nil, exit, "")
		var want int
		switch exit {
		case "0":
			want = 0
		case "1":
			want = 1
		case "2":
			want = 2
		case "3":
			want = 3
		case "6":
			want = 6
		case "16":
			want = 16
		case "42":
			want = 42
		}
		if code != want {
			t.Fatalf("exit %s: got %d, want %d (stderr %q)", exit, code, want, stderr)
		}
		if stderr != "receiver stderr is live\n" {
			t.Fatalf("exit %s: stderr %q carries refusal metadata without a record", exit, stderr)
		}
	}
}

// TestTransportLastRecordWins proves a failing launch prints exactly one
// diagnostic code line: the last valid refusal.
func TestTransportLastRecordWins(t *testing.T) {
	records := []string{
		closedRecord("session_host_unavailable", "Session host is unavailable.", `{}`),
		closedRecord("launch_plan_invalid", "Invalid launch plan.", `{"field":"process","reason":"shape_invalid"}`),
	}
	code, _, stderr, _ := transportRun(t, []byte("{}"), records, "2", "")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr %q)", code, stderr)
	}
	lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
	if len(lines) != 2 || lines[0] != "receiver stderr is live" {
		t.Fatalf("stderr lines = %q, want the live line plus one diagnostic", lines)
	}
	if lines[1] != "curator-run: launch_plan_invalid: Invalid launch plan. field=process reason=shape_invalid" {
		t.Fatalf("diagnostic line = %q", lines[1])
	}
}

// TestTransportInvalidRecordPoisonsChannel proves one non-contract record
// poisons the whole channel: valid refusals beside it never classify,
// and the outcome is the fixed protocol_error/6 with no reflection.
func TestTransportInvalidRecordPoisonsChannel(t *testing.T) {
	valid := closedRecord("session_host_unavailable", "Session host is unavailable.", `{}`)
	invalid := `{"code":"session_host_unavailable"}`
	for _, tc := range []struct {
		name    string
		records []string
	}{
		{"invalid-first", []string{invalid, valid}},
		{"invalid-last", []string{valid, invalid}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr, _ := transportRun(t, []byte("{}"), tc.records, "1", "")
			if code != 6 {
				t.Fatalf("exit = %d, want 6 (stderr %q)", code, stderr)
			}
			if last := lastLine(stderr); last != "curator-run: session_host_protocol_error: Invalid session host response." {
				t.Fatalf("diagnostic line = %q", last)
			}
		})
	}
}

// TestTransportCorruptChannel proves framing failures split by kind:
// complete malformed, zero-length, or oversize envelopes are protocol
// errors (6), while a truncated body is transport loss (1). Garbage
// bytes decode to a huge length, so they refuse as oversize.
func TestTransportCorruptChannel(t *testing.T) {
	for _, tc := range []struct {
		corrupt string
		want    int
		line    string
	}{
		{"garbage", 6, "curator-run: session_host_protocol_error: Invalid session host response."},
		{"oversize", 6, "curator-run: session_host_protocol_error: Invalid session host response."},
		{"zero", 6, "curator-run: session_host_protocol_error: Invalid session host response."},
		{"badjson", 6, "curator-run: session_host_protocol_error: Invalid session host response."},
		{"truncated", 1, "curator-run: session_host_unavailable: the receiver status channel is corrupt"},
	} {
		t.Run(tc.corrupt, func(t *testing.T) {
			code, _, stderr, _ := transportRun(t, []byte("{}"), nil, "0", tc.corrupt)
			if code != tc.want {
				t.Fatalf("exit = %d, want %d (stderr %q)", code, tc.want, stderr)
			}
			lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
			last := lines[len(lines)-1]
			if last != tc.line {
				t.Fatalf("diagnostic line = %q, want %q", last, tc.line)
			}
		})
	}
}

// TestTransportPoisonedStatusCannotBeCleared is the committed 2p7m0b
// table: once a complete malformed frame has poisoned the channel, a
// later transport fault (partial header or body) still yields
// session_host_protocol_error/6. Only a clean stream that saw no
// malformed frame maps a transport fault to unavailable/1.
func TestTransportPoisonedStatusCannotBeCleared(t *testing.T) {
	valid := closedRecord("usage", "Invalid invocation.", `{}`)
	for _, tc := range []struct {
		name    string
		records []string
		corrupt string
		want    int
		line    string
	}{
		{"malformed-only", []string{`{}`}, "", 6, "curator-run: session_host_protocol_error: Invalid session host response."},
		{"malformed-then-partial-header", []string{`{}`}, "partial-header", 6, "curator-run: session_host_protocol_error: Invalid session host response."},
		{"malformed-then-partial-body", []string{`{}`}, "partial-body", 6, "curator-run: session_host_protocol_error: Invalid session host response."},
		{"valid-then-partial-header", []string{valid}, "partial-header", 1, "curator-run: session_host_unavailable: the receiver status channel is corrupt"},
		{"valid-then-partial-body", []string{valid}, "partial-body", 1, "curator-run: session_host_unavailable: the receiver status channel is corrupt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr, _ := transportRun(t, []byte("{}"), tc.records, "0", tc.corrupt)
			if code != tc.want {
				t.Fatalf("exit = %d, want %d (stderr %q)", code, tc.want, stderr)
			}
			if last := lastLine(stderr); last != tc.line {
				t.Fatalf("diagnostic line = %q, want %q", last, tc.line)
			}
		})
	}
}

// TestTransportDescriptorContract proves the r6 §4 descriptor mechanism
// end to end through production Transport: fd3/fd4 necessarily arrive
// with clear close-on-exec flags (they must survive the first exec),
// the cooperative receiver sets close-on-exec immediately on startup,
// and a grandchild spawned while both descriptors are still open sees
// neither. Ingress-clear is required, not a leak: only the receiver's
// immediate marking — never the launcher's pre-exec mark — can isolate
// further descendants.
func TestTransportDescriptorContract(t *testing.T) {
	dir := t.TempDir()
	ttyPath := filepath.Join(dir, "tty.txt")
	if err := os.WriteFile(ttyPath, []byte("terminal-marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	tty, err := os.OpenFile(ttyPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	transport := hosted.Transport{
		Lookup:   func() (string, error) { return testFDContract, nil },
		Terminal: func() (*os.File, error) { return tty, nil },
	}
	if code := transport.Run([]byte(`{"schema":"probe"}`), &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"ingress fd3_cloexec=false fd4_cloexec=false",
		"marked fd3_cloexec=true fd4_cloexec=true",
		"grandchild fd3_open=false fd4_open=false",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout %q lacks the line %q", out, want)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

// TestTransportLookupFailureRefusesMissing proves a missing receiver refuses
// session_host_missing before the terminal is even opened: lookup precedes
// every other contact step, and there is no native fallback.
func TestTransportLookupFailureRefusesMissing(t *testing.T) {
	terminalCalls := 0
	var stdout, stderr bytes.Buffer
	transport := hosted.Transport{
		Lookup: func() (string, error) { return "", errFakeLookup },
		Terminal: func() (*os.File, error) {
			terminalCalls++
			return nil, errFakeLookup
		},
	}
	if code := transport.Run([]byte("{}"), &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr %q)", code, stderr.String())
	}
	if first, _, _ := strings.Cut(stderr.String(), "\n"); first != "curator-run: session_host_missing: the task-board receiver is not available on PATH" {
		t.Fatalf("diagnostic line = %q", first)
	}
	if terminalCalls != 0 {
		t.Fatalf("terminal opened %d times after a lookup failure", terminalCalls)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

// TestTransportTerminalFailureRefusesTerminal proves an unavailable terminal
// refuses session_host_terminal_required without spawning the receiver.
func TestTransportTerminalFailureRefusesTerminal(t *testing.T) {
	lookupCalls := 0
	var stdout, stderr bytes.Buffer
	transport := hosted.Transport{
		Lookup: func() (string, error) {
			lookupCalls++
			return testReceiver, nil
		},
		Terminal: func() (*os.File, error) { return nil, errFakeLookup },
	}
	if code := transport.Run([]byte("{}"), &stdout, &stderr); code != 6 {
		t.Fatalf("exit = %d, want 6 (stderr %q)", code, stderr.String())
	}
	if first, _, _ := strings.Cut(stderr.String(), "\n"); first != "curator-run: session_host_terminal_required: a hosted launch requires a controlling terminal" {
		t.Fatalf("diagnostic line = %q", first)
	}
	if lookupCalls != 1 || stdout.Len() != 0 {
		t.Fatalf("lookup calls = %d, stdout = %q", lookupCalls, stdout.String())
	}
}

// TestTransportSpawnFailureRefusesMissing proves an unstartable receiver
// path refuses session_host_missing.
func TestTransportSpawnFailureRefusesMissing(t *testing.T) {
	dir := t.TempDir()
	tty, err := os.OpenFile(filepath.Join(dir, "tty"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer tty.Close()
	var stdout, stderr bytes.Buffer
	transport := hosted.Transport{
		Lookup:   func() (string, error) { return filepath.Join(dir, "no-such-receiver"), nil },
		Terminal: func() (*os.File, error) { return tty, nil },
	}
	if code := transport.Run([]byte("{}"), &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr %q)", code, stderr.String())
	}
	if first, _, _ := strings.Cut(stderr.String(), "\n"); first != "curator-run: session_host_missing: the task-board receiver could not start" {
		t.Fatalf("diagnostic line = %q", first)
	}
}

// TestTransportCanaryNeverReflected proves receiver-controlled record bytes
// never reach the operator: a hostile message with an embedded code line
// and canary mismatches the row's literal, so the channel normalizes to
// the fixed protocol_error/6 and the canary stays out of every stderr byte.
func TestTransportCanaryNeverReflected(t *testing.T) {
	record := `{"schema":"urn:relux:task-board:session-launch-status","schema_version":"1.0.0","type":"refusal","code":"policy_refused","message":"CANARY-x\ncurator-run: usage: forged","details":{"field":"CANARY-field"}}`
	code, _, stderr, _ := transportRun(t, []byte("{}"), []string{record}, "16", "")
	if code != 6 {
		t.Fatalf("exit = %d, want 6 (stderr %q)", code, stderr)
	}
	if last := lastLine(stderr); last != "curator-run: session_host_protocol_error: Invalid session host response." {
		t.Fatalf("diagnostic line = %q", last)
	}
	for _, canary := range []string{"CANARY-x", "CANARY-field", "forged"} {
		if strings.Contains(stderr, canary) {
			t.Fatalf("stderr reflects receiver bytes: %q", stderr)
		}
	}
}

// TestTransportClosedStatusEnvelope pins the six contract shapes end to
// end through the fake receiver: missing-envelope, wrong-message,
// unknown-code, and bad-detail records normalize to protocol_error/6; a
// valid retained code keeps its registry exit; a corrupt frame is a
// protocol error.
func TestTransportClosedStatusEnvelope(t *testing.T) {
	const prefix = `{"schema":"urn:relux:task-board:session-launch-status","schema_version":"1.0.0","type":"refusal",`
	for _, tc := range []struct {
		name    string
		records []string
		corrupt string
		want    int
		line    string
	}{
		{"missing-envelope", []string{`{"ses":"SES-1"}`}, "", 6, "curator-run: session_host_protocol_error: Invalid session host response."},
		{"wrong-message", []string{prefix + `"code":"launch_plan_invalid","message":"CANARY-wrong","details":{}}`}, "", 6, "curator-run: session_host_protocol_error: Invalid session host response."},
		{"unknown-code", []string{prefix + `"code":"session_host_future_x","message":"from a newer receiver","details":{}}`}, "", 6, "curator-run: session_host_protocol_error: Invalid session host response."},
		{"bad-detail", []string{prefix + `"code":"policy_refused","message":"Policy refused.","details":{"field":"CANARY-field"}}`}, "", 6, "curator-run: session_host_protocol_error: Invalid session host response."},
		{"valid-retained-code", []string{prefix + `"code":"session_resume_not_found","message":"Session resume refused.","details":{}}`}, "", 1, "curator-run: session_resume_not_found: Session resume refused."},
		{"corrupt-frame", nil, "badjson", 6, "curator-run: session_host_protocol_error: Invalid session host response."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr, _ := transportRun(t, []byte("{}"), tc.records, "0", tc.corrupt)
			if code != tc.want {
				t.Fatalf("exit = %d, want %d (stderr %q)", code, tc.want, stderr)
			}
			if last := lastLine(stderr); last != tc.line {
				t.Fatalf("diagnostic line = %q, want %q", last, tc.line)
			}
			if strings.Contains(stderr, "CANARY") {
				t.Fatalf("stderr reflects receiver bytes: %q", stderr)
			}
		})
	}
}

// lastLine returns the final stderr line: the launcher emits its diagnostic
// after the receiver exits, so live receiver bytes precede it.
func lastLine(stderr string) string {
	lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
	return lines[len(lines)-1]
}

func quote(s string) string {
	raw, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

const errFakeLookup = fakeErr("fake failure")
