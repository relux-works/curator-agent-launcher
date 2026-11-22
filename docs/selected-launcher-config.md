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
Schemas 7 and 11 supply optional closed `proxy` and `claude_auth` members under
`curator.launcher-config/1`. Startup verification and verification-centre
configuration stay in Core; neither enters the launcher projection. Schema 8
retains its separate `curator.launcher-config/2` notification contract. Null,
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

Metadata, subprocess, store and network fixture checks run with:

```sh
go test ./internal/launcherconfig ./internal/claudeauth ./internal/network ./internal/migrate
```

These checks cover metadata, synthetic-file resolution, mocked Keychain
responses, network admission and migration. Final-child injection and refusal
controls also run with `go test ./cmd/curator-run`. Fixtures do not establish
real Keychain access, credential acceptance by a vendor, or live runtime
qualification.

For docking against a separately qualified Core executable that supports machine
schema 11, run:

```sh
CURATOR_RUN_CORE_TEST_BINARY=/absolute/path/to/curator go test -p 1 -parallel 1 -count=1 ./cmd/curator-run -run '^(TestStartupCoreDocking|TestStartupCoreHardlinkDocking)$'
```

This optional Unix suite installs profiles only into temporary fixture state,
redirects every native adapter home with its supported selector, and preserves
the real `HOME`. It runs the actual Core subprocess and production selected
reader, fragment resolver and launcher entry, ending at an inert Go child.
Fixtures cover all four levels, frozen complete evidence, legacy native and
manifestless locks, mismatch warnings/refusals, strict same-pin snapshot repair,
unsafe payload links, floor weakening, stderr delivery and preserved frozen
evidence after startup. Paired intact context/MCP cases start the inert child at
every level; an outside-snapshot hardlink then must refuse before the builder
or child and preserve the complete fixture state and both file identities.
It skips when
the executable is not supplied; a normal test run therefore does not claim this
cross-component qualification. It does not install Core, use real credentials,
launch a vendor runtime or qualify network Git transport.
