package hosted

import (
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	claudeSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/claude"
)

// Wire structs transcribe the closed session-launch-plan 1.0.0 shape. Field
// order is documentation only: BuildPayload emits canonical bytes. Slices
// that the schema types as arrays are never nil here, so encoding/json
// renders [] and never null.
type wirePayload struct {
	Schema        string                       `json:"schema"`
	SchemaVersion string                       `json:"schema_version"`
	ContentDigest string                       `json:"content_digest"`
	Producer      wireProducer                 `json:"producer"`
	Environment   string                       `json:"environment"`
	Process       wireProcess                  `json:"process"`
	OwnedLiterals map[string]string            `json:"owned_literals"`
	EnvNames      []string                     `json:"env_names"`
	ManagedHome   wireManagedHome              `json:"managed_home"`
	Fragment      wireFragment                 `json:"fragment"`
	Policy        wirePolicy                   `json:"policy"`
	SessionName   wireSessionName              `json:"session_name"`
	RemoteControl wireRemoteControl            `json:"remote_control"`
	Resume        wireResume                   `json:"resume"`
	Restart       claudeSystem.RestartTemplate `json:"restart"`
	HostBundle    any                          `json:"host_bundle"`
	Network       any                          `json:"network"`
}

type wireProducer struct {
	ModuleVersion string `json:"module_version"`
	ModuleCommit  string `json:"module_commit"`
}

type wireProcess struct {
	Binary    string       `json:"binary"`
	Argv      []string     `json:"argv"`
	Env       []string     `json:"env"`
	Cwd       string       `json:"cwd"`
	Stdin     *wireStdin   `json:"stdin"`
	ExecGuard agentic.Seal `json:"exec_guard"`
}

type wireStdin struct {
	Encoding string `json:"encoding"`
	Bytes    string `json:"bytes"`
}

type wireManagedHome struct {
	Variable string `json:"variable"`
	Path     string `json:"path"`
}

type wireFragment struct {
	ProfileName    string `json:"profile_name"`
	Pin            string `json:"pin"`
	FragmentDigest string `json:"fragment_digest"`
	SystemModules  bool   `json:"system_modules"`
}

type wirePolicy struct {
	PermissionMode        string               `json:"permission_mode"`
	PermissionSource      string               `json:"permission_source"`
	ExecutionProfile      string               `json:"execution_profile"`
	EffectiveNativePolicy *wireEffectivePolicy `json:"effective_native_policy"`
}

type wireEffectivePolicy struct {
	Schema        string                  `json:"schema"`
	SchemaVersion string                  `json:"schema_version"`
	Data          wireEffectivePolicyData `json:"data"`
}

type wireEffectivePolicyData struct {
	Relaxations []string `json:"relaxations"`
	SourcePaths []string `json:"source_paths"`
}

type wireSessionName struct {
	Host   string  `json:"host"`
	Native *string `json:"native"`
}

type wireRemoteControl struct {
	Enabled     bool  `json:"enabled"`
	ArgvIndices []int `json:"argv_indices"`
}

type wireResume struct {
	Kind     string  `json:"kind"`
	Identity *string `json:"identity"`
}
