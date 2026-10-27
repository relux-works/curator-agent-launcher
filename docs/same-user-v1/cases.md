# Source revision 1 — inert records and manual cases

These are **source examples and manual expected verdicts**, not an executable
corpus, runtime qualification or proof that a parser rejects anything. No native
witness has run. The [owning contract](../same-user-executor-wire.md) defines
semantics; [Draft 2020-12 schema](records.schema.json) defines closed structure.
Revision budget for the external JSON/schema slice is **2**. An incompatible
external constraint returns to source review rather than a locally invented
parser convention or adapter.

All fixture IDs, UIDs, revisions, digests of synthetic texts, runtime/vendor/model,
repositories (`example.invalid`) and scope identities are fictional. No fixture
is an authorization, supported tuple, real wait observation or cleanup proof.
`authorization.json` deliberately expires at millisecond **1**. The positive
structural example therefore refuses `admission/authorization_expired`; with a
fresh independently issued A it still refuses `compatibility_unpublished` under
`catalog-empty.json`. Conditional admitted-state examples below describe future
source obligations ONLY, not an executable path through the empty catalog.

## Record inventory and byte custody

Every listed schema field is required; explicit null is different from omission.
All records and nested objects are closed. A record has exactly its listed kind;
requests additionally select exactly one operation/action. `$defs` owns the
complete type/range/enum/conditional inventory, with no external `$ref`.

| Root schema definition | Fixture / purpose |
| --- | --- |
| request: submit | `submit.json`: nine named objects, disabled network, one requested `patch` output, finite budgets; no history/native tail. |
| request: inspect/reconcile/cancel/reserve/ack | `request-inspect.json`, `request-reconcile.json`, `request-cancel.json`, `request-reserve.json`, `request-ack.json`; typed distinct operations, not implicit resubmission. |
| authorization | `authorization.json`: binds raw submit length/hash, controller/store; deliberately expired. A is a separate trusted handle, not part of O. |
| owner / catalog | `owner.json` binds `catalog-empty.json`; catalog has zero entries. Neither supplied caller text nor valid digest creates compatibility. |
| tuple | `tuple.json`: complete structural shape with synthetic exact source/binary/model/effort/profile/home/manager-absence inputs; no supported runtime. |
| snapshot | `state-admitted.json`, `state-intent.json`, `state-spawned.json`, `state-running.json`, `state-final.json`: generations 1..5, one scope/start. `state-unknown.json` is an alternate uncertain generation5, never simultaneous with final5. |
| head | `head.json`: binds final generation5's exact bytes, not a convenient earlier state. |
| observation | `native-exit.json` / `native-signal.json`: separate synthetic actual wait observations, not 128+signal normalization. |
| scopeProof | `scope-absent.json`: synthetic complete/matches/empty conjunction; flags require real owner-method evidence later. |
| result | `result-exit.json`: complete patch/exit0/absence example; `result-signal.json`: alternate signal/missing-output example, not a replacement result for the same retained invocation. |
| cancel | `cancel.json`: immutable first reason/ID. |
| replay | `replay-identical.json`, `replay-conflict.json`: exact-byte digest classification. |
| conflict | `conflict-result.json`: original result digest retained versus a different synthetic digest, trusted writer origin. |
| response | `response-result.json`: refs final snapshot/result; `response-refusal.json`: unpublished compatibility, no native result. |
| reservation / ack | `reservation.json`, `ack.json`: one assignment/effect selection and inert external receipt; not actual import or receipt truth. |

`instruction.txt`, `source.txt`, `profile.txt`, `effective-config.txt`,
`manager-absence.txt`, `curator-input.txt`, `runtime-input.txt`, `launcher-input.txt`
are inert manifest objects. `tuple.json` is its ninth object. `patch.txt` and
`receipt.txt` illustrate an output and separate ack input. File `ref.name` is
these basenames without the extension; storage lookup uses its raw SHA-256, not
that name. All JSON bytes are UTF-8, two-space indent, LF and final newline;
length/hash bind those exact bytes. This presentation is not a canonicalizer.

`home-provenance.txt`, `native-origin.txt` and `auth-binding.txt` are **synthetic
opaque verification-input illustrations only**, excluded from submit's manifest.
They are not sidecar shapes, real origins/credentials, portable authority or
permission to put private records in O/S. Real private origin/auth verification
stays at its source-owned local boundary. No fixture directory is a ready-to-use
O directory or state store; filenames and ownership are presentation, not an
opened/frozen handle or mode/custody witness.

The negative JSON records are parseable on purpose: `negative-resume.json`
adds an excluded history field; `negative-unknown-member.json` adds an unknown
member; `negative-native-exit.json` uses code256; `negative-terminal-running.json`
claims terminal with native running. They must fail structure, not crash.
The three `.json.txt` files are raw parser cases: escape-equivalent duplicate
key, fractional UID `1000.0` in otherwise valid owner shape, and a truncated
response prefix. Generic JSON Schema mathematical-integer acceptance cannot
replace the required lexical rejection of `1000.0`.

## Paired admission and input cases

Each row has a nearby valid **conditional** control and a forbidden neighbor.
"No child" also means no native/provider/tool effect, identity/presence operation,
config/home/auth repair or history adoption. Before authorized store admission,
refusal writes only a complete response if R is valid; no invocation is created.
There are no actual native pass/fail counts in this document.

| Case | Conditional control / expected source verdict | Forbidden neighbor / exact expected verdict and effects |
| --- | --- | --- |
| A01 entry | Six distinct owned descriptors, exact flags: continue validation | Missing/duplicate/unknown flag, FD alias, stdin/tail: `bootstrap/descriptor_invalid`; no child/store creation. Invalid R means transport exit1, no complete response claim. |
| A02 framing | One bounded complete UTF-8 object: continue | Concatenation/BOM/NUL/bad surrogate/invalid UTF-8/truncation: `parse/encoding_invalid`, no partial ID echo. |
| A03 members | Known closed request: continue | Duplicate decoded key (`duplicate-key.json.txt`), comments or unknown kind/member: `parse/schema_invalid`; no ignored extension. |
| A04 numeric/limits | UID1000 and integer budgets in range: structural control | `fractional-number.json.txt`, exponent, -0: `parse/schema_invalid`; >byte/depth/node/string/range ceiling: `parse/bounds_exceeded`. A generic JSON parse alone is insufficient. |
| A05 authorization | Independently issued current A binds exact N length/hash and store/controller: continue | Stale fixture A: `admission/authorization_expired`; altered byte or unauthorized store: `authorization_invalid`, no native effect even for replay. |
| A06 object bytes | Exact manifest/handles/hash/size, same UID and cooperative frozen custody: continue | Mutation at use, wrong length/hash, symlink/hardlink/special/undeclared object: `admission/input_mismatch`; no convenient reread/retry. |
| A07 role/freshness | Development role with null producer; review role names distinct producer, fresh native creation: continue | Review self-producer or wrong controller/consumer: `identity_mismatch`; hidden resume/history/memory: `compatibility/history_contamination`. `negative-resume.json` already fails schema earlier. |
| A08 catalog | Exact tuple plus all three owner publications admitted in immutable store catalog: continue only if actual checks pass | Actual supplied empty catalog: `compatibility_unpublished`; self-supplied supported flag/publication cannot add entry. Contradicted catalog match: `compatibility_mismatch`. |
| A09 manager | Future source-supported required manager, exact registration/transport/capability; or separately positively attested absence: continue | Missing/failed/unknown required manager cannot become absence: `compatibility_mismatch`; absence without publication: `compatibility_unpublished`; no fallback. |
| A10 fragment | Future exact supported v2 or managed Muse v4 route at inspected producer, closed carrier and verified lock/store closure: continue | Published v5 schema alone, unknown discriminator/fields or source closure invented in fragment: `compatibility_mismatch`; no fragment rewrite. |
| A11 resolution | Future separately admitted preparation uses core non-repair primitive with any probe effects explicitly bounded: continue if supported | Current launcher forced `--repair`, implicit probe or stale/unreadable/untrusted managed state: `configuration_unknown` (or exact source code nested under `compatibility_mismatch`); no repair or live probe here. |
| A12 origin/auth | Future source-owned local verification of exact original origin/binding and auth channel; no transfer of private record: continue | Missing/invalid record, today's HOME/link used as authority, raw record in O, auth copy/login: `compatibility_mismatch` (raw undeclared object fails A06 earlier); no sidecar manufacture or credential read/copy. |
| A13 discovery | Owner primitive pins/excludes every system/user/workspace/nested/reload/child/native-home surface: continue | Unknown discovery/hook/skills/plugin/MCP or inability to inspect without loading: `configuration_unknown`; known messaging/board/presence capability: `presence_contamination`. No silent stripping. |
| A14 handles/effects | Exact tool/read/output/provider allowance and closed inherited handles: continue | a2a/board listener/auth channel inherited, undeclared tool/provider/network or arbitrary argv/env overlay: `presence_contamination` or `compatibility_mismatch` for non-presence policy. No child. |
| A15 scope/storage | Future verified complete-scope adapter and supported fsync/locks/clock with reserved capacity: admit | PID group/tree alone: `cleanup/scope_unknown`; unknown storage semantics: `store_unreadable`; capacity: `store_full`; no gated child release. |

Error selection follows the owning contract's phase order, not table order. For
multiple failures, stop at the earliest phase and left-to-right schema/property
order is NOT a promised parser diagnostic. Within a phase priority is encoding,
bounds, schema; then authorization, owner identity, manifest binding, role identity;
then catalog publication/match, private verification, closure, scope, capacity.
Each source code must be preserved exactly when available, never arbitrary stderr.

## Durable transitions and crash windows

The control chain is generation1 admitted → generation2 start_intent →
generation3 spawned → generation4 running → generation5 terminal. Every edge
requires the prior exact-byte publication barrier. The synthetic clock has
intent/exec at tick0, wait at5000, absence at6000 and final delivery deadline7000;
startup deadline5000 and runtime deadline10000 are independent phase budgets. The uncertain generation5 is
an alternative branch after loss, not an additional generation with equal ID.
All snapshots/results in this directory are synthetic structural illustrations;
none overcomes the expired A/empty catalog in a real admission.

| Case | Positive ordering / observation | Negative crash/neighbor / required state and forbidden effects |
| --- | --- | --- |
| T01 exclusive creation | New directory and parent sync, fixed lock inodes, retained request/A/O, admitted1 sync | Crash/partial bootstrap before admitted: existing identity is uncertain/unreadable, never fresh; no auto-complete or child. |
| T02 before intent | Valid admitted1 proves no start attempt; empty scratch may be disposed only with verified absence | Unexplained temporary/generation or missing/corrupt head: `store_unreadable`, preserve all bytes; do not select old admitted1 as convenient recovery. |
| T03 intent | Empty recoverable scope and intent2 synced before gated spawn | Crash after intent2 before actual spawn: attempt consumed, native unknown; reconcile cannot respawn even if scope is empty. |
| T04 gated child | Child contained and held; actual incarnation published in spawned3 before release | Crash between spawn and spawned3: gate EOF aborts child; reconcile exact scope, outcome unknown until evidence, no native replacement. No publication ⇒ no release. |
| T05 release | Exact inputs/plan rechecked, exec ack observed, running4 synced | Crash across gate release/exec ack: start remains consumed; no fabricated exec/readiness/actual tuple. `uncertain`, no second spawn. |
| T06 native exit | Real wait observation matches scope/root/boot; then full absence proof and result before final5 | Exit0 but escaped descendant lives: stopping/pending cleanup, no terminal-clean/import/scratch disposal. Timeout/PID disappearance is not absence. |
| T07 native signal | `native-signal.json` preserves observed signal15 separately from missing output | `negative-native-exit.json` code256, signal normalized to143 or mismatched raw wait: reject result_invalid; no invented native exit. |
| T08 pre-exec failure | Actual spawn/exec OS error recorded, no native wait exit; verified scope cleanup | Generic wrapper exit1 presented as native exit1: invalid evidence; preserve spawn_failed and no task success. |
| T09 controller loss | EOF/byte on C triggers durable cancel, bounded verified-scope TERM/KILL, retained result | Supervisor also dies: no promised detached survivor; caller retains orphan reconciliation obligation. Never claim guaranteed six-second cleanup after dead supervisor. |
| T10 supervisor loss | Reconciler acquires actually released owner lock and matches persisted kernel scope identity; no spawn | Lock timeout/read failure or reused PID/object/boot mismatch: `scope_unknown`/`store_busy`; no numeric PID signal or assume-empty. |
| T11 recovered wait | Exact durable original supervisor observation may be reused with origin retained | No durable wait status after supervisor loss: native unknown even with absence proof; uncertain result is not auto-consumable. |
| T12 deadlines | Current monotonic phase deadline persisted; stop budgets finite and unchanged on recovery | Reboot, clock discontinuity, phase replay or renewed wall expiry cannot renew native budget; uncertain/refuse, no restart. |
| T13 store failure | Prior bytes/generations retained; result synced before terminal/head/response refs | File/dir sync failure, full volume, missing result or pointer to partial bytes: no successful barrier/exit0 delivery claim without complete R; no trim/evict/old-head repair. |
| T14 generation/state | One scope, monotonic consecutive generations and bound tuple/request/writer | Skipped/branched generation, changed identity/tuple, `negative-terminal-running.json`: schema_invalid or cross-record result_invalid, no consumption/restart. |
| T15 cancellation | First cancel reason/ID synced; admitted may finalize no-start; intent+ never rewinds | Identical cancellation: retrieval only; changed cancellation: cancel_conflict. Already-terminal cancellation cannot erase outcome or reserve another effect. |
| T16 cleanup evidence | Exact complete/matches/empty proof with stable no-new-member scope | Any flag false/missing or failed enumeration: pending/unknown/failed; `scope_unknown` or `cleanup_failed`, no absence claim. |

## Replay, result collection and consumption

| Case | Positive control / expected source verdict | Negative neighbor / exact preservation obligation |
| --- | --- | --- |
| R01 same submit | Same raw bytes/ID, renewed trusted A, valid retained head: replay identical snapshot/result | One whitespace/ID/role/tuple/consumer byte changed: invocation_conflict, closed conflict record, original retained; no second scope. |
| R02 native/worker axes | Actual nonzero/signal can be a complete protocol result with separately stated output/cleanup | Exit0 plus missing/partial/invalid output is not complete task success. Worker-supplied envelope/cleanup text cannot become trusted observation. |
| R03 output | Required flat regular `patch` and bounded streams frozen after full absence: complete collection | Missing subset, extra name/path/symlink/archive/special: missing/partial/invalid as defined, zero importable outputs; no post-cleanup active writer. |
| R04 overflow | Exact ceiling is retained with length/hash/truncated=false | Ceiling+1: worker_output overflow/result_overflow, truncated=true for overflowing stream, bounded stop; no silent truncation success or prior-evidence eviction. |
| R05 actual selection | Owner-supported observation records exact real tuple/secret-free argv | Requested tuple copied as observed, secret-bearing argv or selection not enforceable: mismatch/unsupported, null unavailable fields; no fabricated proof. |
| R06 result publication | Identical retained `result-exit.json` bytes: retrieval only | Different result bytes (`conflict-result.json`): result_conflict and freeze consumption, including after prior ack; no overwrite or erasure of performed effects. |
| R07 delivery | Full R synced/closed, validated refs/state: proposed wrapper transport exit0 | `truncated-response.json.txt`, EOF or wrapper exit0 alone: no protocol completion; read authoritative state using separately authorized inspect, never assume task accepted. |
| R08 reservation | Exact consumer/assignment/result, known native outcome, complete output/streams, absence, no error/conflict: one synced reservation | Unknown native/cleanup, error/truncation/conflict, different effect/consumer/retry: consumption_conflict; no effect-authorizing response. |
| R09 repeated reserve | Same assignment/effect/invocation/result: return original selection | Other invocation/store cannot evade controller assignment identity or uncertain predecessor. No automatic winner or replacement. |
| R10 receipt | Matching reservation plus exact admitted receipt: immutable ack | Changed receipt/reservation: consumption_conflict, original receipt retained. Receipt's existence is not its truth or external task acceptance. |
| R11 external crash | Destination supports real idempotency/reconciliation with reserved effect ID | Crash after reserve/effect/before ack: effect_unknown; no repeat import merely because ack/reply is missing. No distributed exactly-once guarantee. |
| R12 independent roles | Source-review consumer and later frozen-input recipient assessment get separate assignments/results | Producer result or source ACCEPT relabeled recipient/native witness: refuse identity_mismatch; original consumer obligation remains open. |

## Next-phase admission packet (not execution authority)

First supported tuple: **NONE**. Required source owners must name exact
runtime/binary/model/effort/platform/adapter, full discovery/fresh-context primitive,
non-repair/probe disposition, local origin/auth implementation and policy, actual
fragment/required-manager or positive absence, SAM construction mode and complete
recoverable scope method. The private catalog admission must bind those exact
publications and tuple bytes; absence of any one is a bounded unsupported case.

Stage2, after accepted published Stage1 **and a separately recorded assignment**,
can implement descriptor/parser, immutable store, composition/admission, gated
scope lifecycle and result/reservation in the existing binary. No parser/CLI or
checker is implemented here. Source mismatch returns to the same independent
review route. Local heavy-suite hold is not lifted by a hosted source check.

Later native grant: one invocation concurrently, isolated owned scratch, one
exact tuple, finite budgets no larger than this contract, admitted provider/tool
channel and custodian preservation/cleanup recipe. Cover every conditional valid
case with actual positive controls and every negative with its intended state/
effect alarm, including escaped descendants, lost supervision and failed sync.
All native/recipient/provider/qualification rows remain **UNRUN**. If required
semantics are unavailable, report exact source/platform constraint and clean
owner alternatives rather than forcing a mock or weaker fallback to pass.
