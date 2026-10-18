// Package selector implements the optional, machine-owned model-selection seam.
package selector

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

const Schema = "curator-run-selector-v1"
const Limit = 64 << 10
const CodeUnavailable = "selector_unavailable"
const CodeAbstained = "selector_abstained"
const MessageUnavailable = "selector unavailable"
const MessageAbstained = "selector abstained"

// Failure exposes only the closed diagnostics vocabulary, never child output.
type Failure struct{ Code, Reason, RefusalCode string }

func (e *Failure) Error() string {
	if e.Code == CodeAbstained {
		return MessageAbstained + " code=" + e.RefusalCode
	}
	return MessageUnavailable + " reason=" + e.Reason
}
func unavailable(reason string) error { return &Failure{Code: CodeUnavailable, Reason: reason} }

type Config struct {
	Mode           string   `json:"mode"`
	Executable     string   `json:"executable"`
	ArgvPrefix     []string `json:"argv_prefix"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

var argument = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

func closed(v fragment.Value, required, optional []string) bool {
	if v.Kind != fragment.KindObject {
		return false
	}
	allowed := map[string]bool{}
	for _, k := range required {
		allowed[k] = true
		if _, ok := v.Get(k); !ok {
			return false
		}
	}
	for _, k := range optional {
		allowed[k] = true
	}
	for _, m := range v.Obj {
		if !allowed[m.Key] {
			return false
		}
	}
	return true
}
func ParseConfig(data []byte) (Config, error) {
	v, err := fragment.ParseJSON(data)
	if err != nil || !closed(v, []string{"schema", "mode", "executable", "argv_prefix"}, []string{"timeout_seconds"}) {
		return Config{}, fmt.Errorf("invalid selector configuration object")
	}
	schema, _ := v.Get("schema")
	mode, _ := v.Get("mode")
	executable, _ := v.Get("executable")
	prefix, _ := v.Get("argv_prefix")
	if schema.Kind != fragment.KindString || schema.Str != Schema || mode.Kind != fragment.KindString || (mode.Str != "shadow" && mode.Str != "enforce") || executable.Kind != fragment.KindString || !filepath.IsAbs(executable.Str) || strings.ContainsRune(executable.Str, 0) || prefix.Kind != fragment.KindArray || len(prefix.Arr) > 4 {
		return Config{}, fmt.Errorf("invalid selector configuration")
	}
	c := Config{Mode: mode.Str, Executable: executable.Str, ArgvPrefix: []string{}, TimeoutSeconds: 5}
	for _, a := range prefix.Arr {
		if a.Kind != fragment.KindString || !argument.MatchString(a.Str) {
			return Config{}, fmt.Errorf("invalid selector argv_prefix")
		}
		c.ArgvPrefix = append(c.ArgvPrefix, a.Str)
	}
	if timeout, ok := v.Get("timeout_seconds"); ok {
		if timeout.Kind != fragment.KindInt || timeout.Int < 1 || timeout.Int > 60 {
			return Config{}, fmt.Errorf("invalid selector timeout_seconds")
		}
		c.TimeoutSeconds = int(timeout.Int)
	}
	return c, nil
}

// Load applies common per-file fallback/security checks to both layers. Any
// user file is forbidden, even when an effective system file is present.
func Load(r configfile.Resolver) (Config, error) {
	var result Config
	for _, layer := range []configfile.Layer{configfile.System, configfile.User} {
		data, present, path, err := r.Read(layer, "selector.json")
		if err == nil && present {
			if layer == configfile.User {
				err = fmt.Errorf("selector.json is machine-only; configure %s", filepath.Join(r.Locations.System, "selector.json"))
			} else {
				result, err = ParseConfig(data)
			}
		}
		if err != nil {
			return Config{}, &defaults.Error{Code: defaults.CodeInvalid, Err: fmt.Errorf("%s: %w", path, err)}
		}
	}
	return result, nil
}
