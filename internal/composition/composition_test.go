package composition_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/agentic/systems/codex"
)

type owner struct {
	t     *testing.T
	req   agentic.LaunchRequest
	env   []string
	err   error
	calls int
}

func (o *owner) ChildEnv(parent []string, req agentic.LaunchRequest) ([]string, error) {
	o.t.Helper()
	o.calls++
	if parent != nil || !reflect.DeepEqual(req, o.req) {
		o.t.Fatal("ownership must use nil parent and exact request")
	}
	return o.env, o.err
}
func equal(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v; want %#v", got, want)
	}
}
func parsed(t *testing.T, env, path string, names []string) fragment.Fragment {
	t.Helper()
	home := filepath.Dir(path)
	f := map[string]any{"fragment": fragment.Identity, "environment": env, "profile": map[string]any{"name": "default", "lock_sha256": strings.Repeat("a", 64)}, "precedence": map[string]string{"winner": "higher-weight", "placement": "winner-last"}, "env": map[string]string{fragment.HomeVariable(env): home}}
	if env != "pi" {
		ch := map[string]any{"kind": "flag", "flag": "--mcp-config", "argument": "path", "with": []string{"--strict-mcp-config"}}
		if env == "codex_cli" {
			ch = map[string]any{"kind": "flag", "flag": "-p", "argument": "name", "name": "curator-mcp"}
		}
		if env == "opencode" {
			ch = map[string]any{"kind": "variable", "variable": "OPENCODE_CONFIG"}
		}
		if names == nil {
			names = []string{}
		}
		f["mcp"] = map[string]any{"path": path, "env_names": names, "channels": []any{ch}}
	}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	frag, err := fragment.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return *frag
}
func TestComposeOrderAndChannels(t *testing.T) {
	for _, env := range []string{"claude_code", "codex_cli", "opencode", "pi"} {
		t.Run(env, func(t *testing.T) {
			f := parsed(t, env, "/managed/default/tool/mcp.json", nil)
			p := agentic.Plan{System: "pi-native", Binary: "/exact/bin", WorkDir: "/exact/work", Argv: []string{"--model", "model with spaces", "--effort", "medium"}, Env: []string{"PATH=/sanitized"}}
			native := []string{"", "-p", "operator", "--dangerously-skip-permissions", "a\nb", "--", "--model=literal"}
			prompt := composition.PromptApplication{Argv: []string{"--selected-prompt", "verbatim content"}, Env: map[string]string{"PROMPT_CHANNEL": "selected"}}
			o := &owner{t: t}
			v, err := composition.Compose(p, o, agentic.LaunchRequest{}, f, prompt, native)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"--model", "model with spaces", "--effort", "medium", "--selected-prompt", "verbatim content"}
			switch env {
			case "claude_code":
				want = append(want, "--mcp-config", f.MCP.Path, "--strict-mcp-config")
			case "codex_cli":
				want = append(want, "-p", "curator-mcp")
			case "opencode":
				equal(t, v.EnvLiterals["OPENCODE_CONFIG"], f.MCP.Path)
			}
			want = append(want, native...)
			equal(t, v.Argv, want)
			equal(t, v.Binary, p.Binary)
			equal(t, v.WorkDir, p.WorkDir)
			equal(t, o.calls, 1)
			equal(t, v.EnvLiterals["PROMPT_CHANNEL"], "selected")
			p.Argv[0] = "changed"
			native[0] = "changed"
			prompt.Argv[0] = "changed"
			prompt.Env["PROMPT_CHANNEL"] = "changed"
			f.Env[fragment.HomeVariable(env)] = "changed"
			equal(t, v.Argv, want)
			equal(t, v.EnvLiterals["PROMPT_CHANNEL"], "selected")
		})
	}
}
func TestComposeEnvironmentBoundary(t *testing.T) {
	req := agentic.LaunchRequest{Env: []string{"REMOVED=must-not-return", "PATH=/unsafe", "SECRET=inherited-secret"}, Home: "/same/request"}
	p := agentic.Plan{Env: []string{"HOME=/parent", "PATH=/sanitized", "SECRET=inherited-secret", "FIGMA_API_KEY=source-secret", "OWN=old=literal", "EMPTY="}}
	f := parsed(t, "opencode", "/managed/default/tool/mcp.json", []string{"CHANNEL", "FIGMA_API_KEY", "OWN"})
	o := &owner{t: t, req: req, env: []string{"OWN=owned-value", "XDG_CONFIG_HOME=owned-home", "CHANNEL=owned-channel"}}
	v, err := composition.Compose(p, o, req, f, composition.PromptApplication{Env: map[string]string{"CHANNEL": "channel-value"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, v.EnvNames, []string{"FIGMA_API_KEY"})
	equal(t, v.EnvLiterals, map[string]string{"OWN": "owned-value", "CHANNEL": "channel-value", "XDG_CONFIG_HOME": "/managed/default/tool", "OPENCODE_CONFIG": f.MCP.Path})
	equal(t, v.Env, []string{"CHANNEL=channel-value", "EMPTY=", "FIGMA_API_KEY=source-secret", "HOME=/parent", "OPENCODE_CONFIG=" + f.MCP.Path, "OWN=old=literal", "PATH=/sanitized", "SECRET=inherited-secret", "XDG_CONFIG_HOME=/managed/default/tool"})
	equal(t, v.Warnings, []string{"environment override: XDG_CONFIG_HOME", "environment override: CHANNEL", "environment literal replaces lookup: CHANNEL", "environment literal replaces lookup: OWN"})
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"inherited-secret", "source-secret", "/parent", "/sanitized", "REMOVED"} {
		if strings.Contains(string(b), secret) {
			t.Fatalf("serialized inherited data: %s", secret)
		}
	}
	equal(t, o.calls, 1)
}
func TestComposeUnownedOverrideNoWarning(t *testing.T) {
	f := parsed(t, "pi", "/managed/default/pi/prompt", nil)
	v, err := composition.Compose(agentic.Plan{Env: []string{"PI_CODING_AGENT_DIR=/inherited"}}, &owner{t: t}, agentic.LaunchRequest{}, f, composition.PromptApplication{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, len(v.Warnings), 0)
}
func TestComposeStdin(t *testing.T) {
	for _, tc := range []struct {
		name     string
		attached bool
		data     []byte
		wire     string
	}{
		{"unattached", false, []byte("ignored"), "null"}, {"empty", true, nil, `{"encoding":"utf-8","bytes":""}`}, {"utf8", true, []byte("hello 日本語\n"), `{"encoding":"utf-8","bytes":"hello 日本語\n"}`}, {"binary", true, []byte{255, 254}, `{"encoding":"base64url","bytes":"__4"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := append([]byte(nil), tc.data...)
			v, err := composition.Compose(agentic.Plan{Stdin: agentic.StdinPayload{Attached: tc.attached, Bytes: data}}, &owner{t: t}, agentic.LaunchRequest{}, parsed(t, "pi", "/managed/default/pi/prompt", nil), composition.PromptApplication{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			b, err := json.Marshal(v.Stdin)
			if err != nil {
				t.Fatal(err)
			}
			equal(t, string(b), tc.wire)
			equal(t, v.RawStdin, agentic.StdinPayload{Attached: tc.attached, Bytes: tc.data})
			if len(data) > 0 {
				data[0] = 0
				equal(t, v.RawStdin.Bytes, tc.data)
			}
		})
	}
}
func TestComposeOwnershipFailure(t *testing.T) {
	sentinel := errors.New("ownership failed")
	v, err := composition.Compose(agentic.Plan{}, &owner{t: t, err: sentinel}, agentic.LaunchRequest{}, fragment.Fragment{}, composition.PromptApplication{}, nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("lost failure: %v", err)
	}
	equal(t, v, composition.Value{})
}
func TestComposeEmptyEnvironment(t *testing.T) {
	v, err := composition.Compose(agentic.Plan{}, &owner{t: t}, agentic.LaunchRequest{}, fragment.Fragment{}, composition.PromptApplication{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.Env == nil {
		t.Fatal("nil environment would inherit parent in os/exec")
	}
}
func TestLaunchBoundaryFilesystem(t *testing.T) {
	for _, kind := range []string{"missing", "regular", "directory", "dangling", "symlink", "unreadable", "parent-file"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "layer.toml")
			write := func(p string) {
				t.Helper()
				if err := os.WriteFile(p, []byte("mcp"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "regular":
				write(path)
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "dangling":
				if err := os.Symlink(filepath.Join(dir, "absent"), path); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				target := filepath.Join(dir, "target")
				write(target)
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "unreadable":
				write(path)
				if err := os.Chmod(path, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(path, 0600) })
				if os.Geteuid() == 0 {
					t.Skip("root bypasses Unix file permissions")
				}
			case "parent-file":
				write(path)
				path = filepath.Join(path, "child")
			}
			f := parsed(t, "codex_cli", path, nil)
			v, err := composition.Compose(agentic.Plan{}, &owner{t: t}, agentic.LaunchRequest{}, f, composition.PromptApplication{}, []string{"-p", "operator"})
			if err != nil {
				t.Fatal("composition must not probe early", err)
			}
			err = v.CheckLaunchBoundary()
			if kind == "regular" || kind == "symlink" {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				code := composition.CodeMCPLayerUnreadable
				if kind == "missing" {
					code = composition.CodeMCPLayerMissing
				}
				var layer *composition.LayerError
				if !errors.As(err, &layer) {
					t.Fatalf("expected %s, got %v", code, err)
				}
				equal(t, layer.Code, code)
				equal(t, layer.Path, path)
			}
			equal(t, v.Argv, []string{"-p", "curator-mcp", "-p", "operator"})
		})
	}
}
func TestLaunchBoundaryLateReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "layer.toml")
	v, err := composition.Compose(agentic.Plan{}, &owner{t: t}, agentic.LaunchRequest{}, parsed(t, "codex_cli", path, nil), composition.PromptApplication{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := v.CheckLaunchBoundary(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	var layer *composition.LayerError
	if !errors.As(v.CheckLaunchBoundary(), &layer) || layer.Code != composition.CodeMCPLayerMissing {
		t.Fatal("late deletion admitted")
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if !errors.As(v.CheckLaunchBoundary(), &layer) || layer.Code != composition.CodeMCPLayerUnreadable {
		t.Fatal("late replacement admitted")
	}
}
func TestLaunchBoundaryUnengaged(t *testing.T) {
	f := parsed(t, "codex_cli", "/missing/layer", nil)
	f.MCP = nil
	v, err := composition.Compose(agentic.Plan{}, &owner{t: t}, agentic.LaunchRequest{}, f, composition.PromptApplication{}, []string{"-p", "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.CheckLaunchBoundary(); err != nil {
		t.Fatal(err)
	}
	equal(t, v.Argv, []string{"-p", "operator"})
}

// This exercises the real released ownership surface, not native Pi admission.
func TestComposeReleasedChildEnvironment(t *testing.T) {
	sys := codex.New()
	req := agentic.LaunchRequest{Env: []string{"PATH=/usr/bin:/tmp/codex-path", "HOME=/parent", "SECRET=inherited-secret", "CODEX_THREAD_ID=removed", "TASK_BOARD_RUN_ID=removed", "FIGMA_API_KEY=local-secret"}}
	env, err := sys.ChildEnv(req.Env, req)
	if err != nil {
		t.Fatal(err)
	}
	v, err := composition.Compose(agentic.Plan{Env: env}, sys, req, parsed(t, "codex_cli", "/managed/default/codex/layer.toml", []string{"FIGMA_API_KEY"}), composition.PromptApplication{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, v.Env, []string{"CODEX_HOME=/managed/default/codex", "FIGMA_API_KEY=local-secret", "HOME=/parent", "PATH=/usr/bin", "SECRET=inherited-secret"})
	equal(t, v.EnvLiterals, map[string]string{"CODEX_HOME": "/managed/default/codex"})
	equal(t, v.EnvNames, []string{"FIGMA_API_KEY"})
}
