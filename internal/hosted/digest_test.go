package hosted_test

import (
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/hosted"
)

// contractExample is the §3.2 complete Phase 1 example, copied verbatim
// except for whitespace. Its content_digest is the independent oracle for
// the digest function: recomputation must reproduce it byte for byte.
const contractExample = `{
 "schema":"urn:relux:task-board:session-launch-plan",
 "schema_version":"1.0.0",
 "content_digest":"sha256:ba036a849e1d8bec3608567006df091e4605261351ee0a9d4a8e47aec1b75447",
 "producer":{"module_version":"0.5.45","module_commit":"1111111111111111111111111111111111111111"},
 "environment":"claude_code",
 "process":{
  "binary":"/synthetic/bin/claude",
  "argv":["--model","example-model","--disallowedTools=AskUserQuestion"],
  "env":["CLAUDE_CONFIG_DIR=/synthetic/profiles/demo/claude","HOME=/synthetic/operator","PATH=/synthetic/bin","TERM=xterm-256color"],
  "cwd":"/synthetic/project","stdin":null,
  "exec_guard":{"schema":"urn:relux:agents-management:exec-guard","schema_version":"1.0.0","data":{"kind":"unsealed"}}
 },
 "owned_literals":{"CLAUDE_CONFIG_DIR":"/synthetic/profiles/demo/claude"},
 "env_names":[],
 "managed_home":{"variable":"CLAUDE_CONFIG_DIR","path":"/synthetic/profiles/demo/claude"},
 "fragment":{"profile_name":"demo","pin":"sha256:2222222222222222222222222222222222222222222222222222222222222222","fragment_digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333","system_modules":false},
 "policy":{"permission_mode":"native","permission_source":"flag","execution_profile":"standard","effective_native_policy":null},
 "session_name":{"host":"claude_code-20261004T040000Z","native":null},
 "remote_control":{"enabled":false,"argv_indices":[]},
 "resume":{"kind":"new","identity":null},
 "restart":{"schema":"urn:relux:agents-management:claude-restart","schema_version":"1.0.0","data":{"argv":["--model","example-model","--disallowedTools=AskUserQuestion"],"identity_slot":{"index":3,"flag":"--resume"}}},
 "host_bundle":null,"network":null
}`

// TestContentDigestMatchesContractExample recomputes the §3.2 example digest
// through the production digest function. The example is the independent
// oracle: agreement proves the launcher's CCJ-1 bytes match the contract's.
func TestContentDigestMatchesContractExample(t *testing.T) {
	got, err := hosted.ContentDigest([]byte(contractExample))
	if err != nil {
		t.Fatal(err)
	}
	want := "sha256:ba036a849e1d8bec3608567006df091e4605261351ee0a9d4a8e47aec1b75447"
	if got != want {
		t.Fatalf("ContentDigest(example) = %s, want %s", got, want)
	}
}
