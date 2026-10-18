package plan

import (
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
)

const agentsManagementModule = "github.com/relux-works/skill-agents-management"

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
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	admitted := false
	if len(parts) == 3 && !strings.ContainsAny(parts[2], "-+") {
		major, e1 := strconv.Atoi(parts[0])
		minor, e2 := strconv.Atoi(parts[1])
		patch, e3 := strconv.Atoi(parts[2])
		admitted = e1 == nil && e2 == nil && e3 == nil && (major > 0 || major == 0 && (minor > 5 || minor == 5 && patch >= 66))
	}
	if !admitted {
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
