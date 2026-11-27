package composition

import (
	"slices"
	"strings"
	"testing"

	"github.com/relux-works/archon-pantheon/pkg/agentic"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-network-profiles/pkg/envpatch"
)

func antigravityV7Fragment() fragment.Fragment {
	return fragment.Fragment{
		Revision:        fragment.IdentityV7,
		Environment:     fragment.EnvAntigravity,
		HomeBinding:     &fragment.HomeBinding{Path: "/manager/environments/default/antigravity", Transport: "flag"},
		CredentialStore: &fragment.CredentialStore{Selection: "file"},
		Env: map[string]string{
			fragment.AntigravityForceFileTokenVariable: fragment.AntigravityForceFileTokenTrue,
		},
	}
}

func TestAntigravityCompositionPolicy(t *testing.T) {
	plan := agentic.Plan{
		Binary:  "/opt/agy/bin/agy",
		WorkDir: "/work",
		Argv:    []string{"--gemini_dir", "/manager/environments/default/antigravity", "--model", "gemini-3.6-flash-high"},
		Env:     []string{"PATH=/usr/bin", "AGY_CLI_FORCE_FILE_TOKEN_STORAGE=true", "AGY_CLI_DISABLE_AUTO_UPDATE=true"},
	}
	frag := antigravityV7Fragment()

	got, err := ComposeAdmittedPlanWithNetwork(plan, []string{"AGY_CLI_FORCE_FILE_TOKEN_STORAGE=true"}, frag, PromptApplication{}, nil, envpatch.Unmanaged())
	if err != nil {
		t.Fatalf("ComposeAdmittedPlanWithNetwork: %v", err)
	}
	if !slices.Contains(got.Env, "AGY_CLI_FORCE_FILE_TOKEN_STORAGE=true") {
		t.Fatalf("missing forced file token env in %v", got.Env)
	}

	// Incompatible plan-owned override refuses.
	if _, err := ComposeAdmittedPlanWithNetwork(plan, []string{"AGY_CLI_FORCE_FILE_TOKEN_STORAGE=false"}, frag, PromptApplication{}, nil, envpatch.Unmanaged()); err == nil || !strings.Contains(err.Error(), "override") {
		t.Fatalf("ownEnv override err = %v, want override refusal", err)
	}

	// Network patch unset refuses.
	unsetPatch := envpatch.Patch{Unset: []string{"agy_cli_force_file_token_storage"}}
	if _, err := ComposeAdmittedPlanWithNetwork(plan, nil, frag, PromptApplication{}, nil, unsetPatch); err == nil || !strings.Contains(err.Error(), "unset") {
		t.Fatalf("network unset err = %v, want unset refusal", err)
	}

	// Network patch override refuses.
	setPatch := envpatch.Patch{Set: []envpatch.Pair{{Name: "AGY_CLI_FORCE_FILE_TOKEN_STORAGE", Value: "false"}}}
	if _, err := ComposeAdmittedPlanWithNetwork(plan, nil, frag, PromptApplication{}, nil, setPatch); err == nil || !strings.Contains(err.Error(), "override") {
		t.Fatalf("network set override err = %v, want override refusal", err)
	}

	// Missing flag-home binding refuses.
	badFrag := frag
	badFrag.HomeBinding = nil
	if _, err := ComposeAdmittedPlanWithNetwork(plan, nil, badFrag, PromptApplication{}, nil, envpatch.Unmanaged()); err == nil {
		t.Fatal("missing HomeBinding admitted")
	}

	// Ambient auth variables in ownEnv, prompt.Env, or network.Patch refuse (CIP-0033 X2 / l1).
	for _, forbidden := range []string{"AGY_ADC_AUTH", "GEMINI_API_KEY", "GOOGLE_API_KEY", "GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_CLOUD_QUOTA_PROJECT"} {
		if _, err := ComposeAdmittedPlanWithNetwork(plan, []string{forbidden + "=1"}, frag, PromptApplication{}, nil, envpatch.Unmanaged()); err == nil {
			t.Fatalf("ownEnv admitted forbidden antigravity auth var %s", forbidden)
		}
		if _, err := ComposeAdmittedPlanWithNetwork(plan, nil, frag, PromptApplication{Env: map[string]string{forbidden: "1"}}, nil, envpatch.Unmanaged()); err == nil {
			t.Fatalf("prompt.Env admitted forbidden antigravity auth var %s", forbidden)
		}
		if _, err := ComposeAdmittedPlanWithNetwork(plan, nil, frag, PromptApplication{}, nil, envpatch.Patch{Set: []envpatch.Pair{{Name: forbidden, Value: "1"}}}); err == nil {
			t.Fatalf("network patch admitted forbidden antigravity auth var %s", forbidden)
		}
	}
}
