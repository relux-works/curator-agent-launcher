# curator-agent-launcher

The Curator agent launcher: the **execution plane** that composes three
independent contracts into one exec —

- the **spawn plane** (`agents-management`): which agentic system, model,
  reasoning effort, and vendor, and whether provider limits admit a launch
  right now — consumed as a built launch plan, never rebuilt;
- the **context plane** (Curator): the launch environment fragment obtained
  through `curator env resolve --format json` and merged into the child
  environment;
- the **session plane** (`ax`): when the machine's `ax` integration is
  configured, every launch goes through `ax`'s instrumentation so the
  session is tracked from birth.

The launcher holds no session state of its own — *fire* is the launcher's
verb, *manage* is `ax`'s. The full contract, including the CLI surface,
the composition algorithm, the system-prompt opt-in and its warnings,
diagnostics, and versioning, lives in [SPEC.md](SPEC.md).

## Status

**Specification draft — not yet implemented.** The specification is
`0.1.0-draft`. The `curator-run` binary in this repository is a stub that
prints its name, specification version, and usage, and refuses everything
else with exit code 2. No composition logic exists yet.

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
make build   # go build ./...
make vet     # go vet ./...
make test    # go test ./... -count=1
make check   # all of the above
```

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
