package hosted_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/skill-agents-management/pkg/agentic"
	claudeSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/claude"

	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/hosted"
)

// validPayloadInputs builds a minimal valid payload input over temp dirs.
// Callers mutate one member per invalid row.
func validPayloadInputs(t *testing.T) hosted.PayloadInputs {
	t.Helper()
	dir := t.TempDir()
	home := filepath.Join(dir, "managed")
	raw := `{"fragment":"launch-env-fragment-v2","environment":"claude_code",` +
		`"profile":{"name":"default","lock_sha256":"` + strings.Repeat("a", 64) + `"},` +
		`"precedence":{"winner":"higher-weight","placement":"winner-last"},` +
		`"permissions":{"mode":"native","locked":false,"source":"default"},` +
		`"env":{"CLAUDE_CONFIG_DIR":` + quote(home) + `}}`
	frag, err := fragment.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	env := []string{
		"CLAUDE_CONFIG_DIR=" + home,
		"HOME=" + dir,
		"PATH=/usr/bin:/bin",
	}
	argv := []string{"--model", "example-model", "--disallowedTools=AskUserQuestion"}
	return hosted.PayloadInputs{
		EnvID: "claude_code",
		Value: composition.Value{
			Binary:      filepath.Join(dir, "claude"),
			WorkDir:     dir,
			Argv:        argv,
			Env:         env,
			EnvLiterals: map[string]string{"CLAUDE_CONFIG_DIR": home},
			EnvNames:    []string{},
		},
		Fragment:         *frag,
		CallerEnv:        append([]string{}, env...),
		PermissionMode:   agentic.PermissionModeNative,
		PermissionSource: "flag",
		HostName:         "claude_code-20261004T040000Z",
		Resume:           agentic.ResumeIntent{Kind: agentic.ResumeNew},
		Restart: claudeSystem.RestartTemplate{
			Schema: claudeSystem.RestartSchema, SchemaVersion: claudeSystem.RestartSchemaVersion,
			Data: claudeSystem.RestartData{
				Argv:         append([]string{}, argv...),
				IdentitySlot: claudeSystem.IdentitySlot{Index: len(argv), Flag: "--resume"},
			},
		},
		Seal: unsealedGuard(),
	}
}

func unsealedGuard() agentic.Seal {
	return agentic.Seal{
		Schema: agentic.ExecGuardSchema, SchemaVersion: agentic.ExecGuardVersion,
		Data: agentic.SealData{Kind: agentic.SealKindUnsealed},
	}
}

func buildErr(t *testing.T, in hosted.PayloadInputs) *hosted.PlanError {
	t.Helper()
	_, err := hosted.BuildPayload(in)
	if err == nil {
		t.Fatal("BuildPayload succeeded, want launch_plan_invalid")
	}
	planErr, ok := err.(*hosted.PlanError)
	if !ok {
		t.Fatalf("error type %T, want *hosted.PlanError", err)
	}
	if planErr.Code != "launch_plan_invalid" {
		t.Fatalf("code = %q, want launch_plan_invalid", planErr.Code)
	}
	return planErr
}

// TestBuildPayloadValid pins the wire projection of a valid input: every
// member lands in the closed shape, stdin and the future carriers stay
// null, the emission is canonical bytes, and the digest recomputes.
func TestBuildPayloadValid(t *testing.T) {
	in := validPayloadInputs(t)
	wire, err := hosted.BuildPayload(in)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(wire, &payload); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"schema":         "urn:relux:task-board:session-launch-plan",
		"schema_version": "1.0.0",
		"environment":    "claude_code",
		"host_bundle":    nil,
		"network":        nil,
	} {
		if payload[key] != want {
			t.Fatalf("%s = %v, want %v", key, payload[key], want)
		}
	}
	digest, _ := payload["content_digest"].(string)
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != 7+64 {
		t.Fatalf("content_digest = %q, want sha256:<64 hex>", digest)
	}
	recomputed, err := hosted.ContentDigest(wire)
	if err != nil || recomputed != digest {
		t.Fatalf("digest recomputation = %q, %v; want %q", recomputed, err, digest)
	}
	parsed, err := fragment.ParseJSON(wire)
	if err != nil {
		t.Fatal(err)
	}
	if string(fragment.Canonical(parsed)) != string(wire) {
		t.Fatal("wire bytes are not the canonical form")
	}
	producer := payload["producer"].(map[string]any)
	if producer["module_version"] != "0.5.53" || producer["module_commit"] != hosted.ProducerModuleCommit {
		t.Fatalf("producer = %v", producer)
	}
	process := payload["process"].(map[string]any)
	if process["binary"] != in.Value.Binary || process["cwd"] != in.Value.WorkDir || process["stdin"] != nil {
		t.Fatalf("process projection = %v", process)
	}
	if !equalStrings(process["argv"], in.Value.Argv) || !equalStrings(process["env"], in.Value.Env) {
		t.Fatal("process argv/env differ from the composed value")
	}
	guard := process["exec_guard"].(map[string]any)
	if guard["schema"] != agentic.ExecGuardSchema || guard["schema_version"] != "1.0.0" ||
		guard["data"].(map[string]any)["kind"] != "unsealed" {
		t.Fatalf("exec_guard = %v", guard)
	}
	if !reflectDeepEqual(payload["owned_literals"], in.Value.EnvLiterals) {
		t.Fatalf("owned_literals = %v", payload["owned_literals"])
	}
	envNames, _ := payload["env_names"].([]any)
	if envNames == nil || len(envNames) != 0 {
		t.Fatalf("env_names = %v, want []", payload["env_names"])
	}
	managed := payload["managed_home"].(map[string]any)
	if managed["variable"] != "CLAUDE_CONFIG_DIR" || managed["path"] != in.Fragment.Env["CLAUDE_CONFIG_DIR"] {
		t.Fatalf("managed_home = %v", managed)
	}
	fragmentRecord := payload["fragment"].(map[string]any)
	if fragmentRecord["profile_name"] != "default" || fragmentRecord["system_modules"] != false ||
		fragmentRecord["fragment_digest"] != in.Fragment.Digest ||
		fragmentRecord["pin"] != "sha256:"+strings.Repeat("a", 64) {
		t.Fatalf("fragment = %v", fragmentRecord)
	}
	policy := payload["policy"].(map[string]any)
	if policy["permission_mode"] != "native" || policy["permission_source"] != "flag" ||
		policy["execution_profile"] != "standard" || policy["effective_native_policy"] != nil {
		t.Fatalf("policy = %v", policy)
	}
	session := payload["session_name"].(map[string]any)
	if session["host"] != in.HostName || session["native"] != nil {
		t.Fatalf("session_name = %v", session)
	}
	remote := payload["remote_control"].(map[string]any)
	indices, _ := remote["argv_indices"].([]any)
	if remote["enabled"] != false || indices == nil || len(indices) != 0 {
		t.Fatalf("remote_control = %v", remote)
	}
	resume := payload["resume"].(map[string]any)
	if resume["kind"] != "new" || resume["identity"] != nil {
		t.Fatalf("resume = %v", resume)
	}
	restart := payload["restart"].(map[string]any)
	restartData := restart["data"].(map[string]any)
	if restart["schema"] != claudeSystem.RestartSchema || restart["schema_version"] != "1.0.0" ||
		!equalStrings(restartData["argv"], in.Value.Argv) {
		t.Fatalf("restart = %v", restart)
	}
	slot := restartData["identity_slot"].(map[string]any)
	if slot["flag"] != "--resume" || slot["index"] != float64(len(in.Value.Argv)) {
		t.Fatalf("identity_slot = %v", slot)
	}
}

// TestBuildPayloadYolo pins the Q-D3 (2026-10-05) export shape for a
// hosted yolo plan: permission_mode stays native per the frozen 1.0.0
// const while execution_profile carries yolo, and the source still
// matches the resolved decision.
func TestBuildPayloadYolo(t *testing.T) {
	in := validPayloadInputs(t)
	in.PermissionMode = agentic.PermissionModeYolo
	in.PermissionSource = "default-interactive"
	wire, err := hosted.BuildPayload(in)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(wire, &payload); err != nil {
		t.Fatal(err)
	}
	policy := payload["policy"].(map[string]any)
	if policy["permission_mode"] != "native" || policy["permission_source"] != "default-interactive" ||
		policy["execution_profile"] != "yolo" {
		t.Fatalf("policy = %v", policy)
	}
	recomputed, err := hosted.ContentDigest(wire)
	if err != nil || recomputed != payload["content_digest"] {
		t.Fatalf("digest recomputation = %q, %v", recomputed, err)
	}
}

func equalStrings(got any, want []string) bool {
	arr, ok := got.([]any)
	if !ok || len(arr) != len(want) {
		return false
	}
	for i := range want {
		if arr[i] != want[i] {
			return false
		}
	}
	return true
}

func reflectDeepEqual(got any, want map[string]string) bool {
	obj, ok := got.(map[string]any)
	if !ok || len(obj) != len(want) {
		return false
	}
	for k, v := range want {
		if obj[k] != v {
			return false
		}
	}
	return true
}

// TestBuildPayloadInvalid drives every validator branch: each row mutates
// one member of a valid input and names the expected field and reason.
func TestBuildPayloadInvalid(t *testing.T) {
	big := func(n int) string { return strings.Repeat("x", n) }
	rows := []struct {
		name   string
		mutate func(*hosted.PayloadInputs)
		field  string
		reason string
	}{
		{"env-not-claude", func(in *hosted.PayloadInputs) { in.EnvID = "codex_cli" }, "environment", "claude_code"},
		{"fragment-env-mismatch", func(in *hosted.PayloadInputs) { in.Fragment.Environment = "pi" }, "fragment", "match"},
		{"mode-unknown", func(in *hosted.PayloadInputs) { in.PermissionMode = "automatic" }, "policy.permission_mode", "native or yolo"},
		{"bad-source", func(in *hosted.PayloadInputs) { in.PermissionSource = "lineup" }, "policy.permission_source", "unknown"},
		{"host-empty", func(in *hosted.PayloadInputs) { in.HostName = "" }, "session_name.host", "must match"},
		{"host-leading-dot", func(in *hosted.PayloadInputs) { in.HostName = ".hidden" }, "session_name.host", "must match"},
		{"host-too-long", func(in *hosted.PayloadInputs) { in.HostName = "a" + big(64) }, "session_name.host", "must match"},
		{"binary-relative", func(in *hosted.PayloadInputs) { in.Value.Binary = "claude" }, "process.binary", "absolute"},
		{"binary-empty", func(in *hosted.PayloadInputs) { in.Value.Binary = "" }, "process.binary", "non-empty"},
		{"binary-nul", func(in *hosted.PayloadInputs) { in.Value.Binary = "/bin/\x00x" }, "process.binary", "NUL"},
		{"binary-too-long", func(in *hosted.PayloadInputs) { in.Value.Binary = "/" + big(64<<10) }, "process.binary", "exceeds"},
		{"argv-nil", func(in *hosted.PayloadInputs) { in.Value.Argv = nil; in.Restart.Data.Argv = nil }, "process.argv", "array"},
		{"argv-too-many", func(in *hosted.PayloadInputs) {
			in.Value.Argv = make([]string, 128)
			for i := range in.Value.Argv {
				in.Value.Argv[i] = "a"
			}
			in.Restart.Data.Argv = append([]string{}, in.Value.Argv...)
			in.Restart.Data.IdentitySlot.Index = len(in.Value.Argv)
		}, "process.argv", "exceed"},
		{"argv-nul", func(in *hosted.PayloadInputs) {
			in.Value.Argv = []string{"a\x00b"}
			in.Restart.Data.Argv = []string{"a\x00b"}
			in.Restart.Data.IdentitySlot.Index = 1
		}, "process.argv[0]", "NUL"},
		{"argv-element-too-long", func(in *hosted.PayloadInputs) {
			in.Value.Argv = []string{big(64<<10 + 1)}
			in.Restart.Data.Argv = append([]string{}, in.Value.Argv...)
			in.Restart.Data.IdentitySlot.Index = 1
		}, "process.argv[0]", "exceeds"},
		{"cwd-relative", func(in *hosted.PayloadInputs) { in.Value.WorkDir = "rel" }, "process.cwd", "absolute"},
		{"cwd-empty", func(in *hosted.PayloadInputs) { in.Value.WorkDir = "" }, "process.cwd", "non-empty"},
		{"env-nil", func(in *hosted.PayloadInputs) { in.Value.Env = nil }, "process.env", "array"},
		{"env-malformed", func(in *hosted.PayloadInputs) { in.Value.Env = append(in.Value.Env, "NOEQUALS") }, "process.env[3]", "NAME=value"},
		{"env-bad-name", func(in *hosted.PayloadInputs) { in.Value.Env = append(in.Value.Env, "9BAD=x") }, "process.env[3]", "NAME=value"},
		{"env-nul", func(in *hosted.PayloadInputs) { in.Value.Env = append(in.Value.Env, "OK=a\x00b") }, "process.env[3]", "NAME=value"},
		{"env-duplicate", func(in *hosted.PayloadInputs) { in.Value.Env = append(in.Value.Env, "HOME=elsewhere") }, "process.env[3]", "duplicate"},
		{"env-entry-too-long", func(in *hosted.PayloadInputs) { in.Value.Env = append(in.Value.Env, "BIG="+big(16<<10)) }, "process.env[3]", "exceeds"},
		{"managed-missing-from-env", func(in *hosted.PayloadInputs) {
			in.Value.Env = []string{"HOME=/tmp", "PATH=/bin"}
			in.Value.EnvLiterals = map[string]string{}
		}, "managed_home.path", "final env"},
		{"managed-mismatch", func(in *hosted.PayloadInputs) {
			in.Value.Env = []string{"CLAUDE_CONFIG_DIR=/elsewhere", "HOME=/tmp", "PATH=/bin"}
			in.Value.EnvLiterals = map[string]string{"CLAUDE_CONFIG_DIR": "/elsewhere"}
		}, "managed_home.path", "final env"},
		{"owned-too-many", func(in *hosted.PayloadInputs) {
			in.Value.EnvLiterals = map[string]string{}
			for i := 0; i < 65; i++ {
				name := "L" + strings.Repeat("0", 3) + string(rune('a'+i/26)) + string(rune('a'+i%26))
				in.Value.EnvLiterals[name] = "v"
				in.Value.Env = append(in.Value.Env, name+"=v")
			}
		}, "owned_literals", "exceed"},
		{"owned-bad-name", func(in *hosted.PayloadInputs) { in.Value.EnvLiterals["9bad"] = "v" }, "owned_literals", "valid"},
		{"owned-value-too-long", func(in *hosted.PayloadInputs) {
			in.Value.Env = append(in.Value.Env, "BIGLIT="+big(5000))
			in.Value.EnvLiterals["BIGLIT"] = big(5000)
		}, "owned_literals", "exceeds"},
		{"owned-value-nul", func(in *hosted.PayloadInputs) { in.Value.EnvLiterals["CLAUDE_CONFIG_DIR"] = "a\x00b" }, "owned_literals", "NUL"},
		{"owned-not-in-env", func(in *hosted.PayloadInputs) { in.Value.EnvLiterals["GHOST"] = "v" }, "owned_literals", "final env"},
		{"owned-differs-from-env", func(in *hosted.PayloadInputs) { in.Value.EnvLiterals["HOME"] = "stale" }, "owned_literals", "final env"},
		{"owned-overlaps-names", func(in *hosted.PayloadInputs) {
			in.Value.EnvNames = []string{"CLAUDE_CONFIG_DIR"}
			in.CallerEnv = append(in.CallerEnv, "CLAUDE_CONFIG_DIR=x")
		}, "owned_literals", "both a literal and a lookup"},
		{"names-nil", func(in *hosted.PayloadInputs) { in.Value.EnvNames = nil }, "env_names", "array"},
		{"names-unsorted", func(in *hosted.PayloadInputs) {
			in.Value.EnvNames = []string{"B_NAME", "A_NAME"}
			in.CallerEnv = append(in.CallerEnv, "B_NAME=1", "A_NAME=2")
		}, "env_names", "sorted unique"},
		{"names-duplicate", func(in *hosted.PayloadInputs) {
			in.Value.EnvNames = []string{"DUP", "DUP"}
			in.CallerEnv = append(in.CallerEnv, "DUP=1")
		}, "env_names", "sorted unique"},
		{"names-bad-name", func(in *hosted.PayloadInputs) { in.Value.EnvNames = []string{"9bad"} }, "env_names[0]", "match"},
		{"names-absent-refuses", func(in *hosted.PayloadInputs) { in.Value.EnvNames = []string{"MISSING_REQ"} }, "env_names", "required_env_missing MISSING_REQ"},
		{"sealed-guard", func(in *hosted.PayloadInputs) { in.Seal.Data.Kind = "sealed" }, "process.exec_guard", "unsealed"},
		{"guard-wrong-version", func(in *hosted.PayloadInputs) { in.Seal.SchemaVersion = "2.0.0" }, "process.exec_guard", "unsealed"},
		{"restart-bad-schema", func(in *hosted.PayloadInputs) { in.Restart.SchemaVersion = "2.0.0" }, "restart", "unsupported restart schema"},
		{"restart-bad-slot-flag", func(in *hosted.PayloadInputs) { in.Restart.Data.IdentitySlot.Flag = "-r" }, "restart.data.identity_slot", "slot"},
		{"restart-bad-slot-index", func(in *hosted.PayloadInputs) { in.Restart.Data.IdentitySlot.Index = 99 }, "restart.data.identity_slot", "slot"},
		{"restart-argv-mismatch", func(in *hosted.PayloadInputs) {
			in.Restart.Data.Argv = []string{"other"}
			in.Restart.Data.IdentitySlot.Index = 1
		}, "restart.data.argv", "must equal"},
		{"restart-argv-nil", func(in *hosted.PayloadInputs) { in.Restart.Data.Argv = nil }, "restart.data.argv", "array"},
		{"resume-identity-on-new", func(in *hosted.PayloadInputs) {
			id := "x"
			in.Resume = agentic.ResumeIntent{Kind: agentic.ResumeNew, Identity: &id}
		}, "resume.identity", "no identity"},
		{"resume-missing-handle-identity", func(in *hosted.PayloadInputs) {
			in.Resume = agentic.ResumeIntent{Kind: agentic.ResumeHandle}
		}, "resume.identity", "require an identity"},
		{"resume-foreign-kind", func(in *hosted.PayloadInputs) {
			id := "x"
			in.Resume = agentic.ResumeIntent{Kind: agentic.ResumeCodexThread, Identity: &id}
		}, "resume.kind", "not admitted"},
		{"policy-unknown-selector", func(in *hosted.PayloadInputs) {
			in.Inspection = agentic.StoredPolicyInspection{
				Support:          agentic.StoredPolicySupported,
				Relaxations:      []agentic.StoredPolicyRelaxation{{Selector: "permissions.unknown", SourcePath: "/abs/settings.json"}},
				SourcesInspected: []string{"/abs/settings.json"},
			}
		}, "policy.effective_native_policy.data.relaxations", "closed vocabulary"},
		{"policy-relative-path", func(in *hosted.PayloadInputs) {
			in.Inspection = agentic.StoredPolicyInspection{
				Support:          agentic.StoredPolicySupported,
				Relaxations:      []agentic.StoredPolicyRelaxation{{Selector: "permissions.allow", SourcePath: "rel/settings.json"}},
				SourcesInspected: []string{"rel/settings.json"},
			}
		}, "policy.effective_native_policy.data.source_paths", "absolute"},
		{"profile-name-empty", func(in *hosted.PayloadInputs) { in.Fragment.Profile.Name = "" }, "fragment.profile_name", "no profile name"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			in := validPayloadInputs(t)
			row.mutate(&in)
			got := buildErr(t, in)
			if got.Field != row.field || !strings.Contains(got.Reason, row.reason) {
				t.Fatalf("field:reason = %q:%q, want %q containing %q", got.Field, got.Reason, row.field, row.reason)
			}
		})
	}
}
