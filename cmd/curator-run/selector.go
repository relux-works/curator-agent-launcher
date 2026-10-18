package main

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/selector"
)

func selectionRequest(inv cli.Invocation, frag *fragment.Fragment, deps launchDeps) (selector.Request, error) {
	cwd, err := deps.workdir()
	if err != nil {
		return selector.Request{}, err
	}
	flags := selector.Flags{}
	if inv.ModelSet {
		flags.Model = &inv.Model
	}
	if inv.EffortSet {
		flags.Effort = &inv.Effort
	}
	return selector.BuildRequest(frag.Environment, frag.Profile.Name, cwd, flags, deps.registry)
}
func observationPath(deps launchDeps) string {
	if deps.observationPath != nil {
		return deps.observationPath()
	}
	home, err := os.UserHomeDir()
	if err != nil && os.Getenv("XDG_STATE_HOME") == "" {
		return ""
	}
	return selector.StatePath(os.Getenv("XDG_STATE_HOME"), home)
}
func startShadowSelection(deps launchDeps) {
	if deps.selectorConfig.Mode != "shadow" || deps.selectorRequest == nil {
		return
	}
	path := observationPath(deps)
	if path == "" {
		return
	}
	job := selector.ShadowJob{Config: deps.selectorConfig, Request: *deps.selectorRequest, Observations: path}
	start := deps.shadowStart
	if start == nil {
		start = func(job selector.ShadowJob) (*selector.Child, error) { return selector.StartShadow(job, "", nil) }
	}
	_, _ = start(job)
}
func observeSelection(deps launchDeps, resolved defaults.Resolved, outcome string) {
	if deps.selectorRequest == nil {
		return
	}
	_, digest, err := selector.Encode(*deps.selectorRequest)
	path := observationPath(deps)
	if err != nil || path == "" {
		return
	}
	_ = selector.Append(path, selector.Observation{Kind: "observation", RequestDigest: digest, Runtime: resolved.Runtime, Model: resolved.Model.Value, Effort: resolved.Effort.Value, Outcome: outcome})
}

// Observe only launcher diagnostic frames while forwarding every byte unchanged.
// A child exit without a launcher refusal is a launched outcome, even if nonzero.
type selectorOutcomeWriter struct {
	io.Writer
	pending []byte
	code    string
}

func (w *selectorOutcomeWriter) Write(p []byte) (int, error) {
	w.pending = append(w.pending, p...)
	for {
		i := bytes.IndexByte(w.pending, '\n')
		if i < 0 {
			break
		}
		line := string(w.pending[:i])
		w.pending = w.pending[i+1:]
		if diagnostics.IsDiagnosticLine(line) {
			rest := strings.TrimPrefix(line, cli.Name+": ")
			w.code, _, _ = strings.Cut(rest, ":")
		}
	}
	// Bound telemetry buffering even for arbitrary unterminated child stderr.
	if len(w.pending) > selector.Limit {
		w.pending = nil
	}
	return w.Writer.Write(p)
}
func (w *selectorOutcomeWriter) outcome() string {
	if w.code != "" {
		return "refused:" + w.code
	}
	return "launched"
}

func selectedEffort(decision selector.Decision) string {
	if decision.Effort == nil {
		return ""
	}
	return *decision.Effort
}
