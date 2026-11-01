// Package claudeauth resolves explicit OAuth metadata only at the native child boundary.
package claudeauth

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
)

const Variable = "CLAUDE_CODE_OAUTH_TOKEN"
const maxBytes = 16 * 1024

// ErrRefused deliberately contains neither source references nor secret bytes.
var ErrRefused = errors.New("plan_refused: selected Claude OAuth source is unavailable or unsafe")

// Resolver never modifies the parent environment or falls back to ambient credentials.
// Keychain is a test seam; nil selects the bounded platform implementation.
type Resolver struct {
	Keychain func(context.Context, string, string) ([]byte, error)
}

// Reserved identifies credential and backend switches that cannot be overlaid
// when a machine-selected OAuth source owns the child credential.
func Reserved(name string) bool {
	switch name {
	case Variable, "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY":
		return true
	}
	return false
}

// CheckOverlays refuses credential literals and destination-local lookups before
// lookup. Selecting a store cannot authorize another configuration source.
func CheckOverlays(literals map[string]string, names []string) error {
	for name := range literals {
		if Reserved(name) {
			return ErrRefused
		}
	}
	for _, name := range names {
		if Reserved(name) {
			return ErrRefused
		}
	}
	return nil
}

// Inject returns a new final-child environment. Call it after all ordinary launch
// checks, never while preparing plans, probing providers, or serializing requests.
func (r Resolver) Inject(ctx context.Context, selected launcherconfig.ClaudeAuth, env []string) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ErrRefused
	}
	if err := launcherconfig.ValidateClaudeAuth(selected); err != nil {
		return nil, ErrRefused
	}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		switch key {
		case "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY":
			if value != "" {
				return nil, ErrRefused
			}
		}
	}
	var raw []byte
	var err error
	switch selected.Store.Type {
	case "file":
		raw, err = readFile(selected.Store.Path)
	case "keychain":
		lookup := r.Keychain
		if lookup == nil {
			lookup = readKeychain
		}
		raw, err = lookup(ctx, selected.Store.Service, selected.Store.Account)
	default:
		return nil, ErrRefused
	}
	defer clear(raw)
	if err != nil || ctx.Err() != nil || len(raw) == 0 || len(raw) > maxBytes {
		return nil, ErrRefused
	}
	// security emits a newline; files may use the same single-line representation.
	token := string(raw)
	if strings.HasSuffix(token, "\n") {
		token = strings.TrimSuffix(strings.TrimSuffix(token, "\n"), "\r")
	}
	if token == "" || !utf8.ValidString(token) || strings.ContainsFunc(token, func(c rune) bool { return unicode.IsSpace(c) || unicode.IsControl(c) }) {
		return nil, ErrRefused
	}
	child := make([]string, 0, len(env)+1)
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if key != Variable {
			child = append(child, entry)
		}
	}
	child = append(child, Variable+"="+token)
	return child, nil
}
