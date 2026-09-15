# curator-agent-launcher

`curator-run` composes Curator's managed environment, agents-management's
admitted interactive plan, and optional ax tracking. The contract is
[SPEC 0.3.0-draft](SPEC.md).

## Production pipeline

The executable supports `claude_code`, `codex_cli`, and native `pi`:

1. Read machine-first `ax.json` before argument validation. Missing configuration
   or `enabled:false` selects direct execution. Malformed or unreadable
   configuration refuses the invocation, including informational flags.
2. Resolve the fragment with `curator env resolve --repair --format json`,
   forwarding Curator stderr unchanged, and map its environment to a supported
   system/provider pair.
3. Resolve model and effort from flags, operator/machine defaults and the tagged
   lineup. Emit each member's origin before admission. Pi uses the ordered
   convention `pi-anthropic`, `pi-openai`, `pi-google`; vendor scores are never
   compared. Explicit models bind their own runtime.
4. Call `vendorplugin.BuildLaunchWithEnvironment` in `LaunchModeInteractive`,
   then enforce a separate `providerlimits.Store.AvailabilityFor` verdict for
   the same runtime/model/managed home. Unknown and failed reads refuse. No
   retry, model downgrade or fallback occurs.
5. Select the requested system-prompt channel and compose argv in plan → prompt
   → MCP → native order. Native arguments after `--` remain opaque.
6. Prepare direct execution or the ax launch-plan document. Immediately before
   either launch, call `systemprompt.PrepareLaunch`, check the provider binary,
   and check the Codex MCP layer. Emit prompt/discovery warnings. A late refusal
   starts neither provider nor ax.

The dependency is the real `skill-agents-management v0.5.13` tag, without a
replace directive or workspace override. Its admitted result supplies both the
plan and owned environment literals from the same prepared, alias-projected
request. Composition never rebuilds the plan or reconstructs that request.

## Environment and transport

Direct execution uses the composed full environment, working directory,
argv and stdin. Tracked transport includes only owned literals, MCP lookup
names, argv suffix, encoded stdin and the four Curator extensions. It never
serializes the full inherited environment. Ax inherits the launcher environment.
Attached empty stdin remains distinct from unattached; binary stdin uses D4
base64url encoding in the ax document and exact bytes for direct execution.

Pi has no MCP channel. Its selected prompt flag and managed-home
`APPEND_SYSTEM.md` / `SYSTEM.md` candidates are checked freshly, including when
no prompt is selected. Warnings distinguish selected flags from conditional
native discovery. Reserved `path_prepend` remains parsed and hashed without
changing PATH or claiming managed command-root support.

Launcher errors render through `internal/diagnostics`; usage exits 2 and
operational refusals exit 1. Foreign Curator/provider/ax evidence is forwarded
without diagnostic framing. Direct child exit codes propagate unchanged and
signal exits return `128 + signal`. An unsuccessful ax handoff returns 1 with
`ax_handoff_failed` and forwards its stderr unchanged; it never falls back.

Execution waits for a child without allocating a PTY or replacing the launcher.
The execution package owns terminal foreground groups, signal forwarding,
stop/continue and terminal restoration. Pathname probes retain a race between
checking and process creation; they are not open-handle guarantees.

## Evidence boundaries

`cmd/curator-run/pipeline_test.go` drives production `run(...)` with the real
fragment parser, tagged admission, a temporary provider-limit store, and compiled
fake provider/ax executables. Six adapter/mode goldens cover argv, environment,
stdin, warning output and side effects. Negative tests cover configuration order,
forbidden flags, admission, foreign-byte transport, exit status and late checks
in both modes. Injected attached stdin tests cover transport of a future admitted
payload; the shipped interactive plugins currently attach no stdin.

These tests do not install binaries or run real providers/ax. Installed umbrella
launches, managed-home repair/current status, model/effort output and MCP remain
the orchestrator's post-landing verification. Independent acceptance and signed
PR delivery also belong to the orchestrator. No cross-platform runtime claim is
made from this host's tests.

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
| `python3` defaults mutant harness | weaken defaults gates individually and run the behavioral suite; restore exact candidate bytes | `python3 .scripts/defaults-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv` (default `.temp/defaults-mutants/`) |
| production pipeline mutants | narrow the three late checks and ax mode selection at `run(...)`; restore exact candidate bytes | `python3 .scripts/pipeline-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv`; 9 named narrowing probes |
| CLI goldens | frozen accepted/rejected §3 shapes | `go test ./internal/cli -run TestGolden -update` to regenerate, then review the diff | `internal/cli/testdata/cases.golden` |
| `.scripts/cli-mutants.sh` | narrowing-mutant harness for the §3 gates: each mutant weakens one gate to admit one rejected shape and the named test must fail | `.scripts/cli-mutants.sh [evidence-dir]` | per-mutant logs and `summary.tsv` under the evidence dir (default `.temp/cli-mutants/`); the source tree is restored on exit |
| `.scripts/fragment-mutants.sh` | narrowing-mutant harness for the §4.1 gates (argv, exit mapping, stderr transport, reader rules, schema closure, CCJ-1 emission, entry-point wiring); the behavioral suite runs against every mutant | `.scripts/fragment-mutants.sh [evidence-dir]` (optional `FRAGMENT_MUTANT_IDS="M27 M28" FRAGMENT_TEST_PATTERN="TestExecutableUnicodePathBoundary|TestConformanceCorpus"` for focused rework) | `mutants.log` under the evidence dir (default `.temp/fragment-mutants/`); sources are restored from byte copies on exit |
| `.scripts/mapping-mutants.py` | §4.2 narrowing probes with named production-entry failures; restores candidate bytes after each mutation | `python3 .scripts/mapping-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv` (default `.temp/mapping-mutants/`) |
| `.scripts/composition-mutants.py` | §4.5 behavioral narrowing probes for environment ownership, collisions, stdin and launch-boundary file refusal; restores candidate bytes after every mutant | `python3 .scripts/composition-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv` (default `.temp/composition-mutants/`); permission probe requires a non-root host |
| `.scripts/systemprompt-mutants.py` | SPEC §5 narrowing probes through exported production APIs; exact source bytes restored after every mutation | `python3 .scripts/systemprompt-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv` (default `.temp/systemprompt-mutants/`) |
| `.scripts/execution-mutants.py` | narrowing probes at tracking-policy Load and execution Launch.Run, using actual fake subprocesses; restores candidate bytes | `python3 .scripts/execution-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv`, default `.temp/execution-mutants/`; permission tests require non-root |
| `.scripts/diagnostics-mutants.sh` | narrowing mutants for the §6 gates (exit mapping, closed membership, no-invention classification, anchored code-line parsing, entry exits, owner/form registry, exact-Detail framing exemption); restores candidate bytes after every mutant | `.scripts/diagnostics-mutants.sh [evidence-dir]` | per-mutant logs and `summary.tsv` (default `.temp/diagnostics-mutants/`) |
| `.scripts/plan-mutants.py` | SPEC §4.4 behavioral narrowing probes at `plan.Build`, restoring exact candidate bytes | `python3 .scripts/plan-mutants.py [evidence-dir]`; narrow validation: `go test ./internal/plan -count=1 -race`, `go vet ./internal/plan`, `go build ./internal/plan` | per-mutant logs and `summary.tsv`, default `.temp/plan-mutants/` |
| execution process helpers | exact document/argv/env/stdin/exit and late-check tests; compiles disposable fake provider/ax and API signal driver; Python 3 supplies isolated POSIX PTY regression | `go test ./internal/axconfig ./internal/execution -count=1` | test stdout; helper binaries use temporary directories and are removed |
| fragment test corpus | `internal/fragment/testdata/schema-cases` is the `launch-env-fragment-v1` slice of curator-spec `conformance/v1/schema-cases` (index rows copied with their verdicts, source commit recorded in `index.json`); `testdata/a0` holds the three fragments and digests the installed Curator printed during A0 verification | `go test ./internal/fragment` | none; 49 indexed names/verdicts and fixture bytes are pinned by an independently upstream-derived manifest hash in `TestConformanceCorpus`; refresh requires reviewing the upstream rows and updating that pin as well as `index.json` |

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
