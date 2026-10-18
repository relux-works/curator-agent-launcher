package composition_test

import (
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-network-profiles/pkg/envpatch"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

// A managed Muse backend beats ambient values, retains HOME, and cannot be
// undone by explicit owned env, a prompt overlay or the final network merge.
func TestMuseV4BackendMerge(t *testing.T) {
	f := fragment.Fragment{Revision: fragment.IdentityV4, Environment: fragment.EnvMuse, CredentialStore: &fragment.CredentialStore{Selection: "file"}, Env: map[string]string{"TBH_CREDENTIAL_BACKEND": "file", "XDG_CONFIG_HOME": "/managed/config"}}
	p := agentic.Plan{Env: []string{"HOME=/native", "TBH_CREDENTIAL_BACKEND=keychain"}}
	for _, root := range []bool{false, true} {
		if root {
			f.RootContext = &fragment.RootContext{Exclusive: true}
		}
		v, err := composition.Compose(p, nil, f, composition.PromptApplication{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if v.EnvLiterals["TBH_CREDENTIAL_BACKEND"] != "file" || !strings.Contains(strings.Join(v.Env, "\n"), "HOME=/native") {
			t.Fatalf("policy lost: %+v", v)
		}
		for _, tc := range []struct {
			name   string
			owned  []string
			prompt map[string]string
			patch  envpatch.Patch
		}{
			{name: "owned", owned: []string{"TBH_CREDENTIAL_BACKEND=keychain"}},
			{name: "prompt", prompt: map[string]string{"TBH_CREDENTIAL_BACKEND": "keychain"}},
			{name: "network set", patch: envpatch.Patch{Set: []envpatch.Pair{{Name: "TBH_CREDENTIAL_BACKEND", Value: "keychain"}}}},
			{name: "network unset", patch: envpatch.Patch{Unset: []string{"TBH_CREDENTIAL_BACKEND"}}},
		} {
			t.Run(tc.name+map[bool]string{false: "-without-root", true: "-root"}[root], func(t *testing.T) {
				_, err := composition.ComposeWithNetwork(p, tc.owned, f, composition.PromptApplication{Env: tc.prompt}, nil, tc.patch)
				if err == nil || !strings.Contains(err.Error(), "backend") {
					t.Fatalf("contradictory overlay: %v", err)
				}
			})
		}
	}
	// A generic proxy patch preserves the fact; relocation needs a snapshot
	// contract and refuses here rather than silently erasing root_context.
	patch := envpatch.Patch{Set: []envpatch.Pair{{Name: "HTTP_PROXY", Value: "http://127.0.0.1:8080"}}}
	if _, err := composition.ComposeWithNetwork(p, nil, f, composition.PromptApplication{}, nil, patch); err != nil {
		t.Fatal(err)
	}
	for _, patch := range []envpatch.Patch{{Set: []envpatch.Pair{{Name: "XDG_CONFIG_HOME", Value: "/other/config"}}}, {Unset: []string{"XDG_CONFIG_HOME"}}} {
		if _, err := composition.ComposeWithNetwork(p, nil, f, composition.PromptApplication{}, nil, patch); err == nil || !strings.Contains(err.Error(), "root_context") {
			t.Fatalf("relocation admitted: %v", err)
		}
	}
}
