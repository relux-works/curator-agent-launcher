// Package protocolchild owns the Launcher's closed, headless protocol-child
// selectors. Parsing grants no ordinary-operation or native-process admission.
package protocolchild

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	Command                = "protocol-child-v1"
	CodeUsage              = "protocol_child_usage"
	CodeEnvironmentRefused = "protocol_child_environment_refused"
	maxValueBytes          = 4096
	maxArgvBytes           = 16384
)

// Error reports a fixed diagnostic without echoing untrusted selector values.
type Error struct {
	Code   string
	Detail string
}

func (e *Error) Error() string { return e.Code + ": " + e.Detail }
func (e *Error) ExitCode() int {
	if e.Code == CodeUsage {
		return 64
	}
	return 78
}

// Selectors are expectations for owning resolution, never a composed plan.
// Null and empty strings stay distinct until ordinary semantics are checked.
type Selectors struct {
	Profile         string
	LockSHA256      string
	FragmentSHA256  string
	Workspace       string
	Model           *string
	Effort          *string
	Permissions     string
	OperationID     string
	MemberAgent     *string
	MemberConfigDir *string
}

var (
	profileName     = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9_-])?$`)
	reservedProfile = regexp.MustCompile(`^(?i:con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)`)
	hexDigest       = regexp.MustCompile(`^[0-9a-f]{64}$`)
	operationID     = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	namedAgent      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

var flagOrder = [...]string{
	"runtime", "kind", "profile", "expected-lock-sha256", "expected-fragment-sha256",
	"workspace", "model-json", "effort-json", "permissions", "service-tier-json",
	"weights-json", "credential-source-json", "child-env-json", "native-tail-json",
	"operation-id", "lifecycle", "member-a2a-agent-json", "member-a2a-config-dir-json",
}

// Parse checks only the bounded CLI grammar. Core/profile, model/effort policy,
// physical workspace, executable and actual operation remain separate checks.
func Parse(args []string) (*Selectors, error) {
	fail := func(detail string) (*Selectors, error) {
		return nil, &Error{Code: CodeUsage, Detail: detail}
	}
	if len(args) != 1+len(flagOrder) || args[0] != Command {
		return fail("expected the exact protocol-child-v1 argument list")
	}
	total := 0
	for _, arg := range args {
		total += len(arg)
		if total > maxArgvBytes || !safeText(arg) {
			return fail("argument payload exceeds bounds or contains invalid text")
		}
	}
	values := make([]string, len(flagOrder))
	for i, name := range flagOrder {
		value, ok := strings.CutPrefix(args[i+1], "--"+name+"=")
		if !ok || len(value) > maxValueBytes {
			return fail("missing, reordered or oversized " + name + " selector")
		}
		values[i] = value
	}
	if values[0] != "codex" || values[1] != "app-server" || values[15] != "create" {
		return fail("only a new Codex app-server is supported")
	}
	if len(values[2]) > 128 || !profileName.MatchString(values[2]) || reservedProfile.MatchString(values[2]) {
		return fail("profile must be a portable installed-profile identifier")
	}
	if !hexDigest.MatchString(values[3]) || !hexDigest.MatchString(values[4]) {
		return fail("expected digests must be bare lowercase SHA256 hex")
	}
	if !filepath.IsAbs(values[5]) || filepath.Clean(values[5]) != values[5] {
		return fail("workspace must be a clean absolute physical path")
	}
	model, ok := nullableString(values[6])
	if !ok {
		return fail("model-json must be a JSON string or null without control characters")
	}
	effort, ok := nullableString(values[7])
	if !ok {
		return fail("effort-json must be a JSON string or null without control characters")
	}
	if values[8] != "native" && values[8] != "yolo" {
		return fail("permissions must be native or yolo")
	}
	if values[9] != "null" || values[10] != "null" || values[11] != "null" || values[12] != "{}" || values[13] != "[]" {
		return fail("tier, weights and credential source must be null; child env and native tail must be empty")
	}
	if !operationID.MatchString(values[14]) {
		return fail("operation-id must be the retained ordinary UUIDv4")
	}
	agent, agentOK := nullableString(values[16])
	root, rootOK := nullableString(values[17])
	if !agentOK || !rootOK || !validMemberPair(agent, root) {
		return fail("member identity must be a paired named-agent and clean absolute root, or both null")
	}
	return &Selectors{Profile: values[2], LockSHA256: values[3], FragmentSHA256: values[4],
		Workspace: values[5], Model: model, Effort: effort, Permissions: values[8], OperationID: values[14],
		MemberAgent: agent, MemberConfigDir: root}, nil
}

func validMemberPair(agent, root *string) bool {
	if agent == nil || root == nil {
		return agent == nil && root == nil
	}
	return len(*agent) <= maxValueBytes && len(*root) <= maxValueBytes &&
		safeText(*agent) && namedAgent.MatchString(*agent) && safeText(*root) &&
		filepath.IsAbs(*root) && filepath.Clean(*root) == *root
}

func nullableString(raw string) (*string, bool) {
	var value *string
	if json.Unmarshal([]byte(raw), &value) != nil {
		return nil, false
	}
	if value != nil && !safeText(*value) {
		return nil, false
	}
	return value, true
}

func safeText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
