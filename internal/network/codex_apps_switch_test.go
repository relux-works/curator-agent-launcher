package network

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/relux-works/curator-network-profiles/pkg/binding"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

func requireAppsRefusal(t *testing.T, name string, err error) {
	t.Helper()
	var refusal *agentic.CodexGatewayRefusalError
	if !errors.Is(err, agentic.ErrCodexGatewayRefused) || !errors.As(err, &refusal) || refusal.ReasonCode() != agentic.CodexGatewayBindingNotCanonical {
		t.Fatalf("%s: got %v, want the launcher's refusal with reason %s", name, err, agentic.CodexGatewayBindingNotCanonical)
	}
}

const codexTestThread = "0190f0c0-0000-7000-8000-000000000000"

// codexOwnedArgs are the six pairs a gateway launch owns, as the pinned
// module renders them: the provider table, the provider selection and the
// four switches.
var codexOwnedArgs = []string{
	"-c", `model_providers.curator-gateway={name="Curator filtering gateway",base_url="http://127.0.0.1:47811/chatgpt/backend-api/codex",wire_api="responses",requires_openai_auth=true,supports_websockets=false,env_http_headers={"X-Gateway-Token"="CURATOR_CODEX_GATEWAY_TOKEN"}}`,
	"-c", `model_provider="curator-gateway"`,
	"-c", "features.apps=false",
	"-c", "features.plugins=false",
	"-c", "analytics.enabled=false",
	"-c", "check_for_update_on_startup=false",
}

// codexSwitches are the four owned switches with the index of each `-c` in
// codexOwnedArgs.
var codexSwitches = map[string]int{"apps": 4, "plugins": 6, "analytics": 8, "update-check": 10}

// codexOptionalArgs are the forms that may follow, in the module's order.
var codexOptionalArgs = [][]string{
	{"-c", `model_instructions_file="/managed/prompt.md"`},
	{"-p", "curator-mcp"},
	{"-m", "gpt-6-astra"},
	{"-c", `model_reasoning_effort="medium"`},
	{"--dangerously-bypass-approvals-and-sandbox"},
	{"--no-alt-screen"},
	{"resume", codexTestThread},
}

// codexArgs builds admitted arguments: the owned pairs and the optional
// forms whose bit is set, in order.
func codexArgs(mask int) []string {
	argv := slices.Clone(codexOwnedArgs)
	for i, form := range codexOptionalArgs {
		if mask&(1<<i) != 0 {
			argv = append(argv, form...)
		}
	}
	return argv
}

// codexNeverAdmitted are argument groups that are no rendered form at any
// position. The first block is what the client reads as a feature selection
// (openai/codex rust-v0.167.0 with clap 4.5.58): Clap strips `=` after a short
// option and accepts an attached value, the client trims the key of every
// Unicode space, `use_legacy_landlock` and the legacy toggles are feature keys
// outside the `features` table, `connectors` is the legacy name of Apps. The
// second block is the same for the other owned settings: plugins, analytics,
// the exporters under `otel`, which `analytics.enabled` does not name, and
// the update check. The third block is forms the module can render for
// request fields this launcher never sets, and other spellings of the forms
// it does render.
var codexNeverAdmitted = map[string][]string{
	"short-equals-on":       {"-c=features.apps=true"},
	"short-equals-off":      {"-c=features.apps=false"},
	"short-attached-on":     {"-cfeatures.apps=true"},
	"short-attached-off":    {"-cfeatures.apps=false"},
	"pair-on":               {"-c", "features.apps=true"},
	"key-newline":           {"-c", "\nfeatures.apps=true"},
	"key-space":             {"-c", " features.apps=true"},
	"key-tab":               {"-c", "\tfeatures.apps=true"},
	"key-carriage-return":   {"-c", "\rfeatures.apps=true"},
	"key-no-break-space":    {"-c", "\u00a0features.apps=true"},
	"key-line-separator":    {"-c", "\u2028features.apps=true"},
	"key-trailing-space":    {"-c", "features.apps =true"},
	"key-newline-off":       {"-c", "\nfeatures.apps=false"},
	"landlock-alias":        {"-c", "use_legacy_landlock=true"},
	"landlock-alias-equals": {"-c=use_legacy_landlock=true"},
	"legacy-toggle":         {"-c", "experimental_use_unified_exec_tool=true"},
	"legacy-apps-name":      {"-c", "features.connectors=true"},
	"whole-table":           {"-c", "features={apps=true}"},
	"quoted-table":          {"-c", `"features".apps=true`},
	"quoted-name":           {"-c", `features."apps"=true`},
	"other-feature":         {"-c", `features.sleep_tool.mode="on"`},
	"config-option":         {"--config", "features.apps=true"},
	"config-joined":         {"--config=features.apps=true"},
	"enable-flag":           {"--enable", "apps"},
	"enable-joined":         {"--enable=apps"},
	"disable-flag":          {"--disable", "apps"},
	"disable-joined":        {"--disable=apps"},

	"plugins-on":              {"-c", "features.plugins=true"},
	"plugins-equals-on":       {"-c=features.plugins=true"},
	"plugins-equals-off":      {"-c=features.plugins=false"},
	"plugins-prefetch":        {"-c", "features.plugin_discovery_prefetch=true"},
	"plugins-enable":          {"--enable", "plugins"},
	"plugins-disable-joined":  {"--disable=plugins"},
	"analytics-on":            {"-c", "analytics.enabled=true"},
	"analytics-equals-on":     {"-c=analytics.enabled=true"},
	"analytics-equals-off":    {"-c=analytics.enabled=false"},
	"analytics-attached-on":   {"-canalytics.enabled=true"},
	"analytics-key-space":     {"-c", " analytics.enabled=true"},
	"analytics-key-newline":   {"-c", "\nanalytics.enabled=false"},
	"analytics-trailing":      {"-c", "analytics.enabled =true"},
	"analytics-whole-table":   {"-c", "analytics={enabled=true}"},
	"analytics-quoted-table":  {"-c", `"analytics".enabled=true`},
	"analytics-quoted-name":   {"-c", `analytics."enabled"=true`},
	"analytics-config-option": {"--config", "analytics.enabled=true"},
	"analytics-config-joined": {"--config=analytics.enabled=true"},
	"otel-metrics-exporter":   {"-c", `otel.metrics_exporter="statsig"`},
	"otel-exporter-equals":    {"-c=otel.exporter=\"otlp-http\""},
	"otel-whole-table":        {"-c", `otel={trace_exporter="otlp-http"}`},
	"otel-config-joined":      {"--config=otel.metrics_exporter=\"statsig\""},
	"update-check-on":         {"-c", "check_for_update_on_startup=true"},
	"update-check-equals-on":  {"-c=check_for_update_on_startup=true"},
	"update-check-equals-off": {"-c=check_for_update_on_startup=false"},
	"update-check-attached":   {"-ccheck_for_update_on_startup=true"},
	"update-check-key-space":  {"-c", " check_for_update_on_startup=true"},
	"update-check-quoted":     {"-c", `"check_for_update_on_startup"=true`},
	"update-check-config":     {"--config", "check_for_update_on_startup=true"},
	"update-check-joined":     {"--config=check_for_update_on_startup=true"},

	"developer-instructions": {"-c", `developer_instructions="text"`},
	"mcp-server":             {"-c", `mcp_servers.tool.command="tool"`},
	"writable-roots":         {"-c", `sandbox_workspace_write.writable_roots=["/work"]`},
	"other-profile":          {"-p", "other"},
	"profile-long":           {"--profile", "curator-mcp"},
	"profile-attached":       {"-pcurator-mcp"},
	"model-long":             {"--model", "gpt-6-astra"},
	"model-attached":         {"-mgpt-6-astra"},
	"model-flag-value":       {"-m", "--enable=apps"},
	"unquoted-effort":        {"-c", "model_reasoning_effort=medium"},
	"unquoted-prompt-file":   {"-c", "model_instructions_file=/managed/prompt.md"},
	"other-provider":         {"-c", `model_provider="other"`},
	"other-provider-table":   {"-c", `model_providers.other={base_url="http://127.0.0.1:1"}`},
	"separator":              {"--"},
	"yolo-alias":             {"--yolo"},
	"resume-picker":          {"resume", "--last"},
	"resume-name":            {"resume", "thread"},
	"other-command":          {"exec"},
	"empty":                  {""},
}

// Claim: a Codex gateway plan that the launcher prepared carries the four
// switches as its third to sixth owned pairs, and the launcher itself refuses
// to bind a process whose final arguments are anything but the rendered
// forms: any one switch missing, switched on, repeated, moved or spelled
// another way, its key in upper case or the token with a character before
// the key, after the key or after the value, or an owned setting selected in
// any other spelling at any position. The refusal is the launcher's own: it
// carries a reason, where the module's argv comparison refuses without one,
// so a launcher that only forwarded to the module would fail here. A refused
// attempt binds nothing; the true arguments still bind afterwards, which is
// each refused row's admitted neighbour. Without a gateway the launcher asks
// for nothing. The fixture is a real module policy basis and a synthetic
// control service; no runtime is started, so this does not show that a Codex
// release honours the switches.
func TestCodexGatewayRequiresTheSwitches(t *testing.T) {
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
	owned := len(codexOwnedArgs)
	if len(argv) < owned || !slices.Equal(argv[4:owned], codexOwnedArgs[4:]) {
		t.Fatalf("pinned module does not render the four switches after the provider selection: %q", argv[:min(len(argv), owned)])
	}
	env := slices.Clone(final.Plan.Env)
	// Unbound, nothing is executed: the process verifier refuses even the true arguments.
	if child := append(slices.Clone(env), codexTokenEnv+"="+codexSyntheticToken); p.VerifyProcess(req.StagedExecutable, argv, child) == nil {
		t.Fatal("an unbound process passed the final verification")
	}
	refused := func(name string, changed []string) {
		t.Helper()
		requireAppsRefusal(t, name, p.BindProcess(req.StagedExecutable, changed, env, nil, nil))
		if p.codex.bound {
			t.Fatalf("%s: a refused attempt bound the process", name)
		}
	}
	refused("nothing", nil)
	for which, at := range codexSwitches {
		off := argv[at+1]
		key, value, _ := strings.Cut(off, "=")
		next := 4 + (at-4+2)%(owned-4)
		for name, mutate := range map[string]func([]string) []string{
			"pair-removed": func(a []string) []string { return append(a[:at], a[at+2:]...) },
			"switched-on":  func(a []string) []string { a[at+1] = strings.TrimSuffix(off, "false") + "true"; return a },
			"second-off":   func(a []string) []string { return append(a, "-c", off) },
			"other-option": func(a []string) []string { a[at] = "--config"; return a },
			"short-equals": func(a []string) []string { a[at] = "-c=" + off; return append(a[:at+1], a[at+2:]...) },
			"swapped":      func(a []string) []string { a[at+1], a[next+1] = a[next+1], a[at+1]; return a },
			"moved-last":   func(a []string) []string { return append(append(a[:at], a[at+2:]...), "-c", off) },
			"upper-key":    func(a []string) []string { a[at+1] = strings.ToUpper(key) + "=" + value; return a },
			"key-prefix":   func(a []string) []string { a[at+1] = "x" + off; return a },
			"key-suffix":   func(a []string) []string { a[at+1] = key + "x=" + value; return a },
			"suffixed":     func(a []string) []string { a[at+1] = off + "x"; return a },
		} {
			// The last switch moved last in a list with nothing after it
			// is the list itself; that row is no change.
			if changed := mutate(slices.Clone(argv)); !slices.Equal(changed, argv) {
				refused(which+"-"+name, changed)
			}
		}
	}
	for name, group := range codexNeverAdmitted {
		for at := 0; at <= len(argv); at++ {
			refused(name, slices.Insert(slices.Clone(argv), at, group...))
		}
	}
	if err := p.BindProcess(req.StagedExecutable, argv, env, nil, nil); err != nil {
		t.Fatalf("the prepared arguments no longer bind after refused attempts: %v", err)
	}
	for _, other := range [][]string{nil, {"-m", "model"}, {"-c", "features.apps=true"}, {"--enable", "apps"}, {"-c=features.apps=true"}, {"-c", "analytics.enabled=true"}, {"-c", "check_for_update_on_startup=true"}} {
		if err := (Prepared{}).BindProcess("/usr/bin/codex", other, nil, nil, nil); err != nil {
			t.Fatalf("a launch without a gateway was asked for a switch: %v", err)
		}
	}
}

// Claim: the launcher's check admits what the pinned module renders. Each row
// is a launch request the launcher can build; the module plans it and the
// launcher binds its final arguments. The rows cover the model and effort
// pairs, the bypass flag and the closed native suffix. The vendor layer
// resolves the model name and requires an effort for this model, so a list
// without them is not built here, nor is a plan with a Curator system-prompt
// file or MCP channel: the pure table below and the command package's
// pipeline tests hold those forms.
func TestCodexGatewayBindsWhatTheModuleRenders(t *testing.T) {
	selected := []string{"-m", "gpt-6.1-sol", "-c", `model_reasoning_effort="high"`}
	with := func(tail ...string) []string { return append(slices.Clone(selected), tail...) }
	for name, tc := range map[string]struct {
		edit func(*vendorplugin.SpawnRequest)
		tail []string
	}{
		"model-and-effort": {nil, selected},
		"bypass": {func(r *vendorplugin.SpawnRequest) { r.PermissionMode = agentic.PermissionModeYolo },
			with("--dangerously-bypass-approvals-and-sandbox")},
		"no-alt-screen": {func(r *vendorplugin.SpawnRequest) { r.NativeArgs = []string{"--no-alt-screen"} }, with("--no-alt-screen")},
		"resume":        {func(r *vendorplugin.SpawnRequest) { r.NativeArgs = []string{"resume", codexTestThread} }, with("resume", codexTestThread)},
		"screen-and-resume": {func(r *vendorplugin.SpawnRequest) {
			r.NativeArgs = []string{"--no-alt-screen", "resume", codexTestThread}
		},
			with("--no-alt-screen", "resume", codexTestThread)},
		"bypass-and-suffix": {func(r *vendorplugin.SpawnRequest) {
			r.PermissionMode, r.NativeArgs = agentic.PermissionModeYolo, []string{"--no-alt-screen", "resume", codexTestThread}
		}, with("--dangerously-bypass-approvals-and-sandbox", "--no-alt-screen", "resume", codexTestThread)},
	} {
		t.Run(name, func(t *testing.T) {
			req, result, _ := codexFixtureFor(t, tc.edit)
			p, err := prepareCodexGateway(context.Background(), req, result, binding.AdapterIdentity{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Lease.Finish(context.Background())
			final, _ := p.CodexPlan()
			argv := final.Plan.Argv
			owned := len(codexOwnedArgs)
			if len(argv) < owned || !slices.Equal(argv[owned:], tc.tail) {
				t.Fatalf("the module rendered %q after its owned pairs, the row expects %q", argv[min(len(argv), owned):], tc.tail)
			}
			if !strings.HasPrefix(argv[1], "model_providers.curator-gateway={") || !slices.Equal(argv[2:owned], codexOwnedArgs[2:]) {
				t.Fatalf("owned pairs: %q", argv[:owned])
			}
			if err := p.BindProcess(req.StagedExecutable, argv, slices.Clone(final.Plan.Env), nil, nil); err != nil {
				t.Fatalf("the launcher refused what the module rendered: %v", err)
			}
		})
	}
}

// Claim: codexSwitchesOff admits exactly the argument lists made of the six
// owned pairs followed by any subset of the seven optional forms in the
// module's order (128 lists), with free values after each pair's opening
// character. It refuses every list in which a group that is no rendered form
// stands at any position of an admitted list, and every list that repeats,
// reorders, alters or drops a form; for each of the four switches that is
// the pair removed, switched on, spaced, with the value in upper case, with a
// trailing space, under the long option, repeated, swapped with another
// switch and moved behind the optional forms, and the token with its key in
// upper case, with a character before the key, after the key or after the
// value. The last four keep a matcher that folds case, or that looks for the
// token or the key inside, at the start or at the end of an argument, from
// admitting a token that is not the rendered one. Each refused list differs
// by one change from an admitted list of the 128; the bare and the full list,
// the two each switch row is changed from, are asserted as admitted beside
// the rows. The table is written from the module's renderer, not rendered by
// it; the test above ties the two together.
func TestCodexSwitchesOff(t *testing.T) {
	owned := len(codexOwnedArgs)
	for mask := 0; mask < 1<<len(codexOptionalArgs); mask++ {
		if argv := codexArgs(mask); !codexSwitchesOff(argv) {
			t.Errorf("refused a rendered list: %q", argv[owned:])
		}
	}
	all, bare := codexArgs(1<<len(codexOptionalArgs)-1), codexArgs(0)
	for _, argv := range [][]string{
		append(slices.Clone(bare), "-c", `model_instructions_file="/managed/features.apps=true"`, "-m", "features.apps=true"),
		append(slices.Clone(bare), "-c", `model_instructions_file="/managed/analytics.enabled=true"`, "-m", "analytics.enabled=true"),
		append(slices.Clone(bare), "-c", "model_reasoning_effort=\"a\nfeatures.apps=true\""),
		append(slices.Clone(bare), "-c", "model_reasoning_effort=\"a\ncheck_for_update_on_startup=true\""),
		append(slices.Clone(bare), "-m", "model=with=equals"),
		append([]string{"-c", `model_providers.curator-gateway={name="-c features.apps=true"}`}, bare[2:]...),
	} {
		if !codexSwitchesOff(argv) {
			t.Errorf("refused a rendered form over its value: %q", argv[owned:])
		}
	}
	for name, group := range codexNeverAdmitted {
		for _, base := range [][]string{bare, all, codexArgs(0b0001111)} {
			for at := 0; at <= len(base); at++ {
				if argv := slices.Insert(slices.Clone(base), at, group...); codexSwitchesOff(argv) {
					t.Errorf("%s at %d admitted: %q", name, at, argv)
				}
			}
		}
	}
	edit := func(base []string, change func([]string) []string) []string { return change(slices.Clone(base)) }
	rows := map[string][]string{}
	for base, list := range map[string][]string{"bare": bare, "all": all} {
		if !codexSwitchesOff(list) {
			t.Fatalf("refused the %s list, the admitted list every switch row is changed from: %q", base, list)
		}
	}
	for which, at := range codexSwitches {
		off := all[at+1]
		key, value, _ := strings.Cut(off, "=")
		next := 4 + (at-4+2)%(owned-4)
		for name, change := range map[string]func([]string) []string{
			"removed":     func(a []string) []string { return append(a[:at], a[at+2:]...) },
			"on":          func(a []string) []string { a[at+1] = strings.TrimSuffix(off, "false") + "true"; return a },
			"spaced":      func(a []string) []string { a[at+1] = strings.Replace(off, "=", " = ", 1); return a },
			"upper":       func(a []string) []string { a[at+1] = strings.TrimSuffix(off, "false") + "FALSE"; return a },
			"trailing":    func(a []string) []string { a[at+1] = off + " "; return a },
			"long-option": func(a []string) []string { a[at] = "--config"; return a },
			"twice":       func(a []string) []string { return slices.Insert(a, at, "-c", off) },
			"twice-last":  func(a []string) []string { return append(a, "-c", off) },
			"swapped":     func(a []string) []string { a[at+1], a[next+1] = a[next+1], a[at+1]; return a },
			"moved-last":  func(a []string) []string { return append(append(a[:at], a[at+2:]...), "-c", off) },
			"moved-first": func(a []string) []string { return append([]string{"-c", off}, append(a[:at], a[at+2:]...)...) },
			"upper-key":   func(a []string) []string { a[at+1] = strings.ToUpper(key) + "=" + value; return a },
			"key-prefix":  func(a []string) []string { a[at+1] = "x" + off; return a },
			"key-suffix":  func(a []string) []string { a[at+1] = key + "x=" + value; return a },
			"suffixed":    func(a []string) []string { a[at+1] = off + "x"; return a },
		} {
			for base, list := range map[string][]string{"bare": bare, "all": all} {
				// The last switch moved last in the bare list is the
				// bare list itself; that row is no change.
				if changed := change(slices.Clone(list)); !slices.Equal(changed, list) {
					rows[which+"-"+name+"-"+base] = changed
				}
			}
		}
	}
	if len(rows) != 4*15*2-1 {
		t.Fatalf("the switch rows are %d, want every change of every switch on both lists but the one that changes nothing", len(rows))
	}
	for name, argv := range rows {
		if codexSwitchesOff(argv) {
			t.Errorf("%s admitted: %q", name, argv)
		}
	}
	for name, argv := range map[string][]string{
		"nothing":               nil,
		"no-owned-pairs":        {"-m", "gpt-6-astra"},
		"only-the-switches":     slices.Clone(codexOwnedArgs[4:]),
		"switches-first":        edit(bare, func(a []string) []string { return append(a[4:owned:owned], a[:4]...) }),
		"provider-table-gone":   edit(all, func(a []string) []string { return a[2:] }),
		"provider-choice-gone":  edit(all, func(a []string) []string { return append(a[:2], a[4:]...) }),
		"provider-table-string": edit(all, func(a []string) []string { a[1] = `model_providers.curator-gateway="x"`; return a }),
		"provider-other-id":     edit(all, func(a []string) []string { a[1] = "model_providers.curator-gateway2={}"; return a }),
		"provider-choice-other": edit(all, func(a []string) []string { a[3] = `model_provider="curator-gateway" `; return a }),
		"prompt-file-twice": edit(bare, func(a []string) []string {
			return append(a, "-c", `model_instructions_file="/a"`, "-c", `model_instructions_file="/b"`)
		}),
		"profile-twice": edit(bare, func(a []string) []string { return append(a, "-p", "curator-mcp", "-p", "curator-mcp") }),
		"model-twice":   edit(bare, func(a []string) []string { return append(a, "-m", "one", "-m", "two") }),
		"effort-twice": edit(bare, func(a []string) []string {
			return append(a, "-c", `model_reasoning_effort="a"`, "-c", `model_reasoning_effort="b"`)
		}),
		"bypass-twice":          edit(bare, func(a []string) []string { return append(a, codexOptionalArgs[4][0], codexOptionalArgs[4][0]) }),
		"no-alt-screen-twice":   edit(bare, func(a []string) []string { return append(a, "--no-alt-screen", "--no-alt-screen") }),
		"profile-before-prompt": edit(bare, func(a []string) []string { return append(a, "-p", "curator-mcp", "-c", `model_instructions_file="/a"`) }),
		"model-before-profile":  edit(bare, func(a []string) []string { return append(a, "-m", "one", "-p", "curator-mcp") }),
		"effort-before-model":   edit(bare, func(a []string) []string { return append(a, "-c", `model_reasoning_effort="a"`, "-m", "one") }),
		"bypass-before-effort": edit(bare, func(a []string) []string {
			return append(a, codexOptionalArgs[4][0], "-c", `model_reasoning_effort="a"`)
		}),
		"suffix-before-bypass": edit(bare, func(a []string) []string { return append(a, "--no-alt-screen", codexOptionalArgs[4][0]) }),
		"resume-before-screen": edit(bare, func(a []string) []string { return append(a, "resume", codexTestThread, "--no-alt-screen") }),
		"option-before-owned":  edit(bare, func(a []string) []string { return append([]string{"-m", "one"}, a...) }),
		"model-missing":        edit(bare, func(a []string) []string { return append(a, "-m") }),
		"model-empty":          edit(bare, func(a []string) []string { return append(a, "-m", "") }),
		"model-is-an-option":   edit(bare, func(a []string) []string { return append(a, "-m", "-c") }),
		"pair-missing-value":   edit(bare, func(a []string) []string { return append(a, "-c") }),
		"resume-missing-id":    edit(bare, func(a []string) []string { return append(a, "resume") }),
		"resume-short-id":      edit(bare, func(a []string) []string { return append(a, "resume", codexTestThread[:35]) }),
		"resume-then-more":     edit(bare, func(a []string) []string { return append(a, "resume", codexTestThread, "-c=features.apps=true") }),
		"resume-twice":         edit(bare, func(a []string) []string { return append(a, "resume", codexTestThread, "resume", codexTestThread) }),
	} {
		if codexSwitchesOff(argv) {
			t.Errorf("%s admitted: %q", name, argv)
		}
	}
}
