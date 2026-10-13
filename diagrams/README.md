# Launcher figures

These PlantUML sources illustrate the [launcher specification](../SPEC.md).
Revision: accepted architecture-review findings (2026-10-13), plus v0.3.0
release alignment. No implementation, qualification or activation is claimed.

Each figure separates the current interactive API (direct and ax launches;
hosted launch unavailable in v0.3.0 until the swarma dispatcher path ships)
from the target agent-account one-shot mode (§10). The target is **not implemented**;
dispatcher Slice 0 is a Unix-socket service from the first slice and admits
local headless work only; queueing, events and long waits come later on the
same transport. Unpublished FD3 staging, intake, receipt and host admission
formats remain open
questions in [§§10.1–10.6](../SPEC.md#101-immutable-input-descriptor-transport-and-limits).

| Figure | Defining spec sections | Phase |
|---|---|---|
| [Components and process authority](components.puml) | [§1.1](../SPEC.md#11-front-doors-and-execution-authority), [§2](../SPEC.md#2-discovery-and-naming), [§4](../SPEC.md#4-composition-algorithm), [§§4.6–4.9](../SPEC.md#46-hand-off-and-exec), [§§10.1–10.6](../SPEC.md#102-caller-dispatcher-privileged-launcher-and-executor) | Current interactive; hosted unavailable; target one-shot (dispatcher Slice 0). |
| [Launch routes and one-shot path](launch-paths.puml) | [§4](../SPEC.md#4-composition-algorithm), [§4.6](../SPEC.md#46-hand-off-and-exec), [§§4.8–4.9](../SPEC.md#48-host-selection), [§6](../SPEC.md#6-errors-and-diagnostics), [§§10.1–10.6](../SPEC.md#101-immutable-input-descriptor-transport-and-limits) | Current interactive; hosted unavailable; target one-shot (dispatcher Slice 0). |
| [Invocation states, refusal and recovery](launch-states.puml) | [§1](../SPEC.md#1-scope-and-non-goals), [§4](../SPEC.md#4-composition-algorithm), [§4.6](../SPEC.md#46-hand-off-and-exec), [§4.9](../SPEC.md#49-hosted-payload-and-receiver-handoff), [§6](../SPEC.md#6-errors-and-diagnostics), [§§10.1–10.6](../SPEC.md#101-immutable-input-descriptor-transport-and-limits) | Current interactive; hosted unavailable; target one-shot (dispatcher Slice 0). |
| [Artifact ownership, transport and cleanup](artifact-layout.puml) | [§4.1](../SPEC.md#41-obtain-the-fragment-context-plane), [§§4.4b–4.7](../SPEC.md#44b-resolve-the-network-profile-network-plane), [§4.9](../SPEC.md#49-hosted-payload-and-receiver-handoff), [§9](../SPEC.md#9-open-items), [§§10.1–10.6](../SPEC.md#101-immutable-input-descriptor-transport-and-limits) | Current interactive; hosted unavailable; target one-shot (dispatcher Slice 0). |

The specification defines the contracts; the figures also link back to the
relevant sections. Persistent state belongs to the named owning components.
Launcher invocation buffers are transient, and credential material remains
separate from metadata.

The user manager's spec §7 and `diagrams/launch.puml` own the single
privileged descriptor lifecycle (R-FD3); the target panels reference it.
The executor receives caller-owned 0–2 and read-only FD3 only. Joint
publication of bounded staging gates activation. Only the dispatcher calls
the privileged launcher; the target host requests `host_start` with a
session-scoped host admission permit for one `logical_session_id`.
Session lifecycle authorisation may permit successors; each physical start
keeps its own claim/fence/receipt interlock, and the previous claim must be
confirmed absent first. `host_stop` uses journaled E-stop, confers no start
authority and stays distinct from owner `cancel`. Joint publication and
qualification remain required.
Cross-account workspaces are standalone repositories with private Git
administration; linked worktrees stay within one account. R-TRANSFER assigns
materialization/export to the dispatcher's workspace contract and import/exact-candidate review/integration to the board's
change-request machinery; exact formats remain open (§10.5). The user manager
owns the single termination scope (§§4.4/4.7) and process fate table (§7, R-FATE), including
the replaced-daemon writer fence. R-TIME gates expiry-driven home deletion
and binding removal on boot-inclusive monotonic elapsed lifetime as well
as expiry; the dispatcher/broker own their durable time watermarks.
Ordinary retirement deletes the home after quiesce and declared-key cleanup;
R-TRANSFER workspace export must be recorded or explicitly operator-waived
before home deletion. Separate retirement metadata export remains restricted
to service-generated closed-schema metadata without agent-controlled data.
Separate operator home retention does not guarantee confidentiality.

All target panels carry the plan, exec-profile, resource, surface-drift
and web opt-in obligations (SPEC §§10.1–10.2). They show protected-machine
exec selection and immutable identity/body pinning, full-invocation and
transitive-input qualification, and the mode-aware manifest from immutable B.
Materialization checks archive-to-manifest equality; export compares mode
and content; board import independently recomputes the manifest from B.
Result fields are the dispatcher's draft (§10.5 owner-linked index), while
transport/lifecycle and qualification remain gated. Current panels show
`session_host_missing` (exit 1) before any hosted handoff, even with task-board
on PATH; earlier validation refusals and native/ax routes remain. The sequence and state figures
show materialization before work and export after account-wide absence,
before retirement. SPEC §10.6 references admission/abuse-control owners;
these sources claim neither enforcement nor rendered qualification.
