package composition

import (
	"fmt"
	"strings"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-network-profiles/pkg/envpatch"
)

// Ambient backend values are replaced by the fragment; explicit plan-owned
// or later overlays may not contradict it. Network relocation cannot replace
// the verified root with a new, unverified home.
func musePolicy(ownEnv []string, f fragment.Fragment, prompt PromptApplication, patch envpatch.Patch) error {
	if f.Environment != fragment.EnvMuse || f.Revision != fragment.IdentityV4 {
		return nil
	}
	const backend = "TBH_CREDENTIAL_BACKEND"
	if f.Env[backend] != "file" || f.CredentialStore == nil || f.CredentialStore.Selection != "file" {
		return fmt.Errorf("Muse requires the validated v4 file backend")
	}
	for _, layer := range []map[string]string{envMap(ownEnv), prompt.Env} {
		if value, ok := layer[backend]; ok && value != "file" {
			return fmt.Errorf("incompatible explicit Muse credential backend override")
		}
	}
	for _, name := range patch.Unset {
		if strings.EqualFold(name, backend) {
			return fmt.Errorf("network patch cannot unset the Muse credential backend")
		}
	}
	for _, pair := range patch.Set {
		if strings.EqualFold(pair.Name, backend) && (pair.Name != backend || pair.Value != "file") {
			return fmt.Errorf("network patch cannot override the Muse credential backend")
		}
	}
	if f.RootContext != nil {
		for _, name := range patch.Unset {
			for key := range f.Env {
				if strings.EqualFold(name, key) {
					return fmt.Errorf("network patch cannot relocate verified Muse root_context")
				}
			}
		}
		for _, pair := range patch.Set {
			for key, value := range f.Env {
				if strings.EqualFold(pair.Name, key) && (pair.Name != key || pair.Value != value) {
					return fmt.Errorf("network patch cannot relocate verified Muse root_context")
				}
			}
		}
	}
	return nil
}
