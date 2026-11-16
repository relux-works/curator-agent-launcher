//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	managerschema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
)

const (
	observeV2Registered = "2026-01-01T00:00:00.125+00:00"
	observeV2Observed   = "2026-10-27T08:08:07Z"
	// Member and key order differ from any sorted encoding, so an output that
	// contains these bytes was copied and not rebuilt from decoded values.
	observeV2Process     = `{"uid":1001,"pid":4242,"start_time":{"ticks":"123456789","kind":"linux-stat-starttime","hz":100},"boot_id":"6f1c0e2a-8b44-4d1e-9a51-2f0c3d7e9b10"}`
	observeV2Termination = `{"witness":"populated-0","scope":"linux-cgroup"}`
)

var (
	observeV2Legacy  = []string{"status/1"}
	observeV2Capable = []string{"status/1", "process-observation/1"}
)

type observeV2Case struct {
	name string
	// capabilities is the scripted describe's list; nil scripts no describe.
	capabilities []string
	// status is the scripted status body; "" scripts no status step.
	status string
	// registeredAt overrides the registry entry's registration time.
	registeredAt string
	// args follow "session-manager observe demo".
	args  []string
	exit  int
	code  string
	calls []string
}

// observeV2Status is a schema-valid enveloped status body in the given state,
// with optional raw members spliced in exactly as a manager would send them.
func observeV2Status(t *testing.T, state string, members ...string) string {
	t.Helper()
	status := observeVector(t, "session")
	status["manager"], status["session"], status["state"] = "demo", "SES-example", state
	body := string(frontJSON(status))
	for i := 0; i+1 < len(members); i += 2 {
		// "runtime" occurs only at the top level of the record.
		body = strings.Replace(body, `"runtime":`, `"`+members[i]+`":`+members[i+1]+`,"runtime":`, 1)
	}
	if strings.Count(body, `"runtime":`) != 1 {
		t.Fatal("fixture anchor is not unique")
	}
	return body
}

func observeV2Frozen(tail ...string) []string {
	return append([]string{"--session", "SES-example", "--json", "--schema", "v2", "--expect-registered-at", observeV2Registered, "--expect-generation", "1"}, tail...)
}

// observeV2Run executes one observation against a scripted manager in a fresh
// configuration home. For every case it asserts the exit, the closed refusal
// with empty stdout, the exact RPC trace, that no launch was composed, and
// that no byte, mode or timestamp under the home changed, including decoy
// registry-adjacent, journal, liveness and Werkstatt files.
func observeV2Run(t *testing.T, binary string, tc observeV2Case) []byte {
	t.Helper()
	deps, runner, loc := configHomeFixture(t)
	root := filepath.Dir(loc.User)
	t.Chdir(root)
	path := filepath.Join(root, "script")
	var steps []observeStep
	if tc.capabilities != nil {
		describe := observeVector(t, "describe")
		describe["capabilities"] = tc.capabilities
		steps = append(steps, observeStep{Operation: "describe", Request: json.RawMessage(`{}`), Body: string(frontJSON(describe))})
	}
	if tc.status != "" {
		steps = append(steps, observeStep{Operation: "status", Request: json.RawMessage(`{"session":"SES-example"}`), Body: tc.status})
	}
	writeFixture(t, path, frontJSON(steps), 0600)
	observeScriptCleanup(t, path)
	registeredAt := observeV2Registered
	if tc.registeredAt != "" {
		registeredAt = tc.registeredAt
	}
	registry := sessionmanager.Empty()
	registry.Managers["demo"] = sessionmanager.Entry{Executable: binary, ArgvPrefix: []string{"script"}, Protocol: sessionmanager.Protocol, RegisteredAt: registeredAt, RegisteredBy: "test fixture"}
	encoded, err := sessionmanager.Encode(registry)
	if err != nil {
		t.Fatal(err)
	}
	writeConfig(t, loc.User, sessionmanager.Filename, string(encoded))
	for _, decoy := range []string{"op.json", filepath.Join("liveness", "SES-example.json"), filepath.Join("werkstatt", "decoy", "record.json"), filepath.Join("werkstatt", "decoy.op.json")} {
		file := filepath.Join(root, ".local", "state", "curator-run", decoy)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		writeFixture(t, file, []byte("unrelated local state"), 0600)
	}
	deps.environ = func() []string {
		return []string{"HOME=" + root, "PATH=" + root, "XDG_STATE_HOME=" + filepath.Join(root, ".local", "state")}
	}
	deps.now = func() time.Time { return time.Date(2026, 10, 27, 9, 8, 7, 999, time.FixedZone("fixture", 3600)) }
	deps.managerService = &sessionmanager.Service{CanWriteSystem: func(string) bool { t.Fatal("observation probed write authority"); return false }}
	if err := os.Chmod(loc.User, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(loc.User, 0700) })
	before := observeSnapshot(t, root, "script.index", "script.trace", "script.pids")
	var out, stderr bytes.Buffer
	exit := run(context.Background(), append([]string{"session-manager", "observe", "demo"}, tc.args...), &out, &stderr, deps)
	if exit != tc.exit {
		t.Fatalf("exit %d want %d: %s %s", exit, tc.exit, out.String(), stderr.String())
	}
	if tc.code != "" {
		if out.Len() != 0 || !strings.HasPrefix(stderr.String(), "curator-run: "+tc.code+":") || strings.Count(stderr.String(), "\n") != 1 {
			t.Fatalf("bad refusal: stdout %q stderr %q", out.String(), stderr.String())
		}
		for _, private := range []string{"4242", "6f1c0e2a", binary, observeV2Registered} {
			if strings.Contains(stderr.String(), private) {
				t.Fatalf("refusal echoed a value: %q", stderr.String())
			}
		}
	} else if stderr.Len() != 0 || bytes.Count(out.Bytes(), []byte{'\n'}) != 1 || !bytes.HasSuffix(out.Bytes(), []byte{'\n'}) {
		t.Fatalf("bad success: stdout %q stderr %q", out.String(), stderr.String())
	}
	if calls := observeTrace(t, path); !slices.Equal(calls, tc.calls) {
		t.Fatalf("RPC trace %v want %v", calls, tc.calls)
	}
	if runner.calls != 0 {
		t.Fatal("observation composed a launch")
	}
	observeUnchanged(t, root, before, "script.index", "script.trace", "script.pids")
	return out.Bytes()
}

func observeV2Document(binary, schema, record, capabilities string) string {
	return `{"schema":"` + schema + `","manager":{"id":"demo","executable":` + string(frontJSON(binary)) + `,"registered_at":"` + observeV2Registered + `"},"session":` + record + `,"observed_at":"` + observeV2Observed + `"` + capabilities + "}\n"
}

// observeV2Record is the envelope-free record the launcher renders for a
// status body: its members in sorted order, each value's bytes unchanged.
func observeV2Record(t *testing.T, body string, drop ...string) string {
	t.Helper()
	var members map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &members); err != nil {
		t.Fatal(err)
	}
	for _, name := range append([]string{"protocol", "ok"}, drop...) {
		delete(members, name)
	}
	return string(frontJSON(members))
}

// Claim: over a manager advertising process-observation/1 with status/1, v2
// output is exactly the validated record with process (or termination) copied
// byte for byte plus capabilities.process_observation true, and validates
// against the published v2 schema; v1 output over the same manager is exactly
// the 1.2.0 projection, byte-identical to v1 over a legacy manager sending the
// record without the member. A legacy manager still works in both versions,
// v2 then reporting process_observation false with no process. Every run
// makes exactly describe then status and changes no local state.
func TestObserveV2CapableAndLegacy(t *testing.T) {
	binary := observeScriptBinary(t)
	v1 := []string{"--session", "SES-example", "--json"}
	both := []string{"describe", "status"}
	for _, member := range []struct{ name, state, raw string }{
		{"process", "idle", observeV2Process},
		{"termination", "down", observeV2Termination},
	} {
		with := observeV2Status(t, member.state, member.name, member.raw)
		without := observeV2Status(t, member.state)
		if err := managerschema.Validate("session.schema.json", []byte(with)); err != nil {
			t.Fatalf("fixture is not schema-valid: %v", err)
		}
		t.Run("capable-v2-"+member.name, func(t *testing.T) {
			out := observeV2Run(t, binary, observeV2Case{capabilities: observeV2Capable, status: with, args: observeV2Frozen("--expect-executable", binary), calls: both})
			want := observeV2Document(binary, "curator-run-session-observation-v2", observeV2Record(t, with), `,"capabilities":{"process_observation":true}`)
			if string(out) != want || !bytes.Contains(out, []byte(`"`+member.name+`":`+member.raw)) {
				t.Fatalf("v2 output:\n%s\nwant:\n%s", out, want)
			}
			if err := managerschema.Validate("session-observation-v2.schema.json", out); err != nil {
				t.Fatal(err)
			}
		})
		t.Run("capable-v1-projection-"+member.name, func(t *testing.T) {
			legacy := observeV2Run(t, binary, observeV2Case{capabilities: observeV2Legacy, status: without, args: v1, calls: both})
			out := observeV2Run(t, binary, observeV2Case{capabilities: observeV2Capable, status: with, args: v1, calls: both})
			want := observeV2Document(binary, "curator-run-session-observation-v1", observeV2Record(t, with, member.name), "")
			if string(out) != want || string(out) != string(legacy) || bytes.Contains(out, []byte(member.name)) {
				t.Fatalf("v1 projection:\n%s\nwant:\n%s\nlegacy:\n%s", out, want, legacy)
			}
			if err := managerschema.Validate("session-observation.schema.json", out); err != nil {
				t.Fatal(err)
			}
			var document struct{ Session json.RawMessage }
			if json.Unmarshal(out, &document) != nil || managerschema.Validate("session-record.schema.json", document.Session) != nil {
				t.Fatalf("projection is not a valid record: %s", out)
			}
		})
	}
	plain := observeV2Status(t, "idle")
	t.Run("capable-v2-without-process", func(t *testing.T) {
		out := observeV2Run(t, binary, observeV2Case{capabilities: observeV2Capable, status: plain, args: observeV2Frozen(), calls: both})
		if want := observeV2Document(binary, "curator-run-session-observation-v2", observeV2Record(t, plain), `,"capabilities":{"process_observation":true}`); string(out) != want {
			t.Fatalf("v2 output:\n%s\nwant:\n%s", out, want)
		}
	})
	t.Run("legacy-v2", func(t *testing.T) {
		out := observeV2Run(t, binary, observeV2Case{capabilities: observeV2Legacy, status: plain, args: observeV2Frozen(), calls: both})
		want := observeV2Document(binary, "curator-run-session-observation-v2", observeV2Record(t, plain), `,"capabilities":{"process_observation":false}`)
		if string(out) != want || bytes.Contains(out, []byte(`"process"`)) {
			t.Fatalf("v2 output:\n%s\nwant:\n%s", out, want)
		}
		if err := managerschema.Validate("session-observation-v2.schema.json", out); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("unknown-token-confers-nothing", func(t *testing.T) {
		out := observeV2Run(t, binary, observeV2Case{capabilities: []string{"status/1", "process-observation/2", "process-observation"}, status: plain, args: observeV2Frozen(), calls: both})
		if !bytes.Contains(out, []byte(`"capabilities":{"process_observation":false}`)) {
			t.Fatalf("unknown token admitted: %s", out)
		}
	})
}

// Claim: without --schema the output is v1, and an explicit --schema v1 is the
// same bytes. Over a legacy manager it is the pre-existing v1 document: the v1
// schema name, no capabilities member, and the record exactly as validated.
func TestObserveDefaultSchemaIsV1(t *testing.T) {
	binary := observeScriptBinary(t)
	plain := observeV2Status(t, "unknown")
	both := []string{"describe", "status"}
	implicit := observeV2Run(t, binary, observeV2Case{capabilities: observeV2Legacy, status: plain, args: []string{"--session", "SES-example", "--json"}, calls: both})
	explicit := observeV2Run(t, binary, observeV2Case{capabilities: observeV2Legacy, status: plain, args: []string{"--session", "SES-example", "--json", "--schema", "v1"}, calls: both})
	want := observeV2Document(binary, "curator-run-session-observation-v1", observeV2Record(t, plain), "")
	if string(implicit) != want || string(explicit) != want {
		t.Fatalf("default output:\n%s\nexplicit v1:\n%s\nwant:\n%s", implicit, explicit, want)
	}
	if bytes.Contains(implicit, []byte("capabilities")) || managerschema.Validate("session-observation.schema.json", implicit) != nil {
		t.Fatalf("default is not v1: %s", implicit)
	}
}

// Claim: a 1.3.0 member from a manager without the token, and a malformed or
// misplaced extension from a manager with it, refuse
// session_host_protocol_error (6) with empty stdout in v1 and in v2 alike;
// nothing is stripped to make the record pass. The token without status/1
// keeps the existing capability refusal before any status call. Beside each
// refusal the same manager with the well-formed record succeeds.
func TestObserveV2ExtensionRefusals(t *testing.T) {
	binary := observeScriptBinary(t)
	v1 := []string{"--session", "SES-example", "--json"}
	both := []string{"describe", "status"}
	process := observeV2Status(t, "idle", "process", observeV2Process)
	termination := observeV2Status(t, "down", "termination", observeV2Termination)
	malformed := observeV2Status(t, "idle", "process", strings.Replace(observeV2Process, `"pid":4242`, `"pid":0`, 1))
	extra := observeV2Status(t, "idle", "process", strings.Replace(observeV2Process, `"pid":4242`, `"pid":4242,"started_at":"2026-01-01T00:00:00Z"`, 1))
	misplaced := observeV2Status(t, "down", "process", observeV2Process)
	unknown := observeV2Status(t, "idle", "process", observeV2Process, "no_start", `"proved"`)
	for _, schema := range []struct {
		name string
		args []string
	}{{"v1", v1}, {"v2", observeV2Frozen()}} {
		for _, tc := range []observeV2Case{
			{name: "control-capable-process", capabilities: observeV2Capable, status: process, calls: both},
			{name: "control-capable-termination", capabilities: observeV2Capable, status: termination, calls: both},
			{name: "control-legacy-plain", capabilities: observeV2Legacy, status: observeV2Status(t, "idle"), calls: both},
			{name: "no-token-process", capabilities: observeV2Legacy, status: process, exit: 6, code: "session_host_protocol_error", calls: both},
			{name: "no-token-termination", capabilities: observeV2Legacy, status: termination, exit: 6, code: "session_host_protocol_error", calls: both},
			{name: "unknown-token-process", capabilities: []string{"status/1", "process-observation/2"}, status: process, exit: 6, code: "session_host_protocol_error", calls: both},
			{name: "malformed-process", capabilities: observeV2Capable, status: malformed, exit: 6, code: "session_host_protocol_error", calls: both},
			{name: "process-extra-member", capabilities: observeV2Capable, status: extra, exit: 6, code: "session_host_protocol_error", calls: both},
			{name: "process-on-down", capabilities: observeV2Capable, status: misplaced, exit: 6, code: "session_host_protocol_error", calls: both},
			{name: "no-start-in-session", capabilities: observeV2Capable, status: unknown, exit: 6, code: "session_host_protocol_error", calls: both},
			{name: "token-without-status", capabilities: []string{"process-observation/1"}, exit: 6, code: "session_host_capability_missing", calls: []string{"describe"}},
		} {
			tc.args = schema.args
			t.Run(tc.name+"-"+schema.name, func(t *testing.T) {
				out := observeV2Run(t, binary, tc)
				if tc.code == "" && len(out) == 0 {
					t.Fatal("valid control produced no observation")
				}
			})
		}
	}
}

// Claim: v2 admits evidence only from the owner incarnation frozen before
// submission. A re-registered id (another registered_at), a changed
// executable when one was frozen, a reused handle with another generation, a
// record naming another manager, a record for another handle and an
// unreadable record each print nothing and refuse
// session_generation_mismatch (1). A registration mismatch refuses before
// any RPC, so the changed incarnation is never executed. Beside them the
// exact match succeeds with and without a frozen executable.
func TestObserveV2FrozenBinding(t *testing.T) {
	binary := observeScriptBinary(t)
	both := []string{"describe", "status"}
	good := observeV2Status(t, "idle", "process", observeV2Process)
	edit := func(from, to string) string {
		body := strings.Replace(good, from, to, 1)
		if body == good {
			t.Fatal("fixture edit did not apply")
		}
		return body
	}
	unreadable := `{"protocol":"curator-session-manager/1","ok":true,"session":"SES-example","operation_id":null,"manager":"demo","runtime":null,"native_id":null,"readable":false,"state":"unknown","labels":{},"generation":null,"started_at":null,"last_activity_at":null,"doorbell":{"state":"off","uncertain":false,"generation":null,"reason":null}}`
	other := filepath.Join(filepath.Dir(binary), "replaced-manager")
	for _, tc := range []observeV2Case{
		{name: "exact-match", capabilities: observeV2Capable, status: good, args: observeV2Frozen(), calls: both},
		{name: "exact-match-with-executable", capabilities: observeV2Capable, status: good, args: observeV2Frozen("--expect-executable", binary), calls: both},
		{name: "re-registered", registeredAt: "2026-01-02T00:00:00.125+00:00", args: observeV2Frozen(), exit: 1, code: "session_generation_mismatch"},
		{name: "re-registered-same-instant-other-spelling", registeredAt: "2026-01-01T00:00:00.125Z", args: observeV2Frozen(), exit: 1, code: "session_generation_mismatch"},
		{name: "changed-executable", args: observeV2Frozen("--expect-executable", other), exit: 1, code: "session_generation_mismatch"},
		{name: "reused-handle-other-generation", capabilities: observeV2Capable, status: edit(`"generation":1,`, `"generation":2,`), args: observeV2Frozen(), exit: 1, code: "session_generation_mismatch", calls: both},
		{name: "foreign-manager-record", capabilities: observeV2Capable, status: edit(`"manager":"demo"`, `"manager":"other"`), args: observeV2Frozen(), exit: 1, code: "session_generation_mismatch", calls: both},
		{name: "other-handle-record", capabilities: observeV2Capable, status: edit(`"session":"SES-example"`, `"session":"SES-other"`), args: observeV2Frozen(), exit: 1, code: "session_generation_mismatch", calls: both},
		{name: "unreadable-record", capabilities: observeV2Capable, status: unreadable, args: observeV2Frozen(), exit: 1, code: "session_generation_mismatch", calls: both},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := observeV2Run(t, binary, tc)
			if tc.code == "" {
				if err := managerschema.Validate("session-observation-v2.schema.json", out); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	// v1 has no frozen binding: the same foreign record keeps its existing
	// protocol-error refusal and another generation stays a valid observation.
	v1 := []string{"--session", "SES-example", "--json"}
	t.Run("v1-foreign-manager-unchanged", func(t *testing.T) {
		observeV2Run(t, binary, observeV2Case{capabilities: observeV2Capable, status: edit(`"manager":"demo"`, `"manager":"other"`), args: v1, exit: 6, code: "session_host_protocol_error", calls: both})
	})
	t.Run("v1-other-generation-unchanged", func(t *testing.T) {
		if out := observeV2Run(t, binary, observeV2Case{capabilities: observeV2Capable, status: edit(`"generation":1,`, `"generation":2,`), args: v1, calls: both}); len(out) == 0 {
			t.Fatal("v1 gained a generation check")
		}
	})
}

// Claim: a v2 flag without --schema v2, a missing required v2 flag, a
// duplicate flag, a malformed value and an unknown --schema value each refuse
// usage (2) with empty stdout before any RPC and without echoing the value.
// Beside them the complete v2 command line succeeds against the same manager.
func TestObserveV2Usage(t *testing.T) {
	binary := observeScriptBinary(t)
	base := []string{"--session", "SES-example", "--json"}
	with := func(tail ...string) []string { return append(append([]string{}, base...), tail...) }
	t.Run("valid-control", func(t *testing.T) {
		if out := observeV2Run(t, binary, observeV2Case{capabilities: observeV2Legacy, status: observeV2Status(t, "idle"), args: observeV2Frozen("--expect-executable", binary), calls: []string{"describe", "status"}}); len(out) == 0 {
			t.Fatal("valid control produced no observation")
		}
	})
	for name, args := range map[string][]string{
		"registered-at-without-schema":   with("--expect-registered-at", observeV2Registered),
		"generation-without-schema":      with("--expect-generation", "1"),
		"executable-without-schema":      with("--expect-executable", binary),
		"all-expect-without-schema":      with("--expect-registered-at", observeV2Registered, "--expect-generation", "1"),
		"expect-with-schema-v1":          with("--schema", "v1", "--expect-registered-at", observeV2Registered, "--expect-generation", "1"),
		"v2-missing-registered-at":       with("--schema", "v2", "--expect-generation", "1"),
		"v2-missing-generation":          with("--schema", "v2", "--expect-registered-at", observeV2Registered),
		"v2-missing-both":                with("--schema", "v2"),
		"v2-only-executable":             with("--schema", "v2", "--expect-executable", binary),
		"duplicate-schema":               observeV2Frozen("--schema", "v2"),
		"duplicate-registered-at":        observeV2Frozen("--expect-registered-at", observeV2Registered),
		"duplicate-generation":           observeV2Frozen("--expect-generation", "1"),
		"duplicate-executable":           observeV2Frozen("--expect-executable", binary, "--expect-executable", binary),
		"schema-v3":                      with("--schema", "v3", "--expect-registered-at", observeV2Registered, "--expect-generation", "1"),
		"schema-v3-alone":                with("--schema", "v3"),
		"schema-empty":                   with("--schema", ""),
		"schema-missing-value":           with("--schema"),
		"schema-uppercase":               with("--schema", "V2"),
		"generation-zero":                with("--schema", "v2", "--expect-registered-at", observeV2Registered, "--expect-generation", "0"),
		"generation-negative":            with("--schema", "v2", "--expect-registered-at", observeV2Registered, "--expect-generation", "-1"),
		"generation-leading-zero":        with("--schema", "v2", "--expect-registered-at", observeV2Registered, "--expect-generation", "01"),
		"generation-above-safe-integer":  with("--schema", "v2", "--expect-registered-at", observeV2Registered, "--expect-generation", "9007199254740992"),
		"generation-not-a-number":        with("--schema", "v2", "--expect-registered-at", observeV2Registered, "--expect-generation", "SENTINEL_SECRET"),
		"registered-at-not-a-timestamp":  with("--schema", "v2", "--expect-registered-at", "SENTINEL_SECRET", "--expect-generation", "1"),
		"registered-at-non-utc-offset":   with("--schema", "v2", "--expect-registered-at", "2026-01-01T01:00:00+01:00", "--expect-generation", "1"),
		"executable-relative":            observeV2Frozen("--expect-executable", "SENTINEL_SECRET/manager"),
		"executable-empty":               observeV2Frozen("--expect-executable", ""),
		"executable-missing-value":       observeV2Frozen("--expect-executable"),
		"unknown-expect-flag":            observeV2Frozen("--expect-manager", "demo"),
		"v2-missing-session":             {"--json", "--schema", "v2", "--expect-registered-at", observeV2Registered, "--expect-generation", "1"},
		"v2-missing-json":                {"--session", "SES-example", "--schema", "v2", "--expect-registered-at", observeV2Registered, "--expect-generation", "1"},
		"registered-at-value-is-a-flag":  with("--schema", "v2", "--expect-registered-at", "--expect-generation", "1"),
		"generation-value-is-a-flag":     with("--schema", "v2", "--expect-generation", "--expect-registered-at", observeV2Registered),
		"executable-value-is-a-flag":     observeV2Frozen("--expect-executable", "--json"),
		"schema-value-is-a-flag":         with("--schema", "--expect-generation", "1"),
		"positional-after-flags":         observeV2Frozen("SENTINEL_SECRET"),
		"registered-at-trailing-newline": with("--schema", "v2", "--expect-registered-at", observeV2Registered+"\n", "--expect-generation", "1"),
	} {
		t.Run(name, func(t *testing.T) {
			deps, runner, loc := configHomeFixture(t)
			root := filepath.Dir(loc.User)
			t.Chdir(root)
			path := filepath.Join(root, "script")
			writeFixture(t, path, []byte("[]"), 0600)
			observeScriptCleanup(t, path)
			observeRegistry(t, loc.User, binary, []string{"script"})
			deps.environ = func() []string { return []string{"HOME=" + root, "PATH=" + root} }
			before := observeSnapshot(t, root)
			var out, stderr bytes.Buffer
			exit := run(context.Background(), append([]string{"session-manager", "observe", "demo"}, args...), &out, &stderr, deps)
			if exit != 2 || out.Len() != 0 || !strings.HasPrefix(stderr.String(), "curator-run: usage:") || strings.Contains(stderr.String(), "SENTINEL") || strings.Contains(stderr.String(), binary) || strings.Contains(stderr.String(), observeV2Registered) {
				t.Fatalf("exit %d stdout %q stderr %q", exit, out.String(), stderr.String())
			}
			if calls := observeTrace(t, path); calls != nil || runner.calls != 0 {
				t.Fatalf("usage error reached a manager: %v", calls)
			}
			observeUnchanged(t, root, before)
		})
	}
}
