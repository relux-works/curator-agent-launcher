# Reliable inbox: native launcher and carrier composition

**Status: PROPOSED source contract, revision 1.** This candidate amends the
boundaries identified below; it neither changes the implemented protocol nor
qualifies a runtime. The proposed record and method names are review vocabulary,
not shipped APIs, capability tokens, CLI commands or reserved wire constants.
Launcher, session-host, a2a and carrier owners must agree their publication
before implementation. All current refusals remain in force.

The target is durable mail exposed to the intended existing native session,
without terminal typing, a human Enter, an empty polling turn or a second
provider owner. Native acceptance, actual inbox intake and application results
are different evidence. Reliability is conditional on supported native input,
ownership, connectivity, scheduling and human-state gates; it is not a promise
of exactly-once reasoning or arbitrary external effects.

## 1. Existing source and the amendment boundary

The following inventory is source inspection at launcher commit
`8ab9cb30735a159589649d32c6544f3b99ebec3a`, not installed-host evidence.

| Existing source | Actual boundary; missing composition |
| --- | --- |
| [`resolveHost`](../cmd/curator-run/host_selection.go) | Explicit native selection returns before registry access. Hosted selection resolves the registered executable, resume intent, pending recovery and `describe`, then calls `client.Required`. It does not select a qualified native wake transport. |
| [`Required` / `RequiredForRequest`](../internal/sessionmanager/client/capabilities.go) | Per-runtime launch/resume, lease, attach and doorbell capabilities; the latter derives semantic needs from validated request bytes. Generic tokens do not attest an exact build/endpoint/owner. Muse doorbell requests refuse. |
| [`delegatedRequest` / `runDelegated`](../cmd/curator-run/delegated.go), [`prepareWerkstatt`](../cmd/curator-run/werkstatt_unix.go) | Project resolved policy into launch/resume requests. `doorbell` carries identity, heartbeat and hourly limit. Claude's transitional hook installation precedes `StartLeased`; Codex's native binding belongs to the manager. Neither request contains the full composition expectation proposed here. |
| [`ValidateRequest` / `DecodeResponse`](../internal/sessionmanager/client/protocol.go), [`schemas/session-manager`](../schemas/session-manager/README.md) | Closed request/response validation. `describe` has version, capabilities and runtime operation lists; session records have native ID and four doorbell fields. There is no composition request, exact native-route manifest or structured delivery health record. Unknown capability strings confer no authority. |
| [`Client.Start`](../internal/sessionmanager/client/transport_unix.go) | Plain launch/resume submission with bounded operation reconciliation; this is not a wake/turn API. `Terminal` carries attach/exact transport, not automated notice input. |
| [`StartLeased` / `SubmitLeased` / `runLeased`](../internal/sessionmanager/client/lease_journal_unix.go), [`Reconciler`](../internal/sessionmanager/client/reconcile_unix.go) | Freeze launch correlation and exact manager registration, persist before acquire/submission, recover through the original manager and operation. These journals are not native-wake attempt journals and cannot prove a lost wake absent. |
| [`WriteDoorbell`](../internal/sessionmanager/client/doorbell.go), [`runManagerObservation`](../cmd/curator-run/observe.go) | Render validated manager evidence, separate from presence. Observation v1 preserves registration spelling and record nulls; it supplies neither process-incarnation proof nor a native input endpoint. |
| [FRONT-DOOR §8](../FRONT-DOOR.md#8-a2a-doorbells-for-hosted-sessions) | Claude uses hooks/cooperative background wait; Codex requires a per-session app-server owner, supervised binding and durable rebind/retirement uncertainty. Historical version ranges are not a qualification of another installed build. |

The persistent attachment implementation belongs to the session-host source
owner. Its identified extraction seams are `tools/board-cli/cmd/tb-sessiond`,
`tools/board-cli/internal/sessionmanager` and `sessionplugin/musebuilds` in
skill-project-management. That source location is not evidence of a landed
public session-host input endpoint. No generic host endpoint is supplied by the
launcher today. This candidate does not add a host to task-board, duplicate a
provider composer or write agents-management.

Proposed changes to the existing boundaries, effective only after agreement:

| FRONT-DOOR section | Proposed amendment |
| --- | --- |
| §5.2 operations/capabilities | Negotiate a separately versioned native-composition requirement, per exact runtime/build/platform/launch mode. Existing `doorbell-claude/1`, `doorbell-codex/1`, launch/attach and heartbeat alone do not satisfy it. Add no implicit operations to `curator-session-manager/1`. |
| §5.3 launch/resume | Carry a frozen caller requirement through a jointly published successor request contract. The existing closed launch/resume members remain unchanged until then. Do not put the requirement in labels, `native_tail`, `child_env` or a raw endpoint environment variable. |
| §5.4 records | A versioned composition result and health projection supplement, rather than overload, `doorbell.state` and `uncertain`. Observation v1 and its home-lease limitations remain intact. |
| §6.4 context | `continue` delivery targets the same native conversation. Notification does not recycle/resume a live thread, fork a replacement or change profile. Restart is an owner-admitted lifecycle action for the exact eligible saved conversation, not a timer reaction. |
| §8.1 Claude | Replace reliance on a model-maintained background wait for target idle reliability with an independently supervised, session-bound qualified native channel/extension. Keep transitional hook containment and legacy receipt semantics until a compatible replacement is accepted. |
| §8.2 Codex | Compose the controller with the host's one app-server/thread owner. Strengthen exact binding, conditional safe admission, dynamic health and both-ledger reconciliation; retain the existing uncertainty fence and no-terminal rule. |
| §8 Muse | Add a native route only after the existing Muse adapter work and exact compatibility evidence. An advertised token cannot remove the current refusal or imply idle intake. |
| §14 delivery | Separate accepted source interfaces, dependency publication, admitted implementation, fixture checks, native qualification and host adoption. All three runtimes are required overall; a fake-manager pass or one working runtime cannot complete the target. |

## 2. Custody and authority

One host owns one native session's process, thread/app-server attachment or
native channel, interactive UI coexistence and lifecycle. A long-lived service
may supervise those owners, but must not impersonate every session's native
RPC owner. The a2a delivery controller owns mail obligations, scheduling and
attempt reconciliation under exactly one active binding generation. Its lock
and durable state prevent a second consumer; hooks use that same arbitration.
The identity carrier/mailbox plane remains separate from native-session custody.

| Source owner | Owned seam and required agreement |
| --- | --- |
| Launcher | Caller requirement, selected registered-manager incarnation, request-derived capability fence, response comparison, redacted status and lifecycle integration. |
| Session-host | Exact native transport, connection identity, per-session owner, safe-state admission, lifecycle stream, durable native attempt evidence, detach/restart/retire. Persistent attachment is a narrow existing dependency, not a requirement to finish unrelated host extraction. |
| a2a source/integrator | Mailbox cursor, capability naming and adapter translation, single controller, durable obligations, throttle/budget, attempt reconciliation, hook arbitration, opt-in intake journal and retained payload migration. |
| Carrier integration owner | Compose the accepted launcher/host/carrier interfaces without introducing a second mailbox consumer or provider composer. |
| Curator/source authority | Accept public host contracts and dependency publication; retain profile, home, permission and source landing custody. |
| Existing host/session owner | Later serial adoption, supervision, eligible active set and rollback. Source agreement grants no live-host action. |

The local endpoint must authenticate the selected host and bind the calling
controller to the exact session/generation. `A2A_AGENT`, cwd, a PID, display
name, labels or a copied binding token are selectors/data, not authority.
Same-user source trust is not an isolation boundary against an arbitrary
malicious process already controlling that user's account. No peer message can
change gates, register a replacement, approve a tool or grant restart authority.

## 3. Versioned semantic packet for source agreement

Call this candidate **composition/1** for discussion. This is a logical typed
packet, not a JSON Schema or an implemented protocol revision. Each record is
closed: all named members are required; `null` is permitted only where stated.
Unknown versions, fields, enum alternatives, omitted required members and type
changes fail before new effects. A later wire publication must specify exact
encoding/canonical digest bytes, transport authentication, code/exit registry,
limits and compatible readers; it may not silently choose defaults here.

Proposed scalar bounds: `Digest` is `sha256:` plus 64 lowercase hex; `Ref` is
an opaque nonempty local evidence reference of at most 256 UTF-8 bytes; `Token`
is printable ASCII, 1–128 bytes, no controls. Arrays below are bounded to 64
members unless a narrower bound is stated; duplicate set members refuse.
Opaque native identities/endpoints are owner-published typed values, never
silently truncated into the current session-record grammar. Whole packet limit
is proposed at 64 KiB including nested records. Source owners must settle any
incompatible native identity bound before publication, not guess an adapter.
No secret, raw argv, mail body or arbitrary plugin error enters this packet.

### 3.1 Frozen caller request

| Record | Required typed members |
| --- | --- |
| `ManagerIncarnation` | `id`, canonical `executable`, exact `registered_at` string from the registry. Compare byte for byte; never round its timestamp. |
| `PolicyExpectation` | Canonical runtime (`claude_code`, `codex_cli`, `muse`); profile name, lock digest, fragment digest; effective home reference; resolved model/effort (nullable only as already admitted native defaults), permission and sandbox policy references; workspace reference. Freeze resolved values, not caller spelling alone. |
| `CompositionRequirement` | `version=composition/1`, unique `request_id:Token`, `manager:ManagerIncarnation`, `policy:PolicyExpectation`, `identity` under the existing a2a grammar, `required_route:Token`, `required_tuple:NativeTuple`, `manifest_digest:Digest`, accepted qualification `Ref`, `launch_mode:Token`, `expected_native:Ref|null`, `previous_binding:BindingKey|null`, `restart_policy` (`eligible-saved-session` or `never`), `limits:DeliveryPolicy`, `requirement_digest:Digest`. |
| `DeliveryPolicy` | Shared minimum interval (at least 30s); explicit hourly limit (1–60, preserving any stricter existing limit); one outstanding logical wake; batch size at most 50; explicit owner gate reference. Existing six/hour callers are not silently raised to a recommended deployment budget. |

The caller freezes the full requirement before handoff and retains it outside
the response. `required_route` names an agreed capability, not a string that
the host is free to choose. Read-only discovery may identify a candidate row,
but the caller must match it against independently accepted source/qualification
evidence and freeze its exact tuple and manifest before effectful handoff.
The manifest and returned tuple cannot define their own expected values. No
route is assumed when it is missing. For resume,
`expected_native` is the saved exact identity. For a new session it is null
only during creation; the host must establish native identity from its owned
connection and launch correlation. The caller then freezes that identity before
binding registration. A changed identity is not an acceptable response echo.

Home and policy references identify already admitted non-secret inputs; this
packet neither carries credentials nor resolves a new profile or permission.
For new launch, runtime build must match the exact admitted capability row before
session creation; unknown build is unsupported. Later binding checks precede
registration/submission. A launch operation ID is not a wake attempt ID.

### 3.2 Capability, binding and composition result

| Record | Required typed members |
| --- | --- |
| `NativeTuple` | `runtime`, exact `build`, binary/source evidence `Ref`, `platform`, `architecture`, `launch_mode`, native `protocol_revision`, native protocol/schema `fingerprint:Digest`, adapter source/build `Ref`. No version range inferred from a nearby tested build. |
| `OwnerIncarnation` | Host instance `Ref`, boot identity `Ref`, PID (positive integer), process-birth identity `Ref`, owner implementation/build `Ref`. PID alone never establishes sameness, death or stop authority. |
| `EndpointIdentity` | Transport kind agreed by its source owner; opaque endpoint `Ref`; endpoint object incarnation `Ref`; authenticated host/connection `Ref`. Not an arbitrary caller-provided socket path. |
| `CapabilityManifest` | `version`, `tuple:NativeTuple`, `route:Token`, supported native identity/endpoint type revisions, safe admission modes, lifecycle observation support, attempt lookup/dedup semantics and history completeness, detach and same-session restart support, human-state preservation constraints, hook limitations, exact qualification evidence `Ref`, `manifest_digest:Digest`. |
| `BindingKey` | Binding `Ref`, opaque generation `Token`. This generation is distinct from a manager session's numeric generation; no implicit conversion. |
| `HostBindingCandidate` | `key:BindingKey`, `manager`, manager session handle and numeric generation, a2a `identity`, exact native identity `Ref`, `owner:OwnerIncarnation`, `endpoint:EndpointIdentity`, `tuple:NativeTuple`, `manifest_digest`, `requirement_digest`, policy and restart references. Host-proven input to registration; it is not an armed binding. |
| `Binding` | Complete `HostBindingCandidate` plus exclusive controller lease `Ref`, atomically settled by registration. No null or absent lease is accepted as bound/ready or for submission. |
| `CompositionResult` | `version`, `request_id`, `requirement_digest`, tagged `outcome`: `bound` with complete `binding`, `manifest`, initial `health`; `refused` with closed reason and `effect=none`; or `unresolved` with durable operation reference and known completed substeps. Branch members cannot mix. |

`bound` requires independent local comparison against the retained caller
requirement, authoritative native connection evidence and an accepted exact
qualification row. A manifest's claim about its own qualification is not proof.
The launcher compares identity, manager registration, policy, route, tuple and
resume identity; the host verifies live owner/boot/endpoint/native identity;
a2a verifies its lease/generation. Tampered/dropped/defaulted values fail, even
when a response echoes the same substituted label. A manifest digest pins bytes,
not truth or ongoing health. Missing evidence stays unknown/unavailable.

Proposed refusal reasons for source agreement are `unsupported_contract`,
`unsupported_tuple`, `missing_capability`, `expectation_mismatch`,
`stale_binding`, `owner_unavailable`, `gate_closed`, `unsafe_state`,
`rate_held`, `budget_held` and `previous_attempt_unresolved`. These are not new
launcher diagnostic registry entries. Refusal means no newly admitted effect;
failure after possible I/O must instead return/preserve `unresolved`. Existing
launch/home-lease refusals keep their existing meanings and protection.

### 3.3 Live observations and health

`SessionObservation` contains the binding key and full identity digest,
boot-scoped monotonic revision, observed native lifecycle, activity, human-state
gate, evidence reference and completeness/freshness. Lifecycle is
`starting|running|stopping|down|unknown`; activity is `idle|busy|unknown`;
human state is `clear|approval|waiting-user|draft|paused|unknown`. These are
independent axes. Missing, gapped or stale observation cannot become idle/down.

`DeliveryHealth` contains version, binding identity digest and generation,
tuple/manifest reference, controller lease/liveness, ingress connectivity,
observation revision/freshness, delivery mode, gate/reason, host and a2a
uncertainty flags, pending/unconfirmed counts, oldest pending age, last durable
mail, last native acceptance, last inbox print, last intake/disposition and
outbound mirror evidence, last application-result reference, throttle/budget
reservations and next eligibility time. Absent evidence is explicit null, never
a synthesized success timestamp. Detailed endpoint/identity records remain
local; ordinary summaries redact them rather than leak paths or peer bodies.

Proposed modes: `native-qualified`, `legacy/degraded`, `unsupported`, `unknown`.
`native-qualified` names installed tuple eligibility, not present readiness.
Ready additionally requires current authenticated ownership and controller,
fresh complete safe-state evidence, open gate and no overdue unresolved delivery.
Busy pending, explicit human/rate/budget holds and uncertainty have their own
visible status, never an idle-ready row. A live PID, heartbeat, hook count,
`armed` token or once-passing startup check is insufficient. Health decays when
evidence/progress expires. A separately supervised incident route must work even
when this recipient cannot report its own failure.

## 4. End-to-end flow and missing wire seam

1. Caller resolves ordinary launch policy and exact registered manager as today.
   For the proposed native delivery mode it retains a `CompositionRequirement`
   and negotiates its version/route/tuple before hooks, acquire or a new launch.
   Unsupported consumers refuse this new mode before those effects. Unenrolled
   legacy launches retain their current contracts; there is no silent downgrade
   after a new-mode refusal.
2. A jointly published successor to the closed launch/resume contract must carry
   that requirement through selection, client validation and the original
   operation journal. `RequiredForRequest` would derive needs from those bytes,
   not a parallel mutable flag. The existing lease acquire/recovery ordering and
   exact manager-incarnation comparison must survive. This wire seam is missing
   today; adding an ignored label does not fill it.
3. The session-host owns the final typed native connection/start/resume, verifies
   the tuple, preserves native policy and interactive UI, and obtains the actual
   native identity. No second exec/resume may take a live thread. An existing
   resume must match the caller's frozen identity. Creation that becomes
   uncertain remains in the original lifecycle journal, not reported no-effect.
4. After independently checking the established binding, the host and a2a use
   an agreed registration seam. a2a acquires its one-controller lease, loads
   retained obligations and reconciles previous attempts. An incomplete
   registration is not a ready composition. No second Matrix cursor or model
   turn is needed to supervise/rearm it.
5. The controller schedules fixed notices from durable mail, checks gates/rate
   state and uses the narrow host surface below. The host arbitrates against
   native state at admission; an earlier idle observation is not an atomic fence.
6. Result/status retain exact binding correlation and separate receipt stages.
   The lifecycle cursor reports gaps explicitly; gaps close admission until
   authoritative resynchronization, not until a timer guesses idle.
7. Gate closure, retirement and ownership transfer follow §6. The caller does
   not mark retired/rebound from a doorbell `off` line alone.

No new public RPC transport is selected here. Launcher request/schema ownership,
host transport ownership and a2a capability/adapter ownership must publish their
agreed finite interfaces. Attach/launch/describe/status cannot be overloaded into
an undocumented generic native RPC proxy.

## 5. Narrow host/controller interface proposal

Every request includes the agreed contract version, request correlation and
expected binding identity/generation. Authentication and identity comparison
precede effects; each response retains that correlation. The following names
describe semantics only. No raw vendor method, shell command, tool approval,
model/profile override, discovery scan or arbitrary text input is exposed.

| Proposed operation | Request and result obligation | Owner of effects |
| --- | --- | --- |
| `RegisterBinding` | Frozen requirement plus `HostBindingCandidate` and manifest. Return exclusive complete `Binding` or no-effect refusal/unresolved partial registration. An identical retry returns the same registration; changed bytes under the ID conflict. Prior uncertainty/active ownership fences replacement. | Host supplies native custody; a2a owns registration lease/ledger. |
| `ObserveSession` | Exact binding; return `SessionObservation` with revision, completeness and hold reason. Reading performs no wake, receipt, rebind or restart. | Host native observer. |
| `SubmitWake` | Immutable attempt ID, binding/digest, obligation reference, exact fixed-notice bytes/digest, allowed delivery mode, expected observation/gate revision and rate reservation. Atomically accept under safe state or reject before native submit; lost/ambiguous result is uncertain. | Host admission and typed native adapter; a2a scheduling. |
| `ObserveAttempt` | Exact binding, attempt ID and payload digest. Return accepted native turn/item/command references, proven no-submit, still-running, or unresolved with completeness evidence. Never infer absence from a truncated history or a missing RPC reply. | Host plus a2a's original durable attempt evidence. |
| `SubscribeLifecycle` | Binding plus boot-scoped cursor; ordered observations and next cursor, or explicit gap/restart. No model polling turns; bounded reconnect belongs to the supervisor. | Host emits lifecycle; a2a consumes it. |
| `SetGate` | Exact binding, expected gate revision, owner-authorized transition and evidence reference. Gate change and host submit admission serialize; no peer-text authority. Return new revision and already admitted attempt references. | Host and a2a compose a fence before acknowledging closure. |
| `RetireBinding` | Exact binding, owner evidence/reason and original deadline. Return retiring substep, blocked/unresolved, or fully retired only after both ledgers settle and owner release/death is proven. No new-generation assignment as a side effect. | Host lifecycle owner plus a2a lease/attempt owner. |

The fixed notice has a versioned host-controlled template that asks the bound
session to read inbox at its next permitted boundary. Only an opaque obligation
or intake-recovery reference may vary. Peer title/body/claimed urgency never
enters it. Ordinary content arrives as framed untrusted tool data through inbox;
a notice conveys no new authority. A safe native external-data queue is allowed
only when its exact tuple guarantees preservation of current work/human input.
If neither conditional native admission nor equivalent host arbitration can
preserve a concurrent draft/approval change, the row is unsupported for that
state; observing idle before an RPC does not repair this race.

## 6. Durable attempts, gates and recovery

Before any possible native I/O, a2a persists the obligation/event references,
binding key and full identity digest, immutable attempt ID and notice bytes/digest,
adapter/manifest references, rate reservation and current gate revision. The
host persists its correlated admission/possible-submission record before its
native I/O. Registration/attempt ID reuse with different bytes is a conflict,
never a new attempt. A2a's and the host's journals reference each other; a crash
between their writes requires reconciliation, not an unjournaled submit.

An attempt progresses from prepared to rejected-before-submit, accepted or
uncertain. Accepted retains native evidence but does not clear unread/intake
obligations. Uncertain requires authoritative observation of the SAME attempt.
When an accepted native call loses its response, the transport timeout does not
permit a fresh attempt. A JSON-RPC request ID is not vendor deduplication.
Incomplete/unavailable history cannot establish no-submit. Without durable
native deduplication or authoritative lookup, automatic retry and no-duplicate
effects cannot both be promised: retain uncertainty and the named upstream gap.

Gates serialize against admission. Closing prevents any subsequently admitted
submit; previously admitted work is listed and reconciled, not retrospectively
called prevented. Pause keeps pending mail and budgets. Human approval, waiting
for the user, drafts, stopping and unknown state cannot be overridden by urgency
or by an hourly/heartbeat timer. Hooks and native dispatch acquire the same
logical obligation/attempt reservation, including after a controller crash.

Retirement checkpoints its gate, original deadline and completed substeps:
close/fence; observe/reconcile both ledgers; stop or cooperatively release the exact
native owner through its admitted lifecycle; verify incarnation/death or release;
retire the controller lease. A deadline yields a visible blocked/retiring state,
not an implicit success, uncertain reset or escalation. Existing stop authority
is not expanded. `off`/abandon, receipt, process exit or clock expiry alone never
clears possible prior native submission. An already authorized stop may proceed
while retaining uncertainty, as the existing contract permits, but cannot be
reported as completed binding retirement or permit transfer. No PID-reuse kill
is permitted.

Generation transfer requires both a2a and host uncertainty resolved and old
ownership released/proven gone. A stale hook cannot mutate a successor. A
material endpoint/owner/boot replacement requires explicit revalidation and a
new generation, never rebinding through an old endpoint string. Supervisor
recovery restores only the persisted eligible active set under the exact saved
native identity/profile/model/permissions. Parked/retired sessions remain parked;
socket failure does not prove death. Reboot invalidates old owner incarnations;
preserve rate reservations and intake obligations, and reconstruct scheduling
times without resetting the budget or replaying old task prompts.

## 7. Runtime-specific projection, still unqualified here

| Runtime | Proposed final native seam | Required evidence and preserved boundary |
| --- | --- | --- |
| Claude Code | Independently supervised session-bound native channel or qualified native background extension. | Exact build/account/policy/launch-mode support and durable rearm while fully idle/detached. Cooperative `a2a wait`/ordinary hooks alone do not establish that. Generic MCP notification, headless SDK streaming or a one-shot background completion is not proof of attachment to the existing interactive CLI. No trust/development flag bypass. |
| Codex CLI | Existing a2a native adapter composed through the host-owned app-server and same thread; fixed notice as native external tool output where supported. | Exact native protocol/build qualification, one per-session attachment owner, CLI UI attached to that same thread, safe admission and same-attempt lookup/completeness. Candidate `turn/start`/tool-output input does not qualify any installed build, and item injection alone is not proof of a turn or intake. No steer/user-instruction rewrite. |
| Muse Code | Source-owned native MSP or native session-messaging adapter after the existing Muse offline/native work's actual evidence. | Exact receiver/session, command identity, nonempty accepted notice, busy/human-state disposition, UI coexistence, detached idle wake and retention/idempotency limits. RPC request ID is not an MSP command's deduplication guarantee. Display/notification/queue acceptance without context entry is incomplete. Headless exec/reviewer success is not interactive delivery. |

All three keep native session identity, CLI interaction, compaction, saved
conversation, model/effort and permission policy. No replacement SDK UI is an
unstated dependency. Muse hooks may provide nonblocking local opportunities;
they cannot block/answer approval or rewrite user input. If a qualified Muse
transport uses a native user-message slot, only the fixed host automation notice
is permitted, never peer bodies, and draft/human-state preservation must be
demonstrated. The current Muse doorbell refusal changes only after separately
reviewed compatible source and verified capability publication, not a token-only
special case. Candidate vendor documentation or old help/schema outputs supply
questions for qualification, not supported version rows. This unit runs no probe.

## 8. Receipts, intake, scheduling and health claims

| Evidence stage | What it establishes; what remains open |
| --- | --- |
| Durable mail | Bound recipient/event/digest persisted; no native submit or context claim. |
| Native acceptance | Exact attempt accepted by the intended native route; no inbox/intake or task completion claim. |
| Inbox print / ordinary receipt | Existing inbox printed selected content and receipts it; a repeated ordinary inbox stays empty. Not proof of native tool-result persistence, understanding or handling. |
| Durable intake/disposition | Opt-in batch journal and local disposition bind the original retained events to this generation; no automatic Matrix ACK or application-effect guarantee. |
| Application result | Separate task/effect evidence under its own authority and idempotency; messaging receipt cannot settle it. |
| Outbound mirror | Separate sanctioned owner-message leg and evidence. Inbound repair does not establish outbound delivery. |

The stronger intake extension belongs to a2a's own versioned schema, payload
retention and migration unit. It must journal batch membership before output,
retain immutable payload references (including exact truncation/body revision),
and permit bound replay after print/receipt but before context persistence or
confirmation. Ordinary inbox semantics remain unchanged. Successful local
disposition closes intake, not linked work; deferral retains its application
reference. Recovery may expose duplicates labeled by event ID and batch. Never
recover arbitrary peer-supplied IDs or clear incomplete intake from unread=0.
Capacity limits stop new cursor advancement rather than evict unresolved data;
bounded dedup/history horizons and initial sync limits remain explicit. A
mailbox cannot guarantee exactly-once arbitrary external effects after a crash.

One controller coalesces mail without postponing its oldest deadline. Proposed
initial scheduling bounds are 250ms coalescing, 1s maximum extension, batch 50,
one outstanding wake; shared 30s identity throttle and explicit 1–60/hour budget
with existing stricter values preserved. Reconnect may use bounded exponential
backoff (1/2/4/8 seconds, cap 30 with jitter) without a model turn. A proposed
5s native request timeout means uncertainty unless no-submit is proven. Progress
checks are controller observations, not model polling. A 30s progress threshold
and 60s unexplained stall incident are proposed; deliberate holds are shown, not
prodded. At most one recovery notice per unresolved batch requires resolved
attempt history; otherwise hold uncertain. No mail means zero mail-induced turns.

Measure arrival, durable ingestion, eligibility, native acceptance, inbox print
and intake separately. Eligibility is the latest of durable arrival, throttle,
budget availability and permitted lifecycle/gate boundary; show all waiting time.
Candidate healthy acceptance targets are p95 ≤5s/p99 ≤10s after eligibility;
arrival p95 ≤35s applies only without additional holds/contention, and intake
within 30s after acceptance requires a responsive native session at a safe
boundary. These are future measurement targets, not unconditional SLOs.
Report held/stalled counts and oldest ages separately without hiding them from
overall latency accounting. No synthetic readiness from hook count or heartbeat.

## 9. Manual contract counterexamples and future qualification

The following are **unexecuted source cases**, not fixture/native results. A
future harness freezes a caller-owned expectation **E** outside the adapter:
exact manager registration, policy, native identity, owner/boot/endpoint object,
binding generation, tuple/route, request and attempt digest, gate revision and
limits. Its oracle includes original draft/approval contents, owner/controller
counts, mailbox/intake bytes and submit/receipt/rebind effect counts. Mutating
an adapter echo never mutates E. A token or both sides agreeing is insufficient.

| Case | Negative or constrained verdict and preserved effects | Nearby valid control |
| --- | --- | --- |
| C01 idle/busy | Unknown state refuses new submit; busy retains pending unless exact safe external-data admission is qualified. Preserve active work; no user steering. | Qualified idle admits one fixed notice; qualified busy queue admits only its proven safe mode and later correlated intake. |
| C02 detached | Detached terminal with no live native owner is unavailable, not ready; no terminal reopen or second owner. | Detached viewer with live qualified owner/controller receives through the same native session. |
| C03 approval/wait-user/draft | Hold, or only an independently proven safe noninterrupting native queue; no answer, text edit or approval. An idle observation raced by a draft must not admit unsafe submission. | Same tuple in clear idle with current revision accepts one notice; preserved human state is compared byte for byte in the held case. |
| C04 old/absent capability | Old consumer, missing version/tuple/manifest or unknown route refuses before new-mode hook/acquire/start/bind/submit effects. | Agreed exact capability plus all frozen E fields crosses the fence. |
| C05 false Muse ready | Generic launch/attach/doorbell token or offline manifest cannot make native-ready, suppress current refusal or count intake. | Future Muse implementation with independently accepted exact tuple and fresh live binding evidence qualifies that route only. |
| C06 owner replacement | Stale generation, reused PID, changed birth/boot or same path with replaced endpoint object refuses submit/rebind; never kill the replacement. | Original exact incarnation and controller lease admits; an explicitly transferred reconciled successor requires its own E. |
| C07 wrong receiver | Wrong native thread/session, a2a recipient or manager registration refuses with no submit or receipt. A rounded registration timestamp also mismatches. | Exact saved native identity and byte-identical registration/recipient compare equal. |
| C08 lost requirement | Drop, default or tamper any binding/policy/route field, including matching changed echoes: reject before effects against E. | Unmodified full request and independent authoritative connection evidence satisfy E. |
| C09 hook/native race | Both paths cannot reserve independent wake attempts for one obligation. Preserve one controller, rate reservation and all mail. | Either path alone obtains the same journaled logical obligation and observes its correlated outcome. |
| C10 accepted/lost reply | Preserve original attempt as uncertain pending reconciliation; no new turn/retry/off-clear. Partial history/not-found without authoritative completeness is not no-submit. | Complete same-attempt evidence reconciles accepted, or authoritative no-submit permits a separately admitted attempt. |
| C11 print/context crash | Ordinary unread=0 cannot clear incomplete opt-in intake; retain original batch/payload and replay as untrusted data, no application-result claim. | Persisted tool result plus exact batch disposition records intake while linked application work stays separate. |
| C12 gate/retire race | Gate closure fences later admission; already admitted work remains visible. Deadline/abandon/off do not clear either ledger or transfer generation. | Both ledgers resolved plus proven owner release/death permits full retirement; a new generation requires explicit registration. |
| C13 reboot/recovery | Old boot/PID/endpoint or parked identity cannot restart/rebind; no task-prompt replay, profile change or budget reset. | Persisted eligible exact saved session restores once through owner lifecycle with revalidated tuple/new generation after reconciliation. |
| C14 throttle/budget | Exhaustion retains durable pending; no hook bypass, timer reset, extra turn or lost event. | After legitimate release one coalesced eligible notice uses the shared reservation, retaining oldest arrival time. |
| C15 no mail | Empty mailbox without intake recovery obligation causes zero mail-induced model turns throughout prolonged idle. | A later durable addressed event awakens the qualified route without human Enter, through all separate receipt stages. |
| C16 receipt/mirror | Native accepted, inbox printed, task handled and outbound mirrored cannot be substituted for each other; copied batch token cannot settle another generation. | Independently correlated exact event/intake/application/outbound evidence establishes only its corresponding leg. |
| C17 upgrade/rollback | Unknown build or unreadable new journal pauses native dispatch; never discard uncertainty to downgrade or call restored terminal route native-ready. | Owner retains readable state and returns to an accepted exact tuple; any admitted legacy recovery is labeled legacy/degraded. |

Every refusal case asserts its intended reason and unchanged protected state,
not merely any error. Future detector controls run only in separately admitted
isolated scratch copies; valid baseline and post-plant valid control must remain:

| Bounded future plant | Intended alarm from an independent expectation |
| --- | --- |
| P01 token alone sets ready | C04/C05 reject ready without full capability, qualified tuple and current custody; unchanged qualified baseline passes. |
| P02 ignore generation/endpoint incarnation | C06/C07 fail zero-submit/zero-rebind assertions against frozen E; original binding still passes. |
| P03 uncertain becomes retry/off-cleared | C10/C12 fail retained attempt and no-second-submit/no-transfer assertions; complete reconciliation control passes. |
| P04 terminal fallback on unsupported | C04/C05/C17 fail zero-terminal-bytes/zero-secondary-owner assertions; supported native control passes. |
| P05 print receipt marks handled | C11/C16 fail independent intake/application evidence assertions; actual disposition/result controls remain distinct. |

No plant, parser, fixture, native session or checker is executed by this docs
unit. Unknown native inputs stay explicit gaps rather than invented tuples.

## 10. Dependency publication, migration and eventual acceptance

1. Original source owners agree the packet, final names/encoding, native safe
   admission limits, exact dependency source refs and compatibility readers.
   A2a integrates its own capability/adapter and intake-retention units; the
   existing Muse work stays with its original owner. No duplicate producer.
2. Publish independently reviewed exact source interfaces/schema modules through
   their owning repositories before a separately admitted implementation. No
   guessed release tag, local replace, root-module/provider upgrade or arbitrary
   extra dependency is implied by this candidate.
3. Implement under a recorded code/test scope, then run isolated logic/fixture
   and detector controls, independent review and genuine green checks; land
   accepted signed heads through each source owner's custody. No doc acceptance
   stands in for an implementation or native input API.
4. Existing host owners choose noncritical eligible canaries and migrate serially
   per runtime. Keepers/control-plane leads supervise rather than become test
   subjects. The old consumer drains/retire-reconciles itself before a qualified
   replacement is armed; never act on another live session to make a test pass.
5. Qualify Claude, Codex and Muse separately on every enrolled actual
   OS/build/launch-mode tuple. Initial plan: at least 20 idle and 20 busy
   deliveries per tuple, deterministic crash/human-state cases, one-hour active
   soak and at least 12 hours untouched idle. Native event/generation/turn/tool
   references, timestamps and actual exits must support each claimed stage.
   Manual Enter invalidates that run's automatic-wake evidence. First serial
   canaries can precede the soak; full acceptance cannot.
6. Retain pending/uncertain/intake state on upgrade/rollback. Untouched legacy
   deployments stay with their admitted owner and route until qualified
   migration. Owner-admitted temporary legacy restoration is legacy/degraded,
   not target acceptance. Counterpart owners choose their own canaries; no
   remote disable of their legacy transport.

Report source/code, synthetic fixture, real native, installed host and unresolved
capability evidence in separate columns for every tuple. All three runtime
families are mandatory overall; undeployed tuples are not invented as tested.
An unsupported required row keeps overall delivery open. The source packet here
has **no runtime/native/host qualification evidence** and does not select a new
writer, accept sources, land defaults, install hooks or activate a session.

Open source-owner decisions are concrete: successor launcher carrier/wire
publication and refusal mapping; native identity/endpoint bounds and local
authentication; host safe admission and complete attempt reconciliation for each
tuple; lifecycle cursor retention; exact a2a capability/adapter and intake payload
retention migration; and host-owned serial qualification/adoption. Vendor API
availability, a method name or a document cannot resolve any of those gates.
