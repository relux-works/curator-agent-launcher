package m0adapter

import (
	"path"
	"strings"
)

// Immutable installation literals (spec §4). They are set only at build time,
// one artifact per installation tuple:
//
//	go build -ldflags "\
//	  -X github.com/relux-works/curator-agent-launcher/internal/m0adapter.brokerExecutable=/abs/broker \
//	  -X github.com/relux-works/curator-agent-launcher/internal/m0adapter.stationHome=/abs/station \
//	  -X github.com/relux-works/curator-agent-launcher/internal/m0adapter.runnerCommand=run-codex \
//	  -X github.com/relux-works/curator-agent-launcher/internal/m0adapter.clockProfile=<profile> \
//	  -X github.com/relux-works/curator-agent-launcher/internal/m0adapter.keymakerMarker=<marker> \
//	  -X github.com/relux-works/curator-agent-launcher/internal/m0adapter.deniedPathPrefix=/abs/l-owned" \
//	  ./cmd/curator-run-m0-adapter
//
// Nothing in this package assigns them: there is no flag, environment
// override or runtime lookup. An empty literal refuses 78.
var (
	// brokerExecutable is E, the absolute approved broker executable.
	brokerExecutable string
	// stationHome is H, the manager-verified station home.
	stationHome string
	// runnerCommand is the broker runner command; "run-codex" is the only
	// implemented mapping (see runnerArgv).
	runnerCommand string
	// clockProfile selects the qualified clock; see qualifiedNowMs.
	clockProfile string
	// keymakerMarker is a configured Keymaker endpoint marker that must not
	// appear in any environment value or runner argv element (hook H3 defence in depth).
	keymakerMarker string
	// deniedPathPrefix is a path prefix owned by L (the dispatcher side) that
	// must not appear in any environment value or runner argv element.
	deniedPathPrefix string
)

// originSocketMarker is the fixed origin-socket deny marker (hook H3 defence in depth).
const originSocketMarker = "origin.sock"

// Literals is the validated immutable installation tuple seen by Run.
type Literals struct {
	Executable       string // E
	Home             string // H
	RunnerCommand    string
	ClockProfile     string
	KeymakerMarker   string
	DeniedPathPrefix string
}

// installedLiterals returns the build-time literals; only Main calls it.
func installedLiterals() Literals {
	return Literals{
		Executable:       brokerExecutable,
		Home:             stationHome,
		RunnerCommand:    runnerCommand,
		ClockProfile:     clockProfile,
		KeymakerMarker:   keymakerMarker,
		DeniedPathPrefix: deniedPathPrefix,
	}
}

func (l Literals) validate() error {
	for _, v := range []string{l.Executable, l.Home, l.RunnerCommand, l.ClockProfile, l.KeymakerMarker, l.DeniedPathPrefix} {
		if v == "" {
			return refuse(ReasonLiteralMissing)
		}
	}
	if !cleanAbsolute(l.Executable) || !cleanAbsolute(l.Home) || !cleanAbsolute(l.DeniedPathPrefix) {
		return refuse(ReasonLiteralInvalid)
	}
	if !runnerCommandShape(l.RunnerCommand) || !profileShape(l.ClockProfile) || !printableASCII(l.KeymakerMarker, maxStringBytes) {
		return refuse(ReasonLiteralInvalid)
	}
	return nil
}

// cleanAbsolute admits one canonical absolute spelling: rooted, not the root
// itself, NUL-free, at most maxPathBytes, and already path.Clean (no "//",
// ".", ".." or trailing slash). It performs no filesystem lookup.
func cleanAbsolute(p string) bool {
	return len(p) > 1 && len(p) <= maxPathBytes && p[0] == '/' &&
		strings.IndexByte(p, 0) < 0 && path.Clean(p) == p
}

// runnerCommandShape is 1..32 bytes: first [a-z], then [a-z0-9-].
func runnerCommandShape(s string) bool {
	if len(s) == 0 || len(s) > 32 || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// profileShape is 1..64 bytes: first [a-z0-9], then [a-z0-9._-].
func profileShape(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
		if i > 0 {
			ok = ok || c == '.' || c == '_' || c == '-'
		}
		if !ok {
			return false
		}
	}
	return true
}

// printableASCII is 1..limit bytes of U+0021..U+007E.
func printableASCII(s string, limit int) bool {
	if len(s) == 0 || len(s) > limit {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x21 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

// deniedMarker reports whether s contains any build-time deny marker: the
// origin socket name, the Keymaker endpoint marker or the L-owned prefix.
func (l Literals) deniedMarker(s string) bool {
	return strings.Contains(s, originSocketMarker) ||
		strings.Contains(s, l.KeymakerMarker) ||
		strings.Contains(s, l.DeniedPathPrefix)
}
