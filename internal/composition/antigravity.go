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
	if f.Revision != fragment.IdentityV7 ||
		f.HomeBinding == nil || f.HomeBinding.Transport != "flag" || f.HomeBinding.Path == "" ||
		f.Env[backend] != required || f.CredentialStore == nil || f.CredentialStore.Selection != "file" {
		return fmt.Errorf("Antigravity requires the validated v7 flag-home and file token storage policy")
	}
	for _, layer := range []map[string]string{envMap(ownEnv), prompt.Env} {
		if value, ok := layer[backend]; ok && value != required {
			return fmt.Errorf("incompatible explicit Antigravity file token storage override")
		}
	}
	for _, name := range patch.Unset {
		if strings.EqualFold(name, backend) {
			return fmt.Errorf("network patch cannot unset Antigravity file token storage")
		}
	}
	for _, pair := range patch.Set {
		if strings.EqualFold(pair.Name, backend) && (pair.Name != backend || pair.Value != required) {
			return fmt.Errorf("network patch cannot override Antigravity file token storage")
		}
	}
	return nil
}
