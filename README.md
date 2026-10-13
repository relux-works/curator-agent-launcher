# curator-agent-launcher

`curator-run` composes Curator's managed environment, agents-management's
admitted interactive plan, and optional ax tracking. The contract is
[SPEC 0.5.1-draft](SPEC.md).

## Front doors

| Entry | Composer | Process/lifetime owner | Status |
|---|---|---|---|
| Direct human `curator-run <env-id> …` | Caller-side interactive launcher | Launcher starts and waits for native child | Existing API below. |
| Hosted or `ax` launch | Same caller-side interactive launcher | Hosted receiver or `ax` | Existing §§4.6/4.8–4.9 contracts; refusal never falls back to direct launch. |
| Agent one-shot exec | Caller composes; dispatcher admits/provisions/tracks | `curator-run` under `worker-<label>` verifies and executes; dispatcher/helper own stop and cleanup | **Target amendment (version pending), not implemented**; [SPEC §10](SPEC.md#10-agent-account-one-shot-exec-contract-target-amendment). |

## Target one-shot exec mode

D-EXECUTOR (owner decision, 2026-10-11) selects a narrow agent-account
mode, with a proposed entry `curator-run exec --plan-fd N`. The privileged
path delivers the immutable plan on **FD3**; exact approved argv and the
closed wire schema/digest preimage remain publication questions in SPEC
§10.1. The caller composes and supplies the admitted SHA-256 plan digest;
the executor verifies and executes it. Under the agent account it never
resolves/repairs profiles, composes plans or infers permission defaults.
No plan or secret comes through an alternative path, stdin, argv or env.
The executor reaches the broker over a declared Unix socket and has no
internet egress. Local trusted harnesses use only their bound provider proxy;
workplaces have zero egress (R-NET-TOOL). This target does not
implement or generalise the interactive API below. Decision 0021's versioned
amendment still needs upstream publication before activation.

The dispatcher owns authority, provisioning, execution claims, supervision
and cleanup. It is an unprivileged Unix-socket service from the first
protected-launch slice, using the broker's connection and framing rules;
recovery and readiness precede admission. An absent or unready service
refuses with no command fallback. Queueing, events and long waits come
later behind capability gates on the same transport. Only the dispatcher
invokes the privileged launcher. `swarma-user-launcher` is the separate root
transition: it
verifies active owned generation, approved executor path/digest and home,
resets the environment, follows the user manager's single descriptor
lifecycle (spec §7 and `diagrams/launch.puml`, R-FD3), writes an atomic
start receipt before privilege drop, and runs `curator-run` under the agent
UID. The executor receives only caller-owned 0–2 and read-only FD3; no
service, broker or root handle reaches it. It opens a fresh peer-authenticated
broker connection. Completely staged, bounded, immutable plan bytes are
preferred; a pipe would need a published and qualified capacity, feeder,
deadline and cleanup order. Joint FD3-mechanism publication and intake,
digest, framing and read-deadline publication gate activation (§10.1). The decoded plan bound is
48 KiB,
base64 64 KiB, privileged request 80 KiB/5 s, helper request 16 KiB/5 s,
dispatcher service frame 128 KiB, broker frame 64 KiB.
The smallest bound refuses before provisioning. These are not the hosted
interactive payload or its terminal/status FD contract.

D-PATH fixes the resolved launch PATH: protected launch tool directory,
then admitted user base PATH in its existing order, then one protected
manager dispatcher directory for checked project commands. Repository
`.agents/bin` never executes in a managed launch. The caller records PATH
under the plan digest; the executor uses the verified value. R-EXEC places
qualified light-check wrappers with fixed subcommand allowlists in that
protected dispatcher directory; heavy entry points are not exposed.
Qualification covers the full invocation and transitive code/configuration
inputs, with worker scripts, imports, plugins, environment/startup injection
and direct/indirect shared-tool bypass refused. A tool that cannot prove
inert worker inputs goes to R-VERIFY. The user manager owns the OS-enforced
executable allowlist; PATH alone is not
an enforcement boundary. Builds, tests and worker-code execution belong
only to R-VERIFY runners on another machine, isolated and without secrets;
`agent-headless-build` is withdrawn. This claims no
implementation of reserved interactive `path_prepend`. One shared launch
library with `swarma-session-runner` is required by D-EXECUTOR; its package
and release are unpublished.

`curator-run` under the agent account is the first broker consumer. Board
runner and session host ask the dispatcher to launch through it; neither
embeds a broker client. The first slice is one protected Claude launch
with cross-user peer/binding proof. `works.relux.curator.credential/1`
contains required-understanding metadata only. The executor resolves and
rechecks `credential_binding/1`: peer `(uid,generation)`, profile, harness,
account, channel, endpoint, approved harness binary SHA-256 and policy
generation. The broker checks its active binding, current caller/leaf
grants and approved lists. `launch_id` (plan digest) is correlation, not
authority; neither a digest nor a capability declaration proves execution.
Conflicting credential sources refuse; no fallback to native login occurs.
`credential-injection/1` requires harness-release qualification for effective
source and uncontrolled children. Material goes only into the harness's
qualified channel, never plans, argv, files, logs or receipts; the harness
can read its own token.

Claude setup-token uses `CLAUDE_CODE_OAUTH_TOKEN`, with no refresh; expiry
or authentication failure requires re-enrollment/restart. Codex personal
is a later, still qualification-dependent lane: one broker auth owner,
external tokens into a per-session app-server, no launch `auth.json` or
broker-mode `codex exec`. The execution-side executor/runner owns renewal;
the broker alone refreshes. Default renewal deadline: 8 s; timeout fails
the turn with no late answer/fallback. Enterprise tokens are outside first
release; Muse subscription qualification is prioritised (2026-10-13),
with broker channel (b) / fallback (c) proposed and qualification open.
Explicit API-key channels and native per-home authentication stay separate.
The first broker store is files 0600 in owner-only 0700 directories.

R-TRANSFER makes materialization/export the dispatcher's workspace contract.
The caller supplies admitted exact base B as a bounded canonical tree archive;
the dispatcher relays/journals its digest and holds no repository. Approved
`swarma-workspace-materialize` writes a standalone child-UID repository with
private Git administration, fresh config, no hooks/remotes/credential helpers.
No cross-account linked worktree is allowed; within-account board worktrees
remain. The composed plan covers B, manifest digest, write scope, exec-profile
identity/body digest pinned by protected machine policy and read-only
task-resource materialization by digest. The manifest covers path, Git kind/mode
and content identity from immutable B; materialization verifies every archive
entry before work, and import independently recomputes it from repository B.
Exact fields
and formats are unpublished with the intake contract (SPEC §10.1).

The worker's Git history never enters the platform. After the round ends and
account-wide absence is confirmed (plus a workplace write fence), approved
`swarma-workspace-export` runs as the child UID with fixed environment and
no network. It compares both kind/mode and content identity with the immutable
manifest, preserving mode-only edits, regular↔symlink transitions and deletion
prior modes, and emits only a capped change set;
unsafe paths, collisions, special files and exceeded caps refuse the whole
export. The dispatcher journals digests and returns it only to the original
caller as `swarma-dispatcher workspace-result/1` (draft result fields published
by the dispatcher; transport/retention and activation still gated). The board accepts data as a resource;
the orchestrator
performs deterministic import, candidate, review and integration through the
board's change-request machinery. Code becomes a repository-signed candidate
C in our repository and a trusted non-producing reviewer reviews exactly C.
Write-scope/`.git`/`.task-board/` refusals and flagged-path hunk acknowledgement
are specified in SPEC §10.2. Rework starts from C with review findings;
accepted exact heads land through the existing signed fast-forward path,
with refresh and re-review when the base moves. There is no worker push or
merge-request path. Export is a cleanup obligation before home deletion,
unless explicitly waived by operator act. Shared-object provenance/access/
retention and exact transfer formats remain open, not ownership.

R-STANDING requires the one-shot executor to check plan digests for settings,
hooks, MCP configuration and instruction files before exec (also restart and
native resume where admitted), or use root-owned surfaces outside the home.
Drift refuses launch (code unpublished); repair is explicit re-materialization.
Standing local loops are trusted-only; outside standing workers use workplaces
with donor-side loops and task-whitelisted OS services.

R-NET-TOOL offers no web tool by default. Tasks carry checked resources as
board resources, also signed-letter attachments for remote workers, read-only
next to the workspace with plan digests. More resources are requested by
ordinary letter/outcome; the worker never fetches. The launch plan switches
off provider-side search unless an operator grants a trusted-circle identity
an explicit opt-in; network rules cannot see search inside provider API calls.
Only a qualified server-search tuple with no local fetch or the platform
Unix-socket web service may be opted in for local trusted identities. Remote
opt-ins follow R-NET-TOOL/CIP-0008; donor search keys never reach the project.
No `direct` or `external-http-proxy` may bind on a platform host. The
interactive operator-machine profiles below do not authorise platform egress.

SPEC §10.6 references R-UNTRUSTED, R-CEILINGS, R-WATCH, R-ALLSTOP,
R-RANGE-DENY, R-SOCKETS and R-SHARED as host/admission dependencies.
Untrusted work is refused in Slice 0/user-manager v0, has no subscriptions,
local loop or shared-service host, and needs qualified class minimums.
Trusted-circle admission never relaxes host boundaries. No OS is excluded
by name; macOS gaps and soft-row acceptability remain qualification/owner
questions. Node-wide emergency stop is manual and journaled; recovery is
explicit. Tokens already delivered retain vendor lifetime.

Cancellation/max-runtime stop uses helper `exec.stop` through the dispatcher,
referencing the user manager's single termination scope (§§4.4/4.7): verify
owned generation/current execution and receipt identity, signal every process
of the UID (TERM, grace, KILL), stop its service domain where present, and
confirm account-wide absence over a quiet period. Completed stops replay
without signalling; an old receipt cannot stop a successor. Caller/sudo
exit is not a stop receipt. Unknown evidence permits neither capacity release
nor replacement. Start receipts identify a start attempt, not readiness;
consumed-plan evidence binds digest, qualified executor and policy generation
without secrets. Exact consumed-plan/exit receipt transport, ownership across
exec, numeric refusal exits and signal encoding are **open publication
questions**, with options in SPEC §§10.4–10.5; the interactive exit table below
does not define this mode. Published typed refusals include
`credential_broker_unavailable`, `credential_source_conflict`,
`principal_ledger_mismatch`, `lease_binding_mismatch`,
`lease_harness_unqualified`, `renew_timeout` and `processes_remain`. Refusals
are terminal and never launch a weaker alternative.

The user manager's spec §7 owns the process **fate table** (R-FATE), next
to R-FD3: kill domains, stop/crash/restart fate, promised survival, affected
executions and the fence against writers from a replaced daemon. SPEC §10.4
references it; `setsid` and UID changes are not survival promises.
R-TIME requires an authenticated or operator-controlled installer time source,
durable dispatcher/broker time watermarks, expiry at `max(now, watermark)`
and `not_before` at `min(now, watermark)`. Untrusted time never advances
the watermark; reset is an explicit operator act. Clock steps can only
narrow authority. Expiry-driven home deletion and binding removal also need
real boot-inclusive monotonic elapsed time of at least the lifetime.

D-WIRE-NAMES selects `swarma-<component> <object>/<n>` for versioned wire
identifiers, including broker grant/revocation signed domains; it invents no
unpublished helper signature grammar. SPEC §§10.2/10.5 describe the
`host_start` and `host_stop` operations in `dispatch-op/1`. The durable
**host admission permit** is session-scoped for one `logical_session_id`.
The owner's admitted start can explicitly authorise successor executions
through **session lifecycle authorisation** (member format unpublished).
Every physical start retains its own `(execution_id, controller_epoch, op_id)`
claim, fence and receipt interlock. Successor `host_start` requires that
authorisation and previous-claim confirmed absence; it is idempotent per
`(logical_session_id, op_id)`. The host requests the start; only the dispatcher
performs it and resolves ambiguous calls without a second launch.
Only the configured session-host principal may request `host_stop`, only
under its permit: journaled E-stop (`exec.stop`, confirmed absence), recorded
as host-requested, distinct from owner `cancel`, with no start authority.
The runner opens a fresh authenticated host channel;
current-epoch registration and readiness precede delivery. Joint transport,
permit, fence/cancel/crash and receipt publication still gate the SH2 backend.
Helper v2 signature publication also remains open. D-GOALS keeps optional
goals authored by
the caller or board/orchestrator, with no agent goal capability, while a
separate self-goal namespace decision is deferred. The host/executor work
without goals, board or orchestrator toolchain. Dispatcher cleanup unbinds
and retires: after quiesce and declared-key cleanup it deletes the home.
Workspace-capable allocations first satisfy R-TRANSFER's recorded-export
or explicit operator-waiver obligation. The separate retirement metadata
export admits only service-generated closed-schema metadata, with no
agent-controlled strings, names, links or bodies. Home retention requires
an explicit separate operator operation stating that confidentiality is
not guaranteed. Code/results keep their separate git/acceptance policy,
without a secret-free-content assurance. D-IDENTITY allows separate
post-retirement revocation only for
identities registered as one-shot. Standing identities remain operator-only.
Donor provisioning is a separate component and variant B remains disabled
pending its spec. D-RESUME's same-machine semver rule has no fallback/portable
seed and does not enable resume in this local headless dispatcher v0 mode.

See SPEC §10 for the canonical source, all refusal boundaries and open
questions. These requirements and owner decisions are documentation targets,
not evidence of implemented broker intake, isolation, receipts or execution.

## Production pipeline

The module figures are PlantUML sources:

- [Components and process authority](diagrams/components.puml) — current interactive planes and target one-shot adapter ([SPEC §§1.1, 4, 10](SPEC.md#11-front-doors-and-execution-authority)).
- [Launch paths](diagrams/launch-paths.puml) — direct, ax, hosted and target one-shot refusal/stop branches ([SPEC §§4.6, 4.9, 10.1–10.5](SPEC.md#46-hand-off-and-exec)).
- [Invocation states](diagrams/launch-states.puml) — transient states, terminal refusals and confirmed cleanup boundaries ([SPEC §§4, 6, 10.4](SPEC.md#4-composition-algorithm)).
- [Artifact layout](diagrams/artifact-layout.puml) — persistent owners, generated payloads, descriptors and metadata versus secret material ([SPEC §§4.1, 4.5–4.7, 4.9, 10](SPEC.md#47-the-launchers-configuration-file-family)).

Each figure labels current and target phases separately. The target is not
implemented; unpublished intake, receipts and activation details remain visible
as spec gaps. See the [figure index](diagrams/README.md).

The executable supports `claude_code` (alias `claude`), `codex_cli`
(alias `codex`), native `pi`, and Muse (`muse`).

Muse has a registered `muse` system/provider mapping and accepts
`launch-env-fragment-v3` with four XDG parents under one managed home,
preserving inherited `HOME`. The pinned agents-management v0.5.62 plugin
declares interactive mode and admits any well-formed build attested by the
selected binary, including 1.4.6, with no release table. The frozen-copy tuple
is optional; without it `muse` resolves on the handed PATH. `muse-wonder`
resolves to `muse-wonder-1.0-contributor` with module-owned effort rules.
`curator-run muse` builds
an interactive plan with XDG overrides and inherited `HOME`; it supplies no
context carrier because Muse v3 declares no prompt or MCP channels. Reserved
members remain non-operative. Native adds no
posture flag; yolo adds exactly one `--yolo` only after bounded binary help
evidence declares it. Malformed build evidence and a probed release claim
that differs from the binary attestation fail closed. For example:

```sh
curator run muse --profile P --model muse-wonder
```

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
   release-bound permission mapping before admission. Muse yolo reports
   `mapped=module-attested` only after the builder admits its help evidence.
   Pi uses the ordered convention `pi-anthropic`, `pi-openai`, `pi-google`; vendor
   scores are never compared. Explicit models bind their own runtime.
4. Resolve the host from the `--hosted`/`--native`/`--untracked` flag and
   the v3 `host` default: machine lock, then flag, then operator default,
   then native. A hosted flag or default with `ax` enabled refuses
   `host_configuration_conflict`; a machine hosted default refuses
   `session_host_default_not_ready` until upgrade-without-hangup, while an
   operator hosted default resolves the request but cannot start a host; any
   `--hosted --network` selection refuses `network_scope_unsupported` (exit
   16; native `--network` is the direct path below). No flag and no
   default stays native with no session-host probe. Hosted launches resolve
   permissions through the configured ladder with silence defaulting as on
   the native path and yolo admitted by the builder (Q-D3), with the dormant
   contract limited to Claude in Phase 1,
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
   starts neither provider nor ax. Hosted launch is unavailable until the
   swarma dispatcher path ships.
   Production receiver lookup fails closed with `session_host_missing`
   (exit 1), before any handoff, even with `task-board` on PATH; there is
   no native fallback. Existing injected-receiver tests cover the dormant
   payload contract and do not demonstrate a working hosted launch.

The permission interface is implemented against upstream
`skill-agents-management v0.5.62` (F-M1). It owns
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
go install github.com/relux-works/curator-agent-launcher/cmd/curator-run@v0.3.0
curator-run --version
```

The v0.3.0 release reports `0.3.0` (specification `0.5.1-draft`).
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
| `--hosted`, `--native`, `--untracked` | Hosted launch is unavailable until the swarma dispatcher path ships; production lookup refuses `session_host_missing` before handoff. One host request slot; `--untracked` equals `--native`; repeats and combinations refuse |
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
default with `ax` enabled refuses `host_configuration_conflict`. Hosted launch
is unavailable until the swarma dispatcher path ships: production receiver
lookup refuses `session_host_missing` before any handoff, and machine/operator
hosted defaults cannot start a host.
A failed handoff never starts a direct child. Repository tests use **fake ax
only**; they do not demonstrate an installed ax integration.

### Diagnostics and exit codes

Launcher failures print a stable code line followed by human-readable detail
on stderr. Foreign Curator/provider/ax output is forwarded unchanged.

| Result / diagnostic codes | Exit |
|---|---|
| Help/version, successful direct execution, or ax handoff | 0 |
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
and `macos-latest` with the toolchain from `go.mod`. An optional self-hosted
job runs `make check` on `[self-hosted, macOS, ARM64]` when its repository
opt-in is enabled; the workflow defines that setting. Enable it after
registering a matching runner. Goldens run as part of `make test` and `make race`. There are no
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
| `internal/mutantguard` | committed check that runs the required hosted mutants M-H1, M-H2, M-H3 and M-H22 through the harness in a scratch copy of the tree and requires each to be applied and killed | `go test ./internal/mutantguard` | scratch copy under the test temp dir; the working tree is never mutated |
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
