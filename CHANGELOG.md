# Changelog

## Unreleased

### Fixed

- Preserve exact fractional manager registration in Werkstatt liveness and
  automatic status recovery, matching the journal and lease holder. A started
  session retains its handle and prepared protection at the existing bind gate;
  unrelated liveness timestamps keep their whole-second contract.

### Added

- Read the operator model catalog `~/.curator/catalog.toml` (SAM
  `model-catalog/1`) before building the spawn-plane registry, so a new model or
  reasoning word is a configuration edit with no launcher or module release.
  Catalog rows are admitted when named by flag or `defaults.json`, and never
  picked by the lineup fallback; a system whose candidates are all catalog rows
  refuses `defaults_unresolvable` instead of choosing one. A missing catalog
  changes nothing; an unreadable, malformed, conflicting or unsupported one
  refuses every launch with `plan_refused`, never falling back to the module's
  rows alone. The catalog is read only on launch paths, after help, version,
  flag grammar and `session-manager` administration, so a broken file never
  takes those away; Werkstatt recycle checks it before it stops the running
  session, and the first load result, a refusal included, holds for the whole
  process. Selector candidates and Werkstatt admission use the same
  effective rows: catalog effort words on module models are offered, and a
  catalog model is offered only when `--model` names it. The session-manager README now states that model and effort are
  admitted once, by the launcher, and forwarded by the manager as opaque strings.

- Specify a proposed same-user executor entry, descriptor custody, closed
  records, immutable assignment/atomic attempt admission, failed-conflict interlock,
  cancel/start ordering, allocation recovery and original stop deadlines with
  inert manual cases.
  This source contract ships no member command or supported native tuple.

- Document the separate same-user trusted development-member non-chat target:
  fresh native context, no child messaging/board presence, exact input/result
  admission and a qualification plan. This documentary amendment adds no
  implemented command or qualified runtime capability.

- Require public Curator home leases for hosted launch/resume, with a durable
  acquire-first recovery journal, exact route/registration evidence, terminal
  no-RPC settlement and bidirectional Werkstatt allocation checks. Native
  execution makes no lease call. Start currently retains prepared protection
  and refuses `process_incarnation_unavailable`: session-record v1 cannot
  supply authoritative OS incarnation for bind. No release/break is sent;
  explicit abandon retains evidence and prints a separate operator hint.

## 0.6.0 — 2026-10-27

### Removed

- Remove the ax tracked execution route, `ax.json` policy parsing,
  `--name`, `--ax-profile`, tracked permission defaults and handoff diagnostics.
  Any leftover `ax.json` now refuses `ax_tracking_removed` (exit 2) before
  resolve or launch. Migration lists it, backs up regular files and removes
  them; non-regular entries require manual removal. `--untracked` remains a
  deprecated `--native` alias for two releases with one warning.

### Added

- Add a signed-tag-gated artifact pipeline, with optional macOS signing and
  notarization requiring all five configuration fields or none. Generate a
  Homebrew formula without pushing a tap, and package Linux deb/rpm release
  assets without publishing a package repository.

- Add read-only `session-manager observe <manager-id> --session <SES-handle>
  --json`: one closed observation with the registration incarnation, validated
  session record and launcher UTC timestamp, without local state access or
  reconciliation. Unknown/unreadable evidence remains explicit. Publish the
  observation schema and vectors in the standalone session-manager module.

- Enforce one profile per Werkstatt conversation: a known profile change in
  continue mode refuses early with `werkstatt_profile_changed` (exit 1, `id`
  only). Recycle, `up --new` and fresh mode start new generations. Optional
  liveness and journal profile evidence survives refresh and crash recovery;
  older files retain unknown evidence and the manager's existing guard.

### Changed

- Pin skill-agents-management v0.5.74 (was v0.5.68). It adds
  `permission-grammar-v3` for codex-cli 0.164.0, so `--permissions yolo` maps
  on that release instead of failing closed; 0.153.2 and other rows are
  unchanged.
- Diagnose unwritable explicit registry scopes as
  `session_manager_registry_unwritable` (exit 2, directory `path` and `scope`),
  separately from invalid/unreadable registry content, before any lock or write.

- Add `session-manager use` and explicit mutation `--scope user|system` with
  no fallback. Omitted scope preserves layer selection; list/show report the
  layer read in text and CLI JSON, without changing the registry file schema.

- Add `werkstatt update` for task/queue dispatch edits and `werkstatt recover`
  for manager rebind or abandoned-operation overrides, with the same locking,
  CAS, diagnostics and status output as the former mutating status flags.

### Deprecated

- `session-manager select` is deprecated in 0.6.0; use `session-manager use`.
  The alias preserves stdout and effects with one stderr warning. Removal is
  no earlier than 0.8.0, two releases after deprecation.

- Mutating `werkstatt status` flags are deprecated in 0.6.0, removed no earlier
  than 0.8.0: removed two releases after it was deprecated. Use `update` or
  `recover` instead. Aliases preserve stdout and print one warning on stderr.

## 0.5.0 — 2026-10-27

### Added

- Optional `profile_lock_sha256` in Werkstatt records. `up` refuses a resolved
  profile lock mismatch before hooks or launch/resume, preserving liveness
  and omitting both hashes from the diagnostic.

## 0.4.0 — 2026-10-22

### Added

- A session-manager registry with register, list, select and unregister
  commands, layered configuration and serialized mutations. Launches use the
  selected manager without searching PATH for a host.
- A validated session-manager protocol client with capability checks,
  launch/resume/attach, durable operation reconciliation and owner binding.
  A durable fake manager and conformance suite qualify the contract without
  claiming a real provider integration. The standard-library-only protocol
  schemas are also available as a standalone Go module.
- Hosted Werkstatt up, down, recycle, status and list, with durable liveness,
  generation fencing, explicit manager rebind and abandoned-operation recovery.
- An external model/effort selector with bounded enforcing decisions and
  asynchronous shadow observations; invalid or refused enforcing decisions
  never fall back to a different choice.
- Hosted doorbell requests, staged Claude hook installation and separate
  manager/a2a status evidence. This release does not ship `doorbell-codex/1`.
- Static nested help for the registry and Werkstatt commands, plus parse
  errors that name the grammar token and its actionable help command.
- Reproducible release-binary and two-checkout verification targets, with
  inert, validated build inputs. Release signing and packaging remain the
  distributor's responsibility.

### Changed

- Activate `launch-delegated/1` for Claude and Codex after the Q1 sign-off.
  A registered and selected conforming manager supplies hosted execution.
  `auto` without a registered or selected manager launches natively with a
  one-line notice; explicit hosted refuses `session_host_missing` with its
  reason. Explicit native execution does not contact a manager.
- Move launcher configuration to `swarma/launcher`, with per-file legacy-path
  fallback and transactional migration. Machine `host_mode` replaces the
  withdrawn per-environment host default. Parse arguments before reading
  configuration; help/version read none.
- Pin agents-management v0.5.68. Read v4 fragments and require v4 for Muse,
  preserving its exclusive root-context and file credential-store policy.
  Unsupported Codex file/Keychain credential application refuses explicitly.
  Hosted Muse forwarding is tested against the fake manager; real Muse
  hosting, lazy registration and exact-plan activation remain proposals
  subject to FRONT-DOOR's own gates. See README's "Front doors" section.

### Fixed

- Use the published standalone schema module without a local replacement for
  versioned Go installs. A shallow-checkout-safe parity guard requires all
  contract files to match the pin; only the module README prose is excluded.
- Resolve registry paths correctly with a symlinked temporary directory and
  serialize concurrent registry edits instead of losing another writer's
  update.
- Validate delegated profile digests and launch semantics consistently with
  the published schema identities; retain operation journals until settled
  liveness is durable and preserve typed refusals from owner reads.
- Keep shadow observations independent of launch completion while preserving
  hosted handles and stderr destinations; reject selector refusals that also
  carry a choice.
- Stage and commit Claude hooks through a pinned directory, preserve unrelated
  settings, and refuse visible target changes or a substituted directory
  before start. This is a best-effort conflict check, not an atomic writer fence.
- Keep draining reason and doorbell uncertainty in readable manager evidence;
  retiring a fake session clears its draining reason.

## 0.3.0 — 2026-10-13

### Changed

- Pin `github.com/relux-works/skill-agents-management v0.5.62`.
  Muse interactive launches admit any well-formed build attested by the
  selected binary, including 1.4.6, with no release table. The optional
  frozen-copy tuple stays supported; without it the binary resolves on the
  handed PATH. Admit the `muse-wonder` alias and Curator launch fragment v3;
  the module lineup now defaults Muse to `muse-wonder-1.0-contributor`
  with `xhigh` effort.
  Preserve the probed `ToolRelease` claim and refuse a mismatch with the
  binary's attestation. Muse yolo requires bounded help evidence from the
  selected binary; native contributes no posture override.
- Hosted launch is unavailable until the swarma dispatcher path ships.
  Production `--hosted` receiver lookup fails closed with the existing
  `session_host_missing` (exit 1), before any handoff, even with `task-board`
  on PATH. Earlier validation refusals remain in force; machine and operator
  hosted defaults cannot start a hosted launch. `--native` and `--untracked`
  retain native execution and the explicit ax bypass.

### Added

- Direct `curator run --network <profile>` as already landed, through
  `curator-network-profiles v0.3.1`: explicit catalog/selection validation,
  bounded proxy preflight, final child-environment patch and provenance.
  A named `kind = "direct"` profile clears inherited proxies and skips
  probing. The default optimistic policy admits known vendor builds as unqualified,
  subject to fail-closed known-bad policy and the independent child-scope
  ceiling (generic adapter, Claude print). `strict` and sensitive-egress
  profiles require independent qualification; the qualification allowlist
  remains empty in this release. `pinned:<sha256>` also requires that exact
  binary but does not qualify it. Muse network tuples remain outside the
  child-scope ceiling; interactive, tracked and hosted network launches
  remain unsupported.
  See SPEC §4.4b and §6.

## 0.2.0 — 2026-10-02

The binary reports `0.2.0`; the specification remains `0.5.0-draft`.

### Added

- Register the Muse launcher/provider mapping and read v3 launch fragments
  with four XDG parents, preserving inherited `HOME`. The v0.5.37 module
  admits interactive Muse root-session plans with release probing and native/yolo
  permissions: native adds no posture flag; yolo adds `--yolo` once.
  Unlisted releases fail closed. Add native, yolo, and yolo-alias Muse goldens
  for this newly admitted interactive path.

### Changed

- Pin `github.com/relux-works/skill-agents-management v0.5.37`. Claude plans
  now own `CLAUDE_CODE_ENABLE_PROMPT_SUGGESTION=false`; update direct and
  tracked Claude goldens, delivering the launcher half of curator#102.
- Keep reserved fragment members out of the shared execution context carrier.
  A valid `path_prepend` still parses and contributes to the original digest,
  without changing PATH or causing Claude/Codex launches to refuse.
- Claude/Codex file-backed prompt and MCP channels use the shared typed Curator
  construction API. Plugins own argv order; native arguments stay last and
  verbatim. Native overrides colliding with fragment prompt/MCP channels now
  refuse with a diagnostic naming the arguments and channel. Codex profile
  overrides conflict with a fragment MCP layer. Non-colliding arguments retain
  pass-through behavior. v2/v3 context projects the v1 subset while preserving
  permission mapping and transport metadata.
- Refuse `openai-infra` and `anthropic-infra` as deprecated launcher aliases at
  the `curator-run` entry point; canonical environment ids and `claude`/`codex`
  aliases keep their existing behavior.

### Security

- Validate `defaults.json` and `ax.json` before use: reject symlinks, files
  owned by a different identity than their configuration directory, and
  group/world-writable POSIX files; inspect Windows DACL write grants
  (TASK-260916-1ihonr).

## 0.1.0

First tagged release. The binary reports `0.1.0`; the specification remains
`0.5.0-draft`.

### Release

- Install the versioned Go module with
  `go install github.com/relux-works/curator-agent-launcher/cmd/curator-run@v0.1.0`.
- No prebuilt binaries are published; the tagged module is built locally by Go.

### Added

- SPEC 0.5.0-draft and README permission interface per Decision 0018
  (TASK-260922-1zfqq0): typed permission mode, fragment-v2 transport,
  precedence, headless/tracked rules, diagnostics, and native-policy
  provenance. Runtime behavior and its tests are the companion F-L1b leaf.

- F-L1b permission-mode resolution and transport (TASK-260922-2u5jzw):
  CLI modes and alias, source-aware precedence, v2 fragment transport, tracked
  and force-native refusals, release-bound agents-management composition,
  headless classification, and effective-native-policy reporting.

- Closed launcher CLI parsing, opaque native arguments, usage diagnostics and
  initial hosted CI (PR #4).
- Curator fragment resolution with repair, closed validation, CCJ-1 digest and
  pinned conformance corpus (PR #5).
- Explicit environment/system/provider mapping for Claude Code, Codex CLI and
  native Pi; unsupported environments refuse (PR #6).
- Composition of admitted argv, owned environment and stdin, including MCP
  transport and collision handling (PR #9).
- Explicit system-prompt opt-in and fresh Pi managed-home file checks (PR #10).
- Direct child execution, signal/terminal handling and tracked launch-plan
  transport verified with fake ax only (PR #11).
- Stable diagnostic families and conformance gates (PR #12).
- Interactive plan admission and separate managed-home provider-limit checks
  (PR #13).
- Per-member flags/operator/machine/lineup defaults with machine locks and
  ordered Pi runtime preference (PR #14).
- Production main pipeline connecting resolution, defaults, admission,
  composition and direct/tracked execution, with entry-point goldens (PR #15).
- Installation/configuration help and an opt-in `Test (rose-air)` ARM64 CI lane;
  hosted build, formatting, vet, tests, race and goldens remain enabled.
- E4: the §4.3 stderr line-group reports the resolved provider path
  (`curator-run: provider: path=<absolute path>`, symlink-resolved own
  executable, `path=unavailable` fallback that never fails the launch)
  before the defaults line at every launch; path-only in this revision
  (SPEC `0.4.0-draft`).

### Corrected

- Plan/environment value-contract errata (PR #7) and Pi prompt precedence
  errata (PR #8). Pi has no MCP channel.
- SPEC `0.4.1-draft`: fold the four 0.2.1-review minors
  (TASK-260906-2t2t6w) — the resolve pass-through clause for unmapped
  `--repair` diagnostics (§4.1/§6), the ordered pre-launch checks with
  the binary check first (§4.6; the implementation now runs
  binary → layer stat → §5 probe and reports the first failure), the
  silent-MCP-absence residual for `claude_code`/`opencode` (§9), and a
  version-pin test that reads SPEC.md and README.md.

### Dependencies

- `github.com/relux-works/skill-agents-management v0.5.22`, the upstream
  agents-management module, with no replace directive or workspace override.
  Curator and ax remain CLI contracts.
