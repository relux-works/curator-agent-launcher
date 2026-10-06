package hosted

import (
	"errors"

	"github.com/relux-works/skill-agents-management/pkg/agentic"
	claudeSystem "github.com/relux-works/skill-agents-management/pkg/agentic/systems/claude"
)

// ExportRestart exports the closed claude-restart template for the composed,
// selector-free argv through the registered system. The module owns the
// template shape and validates the exact transformation; the launcher never
// recomposes. Any failure is launch_plan_invalid: elevation already passed,
// so a fault here is a plan-shape fault, not a selector conflict.
func ExportRestart(reg *agentic.Registry, system string, argv []string) (claudeSystem.RestartTemplate, error) {
	if reg == nil {
		return claudeSystem.RestartTemplate{}, planErr("restart", "no system registry wired")
	}
	sys, ok := reg.Lookup(agentic.SystemID(system))
	if !ok {
		return claudeSystem.RestartTemplate{}, planErr("restart", "system is not registered")
	}
	exporter, ok := sys.(interface {
		ExportRestartTemplate([]string) (claudeSystem.RestartTemplate, error)
	})
	if !ok {
		return claudeSystem.RestartTemplate{}, planErr("restart", "system exports no restart template")
	}
	template, err := exporter.ExportRestartTemplate(argv)
	if err != nil {
		var restartErr *claudeSystem.RestartInvalidError
		if errors.As(err, &restartErr) {
			return claudeSystem.RestartTemplate{}, planErr("restart", restartErr.Reason)
		}
		var resumeErr *agentic.ResumeInvalidError
		if errors.As(err, &resumeErr) {
			return claudeSystem.RestartTemplate{}, planErr("restart.data.argv", resumeErr.Reason)
		}
		return claudeSystem.RestartTemplate{}, planErr("restart", "restart export failed")
	}
	return template, nil
}
