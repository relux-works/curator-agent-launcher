package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
)

// Configuration loading keeps the existing launcher -> selected Core ->
// selector order and returns no partially loaded dependency set on refusal.
// All paths and selected projections here are synthetic, with no Core process.
func TestOrdinaryPolicyLoadOrder(t *testing.T) {
	for _, kind := range []string{"launcher-invalid", "selected-refused", "selector-invalid", "valid"} {
		t.Run(kind, func(t *testing.T) {
			deps := testDeps(t, fragment.NewWithRunner("curator", forbiddenRunner{t}))
			resolver := configfile.Resolver{Locations: configfile.Locations{
				System: filepath.Dir(deps.defaults.Machine), User: filepath.Dir(deps.defaults.Operator),
			}}
			if kind == "launcher-invalid" {
				if err := os.WriteFile(filepath.Join(resolver.Locations.System, "launcher.json"), []byte(`{"schema":"invalid"}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "selected-refused" || kind == "selector-invalid" {
				if err := os.WriteFile(filepath.Join(resolver.Locations.System, "selector.json"), []byte(`{"schema":"invalid"}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			deps.readSelectedConfig = func(context.Context) (launcherconfig.Selected, error) {
				calls++
				if kind == "selected-refused" {
					return launcherconfig.Selected{}, errors.New("private-fixture-detail")
				}
				return launcherconfig.Selected{ConfigSchema: 5, ConfigPath: "fixture-selected.json"}, nil
			}
			got, err := loadOrdinaryPolicy(context.Background(), resolver, deps)
			wantCalls := 1
			if kind == "launcher-invalid" {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Fatalf("selected Core calls=%d want %d", calls, wantCalls)
			}
			if kind == "valid" {
				if err != nil || got.selectedConfig.ConfigPath != "fixture-selected.json" || got.configResolver.Locations != resolver.Locations {
					t.Fatalf("valid policy load changed: err=%v selected=%+v", err, got.selectedConfig)
				}
				return
			}
			if err == nil || got.configResolver.Locations != (configfile.Locations{}) || got.selectedConfig.ConfigPath != "" || got.resolver != nil {
				t.Fatalf("refusal returned partial policy: err=%v selected=%+v", err, got.selectedConfig)
			}
			if kind == "selected-refused" && (err != launcherconfig.ErrSelected || strings.Contains(err.Error(), "private-fixture-detail")) {
				t.Fatalf("selected Core failure lost sanitization or precedence: %v", err)
			}
		})
	}
}

// Shared resolution preserves per-member precedence and the same permission
// files without resolving a profile or entering launch. These are isolated
// file/registry controls, not a protocol-child or native-admission witness.
func TestOrdinaryResolutionReusableInputs(t *testing.T) {
	deps := testDeps(t, fragment.NewWithRunner("curator", forbiddenRunner{t}))
	machine := `{"schema":"curator-run-defaults-v3","defaults":{"codex_cli":{"model":"gpt-6-astra","effort":"low","permissions":"native"}}}`
	operator := `{"schema":"curator-run-defaults-v3","defaults":{"codex_cli":{"model":"gpt-5.6-sol"}}}`
	for path, body := range map[string]string{deps.defaults.Machine: machine, deps.defaults.Operator: operator} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	frag, err := fragment.Parse([]byte(fragmentLineFor("codex_cli")))
	if err != nil {
		t.Fatal(err)
	}
	got, failure := resolveOrdinary(context.Background(), cli.Invocation{EnvID: "codex_cli"}, frag, configfile.Resolver{}, deps)
	if failure != nil {
		t.Fatal(failure.err)
	}
	if got.resolved.Model.Value != "gpt-5.6-sol" || got.resolved.Effort.Value != "low" || got.resolved.Runtime != "codex" {
		t.Fatalf("ordinary precedence changed: %+v", got.resolved)
	}
	permission, err := got.files.PermissionDefault("codex_cli")
	if err != nil || !permission.Present || permission.Value != "native" {
		t.Fatalf("permission input changed: %+v %v", permission, err)
	}
	if got.deps.defaults != deps.defaults {
		t.Fatal("configuration binding changed")
	}
}

// Invalid configuration is terminal before even preparing a selector request;
// discovery failures and parsed-file refusals retain their frontend diagnostic
// distinction. Fixing the same isolated file restores valid resolution.
func TestOrdinaryResolutionConfigurationRefusals(t *testing.T) {
	for _, kind := range []string{"discovery", "invalid-operator"} {
		t.Run(kind, func(t *testing.T) {
			deps := testDeps(t, fragment.NewWithRunner("curator", forbiddenRunner{t}))
			frag, err := fragment.Parse([]byte(fragmentLineFor("codex_cli")))
			if err != nil {
				t.Fatal(err)
			}
			deps.selectorConfig.Mode = "enforce"
			deps.workdir = func() (string, error) {
				t.Fatal("invalid configuration reached selector request construction")
				return "", nil
			}
			wantRaw := kind == "invalid-operator"
			if kind == "discovery" {
				deps.configPaths = func() (defaults.Paths, error) { return defaults.Paths{}, errors.New("fixture path discovery failed") }
			} else if err := os.WriteFile(deps.defaults.Operator, []byte(`{"schema":"unknown-fixture"}`), 0600); err != nil {
				t.Fatal(err)
			}
			got, failure := resolveOrdinary(context.Background(), cli.Invocation{EnvID: "codex_cli"}, frag, configfile.Resolver{}, deps)
			if got != nil || failure == nil || failure.code != defaults.CodeInvalid || failure.rawDetail != wantRaw {
				t.Fatalf("configuration refusal changed: got=%+v failure=%+v", got, failure)
			}
			deps.configPaths = nil
			deps.selectorConfig.Mode = ""
			body := `{"schema":"curator-run-defaults-v3","defaults":{"codex_cli":{"model":"gpt-6-astra","effort":"low"}}}`
			if err := os.WriteFile(deps.defaults.Operator, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			got, failure = resolveOrdinary(context.Background(), cli.Invocation{EnvID: "codex_cli"}, frag, configfile.Resolver{}, deps)
			if failure != nil || got == nil || got.resolved.Model.Value != "gpt-6-astra" {
				t.Fatalf("repaired fixture refused: got=%+v failure=%+v", got, failure)
			}
		})
	}
}

// The same selector-request failure is terminal under enforce and advisory
// under shadow. Neither path starts a selector or a runtime in this fixture.
func TestOrdinaryResolutionSelectorRequestFailure(t *testing.T) {
	for _, mode := range []string{"enforce", "shadow"} {
		t.Run(mode, func(t *testing.T) {
			deps := testDeps(t, fragment.NewWithRunner("curator", forbiddenRunner{t}))
			body := `{"schema":"curator-run-defaults-v3","defaults":{"codex_cli":{"model":"gpt-6-astra","effort":"low"}}}`
			if err := os.WriteFile(deps.defaults.Operator, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			frag, err := fragment.Parse([]byte(fragmentLineFor("codex_cli")))
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			deps.workdir = func() (string, error) { calls++; return "", errors.New("fixture workspace unavailable") }
			deps.selectorConfig.Mode = mode
			got, failure := resolveOrdinary(context.Background(), cli.Invocation{EnvID: "codex_cli"}, frag, configfile.Resolver{}, deps)
			if calls != 1 {
				t.Fatalf("workspace lookups=%d want 1", calls)
			}
			if mode == "enforce" {
				if got != nil || failure == nil || failure.code != diagnostics.CodeSelectorUnavailable || failure.err.Error() != "selector unavailable reason=invalid" || failure.rawDetail {
					t.Fatalf("enforce did not preserve exact refusal: got=%+v failure=%+v", got, failure)
				}
			} else if failure != nil || got == nil || got.deps.selectorRequest != nil || got.resolved.Model.Value != "gpt-6-astra" {
				t.Fatalf("shadow request failure changed ordinary resolution: got=%+v failure=%+v", got, failure)
			}
		})
	}
}

// A locked explicit flag, including an identical value, refuses before selector
// request construction. The nearby no-flag resolution is a valid positive.
func TestOrdinaryResolutionLockedFlagBeforeSelector(t *testing.T) {
	deps := testDeps(t, fragment.NewWithRunner("curator", forbiddenRunner{t}))
	body := `{"schema":"curator-run-defaults-v1","locked":true,"defaults":{"codex_cli":{"model":"gpt-6-astra","effort":"low"}}}`
	if err := os.WriteFile(deps.defaults.Machine, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	frag, err := fragment.Parse([]byte(fragmentLineFor("codex_cli")))
	if err != nil {
		t.Fatal(err)
	}
	inv := cli.Invocation{EnvID: "codex_cli", ModelSet: true, Model: "gpt-6-astra"}
	deps.selectorConfig.Mode = "enforce"
	calls := 0
	deps.workdir = func() (string, error) { calls++; t.Fatal("selector request ran before lock refusal"); return "", nil }
	got, failure := resolveOrdinary(context.Background(), inv, frag, configfile.Resolver{}, deps)
	if failure == nil || failure.code != defaults.CodeUsage || !strings.Contains(failure.err.Error(), "overrides locked model") || got != nil || calls != 0 {
		t.Fatalf("lock refusal changed: got=%+v failure=%+v calls=%d", got, failure, calls)
	}
	deps.selectorConfig.Mode = ""
	inv.ModelSet = false
	got, failure = resolveOrdinary(context.Background(), inv, frag, configfile.Resolver{}, deps)
	if failure != nil || got == nil || got.resolved.Model.Value != "gpt-6-astra" {
		t.Fatalf("no-flag positive refused: %+v %+v", got, failure)
	}
}
