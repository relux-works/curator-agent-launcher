package sessionmanager

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
)

type Command struct {
	Operation, ID, Manifest string
	JSON, Select, NoSelect  bool
}

// Service boundaries allow registration policy and failed writes to be tested
// without terminal interaction, privilege changes, or invoking a manager.
type Service struct {
	Resolver       configfile.Resolver
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	IsTerminal     func() bool
	Now            func() time.Time
	CanWriteSystem func(string) bool
	WriteFile      func(string, []byte, os.FileMode) error
	promptTimeout  time.Duration // zero uses the production answer deadline
}

func (s Service) Run(c Command) error {
	if c.Select && c.NoSelect {
		return &Error{Code: "usage"}
	}
	if (c.Operation == "select" || c.Operation == "unregister" || c.Operation == "show") && !ValidID(c.ID) {
		return &Error{Code: "usage"}
	}
	if s.Stdin == nil {
		s.Stdin = os.Stdin
	}
	if s.Stdout == nil {
		s.Stdout = io.Discard
	}
	if s.Stderr == nil {
		s.Stderr = io.Discard
	}
	if s.Now == nil {
		s.Now = time.Now
	}
	if s.IsTerminal == nil {
		s.IsTerminal = func() bool { return false }
	}
	if s.CanWriteSystem == nil {
		s.CanWriteSystem = writableDirectory
	}
	if s.WriteFile == nil {
		s.WriteFile = atomicWrite
	}
	if c.Operation == "list" || c.Operation == "show" {
		return s.display(c)
	}
	layer, path, mode := s.destination()
	// Machine authority precedes touching the user home; check again under
	// the lock in case a system registry appeared while this mutation waited.
	if layer == configfile.User {
		if err := s.checkMachineAuthority(); err != nil {
			return err
		}
	}
	lock, err := acquireRegistryLock(path, mode)
	if err != nil {
		return err
	}
	defer lock.Close()
	registry, err := s.readTarget(layer)
	if err != nil {
		return err
	}
	switch c.Operation {
	case "register":
		data, err := readManifest(c.Manifest, s.Stdin)
		if err != nil {
			return invalid(c.Manifest, "unreadable", err)
		}
		id, entry, err := ParseManifest(data)
		if e, ok := err.(*Error); ok {
			e.Path = c.Manifest
		}
		if err != nil {
			return err
		}
		resolved, err := filepath.EvalSymlinks(entry.Executable)
		if err != nil || !ExecutableAvailable(resolved) {
			return invalid(c.Manifest, "schema", fmt.Errorf("manifest executable is not a regular executable file"))
		}
		entry.Executable = resolved
		entry.RegisteredAt = s.Now().UTC().Format(time.RFC3339Nano)
		old, exists := registry.Managers[id]
		var notice string
		if exists && old.Executable != entry.Executable {
			notice = fmt.Sprintf("session manager %s: executable changed %s -> %s\n", id, oneLine(old.Executable), oneLine(entry.Executable))
		}
		switch {
		case c.Select:
			registry.Selected = id
		case c.NoSelect:
		case exists:
		case len(registry.Managers) == 0:
			registry.Selected = id
		case s.IsTerminal():
			current := registry.Selected
			if current == "" {
				current = "none"
			}
			if _, err := fmt.Fprintf(s.Stderr, "Select session manager %s instead of %s? [y/N] ", id, current); err != nil {
				return err
			}
			timeout := s.promptTimeout
			if timeout == 0 {
				timeout = promptAnswerTimeout
			}
			answer, err := readPrompt(s.Stdin, timeout)
			if errors.Is(err, os.ErrDeadlineExceeded) {
				return &Error{Code: "usage", Err: err}
			}
			if err != nil && err != io.EOF {
				return invalid(path, "unreadable", err)
			}
			if strings.EqualFold(strings.TrimSpace(answer), "y") || strings.EqualFold(strings.TrimSpace(answer), "yes") {
				registry.Selected = id
			}
		default:
			current := registry.Selected
			if current == "" {
				current = "none"
			}
			notice += fmt.Sprintf("registered %s; selection unchanged (%s); run: curator-run session-manager select %s\n", id, current, id)
		}
		registry.Managers[id] = entry
		// Report output errors before commit, so every failed command preserves
		// the previous registry. Successful return acknowledges the later commit.
		if _, err := io.WriteString(s.Stdout, notice); err != nil {
			return err
		}
		if err := s.WriteFile(path, mustEncode(registry), mode); err != nil {
			return invalid(path, "unreadable", err)
		}
		return nil
	case "select", "unregister":
		if _, ok := registry.Managers[c.ID]; !ok {
			return &Error{Code: CodeNotRegistered, ID: c.ID}
		}
		if c.Operation == "select" {
			registry.Selected = c.ID
		} else {
			delete(registry.Managers, c.ID)
			if registry.Selected == c.ID {
				registry.Selected = ""
			}
		}
		if err := s.WriteFile(path, mustEncode(registry), mode); err != nil {
			return invalid(path, "unreadable", err)
		}
		return nil
	default:
		return fmt.Errorf("unknown session-manager operation")
	}
}
func mustEncode(r Registry) []byte { data, _ := Encode(r); return data }
func oneLine(s string) string      { return strings.NewReplacer("\r", `\r`, "\n", `\n`).Replace(s) }
func readManifest(path string, in io.Reader) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(in)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("manifest must be regular")
	}
	return os.ReadFile(path)
}
func (s Service) destination() (configfile.Layer, string, os.FileMode) {
	layer, mode := configfile.User, os.FileMode(0600)
	if s.CanWriteSystem(s.Resolver.Locations.System) {
		layer, mode = configfile.System, 0644
	}
	dir, _ := s.Resolver.Locations.Directories(layer)
	return layer, filepath.Join(dir, Filename), mode
}
func (s Service) readTarget(layer configfile.Layer) (Registry, error) {
	if layer == configfile.User {
		if err := s.checkMachineAuthority(); err != nil {
			return Registry{}, err
		}
	}
	data, present, readPath, err := s.Resolver.Read(layer, Filename)
	if err != nil {
		return Registry{}, invalid(readPath, fileReason(err), err)
	}
	r := Empty()
	if present {
		r, err = Parse(data)
		if e, ok := err.(*Error); ok {
			e.Path = readPath
		}
	}
	return r, err
}
func (s Service) checkMachineAuthority() error {
	_, present, path, err := s.Resolver.Read(configfile.System, Filename)
	if present {
		return &Error{Code: CodeShadowed, Path: path}
	}
	if err != nil {
		return invalid(path, fileReason(err), err)
	}
	return nil
}
func (s Service) display(c Command) error {
	r, _, err := Load(s.Resolver)
	if err != nil {
		return err
	}
	if c.Operation == "show" {
		e, ok := r.Managers[c.ID]
		if !ok {
			return &Error{Code: CodeNotRegistered, ID: c.ID}
		}
		if c.JSON {
			return json.NewEncoder(s.Stdout).Encode(struct {
				ID string `json:"id"`
				Entry
			}{c.ID, e})
		}
		_, err = fmt.Fprintf(s.Stdout, "%s: %s\nprotocol: %s\nargv_prefix: %s\nregistered_at: %s\nregistered_by: %s\n", c.ID, oneLine(e.Executable), e.Protocol, strings.Join(e.ArgvPrefix, " "), e.RegisteredAt, oneLine(e.RegisteredBy))
		return err
	}
	if c.JSON {
		return json.NewEncoder(s.Stdout).Encode(r)
	}
	ids := make([]string, 0, len(r.Managers))
	for id := range r.Managers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		marker := " "
		if id == r.Selected {
			marker = "*"
		}
		if _, err = fmt.Fprintf(s.Stdout, "%s %s %s\n", marker, id, oneLine(r.Managers[id].Executable)); err != nil {
			return err
		}
	}
	return nil
}
