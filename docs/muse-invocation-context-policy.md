# Managed Muse invocation context policy — proposed contract, revision 1

**Status: PROPOSED source/interface candidate.** This document is neither an
accepted Curator amendment nor an implemented SAM API or launcher feature.
`--allow-foreign-personal-context` is a proposed launcher option, not runnable
usage. The shipped [FRONT-DOOR §7](../FRONT-DOOR.md#7-muse) remains authoritative
until the source owners accept and publish the amendments and dependencies below.
No native qualification, account readiness or actual foreign-context loading is
established here. All uppercase requirements below describe the candidate target.

## 1. Scope and source baseline

The target is one deliberate invocation of latest-installed Muse through
`curator run muse --native --profile P --yolo`, retaining the selected managed
profile and adding the proposed allowance. `--native` selects native execution;
`--yolo` selects permission posture. Neither selects this context policy.
The allowance is restricted to **native interactive managed Muse**. It does not
admit hosted, exec, serve, dry-run, other runtimes, an inherited environment or
configuration default, or a headless reviewer exemption. Latest-installed
selection continues through the existing binary resolution; no frozen-copy,
expected-lock, no-repair or alternate-home feature is introduced.

The examined source baseline is:

- Launcher [`8ab9cb3`](https://github.com/relux-works/curator-agent-launcher/tree/8ab9cb30735a159589649d32c6544f3b99ebec3a):
  [SPEC §§4.1–4.6](../SPEC.md#4-composition-algorithm), FRONT-DOOR §7 and the
  concrete call sites in the table below. The actual `go.mod` pins SAM v0.5.74;
  older version numbers in historical SPEC text do not establish this capability.
- SAM **v0.5.74**, commit
  [`2941b4b`](https://github.com/relux-works/skill-agents-management/tree/2941b4bbfb6d8aa2ca4e3ed7bafd335d493f773f).
  Its typed fragment transport, strict exclusion and seal are existing;
  the invocation-policy carrier and receipt proposed here are missing.
- Curator-spec
  [`a9c5e03`](https://github.com/relux-works/curator-spec/tree/a9c5e03b2857a10a66433e1a60bb255a89b9d741):
  [environments §§7.1, 7.9, 10.2](https://github.com/relux-works/curator-spec/blob/a9c5e03b2857a10a66433e1a60bb255a89b9d741/protocol/environments.md)
  and [CIP-0017](https://github.com/relux-works/curator-spec/blob/a9c5e03b2857a10a66433e1a60bb255a89b9d741/cips/CIP-0017-muse-root-context-and-credential-store.md).
  They currently require exclusion when the fact is present. Their accepted
  status does **not** extend to this proposed exception.

### Existing surfaces and missing interfaces

SAM paths below are relative to that exact SAM commit, not to this repository.

| Existing source/surface | Existing behavior | Required proposed extension |
| --- | --- | --- |
| Launcher `cmd/curator-run/main.go`, `internal/plan/plan.go` (`Request`, `SpawnRequest`, `Build`) | Passes context/home/native arguments to `BuildLaunchWithEnvironment` in `LaunchModeInteractive`; native/hosted routing is separate | Parse and guard one invocation choice, retain a caller-owned expectation, call a versioned policy entry, verify its receipt |
| Launcher `internal/plan/context.go` (`Context`) | Copies raw `RootContext`, credentials, profile/lock, env and channels into `agentic.CuratorContext` | Keep this projection unchanged; carry policy separately |
| SAM [`pkg/vendorplugin/spawn.go`](https://github.com/relux-works/skill-agents-management/blob/2941b4bbfb6d8aa2ca4e3ed7bafd335d493f773f/pkg/vendorplugin/spawn.go) | `SpawnRequest.Context`, `NativeArgs`, `PermissionMode`, `ToolRelease`; snapshots and `checkLaunchFidelity`; system-only `passthroughLaunchRequest` | Carry required versioned policy through both system-only and vendor paths; reject omission/replacement before observations/preflight |
| SAM `pkg/agentic/system.go`, `plan.go`, `curator_context.go` | `LaunchRequest.Context`; `CuratorContextValidator`; `Plan.CuratorContextProvenanceSnapshot`; owned environment in `PlanWithEnvironment` | Separate typed request choice, immutable effective decision and plan receipt, with exact input binding |
| SAM `pkg/agentic/systems/muse/context.go` | `System.ValidateCuratorContext` validates raw context then per-mode exclusion | Consume the single effective decision without weakening raw validation |
| SAM `pkg/agentic/systems/muse/muse.go` | `ChildEnv` repeats raw validation and mode-less `validateForeignContextExclusionInSomeForm` | Receive a bound decision that includes mode; mixed cannot be inferred just from a new boolean or missing exclusion argv |
| SAM `pkg/agentic/systems/muse/args.go`, `personal_context.go`, `policy.go` | `Args`/`interactiveArgs` alone construct argv; `requiresForeignContextExclusion` reads the fact; `scanMuseNativePolicy` guards native arguments | Use effective exclusion requirement, retain form/permission checks, scan policy contradictions for both choices |
| SAM `pkg/agentic/systems/muse/seal.go`, `network_launch.go` | `SealExecPlan` / `sealInteractiveExecPlan` bind binary, build, argv, permissions, help and selected env; raw provenance triggers strict qualification again | Bind policy version/choice/effective decision and original fact identity; qualify exclusion only when required; retain every other seal check |
| Launcher `internal/composition/composition.go`, `internal/composition/muse.go` | `ComposeAdmittedPlanWithNetwork` preserves admitted native suffix and backend/home guards; returns `composition.Value` | Preserve receipt/verifier through composition; never construct or strip Muse policy argv |
| Launcher `cmd/curator-run/main.go` (`runNative`), `internal/execution/execution.go` (`Prepare`, `Launch.Run`) | Native execution receives `composition.Value`; its late boundary currently checks prompt files, not the SAM plan seal | Missing native integration: retain the SAM verifier and bound receipt, verify the exact final process immediately before child start |

The final row is a source-observed interface gap, not a claim that current native
execution verifies SAM policy. SAM remains the **sole native-argv owner**. This
candidate commissions no implementation or separate SAM writer.

## 2. Proposed Curator normative amendment packet

These are explicit proposed edits for Curator source-owner review. This launcher
candidate does not edit Curator source or widen released fragment schemas.

**Environments §10.2:** retain the complete v4 shape and producer rules, including
`root_context` exactly `{"exclusive":true}` when Curator establishes the target.
Replace the unconditional requirement in **Muse managed launch argv** with:

> The consumer MUST preserve the validated producer fact and fragment identity.
> With no explicit admitted invocation allowance, presence requires the existing
> form-specific exclusion and form/build qualification, with identical dry-run
> mapping. Only a native interactive managed Muse invocation may explicitly
> choose `allow-foreign-personal-context` through the versioned consumer policy
> contract. For that invocation alone, the effective policy is MIXED and the
> exclusion requirement is waived. The consumer MUST omit its exclusion override
> without adding an enabling provider flag. It MUST NOT omit or rewrite the raw
> fact, fragment, profile, lock or home. A missing policy is strict, never an
> allowance. Unsupported choices, carriers, consumers and forms MUST refuse
> before provider start; an explicit choice MUST NOT be silently ignored.
>
> The exception waives only required foreign-personal-context exclusion. All
> other admission and integrity rules remain. It grants neither account/context
> isolation nor assurance that any foreign rule or skill actually loads. It MUST
> NOT be reported as qualified-exclusive. Native argv MUST NOT replace the typed
> choice. Exec, dry-run and serve gain no exception; their existing obligations
> and refusals remain. No filesystem inspection may decide or reconstruct the
> producer fact or invocation choice.

The existing default exclusion spellings stay exact: exec/dry-run use
`--no-foreign-personal-context`; interactive uses the two elements `-c` and
`context.foreign_personal_skills=false` (JSON boolean false). Mixed interactive
emits neither exclusion and invents no `...=true` or other enabling option.

**Environments §7.1:** append a cross-reference after the existing Muse loader
observations: “These are producer/loader observations. The §10.2 explicit
native-interactive consumer policy may permit MIXED context without changing
materialization, home layout, credential policy or the raw exclusive fact.
Absent that choice, the existing exclusion obligation remains.” Preserve all
rules-fallback, workspace-precedence, skills and model-visible residuals.

**Environments §7.9:** append to Muse qualification order: “Form/build exclusion
qualification remains mandatory whenever effective policy requires exclusion.
An admitted explicit MIXED invocation waives that requirement only; it creates
no exclusion qualification row and does not relax binary/build identity, model,
help, permission, credentials, network or fragment admission.” In particular,
this proposal adds **no Muse 1.4.14 qualification**. At SAM v0.5.74 interactive
qualification contains only `1.4.7-R6101.1`; the exec-only records for 1.4.10,
1.4.11 and 1.4.13 are not interactive evidence. Strict unqualified 1.4.14 remains
refused. Latest-installed is selection, not qualification.

**CIP-0017:** append an explicitly dated invocation-policy amendment after
Recommendation 1, cross-reference it from Summary and Compatibility, and qualify
Recommendation 1's unconditional mapping sentence and table as **default strict**.
The amendment MUST carry the §10.2 exception above and the §§7.1/7.9 residuals,
require the separate versioned typed consumer carrier and fidelity checks in
§§3–5 below, and retain credential Recommendation 2 unchanged. Clarify its
Security paragraph: the raw fact records verified materialization and the default
loader obligation; effective exclusion is determined by the separate invocation
policy. Historical evidence and prior implementation/test plans remain historical.

No change to v1–v4 released schema bytes, conformance vectors, Curator emission,
profile contents, credential provisioning, renderer output or permission locks is
proposed. Policy is **not a new fragment member** or a persistent profile setting.

## 3. Proposed typed choice and one authoritative decision

All names and tokens in this section are **proposed**, absent from SAM v0.5.74,
and require SAM engineering agreement before dependent launcher implementation.

| Proposed concept | Closed target contract |
| --- | --- |
| `MuseInvocationContextChoice` | Exactly `strict` or `allow-foreign-personal-context`; case-sensitive, no aliases, trim/coercion or truthy values |
| Launcher input default | Option absent normalizes once to `strict`; option present selects allowance only for this invocation. No env/config/profile/yolo inheritance; duplicate, boolean-value or unsupported-form uses refuse as usage |
| `MuseInvocationContextRequestV1` | Required revision `muse-invocation-context/1`, required normalized choice, native execution kind, interactive mode, exact original fragment digest and validated raw-context identity. No optional/defaulted choice in this carrier |
| `MuseInvocationContextDecisionV1` | SAM-owned immutable decision binding the entire request and raw context: strict requires exclusion when fact present; allowance yields `MIXED`, exclusion not required. Only SAM's common policy gate creates it |
| `MuseInvocationContextReceiptV1` | Detached plan result view of the same decision, including revision, requested choice, execution kind/mode, fragment digest, raw identity and effective result; bound by the plan seal, not a caller-written success marker |

For this bounded allowance the raw input MUST be valid managed Muse v4 with
`root_context={"exclusive":true}`. Explicit allowance without that fact refuses
rather than inventing a materialization claim. Existing strict launches with an
absent fact retain their standing residual and neither exclusion; absence never
permits file inference. Invalid false/null/extra facts refuse before policy.
The allowance does not create support for otherwise inadmissible context channels.

The single decision rule is: **after raw validation and scope/capability checks,
strict preserves the existing obligation; the explicit admitted choice waives
only that obligation.** `MIXED` means permitted mixture, not observed loading.
A strict result reports required exclusion/qualification separately from the raw
`exclusive=true`; even a qualified exclusion is not general isolation evidence.

The policy gate MUST snapshot inputs before vendor dispatch, preparation,
observation or preflight. No native argument, permission mode, help response,
filesystem state or adapter may recompute a different choice. The launcher keeps
an independent immutable expectation made directly from parsed input and the
original resolved fragment, and compares the returned receipt against it.
The digest uses the launcher's existing whole-fragment identity algorithm; it is
not a new digest of a rewritten projection. Raw context identity includes the
validated projection and existing Curator provenance, in addition to that digest.
An adapter must not simply echo a caller-claimed digest for modified raw fields.

The proposed field on both `vendorplugin.SpawnRequest` and
`agentic.LaunchRequest` is `InvocationContext *MuseInvocationContextRequestV1`.
Nil on the legacy entry means strict-only; nil on the V1 entry is invalid.
The required request members are `Revision`, `Choice`, `ExecutionKind` (exactly
`native`), `Mode` (exactly interactive), `FragmentDigest` and `RawContextIdentity`
(the detached validated context/provenance binding described above). The receipt
echoes those exact members and adds `Effective` (exactly `strict` or `MIXED`)
and `ExclusionRequired` (true for strict with the fact, false for MIXED or
strict without the fact). No
extra/unknown member or empty revision/choice is supported. An opaque SAM-owned
decision handle accompanies the prepared request; its private snapshot binds
all those values, not just `ExclusionRequired`. These are proposed logical
records, not a claim of an existing serialized request schema.

The decision travels as a separate carrier, never inside
`CuratorContext.RootContext`. Vendor dispatch snapshots and compares
it in `checkLaunchFidelity`; system-only passthrough carries it too. Request
preparation and alias projection MUST retain it. `BuildPlan` checks raw identity
against the original and freezes the decision into the plan before adapter calls.
The receipt MUST originate from that bound decision, not from argv inspection.

`ValidateCuratorContext`, `ChildEnv`, `Args` and seal construction/verification
consume that **same** decision. `ChildEnv` has no mode argument today: the
proposed prepared carrier supplies its bound mode. A direct adapter caller with
an allowance but no SAM-issued matching decision MUST refuse; direct legacy
strict calls keep their current validation. Missing decisions on the new entry
are errors, not permission to recreate a default. No unconditional early return
may skip raw validation, backend checks or permission admission.

## 4. Proposed adapter, seal and launcher fidelity

SAM `Args` remains the only constructor of Muse argv. For strict, current
qualification and exclusion remain, including the repeat check in the seal
against the attested full build. For MIXED, only exclusion-specific emission
and qualification are disabled. Permission-mode resolution, yolo help evidence,
attested well-formed build, binary hashing, release cross-check, updater pin,
model/effort admission and existing network refusal remain mandatory.

The adapter MUST scan native arguments for context-policy conflicts in both
permission modes and both choices, including equivalent config-option spellings.
For this target, explicit exclusion/enabling switches or overrides of
`context.foreign_personal_skills` in the native option suffix refuse, even if
apparently redundant. Operator policy belongs in the typed carrier; the plugin
must distinguish option position from prompt/data after the provider separator.
Unparseable policy-affecting option syntax refuses. An exec/serve command word
in command position cannot turn the interactive allowance into another form.
Launcher form guards and SAM form gates are complementary; neither strips the
profile or retries a weaker mode. No new provider flag is inferred from help.

The proposed policy-aware interactive seal MUST bind revision, requested choice,
effective decision, native/interactive scope, original fragment digest, complete
raw provenance/context identity and existing final process bindings. Its verifier
MUST detect a changed choice, missing/changed fact, home/backend/channel change,
or substituted decision even when argv is unchanged. Provenance accessors and
receipts return copies; mutating a view cannot mutate the private sealed basis.
A seal exported/imported with policy MUST use the distinct version selector in
§5; old seals/importers cannot authorize it. Existing binary/env/help/integrity
checks are retained; no resealing of tampered inputs to make them current.

Launcher composition MUST carry the original receipt and verifier to native
execution. It MUST compare the final composed process against SAM's sealed
basis through the SAM-owned finalization/verification interface immediately
before start. `ComposeAdmittedPlanWithNetwork` may retain its existing suffix
placement; it may not interpret, delete or append policy argv. Any unsupported
composition overlay, changed raw fact/digest, receipt mismatch or missing
verifier refuses with no provider start or native-default fallback. The policy
receipt is provenance, not execution authority by itself. SAM must agree how
its existing `FinalizePlan` / `VerifyBeforeExec` interfaces bind the final native
value; today's `composition.Value` / `runNative` gap must not be papered over by
a launcher-side boolean check.

Diagnostics MUST distinguish requested choice, effective MIXED versus strict
exclusion requirement, and raw producer fact. They MUST NOT describe MIXED as
qualified-exclusive or print profile contents, credential values, full native
argv or personal paths. Unsupported input is usage; unsupported capability,
carrier/fact mismatch and seal mismatch are typed refusals with distinct stable
reason categories to be allocated by SAM/launcher owners. Existing model,
permission and other refusal identities remain unchanged. No new numeric
exit/refusal allocation is asserted by this source candidate.

## 5. Concrete compatibility proposal and publication gates

**Recommendation: require a versioned in-process entry and receipt, plus an
adapter capability and policy seal successor.** A field on the old API, a module
version comparison, or absence of an exclusion argument cannot demonstrate
support. This unit does not define a hosted JSON extension or change any pinned
hosted schema; explicit allowance is rejected before hosted routing.

Proposed SAM entry `BuildLaunchWithContextPolicyV1` accepts `SpawnRequest`
with its required `InvocationContext` V1 carrier and the launch mode, and returns
`PlanWithEnvironment` plus the bound V1 receipt. There is no second policy
argument or independent adapter default. Both strict and allowance through this new path
require every V1 field and exact agreement with runtime, mode and raw context.
The compile-time entry cannot resolve against old SAM. The launcher MUST use it
for the new policy path and MUST NOT retry `BuildLaunchWithEnvironment` on any
error. Legacy entry points remain strict-only; an allowance carrier reaching
one MUST refuse, never be ignored.

Before vendor dispatch or effectful planning, this entry requires the resolved
Muse adapter to declare exact capability **`muse-invocation-context/1`** and its
policy-aware decision/sealer interfaces. A missing/different token or interface
refuses without attempting an old adapter. A declaration alone is insufficient:
required carrier fidelity, adapter use of the issued decision, exact receipt and
policy seal are verified before success and again before execution. A stub
adapter that advertises the token but drops policy fails those checks.

Proposed seal selector **`muse-invocation-context-version=1`** and required bound
policy fields distinguish the successor. Importers MUST reject missing/unknown
versions, unknown fields, a missing required policy field or an integrity/binding
mismatch. A V1 request cannot accept an older seal as strict success, even when
the requested choice was strict. Legacy strict seals retain their legacy path;
they cannot carry or prove an allowance. No existing closed seal or released
schema is silently widened. If future transport crosses a process boundary,
a separately accepted closed versioned envelope with required choice, receipt
and matching seal is needed; current hosted forms remain refused.

The launcher compares the V1 receipt **and** bound plan against its independent
pre-dispatch expectation: same normalized choice/version, exact raw context,
digest, profile/lock, home/XDG/backend, form and effective result. Omitted,
dropped, defaulted, replaced or tampered values at any boundary refuse. In
particular, `allow` dropped to `strict` must refuse for fidelity, even if the
selected build could otherwise pass strict; a stricter accidental launch is not
successful fulfillment of an explicit request. A complete old consumer cannot
parse the new launcher option; a new consumer with old SAM cannot call the new
entry; a new SAM with an old adapter fails capability; a new adapter with an old
seal fails version/binding. None has a fallback-to-native success route.
These guarantees assume the trusted caller retains its expectation; they do
not detect a caller intentionally rewriting its own input before the boundary.

**Actual dependency state:** this repository pins **v0.5.74**, which does not
provide these proposed APIs/tokens. There is **no agreed/published compatible
SAM version in this candidate**. Its source owner must accept the exact carrier,
decision, capability and seal design, resolve source overlap/review and the push
window, publish the reviewed compatible release under a new unused version, and
name its exact commit/tag/capability in a follow-up delivery record. The launcher
then pins that exact publication; neither `>=v0.5.66`, an assumed next tag,
local replacement nor a launcher-only conditional establishes this capability.

The ordered custody gates are:

1. Curator source owner accepts and publishes environments/CIP-0017 amendments;
   raw v4 schemas/conformance remain frozen. Source acceptance is still pending.
2. Existing SAM source custody agrees the engineering interfaces, overlap,
   independent review and publication window. No second SAM writer is created.
3. SAM publishes the accepted compatible source/version; launcher owner receives
   exact dependency evidence and a separately bounded implementation assignment.
4. Launcher implements only the agreed CLI/carrier/composition/provenance path,
   tests it under actual admission, and delivers independently reviewed signed
   source with real required checks and a versioned artifact.
5. Existing host custodian separately admits artifact adoption and native
   acceptance on an exact installed tuple. No credential-diagnostic investigation,
   account repair or host operation is part of this candidate. A usable Muse
   review route and its admission are separate from authoring this document.

## 6. Paired contract witnesses and later qualification plan

**Manual source counterexamples / future tests only: none executed here.**
Expected verdicts below are derived from the rules above, not from two
implementations agreeing. Define frozen independent input `F`: the original
valid v4 fragment bytes and digest, raw fact true, profile/name/lock, precedence,
managed parent, four XDG values, inherited HOME, file backend and all admitted
context-channel data. Muse currently admits no system-prompt/MCP fragment
members; do not create one as a positive. Freeze expected values **before** any
mutation and retain them independently of requests, receipts and adapter output.
Use preprovisioned temporary fixtures so rejection witnesses expect unchanged
profile/home/auth/config bytes and zero forbidden writes, provider starts,
manager starts, fallback attempts or retries. Existing resolve/repair semantics
are unchanged; the allowance itself authorizes no write, repair or extra read.
Ordinary separately admitted planner probes must be counted separately from
provider-session starts; this plan authorizes no probe execution now.

| Pair | Positive control and independent expected verdict | Negative/counterexample and intended refusal |
| --- | --- | --- |
| W1 strict default | `F`, option absent, qualified interactive `1.4.7-R6101.1`: strict receipt and exactly the existing interactive exclusion; all other gates pass | Same request on unqualified interactive 1.4.14: existing exclusion-unqualified refusal, no start; exec-only qualification is insufficient |
| W2 explicit MIXED | Same `F`, explicit allowance, native interactive and otherwise admitted binary/model/permission: MIXED receipt, no exclusion/enabling argv, same raw `exclusive=true`; both native/yolo tested | Same request without allowance on the unqualified build refuses as W1; yolo alone must not select MIXED |
| W3 preservation | Qualified strict and allowed variants with identical `F`: exact original bytes/digest, fact/profile/lock, precedence, HOME/four XDG/file backend/channels before/after composition and execution preparation | Drop root fact, set false/null/extra, change fragment/digest, profile/lock, home/backend or channel: raw validation or expected-identity refusal; no repair/fallback |
| W4 scope/value | One exact V1 choice on native interactive Muse | Hosted/exec/serve/dry-run/other runtime, missing managed fact, duplicate/unknown/coerced choice or mode/form mismatch: usage/scope/carrier refusal, never start another form |
| W5 capability | New entry + matching adapter capability/interfaces + V1 receipt/seal | Old SAM API, missing/unknown adapter capability, token-only stub, missing/old seal or unsupported transport: unsupported-capability/version refusal; no legacy retry |
| W6 fidelity | Immutable caller expectation survives vendor and system-only routes, preparation, result copy and final verification | Drop/replace/default policy at each boundary, including allow-to-strict on a strict-qualified build, or tamper raw context while echoing old digest: fidelity refusal, not a different successful launch |
| W7 native argv | Valid opaque prompt data remains data; SAM supplies strict exclusion or omits it for MIXED | Policy-affecting exclusion/enabling/config options in option position, equivalent spellings, or form-switch command words: argv-policy/form conflict; no suffix stripping |
| W8 seal | Unchanged final process/receipt verifies; mutating a returned provenance/receipt view cannot alter the sealed basis | Swap choice/fact/digest/receipt after plan, change argv/binary/env/help/build, remove verifier or overlay a different home: seal/finalization refusal before provider start; no reseal |
| W9 unrelated gates | Valid MIXED with admitted model/effort and each otherwise valid permission posture | Invalid/unsupported model/effort, yolo without required help, permission lock conflict, malformed build, network/context/backend refusal: same independent refusal as strict; no generic mixed bypass |
| W10 absent-fact baseline | Legacy strict valid Muse v4 with no declared root fact: neither exclusion, residual retained, no content inspection | Explicit allowance without the required fact refuses; manufacturing a nil context to bypass strict must fail W3/W6 |

Eventual bounded detector controls MUST run only after separate test admission,
in isolated disposable source/fixture copies, one plant per run, with finite
wall/lifetime bounds, owned-process cleanup and unchanged positive controls.
No live profile/auth files or installed providers are mutation targets.

| Planned plant | Intended failing assertion (not any error/crash) | Nearby valid control |
| --- | --- | --- |
| Strip `RootContext` only for allowance or change the original digest | W3 independently frozen full `F` comparison must fail at the changed member | Unmodified strict/allow share exact `F` |
| Default every request to allowance, or delete strict qualification | W1 unqualified strict must fail its exact refusal/no-start assertion; W2 yolo-alone must not be MIXED | Qualified strict still emits exclusion; explicit MIXED remains eligible |
| Drop allowance in vendor passthrough or return a strict receipt | W6 must fail fidelity even on the qualified strict build, preventing an unrelated qualification error from masking the plant | Unmodified V1 echo through vendor and system-only paths |
| Ignore adapter capability or accept an old seal | W5 must fail its exact unsupported/version/no-start assertion | Fully capable V1 adapter and seal |
| Skip native conflict scan or skip execution-time binding verification | W7 conflict / W8 changed-choice-or-process must fail their specific refusal assertion | Unchanged allowed process and valid prompt-data suffix |
| Return early from validation for MIXED | W9 must fail the preserved model/permission/context refusal and no-effect assertions | Valid allowed request with every unrelated gate admitted |

A plant not applied, a surviving plant, skipped test or unrelated setup failure
is not a killed control. Record the actual failing assertion and unchanged
positive result, exact source/dependency identities and exits. These source
plans prove no native loading, authentication, billing, refresh, exclusion on
an unqualified release, installed-consumer support or runtime readiness.
