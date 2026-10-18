package sessionmanager

import (
	"bufio"
	"encoding/json"
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
	registry, path, mode, err := s.target()
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
			answer, err := bufio.NewReader(s.Stdin).ReadString('\n')
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
			notice += fmt.Sprintf("registered %s; selection unchanged (%s); run: curator run session-manager select %s\n", id, current, id)
		}
		registry.Managers[id] = entry
		if err := s.WriteFile(path, mustEncode(registry), mode); err != nil {
			return invalid(path, "unreadable", err)
		}
		_, err = io.WriteString(s.Stdout, notice)
		return err
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
func (s Service) target() (Registry, string, os.FileMode, error) {
	layer := configfile.User
	mode := os.FileMode(0600)
	if s.CanWriteSystem(s.Resolver.Locations.System) {
		layer = configfile.System
		mode = 0644
	} else {
		_, present, path, err := s.Resolver.Read(configfile.System, Filename)
		if present {
			return Registry{}, "", 0, &Error{Code: CodeShadowed, Path: path}
		}
		if err != nil {
			return Registry{}, "", 0, invalid(path, fileReason(err), err)
		}
	}
	dir, _ := s.Resolver.Locations.Directories(layer)
	path := filepath.Join(dir, Filename)
	data, present, readPath, err := s.Resolver.Read(layer, Filename)
	if err != nil {
		return Registry{}, "", 0, invalid(readPath, fileReason(err), err)
	}
	r := Empty()
	if present {
		r, err = Parse(data)
		if e, ok := err.(*Error); ok {
			e.Path = readPath
		}
	}
	return r, path, mode, err
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
