# Same-user development-member non-chat contract

Document revision: **same-user-nonchat-contract/1**.
Status: **target amendment proposal; not implemented, qualified or activated**.
This revision versions the documentary contract, not an executable, CLI, wire
schema, refusal registry or capability. Source publication alone enables none
of them. The qualification section is a PLAN, with no executed witnesses.

The follow-up [executor protocol source revision 2](same-user-executor-wire.md)
concretizes the proposed entry, closed records and custody ordering with inert
fixtures/manual cases. It adds no implementation or positive compatibility tuple;
this contract's retained scope and separate qualification gates continue to apply.

## 1. Scope and source boundaries

This target admits bounded trusted development, independent source review and
separate recipient assessment under the **already admitted current user**.
It explicitly amends the interactive-only/no-headless-human boundary in
[SPEC §§1–3](../SPEC.md#1-scope-and-non-goals) and
[Decision 0021](https://github.com/relux-works/curator-spec/blob/8d4c1ad415d145509c80be9aace77c5ce9837a83/decisions/0021-sessions-enter-through-curator-run.md):
that decision needs its distinct amendment v3 for this scope. Its original
adoption and amendments v1/v2 are historical/current boundaries, not implicit
permission for this mode. Both owners must accept the matched amendments.
No existing native argument, non-TTY input, hosted option or permission default
selects it. A future admitted adapter belongs to the canonical Curator/launcher
route; this document introduces no third launcher, shell wrapper or bypass.

Same-user separation is **cooperative**. A profile lock, private directory,
input digest or fresh conversation is not protection against a hostile process
with the same UID. This scope admits no untrusted workplace, separate worker
account, broker lease, service credentials, privilege transition, new network
access or host/service authority. A required capability outside this scope
refuses; it never falls back here from another route.

[SPEC §10](../SPEC.md#10-agent-account-one-shot-exec-contract-target-amendment)
remains the protected dispatcher/worker-account/broker target, unimplemented
and unqualified under its own gates. This contract is neither its interim
implementation nor its qualification. Existing interactive/native/hosted
semantics and Curator-owned policy/authentication/home refusals still apply.

| Boundary | This same-user target | Protected SPEC §10 target retained |
| --- | --- | --- |
| Admission/privilege | Explicit bounded current-user assignment; no self-admission by the member | Dispatcher admission, generation/placement/class/grants; privileged user launcher and UID transition |
| Input | Frozen declared instructions/artifacts and exact execution tuple; bounded private intake below | Joint plan schema/digest publication, immutable FD3, R-FD3 ownership and path-specific bounds |
| Credentials/provider | Only an already admitted compatible current-user source/channel; no repair, copy or substitution | Broker peer/binding, leases/renewal, first-provider and subsequent provider qualification |
| Network/tools | Only the assignment's already admitted tool/network scope; this document grants no egress | R-NET-TOOL/provider proxy, workplace zero egress, R-EXEC and R-VERIFY restrictions |
| Identity/lifetime | Stable accountable member outside the child; one new native context per invocation | Dispatcher execution/account identities, user-manager fate/stop/retirement and standing-session gates |
| Result/acceptance | Correlated bounded evidence for the caller; acceptance is separate | Dispatcher/helper receipts, transfer/import/exact-candidate review and lifecycle publication |
| Activation | Separate source, implementation, native qualification and adoption gates (§8) | All §10 intake/FD/broker/network/provider/stop/activation gates remain necessary |

Core owns manager, fragment, managed/native-home provenance and compatibility;
the launcher owns admission consumption, execution/result and cleanup semantics;
the runtime owner owns native context/config/provider behavior. A caller or
member cannot substitute guesses for any owner's missing compatibility evidence.

## 2. Member, assignment and independent consumers

The controller is the existing admitted caller that owns submission, evidence
custody and delivery. The member is a stable accountable development identity in
that caller's records, **not an a2a account or child board registration**. Each
assignment has its own immutable input identity and each execution attempt has
a new invocation identity. These three identities MUST NOT be interchanged.
They grant no authority by themselves.

Every invocation MUST create a **NEW native context** with only the admitted
profile subset and that assignment's declared instructions/artifacts. It MUST
NOT import a parent/producer conversation, resume an earlier thread, seed from
transcripts, memory exports, prior native state or hidden continuity. This also
applies to retry and rework: findings may be frozen explicit task artifacts;
conversation state may not. The exact runtime adapter must supply verifiable
fresh-creation evidence, not a model's assertion that it forgot history. An
unsupported or unknown fresh-context path refuses before starting work.

| Consumer | Frozen input and responsibility | Result and independence |
| --- | --- | --- |
| Development | Declared task, source/base and write scope; only admitted tools/effects | Proposed artifacts/patch and observations, never self-acceptance |
| Independent source review | Exact candidate head/tree, complete diff and explicitly declared references, as inert data | Separate member from producer; verdict binds that candidate and input digest only |
| Recipient assessment | Separately frozen recipient artifact set, instructions and assessment criteria | New assignment/context/result; source-review output is not recipient evidence or an assessment by implication |

Source review may read an explicitly supplied producer artifact as data, never
its native history. An assessment does not silently inherit review inputs or
verdicts. Shared stable membership does not imply shared conversation, automatic
wake, persistent worker service, terminal ownership or lifecycle implementation.
The persistent threads used to author/review this document are not witnesses
of the fresh invocation property specified here.

## 3. Exact admission tuple and frozen intake

The following are **target record concepts**, not released member names or a
wire schema. Admission MUST bind and verify every applicable row. Unknown,
missing, stale, incompatible or conflicting evidence refuses; an explicit
owner-validated absence is required when a component is not used.

| Bound evidence | Required meaning |
| --- | --- |
| Controller/member/consumer | Current-user authority, accountable member, assignment and unique invocation identities; development/review/assessment role and distinct reviewer when required |
| Tool/source | Exact launcher and runtime binary identities/digests, source revisions and admitted adapter contract revisions; a mutable path or floating version alias is insufficient |
| Profile/config | Exact selected profile, source closure, config revision and lock/content digests; complete effective configuration/discovery closure, not the profile name alone |
| Runtime/model/effort | Actual resolved runtime/vendor/release/model ID and effort, with release-specific compatibility; never infer the effective choice from a default |
| Home/auth/manager | Admitted current UID, workspace and private per-invocation home provenance; declared current-user credential source/channel without material; exact manager identity/revision/capabilities when required, otherwise explicit compatible absence |
| Inputs | Immutable trusted instructions separately identified from inert source artifacts; per-object exact-byte SHA-256 and length, input envelope digest, exact source/candidate/base identity |
| Effects/budget | Permitted tools, read/write roots, network scope, output limits, startup deadline, maximum runtime, cancellation/cleanup deadlines and result custodian |

Core must determine a supported compatible home/config subset before execution.
An adapter that requires a manager cannot treat a missing/incompatible manager
as permission for unmanaged/native fallback. No source pin, manager capability,
authentication method or native-home provenance is changed by this revision.

### Intake ownership and proposed bounds

The controller freezes one closed UTF-8 JSON intake document and a flat manifest
of regular artifact files before admission. Hashes cover **exact bytes**, not
reformatted JSON; duplicate JSON members, unknown versions/members, malformed
encoding, incomplete reads or missing required concepts refuse. The expected
intake SHA-256 is bound in the controller's admission record separately from the
intake, not accepted solely from a self-supplied digest inside untrusted bytes.
Each manifest entry has a unique relative logical path, length, digest and role
(instruction or inert artifact). Absolute/escaping/ambiguous paths, symlinks,
special files, archive auto-extraction and undeclared extra inputs are excluded.
Instructions have explicit trusted-source provenance; source/candidate bytes
never become instructions by being named in the manifest.

The proposed target transport is one pre-staged, immutable, read-only seekable
intake handle plus bounded read-only artifact handles, retained by the controller
until receipt reconciliation. The launcher reads/verifies once before start;
no feeder may wait for a child that cannot yet start. Handle numbers, closed
serialization and the adapter entry invocation need source publication before
implementation; **no FD3/FD4, stdin, path, environment or hosted payload reuse is
implied**. A path/stdio/env fetch or a mutable checkout is not an alternate intake.
Any later input change needs new admission. Same-UID mutation resistance is not
claimed; cooperative custody and revalidation at consumption are mandatory.

Proposed **target-only ceilings** for this revision: intake 64 KiB; at most 128
artifact files, 4 MiB each and 16 MiB total; trusted instructions at most 64 KiB
within those totals. These are independent of §10's message bounds. Each
admission fixes finite positive startup/runtime/stop/result deadlines and may
narrow these ceilings; unset/unbounded deadlines refuse. Every read/stream is
capped before allocation/consumption. Out-of-bound intake refuses before start,
without a partially accepted assignment or truncating existing outputs. These
values do not assert a shipped limit or override an existing smaller bound.

## 4. Effective configuration and no presence

The initial **target admissible profile subset** contains only declared static
instructions/settings, inert task artifacts and explicitly allowed native tools.
It excludes executable startup hooks, messaging/board tools, arbitrary MCP
servers, ambient plugins/skills and any instruction that joins chat, registers a
worker, forwards messages or binds a parent identity. A declarative profile that
contains an excluded member is refused, not silently stripped or rewritten.
A future broader subset requires its own revision and real compatibility proof.

Admission and the runtime adapter MUST enumerate the actual effective closure:
CLI/options, environment, user/system/organization policy, managed and native
home surfaces, workspace and nested instruction/config discovery, executable
resolution, settings/helpers, hooks, plugins, skills, MCP, auto-loaded memory,
resume stores and inherited handles/sockets. Policy that cannot be enumerated,
excluded or pinned with owner-supported semantics is **unknown**, hence refuses.
Environment scrubbing alone is insufficient. Candidate control files remain
inert outside all discovery roots; entering a candidate tree requires a
qualified strict-context adapter, otherwise it refuses. Candidate bytes may
be inspected as data, never auto-adopted at startup, reload, nested read or
child entry; native resume is not admitted by this contract.

The child MUST have no a2a identity, chat registration, parent messaging
credential, hook/binding or board presence. The member identifier remains
controller-side metadata, never a borrowed `A2A_AGENT` or messaging account.
The child receives no parent messaging sockets or board-control capability and
cannot inherit one through config discovery. Submission/result delivery are
controller operations through its already admitted route, not child chat.
A workspace's board files confer no child registration or authority.

Only declared task-owned scratch may be materialized. Native/managed homes,
existing credentials, user/source configuration, hooks, consumers, identities
and processes MUST be preserved. Authentication may use only an existing
source/channel already approved by its owners for the exact tuple. Needing
credential copies, reauthentication, login repair, runtime-config rewriting or
policy relaxation is a refusal/gap, not permission to do it. No service/broker
credential is admitted. If freshness, discovery exclusion or credential/home
preservation cannot coexist on a platform, record the constraint and refuse
that tuple until its source owners resolve it; do not invent a mock-only lane.

## 5. Execution and observable outcomes

The launch adapter validates admission, input bytes, tuple and effective closure
before any harness start. A bounded admission/refusal record is allowed in
controller-owned scratch; no refusal writes live homes, repairs configuration,
registers presence, executes task tools or starts a weaker child. A start marker
is durable before the start attempt. Only one attempt consumes an invocation
identity. Observing a created process is not readiness or accepted work.

A retained launcher-side supervisor, not an exec-replaced process with an
imaginary post-exit emitter, owns observation and the result channel. The
controller owns durable custody/reconciliation. The exact implementation must
publish who survives controller/supervisor failure before claiming such
survival; process detachment alone is not that proof.

These are **documentary outcome distinctions**, not new wire enums or numeric
launcher exit codes. A result keeps the independent axes separate:

| Axis | Required distinctions and effects |
| --- | --- |
| Admission | Not attempted; validated; refused before start. A refusal is not a harness failure. |
| Start | No start; attempted with known spawn/exec failure; started; start evidence unknown. Preserve the actual OS/native error without inventing a harness exit. |
| Harness | Observed exit with its actual native code; observed signal with native signal evidence; running; interrupted/cancelled with observed outcome; unknown. Do not invent refusal numbers or `128+signal` mappings. |
| Result | Complete validated envelope/artifacts; missing; partial/truncated; malformed; mismatched; conflicting. Exit zero does not turn missing/partial output into a complete result. |
| Cleanup | Confirmed owned-scope absence and scratch disposition; pending; failed; unknown. Native exit, timeout or process disappearance alone proves none of these. |
| Acceptance | Separate caller/source-owner decision bound to the exact input/candidate/result. No start/argv/fragment/build receipt, CI pass, result completeness or exit zero is task acceptance. |

Cancellation is a request, not an immediate terminal success. Before a known
start it closes admission with no child; after or during an ambiguous start it
reconciles the same invocation and owns only that execution's bounded stop.
Observe and preserve the real native outcome even when a cancellation was
requested. Missing/contradictory evidence remains unknown; never retry or report
clean stop solely because a deadline elapsed or a PID disappeared.

Stop uses only the admitted task's verified process-incarnation/owned child
scope and its published escalation/deadline policy. No pattern kill, account-wide
signal, shared service stop or action on unrelated consumers is allowed. A process
group alone is not proof that descendants could not escape. Supported adapters
must establish their complete cleanup scope with real witnesses; if scope or
absence cannot be established, preserve the marker/evidence, report cleanup
unknown and block automatic replacement/reuse. Never import §10's cross-account
stop/retirement authority into the current user's process space. Delete only
owned scratch after input/result custody and confirmed absence; interrupted or
failed cleanup retains evidence and a caller-owned reconciliation obligation.

## 6. Result transport, duplicate handling and acceptance

The controller pre-creates a private bounded result channel. The retained
supervisor emits one closed result envelope from its observations; worker text
and proposed artifacts are untrusted task output within it, not supervisor
truth or acceptance authority. Stdout/stderr are separately bounded diagnostic
streams and never substitute for that envelope. Proposed target ceilings:
envelope 64 KiB, up to 128 regular result files (4 MiB each, 16 MiB total),
stdout and stderr 1 MiB each. Overflow/truncation is explicit and cannot silently
produce an accepted complete result. A diagnostic may be truncated under its
cap with that fact recorded; truncated result data is incomplete.

The envelope binds the admitted assignment/invocation/member/consumer, exact
input digest and per-object identities, actual tool/profile/config/lock/runtime/
model/effort/home/manager tuple, actual invocation, observed outcome and cleanup
evidence, and output lengths/digests. Unavailable actual fields stay explicit
unknowns; they are not filled from intended argv/defaults. No secret values or
credential material enter records/logs. Actual invocation evidence retains the
admitted non-secret argv and input identities; credentials are never admissible
argv/task payload. This is not a general secret-free-content classifier.
Exact native status is retained even
where an adapter cannot normalize it. Output artifacts obey the admitted write
scope and path/kind/cap restrictions; no links/special files or partial import.

The controller validates framing/completeness, all bindings and digests before
exposing a result to its named consumer. Staging and atomic completion preserve
the prior record on failure. Malformed, missing, mismatched or conflicting
results are retained as bounded diagnostic evidence, not accepted or imported.
Read failure is not absence. An envelope's self-declared identity/digest does
not establish trusted origin or a clean stop; custody and observed evidence do.

| Repeated/conflicting event | Target behavior and post-failure state |
| --- | --- |
| Same invocation, identical intake | Return recorded phase/outcome; never start a second harness. If start is ambiguous, hold and reconcile; do not silently repeat the start. |
| Same invocation, changed input/tuple/consumer | Refuse conflict, preserve original admission/start/result; no second process or overwrite. |
| Byte-identical complete result replay | Idempotent retrieval only, no duplicate artifact import or acceptance effect. |
| Different result for the same invocation | Preserve the original and bounded conflict evidence; block new consumption/automatic acceptance and require reconciliation. Never silently replace a result already shown. |
| Retry after failure/unknown | Explicit new admission and invocation identity; retain predecessor correlation. No retry until previous owned scope is confirmed absent; a new identity does not erase an unknown start. |

The controller consumes at most one explicitly selected result per assignment
for a task effect. Replays and later attempts cannot duplicate that effect;
subsequent rework requires a new assignment/admission. A complete nonzero or
signal outcome may still carry useful artifacts, but no automatic acceptance.
Independent source-review and recipient-assessment verdicts remain different
results, even if their input artifacts happen to have equal bytes.

## 7. Qualification PLAN — not executed

Fixture passes are necessary instrument checks only. Every admitted real
runtime/tool/profile/config/model/effort/home/manager tuple needs separate
**real native** witnesses for freshness, effective discovery, actual outcome
and complete cleanup. No universal model/runtime/provider parity follows from
one row; unsupported rows remain refused. An argv/log/build/exit receipt or a
persistent authoring/review thread is not a native qualification witness.

Each future row records exact tuple and immutable inputs, commands/invocation,
positive and negative native outcomes, observed state/effects, finite deadlines,
owned cleanup scope, output/result digests and unresolved gaps. Fixtures run
only in isolated owned scratch; no negative plant alters live state. Preserve
failed/unknown evidence rather than count any error as an intended refusal.
No provider/native execution is authorized by this PLAN.

| Row | Nearby valid positive | Targeted negative and required state/effect assertion |
| --- | --- | --- |
| Q01 Tuple | Exact owner-qualified tuple, explicit manager use/absence | Change each required source/tool/profile/config/lock/runtime/model/effort/home/manager binding or compatibility proof: admission refuses before child/task effects; prior homes/records unchanged. |
| Q02 Intake | Complete frozen bytes and allowed regular artifacts at the declared cap | Wrong digest/length, stale base, duplicate/unknown member, truncated document, extra file, escaping path/symlink or over-cap input: exact intake rejection, no start or partial acceptance. |
| Q03 Fresh context | Two independent native creations consume only their respective declared inputs | Resume/parent/producer IDs, transcript/memory seed, implicit native history or unknown creation evidence: refuse before work; inspect native creation/input provenance, not model self-report. A prior-context sentinel must not enter the second invocation's input. |
| Q04 Config/discovery | Declared static subset, candidate controls inert outside discovery | Undeclared user/system/workspace/nested config, hook, skill, plugin, MCP or discovery root; attempt startup/reload/nested/child adoption: refuse/exclude before effect as specified in §4. Inert callback markers remain absent; control files and effective config are unchanged. |
| Q05 No presence | New native child with controller-only membership and result delivery | Seed a2a identity/config/credentials, parent messaging hook/socket/binding, chat/board registration or board control: refuse contamination; no registration/message/callback or borrowed identity, existing presence unchanged. |
| Q06 Consumer | Independent reviewer and separate frozen recipient assessment each bind their own declared inputs | Producer-as-independent-reviewer, reused assessment invocation, wrong consumer/candidate or producer-history seed: refuse admission/result consumption; no substituted verdict or task acceptance. |
| Q07 Start replay | Identical invocation replay retrieves one recorded start/outcome | Concurrent duplicate, changed input under same identity, or retry while prior start is unknown: at most one harness; preserve original and conflict/unknown evidence; no automatic successor. |
| Q08 Result binding | Complete envelope matches actual tuple, immutable input and outputs | Wrong invocation/member/consumer/input/actual tuple or artifact digest; self-supplied origin claim: reject consumption, no imports/acceptance, preserve prior valid record. |
| Q09 Result replay | Identical complete replay has zero further effects | Different envelope/artifacts for the same invocation, including after consumption: retain conflict and prior record; freeze automatic consumption, no second import/acceptance. |
| Q10 Native outcome | Separately witness known start, actual zero/nonzero exit and signal | Preflight refusal, actual spawn/exec failure and lost observation must remain distinct; never synthesize a native exit, clean stop or success from a wrapper exit/argv receipt. Unrelated state unchanged. |
| Q11 Missing result | Atomic complete result survives ordinary retrieval | Kill/fail writer or interrupt framing/staging: missing/partial/malformed stays unaccepted even with native exit zero; no partial overwrite/import. Bound retained diagnostics. |
| Q12 Cancel/cleanup | Cancellation of a verified owned scope confirms absence and preserves result custody | Cancel during uncertain start, controller/supervisor failure, PID reuse, surviving/escaped descendant or interrupted deletion: stop only proven-owned scope, report unknown/pending where necessary, retain markers/input/results; no replacement or unrelated kill. |
| Q13 Output bounds | Valid files and bounded diagnostic truncation recorded honestly | Oversized/escaping/link/special-file result or incomplete result stream: reject whole result, no partial task effect; capped diagnostics cannot imply complete task output. |
| Q14 Preservation | Isolated invocation leaves all unrelated homes/config/hooks/identities/consumers intact | Adapter needs credential copy/repair, login, config rewrite or unknown policy exclusion: refuse; custodian-approved pre/post preservation witnesses must show no forbidden effects without exposing secrets. |

Native rows must exercise actual discovery surfaces, not just an env-only
fixture. Where the platform cannot make configuration/freshness/cleanup evidence
observable, retain that row as an unsupported gap with the responsible source
owner; do not replace it with a stub or infer success from process exit. Before
implementation, each row needs a concrete owner-approved fixture/native recipe,
limits and allowed effects. Document review/counterexample checks are not those
runs and cannot mark any Q-row passed.

## 8. Publication, implementation and adoption gates

1. **Paired source acceptance/publication:** independently review the exact
   launcher contract revision and Decision0021 amendment v3 candidates together.
   Each source owner retains its own default-branch custody; a feature PR is a
   proposal, not acceptance. Preserve v1/v2 and all §10 gates.
2. **Implementation admission:** separately authorize a bounded adapter/API and
   publish its closed schemas, entry/handle mechanism, compatibility records,
   actual refusal names/exit treatment and supervisor/result/cleanup ownership.
   Do not infer these from this documentary revision or reuse interactive codes.
3. **Fixture and native qualification admission:** commission bounded runs and
   their tool/provider/credential/effect budgets separately. Record fixture and
   real native results distinctly for each exact tuple; no documentary CI or
   existing recorded run substitutes for either.
4. **Host adoption/activation:** separate owner admission of supported qualified
   tuples and consumer window. No install, repair, auth or shared-service change
   follows automatically from source acceptance or qualification evidence.
5. **Independent task acceptance:** consumer/source-owner verdict on the exact
   produced artifacts, with current checks/review and any separate landing
   obligations. Native exit, clean stop and task acceptance remain separate.

Open publication work is the concrete wire/entry/handle and effective-config
adapter, supported per-runtime compatibility, result custody across failures
and qualified process-scope cleanup. None is filled by existing hosted snapshot
amendment v2, §10 FD3/receipt names, provider flags or a new ad hoc executor.
