// The tmux terminal backend provides a PTY and persistence. The launcher in
// that PTY still owns fragment validation, policy, child start and cleanup.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/doorbell"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

type terminalRequest struct {
	Identity string
	Args     []string
	Fragment string
}

func tmuxRequest(inv cli.Invocation, frag *fragment.Fragment) terminalRequest {
	args := []string{inv.EnvID, "--native", "--profile", frag.Profile.Name}
	networkPolicy := inv.NetworkMode
	if networkPolicy == "pinned" {
		networkPolicy = "pinned:" + strings.TrimPrefix(inv.NetworkPin, "sha256-")
	}
	for _, p := range [][2]string{{"--model", inv.Model}, {"--effort", inv.Effort}, {"--permissions", string(inv.PermissionMode)},
		{"--system-prompt", string(inv.SystemPrompt)}, {"--network", inv.Network}, {"--network-policy", networkPolicy},
		{"--network-known-bad", inv.NetworkKnownBad}} {
		if p[1] != "" {
			args = append(args, p[0], p[1])
		}
	}
	if inv.GatewayRecoveryEnabled {
		args = append(args, "--gateway-recovery")
	}
	native := inv.Native
	args = append(append(args, "--"), native...)
	return terminalRequest{Args: args, Fragment: frag.Digest, Identity: inv.A2A}
}

func startTmux(ctx context.Context, host hostSelection, inv cli.Invocation, frag *fragment.Fragment, wd, providerBinary string, stderr io.Writer) int {
	fail := func(err error) int { return emitFailure(stderr, diagnostics.CodePlanRefused, err) }
	request := tmuxRequest(inv, frag)
	binary := host.client.Entry.Executable
	wd, err := filepath.EvalSymlinks(wd)
	if err != nil {
		return fail(err)
	}
	key, _ := json.Marshal([]any{wd, request, host.client.ID, host.client.Entry.RegisteredAt})
	digest := fmt.Sprintf("%x", sha256.Sum256(key))
	tail := request.Args[slices.Index(request.Args, "--")+1:]
	var label string
	if len(tail) == 2 && (tail[0] == "resume" || tail[0] == "--resume") && convertedUUID.MatchString(tail[1]) {
		// One running pane per provider thread in its actual managed home.
		label = fmt.Sprintf("curator-%x", sha256.Sum256([]byte(frag.Home()+"\x00"+tail[1])))[:32]
	} else {
		nonce := make([]byte, 12)
		if _, err := rand.Read(nonce); err != nil {
			return fail(err)
		}
		label = "curator-" + hex.EncodeToString(nonce)
	}
	tmux := func(args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, binary, append([]string{"-L", label, "-f", os.DevNull}, args...)...)
		cmd.Dir = wd
		cmd.Env = slices.DeleteFunc(os.Environ(), func(v string) bool { return strings.HasPrefix(v, "TMUX=") })
		return cmd
	}
	if tmux("has-session", "-t", "agent").Run() == nil {
		old, err := tmux("show-options", "-qv", "-t", "agent", "@curator_ready").Output()
		if err != nil || strings.TrimSpace(string(old)) != digest {
			return fail(errors.New("tmux session exists with a different or pending launch; inspect " + label))
		}
	} else {
		if inv.A2A != "" {
			if err := requireReleasedIdentity(ctx, inv.A2A, wd, stderr); err != nil {
				return fail(err)
			}
			installer := doorbell.Installer{Environment: os.Environ(), InvocationDirectory: wd, Diagnostics: stderr}
			if inv.EnvID == fragment.EnvClaudeCode {
				if err := installer.Ensure(ctx, wd, inv.A2A); err != nil {
					return fail(err)
				}
			} else {
				path, err := installer.EnsureCodex(ctx, wd, frag.Home(), inv.A2A)
				if err != nil {
					return fail(err)
				}
				fmt.Fprintf(stderr, "curator-run: a2a hooks %s; inspect /hooks in this Codex profile for trust\n", path)
			}
		}
		self, err := os.Executable()
		if err != nil {
			return fail(err)
		}
		dir, err := os.MkdirTemp("", "cr-")
		if err != nil {
			return fail(err)
		}
		defer os.Remove(dir)
		address := filepath.Join(dir, "ack")
		if rel, e := filepath.Rel(wd, address); e == nil && len(rel) < len(address) {
			address = rel
		}
		listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: address, Net: "unix"})
		if err != nil {
			return fail(err)
		}
		defer listener.Close()
		// Only fixed launcher operands reach tmux's command parser. Native
		// argv and policy data travel over the private socket as JSON.
		cmd := tmux("new-session", "-d", "-s", "agent", "-c", wd, self, "__tmux-child", address)
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			return fail(err)
		}
		fmt.Fprintf(stderr, "curator-run: tmux socket=%s session=agent\n", label)
		_ = listener.SetDeadline(time.Now().Add(20 * time.Second))
		conn, err := listener.AcceptUnix()
		if err != nil {
			return fail(err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Minute))
		if err := json.NewEncoder(conn).Encode(request); err != nil {
			return fail(err)
		}
		var ready string
		if err := json.NewDecoder(conn).Decode(&ready); err != nil {
			return fail(fmt.Errorf("tmux child outcome unknown (%s): %w", label, err))
		}
		if ready != "started" {
			return fail(fmt.Errorf("tmux child failed before provider start: %s", ready))
		}
		if err := tmux("set-option", "-t", "agent", "@curator_ready", digest).Run(); err != nil {
			return fail(err)
		}
	}
	fmt.Fprintf(stderr, "curator-run: attach with tmux -L %s attach -t agent\n", label)
	if inv.A2A != "" {
		installer := doorbell.Installer{Environment: os.Environ(), InvocationDirectory: wd}
		if inv.EnvID == fragment.EnvCodexCLI {
			if err := installer.CodexTrust(ctx, providerBinary, wd, frag.Home(), inv.A2A); err != nil {
				fmt.Fprintf(stderr, "curator-run: provider_started=true hooks=unverified wake=unverified: %v; inspect /hooks in the existing pane\n", err)
				if inv.Detach {
					return diagnostics.ExitPolicy
				}
			} else {
				fmt.Fprintln(stderr, "curator-run: codex_hooks=trusted_enabled")
			}
		}
		if err := installer.WaitForTerminal(ctx, wd, inv.A2A); err != nil {
			fmt.Fprintf(stderr, "curator-run: provider_started=true terminal_binding=unverified wake=unverified: %v\n", err)
			if inv.Detach {
				return diagnostics.ExitPolicy
			}
		} else {
			fmt.Fprintf(stderr, "curator-run: a2a identity=%s terminal_binding=current lane_readiness=ready idle_wake=unverified\n", inv.A2A)
		}
	}
	if inv.Detach {
		return 0
	}
	cmd := tmux("attach-session", "-t", "agent")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, stderr
	if err := cmd.Run(); err != nil {
		return fail(err)
	}
	return 0
}

func runTmuxChild(address string, deps launchDeps) int {
	conn, err := net.DialTimeout("unix", address, 5*time.Second)
	if err != nil {
		return 1
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	var request terminalRequest
	if err := json.NewDecoder(io.LimitReader(conn, 2<<20)).Decode(&request); err != nil {
		return 1
	}
	inv, err := cli.Parse(request.Args)
	if err != nil || !cli.ExplicitNative(inv.HostFlag) || !inv.ProfileSet || request.Fragment == "" {
		return 1
	}
	// Runtime parent selectors belong to the calling session. The trusted
	// SessionStart hook in this pane must bind the newly launched child.
	for _, key := range []string{"CODEX_THREAD_ID", "A2A_SESSION_ID", "CLAUDE_CODE_SESSION_ID", "A2A_AGENT"} {
		_ = os.Unsetenv(key)
	}
	if request.Identity != "" {
		_ = os.Setenv("A2A_AGENT", request.Identity)
	}
	started := false
	deps.terminalFragment = request.Fragment
	deps.terminalStarted = func() { started = true; _ = json.NewEncoder(conn).Encode("started"); _ = conn.Close() }
	code := run(context.Background(), request.Args, os.Stdout, os.Stderr, deps)
	if !started {
		_ = json.NewEncoder(conn).Encode(fmt.Sprintf("exit %d", code))
	}
	return code
}
