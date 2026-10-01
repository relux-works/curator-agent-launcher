package composition_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-network-profiles/pkg/envpatch"
	"github.com/relux-works/curator-network-profiles/pkg/netprofile"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

// genericPatch builds the real generic patch for a test profile. The
// profile is constructed, not parsed: Generic.Patch takes the normalized
// struct, so no catalog fixture is needed here.
func genericPatch() envpatch.Patch {
	return envpatch.Generic{}.Patch(netprofile.Profile{
		Name: "egress-a", Kind: netprofile.KindExternalHTTPProxy,
		Endpoint: "http://127.0.0.1:18080", BypassHosts: []string{"127.0.0.1", "::1", "localhost"},
		CredentialMode: netprofile.CredentialModeNone,
	})
}

// TestComposeWithNetworkAppliesPatchLast drives the production Compose
// path with a managed patch: ambient proxy values in every letter case
// vanish from both env and literals, the set half lands in both, and
// sibling names and MCP lookups are untouched.
func TestComposeWithNetworkAppliesPatchLast(t *testing.T) {
	f := parsed(t, "codex_cli", "/managed/default/codex/layer.toml", []string{"FIGMA_API_KEY"})
	p := agentic.Plan{Env: []string{
		"PATH=/sanitized", "HOME=/parent",
		"HTTP_PROXY=http://ambient:8080", "Http_Proxy=http://mixed:8080",
		"http_proxy=http://ambient:8080", "NO_PROXY=ambient", "no_proxy=ambient",
		"ALL_PROXY=http://ambient:8080", "FTP_PROXY=http://ambient:8080",
		"FIGMA_API_KEY=source-secret", "SIBLING=kept",
	}}
	owned := []string{"OWN=owned-value", "HTTP_PROXY=owned-proxy", "Http_Proxy=owned-mixed"}
	v, err := composition.ComposeWithNetwork(p, owned, f, composition.PromptApplication{}, nil, genericPatch())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range v.Env {
		name, _, _ := strings.Cut(entry, "=")
		for _, u := range envpatch.UnsetNames() {
			if strings.EqualFold(name, u) && !isPatchSetName(name) {
				t.Fatalf("ambient proxy %q survives in env", entry)
			}
		}
		if strings.Contains(entry, "ambient") || strings.Contains(entry, "mixed") {
			t.Fatalf("ambient proxy value survives: %q", entry)
		}
	}
	wantSet := map[string]string{
		"HTTP_PROXY": "http://127.0.0.1:18080", "HTTPS_PROXY": "http://127.0.0.1:18080",
		"http_proxy": "http://127.0.0.1:18080", "https_proxy": "http://127.0.0.1:18080",
		"NO_PROXY": "127.0.0.1,::1,localhost", "no_proxy": "127.0.0.1,::1,localhost",
	}
	for name, value := range wantSet {
		if got := v.EnvLiterals[name]; got != value {
			t.Fatalf("literals[%q] = %q, want %q", name, got, value)
		}
		if !slicesContains(v.Env, name+"="+value) {
			t.Fatalf("env lacks %q", name+"="+value)
		}
	}
	if v.EnvLiterals["OWN"] != "owned-value" || !slicesContains(v.Env, "SIBLING=kept") {
		t.Fatalf("sibling names disturbed: literals=%v env=%v", v.EnvLiterals, v.Env)
	}
	equal(t, v.EnvNames, []string{"FIGMA_API_KEY"})
	for _, w := range v.Warnings {
		if strings.Contains(strings.ToLower(w), "proxy") {
			t.Fatalf("patch application warned: %q", w)
		}
	}
}

func isPatchSetName(name string) bool {
	for _, kv := range genericPatch().Set {
		if kv.Name == name {
			return true
		}
	}
	return false
}

func slicesContains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// TestComposeWithNetworkRefusesProxyOverlays: while managed, a
// proxy-family name in frag.Env, prompt.Env, or MCP env_names refuses
// with network_configuration_conflict in every letter case. The same
// overlays compose untouched when unmanaged.
func TestComposeWithNetworkRefusesProxyOverlays(t *testing.T) {
	managed := genericPatch()
	mkfrag := func(t *testing.T) fragment.Fragment {
		return parsed(t, "codex_cli", "/managed/default/codex/layer.toml", []string{"FIGMA_API_KEY"})
	}
	cases := []struct {
		name   string
		mutate func(f *fragment.Fragment, prompt *composition.PromptApplication)
	}{
		{"frag canonical upper", func(f *fragment.Fragment, _ *composition.PromptApplication) {
			f.Env["HTTP_PROXY"] = "http://frag:8080"
		}},
		{"frag canonical lower", func(f *fragment.Fragment, _ *composition.PromptApplication) {
			f.Env["no_proxy"] = "frag"
		}},
		{"prompt mixed case", func(_ *fragment.Fragment, prompt *composition.PromptApplication) {
			prompt.Env = map[string]string{"Http_Proxy": "http://prompt:8080"}
		}},
		{"prompt all proxy", func(_ *fragment.Fragment, prompt *composition.PromptApplication) {
			prompt.Env = map[string]string{"ALL_PROXY": "http://prompt:8080"}
		}},
		{"mcp mixed case", func(f *fragment.Fragment, _ *composition.PromptApplication) {
			f.MCP.EnvNames = []string{"FIGMA_API_KEY", "Http_Proxy"}
		}},
		{"mcp canonical", func(f *fragment.Fragment, _ *composition.PromptApplication) {
			f.MCP.EnvNames = []string{"HTTPS_PROXY"}
		}},
		{"mcp alternating case", func(f *fragment.Fragment, _ *composition.PromptApplication) {
			f.MCP.EnvNames = []string{"fTp_PrOxY"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, prompt := mkfrag(t), composition.PromptApplication{}
			tc.mutate(&f, &prompt)
			_, err := composition.ComposeWithNetwork(agentic.Plan{}, nil, f, prompt, nil, managed)
			r, ok := refusal.As(err)
			if !ok || r == nil || r.Code != refusal.CodeConfigurationConflict {
				t.Fatalf("managed error = %v, want %s", err, refusal.CodeConfigurationConflict)
			}
			// The same overlays compose untouched when unmanaged: the
			// refusal is managed-only.
			if _, err := composition.Compose(agentic.Plan{}, nil, f, prompt, nil); err != nil {
				t.Fatalf("unmanaged error = %v, want nil", err)
			}
		})
	}
}

// TestComposeWithNetworkOrderPatchLast proves the application order with
// a colliding synthetic patch: the patch wins over every earlier layer,
// so a mutant that applies it first fails here.
func TestComposeWithNetworkOrderPatchLast(t *testing.T) {
	f := parsed(t, "opencode", "/managed/default/tool/mcp.json", nil)
	f.Env["X_ORDER"] = "frag"
	patch := envpatch.Patch{Set: []envpatch.Pair{{Name: "X_ORDER", Value: "patch"}}}
	v, err := composition.ComposeWithNetwork(agentic.Plan{Env: []string{"X_ORDER=plan"}},
		[]string{"X_ORDER=owned"}, f,
		composition.PromptApplication{Env: map[string]string{"X_ORDER": "prompt"}}, nil, patch)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.EnvLiterals["X_ORDER"]; got != "patch" {
		t.Fatalf("literals[X_ORDER] = %q, want patch (patch applied last)", got)
	}
	if !slicesContains(v.Env, "X_ORDER=patch") {
		t.Fatalf("env = %q, want the patch value last", v.Env)
	}
}

// TestComposeWithNetworkRechecksDisjointness: a patch literal wins over
// a same-named lookup with the literal-versus-lookup warning. The
// production generic patch cannot collide (its set names are
// proxy-family and refused in env_names), so this drives the mechanism
// with a synthetic patch.
func TestComposeWithNetworkRechecksDisjointness(t *testing.T) {
	f := parsed(t, "opencode", "/managed/default/tool/mcp.json", []string{"FIGMA_API_KEY", "X_LOOKUP"})
	patch := envpatch.Patch{Set: []envpatch.Pair{{Name: "X_LOOKUP", Value: "lit"}}}
	v, err := composition.ComposeWithNetwork(agentic.Plan{}, nil, f, composition.PromptApplication{}, nil, patch)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, v.EnvNames, []string{"FIGMA_API_KEY"})
	equal(t, v.EnvLiterals["X_LOOKUP"], "lit")
	if !slicesContains(v.Warnings, "environment literal replaces lookup: X_LOOKUP") {
		t.Fatalf("warnings = %q, want the literal-versus-lookup warning", v.Warnings)
	}
}

// TestComposeWithNetworkUnsetOnlyPatchIsManaged: an unset-only patch is
// managed, not empty (v0.2.0 kind=direct forward-compat: Empty() must
// stay false for it). The unset half clears both env and literals in
// every letter case, siblings survive, and proxy overlays still refuse.
func TestComposeWithNetworkUnsetOnlyPatchIsManaged(t *testing.T) {
	unsetOnly := envpatch.Patch{Unset: []string{"HTTP_PROXY", "NO_PROXY"}}
	if unsetOnly.Empty() {
		t.Fatal("unset-only patch reports Empty: the managed gate would treat kind=direct as unmanaged")
	}
	f := parsed(t, "codex_cli", "/managed/default/codex/layer.toml", []string{"FIGMA_API_KEY"})
	v, err := composition.ComposeWithNetwork(agentic.Plan{Env: []string{
		"HTTP_PROXY=http://ambient:8080", "Http_Proxy=http://mixed:8080",
		"NO_PROXY=*", "SIBLING=kept",
	}}, nil, f, composition.PromptApplication{}, nil, unsetOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range v.Env {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "HTTP_PROXY") || strings.EqualFold(name, "NO_PROXY") {
			t.Fatalf("unset name %q survives in env", entry)
		}
	}
	for name := range v.EnvLiterals {
		if strings.EqualFold(name, "HTTP_PROXY") || strings.EqualFold(name, "NO_PROXY") {
			t.Fatalf("unset name %q survives in literals", name)
		}
	}
	if !slicesContains(v.Env, "SIBLING=kept") {
		t.Fatalf("sibling disturbed: %q", v.Env)
	}
	equal(t, v.EnvNames, []string{"FIGMA_API_KEY"})
	// Proxy overlays refuse while managed by an unset-only patch too.
	f.Env["HTTP_PROXY"] = "http://frag:8080"
	if _, err := composition.ComposeWithNetwork(agentic.Plan{}, nil, f, composition.PromptApplication{}, nil, unsetOnly); err == nil {
		t.Fatal("unset-only managed composition admitted a fragment proxy overlay")
	} else if r, ok := refusal.As(err); !ok || r == nil || r.Code != refusal.CodeConfigurationConflict {
		t.Fatalf("error = %v, want %s", err, refusal.CodeConfigurationConflict)
	}
}

// TestComposeWithNetworkMatchesPatchApply: the composed env equals the
// library's Patch.Apply over the same input — mixed-case ambient names,
// an ambient NO_PROXY=*, and duplicate set names (last wins in the map).
// Application delegates to Apply, so this pins the delegation against
// re-implementation drift.
func TestComposeWithNetworkMatchesPatchApply(t *testing.T) {
	patch := envpatch.Patch{
		Unset: []string{"HTTP_PROXY", "NO_PROXY"},
		Set: []envpatch.Pair{
			{Name: "X_DUP", Value: "first"},
			{Name: "X_DUP", Value: "second"},
			{Name: "NO_PROXY", Value: "lit"},
		},
	}
	planEnv := []string{
		"PATH=/sanitized", "Http_Proxy=http://odd:1", "NO_PROXY=*",
		"X_DUP=old", "SIBLING=kept",
	}
	f := parsed(t, "opencode", "/managed/default/tool/mcp.json", nil)
	v, err := composition.ComposeWithNetwork(agentic.Plan{Env: planEnv},
		nil, f, composition.PromptApplication{}, nil, patch)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{}
	for _, entry := range patch.Apply(planEnv) {
		name, value, _ := strings.Cut(entry, "=")
		want[name] = value
	}
	// The fragment contributes its closed home variable; drop it before
	// the comparison so both sides cover the same names.
	delete(want, fragment.HomeVariable("opencode"))
	got := map[string]string{}
	for _, entry := range v.Env {
		name, value, _ := strings.Cut(entry, "=")
		got[name] = value
	}
	delete(got, fragment.HomeVariable("opencode"))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("composed env = %v, want Patch.Apply %v", got, want)
	}
	if v.EnvLiterals["X_DUP"] != "second" || v.EnvLiterals["NO_PROXY"] != "lit" {
		t.Fatalf("literals = %v, want duplicate last-wins and the set half", v.EnvLiterals)
	}
	if !slicesContains(v.Env, "X_DUP=second") || slicesContains(v.Env, "X_DUP=first") {
		t.Fatalf("duplicate set names mishandled: %q", v.Env)
	}
}

// TestComposeWithNetworkUnmanagedIdentical: the empty patch composes
// byte-identically to Compose, so existing behavior is untouched.
func TestComposeWithNetworkUnmanagedIdentical(t *testing.T) {
	f := parsed(t, "opencode", "/managed/default/tool/mcp.json", []string{"CHANNEL", "FIGMA_API_KEY", "OWN"})
	p := agentic.Plan{System: "s", Binary: "/bin", WorkDir: "/work",
		Argv: []string{"--model", "m"}, Env: []string{"PATH=/p", "OWN=old", "EMPTY="},
		Stdin: agentic.StdinPayload{Attached: true, Bytes: []byte("in")}}
	owned := []string{"OWN=owned-value", "XDG_CONFIG_HOME=owned-home", "CHANNEL=owned-channel"}
	prompt := composition.PromptApplication{Argv: []string{"--prompt", "p"}, Env: map[string]string{"CHANNEL": "channel-value"}}
	native := []string{"resume", "--last"}
	a, err := composition.Compose(p, owned, f, prompt, native)
	if err != nil {
		t.Fatal(err)
	}
	b, err := composition.ComposeWithNetwork(p, owned, f, prompt, native, envpatch.Unmanaged())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("unmanaged ComposeWithNetwork differs:\nCompose=%#v\nWithNetwork=%#v", a, b)
	}
}

// TestComposeAdmittedPlanWithNetworkSuffix: the admitted-plan form keeps
// the native-suffix contract and carries the patch.
func TestComposeAdmittedPlanWithNetworkSuffix(t *testing.T) {
	f := parsed(t, "claude_code", "/managed/default/claude/mcp.json", nil)
	native := []string{"resume", "--last"}
	plan := agentic.Plan{Argv: []string{"--model", "m", "resume", "--last"}, Env: []string{"HTTP_PROXY=http://ambient:8080"}}
	v, err := composition.ComposeAdmittedPlanWithNetwork(plan, nil, f,
		composition.PromptApplication{Argv: []string{"--prompt", "p"}}, native, genericPatch())
	if err != nil {
		t.Fatal(err)
	}
	equal(t, v.Argv, []string{"--model", "m", "--prompt", "p", "resume", "--last"})
	if !slicesContains(v.Env, "HTTP_PROXY=http://127.0.0.1:18080") {
		t.Fatalf("patch not applied: %q", v.Env)
	}
	_, err = composition.ComposeAdmittedPlanWithNetwork(agentic.Plan{Argv: []string{"--model", "m", "resume", "different"}},
		nil, f, composition.PromptApplication{}, native, genericPatch())
	if err == nil || !strings.Contains(err.Error(), "requested native argument suffix") {
		t.Fatalf("suffix drift error = %v", err)
	}
}
