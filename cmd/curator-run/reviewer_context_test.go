package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
	"reflect"
	"testing"
)

func TestReviewerContextSelectorSpellings(t *testing.T) {
	rows := []struct {
		env, channel string
		native       []string
	}{
		{"claude_code", "mcp-servers", []string{"--mcp-config=other.json"}},
		{"claude_code", "system-prompt", []string{"--system-prompt=override"}},
		{"claude_code", "system-prompt", []string{"--append-system-prompt-file=other.md"}},
		{"claude_code", "system-prompt", []string{"--append-system-prompt=override"}},
		{"codex_cli", "mcp-servers", []string{"-pother"}},
		{"codex_cli", "mcp-servers", []string{"-p=other"}},
		{"codex_cli", "mcp-servers", []string{"--profile=other"}},
		{"codex_cli", "mcp-servers", []string{"-cmcp_servers.other.command=\"other\""}},
		{"codex_cli", "mcp-servers", []string{"--config", "mcp_servers={}"}},
		{"codex_cli", "system-prompt", []string{"-cmodel_instructions_file=\"other.md\""}},
		{"codex_cli", "system-prompt", []string{"--config", " model_instructions_file = \"other.md\""}},
	}
	for i, row := range rows {
		for _, tracked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/%s/%v", i, row.env, tracked), func(t *testing.T) {
				f := entryFixture(t, row.env, tracked)
				f.args = append(f.args[:8], row.native...)
				insertLauncherArgs(f, "--permissions", "native")
				real := f.deps.build
				var seen error
				f.deps.build = func(ctx context.Context, r *vendorplugin.Registry, req vendorplugin.SpawnRequest, mode agentic.LaunchMode) (agentic.PlanWithEnvironment, error) {
					p, e := real(ctx, r, req, mode)
					seen = e
					return p, e
				}
				code, out, stderr := f.run()
				var conflict *agentic.ContextDescriptorConflictError
				if code != 1 || len(out) != 0 || !errors.As(seen, &conflict) || string(conflict.Channel) != row.channel || !bytes.Contains(stderr, []byte(row.channel)) {
					t.Fatalf("exit=%d typed=%T stderr=%s", code, seen, stderr)
				}
				f.assertNoChild(t)
			})
		}
	}
}
func TestReviewerContextSeparatorPassThrough(t *testing.T) {
	for _, env := range []string{"claude_code", "codex_cli"} {
		for _, tracked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", env, tracked), func(t *testing.T) {
				f := entryFixture(t, env, tracked)
				native := []string{"--", "--mcp-config=prompt-text", "-pother", "--system-prompt-file=prompt-text", ""}
				f.args = append(f.args[:8], native...)
				insertLauncherArgs(f, "--permissions", "native")
				code, out, stderr := f.run()
				if code != 0 {
					t.Fatalf("exit=%d stderr=%s", code, stderr)
				}
				argv, _, _ := capturedLaunch(t, out, tracked)
				if !reflect.DeepEqual(argv[len(argv)-len(native):], native) {
					t.Fatalf("argv=%q", argv)
				}
			})
		}
	}
}
