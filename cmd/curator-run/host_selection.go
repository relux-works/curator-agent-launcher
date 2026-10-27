package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/hosted"
	"github.com/relux-works/curator-agent-launcher/internal/mapping"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

type hostSelection struct {
	client   client.Client
	route    client.Route
	terminal *os.File
	tail     []string
	intent   agentic.ResumeIntent
}

func emitHostFailure(w io.Writer, err error) int {
	var version *fragment.VersionError
	if errors.As(err, &version) {
		err = hostError(fragment.CodeVersionUnsupported, map[string]string{"environment": version.Environment, "required": version.Required})
	}
	if client.IsCode(err, "network_scope_unsupported") {
		_ = diagnostics.Emit(w, "network_scope_unsupported", "managed network selections are not supported by hosted execution")
		return diagnostics.ExitPolicy
	}
	var remote *client.Error
	if errors.As(err, &remote) {
		fmt.Fprintln(w, "curator-run: "+remote.Diagnostic())
		return remote.ExitCode()
	}
	var registry *sessionmanager.Error
	if errors.As(err, &registry) {
		return emitFailure(w, registry.Code, registry)
	}
	var resume *hosted.ResumeError
	if errors.As(err, &resume) {
		return emitFailure(w, "session_resume_invalid", resume)
	}
	return emitFailure(w, "plan_refused", err)
}
func hostError(code string, details map[string]string) error {
	return &client.Error{Code: code, Details: details}
}

func hostNotice(w io.Writer, count int) {
	if count == 0 {
		fmt.Fprintln(w, "curator-run: no session manager registered; launching natively (curator-run session-manager list)")
	} else {
		fmt.Fprintf(w, "curator-run: no session manager selected (%d registered); launching natively (curator-run session-manager use <id>)\n", count)
	}
}

// resolveHost follows the front-door order. Native authority returns before any
// registry read. Auto reads the registry before hosted checks; explicit hosted
// does those checks first. Only the capability matrix may select native later.
func resolveHost(ctx context.Context, inv cli.Invocation, frag *fragment.Fragment, stderr io.Writer, deps launchDeps) (result hostSelection, err error) {
	result.route.Kind = "native"
	mode := deps.launcherConfig.HostMode.Value
	if mode == "" {
		mode = "auto"
	}
	if cli.ExplicitNative(inv.HostFlag) || (inv.HostFlag == "" && mode == "native") {
		return result, nil
	}
	explicit := inv.HostFlag == cli.HostFlagHosted || mode == "hosted"
	checks := func() error {
		if inv.NetworkSet {
			return hostError("network_scope_unsupported", nil)
		}
		if inv.Detach {
			return nil
		}
		open := deps.receiverTerminal
		if open == nil {
			open = hosted.OpenTerminal
		}
		tty, e := open()
		if e != nil || tty == nil {
			if tty != nil {
				tty.Close()
			}
			return hostError("session_host_terminal_required", nil)
		}
		hosted.MarkCloseOnExec(tty)
		if hosted.ValidateTerminal(tty) != nil {
			tty.Close()
			return hostError("session_host_terminal_required", nil)
		}
		result.terminal = tty
		terminal := deps.stdinTerminal
		if terminal == nil {
			in := deps.stdin
			if in == nil {
				in = os.Stdin
			}
			terminal = func() bool { return execution.StdinIsTerminal(in) }
		}
		if !terminal() {
			return hostError("session_host_stdin_unsupported", nil)
		}
		return nil
	}
	defer func() {
		if err != nil && result.terminal != nil {
			result.terminal.Close()
			result.terminal = nil
		}
	}()
	if explicit {
		if err = checks(); err != nil {
			return result, err
		}
	}
	registry, _, err := sessionmanager.Load(deps.configResolver)
	if err != nil {
		return result, err
	}
	state, err := sessionmanager.Resolve(registry, sessionmanager.ExecutableAvailable)
	if err != nil {
		return result, err
	}
	if state.Kind != "selected" {
		if explicit {
			reason := "no manager selected"
			if state.Kind == "empty" {
				reason = "no manager registered"
			}
			return result, hostError("session_host_missing", map[string]string{"reason": reason})
		}
		hostNotice(stderr, len(registry.Managers))
		return result, nil
	}
	if !explicit {
		if err = checks(); err != nil {
			return result, err
		}
	}
	// Admit profile runtimes before negotiating the manager capability fence.
	if frag.Environment != fragment.EnvClaudeCode && frag.Environment != fragment.EnvCodexCLI && frag.Environment != fragment.EnvMuse {
		return result, hostError("session_host_provider_unsupported", map[string]string{"environment": frag.Environment})
	}
	systems := deps.systems
	if systems == nil {
		systems = agentic.Default
	}
	wrapper, err := hosted.WrapperArgs(inv.ResumeRequested, inv.ResumeHandle, inv.ResumeHandleSet, inv.ResumeID, inv.ResumeIDSet)
	if err != nil {
		return result, err
	}
	target, err := mapping.Resolve(frag.Environment)
	if err != nil {
		return result, err
	}
	elevation, err := hosted.ElevateResume(systems, target.System, wrapper, inv.Native)
	if err != nil {
		return result, err
	}
	result.tail, result.intent = elevation.NativeArgs, elevation.Intent
	wd, err := deps.workdir()
	if err != nil {
		return result, err
	}
	result.client = client.Client{ID: state.ID, Entry: state.Entry, Environment: deps.environ(), CWD: wd}
	response, err := result.client.Call(ctx, "describe", []byte(`{}`))
	if err != nil {
		return result, err
	}
	result.route, err = client.Required(*response.Describe, client.Needs{
		Runtime: frag.Environment, HostMode: mode, ExplicitHosted: inv.HostFlag == cli.HostFlagHosted,
		Detached: inv.Detach, Resume: elevation.Intent.Kind != agentic.ResumeNew, BoardGoal: inv.BoardGoal,
		DelegatedTrial: deps.launcherConfig.DelegatedTrial, SignedOff: client.Q1SignedOff, Doorbell: inv.A2A != "",
	})
	if err != nil {
		return result, err
	}
	if result.route.Kind == "native" {
		hostNotice(stderr, len(registry.Managers))
		return result, nil
	}
	if !slices.Contains(response.Describe.Capabilities, "survives-upgrade") && !slices.Contains(response.Describe.Capabilities, "drains-on-upgrade") {
		fmt.Fprintf(stderr, "curator-run: session manager %s ends hosted sessions when it is upgraded\n", state.ID)
	}
	return result, nil
}
