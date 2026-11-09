# Vendored claude_env proposal corpus

Copied byte for byte from relux-works/curator-spec at commit
`32a8bf140a8ad84015c29d900adecbc164b2db7f`,
`conformance/machine-claude-env-v1/`. Git blob IDs:

| File | Blob |
| --- | --- |
| `cases.json` | `db4019f89cffd3726dc3f281dd57d22c08f02d66` |
| `plants.json` | `e321757deed379795135a798499561d23d295250` |
| `containing-cases.json` | `51837cb9950c8d441f4e7fc14f3eed0a48b2a3a6` |

`claude_env_test.go` checks these IDs, so an edited copy fails. Refresh only
from a landed spec commit, and update this table and the test together.
The corpus checks the proposal's input and refusal rules; it is not a runtime
qualification.
