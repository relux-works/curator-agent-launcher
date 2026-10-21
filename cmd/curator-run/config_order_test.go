package main

import (
	"context"
	"strings"
	"testing"
)

// FRONT-DOOR §3.2 parses the flags before reading ax.json and launcher.json.
// With a malformed machine ax.json, a usage error reports usage (2) first, and
// a well-formed launch reports defaults_config_invalid (1) before any resolver
// contact. Both broken files are covered, with a valid-config control.
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
			if code := run(context.Background(), []string{"pi"}, &out, &stderr, deps); code != 1 || !strings.Contains(stderr.String(), "defaults_config_invalid") || runner.calls != 0 {
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
