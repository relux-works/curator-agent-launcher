package main

import (
	"context"
	"strings"
	"testing"
)

// Grammar errors win over leftovers and broken active policy; a valid launch
// refuses retired entries by existence, or malformed active policy by parsing.
func TestFlagsParseBeforeConfigRead(t *testing.T) {
	for _, file := range []string{"ax.json", "launcher.json"} {
		t.Run(file, func(t *testing.T) {
			deps, runner, loc := configHomeFixture(t)
			writeConfig(t, loc.System, file, "{malformed")
			var out, stderr strings.Builder
			if code := run(context.Background(), []string{"pi", "--bogus-flag"}, &out, &stderr, deps); code != 2 || !strings.Contains(stderr.String(), "curator-run: usage:") || strings.Contains(stderr.String(), "defaults_config_invalid") || runner.calls != 0 {
				t.Fatalf("usage error with broken %s: exit=%d stderr=%q", file, code, stderr.String())
			}
			stderr.Reset()
			want, diagnostic := 1, "defaults_config_invalid"
			if file == "ax.json" {
				want, diagnostic = 2, "ax_tracking_removed"
			}
			if code := run(context.Background(), []string{"pi"}, &out, &stderr, deps); code != want || !strings.Contains(stderr.String(), diagnostic) || runner.calls != 0 {
				t.Fatalf("launch with broken %s: exit=%d stderr=%q", file, code, stderr.String())
			}
		})
	}
	// Control: the same bad flag with valid config is still a usage error.
	deps, runner, _ := configHomeFixture(t)
	var out, stderr strings.Builder
	if code := run(context.Background(), []string{"pi", "--bogus-flag"}, &out, &stderr, deps); code != 2 || runner.calls != 0 {
		t.Fatalf("control: exit=%d stderr=%q", code, stderr.String())
	}
}
