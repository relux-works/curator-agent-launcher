package network

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/relux-works/curator-network-profiles/pkg/binding"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

func requireAppsRefusal(t *testing.T, name string, err error) {
	t.Helper()
	var refusal *agentic.CodexGatewayRefusalError
	if !errors.Is(err, agentic.ErrCodexGatewayRefused) || !errors.As(err, &refusal) || refusal.ReasonCode() != agentic.CodexGatewayBindingNotCanonical {
		t.Fatalf("%s: got %v, want the launcher's refusal with reason %s", name, err, agentic.CodexGatewayBindingNotCanonical)
	}
}

// Claim: a Codex gateway plan that the launcher prepared carries the Apps
// switch as its third owned pair, and the launcher itself refuses to bind a
// process whose final arguments lack it, carry it switched on, override it
// later or select a feature another way. The refusal is the launcher's own:
// it carries a reason, where the module's argv comparison refuses without
// one, so a launcher that only forwarded to the module would fail here. A
// refused attempt binds nothing; the true arguments still bind afterwards.
// Without a gateway the launcher asks for nothing. The fixture is a real
// module policy basis and a synthetic control service; no runtime is started,
// so this does not show that a Codex release honours the switch.
func TestCodexGatewayRequiresTheAppsSwitch(t *testing.T) {
	req, result, _ := codexFixture(t)
	p, err := prepareCodexGateway(context.Background(), req, result, binding.AdapterIdentity{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Lease.Finish(context.Background())
	final, ok := p.CodexPlan()
	if !ok {
		t.Fatal("missing module plan")
	}
	argv := final.Plan.Argv
	if len(argv) < 6 || !slices.Equal(argv[4:6], []string{"-c", "features.apps=false"}) {
		t.Fatalf("pinned module renders no Apps switch after the provider selection: %q", argv[:min(len(argv), 6)])
	}
	env := slices.Clone(final.Plan.Env)
	// Unbound, nothing is executed: the process verifier refuses even the true arguments.
	if child := append(slices.Clone(env), codexTokenEnv+"="+codexSyntheticToken); p.VerifyProcess(req.StagedExecutable, argv, child) == nil {
		t.Fatal("an unbound process passed the final verification")
	}
	for name, mutate := range map[string]func([]string) []string{
		"pair-removed":     func(a []string) []string { return append(a[:4], a[6:]...) },
		"switched-on":      func(a []string) []string { a[5] = "features.apps=true"; return a },
		"overridden-later": func(a []string) []string { return append(a, "-c", "features.apps=true") },
		"second-off":       func(a []string) []string { return append(a, "-c", "features.apps=false") },
		"enable-flag":      func(a []string) []string { return append(a, "--enable", "apps") },
		"enable-joined":    func(a []string) []string { return append(a, "--enable=apps") },
		"disable-flag":     func(a []string) []string { return append(a, "--disable", "apps") },
		"config-option":    func(a []string) []string { return append(a, "--config", "features.apps=true") },
		"config-joined":    func(a []string) []string { return append(a, "--config=features.apps=true") },
		"legacy-name":      func(a []string) []string { return append(a, "-c", "features.connectors=true") },
		"whole-table":      func(a []string) []string { return append(a, "-c", "features={apps=true}") },
		"quoted-key":       func(a []string) []string { return append(a, "-c", `"features".apps=true`) },
		"other-option":     func(a []string) []string { a[4] = "--config"; return a },
	} {
		requireAppsRefusal(t, name, p.BindProcess(req.StagedExecutable, mutate(slices.Clone(argv)), env, nil, nil))
		if p.codex.bound {
			t.Fatalf("%s: a refused attempt bound the process", name)
		}
	}
	if err := p.BindProcess(req.StagedExecutable, argv, env, nil, nil); err != nil {
		t.Fatalf("the prepared arguments no longer bind after refused attempts: %v", err)
	}
	for _, other := range [][]string{nil, {"-m", "model"}, {"-c", "features.apps=true"}, {"--enable", "apps"}} {
		if err := (Prepared{}).BindProcess("/usr/bin/codex", other, nil, nil, nil); err != nil {
			t.Fatalf("a launch without a gateway was asked for the Apps switch: %v", err)
		}
	}
}

// Claim: codexAppsOff admits arguments with exactly one feature selector,
// the pair `-c features.apps=false`, beside any other argument, and refuses
// every other way of selecting a feature and every argument list without the
// pair. Keys that only contain the word, values that mention it, a feature
// key under another table and a profile flag are not selectors.
func TestCodexAppsOff(t *testing.T) {
	pair := []string{"-c", "features.apps=false"}
	with := func(before, after []string) []string {
		return append(append(slices.Clone(before), pair...), after...)
	}
	for _, argv := range [][]string{
		pair,
		with([]string{"-c", `model_provider="curator-gateway"`}, []string{"-m", "model", "-c", `model_reasoning_effort="high"`}),
		with(nil, []string{"-p", "managed", "--no-alt-screen", "resume", "0190f0c0-0000-7000-8000-000000000000"}),
		with(nil, []string{"-c", `mcp_servers.features.command="x"`, "-c", "mcp_servers.apps.enabled=true"}),
		with(nil, []string{"-c", `developer_instructions="set features.apps=true"`, "-m", "features"}),
		with(nil, []string{"-c", "featuresx.apps=true", "-c", "feature.apps=true", "-c", "profiles.x.features.apps=true"}),
	} {
		if !codexAppsOff(argv) {
			t.Errorf("refused: %q", argv)
		}
	}
	for _, argv := range [][]string{
		nil, {"-m", "model"}, {"-c", "features.apps=true"}, {"-c", "features.apps = false"}, {"-c", "features.apps=FALSE"},
		{"--config", "features.apps=false"}, {"--config=features.apps=false"}, {"-cfeatures.apps=false"},
		{"-c", "features={apps=false}"}, {"-c", `"features".apps=false`}, {"-c", "features.connectors=false"},
		with(nil, pair), with(nil, []string{"-c", "features.apps=true"}), with([]string{"-c", "features.apps=true"}, nil),
		with(nil, []string{"-c", `features.sleep_tool.mode="on"`}), with(nil, []string{"--enable", "apps"}),
		with(nil, []string{"--enable=web_search"}), with(nil, []string{"--disable", "apps"}), with(nil, []string{"--disable=apps"}),
		with(nil, []string{"--config", "features.apps=true"}), with(nil, []string{"--config=features.connectors=true"}),
		with(nil, []string{"-cfeatures.apps=true"}), with(nil, []string{"-c", " features . apps = true"}),
		with(nil, []string{"-c", `'features'.apps=true`}), with(nil, []string{"-c"}), {"-c"},
	} {
		if codexAppsOff(argv) {
			t.Errorf("admitted: %q", argv)
		}
	}
}
