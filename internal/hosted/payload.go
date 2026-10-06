package hosted

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/relux-works/skill-agents-management/pkg/agentic"
	claudeSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/claude"

	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

// Wire limits of the closed payload shape (§3). Violations refuse
// launch_plan_invalid before any receiver contact.
const (
	LimitWireBytes          = 1 << 20
	LimitDepth              = 16
	LimitArgvElements       = 128 // including the binary
	LimitArgvElementBytes   = 64 << 10
	LimitArgvEncodedBytes   = 256 << 10
	LimitEnvEntries         = 512
	LimitEnvTotalBytes      = 256 << 10
	LimitEnvEntryBytes      = 16 << 10
	LimitOwnedEntries       = 64
	LimitOwnedValueBytes    = 4096
	LimitVersionedDataBytes = 64 << 10
)

// ManagedHomeVariable is the Phase 1 managed-home variable.
const ManagedHomeVariable = "CLAUDE_CONFIG_DIR"

// PlanError is a launch_plan_invalid refusal: the composed plan cannot be
// projected into the closed payload shape. Field names the offending member
// (with an index where one applies); Reason names the rule. Neither carries
// values: names, indices, and sizes only.
type PlanError struct {
	Code   string
	Field  string
	Reason string
}

func (e *PlanError) Error() string { return e.Code + ": " + e.Field + ": " + e.Reason }

func planErr(field, reason string) error {
	return &PlanError{Code: CodeLaunchPlanInvalid, Field: field, Reason: reason}
}

// PayloadInputs are the validated pipeline products the payload projects.
// Value is the once-composed plan (module/owned snapshot, fragment env,
// prompt env, selector-free native tail); Resume carries the elevated intent;
// Restart and Seal are the module exports for the composed process;
// Inspection is the stored-policy inspection of the admitted plan.
type PayloadInputs struct {
	EnvID            string
	Value            composition.Value
	Fragment         fragment.Fragment
	CallerEnv        []string
	PermissionMode   agentic.PermissionMode
	PermissionSource string
	HostName         string
	Resume           agentic.ResumeIntent
	Restart          claudeSystem.RestartTemplate
	Seal             agentic.Seal
	Inspection       agentic.StoredPolicyInspection
	// Session is the module-filled native session name and
	// remote-control intent for the final argv, taken from Plan.Session
	// of the same plan the payload is built from (see SessionFromPlan).
	// Nil for a plan whose system defines no native session surface:
	// the payload then carries the schema-zero metadata. The launcher
	// never parses provider options to fill this in.
	Session *SessionProjection
}

// SessionProjection is the module-owned native session metadata for the
// composed final argv: the native session name (nil when the plan sets
// none) and the remote-control intent (enabled with the exact final-argv
// indices of the RC tokens, or disabled with no indices). An enabled RC
// with empty indices is the settings-origin form. The projection MUST
// come from the module's typed grammar (agentic.Plan.Session) — hand-parsing
// provider flags in the launcher would duplicate and drift from the module's
// arity tables, so a nil projection emits zero metadata rather than guessed
// metadata.
type SessionProjection struct {
	NativeName *string
	RCEnabled  bool
	RCIndices  []int
}

// BuildPayload validates the inputs against the closed
// urn:relux:task-board:session-launch-plan 1.0.0 shape and returns the
// canonical wire bytes with the content digest attached. Anything the shape
// cannot honestly carry refuses *PlanError before any receiver contact.
func BuildPayload(in PayloadInputs) ([]byte, error) {
	if in.EnvID != PayloadEnvClaude {
		return nil, planErr("environment", "must be claude_code in Phase 1")
	}
	if in.Fragment.Environment != PayloadEnvClaude {
		return nil, planErr("fragment", "fragment environment must match the payload environment")
	}
	// Q-D3 (2026-10-05) admits hosted yolo. The frozen 1.0.0 schema
	// constrains permission_mode to native, so posture rides
	// execution_profile: native maps to standard, yolo to yolo.
	// Anything outside the closed mode pair cannot be projected.
	var executionProfile string
	switch in.PermissionMode {
	case agentic.PermissionModeNative:
		executionProfile = "standard"
	case agentic.PermissionModeYolo:
		executionProfile = "yolo"
	default:
		return nil, planErr("policy.permission_mode", "must be native or yolo")
	}
	switch in.PermissionSource {
	case "flag", "profile", "global", "default-headless", "default-interactive":
	default:
		return nil, planErr("policy.permission_source", "unknown permission source")
	}
	if !validSessionName(in.HostName) {
		return nil, planErr("session_name.host", "must match [A-Za-z0-9][A-Za-z0-9._-]{0,63}")
	}
	if err := checkProcess(in.Value); err != nil {
		return nil, err
	}
	finalEnv, err := envMap(in.Value.Env, "process.env")
	if err != nil {
		return nil, err
	}
	managedPath, ok := in.Fragment.Env[ManagedHomeVariable]
	if !ok || managedPath == "" {
		return nil, planErr("managed_home.path", "admitted fragment carries no managed home")
	}
	if !isAbsolutePath(managedPath) {
		return nil, planErr("managed_home.path", "must be absolute")
	}
	if got, ok := finalEnv[ManagedHomeVariable]; !ok || got != managedPath {
		return nil, planErr("managed_home.path", "must equal the final env entry")
	}
	if err := checkOwned(in.Value.EnvLiterals, finalEnv, in.Value.EnvNames); err != nil {
		return nil, err
	}
	if err := checkEnvNames(in.Value.EnvNames, in.CallerEnv); err != nil {
		return nil, err
	}
	if !in.Seal.HostedAdmissible() {
		return nil, planErr("process.exec_guard", "only the closed unsealed guard is admissible in Phase 1")
	}
	if err := checkRestart(in.Restart, in.Value.Argv); err != nil {
		return nil, err
	}
	resume, err := projectResume(in.Resume)
	if err != nil {
		return nil, err
	}
	nativeName, rcEnabled, rcIndices, err := projectSession(in.Session, in.Value.Argv)
	if err != nil {
		return nil, err
	}
	effective, err := projectEffectivePolicy(in.Inspection)
	if err != nil {
		return nil, err
	}
	if in.Fragment.Profile.Name == "" {
		return nil, planErr("fragment.profile_name", "admitted fragment carries no profile name")
	}

	owned := map[string]string{}
	for name, value := range in.Value.EnvLiterals {
		owned[name] = value
	}
	payload := wirePayload{
		Schema:        PayloadSchema,
		SchemaVersion: PayloadVersion,
		Producer:      wireProducer{ModuleVersion: ProducerModuleVersion, ModuleCommit: ProducerModuleCommit},
		Environment:   PayloadEnvClaude,
		Process: wireProcess{
			Binary: in.Value.Binary, Argv: append([]string{}, in.Value.Argv...),
			Env: append([]string{}, in.Value.Env...), Cwd: in.Value.WorkDir,
			ExecGuard: in.Seal,
		},
		OwnedLiterals: owned,
		EnvNames:      append([]string{}, in.Value.EnvNames...),
		ManagedHome:   wireManagedHome{Variable: ManagedHomeVariable, Path: managedPath},
		Fragment: wireFragment{
			ProfileName: in.Fragment.Profile.Name, Pin: "sha256:" + in.Fragment.Profile.LockSHA256,
			FragmentDigest: in.Fragment.Digest, SystemModules: in.Fragment.SystemPrompt != nil,
		},
		Policy: wirePolicy{
			PermissionMode: string(agentic.PermissionModeNative), PermissionSource: in.PermissionSource,
			ExecutionProfile: executionProfile, EffectiveNativePolicy: effective,
		},
		SessionName:   wireSessionName{Host: in.HostName, Native: nativeName},
		RemoteControl: wireRemoteControl{Enabled: rcEnabled, ArgvIndices: rcIndices},
		Resume:        resume,
		Restart:       in.Restart,
	}
	undigested, err := json.Marshal(payload)
	if err != nil {
		return nil, planErr("payload", "cannot encode the payload object")
	}
	digest, err := ContentDigest(undigested)
	if err != nil {
		return nil, err
	}
	payload.ContentDigest = digest
	wire, err := json.Marshal(payload)
	if err != nil {
		return nil, planErr("payload", "cannot encode the payload object")
	}
	wireValue, err := fragment.ParseJSON(wire)
	if err != nil {
		return nil, planErr("payload", "encoded payload is not CCJ-1 JSON")
	}
	wire = fragment.Canonical(wireValue)
	if len(wire) > LimitWireBytes {
		return nil, planErr("payload", fmt.Sprintf("wire object exceeds %d bytes", LimitWireBytes))
	}
	if depth := valueDepth(wireValue, 0); depth > LimitDepth {
		return nil, planErr("payload", fmt.Sprintf("object depth %d exceeds %d", depth, LimitDepth))
	}
	if err := checkEncodedLimits(wireValue); err != nil {
		return nil, err
	}
	return wire, nil
}

func checkProcess(v composition.Value) error {
	if v.Binary == "" || strings.IndexByte(v.Binary, 0) >= 0 {
		return planErr("process.binary", "must be a non-empty NUL-free path")
	}
	if !isAbsolutePath(v.Binary) {
		return planErr("process.binary", "must be absolute")
	}
	if len(v.Binary) > LimitArgvElementBytes {
		return planErr("process.binary", fmt.Sprintf("exceeds %d bytes", LimitArgvElementBytes))
	}
	if v.Argv == nil {
		return planErr("process.argv", "must be an array")
	}
	if 1+len(v.Argv) > LimitArgvElements {
		return planErr("process.argv", fmt.Sprintf("%d elements including the binary exceed %d", 1+len(v.Argv), LimitArgvElements))
	}
	for i, arg := range v.Argv {
		if strings.IndexByte(arg, 0) >= 0 {
			return planErr(fmt.Sprintf("process.argv[%d]", i), "entry carries NUL")
		}
		if len(arg) > LimitArgvElementBytes {
			return planErr(fmt.Sprintf("process.argv[%d]", i), fmt.Sprintf("exceeds %d bytes", LimitArgvElementBytes))
		}
	}
	if v.WorkDir == "" || strings.IndexByte(v.WorkDir, 0) >= 0 {
		return planErr("process.cwd", "must be a non-empty NUL-free path")
	}
	if !isAbsolutePath(v.WorkDir) {
		return planErr("process.cwd", "must be absolute")
	}
	return nil
}

// envMap parses a final env into a name map, enforcing entry shape, count,
// and size. Names are unique by construction of the composer; a duplicate
// here is a caller bug and refuses.
func envMap(env []string, field string) (map[string]string, error) {
	if env == nil {
		return nil, planErr(field, "must be an array")
	}
	if len(env) > LimitEnvEntries {
		return nil, planErr(field, fmt.Sprintf("%d entries exceed %d", len(env), LimitEnvEntries))
	}
	total := 0
	out := make(map[string]string, len(env))
	for i, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || !validEnvName(name) || strings.IndexByte(value, 0) >= 0 {
			return nil, planErr(fmt.Sprintf("%s[%d]", field, i), "entry must be NAME=value with a valid name and no NUL")
		}
		if len(entry) > LimitEnvEntryBytes {
			return nil, planErr(fmt.Sprintf("%s[%d]", field, i), fmt.Sprintf("entry exceeds %d bytes", LimitEnvEntryBytes))
		}
		if _, dup := out[name]; dup {
			return nil, planErr(fmt.Sprintf("%s[%d]", field, i), "duplicate environment name")
		}
		out[name] = value
		total += len(entry)
	}
	if total > LimitEnvTotalBytes {
		return nil, planErr(field, fmt.Sprintf("env exceeds %d bytes", LimitEnvTotalBytes))
	}
	return out, nil
}

func checkOwned(literals map[string]string, final map[string]string, names []string) error {
	if len(literals) > LimitOwnedEntries {
		return planErr("owned_literals", fmt.Sprintf("%d entries exceed %d", len(literals), LimitOwnedEntries))
	}
	lookup := make(map[string]bool, len(names))
	for _, name := range names {
		lookup[name] = true
	}
	for _, name := range slices.Sorted(maps.Keys(literals)) {
		value := literals[name]
		if !validEnvName(name) || strings.IndexByte(value, 0) >= 0 {
			return planErr("owned_literals", "names must be valid and values NUL-free")
		}
		if len(value) > LimitOwnedValueBytes {
			return planErr("owned_literals", fmt.Sprintf("value for %s exceeds %d bytes", name, LimitOwnedValueBytes))
		}
		if lookup[name] {
			return planErr("owned_literals", fmt.Sprintf("%s is both a literal and a lookup name", name))
		}
		if got, ok := final[name]; !ok || got != value {
			return planErr("owned_literals", fmt.Sprintf("value for %s must equal the final env entry", name))
		}
	}
	return nil
}

// checkEnvNames enforces the lookup contract: at most 64 sorted unique
// valid names, each present in the original caller env. A present empty
// value satisfies the requirement; an absent name refuses
// required_env_missing. Absent and empty are never conflated.
func checkEnvNames(names []string, caller []string) error {
	if names == nil {
		return planErr("env_names", "must be an array")
	}
	if len(names) > 64 {
		return planErr("env_names", "more than 64 lookup names cannot be projected")
	}
	present := make(map[string]bool, len(caller))
	for _, entry := range caller {
		if name, _, ok := strings.Cut(entry, "="); ok {
			present[name] = true
		}
	}
	for i, name := range names {
		if !validEnvName(name) {
			return planErr(fmt.Sprintf("env_names[%d]", i), "name must match [A-Za-z_][A-Za-z0-9_]{0,127}")
		}
		if i > 0 && names[i-1] >= name {
			return planErr("env_names", "names must be sorted unique")
		}
		if !present[name] {
			return planErr("env_names", "required_env_missing "+name)
		}
	}
	return nil
}

func checkRestart(restart claudeSystem.RestartTemplate, argv []string) error {
	if restart.Schema != claudeSystem.RestartSchema || restart.SchemaVersion != claudeSystem.RestartSchemaVersion {
		return planErr("restart", "unsupported restart schema")
	}
	slot := restart.Data.IdentitySlot
	if restart.Data.Argv == nil {
		return planErr("restart.data.argv", "must be an array")
	}
	if slot.Flag != "--resume" || slot.Index < 0 || slot.Index > len(restart.Data.Argv) {
		return planErr("restart.data.identity_slot", "slot must be --resume at an index within the argv")
	}
	if !slices.Equal(restart.Data.Argv, argv) {
		return planErr("restart.data.argv", "new-process argv must equal the restart argv")
	}
	return nil
}

// projectSession carries the module-owned native session metadata into
// the payload: the native name verbatim and the RC intent with its
// final-argv indices. A nil projection emits the schema-zero metadata
// (null name, disabled RC): the plan's system defines no session surface,
// and guessed metadata would be worse than none. Indices validate for
// well-formedness only — unique, in-bounds positions — while exactness
// (every index an RC token, no RC token unindexed) is the module's
// guarantee at projection time. Enabled RC with empty indices is the
// settings-origin form; disabled RC with indices is incoherent.
func projectSession(projection *SessionProjection, argv []string) (*string, bool, []int, error) {
	if projection == nil {
		return nil, false, []int{}, nil
	}
	if !projection.RCEnabled && len(projection.RCIndices) != 0 {
		return nil, false, nil, planErr("remote_control.argv_indices", "disabled remote control carries no indices")
	}
	seen := make(map[int]bool, len(projection.RCIndices))
	out := make([]int, 0, len(projection.RCIndices))
	for _, index := range projection.RCIndices {
		if index < 0 || index >= len(argv) {
			return nil, false, nil, planErr("remote_control.argv_indices", "index falls outside the final argv")
		}
		if seen[index] {
			return nil, false, nil, planErr("remote_control.argv_indices", "indices must be unique")
		}
		seen[index] = true
		out = append(out, index)
	}
	return projection.NativeName, projection.RCEnabled, out, nil
}

func projectResume(intent agentic.ResumeIntent) (wireResume, error) {
	switch intent.Kind {
	case agentic.ResumeNew, agentic.ResumeLatest:
		if intent.Identity != nil {
			return wireResume{}, planErr("resume.identity", "new and latest carry no identity")
		}
		return wireResume{Kind: string(intent.Kind)}, nil
	case agentic.ResumeHandle, agentic.ResumeClaudeUUID:
		if intent.Identity == nil || *intent.Identity == "" {
			return wireResume{}, planErr("resume.identity", "handle and provider identities require an identity")
		}
		out := *intent.Identity
		return wireResume{Kind: string(intent.Kind), Identity: &out}, nil
	default:
		return wireResume{}, planErr("resume.kind", "kind is not admitted in Phase 1")
	}
}

// projectEffectivePolicy carries only sorted detected selectors and sorted
// inspected absolute source paths, mirroring the native reporter's filter:
// relaxations from uninspected, unknown, or doubly-listed sources are never
// projected. No relaxation means null. Selectors outside the closed
// vocabulary, non-absolute paths, and more than 64 paths cannot be projected
// and refuse.
func projectEffectivePolicy(in agentic.StoredPolicyInspection) (*wireEffectivePolicy, error) {
	inspected := make(map[string]bool, len(in.SourcesInspected))
	for _, source := range in.SourcesInspected {
		inspected[source] = true
	}
	uninspected := make(map[string]bool, len(in.SourcesNotInspected))
	for _, source := range in.SourcesNotInspected {
		uninspected[source.SourcePath] = true
	}
	bySource := map[string]map[string]bool{}
	for _, relaxation := range in.Relaxations {
		if relaxation.SourcePath == "" || relaxation.Selector == "" ||
			!inspected[relaxation.SourcePath] || uninspected[relaxation.SourcePath] {
			continue
		}
		if bySource[relaxation.SourcePath] == nil {
			bySource[relaxation.SourcePath] = map[string]bool{}
		}
		bySource[relaxation.SourcePath][relaxation.Selector] = true
	}
	if len(bySource) == 0 {
		return nil, nil
	}
	selectors := map[string]bool{}
	var paths []string
	for path, set := range bySource {
		paths = append(paths, path)
		for selector := range set {
			selectors[selector] = true
		}
	}
	slices.Sort(paths)
	relaxations := slices.Sorted(maps.Keys(selectors))
	for _, selector := range relaxations {
		switch selector {
		case "permissions.defaultMode", "permissions.allow", "permissions.additionalDirectories":
		default:
			return nil, planErr("policy.effective_native_policy.data.relaxations", "selector is outside the closed vocabulary")
		}
	}
	for _, path := range paths {
		if !isAbsolutePath(path) {
			return nil, planErr("policy.effective_native_policy.data.source_paths", "source paths must be absolute")
		}
		if len(path) > 4096 {
			return nil, planErr("policy.effective_native_policy.data.source_paths", "source paths must not exceed 4096 bytes")
		}
	}
	if len(paths) > 64 {
		return nil, planErr("policy.effective_native_policy.data.source_paths", "more than 64 source paths cannot be projected")
	}
	return &wireEffectivePolicy{
		Schema: agentic.ClaudeEffectivePolicySchema, SchemaVersion: agentic.HostedSchemaVersion,
		Data: wireEffectivePolicyData{Relaxations: relaxations, SourcePaths: paths},
	}, nil
}

// checkEncodedLimits enforces the limits measured over canonical bytes: the
// encoded argv array and each versioned data object.
func checkEncodedLimits(wire fragment.Value) error {
	process, ok := wire.Get("process")
	if !ok {
		return planErr("process", "payload carries no process")
	}
	argv, ok := process.Get("argv")
	if !ok || argv.Kind != fragment.KindArray {
		return planErr("process.argv", "payload carries no argv array")
	}
	if len(fragment.Canonical(argv)) > LimitArgvEncodedBytes {
		return planErr("process.argv", fmt.Sprintf("encoded argv exceeds %d bytes", LimitArgvEncodedBytes))
	}
	guard, ok := process.Get("exec_guard")
	if !ok {
		return planErr("process.exec_guard", "payload carries no exec guard")
	}
	if data, ok := guard.Get("data"); !ok {
		return planErr("process.exec_guard.data", "guard carries no data")
	} else if len(fragment.Canonical(data)) > LimitVersionedDataBytes {
		return planErr("process.exec_guard.data", fmt.Sprintf("versioned data exceeds %d bytes", LimitVersionedDataBytes))
	}
	restart, ok := wire.Get("restart")
	if !ok {
		return planErr("restart", "payload carries no restart template")
	}
	if data, ok := restart.Get("data"); !ok {
		return planErr("restart.data", "restart template carries no data")
	} else if len(fragment.Canonical(data)) > LimitVersionedDataBytes {
		return planErr("restart.data", fmt.Sprintf("versioned data exceeds %d bytes", LimitVersionedDataBytes))
	}
	policy, ok := wire.Get("policy")
	if !ok {
		return planErr("policy", "payload carries no policy")
	}
	if effective, ok := policy.Get("effective_native_policy"); ok && effective.Kind != fragment.KindNull {
		if data, ok := effective.Get("data"); !ok {
			return planErr("policy.effective_native_policy.data", "effective policy carries no data")
		} else if len(fragment.Canonical(data)) > LimitVersionedDataBytes {
			return planErr("policy.effective_native_policy.data", fmt.Sprintf("versioned data exceeds %d bytes", LimitVersionedDataBytes))
		}
	}
	return nil
}

func valueDepth(v fragment.Value, depth int) int {
	deepest := depth
	switch v.Kind {
	case fragment.KindArray:
		for _, el := range v.Arr {
			if d := valueDepth(el, depth+1); d > deepest {
				deepest = d
			}
		}
	case fragment.KindObject:
		for _, m := range v.Obj {
			if d := valueDepth(m.Value, depth+1); d > deepest {
				deepest = d
			}
		}
	}
	return deepest
}

func validEnvName(name string) bool {
	if len(name) == 0 || len(name) > 128 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		ok := c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (i > 0 && c >= '0' && c <= '9')
		if !ok {
			return false
		}
	}
	return true
}

func isAbsolutePath(path string) bool {
	return len(path) > 0 && path[0] == '/' && strings.IndexByte(path, 0) < 0
}

// validSessionName is the contract's host-name grammar, shared with the CLI
// --name rule: [A-Za-z0-9][A-Za-z0-9._-]{0,63}.
func validSessionName(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		alnum := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if i == 0 {
			if !alnum {
				return false
			}
			continue
		}
		if !alnum && c != '.' && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

// SessionFromPlan lifts the module-filled Plan.Session into the payload
// projection. The record is read in-process from the plan BuildPlan
// returned; the module verifies it only in-process, so it is never taken
// from an imported or deserialized plan.
//
// RC indices are positions in the plan's own argv. They describe the payload
// argv only while the composed argv IS the plan argv, which holds for Claude
// (the module builds the context channels into the plan, so composition adds
// no tokens). When the two differ the indices would point at the wrong
// tokens and the launcher refuses instead of shifting them: re-deriving
// positions would be the provider-grammar knowledge this boundary must not
// hold. A nil Session (no session surface) stays nil.
func SessionFromPlan(session *agentic.PlanSession, planArgv, composedArgv []string) (*SessionProjection, error) {
	if session == nil {
		return nil, nil
	}
	if !slices.Equal(planArgv, composedArgv) {
		return nil, planErr("remote_control.argv_indices", "composed argv differs from the plan argv the session indices describe")
	}
	out := &SessionProjection{RCEnabled: session.RCEnabled, RCIndices: make([]int, 0, len(session.RCIndices))}
	if session.Name != nil {
		name := *session.Name
		out.NativeName = &name
	}
	for _, index := range session.RCIndices {
		if index < 0 || index >= len(planArgv) {
			return nil, planErr("remote_control.argv_indices", "index falls outside the plan argv")
		}
		out.RCIndices = append(out.RCIndices, index)
	}
	return out, nil
}
