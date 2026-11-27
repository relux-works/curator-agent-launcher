package composition

import (
	"fmt"
	"strings"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-network-profiles/pkg/envpatch"
)

// antigravityPolicy enforces CIP-0033 managed Antigravity invariants across
// fragment, plan-owned env, prompt overlays, and network patches: the forced
// file-token storage constant AGY_CLI_FORCE_FILE_TOKEN_STORAGE=true and flag
// home binding must never be contradicted or unset.
func antigravityPolicy(ownEnv []string, f fragment.Fragment, prompt PromptApplication, patch envpatch.Patch) error {
	if f.Environment != fragment.EnvAntigravity {
		return nil
	}
	const backend = fragment.AntigravityForceFileTokenVariable
	const required = fragment.AntigravityForceFileTokenTrue
	if (f.Revision != fragment.IdentityV7 && f.Revision != fragment.IdentityV9) ||
		f.HomeBinding == nil || f.HomeBinding.Transport != "flag" || f.HomeBinding.Path == "" ||
		f.Env[backend] != required || f.CredentialStore == nil || f.CredentialStore.Selection != "file" {
		return fmt.Errorf("Antigravity requires the validated v7 flag-home and file token storage policy")
	}
	for _, layer := range []map[string]string{envMap(ownEnv), prompt.Env} {
		for name, value := range layer {
			if strings.EqualFold(name, backend) {
				if name != backend || value != required {
					return fmt.Errorf("incompatible explicit Antigravity file token storage override")
				}
				continue
			}
			if strings.EqualFold(name, fragment.AntigravityDisableAutoUpdateVariable) {
				if name != fragment.AntigravityDisableAutoUpdateVariable || value != "true" {
					return fmt.Errorf("incompatible explicit Antigravity auto-update override")
				}
				continue
			}
			if fragment.IsAntigravityForbiddenEnv(name) {
				return fmt.Errorf("incompatible explicit Antigravity ambient auth variable %q", name)
			}
		}
	}
	for _, name := range patch.Unset {
		if strings.EqualFold(name, backend) {
			return fmt.Errorf("network patch cannot unset Antigravity file token storage")
		}
		if strings.EqualFold(name, fragment.AntigravityDisableAutoUpdateVariable) {
			return fmt.Errorf("network patch cannot unset Antigravity auto-update pin")
		}
	}
	for _, pair := range patch.Set {
		if strings.EqualFold(pair.Name, backend) {
			if pair.Name != backend || pair.Value != required {
				return fmt.Errorf("network patch cannot override Antigravity file token storage")
			}
			continue
		}
		if strings.EqualFold(pair.Name, fragment.AntigravityDisableAutoUpdateVariable) {
			if pair.Name != fragment.AntigravityDisableAutoUpdateVariable || pair.Value != "true" {
				return fmt.Errorf("network patch cannot override Antigravity auto-update pin")
			}
			continue
		}
		if fragment.IsAntigravityForbiddenEnv(pair.Name) {
			return fmt.Errorf("network patch cannot inject Antigravity ambient auth variable %q", pair.Name)
		}
	}
	return nil
}
