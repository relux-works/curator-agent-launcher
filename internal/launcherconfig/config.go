// Package launcherconfig validates machine policy without selecting a host or
// enabling delegated execution.
package launcherconfig

import (
	"fmt"
	"path/filepath"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

const Schema = "curator-run-launcher-v1"
const LegacySchema = Schema
const CodeInvalid = "defaults_config_invalid"

type Error struct {
	Path string
	Err  error
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s: %v", CodeInvalid, e.Path, e.Err) }
func (e *Error) Unwrap() error { return e.Err }

// Mode is effective machine policy. Source is machine only for an explicit
// system host_mode; absent file or member means auto/default.
type Mode struct{ Value, Source string }

// Config is validated machine policy. Host resolution consumes the mode and
// delegated trial opt-in before selecting a session-manager route.
type Config struct {
	HostMode          Mode
	DelegatedTrial    bool
	DelegatedTrialSet bool
}

func Parse(data []byte) (Config, error) { return parse(data, "") }

func parse(data []byte, userSystemPath string) (Config, error) {
	root, err := fragment.ParseJSON(data)
	if err != nil {
		// Duplicate keys and malformed input can contain inline secrets. Do
		// not propagate parser text that includes those untrusted bytes.
		return Config{}, fmt.Errorf("invalid launcher configuration JSON")
	}
	if root.Kind != fragment.KindObject {
		return Config{}, fmt.Errorf("root must be an object")
	}
	if userSystemPath != "" {
		for _, name := range []string{"host_mode", "delegated_trial", "proxy", "claude_auth", "notification"} {
			if _, ok := root.Get(name); ok {
				return Config{}, fmt.Errorf("%s is machine-only; configure %s", name, userSystemPath)
			}
		}
	}
	schema, ok := root.Get("schema")
	if !ok || schema.Kind != fragment.KindString || (schema.Str != Schema && schema.Str != LegacySchema) {
		return Config{}, fmt.Errorf("invalid schema")
	}
	result := Config{HostMode: Mode{Value: "auto", Source: "default"}}
	for _, member := range root.Obj {
		switch member.Key {
		case "schema":
		case "host_mode":
			if member.Value.Kind != fragment.KindString {
				return Config{}, fmt.Errorf("host_mode must be hosted, native or auto")
			}
			switch member.Value.Str {
			case "hosted", "native", "auto":
				result.HostMode = Mode{member.Value.Str, "machine"}
			default:
				return Config{}, fmt.Errorf("host_mode must be hosted, native or auto")
			}
		case "delegated_trial":
			if member.Value.Kind != fragment.KindBool {
				return Config{}, fmt.Errorf("delegated_trial must be a boolean")
			}
			result.DelegatedTrial, result.DelegatedTrialSet = member.Value.Bool, true
		case "proxy", "claude_auth", "notification":
			return Config{}, fmt.Errorf("launcher selections require the Core-selected machine configuration")
		default:
			return Config{}, fmt.Errorf("unknown launcher configuration member")
		}
	}
	return result, nil
}

// Load validates both layers, even when system policy is explicit. The user
// layer may carry the schema but has no authority to supply either policy knob.
func Load(r configfile.Resolver) (Config, error) {
	result := Config{HostMode: Mode{Value: "auto", Source: "default"}}
	for _, layer := range []configfile.Layer{configfile.System, configfile.User} {
		data, present, path, err := r.Read(layer, "launcher.json")
		if err != nil {
			return Config{}, &Error{path, err}
		}
		if !present {
			continue
		}
		userSystemPath := ""
		if layer == configfile.User {
			userSystemPath = filepath.Join(r.Locations.System, "launcher.json")
		}
		parsed, err := parse(data, userSystemPath)
		if err != nil {
			return Config{}, &Error{path, err}
		}
		if layer == configfile.System {
			result = parsed
		}
	}
	return result, nil
}
