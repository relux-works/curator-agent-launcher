package main

import (
	"encoding/json"
	"fmt"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"strings"
	"testing"
)

func TestReviewerReservedPathPrependPreservesLaunch(t *testing.T) {
	for _, environment := range []string{"claude_code", "codex_cli", "pi"} {
		for _, tracked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", environment, tracked), func(t *testing.T) {
				f := entryFixture(t, environment, tracked)
				insertLauncherArgs(f, "--permissions", "native")
				var obj map[string]any
				if err := json.Unmarshal([]byte(f.resolver.stdout), &obj); err != nil {
					t.Fatal(err)
				}
				obj["path_prepend"] = f.home + "/reserved-bin"
				raw, err := json.Marshal(obj)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := fragment.Parse(raw); err != nil {
					t.Fatalf("input must be valid: %v", err)
				}
				f.resolver.stdout = string(raw)
				code, out, stderr := f.run()
				if code != 0 {
					t.Fatalf("valid reserved member must preserve launch, exit=%d stderr=%s", code, stderr)
				}
				if len(out) == 0 {
					t.Fatal("no child output")
				}
				if strings.Contains(string(out), "PATH="+f.home+"/reserved-bin") {
					t.Fatal("reserved member changed PATH")
				}
			})
		}
	}
}
