# Launcher figures

These PlantUML sources illustrate the [launcher specification](../SPEC.md).
Each figure separates the current interactive API (direct and ax launches,
plus hosted Phase 1: Claude with terminal stdin only) from the target
agent-account one-shot mode (§10). The target is **not implemented**;
dispatcher v0 is a Unix-socket service from the first slice and admits
local headless work only; queueing, events and long waits come later on the
same transport. Unpublished FD3 staging, intake, receipt and host admission
formats remain open
questions in [§§10.1–10.5](../SPEC.md#101-immutable-input-descriptor-transport-and-limits).

| Figure | Defining spec sections | Phase |
|---|---|---|
| [Components and process authority](components.puml) | [§1.1](../SPEC.md#11-front-doors-and-execution-authority), [§2](../SPEC.md#2-discovery-and-naming), [§4](../SPEC.md#4-composition-algorithm), [§§4.6–4.9](../SPEC.md#46-hand-off-and-exec), [§§10.2–10.5](../SPEC.md#102-caller-dispatcher-privileged-launcher-and-executor) | Current interactive; hosted Phase 1; target one-shot (dispatcher v0). |
| [Launch routes and one-shot path](launch-paths.puml) | [§4](../SPEC.md#4-composition-algorithm), [§4.6](../SPEC.md#46-hand-off-and-exec), [§§4.8–4.9](../SPEC.md#48-host-selection), [§6](../SPEC.md#6-errors-and-diagnostics), [§§10.1–10.5](../SPEC.md#101-immutable-input-descriptor-transport-and-limits) | Current interactive; hosted Phase 1; target one-shot (dispatcher v0). |
| [Invocation states, refusal and recovery](launch-states.puml) | [§1](../SPEC.md#1-scope-and-non-goals), [§4](../SPEC.md#4-composition-algorithm), [§4.6](../SPEC.md#46-hand-off-and-exec), [§4.9](../SPEC.md#49-hosted-payload-and-receiver-handoff), [§6](../SPEC.md#6-errors-and-diagnostics), [§§10.1–10.5](../SPEC.md#101-immutable-input-descriptor-transport-and-limits) | Current interactive; hosted Phase 1; target one-shot (dispatcher v0). |
| [Artifact ownership, transport and cleanup](artifact-layout.puml) | [§4.1](../SPEC.md#41-obtain-the-fragment-context-plane), [§§4.4b–4.7](../SPEC.md#44b-resolve-the-network-profile-network-plane), [§4.9](../SPEC.md#49-hosted-payload-and-receiver-handoff), [§9](../SPEC.md#9-open-items), [§§10.1–10.5](../SPEC.md#101-immutable-input-descriptor-transport-and-limits) | Current interactive; hosted Phase 1; target one-shot (dispatcher v0). |

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
administration; linked worktrees stay within one account. Source/result
transfer ownership remains open (§10.5). The user manager owns the single
termination scope (§§4.4/4.7) and process fate table (§7, R-FATE), including
the replaced-daemon writer fence. R-TIME gates expiry-driven home deletion
and binding removal on boot-inclusive monotonic elapsed lifetime as well
as expiry; the dispatcher/broker own their durable time watermarks.
Ordinary retirement deletes the home after quiesce and declared-key cleanup;
export is restricted to service-generated closed-schema metadata without
agent-controlled data. Separate operator home retention does not guarantee
confidentiality.
