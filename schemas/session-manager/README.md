# Session-manager protocol module

These closed draft 2020-12 schemas describe `curator-session-manager/1`
requests, responses and local correlation records. References are absolute
URNs resolved from the embedded `$id` catalog; external validators should
register these documents by `$id`. The launch/resume URNs
have version 1.0.0. The wire members follow FRONT-DOOR §§5.1–5.7 and §11,
including the scalar, reserved-name, doorbell and digest-spelling clarifications.
`profile.lock_sha256` is bare lowercase hex (64 characters);
`profile.fragment_digest` is `sha256:` plus 64 lowercase hex characters,
matching the production fragment resolver without conversion.

`Validate(name, bytes)` accepts the filename or published URN through the same
wire validator, and applies a standard-library-only strict JSON reader: duplicate members, invalid UTF-8, unsafe integers,
trailing documents and documents over 64 KiB are rejected. Only the JSON
Schema subset used by these files is implemented; unsupported schema keywords
are errors. This is a validator for this published schema set, not a general
JSON Schema implementation.

Launch wire checks bound `child_env` values in UTF-8 bytes (JSON Schema
`maxLength` counts characters). Provider semantics belong to the consumer:
**a plugin must apply the `native_tail` selector check using its own provider
module**. The tail must remain unchanged and selector-free under that module's
typed-intent elevation. The launcher applies this in
`internal/sessionmanager/client.ValidateRequest` and returns the same
`launch_plan_invalid` refusal for selectors; `Validate` alone grants no launch
authority. The manager still admits model, effort,
service tier, weights and credentials; structural validation grants no such
authority. The independent exact-plan transport retains SPEC §4.9's 1 MiB,
depth-16 bounds.

Nullable members remain required. A missing member and an explicit `null`
are different. Outcome members `session` and `code` are present only for
`started` and `failed`, respectively. Refusal details are optional members of
a code-specific closed vocabulary; launcher-sourced paths, identities and field names
are deliberately unavailable to plugin responses. Unknown describe capability
strings confer no authority. RPC refusals and historical failed outcomes admit only plugin-owned codes.
The fd-4 transport retains the existing SPEC §4.9 registry unchanged. UTC
timestamps use whole seconds, exactly `YYYY-MM-DDTHH:MM:SSZ`.

Session responses carry the envelope; list rows use `session-record` without
one. Every record carries `readable`. Unreadable records preserve their index
handle, manager and labels, use state `unknown` and an inactive doorbell, and
carry null lifecycle fields. Unknown evidence never becomes `down` or `idle`. A started outcome embeds an
envelope-free record with `readable:true`; an unreadable operation result must
use outcome `unknown`.

Every file in `vectors/` is an array of `{name, valid, document}` fixtures.
They enumerate nullable/optional combinations, code-specific detail presence,
doorbell relations, bounds and rejection cases, with valid controls. Every vector
runs through both the schema filename and its published URN. The four
selector-tail documents remain in the corpus as structurally valid shapes;
launcher regression tests require their provider-semantic refusal alongside
accepted value-position controls.

```sh
go -C schemas/session-manager test ./... -count=1
go test ./internal/sessionmanager/client -race -count=10
python3 .scripts/plugin-client-mutants.py .temp/plugin-client-mutants
```

The Go checks print results on stdout. The mutation driver writes per-control
logs and `summary.tsv` under the chosen evidence directory and restores the
candidate source bytes after every control. Run it alone, before acceptance
checks; it temporarily edits the working tree.

The editable `werkstatt-record-v1` requires its schema discriminator. Its
`child_env` shares the launch rule. Liveness and operation files have no
schema member. A pre-start operator override is the sole liveness shape with
null session/generation/start/heartbeat; it must be down with inactive doorbell
and an abandonment tombstone. `readable` belongs only to manager projections.
`werkstatt-status-v1` is a bare array of exactly five-member rows; its nullable
manager projection retains the original protocol envelope and explicit nulls.

Label values use `[A-Za-z0-9._:/@+=-]{1,128}` so raw a2a identities
containing `=` fit without encoding. Selector value grammar remains unchanged.

The Go module path is
`github.com/relux-works/curator-agent-launcher/schemas/session-manager`.
It has no dependencies outside the standard library. Plugins require this
nested module directly at a published module version, without requiring the
launcher root module. The launcher also requires the published module without
an in-tree `replace`, so version-suffixed Go installs work, and keeps its
provider dependencies separate. `Documents`, `Limit`, `Validate` and
`ReservedChildName` retain their APIs. `CanonicalJSON` exposes the same strict
reader and CCJ-1 canonical bytes for consumers that need them; it omits a
top-level `sig` member as CCJ-1 specifies, and does not impose the protocol
byte limit. `Validate` imposes that limit.

`make build`, `make vet`, `make test` and `make race` in the launcher include
explicit nested-module steps, so `make check` and hosted CI cover both modules.
The module tests inspect `go list -deps -test -json` in a temporary standalone
copy with workspace and network resolution disabled. Its known-bad control
adds a Go import of the launcher internal fragment path with a local fixture
replacement and requires the specific non-standard-dependency violation.
Launcher alignment tests compare both readers on every existing fragment
corpus JSON file and every protocol vector, including strict-reader negative
neighbors and byte-for-byte canonical output.


## Re-pinning the launcher

The launcher root module pins a published version of this module. Its
`internal/schemaparity` tests resolve that version with `go list -m -json`
and compare every file and directory against this checkout, byte for byte.
They require no Git history, so depth-one CI checkouts work. Resolution or
read failures refuse; there is no fallback to an in-tree replacement.
The only excluded path is `schemas/session-manager/README.md`, whose prose
may advance separately. Other READMEs, `go.mod`, Go source/tests, schemas and
vectors are all compared; added and removed paths also fail.

After editing the contract, publish the nested module commit first, then
select its version in the launcher root (replace `<version>` below):

```sh
go mod edit -require=github.com/relux-works/curator-agent-launcher/schemas/session-manager@<version>
go mod download github.com/relux-works/curator-agent-launcher/schemas/session-manager
go test ./internal/schemaparity -count=1 -v
make check
```

Keep the in-repo contract tree identical to that published version and review
the schema requirement and checksum changes together. Do not restore a local
`replace`: it prevents version-suffixed `go install`. The guard is part of
`go test ./...`, hence `make test`, `make race`, `make check` and hosted CI.
Its results go to stdout; temporary comparison fixtures use `t.TempDir`.
