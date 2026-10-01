package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

func capturedLaunch(t *testing.T, out []byte, tracked bool) ([]string, map[string]string, []string) {
	t.Helper()
	var child childCapture
	if err := json.Unmarshal(out, &child); err != nil {
		t.Fatal(err)
	}
	if tracked {
		var doc struct {
			Argv  []string          `json:"argv_suffix"`
			Env   map[string]string `json:"env_literals"`
			Names []string          `json:"env_names"`
		}
		if err := json.Unmarshal(child.Stdin, &doc); err != nil {
			t.Fatal(err)
		}
		return doc.Argv, doc.Env, doc.Names
	}
	env := map[string]string{}
	for _, entry := range child.Env {
		k, v, _ := strings.Cut(entry, "=")
		env[k] = v
	}
	return child.Argv, env, nil
}

// Production call path for all carrier regressions: run -> launch -> plan.Build
// -> vendorplugin.BuildLaunchWithEnvironment -> composition -> execution.
func TestProductionContextCarrierProfileNativeSuffixAndChannels(t *testing.T) {
	for _, environment := range []string{"claude_code", "codex_cli"} {
		for _, tracked := range []bool{false, true} {
			for _, intent := range []string{"append", "replace"} {
				if environment == "codex_cli" && intent == "append" {
					continue
				}
				t.Run(fmt.Sprintf("%s/%v/%s", environment, tracked, intent), func(t *testing.T) {
					f := entryFixture(t, environment, tracked)
					f.args[6] = intent
					f.args = append(f.args[:7], "--profile", "team", "--", "", "--model=native", "literal\nvalue")
					var obj map[string]any
					if err := json.Unmarshal([]byte(f.resolver.stdout), &obj); err != nil {
						t.Fatal(err)
					}
					obj["profile"].(map[string]any)["name"] = "team"
					raw, _ := json.Marshal(obj)
					f.resolver.stdout = string(raw)
					code, out, stderr := f.run()
					if code != 0 {
						t.Fatalf("exit=%d stderr=%s", code, stderr)
					}
					c := f.request.Context
					if c == nil || c.Profile.Name != "team" || c.Profile.LockSHA256 != strings.Repeat("a", 64) || c.MCP.Path != f.layer || c.SystemPrompt.Path != f.prompt || string(c.SystemPrompt.Intent) != intent {
						t.Fatalf("carrier=%+v", c)
					}
					provenance, ok := f.plan.CuratorContextProvenanceSnapshot()
					if !ok || provenance.ProfileName != "team" || provenance.SystemPromptIntent != agentic.CuratorSystemPromptIntent(intent) {
						t.Fatalf("provenance=%+v ok=%v", provenance, ok)
					}
					argv, _, _ := capturedLaunch(t, out, tracked)
					native := []string{"", "--model=native", "literal\nvalue"}
					if len(argv) < len(native) || !reflect.DeepEqual(argv[len(argv)-len(native):], native) {
						t.Fatalf("native suffix=%q", argv)
					}
					if !reflect.DeepEqual(argv, f.plan.Argv) {
						t.Fatalf("launcher rebuilt context argv: %q vs %q", argv, f.plan.Argv)
					}
					if !reflect.DeepEqual(f.resolver.argv, []string{"env", "resolve", environment, "--profile", "team", "--repair", "--format", "json"}) {
						t.Fatalf("resolve=%q", f.resolver.argv)
					}
				})
			}
		}
	}
}

func TestProductionContextCarrierRefusesNativeConflicts(t *testing.T) {
	rows := []struct {
		environment, flag, channel string
		native                     []string
	}{
		{"claude_code", "--mcp-config", "mcp-servers", []string{"--mcp-config", "other.json"}},
		{"claude_code", "--system-prompt-file", "system-prompt", []string{"--system-prompt-file=other.md"}},
		{"claude_code", "--append-system-prompt", "system-prompt", []string{"--append-system-prompt", "other"}},
		{"codex_cli", "--profile", "mcp-servers", []string{"--profile", "other"}},
		{"codex_cli", "-c", "system-prompt", []string{"-c", "model_instructions_file=\"other.md\""}},
		{"codex_cli", "--config", "mcp-servers", []string{"--config=mcp_servers.other.command=\"other\""}},
	}
	for _, row := range rows {
		for _, tracked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%s/%v", row.environment, row.flag, tracked), func(t *testing.T) {
				f := entryFixture(t, row.environment, tracked)
				f.args = append(f.args[:8], row.native...)
				code, out, stderr := f.run()
				if code != 1 || len(out) != 0 || !bytes.Contains(stderr, []byte(row.flag)) || !bytes.Contains(stderr, []byte(row.channel)) || !bytes.Contains(stderr, []byte("curator-run: plan_refused:")) {
					t.Fatalf("exit=%d stderr=%s", code, stderr)
				}
				if f.builds != 1 || f.verdicts != 0 {
					t.Fatalf("calls=%d/%d", f.builds, f.verdicts)
				}
				f.assertNoChild(t)
			})
		}
	}
}

func TestProductionContextCarrierEnvironmentOverlayOwnedSnapshot(t *testing.T) {
	for _, environment := range []string{"claude_code", "codex_cli"} {
		for _, tracked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", environment, tracked), func(t *testing.T) {
				f := entryFixture(t, environment, tracked)
				real := f.deps.build
				f.deps.build = func(ctx context.Context, r *vendorplugin.Registry, req vendorplugin.SpawnRequest, mode agentic.LaunchMode) (agentic.PlanWithEnvironment, error) {
					built, err := real(ctx, r, req, mode)
					// Deliberately independent snapshot: composition must consume it,
					// rather than infer ownership by diffing the parent environment.
					built.OwnedEnv = append(built.OwnedEnv, "OWNED_ONLY=snapshot", "FIGMA_API_KEY=owned-secret")
					built.Plan.Env = append(built.Plan.Env, fragment.HomeVariable(environment)+"=/before-overlay")
					return built, err
				}
				code, out, stderr := f.run()
				if code != 0 {
					t.Fatalf("exit=%d stderr=%s", code, stderr)
				}
				_, env, names := capturedLaunch(t, out, tracked)
				if env[fragment.HomeVariable(environment)] != f.home {
					t.Fatalf("overlay=%v", env)
				}
				if tracked {
					if env["OWNED_ONLY"] != "snapshot" || env["FIGMA_API_KEY"] != "owned-secret" || len(names) != 0 || env["PARENT"] != "" {
						t.Fatalf("snapshot=%v names=%v", env, names)
					}
				} else if env["PARENT"] != "direct-only" || env["FIGMA_API_KEY"] != "source-secret" || env["OWNED_ONLY"] != "" {
					t.Fatalf("direct env=%v", env)
				}
				if !bytes.Contains(stderr, []byte("environment literal replaces lookup: FIGMA_API_KEY")) {
					t.Fatalf("warning=%s", stderr)
				}
			})
		}
	}
}

func TestProductionContextCarrierV2PermissionsUnchanged(t *testing.T) {
	for _, environment := range []string{"claude_code", "codex_cli"} {
		for _, tracked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", environment, tracked), func(t *testing.T) {
				f := entryFixture(t, environment, tracked)
				permission := "yolo"
				if tracked {
					permission = "native"
				}
				f.usePermissionsFragment(t, permission, false, "profile")
				code, out, stderr := f.run()
				if code != 0 {
					t.Fatalf("exit=%d stderr=%s", code, stderr)
				}
				if f.request.PermissionMode != agentic.PermissionMode(permission) || f.request.Context == nil || f.request.Context.Revision != fragment.Identity || f.request.Context.SystemPrompt == nil || f.request.Context.MCP == nil {
					t.Fatalf("request=%+v", f.request)
				}
				argv, _, _ := capturedLaunch(t, out, tracked)
				flag := "--dangerously-skip-permissions"
				if environment == "codex_cli" {
					flag = "--dangerously-bypass-approvals-and-sandbox"
				}
				if strings.Contains(strings.Join(argv, "\n"), flag) != !tracked {
					t.Fatalf("permissions argv=%q", argv)
				}
			})
		}
	}
}

func TestProductionDeprecatedInfraAliasesRefused(t *testing.T) {
	for _, alias := range []string{"openai-infra", "anthropic-infra"} {
		t.Run(alias, func(t *testing.T) {
			f := entryFixture(t, "claude_code", false)
			f.args[0] = alias
			code, out, stderr := f.run()
			if code != 2 || len(out) != 0 || f.resolver.calls != 0 || f.builds != 0 || !bytes.Contains(stderr, []byte("usage")) {
				t.Fatalf("exit=%d stderr=%s", code, stderr)
			}
			f.assertNoChild(t)
		})
	}
}

func TestProductionContextCarrierRefusesAdmittedNativeSuffixDrift(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		t.Run(fmt.Sprint(tracked), func(t *testing.T) {
			f := entryFixture(t, "claude_code", tracked)
			real := f.deps.build
			f.deps.build = func(ctx context.Context, r *vendorplugin.Registry, req vendorplugin.SpawnRequest, mode agentic.LaunchMode) (agentic.PlanWithEnvironment, error) {
				built, err := real(ctx, r, req, mode)
				if err == nil {
					built.Plan.Argv[len(built.Plan.Argv)-len(req.NativeArgs)] = "changed-empty-native"
				}
				return built, err
			}
			code, out, stderr := f.run()
			if code != 1 || len(out) != 0 || !bytes.Contains(stderr, []byte("requested native argument suffix")) {
				t.Fatalf("exit=%d stderr=%s", code, stderr)
			}
			f.assertNoChild(t)
		})
	}
}

func TestProductionContextCarrierUnselectedPromptAndAbsentMCP(t *testing.T) {
	for _, environment := range []string{"claude_code", "codex_cli"} {
		for _, tracked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", environment, tracked), func(t *testing.T) {
				f := entryFixture(t, environment, tracked)
				var obj map[string]any
				if err := json.Unmarshal([]byte(f.resolver.stdout), &obj); err != nil {
					t.Fatal(err)
				}
				delete(obj, "mcp")
				raw, _ := json.Marshal(obj)
				f.resolver.stdout = string(raw)
				native := []string{"--system-prompt-file", "native.md"}
				if environment == "codex_cli" {
					native = []string{"--profile", "native-profile", "-c", "model_instructions_file=\"native.md\""}
				}
				f.args = append(f.args[:5], "--")
				f.args = append(f.args, native...)
				code, out, stderr := f.run()
				if code != 0 {
					t.Fatalf("exit=%d stderr=%s", code, stderr)
				}
				if f.request.Context == nil || f.request.Context.SystemPrompt != nil || f.request.Context.MCP != nil {
					t.Fatalf("context=%+v", f.request.Context)
				}
				argv, _, _ := capturedLaunch(t, out, tracked)
				if !reflect.DeepEqual(argv[len(argv)-len(native):], native) || bytes.Contains(stderr, []byte("applied")) {
					t.Fatalf("argv=%q stderr=%s", argv, stderr)
				}
			})
		}
	}
}
