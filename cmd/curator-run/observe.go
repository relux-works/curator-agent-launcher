package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"slices"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	managerschema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
)

// Observation has no store, reconciliation or mutation boundary: the registry
// supplies an incarnation and the existing client makes two bounded read RPCs.
//
// Schema v2 additionally compares the owner binding the caller froze before
// submission. The registry half (registration time and, when frozen, the
// executable) is compared before any RPC, so a changed incarnation is never
// executed; the record half (manager, session, generation) is compared by the
// client on the validated status record. Either mismatch prints nothing.
func runManagerObservation(ctx context.Context, command sessionmanager.Command, stdout, stderr io.Writer, resolver configfile.Resolver, deps launchDeps) int {
	// The frozen check must not rest on the CLI parser alone: a v2 command
	// built without its required binding never reaches the registry, so no
	// record can be printed as if the comparison had taken place.
	if command.Schema == "v2" && (command.ExpectGeneration < 1 || command.ExpectRegisteredAt == "") {
		return emitHostFailure(stderr, &client.Error{Code: "session_generation_mismatch"})
	}
	// Keep refusals to one closed diagnostic even for a deprecated registry path.
	resolver.Stderr = io.Discard
	registry, _, err := sessionmanager.Load(resolver)
	if err != nil {
		return emitHostFailure(stderr, err)
	}
	entry, ok := registry.Managers[command.ID]
	if !ok {
		return emitHostFailure(stderr, &sessionmanager.Error{Code: sessionmanager.CodeNotRegistered, ID: command.ID})
	}
	v2 := command.Schema == "v2"
	if v2 && (entry.RegisteredAt != command.ExpectRegisteredAt || (command.ExpectExecutable != "" && entry.Executable != command.ExpectExecutable)) {
		return emitHostFailure(stderr, &client.Error{Code: "session_generation_mismatch"})
	}
	environment := os.Environ()
	if deps.environ != nil {
		environment = deps.environ()
	}
	remote := client.Client{ID: command.ID, Entry: entry, Environment: environment}
	description, err := remote.Call(ctx, "describe", []byte(`{}`))
	if err != nil {
		return emitHostFailure(stderr, err)
	}
	if !slices.Contains(description.Describe.Capabilities, "status/1") {
		return emitHostFailure(stderr, &client.Error{Code: "session_host_capability_missing", Details: map[string]string{"capability": "status/1"}})
	}
	// The members are admitted only under the token of this same describe. The
	// complete record is validated before either schema version is rendered.
	capable := client.ProcessObservation(*description.Describe)
	remote.Admission = client.Admission{ProcessObservation: capable}
	if v2 {
		remote.Admission.FrozenGeneration = command.ExpectGeneration
	}
	request, _ := json.Marshal(map[string]string{"session": command.Session})
	status, err := remote.Call(ctx, "status", request)
	if err != nil {
		return emitHostFailure(stderr, err)
	}
	record := status.SessionRecord
	if !v2 {
		// v1 stays the 1.2.0 projection of an already valid record.
		if record, err = client.SessionRecordV1(record); err != nil {
			return emitHostFailure(stderr, err)
		}
	}
	now := time.Now
	if deps.now != nil {
		now = deps.now
	}
	observation := struct {
		Schema  string `json:"schema"`
		Manager struct {
			ID           string `json:"id"`
			Executable   string `json:"executable"`
			RegisteredAt string `json:"registered_at"`
		} `json:"manager"`
		Session      json.RawMessage `json:"session"`
		ObservedAt   string          `json:"observed_at"`
		Capabilities *struct {
			ProcessObservation bool `json:"process_observation"`
		} `json:"capabilities,omitempty"`
	}{Schema: "curator-run-session-observation-v1", Session: record, ObservedAt: now().UTC().Truncate(time.Second).Format(time.RFC3339)}
	schema := "session-observation.schema.json"
	if v2 {
		observation.Schema = "curator-run-session-observation-v2"
		schema = "session-observation-v2.schema.json"
		observation.Capabilities = &struct {
			ProcessObservation bool `json:"process_observation"`
		}{ProcessObservation: capable}
	}
	observation.Manager.ID = command.ID
	observation.Manager.Executable = entry.Executable
	observation.Manager.RegisteredAt = entry.RegisteredAt
	data, err := json.Marshal(observation)
	if err != nil || managerschema.Validate(schema, data) != nil {
		return emitHostFailure(stderr, &client.Error{Code: "session_host_protocol_error"})
	}
	if _, err := stdout.Write(append(data, '\n')); err != nil {
		return emitHostFailure(stderr, &client.Error{Code: "session_host_unavailable", Details: map[string]string{"reason": "transport"}})
	}
	return 0
}
