package launcherconfig

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

// ClaudeEnvVariable is the only name extensions.claude_env admits. The
// proposal (curator-spec protocol/machine-claude-env-proposal.md) adds no
// generic environment map.
const ClaudeEnvVariable = "BASH_MAX_OUTPUT_LENGTH"

// ClaudeEnvPath is the fixed diagnostic path for every member refusal.
const ClaudeEnvPath = "extensions.claude_env"

// claudeEnvBound is the proposed manager policy cap, not a vendor maximum.
const claudeEnvBound = 65536

// Stable refusal codes from contract section 4.
const (
	CodeClaudeEnvInvalid           = "claude_env_invalid"
	CodeClaudeEnvNameNotAllowed    = "claude_env_name_not_allowed"
	CodeClaudeEnvValueNotInteger   = "claude_env_value_not_integer"
	CodeClaudeEnvValueNonpositive  = "claude_env_value_nonpositive"
	CodeClaudeEnvValueOverBound    = "claude_env_value_over_bound"
	CodeClaudeEnvRuntimeIneligible = "claude_env_runtime_ineligible"
)

// ClaudeEnvError carries only a stable code and the fixed member path. It
// never contains a supplied value, an unknown name or parser text. It still
// matches ErrSelected, so callers that only know the closed projection
// refusal keep refusing.
type ClaudeEnvError struct{ Code string }

func (e *ClaudeEnvError) Error() string        { return e.Code + ": " + ClaudeEnvPath }
func (e *ClaudeEnvError) Is(target error) bool { return target == ErrSelected }

func claudeEnvRefusal(code string) error { return &ClaudeEnvError{Code: code} }

// ClaudeEnv is the validated machine selection. An empty value means the
// member was absent or {}: no override and no contribution.
type ClaudeEnv struct {
	BashMaxOutputLength string
}

// Selected reports whether the selection contributes anything.
func (c *ClaudeEnv) Selected() bool { return c != nil && c.BashMaxOutputLength != "" }

// Contribution is this extension's part of one child environment for the
// given runtime: the selected name for claude_code, nothing otherwise.
func (c *ClaudeEnv) Contribution(runtime string) map[string]string {
	if !c.Selected() || runtime != "claude_code" {
		return map[string]string{}
	}
	return map[string]string{ClaudeEnvVariable: c.BashMaxOutputLength}
}

var signedDecimal = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

// claudeEnvValue classifies one declared value. Only a JSON string holding a
// canonical ASCII decimal in 1..65536 is admitted; a spelling matching the
// signed decimal grammar is classified by sign and magnitude, and everything
// else (numbers, booleans, null, spaces, fractions, exponents, leading zeros,
// plus signs, non-ASCII digits) is not an integer.
func claudeEnvValue(v fragment.Value) (string, error) {
	if v.Kind != fragment.KindString || !signedDecimal.MatchString(v.Str) {
		return "", claudeEnvRefusal(CodeClaudeEnvValueNotInteger)
	}
	if strings.HasPrefix(v.Str, "-") || v.Str == "0" {
		return "", claudeEnvRefusal(CodeClaudeEnvValueNonpositive)
	}
	// Longer than the bound's five digits is over the bound without parsing,
	// so an overflowing spelling never reaches strconv.
	if len(v.Str) > len(strconv.Itoa(claudeEnvBound)) {
		return "", claudeEnvRefusal(CodeClaudeEnvValueOverBound)
	}
	if n, err := strconv.Atoi(v.Str); err != nil || n > claudeEnvBound {
		return "", claudeEnvRefusal(CodeClaudeEnvValueOverBound)
	}
	return v.Str, nil
}

// parseClaudeEnv validates the member after strict decoding. Precedence is
// the contract's: shape, then every name, then the value.
func parseClaudeEnv(v fragment.Value) (*ClaudeEnv, error) {
	if v.Kind != fragment.KindObject {
		return nil, claudeEnvRefusal(CodeClaudeEnvInvalid)
	}
	for _, member := range v.Obj {
		if member.Key != ClaudeEnvVariable {
			return nil, claudeEnvRefusal(CodeClaudeEnvNameNotAllowed)
		}
	}
	result := &ClaudeEnv{}
	if declared, ok := v.Get(ClaudeEnvVariable); ok {
		value, err := claudeEnvValue(declared)
		if err != nil {
			return nil, err
		}
		result.BashMaxOutputLength = value
	}
	return result, nil
}

var claudeEnvRuntimes = []string{"claude_code", "codex_cli", "opencode", "pi", "muse"}

// ValidateContribution checks one synthetic extension contribution
// {"runtime": R, "env": {...}}, never a whole child environment. An empty
// contribution is valid for every admitted runtime; a non-empty one is valid
// only for claude_code. Member defects take precedence over eligibility.
func ValidateContribution(data []byte) error {
	root, err := fragment.ParseJSON(data)
	if err != nil || !exactMembers(root, "runtime", "env") {
		return claudeEnvRefusal(CodeClaudeEnvInvalid)
	}
	runtime := stringMember(root, "runtime")
	if !slices.Contains(claudeEnvRuntimes, runtime) {
		return claudeEnvRefusal(CodeClaudeEnvInvalid)
	}
	env, _ := root.Get("env")
	selected, err := parseClaudeEnv(env)
	if err != nil {
		return err
	}
	if runtime != "claude_code" && selected.Selected() {
		return claudeEnvRefusal(CodeClaudeEnvRuntimeIneligible)
	}
	return nil
}

// ApplyClaudeEnv returns a copy of env for a Claude Code child with the
// selected value replacing every inherited entry of the same name. Other
// entries keep their order and values. The caller's slice is not modified,
// and an empty selection returns an unchanged copy.
func ApplyClaudeEnv(env []string, selected *ClaudeEnv) []string {
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if selected.Selected() && strings.HasPrefix(entry, ClaudeEnvVariable+"=") {
			continue
		}
		out = append(out, entry)
	}
	if selected.Selected() {
		out = append(out, ClaudeEnvVariable+"="+selected.BashMaxOutputLength)
	}
	return out
}
