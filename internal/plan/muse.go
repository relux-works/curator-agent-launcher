package plan

import (
	"fmt"
	"runtime/debug"

	"golang.org/x/mod/semver"
)

const agentsManagementModule = "github.com/relux-works/curator-runtimes"

func linkedModuleVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == agentsManagementModule {
				if dep.Replace != nil {
					return dep.Replace.Version
				}
				return dep.Version
			}
		}
	}
	return ""
}

func checkMuseModuleVersion(version string) error {
	if !semver.IsValid(version) || semver.Compare(version, "v0.5.66") < 0 {
		return &RefusedError{Detail: fmt.Sprintf("agents-management module version=%s required=v0.5.66-or-later for managed Muse", version)}
	}
	return nil
}

// FormError identifies a native tail that cannot retain the interactive v4
// exclusion. No argv is rewritten and no alternate form is silently selected.
type FormError struct{ Form string }

func (e *FormError) Error() string {
	return "Muse root_context requires an admitted interactive form; form=" + e.Form
}

func checkMuseInteractiveForm(req Request) error {
	if req.Context.RootContext == nil {
		return nil
	}
	// The public entry point is interactive. Native command words before the
	// provider's -- separator cannot select a different form with this fact.
	// Values of known global value options are data, including exec/serve.
	valueFlags := map[string]bool{"-c": true, "--config": true, "--model": true, "--reasoning-effort": true, "--workspace": true, "--provider": true}
	for i := 0; i < len(req.NativeArgs); i++ {
		arg := req.NativeArgs[i]
		if arg == "--" {
			break
		}
		if valueFlags[arg] {
			i++
			continue
		}
		if arg == "exec" || arg == "serve" {
			return &RefusedError{Detail: "selected Muse form cannot apply the required exclusion", Cause: &FormError{Form: arg}}
		}
	}
	return nil
}
