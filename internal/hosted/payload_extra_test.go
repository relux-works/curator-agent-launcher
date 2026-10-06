package hosted_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/relux-works/skill-agents-management/pkg/agentic"

	"github.com/relux-works/curator-agent-launcher/internal/hosted"
)

// TestBuildPayloadEnvNamesAbsentVsEmpty proves the lookup contract: a
// present empty value satisfies the requirement, while an absent name
// refuses required_env_missing. Absent and empty are never conflated.
func TestBuildPayloadEnvNamesAbsentVsEmpty(t *testing.T) {
	in := validPayloadInputs(t)
	in.Value.EnvNames = []string{"PRESENT_EMPTY"}
	in.CallerEnv = append(in.CallerEnv, "PRESENT_EMPTY=")
	if _, err := hosted.BuildPayload(in); err != nil {
		t.Fatalf("present-empty lookup refused: %v", err)
	}
	in.Value.EnvNames = []string{"ABSENT_REQ"}
	if err := buildErr(t, in); err.Field != "env_names" || !strings.Contains(err.Reason, "required_env_missing ABSENT_REQ") {
		t.Fatalf("absent lookup: %+v", err)
	}
}

// TestBuildPayloadResumeKinds pins the resume projection for every admitted
// kind: new and latest carry null identity, handle and UUID carry theirs.
func TestBuildPayloadKinds(t *testing.T) {
	uuid := "01234567-89ab-cdef-0123-456789abcdef"
	handle := "SES-abc123"
	for _, tc := range []struct {
		intent agentic.ResumeIntent
		kind   string
		id     *string
	}{
		{agentic.ResumeIntent{Kind: agentic.ResumeNew}, "new", nil},
		{agentic.ResumeIntent{Kind: agentic.ResumeLatest}, "latest", nil},
		{agentic.ResumeIntent{Kind: agentic.ResumeHandle, Identity: &handle}, "handle", &handle},
		{agentic.ResumeIntent{Kind: agentic.ResumeClaudeUUID, Identity: &uuid}, "claude_uuid", &uuid},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			in := validPayloadInputs(t)
			in.Resume = tc.intent
			wire, err := hosted.BuildPayload(in)
			if err != nil {
				t.Fatal(err)
			}
			var payload map[string]any
			if err := json.Unmarshal(wire, &payload); err != nil {
				t.Fatal(err)
			}
			resume := payload["resume"].(map[string]any)
			if resume["kind"] != tc.kind {
				t.Fatalf("kind = %v, want %s", resume["kind"], tc.kind)
			}
			if tc.id == nil {
				if resume["identity"] != nil {
					t.Fatalf("identity = %v, want null", resume["identity"])
				}
				return
			}
			if resume["identity"] != *tc.id {
				t.Fatalf("identity = %v, want %s", resume["identity"], *tc.id)
			}
		})
	}
}

// TestBuildPayloadEffectivePolicy pins the closed projection: only sorted
// detected selectors and sorted inspected absolute source paths travel,
// uninspected and doubly-listed sources never project, and permission
// values never leave the inspection.
func TestBuildPayloadEffectivePolicy(t *testing.T) {
	t.Run("empty-inspection-is-null", func(t *testing.T) {
		in := validPayloadInputs(t)
		wire, err := hosted.BuildPayload(in)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(wire, &payload); err != nil {
			t.Fatal(err)
		}
		if got := payload["policy"].(map[string]any)["effective_native_policy"]; got != nil {
			t.Fatalf("effective_native_policy = %v, want null", got)
		}
	})
	t.Run("relaxations-project-sorted-without-values", func(t *testing.T) {
		in := validPayloadInputs(t)
		in.Inspection = agentic.StoredPolicyInspection{
			Support: agentic.StoredPolicySupported,
			Relaxations: []agentic.StoredPolicyRelaxation{
				{Selector: "permissions.allow", Value: "SECRET-CANARY-VALUE", SourcePath: "/b/settings.json"},
				{Selector: "permissions.defaultMode", Value: "acceptEdits", SourcePath: "/a/settings.json"},
				{Selector: "permissions.allow", Value: "other", SourcePath: "/a/settings.json"},
			},
			SourcesInspected: []string{"/a/settings.json", "/b/settings.json"},
		}
		wire, err := hosted.BuildPayload(in)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(wire), "SECRET-CANARY-VALUE") {
			t.Fatal("permission value leaked into the payload")
		}
		var payload map[string]any
		if err := json.Unmarshal(wire, &payload); err != nil {
			t.Fatal(err)
		}
		effective := payload["policy"].(map[string]any)["effective_native_policy"].(map[string]any)
		if effective["schema"] != agentic.ClaudeEffectivePolicySchema || effective["schema_version"] != "1.0.0" {
			t.Fatalf("effective policy envelope = %v", effective)
		}
		data := effective["data"].(map[string]any)
		if !equalStrings(data["relaxations"], []string{"permissions.allow", "permissions.defaultMode"}) {
			t.Fatalf("relaxations = %v", data["relaxations"])
		}
		if !equalStrings(data["source_paths"], []string{"/a/settings.json", "/b/settings.json"}) {
			t.Fatalf("source_paths = %v", data["source_paths"])
		}
	})
	t.Run("uninspected-and-doubly-listed-never-project", func(t *testing.T) {
		in := validPayloadInputs(t)
		in.Inspection = agentic.StoredPolicyInspection{
			Support: agentic.StoredPolicySupported,
			Relaxations: []agentic.StoredPolicyRelaxation{
				{Selector: "permissions.allow", SourcePath: "/uninspected/settings.json"},
				{Selector: "permissions.allow", SourcePath: "/both/settings.json"},
				{Selector: "", SourcePath: "/inspected/settings.json"},
				{Selector: "permissions.allow", SourcePath: ""},
			},
			SourcesInspected: []string{"/both/settings.json", "/inspected/settings.json"},
			SourcesNotInspected: []agentic.StoredPolicySourceIssue{
				{SourcePath: "/uninspected/settings.json", Reason: agentic.StoredPolicySourceUnreadable},
				{SourcePath: "/both/settings.json", Reason: agentic.StoredPolicySourceUnreadable},
			},
		}
		wire, err := hosted.BuildPayload(in)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(wire, &payload); err != nil {
			t.Fatal(err)
		}
		if got := payload["policy"].(map[string]any)["effective_native_policy"]; got != nil {
			t.Fatalf("effective_native_policy = %v, want null", got)
		}
	})
	t.Run("too-many-paths-refuse", func(t *testing.T) {
		in := validPayloadInputs(t)
		inspection := agentic.StoredPolicyInspection{Support: agentic.StoredPolicySupported}
		for i := 0; i < 65; i++ {
			path := fmt.Sprintf("/s%02d/settings.json", i)
			inspection.Relaxations = append(inspection.Relaxations, agentic.StoredPolicyRelaxation{Selector: "permissions.allow", SourcePath: path})
			inspection.SourcesInspected = append(inspection.SourcesInspected, path)
		}
		in.Inspection = inspection
		got := buildErr(t, in)
		if got.Field != "policy.effective_native_policy.data.source_paths" {
			t.Fatalf("field = %q", got.Field)
		}
	})
}

// TestBuildPayloadEncodedLimits pins the limits measured over canonical
// bytes: the encoded argv array and each versioned data object. The restart
// data mirrors the process argv, so a large argv trips the versioned-data
// bound even when every argv element is within its own bound.
func TestBuildPayloadEncodedLimits(t *testing.T) {
	t.Run("encoded-argv-bound", func(t *testing.T) {
		in := validPayloadInputs(t)
		// Five 60 KiB elements: each within the element bound, the encoded
		// array past 256 KiB.
		in.Value.Argv = nil
		for i := 0; i < 5; i++ {
			in.Value.Argv = append(in.Value.Argv, strings.Repeat("a", 60<<10))
		}
		in.Restart.Data.Argv = append([]string{}, in.Value.Argv...)
		in.Restart.Data.IdentitySlot.Index = len(in.Value.Argv)
		got := buildErr(t, in)
		if got.Field != "process.argv" || !strings.Contains(got.Reason, "encoded argv exceeds") {
			t.Fatalf("field:reason = %q:%q", got.Field, got.Reason)
		}
	})
	t.Run("restart-data-bound", func(t *testing.T) {
		in := validPayloadInputs(t)
		// Two 35 KiB elements: argv limits pass, restart data past 64 KiB.
		in.Value.Argv = []string{strings.Repeat("a", 35<<10), strings.Repeat("b", 35<<10)}
		in.Restart.Data.Argv = append([]string{}, in.Value.Argv...)
		in.Restart.Data.IdentitySlot.Index = len(in.Value.Argv)
		got := buildErr(t, in)
		if got.Field != "restart.data" || !strings.Contains(got.Reason, "versioned data exceeds") {
			t.Fatalf("field:reason = %q:%q", got.Field, got.Reason)
		}
	})
	t.Run("env-total-bound", func(t *testing.T) {
		in := validPayloadInputs(t)
		for i := 0; i < 20; i++ {
			in.Value.Env = append(in.Value.Env, fmt.Sprintf("PAD%02d=%s", i, strings.Repeat("p", 13<<10)))
		}
		got := buildErr(t, in)
		if got.Field != "process.env" || !strings.Contains(got.Reason, "exceeds") {
			t.Fatalf("field:reason = %q:%q", got.Field, got.Reason)
		}
	})
	t.Run("wire-bound", func(t *testing.T) {
		in := validPayloadInputs(t)
		// The member limits compose below 1 MiB, so the wire backstop
		// is probed through the one unbounded member: cwd carries no
		// member limit in the contract, and a 2 MiB cwd passes every
		// member check but exceeds the wire object bound.
		in.Value.WorkDir = "/" + strings.Repeat("w", 2<<20)
		got := buildErr(t, in)
		if got.Field != "payload" || !strings.Contains(got.Reason, "exceeds") {
			t.Fatalf("field:reason = %q:%q", got.Field, got.Reason)
		}
	})
	t.Run("env-names-maxitems", func(t *testing.T) {
		names := func(n int) ([]string, []string) {
			var lookup, caller []string
			for i := 0; i < n; i++ {
				name := fmt.Sprintf("LOOKUP_%02d", i)
				lookup = append(lookup, name)
				caller = append(caller, name+"=fixture")
			}
			return lookup, caller
		}
		in := validPayloadInputs(t)
		lookup, caller := names(64)
		in.Value.EnvNames = lookup
		in.CallerEnv = append(in.CallerEnv, caller...)
		if _, err := hosted.BuildPayload(in); err != nil {
			t.Fatalf("64 lookup names refused: %v", err)
		}
		in = validPayloadInputs(t)
		lookup, caller = names(65)
		in.Value.EnvNames = lookup
		in.CallerEnv = append(in.CallerEnv, caller...)
		got := buildErr(t, in)
		if got.Field != "env_names" || !strings.Contains(got.Reason, "64") {
			t.Fatalf("field:reason = %q:%q", got.Field, got.Reason)
		}
	})
	t.Run("session-projection", func(t *testing.T) {
		// A module-owned projection carries verbatim: the native
		// name and the RC intent with exact final-argv indices.
		in := validPayloadInputs(t)
		name := "NATNAME"
		in.Session = &hosted.SessionProjection{NativeName: &name, RCEnabled: true, RCIndices: []int{2, 0}}
		wire, err := hosted.BuildPayload(in)
		if err != nil {
			t.Fatalf("projected session refused: %v", err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(wire, &decoded); err != nil {
			t.Fatal(err)
		}
		session := decoded["session_name"].(map[string]any)
		if session["native"] != "NATNAME" {
			t.Fatalf("session_name.native = %v, want NATNAME", session["native"])
		}
		rc := decoded["remote_control"].(map[string]any)
		if rc["enabled"] != true {
			t.Fatalf("remote_control.enabled = %v, want true", rc["enabled"])
		}
		indices, ok := rc["argv_indices"].([]any)
		if !ok || len(indices) != 2 || indices[0] != 2.0 || indices[1] != 0.0 {
			t.Fatalf("remote_control.argv_indices = %v, want [2 0] in order", rc["argv_indices"])
		}
		// Settings-origin RC: enabled with empty indices.
		in = validPayloadInputs(t)
		in.Session = &hosted.SessionProjection{RCEnabled: true, RCIndices: []int{}}
		wire, err = hosted.BuildPayload(in)
		if err != nil {
			t.Fatalf("settings-origin RC refused: %v", err)
		}
		if err := json.Unmarshal(wire, &decoded); err != nil {
			t.Fatal(err)
		}
		rc = decoded["remote_control"].(map[string]any)
		if rc["enabled"] != true || len(rc["argv_indices"].([]any)) != 0 {
			t.Fatalf("settings-origin RC = %v, want enabled with []", rc)
		}
		// Nil projection emits the schema-zero metadata (the F2
		// gap baseline: the pinned module supplies no projection).
		in = validPayloadInputs(t)
		wire, err = hosted.BuildPayload(in)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(wire, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded["session_name"].(map[string]any)["native"] != nil {
			t.Fatalf("nil projection native = %v, want null", decoded["session_name"])
		}
		rc = decoded["remote_control"].(map[string]any)
		if rc["enabled"] != false || len(rc["argv_indices"].([]any)) != 0 {
			t.Fatalf("nil projection RC = %v, want disabled with []", rc)
		}
		// Incoherent projections refuse before contact.
		for _, tc := range []struct {
			name string
			proj hosted.SessionProjection
		}{
			{"disabled-with-indices", hosted.SessionProjection{RCIndices: []int{0}}},
			{"duplicate-index", hosted.SessionProjection{RCEnabled: true, RCIndices: []int{1, 1}}},
			{"negative-index", hosted.SessionProjection{RCEnabled: true, RCIndices: []int{-1}}},
			{"index-past-end", hosted.SessionProjection{RCEnabled: true, RCIndices: []int{3}}},
		} {
			in = validPayloadInputs(t)
			proj := tc.proj
			in.Session = &proj
			got := buildErr(t, in)
			if got.Field != "remote_control.argv_indices" {
				t.Fatalf("%s: field = %q, want remote_control.argv_indices", tc.name, got.Field)
			}
		}
	})
	t.Run("source-path-length", func(t *testing.T) {
		in := validPayloadInputs(t)
		path := "/" + strings.Repeat("p", 4096)
		in.Inspection = agentic.StoredPolicyInspection{
			Relaxations:      []agentic.StoredPolicyRelaxation{{Selector: "permissions.allow", SourcePath: path}},
			SourcesInspected: []string{path},
		}
		got := buildErr(t, in)
		if got.Field != "policy.effective_native_policy.data.source_paths" || !strings.Contains(got.Reason, "4096") {
			t.Fatalf("field:reason = %q:%q", got.Field, got.Reason)
		}
	})
}

// TestBuildPayloadDiagnosticsCarryNoValues forces several refusals over a
// secret-bearing env and proves the details name fields and reasons only.
func TestBuildPayloadDiagnosticsCarryNoValues(t *testing.T) {
	const canary = "SECRET-CANARY-29f3b1"
	base := func(t *testing.T) hosted.PayloadInputs {
		t.Helper()
		in := validPayloadInputs(t)
		in.Value.Env = append(in.Value.Env, "TOKEN="+canary)
		in.Value.EnvLiterals["TOKEN"] = canary
		in.CallerEnv = append(in.CallerEnv, "TOKEN="+canary)
		return in
	}
	mutations := []func(*hosted.PayloadInputs){
		func(in *hosted.PayloadInputs) { in.Value.EnvNames = []string{"MISSING_REQ"} },
		func(in *hosted.PayloadInputs) { in.Value.Binary = "relative" },
		func(in *hosted.PayloadInputs) { in.Value.EnvLiterals["HOME"] = "stale" },
		func(in *hosted.PayloadInputs) { in.HostName = "bad name" },
	}
	for i, mutate := range mutations {
		in := base(t)
		mutate(&in)
		_, err := hosted.BuildPayload(in)
		if err == nil {
			t.Fatalf("row %d: BuildPayload succeeded", i)
		}
		if strings.Contains(err.Error(), canary) {
			t.Fatalf("row %d: diagnostic %q carries the secret value", i, err.Error())
		}
	}
	// The valid projection carries the value on the wire (stdin-only in
	// production) but the builder reports nothing.
	in := base(t)
	wire, err := hosted.BuildPayload(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wire), canary) {
		t.Fatal("valid wire lost the env value")
	}
}

// TestProducerMatchesBuild pins the payload producer identity to the build:
// the version equals the go.mod require resolved by the go command, and the
// commit is a non-placeholder 40-hex digest. It reads the module graph the
// same way the release pin test does: test binaries in this tree carry no
// dependency build info, so debug.ReadBuildInfo is not an oracle here.
func TestProducerMatchesBuild(t *testing.T) {
	if hosted.ProducerModuleVersion == "" {
		t.Fatal("empty producer version")
	}
	cmd := exec.Command("go", "list", "-m", "-json", "github.com/relux-works/skill-agents-management")
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GOWORK=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("read module identity: %v", err)
	}
	var module struct {
		Version string
	}
	if err := json.Unmarshal(out, &module); err != nil {
		t.Fatal(err)
	}
	if module.Version != "v"+hosted.ProducerModuleVersion {
		t.Fatalf("producer version %q != resolved module %q", hosted.ProducerModuleVersion, module.Version)
	}
	commit := hosted.ProducerModuleCommit
	if len(commit) != 40 {
		t.Fatalf("producer commit %q is not 40 hex", commit)
	}
	for i := 0; i < len(commit); i++ {
		if !strings.ContainsRune("0123456789abcdef", rune(commit[i])) {
			t.Fatalf("producer commit %q is not lowercase hex", commit)
		}
	}
	for _, placeholder := range []string{
		strings.Repeat("0", 40), strings.Repeat("1", 40),
	} {
		if commit == placeholder {
			t.Fatalf("producer commit is the placeholder %q", commit)
		}
	}
}

// TestSessionFromPlan pins the Plan.Session lift: a nil record stays nil, the
// name is copied, indices carry over while the composed argv equals the plan
// argv, and a composed argv that differs or an index outside it refuses.
func TestSessionFromPlan(t *testing.T) {
	name := "N"
	argv := []string{"a", "--remote-control", "N", "-n", "N"}
	t.Run("nil-stays-nil", func(t *testing.T) {
		got, err := hosted.SessionFromPlan(nil, argv, argv)
		if got != nil || err != nil {
			t.Fatalf("got %v, %v", got, err)
		}
	})
	t.Run("carries-name-and-indices", func(t *testing.T) {
		in := &agentic.PlanSession{Name: &name, RCEnabled: true, RCIndices: []int{1}}
		got, err := hosted.SessionFromPlan(in, argv, slices.Clone(argv))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got.RCIndices, []int{1}) || got.NativeName == nil || *got.NativeName != "N" || !got.RCEnabled {
			t.Fatalf("got %+v", got)
		}
		if got.NativeName == in.Name {
			t.Fatal("name pointer aliases the module record")
		}
	})
	t.Run("settings-origin-keeps-empty-indices", func(t *testing.T) {
		got, err := hosted.SessionFromPlan(&agentic.PlanSession{RCEnabled: true, RCIndices: []int{}}, argv, argv)
		if err != nil || !got.RCEnabled || got.RCIndices == nil || len(got.RCIndices) != 0 {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
	for caseName, tc := range map[string]struct {
		index    int
		composed []string
	}{
		"index-beyond-argv":       {index: 5, composed: argv},
		"negative-index":          {index: -1, composed: argv},
		"composed-inserts-tokens": {index: 1, composed: []string{"a", "--extra", "--remote-control", "N", "-n", "N"}},
		"composed-truncated":      {index: 1, composed: argv[:4]},
	} {
		t.Run(caseName, func(t *testing.T) {
			_, err := hosted.SessionFromPlan(&agentic.PlanSession{RCEnabled: true, RCIndices: []int{tc.index}}, argv, tc.composed)
			var planErr *hosted.PlanError
			if !errors.As(err, &planErr) {
				t.Fatalf("err = %v, want *PlanError", err)
			}
		})
	}
}
