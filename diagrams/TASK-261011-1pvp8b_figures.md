# TASK-261011-1pvp8b — R-LR2 figure handoff

Docs-only candidate, ready for review. No product behavior changed.

| Figure | File | Defining spec sections | Audit row |
|---|---|---|---|
| Launcher components and process authority | [components.puml](components.puml) | [§1.1](../SPEC.md#11-front-doors-and-execution-authority), §§2, 4, 4.6–4.9, [§§10.2–10.5](../SPEC.md#102-caller-dispatcher-privileged-launcher-and-executor) | B4 / M-LR1 |
| Current launch routes and target one-shot path | [launch-paths.puml](launch-paths.puml) | [§4](../SPEC.md#4-composition-algorithm), §§4.6, 4.8–4.9, 6, [§§10.1–10.5](../SPEC.md#101-immutable-input-descriptor-transport-and-limits) | B4 / M-LR1 |
| Invocation states, refusal and recovery | [launch-states.puml](launch-states.puml) | [§1](../SPEC.md#1-scope-and-non-goals), §§4, 4.6, 4.9, 6, [§§10.1–10.5](../SPEC.md#101-immutable-input-descriptor-transport-and-limits) | B4 / M-LR2 |
| Artifact ownership, transport and cleanup boundaries | [artifact-layout.puml](artifact-layout.puml) | [§4.1](../SPEC.md#41-obtain-the-fragment-context-plane), §§4.4b–4.7, 4.9, 9, [§§10.1–10.5](../SPEC.md#101-immutable-input-descriptor-transport-and-limits) | B4 / M-LR2 |

All four sources are linked from README and SPEC §1.1, with further links at the relevant composition, handoff, configuration, hosted and target sections. Each source links back to the defining spec sections. Current direct/ax/hosted paths and target agent-account one-shot panels are explicitly separate.

Source baseline:

- [Audit at 48789217, B4 rows M-LR1–M-LR2](https://github.com/relux-works/curator/blob/48789217/.research/261010_platform-docs-audit.md#b4-missing-or-insufficient-module-diagrams--exact-proposed-files): exact four filenames, kinds and participants.
- [Launcher SPEC at 1d608047](https://github.com/relux-works/curator-agent-launcher/blob/1d608047/SPEC.md): current checkout baseline including the target amendment, plus its README for direct wait, terminal forwarding/restoration and signal exit behavior.
- [Contract table at 177cb554](https://github.com/relux-works/curator/blob/177cb554/.research/261010_platform-contract-table.md): baseline ownership and message bounds; SPEC §10 records the overriding owner decisions. Its earlier open P1/N6/C9 choices are not presented as current blockers.

Visible bounds / findings:

- Existing interactive API is current; hosted Phase 1 is Claude/terminal-only. Target one-shot remains unimplemented and unqualified, dispatcher v0 local headless only; v1 daemon supervision and later provider lanes are separately labelled. SH2 ticket activation remains a publication question.
- The amendment is included as instructed. SPEC §10 still reserves supported release/wire publication before activation; figures retain that exact limitation rather than infer activation from amendment availability.
- Intake envelope/digest placement and dedicated invocation, consumed-plan/exit evidence transport, ownership across exec, numeric refusal exits and signal encoding remain unspecified in SPEC §§10.1, 10.4–10.5. No invented receipt format, post-exec emitter or renewal loop appears.
- Service/destination UIDs, hosted durable layout, native snapshot removal and agent-home materialization details are labelled unspecified where the launcher spec does not define them.
- Persistent state belongs to Curator, ax, dispatcher, root helper/launcher or broker as named. Launcher invocation buffers are transient; credential material is execution-scoped and separate from metadata. Unknown process evidence does not release capacity or permit replacement.
- Recovery is a deliberate new interactive invocation/Curator repair or Claude re-enrollment/restart, where specified; refusal never downgrades the launch. No automatic recovery from unknown one-shot process evidence is invented.

Verification: source-to-spec reading and diff inspection only. No renderer is available on this host; conservative PlantUML is used, without includes/themes/macros. Tests, lint, builds and renderer commands were not run by this producer, per the task's exclusive docs-only brief; configured CR validation is delegated to `task-board handoff`. No passing command gate is claimed. No LOGBOOK edit, per the explicit brief. Changes remain uncommitted for handoff.
