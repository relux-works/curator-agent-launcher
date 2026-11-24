package mapping

import (
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"testing"
)

func TestResolveClosedMapping(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want Target
	}{
		{"claude_code", Target{"claude-code"}},
		{"codex_cli", Target{"codex"}},
		{"pi", Target{"pi-native"}},
		{"muse", Target{"muse"}},
		{"antigravity", Target{"antigravity"}},
		{"agy", Target{}}, {"claude", Target{}}, {"codex", Target{}}, {"opencode", Target{}}, {"future_env", Target{}}, {"", Target{}}, {"PI", Target{}}, {"pi-native", Target{}},
	} {
		t.Run(tc.env, func(t *testing.T) {
			got, err := Resolve(tc.env)
			if got != tc.want || (err != nil) != (tc.want == (Target{})) {
				t.Fatalf("Resolve(%q)=%+v, %v; want %+v", tc.env, got, err, tc.want)
			}
		})
	}
}

func TestKnownEnvironmentIsAClosedAllowlist(t *testing.T) {
	for environment, want := range map[string]bool{
		"claude_code": true, "codex_cli": true, "pi": true, "muse": true,
		"opencode": true, "antigravity": true,
		"future_env": false, "agy": false, "claude": false, "codex": false,
		"claude-code": false, "codex-cli": false, "pi-native": false, "gemini-cli": false, "": false,
	} {
		if got := KnownEnvironment(environment); got != want {
			t.Errorf("KnownEnvironment(%q) = %v, want %v", environment, got, want)
		}
	}
}

func TestKnownUnsupportedIsNotUnknown(t *testing.T) {
	if fragment.HomeVariable("opencode") == "" || fragment.HomeVariable("future_env") != "" {
		t.Fatal("adapter registry conflates unsupported and unknown")
	}
}
