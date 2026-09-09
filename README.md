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

SPEC §6 is implemented as a stable contract in `internal/diagnostics`:
the closed 18-code family table, the exit rule (usage 2, every
operational failure 1, unknown codes still 1 — never silent success),
and the deterministic `curator-run: <code>: <detail>` rendering. The
entry point reports its own families (usage, resolve, environment)
through it; later families are classified from their owners' real error
types (`axconfig.Error`, `composition.LayerError`, `systemprompt.Refusal`)
at their own production APIs. Absence and read failure stay distinct
(missing layer vs unreadable layer, absent config vs broken config,
absent home file vs unreadable file), warnings and child stderr are
never diagnostic code lines, and no failure degrades into a weaker
launch. The remaining main call-site obligations (ax policy before
parsing, defaults/lineup, plan and provider limits, late boundary
binding) are enumerated in `diagnostics.RemainingObligations` and below.

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
main integration remain later stories' obligations. The execution API below
now owns tracked schema/extensions, stderr delivery and actual subprocesses.
The existing executable still refuses `not_implemented`.

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

## Execution and tracking-policy APIs

`axconfig.Load(machineDir, operatorDir)` reads `ax.json` with closed schema
`curator-run-ax-v1` and a required boolean `enabled`. Machine policy, including
false, decides without even inspecting the ignored operator directory. Both
absent means false. Unknown/duplicate/wrong-typed members, broken ancestors,
dangling links, nonregular files and read failures return
`defaults_config_invalid`; the API writes nothing. Supply `/etc/curator-run`
and the operator's `$XDG_CONFIG_HOME/curator-run` (or
`~/.config/curator-run`) directories explicitly. Integration must call this
**before `cli.Parse`**, including invalid argv, and pass `AxConfigured`.

`execution.Prepare(value, fragment, invocation, target, compositionTime)` takes
an actual `composition.Compose` result, validated fragment, parsed invocation
and mapped target. It snapshots the D3.2 document and direct argv/env/stdin.
Pass the composition timestamp; default names use its UTC value. Explicit
name, profile and workspace become the exact §4.6 ax argv. The closed document
contains schema/version, the entire argv suffix, owned literals, lookup names,
D4 stdin and exactly four Curator extensions. It never serializes Binary or
the full inherited environment. Native arguments remain untouched.

`Launch.Run(execution.Options{Boundary: probe, IO: streams, AxBinary: path})`
starts and waits for a real `os/exec` child on Darwin/Linux. `AxBinary` may be
an explicit executable path; otherwise `ax` resolves on the launcher's PATH.
Validation always supplies a compiled **fake ax**, never an installed real ax.
Tracked transport inherits the launcher environment (with os/exec's working
`PWD`); it never uses the direct environment as ax's environment. Nonzero or
not-startable ax returns 1 with `ax_handoff_failed`, then its stderr bytes
verbatim, and never falls back. Successful ax stdout/stderr are forwarded.

Direct execution uses the composed binary, argv, working directory and exact
full environment; empty means empty. Bare provider names resolve against the
composed PATH, relative paths against WorkDir. Unattached stdin shares the
supplied input; attached empty or binary stdin uses exactly the plan bytes.
Default stdio shares `os.Stdin`, `os.Stdout`, `os.Stderr`, including terminal
file descriptors. Normal child exit codes are returned unchanged; signal exits
return `128 + signal`. The caller must propagate that return code. Incoming
SIGINT/TERM/HUP/QUIT sent only to the launcher are forwarded to the child
process group. The child owns a separate foreground group on a controlling
terminal, avoiding duplicate delivery of terminal-generated interrupts. A child
stop suspends the launcher and restores its terminal group; continuing the
launcher restores the child foreground group and resumes it. Terminal ownership
returns after exit. This is a waited child, without process replacement or PTY
allocation. Real isolated PTY tests exercise one Ctrl-C, foreground reads,
Ctrl-Z, continuation and terminal restoration in both routes (five trials each).
Parent-only TERM and direct signal exit status have separate real-process tests.
Full interactive shell bg/disown semantics and Linux runtime remain unverified.

Both routes print composition's name-only warnings and require a non-nil typed
`execution.Boundary`. Immediately before process creation they invoke that
callback, freshly resolve/check the provider executable and call
`Value.CheckLaunchBoundary`. Callback errors are terminal; no implicit success
exists. These pathname probes retain a replacement/permission-change race up
to process creation; they are not open-handle guarantees. The upstream §5 package and its tests are carried byte-for-byte from
`adf627607eb334e9839288cfffce63e1268ae688`, with its README section and mutant
harness preserved. The callback remains the final pipeline's explicit obligation:
bind `systemprompt.PrepareLaunch` (including `ProbeFiles` and warning formatting)
rather than a second implementation. Merely carrying that API does not wire it
into main. The managed branch checkpoint remains
`84747c326eee9863ddfd7e86ac65be1056718fbc`.

**Pending production wiring belongs to TASK-260908-1o7i8y:** load ax policy
before usage validation, resolve defaults, obtain an admitted single plan via
the settled spawn-plane API, select/apply §5 prompt policy, compose once,
prepare at composition time, bind the real third late probe, run and propagate
its exit code. Main and SPEC remain untouched by this API task. The current
executable still refuses `not_implemented`. Pi-shaped process values do not
claim native Pi admission on module v0.5.10. Independent review and signed
publication/landing belong to the parent; the managed producer leaves this
candidate uncommitted for the review snapshot.

## Diagnostics contract and remaining call sites

`internal/diagnostics` owns the SPEC §6 table (18 codes), `ExitForCode`
(usage 2, operational 1, unknown 1), `Line`/`Emit` rendering, `CodeOf`
classification over the owners' concrete error types, and
`IsDiagnosticLine` transport separation. Each wiring step below must
choose the named family code at its boundary, render through `Emit`, and
exit through `ExitForCode`, with no fallback to a weaker launch shape:

1. `axconfig.Load` before `cli.Parse` (`defaults_config_invalid` even
   when argv is also a usage error) — TASK-260908-1o7i8y.
2. `defaults.json` read, locked-member refusal, lineup fallback,
   `defaults_unresolvable`, per-member stderr line-group —
   TASK-260909-2vy977 (SPEC §4.3 lineup/defaults delivery).
3. `vendorplugin.BuildLaunch` admission as `plan_refused` and
   `providerlimits.Store.AvailabilityFor` enforcement as
   `plan_provider_limited` with verbatim verdict evidence (state,
   `Until`, `Checked`, `Observed`, `Failures`); indeterminate reads stay
   terminal refusals, never healthy by inference —
   TASK-260908-2so46q (SPEC §4.4 plan delivery).
4. `composition.Value.CheckLaunchBoundary` with the binary check and
   `systemprompt.PrepareLaunch` as the execution boundary immediately
   before both handoff and exec — TASK-260908-1o7i8y.
5. `ax` Structured Error pass-through after the launcher's own
   `ax_handoff_failed` line (already owned by `execution.Launch.Run`;
   no untracked fallback) — preserved, not re-implemented here.

`defaults_unresolvable`, `plan_refused`, and `plan_provider_limited`
have no producer in this build; they are declared bounds, not dropped
rows. `.scripts/diagnostics-mutants.sh` carries 21 mutants for the new
gates: 14 narrowing probes (single-member exit/classifier/framing
weakenings, each requiring its named single-member assertion in the
log) plus 7 retained broad, drop-one, token-preserving-broad, and
formatting probes that pin useful failures without proving a bound.
The narrowing set includes the adopted owner/form gate probes
(D13–D21: one foreign admission per mutable owner, one joined-owned
rejection, one rejection per fixed-owner non-direct form, and one
exact-Detail framing exemption at the real main/resolver entry).

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
| `.scripts/execution-mutants.py` | narrowing probes at tracking-policy Load and execution Launch.Run, using actual fake subprocesses; restores candidate bytes | `python3 .scripts/execution-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv`, default `.temp/execution-mutants/`; permission tests require non-root |
| `.scripts/diagnostics-mutants.sh` | narrowing mutants for the §6 gates (exit mapping, closed membership, no-invention classification, anchored code-line parsing, entry exits, owner/form registry, exact-Detail framing exemption); restores candidate bytes after every mutant | `.scripts/diagnostics-mutants.sh [evidence-dir]` | per-mutant logs and `summary.tsv` (default `.temp/diagnostics-mutants/`) |
| execution process helpers | exact document/argv/env/stdin/exit and late-check tests; compiles disposable fake provider/ax and API signal driver; Python 3 supplies isolated POSIX PTY regression | `go test ./internal/axconfig ./internal/execution -count=1` | test stdout; helper binaries use temporary directories and are removed |
| fragment test corpus | `internal/fragment/testdata/schema-cases` is the `launch-env-fragment-v1` slice of curator-spec `conformance/v1/schema-cases` (index rows copied with their verdicts, source commit recorded in `index.json`); `testdata/a0` holds the three fragments and digests the installed Curator printed during A0 verification | `go test ./internal/fragment` | none; 49 indexed names/verdicts and fixture bytes are pinned by an independently upstream-derived manifest hash in `TestConformanceCorpus`; refresh requires reviewing the upstream rows and updating that pin as well as `index.json` |

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
