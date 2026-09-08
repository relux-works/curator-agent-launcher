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
		{"claude_code", Target{"claude-code", "claude"}},
		{"codex_cli", Target{"codex", "codex"}},
		{"pi", Target{"pi-native", "pi"}},
		{"opencode", Target{}}, {"future_env", Target{}}, {"", Target{}}, {"PI", Target{}}, {"pi-native", Target{}},
	} {
		t.Run(tc.env, func(t *testing.T) {
			got, err := Resolve(tc.env)
			if got != tc.want || (err != nil) != (tc.want == (Target{})) {
				t.Fatalf("Resolve(%q)=%+v, %v; want %+v", tc.env, got, err, tc.want)
			}
		})
	}
}
func TestKnownUnsupportedIsNotUnknown(t *testing.T) {
	if fragment.HomeVariable("opencode") == "" || fragment.HomeVariable("future_env") != "" {
		t.Fatal("adapter registry conflates unsupported and unknown")
	}
}
