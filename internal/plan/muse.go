package plan

import (
	"fmt"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
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

// commitAfterRelease is Go's pseudo-version of a commit that follows a
// release tag: vX.Y.Z-0.<UTC time>-<12 hex>, where the release it follows is
// vX.Y.(Z-1). It is the form a module pinned between two tags reports.
var commitAfterRelease = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.([1-9][0-9]*)-0\.([0-9]{14})-[0-9a-f]{12}$`)

func museRelease(major, minor, patch int) bool {
	return major > 0 || major == 0 && (minor > 5 || minor == 5 && patch >= 66)
}

// checkMuseModuleVersion admits a plain release of at least v0.5.66, and a
// commit after such a release in Go's pseudo-version form, judged by the
// release it follows. Every other spelling refuses: a pre-release, build
// metadata, the other pseudo-version forms and anything unreadable.
func checkMuseModuleVersion(version string) error {
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	admitted := false
	if len(parts) == 3 && !strings.ContainsAny(parts[2], "-+") {
		major, e1 := strconv.Atoi(parts[0])
		minor, e2 := strconv.Atoi(parts[1])
		patch, e3 := strconv.Atoi(parts[2])
		admitted = e1 == nil && e2 == nil && e3 == nil && museRelease(major, minor, patch)
	} else if m := commitAfterRelease.FindStringSubmatch(version); m != nil {
		major, e1 := strconv.Atoi(m[1])
		minor, e2 := strconv.Atoi(m[2])
		patch, e3 := strconv.Atoi(m[3])
		_, e4 := time.Parse("20060102150405", m[4])
		admitted = e1 == nil && e2 == nil && e3 == nil && e4 == nil && museRelease(major, minor, patch-1)
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
