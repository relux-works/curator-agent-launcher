# Selected Core launcher configuration

Launcher selections come from `curator config launcher`, using Core's normal
selected machine configuration, trust checks and precedence. The launcher asks
Core once before fragment repair or route selection; it inherits the same
working directory and environment used for fragment resolution. It does not
search for another machine filename. A failed command, malformed projection or
unknown schema refuses; there is no fallback to absent selections. The query
has a 15-second execution cap, a further one-second pipe-drain cap and a
64-KiB stdout cap. A retained output pipe refuses even after a successful exit.
Its stderr and input details are not copied to launcher diagnostics.

The transport is a closed object:

```json
{
  "schema": "curator.launcher-config/1",
  "config_schema": 7,
  "config_path": "/operator/selected/config.json",
  "extensions": {
    "proxy": {"profile": "filtered"},
    "claude_auth": {
      "type": "oauth",
      "store": {"type": "keychain", "service": "curator-example", "account": "subscription"}
    }
  }
}
```

Supported older Core schemas 1, 2, 3 and 5 may supply only empty extensions.
Schema 7 supplies optional closed `proxy` and `claude_auth` members. Null,
duplicate, unknown, malformed and mixed store metadata refuse without exposing
input values or returning partial selections. References are metadata, not a
qualification or permission to access a store.

Both user and system `launcher.json` retain the existing
`curator-run-launcher-v1` host-policy contract. Both reject `proxy` and
`claude_auth`; `host_mode` and `delegated_trial` keep their existing authority
and behavior. Migration does not promote that file to another source.

`proxy.profile` references an existing network-catalog profile; it does not
introduce another endpoint registry. A selected proxy must retain the network
profile's validation, admission, child-environment, lease and cleanup contracts.
Explicit `--network` wins over the machine `proxy.profile`, which wins over
the existing operator default. Machine-selected records use origin
`launcher-config`; explicit CLI records retain `explicit`. All paths retain
the existing catalog, exact-build admission, network guard and cleanup. Hosted
start/resume refuses a selected network before manager contact because the
destination does not enforce that binding. Native fresh and native runtime
resume tails pass through the same admission path.

`claude_auth.type` is exactly `oauth`. Its `store` is exactly one of:

- `{"type":"keychain","service":"curator-claude","account":"subscription"}`
- `{"type":"file","path":"/absolute/private/token-file"}`

References must be nonempty, bounded strings without surrounding whitespace or
control characters. File paths must be absolute and clean. A store cannot mix
the two shapes or carry inline token bytes. Parsing and serialization never
open a store, test token validity, change Keychain ACLs or enable a launch.

The execution contract requires resolving a selected store only for the final
Claude child, after late launch checks and before start. File resolution must
validate and read the same opened regular object, owned by the caller with
exact mode 0600. Missing, unsafe, unreadable or empty inputs refuse with a
sanitized stable diagnostic and no child or native/other-store fallback.
The resolver must leave stored bytes, permissions and existing sessions intact.

Only that child's environment receives `CLAUDE_CODE_OAUTH_TOKEN`. Token bytes
must never enter arguments, global process environment, admission plans,
fragments, markers, provenance, logs or hosted transport. Help, metadata
inspection, another runtime and refused preparation must not read the store.
Credential refusal must preserve gateway cleanup and signal/exit behavior.
Hosted/delegated paths require an explicit supported integration or a clear
refusal; silently dropping the selection is forbidden.

This standalone section does not enable the later node-source mode or claim
`credential-injection/1` qualification. No node enrollment, protected authorizer,
generation, revoke or manager API is introduced by the section.

Schema 8 arrives as `curator.launcher-config/2` and adds the optional
`notification` member. From schema 8 on, `config_path` must be a clean
absolute path on the target platform; a relative or unclean path refuses.
Schemas 1 to 7 keep their earlier reference grammar.

## Candidate: schema 9, `/3`, `claude_env`

Schema 9 arrives as `curator.launcher-config/3` and adds the optional
`claude_env` member from the curator-spec proposal
`protocol/machine-claude-env-proposal.md` (CIP-0029, Decision 0035). That
proposal is not accepted yet, so production builds keep refusing `/3` with
the generic projection refusal; only tests open the candidate gate. `/2` and
earlier projections refuse a `claude_env` member instead of dropping it.

```json
{"extensions": {"claude_env": {"BASH_MAX_OUTPUT_LENGTH": "8000"}}}
```

Absence and `{}` select nothing. The only name is `BASH_MAX_OUTPUT_LENGTH`;
its value is a JSON string holding a canonical decimal from 1 to 65536 (a
manager policy cap, not a vendor limit). Refusals are checked in order:
shape, then names, then value, and each reports only a stable code and the
fixed path `extensions.claude_env`:

| Condition | Code |
| --- | --- |
| Not an object, null, or a contribution with a bad shape or runtime | `claude_env_invalid` |
| Any name other than `BASH_MAX_OUTPUT_LENGTH` | `claude_env_name_not_allowed` |
| Not a string, or not a canonical decimal spelling | `claude_env_value_not_integer` |
| Zero or negative, including `-0` | `claude_env_value_nonpositive` |
| Above 65536, including overflow | `claude_env_value_over_bound` |
| A non-Claude contribution that contains the name | `claude_env_runtime_ineligible` |

A projection that is not strict JSON keeps the closed generic refusal, which
is the file-level form of `claude_env_invalid`.

The selected value reaches only a native Claude Code child, after the late
launch checks, and replaces an inherited value of the same name. It is not
written to arguments, the plan, provenance or diagnostics, and it changes no
settings file, hook, permission or managed home. Other runtimes receive no
contribution, and the same machine selection does not refuse them. With a
Claude OAuth store also selected, the value is applied before the token is
injected; with a filtering gateway, the final composed-environment check runs
on the result.

The hosted route and Werkstatt stations refuse a non-empty Claude selection
before any plan or manager contact (`selected claude_env requires native
execution`), because the hosted manager owns that child's environment and
resume and has no carrier for it yet. Native launches have no resume path,
so "launch or resume" is covered by native launch only. Status reporting of
the selection is not implemented. The vendored proposal corpus lives in
`internal/launcherconfig/testdata/machine-claude-env-v1/`. A real Claude
runtime witness of the limit's effect is not part of these checks.

Metadata, subprocess, store and network fixture checks run with:

```sh
go test ./internal/launcherconfig ./internal/claudeauth ./internal/network ./internal/migrate
```

These checks cover metadata, synthetic-file resolution, mocked Keychain
responses, network admission and migration. Final-child injection and refusal
controls also run with `go test ./cmd/curator-run`. Fixtures do not establish
real Keychain access, credential acceptance by a vendor, or live runtime
qualification.
