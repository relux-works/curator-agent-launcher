# Session-manager wire schemas

These closed draft 2020-12 schemas describe `curator-session-manager/1`
requests, responses and local correlation records. References are absolute
URNs resolved from the embedded `$id` catalog; external validators should
register these documents by `$id`. The launch/resume URNs
have version 1.0.0. The wire members follow FRONT-DOOR §§5.1–5.7 and §11,
including the scalar, reserved-name and doorbell clarifications.

`Validate(name, bytes)` loads the embedded schema and applies the repository's
strict JSON reader: duplicate members, invalid UTF-8, unsafe integers,
trailing documents and documents over 64 KiB are rejected. Only the JSON
Schema subset used by these files is implemented; unsupported schema keywords
are errors. This is a validator for this published schema set, not a general
JSON Schema implementation.

Two launch checks also require semantic validation in Go: `child_env` values
are bounded in UTF-8 bytes (JSON Schema `maxLength` counts characters), and
`native_tail` must remain unchanged and selector-free under the runtime
module's typed-intent elevation. The manager still admits model, effort,
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
doorbell relations, bounds and rejection cases, with valid controls.

```sh
go test ./schemas/session-manager -count=1
go test ./internal/sessionmanager/client -race -count=10
python3 .scripts/plugin-client-mutants.py .temp/plugin-client-mutants
```

The Go checks print results on stdout. The mutation driver writes per-control
logs and `summary.tsv` under the chosen evidence directory and restores the
candidate source bytes after every control. Run it alone, before acceptance
checks; it temporarily edits the working tree.
