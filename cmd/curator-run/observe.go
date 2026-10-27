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
func runManagerObservation(ctx context.Context, command sessionmanager.Command, stdout, stderr io.Writer, resolver configfile.Resolver, deps launchDeps) int {
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
	request, _ := json.Marshal(map[string]string{"session": command.Session})
	status, err := remote.Call(ctx, "status", request)
	if err != nil {
		return emitHostFailure(stderr, err)
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
		Session    json.RawMessage `json:"session"`
		ObservedAt string          `json:"observed_at"`
	}{Schema: "curator-run-session-observation-v1", Session: status.SessionRecord, ObservedAt: now().UTC().Truncate(time.Second).Format(time.RFC3339)}
	observation.Manager.ID = command.ID
	observation.Manager.Executable = entry.Executable
	observation.Manager.RegisteredAt = entry.RegisteredAt
	data, err := json.Marshal(observation)
	if err != nil || managerschema.Validate("session-observation.schema.json", data) != nil {
		return emitHostFailure(stderr, &client.Error{Code: "session_host_protocol_error"})
	}
	if _, err := stdout.Write(append(data, '\n')); err != nil {
		return emitHostFailure(stderr, &client.Error{Code: "session_host_unavailable", Details: map[string]string{"reason": "transport"}})
	}
	return 0
}
