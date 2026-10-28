# Source revision 2 — inert records and manual cases

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
| request: submit | `submit.json`: ten named objects (nine logical inputs plus assignment binding), disabled network, one requested `patch` output, finite budgets; no history/native tail. |
| request: inspect/reconcile/cancel/reserve/ack | `request-inspect.json`, `request-reconcile.json`, `request-cancel.json`, `request-reserve.json`, `request-ack.json`; typed distinct operations, not implicit resubmission. |
| authorization | `authorization.json`: binds raw submit length/hash, controller/store; deliberately expired. A is a separate trusted handle, not part of O. |
| owner / catalog | `owner.json` binds `catalog-empty.json`; catalog has zero entries. Neither supplied caller text nor valid digest creates compatibility. |
| tuple | `tuple.json`: complete structural shape with synthetic exact source/binary/model/effort/profile/home/manager-absence inputs; no supported runtime. |
| snapshot | `state-admitted.json`, `state-intent.json`, `state-spawned.json`, `state-running.json`, `state-final.json`: generations 1/4/5/7/8, one scope/start. Allocation generations2/3 and gate release-intent6 are below. `state-unknown.json` is an alternate uncertain generation8, never simultaneous with final8. |
| head | `head.json`: binds final generation8's exact bytes, not a convenient earlier state. |
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
are inert manifest objects. `tuple.json` is its ninth logical input; `assignment.json` is the tenth manifest object. Assignment excludes itself from its logical array, avoiding a hash cycle. `patch.txt` and
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
key in an otherwise valid inspect (both values inspect), fractional UID `1000.0` in otherwise valid owner shape, and a truncated
response prefix. Generic JSON Schema mathematical-integer acceptance cannot
replace the required lexical rejection of `1000.0`.

## Paired admission and input cases

Each row has a nearby valid **conditional** control and a forbidden neighbor.
"No child" also means no native/provider/tool effect, identity/presence operation,
config/home/auth repair or history adoption. Before authorization/store custody, refusal writes only a complete response if R is valid. After that, a guarded refusal can append journal/conflict evidence but creates no native invocation attempt unless atomic assignment admission completed.
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

The nominal chain is admitted1 → allocating2 → allocated3 → start_intent4 →
spawned5 (gate held) → spawned6 (gate release_intent) → running7 → terminal8.
Each edge requires durable publication including journal completion before its
permitted task effect. All ticks are synthetic: allocation/intent/exec at0,
wait at5000, absence at6000 and delivery deadline7000; startup5000/runtime10000.
The uncertain8, cancel-won2, cancel-after-intent5, cancel-before-release6 and
stopping8..12 are ALTERNATIVE branches, not competing publications in one store.
All examples remain conditional on future source-supported tuples/fresh A.

| Case | Positive ordering / observation | Negative crash/neighbor / required state and forbidden effects |
| --- | --- | --- |
| T01 exclusive creation | New directory and parent sync, fixed lock inodes, retained request/A/O, admitted1 sync | Crash/partial bootstrap before admitted: existing identity is uncertain/unreadable, never fresh; no auto-complete or child. |
| T02 before intent | Valid admitted1/not_requested proves no start AND no allocation; only verified owned scratch may be disposed | Unexplained temporary/generation or missing/corrupt head: `store_unreadable`, preserve all bytes; do not select old admitted1 as convenient recovery. |
| T03 intent | Empty recoverable scope and intent4 synced before gated spawn | Crash after intent4 before actual spawn: attempt consumed, native unknown; reconcile cannot respawn even if scope is empty. |
| T04 gated child | Child contained and held; actual incarnation published in spawned5 before release | Crash between spawn and spawned5: gate EOF aborts child; reconcile exact scope, outcome unknown until evidence, no native replacement. No publication ⇒ no release. |
| T05 release | Exact inputs/plan rechecked, exec ack observed, running7 synced | Crash across gate release/exec ack: start remains consumed; no fabricated exec/readiness/actual tuple. `uncertain`, no second spawn. |
| T06 native exit | Real wait observation matches scope/root/boot; then full absence proof and result before final8 | Exit0 but escaped descendant lives: stopping/pending cleanup, no terminal-clean/import/scratch disposal. Timeout/PID disappearance is not absence. |
| T07 native signal | `native-signal.json` preserves observed signal15 separately from missing output | `negative-native-exit.json` code256, signal normalized to143 or mismatched raw wait: reject result_invalid; no invented native exit. |
| T08 pre-exec failure | Actual spawn/exec OS error recorded, no native wait exit; verified scope cleanup | Generic wrapper exit1 presented as native exit1: invalid evidence; preserve spawn_failed and no task success. |
| T09 controller loss | EOF/byte on C triggers durable cancel, bounded verified-scope TERM/KILL, retained result | Supervisor also dies: no promised detached survivor; caller retains orphan reconciliation obligation. Never claim guaranteed six-second cleanup after dead supervisor. |
| T10 supervisor loss | Reconciler acquires actually released owner lock and matches persisted kernel scope identity; no spawn | Lock timeout/read failure or reused PID/object/boot mismatch: `scope_unknown`/`store_busy`; no numeric PID signal or assume-empty. |
| T11 recovered wait | Exact durable original supervisor observation may be reused with origin retained | No durable wait status after supervisor loss: native unknown even with absence proof; uncertain result is not auto-consumable. |
| T12 deadlines | Current monotonic phase deadline persisted; stop budgets finite and unchanged on recovery | Reboot, clock discontinuity, phase replay or renewed wall expiry cannot renew native budget; uncertain/refuse, no restart. |
| T13 store failure | Prior bytes/generations retained; result synced before terminal/head/response refs | File/dir sync failure, full volume, missing result or pointer to partial bytes: no successful barrier/exit0 delivery claim without complete R; no trim/evict/old-head repair. |
| T14 generation/state | One scope, monotonic consecutive generations and bound tuple/request/writer | Skipped/branched generation, changed identity/tuple, `negative-terminal-running.json`: schema_invalid or cross-record result_invalid, no consumption/restart. |
| T15 cancellation | First cancel reason/ID synced under store→edit; cancel winning before intent forbids child, intent winning never rewinds | Identical cancellation: retrieval only; changed cancellation: cancel_conflict. Already-terminal cancellation cannot erase outcome or reserve another effect. |
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
| R08 reservation | Exact consumer/assignment/result, known native outcome, complete output/streams, absence, no error/conflict: one synced reservation | Unknown native/cleanup, error/truncation or different effect/consumer/retry: consumption_conflict. Unresolved/conflict journal is checked earlier and returns store/store_interlocked. Neither response authorizes an effect. |
| R09 repeated reserve | Same assignment/effect/invocation/result: return original selection | Other invocation/store cannot evade controller assignment identity or uncertain predecessor. No automatic winner or replacement. |
| R10 receipt | Matching reservation plus exact admitted receipt: immutable ack | Changed receipt/reservation: consumption_conflict, original receipt retained. Receipt's existence is not its truth or external task acceptance. |
| R11 external crash | Destination supports real idempotency/reconciliation with reserved effect ID | Crash after reserve/effect/before ack: effect_unknown; no repeat import merely because ack/reply is missing. No distributed exactly-once guarantee. |
| R12 independent roles | Source-review consumer and later frozen-input recipient assessment get separate assignments/results | Producer result or source ACCEPT relabeled recipient/native witness: refuse identity_mismatch; original consumer obligation remains open. |

## Revision 2 paired durable-order counterexamples

These rows are **manual expected verdicts**, not a transition executor or native
pass/fail count. JSON files named here are structurally valid unless listed as
one of the four `negative-*` schema cases. In particular changed-input/second-first/
unknown-predecessor submits and renewed stop deadlines are semantic negatives:
Draft acceptance does not admit them. Every reference is to synthetic bytes.

| Record group | Inert source examples / custody |
| --- | --- |
| assignment / attempt | `assignment.json`, `attempt-first.json`, `attempt-retry.json`; unchanged raw binding and ordinal1→2. `submit-retry.json` changes only request/invocation/predecessor, with the SAME binding. `assignment-changed.json` and `submit-changed-input.json` form a self-consistent changed logical request, rejected against retained binding. |
| first/predecessor neighbors | `submit-second-first.json` claims null predecessor after ordinal1; `submit-unknown-predecessor.json` names an unowned ID. Neither is a second admitted attempt. |
| journal | `operation-submit.json` + `operation-clear.json` are sequence1 admission. `operation-phase-2.json` through `operation-phase-8.json` and their `-clear.json` partners bind the nominal snapshot2..8 publications, giving a consecutive clear prefix before sequence9. `operation-reserve.json` + `operation-reserve-clear.json` are a conditional normal sequence9 reservation. ALTERNATIVE sequence9 `operation-result-conflict.json` precedes comparing `result-signal.json` to immutable `result-exit.json`. `operation-conflict.json` is only the successful conflict-publication branch; failed-create/no-temp has NO completion/conflict file. |
| allocation | `state-allocating.json`, `state-allocated.json`, `state-allocation-unknown.json`: before-call and after-call-before-publication both retain the SAME intent until exact identity evidence exists. |
| cancel / gate | `state-cancel-won.json` + `result-cancel-before-allocation.json`: no child/resource. `state-cancel-after-intent.json`: consumed attempt, scope pending, no rewind. `state-release-intent.json` and `state-cancel-before-release.json`: alternate gate decision6 after spawned5. |
| stop | `state-stop-term-intent.json`, `state-stop-term-observe.json`, `state-stop-kill-intent.json`, `state-stop-kill-observe.json`, `state-stop-unknown.json`: begun4000, original TERM5000/KILL6000, stages8..12. `state-stop-renewed.json` is a structurally valid but forbidden deadline rewrite. |

| Case / finding | Nearby conditional positive | Crash/race/negative and exact manual verdict |
| --- | --- | --- |
| D01 B1 normal reserve | Clear operations1..8, terminal8, no conflicts, sequence9 reserve intent → reservation → clear completion → response | No effect authorization before the final completion sync. Existing reservation with missing completion stays effect-unknown/unavailable, never assumed delivered. |
| D02 B1 failed create/no temp | Sequence9 result-conflict intent synced BEFORE comparison | Conflict detected, conflict temporary creation leaves NO bytes, supervisor dies: sequence9 lacks completion. Every later reserve/start/gate sees unresolved intent ⇒ `store/store_interlocked`; original result unchanged, zero new import authorization. |
| D03 B1 failed sync/temp | Same prior durable intent | Conflict temp exists but file/dir sync fails: unresolved intent + temp, same store-wide freeze. Recovery preserves both; it cannot select old terminal8 as clean or write speculative clear. |
| D04 B1 initial-intent failure | Successful initial intent permits comparison | Failure before any initial intent/directory bytes: NO target comparison or conflict classification occurred, `store_unreadable`, no effect response. Partial directory instead freezes. A later clean reserve is allowed ONLY in the former no-comparison/no-bytes case, not D02. |
| D05 B1 conflict completed | Exact differing bytes produce conflict + conflict completion | Completion succeeds or fails: conflict or unresolved entry blocks all subsequent consumption. A prior authorized effect/ack is retained, not erased or falsely revoked. |
| D06 B2 same-input retry | First attempt terminal8/absent/no consumption, unchanged `assignment.json`, I2 predecessor I1 ⇒ ordinal2 under store lock | `submit-changed-input.json` uses self-consistent changed max_processes/binding: `admission/assignment_conflict`; preserve binding/attempt1, no new attempt or child. New assignment requires separate authorization. |
| D07 B2 two firsts | First null-predecessor wins ordinal1 under store lock | `submit-second-first.json` waits then sees latest I1: assignment_conflict, no ordinal2/start. Its conflict interlock may stop winner/consumption; cannot authorize a second native attempt. |
| D08 B2 predecessor/cross-store | Exact latest predecessor with durable no-allocation or complete absence | Unknown ID, older predecessor, ordinal gap, partial admission or allocation-unknown: assignment_conflict/store_unreadable, no retry. Different S/controller custody refuses; copying proof or issuing a second owner label cannot transfer ownership. |
| D09 B3 cancel first | Cancel.json synced under store→edit before start_intent | Supervisor later gets locks and must refuse intent/spawn. With admitted1/not_requested, cancel-won2 finalizes not-started/no resource; polling is irrelevant. If cancel snapshot sync failed, durable cancel still wins; if cancel publication ambiguous, journal blocks start. |
| D10 B3 intent first | Intent4 synced before cancel obtains locks; attempt consumed | Cancel then wins before gated spawn: no spawn, consumed/unknown attempt + exact scope cleanup; never not-attempted. Cancel after spawned5 but before release closes gate/cleanup, no native exec. |
| D11 B3 gate/lost custody | Release intent6, immediate C/custody/cancel checks, actual gate release under store→edit | Cancel wins before critical section: no release. Death or C loss across final observation/release: ambiguous start, no replay of gate, bounded stop. Cancellation after release cannot claim retrospective zero effects. |
| D12 S1 TERM crash | term_intent8 fixes begun4000, TERM5000, KILL6000 and action IDs | Crash before TERM and after TERM/before term_observe both recover ambiguous SAME action: idempotent reissue only if owner supports it and tick<5000; otherwise unknown. Never a new 1000ms grace. |
| D13 S1 KILL crash | kill_intent10 precedes KILL, kill_observe11 preserves both deadlines | Crash before/after KILL stays in original kill stage. At5500 no return to TERM; at>=6000 observe/retain only then unknown absent proof. `state-stop-renewed.json` refuses result_invalid, not a renewed budget. |
| D14 S1 recovered clock | Original boot and monotonic clock, original deadlines retained | Reboot/clock/scope mismatch: unknown; generation count and wall time cannot reconstruct a new stop stage or signal a recycled identity. |
| D15 S2 before allocator | admitted1/no allocation intent ⇒ no allocation and no native start | Persist allocating2 then crash BEFORE call: possible allocation cannot be excluded from that durable state. Without exact evidence, allocation-unknown3, cleanup unknown, no replacement/consumption/scratch disposal. |
| D16 S2 orphan empty scope | Allocator returns identity and allocated3 syncs ⇒ exact scope can be recovered | Allocator creates empty scope then dies BEFORE identity publication: same unresolved allocating2 as D15, no second allocation. Retain controller orphan duty; no assumption of automatic kernel disposal or available lookup primitive. |

## Next-phase admission packet (not execution authority)

First supported tuple: **NONE**. Required source owners must name exact
runtime/binary/model/effort/platform/adapter, full discovery/fresh-context primitive,
non-repair/probe disposition, local origin/auth implementation and policy, actual
fragment/required-manager or positive absence, SAM construction mode and complete
recoverable scope method. The private catalog admission must bind those exact
publications and tuple bytes; absence of any one is a bounded unsupported case.

Stage2, after accepted published Stage1 **and a separately recorded assignment**,
can implement descriptor/parser, immutable assignment/attempt and journal interlock, cancel/start gate ordering, durable allocation and stop stages, composition/admission, gated
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


## Bounded decoder implementation coverage (nonnormative)

`internal/memberwire` implements only untrusted request/authorization syntax and
closed structure, plus the six descriptor-number argument grammar. The earlier
Stage1 inventory and manual transition verdicts remain source examples; this
annotation does not turn them into an executor or native qualification corpus.
The wire §2 counting clarification is reviewed with the decoder boundary tests;
the frozen Stage1 baseline did not define that counting convention.

| Source obligation | Executable source tests / exact limit |
| --- | --- |
| Six request branches, separate A, original bytes/length/SHA-256 and ownership | `TestPublishedStructuralSnapshots`, `TestAuthorizationStructure`; expired A remains structurally valid, never authenticated or current. |
| A01 flag grammar | `TestDescriptorPermutations`, `TestDescriptorRefusals`; all720 orders, number endpoints and forbidden neighbors, no actual FD inspection. |
| A02 complete object, raw ceilings and framing | `TestRawCeilingsAndFraming`, `TestLexicalNumbersAndFraming`; exact caps and typed nil-result refusals, no partial response or ID echo. |
| A03 duplicate decoded keys, closure, null and branch selection | `TestDecodedNamesAndUnicode`, `TestRequestClosureAndStaticErrors`, `TestSchemaFieldBoundsAndBranches`; nested and escaped duplicates, recursive required fields, static nonreflecting tags. |
| A04 numbers, depth16, nodes8192, decoded-string4096 bytes | `TestCountingAndDecodedByteBoundaries`, `TestLexicalNumbersAndFraming`; inclusive/over-limit controls, containers-only depth, every value once excluding keys. Synthetic lexical positives explicitly fail public request structure. |
| Recursive bounds, enum/const, array cardinality/uniqueness and network conditions | `TestSchemaFieldBoundsAndBranches`, `TestEffectArraysAndNetworkBranches`; structurally permitted duplicate artifacts and role combinations do not prove semantic admission. |

These tests run through ordinary hosted PR checks. Local Go/build/test/race and
native/fixture-executor execution were held during authorship. A05 expiry/binding,
A06 handle/object custody, A07–A15 admission/compatibility and all durable lifecycle,
replay, effect and cleanup rows remain unimplemented by this package. No output
from this decoder authorizes any such operation; the first supported tuple remains
**NONE**.


## Bounded N/A handle-byte implementation coverage (nonnormative)

`internal/memberinput.ReadPair` observes two explicit already-open request and
authorization files on Darwin/Linux. It returns only untrusted immutable raw
bytes/length/SHA-256. It does not decode, bind or authenticate A, and does not
implement the six-descriptor gate, objects/store/response/controller custody,
entry wiring or any runtime/admission path. Unsupported OS builds explicitly
refuse without accessing the supplied handles.

| Source obligation / neighbor | Ordinary executable source tests / precise boundary |
| --- | --- |
| N/A byte custody, fixed fixture digests, nonzero shared offsets, caller lifetime | `TestPublishedHandleSnapshots`; all six N fixtures and expired A as bytes only, caller originals/duplicate offsets and flags unchanged, independent retained snapshots after caller mutation/close. |
| Inclusive raw byte ceilings, empty input vs JSON validity | `TestRawHandleBoundaries`; exact 64 KiB/4 KiB and empty/non-JSON bytes are intake positives; one-over metadata returns `parse/bounds_exceeded` before either read. No structural-validity claim. |
| Types/access and validation before content | `TestHandleTypeAndAccessRefusals`, `TestLinuxPathOnlyRefusal` / `TestDarwinEventOnlyRefusal`; invalid neighbors in either position return `bootstrap/descriptor_invalid`, zero positional reads, no partial pair and caller handles/content/offsets/modes preserved. |
| Actual owner/mode/one-link/distinct identity observations | `TestModesLinksAndAliases`; real mode/link/alias fixtures, distinct equal-byte files positive. `TestSyntheticMetadataRefusals` isolates wrong UID and negative size as synthetic predicate cases, not a foreign-owner/chown OS witness. Filesystems not retaining special bits are explicitly skipped. |
| Bounded reads, short/growth/change/error and all-or-nothing result | `TestObservedChangesAndReadFailures`; per-call read seam delegates to real Pread with deterministic owned-file mutations, plus partial-progress positive. Exact static errors and no pair; explicit test mutations persist, with no additional input repair/write/offset/close effects. |
| Honest freeze/lifetime limit | `TestLaterWriterDoesNotChangeSnapshot`; retained bytes survive a later same-UID writer, but no absence-of-writer/mapping, future freeze or complete custody is attested. |
| Unsupported-platform source route | `TestUnsupportedRefusesWithoutInputEffects` is authored for unsupported OS builds; existing Darwin/Linux CI does not execute that branch. |

Production uses no duplicate/reopen/close or descriptor-owning finalizer: it
pins caller os.File lifetimes only during observation through SyscallConn Control.
Thus there is no temporary descriptor to release or leak, including refusal
paths. Metadata comparison covers native device/inode, UID/mode/link/size/access
flags and observed mtime/ctime, excluding access time which reading may update.
It does not detect every same-size transient mutation, prove no write mapping,
freeze files, or promise a disk-I/O deadline. Cooperative frozen sources and
stable caller handles remain caller preconditions. All six descriptors must be
validated together before content in later entry integration, with renewed
consumption/binding/custody checks; this primitive alone must not admit work.

These ordinary tests execute only in genuine hosted PR CI during this unit;
local Go/build/test/race/repro/parser/checker/mutation/native/cache operations
remain held. Source cases above remain inert lifecycle expectations. Full A06
object custody, all other descriptors/admission/lifecycle obligations and native
qualification remain undelivered; first supported tuple remains **NONE**.


## Retained O object implementation coverage (nonnormative)

`internal/memberobjects.Acquire` observes only an explicit already-open O handle
and a typed untrusted reference projection on Darwin/Linux. It owns read-only
CLOEXEC root/object handles, immutable exact bytes/length/SHA-256 and a private
projection copy. `Snapshot` returns only an original byte observation with fresh
`Raw` copies; `References` returns an independent copy. Digest basenames alone
are used for descriptor-relative lookups. Same-digest/equal-length aliases count
once. Empty refs require exactly empty O; this is not request validity or admission.

| Source obligation / neighbor | Ordinary hosted source tests / precise boundary |
| --- | --- |
| Exact fixed bytes, aliases, independent views, caller and owned handle lifetimes | `TestSnapshotsAliasesAndLifetime`; fixed abc/empty digest oracles, caller reference/Raw mutations, retained nonzero file offsets, nonzero shared caller directory offset and flags; real CLOEXEC/EBADF checks. Caller closure leaves owned custody live; explicit repeated Close invalidates it. Prior snapshots remain only bytes. |
| Inclusive caps and bounded declaration preflight | `TestInclusiveCapsAndEmpty`, `TestProjectionRefusalsBeforeAccess`; real 128-name alias and four distinct 4 MiB files/16 MiB positives; invalid name/digest/duplicate/conflicting-length, one-over count/length/total and uint64-max refuse before root open/content. Per-object and total caps count unique raw bytes; this layer does not enforce trusted-instruction/role policy. |
| Actual O/root and flat-set contracts | `TestRootContractRefusals`, `TestObjectRefusals`; invalid roots, exact-mode/special-bit neighbors, missing/extra/noncanonical names, subdirectory, symlink to owned external fixture, FIFO, socket, file mode/link/size/hash negatives; no partial collection, source repair or external-target write. Nonblocking no-follow opening precedes regular-file validation. Unavailable special bits skip explicitly. |
| Platform unreadable handles and UID predicates | `TestPlatformUnreadableHandles` opens actual O_PATH (Linux) / O_EVTONLY (Darwin) root/object handles and requires refusal before content; normal O is nearby. `TestSyntheticMetadataPredicates` labels UID/negative-size/writable-flag predicate cases synthetic, never a foreign-owner OS witness or chown. |
| Bounded reads and all-or-nothing resource disposal | `TestReadFaultsAndAllOrNothing`, `TestOpenAndRecheckReadErrors`; deterministic actual short/growth/same-size writes and later read error must fire with exact static errors, nil collection and native EBADF for all owned descriptors. Late invalid object follows an earlier successful read. Positive partial progress excludes reject-everything. |
| Consuming recheck, original snapshots and failure disposal | `TestConsumingRecheckAndFailureDisposal`; unchanged real reread positive, then actual bytes/rename-replacement/extra/mode/root-mode/link/missing changes refuse, close all owned custody, retain earlier bytes and never adopt or repair input. |
| Explicit deterministic Close and ambiguous failure | `TestCloseFailureAndFDReuse`; injected error follows a real owned close, remaining handles still close, custody stays unavailable, and repeated Close cannot close an actually reused test-owned FD number. No system-wide FD scan. |
| Unsupported source route | `TestUnsupportedNoInputAccess` is authored but unrun by the existing Darwin/Linux matrix; it refuses without accessing input, not platform qualification. |

Projection count >128, length >4 MiB or unique total >16 MiB returns
`parse/bounds_exceeded`; invalid declaration/collection/file/hash/observed change,
closed custody or ambiguous disposal returns `admission/input_mismatch`.
Invalid O contract/acquisition returns `bootstrap/descriptor_invalid`. All tags
are static, bounded and nonreflecting, with no logging or stdout/stderr output.
Enumeration uses a separate descriptor-relative open-file description and reads
at most unique-count+one names, preserving caller offsets. Object reads use
bounded positional byte0 reads and native metadata/hash checks; retained unique
raw bytes stay within 16 MiB. Recheck temporarily reads at most one bounded
object at a time in addition to originals. Every created descriptor is CLOEXEC;
all are closed on acquisition/recheck refusal, never caller originals.

Consumers must call `Recheck` immediately before each separately admitted use,
retain the collection through it and `Close` afterwards. Recheck is not a
continuous watch or atomic turn fence. Any recheck refusal invalidates custody;
`Snapshot`, `References` and further rechecks then refuse. `Close` marks custody
unavailable before attempting every close once; an ambiguous close error is
retained on repeats and raw FD numbers are never retried. Snapshots obtained
before disposal remain immutable observations, not handles or authority.

Stable caller handles/flags and cooperative frozen directory/files, no other
writer/mapping, remain preconditions. Native observations exclude atime, which
reading may update; they cannot establish hostile same-UID write protection,
every transient same-size write, coherent concurrent snapshot, future freeze
or a wall-clock disk deadline. Opaque bytes are not archive-content, role or
trusted-instruction validation. There is no extraction, materialization, path/
HOME discovery, store/lock/response effect, child or native execution. All six
FDs must still be validated together before content in later integration.
No A/request binding/expiry, semantic admission, entry wiring, supported tuple,
full custody gate or overall executor is delivered here. Local execution remains
held; these new tests run only in ordinary genuine hosted PR checks.


## Untrusted owner/catalog/tuple decoder coverage (nonnormative)

`internal/memberwire.DecodeOwner`, `DecodeCatalog` and `DecodeTuple` accept only
these three closed structural roots, bounded before copying/parsing at 4 KiB,
64 KiB and 64 KiB respectively. They reuse the existing strict parser and owned
`UntrustedRecord` with fresh Raw copies and recursive Value copies. Exact raw
whitespace, lengths, hashes and every required field are retained. No network,
source resolution, UID/home observation, private verification, catalog admission,
FD/store effect or native execution follows from a successful structural decode.

| Source obligation | Ordinary hosted executable source coverage / limit |
| --- | --- |
| Published owner, empty catalog and synthetic tuple bytes; immutable observations | `TestCompatibilityPublishedSnapshots`; independently fixed fixture lengths/hashes and pre-mutation complete raw/value expectations; caller input, Raw, nested tuple document and existing catalog entry/owner-list elements are mutated, with fresh and earlier Value views checked independently. A synthetic nonempty catalog is memory-only test data. |
| Per-root inclusive raw caps, exact kind and framing | `TestCompatibilityRawBoundsAndRoots`; padded valid records at each cap and one-over malformed/valid neighbors; no cross-kind acceptance or partial returned record. |
| Every reachable closed shape and both manager/effort alternatives | `TestCompatibilityRecursiveClosureAndTypes`, `TestCompatibilityManagerAndEffortBranches`; missing/null/unknown/incompatible scalar/array/object fields, exact branches and forbidden cross-branch fields with nearby valid controls. Unavailable is not compatible absence. |
| Grammar/range boundaries | `TestCompatibilityFieldGrammars`; UID1..4294967295, IDs/digests/names/refs, exact HTTPS ASCII repository grammar/2048 bound and 40-or-64 lowercase revisions with invalid neighbors. These are data checks, not actual source publications or host compatibility. |
| Unicode scalar versus byte counts | `TestCompatibilityUnicodeStringBounds`; free strings at1/max versus0/max+1 for ASCII, multibyte and astral codepoints, including escaped surrogate pairs. The separate existing global4096 decoded-byte cap remains in force; no floating-model/fragment support policy is inferred. |
| Owner coverage and array bounds, deliberate schema asymmetry | `TestCompatibilityOwnerCoverageAndCardinality`; tuple requires at least one curator/runtime/launcher, permits repeats with coverage and3..16 entries. Catalog entry owner_inputs has3..16 items but NO contains requirement: all-same known owners remain structurally valid. Catalog0..32 entries may repeat tuple refs. Admission must separately reject unapproved/duplicate/contradictory publications. |
| Strict parser routing and static refusal privacy | `TestCompatibilityStrictEntryRefusals`; each API rejects duplicate escaped/nested names, malformed UTF-8/BOM/NUL/surrogates and forbidden number forms, with exact nil-result static parse tags and unchanged caller bytes. Existing depth16/node8192/decoded4096-byte boundary suite is reused unchanged. |

All positives here establish untrusted structure only. The published native
catalog still has zero positive entries; repeated refs and synthetic entries do
not become authority. Cross-record binding/expiry, role policy, actual effective
profile/home/origin/manager compatibility, store/admission, six-FD entry and
native qualification remain separate. Local execution was held during authorship;
new tests execute only in genuine ordinary hosted PR checks. The first supported
native tuple remains **NONE**.


## Untrusted lifecycle decoder coverage (nonnormative)

`internal/memberwire.DecodeSnapshot`, `DecodeHead`, `DecodeCancel`,
`DecodeObservation` and `DecodeScopeProof` check only their closed structural
roots, bounded before copying/parsing at 16/16/4/64/16 KiB respectively. They
reuse the strict parser and immutable `UntrustedRecord`; all earlier source,
schema, fixtures and tests remain unchanged. No file, network, clock, process,
store, admission or native observation effect follows from decoding these bytes.

| Source obligation | Ordinary hosted executable source coverage / limit |
| --- | --- |
| Published lifecycle byte anchors and owned snapshots | `TestLifecyclePublishedSnapshots`; fixed lengths/digests for nineteen state snapshots, head, cancel, both observation examples, scope proof and the terminal-running structural negative. Complete independent raw/value expectations precede decoding and applied caller/Raw/nested-map/existing-array mutations; fresh and earlier Value views remain frozen. |
| Per-root exact kinds, inclusive raw caps and framing | `TestLifecycleCapsAndRoots`; cap-1/cap padded positives, valid and malformed cap+1 bounds refusals, cross-kind and incomplete/nonobject inputs with exact nil/static tags. |
| Every closed reachable union alternative and required null/type rule | `TestLifecycleRecursiveClosure`, `TestLifecycleUnionSelection`; deletion, unknown member, incompatible types and cross-branch fields, all start/native/cleanup/allocation/wait variants, required nullable fields, writer/identity roles and stop enums. |
| Snapshot phase conditionals | `TestLifecyclePhaseConditionals`; all nine phases, all five gates, each property constrained by each published if/then independently violated with otherwise valid structure. Terminal native/cleanup alternatives and unconstrained phase properties remain positive controls; uncertain has no invented condition. |
| All transitive grammar/range occurrences | `TestLifecycleGrammarsAndRanges`; IDs/digests/names/refs/revisions, generation1..256, raw_status0..4294967295, exit0..255/signal1..255, zero-permitted observation/begun ticks versus positive request/deadline ticks. |
| Error records as data | `TestLifecycleErrorData`; all thirty allowed codes across eleven exact stages and forbidden cross-pairs, independently nullable source/evidence, source code ASCII1..128 including initial punctuation, with invalid neighbors. A valid error record is not a newly observed OS failure. |
| Untrusted scope/wait fields | `TestLifecycleSignalsAndUntrustedProofs`; both boolean values, all eight proof-bit combinations, uppercase/digit signal names, and deliberately inconsistent boot/raw-status/deadline data that structural schema does not compare. False proof bits never establish absence. |
| Null versus empty argv and Unicode/byte caps | `TestLifecycleObservationArguments`; null/empty/1/128 versus129 arguments, nonempty exact scalar strings and separate4096 decoded-byte ceiling including literal/escaped multibyte and astral values. Existing array elements are mutated against independently frozen full-value expectations; no real argv is observed. |
| Each API's strict entry/refusal/privacy route | `TestLifecycleStrictEntryRefusals`; escaped and nested decoded duplicates, malformed UTF-8/BOM/NUL/surrogates, forbidden lexical numbers and unknown private markers, nil results/static parse tags and unchanged inputs. Existing depth16/node8192/decoded-byte suite is reused unchanged. |

`state-stop-renewed.json` and other semantics-only-invalid states remain
structurally positive. `negative-terminal-running.json` violates the snapshot
conditional; `negative-native-exit.json` is an out-of-scope result whose native
code256 is separately projected into a synthetic snapshot negative. None of
these labels supplies a transition executor or native evidence. Required
cross-record equality, deadline ordering/history, semantic roles, trusted proof
origin, store/replay/supervisor actions and admission remain separate. Local
execution was held during authorship; new tests run only in ordinary genuine
hosted PR CI. No supported native tuple or usable executor is established.


## Untrusted outcome decoder coverage (nonnormative)

`internal/memberwire.DecodeResult`, `DecodeResponse`, `DecodeReservation`,
`DecodeAck` and `DecodeConflict` check only closed untrusted record structure.
Raw caps are 64/16/16/16/16 KiB respectively, before copying/parsing. Replay is
fully validated inside the response cap; there is no standalone replay intake
or separate replay ceiling. Existing parser, immutable copies and all previous
source/tests/schema/fixtures remain unchanged.

| Source obligation | Ordinary hosted executable source coverage / limit |
| --- | --- |
| Exact original byte anchors and immutable views | `TestOutcomePublishedBytes`; twelve fixed lengths/digests checked before decoding, including two replay examples embedded into responses, native-code256 structural negative and truncated-response encoding negative. The unchanged independent frozen-before-decode oracle changes caller/Raw bytes, all nested maps and existing argv/output array elements, then checks retained/fresh/earlier views against original expectations. |
| Five raw caps/kinds and nested replay | `TestOutcomeCapsAndRoots`; cap-1/cap padded positives, valid/malformed cap+1 bounds-first negatives and wrong kind/root/framing refusals. Whitespace inside replay fills the complete response cap without a separate nested raw limit. |
| Complete transitive closure | `TestOutcomeRecursiveClosure`, `TestOutcomeGrammarsAndRanges`; required deletion, null/type/unknown and all native/cleanup alternatives, output refs, stream/replay/error/source objects, IDs/digests/names/revisions/ref lengths and numeric boundary neighbors. |
| Exact response conditionals | `TestOutcomeResponseConditionals`; all five outcomes and twenty-two individually violated then-properties. Refused/conflict snapshot and consumption ack remain nullable; correlation and operation combinations are structural data, not policy. |
| Complete replay data | `TestOutcomeNestedReplay`; all four keys, four classifications and eight independent hash-null masks, including contradictory/equal/different hash examples. Nested kind/fields are validated; no actual replay or comparison is performed. |
| Independent result/consumer axes | `TestOutcomeIndependentDataAxes`; all native/cleanup/worker-output choices with duplicate output refs and truncated streams, all conflict categories with equal hashes, writer/identity roles and signal spelling/boolean controls. Native running/unknown never becomes a terminal or consume capability. |
| Required nullable values and arrays | `TestOutcomeArraysAndNulls`; five independent result nulls, argv/outputs0/1/128vs129, malformed first/last elements, ref closure, null versus empty argv, literal/escaped Unicode and the separate4096 decoded-byte cap. |
| Shared errors and strict entry/refusal/privacy | `TestOutcomeErrorRoutes` checks both new error-bearing wrapper routes while reusing the unchanged complete lifecycle stage/code matrix. `TestOutcomeStrictEntryRefusals` covers every new API's encoding/decoded-duplicate/numeric/static-nil route. Conflict has no numeric field, so lexical-priority probes use an unknown member; no valid property is invented. |

These are inert source cases until ordinary hosted PR CI executes them. No local
Go/parser/schema/checker/mutation/native execution was used during authorship.
A structural result may contain nonterminal native status, noncomplete output,
truncated streams or independent cleanup data. Replay hashes need not agree;
conflict hashes may be equal. None establishes evidence truth, result import,
consumption/receipt authority, replay semantics, transition/store/supervisor work,
a supported native tuple or a usable executor.


## Untrusted binding and operation record decoding coverage

`internal/memberwire/bindings_test.go` adds source regressions for four pure
structural APIs: assignment (64 KiB), attempt, operation intent and operation
completion (16 KiB each). All original inert fixture bytes remain unchanged.
Literal raw length/digest anchors precede decoding all assignment/attempt and
operation publications, including changed assignments and conflict data.

The cases cover closed required/null/type/unknown members, full artifact/ref/
effects/deadline closure, ID/digest/name and numeric boundaries, array caps and
invalid first/last elements. Artifact duplicates and all seven artifact roles
are structurally valid; effects names must be unique after escape decoding.
Both network alternatives have nearby valid controls and each of their three
conditional properties is independently violated. Both completion dispositions
have a valid control and the opposite conflict-null shape must refuse.

Role/producer and ordinal/predecessor combinations, equal IDs and repeated
artifact references remain untrusted data, without manifest or history policy.
Frozen independent predecode raw/value/hash/length expectations check caller,
returned Raw, deep maps and existing artifact/effects-name array mutations.
Exact padded raw caps, malformed oversize priority, strict lexical/static nil
refusals and unchanged caller bytes cover every entry; shared depth/node/string
and request predicates retain their existing regression coverage.

These are source tests for ordinary hosted PR CI, not executed schema-engine,
mutation, native or qualification evidence. Success neither admits an assignment
nor selects an attempt, compares records, publishes an intent, resolves an
interlock or performs a journal/store/consumer effect. No executor is provided.
