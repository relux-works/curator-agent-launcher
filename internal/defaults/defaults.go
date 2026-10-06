// Package defaults loads launcher-owned defaults.json files and resolves the
// flags/operator/machine portion of SPEC §4.3. Unset members await lineup;
// this package performs no model admission and writes no configuration.
package defaults

import (
	"fmt"
	"path/filepath"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

const Schema = "curator-run-defaults-v1"
const SchemaV2 = "curator-run-defaults-v2"

// SchemaV3 adds the per-environment host member; v1 and v2 stay accepted
// unchanged and reject it as an unknown member.
const SchemaV3 = "curator-run-defaults-v3"
const CodeInvalid = "defaults_config_invalid"
const CodeUsage = "usage"

// Host values of the v3 host member.
const (
	HostNative = "native"
	HostHosted = "hosted"
)

// Error preserves the diagnostic family and underlying filesystem/parser error.
type Error struct {
	Code string
	Err  error
}

func (e *Error) Error() string { return e.Code + ": " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }
func invalid(err error) error  { return &Error{CodeInvalid, err} }

// Member distinguishes an explicitly supplied empty string from absence.
type Member struct {
	Value   string
	Present bool
}
type Pair struct{ Model, Effort, Permissions, Host Member }

// Files is validated configuration. Its zero value represents absent files.
// Keeping entries private prevents callers bypassing schema validation.
type Files struct{ machine, operator file }
type file struct {
	locked  bool
	entries map[string]Pair
}

// Paths names explicit files; no empty-path or ambient fallback is performed.
type Paths struct{ Machine, Operator string }

// ConfigPaths uses process inputs, never the fragment-applied child environment.
// Pass os.Getenv("XDG_CONFIG_HOME") and the process user's home directory.
// Empty XDG_CONFIG_HOME follows the XDG default. It does not create directories.
func ConfigPaths(xdgConfigHome, userHome string) Paths {
	if xdgConfigHome == "" {
		xdgConfigHome = filepath.Join(userHome, ".config")
	}
	return Paths{Machine: "/etc/curator-run/defaults.json", Operator: filepath.Join(xdgConfigHome, "curator-run", "defaults.json")}
}

// Load reads and validates both files, including ignored operator entries.
// Only a missing path is optional: dangling links and existing unreadable files
// are errors. No partial configuration is returned on failure.
func Load(paths Paths) (Files, error) {
	machine, err := loadFile(paths.Machine)
	if err != nil {
		return Files{}, err
	}
	operator, err := loadFile(paths.Operator)
	if err != nil {
		return Files{}, err
	}
	return Files{machine, operator}, nil
}

func loadFile(path string) (file, error) {
	data, present, err := configfile.Read(path)
	if err != nil {
		return file{}, invalid(fmt.Errorf("%s: %w", path, err))
	}
	if !present {
		return file{}, nil
	}
	f, err := parse(data)
	if err != nil {
		return file{}, invalid(fmt.Errorf("%s: %w", path, err))
	}
	return f, nil
}

func parse(data []byte) (file, error) {
	root, err := fragment.ParseJSON(data)
	if err != nil {
		return file{}, err
	}
	if root.Kind != fragment.KindObject {
		return file{}, fmt.Errorf("root must be an object")
	}
	schemaValue, hasSchema := root.Get("schema")
	if !hasSchema || schemaValue.Kind != fragment.KindString {
		return file{}, fmt.Errorf("schema must be a string")
	}
	schema := schemaValue.Str
	if schema != Schema && schema != SchemaV2 && schema != SchemaV3 {
		return file{}, fmt.Errorf("invalid schema")
	}
	f := file{entries: map[string]Pair{}}
	for _, m := range root.Obj {
		switch m.Key {
		case "schema":
			if m.Value.Kind != fragment.KindString || m.Value.Str != schema {
				return file{}, fmt.Errorf("invalid schema")
			}
		case "locked":
			if m.Value.Kind != fragment.KindBool {
				return file{}, fmt.Errorf("locked must be boolean")
			}
			f.locked = m.Value.Bool
		case "defaults":
			if m.Value.Kind != fragment.KindObject {
				return file{}, fmt.Errorf("defaults must be an object")
			}
			for _, env := range m.Value.Obj {
				if fragment.HomeVariable(env.Key) == "" {
					return file{}, fmt.Errorf("unknown environment %q", env.Key)
				}
				if env.Value.Kind != fragment.KindObject || len(env.Value.Obj) == 0 {
					want := "model, effort, or permissions"
					if schema == SchemaV3 {
						want = "model, effort, permissions, or host"
					}
					return file{}, fmt.Errorf("%s must contain %s", env.Key, want)
				}
				var pair Pair
				for _, member := range env.Value.Obj {
					if member.Value.Kind != fragment.KindString {
						return file{}, fmt.Errorf("%s.%s must be a string", env.Key, member.Key)
					}
					value := Member{member.Value.Str, true}
					switch member.Key {
					case "model":
						pair.Model = value
					case "effort":
						pair.Effort = value
					case "permissions":
						if schema != SchemaV2 && schema != SchemaV3 {
							return file{}, fmt.Errorf("%s.permissions requires %s", env.Key, SchemaV2)
						}
						if value.Value != "native" && value.Value != "yolo" {
							return file{}, fmt.Errorf("%s.permissions must be native or yolo", env.Key)
						}
						pair.Permissions = value
					case "host":
						if schema != SchemaV3 {
							return file{}, fmt.Errorf("%s.host requires %s", env.Key, SchemaV3)
						}
						if value.Value != HostNative && value.Value != HostHosted {
							return file{}, fmt.Errorf("%s.host must be native or hosted", env.Key)
						}
						pair.Host = value
					default:
						return file{}, fmt.Errorf("unknown member %s.%s", env.Key, member.Key)
					}
				}
				f.entries[env.Key] = pair
			}
		default:
			return file{}, fmt.Errorf("unknown member %q", m.Key)
		}
	}
	for _, key := range []string{"schema", "defaults"} {
		if _, ok := root.Get(key); !ok {
			return file{}, fmt.Errorf("missing %s", key)
		}
	}
	return f, nil
}

type Origin string

const (
	OriginFlag     Origin = "flag"
	OriginOperator Origin = "operator"
	OriginMachine  Origin = "machine"
)

// ResolvedMember has an empty origin exactly when the member is absent.
type ResolvedMember struct {
	Member
	Origin Origin
}
type Partial struct{ Model, Effort, Permissions ResolvedMember }

// Resolve applies per-member precedence and refuses flags for locked members,
// even if the flag repeats the machine value. Operator locked has no effect.
// An ignored operator entry still must be valid at Load time.
func (f Files) Resolve(environment string, flags Pair) (Partial, error) {
	if fragment.HomeVariable(environment) == "" {
		return Partial{}, invalid(fmt.Errorf("unknown environment %q", environment))
	}
	machine, named := f.machine.entries[environment]
	operator := f.operator.entries[environment]
	locked := f.machine.locked && named
	if locked {
		operator = Pair{}
	}
	var result Partial
	for _, row := range []struct {
		name                    string
		flag, operator, machine Member
		out                     *ResolvedMember
	}{
		{"model", flags.Model, operator.Model, machine.Model, &result.Model},
		{"effort", flags.Effort, operator.Effort, machine.Effort, &result.Effort},
		{"permissions", flags.Permissions, operator.Permissions, machine.Permissions, &result.Permissions},
	} {
		if locked && row.flag.Present && row.machine.Present {
			return Partial{}, &Error{CodeUsage, fmt.Errorf("--%s overrides locked %s for %s", row.name, row.name, environment)}
		}
		for _, candidate := range []ResolvedMember{{row.flag, OriginFlag}, {row.operator, OriginOperator}, {row.machine, OriginMachine}} {
			if candidate.Present {
				*row.out = candidate
				break
			}
		}
	}
	return result, nil
}

// PermissionDefault returns the merged launcher-global permission value for
// one canonical environment, before the higher Curator profile level and CLI
// flag are applied. The machine lock has the same named-environment scope as
// the model and effort lock.
func (f Files) PermissionDefault(environment string) (Member, error) {
	if fragment.HomeVariable(environment) == "" {
		return Member{}, invalid(fmt.Errorf("unknown environment %q", environment))
	}
	machine, named := f.machine.entries[environment]
	if f.machine.locked && named {
		return machine.Permissions, nil
	}
	operator := f.operator.entries[environment]
	if operator.Permissions.Present {
		return operator.Permissions, nil
	}
	return machine.Permissions, nil
}

// Host origins of ResolveHost. OriginDefault is the implicit native outcome:
// no flag and no configured default select native with no session-host probe.
const (
	OriginHostFlag     Origin = "flag"
	OriginHostOperator Origin = "operator"
	OriginHostMachine  Origin = "machine"
	OriginHostDefault  Origin = "default"
)

// ResolvedHost is the selected host and the precedence level that produced
// it. Host is HostNative or HostHosted.
type ResolvedHost struct {
	Host   string
	Origin Origin
}

// ResolveHost applies the SPEC §4.8 host precedence table for one canonical
// environment. hostFlag is the literal CLI host request ("--hosted",
// "--native", "--untracked") or empty when no flag was given:
//
//   - machine locked and naming the environment: the entire operator entry
//     is ignored; a flag with a machine host present refuses usage, even
//     when equal; otherwise flag, then machine host, then native;
//   - otherwise: flag, then operator host, then machine host, then native.
//
// The machine lock has the same named-environment scope as the model and
// effort lock. An ignored operator entry still must be valid at Load time.
func (f Files) ResolveHost(environment string, hostFlag string) (ResolvedHost, error) {
	if fragment.HomeVariable(environment) == "" {
		return ResolvedHost{}, invalid(fmt.Errorf("unknown environment %q", environment))
	}
	var flag Member
	switch hostFlag {
	case "":
	case "--hosted":
		flag = Member{Value: HostHosted, Present: true}
	case "--native", "--untracked":
		flag = Member{Value: HostNative, Present: true}
	default:
		return ResolvedHost{}, invalid(fmt.Errorf("unknown host flag %q", hostFlag))
	}
	machine, named := f.machine.entries[environment]
	operator := f.operator.entries[environment]
	lockedHost := f.machine.locked && named
	if lockedHost {
		operator = Pair{}
		if flag.Present && machine.Host.Present {
			return ResolvedHost{}, &Error{CodeUsage, fmt.Errorf("%s overrides locked host for %s", hostFlag, environment)}
		}
		if flag.Present {
			return ResolvedHost{flag.Value, OriginHostFlag}, nil
		}
		if machine.Host.Present {
			return ResolvedHost{machine.Host.Value, OriginHostMachine}, nil
		}
		return ResolvedHost{HostNative, OriginHostDefault}, nil
	}
	if flag.Present {
		return ResolvedHost{flag.Value, OriginHostFlag}, nil
	}
	if operator.Host.Present {
		return ResolvedHost{operator.Host.Value, OriginHostOperator}, nil
	}
	if machine.Host.Present {
		return ResolvedHost{machine.Host.Value, OriginHostMachine}, nil
	}
	return ResolvedHost{HostNative, OriginHostDefault}, nil
}
