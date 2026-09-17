# Changelog

## 0.1.0 — unreleased

Release-readiness candidate; source builds report `0.1.0-dev`, against
SPEC `0.4.0-draft`. No tag or release is created by this work.

### Added

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

### Dependencies

- `github.com/relux-works/skill-agents-management v0.5.13`, the upstream
  agents-management module, with no replace directive or workspace override.
  Curator and ax remain CLI contracts.
