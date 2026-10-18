// Package sessionmanager owns the launcher registry. Reading or resolving it
// never invokes a manager or searches PATH.
package sessionmanager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

const (
	Filename          = "session-managers.json"
	Schema            = "curator-run-session-managers-v1"
	Protocol          = "curator-session-manager/1"
	CodeBusy          = "session_manager_registry_busy"
	CodeInvalid       = "session_manager_registry_invalid"
	CodeShadowed      = "session_manager_registry_shadowed"
	CodeNotRegistered = "session_manager_not_registered"
	CodeHostMissing   = "session_host_missing"
)

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
var timestampPattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-]00:00)$`)
var registeredByPattern = regexp.MustCompile(`^[ -~]{1,128}$`)
var argumentPattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

func ValidID(id string) bool { return idPattern.MatchString(id) }

type Error struct {
	Code, Path, Reason, ID string
	Err                    error
}

func (e *Error) Error() string {
	message := map[string]string{CodeBusy: "session-manager registry is busy", CodeInvalid: "invalid session-manager registry", CodeShadowed: "session-manager registry is shadowed", CodeNotRegistered: "session manager is not registered", CodeHostMissing: "session host missing"}[e.Code]
	if message == "" {
		message = "session-manager command failed"
	}
	if e.Path != "" {
		message += fmt.Sprintf(" path=%q", e.Path)
	}
	if e.Reason != "" {
		message += fmt.Sprintf(" reason=%q", e.Reason)
	}
	if e.ID != "" {
		message += fmt.Sprintf(" id=%s", e.ID)
	}
	// Registry diagnostics have closed detail vocabularies. Do not print parser
	// text or arbitrary input fields; Err is available to local callers via Unwrap.
	return message
}
func (e *Error) Unwrap() error { return e.Err }
func invalid(path, reason string, err error) error {
	return &Error{Code: CodeInvalid, Path: path, Reason: reason, Err: err}
}

type Entry struct {
	Executable   string   `json:"executable"`
	ArgvPrefix   []string `json:"argv_prefix"`
	Protocol     string   `json:"protocol"`
	RegisteredAt string   `json:"registered_at"`
	RegisteredBy string   `json:"registered_by"`
}
type Registry struct {
	Schema   string           `json:"schema"`
	Selected string           `json:"selected,omitempty"`
	Managers map[string]Entry `json:"managers"`
}

func Empty() Registry { return Registry{Schema: Schema, Managers: map[string]Entry{}} }

func object(v fragment.Value, required, optional []string) error {
	if v.Kind != fragment.KindObject {
		return fmt.Errorf("expected object")
	}
	allowed := map[string]bool{}
	for _, key := range append(append([]string{}, required...), optional...) {
		allowed[key] = true
	}
	for _, member := range v.Obj {
		if !allowed[member.Key] {
			return fmt.Errorf("unknown member")
		}
	}
	for _, key := range required {
		if _, ok := v.Get(key); !ok {
			return fmt.Errorf("missing member")
		}
	}
	return nil
}
func stringMember(v fragment.Value, key string) (string, error) {
	field, ok := v.Get(key)
	if !ok || field.Kind != fragment.KindString {
		return "", fmt.Errorf("expected string")
	}
	return field.Str, nil
}
func parseEntry(v fragment.Value, manifest bool) (Entry, error) {
	required := []string{"executable", "argv_prefix", "protocol", "registered_by"}
	if manifest {
		required = append(required, "id")
	} else {
		required = append(required, "registered_at")
	}
	if err := object(v, required, nil); err != nil {
		return Entry{}, err
	}
	var entry Entry
	var err error
	if entry.Executable, err = stringMember(v, "executable"); err != nil || !filepath.IsAbs(entry.Executable) || strings.ContainsRune(entry.Executable, 0) {
		return Entry{}, fmt.Errorf("executable must be absolute")
	}
	if entry.Protocol, err = stringMember(v, "protocol"); err != nil || entry.Protocol != Protocol {
		return Entry{}, fmt.Errorf("unsupported protocol")
	}
	if entry.RegisteredBy, err = stringMember(v, "registered_by"); err != nil || !registeredByPattern.MatchString(entry.RegisteredBy) {
		return Entry{}, fmt.Errorf("invalid registered_by")
	}
	prefix, _ := v.Get("argv_prefix")
	if prefix.Kind != fragment.KindArray || len(prefix.Arr) > 4 {
		return Entry{}, fmt.Errorf("invalid argv_prefix")
	}
	entry.ArgvPrefix = []string{}
	for _, item := range prefix.Arr {
		if item.Kind != fragment.KindString || !argumentPattern.MatchString(item.Str) {
			return Entry{}, fmt.Errorf("invalid argv_prefix")
		}
		entry.ArgvPrefix = append(entry.ArgvPrefix, item.Str)
	}
	if !manifest {
		entry.RegisteredAt, err = stringMember(v, "registered_at")
		if err != nil {
			return Entry{}, err
		}
		stamp, e := time.Parse(time.RFC3339Nano, entry.RegisteredAt)
		_, offset := stamp.Zone()
		if e != nil || offset != 0 || !timestampPattern.MatchString(entry.RegisteredAt) {
			return Entry{}, fmt.Errorf("registered_at must be UTC RFC3339")
		}
	}
	return entry, nil
}

func Parse(data []byte) (Registry, error) {
	root, err := fragment.ParseJSON(data)
	if err != nil {
		return Registry{}, invalid("", "parse", err)
	}
	fail := func(err error) (Registry, error) { return Registry{}, invalid("", "schema", err) }
	if err = object(root, []string{"schema", "managers"}, []string{"selected"}); err != nil {
		return fail(err)
	}
	schema, err := stringMember(root, "schema")
	if err != nil || schema != Schema {
		return fail(fmt.Errorf("invalid schema"))
	}
	registry := Empty()
	managers, _ := root.Get("managers")
	if managers.Kind != fragment.KindObject {
		return fail(fmt.Errorf("invalid managers"))
	}
	for _, member := range managers.Obj {
		if !ValidID(member.Key) {
			return fail(fmt.Errorf("invalid id"))
		}
		entry, e := parseEntry(member.Value, false)
		if e != nil {
			return fail(e)
		}
		registry.Managers[member.Key] = entry
	}
	if _, ok := root.Get("selected"); ok {
		registry.Selected, err = stringMember(root, "selected")
		if err != nil || !ValidID(registry.Selected) {
			return fail(fmt.Errorf("invalid selected"))
		}
		if _, ok := registry.Managers[registry.Selected]; !ok {
			return fail(fmt.Errorf("selected is not registered"))
		}
	}
	return registry, nil
}

func ParseManifest(data []byte) (string, Entry, error) {
	root, err := fragment.ParseJSON(data)
	if err != nil {
		return "", Entry{}, invalid("", "parse", err)
	}
	entry, err := parseEntry(root, true)
	if err != nil {
		return "", Entry{}, invalid("", "schema", err)
	}
	id, err := stringMember(root, "id")
	if err != nil || !ValidID(id) {
		return "", Entry{}, invalid("", "schema", fmt.Errorf("invalid id"))
	}
	return id, entry, nil
}

// Load applies machine-first authority after per-file legacy fallback. A
// present machine registry prevents any inspection of either user path.
func Load(r configfile.Resolver) (Registry, string, error) {
	for _, layer := range []configfile.Layer{configfile.System, configfile.User} {
		data, present, path, err := r.Read(layer, Filename)
		if err != nil {
			return Registry{}, path, invalid(path, fileReason(err), err)
		}
		if !present {
			continue
		}
		registry, err := Parse(data)
		if e, ok := err.(*Error); ok {
			e.Path = path
		}
		return registry, path, err
	}
	return Empty(), "", nil
}
func fileReason(err error) string {
	switch {
	case strings.Contains(err.Error(), "symlink"):
		return "symlink"
	case strings.Contains(err.Error(), "different identity"), strings.Contains(err.Error(), "owner"):
		return "owner"
	case strings.Contains(err.Error(), "group/world-writable"), strings.Contains(err.Error(), "DACL"):
		return "mode"
	default:
		return "unreadable"
	}
}

type State struct {
	Kind  string
	ID    string
	Entry Entry
}

// Resolve is pure: its only executable observation is supplied by the caller.
// It reports a broken selected registration without reclassifying it as empty.
func Resolve(registry Registry, executableAvailable func(string) bool) (State, error) {
	if len(registry.Managers) == 0 {
		return State{Kind: "empty"}, nil
	}
	if registry.Selected == "" {
		return State{Kind: "unselected"}, nil
	}
	entry, ok := registry.Managers[registry.Selected]
	if !ok {
		return State{}, invalid("", "schema", fmt.Errorf("unknown selection"))
	}
	state := State{Kind: "selected", ID: registry.Selected, Entry: entry}
	if !executableAvailable(entry.Executable) {
		return state, &Error{Code: CodeHostMissing, Reason: "registered executable missing"}
	}
	return state, nil
}
func ExecutableAvailable(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0
}
func Encode(registry Registry) ([]byte, error) {
	data, err := json.MarshalIndent(registry, "", "  ")
	return append(data, '\n'), err
}
