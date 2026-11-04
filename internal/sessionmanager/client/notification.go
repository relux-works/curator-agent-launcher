package client

import "encoding/json"

// NotificationBinding retains the complete validated Host binding. These
// references are correlation data, not enrollment or permission to trigger.
type NotificationBinding struct {
	Execution          string `json:"execution"`
	ControllerEpoch    string `json:"controller_epoch"`
	RunnerInstance     string `json:"runner_instance"`
	NativeThread       string `json:"native_thread"`
	MemberRef          string `json:"member_ref"`
	ConnectionRef      string `json:"connection_ref"`
	ConfigurationRef   string `json:"configuration_ref"`
	ProfileLockRef     string `json:"profile_lock_ref"`
	ConsumerGeneration string `json:"consumer_generation"`
	EnrollmentRef      string `json:"enrollment_ref"`
	WorkflowReference  string `json:"workflow_reference"`
	WorkflowSHA256     string `json:"workflow_sha256"`
}

// NotificationResult is populated only after closed wire-schema validation.
// Omitted members depend on the phase: only ready contains a binding, while
// off contains neither attachment nor workflow. No phase asserts a peer reply.
type NotificationResult struct {
	Contract          string               `json:"contract"`
	State             string               `json:"state"`
	AttachmentRef     string               `json:"attachment_ref,omitempty"`
	WorkflowReference string               `json:"workflow_reference,omitempty"`
	WorkflowSHA256    string               `json:"workflow_sha256,omitempty"`
	Binding           *NotificationBinding `json:"binding,omitempty"`
	Reason            string               `json:"reason,omitempty"`
}

// The ready thread must match the actual session observation. Workflow,
// member, enrollment and attachment admission still require the owning
// lifecycle's records; a syntactically valid response cannot create them.
func notificationMatchesSession(s *Session) bool {
	if s.Notification == nil || s.Notification.State != "ready" {
		return true
	}
	n := s.Notification
	return s.Readable && s.NativeID != nil && n.Binding != nil && n.Binding.NativeThread == *s.NativeID
}

// Request validation owns the closed selection grammar. This additional check
// binds a launch/resume response to its requested workflow; it grants no member
// admission and cannot replace the manager's retained lifecycle observations.
func notificationMatchesRequest(operation string, request []byte, s *Session) bool {
	if operation != "launch" && operation != "resume" {
		return true
	}
	var envelope struct {
		Notification json.RawMessage `json:"notification"`
	}
	if json.Unmarshal(request, &envelope) != nil {
		return false
	}
	if len(envelope.Notification) == 0 {
		return true
	}
	var selected *struct {
		Contract string `json:"contract"`
		Backend  string `json:"backend"`
		Workflow struct {
			Reference string `json:"reference"`
			SHA256    string `json:"sha256"`
		} `json:"workflow"`
	}
	if json.Unmarshal(envelope.Notification, &selected) != nil || selected == nil ||
		selected.Contract != "curator-host-notification/1" || selected.Backend != "host" ||
		selected.Workflow.Reference == "" || selected.Workflow.SHA256 == "" ||
		s == nil || s.Notification == nil {
		return false
	}
	n := s.Notification
	switch n.State {
	case "ready":
		return n.Binding != nil && n.Binding.WorkflowReference == selected.Workflow.Reference &&
			n.Binding.WorkflowSHA256 == selected.Workflow.SHA256
	case "preparing", "unresolved":
		return n.WorkflowReference == selected.Workflow.Reference && n.WorkflowSHA256 == selected.Workflow.SHA256
	default:
		return false
	}
}
