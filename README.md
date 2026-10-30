# curator-agent-launcher

`curator-run` composes Curator's managed environment, agents-management's
admitted interactive plan, and native or hosted execution. The contract is
[SPEC 0.5.1-draft](SPEC.md).

Documentation revision: accepted architecture-review findings (2026-10-13)
clarify role/placement admission and inert candidate review; the workplace
lifetime requirement and optional reviewer policies remain open in SPEC §9.
No implementation, qualification or activation is claimed.

## Front doors

| Entry | Composer | Process/lifetime owner | Status |
|---|---|---|---|
| Direct human `curator-run <env-id> …` | Caller-side interactive launcher | Launcher starts and waits for native child | Existing API below. |
| Hosted launch | Validated caller policy; plugin composes delegated launches | Registered session manager | Delegated launch requires a lease-capable selected manager; start currently retains protection at the process-incarnation gate. Refusal never falls back to native. |
| Trusted same-user non-chat | Explicit current-user assignment, frozen inputs and exact execution tuple | Target launcher adapter/supervisor; no child a2a/board presence | [Contract/1](docs/same-user-nonchat-contract.md) and [Stage1 wire source/2](docs/same-user-executor-wire.md), with closed records and inert manual cases; untrusted decoding/FD-argument syntax in `internal/memberwire` and bounded explicit N/A handle byte observations in `internal/memberinput` (Darwin/Linux), and retained content-addressed O observations in `internal/memberobjects` (Darwin/Linux), without full six-FD custody. **No admission, qualified tuple, executor or shipped CLI.** |
| Agent one-shot exec | Caller composes; dispatcher admits/provisions/tracks | `curator-run` under `worker-<label>` verifies and executes; dispatcher/helper own stop and cleanup | **Target amendment (version pending), not implemented**; [SPEC §10](SPEC.md#10-agent-account-one-shot-exec-contract-target-amendment). |

Front-door amendment (partially implemented; delegated activation signed off):
[FRONT-DOOR.md](FRONT-DOOR.md) makes `curator run` hosted by default through
a registered session-manager plugin, adds `curator run werkstatt up|down`,
Muse v4, setup-token injection, a2a doorbells and the signed distribution.
Delegated activation follows
[FRONT-DOOR §5.8](FRONT-DOOR.md#58-activation-boundary-for-launch-delegated1).
In `auto` mode, no registered or selected manager means native launch with a
one-line notice; explicit hosted instead refuses `session_host_missing` with
`reason=no manager registered|no manager selected` (after earlier validation).
Lazy registration, exact-plan activation and real Muse hosting remain proposals
subject to their own FRONT-DOOR gates. The launcher can request a Codex doorbell,
but `doorbell-codex/1` does not ship in this release.

The launcher-side protocol library is available in
`internal/sessionmanager/client`: validated RPCs, terminal fd4 transport,
request-derived capability decisions, and durable operation reconciliation.
Its [schemas and vectors](schemas/session-manager/README.md) are executable
through Go tests. The library is wired into host selection and plain hosted
launch/resume/attach and the Werkstatt lifecycle described below. A [reference fake manager and qualification suite](internal/sessionmanager/conformance/README.md)
exercise the plugin contract with a durable no-op provider. Real-binary tests
cover host-mode resolution, launch projection and refusal ordering.

## Werkstatt lifecycle

`werkstatt-record-v1` is a closed JSON file at
`${XDG_CONFIG_HOME:-~/.config}/swarma/launcher/werkstatt/<id>.json`.
The [record schema](schemas/session-manager/werkstatt-record.schema.json)
and [vectors](schemas/session-manager/vectors/werkstatt-record.json) show its
required fields and optional settings. The future board station source has
an interface only.

One profile belongs to each Werkstatt conversation. If a resumable target has
known liveness `profile` evidence that differs from the edited record, `up`
refuses `werkstatt_profile_changed` (exit 1, `id` only) before resolving the
fragment, writing hooks or the operation journal, or calling launch/resume.
Use `curator-run werkstatt recycle <id>` or
`curator-run werkstatt up <id> --new` to start a new generation and thread under
the new profile; fresh mode also starts a new generation. Liveness records the
submitted profile and preserves it through status, stop and operator edits.
The operation journal retains it for crash recovery even if the record changes.
Older liveness without `profile` skips the early check and keeps the manager's
existing resume-identity guard. Older journals settle with unknown profile
evidence. Neither missing value is inferred; malformed present values refuse.

Optional `profile_lock_sha256` pins the required profile lock as exactly 64
lowercase hex characters without a prefix. After resolving the profile with
repair, `up` compares it with the fragment’s `profile.lock_sha256`. A mismatch
refuses `werkstatt_profile_lock_mismatch` (exit 1, `id` only) before writing
hooks or `op.json` or calling launch/resume, and leaves liveness unchanged.
Neither hash is printed. An absent member preserves existing behavior; null
or malformed values refuse `werkstatt_record_invalid` (exit 2) with
`field=profile_lock_sha256` before any manager call.

```sh
curator-run werkstatt up example
curator-run werkstatt up example --attach --task-class code.implement --difficulty high
curator-run werkstatt down example
curator-run werkstatt down example --force
curator-run werkstatt recycle example
curator-run werkstatt update example --set-task TASK-example --queue-depth 1 --json
curator-run werkstatt update example --clear-task
curator-run werkstatt status example --json
curator-run werkstatt recover example --rebind-manager
curator-run werkstatt list --json
```

`up` always uses the selected session manager (delegated launches are signed
off; the trial flag is withdrawn). It checks existing ownership before repairing the
profile, verifies both lexical and physical strict ancestors for context leaks,
and checks only the presence of the identity file. Continue resumes the bound
conversation; `--new`, fresh mode, recycle or a manager switch starts a new one.
The child environment comes only from the record plus launcher-owned identity
and separate-git values. Credentials remain null. Doorbell configuration is
passed through. Claude transitional hooks are installed and verified before
start; runtime binding remains the manager's responsibility.

State, operation journal and lock files live under
`${XDG_STATE_HOME:-~/.local/state}/swarma/launcher/werkstatt/`. Legacy journals stay
until a terminal outcome and a successful liveness write. Lease-bearing
started journals additionally retain pending bind correlation as described below. Owner binding and
expected tuple CAS prevent stale writes. `recover --rebind-manager` explicitly
accepts a re-registered owner after a matching session read;
`recover --abandon-operation` records an operator override and a tombstone,
without stopping or cancelling anything remote. Dispatch edits require
liveness with a non-null session (`werkstatt_not_up` for both a missing
record and a pre-start override). A first settlement after a pre-start
override takes the manager's own generation on either binding.

`update` requires at least one dispatch field flag. `recover` requires exactly
one of `--rebind-manager` or `--abandon-operation`; each refuses the other
command's flags. Both accept `--json` and report the resulting status row.
The former mutating `status` forms retain their effects and stdout as
deprecated aliases, with exactly one replacement warning on stderr. They are
deprecated in 0.6.0 and removed no earlier than 0.8.0 (two releases later).

Ordinary `status` and `list` retain manager-evidence refresh and pending
operation reconciliation. They perform no dispatch edit, rebind or abandonment
without the deprecated flags. A settled read calls no mutating manager RPC;
reconciling a prepared/not-found pending operation retains its cancellation
fence. Mixed legacy dispatch/recovery flags keep recovery-before-edit ordering
and print one warning naming both replacement commands.

Status JSON is a bare array of closed rows
`{record,liveness,manager_session,a2a,stale}`. Manager and a2a evidence stay
separate. The bounded `a2a who` read uses `--stale-after` at three times
the configured doorbell heartbeat, or 3m without one; failures produce null
a2a evidence. This threshold is display-only. An unreadable manager record
sets state to unknown, state_source to manager and the heartbeat time; every
other field is preserved. `last_activity_at` may be null in any record.
A lock timeout always returns the JSON array as a read-only stale snapshot,
including null liveness before the first start. Recycle polls draining every
second for at most 20 seconds; a timeout never starts another generation.

## Hosted home leases

Every new hosted launch or resume requires the public Curator home-lease API.
Native launches make no lease calls. Fresh admission performs one manager
`describe`; only delegated routes with admission, status and cancel support
are eligible. An exact route refuses `session_home_lease_unavailable` with
`reason=route_not_lease_capable` before any lease call or further manager RPC.
The launcher probes `curator env lease list --format json`, persists the
original validated request and admitted registration/capability evidence, then
acquires with a stable request id. Missing, malformed or unsupported lease
support refuses hosted execution; there is no bypass.

Plain hosted journals live under
`${XDG_STATE_HOME:-~/.local/state}/swarma/launcher/hosted/`; Werkstatt uses its
existing journal directory. Recovery replays acquire before any manager RPC.
Released/broken replay retains a not-started settlement and makes zero manager
calls. A still-prepared, never-submitted request revalidates its frozen route
before one submission; a possibly submitted request uses status/cancel and is
never resent. Plain recovery matches canonical environment/profile/cwd and the
exact manager registration, skips held locks, refuses multiple matches and
ends the invocation after recovering one. Run the command again for a fresh
admission after settlement.

**Current bind limit:** session-record v1 supplies no authoritative provider
`{pid, started_at}`. After start, the launcher reports the validated session
handle and `session_home_lease_unavailable`,
`reason=process_incarnation_unavailable`. It retains the prepared lease and
pending bind journal, makes no fabricated bind call and does not attach or
report successful hosted admission. Recovery reconciles the same operation;
it never starts another generation to repair this gate.

Werkstatt checks all effective file records and the complete Curator lease
inventory before admission, then repeats the lease inventory check after
acquire. Conflicting home-to-lane or lane-to-home assignments refuse
`reason=allocation_conflict`; incomplete inventory also refuses. A configured
Get-only station source cannot establish complete inventory. Old blocking
allocations survive down, recycle, manager switches and abandon. A new tuple
is eligible only after authoritative terminal disposition of the old one.

The launcher sends **no release or break command on any path**. Explicit
`werkstatt recover <id> --abandon-operation` retains a settlement and prints a
separate operator break command using the current lease id/revision; an
unavailable revision produces an inspection hint instead. Neither abandonment
nor terminal manager status proves release authority. Settlements live under
`${XDG_STATE_HOME:-~/.local/state}/swarma/launcher/operations/settled/`.
Exact bind and safe release remain qualification gaps pending authoritative
process and independent observation contracts.

## Hosted doorbells

```sh
curator-run claude_code --hosted --a2a example.dev
curator-run codex_cli --hosted --detach --a2a example.dev
```

`--a2a` accepts `[a-z0-9._=-]{1,64}` and requires hosted execution. It asks
for a doorbell with heartbeat `1m` and six wakes per hour. Werkstatt records
supply their own values. A doorbell selects delegated launch/resume, even
when the manager also supports exact launch, under the post-sign-off policy
in FRONT-DOOR §5.8; `delegated_trial` no longer changes routing. Exact-only
managers refuse `session_home_lease_unavailable`, `reason=route_not_lease_capable`; missing runtime doorbell capabilities
refuse before hook installation or start.

For Claude, the launcher runs the resolved absolute `a2a` executable with
`hook install --agent <identity> --runtime claude --settings <staged file>`.
Its invocation PATH choice is frozen and printed in diagnostics; relative
PATH entries and candidates inside lexical or physical workspace,
`GIT_WORK_TREE` or `GIT_DIR` paths are excluded. The installer is bounded to
ten seconds. The private staging directory under the launcher state directory
is removed after success or refusal.

The launcher opens the workspace and `.claude` without following symlinks,
then pins the `.claude` descriptor. That directory must be owned by the
invoking user and have no group/world write permission. Settings reads are
bounded to 1 MiB; existing files must be regular, owned by that user and have
one hard link. The helper receives a copy of those bytes, or an absent staged
file when the target is absent. The launcher verifies `env.A2A_AGENT` and
matching command hooks for `SessionStart`, `UserPromptSubmit` and `Stop` before
committing through the pinned descriptor using an exclusive temporary file,
sync and same-directory rename. Existing permission bits are preserved;
new files get `0600`. A changed target device/inode, or an unexpected new
file, refuses `step=install`. This check detects a visible conflict; it is
not atomic with rename under the single-writer contract.

Readback and a fresh no-follow workspace/directory identity check follow the
commit. A moved pinned directory can receive the commit, but a substituted
symlink never receives a write and causes `step=verify`. Unrelated settings
and hooks may coexist. Any failure refuses
`doorbell_setup_failed step=install|verify` before start. Codex binding and
long-lived owners belong to the manager; the launcher installs no Claude
hooks for Codex.

A successful start reports one doorbell line on stderr. Text status reports
one line per returned manager session, including reason and uncertainty;
JSON retains the complete manager record without an extra doorbell line.
Presence from `a2a who` remains independent of this manager evidence.

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
C in our repository. The reviewer holds an explicit review-role grant, is
either a trusted-class local agent placement or a person admitted to the role,
and is outside the untrusted class and distinct from the producer. It reviews
exactly C; circle membership alone supplies no reviewer authority.
Write-scope/`.git`/`.task-board/` refusals and flagged-path hunk acknowledgement
are specified in SPEC §10.2. Review first consumes exact-C diffs/blobs as inert,
read-only resources bound by digest outside harness discovery roots. Candidate
instructions, hooks, MCP, permissions, skills and executable discovery are
never automatically adopted, including after acknowledgement or acceptance.
Tree entry through a discovery-capable harness requires prior qualification
of a strict-context adapter; root-owned launch surfaces alone do not suffice.
Acceptance does not admit control surfaces; CIP-0002 control-context admission
is separate. Semantic prompt injection remains a review residual.
Rework starts from C with review findings;
accepted exact heads land through the existing signed fast-forward path,
with refresh and re-review when the base moves. There is no worker push or
merge-request path. Export is a cleanup obligation before home deletion,
unless explicitly waived by operator act. Shared-object provenance/access/
retention and exact transfer formats remain open, not ownership.

R-STANDING requires the one-shot executor to check plan digests for settings,
hooks, MCP configuration and instruction files before exec (also restart and
native resume where admitted), or use root-owned surfaces outside the home.
Drift refuses launch (code unpublished); repair is explicit re-materialization.
Standing host loops require trusted class and local agent placement; circle
membership does not change placement. Outside standing workers use workplaces
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
Trusted-circle admission never relaxes host boundaries or supplies review-role
or host-loop eligibility. The optional local-only reviewer and requester-exclusion
policies remain unchosen and off (SPEC §9). The bridge/helper/dispatcher's stage-A
package must resolve the joint ingress/relay/start/export/reconnect lifetime and
generation-bound process-creation fence; SPEC §9 registers this requirement
without selecting a mechanism or changing normative behaviour. No OS is excluded
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
- [Launch paths](diagrams/launch-paths.puml) — native, hosted and target one-shot refusal/stop branches ([SPEC §§4.6, 4.9, 10.1–10.5](SPEC.md#46-hand-off-and-exec)).
- [Invocation states](diagrams/launch-states.puml) — transient states, terminal refusals and confirmed cleanup boundaries ([SPEC §§4, 6, 10.4](SPEC.md#4-composition-algorithm)).
- [Artifact layout](diagrams/artifact-layout.puml) — persistent owners, generated payloads, descriptors and metadata versus secret material ([SPEC §§4.1, 4.5–4.7, 4.9, 10](SPEC.md#47-the-launchers-configuration-file-family)).

Each figure labels current and target phases separately. The target is not
implemented; unpublished intake, receipts and activation details remain visible
as spec gaps. See the [figure index](diagrams/README.md).

The executable supports `claude_code` (alias `claude`), `codex_cli`
(alias `codex`), native `pi`, and Muse (`muse`).

Muse requires `launch-env-fragment-v4`: exactly four XDG parents under one
managed home plus `TBH_CREDENTIAL_BACKEND=file`, preserving inherited `HOME`.
Older Muse fragments refuse `fragment_version_unsupported` before launch.
The pinned agents-management v0.5.74 plugin owns the native plan; managed
Muse refuses module versions older than v0.5.66. The optional frozen-copy
tuple otherwise leaves binary resolution on the handed PATH. `muse-wonder`
resolves to `muse-wonder-1.0-contributor` with module-owned effort rules.

An explicit `root_context: {"exclusive":true}` supplies exactly
`-c context.foreign_personal_skills=false` in interactive launch. A native
`exec` or `serve` command with that fact refuses; this launcher exposes no
exec or dry-run command. Module-level Exec and DryRun tests both require
`--no-foreign-personal-context`. The fact is never inferred from files.
Contradictory backend overlays and native exclusion overrides refuse.
Native adds no posture flag; yolo adds exactly one `--yolo` after bounded
binary help evidence declares it. Malformed build evidence and mismatched
binary attestations fail closed. For example:

```sh
curator run muse --profile P --model muse-wonder
```

Help and version are static: `--help`, `-h` and `--version`, and `--help` or
`-h` anywhere in a `werkstatt`, `session-manager` or `migrate` invocation
(for example `curator run werkstatt status <id> --help`), are answered before
any configuration, registry or station state is read. They exit 0 even when
that state is malformed.

The launch pipeline follows these steps:

1. Parse the flag grammar, then check all four current/deprecated configuration
   locations for retired `ax.json` entries, without reading their contents.
   Any entry refuses `ax_tracking_removed` (exit 2), before policy parsing,
   resolution or launch. A usage error reports first; static help reads no config.
2. Normalize the `<env-id>` operand (`claude` → `claude_code`,
   `codex` → `codex_cli`) before validation or lookup, then resolve the
   fragment with `curator env resolve --repair --format json`, forwarding
   Curator stderr unchanged, and map its environment to a supported
   spawn system. Outputs carry the canonical id only; aliases are
   never persisted.
3. Resolve model and effort from flags, operator/machine defaults and the tagged
   lineup. Resolve permission mode by flag, Curator profile setting, launcher
   global default, then the built-in default. The mode is passed to
   `LaunchRequest.PermissionMode`; emit model/effort origins and the module's
   release-bound permission mapping before admission. Muse yolo reports
   `mapped=module-attested` only after the builder admits its help evidence.
   Pi uses the ordered convention `pi-anthropic`, `pi-openai`, `pi-google`; vendor
   scores are never compared. Explicit models bind their own runtime.
4. Resolve host mode with [FRONT-DOOR §3.2](FRONT-DOOR.md#32-resolution).
   Explicit native and machine native skip the registry. Explicit hosted checks
   network, terminal and stdin before reading the registry; auto
   reads the registry first. An empty or unselected auto registry prints the
   fixed native notice. Selected managers answer `describe` once; capabilities
   and the per-runtime activation matrix decide the route without failure
   fallback. Delegated launches are signed off (Q1, 2026-10-21): explicit
   `--hosted`, implicit auto and implicit hosted all use the selected manager,
   including a delegated-only one.
   A manager without an upgrade capability prints the upgrade disclosure.
   Hosted Claude/Codex permissions use the native ladder, including yolo.
   Resume selectors are elevated through the module's typed grammar.
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
7. Prepare native execution or a plugin request.
   Native launch retains its existing pre-exec checks and prompt preparation.
   Delegated launch sends the resolved profile hashes, model/effort, permission
   source, prompt semantics and selector-free native tail to `launch`, with a durable home-lease journal before submission.
   Current starts stop at the process-incarnation bind gate described above;
   generation-fenced `attach` is reachable only after successful binding. `--board-goal` asks for
   `workspace` and requires `board-goal/1`; ordinary requests use `none`.
   Selector decision ids are copied verbatim to the `selector` label. Credential
   requests remain null. Doorbell requests follow the hosted doorbell path
   above. Unsupported fragment revisions refuse before manager contact. Hosted
   Muse requires a manager-advertised launch shape and attach operation,
   including their global capabilities. Exact-plan routes refuse before lease acquisition.
   Lost launch/resume answers reconcile through their persisted original
   operation and owner; unresolved outcomes retain correlation and refuse
   `session_liveness_unknown`. No hosted refusal executes natively.

The permission interface is implemented against upstream
`skill-agents-management v0.5.74`. It owns
`LaunchRequest.PermissionMode`, the release-bound mapping, versioned native
argument classification, stored-policy inspection, and the capability table.
Claude Code and Codex CLI 0.153.2 use `permission-grammar-v2`, Codex CLI 0.164.0 uses `permission-grammar-v3`; Pi uses
`permission-grammar-v1`. The admitted result supplies
the plan and owned environment literals from the same prepared,
alias-projected request; composition never rebuilds the plan.

Codex v4 `credential_store` accepts absent or `native` policy without an auth
store override. The reader validates `file` and `keychain` with their matching
`config_overrides`, but the pinned module explicitly refuses both with
`plan_refused`; it leaves `config.toml` unchanged. Positive `-c` application
is an open upstream dependency: v0.5.74 sets
`curatorCodexCredentialStoreApplied=false` in
`pkg/agentic/curator_context.go:714–725`. A future released consumer is
required before those selections can launch. The launcher does not implement
a substitute argv mapping.

## Environment and transport

Native children receive the full composed environment, argv and raw stdin.
Hosted payloads export only owned literals and declared lookup names, never
the inherited environment. Attached empty stdin remains distinct from
unattached; native execution preserves binary stdin bytes exactly. Hosted
stdin capability checks remain independent.

Pi has no MCP channel. Its selected prompt flag and managed-home
`APPEND_SYSTEM.md` / `SYSTEM.md` candidates are checked freshly, including when
no prompt is selected. Warnings distinguish selected flags from conditional
native discovery. Reserved `path_prepend` remains parsed and hashed without
changing PATH or claiming managed command-root support. Reserved members are
marked in the fragment type and excluded from the execution context carrier;
hosted provenance retains the original fragment digest.

Launcher errors render through `internal/diagnostics`; usage exits 2 and
operational refusals exit 1. Foreign Curator/provider evidence is forwarded
without diagnostic framing. Direct child exit codes propagate unchanged and
signal exits return `128 + signal`. Hosted failures never fall back to a native child.

Execution waits for a child without allocating a PTY or replacing the launcher.
The execution package owns terminal foreground groups, signal forwarding,
stop/continue and terminal restoration. Pathname probes retain a race between
checking and process creation; they are not open-handle guarantees.

## Evidence boundaries

`cmd/curator-run/pipeline_test.go` drives production `run(...)` with the real
fragment parser, tagged admission, a temporary provider-limit store, and compiled
fake provider executables. Three native adapter goldens cover argv, environment,
stdin, warning output and side effects. Negative tests cover configuration order,
forbidden flags, admission, foreign-byte transport, exit status and late checks
on the native path. Injected attached stdin tests cover transport of a future admitted
payload; the shipped interactive plugins currently attach no stdin.

These tests do not install binaries or run real providers. Installed umbrella
launches, managed-home repair/current status, model/effort output and MCP remain
the orchestrator's post-landing verification. Independent acceptance and signed
PR delivery also belong to the orchestrator. No cross-platform runtime claim is
made from this host's tests.

## Install and discovery

Install the tagged release with the Go toolchain specified by `go.mod`:

```bash
go install github.com/relux-works/curator-agent-launcher/cmd/curator-run@v0.6.0
curator-run --version
```

The v0.6.0 release reports `0.6.0` (specification `0.5.1-draft`).
Compatible with curator v0.15.0-rc.3 or later, once published.

Alternatively, hosts without Go can download GitHub release artifacts.
Curator's distribution remains the canonical host channel; Curator's bundle
may also consume these artifacts if `curator_orch` chooses. Archives contain
`curator-run`, LICENSE, NOTICE and this README. The release matrix is Darwin
and Linux, each for arm64 and amd64. On an Apple Silicon Mac, choose a release
with downloadable artifacts and substitute its version for `X.Y.Z` below:

```bash
set -e
version=X.Y.Z
artifact="curator-run_${version}_darwin_arm64.tar.gz"
base="https://github.com/relux-works/curator-agent-launcher/releases/download/v${version}"
curl --fail --location --remote-name "$base/$artifact"
curl --fail --location --remote-name "$base/checksums.txt"
curl --fail --location --remote-name "$base/checksums.txt.pem"
curl --fail --location --remote-name "$base/checksums.txt.sig"
cosign verify-blob \
  --certificate checksums.txt.pem --signature checksums.txt.sig \
  --certificate-identity "https://github.com/relux-works/curator-agent-launcher/.github/workflows/release.yml@refs/tags/v${version}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
grep -F "  $artifact" checksums.txt | shasum -a 256 --check
tar -xzf "$artifact" curator-run
codesign --verify --strict --verbose=2 ./curator-run
codesign --display --verbose=4 ./curator-run
spctl --assess --type execute --verbose=4 ./curator-run
./curator-run --version
```

Require a successful checksum/signature check, then on macOS a Developer ID
Application signature for Relux Works, LLC and an accepted notarized result
from `spctl`. Before release builds, the pipeline requires either all five
signing fields (`MACOS_SIGN_P12`, `MACOS_SIGN_P12_PASSWORD`, `ASC_ISSUER_ID`,
`ASC_KEY_ID`, `ASC_KEY_P8`) or none. A partial configuration fails without
printing values. With all five, Darwin signing and notarization are required;
a signing or notarization error fails the release before publication. With
none, the unsigned path remains available, including for release publication;
its artifacts do not satisfy these macOS checks. Bare CLI binaries have no
stapled ticket, so
Gatekeeper assessment may need network access. On Linux choose `linux_amd64`
or `linux_arm64`, verify cosign and the checksum, and omit the macOS commands.
After verification, place the binary in a trusted operator-owned directory.

Linux `.deb` and `.rpm` packages for amd64 and arm64 are also generated as
GitHub release artifacts. They wrap the same unsigned Linux binaries and
install `curator-run` into `/usr/bin`. Choose the format and architecture for
your host, download the package and verify its entry in the authenticated
`checksums.txt` as above before installing, for example:

```bash
version=X.Y.Z
sudo apt install "./curator-run_${version}_linux_amd64.deb"
# Or on an RPM-based host:
sudo dnf install "./curator-run_${version}_linux_amd64.rpm"
```

Use the actual downloaded filename; no apt or yum repository is published.
Homebrew support is coming: the pipeline generates a `curator-run.rb`
formula with upload disabled. The tap and repository are still being chosen,
so no `brew tap` or tap-based installation is promised yet.

Ensure `$(go env GOPATH)/bin` is on `PATH`, or set `GOBIN` to a dedicated
trusted operator-owned directory on `PATH`. Do **not** install into Curator's
user-bin shim directory (`~/.local/bin`), a managed skill bin directory, or
beneath the environments root: umbrella discovery refuses these locations
with `subcommand_provider_untrusted` (environments.md §11).

Install Curator and the desired provider separately and make them available on
`PATH`; configure the provider's credentials and a Curator profile before launch.
The launcher installs neither providers nor profiles.

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
hosted session labels, and the `curator env resolve` call carry the
canonical id, and any other unknown spelling keeps the existing refusal.
Without `--profile`, Curator uses the current profile for the applicable
scope. Resolution always requests repair.

### Launcher options

| Option | Meaning |
|---|---|
| `--hosted`, `--native`, `--untracked` | Select one host request. Hosted uses the registered manager and activation gate; native skips the registry. `--untracked` is a deprecated exact alias of `--native`, retained for two releases with one warning; repeats and combinations refuse |
| `--board-goal` | Hosted workspace goal request; requires `board-goal/1` |
| `--detach` | Hosted launch without attach |
| `--a2a <identity>` | Hosted runtime doorbell request; delegated route and per-runtime capability required |
| `resume [SES-HANDLE]`, `--resume <id>` | Hosted-only resume selectors; usage errors in native mode |
| `--profile <name>` | Curator profile, forwarded to environment resolution |
| `--system-prompt <append\|replace>` | Explicit prompt-channel opt-in; unavailable semantics refuse the launch |
| `--model <model>`, `--effort <effort>` | Explicit spawn-plane selection, subject to admission and machine locks |
| `--permissions <native\|yolo>` | Resolve the launcher permission mode |
| `--yolo` | Exact alias of `--permissions yolo` at the same precedence level |
| `-d`, `--danger` | Rejected as usage errors before `--`; after `--`, arguments are native input |
| `--network-policy <policy>` | `optimistic` (default), `strict`, or `pinned:<binary SHA-256>`; sensitivity cannot be downgraded |
| `--network-known-bad <path>` | Explicit operator known-bad list; any load error refuses |
| `--network <profile>` | Network profile for direct execution; resolved after admission; refused with exit 16 when combined with `--hosted` |
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
in it, and a launch that composition refuses prints no Record. Hosted
launches refuse `--network` with `network_scope_unsupported`: the
source host cannot validate the destination.

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
provenance. Under strict or sensitive-egress policy the only qualified builds
are the operator's entries in `~/.curator/network-qualified.toml`; other
launches never read that file. An explicit `--network-policy strict` or
`pinned:` reads it on any profile. Each entry names an exact build by the SHA-256
of its executable, never by a version label, together with its qualification
evidence and date:

```toml
schema = "curator-network-qualified-v1"
project_roots = ["/Users/op/src"]   # filtering-gateway launches only

[[builds]]
harness       = "claude-code"
entrypoint    = "exec"
binary_sha256 = "<64 lowercase hex: sha256 of the executable>"
platform      = "darwin"
evidence      = "<the qualification record this entry rests on>"
qualified_on  = "2026-10-29"
```

An absent file is an empty list, so strict admission refuses every build as
before. A present file that cannot be read or does not validate refuses the
launch. Claude Code is the only harness the file admits in this revision.
Qualification never widens the independent child-scope ceiling, which admits
Claude print only, except under a filtering gateway: there the ceiling also
admits Claude Code's interactive session, and only a build listed for the
`interactive` entrypoint qualifies it (recipe `claude-interactive-v1`; an
`exec` entry does not). Hosted network launches remain unsupported.

A `kind = "filtering-gateway"` profile (curator-network-profiles N14) names a
local filtering gateway by its control socket and requires
`sensitive_egress = true`. Before the child starts, the launcher checks the
gateway's health and registers the launch (project root = the launch working
directory, an audit session id, runtime `claude-code`) over
`netproxy-control/1`. Any failure refuses `network_proxy_unreachable` with no
unfiltered fallback. The child then gets `ANTHROPIC_BASE_URL` set to the
gateway's base, `ANTHROPIC_CUSTOM_HEADERS` carrying the launch token in the
gateway's header, and `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`, in its
launch-private environment only. Inherited endpoint selectors are removed
from the child: every `ANTHROPIC_*`, `CLAUDE_*` or `CLAUDE_CODE_*` name ending
in `_URL`, `_URLS`, `_SOCKET`, `_ENDPOINT`, `_HOST`, `_BASE`, `_RESOURCE`,
`_PROXY` or `_GATEWAY` (Claude Code reads dozens, `ANTHROPIC_UNIX_SOCKET` and
`CLAUDE_CODE_API_BASE_URL` among them), every `CLAUDE_CODE_USE_*` selector
except the two that pick a local tool (`CLAUDE_CODE_USE_POWERSHELL_TOOL`,
`CLAUDE_CODE_USE_NATIVE_FILE_SEARCH`, which are kept), and the
`AWS_ENDPOINT_URL` names.

Claude Code applies the `env` block of its settings over its process
environment, so a settings entry would send the conversation, with the token
header, wherever it points. Before registering, and again at the pre-spawn
boundary, the launcher therefore refuses `network_configuration_conflict` when
a composed overlay, an MCP lookup, or any settings source it can read sets one
of those names, the gateway's own names or a proxy-family name (compared
without case). The sources are the
child's user settings (`CLAUDE_CONFIG_DIR`, else `~/.claude`), both
`settings.json` and `cowork_settings.json`; the legacy global configuration
(the one file Claude selects: the legacy `.config.json` there when it is a
regular file that is non-empty, or writable while `.claude.json` is absent,
else `.claude.json` in `CLAUDE_CONFIG_DIR` or the home directory; the other
file and any `.claude-*.json` are not sources), whose `env` Claude applies
first; every `.claude/settings.json`
and `.claude/settings.local.json` from the project root up to `/`, along the
given and the resolved physical path; and the managed settings
(`/Library/Application Support/ClaudeCode` on macOS, `/etc/claude-code` on
Linux: `managed-settings.json` and the `managed-settings.d` drop-ins), which
also refuse when they carry a policy helper or default policy payload. A
source that exists but is not a readable regular file (a dangling link, a
directory, a FIFO) or does not decode refuses; sources are read non-blocking,
a special file is refused from its inspection without being opened, and a
file replaced between inspection and open refuses. Each check runs under a
time budget (5 s) that is checked before every file operation, inside each
source read, and once more before the check admits; the managed drop-in
directory is opened as a directory and read for at most 65 entries, and more
than 64 refuse. A held SIGINT or SIGTERM cancels the check; either ends it
with a refusal. Registration itself is never cut short, so its outcome
is always known and ended. The operator catalog and its confirmation ledger
are read the same non-blocking way, whether the gateway is selected
explicitly or by default. A single system call that stalls inside the kernel
(a hung network filesystem) is not interrupted.
The native tail after `--` is held to an allowlist of Claude Code options
that stay local to the session they start, parsed with the runtime's own
value rules: `-p`/`--print`, `--verbose`, `-d`/`--debug`, `--debug-file`,
`--output-format`, `--input-format`, `--json-schema`, `--include-hook-events`,
`--include-partial-messages`, `--replay-user-messages`, the two
`--dangerously-skip-permissions` switches, `--permission-mode`,
`--permission-prompt-tool`, `--thinking`, `--thinking-display`,
`--max-thinking-tokens`, `--max-turns`, `--max-budget-usd`, the allowed and
disallowed tool lists, `--tools`, `--mcp-config`, `--strict-mcp-config`, the
four system-prompt options, `--exclude-dynamic-system-prompt-sections`,
`--model`, `--fallback-model`, `--effort`, `--agent`, `--betas`, `--session-id`,
`-n`/`--name`, `--no-session-persistence` and `--disable-slash-commands`.
Every other option refuses `network_configuration_conflict`: settings sources
(`--settings`, `--setting-sources`, ...), directories (`--add-dir`,
`-w`/`--worktree`, ...), stored sessions (`-r`, `-c`, `--from-pr`, ...), plugins,
and cloud or remote placement (`--cloud`, `--remote`, `--environment`, `--pool`,
`--attach-serve`, `--teleport`, `--remote-control`, `--sdk-url`, ...), as do
short-option clusters and an option the runtime adds later. Claude Code's
entry path also dispatches on the raw argument list before its option parser
runs, so option values and arguments after `--` count: any native argument
equal to a word it dispatches on refuses wherever it stands. These are every
command name and alias (`doctor`, `mcp`, `ultrareview`, `remote-control`,
`i`, `autoremove`, ...), the entry path's own modes (`remote`, `sync`,
`bridge`, `daemon`, `self-hosted-runner`, the background verbs, `import`),
and the raw tokens it or a later mode getter searches for (`--handle-uri`,
`--session-tunnel`, `--agent-teams`, `--hard-fail`, `--worktree`, `-w`,
`--tmux`, `--bg`, `--background`, `-c`, `--continue`, `-r`, `--resume`,
`--from-pr`, `--gh-standin`, the version shortcuts and the dashed helper
modes). `--safe-mode` and `--bare` are searched for only before the first
standalone `--`, so they refuse before it and are ordinary operands after
it. A prompt that is exactly such a word refuses too. The environment names
the runtime reads to pick another mode or session (the deep-link URL
handler's `__CFBundleIdentifier`, `CLAUDE_INTERNAL_FC_OVERRIDES`,
`CLAUDE_CODE_SESSION_KIND`, `CLAUDE_CODE_SAFE_MODE`, `CLAUDE_CODE_SIMPLE`,
`CLAUDE_CODE_RESTRICTED`, `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS`,
`CLAUDE_CODE_ACTION`, `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_PLUGIN_*`, and
the remote-session families `CCR_*` and `CLAUDE_CODE_REMOTE*`) refuse like
routes and are removed when inherited. The color flags the runtime also
reads from the raw argument list change only output colouring and are not
reserved. Claude Code's own settings sources are read from the
environment the child is composed with, the profile fragment's
`CLAUDE_CONFIG_DIR` included. Once the child has started,
the launcher activates the registration within its activation window (no
attempt starts or outlasts it; on expiry the launcher alerts and ends the
registration) and renews it hourly. After the child exits, whatever its
status, the launcher unregisters it, retrying and alerting on stderr until the
gateway confirms; a refusal between registration and spawn unregisters as
well. From registration until the unregister completes, curator-run holds
SIGINT and SIGTERM: one before the boundary check aborts the launch, one after
it reaches the child as soon as the child starts, those while it runs reach
the child, and one during the unregister retries abandons them with an alert
that the registration ends at the gateway's idle bound. The token never appears in argv, the binding Record or
any diagnostic. Without `--network`, an operator `default` that names a
filtering gateway selects it with origin `operator-default`, and every hosted
route, resume included, then refuses `network_scope_unsupported` before any
session-manager call. Defaults of the other kinds still need `--network`. A
`network.toml` that exists but cannot be read, a dangling link included,
refuses every launch. Codex launches through a gateway are
refused in this revision.

Claude Code under a filtering gateway is also held to admission contract A,
checked before registering and again at the pre-spawn boundary. A launch that
fails it refuses `network_scope_unsupported` with no gateway call:

- "exact model" is the exact qualified Claude Code build: its binary
  SHA-256 in the strict tuple (above). Routing does not depend on the LLM
  model, so the model is not checked;
- the child runs in a managed home: `CLAUDE_CONFIG_DIR` is an absolute ASCII
  path (so it equals the NFC form Claude Code reads settings from), its
  `settings.json` exists, and it, `cowork_settings.json` and the legacy
  `.config.json`, where present, are regular files with no write bit;
  `.claude.json`, which Claude Code writes itself, is read but may be
  writable;
- the physical working directory, after resolving every symbolic link, is
  inside one of the operator's `project_roots` in
  `~/.curator/network-qualified.toml` (absolute, clean, existing paths; none
  listed admits no gateway launch);
- the native tail holds only allowlisted options, so worktree, resume, cloud and remote placement refuse (above);
- the host has no Claude Code managed preferences
  (`/Library/Managed Preferences/com.anthropic.claudecode.plist`, machine-wide
  or for the current user) and the managed home holds no cached server policy
  (`remote-settings.json`, `policy-limits.json`).

Contract A narrows the launch; it does not isolate it. Its written residual:

- Claude Code re-reads its settings during a session, so a settings change
  after the boundary check (a mid-session reroute) leaves the rest of that
  conversation unfiltered;
- "not writable" is checked on file modes; while a session runs as the same
  OS user as its managed home, it can change those modes, and the managed
  home directory itself stays writable because Claude Code writes there;
- `<CLAUDE_CONFIG_DIR>/.claude.json` stays writable, because Claude Code
  writes it; the guard refuses a route-bearing `env` in it before spawn, and
  an edit after the check is the same reroute as the first item;
- MDM profiles and server-managed policy cannot be read by the launcher. The
  gate is the absence of their local traces named above (the MDM plists and
  the policy-cache files); policy a runtime fetches from its server at start
  or refresh is not checked;
- the launch token is usable only at the gateway's same-host loopback
  listener.

Per-user packet filtering, with each session running as its own OS user, is
the fix for the first two items and is not part of this launcher. Until it
exists, the gateway is selected per launch with `--network`; the operator
`default` stays unset on shared hosts.

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
have no launcher-owned session record. Hosted guards refuse network
admission until their session owner stores that provenance. The existing
network binding line remains separate.

Admission identifies the native binary bytes, not the version label. The
launcher executes those exact bytes from a private staging directory and
removes the stage after the child exits. Scripts/interpreter wrappers refuse
with `runtime_closure_unsupported`; no dependency closure is claimed as
qualified merely because a file has a native executable header. See SPEC
§4.4b and §6.

### Configuration family

The launcher owns `defaults.json`, `launcher.json`, `session-managers.json` and `selector.json` in the operator directory
`${XDG_CONFIG_HOME:-~/.config}/swarma/launcher` (default `~/.config/swarma/launcher`) and machine
directory `/etc/swarma/launcher`. These are separate from Curator's configuration.
All use closed schemas: unknown members, malformed data and unreadable files
refuse the invocation; absence alone permits fallback.

`launcher.json` uses the closed schema `curator-run-launcher-v1` and optional
`host_mode` (`auto`, `native`, `hosted`) in the system layer only. Absence
means `auto`; an explicit value has source `machine`, absence source `default`.
The optional system-only `delegated_trial` boolean is withdrawn since the Q1
sign-off: it is still validated as a boolean but changes no route.
A user-layer `host_mode` or `delegated_trial` refuses and names the system path. Host selection uses this mode and the per-launch flag. The v3 defaults `host` member is withdrawn: it refuses
`host_member_withdrawn` (exit 2), naming `launcher.json`; see
[FRONT-DOOR §3.1](FRONT-DOOR.md#31-the-machine-mode).

For one release, missing active configuration files fall back individually to
`/etc/curator-run/<name>` or `${XDG_CONFIG_HOME:-~/.config}/curator-run/<name>`.
Each read prints `curator-run: deprecated config path <old>; move it to <new> (curator-run migrate)`.
A present new file wins without opening the old one; all selected files keep
the same file-security checks.

Run `curator-run migrate --dry-run` to print the complete migration plan,
then `curator-run migrate` to apply it. Active legacy sources remain untouched;
retired ax policy files are handled as described below. Destinations are copied atomically, with `.bak` bytes for rewritten copies, and rollback
on failure. Destination equality compares the final transformed bytes; a
different file refuses `migrate_destination_exists` before writing anything.
An exact raw legacy copy can be transformed in place with its original bytes
in `.bak`; final transformed bytes count as already migrated.
Uniform old machine `host` values become `launcher.json` `host_mode`; mixed
values refuse. User `host` values are listed and dropped. Unwritable system
files are listed once for a privileged installer. A repeated run is a no-op.

`selector.json` is optional, system-only policy with schema
`curator-run-selector-v1`. It requires `mode` (`shadow` or `enforce`), an
absolute `executable`, and `argv_prefix` (0–4 words matching
`[a-z0-9-]{1,32}`). Optional `timeout_seconds` is 1–60, default 5, and applies
only to enforce. Any user-layer file refuses `defaults_config_invalid`.

The selector receives one bounded JSON request containing the profile, cwd,
explicit model/effort flags, and exact admitted runtime/model/effort tuples
from the same agents-management lineup and registry used by defaults. Plain
launches carry null role, project, Werkstatt and task fields. An effort-less
candidate and response use JSON null; empty effort strings are invalid.
Responses are closed, limited to 64 KiB, and must choose an exact candidate.
Decision/runtime/model/effort strings follow `[A-Za-z0-9._:/@+-]{1,128}`;
non-null weights use `gguf-sha256:` followed by 64 lowercase hex digits.

Enforce waits within the deadline, applies the choice above file defaults and
below explicit flags, and reports `selector=<decision_id>` in the defaults
line. Runtime remains bound through the existing mapped system; the selector's
runtime is retained as provenance. A typed abstention refuses
`selector_abstained` (exit 1, `code`) only when runtime, model, effort and
weights are all null; a mixed refusal and choice is invalid. Transport or validation failure refuses
`selector_unavailable` (exit 1, `reason=timeout/exit/invalid`). Neither falls
back to defaults.

Shadow leaves composition and output unchanged. After composition it starts a
worker in a separate session/process group without a terminal. The worker owns
the selector IO and appends a decision record; the launcher appends its applied
values and launch outcome. Records join by the SHA-256 digest of the CCJ-1
request in `${XDG_STATE_HOME:-~/.local/state}/swarma/launcher/selector/observations.ndjson`.
The launcher attempts the append lock without waiting; contention or failure
drops the line and best-effort increments `selector/dropped` beside the log.
The detached worker can wait for this lock for at most five seconds. Observation
and counter paths must be regular files, never symlinks. Selector and observation
failures never change the launch. A fresh hosted launch carries the decision id
verbatim in its `selector` label. Plain resume uses an empty label filter and
retains the admitted session labels and policy.

The session-manager registry uses the closed schema
`curator-run-session-managers-v1`: `{schema, selected?, managers}`. A system
registry decides alone; the user registry is opened only when it is absent.
Each manager has an id matching `[a-z0-9][a-z0-9-]{0,31}`, an absolute
`executable`, 0–4 `argv_prefix` tokens matching `[a-z0-9-]{1,32}`, protocol
`curator-session-manager/1`, a UTC RFC 3339 `registered_at`, and
`registered_by` (1–128 printable ASCII bytes). No manager lookup searches
`PATH`. The state reader exposes empty, unselected, or selected; a broken
selected executable refuses rather than becoming empty. Launch selection
is not wired to the registry in this slice.

```sh
curator-run session-manager list [--json]
curator-run session-manager show <id> [--json]
curator-run session-manager register --manifest <path|-> [--select | --no-select] [--scope user|system]
curator-run session-manager use <id> [--scope user|system]
curator-run session-manager unregister <id> [--scope user|system]
curator-run session-manager observe <manager-id> --session <SES-handle> --json
```

Observation requires both flags and uses the explicit registration, even with no
selected manager. It sends only `describe` and one `status`, with the existing
10 s per-call deadline. It emits the registration incarnation, the validated
session record (without its RPC envelope), and a launcher UTC timestamp. Nulls,
unknown and unreadable evidence remain unchanged; unknown never means down.
Refusals leave stdout empty and print one closed diagnostic, with no plugin text.
It never reads or writes launcher liveness, journals or Werkstatt state, and
never locks or changes the registry. See [the contract](FRONT-DOOR.md#read-only-session-observation)
and [the output schema](schemas/session-manager/session-observation.schema.json).

`list --json` prints the registry object plus `layer`; `show --json` prints
the entry plus `id` and `layer`. The CLI-only layer is `system`, `user` or
`none` (list when no file exists), including legacy fallback. Text starts with
`layer: <layer>`, then listing sorts by id and marks the selected manager with
`*`. The registry file schema remains unchanged.
The registration manifest is an entry plus `id`, without `registered_at`;
registration resolves executable symlinks, validates a regular executable,
and stamps UTC. All mutations write the system layer if writable, otherwise
the user layer; a system registry makes a user mutation refuse as shadowed.
Explicit `--scope system` refuses if unwritable (`session_manager_registry_unwritable`,
exit 2, `path` = registry directory, `scope=system`), before locking or writing,
without falling back.
Explicit `--scope user` writes user only and still refuses when a system
registry exists. An unshadowed, unwritable explicit user target likewise refuses
`session_manager_registry_unwritable` before locking or writing, `scope=user`.
This diagnoses directory write authority separately from invalid/unreadable
registry content. Missing, invalid or duplicate scope is `usage` (exit 2) with
no effect. Omitted scope preserves the existing layer policy. `select` remains
a deprecated alias for `use`: identical stdout and effect, one stderr warning;
deprecated in 0.6.0, removed no earlier than 0.8.0 (two releases).
A newly created system registry starts empty. Explicit flags decide selection;
otherwise an existing id keeps it, the first manager is selected, and adding
a peer asks once on a terminal (default: keep). Without a terminal the peer
is registered without selecting and the command prints the explicit `use`
command. Unregister never selects a replacement. Writes use a same-directory
temp file, file fsync, rename, and directory fsync. Each mutation holds an
OS advisory `session-managers.json.lock` in its target directory across reading,
selection, writing and rollback, including the terminal question. A contender
waits up to 10 seconds before `session_manager_registry_busy` (exit 1, no details).
A terminal answer expires after 120 seconds with `usage` (exit 2). Failed
commands leave the previous registry bytes unchanged. The lock file persists;
the OS releases its lock when the holder exits or crashes.

Model and effort resolve independently: flags, then operator defaults over
machine defaults per member, then the admitted lineup. Permission mode has its
own precedence: `--permissions` or `--yolo`, Curator's per-profile setting in the
fragment, the launcher-global file value, then the built-in default. A v3 file can set model, effort and permission defaults independently:

```json
{
  "schema": "curator-run-defaults-v3",
  "locked": false,
  "defaults": {
    "codex_cli": { "effort": "high" },
    "claude_code": { "permissions": "yolo" }
  }
}
```

Each environment entry may contain `model`, `effort`, and/or `permissions`;
permission values are exactly `native` or `yolo`. A v3 reader accepts v1 and v2 files unchanged; older
readers reject the members they do not know. The permission
fragment member uses the F-S2 names `permissions.mode`, `permissions.locked`,
and `permissions.source`; `source=default` means the profile level is silent.
Native and hosted silence default to `yolo` with `source=default-interactive`
on every stdio shape; headless observations never change that default.
`native` means no launcher override and does not guarantee prompting; native
arguments after `--` remain untouched. Curator's `permissions.locked=true`
force-native lock outranks the ladder: launcher-visible `yolo` is `usage`,
while silence resolves to native. Without established permission transport,
a would-be `yolo` refuses `permission_policy_unsupported`.

Hosted posture exports as `execution_profile` `standard`/`yolo` in the payload. A piped launcher stdin
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
clear.
Long `permissions.allow` lists are summarized by rule count in stderr.

Pi fallback uses the ordered convention `pi-anthropic`, `pi-openai`,
`pi-google`, selecting the first runtime with a driven model and ranking only
within that runtime. This is an operator convention, not a comparison of vendor
scores. An explicit/configured model binds its own runtime independently.

### Removed ax tracking

Ax tracking was removed on 2026-10-27. `--name` and `--ax-profile` before
`--` are usage errors (exit 2), with a message saying removed in 0.6.0.
`--untracked` remains an exact alias of `--native` for two releases and
prints exactly one deprecation line.

Any `ax.json` entry in a current system/user launcher directory or a deprecated
system/user `curator-run` directory refuses `ax_tracking_removed` (exit 2).
The diagnostic names only its launcher-sourced path and asks to delete it.
The guard never reads/parses the contents, applies regardless of host flags,
and also rejects symlinks, directories and FIFOs. Static help/version remain
configuration-free.

`curator-run migrate --dry-run` lists every leftover. Apply backs up each
regular file as `.bak` and removes it transactionally. It never copies the
retired policy or backup into the new layout. Non-regular entries require
manual removal: they are listed without following them, and apply refuses
before any writes. Existing system-write authority rules still apply.

### Diagnostics and exit codes

Launcher failures print a stable code line followed by human-readable detail
on stderr. Foreign Curator/provider output is forwarded unchanged.

| Result / diagnostic codes | Exit |
|---|---|
| Help/version, successful native execution or hosted operation | 0 |
| `ax_tracking_removed` (retired policy entry; delete the named file) | 2 |
| `usage` (invalid arguments, locked-member override, host-request repeat, native resume selectors, or visible `yolo` under Curator's force-native lock) | 2 |
| `session_manager_registry_busy` (lock contention after 10 seconds) | 1 |
| `session_resume_invalid`, `launch_plan_invalid`, `host_member_withdrawn`, `migrate_destination_exists`, `session_manager_registry_invalid`, `session_manager_registry_unwritable`, `session_manager_registry_shadowed`, `session_manager_not_registered` | 2 |
| `resolve_invocation_failed`, `resolve_environment_unknown`, `resolve_profile_unknown`, `resolve_repair_failed`, `resolve_lock_unavailable`, `resolve_fragment_invalid` | 1 |
| `defaults_config_invalid`, `defaults_unresolvable` | 1 |
| `permission_policy_unsupported`, `permission_mode_unsupported` | 1 |
| `plan_refused`, `plan_provider_limited`, `env_unsupported` | 1 |
| `exec_provider_missing` | 1 |
| `mcp_layer_missing`, `mcp_layer_unreadable` | 1 |
| `sysprompt_channel_unavailable`, `sysprompt_file_unreadable` | 1 |
| `network_profile_unknown`, `network_profile_denied`, `network_scope_unsupported`, `network_configuration_conflict`, `network_proxy_unreachable`, `network_proxy_auth_failed`, `network_profile_invalid`, `network_file_unreadable` | 1 (hosted `--network`: `network_scope_unsupported` exits 16) |
| `session_host_missing`, `session_host_unavailable`, `session_home_lease_unavailable` | 1 |
| `session_host_protocol_unsupported`, `session_host_provider_unsupported`, `session_host_scope_unsupported`, `session_host_execution_profile_unsupported`, `session_host_terminal_required`, `session_host_stdin_unsupported` | 6 |
| `secret_policy_violation`, `policy_refused`, `session_host_delegated_unauthorized` | 16 |
| Direct child failure | Child's exit code unchanged |
| Direct child terminated by signal | `128 + signal` |
| Hosted receiver exit without a refusal record | Receiver exit unchanged |

See [SPEC §6](SPEC.md#6-errors-and-diagnostics) for each refusal condition.
No refusal retries with a different model or weaker launch.

## Development

```bash
make build      # go build ./...
make fmt-check  # gofmt -l over cmd, internal and schemas; fails on unformatted files
make vet        # go vet ./...
make test       # go test ./... -count=1 (includes the CLI goldens)
make race       # go test ./... -count=1 -race -timeout=12m (per package)
make check      # all of the above
make release-binary VERSION=0.6.0 [GOOS=… GOARCH=…]  # reproducible binary + .sha256 in dist/
make release-repro                                 # two checkouts, identical bytes required
```

`make release-binary` produces the binary that Curator's signed distribution
ships (FRONT-DOOR §10). It builds with `CGO_ENABLED=0`, `-trimpath`,
`-buildvcs=false` and `-ldflags "-s -w -buildid= -X main.buildVersion=…"`, using
`GOFLAGS=-mod=readonly` and the toolchain pinned from `go.mod`. Its inputs are
treated as data and validated before use, so a crafted `VERSION` cannot run
anything. It writes
`dist/curator-run-<version>-<os>-<arch>` and a checksum file in `sha256sum`
format. Signing, notarization, packing and the release tag belong to
Curator's release workflow, not to this repository.

CI (`.github/workflows/ci.yml`) runs the same targets on `ubuntu-latest`
and `macos-latest` with the toolchain from `go.mod`. An optional self-hosted
job runs `make check` on `[self-hosted, macOS, ARM64]` when its repository
opt-in is enabled; the workflow defines that setting. Enable it after
registering a matching runner. Goldens run as part of `make test` and `make race`. A
`release-repro` job runs `make release-repro` on both hosted runners. There
are no release or tag jobs. Windows is not in this launcher's hosted matrix;
platform execution evidence must come from the corresponding runner.
The race target allows 12 minutes per package for the conformance suite's
intentional deadline controls and slower race runners; the hosted race job
retains its separate 15-minute overall limit. Repeated standalone conformance
runs need their own explicit timeout budget, for example
`go test ./internal/sessionmanager/conformance -race -count=2 -timeout=20m`.
The filtering-gateway signal tests build a second launcher with the
`gatewayphases` build tag (`go test ./cmd/curator-run -run
TestGatewayRealSignalBarriers`). In that build only, the launcher waits at
named phases of a gateway launch (the final recheck, the child's start and
exit) while the test sends it a real signal; in every other build the phase
marks do nothing. Release binaries are built without the tag.

### Tools

Workflow actions are pinned to full commit SHAs, with patch-version comments.
To update a pin, resolve the selected tag in that action's own repository with
`gh api repos/<owner>/<repo>/git/ref/tags/<version>`. If `object.type` is
`tag`, follow `gh api repos/<owner>/<repo>/git/tags/<object.sha>` until it
names a commit. Review that commit's action entrypoint and execution code,
then update the SHA and version comment together. Run `actionlint` and the
release admission controls before review; never replace a pin with a mutable tag.
The release admission job has only `contents: read`; the dependent publish job
gets release/OIDC permissions. Only its pinned GoReleaser step receives the
five signing secrets.

| Tool | Purpose | Entry point | Output |
|---|---|---|---|
| `curator env lease` | public home-lease probe and reservation/recovery; the launcher never releases or breaks | `curator env lease list --format json`; hosted launch/resume invokes `acquire` with its persisted holder | closed JSON on stdout; launcher journals and settlements under the XDG state home, as described above |
| `curator-run session-manager` | inspect and atomically update the manager registry | `curator-run session-manager list/show/register/use/unregister (deprecated alias: select)` (forms above) | stdout or JSON; `session-managers.json` in the effective target config home |
| `curator-run session-manager observe` | read one registered session without local state changes or reconciliation | `curator-run session-manager observe <manager-id> --session <SES-handle> --json` | one closed `curator-run-session-observation-v1` JSON object on stdout; no files |
| `curator-run werkstatt` | hosted station lifecycle and separate manager/a2a status | commands above; `go test ./cmd/curator-run -run TestWerkstattBinary -count=1 -v` | liveness and journal in the XDG state home; bounded test children in `t.TempDir` |
| `.scripts/werkstatt-mutants.py` | owner, workspace, admission, CAS, stale output, evidence and dispatch negative controls | `python3 .scripts/werkstatt-mutants.py [evidence-dir]` | logs and `summary.tsv`, default `.temp/werkstatt-mutants/`; restores exact source bytes; run alone |
| `.scripts/selector-mutants.py` | bounded selector config, response, transport, precedence and shadow controls | `python3 .scripts/selector-mutants.py [evidence-dir]` | per-probe logs and `summary.tsv`, default `.temp/selector-mutants/`; restores source bytes |
| `.scripts/doorbell-mutants.py` | bounded hook containment, refusal schema, PATH trust, doorbell projection, routing and surfacing controls | `python3 .scripts/doorbell-mutants.py [evidence-dir]`; inventory: `python3 .scripts/doorbell-mutants.py --list` | per-control logs and `summary.tsv`, default `.temp/doorbell-mutants/`; restores source bytes; run before other checks |
| `.scripts/plugin-client-mutants.py` | bounded plugin transport, schema, capability, reconciliation and CAS controls | `python3 .scripts/plugin-client-mutants.py [evidence-dir]`; inventory: `python3 .scripts/plugin-client-mutants.py --list` | per-control logs and `summary.tsv`, default `.temp/plugin-client-mutants/`; restores candidate bytes; run before other checks |
| `.scripts/session-manager-mutants.py` | bounded registry/registration/atomic-write, transaction-lock and deadline negative controls | `python3 .scripts/session-manager-mutants.py [evidence-dir]` | per-probe logs and `summary.tsv`, default `.temp/session-manager-mutants/`; restores source bytes |
| `curator-run-fake-manager` | durable reference session-manager plugin and separate fixture adapter | `go build -o .temp/curator-run-fake-manager ./cmd/curator-run-fake-manager`; see [qualification](internal/sessionmanager/conformance/README.md) | explicit `--state` directory; no-op provider, no daemon |
| `curator-run-fake-manager-fixture` | separate out-of-band qualification controls | `go build -o .temp/curator-run-fake-manager-fixture ./cmd/curator-run-fake-manager-fixture` | explicit `--state` directory shared with the fake plugin; JSON fixture evidence |
| Session-manager conformance | qualify advertised plugin operations and controlled lifecycle faults; prove known-bad detections | `go test ./internal/sessionmanager/conformance -v`; `go test ./internal/sessionmanager/conformance -race -count=5 -timeout=45m` | Go test output; child binaries/state in `t.TempDir` |
| `.scripts/fake-manager-mutants.py` | bounded source controls for fake-manager/conformance invariants | `python3 .scripts/fake-manager-mutants.py [evidence-dir]` | per-probe logs and `summary.tsv`, default `.temp/fake-manager-mutants/`; restores exact source bytes |
| `curator-run migrate` | copy legacy configs and remove withdrawn host defaults | `curator-run migrate [--dry-run]` | plan on stdout; new files in `/etc/swarma/launcher/` and `${XDG_CONFIG_HOME:-~/.config}/swarma/launcher/`, rewritten copies keep `.bak` |
| Session-manager protocol module | dependency-free wire schemas, strict reader and vectors; provider checks stay with the consumer | `go -C schemas/session-manager test ./... -count=1`; included in `make check` | stdout; dependency-control children in `t.TempDir` |
| Schema module parity | shallow-checkout-safe comparison of the in-repo contract against the published schema pin; excludes only its root README | `go test ./internal/schemaparity -count=1 -v`; included in `make check` and CI | stdout; comparison fixtures in `t.TempDir` |
| `internal/memberwire` | pure untrusted request/authorization/owner/catalog/tuple, lifecycle snapshot/head/cancel/observation/scope-proof and result/response/reservation/ack/conflict structure and six descriptor-argument syntax; no handle I/O, compatibility admission or private verification | `go test ./internal/memberwire`; included in existing PR test/race jobs | test stdout; published inert records read as bytes, no native execution |
| `internal/memberobjects` | bounded explicit O collection, immutable bytes and owned readonly CLOEXEC handles; explicit recheck/Close, cooperative freeze only | `go test ./internal/memberobjects`; included in existing PR test/race jobs | test stdout and owned TempDir fixtures only; no admission/native execution |
| `internal/memberinput` | bounded offset-independent N/A handle byte snapshots on Darwin/Linux; caller-owned cooperative freeze, no full custody/admission | `go test ./internal/memberinput`; included in existing PR test/race jobs | test stdout and owned TempDir files only; no native execution |
| `go` (build, vet, test, race) | build and behavioral suite | `make check` and the targets above | stdout; nothing written to the tree |
| `make release-binary` | reproducible release binary for the Curator distribution | `make release-binary VERSION=<semver> [GOOS=… GOARCH=…] [DIST=dist]` | `dist/curator-run-<version>-<os>-<arch>` and `.sha256` (gitignored) |
| GoReleaser v2.17.0 | four-platform archives, Linux deb/rpm and a Homebrew formula generated without tap upload; optional Darwin Developer ID signing/notarization, then checksum-only keyless cosign | `goreleaser check`; `goreleaser release --snapshot --clean --skip=sign,notarize,publish` | `dist/` (gitignored); snapshot is unsigned and never published |
| Homebrew and package inspection | generated-formula style and read-only deb/rpm payload inspection; not needed on the release runner | `HOMEBREW_NO_AUTO_UPDATE=1 brew style dist/homebrew/Formula/curator-run.rb` (requires Homebrew's style gems); `dpkg-deb -c <package.deb>`; `rpm -qlp <package.rpm>` | stdout; extraction for byte comparison stays under `.temp/` |
| `scripts/release-go.sh` | GoReleaser's Go tool pinned from go.mod, matching `release-binary` | `scripts/release-go.sh version`; invoked by `.goreleaser.yml` | stdout and GoReleaser's `dist/` |
| `scripts/release-gate.sh` | require an annotated SSH-signed stable tag from `.github/release-signers`, its exact checkout and matching buildVersion | `scripts/release-gate.sh v<semver>`; tag-push-only `.github/workflows/release.yml` runs it before credentials reach GoReleaser | stdout; no mutations |
| `scripts/release-signing-gate.sh` | require all five signing fields or none before GoReleaser release builds; print only configuration mode | GoReleaser `before.hooks`; `scripts/release-signing-gate.sh` | stdout/stderr; no build, signing or network effects |
| Release admission fixtures | trusted SSH positive and 13 tag refusals; all 32 signing-field subsets plus explicit-empty control, with unchanged state and no values logged | `make release-gate-test`; included in `make check` and CI | isolated `.temp/release-gate-test.*`, removed on exit |
| actionlint | static validation of GitHub workflows | `actionlint .github/workflows/release.yml .github/workflows/ci.yml` | stdout |
| cosign, curl, shasum, tar, codesign, spctl | authenticate, download, unpack and verify release artifacts | installation commands above | downloaded archives, checksums/signatures and extracted `curator-run` in the current directory |
| `scripts/release-binary.sh` | the release build itself; validates VERSION, GOOS, GOARCH and DIST before any use (the Makefile passes them as unexpanded data) | `make release-binary …` (not run directly) | as above; invalid input exits 2 and writes nothing |
| `scripts/release-repro.sh` | committed-HEAD reproducibility across two checkout paths, checksum and `--version` checks, a different-version control, and crafted-input negatives that must run and write nothing | `make release-repro` | temporary directory, removed on exit; one `OK <sha256>` line |
| `python3` defaults mutant harness | weaken defaults gates individually and run the behavioral suite; restore exact candidate bytes | `python3 .scripts/defaults-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv` (default `.temp/defaults-mutants/`) |
| `.scripts/context-mutants.py` | check positive baselines, then narrow carrier engagement, reserved-member exclusion, permissions, deprecated-alias refusal, native suffix and replacement of the selected dependency in an isolated candidate copy; scrub fixture Git routing/config, with a five-minute bound per Go invocation | `python3 .scripts/context-mutants.py [evidence-dir]` | isolated copy, positive/per-mutant logs, `selected-module.json`, `restoration.json` and `summary.tsv`, default `.temp/context-mutants/` |
| production pipeline mutants | narrow native late checks at `run(...)` in a scratch copy | `python3 .scripts/pipeline-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv`; 6 named narrowing probes |
| CLI goldens | frozen accepted/rejected §3 shapes | `go test ./internal/cli -run TestGolden -update` to regenerate, then review the diff | `internal/cli/testdata/cases.golden` |
| `.scripts/hosted-mutants.sh` / `.scripts/host-launch-mutants.py` | Bounded host resolution, matrix and request controls; each plant requires its passing control and named behavioral assertion. Missing anchors fail closed. | `.scripts/hosted-mutants.sh [evidence-dir]` (`MUTANTS_ONLY=M-H1,M-H2` selects a subset) | isolated source copy, per-control logs and `summary.tsv` under `.temp/hosted-mutants/` by default; working source is untouched |
| `internal/mutantguard` | committed check that runs required withdrawn-host, no-fallback, native-bypass and missing-registration mutants M-H1, M-H2, M-H3 and M-H22 through the harness in a scratch copy of the tree and requires each to be applied and killed | `go test ./internal/mutantguard` | scratch copy under the test temp dir; the working tree is never mutated |
| `.scripts/cli-mutants.sh` | narrowing-mutant harness for the §3 gates: each mutant weakens one gate to admit one rejected shape and the named test must fail | `.scripts/cli-mutants.sh [evidence-dir]` (optional `MUTANT_PKGS`, `MUTANT_RUN` for bounded batches) | per-mutant logs and `summary.tsv` under the evidence dir (default `.temp/cli-mutants/`); the source tree is restored on exit |
| `.scripts/fragment-mutants.sh` | narrowing-mutant harness for the §4.1 gates (argv, exit mapping, stderr transport, reader rules, schema closure, CCJ-1 emission, entry-point wiring); the behavioral suite runs against every mutant | `.scripts/fragment-mutants.sh [evidence-dir]` (optional `FRAGMENT_MUTANT_IDS="M27 M28" FRAGMENT_TEST_PATTERN="TestExecutableUnicodePathBoundary|TestConformanceCorpus"` for focused rework) | `mutants.log` under the evidence dir (default `.temp/fragment-mutants/`); sources are restored from byte copies on exit |
| `.scripts/mapping-mutants.py` | §4.2 narrowing probes with named production-entry failures; restores candidate bytes after each mutation | `python3 .scripts/mapping-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv` (default `.temp/mapping-mutants/`) |
| `.scripts/composition-mutants.py` | §4.5 behavioral narrowing probes for environment ownership, collisions, stdin and launch-boundary file refusal; restores candidate bytes after every mutant | `python3 .scripts/composition-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv` (default `.temp/composition-mutants/`); permission probe requires a non-root host |
| `.scripts/systemprompt-mutants.py` | SPEC §5 narrowing probes through exported production APIs; exact source bytes restored after every mutation | `python3 .scripts/systemprompt-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv` (default `.temp/systemprompt-mutants/`) |
| `.scripts/execution-mutants.py` | native execution and terminal-ownership narrowing probes, using disposable fake subprocesses in a scratch copy | `python3 .scripts/execution-mutants.py [evidence-dir]` | per-mutant logs and `summary.tsv`, default `.temp/execution-mutants/`; permission tests require non-root |
| `.scripts/diagnostics-mutants.sh` | narrowing mutants for the §6 gates (exit mapping, closed membership, no-invention classification, anchored code-line parsing, entry exits, owner/form registry, exact-Detail framing exemption); restores candidate bytes after every mutant | `.scripts/diagnostics-mutants.sh [evidence-dir]` (optional `MUTANT_RUN` for bounded batches) | per-mutant logs and `summary.tsv` (default `.temp/diagnostics-mutants/`) |
| `.scripts/plan-mutants.py` | SPEC §4.4 behavioral narrowing probes at `plan.Build`, restoring exact candidate bytes | `python3 .scripts/plan-mutants.py [evidence-dir]`; narrow validation: `go test ./internal/plan -count=1 -race`, `go vet ./internal/plan`, `go build ./internal/plan` | per-mutant logs and `summary.tsv`, default `.temp/plan-mutants/` |
| execution process helpers | exact argv/env/stdin/exit and late-check tests; compiles disposable fake provider and API signal driver; Python 3 supplies isolated POSIX PTY regression | `go test ./internal/execution -count=1` | test stdout; helper binaries use temporary directories and are removed |
| v4 Muse/Codex witnesses | strict reader policy, native Muse argv and environment, typed Codex refusals, Exec/DryRun module boundary and fake-manager routing/digest fences | `go test ./internal/fragment ./cmd/curator-run -run 'TestV4|TestMuseV4|TestCodexV4' -count=1` | stdout; bounded child fixtures under Go test temporary directories; source-pinned v4 schema in `internal/fragment/testdata/v4` |
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
