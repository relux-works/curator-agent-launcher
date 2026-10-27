# Same-user executor protocol, source revision 1

Status: **Stage1 source candidate; no implementation, supported native tuple,
qualification or usable executor**. This publication makes new launcher-owned
protocol choices; none is inferred from an existing native flag or from the
proposed values in [contract/1](same-user-nonchat-contract.md). It concretizes
that contract without relaxing its same-user, freshness, no-presence or SPEC
§10 boundaries. The executable/spec release and dependencies are unchanged.
The [closed record schema](same-user-v1/records.schema.json) and
[inert cases](same-user-v1/cases.md) are part of this source contract.

## 1. Entry and descriptor custody

The reserved canonical binary entry is:

```text
curator-run member --request-fd N --authorization-fd A --objects-fd O --state-fd S --response-fd R --controller-fd C
```

This is a proposed subcommand of the existing launcher, not a helper binary,
shell wrapper, service, `exec` alias, interactive native tail, hosted launch or
SPEC §10 entry. Core has accepted this spelling for source review; Curator-side routing
still requires its source owner's paired publication; the existing `curator run` parser does not acquire it by this text.
Until then no current command is an operational spelling of this entry.
The initial transport is local POSIX descriptors only; other platforms require
a new owner-published carrier, not a path/stdio/environment fallback.

All six flags occur exactly once, in any order; decimal integers have no sign,
leading zero, whitespace or exponent and are in 3..255. They are pairwise
distinct, including underlying file identities where applicable. There are no
operands, other options, optional defaults, native arguments or `--` tail.
Standard input is not read. Standard output is empty. Stderr contains only a
bounded static protocol error tag (at most 256 ASCII bytes), never input bytes,
paths, environment, credentials or upstream free-form messages. Dynamic N/A/O/S/R/C
are caller allocations, not fixed FD3/FD4 meanings imported from SPEC §10.

| Descriptor | Controller prepares | Launcher validation and lifetime |
| --- | --- | --- |
| N | Read-only regular file containing one frozen request, at most 64 KiB | Require regular, seekable, read-only, current UID, mode 0600, one link, no write mapping/producer; read with offset-independent bounded reads from byte 0 through exact size. Retain raw bytes. |
| A | Independently frozen read-only authorization record, at most 4 KiB | Same file checks. It binds N's exact length/digest and store/controller, outside untrusted artifact bytes. No self-hash inside N substitutes for A. |
| O | Read-only current-user directory, mode 0700, with flat content-addressed regular files | Open each declared SHA-256 basename relative to O with no symlink traversal, require mode 0400/0600 read-only handle, current UID, single link and exact length/hash. Keep verified handles until their admitted use completes. |
| S | Open current-user private state directory, mode 0700, on a local filesystem with supported lock/atomic-publication durability semantics | Verify immutable `owner.json` against A, UID and actual directory identity. No path lookup, `$HOME`, default discovery, existing-home repair or store auto-selection. |
| R | Fresh write-only seekable regular file, current UID, mode 0600, one link, size 0 | One response, at most 16 KiB, starting at byte 0; no truncation of pre-existing data. Write only after its authoritative state/result publication, then sync and close. |
| C | Read end of a private anonymous pipe | Controller retains the only write end without writing data. EOF or any received byte means lost/invalid controller custody, triggering bounded cancellation after admission. It is not an instruction or result channel. |

Descriptor validation precedes file contents, authorization, store mutation and
any child. An invalid R means no response can be promised: return transport
failure with only the static stderr tag. No descriptor or inherited listener,
a2a socket, board capability or state/result channel is passed to the harness.
The native adapter gets only its explicitly published input/output/auth handles;
every inherited descriptor not on that adapter's exact allowlist is closed.
Descriptor numbers never establish origin or authority by themselves.

O contains exactly the unique declared object digests: no undeclared files,
subdirectories, symlinks, special files or archives. Equal digest/length objects
may be referenced by more than one name/role, but names are unique. All reads
recheck size/identity/hash at consumption; any mutation refuses, not a retry.
This is cooperative same-UID custody, not hostile same-UID write protection.
The controller freezes these files and retains them until durable reconciliation;
a writer continuing after admission violates the contract. No candidate tree is
entered or materialized into a discovery root.

The wrapper's proposed process exit is **0 only when a complete protocol response
was synced to R**, including a typed refusal; **1 when delivery is incomplete or
unavailable**. These are newly proposed transport outcomes, never native harness
codes, task success or acceptance. Observed wrapper signals remain actual wrapper
signals. A consumer must validate the response AND authoritative stored record;
exit 0, a parseable prefix or EOF alone is insufficient.

## 2. Bytes, schema and limits

The accepted JSON subset is [RFC 8259](https://datatracker.ietf.org/doc/html/rfc8259)
UTF-8 without BOM, invalid encodings, unpaired surrogates or embedded NUL.
Exactly one root object, with only JSON whitespace before/after it, is permitted.
Object member names are unique **after escape decoding at every depth**. Reject
unknown kinds/members, comments, concatenated values and incomplete reads.
Numbers are lexical `0` or `[1-9][0-9]*`, up to 9007199254740991; no negative,
fractional, exponent, NaN or Infinity forms. Native OS observations that cannot
fit the explicitly published POSIX observation types stay unknown/unsupported,
not normalized into invented values. Maximum nesting is 16, maximum total value
nodes 8192, maximum decoded string 4096 UTF-8 bytes. Schema limits may be smaller.

[JSON Schema Draft 2020-12](https://json-schema.org/draft/2020-12/json-schema-core)
with its [validation vocabulary](https://json-schema.org/draft/2020-12/json-schema-validation)
is the structural reference. Every object is closed; all listed fields are
required, including explicit nulls. Local `$defs` only, no network `$ref` lookup,
default insertion or `format`-annotation dependence. A schema pass does not
check duplicate keys, lexical integers, UTF-8 byte caps, hashes, custody,
cross-record equality, state transitions or runtime compatibility. Those rules
are separately normative below. Kind strings allocated here are proposed
launcher-owned record discriminators, not signed domains or shipped capabilities.

Digests are 64 lowercase hex characters, SHA-256 of the exact raw file bytes,
including whitespace/newline if present. No canonicalization, reconstructed
JSON, encoded digest self-field or mutable source path is hashed instead.
A `ref` is `{name, length, sha256}`; names are flat ASCII identifiers 1..64
characters and never filesystem paths. File lookup uses the digest only.
A source locator binds repository URL, exact Git revision and document digest;
it is provenance data, never permission to fetch or execute it.

| Value | Hard ceiling in this proposed protocol |
| --- | ---: |
| Request / tuple / result / owner input individually | 64 KiB |
| Authorization / state owner | 4 KiB |
| Response / snapshot / consumption record individually | 16 KiB |
| Objects, including tuple and owner evidence | 128 unique names, 4 MiB each, 16 MiB total unique bytes |
| Trusted instruction objects combined | 64 KiB, included in object totals |
| Result files | 128, 4 MiB each, 16 MiB combined |
| Harness stdout and stderr | 1 MiB each, with explicit truncation |
| Persisted phase snapshots per invocation | 256; last 4 slots reserved for stop/final/uncertain evidence |
| Total retained invocation state, input, outputs and diagnostics | 64 MiB, reserved before start |
| Startup / runtime | 60000 ms / 3600000 ms |
| TERM grace / KILL-observation / result delivery | 6000 ms each |
| State lock acquisition / cancellation polling interval | 1000 ms / 100 ms |

Every admission supplies positive deadlines within these bounds; no absent or
unbounded defaults. The admission's wall-clock expiry is checked before effects.
The startup deadline begins at durable start-intent and includes gated spawn;
runtime begins at observed exec acknowledgment. TERM and KILL-observation budgets
begin at their respective durable stopping barriers; result delivery begins at
quiescence or sealing an uncertain outcome. Each snapshot persists the current
absolute monotonic deadline before the next effect; a recovered phase keeps its
original deadline. Switching phase is not permission to renew an expired budget.
After start, elapsed deadlines use a monotonic clock and recorded boot identity;
a clock reset/reboot never renews a budget. Unsupported filesystem, time,
locking, descriptor, scope or observation semantics refuse before child release.
Capacity exhaustion preserves prior bytes and refuses new starts; no eviction
or evidence trimming to make a request fit. Emergency evidence slots do not
promise storage availability after filesystem failure.

## 3. Authorization, exact tuple and effective closure

The controller already has current-user task authority. It independently issues
`authorization` for one request digest/length, controller/store and expiry.
This is a nonportable controller admission act conveyed through private handle
custody, **not** a cryptographic grant, signing domain or a new authority source.
The immutable store owner record identifies controller/store/UID. A matching
label/digest alone proves neither authorship nor authority; the controller's
existing admission and descriptor/store custody are prerequisites. No signing
key, bearer or parent a2a identity is copied into the child.

`submit` binds identity, tuple reference, exact artifact manifest, effects and
deadlines. Its `tuple` object must be included in the manifest with role `tuple`.
Non-secret static profile/config/lock/fragment/effective-closure inputs and
owner publications are immutable objects with declared roles. Private home/origin
and authentication material are NOT manifest objects: the tuple carries opaque
verification-input digests, not their paths, records, credentials or authority. The tuple has **no supported
entry by default**. Structural example acceptance is not runtime admission.

| Tuple member | Exact input and consuming owner |
| --- | --- |
| launcher / curator | Source repository/revision, binary digest and release; launcher verifies actual binaries, not floating aliases or PATH guesses. |
| agents_management | Exact module version/revision; existing single construction-site owner must publish a compatible non-chat composition mode. Interactive mode is not converted by the receiver. |
| runtime | Runtime/vendor, exact release and binary digest, resolved model, effort or owner-attested not-applicable evidence, OS/architecture and adapter identity/revision. |
| profile | Profile ID/revision, source closure/config/lock digests, fragment kind/exact-byte digest, effective-config and permission-posture digests. No profile-name-only or reconstructed-fragment comparison. |
| home | Current UID, managed-snapshot/native-strict choice, opaque local verification-input digests for managed/native-origin provenance and credential-channel binding; no private origin record, path or credential material. |
| manager | Either `required` with exact registration, transport, capabilities and compatibility digests, or `compatible_absence` with positive owner compatibility evidence. Unavailable is neither absence nor fallback. |
| owner_inputs | Exact immutable Curator/runtime/launcher publications and evidence objects, matched to the tuple and controller-approved compatibility catalog. A caller-supplied `supported` flag cannot add a catalog entry. |

The `catalog` is an immutable controller-owned store record, bound by the exact
`catalog_sha256` in `owner.json`. It is prepared through the controller's existing
trusted source-admission procedure, never derived from N, A, O or worker output.
Its entries match **exact tuple bytes**, not wildcard/predicate/"supported" flags.
Each binds the tuple reference and exact Curator/runtime/launcher owner inputs,
plus source-contract digests for construction, private verification and complete
scope semantics. The referenced publications must actually describe the same
binaries/platform/config/home/manager tuple. Duplicate tuple digests, unknown
owners, contradictions or a matching digest without that publication refuse.
A missing entry is `compatibility_unpublished`; a contradicted approved entry is
`compatibility_mismatch`. Catalog policy is necessary, never sufficient for
actual state verification, qualification or authority.

This publication supplies **zero positive native entries** (`catalog-empty.json`).
Changing a catalog requires a separately admitted new store; it cannot upgrade
an existing invocation or its retained evidence. Source digests name publications,
not a new shipped Curator compatibility receipt/token. Core/runtime ownership,
independent source review, native qualification and adoption remain separate.
A caller-supplied owner document cannot add an entry.

Admission checks the full closure: system/organization/user policy, managed and
native homes, workspace/nested discovery at startup/reload/tool/child entry,
settings/helpers/hooks/MCP/plugins/skills/memory/resume stores, executable lookup,
environment and inherited handles. `effective_config` is a pinned Curator/runtime
owner output, not a launcher allowlist guessed from environment. Only published
static instructions/settings plus explicitly admitted tools may remain. Excluded
content refuses, never silently strips. Unknown policy/discovery refuses before
native work. Candidate controls stay inert as content-addressed data outside
all discovery roots. An adapter unable to inspect data without auto-loading its
controls is unsupported. Fresh creation must exclude parent/producer/resume IDs,
history/transcript/memory seeds; no continuity field exists in the closed intake.

Effects name allowed native tools, declared read objects/output names and an
already admitted provider-channel binding or disabled network. They grant no
new egress, broker/service credential, privilege or cross-account authority.
Actual environment/argv are produced at the existing approved construction site
from this exact tuple; arbitrary caller argv/environment overlays are absent.
The launcher compares the composed plan with the admitted closure before release.
No `--repair`, live resolution, auth copy/login, config rewrite, profile fallback
or weakening policy is a preflight step of this entry.

## 4. State store and publication barriers

S is controller-owned, distinct from candidate inputs and native work/output
roots. Only the entry's trusted supervisor/reconciler writes observations there;
worker stdout/artifacts cannot supply a snapshot or result envelope. The harness
is not passed store paths/handles. Cooperative same-UID limits still apply.

Fixed layout, names derived only from validated hexadecimal IDs:

```text
owner.json
catalog.json
store.lock
invocations/<invocation_id>/owner.lock
invocations/<invocation_id>/edit.lock
invocations/<invocation_id>/request.json
invocations/<invocation_id>/authorization.json
invocations/<invocation_id>/objects/<sha256>
invocations/<invocation_id>/records/<generation>.json
invocations/<invocation_id>/head.json
invocations/<invocation_id>/cancel.json
invocations/<invocation_id>/conflicts/<conflicting_sha256>.json
invocations/<invocation_id>/result.json
invocations/<invocation_id>/outputs/<sha256>
invocations/<invocation_id>/diagnostics/stdout
invocations/<invocation_id>/diagnostics/stderr
assignments/<assignment_id>/reservation.json
assignments/<assignment_id>/ack.json
```

`owner.lock`, `edit.lock` and `store.lock` are fixed inodes, never unlinked or
replaced while the store exists. Acquire the store lock for identity-directory
creation and assignment reservation, and the edit lock for snapshot/cancel
publication. Lock order is store then edit; never acquire store while holding
edit. The submitting process holds the invocation owner lock throughout its
supervisor lifetime; it does not hold edit while waiting for the harness.
Inspect/cancel can therefore proceed. Lock failure/timeouts are `store_busy`,
not evidence that a supervisor died. A lock is concurrency custody, not a native
process-incarnation proof or a hostile-UID security boundary.

Create an invocation directory exclusively and sync its parent **before** an
admission record or start. Any existing directory, including partial/corrupt
ones, is not fresh. An identical request retrieves state; changed bytes conflict.
A failed partial creation never authorizes another submit with that identity.
`store.lock` also serializes controller bootstrap of the fixed owner/edit lock
inodes before other operations can enter that directory. Those lock files cannot
be absent/replaced after initialization. A partial bootstrap is quarantined as
unreadable, never completed by guessing it was an unconsumed identity.
Copy N/A and every admitted input object into bounded private retained custody,
verify bytes again and sync files/directories before admitting start.

For each immutable publication: exclusively create a mode0600 temporary file in
the same directory, write bounded complete bytes, sync the file, publish without
replacing a differing object, then sync the containing directory. Existing
identical bytes may be reused; differing bytes are conflict evidence. A new
snapshot is an immutable generation file. Only then replace `head.json` through
a same-directory synced temporary file, atomic rename and directory sync.
The pointer binds generation, exact snapshot length/digest. Old generations
remain intact. No cross-filesystem rename or stale-head recovery is assumed. Trusted observations
and scope proofs use the same immutable object publication under retained
`objects/<sha256>` before a result references them; private origin records and
auth material are forbidden there. Diagnostics are retained before their digest
is published. A conflict uses the closed `conflict` record in `conflicts/` before
any conflicting response; category/original/conflicting hashes must differ and
bind retained authoritative bytes versus the bounded conflicting digest. At most
32 conflicts per invocation are retained; further conflicts refuse as store_full.
Failure to persist conflict leaves uncertain custody and freezes consumption,
never overwrites the original or silently permits an effect.

No task effect crosses a barrier before its preceding durable publication is
acknowledged. Failure during file/directory sync is not success. On recovery,
missing/truncated/conflicting head, any unexplained intent/temp or unreadable
record is uncertain evidence, never absence. Retain bounded evidence and block
new work; do not repair history, select a convenient older generation or delete
an invocation to make its identity fresh. A result file is published and synced
before any snapshot/response refers to it. No partial outputs are imported.

## 5. Start, supervision and crash ordering

The submitting `curator-run member` process **remains the foreground supervisor**;
it never exec-replaces itself or double-forks into a service. It owns native
observation, bounded streams, store custody and final delivery. Controller
submission and result acceptance are separate roles. Worker text is untrusted
output; it cannot label its own origin/native exit/cleanup as observed.

| Transition | Owner / required durable barrier | Allowed next effects and crash disposition |
| --- | --- | --- |
| absent → admitted | Submitter under exclusive directory creation; persist raw request/authorization/input bytes and snapshot | No native child; allocate only admitted private scratch and an empty recoverable scope. Partial admission is held, not retried. |
| admitted → start_intent | Supervisor owns a complete recoverable scope identity and current incarnation; snapshot records intended single attempt, scope and deadline/boot evidence | Only after sync may it perform the single gated spawn. A crash here consumes the identity even if no native process actually started. |
| start_intent → spawned | Supervisor records actual child incarnation within the scope while the child is held before native exec | Persist identity/scope membership before releasing the gate. EOF/failure of that private gate aborts; it cannot auto-exec. No task code before containment. |
| spawned → running | Release only after revalidating tuple/inputs/discovery and durable spawn evidence; observe actual native exec success separately | Exec acknowledgment is not initialization/readiness/task acceptance. Crash across release is start/native unknown unless a durable observation proves more. |
| admitted/start_intent/spawned/running → stopping | Controller cancel record, controller-pipe loss, deadline or supervisor signal; durable intent before each bounded stop action when storage works | Preserve actual observed native outcome. TERM then KILL only under admitted scope policy and deadlines. No global/pattern signals. |
| observed exit/spawn failure → terminal | Supervisor persists actual native evidence, outputs/status and complete-scope absence proof; publishes result then terminal snapshot | No child replacement. Native exit with remaining descendants goes through stopping, not terminal-clean by implication. |
| any nonterminal → uncertain | Missing observation, owner loss, bad store, scope identity drift, failed/unknown cleanup or publication | Retain original evidence; no automatic retry/replacement/import, no invented exit or clean stop. |

The single attempt is the durable start intent. Concurrent identical submits
never allocate another scope or spawn another harness. Native exec failure
records actual OS operation/error evidence as `spawn_failed`, not native exit.
A release acknowledgment and observed actual tuple are separate evidence;
unavailable actual model/effort/config stays null/unknown, not copied from the
requested tuple. A tuple whose adapter cannot enforce its exact selection before
work is not admitted by the catalog.

### Survival and loss

No controller/supervisor/host-crash survival is assumed. If only the controller
loses C, a still-running supervisor cancels within the admitted finite budgets,
retains custody/result in S and attempts no further delivery to an abandoned R.
The supervisor may itself be killed by the OS, terminal group, power loss or
storage failure; no detachment or presence of `owner.lock` promises otherwise.

A `reconcile` process may acquire owner.lock only after its prior holder is
actually gone; lock-read failure is not permission. It NEVER starts a harness.
With a valid pre-intent record it may attest no start attempt; with any intent
or unexplained partial record it treats start as consumed/uncertain. It reopens
only the exact persisted recoverable scope through the approved scope adapter,
compares boot/namespace/object/incarnation identity and observes/stops that scope.
PID reuse, a missing namespace or unmatched scope is `scope_unknown`, never a
reason to signal a numeric PID. A new reconciler's incarnation is recorded.

A reconciler may reuse a complete durable supervisor native observation by its
exact digest, explicitly marking recovered origin. It cannot manufacture a
wait status for a child it never observed. Without such evidence, native outcome
remains unknown even if complete scope absence can later be proven. Existing
result bytes are returned unchanged. Different result bytes never replace them;
a new observation is an appended snapshot/conflict, not a rewritten result.
Controller loss plus failed supervision leaves an explicit caller-owned orphan
reconciliation obligation; this protocol promises no autonomous survivor.

## 6. Complete process scope and cleanup

A supported scope adapter is linked into the existing launcher; it is not a
caller executable, shell command, PID list or third wrapper. Before start it
must provide these exact primitive semantics to the supervisor:

1. Allocate an empty private scope with stable recoverable kernel-object identity,
   boot/namespace identity and no unrelated members.
2. Gate creation/exec so no native task code can run before membership; every
   descendant remains in that scope despite fork, double-fork, setsid or reparent.
   The child has no admitted operation/handle allowing it to move scope membership.
3. Identify the root's process incarnation without PID-only equality; persist
   scope and root identities before releasing native work.
4. Observe actual native root outcomes and scope membership/emptiness without
   treating a failed enumeration as empty. TERM/KILL affect only verified members
   of that exact scope, not a recycled group or another consumer.
5. Recover and empty the same scope after supervisor loss without importing a
   different object; prove stable complete emptiness before scratch disposal.

An ordinary process group, `kill(pid,0)`, elapsed timeout, root wait status or
one `/proc`/process-tree scan meets none of the complete-scope requirements alone.
The launcher must stop surviving descendants after root exit and then prove
absence. If it cannot, cleanup is unknown/failed and automatic replacement,
scratch deletion and result consumption stay blocked. Scope custody stays with
the controller through its persisted object identity and reconciliation duty.
No cross-account signal, user-manager stop or shared-service takeover is granted.

No present Linux/macOS runtime/scope tuple is established by this source. A
pre-existing properly delegated containment primitive could be evaluated under
a separate owner grant; this text does not create delegation, privilege or a
service. A platform without a suitable complete-scope primitive is precisely
unsupported for this cleanup contract, even if native non-chat flags exist.
Weakening the property to process groups would need a new explicit source
contract and consumer limit, not an implementation workaround.

## 7. Closed outcomes, replay and consumption

The schema defines `request`, `authorization`, `owner`, `catalog`, `tuple`, `snapshot`,
`head`, `cancel`, `result`, `response`, `replay`, `conflict`, `reservation` and
`ack` records, plus trusted `observation` and `scope-proof` records.
No extension bag is available. Error codes/stages are closed in `$defs.error`;
underlying source error namespaces/codes are bounded data, not new launcher exits.
Unknown errors cannot be silently translated to success or absence.

Native states are `not_started`, `running`, `exit`, `signal` or `unknown`.
An exit carries only an actually observed POSIX 0..255 code and evidence ref;
a signal carries its actual signal number/name and evidence ref, never 128+N.
Preflight refusal has no native exit. Spawn failure has OS evidence but no native
exit. Missing/partial/overflow worker output and cleanup evidence are independent
axes. A protocol-complete result can truthfully contain native/cleanup unknowns;
it remains ineligible for automatic consumption. Complete payload, known exit,
confirmed cleanup and effect reservation still do not mean task acceptance.

| Event | Exact comparison / authoritative state | Response and forbidden effect |
| --- | --- | --- |
| First submit | New exclusive invocation identity; A binds exact N and store | Admit once if every preflight passes; otherwise typed refusal, no native work. |
| Identical submit | Same invocation and exact raw request digest/length; fresh controller authorization may renew expiry without changing N | Return recorded snapshot/result with replay `identical`; no second scope/start. |
| Conflicting submit | Same invocation with any changed request byte/role/tuple/consumer | `invocation_conflict`; preserve original and bounded conflict hash, no overwrite/start. |
| Inspect | Valid control request and authorization, exact store/invocation | Current validated snapshot/result; no signal, spawn, consumption or acceptance. |
| Cancel | First immutable cancel record binds cancel ID and reason | Publish before signaling; supervisor polls at most every100 ms. Identical cancel is idempotent; changed record conflicts, never weakens the first cancellation. |
| Reconcile | Owner lock acquired, validated durable evidence and exact scope identity | Observe/stop only; append new evidence, no restart. Existing result immutable. |
| Same result bytes | Same invocation/input/result digest/length from trusted channel and stored origin | Idempotent retrieval; zero duplicate import/effect. |
| Different result | Same invocation but different bytes, including after consumption | `result_conflict`, retain original and bounded new hash; freeze new consumption, never erase prior decisions/effects. |

`request_id` is response correlation, not an unrestricted duplicate-start token.
Submit idempotency binds the entire raw submit including request_id; the caller
must replay those exact bytes. Cancel idempotency binds cancel ID and exact
record. Read/reconcile responses may advance with newly observed state; their
request IDs do not freeze history. Snapshot generations strictly increase by one
and cannot branch or skip; head/hash corruption refuses, not a stale read.

`consume` has actions `reserve` and `ack`. Reserve requires the exact immutable
result, matching named consumer/assignment, known observed native outcome,
complete worker output, confirmed cleanup and no unresolved conflict. Under
store.lock create `assignments/<id>/reservation.json` exclusively and sync it
before exposing an effect-authorizing response. It binds one effect ID, result,
consumer and invocation across all retry attempts of that assignment. Same
reservation is retrieval only; any different selection conflicts. A later native
retry cannot erase or create another reservation. No automatic winner selection.

The consumer performs only its separately admitted effect, using that effect ID
with a genuinely idempotent destination or explicit reconciliation. `ack` records
its exact external receipt digest after reservation; receipt bytes are inert
input objects, copied and synced under retained objects before ack publication. A crash after reserve/before ack is **effect unknown**, not a
license to repeat an import. Missing ack, partial publication or a lost reply
blocks automatic repetition. This is not a distributed exactly-once transaction,
a truth proof of the external receipt, a review verdict or task acceptance.
Cancellation/reconciliation and conflict retention remain available after reserve.

### Admission order and cross-record invariants

The receiver applies this deterministic order, stopping at the first failed
phase: (1) descriptor types/custody and byte ceilings; (2) strict JSON framing,
lexical rules and schema; (3) A/request byte binding, expiry and S owner/catalog
identity; (4) declared object bytes/unique names and cross-record identity;
(5) existing invocation lookup under locks; (6) new-submit catalog, private
verification, exact composition/effective closure and resource admission;
(7) immutable admission publication. Read/cancel/reconcile/consume never rerun
profile resolution or compatibility preparation. Replays may retrieve old state
with renewed A but cannot run a child against old admission. Missing invocation
is `store_unreadable`, with no creation by a control operation. An O object not
referenced by submit's manifest, or ack's receipt for `consume/ack`, refuses;
other control operations require an empty O. Expired A refuses even a replay.

`identity.controller_id` equals A/S; its invocation ID equals directory/head/
snapshot/observation/result/cancel IDs; assignment/member/consumer/role and exact
submit/tuple digests remain fixed across every generation. Source-review roles
require a nonnull distinct producer_member_id; other roles require null.
Predecessor IDs are never native history. A replacement invocation requires the
same assignment, prior complete scope absence and no unknown effect/conflict;
it cannot bypass an existing assignment reservation. No cross-store retry may
escape these controller-owned assignment obligations. Digest equality is checked
against retained bytes, not a record's unsupported claim.

A snapshot's scope/boot/root incarnation must agree with the durable intent and
native evidence. Admitted requires null scope, not-attempted/not-started and no result;
pre-start cancellation may later retain null scope in a no-start terminal result. Start-intent has a recoverable empty-scope identity, unknown outcome and
no root incarnation; spawned/running have the same nonnull root incarnation.
No transition may move back to admitted/start-intent or allocate a second scope.
Stopping/uncertain retain the greatest proven start/native evidence, never
convert an unknown start to not-started. A terminal snapshot has the exact
result whose final_generation equals its generation; its identity, scope,
native, cleanup and error equal that result. `terminal` excludes native running
or unknown and requires confirmed absence (or no allocated scope/no start).
Unknown outcomes/results use `uncertain`, even if the protocol response completes.
Recovery may append evidence to uncertain, never perform another start. Terminal
records allow later separate conflict/cancel files without rewriting the final
snapshot/result. Contradictory native/scope evidence can only advance to uncertain
with the original result retained; it cannot create another terminal result.
Consumption is frozen by any unresolved conflicting observation.

For `confirmed_absent`, the proof's invocation, full scope, observer and boot
match retained custody; complete/matches/empty are all true. The source-approved
method proves no remaining actor can add members after the observation. Root
exit alone and an enumeration error cannot produce this proof. Known native
exit/signal fields equal the exact trusted observation's decoded wait status,
scope, incarnation and supervisor; recovered reuse preserves original observer
identity. Raw OS status is decoded by the exact platform adapter, never assumed
portable. Actual tuple/argv nulls mean unavailable observation, not requested
values. Actual argv is retained only if its owner contract guarantees no secrets;
credential-bearing argv is an unsupported tuple, not a reason to redact into a
false exact observation. Results do not disclose private provenance or auth.

The worker emits no trusted result envelope. Its only deliverables are regular,
flat files named in `effects.output_names` in a separately admitted private
output root; no path/symlink/archive/special/extra file import. After complete
scope absence the supervisor freezes and hashes those files, retained streams
and trusted observations, then publishes the result. `complete` means every
required output name exists and validates (including an intentionally empty
set); `missing` means none of a nonempty set exists; `partial` means a strict
subset; `invalid` means a forbidden type/name/byte binding; `overflow` means any
output/stream/state bound exceeded. These are output collection states, never
worker truth or semantic acceptance. Non-complete results expose no importable
outputs. Stream length/hash describe retained bounded bytes; truncation or
stream overflow forbids consumption. Result/error evidence is written only by
the trusted supervisor/reconciler. Output overflow records `result_overflow`,
stops within the admitted budgets and never frees prior evidence for capacity.

Cancellation before start can finalize not-started with no scope; after intent
it never rewinds the attempt. A cancel request reason/ID is immutable; the first
internal loss/deadline/signal reason is also frozen. Later causes are diagnostic
observations, not cancellation replacement. Reconcile may seal an unknown result
only as uncertain; no result writer invents native success. An existing result
prevents another finalization; new facts are appended snapshots/conflict hashes.

Response correlation equals the validated request ID/raw digest/operation. For
unparseable N these fields are null (digest may be present only if all raw bytes
were bounded/read); no attacker-supplied partial ID is echoed. An early refusal
has no snapshot/result/reservation/ack; an authenticated conflict may reference
the validated existing snapshot. Snapshot/result/consumption responses reference
only already synced records. The closed error's `source` is an exact bounded
machine code from an owning adapter, never free-form stderr/credential/path data.
No reusable automatic consumption is allowed for uncertain native/cleanup,
truncated streams, any result error or unresolved conflict. The separately
admitted consumer still decides whether an observed nonzero/signal result is
useful; protocol completeness never decides the task verdict.

## 8. Source evidence, compatibility gaps and next-phase packet

At [launcher source base](https://github.com/relux-works/curator-agent-launcher/tree/968d8576f3de8d3001970d09409fc98c99d60ab1):
`internal/plan/plan.go` selects interactive composition and inherited caller env;
`internal/fragment/resolve.go` unconditionally uses `--repair`; and
`internal/execution/process.go` creates a waited process group. Those existing
paths do not implement this protocol, non-discovery, home preservation or full
escaped-descendant cleanup. Neither CLI help nor a source pin attests a native
tuple. Published [Decision0021/v3](https://github.com/relux-works/curator-spec/blob/74a7dcc503c0fd7acd491299ab71290ae40e708e/decisions/0021-sessions-enter-through-curator-run.md#amendment-v3--same-user-trusted-development-member-non-chat-target)
is the owning same-user scope, not a new compatible fragment/adapter.

Core's immutable source disposition at Curator-spec
[`74a7dcc`](https://github.com/relux-works/curator-spec/tree/74a7dcc503c0fd7acd491299ab71290ae40e708e)
and Curator [`933d278`](https://github.com/relux-works/curator/tree/933d2785e2544edd5b9b1bc345dbf41674e8c4aa)
establishes these precise inputs and limits:

| Source | Existing meaning / remaining dependency |
| --- | --- |
| [Environments §10.1](https://github.com/relux-works/curator-spec/blob/74a7dcc503c0fd7acd491299ab71290ae40e708e/protocol/environments.md#101-curator-env-resolve), §9.3; manager §1 | Explicit profile or scoped current; non-repair resolution verifies managed state and refuses stale/unreadable/untrusted state without fragment. Release detection may still probe a runtime: read-only does not mean no subprocess. Stage1 runs neither resolution nor probe. A future preparation phase needs separately admitted probe effects or a source-owned non-probing primitive; the forced-repair launcher route is unsuitable. |
| [Decision0019 §1](https://github.com/relux-works/curator-spec/blob/74a7dcc503c0fd7acd491299ab71290ae40e708e/decisions/0019-fragment-consumers-and-one-construction-site.md) | Agents-management alone constructs argv/channels; it starts nothing and does not resolve profiles. No competing composer or interactive shell bypass. |
| Environments §7.4, §10.3, §10.5; login-readiness §2.1 | Credential policy is operator-owned, separate from profile. Projection/readiness is not login, native qualification or authorization for this route. Existing snapshot lifetime does not admit member execution. |
| [Decision0029](https://github.com/relux-works/curator-spec/blob/74a7dcc503c0fd7acd491299ab71290ae40e708e/decisions/0029-provisioning-origin.md) and [provisioning-origin/1](https://github.com/relux-works/curator-spec/blob/74a7dcc503c0fd7acd491299ab71290ae40e708e/protocol/provisioning-origin.md) | Private origin sidecar captured before mutation, exact adapter/profile/lock/target, original selector/operator anchor/native root and recoverable whole-home lifecycle. Missing/invalid refuses. The raw record NEVER enters fragments, locks, portable input/snapshots or public evidence. No ambient HOME/selector/link fallback, manufactured origin, credential copy or implicit legacy adoption. |
| Curator `internal/envprofile/managed.go` at inspected revision | Still process-native resolution; adopted origin carrier/lifecycle is not established as implemented/qualified. Its separate source-owner implementation, retained-home/orphan lifecycle and contribution custody must be resolved, not recreated here. Opaque tuple hashes cannot fill this gap. |
| Environments §10.2; `protocol/rc7-carriers.md`; Curator `internal/envfragment/envfragment.go` | Closed fragment identity/lock/precedence/paths/channels. Ordinary emission is v2 and managed Muse v4 at inspected source; other published shapes do not imply emission/support. Source closure comes from verified lock/store, never added fragment fields. |
| Decision0021/v3 | Required-manager failure is not compatible absence. No positive strict runtime/home/config/manager-absence tuple or shipped member receipt is supplied. |

Private origin verification must remain within its source-owned local boundary;
the proposed catalog's `private_verification_contract_sha256` identifies the
required callable semantics, not an invented callable API. Until core publishes
that compatible implementation, the tuple refuses. Do not copy the private
sidecar into O/S, expose its raw bytes through a result or inspect credentials to
fill it. Synthetic digest examples confer no authority.

The exact remaining owner-input packet is: source-owned non-repair resolution/probe disposition and
runtime static/tool closure enforcement; non-mutating home/auth/native-origin provenance
with dependency on its separate native-origin fix; exact fragment and manager
required/positive-absence rules; runtime-owned native fresh-creation/discovery
and enforceable model/effort evidence; launcher-owned recoverable complete-scope
primitive for the exact OS/kernel/filesystem. Each input needs source revision,
document digest, callable semantics and positive/negative compatibility limits.
A generic `supported=true`, copied login, guessed home or new wrapper is no input.

The **first supported tuple is currently none**. The bounded first candidate is
one exact already-admitted current-user runtime/release/model/effort, one frozen
static profile, one owner-supported home/auth channel, one exact manager or
attested absence, and one complete-scope implementation. Owners must populate
those exact values before native work; no provider/model or host is selected by
this source. Clean options for an unavailable primitive are owner-supported
publication of one, or explicitly refusing that platform; no forced fit.

After independent Stage1 acceptance and source-owner publication, a **separate
recorded Stage2 assignment** may implement: closed parser and descriptor intake;
immutable store/locks/barriers/replay; exact Curator/SAM adapter admission; gated
spawn and native observations; scope recovery/cancel/cleanup; result staging and
consumer reservation. Implement the existing binary/library construction site,
not an alternate composer. A contract mismatch returns to source review.

The next-phase packet must bind accepted source/schema/fixture digests, exact
binary/module/native tuple and owner inputs, controller/member/consumer/input
identities, filesystem/clock/scope limits, and admitted output/network/provider
budgets. Source implementation requires meaningful positive/negative regression
coverage and independent exact-head review. Its local build/test/race/repro
commands remain held until custodian release; ordinary hosted source CI may run.
New executable checkers/controls are a separate bounded implementation item.

A later **separate native qualification grant** must name one tuple, host/custodian,
finite wall/runtime/provider/token/tool/output/process budgets and isolated scratch,
plus preservation witnesses that reveal no secrets. Required native recipes cover
fresh context sentinel exclusion; system/user/workspace/nested reload and child
control adoption; no a2a/board callbacks; actual selected model/effort; real
exit/nonzero/signal/spawn failure; controller/supervisor loss; escaped descendants,
PID/scope reuse, missing result, replay/conflict and interrupted publication.
Each has nearby real positive controls and exact state/effect assertions from
the inert cases. Maximum concurrent native invocations is one; no retry until
predecessor scope absence. All such witnesses are **UNRUN**. Failure preserves
input/result/orphan evidence and unrelated homes/config/credentials/consumers;
rollback disposes only verified-absent owned scratch. No installation, auth repair,
host adoption, original recipient assessment or usable-executor claim follows.
