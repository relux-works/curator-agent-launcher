# curator-agent-launcher

The Curator agent launcher: the **execution plane** that composes three
independent contracts into one exec —

- the **spawn plane** (`agents-management`): which agentic system, model,
  reasoning effort, and vendor, and whether provider limits admit a launch
  right now — consumed through `vendorplugin.BuildLaunch` for the plan
  and an explicit `providerlimits.Store.AvailabilityFor` check, never rebuilt;
- the **context plane** (Curator): the launch environment fragment obtained
  through `curator env resolve --repair --format json` and merged into the child
  environment;
- the **session plane** (`ax`): when the machine's `ax` integration is
  configured, every launch goes through `ax`'s instrumentation so the
  session is tracked from birth.

The launcher holds no session state of its own — *fire* is the launcher's
verb, *manage* is `ax`'s. The full contract, including the CLI surface,
the composition algorithm, the system-prompt opt-in and its warnings,
diagnostics, and versioning, lives in [SPEC.md](SPEC.md).

## Status

**Specification draft — partially implemented.** The specification is
`0.3.0-draft`. This build implements SPEC.md §3, the closed CLI surface
(`internal/cli`) — flag grammar, the `--` boundary with a verbatim native
tail, the `usage` diagnostic family with exit 2, and the informational
flags — and §4.1, fragment resolution (`internal/fragment`): the
`curator env resolve <env-id> [--profile <name>] --repair --format json`
subprocess with Curator's stderr forwarded verbatim, the closed
`launch-env-fragment-v1` parser (CCJ-1 reader rules, the conformance
schema, and the adapter channel registry), the CCJ-1 digest computed from
the parsed object, and the `resolve_*` diagnostic family. §4.2 maps the
resolved environment to its system/provider pair (`internal/mapping`):
`claude_code` → `claude-code`/`claude`, `codex_cli` → `codex`/`codex`,
`pi` → `pi-native`/`pi`. Unsupported IDs (including known `opencode`)
refuse `env_unsupported` with exit 1 before later stages. The executable wiring for the later
steps (§4.3–§4.6), main integration of system-prompt policy (§5), and the
`defaults.json`/`ax.json` file family (§4.7) are not delivered: a
well-formed launch invocation parses, resolves its fragment (repairing the
managed home when Curator finds it stale), maps its supported system/provider pair, and is then refused with exit 1
and a `not_implemented` line that reports the mapping, home and fragment digest,
launching nothing. A resolve failure is reported as its `resolve_*` code
with exit 1.
Because no `ax.json` is read yet, the binary always parses as an untracked
machine, so `--ax-profile` is currently always a usage error and `--name`
is accepted without effect.

## Composition API boundary

`internal/composition.Compose` implements the §4.5 value API over an already
admitted `agentic.Plan`, its original system/request, a validated fragment,
and an already selected/encoded `PromptApplication`. It calls only
`System.ChildEnv(nil, sameRequest)` to obtain owned literals; it never builds
another plan. Argv retains plan → prompt → MCP → native order. Native arguments
are uninspected, including duplicate `-p` and operator permission flags.
`Binary` and `WorkDir` are preserved. Direct `Env` starts from all of `Plan.Env`;
tracked JSON omits that environment, executable, work directory and raw stdin.
It includes only argv suffix, owned literals, disjoint lookup names and D4 stdin.
`RawStdin` preserves bytes for direct execution; attached empty is distinct from
unattached. `Warnings` contains names only, and both launch modes must print
these warnings to stderr.

The only direct module dependency is the real `skill-agents-management`
`v0.5.10` release (commit `12f443d10bc217ca7a48e2edab19c739f441df9c`),
which requires Go 1.25.5. No replacement, workspace override or pseudo-version
is used. Pi-shaped input values test composition only: they do **not** claim
native Pi admission in that release. Integration must use the later real
operator-tagged native Pi release before making that claim.

`Value.CheckLaunchBoundary` freshly checks the codex MCP layer for a readable
regular file, distinguishing missing from dangling, unreadable and nonregular.
It never repairs or silently drops MCP flags. The execution Story must call it
**immediately before both direct process creation and ax handoff**, with the
binary and §5 file-kind checks; calling it during composition is insufficient.
The pathname check has a residual replacement window before process creation.
No main call site or execution guarantee is claimed here. Full pipeline tests,
BuildLaunch admission, model/default resolution, §5 prompt API integration,
tracked schema/extensions, stderr delivery and actual exec/ax remain later
stories' obligations. The existing executable still refuses `not_implemented`.

Reserved `path_prepend` parsing and hashing remain unchanged. Composition does
not transform PATH for that reserved field or claim managed command-root
support; environments §9.4 and the future skill-command-roots proposal bound it.

## System-prompt API boundary

The reusable `internal/systemprompt` API implements §5 selection and encoding,
typed `sysprompt_channel_unavailable` / `sysprompt_file_unreadable` refusals,
and late Pi file validation. `Select` accepts a validated fragment and an
explicit `fragment.Semantics` (empty means no opt-in), returning only channel
argv/env for composition. No system-prompt variable channel exists in the
closed revision-1 registry, so `Selection.Env()` is empty; no adapter support
is inferred. Codex uses `-c` plus a TOML-quoted `model_instructions_file` value.
That native override remains **docs-confidence** in the accepted A0 evidence
(Codex 0.153.4), while encoding is covered by exact-argv tests.

**Execution Story obligation:** main still does not call this API. Immediately
before **every** tracked handoff or untracked exec it must call
`systemprompt.PrepareLaunch(fragment, optIn)`, refuse its errors, compose
`Selection.Argv()` / `Selection.Env()`, and emit every returned warning line
to stderr. It must not cache the launch-boundary result. The operation rechecks
Pi's selected polymorphic flag path and both registry home filenames even
without a system-prompt section. `ProbeFiles` also exposes the independent
home probe; `FormatWarnings` is pure. Warnings distinguish flag suppression
from conditional discovery under native-flag and trusted-project precedence.
The API writes nothing and does not inspect native argv or project files;
probe-to-exec races and native source selection remain outside its evidence.

## Install and discovery

The launcher ships the `curator-run` executable. Curator dispatches
umbrella subcommands by the established external-subcommand convention
(the `git`/`kubectl`/`docker` plugin model): a subcommand Curator does not
implement resolves to an executable named `curator-<name>` on `PATH`.
Installing `curator-run` on `PATH` therefore makes `curator run …` work;
the binary is equally invocable directly as `curator-run …`. Curator
carries no knowledge of the launcher beyond that discovery rule.

## Development

```bash
make build      # go build ./...
make fmt-check  # gofmt -l over cmd and internal; fails on unformatted files
make vet        # go vet ./...
make test       # go test ./... -count=1 (includes the CLI goldens)
make race       # go test ./... -count=1 -race
make check      # all of the above
```

CI (`.github/workflows/ci.yml`) runs the same targets on `ubuntu-latest`
and `macos-latest` with the toolchain from `go.mod`. There is no release
or tag workflow yet.

### Tools

| Tool | Purpose | Entry point | Output |
|---|---|---|---|
| `go` (build, vet, test, race) | build and behavioral suite | `make check` and the targets above | stdout; nothing written to the tree |
| CLI goldens | frozen accepted/rejected §3 shapes | `go test ./internal/cli -run TestGolden -update` to regenerate, then review the diff | `internal/cli/testdata/cases.golden` |
| `.scripts/cli-mutants.sh` | narrowing-mutant harness for the §3 gates: each mutant weakens one gate to admit one rejected shape and the named test must fail | `.scripts/cli-mutants.sh [evidence-dir]` | per-mutant logs and `summary.tsv` under the evidence dir (default `.temp/cli-mutants/`); the source tree is restored on exit |
| `.scripts/fragment-mutants.sh` | narrowing-mutant harness for the §4.1 gates (argv, exit mapping, stderr transport, reader rules, schema closure, CCJ-1 emission, entry-point wiring); the behavioral suite runs against every mutant | `.scripts/fragment-mutants.sh [evidence-dir]` (optional `FRAGMENT_MUTANT_IDS="M27 M28" FRAGMENT_TEST_PATTERN="TestExecutableUnicodePathBoundary|TestConformanceCorpus"` for focused rework) | `mutants.log` under the evidence dir (default `.temp/fragment-mutants/`); sources are restored from byte copies on exit |
| `.scripts/mapping-mutants.py` | §4.2 narrowing probes with named production-entry failures; restores candidate bytes after each mutation | `python3 .scripts/mapping-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv` (default `.temp/mapping-mutants/`) |
| `.scripts/composition-mutants.py` | §4.5 behavioral narrowing probes for environment ownership, collisions, stdin and launch-boundary file refusal; restores candidate bytes after every mutant | `python3 .scripts/composition-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv` (default `.temp/composition-mutants/`); permission probe requires a non-root host |
| `.scripts/systemprompt-mutants.py` | SPEC §5 narrowing probes through exported production APIs; exact source bytes restored after every mutation | `python3 .scripts/systemprompt-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv` (default `.temp/systemprompt-mutants/`) |
| fragment test corpus | `internal/fragment/testdata/schema-cases` is the `launch-env-fragment-v1` slice of curator-spec `conformance/v1/schema-cases` (index rows copied with their verdicts, source commit recorded in `index.json`); `testdata/a0` holds the three fragments and digests the installed Curator printed during A0 verification | `go test ./internal/fragment` | none; 49 indexed names/verdicts and fixture bytes are pinned by an independently upstream-derived manifest hash in `TestConformanceCorpus`; refresh requires reviewing the upstream rows and updating that pin as well as `index.json` |

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
