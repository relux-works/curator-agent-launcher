# Changelog

## Unreleased

### Added

- Direct `curator run --network <profile>`: resolve and validate the
  explicit selection through `curator-network-profiles v0.2.1` after
  plan admission, probe the proxy once within a bounded preflight, and
  apply the patch last in composition to the direct child environment.
  A named `kind = "direct"` profile clears inherited proxy variables
  and sets nothing, skipping every probe step. Managed launches print
  a `curator-run: network: …` provenance line carrying the binding
  Record once composition succeeds. Support is the STRICT network
  policy: an explicit allowlist of verified (adapter, harness, build,
  entrypoint) tuples, matched at the actual admitted entrypoint, which
  holds exactly one verified tuple —
  `(generic-env-v1, claude-code, 2.1.287, exec)` — the one-shot
  `claude -p` shape. Every other `--network` launch refuses with
  `network_scope_unsupported`, including interactive launches, which
  the pinned verification expressly excludes. Tracked launches
  refuse the same way. See SPEC §4.4b and §6.
- Host selection and the hosted session-launch handoff
  (TASK-261004-38ba6c). `curator-run` accepts `--hosted`, `--native`,
  and `--untracked` in one host request slot, `resume [SES-HANDLE]`
  and `--resume <id>` selectors, and the existing `--network <profile>`.
  `curator-run-defaults-v3` adds the per-environment `host` member
  (v1 and v2 stay accepted); machine lock, flag, operator default,
  and the implicit native default resolve per the SPEC §4.8 table.
  Explicit `--hosted` builds the same plan once and hands the
  versioned `session-launch-plan` 1.0.0 payload with its content
  digest to the `task-board` receiver over the private fd3/fd4
  transport instead of exec'ing; machine hosted defaults refuse
  `session_host_default_not_ready` until upgrade-without-hangup while
  operator hosted defaults are admitted (Q-D1a = yes, 2026-10-05),
  a `--hosted --network` selection refuses `network_scope_unsupported`
  (exit 16; the native `--network` path above is unchanged),
  and non-Claude environments refuse
  `session_host_provider_unsupported`. Hosted permission resolution
  follows the configured flag/profile/global ladder with the `yolo`
  built-in default on every stdio shape, and hosted `yolo` builds and
  exports with `execution_profile` `yolo` (Q-D3, 2026-10-05, literal:
  headless signals never change the default; Decisions 0018/0013
  amendments pending). Phase 1 admits
  Claude only. No flag and no default stays native with no
  session-host probe. A piped launcher stdin refuses before the plan
  build; an attached plan stdin refuses after it. The fd4 status
  channel accepts only closed refusal envelopes (135-member registry,
  literal messages, D0/D1/D2 details) and normalizes anything else to
  `session_host_protocol_error` with empty details; truncated frames
  refuse `session_host_unavailable` unless a malformed frame already
  poisoned the channel, which stays `session_host_protocol_error`. A
  missing or invalid terminal descriptor refuses
  `session_host_terminal_required` before the plan build with zero
  receiver lookup, and the launcher marks and validates both fd3/fd4
  descriptors before contact. At most 64 lookup names project.
  Hosted diagnostics never copy native argument values (conflict names
  the channel, unknown policy mode reports the fixed sentinel). The
  payload carries the native session name and remote-control intent
  read from the module's typed `Plan.Session` of the same plan (RC
  indices carried only while the composed argv equals the plan argv,
  no provider-flag parsing);
  an authored tail naming two different sessions refuses
  `plan_refused` from the module before any receiver contact.

### Changed

- Pin `github.com/relux-works/skill-agents-management v0.5.53` for the
  resume typed-intent API, the closed `claude-restart` template, the
  pinned hosted schemas, `Plan.ExportSeal`, and the typed
  `Plan.Session`. Claude plans carry the module's unconditional
  `--disallowedTools=AskUserQuestion` denial after the prompt channel;
  update direct and tracked Claude goldens. Hosted Codex argv and
  environment are unchanged; the launcher registers no local-model
  vendor, so no fixture needs `model_catalog_json`.

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
