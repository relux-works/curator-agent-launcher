//go:build unix

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/doorbell"
	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/hosted"
	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
	"github.com/relux-works/curator-agent-launcher/internal/mapping"
	"github.com/relux-works/curator-agent-launcher/internal/plan"
	"github.com/relux-works/curator-agent-launcher/internal/selector"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	"github.com/relux-works/curator-agent-launcher/internal/werkstatt"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

func environmentValue(env []string, key string) string {
	for _, v := range env {
		if len(v) > len(key) && v[:len(key)+1] == key+"=" {
			return v[len(key)+1:]
		}
	}
	return ""
}
func runWerkstatt(ctx context.Context, c werkstatt.Command, stdout, stderr io.Writer, config configfile.Resolver, deps launchDeps) int {
	env := os.Environ()
	if deps.environ != nil {
		env = deps.environ()
	}
	home := environmentValue(env, "HOME")
	state := environmentValue(env, "XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(home, ".local", "state")
	}
	credentials := environmentValue(env, "A2A_CONFIG_DIR")
	if credentials == "" {
		credentials = filepath.Join(home, ".config", "a2a-swarm")
	}
	invocationDirectory, _ := os.Getwd()
	if deps.workdir != nil {
		invocationDirectory, _ = deps.workdir()
	}
	installer := doorbell.Installer{Environment: env, InvocationDirectory: invocationDirectory, Diagnostics: stderr}
	service := werkstatt.Service{
		Loader:            werkstatt.Loader{Directory: filepath.Join(config.Locations.User, "werkstatt")},
		Store:             client.Store{Directory: filepath.Join(state, "swarma", "launcher", "werkstatt")},
		IdentityDirectory: filepath.Join(credentials, "agents"), Environment: env, Now: deps.now, Stdout: stdout, Stderr: stderr,
		Registry: func() (sessionmanager.Registry, error) { r, _, e := sessionmanager.Load(config); return r, e },
		Trial:    func() (bool, error) { p, e := launcherconfig.Load(config); return p.DelegatedTrial, e },
	}
	service.Prepare = func(ctx context.Context, r *werkstatt.Record, c werkstatt.Command, target string) (werkstatt.Prepared, error) {
		prepared, err := prepareWerkstatt(ctx, r, c, target, stderr, config, deps, env)
		if err == nil && r.Runtime == fragment.EnvClaudeCode && r.Doorbell != nil {
			err = installer.Ensure(ctx, r.Clone, r.Identity)
		}
		return prepared, err
	}
	service.Attach = func(ctx context.Context, id string, e sessionmanager.Entry, v *client.Session) (int, error) {
		open := deps.receiverTerminal
		if open == nil {
			open = hosted.OpenTerminal
		}
		tty, err := open()
		if err != nil {
			return 0, hostError("session_host_terminal_required", nil)
		}
		defer tty.Close()
		b, _ := json.Marshal(map[string]any{"session": v.Session, "expected_generation": v.Generation})
		result, err := (client.Client{ID: id, Entry: e, Environment: env}).Terminal(ctx, "attach", b, terminalStreams(tty, stdout, stderr, deps.stdin))
		if err != nil {
			return 0, err
		}
		if result.Refusal != nil {
			return 0, result.Refusal
		}
		return result.Exit, nil
	}
	service.Presence = func(ctx context.Context, record *werkstatt.Record) any {
		identity := record.Identity
		staleAfter := 3 * time.Minute
		if record.Doorbell != nil {
			heartbeat, _ := time.ParseDuration(record.Doorbell.Heartbeat)
			staleAfter = 3 * heartbeat
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		executable, err := installer.Resolve(record.Clone)
		if err != nil {
			return nil
		}
		cmd := exec.CommandContext(ctx, executable, "who", "--stale-after", staleAfter.String(), "--json")
		cmd.Env = env
		cmd.WaitDelay = 100 * time.Millisecond
		// Bound untrusted CLI output using the same writer contract as other helpers.
		out := &presenceOutput{}
		cmd.Stdout = out
		if cmd.Run() != nil || out.overflow {
			return nil
		}
		var rows []map[string]any
		if json.Unmarshal(out.data, &rows) != nil {
			return nil
		}
		for _, row := range rows {
			if row["identity"] == identity || row["agent"] == identity || row["name"] == identity {
				return row
			}
		}
		return nil
	}
	exit, err := service.Run(ctx, c)
	if err == nil {
		return exit
	}
	var resolve *fragment.ResolveError
	if errors.As(err, &resolve) {
		return emitFailure(stderr, resolve.Code, resolve)
	}
	var setting *defaults.Error
	if errors.As(err, &setting) {
		return emitFailure(stderr, setting.Code, setting.Err)
	}
	var policy *launcherconfig.Error
	if errors.As(err, &policy) {
		return emitFailure(stderr, diagnostics.CodeDefaultsInvalid, policy)
	}
	var selection *selector.Failure
	if errors.As(err, &selection) {
		return emitFailure(stderr, selection.Code, selection)
	}
	var permission *execution.PermissionError
	if errors.As(err, &permission) {
		return emitPermissionFailure(stderr, err)
	}
	var limited *plan.LimitedError
	if errors.As(err, &limited) {
		return emitFailure(stderr, diagnostics.CodePlanProviderLimited, err)
	}
	return emitHostFailure(stderr, err)
}

type presenceOutput struct {
	data     []byte
	overflow bool
}

func (p *presenceOutput) Write(b []byte) (int, error) {
	n := len(b)
	left := client.Limit - len(p.data)
	if n > left {
		p.overflow = true
		b = b[:left]
	}
	p.data = append(p.data, b...)
	return n, nil
}

func prepareWerkstatt(ctx context.Context, r *werkstatt.Record, c werkstatt.Command, target string, stderr io.Writer, config configfile.Resolver, deps launchDeps, env []string) (werkstatt.Prepared, error) {
	deps.configResolver = config
	var err error
	deps.selectorConfig, err = selector.Load(config)
	if err != nil {
		return werkstatt.Prepared{}, err
	}
	resolver := deps.resolver
	if resolver == nil {
		resolver = fragment.New()
	}
	frag, err := resolver.Resolve(ctx, fragment.Request{EnvID: r.Runtime, Profile: r.Profile, ProfileSet: true, Dir: r.Clone, Env: env, Stderr: stderr})
	if err != nil {
		return werkstatt.Prepared{}, err
	}
	files, err := defaults.LoadResolved(config)
	if err != nil {
		return werkstatt.Prepared{}, err
	}
	flags := defaults.Pair{}
	if r.Model != nil {
		flags.Model = defaults.Member{Value: *r.Model, Present: true}
	}
	if r.Effort != nil {
		flags.Effort = defaults.Member{Value: *r.Effort, Present: true}
	}
	if _, err = files.Resolve(r.Runtime, flags); err != nil {
		return werkstatt.Prepared{}, err
	}
	resolved, err := files.Complete(r.Runtime, flags, deps.registry)
	if err != nil {
		return werkstatt.Prepared{}, err
	}
	if deps.selectorConfig.Mode != "" {
		request, e := selector.BuildRequest(r.Runtime, r.Profile, r.Clone, selector.Flags{}, deps.registry)
		if e != nil {
			return werkstatt.Prepared{}, e
		}
		request.Role, request.Project, request.Werkstatt = &r.Role, &r.Project, &r.ID
		if c.TaskClass != "" {
			request.Task = &selector.Task{Class: c.TaskClass}
			if c.Difficulty != "" {
				request.Task.Difficulty = &c.Difficulty
			}
		}
		deps.selectorRequest = &request
		if deps.selectorConfig.Mode == "enforce" {
			decision, e := selector.Enforce(ctx, deps.selectorConfig, request)
			if e != nil {
				return werkstatt.Prepared{}, e
			}
			// Record model is pinned; only dispatch effort is applied. Runtime/model
			// from the selector remain telemetry and cannot move this station.
			flags.Effort = defaults.Member{}
			selected := defaults.Pair{Effort: defaults.Member{Value: selectedEffort(decision), Present: true}}
			resolved, e = files.CompleteWithSelection(r.Runtime, flags, selected, deps.registry)
			if e != nil {
				return werkstatt.Prepared{}, e
			}
			resolved.SelectorDecisionID, resolved.SelectorRuntime = decision.DecisionID, decision.Runtime
		}
	}
	inv := cli.Invocation{EnvID: r.Runtime, Profile: r.Profile, ProfileSet: true, Detach: true, Name: r.ID, NameSet: true}
	targetMapping, err := mapping.Resolve(r.Runtime)
	if err != nil {
		return werkstatt.Prepared{}, err
	}
	permission, toolRelease, _, err := resolvePermission(ctx, inv, frag, targetMapping, files, env, stderr, deps, true)
	if err != nil {
		return werkstatt.Prepared{}, err
	}
	// Admission is independent of defaults resolution. Even a resume must not
	// pass an invalid pinned model or effort to the manager.
	availability, err := deps.availability()
	if err != nil {
		return werkstatt.Prepared{}, err
	}
	build := deps.build
	if build == nil {
		build = vendorplugin.BuildLaunchWithEnvironment
	}
	admitted, err := plan.Build(ctx, plan.Deps{Registry: deps.registry, BuildLaunch: build, Availability: availability}, plan.Request{
		Runtime: resolved.Runtime, Model: resolved.Model.Value, Effort: resolved.Effort.Value,
		PermissionMode: permission.Mode, ToolRelease: toolRelease, NativeArgs: []string{},
		Home: frag.Home(), WorkDir: r.Clone, Env: env,
	})
	if err != nil {
		return werkstatt.Prepared{}, err
	}
	if admitted.Plan.Stdin.Attached {
		return werkstatt.Prepared{}, hostError("session_host_stdin_unsupported", nil)
	}
	intent := agentic.ResumeIntent{Kind: agentic.ResumeNew}
	if target != "" {
		intent = agentic.ResumeIntent{Kind: agentic.ResumeHandle, Identity: &target}
	}
	at := time.Now()
	if deps.now != nil {
		at = deps.now()
	}
	_, data, err := delegatedRequest(inv, frag, resolved, permission, r.Clone, []string{}, intent, at)
	if err != nil {
		return werkstatt.Prepared{}, err
	}
	var request map[string]any
	_ = json.Unmarshal(data, &request)
	request["admission_key"] = "werkstatt:" + r.ID
	if r.Doorbell != nil {
		request["doorbell"] = map[string]any{"identity": r.Identity, "heartbeat": r.Doorbell.Heartbeat, "max_wakes_per_hour": r.Doorbell.MaxWakesPerHour}
	}
	if target != "" {
		request["scope"] = map[string]any{"cwd": r.Clone, "labels": map[string]string{"werkstatt": r.ID}}
	} else {
		labels := map[string]string{"werkstatt": r.ID, "identity": r.Identity}
		if resolved.SelectorDecisionID != "" {
			labels["selector"] = resolved.SelectorDecisionID
		}
		request["labels"] = labels
		child := map[string]string{}
		for k, v := range r.ChildEnv {
			child[k] = v
		}
		child["A2A_AGENT"] = r.Identity
		if r.GitDir != "" {
			child["GIT_DIR"], child["GIT_WORK_TREE"] = r.GitDir, r.Clone
		}
		request["child_env"], request["weights_id"], request["harness_memory"] = child, r.WeightsID, *r.HarnessMemory
		roots := []string{}
		if r.Sandbox == "workspace-write" {
			roots = append(roots, r.Clone)
		}
		request["sandbox"] = map[string]any{"mode": r.Sandbox, "writable_roots": roots}
	}
	data, err = json.Marshal(request)
	if err != nil {
		return werkstatt.Prepared{}, err
	}
	op := "launch"
	if target != "" {
		op = "resume"
	}
	if err = client.ValidateRequest(op, data); err != nil {
		return werkstatt.Prepared{}, err
	}
	provider := defaults.ResolveProviderPath()
	if deps.providerPath != nil {
		provider = deps.providerPath()
	}
	_ = defaults.EmitGroupWithProvider(stderr, resolved, provider)
	prepared := werkstatt.Prepared{Request: data}
	if deps.selectorConfig.Mode == "shadow" {
		startShadowSelection(deps)
		prepared.Observe = func(outcome string) { observeSelection(deps, resolved, outcome) }
	}
	return prepared, nil
}
