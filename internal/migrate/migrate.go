// Package migrate plans and transactionally copies the legacy configuration
// home. Retired ax policy files alone are backed up and removed in place.
package migrate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
)

const CodeDestinationExists = "migrate_destination_exists"
const MessageDestinationExists = "migration destination exists with different bytes"

type Error struct {
	Code, Path, Reason string
	Err                error
}

func (e *Error) Error() string {
	message := MessageDestinationExists
	if e.Code == defaults.CodeHostWithdrawn {
		message = defaults.MessageHostWithdrawn
	}
	if e.Code == defaults.CodeInvalid {
		message = "invalid migration configuration"
	}
	detail := fmt.Sprintf("%s: path=%q", message, e.Path)
	if e.Reason != "" {
		detail += " reason=" + e.Reason
	}
	if e.Err != nil {
		detail += ": " + e.Err.Error()
	}
	return detail
}
func (e *Error) Unwrap() error { return e.Err }
func invalid(path string, err error) error {
	return &Error{Code: defaults.CodeInvalid, Path: path, Err: err}
}
func conflict(path string) error { return &Error{Code: CodeDestinationExists, Path: path} }

// Options exposes the write boundary for deterministic failure tests. Rollback
// always uses the real atomic writer, so a failing injected commit cannot also
// disable recovery. Nil fields select the production implementations.
type Options struct {
	CanWriteSystem func(string) bool
	WriteFile      func(string, []byte, os.FileMode) error
	RemoveFile     func(string) error
}

type change struct {
	path         string
	data, before []byte
	remove       bool
	existed      bool
	mode         os.FileMode
	dirMode      os.FileMode
}
type transaction struct {
	newMode, dirMode os.FileMode
	changes          []change
	names            map[string][]byte
	lines            []string
}

// Run computes and prints the complete plan before writing anything. All
// destination and backup conflicts are checked before apply, including dry run.
func Run(loc configfile.Locations, dryRun bool, out io.Writer, opts Options) error {
	if opts.CanWriteSystem == nil {
		opts.CanWriteSystem = writableDirectory
	}
	if opts.RemoveFile == nil {
		opts.RemoveFile = os.Remove
	}
	if opts.WriteFile == nil {
		opts.WriteFile = atomicWrite
	}
	p := transaction{names: map[string][]byte{}}
	manual, err := p.retiredAX(loc, opts.CanWriteSystem)
	if err != nil {
		return err
	}
	for _, layer := range []configfile.Layer{configfile.System, configfile.User} {
		dir, old := loc.Directories(layer)
		p.newMode, p.dirMode = 0600, 0700
		if layer == configfile.System {
			p.newMode, p.dirMode = 0644, 0755
		}
		if old == "" {
			continue
		}
		entries, err := configfile.ReadDirectory(old)
		if err != nil {
			return invalid(old, err)
		}
		canCopy := layer == configfile.User || opts.CanWriteSystem(dir)
		if canCopy && len(entries) > 0 {
			if err := preserveSources(dir, loc); err != nil {
				return err
			}
		}
		type source struct {
			name, path        string
			original, desired []byte
		}
		var sources []source
		var skipped []string
		var systemMode, defaultsPath string
		for _, entry := range entries {
			if entry.Name() == "ax.json" || entry.Name() == "ax.json.bak" {
				continue
			}
			src := filepath.Join(old, entry.Name())
			data, present, err := configfile.Read(src)
			if err != nil {
				return invalid(src, err)
			}
			if !present {
				return invalid(src, fmt.Errorf("source disappeared while planning"))
			}
			desired := data
			if entry.Name() == "defaults.json" {
				var hosts []string
				desired, hosts, err = stripHosts(data)
				if err != nil {
					return invalid(src, err)
				}
				if layer == configfile.System {
					defaultsPath = src
					for _, host := range hosts {
						_, value, _ := strings.Cut(host, "=")
						if systemMode != "" && systemMode != value {
							return &Error{Code: defaults.CodeHostWithdrawn, Path: src, Reason: "mixed_values"}
						}
						systemMode = value
					}
				} else if len(hosts) > 0 {
					p.lines = append(p.lines, "curator-run: user host values dropped: "+strings.Join(hosts, ", "))
				}
			}
			sources = append(sources, source{entry.Name(), src, data, desired})
			if !canCopy {
				skipped = append(skipped, src)
			}
		}
		if !canCopy {
			if len(skipped) > 0 {
				p.lines = append(p.lines, "curator-run: system files not copied: "+strings.Join(skipped, ", "))
			}
			continue
		}
		if systemMode != "" {
			desired, _ := json.MarshalIndent(map[string]string{"schema": launcherconfig.Schema, "host_mode": systemMode}, "", "  ")
			desired = append(desired, '\n')
			found := false
			for i := range sources {
				if sources[i].name != "launcher.json" {
					continue
				}
				if _, err := launcherconfig.Parse(sources[i].original); err != nil {
					return invalid(sources[i].path, err)
				}
				var original map[string]json.RawMessage
				if err := json.Unmarshal(sources[i].original, &original); err != nil {
					return invalid(sources[i].path, err)
				}
				original["host_mode"], _ = json.Marshal(systemMode)
				merged, _ := json.MarshalIndent(original, "", "  ")
				sources[i].desired = append(merged, '\n')
				found = true
			}
			if !found {
				sources = append(sources, source{"launcher.json", defaultsPath, desired, desired})
			}
			p.lines = append(p.lines, fmt.Sprintf("curator-run: machine host mode from %s: %s", defaultsPath, systemMode))
		}
		for _, src := range sources {
			if err := p.copy(filepath.Join(dir, src.name), src.original, src.desired); err != nil {
				return err
			}
		}
	}

	for _, line := range p.lines {
		if _, err := fmt.Fprintln(out, oneLine(line)); err != nil {
			return err
		}
	}
	if len(p.changes) == 0 {
		if manual != "" && !dryRun {
			return invalid(manual, fmt.Errorf("retired policy is not a regular file; remove it manually"))
		}
		_, err := fmt.Fprintln(out, "curator-run: nothing left to do")
		return err
	}
	for _, c := range p.changes {
		if _, err := fmt.Fprintf(out, "curator-run: %s %s\n", map[bool]string{false: "write", true: "remove"}[c.remove], oneLine(c.path)); err != nil {
			return err
		}
	}
	if dryRun {
		return nil
	}
	if manual != "" {
		return invalid(manual, fmt.Errorf("retired policy is not a regular file; remove it manually"))
	}
	return p.apply(opts.WriteFile, opts.RemoveFile)
}

// retiredAX deliberately has no per-file fallback or precedence. Non-regular
// entries are reported for manual removal without opening their contents.
func (p *transaction) retiredAX(loc configfile.Locations, canWriteSystem func(string) bool) (manual string, err error) {
	for _, path := range loc.AXPaths() {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", invalid(path, err)
		}
		p.lines = append(p.lines, "curator-run: retired ax.json: "+path)
		if !info.Mode().IsRegular() {
			p.lines = append(p.lines, "curator-run: remove manually (non-regular file; not followed): "+path)
			manual = path
			continue
		}
		system := filepath.Dir(path) == loc.System || filepath.Dir(path) == loc.LegacySystem
		if system && !canWriteSystem(filepath.Dir(path)) {
			p.lines = append(p.lines, "curator-run: system file not removed (not writable): "+path)
			continue
		}
		p.newMode, p.dirMode = 0600, 0700
		if system {
			p.newMode, p.dirMode = 0644, 0755
		}
		data, present, err := configfile.Read(path)
		if err != nil || !present {
			if err == nil {
				err = fmt.Errorf("retired policy disappeared while planning")
			}
			return "", invalid(path, err)
		}
		backup := path + ".bak"
		b, exists, err := configfile.Read(backup)
		if err != nil {
			return "", invalid(backup, err)
		}
		if exists && !bytes.Equal(b, data) {
			return "", conflict(backup)
		}
		if !exists {
			if err := p.add(backup, data); err != nil {
				return "", err
			}
		}
		if err := p.add(path, data); err != nil {
			return "", err
		}
		p.changes[len(p.changes)-1].remove = true
	}
	return manual, nil
}

// Plan lines stay physical single lines even for unusual legacy filenames.
func oneLine(s string) string { return strings.NewReplacer("\r", `\r`, "\n", `\n`).Replace(s) }

// Parent symlinks are permitted by the reader, but migration must never write
// into a legacy home, including through aliases or not-yet-created children.
func preserveSources(destination string, loc configfile.Locations) error {
	dst, err := canonicalHome(destination)
	if err != nil {
		return invalid(destination, err)
	}
	for _, old := range []string{loc.LegacySystem, loc.LegacyUser} {
		if old == "" {
			continue
		}
		src, err := canonicalHome(old)
		if err != nil {
			return invalid(old, err)
		}
		forward, e1 := filepath.Rel(src, dst)
		backward, e2 := filepath.Rel(dst, src)
		if (e1 == nil && filepath.IsLocal(forward)) || (e2 == nil && filepath.IsLocal(backward)) {
			return invalid(destination, fmt.Errorf("destination overlaps legacy configuration home %q", old))
		}
	}
	return nil
}

func canonicalHome(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) || filepath.Dir(path) == path {
			return "", err
		}
		missing = append(missing, filepath.Base(path))
		path = filepath.Dir(path)
	}
}

func stripHosts(data []byte) ([]byte, []string, error) {
	// ParseJSON rejects duplicate keys and invalid Unicode before map decoding.
	if _, err := fragment.ParseJSON(data); err != nil {
		return nil, nil, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, nil, err
	}
	var schema string
	if err := json.Unmarshal(root["schema"], &schema); err != nil {
		return nil, nil, err
	}
	if schema != defaults.SchemaV3 {
		return data, nil, defaults.Validate(data)
	}
	var entries map[string]map[string]json.RawMessage
	if err := json.Unmarshal(root["defaults"], &entries); err != nil {
		return nil, nil, err
	}
	var hosts []string
	for name, entry := range entries {
		value, ok := entry["host"]
		if !ok {
			continue
		}
		var host string
		if err := json.Unmarshal(value, &host); err != nil || (host != "native" && host != "hosted") {
			return nil, nil, fmt.Errorf("%s.host must be native or hosted", name)
		}
		if fragment.HomeVariable(name) == "" {
			return nil, nil, fmt.Errorf("unknown environment %q", name)
		}
		hosts = append(hosts, name+"="+host)
		delete(entry, "host")
		if len(entry) == 0 {
			delete(entries, name)
		}
	}
	if len(hosts) == 0 {
		return data, nil, defaults.Validate(data)
	}
	sort.Strings(hosts)
	root["defaults"], _ = json.Marshal(entries)
	result, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	result = append(result, '\n')
	if err := defaults.Validate(result); err != nil {
		return nil, nil, err
	}
	return result, hosts, nil
}

func (p *transaction) copy(path string, original, desired []byte) error {
	existing, present, err := configfile.Read(path)
	if err != nil {
		return invalid(path, err)
	}
	if present && bytes.Equal(existing, desired) {
		return nil
	}
	if present && !bytes.Equal(existing, original) {
		return conflict(path)
	}
	// A rewritten copy keeps the pre-rewrite bytes even if the destination did
	// not exist yet. Plain copies have no previous destination to preserve.
	if !bytes.Equal(original, desired) {
		backup := path + ".bak"
		b, present, err := configfile.Read(backup)
		if err != nil {
			return invalid(backup, err)
		}
		if present && !bytes.Equal(b, original) {
			return conflict(backup)
		}
		if !present {
			if err := p.add(backup, original); err != nil {
				return err
			}
		}
	}
	return p.add(path, desired)
}
func (p *transaction) add(path string, data []byte) error {
	if prior, ok := p.names[path]; ok {
		if !bytes.Equal(prior, data) {
			return conflict(path)
		}
		return nil
	}
	before, present, err := configfile.Read(path)
	if err != nil {
		return invalid(path, err)
	}
	mode := p.newMode
	if present {
		info, err := os.Stat(path)
		if err != nil {
			return invalid(path, err)
		}
		mode = info.Mode().Perm()
	}
	p.names[path] = data
	p.changes = append(p.changes, change{path: path, data: data, before: before, existed: present, mode: mode, dirMode: p.dirMode})
	return nil
}

func (p *transaction) apply(write func(string, []byte, os.FileMode) error, remove func(string) error) error {
	var applied []change
	var dirs []string
	rollback := func(cause error) error {
		var failures []error
		for i := len(applied) - 1; i >= 0; i-- {
			c := applied[i]
			var err error
			if c.existed {
				err = atomicWrite(c.path, c.before, c.mode)
			} else {
				err = os.Remove(c.path)
				if os.IsNotExist(err) {
					err = nil
				}
			}
			if err != nil {
				failures = append(failures, fmt.Errorf("restore %s: %w", c.path, err))
			}
		}
		for i := len(dirs) - 1; i >= 0; i-- {
			if err := os.Remove(dirs[i]); err != nil && !os.IsNotExist(err) {
				failures = append(failures, err)
			}
		}
		return errors.Join(append([]error{cause}, failures...)...)
	}
	for _, c := range p.changes {
		// Recheck the snapshot so an intervening destination change is never
		// overwritten. This does not claim cross-process transaction isolation.
		now, present, err := configfile.Read(c.path)
		if err != nil {
			return rollback(invalid(c.path, err))
		}
		if present != c.existed || !bytes.Equal(now, c.before) {
			return rollback(conflict(c.path))
		}
		if err := makeDirectories(filepath.Dir(c.path), &dirs, c.dirMode); err != nil {
			return rollback(invalid(c.path, err))
		}
		applied = append(applied, c)
		var applyErr error
		if c.remove {
			applyErr = remove(c.path)
		} else {
			applyErr = write(c.path, c.data, c.mode)
		}
		if err := applyErr; err != nil {
			return rollback(invalid(c.path, err))
		}
	}
	return nil
}
func makeDirectories(path string, created *[]string, mode os.FileMode) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("not a directory: %s", path)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	if err := makeDirectories(filepath.Dir(path), created, mode); err != nil {
		return err
	}
	if err := os.Mkdir(path, mode); err != nil {
		return err
	}
	*created = append(*created, path)
	return os.Chmod(path, mode)
}
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".curator-run-migrate-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
