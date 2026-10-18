// Package client implements the session-manager protocol without choosing a
// host or wiring launcher commands. All remote values cross a closed validator.
package client

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/hosted"
	managerschema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
)

const Protocol = "curator-session-manager/1"
const Limit = 64 << 10

// Error carries only registry text and validated detail tokens. stderr bytes,
// plugin text and arbitrary start errors are never retained here. Launcher
// workspace checks may carry local paths, which Diagnostic quotes.
type Error struct {
	Code        string
	Details     map[string]string
	StderrBytes int64
}

func (e *Error) ExitCode() int {
	if x, ok := diagnostics.PluginDiagnosticFor(e.Code); ok {
		return x.Exit
	}
	if exit, ok := hosted.StatusExitForCode(e.Code); ok {
		return exit
	}
	return diagnostics.ExitForCode(e.Code)
}
func Message(code string) string {
	if x, ok := diagnostics.PluginDiagnosticFor(code); ok {
		return x.Message
	}
	if _, s, ok := hosted.StatusRegistryRow(code); ok {
		return s
	}
	return "Launch refused."
}
func (e *Error) Error() string { return e.Code + ": " + Message(e.Code) }

// Diagnostic renders only closed detail values, sorted for deterministic output.
func (e *Error) Diagnostic() string {
	s := e.Error()
	keys := make([]string, 0, len(e.Details))
	for k := range e.Details {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		value := e.Details[k]
		if k == "path" {
			value = strconv.Quote(value)
		}
		s += " " + k + "=" + value
	}
	return s
}
func refusal(code string, details map[string]string) *Error {
	return &Error{Code: code, Details: details}
}
func protocolError() *Error { return refusal("session_host_protocol_error", nil) }
func unavailable(reason string) *Error {
	return refusal("session_host_unavailable", map[string]string{"reason": reason})
}

type Doorbell struct {
	State      string  `json:"state"`
	Uncertain  bool    `json:"uncertain"`
	Generation *string `json:"generation"`
	Reason     *string `json:"reason"`
}

// Session is a validated record. If Readable is false, lifecycle fields
// have no evidence and decode to zero values; only its handle, manager and
// index labels may be used. The schema enforces the exact null projection.
type Session struct {
	Protocol       string            `json:"protocol,omitempty"`
	OK             bool              `json:"ok,omitempty"`
	Session        string            `json:"session"`
	OperationID    string            `json:"operation_id"`
	Manager        string            `json:"manager"`
	Runtime        string            `json:"runtime"`
	Readable       bool              `json:"readable"`
	NativeID       *string           `json:"native_id"`
	State          string            `json:"state"`
	Labels         map[string]string `json:"labels"`
	Generation     int64             `json:"generation"`
	StartedAt      string            `json:"started_at"`
	LastActivityAt *string           `json:"last_activity_at"`
	Doorbell       Doorbell          `json:"doorbell"`
}
type Outcome struct {
	Outcome string   `json:"outcome"`
	Session *Session `json:"session,omitempty"`
	Code    string   `json:"code,omitempty"`
}
type Describe struct {
	Protocol     string              `json:"protocol"`
	OK           bool                `json:"ok"`
	ID           string              `json:"id"`
	Version      string              `json:"version"`
	Capabilities []string            `json:"capabilities"`
	Runtimes     map[string][]string `json:"runtimes"`
}
type ListPage struct {
	Sessions   []Session `json:"sessions"`
	NextCursor *string   `json:"next_cursor"`
}
type Response struct {
	Session  *Session
	Outcome  *Outcome
	Describe *Describe
	Page     *ListPage
}

func ValidateRequest(operation string, data []byte) error {
	schema := operation + "-request.schema.json"
	if operation == "launch" || operation == "resume" {
		schema = operation + ".schema.json"
	}
	if err := managerschema.Validate(schema, data); err != nil {
		if operation == "launch" && strings.Contains(err.Error(), "$.child_env") {
			var request struct {
				ChildEnv map[string]json.RawMessage `json:"child_env"`
			}
			if json.Unmarshal(data, &request) == nil {
				for name := range request.ChildEnv {
					if managerschema.ReservedChildName(name) {
						return refusal("credential_reserved_name", map[string]string{"name": name})
					}
				}
			}
			return refusal("secret_policy_violation", nil)
		}
		return refusal("launch_plan_invalid", nil)
	}
	return nil
}

// DecodeResponse validates the entire body before retaining any field. A failed
// historical outcome is still a successful status operation; its code is data.
func DecodeResponse(operation string, request, body []byte, exit int, manager string) (Response, error) {
	var envelope struct {
		OK       bool              `json:"ok"`
		Protocol string            `json:"protocol"`
		Code     string            `json:"code"`
		Details  map[string]string `json:"details"`
	}
	if len(body) > Limit || json.Unmarshal(body, &envelope) != nil || envelope.Protocol != Protocol {
		return Response{}, protocolError()
	}
	if !envelope.OK {
		if exit == 0 || managerschema.Validate("refusal.schema.json", body) != nil {
			return Response{}, protocolError()
		}
		return Response{}, refusal(envelope.Code, envelope.Details)
	}
	if exit != 0 {
		return Response{}, protocolError()
	}
	var result Response
	var schema string
	switch operation {
	case "describe":
		schema = "describe"
		result.Describe = &Describe{}
	case "list":
		schema = "list-page"
		result.Page = &ListPage{}
	case "cancel":
		schema = "outcome"
		result.Outcome = &Outcome{}
	case "status":
		var r map[string]json.RawMessage
		if json.Unmarshal(request, &r) != nil {
			return Response{}, protocolError()
		}
		if _, ok := r["operation_id"]; ok {
			schema = "outcome"
			result.Outcome = &Outcome{}
		} else {
			schema = "session"
			result.Session = &Session{}
		}
	case "launch", "resume", "stop":
		schema = "session"
		result.Session = &Session{}
	default:
		return Response{}, protocolError()
	}
	if managerschema.Validate(schema+".schema.json", body) != nil {
		return Response{}, protocolError()
	}
	switch {
	case result.Describe != nil:
		_ = json.Unmarshal(body, result.Describe)
		if result.Describe.ID != manager {
			return Response{}, protocolError()
		}
	case result.Page != nil:
		_ = json.Unmarshal(body, result.Page)
		var filter struct {
			Labels map[string]string `json:"labels"`
		}
		_ = json.Unmarshal(request, &filter)
		for _, record := range result.Page.Sessions {
			if record.Manager != manager {
				return Response{}, protocolError()
			}
			for key, value := range filter.Labels {
				if record.Labels[key] != value {
					return Response{}, protocolError()
				}
			}
		}
	case result.Outcome != nil:
		_ = json.Unmarshal(body, result.Outcome)
		if result.Outcome.Session != nil && result.Outcome.Session.Manager != manager {
			return Response{}, protocolError()
		}
	case result.Session != nil:
		_ = json.Unmarshal(body, result.Session)
		if result.Session.Manager != manager || (!result.Session.Readable && operation != "status") {
			return Response{}, protocolError()
		}
	}
	var correlation struct {
		OperationID        string `json:"operation_id"`
		Session            string `json:"session"`
		Runtime            string `json:"runtime"`
		ExpectedGeneration int64  `json:"expected_generation"`
	}
	_ = json.Unmarshal(request, &correlation)
	record := result.Session
	if result.Outcome != nil {
		record = result.Outcome.Session
	}
	if record != nil && ((correlation.OperationID != "" && record.OperationID != correlation.OperationID) ||
		(correlation.Session != "" && record.Session != correlation.Session) ||
		(correlation.Runtime != "" && record.Runtime != correlation.Runtime) ||
		(correlation.ExpectedGeneration != 0 && record.Generation != correlation.ExpectedGeneration)) {
		return Response{}, protocolError()
	}
	return result, nil
}

func IsCode(err error, code string) bool { var e *Error; return errors.As(err, &e) && e.Code == code }
