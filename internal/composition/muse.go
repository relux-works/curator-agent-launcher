package composition

import (
	"fmt"
	"strings"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-network-profiles/pkg/envpatch"
)

// Ambient backend values are replaced by the fragment in the file form;
// explicit plan-owned or later overlays may not contradict it. In the native
// form (Curator's shared-home Keychain form) the fragment sets no backend, so
// any inherited, plan-owned, prompt or network value would silently choose a
// store: that refuses rather than being stripped. Network relocation cannot
// replace the verified root with a new, unverified home.
func musePolicy(ownEnv, planEnv []string, f fragment.Fragment, prompt PromptApplication, patch envpatch.Patch) error {
	if f.Environment != fragment.EnvMuse || f.Revision != fragment.IdentityV4 {
		return nil
	}
	const backend = fragment.MuseCredentialBackend
	value, present := f.Env[backend]
	switch {
	case f.CredentialStore == nil:
		return fmt.Errorf("Muse requires a validated v4 credential store")
	case f.CredentialStore.Selection == "file" && present && value == "file":
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
	case f.CredentialStore.Selection == "native" && !present:
		for _, layer := range []map[string]string{envMap(ownEnv), envMap(planEnv), prompt.Env} {
			for name := range layer {
				if strings.EqualFold(name, backend) {
					return fmt.Errorf("Muse native credential form refuses an inherited or explicit %s; unset it", backend)
				}
			}
		}
		for _, pair := range patch.Set {
			if strings.EqualFold(pair.Name, backend) {
				return fmt.Errorf("network patch cannot set the Muse credential backend")
			}
		}
	default:
		return fmt.Errorf("Muse requires the validated v4 file or native credential form")
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
