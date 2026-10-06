# curator-agent-launcher

`curator-run` composes Curator's managed environment, agents-management's
admitted interactive plan, and optional ax tracking. The contract is
[SPEC 0.5.0-draft](SPEC.md).

## Production pipeline

The executable supports `claude_code` (alias `claude`), `codex_cli`
(alias `codex`), native `pi`, and Muse (`muse`).

Muse has a registered `muse` system/provider mapping and accepts
`launch-env-fragment-v3` with four XDG parents under one managed home,
preserving inherited `HOME`. The pinned agents-management v0.5.53 plugin
declares interactive mode, probes the installed release, and maps native/yolo
permissions for its verified releases (1.4.1 and 1.4.2). `curator-run muse` builds
an interactive plan with XDG overrides and inherited `HOME`; it supplies no
context carrier because Muse v3 declares no prompt or MCP channels. Reserved
members remain non-operative. Native adds no
posture flag and yolo adds exactly one `--yolo`. Unlisted releases fail closed.

The launch pipeline follows these steps:

1. Read machine-first `ax.json` before argument validation. Missing configuration
   or `enabled:false` selects direct execution. Malformed or unreadable
   configuration refuses the invocation, including informational flags.
2. Normalize the `<env-id>` operand (`claude` → `claude_code`,
   `codex` → `codex_cli`) before validation or lookup, then resolve the
   fragment with `curator env resolve --repair --format json`, forwarding
   Curator stderr unchanged, and map its environment to a supported
   system/provider pair. Outputs carry the canonical id only; aliases are
   never persisted.
3. Resolve model and effort from flags, operator/machine defaults and the tagged
   lineup. Resolve permission mode by flag, Curator profile setting, launcher
   global default, then the built-in default. The mode is passed to
   `LaunchRequest.PermissionMode`; emit model/effort origins and the module's
   release-bound permission mapping before admission.
   Pi uses the ordered convention `pi-anthropic`, `pi-openai`, `pi-google`; vendor
   scores are never compared. Explicit models bind their own runtime.
4. Resolve the host from the `--hosted`/`--native`/`--untracked` flag and
   the v3 `host` default: machine lock, then flag, then operator default,
   then native. A hosted flag or default with `ax` enabled refuses
   `host_configuration_conflict`; a machine hosted default refuses
   `session_host_default_not_ready` until upgrade-without-hangup, while an
   operator hosted default is the operator's own opt-in (Q-D1a = yes); any
   `--hosted --network` selection refuses `network_scope_unsupported` (exit
   16; native `--network` is the direct path below). No flag and no
   default stays native with no session-host probe. Hosted launches resolve
   permissions through the configured ladder with silence defaulting as on
   the native path and yolo admitted (Q-D3), admit Claude only in Phase 1,
   and elevate resume selectors through the module typed-intent grammar.
5. Call `vendorplugin.BuildLaunchWithEnvironment` in `LaunchModeInteractive`
   with `LaunchRequest.ToolRelease` and `LaunchRequest.NativeArgs` for the
   release-bound permission grammar (v2 for Claude Code and Codex CLI, v1 for
   Pi), then enforce a separate
   `providerlimits.Store.AvailabilityFor` verdict for the same runtime/model/
   managed home. Unknown and failed reads refuse. No retry, model downgrade or
   fallback occurs.
6. Pass Claude/Codex profile context and file-backed system-prompt/MCP channels
   through `SpawnRequest.Context` into the shared agents-management construction
   API. The plugin owns argv order: Claude model/effort → MCP → prompt → native;
   Codex prompt → MCP → model/effort → native. Native arguments stay last and
   verbatim. Colliding native prompt, MCP or Codex profile overrides refuse with
   `plan_refused`, naming the native arguments and fragment channel. Non-colliding
   arguments pass through. Pi retains its existing prompt/discovery path.
   v2/v3 Claude/Codex fragments project their v1-compatible context subset;
   permission mapping and original fragment transport metadata remain unchanged.
7. Prepare direct execution, the ax launch-plan document, or the hosted
   session-launch-plan payload. Immediately before either native launch, call
   `systemprompt.PrepareLaunch`, check the provider binary,
   and check the Codex MCP layer. Emit prompt/discovery warnings. A late refusal
   starts neither provider nor ax. A hosted launch hands the versioned payload
   with its content digest to the `task-board` receiver over the private
   terminal/status transport instead of exec'ing; a missing receiver refuses
   `session_host_missing`, never a native fallback.

The permission interface is implemented against upstream
`skill-agents-management v0.5.53` (F-M1). It owns
`LaunchRequest.PermissionMode`, the release-bound mapping, versioned native
argument classification, stored-policy inspection, and the capability table.
Claude Code and Codex CLI use `permission-grammar-v2`; Pi uses
`permission-grammar-v1`. The admitted result supplies
the plan and owned environment literals from the same prepared,
alias-projected request; composition never rebuilds the plan.

## Environment and transport

Direct execution uses the composed full environment, working directory,
argv and stdin. Tracked transport includes only owned literals, MCP lookup
names, argv suffix, encoded stdin and the four base Curator extensions. A
tracked native launch can also include the optional
`works.relux.curator.effective-native-policy` extension. It never serializes the
full inherited environment. Ax inherits the launcher environment.
Attached empty stdin remains distinct from unattached; binary stdin uses D4
base64url encoding in the ax document and exact bytes for direct execution.

Pi has no MCP channel. Its selected prompt flag and managed-home
`APPEND_SYSTEM.md` / `SYSTEM.md` candidates are checked freshly, including when
no prompt is selected. Warnings distinguish selected flags from conditional
native discovery. Reserved `path_prepend` remains parsed and hashed without
changing PATH or claiming managed command-root support. Reserved members are
marked in the fragment type and excluded from the execution context carrier;
tracked provenance retains the original fragment digest.

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

Install the tagged release with the Go toolchain specified by `go.mod`:

```bash
go install github.com/relux-works/curator-agent-launcher/cmd/curator-run@v0.2.0
curator-run --version
```

The v0.2.0 release reports `0.2.0` (specification `0.5.0-draft`).
Compatible with curator v0.15.0-rc.3 or later, once published.
Ensure `$(go env GOPATH)/bin` is on `PATH`, or set `GOBIN` to a dedicated
trusted operator-owned directory on `PATH`. Do **not** install into Curator's
user-bin shim directory (`~/.local/bin`), a managed skill bin directory, or
beneath the environments root: umbrella discovery refuses these locations
with `subcommand_provider_untrusted` (environments.md §11).

Install Curator and the desired provider separately and make them available on
`PATH`; configure the provider's credentials and a Curator profile before launch.
The launcher installs neither providers nor profiles. When tracking is enabled,
`ax` must also be available and support the launch-plan contract.

Curator discovers unknown subcommands as `curator-<name>` on trusted `PATH`.
The two invocation forms pass the same launcher arguments:

```bash
curator run codex_cli --profile companyA -- resume --last
curator-run codex_cli --profile companyA -- resume --last
curator run pi --profile companyA --system-prompt append
curator-run --help
```

The general umbrella form is `curator run <env> --profile <p> -- <args>`.
Supported environments are `claude_code` (alias `claude`), `codex_cli`
(alias `codex`), `pi`, and Muse (`muse`); `opencode` is currently refused with
`env_unsupported`. The operand normalizes to the canonical id before
validation or lookup, so `curator-run claude` behaves exactly as
`curator-run claude_code` (and `codex` as `codex_cli`); provenance lines,
the default ax session name, and the `curator env resolve` call carry the
canonical id, and any other unknown spelling keeps the existing refusal.
Without `--profile`, Curator uses the current profile for the applicable
scope. Resolution always requests repair.

### Launcher options

| Option | Meaning |
|---|---|
| `--hosted`, `--native`, `--untracked` | One host request slot; `--untracked` equals `--native`; repeats and combinations refuse |
| `resume [SES-HANDLE]`, `--resume <id>` | Hosted-only resume selectors; usage errors in native mode |
| `--profile <name>` | Curator profile, forwarded to environment resolution |
| `--system-prompt <append\|replace>` | Explicit prompt-channel opt-in; unavailable semantics refuse the launch |
| `--model <model>`, `--effort <effort>` | Explicit spawn-plane selection, subject to admission and machine locks |
| `--permissions <native\|yolo>` | Resolve the launcher permission mode |
| `--yolo` | Exact alias of `--permissions yolo` at the same precedence level |
| `-d`, `--danger` | Rejected as usage errors before `--`; after `--`, arguments are native input |
| `--name <session-name>` | Tracked session name; `[A-Za-z0-9][A-Za-z0-9._-]{0,63}`; accepted without effect when untracked |
| `--ax-profile <standard\|yolo>` | Tracked execution profile; usage error when untracked; absent uses ax's default |
| `--network-policy <policy>` | `optimistic` (default), `strict`, or `pinned:<binary SHA-256>`; sensitivity cannot be downgraded |
| `--network-known-bad <path>` | Explicit operator known-bad list; any load error refuses |
| `--network <profile>` | Network profile for direct execution; resolved after admission, refused when tracked; refused with exit 16 when combined with `--hosted` |
| `--help`, `-h`, `--version` | Print information and exit, after reading tracking configuration |
| `--` | End launcher parsing; all following arguments pass through verbatim |

Value flags accept `--flag value` or `--flag=value`. Repeated flags, unknown
flags, missing values, and extra operands before `--` are usage errors. The
permission flag accepts only `native` or `yolo`; `--yolo` is the no-value exact
alias, and the two forms cannot be combined. The host flags likewise share one
request slot and take no value. After `<env-id>`, at most the `resume` selector
and its optional handle may follow; anything else before `--` is stray.
`-d` and `--danger` are rejected
launcher flags before the native-argument boundary.
Prompt-file discovery can still apply without the prompt opt-in; see the warnings
and Pi precedence described above. Pi has no MCP channel.

With `--network <profile>`, the launcher resolves the named profile from
the operator catalog after plan admission, probes the proxy once within
a bounded preflight, and applies the patch last in composition — after
the fragment, prompt, and MCP layers — to the direct child environment.
A named `kind = "direct"` profile instead clears inherited proxy
variables and sets nothing: it skips every probe step and its Record
carries skipped/skipped/skipped. The profile must be confirmed at its
current digest (`curator network confirm <profile>`); any resolve,
validate, or preflight error ends the launch, never an unmanaged
fallback. A managed launch prints a `curator-run: network: …`
provenance line carrying the binding Record; the endpoint never appears
in it, and a launch that composition refuses prints no Record. Tracked
launches refuse `--network` with `network_scope_unsupported`: the
source host cannot validate the destination. Hosted launches also refuse.

Adapter admission uses `adapterprobe.Evaluate` from v0.3.1. The default
`--network-policy optimistic` admits a known vendor build as **unqualified**,
subject to the library's fail-closed known-bad policy and the launcher's
independent child-scope ceiling (currently `generic-env-v1`, Claude print).
`--network-policy strict` requires independent qualification;
`--network-policy pinned:<64 lowercase hex SHA-256>` additionally requires
that exact binary. A pin proves identity, so an unproven pinned build remains
unqualified. `sensitive_egress = true` requires qualification even under
optimistic or pinned policy; a flag cannot downgrade it.

The legacy N-B version-label list is retired, not migrated: it has no SHA-256
provenance. `Policy.Allowlist` stays empty until independently qualified
`AllowedBuild` records exist. Thus every known-vendor build is unqualified
under optimistic policy and refuses under strict or sensitive-egress policy,
including the previously listed release. Central qualification (A) is outside
this revision. Qualification never widens the independent child-scope ceiling;
interactive, tracked and hosted network launches remain unsupported.

The operator known-bad list is `~/.curator/adapter-knownbad.json`, or the path
selected by `--network-known-bad <path>`. The library reads it through
`SafeKnownBadRead`: genuine absence of the default file is optional; a present,
unreadable or malformed file, or a missing explicitly selected file, refuses.
No local probe attempts to manufacture qualification.

At launch an unqualified build prints `Provenance.OperatorText()` verbatim on
stderr, followed by exactly one `curator-run: adapter-provenance: <JSON>` line
carrying the full `relux-adapter-provenance-v1` record. Quiet, silent and
machine-output native flags cannot suppress either stderr line. A wrapper
that owns a session can parse and persist the JSON record; direct launches
have no launcher-owned session record. Tracked/hosted guards refuse network
admission until their session owner stores that provenance. The existing
network binding line remains separate.

Admission identifies the native binary bytes, not the version label. The
launcher executes those exact bytes from a private staging directory and
removes the stage after the child exits. Scripts/interpreter wrappers refuse
with `runtime_closure_unsupported`; no dependency closure is claimed as
qualified merely because a file has a native executable header. See SPEC
§4.4b and §6.

### Configuration family

The launcher owns `defaults.json` and `ax.json` in the operator directory
`$XDG_CONFIG_HOME/curator-run` (default `~/.config/curator-run`) and machine
directory `/etc/curator-run`. These are separate from Curator's configuration.
Both use closed schemas: unknown members, malformed data and unreadable files
refuse the invocation; absence alone permits fallback.

Model and effort resolve independently: flags, then operator defaults over
machine defaults per member, then the admitted lineup. Permission mode has its
own precedence: `--permissions` or `--yolo`, Curator's per-profile setting in the
fragment, the launcher-global file value, then the built-in default. The host
resolves separately: machine lock, then host flag, then operator default, then
native. A v3 file can set an effort for one environment, a permission default
for another, and a host default for a third:

```json
{
  "schema": "curator-run-defaults-v3",
  "locked": false,
  "defaults": {
    "codex_cli": { "effort": "high" },
    "claude_code": { "permissions": "yolo", "host": "native" }
  }
}
```

Each environment entry may contain `model`, `effort`, `permissions`, and/or
`host`; permission values are exactly `native` or `yolo`, host values exactly
`native` or `hosted`. A v3 reader accepts v1 and v2 files unchanged; older
readers reject the members they do not know. The permission
fragment member uses the F-S2 names `permissions.mode`, `permissions.locked`,
and `permissions.source`; `source=default` means the profile level is silent.
Untracked silence defaults to `yolo` with `source=default-interactive` on
every stdio shape (Q-D3 literal: headless signals never change the
default); tracked native silence defaults to `native` with
`source=default-headless`. `native` adds no
launcher override and does not guarantee prompting; untracked native arguments
after `--` remain verbatim. The closed headless marker set is versioned in SPEC
§4.6 and mirrored by environments §10.1.
Curator's force-native lock is above all permission levels: visible `yolo` is a
`usage` error, silence resolves to `native`. A tracked native `yolo` request
from any level fails with `permission_mode_tracked_unsupported`; the launcher
never falls back to untracked execution. Hosted launches are exempt from
tracked permission semantics (Q-D3): the flag/profile/global ladder applies,
unconfigured silence defaults to `yolo` on every stdio shape, and the resolved
posture exports as
`execution_profile` `standard`/`yolo` in the payload. A piped launcher stdin
refuses before the plan build; an attached plan stdin refuses after it. A
missing or invalid terminal descriptor refuses before the plan build with
zero receiver lookup, and the launcher marks and validates both private
descriptors before contact. The
fd4 status channel accepts only closed refusal envelopes and normalizes
anything else to `session_host_protocol_error` without reflecting receiver
bytes; a poisoned channel stays a protocol error even when a later frame
is truncated. When the machine defaults
file is locked,
operator entries are ignored for its environments and flags overriding any
member it sets are usage errors. Model and effort origins are printed before
admission. Native launches use the upstream stored-policy inspector and report
known relaxations only from files it fully inspected. The report is not a claim
that a setting won provider precedence or that uninspected managed policy is
clear. Tracked launches record the same selectors and their inspected source.
Long `permissions.allow` lists are summarized by rule count in stderr.

Pi fallback uses the ordered convention `pi-anthropic`, `pi-openai`,
`pi-google`, selecting the first runtime with a driven model and ranking only
within that runtime. This is an operator convention, not a comparison of vendor
scores. An explicit/configured model binds its own runtime independently.

### Tracked mode

Enable tracking with this `ax.json` document:

```json
{ "schema": "curator-run-ax-v1", "enabled": true }
```

Here **machine wins**: an existing `/etc/curator-run/ax.json` decides;
the operator file is read only when the machine file is absent. Absence of
both files or `enabled: false` selects direct execution. This read precedes
argument validation, including help/version. Invalid configuration is
`defaults_config_invalid`, never an untracked fallback.

Tracked launches call `ax start <name> --provider <id> --launch-plan -
[--profile <ax-profile>] --workspace <cwd>`. Without `--name`, the name is
`<env-id>-<YYYYMMDDTHHMMSSZ>` in UTC, with the canonical `<env-id>`: an
alias launch names the session `claude_code-…` or `codex_cli-…`. An explicit
`--native` or `--untracked` flag bypasses `ax` on any machine (with
`--ax-profile` a usage error, since the profile would be discarded); a
configured native default keeps the existing `ax` behavior. A hosted flag or
default with `ax` enabled refuses `host_configuration_conflict`.
A failed handoff never starts a direct child. Repository tests use **fake ax
only**; they do not demonstrate an installed ax integration.

### Diagnostics and exit codes

Launcher failures print a stable code line followed by human-readable detail
on stderr. Foreign Curator/provider/ax output is forwarded unchanged.

| Result / diagnostic codes | Exit |
|---|---|
| Help/version, successful direct execution, ax handoff, or hosted handoff | 0 |
| `usage` (invalid arguments, locked-member override, host-request repeat, native resume selectors, or visible `yolo` under Curator's force-native lock) | 2 |
| `host_configuration_conflict`, `session_resume_invalid`, `launch_plan_invalid` | 2 |
| `resolve_invocation_failed`, `resolve_environment_unknown`, `resolve_profile_unknown`, `resolve_repair_failed`, `resolve_lock_unavailable`, `resolve_fragment_invalid` | 1 |
| `defaults_config_invalid`, `defaults_unresolvable` | 1 |
| `permission_policy_unsupported`, `permission_mode_tracked_unsupported`, `permission_mode_unsupported` | 1 |
| `plan_refused`, `plan_provider_limited`, `env_unsupported` | 1 |
| `exec_provider_missing`, `ax_handoff_failed` | 1 |
| `mcp_layer_missing`, `mcp_layer_unreadable` | 1 |
| `sysprompt_channel_unavailable`, `sysprompt_file_unreadable` | 1 |
| `network_profile_unknown`, `network_profile_denied`, `network_scope_unsupported`, `network_configuration_conflict`, `network_proxy_unreachable`, `network_proxy_auth_failed`, `network_profile_invalid`, `network_file_unreadable` | 1 (hosted `--network`: `network_scope_unsupported` exits 16) |
| `session_host_missing`, `session_host_unavailable` | 1 |
| `session_host_protocol_unsupported`, `session_host_provider_unsupported`, `session_host_scope_unsupported`, `session_host_execution_profile_unsupported`, `session_host_terminal_required`, `session_host_stdin_unsupported` | 6 |
| `secret_policy_violation`, `policy_refused`, `session_host_default_not_ready` | 16 |
| Direct child failure | Child's exit code unchanged |
| Direct child terminated by signal | `128 + signal` |
| Hosted receiver exit without a refusal record | Receiver exit unchanged |

See [SPEC §6](SPEC.md#6-errors-and-diagnostics) for each refusal condition.
No refusal retries with a different model or weaker launch.

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
and `macos-latest` with the toolchain from `go.mod`. The optional `Test (rose-air)` job runs `make check` on
`[self-hosted, macOS, ARM64]` only when the repository variable
`ROSE_AIR_RUNNER` is exactly `true`. Enable it after registering a matching
runner. Goldens run as part of `make test` and `make race`. There are no
release or tag jobs. Windows is not in this launcher's hosted matrix;
platform execution evidence must come from the corresponding runner.

### Tools

| Tool | Purpose | Entry point | Output |
|---|---|---|---|
| `go` (build, vet, test, race) | build and behavioral suite | `make check` and the targets above | stdout; nothing written to the tree |
| `python3` defaults mutant harness | weaken defaults gates individually and run the behavioral suite; restore exact candidate bytes | `python3 .scripts/defaults-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv` (default `.temp/defaults-mutants/`) |
| `.scripts/context-mutants.py` | narrow carrier engagement, reserved-member exclusion, permissions, deprecated-alias refusal, native suffix and release configuration in an isolated candidate copy | `python3 .scripts/context-mutants.py [evidence-dir]` | isolated copy, per-mutant logs and `summary.tsv`, default `.temp/context-mutants/` |
| production pipeline mutants | narrow the three late checks and ax mode selection at `run(...)`; restore exact candidate bytes | `python3 .scripts/pipeline-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv`; 9 named narrowing probes |
| CLI goldens | frozen accepted/rejected §3 shapes | `go test ./internal/cli -run TestGolden -update` to regenerate, then review the diff | `internal/cli/testdata/cases.golden` |
| `.scripts/hosted-mutants.sh` | narrowing-mutant harness for the §4.8/§4.9 hosted gates (precedence, fallback, probe-on-native, defaults gate, network gate, status contract, payload limits, session projection); fails closed: a mutant whose anchor matches no source is reported `NOT_APPLIED` by name and exits non-zero | `.scripts/hosted-mutants.sh [evidence-dir]` (optional `MUTANTS_ONLY=M-H1,M-H2`, `MUTANT_PKGS`, `MUTANT_RUN` for bounded batches) | per-mutant logs and `summary.tsv` (default `.temp/hosted-mutants/`); the source tree is restored on exit |
| `internal/mutantguard` | committed check that runs the required hosted mutants M-H1, M-H2 and M-H3 through the harness in a scratch copy of the tree and requires each to be applied and killed | `go test ./internal/mutantguard` | scratch copy under the test temp dir; the working tree is never mutated |
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

Adapter-policy verification uses the existing Go tools with bounded package
selections: `go test ./cmd/curator-run -run '^TestOptionC' -count=1 -v`
executes baseline properties and seam mutants, and
`go test ./internal/cli ./internal/execution -run 'Test(NetworkPolicy|ArtifactSnapshot)' -count=1`
checks flag grammar and staging. `TestOptionCMutants` retains each expected-red
subprocess log under `.temp/TASK-261005-yoogtw/mutants/`; each mutant must fail
its named behavioral test with actual exit 1. No literal source replacement
is used for this policy gate.
