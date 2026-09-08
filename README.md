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
refuse `env_unsupported` with exit 1 before later stages. The later
composition steps (§4.3–§4.6), system-prompt application (§5), and the
`defaults.json`/`ax.json` file family (§4.7) are not delivered: a
well-formed launch invocation parses, resolves its fragment (repairing the
managed home when Curator finds it stale), maps its supported system/provider pair, and is then refused with exit 1
and a `not_implemented` line that reports the mapping, home and fragment digest,
launching nothing. A resolve failure is reported as its `resolve_*` code
with exit 1.
Because no `ax.json` is read yet, the binary always parses as an untracked
machine, so `--ax-profile` is currently always a usage error and `--name`
is accepted without effect.

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
| fragment test corpus | `internal/fragment/testdata/schema-cases` is the `launch-env-fragment-v1` slice of curator-spec `conformance/v1/schema-cases` (index rows copied with their verdicts, source commit recorded in `index.json`); `testdata/a0` holds the three fragments and digests the installed Curator printed during A0 verification | `go test ./internal/fragment` | none; 49 indexed names/verdicts and fixture bytes are pinned by an independently upstream-derived manifest hash in `TestConformanceCorpus`; refresh requires reviewing the upstream rows and updating that pin as well as `index.json` |

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
