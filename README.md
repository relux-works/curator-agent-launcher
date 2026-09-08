# curator-agent-launcher

The Curator agent launcher: the **execution plane** that composes three
independent contracts into one exec —

- the **spawn plane** (`agents-management`): which agentic system, model,
  reasoning effort, and vendor, and whether provider limits admit a launch
  right now — consumed as a built launch plan, never rebuilt;
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
`0.2.1-draft`. This build implements SPEC.md §3 only: the closed CLI
surface (`internal/cli`) — flag grammar, the `--` boundary with a verbatim
native tail, the `usage` diagnostic family with exit 2, and the
informational flags. Composition (§4), system-prompt application (§5), the
`defaults.json`/`ax.json` file family (§4.7), and the `ax` handoff (§4.6)
are not delivered: a well-formed launch invocation parses and is then
refused with exit 1 and a `not_implemented` line, launching nothing.
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

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
