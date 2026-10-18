// Package werkstatt owns validated station records and read-only workspace checks.
package werkstatt

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	schema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
)

type Doorbell struct {
	Heartbeat       string `json:"heartbeat"`
	MaxWakesPerHour int    `json:"max_wakes_per_hour"`
}
type Record struct {
	Schema           string            `json:"schema"`
	ID               string            `json:"id"`
	Project          string            `json:"project"`
	Role             string            `json:"role"`
	Runtime          string            `json:"runtime"`
	Model            *string           `json:"model"`
	Effort           *string           `json:"effort"`
	WeightsID        *string           `json:"weights_id,omitempty"`
	Profile          string            `json:"profile"`
	Clone            string            `json:"clone"`
	GitDir           string            `json:"git_dir,omitempty"`
	Identity         string            `json:"identity"`
	Room             string            `json:"room"`
	Orchestrator     string            `json:"orchestrator"`
	ContextMode      string            `json:"context_mode,omitempty"`
	HarnessMemory    *bool             `json:"harness_memory,omitempty"`
	Sandbox          string            `json:"sandbox"`
	ChildEnv         map[string]string `json:"child_env,omitempty"`
	TrustClass       string            `json:"trust_class"`
	Doorbell         *Doorbell         `json:"doorbell"`
	ConfigDecisionID string            `json:"config_decision_id,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)

func ValidID(id string) bool { return idPattern.MatchString(id) }
func failure(code string, details map[string]string) error {
	return &client.Error{Code: code, Details: details}
}
func invalid(id, field string) error {
	return failure("werkstatt_record_invalid", map[string]string{"id": id, "field": field})
}

// StationSource is the future board station boundary. No board command is
// invoked until that source has a published implementation.
type StationSource interface {
	Get(context.Context, string) (*Record, error)
}
type Loader struct {
	Directory string
	Station   StationSource
}

func Parse(data []byte, id string) (*Record, error) {
	if !ValidID(id) {
		return nil, invalid("", "id")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err == nil {
		if env, ok := fields["child_env"]; ok {
			if schema.Validate("child-env.schema.json", env) != nil {
				var names map[string]json.RawMessage
				_ = json.Unmarshal(env, &names)
				for name := range names {
					if schema.ReservedChildName(name) {
						return nil, failure("credential_reserved_name", map[string]string{"name": name})
					}
				}
				return nil, failure("secret_policy_violation", nil)
			}
		}
	}
	if err := schema.Validate("werkstatt-record.schema.json", data); err != nil {
		return nil, invalid(id, "record")
	}
	var r Record
	if json.Unmarshal(data, &r) != nil || r.ID != id {
		return nil, invalid(id, "id")
	}
	for k, v := range r.ChildEnv {
		if schema.ReservedChildName(k) {
			return nil, failure("credential_reserved_name", map[string]string{"name": k})
		}
		if len(v) > 4096 {
			return nil, failure("secret_policy_violation", nil)
		}
	}
	if r.ContextMode == "" {
		r.ContextMode = "continue"
	}
	if r.HarnessMemory == nil {
		v := r.ContextMode == "continue"
		r.HarnessMemory = &v
	}
	return &r, nil
}
func (l Loader) Load(ctx context.Context, id string) (*Record, string, error) {
	if !ValidID(id) {
		return nil, "", invalid("", "id")
	}
	if l.Station != nil {
		r, err := l.Station.Get(ctx, id)
		if err != nil {
			return nil, "", err
		}
		if r != nil {
			b, _ := json.Marshal(r)
			checked, err := Parse(b, id)
			return checked, "station", err
		}
	}
	path := filepath.Join(l.Directory, id+".json")
	b, present, err := configfile.Read(path)
	if err != nil {
		return nil, "", invalid(id, "file")
	}
	if !present {
		return nil, "", failure("werkstatt_not_found", map[string]string{"id": id})
	}
	r, err := Parse(b, id)
	return r, path, err
}
