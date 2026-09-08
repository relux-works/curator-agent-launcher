// Package systemprompt implements SPEC §5 policy for validated fragments.
// Execution must call PrepareLaunch immediately before EVERY handoff or exec.
// It does not compose a plan, inspect native arguments, print, or write files.
package systemprompt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

const (
	CodeUnavailable = "sysprompt_channel_unavailable"
	CodeUnreadable  = "sysprompt_file_unreadable"
)

// Refusal is a terminal launch diagnostic. Cause preserves filesystem errors.
type Refusal struct {
	Code, Path string
	Cause      error
}

func (e *Refusal) Error() string { return fmt.Sprintf("%s: %q: %v", e.Code, e.Path, e.Cause) }
func (e *Refusal) Unwrap() error { return e.Cause }

// Selection contains only the launcher-applied channel, never native inputs.
// Its argv and environment are copied out for later composition.
type Selection struct {
	channel fragment.Channel
	path    string
	argv    []string
}

func (s Selection) Argv() []string { return append([]string(nil), s.argv...) }

// Env is empty in registry revision 1: no system-prompt variable is declared.
func (s Selection) Env() map[string]string { return nil }

// Select accepts a fragment from fragment.Parse and an explicit append/replace
// opt-in, or "" for no opt-in. It performs no I/O. File channels cannot opt in.
func Select(f *fragment.Fragment, opt fragment.Semantics) (Selection, error) {
	if opt == "" {
		return Selection{}, nil
	}
	if f != nil && f.SystemPrompt != nil && (opt == fragment.SemanticsAppend || opt == fragment.SemanticsReplace) {
		for _, c := range f.SystemPrompt.Channels {
			if c.Semantics != opt || c.Kind == fragment.KindFile {
				continue
			}
			s := Selection{channel: c, path: f.SystemPrompt.Path}
			switch c.Kind {
			case fragment.KindFlag:
				if c.Argument != fragment.ArgumentPath {
					continue
				}
				s.argv = append([]string{c.Flag, s.path}, c.With...)
			case fragment.KindConfigKey:
				if f.Environment != fragment.EnvCodexCLI {
					continue
				}
				s.argv = []string{"-c", c.Key + "=" + tomlString(s.path)}
			default:
				// No variable channel or contents/name prompt flag exists in the closed
				// registry. New adapters require a protocol revision, not a fallback.
				continue
			}
			return s, nil
		}
	}
	return Selection{}, &Refusal{Code: CodeUnavailable, Cause: fmt.Errorf("no non-file system-prompt channel for %q", opt)}
}

// Codex -c parses a TOML value, not shell text. Go's \x escapes are not TOML.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, "\\u%04X", r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Observation records a readable regular managed-home file, not application.
type Observation struct {
	Filename  string
	Semantics fragment.Semantics
}

// ProbeFiles probes the closed registry set regardless of SystemPrompt presence.
// Absence returns no observation; errors (including dangling links) refuse.
// Symlinks to readable regular files are accepted. No contents are consumed.
func ProbeFiles(f *fragment.Fragment) ([]Observation, error) {
	if f.Environment != fragment.EnvPi {
		return nil, nil
	}
	files := []Observation{
		{Filename: "APPEND_SYSTEM.md", Semantics: fragment.SemanticsAppend},
		{Filename: "SYSTEM.md", Semantics: fragment.SemanticsReplace},
	}
	var observed []Observation
	for _, file := range files {
		present, err := readable(filepath.Join(f.Home(), file.Filename), true)
		if err != nil {
			return nil, err
		}
		if present {
			observed = append(observed, file)
		}
	}
	return observed, nil
}

func readable(path string, allowAbsent bool) (bool, error) {
	fail := func(err error) (bool, error) { return false, &Refusal{Code: CodeUnreadable, Path: path, Cause: err} }
	_, err := os.Lstat(path)
	if err != nil {
		if allowAbsent && errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return fail(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fail(err)
	}
	if !info.Mode().IsRegular() {
		return fail(fmt.Errorf("not a regular file"))
	}
	// Open only after the regular-file check, so stable FIFOs cannot block.
	file, err := os.Open(path)
	if err != nil {
		return fail(err)
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return fail(err)
	}
	if !info.Mode().IsRegular() {
		return fail(fmt.Errorf("opened object is not a regular file"))
	}
	return true, nil
}

// Launch contains composition inputs and warning lines (without a trailing LF).
// Both tracked and untracked execution must emit every line before launching.
type Launch struct {
	Selection Selection
	Warnings  []string
}

// PrepareLaunch is the execution boundary operation: select, freshly validate
// Pi's polymorphic flag path and managed-home files, then format warnings.
// Do not cache its result across launches. It does not attest project discovery
// or prevent changes between this probe and the native tool's subsequent open.
func PrepareLaunch(f *fragment.Fragment, opt fragment.Semantics) (Launch, error) {
	s, err := Select(f, opt)
	if err != nil {
		return Launch{}, err
	}
	if f.Environment == fragment.EnvPi && s.channel.Kind == fragment.KindFlag {
		if _, err = readable(s.path, false); err != nil {
			return Launch{}, err
		}
	}
	observed, err := ProbeFiles(f)
	if err != nil {
		return Launch{}, err
	}
	return Launch{Selection: s, Warnings: FormatWarnings(f.Profile.Name, s, observed)}, nil
}

const cacheWarning = " A custom system prefix can change request caching and billing: the default may use shared prompt caching; a custom prefix forms its own cache prefix."

// FormatWarnings is pure and shared by both execution modes. Native arguments
// remain opaque, so only the selected launcher flag establishes suppression.
func FormatWarnings(profile string, s Selection, observed []Observation) []string {
	var lines []string
	if s.channel.Kind != "" {
		line := fmt.Sprintf("warning: profile %q: applied %s/%s system-prompt channel", profile, s.channel.Kind, s.channel.Semantics)
		if s.channel.Kind == fragment.KindFlag {
			line += fmt.Sprintf(" %q", s.channel.Flag)
		}
		line += "."
		if s.channel.Semantics == fragment.SemanticsReplace {
			line += " Replacement discards the tool's built-in system behavior entirely."
		}
		lines = append(lines, line+cacheWarning)
	}
	for _, file := range observed {
		line := fmt.Sprintf("warning: profile %q: observed file/%s %q in managed home; ", profile, file.Semantics, file.Filename)
		if s.channel.Kind == fragment.KindFlag && s.channel.Semantics == file.Semantics {
			line += fmt.Sprintf("discovery suppressed by applied launcher flag %q.", s.channel.Flag)
		} else {
			line += "conditional native-discovery candidate, subject to native flags and trusted-project precedence."
			if file.Semantics == fragment.SemanticsReplace {
				line += " If selected, replacement discards the tool's built-in system behavior entirely."
			}
		}
		lines = append(lines, line+cacheWarning)
	}
	return lines
}
