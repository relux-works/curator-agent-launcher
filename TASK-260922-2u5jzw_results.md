# TASK-260922-2u5jzw — F-L1b producer results

Status at writing: implementation is ready for developer handoff; the task is
not accepted. The handoff runtime is responsible for running hosted CI once.

## Implementation

- `go.mod` and `go.sum` pin `skill-agents-management v0.5.22`. The launcher
  parses `--permissions native|yolo`, `--yolo`, rejects mixed/repeated forms
  and `-d`/`--danger`, and resolves source provenance through the CLI, Curator
  profile, merged launcher defaults, and interactive/headless defaults.
- Fragment v2 and defaults v2 carry the closed permission values. The resolved
  typed mode, verified release, and opaque native suffix enter
  `vendorplugin.BuildLaunchWithEnvironment` through `SpawnRequest`.
  Composition consumes the admitted native suffix and places it after prompt
  and MCP arguments; it contains no provider permission flag spelling.
- Execution refuses locked visible `yolo`, unestablished fragment transport,
  and tracked `yolo` with the specified diagnostics. Native launches print the
  choice-4 policy line only for known relaxations in inspected sources; tracked
  launches record those selectors and source under
  `works.relux.curator.effective-native-policy`. Long `permissions.allow`
  lists are summarized by rule count in stderr.
- README and CHANGELOG describe the v0.5.22 integration and permission
  behavior. The launcher SPEC was not edited.

## Choice-5 real-entry row counts

All rows below call `run(...)`, the production entry called by `main`, with a
scripted fragment resolver and compiled fake provider/ax executables. No real
provider, real `ax`, or network call was used.

| Row family | Executed rows | Result |
|---|---:|---|
| Environment mapping and mode placement | 6 | Claude/Codex native and yolo mappings, Pi native and unsupported yolo |
| Precedence and provenance | 5 | flag, profile, global, default-interactive, default-headless |
| Invalid values and v1 rejection | 5 | v1 defaults member, unknown defaults/profile/CLI values, v1 fragment member |
| Conflicting launcher permission forms | 1 | `--permissions` combined with `--yolo` |
| Curator force-native lock | 3 | flag refusal, global refusal, silent native control |
| Tracked yolo | 3 | flag, profile, and global sources refused |
| Headless/CI silence | 5 | non-TTY, `CI`, present-empty `GITHUB_ACTIONS`, tracked silence, Codex `exec` classifier |
| Legacy-fragment would-be-yolo | 3 | flag, global, and default-interactive transport refusals |
| Upstream native-policy conflicts | 23 | all six Claude permission modes, Claude selectors, Codex approval/sandbox/config selectors, and `exec` placement |
| Unknown native-policy values | 4 | Claude mode, Codex approval and sandbox values, Codex config key |
| Duplicate module-mapped permission argument | 2 | Claude and Codex; mapping read dynamically from agents-management |
| Effective-native-policy line and tracked record | 2 | untracked stderr and tracked structured record; nine allow rules summarized |

The first eight families contain 31 rows. The provider-policy, duplicate-map,
and policy-report families contain another 31 rows. The module-level source
scan passed and rejects provider bypass spellings in non-test Go sources; the
launcher-owned `--yolo` alias is allowed only in `internal/cli/cli.go`.

The fragment lattice requires `locked=true` iff `source=global` and requires
non-profile sources to carry native mode. Therefore profile yolo with the
Curator force-native lock is invalid at fragment parsing, and a v1 fragment
cannot carry a profile permission value at all. Those impossible combinations
are not fabricated; the real-entry rows test the representable lock and legacy
sources, plus rejection of a v1 fragment carrying a permission member.

## Narrowing mutants

Each mutant ran in the disposable copy
`.temp/TASK-260922-2u5jzw-mutants`; the Story worktree was not mutated. Every
focused `go test` returned exit 1 because its assertion caught the narrowed
gate. No mutant survived.

| Refusal bound | Narrowing mutant | Focused test command | Exit |
|---|---|---|---:|
| Tracked yolo from flag | Let flag-origin yolo pass the tracked check | `go test ./cmd/curator-run -run 'TestChoice5PermissionRowsThroughRealCuratorRun/tracked-yolo/flag$' -count=1` | 1, killed |
| Tracked yolo from profile | Let profile-origin yolo pass the tracked check | `go test ./cmd/curator-run -run 'TestChoice5PermissionRowsThroughRealCuratorRun/tracked-yolo/profile$' -count=1` | 1, killed |
| Tracked yolo from global | Let global-origin yolo pass the tracked check | `go test ./cmd/curator-run -run 'TestChoice5PermissionRowsThroughRealCuratorRun/tracked-yolo/global$' -count=1` | 1, killed |
| Force-native lock | Let flag-origin yolo bypass the engaged lock | `go test ./cmd/curator-run -run 'TestChoice5PermissionRowsThroughRealCuratorRun/force-native-lock/flag-yolo-refused$' -count=1` | 1, killed |
| Legacy transport | Admit flag-origin yolo without fragment-v2 transport | `go test ./cmd/curator-run -run 'TestChoice5PermissionRowsThroughRealCuratorRun/legacy-yolo-transport/flag$' -count=1` | 1, killed |
| Unknown permission value | Accept an unknown CLI permission value | `go test ./cmd/curator-run -run 'TestChoice5PermissionRowsThroughRealCuratorRun/invalid-configuration/cli-unknown-mode$' -count=1` | 1, killed |
| Conflicting permission forms | Admit combined `--permissions` and `--yolo` | `go test ./cmd/curator-run -run 'TestChoice5PermissionRowsThroughRealCuratorRun/cli-conflict/permissions-and-alias$' -count=1` | 1, killed |
| `-d` rejection | Skip the explicit `-d` refusal | `go test ./cmd/curator-run -run 'TestProductionForbiddenFlags/-d/false$' -count=1` | 1, killed |
| `--danger` rejection | Skip the explicit `--danger` refusal | `go test ./cmd/curator-run -run 'TestProductionForbiddenFlags/--danger/false$' -count=1` | 1, killed |
| Module conflict grammar input | Hide native args from the module scanner, then restore them into the admitted argv | `go test ./cmd/curator-run -run 'TestChoice5ProviderConflictRowsThroughRealCuratorRun/claude_code/permission-mode$' -count=1` | 1, killed |

The mutant tests intentionally exited non-zero; they prove that the focused
entry-point assertions detect each narrowed bound.

## Validation and gate evidence

- `go get github.com/relux-works/skill-agents-management@v0.5.22`: exit 0.
- `go mod tidy`: exit 0.
- `go list -m github.com/relux-works/skill-agents-management`: exit 0;
  selected version `v0.5.22`.
- `go test ./internal/cli ./internal/defaults ./internal/fragment ./internal/execution ./internal/plan -count=1`: exit 0.
- `go test ./cmd/curator-run -run '^TestChoice5PermissionRowsThroughRealCuratorRun$' -count=1 -v`: exit 0; emitted the row counts above.
- `go test ./cmd/curator-run -run '^TestChoice5ProviderConflictRowsThroughRealCuratorRun$' -count=1 -v`: exit 0; 23 conflict rows and 4 unknown-value refusals.
- `go test ./cmd/curator-run ./internal/composition -run '^(TestChoice5ProviderConflictRowsThroughRealCuratorRun|TestChoice5MappedBypassDuplicateRowsThroughRealCuratorRun|TestChoice5EffectiveNativePolicyLineAndTrackedRecord|TestChoice5V2FragmentCarriesPermissionMember|TestModuleSourcesDoNotSpellProviderPermissionBypassFlags|TestComposeAdmittedPlanRepositionsNativeSuffixAndRefusesDrift)$' -count=1 -v`: exit 0 before the provider-conflict table was expanded; the final full gate below reran all final tests.
- `go test ./cmd/curator-run -run '^(TestProductionPipelineGoldens|TestProductionAliasEquivalence|TestProductionForbiddenFlags|TestRunHelpGolden)$' -count=1`: exit 0.
- `go test ./cmd/curator-run -run '^(TestRunLineupEnvsPrintGroupBeforeRefusal|TestRunMapping)$' -count=1`: exit 0 after adapting the v0.5.22 Claude lineup expectations.
- First `make check`: exit 2. Build and vet passed; two tests still expected the pre-v0.5.22 Claude lineup model `claude-fable-5-1`. Updated those assertions to `claude-opus-5-5` in `internal/defaults/lineup_test.go` and `cmd/curator-run/main_test.go`.
- Final `make check`: exit 0. This ran `go build ./...`, `fmt-check`, `go vet ./...`, `go test ./... -count=1`, and `go test ./... -count=1 -race`; every package passed.
- `git diff --check`: exit 0.
- The repository has no separate linter target; formatting and `go vet` are the configured static checks and both passed in `make check`.
- Hosted CI was not run manually. The handoff runtime runs the configured hosted landing gate once.

## Follow-ups and handoff notes

- The launcher SPEC still names `permission-grammar-v1` in §4.4; upstream
  v0.5.22 uses grammar v2 for Claude/Codex and v1 for Pi. The SPEC also does
  not enumerate the stored-policy inspector's inspected/not-inspected source
  families. These are SPEC follow-ups; no SPEC change was made in this task.
- The v0.5.22 pin changes Claude's selected fallback model; the two affected
  existing expectations above were updated as the only unrelated call-site
  adaptation.
- The brief prohibits `LOGBOOK.md` edits. The relevant lineup change and SPEC
  follow-ups are recorded here and in the task board notes instead.
- Work remains uncommitted in the assigned Story worktree. Hosted CI and the
  role status transition are delegated to `task-board handoff`.
