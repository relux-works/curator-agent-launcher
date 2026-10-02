# Changelog

## Unreleased

### Added

- Direct `curator run --network <profile>`: resolve and validate the
  explicit selection through `curator-network-profiles v0.1.0` after
  plan admission, probe the proxy once within a bounded preflight, and
  apply the patch last in composition to the direct child environment.
  Managed launches print a `curator-run: network: …` provenance line
  carrying the binding Record once composition succeeds. Support is an
  explicit allowlist of verified (adapter, harness, build, entrypoint)
  tuples, which holds exactly one verified tuple —
  `(generic-env-v1, claude-code, 2.1.287, exec)` — so every other
  `--network` launch refuses with `network_scope_unsupported`. Tracked
  launches refuse the same way. See SPEC §4.4b and §6.

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
