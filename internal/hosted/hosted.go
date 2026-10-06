// Package hosted implements the launcher side of the hosted session-launch
// contract (SPEC §4.8, §4.9): resume elevation through the module typed-intent
// API, the versioned session-launch-plan payload with its content digest,
// and the private receiver transport over fd3/fd4.
//
// The package builds payloads but never sends them except through Run, and
// Run never falls back: every failure is a typed refusal with the mapped
// exit. Values travel only on the private receiver stdin; diagnostics carry
// fields, reasons, and sizes, never values.
package hosted

// Owner codes of the hosted families. Diagnostics transcribes them; the
// owner stays the single source of truth.
const (
	CodeHostConflict                           = "host_configuration_conflict"
	CodeSessionHostMissing                     = "session_host_missing"
	CodeSessionHostUnavailable                 = "session_host_unavailable"
	CodeSessionHostProviderUnsupported         = "session_host_provider_unsupported"
	CodeSessionHostProtocolUnsupported         = "session_host_protocol_unsupported"
	CodeSessionHostScopeUnsupported            = "session_host_scope_unsupported"
	CodeSessionHostExecutionProfileUnsupported = "session_host_execution_profile_unsupported"
	CodeSessionHostTerminalRequired            = "session_host_terminal_required"
	CodeSessionHostStdinUnsupported            = "session_host_stdin_unsupported"
	CodeSessionHostDefaultNotReady             = "session_host_default_not_ready"
	CodeSessionResumeInvalid                   = "session_resume_invalid"
	CodeLaunchPlanInvalid                      = "launch_plan_invalid"
	CodeNetworkScopeUnsupported                = "network_scope_unsupported"
	CodeSecretPolicyViolation                  = "secret_policy_violation"
	CodePolicyRefused                          = "policy_refused"
)

// Payload identity of the closed session-launch-plan shape.
const (
	PayloadSchema    = "urn:relux:task-board:session-launch-plan"
	PayloadVersion   = "1.0.0"
	PayloadEnvClaude = "claude_code"
)

// Producer identity of the pinned agents-management module. ModuleVersion is
// pinned to the go.mod require by TestProducerMatchesBuild; ModuleCommit is
// the peeled commit of the v0.5.53 tag, resolved by git ls-remote at pin
// time (Go exposes no dependency commit at runtime).
const (
	ProducerModuleVersion = "0.5.53"
	ProducerModuleCommit  = "f07bbf3259679c45af13e2d665dd37bb76126ec5"
)
