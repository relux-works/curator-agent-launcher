//go:build unix

package client

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

// Production fragment parsing supplies the launch/resume digest unchanged.
// Lock hashes remain bare hex; missing/wrong digest prefixes, length, case and
// trailing bytes refuse as launch_plan_invalid for both delegated operations.
func TestResolvedProfileDigests(t *testing.T) {
	data, err := os.ReadFile("../../fragment/testdata/a0/codex_cli.json")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := fragment.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"launch", "resume"} {
		for _, tc := range []struct {
			name, lock, digest string
			valid              bool
		}{
			{"resolved", resolved.Profile.LockSHA256, resolved.Digest, true},
			{"bare-digest", resolved.Profile.LockSHA256, strings.TrimPrefix(resolved.Digest, "sha256:"), false},
			{"prefixed-lock", "sha256:" + resolved.Profile.LockSHA256, resolved.Digest, false},
			{"wrong-prefix", resolved.Profile.LockSHA256, strings.Replace(resolved.Digest, "sha256:", "sha512:", 1), false},
			{"short", resolved.Profile.LockSHA256, resolved.Digest[:len(resolved.Digest)-1], false},
			{"long", resolved.Profile.LockSHA256, resolved.Digest + "0", false},
			{"uppercase", resolved.Profile.LockSHA256, "sha256:" + strings.Repeat("A", 64), false},
			{"trailing-newline", resolved.Profile.LockSHA256, resolved.Digest + "\n", false},
		} {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				var request map[string]any
				if err := json.Unmarshal(document(t, operation), &request); err != nil {
					t.Fatal(err)
				}
				request["profile"] = map[string]string{"name": resolved.Profile.Name, "lock_sha256": tc.lock, "fragment_digest": tc.digest}
				err := ValidateRequest(operation, encoded(request))
				if tc.valid {
					if err != nil {
						t.Fatal("production resolver digest refused", err)
					}
				} else {
					requireCode(t, err, "launch_plan_invalid")
				}
			})
		}
	}
}
