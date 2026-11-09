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

// In the native credential form nothing in the child chooses a backend: the
// composed environment has no backend variable, and an inherited, owned,
// prompt or network-set value refuses instead of being stripped. A network
// unset of the absent name is harmless and admitted.
func TestMuseV4NativeForm(t *testing.T) {
	f := fragment.Fragment{Revision: fragment.IdentityV4, Environment: fragment.EnvMuse, CredentialStore: &fragment.CredentialStore{Selection: "native"}, Env: map[string]string{"XDG_CONFIG_HOME": "/managed/config"}}
	p := agentic.Plan{Env: []string{"HOME=/native"}}
	v, err := composition.Compose(p, nil, f, composition.PromptApplication{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(v.Env, "\n"), "TBH_CREDENTIAL_BACKEND") || !strings.Contains(strings.Join(v.Env, "\n"), "HOME=/native") {
		t.Fatalf("native child env: %v", v.Env)
	}
	if _, err := composition.ComposeWithNetwork(p, nil, f, composition.PromptApplication{}, nil, envpatch.Patch{Unset: []string{"TBH_CREDENTIAL_BACKEND"}}); err != nil {
		t.Fatalf("harmless unset refused: %v", err)
	}
	for _, tc := range []struct {
		name   string
		plan   []string
		owned  []string
		prompt map[string]string
		patch  envpatch.Patch
	}{
		{name: "inherited", plan: []string{"HOME=/native", "TBH_CREDENTIAL_BACKEND=file"}},
		{name: "inherited-lowercase", plan: []string{"HOME=/native", "tbh_credential_backend=file"}},
		{name: "owned", owned: []string{"TBH_CREDENTIAL_BACKEND=keychain"}},
		{name: "prompt", prompt: map[string]string{"TBH_CREDENTIAL_BACKEND": "file"}},
		{name: "network set", patch: envpatch.Patch{Set: []envpatch.Pair{{Name: "TBH_CREDENTIAL_BACKEND", Value: "file"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := p
			if tc.plan != nil {
				plan = agentic.Plan{Env: tc.plan}
			}
			_, err := composition.ComposeWithNetwork(plan, tc.owned, f, composition.PromptApplication{Env: tc.prompt}, nil, tc.patch)
			if err == nil || !strings.Contains(err.Error(), "backend") && !strings.Contains(err.Error(), "TBH_CREDENTIAL_BACKEND") {
				t.Fatalf("native form admitted a backend: %v", err)
			}
		})
	}
	// A mixed fragment that bypassed parsing refuses at composition too.
	for _, bad := range []fragment.Fragment{
		{Revision: fragment.IdentityV4, Environment: fragment.EnvMuse, CredentialStore: &fragment.CredentialStore{Selection: "native"}, Env: map[string]string{"TBH_CREDENTIAL_BACKEND": "file"}},
		{Revision: fragment.IdentityV4, Environment: fragment.EnvMuse, CredentialStore: &fragment.CredentialStore{Selection: "file"}, Env: map[string]string{}},
		{Revision: fragment.IdentityV4, Environment: fragment.EnvMuse, Env: map[string]string{}},
	} {
		if _, err := composition.Compose(p, nil, bad, composition.PromptApplication{}, nil); err == nil {
			t.Fatalf("mixed form composed: %+v", bad)
		}
	}
}
