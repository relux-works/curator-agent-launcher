// Package defaults loads launcher-owned defaults.json files and resolves the
// flags/operator/machine portion of SPEC §4.3. Unset members await lineup;
// this package performs no model admission and writes no configuration.
package defaults

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

const Schema = "curator-run-defaults-v1"
const CodeInvalid = "defaults_config_invalid"
const CodeUsage = "usage"

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
type Pair struct{ Model, Effort Member }

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
	if path == "" {
		return file{}, invalid(fmt.Errorf("empty defaults path"))
	}
	// Inspect each component without following symlinks when classifying absence.
	// Lstat on only the final path would misclassify a dangling parent link.
	if err := inspect(path); err != nil {
		if os.IsNotExist(err) {
			return file{}, nil
		}
		return file{}, invalid(fmt.Errorf("%s: %w", path, err))
	}
	info, err := os.Stat(path)
	if err != nil {
		return file{}, invalid(fmt.Errorf("%s: %w", path, err))
	}
	if !info.Mode().IsRegular() {
		return file{}, invalid(fmt.Errorf("%s: defaults must be a regular file", path))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return file{}, invalid(fmt.Errorf("%s: %w", path, err))
	}
	f, err := parse(data)
	if err != nil {
		return file{}, invalid(fmt.Errorf("%s: %w", path, err))
	}
	return f, nil
}

func inspect(path string) error {
	parent := filepath.Dir(path)
	if parent != path && parent != "." {
		if err := inspect(parent); err != nil {
			return err
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if _, err := os.Stat(path); err != nil {
			// Deliberately do not wrap ENOENT: the link exists, so this is not absence.
			return fmt.Errorf("unreadable symlink %s: %v", path, err)
		}
	}
	return nil
}

func parse(data []byte) (file, error) {
	root, err := fragment.ParseJSON(data)
	if err != nil {
		return file{}, err
	}
	if root.Kind != fragment.KindObject {
		return file{}, fmt.Errorf("root must be an object")
	}
	f := file{entries: map[string]Pair{}}
	for _, m := range root.Obj {
		switch m.Key {
		case "schema":
			if m.Value.Kind != fragment.KindString || m.Value.Str != Schema {
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
					return file{}, fmt.Errorf("%s must contain model or effort", env.Key)
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
type Partial struct{ Model, Effort ResolvedMember }

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
