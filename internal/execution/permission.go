package execution

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
)

// PermissionRequest is the input to the closed §4.3 permission ladder. The
// profile value is present only when the v2 fragment identifies a named
// profile setting; source=default remains silent and falls through.
type PermissionRequest struct {
	Flag        cli.PermissionMode
	FlagPresent bool
	Profile     *fragment.Permissions
	Global      defaults.Member
	Headless    bool
	Tracked     bool
	Transport   bool
}

// PermissionDecision is the effective typed mode and its launcher-visible
// provenance. Provider mappings remain agents-management's responsibility.
type PermissionDecision struct {
	Mode   agentic.PermissionMode
	Source string
}

// PermissionError is a SPEC §6 permission refusal. Its code is selected at the
// owning execution boundary so no caller can degrade a refusal into a launch.
type PermissionError struct {
	Code   string
	Detail string
}

func (e *PermissionError) Error() string { return e.Code + ": " + e.Detail }

// ResolvePermission implements the closed mode precedence and the two
// terminal gates: force-native lock, fragment transport, and tracked mode.
func ResolvePermission(req PermissionRequest) (PermissionDecision, error) {
	locked := req.Profile != nil && req.Profile.Locked
	if locked {
		if (req.FlagPresent && req.Flag == cli.PermissionYolo) || (req.Global.Present && req.Global.Value == string(agentic.PermissionModeYolo)) {
			return PermissionDecision{}, &PermissionError{
				Code:   diagnostics.CodeUsage,
				Detail: "yolo conflicts with Curator's force-native permission lock",
			}
		}
		if req.FlagPresent {
			return PermissionDecision{Mode: agentic.PermissionModeNative, Source: "flag"}, nil
		}
		if req.Global.Present {
			return PermissionDecision{Mode: agentic.PermissionModeNative, Source: "global"}, nil
		}
		// The lock forces the built-in outcome to native; report the native
		// built-in provenance because the fragment source=global is the lock,
		// not the launcher's defaults.json source=global.
		return PermissionDecision{Mode: agentic.PermissionModeNative, Source: "default-headless"}, nil
	}

	decision := PermissionDecision{}
	switch {
	case req.FlagPresent:
		decision = PermissionDecision{Mode: agentic.PermissionMode(req.Flag), Source: "flag"}
	case req.Profile != nil && req.Profile.Source == "profile":
		decision = PermissionDecision{Mode: agentic.PermissionMode(req.Profile.Mode), Source: "profile"}
	case req.Global.Present:
		decision = PermissionDecision{Mode: agentic.PermissionMode(req.Global.Value), Source: "global"}
	case req.Headless || req.Tracked:
		decision = PermissionDecision{Mode: agentic.PermissionModeNative, Source: "default-headless"}
	default:
		decision = PermissionDecision{Mode: agentic.PermissionModeYolo, Source: "default-interactive"}
	}

	if decision.Mode == agentic.PermissionModeYolo && !req.Transport {
		return PermissionDecision{}, &PermissionError{
			Code:   diagnostics.CodePermissionPolicyUnsupported,
			Detail: "the fragment does not establish permission-mode transport",
		}
	}
	if decision.Mode == agentic.PermissionModeYolo && req.Tracked {
		return PermissionDecision{}, &PermissionError{
			Code:   diagnostics.CodePermissionModeTrackedUnsupported,
			Detail: "tracked launches cannot use yolo permission mode",
		}
	}
	return decision, nil
}

// HasNonInteractiveMarker checks the closed, versioned marker set from SPEC
// §4.6. Presence is the signal, including a present empty value.
func HasNonInteractiveMarker(env []string) bool {
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if name == "CI" || name == "GITHUB_ACTIONS" {
			return true
		}
	}
	return false
}

// ReportEffectiveNativePolicy prints only relaxations returned by the
// agents-management inspector. The returned record is included in the ax
// document only for tracked launches. Sources not inspected are never
// converted into a clean or relaxed-policy claim.
func ReportEffectiveNativePolicy(w ioWriter, inspection agentic.StoredPolicyInspection, tracked bool) *EffectiveNativePolicy {
	if len(inspection.Relaxations) == 0 {
		return nil
	}
	inspected := make(map[string]bool, len(inspection.SourcesInspected))
	for _, source := range inspection.SourcesInspected {
		inspected[source] = true
	}
	uninspected := make(map[string]bool, len(inspection.SourcesNotInspected))
	for _, source := range inspection.SourcesNotInspected {
		uninspected[source.SourcePath] = true
	}
	bySource := map[string]map[string]int{}
	for _, relaxation := range inspection.Relaxations {
		if relaxation.SourcePath == "" || relaxation.Selector == "" || !inspected[relaxation.SourcePath] || uninspected[relaxation.SourcePath] {
			continue
		}
		if bySource[relaxation.SourcePath] == nil {
			bySource[relaxation.SourcePath] = map[string]int{}
		}
		bySource[relaxation.SourcePath][relaxation.Selector]++
	}
	sources := make([]string, 0, len(bySource))
	for source := range bySource {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	var all []string
	for _, source := range sources {
		selectors := make([]string, 0, len(bySource[source]))
		for selector, count := range bySource[source] {
			all = append(all, selector)
			if selector == "permissions.allow" && count > 5 {
				selectors = append(selectors, fmt.Sprintf("permissions.allow (%d rules)", count))
			} else {
				selectors = append(selectors, selector)
			}
		}
		sort.Strings(selectors)
		fmt.Fprintf(w, "curator-run: effective-native-policy: relaxation=%s source=%s\n", strings.Join(selectors, ","), foldLineValue(source))
	}
	if !tracked || len(sources) == 0 {
		return nil
	}
	sort.Strings(all)
	all = compactStrings(all)
	return &EffectiveNativePolicy{Relaxations: all, Source: strings.Join(sources, ",")}
}

type ioWriter interface{ Write([]byte) (int, error) }

func foldLineValue(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\n", "\n  ")
}

func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

// InteractiveStdio reports whether both standard input and output are TTYs.
// Failure to establish either side is treated as headless.
func InteractiveStdio(stdin, stdout *os.File) bool {
	return stdin != nil && stdout != nil && isTerminalFD(stdin.Fd()) && isTerminalFD(stdout.Fd())
}
