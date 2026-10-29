# Reliable inbox: native launcher and carrier composition

**Status: PROPOSED source contract, revision 2.** This candidate amends the
boundaries identified below; it neither changes the implemented protocol nor
qualifies a runtime. The proposed record and method names are review vocabulary,
not shipped APIs, capability tokens, CLI commands or reserved wire constants.
Launcher, session-host, a2a and carrier owners must agree their publication
before implementation. All current refusals remain in force.

Revision 2 composes detection, context delivery, watcher supervision and periodic
reconciliation; requires explicit-only thread eligibility and visible migration;
and makes actual session-host extraction a prerequisite to host-specific
implementation. These requirements supersede the earlier narrow in-tree host
sequence. Independent launcher/a2a contracts and non-host-dependent adapters may
continue. Neither this amendment nor supporting incident reports establish an
installed scheduler, working native capability or accepted host API.

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

The session-host owner reports prepared lifecycle/attachment/observation ports,
with compile/runtime closure and the final exported version still pending.
Prepared reference assets and a runtime-build metadata candidate are not an
accepted session-host input or supervision API. Host-specific reliable-inbox
implementation must wait for actual extraction completion and consume its
versioned exported API. It must not be added in-tree to task-board first.

Proposed changes to the existing boundaries, effective only after agreement:

| FRONT-DOOR section | Proposed amendment |
| --- | --- |
| §5.2 operations/capabilities | Negotiate a separately versioned native-composition requirement, per exact runtime/build/platform/launch mode, including four-responsibility coverage, independent supervision and the shared eligibility contract. Existing `doorbell-claude/1`, `doorbell-codex/1`, launch/attach and heartbeat alone do not satisfy it. Add no implicit operations to `curator-session-manager/1`. |
| §5.3 launch/resume | Carry frozen caller policy, eligibility and supervision requirements through a jointly published successor request contract. The existing closed launch/resume members remain unchanged until then. Do not put requirements in labels, `native_tail`, `child_env` or a raw endpoint environment variable. |
| §5.4 records | A versioned composition result and health projection supplement, rather than overload, `doorbell.state` and `uncertain`; expose watcher/transport/sweep health, eligibility revision and causal rescue accounting separately. Observation v1 and its home-lease limitations remain intact. |
| §6.4 context | `continue` delivery targets the same native conversation. Notification does not recycle/resume a live thread, fork a replacement or change profile. Restart is an owner-admitted lifecycle action for the exact eligible saved conversation, not a timer reaction. |
| §8.1 Claude | Replace reliance on a model-maintained background wait for target idle reliability with an independently supervised, session-bound qualified native channel/extension. Keep transitional hook containment and legacy receipt semantics until a compatible replacement is accepted. |
| §8.2 Codex | Compose the controller with the host's one app-server/thread owner. Strengthen exact binding, conditional safe admission, dynamic health and both-ledger reconciliation; retain the existing uncertainty fence and no-terminal rule. |
| §8 Muse | Admit native Monitor/background events as first-class candidates, composed with independent supervision rather than required to survive their own death. Add a native route only after the existing Muse adapter work and exact compatibility evidence. Keep hooks nonblocking; an advertised token cannot remove the current refusal or imply idle intake. |
| §14 delivery | Separate accepted source interfaces, dependency publication, actual host extraction before host-specific implementation, admitted implementation, fixture checks, native qualification and host adoption. Require measured fallback retirement and shared explicit-only eligibility/migration. All three runtimes are required overall; a fake-manager pass or one working runtime cannot complete the target. |

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
| Session-host | Exact native transport, connection identity, per-session owner, safe-state admission, lifecycle stream, durable native attempt evidence, detach/restart/retire, and its watcher/scheduler lifecycle ports. Host-specific implementation follows actual extraction and accepted exported API publication; contracts can proceed now. |
| a2a source/integrator | Sole shared classifier/subscription-migration, mailbox cursor, capability naming and adapter translation, single obligation arbiter/dispatch authority, throttle/budget, attempts, hook/Monitor/sweep arbitration, intake journal and retained payload migration. |
| Carrier integration owner | Project the accepted requirement, eligibility and supervision plans through the carrier and compose the extracted host interface; no second classifier, mailbox consumer or provider composer. |
| Curator/source authority | Accept public host contracts and dependency publication; retain profile, home, permission and source landing custody. |
| Existing host/session owner | Later serial adoption, supervision, eligible active set and rollback. Source agreement grants no live-host action. |

The local endpoint must authenticate the selected host and bind the calling
controller to the exact session/generation. `A2A_AGENT`, cwd, a PID, display
name, labels or a copied binding token are selectors/data, not authority.
Same-user source trust is not an isolation boundary against an arbitrary
malicious process already controlling that user's account. No peer message can
change gates, register a replacement, approve a tool or grant restart authority.

The a2a controller (Package E) reuses the source-owned issue107/PR111
`detachedConsumerEnv` allowlist, not a second environment filter. Its inspected
source candidate passes process basics, locale, proxy/CA settings and non-secret
identity/routing selectors; config root and session binding are explicit
controlled inputs. It must not wholesale inherit interactive/session messaging
or registration secrets. Selectors are not authority. Any native adapter
transport credential is separately scoped to its owning native session through
the original source custodian, never a shared broad environment. Capability
agreement must retain this dependency; it introduces no new public packet field
or credential mechanism and does not claim PR111 is landed or installed.

### 2.1 Four responsibilities, one arbiter

| Responsibility | Proposed composed mechanism and evidence | Forbidden substitution |
| --- | --- | --- |
| Event detection | Native Monitor, supported event subscription/long-poll, hook opportunity or binding consumer signals a stable durable event/intake reference and source progress. | Receipt unseen mail, infer consent from participation, or treat a live PID as a healthy upstream connection. |
| Native context delivery | Qualified native binding, channel, Monitor event or background completion supplies the fixed notice to the exact session; subsequent framed inbox intake is separate evidence. | Terminal input, approval/draft consumption, a second provider owner, or display-only output labeled context entry. |
| Watcher supervision | An independently scheduled parent/service detects exit, lease expiry, missed health or stalled transport and requests authorized repair through the lifecycle owner. | Require a dead watcher to report or restart itself, or let the watched event stream be its only supervisor. |
| Periodic reconciliation | A deterministic pre-model sweep compares bounded non-receipting metadata, attempts and intake journals, then upserts missing obligations. | Call ordinary inbox for the model, generate an empty report turn, or own a second dispatch queue/cursor. |

One mechanism may cover several rows; all four responsibilities must be covered
for every qualified Claude/Codex/Muse tuple. Hooks, Monitor, native bind and
scheduled sweeps submit signals to the SAME stable obligation arbiter and ONE
dispatch authority. Redundant observation is allowed; parallel native owners or
independent retry loops are not. No source-owned state is discarded because
another mechanism already rang.

### 2.2 Deployment responsibility and evidence register

The table uses public deployment aliases, not private host identifiers. Each
alias represents a separate deployment in the private owner inventory; it is
not evidence that its listed components are installed. Additional enrolled hosts
need their own row. No live inventory was inspected in this source unit.

| Deployment | Confirmed custody/source facts | Proposed responsibility allocation | Unconfirmed operational facts; required owner return |
| --- | --- | --- | --- |
| L — launcher-side deployment | Launcher source has selection/client/status seams, not a general native input API; launcher source owner retains them. Session-host extraction owner retains native custody. | Launcher carries the frozen plan; extracted host supplies native owner, watcher-parent and scheduler integration; carrier owner composes their ports; a2a owns arbiter/classifier. Deployment owner selects the independent service manager/scheduler. | No accepted exported lifecycle endpoint/version, installed supervisor/service name, schedule registration, failure-domain proof or local capability tuple is established. Host source owner returns extraction/API publication; deployment owner returns actual owner/component/parent/scheduler identities and evidence before adoption. |
| A — agent-side deployment | The supporting source author reports an interim Muse Monitor and incident observations; these are attributed reports, not producer-reproduced qualification. Existing a2a/Muse and extraction custodians retain their work. | Same four responsibilities and one arbiter; native Monitor may supply detection/context while an independent host parent supplies supervision and deterministic reconciliation. | Reported interim behavior does not establish complete supervision, native idle/busy parity, pre-model gating or a qualified installed scheduler. Existing deployment owner must identify actual supervisor/scheduler endpoints and failure domains; source owners supply the agreed contracts. |
| C — counterpart deployment | Existing counterpart operator retains admission, legacy delivery and canary choice; no remote disable or takeover is authorized. | Its own host owner maps the same source contract to its actual lifecycle/scheduler; no shared remote daemon gains native custody. | Exact operational owner allocation, components, native tuple, timer independence and rescue instrumentation remain unconfirmed here. Counterpart owner supplies its own per-host row and adoption evidence; silence is not agreement. |

These are bounded owner returns, not blockers to this docs amendment or grants
to discover/install services. Launcher and extracted host sources specify the
supervisor/scheduler contract; the carrier owner preserves it end to end; each
deployment owner names and operates its actual service/parent. Source-owner role
assignment is not an invented concrete operational service. A schedule inside
the watched process alone fails independence unless a separate parent restarts
that process. Machine-wide outage remains a shared failure domain requiring
external reporting, not a local self-repair promise.

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
| `CompositionRequirement` | `version=composition/1`, unique `request_id:Token`, `manager:ManagerIncarnation`, `policy:PolicyExpectation`, `identity` under the existing a2a grammar, `required_route:Token`, `required_tuple:NativeTuple`, `manifest_digest:Digest`, accepted qualification `Ref`, `launch_mode:Token`, `expected_native:Ref|null`, `previous_binding:BindingKey|null`, `restart_policy` (`eligible-saved-session` or `never`), `limits:DeliveryPolicy`, `supervision:SupervisionRequirement`, `eligibility:EligibilityRequirement`, `requirement_digest:Digest`. |
| `DeliveryPolicy` | Shared minimum interval (at least 30s); explicit hourly limit (1–60, preserving any stricter existing limit); one outstanding logical wake; batch size at most 50; explicit owner gate reference. Existing six/hour callers are not silently raised to a recommended deployment budget. |
| `SupervisionRequirement` | Versioned plan reference/digest; all four responsibility allocations; independent parent/scheduler owner and failure-domain evidence references; health tick, full reconciliation interval, total sweep timeout and scheduling-lateness bound; bounded non-receipting metadata capability reference; rescue-instrumentation and retirement-policy references. Missing concrete allocation is unresolved, not a default service name. |
| `EligibilityRequirement` | Source-owned shared eligibility contract/policy revision and digest; explicit-only thread consent rule; canonical room/thread identity contract; explicit-follow provenance and scope contract; effective mute policy; migration checkpoint/revision and legacy-tail policy references. Direct/mention/pinned-owner rules retain their existing authority. No invented classifier RPC name. |

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
The supervision and eligibility requirements are included in the retained
request/digest, not response-only labels. A source-owned capability must make
the shared eligibility and bounded non-receipting metadata available; these
publication seams are pending. No launcher-local classifier or receipting
inbox fallback may compensate for their absence.

### 3.2 Capability, binding and composition result

| Record | Required typed members |
| --- | --- |
| `NativeTuple` | `runtime`, exact `build`, binary/source evidence `Ref`, `platform`, `architecture`, `launch_mode`, native `protocol_revision`, native protocol/schema `fingerprint:Digest`, adapter source/build `Ref`. No version range inferred from a nearby tested build. |
| `OwnerIncarnation` | Host instance `Ref`, boot identity `Ref`, PID (positive integer), process-birth identity `Ref`, owner implementation/build `Ref`. PID alone never establishes sameness, death or stop authority. |
| `EndpointIdentity` | Transport kind agreed by its source owner; opaque endpoint `Ref`; endpoint object incarnation `Ref`; authenticated host/connection `Ref`. Not an arbitrary caller-provided socket path. |
| `CapabilityManifest` | `version`, `tuple:NativeTuple`, `route:Token`, supported native identity/endpoint type revisions, safe admission modes, lifecycle observation support, attempt lookup/dedup semantics and history completeness, detach and same-session restart support, human-state preservation constraints, hook limitations, four-responsibility plan/independence and bounded metadata capabilities, shared eligibility/migration revisions, rescue-accounting support, exact qualification evidence `Ref`, `manifest_digest:Digest`. |
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
Manifest evidence separates event waiting, visible output, native context entry,
untouched idle activation and busy/human-state behavior. A Monitor/background
completion name cannot make those columns pass. The complete bound result must
also correlate the accepted supervision/eligibility plan and actual ownership
evidence to the caller's frozen requirements. Old consumers or omitted/tampered
plan/revision fields refuse the proposed mode before new effects.

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

The proposed health extension separately reports watcher PID/heartbeat/lease,
upstream connection and successful synchronization watermark/freshness, native
attachment progress, independent parent/scheduler identity and liveness, expected
and actual health/sweep times, lateness/overdue reason, sweep ownership/checkpoint,
eligibility policy and migration revision/effective mute state, and legacy-tail
counts. Quiet mail alone does not prove a stalled cursor: use bounded successful
sync evidence, not a requirement that a quiet cursor always change. Unknown
transport freshness cannot be masked by a fresh local heartbeat. Include all
seven durable rescue/sweep counters in §8.3, with per-path evidence references.

## 4. End-to-end flow and missing wire seam

1. Caller resolves ordinary launch policy and exact registered manager as today.
   For the proposed native delivery mode it retains a `CompositionRequirement`
   and negotiates its version/route/tuple, shared eligibility and independent
   supervision requirements before hooks, acquire or a new launch.
   Unsupported consumers refuse this new mode before those effects. Unenrolled
   legacy launches retain their current contracts; there is no silent downgrade
   after a new-mode refusal.
2. A jointly published successor to the closed launch/resume contract must carry
   that requirement through selection, client validation and the original
   operation journal. `RequiredForRequest` would derive needs from those bytes,
   not a parallel mutable flag. The existing lease acquire/recovery ordering and
   exact manager-incarnation comparison must survive. This wire seam is missing
   today; adding an ignored label does not fill it. `RequiredForRequest` and
   `ValidateRequest` need a separately published successor before they can
   validate/retain these requirements; no current schema is changed here.
3. The session-host owns the final typed native connection/start/resume, verifies
   the tuple, preserves native policy and interactive UI, and obtains the actual
   native identity. No second exec/resume may take a live thread. An existing
   resume must match the caller's frozen identity. Creation that becomes
   uncertain remains in the original lifecycle journal, not reported no-effect.
4. After independently checking the established binding candidate, the host and a2a use
   an agreed registration seam. a2a acquires its one-controller lease, loads
   retained obligations and reconciles previous attempts. An incomplete
   registration is not a ready composition. No second Matrix cursor or model
   turn is needed to supervise/rearm it. Registration verifies the parent/scheduler
   plan and source-owned eligibility/metadata capabilities; it cannot synthesize
   these from launch/attach/heartbeat. The host part follows actual extraction.
5. Before new identity-mailbox admission, a2a's sole shared classifier establishes
   canonical recipient/room/thread, effective explicit subscription/mute and
   direct/mention/owner relevance with its policy/migration revision (§8.1).
   A rejected future thread-only arrival creates zero new identity-mailbox
   entries, obligations or model wakes; shared history may retain the event.
   Hooks, Monitor, bind and sweep consume that same decision, not independent
   participation heuristics. Already-durable legacy obligations remain distinct.
   The controller schedules fixed notices from admitted durable mail, checks gates/rate
   state and uses the narrow host surface below. The host arbitrates against
   native state at admission; an earlier idle observation is not an atomic fence.
6. Result/status retain exact binding correlation and separate receipt stages.
   The lifecycle cursor reports gaps explicitly; gaps close admission until
   authoritative resynchronization, not until a timer guesses idle. The proposed
   successor result flows through `DecodeResponse`, `WriteDoorbell`, manager
   observation and Werkstatt status with exact supervision/eligibility/health
   evidence; today's four-field doorbell/observation v1 cannot express it.
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
| Claude Code | Independently supervised session-bound native channel or native background-completion extension as a first-class candidate. | Exact build/account/policy/launch-mode support and durable rearm while fully idle/detached. A native completion need not supervise its own death when its independent parent supplies recovery. Cooperative `a2a wait`/ordinary hooks alone do not establish that. Generic MCP notification, headless SDK streaming or one unqualified completion is not proof of attachment to the existing interactive CLI. No trust/development flag bypass. |
| Codex CLI | Existing a2a native adapter composed through the host-owned app-server and same thread; fixed notice as native external tool output where supported. | Exact native protocol/build qualification, one per-session attachment owner, CLI UI attached to that same thread, safe admission and same-attempt lookup/completeness. Candidate `turn/start`/tool-output input does not qualify any installed build, and item injection alone is not proof of a turn or intake. No steer/user-instruction rewrite. |
| Muse Code | Source-owned native MSP, native session messaging or native Monitor/event adapter after the existing Muse work's actual evidence. | Exact receiver/session, command identity, nonempty accepted notice, busy/human-state disposition, UI coexistence, detached idle wake and retention/idempotency limits. Monitor is a first-class candidate; its independent parent may supply supervision. Event wait, visible output, context entry and automatic idle activation need separate evidence. RPC request ID is not an MSP command's deduplication guarantee. Display/notification/queue acceptance without context entry is incomplete. Headless exec/reviewer success is not interactive delivery. |

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

Monitor/long-poll event waiting is not model polling. It may wait without idle
model turns; verify that property rather than infer it from its name. Output
visible only after a human prompt can establish detection/display, not idle
delivery. Compose such detection with a qualified native input route instead
of discarding the responsibility. Exact-session targeting, generation-scoped
teardown, rearm after failure and safe busy delivery remain required of the
composed system. Supporting reports of fast interim wakes or long silence are
attributed incident observations, not newly reproduced qualification here.
Generic Stop-block examples do not authorize Muse `decision:block`; its hooks
remain nonblocking, and no Claude/Codex envelope is changed by this amendment.

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

### 8.1 Shared explicit-only eligibility and visible migration

A post is authorship/delivery, not consent to future thread notifications.
Creating a topic, answering an assignment, publishing a verdict, attaching a
file or quoting/referencing a thread must not implicitly follow it. Future
thread-only eligibility requires persisted explicit intent through the supported
source-owned follow/post-follow/subscription contract. An explicit broad room
subscription applies only to its actual configured scope; historical
participation is not such a subscription.

The classifier's decision must retain recipient, canonical room/thread, relevance
reason, explicit action/scope/provenance, effective mute state and policy/migration
revision. These semantics require a versioned a2a integration capability; this
candidate invents neither a wire operation nor a second launcher classifier.
The caller freezes the expected policy revision and the controller consumes
source-owned admission evidence. A copied label or later local heuristic cannot
upgrade rejected history into an admitted identity-mailbox event.

Consume source118's effective policy: the supported explicit follow operation
clears the thread mute and records true intent/provenance. Expose and test that
effect and effective state; no separate unmute API is required. Posting, replies,
topic creation, attachments, history replay, restart/rebind and other implicit
participation never lift a mute or infer follow. Broader explicit subscription
scope and mute precedence remain governed by the published source-owned policy,
not a launcher-invented alternative or second owner. An effective thread mute
suppresses thread-only relevance under that policy. Direct-to, existing mentions and pinned ordinary
owner delivery retain their independent existing routing/authority, with and
without a follow; a thread mute cannot suppress them. Notification still grants
no execution authority.

Before each identity's cutover, a2a's migration owner records a readable inventory
of verified explicit follows/subscriptions, participation-derived follows,
unknown-origin state, mutes and already-durable pending mail/intake. Preserve
explicit provenance and mutes. Remove implicit future eligibility unless an
authorized actor explicitly re-follows. Unknown origin is visibly unresolved,
never silently relabeled explicit; absent verified intent, it cannot qualify a
new thread-only arrival. No live inventory or migration is run by this unit.

Publish a policy revision and durable cutover checkpoint with deterministic
ordering against concurrent explicit follow/unfollow/mute changes. An atomic
or restartable source-owned transition preserves those changes and cannot let
concurrent posts recreate consent. Repeated migration is idempotent; crash
recovery retains inventory, intent and unfinished checkpoint until completion is
proven. Restart, history resync and binding transfer preserve consent separately
from generation; none can regenerate it from participation. The exact migration
storage/ordering API remains the a2a owner's publication dependency.

Already-durable mail/intake is a separate ledger axis. Never delete, receipt,
mark handled or reclassify it away to obtain a zero-new-entry count. Retain known
legacy origin and drain through ordinary intake/disposition, reporting a bounded
legacy notification tail honestly. After cutover, rejected future thread-only
arrivals cause zero new identity-mailbox entries, obligations and model wakes;
they may remain in shared history for explicit retrieval. Hook/Monitor/bind/sweep
must not resurrect rejected arrivals. Genuine prior durable obligations remain
recoverable even if future eligibility has changed.

### 8.2 Independent supervision and deterministic pre-model sweeps

The launcher carries the requirement; the extracted host/source owner supplies
the watcher-parent/scheduler lifecycle contract; carrier integration preserves
it; the deployment owner names the actual independent components in §2.2.
No installed Cron/service/API is asserted. A runtime-native Cron that starts a
model turn before inspecting state fails this contract. A deterministic host
timer/scheduler or supported source-owned pre-model gate is allowed; no keeper
shim, live cron registration or second queue is introduced here.

Proposed tunable policy values, requiring source/host agreement: health tick
60s, full reconciliation 10min during migration, total sweep timeout 10s,
and immediate parent observation on child exit where supported. Each deployment
must also publish its maximum admitted scheduler lateness. Under a healthy
independent scheduler, a fault detectable on a health tick has a proposed bound
of 60s + lateness + 10s; a discrepancy detectable only by full reconciliation
has a distinct 10min + lateness + 10s bound. These include detection/check time,
not eventual provider repair or model intake. Stricter existing budgets remain;
these are not installed defaults or unconditional guarantees. Exceeded lateness,
missing ticks or unknown sync freshness are overdue/unknown, not silently within
bound. Watcher PID/heartbeat and connection/cursor progress are separate facts.

The schedule must run outside the watcher or have an independent parent that
restarts its schedule. The watched stream cannot be the only signal that the
schedule itself died. Record the actual parent/clock/lease failure domains and
external report route. A machine-wide outage cannot be repaired locally while
it is down; preserve obligations and report through an admitted independent
host monitor when available. No local self-supervision guarantee fills that gap.

Each bounded sweep performs this deterministic sequence before model activation:

1. Acquire one generation-fenced sweep authority/checkpoint; serialize overlapping
   ticks. Read supported non-receipting mailbox metadata and obligation/attempt/
   intake journals. Do not call ordinary inbox, pull peer bodies into instructions,
   create another transport cursor or infer mail absence from a failed read.
2. Validate recipient, generation, boot/birth/endpoint custody, active-set policy,
   gate, scheduler authority and shared eligibility evidence. Preserve paused,
   parked, retired and human-controlled state; source failure is a hold, not an
   implicit idle/empty result.
3. Compare watcher lease/heartbeat with independent upstream sync freshness,
   native attachment progress and overdue health/sweep evidence. A recent local
   heartbeat cannot prove an upstream connection made progress.
4. Request only its owned, already-authorized component repair through the
   lifecycle owner. Journal intent/result, including unsupported/held/failed
   repairs. Native owner/attempt uncertainty remains fenced; never kill a reused
   PID, seize another session or start/resume a live thread as a repair.
5. Upsert mail or incomplete intake under its existing stable obligation key.
   Unread=0 must not suppress an incomplete intake batch. Rejecting a new
   thread-only arrival is not permission to erase a genuine legacy obligation.
6. Send signals to the same arbiter. It reconciles the original uncertain attempt
   and applies human gates, shared 30s throttle, budgets and qualified native
   admission before any dispatch. A sweep has no independent submit authority.
7. With no eligible obligation, produce zero model turns and zero notifications.
   Empty successful sweeps may increment aggregate counters or deterministic
   bounded logs. A model-generated one-line empty report fails the pre-model
   gate. Actionable faults use bounded redacted diagnostics, not an empty turn.

Before possible repair/re-enqueue, persist sweep correlation and completed
substeps. After crash, reconcile the same action, not a new blind repair. After
sleep/reboot, perform one bounded reconciliation under current custody, not a
burst of missed ticks. Preserve pending ages, attempts and rate reservations;
wall-clock changes cannot manufacture budgets or restart retired identities.

Repeated detections from hook/Monitor/bind/sweep upsert the same identity plus
durable mail/intake obligation key, recording signal source. Generation fences
who may act; transfer adopts unfinished obligations instead of inventing new
business work. A shown/rung flag cannot suppress new mail forever. A signal is
neither a receipt nor handling. Accepted-but-unacknowledged native submission
remains uncertain across fallback ticks and both ledgers; repetition of a signal
does not authorize another turn or external business effect.

Fairness is an explicit controller dependency: keep independent pending batches,
their ages and fair selection visible; one unrelated ambiguous batch must not
erase or permanently hide new mail. Across bindings, that uncertainty must not
block independent qualified bindings. Within one binding, the one-outstanding-
attempt and no-blind-retry fences remain: progressing another batch while the
native outcome is ambiguous requires a source-owned qualified isolation/admission
primitive. If unavailable, expose the blocked batch, waiting mail and exact
primitive gap; do not fake fairness by repeating the uncertain native effect.

### 8.3 Causal rescue accounting and evidence-gated retirement

For each path and binding, retain detection time, stable obligation and signal
source, current generation, prior primary deadline/progress/attempt evidence,
repair/re-enqueue action and actual subsequent correlated intake/recovery result.
Proposed separate durable counters are:

| Counter | Evidence meaning |
| --- | --- |
| `fallback_observed` | Another path noticed the same pending work; not necessarily a primary defect or rescue. |
| `fallback_repair_attempted` | An authorized repair/re-enqueue was attempted, with its original action correlation; no completion inferred. |
| `fallback_rescue_completed` | A demonstrably missed/overdue primary or failed component required action, the fallback causally repaired/re-enqueued it, and correlated intake/recovery actually progressed. |
| `fallback_rescue_unresolved` | A real rescue need remains uncompleted, including held/failed recovery; retain its evidence and age. |
| `empty_sweep` | A successful pre-model sweep found no eligible obligation; no notification/model activation/receipt. |
| `overdue_sweep` | A scheduled check missed its admitted bound or remains incomplete; distinguish from a healthy empty check. |
| `duplicate_signal_coalesced` | Repeated observation merged into the existing stable obligation rather than creating another dispatch. |

Concurrent discovery alone is not a rescue. A legitimate human/rate hold is not
a missed eligible primary. Native acceptance alone is not rescue completion.
Record the demonstrable overdue/missing-primary state, causal action and later
intake evidence; do not invent counterfactual success. Counters/evidence survive
restart/upgrade and generation transitions with attributable continuity. A
resolved incident may leave the unresolved set but its historical failure/action
is retained; never zero history or omit failed repairs to pass a soak.

Keep redundant paths enabled and instrumentation working through representative
traffic and idle observation. Fault controls must prove both intended rescue and
counting, independently of ordinary traffic. Record controlled faults separately;
their planned rescues neither masquerade as ordinary incidents nor disappear
from evidence. Minimum plan remains the per-tuple delivery counts, one-hour
active soak and at least 12-hour idle soak plus representative production
observation. No traffic, disabled fallback or broken instrumentation cannot
establish a meaningful zero.

Retirement requires a qualified exact native/host replacement, zero ordinary
completed rescues, zero unresolved rescue needs and no missed eligible obligations
through that exercised period. Any genuine rescue or unmet need requires
diagnosis/fix and a new qualifying soak; a planned isolated control is reported
separately. Source/host owners review and retire one redundant path at a time
with tested rollback. Independent supervision is permanent until its
responsibility has its own qualified independent replacement; do not retire it
because a native adapter exists. Remaining supervision must still repair silent
watcher death after a redundant path is removed. This grants no terminal fallback
as target architecture and no live retirement in this source unit.

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
| C14 throttle/budget | Freeze a shared reservation at t0 and two admitted durable events at t0 and t0+15s within its 30s throttle window. Both obligations remain durable: holding/coalescing must never silently drop the second event or yield zero eventual notice. Exhaustion/stricter hourly budgets, human holds, one outstanding attempt and uncertainty fences remain; no timer reset or bypass. | Once the 30s reservation and any stricter holds legitimately release, one eventual coalesced eligible notice covers both retained obligations with their original ages. Pair with a second event just after the window (t0+30s+epsilon), otherwise clear gates/budget and a reconciled first attempt: it gets a separately eligible notice without loss. These are planned timing baselines, not executed real-time proof. |
| C15 no mail | Empty mailbox without intake recovery obligation causes zero mail-induced model turns throughout prolonged idle. | A later durable addressed event awakens the qualified route without human Enter, through all separate receipt stages. |
| C16 receipt/mirror | Native accepted, inbox printed, task handled and outbound mirrored cannot be substituted for each other; copied batch token cannot settle another generation. | Independently correlated exact event/intake/application/outbound evidence establishes only its corresponding leg. |
| C17 upgrade/rollback | Unknown build or unreadable new journal pauses native dispatch; never discard uncertainty to downgrade or call restored terminal route native-ready. | Owner retains readable state and returns to an accepted exact tuple; any admitted legacy recovery is labeled legacy/degraded. |
| C18 empty pre-model sweep | Empty supported metadata plus no incomplete intake yields zero model activations, notifications and receipts. A model-generated empty report fails even if brief. | Eligible mail under the same gate creates one stable obligation and one admitted notice; deterministic empty-check counters/logs remain allowed. |
| C19 dead or silently stalled watcher | A dead watcher or alive PID with expired progress cannot report healthy solely from PID/lease. An independent parent detects within its admitted bound and journals only owned repair. | Healthy event waiting with fresh authoritative progress remains idle without model polling; exit-triggered repair preserves generation and pending work. |
| C20 heartbeat-live transport stall | Fresh local heartbeat with stale upstream sync/cursor evidence is stalled/unknown, not empty or healthy. No inbox receipt or invented reconnect success. | Quiet transport with fresh source-owned sync evidence passes without requiring new mail; a correlated repair restores progress separately. |
| C21 redundant detector race | Hook, Monitor, bind reconciliation and sweep cannot reserve separate dispatches for the same durable obligation. Count coalescence; preserve oldest age and one rate reservation. | Any one detector alone upserts the same key and observes the original correlated attempt. |
| C22 fallback after uncertain acceptance | A sweep encountering accepted-but-lost reply retains the original uncertain attempt in both ledgers; zero second submit, business effect or off-clear. | Complete same-attempt reconciliation resolves accepted, or authoritative no-submit permits a separately admitted attempt under existing rate limits. |
| C23 print-crash sweep | Unread=0 with an incomplete intake batch is not an empty sweep; preserve exact retained payload and no handling claim. | Supported non-receipting intake metadata identifies the batch; persisted tool result/disposition completes intake without equating it to application result. |
| C24 overlap/crashed tick | Overlapping ticks or a crash after repair intent cannot duplicate repair/re-enqueue, lose substeps or renew rate budgets. Unknown action outcome remains unresolved. | One serialized sweep checkpoints action/result; restart reconciles that same action before another permitted repair. |
| C25 held or stale sweep | Paused, parked, retired, human-controlled or wrong-generation state refuses new dispatch/repair outside its authority; no restart, steering or state erasure. | A current eligible binding with independently clear human state and owned repair permission proceeds once. |
| C26 sleep/clock/reboot | Missed intervals do not cause a burst, age erasure, budget reset or retired-identity resurrection; wall-clock movement cannot create authority. | One bounded current-custody reconciliation after resume preserves attempts/ages and reports scheduler lateness honestly. |
| C27 causal rescue | Concurrent duplicate discovery or native acceptance alone cannot increment completed rescue. A deliberate missed-primary fault needs correlated action and subsequent intake progress. | The planned fault records one causal completed rescue; concurrent healthy discovery records observation/coalescence only. Controlled and ordinary samples stay separate. |
| C28 false zero | Disabled fallback, broken counters, no representative traffic or missing overdue checks cannot pass retirement as zero rescues. Preserve failed repairs and unresolved needs. | Enabled instrumentation and known fault controls detect the intended deficit; healthy exercised ordinary traffic can establish the bounded observed zero. |
| C29 circular supervision | A watcher whose only parent/check schedule dies with itself cannot satisfy independent supervision or a detection bound. Machine outage cannot promise local repair. | Independently evidenced parent/scheduler detects silent death; an admitted external monitor reports machine outage without pretending local recovery. |
| C30 soak reset | An ordinary genuine rescue/unmet need invalidates the qualifying soak; counter reset or hiding failed repairs cannot restore it. | Diagnose/fix and restart the qualifying soak; separately labeled planned controls prove recovery/counting without being disguised as ordinary traffic. |
| C31 reply without follow | After a reply without explicit follow, a later unrelated thread-only arrival creates zero new identity-mailbox entries, obligations or wakes. | Explicit follow admits an in-scope thread arrival without duplicate delivery on repeated follow; an unrelated unfollowed thread remains excluded under the same classifier. |
| C32 participation is not consent | Topic creation, attachment, required verdict, reference or authorship alone cannot grant future thread-only eligibility. No implicit follow side effect. | An explicit supported follow/subscription with retained scope/provenance admits only that scope; direct-to relevance remains independently valid. |
| C33 mute and independent routes | Effective thread mute suppresses thread-only relevance under source policy; posting/history/restart/rebind/implicit participation cannot lift it or infer follow. Do not suppress independent direct-to, mention or pinned-owner delivery. | Supported explicit follow visibly clears mute, records intent/provenance and permits scoped thread delivery without a separate unmute API; direct/mention/owner controls pass with and without follow. Broader subscription precedence follows the published source policy. |
| C34 history/restart/rebind | History replay, restart or generation transfer cannot reconstruct explicit consent from participation or resurrect rejected future mail. | Persisted explicit intent survives those transitions with its scope/mute/policy evidence; shared history remains explicitly retrievable. |
| C35 visible migration and legacy tail | Implicit/unknown-origin follows cannot be relabeled explicit, and migration cannot delete/receipt/handle already-durable pending mail/intake to reach zero. | Visible inventory preserves verified explicit intent/mutes, removes implicit future eligibility, retains unknown-state gaps and drains the separate legacy tail honestly. |
| C36 concurrent/crashed/repeated migration | Crash, repeat or concurrent follow/unfollow/mute cannot lose true intent or apply participation as consent; unfinished cutover stays visible. | Deterministic atomic/restartable checkpoint replay is idempotent and preserves explicitly ordered concurrent changes and retained pending state. |
| C37 shared pre-entry decision | Hook, Monitor, bind and sweep cannot create new identity entries/obligations/wakes for the same rejected future thread-only arrival or override source-owned eligibility. | All paths consume the same accepted explicit policy decision; genuine pre-cutover durable intake remains recoverable as a labeled legacy obligation. |
| C38 extraction-first boundary | Prepared in-tree lifecycle ports or a source proposal cannot authorize host-specific implementation or claim a shipped exported API before actual extraction. | Independent contracts/non-host-dependent adapters may progress; host-specific implementation becomes eligible only after actual extraction and accepted versioned API publication plus its separate implementation grant. |

Extend frozen E for C18–C38 with independently supplied owner/parent/scheduler
identities and failure domains, timing bounds, sync freshness, stable obligation
and sweep substeps, ordinary-versus-controlled samples and durable counter
history. For eligibility, freeze canonical room/thread, explicit intent/scope/
provenance, mute and policy/migration revisions, concurrent intent ordering and
the pre-cutover pending/intake inventory. An echoed policy label, rescue counter
or migration-complete flag cannot supply its own expected verdict. Apply these
plans to Claude, Codex and Muse separately; source, fixture, native and installed
host evidence remain distinct and all execution columns are **not run** here.

The controller environment control pairs a forbidden inherited interactive/
session messaging or registration secret with valid allowlisted process/locale/
proxy/CA essentials and explicit config-root/session inputs. The former must be
absent from the detached controller environment; the latter must remain usable.
A separately scoped owning-session adapter channel remains valid without broad
secret inheritance. Freeze expected names/presence outside the filter; never
record secret values or accept rejection of every valid transport as success.
This is source-plan coverage, not an executed environment or credential probe.

For C14, a future isolated control that silently drops the t0+15s event must
trigger the intended retained-obligation/eventual-notice assertion, not merely
a timeout or unrelated error. Compare both durable event references and eventual
notice coverage against frozen expectations. The just-after-window control and
unmodified inside-window coalescence must still pass. This supplements existing
plans without adding an executable checker or weakening uncertainty/rate fences.

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
| P06 move empty gate after model start | C18 fails zero model-activation/notification/receipt assertions; eligible-mail delivery and deterministic empty logging controls still pass. |
| P07 trust heartbeat or self-supervision alone | C19/C20/C29 reject false health/detection bounds with dead parent or stalled transport; healthy quiet sync and independently supervised recovery controls pass. |
| P08 count discovery as rescue or disable instrumentation | C27/C28/C30 reject fabricated rescue/zero/soak evidence; causal controlled rescue and exercised healthy ordinary-traffic controls remain distinguishable. |
| P09 fallback blindly resubmits uncertain attempt | C22 fails retained-uncertainty and zero second-submit/effect assertions; complete original-attempt reconciliation control passes. |
| P10 infer follow from post/history | C31/C32/C34 fail zero new identity-entry/obligation/wake assertions for unfollowed future arrivals; explicit scoped follow and independent direct-to controls pass. |
| P11 implicit mute lifting or ignored effective source policy | C33 fails thread-only exclusion/effective-state assertions for posting/history/implicit mute lifting; supported explicit follow clearing mute and independent direct/mention/owner controls pass. |
| P12 corrupt migration ordering or legacy ledger | C35/C36/C37 fail preserved explicit intent/mute/pending-state and future-entry exclusion assertions; idempotent checkpoint replay and genuine legacy recovery controls pass. |
| P13 treat prepared ports as accepted extracted API | C38 fails source dependency/admission evidence assertions; independent contract work and actually published accepted API controls remain distinct. |

No plant, parser, fixture, native session or checker is executed by this docs
unit. Unknown native inputs stay explicit gaps rather than invented tuples.

## 10. Dependency publication, migration and eventual acceptance

1. Original source owners agree the packet, final names/encoding, native safe
   admission limits, exact dependency source refs and compatibility readers.
   A2a integrates its own capability/adapter, shared explicit-only classifier,
   visible consent migration and intake-retention units; the existing Muse work
   stays with its original owner. No duplicate producer or launcher classifier.
2. Publish independently reviewed exact source interfaces/schema modules through
   their owning repositories before a separately admitted implementation. No
   guessed release tag, local replace, root-module/provider upgrade or arbitrary
   extra dependency is implied by this candidate.
3. Independent launcher/a2a contracts and non-host-dependent adapters may proceed
   through their own admitted scopes. Host-specific reliable-inbox implementation
   waits for actual session-host extraction completion and accepted versioned
   exported API publication, then consumes that module. Prepared in-tree ports
   are not a bypass. Existing host/core/PTY/plugin/composition closure and landing
   custody remain with their source owners; no competing task-board host.
4. Implement under a recorded code/test scope, then run isolated logic/fixture
   and detector controls, independent review and genuine green checks; land
   accepted signed heads through each source owner's custody. No doc acceptance
   stands in for an implementation or native input API.
5. Existing host owners return actual owner/parent/scheduler/failure-domain
   allocations and choose noncritical eligible canaries; migrate serially
   per runtime. Keepers/control-plane leads supervise rather than become test
   subjects. The old consumer drains/retire-reconciles itself before a qualified
   replacement is armed; never act on another live session to make a test pass.
   Eligibility cutover preserves explicit intent/mutes and all durable pending
   state; publish remaining legacy-tail counts separately from future exclusions.
6. Qualify Claude, Codex and Muse separately on every enrolled actual
   OS/build/launch-mode tuple. Initial plan: at least 20 idle and 20 busy
   deliveries per tuple, deterministic crash/human-state cases, one-hour active
   soak and at least 12 hours untouched idle. Native event/generation/turn/tool
   references, timestamps and actual exits must support each claimed stage.
   Manual Enter invalidates that run's automatic-wake evidence. First serial
   canaries can precede the soak; full acceptance cannot.
   Exercise pre-model empty sweeps, independent supervision, shared eligibility
   and causal rescue controls. Keep fallback enabled/instrumented through the
   exercised zero-ordinary-rescue period of §8.3; review/retire one path at a time
   with rollback. Keep independent supervision until its own qualified replacement.
7. Retain pending/uncertain/intake state on upgrade/rollback. Untouched legacy
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
retention migration; shared eligibility/provenance/mute and atomic consent
migration publication; bounded non-receipting metadata and fairness/isolation
primitives; actual extracted API and per-deployment independent supervisor/
scheduler/failure-domain allocation; and host-owned serial qualification/adoption. Vendor API
availability, a method name or a document cannot resolve any of those gates.
