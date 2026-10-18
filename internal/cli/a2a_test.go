package cli

import (
	"strings"
	"testing"
)

// --a2a accepts the identity grammar and both value spellings, rejects invalid
// or repeated values and explicit native requests, and leaves native tails alone.
func TestA2AFlag(t *testing.T) {
	for _, identity := range []string{"example.dev", "a=b_-.", strings.Repeat("a", 64)} {
		for _, args := range [][]string{{"claude_code", "--hosted", "--a2a", identity}, {"claude_code", "--a2a=" + identity}} {
			inv, err := Parse(args, Options{})
			if err != nil || inv.A2A != identity {
				t.Fatalf("valid identity refused: %v %+v", err, inv)
			}
		}
	}
	for _, tail := range [][]string{{"--a2a"}, {"--a2a="}, {"--a2a", "Upper"}, {"--a2a", "has space"}, {"--a2a", strings.Repeat("a", 65)}, {"--a2a", "a;cmd"}, {"--a2a", "a", "--a2a", "b"}, {"--a2a", "a", "--native"}, {"--untracked", "--a2a", "a"}} {
		if _, err := Parse(append([]string{"claude_code"}, tail...), Options{}); !IsUsage(err) {
			t.Fatalf("invalid shape accepted: %q %v", tail, err)
		}
	}
	inv, err := Parse([]string{"claude_code", "--native", "--", "--a2a", "provider-value"}, Options{})
	if err != nil || inv.A2A != "" || len(inv.Native) != 2 {
		t.Fatalf("native tail changed: %+v %v", inv, err)
	}
}
