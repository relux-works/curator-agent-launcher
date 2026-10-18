# curator run as the front door — specification amendment

**Amends:** [SPEC](SPEC.md) `0.5.1-draft` → `0.6.0-draft` (target; nothing
here is implemented yet).
**Status:** revision 7, draft. The independent review accepted revision 6 (f8c2756); 6.1 adds only its three non-blocking clarifications. 6.2 adds the model-selection seam (§3.4) at the router lane's request. 7 folds in the task-board host review TASK-261018-m9kukq, findings F1–F14 (§§3.3, 5, 8, 11, 12). 6.4 revises §3.4 after the router lane's review. 6.3 adds infra-exit's conditions: admission composes with digest admission, and `busy` is defined per runtime. `ivmbp_curator_orch` accepted revision 1
(0ed45c5) as the basis on 2026-10-18 with four must-change points. Revision 2
folds those in, together with the independent reviews `review-1` and
`review-2` and `review-3` (all rework, converging). Revision 3 also adopts the accepted
`protocol/local-setup-token-source.md`, which revision 2 had missed (§9). Implementation of S1–S3
is authorised against the fake manager.
**Board:** `relux-works/curator-run-board`, EPIC-261018-cuz7sp `front-door`,
TASK-261018-st9yo0.

Five shape questions went to `ivmbp_curator_orch` on 2026-10-18. It decided
Q2–Q5 as Ivan's delegate, as recommended; those sections are marked
**[decided Qn]**. Q1 (`launch-delegated/1`) is recommended by
`ivmbp_curator_orch` and **needs Ivan's sign-off**; those sections are marked
**[pending Q1, Ivan]**. §15 records the answers. §0.1 lists every change this
amendment proposes to an accepted text.

## 0. Sources and decisions applied

| Source | What it fixes here |
| --- | --- |
| Ivan, mission brief, 2026-10-18 | The seven deliverables: hosted by default, pluggable session managers, `werkstatt up/down`, Muse, setup-token, doorbells, signed distribution. The managed home comes from the profile and has no binding to a board's primary goal. `--native`. Mode `auto`. Machine config `hosted\|native\|auto`. |
| Ivan, via `ivmbp_curator_orch`, 2026-10-18 (binding) | A session manager registers once, at install. It is automatic when no other manager is registered. When one or more are, the installer asks once, and an agent installer asks its human. After that the choice is fixed in config, with no prompt on any launch. curator-run is installed first, together with Curator, in the signed distribution. Without a session manager an ordinary launch goes native and prints one line, and `werkstatt up` refuses. |
| Curator Decision 0010 §6 | The launcher composes three planes: spawn (agents-management), context (`curator env resolve`) and session (now a session-manager plugin). |
| Decision 0021 and its amendment v2; CIP-0016 / `protocol/local-hosted-launches.md` | One interactive entry point. A host never composes in the exact-plan mode. No fallback after a hosted refusal. Snapshots, not live homes, for exact-plan sessions. Typed resume that never becomes a new session. Section 8 question 2 (backend configuration) is answered by §4 here. |
| curator-spec `protocol/local-setup-token-source.md` revision 1 (accepted 2026-10-16) | The setup-token source, custody, binding, final boundary and API (`pkg/localcredential`). Curator owns all of it; curator-run consumes it (§9). |
| CIP-0010 (draft) C3, C6 and its test plan | setup-token: a user-owned source choice that no system lock can select; a protected binding re-resolved by the final executor; every competing source refuses; injected names never reach MCP children; nothing in plans, fragments or logs. |
| CIP-0017, environments §10.2 | `launch-env-fragment-v4`, Muse `root_context`. |
| environments §9.4, §11 | `curator-*` providers come only from Curator's trust roots, never from profiles. Revision A selects by `PATH` with a warning; revision B refuses outside the roots. |
| Swarm config-home convention (Ivan, 2026-10-18) | `${XDG_CONFIG_HOME:-~/.config}/swarma/<element>/`, `/etc/swarma/<element>/`, `${XDG_STATE_HOME:-~/.local/state}/swarma/<element>/`. Precedence: Werkstatt, then user, then system, then default. The old path is read for one release. |
| Werkstatt note (`swarma-docs` `editorial/werkstatt.ru.md`) and pilot runbook | The Werkstatt model, the clone with the git dir outside the worktree, no `CLAUDE.md`/`AGENTS.md` above the clone root, context modes, one setup-token for every home, no Remote Control. |
| Router lane, `#model-routing` › status, 2026-10-18 | Station snapshot fields and the liveness vocabulary `idle\|busy\|starting\|draining\|down`. Dispatch returns (station, effort). |
| a2a `docs/cli.md` | Hook doorbells; `runtime bind/gate/abandon`, with abandon only after the registered owner is dead. |

### 0.1 Changes to accepted texts

Each row is a change this amendment proposes. None takes effect until the
owner of the changed text accepts it. "Decided" means `ivmbp_curator_orch`
decided it as Ivan's delegate.

| # | Changed text | Change | Disposition |
| --- | --- | --- | --- |
| C1 | CIP-0016 / Decision 0021 amendment v2 (exact plan only; snapshots) | A transitional `launch-delegated/1`: the manager composes, against the live managed home | Q1, pending Ivan; Appendix A |
| C2 | CIP-0016 §1 (Claude, Codex and Muse qualify together for the first release) | Kept for `launch-exact/2`. For `launch-delegated/1`, activation goes runtime by runtime: Claude and Codex first, Muse when task-board's v4 consumer lands. | Part of Q1, pending Ivan |
| C3 | Decision 0021 §7, SPEC §4.8 (machine hosted defaults refuse until upgrade without hang-up) | The machine mode replaces the gate; upgrade behaviour becomes a disclosure line | Q2, decided |
| C4 | CIP-0016 §2 (explicit `--hosted` with no backend selection means `local-task-board`, stable across upgrades) | The registry replaces the implicit backend. With no registered manager, `--hosted` refuses. task-board's installer registers it, so a machine with task-board installed keeps the meaning. | Proposed here; falls under C1's amendment |
| C5 | Decision 0021 §4 (board behaviour follows the workspace) | The board goal is bound only on explicit `--board-goal` | Proposed here (mission: no board goal binding) |
| C6 | SPEC §4.3 (`defaults.json` per-env `host`) | Withdrawn in favour of the machine `host_mode` | Proposed here |
| C7 | SPEC §§4.3, 4.7 paths (`~/.config/curator-run/`, `/etc/curator-run/`) | Move to `swarma/launcher/`, with one release of fallback | Q3, decided |
| C8 | environments §11 (no provider-specific version coupling) | Curator's release reports the bundled `curator-run` version (§10) | Proposed; owner `ivmbp_curator_orch` |
| C9 | Decision 0021 open question 4 (how a non-task-board host learns about boards) | Answered by `board-goal/1` and an explicit request member | Proposed here |
| C10 | `local-setup-token-source.md` §7 (interactive PTY separately qualified; hosted token route needs an amendment; no daemon lookup) | Two new routes: direct local interactive, and delegated hosted through a per-session owner calling Curator's API | Proposed; Appendix B; owner `ivmbp_curator_orch`. This supersedes the Keychain answer to Q5, which conflicted with "no node Keychain backend". |

Not changed: the CIP-0016 receiver transport (private plan stdin, terminal
fd3, status fd4, the §4.9 framing and classification). `launch-exact/2`
uses it byte for byte (§5.1).

## 1. Goal and planes

`curator run` is the one front door for people and agents. Every interactive
session starts there. task-board, and any other session manager, is the
technical API underneath. People and agents do not call it directly to start
sessions.

```text
                curator run <env> | curator run werkstatt up <id>
                                   │
        ┌──────────────────────────┼──────────────────────────────┐
  context plane              spawn plane                    session plane
  curator env resolve        agents-management module        session-manager plugin
  (profile → fragment,       (plan: binary, argv, env;       (host, terminal, resume,
   managed home)              model/effort admission)         liveness, doorbell)
```

The launcher keeps its non-goals (SPEC §1). It has two kinds of
session-related state: the Werkstatt liveness record (§6.5) and the pending
operation journal `op.json` (§5.7). Both are observation and correlation,
not authority. The session manager remains the truth about processes.

## 2. Surface

```text
curator-run <env-id> [--hosted | --native] [--board-goal]
            [--a2a <identity>] [existing SPEC §3 flags] [--] <native args...>

curator-run werkstatt up <id> [--attach] [--new] [--task-class C [--difficulty D]]
curator-run werkstatt down <id> [--force]
curator-run werkstatt recycle <id>
curator-run werkstatt status [<id>] [--json]
            [--set-task <ID> | --clear-task] [--queue-depth N] [--abandon-operation | --rebind-manager]
curator-run werkstatt list [--json]

curator-run session-manager list [--json]
curator-run session-manager show <id> [--json]
curator-run session-manager register --manifest <path|-> [--select | --no-select]
curator-run session-manager select <id>
curator-run session-manager unregister <id>

curator-run migrate [--dry-run]
```

- `werkstatt`, `session-manager` and `migrate` are reserved
  operands. They are not environment ids. A future environment id with one
  of these names is a specification conflict.
- `--untracked` remains an exact synonym of `--native` (SPEC §3).
- `--board-goal` (new) asks the session manager to bind the workspace
  board's primary goal (§3.3). Without it, no goal is bound.
- `--a2a <identity>` (new) asks for doorbell wiring for that a2a identity
  (§8). It is valid only with a hosted launch. With `--native` it is a
  `usage` error.
- The existing hosted selectors stay: `resume [SES-HANDLE]` and
  `--resume <id>` (SPEC §§3, 4.8). They travel as typed intent (§5.3), never
  in the native tail.
- The `curator run …` umbrella spelling and the direct `curator-run …`
  spelling stay the same surface (SPEC §2).

## 3. Host mode: hosted by default

### 3.1 The machine mode

The host mode is machine policy. It lives in `launcher.json` (schema
`curator-run-launcher-v1`, closed) in the system layer (§4.1):

```json
{ "schema": "curator-run-launcher-v1", "host_mode": "auto" }
```

`delegated_trial`, an optional boolean, is the only other member. It is
system-layer only and is defined in §5.8.

- `host_mode` is exactly `hosted`, `native` or `auto`. Any other value, or
  an unknown member, is `defaults_config_invalid`.
- An absent system file, or one without `host_mode`, means `auto`.
- A user-layer `launcher.json` that carries `host_mode` is
  `defaults_config_invalid`, and the detail names the system path. The user
  layer has no `host_mode`.
- Both layers get the SPEC §4.7 file checks.

The SPEC §4.3 `defaults.json` per-environment `host` member is withdrawn
(C6). A v3 file that still carries `host` refuses `host_member_withdrawn`
(exit 2), naming `launcher.json`. `curator-run migrate` removes it (§13).

### 3.2 Resolution

"Selected manager" is defined in §4.2. The per-launch flag decides first,
then the machine mode. Closed table, first match wins:

| Flag | Machine mode | Registry state (§4.2) | Result |
| --- | --- | --- | --- |
| `--native` | any | any | Native. Zero registry read, zero manager contact. |
| `--hosted` | any | selected | Hosted through the selected manager. |
| `--hosted` | any | empty, or unselected | Refuse `session_host_missing` (1), with the detail `no manager registered` or `no manager selected`. |
| none | `native` | any | Native. Zero registry read. |
| none | `hosted` | selected | Hosted. |
| none | `hosted` | empty, or unselected | Refuse `session_host_missing` (1), with the same details. |
| none | `auto` | selected | Hosted, subject to the activation boundary and its capability exception (§5.8). |
| none | `auto` | empty | Native, and exactly one stderr line, `notice-empty`. |
| none | `auto` | unselected | Native, and exactly one stderr line, `notice-unselected`. |

The two `auto` lines are fixed:

```text
notice-empty:      curator-run: no session manager registered; launching natively (curator run session-manager list)
notice-unselected: curator-run: no session manager selected (<N> registered); launching natively (curator run session-manager select <id>)
```

**Resolution algorithm.** This is the one order. The first refusal wins.

1. Parse the flags, then read `ax.json` and the system `launcher.json`.
2. `--native` (or `--untracked`), or `host_mode: native` with no flag:
   native. No registry read, no hosted checks.
3. Explicit `--hosted`, or `host_mode: hosted` with no flag:
   1. if `ax.json` is enabled, `host_configuration_conflict` (2);
   2. the SPEC §4.8 hosted checks: `network_scope_unsupported` (16),
      `session_host_terminal_required` (6), `session_host_stdin_unsupported`
      (6);
   3. the registry: invalid is `session_manager_registry_invalid` (2);
      empty or unselected is `session_host_missing` (1);
   4. `describe` and the capability checks (§5.2);
   5. the activation matrix (§5.8).
4. `host_mode: auto` with no flag:
   1. the registry: invalid is `session_manager_registry_invalid` (2), never
      empty;
   2. empty or unselected gives native, plus its notice. If `ax.json` is
      enabled, `ax` routing applies;
   3. selected:
      1. if `ax.json` is enabled, `host_configuration_conflict` (2);
      2. the hosted checks of step 3.2;
      3. `describe` and the capability checks;
      4. the activation matrix. Its native row gives `notice-unselected`.

   After the registry read, nothing in `auto` turns a refusal into native.
   The matrix row is decided by capability, not by a failure.

`werkstatt up` follows step 3, except that it needs no terminal unless
`--attach` is given.

Rules that carry over:

- A hosted refusal never falls back to native, to another manager, or to
  another runtime (CIP-0016 §2).
- `--native` beats the machine mode. The mode is a default for launches,
  not a lock (`host_mode_locked` is open, §15).
- Hosted permissions stay as in SPEC §4.8 (Q-D3).

### 3.3 The managed home and the board goal

- **The managed home is the profile's**: `--profile <name>`, else Curator's
  current profile for the scope. The launcher resolves it through
  `curator env resolve <env> [--profile P] --repair --format json` (SPEC
  §4.1) before any manager contact. Nothing binds it to a board.
- **A hosted session binds no board goal by default (C5).** The request
  carries `"board_goal": "none"`; with `--board-goal` it carries
  `"workspace"`.
  - A manager without `board-goal/1` refuses at `describe` with
    `session_host_capability_missing` (6), before any launch.
  - A manager with it that finds no board for the workspace refuses
    `session_board_goal_unavailable` (1).
  - `workspace` resolves the workspace board and reserves that board's
    existing `parent/primary` goal slot through **that board's own
    manager**. The manager routes the launch there and keeps a private
    routing index for `session` and `operation_id`. Another live holder of
    the slot refuses, and nothing takes the slot over.
  - Goal-less sessions live in the board-less user-state manager. Its key
    and namespace are separate from every board's key, so the two never
    collide.
  - It never binds a goal silently.
  - The `task-board claude|codex` aliases pass `--board-goal` for
    compatibility.
- **[decided Q2] Upgrade without hang-up.** The machine mode replaces the
  old defaults gate (C3). A hosted launch through a manager that advertises
  neither `survives-upgrade` nor `drains-on-upgrade` proceeds and prints one
  line: `curator-run: session manager <id> ends hosted sessions when it is
  upgraded`. The code `session_host_default_not_ready` is retired.

### 3.4 Model-selection seam

A launch's runtime, model and effort can be chosen by an external selector
before composition, such as the model router (`cmr`). The selector can move
to this seam without wrapping the CLI.

**Configuration.** `selector.json`, schema `curator-run-selector-v1`,
closed, system layer only:

```json
{ "schema": "curator-run-selector-v1", "mode": "shadow|enforce",
  "executable": "/abs/path/cmr", "argv_prefix": ["select"],
  "timeout_seconds": 5 }
```

When the file is absent, the seam is off. `timeout_seconds` is an integer
from 1 to 60 and applies to `enforce` only.

**Request.** One JSON object on stdin, at most 64 KiB:

```json
{ "env_id": "codex_cli", "profile": "…", "cwd": "/abs",
  "role": null, "project": null, "werkstatt": null,
  "task": { "id": null, "class": "code.implement", "difficulty": "high",
            "sensitivity": null, "language": "go", "platform": null },
  "flags": { "model": null, "effort": null },
  "candidates": [ { "runtime": "codex_cli", "model": "…", "effort": "…" } ],
  "admission_source": "agents-management vX.Y.Z lineup" }
```

- `task` is `null` for a plain launch. For a Werkstatt it comes from the
  task the orchestrator dispatches, as `werkstatt up --task-class C
  [--difficulty D]`.
- `candidates` is the admitted set, computed by the launcher from the
  spawn plane's admission. That is the same lineup and runtime
  compatibility registry that SPEC §4.3 uses. The selector ranks only
  inside this set and never queries a board.

**Response.** Bounded and validated like a §5.6 response:

```json
{ "decision_id": "…", "runtime": "…", "model": "…", "effort": "…",
  "weights_id": null, "refusal": null }
```

- Values match `[A-Za-z0-9._:/@+-]{1,128}`. `weights_id` matches
  `gguf-sha256:[0-9a-f]{64}` or another published weights grammar.
- A response with a non-null `refusal` MUST have `runtime`, `model`,
  `effort` and `weights_id` all null. A refusal that also carries a choice
  is invalid (`selector_unavailable`, `reason=invalid`).
- `refusal` is `{code}`, a typed abstention such as
  `no_qualified_candidate`. Its code follows `[a-z0-9_]{1,64}` and is
  displayed as an enumerated value of `selector_abstained`.
- A chosen model outside `candidates` is an invalid response. The
  response's `(runtime, model, effort)` must equal one candidate tuple
  exactly.
- A model without an effort axis carries `effort: null`, in `candidates`
  and in the response. It is never an empty string.
- A null effort selects only an effort-less candidate, and a non-null effort
  is never valid for one.

**Precedence.**

- Explicit flags always win.
- For a Werkstatt, the record's pinned model wins, and the selector may set
  only `effort`, as dispatch returns (station, effort).
- Below those, a selector value enters as a new level, directly above
  `defaults.json`.
- A selector `runtime` that differs from `<env-id>` is recorded only, and
  never applied in v1.

**`shadow` mode.** The launch never waits for the selector.

- The launcher starts the selector detached, with the same request, after
  composition is complete.
- It appends one line to `<state>/selector/observations.ndjson` with:
  - the request digest;
  - the applied runtime, model, effort and `weights_id`;
  - the launch outcome: `launched`, `refused:<code>` or `hosted:<handle>`;
  - the selector's `decision_id` when it answers. A late answer is joined
    by request digest.
- A selector failure never affects the launch. Composition is
  byte-identical to a launch with the seam off.
- Observation writes never block the launch.
  - The launcher takes the append lock only by a non-blocking attempt.
  - If the lock is contended, or the write fails, the line is dropped. The
    count of dropped lines is kept in `<state>/selector/dropped`, and that
    write is best effort as well.
  - The detached child may wait for the lock, but only up to its own 5 s
    bound.

**`enforce` mode.** The selector runs synchronously, within
`timeout_seconds`.

- A valid choice applies, under the precedence above.
- A typed abstention refuses `selector_abstained` (1), with the detail
  `code=<selector code>`.
- A transport failure, timeout or invalid response refuses
  `selector_unavailable` (1).
- Neither case falls back to defaults: a configured enforcing selector is
  policy.

**Provenance.** The `decision_id` is passed on as the label
`selector=<decision_id>` in hosted requests, and shown in the §4.3
line-group.

## 4. Session-manager registry

### 4.1 Locations [decided Q3]

| Layer | Directory | Who writes |
| --- | --- | --- |
| System | `/etc/swarma/launcher/` | A privileged installer only (root-owned files, mode 0644) |
| User | `${XDG_CONFIG_HOME:-~/.config}/swarma/launcher/` | The operator and the operator's installers (files owned by the user, no group or world write) |
| State | `${XDG_STATE_HOME:-~/.local/state}/swarma/launcher/` | curator-run (Werkstatt liveness, locks) |

- **Fallback:** for one release, a file name that is absent in the new
  directory is read from the old `/etc/curator-run/` or
  `${XDG_CONFIG_HOME:-~/.config}/curator-run/`. Each such read prints one
  deprecation line naming the new path.
- **Both present:** a file present in both places is read from the new one.
  The old one is not opened.
- **Checks:** the SPEC §4.7 file checks apply to whichever path is read.
- **Migration:** `curator-run migrate` copies the files (§13). Nothing is
  deleted automatically.

### 4.2 The registry file

`session-managers.json`, schema `curator-run-session-managers-v1`, closed:

```json
{
  "schema": "curator-run-session-managers-v1",
  "selected": "task-board",
  "managers": {
    "task-board": {
      "executable": "/usr/local/bin/task-board",
      "argv_prefix": ["session-manager", "plugin"],
      "protocol": "curator-session-manager/1",
      "registered_at": "2026-10-18T09:00:00Z",
      "registered_by": "task-board installer 0.9.0"
    }
  }
}
```

- **Ids:** `managers` keys match `[a-z0-9][a-z0-9-]{0,31}`.
- **`executable`:**
  - It is absolute and resolved once, at registration, with symlinks
    resolved.
  - The launcher never searches `PATH` for a manager.
  - It never takes an endpoint, socket or binary from a repository, a
    profile, a fragment or the environment (CIP-0016 §2).
- **Required members.** Each entry requires `executable`, `argv_prefix`,
  `protocol`, `registered_at` and `registered_by`.
  - `argv_prefix` has 0–4 elements, each `[a-z0-9-]{1,32}`.
  - `protocol` is exactly `curator-session-manager/1`.
  - `registered_at` is a UTC RFC 3339 timestamp.
  - `registered_by` is 1–128 bytes of printable ASCII, with no control
    characters.

  At top level, `selected` is optional.
- **`selected`:** names one key of `managers`, or is absent.
- **Effective registry:** if the system file exists, it alone decides, and
  the user file is not opened. Otherwise the user file decides. This is the
  same machine-first rule as `ax.json` (SPEC §4.6).
- **Checks:** every read applies the SPEC §4.7 file checks. A file that
  fails them, or does not parse, is `session_manager_registry_invalid` (2).

Registry states for §3.2:

| State | Condition |
| --- | --- |
| **empty** | No effective file exists, or the file has no `managers` entries. |
| **unselected** | `managers` is non-empty and `selected` is absent. |
| **selected** | `selected` names an entry. If that entry's `executable` is missing, is not a regular file, or is not executable, the result is `session_host_missing` (1) with the detail `registered executable missing`. That case is not "empty", so `auto` does not go native. |

### 4.3 Registration happens once, at install

A session manager's installer registers it:

```text
curator-run session-manager register --manifest - [--select | --no-select]
```

task-board exposes this as `task-board session-manager register --for
curator-run`. The manifest is the entry of §4.2 plus `id`, without
`registered_at`, which the launcher fills in.

Rules, closed, applied in order:

1. **Target layer.** A process that can write `/etc/swarma/launcher/`
   writes the system file. Any other process writes the user file. If the
   target is the user file but a system registry exists, the command
   refuses `session_manager_registry_shadowed` (2): the registration would
   be ignored. A privileged installer must register in that case.
2. **Explicit flags win.**
   - `--select` registers and selects.
   - `--no-select` registers without changing `selected`, even when this is
     the first manager.
   - The two flags together are `usage`.
3. **Same id, same executable.** The entry is updated and the selection
   kept. An upgrade never asks again.
4. **Same id, different executable.** The entry is updated, the selection
   kept, and one line printed: `session manager <id>: executable changed
   <old> -> <new>`.
5. **First manager** (registry empty) and no flag: registered and selected,
   without a question, with or without a TTY.
6. **Other managers exist**, the id is new, and no flag is given:
   - With a TTY, the command asks once, naming the current selection and the
     new manager. The default answer keeps the current selection.
   - Without a TTY, it registers without selecting, exits 0, and prints one
     line: `registered <id>; selection unchanged (<current or none>); run:
     curator run session-manager select <id>`. An agent installer has no
     TTY: it asks its human in its own channel, then passes `--select` or
     `--no-select`, or runs `select` later.
7. **Writes are atomic**: a temp file in the same directory, fsync, rename,
   and fsync of the directory.
8. **Mutations are serialised.** `register`, `select` and `unregister` each
   take one exclusive lock, `session-managers.json.lock` in the target
   layer's directory. They hold it across the whole read–decide–write
   transaction. That includes the TTY question: an interactive prompt holds
   the lock, with a 120 s answer deadline that refuses `usage` and leaves
   the file unchanged.
   - A concurrent mutation waits up to 10 s, then refuses
     `session_manager_registry_busy` (1).
   - A mutation that fails after taking the lock leaves the previous file
     byte-identical.
   - Two acknowledged registrations can therefore never overwrite each
     other.

After registration, no launch prompts about managers. `select <id>` and
`unregister <id>` are explicit commands. An unknown id is
`session_manager_not_registered` (2).

All three mutating commands use rule 1 to choose their target layer. A
user-layer mutation while a system registry exists refuses
`session_manager_registry_shadowed`. A new system file starts empty and never
imports the user registry. Unregistering the selected manager
leaves `selected` absent, so the state becomes **unselected**, or **empty**
if it was the last.

### 4.4 Install order

1. The signed Curator distribution installs `curator` and `curator-run`
   together (§10).
2. Session managers install afterwards and register (§4.3).
3. A machine with only step 1 works: launches go native under `auto` with
   `notice-empty`, and `werkstatt up` refuses.

## 5. The session-manager plugin protocol

### 5.1 Transport

A session manager is an executable plugin, in the same family as git
credential helpers. Each operation is one subprocess:
`<executable> <argv_prefix…> <operation>`.

| Operation | stdin | stdout | fd 3 | fd 4 | Terminal |
| --- | --- | --- | --- | --- | --- |
| `describe`, `status`, `list`, `stop`, `cancel`, `launch`, `resume` | request JSON | response JSON | — | — | none |
| `attach` | terminal | terminal | request JSON | status records | fd 0–2 |
| `launch-exact` | CIP-0016 plan payload | terminal | terminal (read/write) | status records | as SPEC §4.9 |

**Request/response operations**

- The request is one JSON object of at most 64 KiB, then EOF.
- The response is one JSON object on stdout, at most 64 KiB, then EOF.
- Exit 0 carries `ok: true`. Any other exit carries a refusal object (§5.6).
- These are `session_host_protocol_error` (6):
  - an exit status that disagrees with `ok`;
  - invalid JSON;
  - a body over the bound;
  - a missing or different `protocol` member;
  - a response that fails §5.6 validation.
- stderr is captured up to 64 KiB and then discarded. Its bytes never reach
  the launcher's output; only their count may appear, in `--json`
  diagnostics.
- No operation carries secrets. Credentials never pass through this
  protocol (§9).

**Terminal operations** (`attach`, `launch-exact`)

- They reuse the SPEC §4.9 status channel exactly: fd 4 carries 32-bit
  big-endian length-prefixed JSON refusal records of at most 64 KiB, with
  registry classification, poison and truncation rules.
- A typed refusal exists only as an fd-4 record. Without one, the exit
  status is the session's own and is never reclassified.
- For `launch-exact`, the argv is
  `<executable> <argv_prefix…> launch-exact --plan - --terminal-fd 3 --status-fd 4`.
  The tail after the operation word, fd 3, fd 4 and the stdin plan are the
  CIP-0016 receiver transport byte for byte, so that shape is unchanged
  (§0.1). The plan payload keeps its own SPEC §4.9 bounds: a 1 MiB object
  and depth 16. The 64 KiB bound applies only to the JSON request/response
  operations.
  - Because it is a lifetime terminal operation, it cannot serve a detached
    launch (`werkstatt up`) or return a session record.
  - Detached exact-plan launches stay unavailable until the CIP-0016
    successor publishes a detached form. A launch that needs detach through
    an exact-only manager refuses `session_host_capability_missing` (6),
    with the detail `capability=launch-exact-detached`.

**Plugin environment and descriptors**

- The launcher composes the plugin's environment from these, and nothing
  else:
  - `PATH` and `HOME` from the caller;
  - `XDG_STATE_HOME` and `XDG_CONFIG_HOME`, only when present and absolute,
    after canonicalisation;
  - for `attach` only, `TERM`;
  - `CURATOR_RUN_PROTOCOL=curator-session-manager/1`.

  The manager persists the state root it resolved from these values, so
  every later call for a session reaches the same daemon.
- The plugin owns its transport to its daemon (for task-board, the private
  control socket of `tb-sessiond`).
- The launcher marks its own copies of fd 3 and fd 4 close-on-exec, and
  hands the plugin exactly the descriptors in the table.

**Deadlines**

| Operations | Deadline |
| --- | --- |
| `describe`, `status`, `list`, `stop`, `cancel` | 10 s |
| `launch`, `resume` | 60 s |
| terminal operations | none |

An overrun kills the plugin's process group. For `launch` and `resume`, the
overrun is followed by reconciliation (§5.7); otherwise it is
`session_host_unavailable` (1).

### 5.2 Operations and capabilities

| Operation | Request | Response | Capability |
| --- | --- | --- | --- |
| `describe` | `{}` | `{id, version, capabilities[], runtimes{env-id: [operation…]}}` | always |
| `launch` | launch request (§5.3) | session record (§5.4) | `launch-delegated/1` |
| `launch-exact` | CIP-0016 successor payload | terminal session | `launch-exact/2` |
| `resume` | resume request (§5.3) | session record | `resume/1` |
| `attach` | `{session, expected_generation}` on fd 3 | session exit status | `attach/1` |
| `status` | exactly one of `{session}` or `{operation_id}` | session record, or an operation outcome (§5.7) | `status/1` |
| `cancel` | `{operation_id}` | an operation outcome | `admission/1` |
| `list` | `{labels?, cursor?, limit?}` | `{sessions: [record…], next_cursor: null\|string}` | `list/1` |
| `stop` | `{session, expected_generation, mode: "graceful"\|"force", precondition: "none"\|"not-busy"}` | session record | `stop/1` |

The v1 capability tokens are closed. An unknown token confers nothing.

- **`launch-delegated/1` [pending Q1, Ivan].** The plugin composes through
  the same agents-management construction site, from the profile the request
  names. This is the proposed CIP-0016 amendment (Appendix A). The activation
  boundary is in §5.8.
- **`launch-exact/2`.** The CIP-0016 exact-plan shape, with the jointly
  published successor payload. The launcher prefers it **per runtime**: when
  `describe` lists `launch-exact` for that env-id and the launch is attached
  and requests no doorbell. Exact support for one runtime never retires
  delegated support for another.
  - The `launch-exact/2` payload cannot carry a doorbell or return a
    session record. A launch that is detached or that requests a doorbell
    (`--a2a`, or a Werkstatt record with a doorbell) therefore uses the
    delegated route, and the §5.8 matrix applies to it.
  - An exact-only manager refuses such a launch with
    `session_host_capability_missing`, with `capability=launch-exact-doorbell`
    for a doorbell, or `launch-exact-detached` for a detached launch.
- **`resume/1`, `attach/1`, `status/1`, `list/1`, `stop/1`.** A manager
  that advertises `stop/1` defines `busy` per runtime, as an observable
  state. The `not-busy` precondition and `state: busy` use the same
  definition:
  - Codex: a turn is in flight on the session's thread, by the app-server's
    own thread status;
  - Claude Code: the provider is processing a turn, not idle at its input
    prompt. Each manager documents how it observes this from its PTY or
    hook state;
  - Muse: a turn is in flight, by its PTY or session API.

  A manager that cannot observe `busy` for a runtime reports `unknown`, not
  `idle`. `not-busy` on `unknown` refuses `werkstatt_busy`, with
  `state=busy`.
- **`admission/1`.** The manager enforces, atomically, one active generation
  per `admission_key` across launch and resume, `operation_id` idempotency,
  operation outcomes, and `cancel` fencing (§5.7).
- **`board-goal/1`.** Workspace board goal binding (§3.3).
- **`doorbell-claude/1`, `doorbell-codex/1`.** See §8.
- **`credential-node/1`.** The manager starts Claude through Curator's
  protected local credential API, as the hosted token route of Appendix B
  requires. It is only an intake gate (§9).
- **`labels/1`.** Labels are retained, and `list` filters by them.
- **`survives-upgrade`, `drains-on-upgrade`.** The manager's upgrade
  behaviour (§3.3).

**Required capabilities**

The launcher calls `describe` once per invocation, before composition, and
computes the capabilities the launch needs:

- a launch shape for the runtime;
- `attach/1`, unless detached;
- `resume/1`, for resume selectors;
- `board-goal/1`, for `--board-goal`;
- `credential-node/1`, when the profile selects a node source (§9);
- the runtime's doorbell capability, when a doorbell is requested;
- for Werkstätten, also `admission/1`, `labels/1`, `list/1`, `status/1` and
  `stop/1`.

A missing token is `session_host_capability_missing` (6). A missing env-id
is `session_host_provider_unsupported` (6). Both are checked before any
`launch` or `resume`.

**Rules on operations (from the task-board host review, TASK-261018-m9kukq).**

- **Advertise only what works.** `describe` advertises only operations
  that are implemented and qualified, per runtime. A scaffold or a closed
  gate is never advertised. `survives-upgrade` and `drains-on-upgrade` are
  never inferred from daemon re-exec support.
- **`list` pagination.**
  - A paginated listing is a stable snapshot with an opaque cursor,
    matching `[A-Za-z0-9._-]{1,256}`.
  - `limit` is 1–100, and the default is 50.
  - A listing is complete only when `next_cursor` is `null`. The launcher
    follows cursors until then, and never treats a partial page as
    absence.
  - Unreadable records are listed with `state: unknown`, never dropped.
    Their projection is fixed:
    - `readable: false`, and `state: unknown`;
    - the `session` handle, `manager` and `labels`, from the manager's own
      index. The index stores labels for every entry;
    - `doorbell`: `{state: "off", uncertain: false, generation: null,
      reason: null}`;
    - every other record member `null`.

    The common envelope (`protocol`, `ok`) belongs to the response, not to
    the row. Rows inside a `list` page carry no envelope, while a `status`
    response wraps its record in the envelope.
    - Label filtering uses the index's labels.
    - If the index cannot establish an entry's labels, a filtered page that
      might include that entry refuses `session_host_unavailable`
      (`reason=index_unreadable`). Missing labels are never treated as
      non-membership.
    - Every session record carries `readable`. When it is `true`, the
      normal §5.4 rules apply.
    - If the index itself cannot produce a handle for an entry, the whole
      page refuses `session_host_unavailable`, with
      `reason=index_unreadable`. A page never silently omits a row.
  - `labels` matching is an exact key/value subset.
- **Stopping and attaching a generation.**
  - `stop` and `attach` carry `expected_generation`. A different current
    generation refuses with no effect, so a delayed stop for generation N
    never stops N+1.
    - On `stop`, which answers in JSON, the code is
      `session_generation_mismatch` (1).
    - On `attach`, whose only typed channel is the fd-4 status stream with
      the SPEC §4.9 registry, the code is the registry's existing
      `session_generation_stale` (1, no details).
    - The fd-4 registry is not extended, so the CIP-0016 receiver contract
      stays intact.
  - **One shared 10 s budget for `stop`.** Doorbell retirement step 2 (§8.2)
    may use at most 2 s waiting for an in-flight a2a attempt. The provider
    graceful wait gets at most 6 s. The rest is overhead.
  - `graceful` asks the provider to exit and does not escalate. A provider
    still alive when its share runs out leaves the session in `draining`,
    and the record says so. A timeout is never evidence of `down`.
  - **After `draining`, retirement continues inside the manager.** The
    persisted step index of §8.2 carries it: owner exit, abandon, then
    `off`. The session becomes `down` only when the provider is gone and
    retirement has finished.
  - `force` kills the provider's process group.
  - `not-busy` fences new turns (manager submissions, app-server writes,
    doorbell turns) atomically with the busy check. A refusal changes
    nothing.
- **`attach`** names exactly one persisted native session or thread. A
  manager never uses `--last` or a native picker for it.
- **Doorbells versus board notices.** `doorbell-*` means the a2a mechanisms
  of §8 only, never task-board's board-notice delivery. Board notices keep
  their own contract unchanged.

### 5.3 Launch and resume requests

**Launch** — `urn:relux:curator-run:delegated-launch` `1.0.0`, closed:

| Member | Content |
| --- | --- |
| `operation_id` | UUIDv4 (§5.7). |
| `submitted_at` | UTC RFC 3339 time at which the launcher created the operation. |
| `admission_key` | `null`, or `werkstatt:<id>`. |
| `runtime` | Canonical env-id: `claude_code`, `codex_cli` or `muse`. |
| `profile` | `{name, lock_sha256, fragment_digest}` from the launcher's resolve. `lock_sha256` is spelled as in the fragment: 64 lowercase hex characters, no prefix. `fragment_digest` is spelled as the SPEC §4.6 digest: `sha256:` followed by 64 lowercase hex characters, over the CCJ-1 canonical bytes of the parsed fragment. The plugin MUST resolve the same profile, and refuse `policy_refused` (16) when its fragment digest differs. |
| `cwd` | Absolute workspace. |
| `model`, `effort` | Resolved by SPEC §4.3. They are values, never flags. Either may be `null`, meaning the runtime's native default where the module admits one (agents-management v0.5.68 or later for managed Codex). |
| `service_tier` | `null`, or a value the module admits. A plugin that cannot apply it refuses rather than ignoring it. |
| `weights_id` | `null`, or an exact local-weights id. The plugin passes it to the module as the weights pin. A module without weights pinning refuses `session_host_capability_missing` with the detail `capability=weights-pin`. |
| `permissions` | `{mode: native\|yolo, source}` from SPEC §4.3. |
| `system_prompt` | `null`, or `{semantics: "append"\|"replace"}` (the SPEC §5 opt-in). |
| `sandbox` | `{mode: "none"\|"workspace-write", writable_roots: [abs…]}`. |
| `harness_memory` | Boolean. |
| `session_name` | The SPEC §3 `--name` grammar. |
| `labels` | At most 16 entries. Keys match `[a-z0-9.-]{1,32}`. Values match `[A-Za-z0-9._:/@+=-]{1,128}`, which contains both the §3.4 selector value set and the a2a identity set `[a-z0-9._=-]`, so `selector=<decision_id>` and `identity=<a2a id>` labels always fit. |
| `child_env` | Non-secret literals: at most 32 names `[A-Z][A-Z0-9_]{0,63}`, none reserved (§9.2), values at most 4096 bytes, passing the **secret check** below. Otherwise `secret_policy_violation` (16). |
| `board_goal` | `none` or `workspace`. |
| `doorbell` | `null`, or `{identity, heartbeat, max_wakes_per_hour}` (§8). `identity` matches the a2a identity grammar `[a-z0-9._=-]{1,64}`. `heartbeat` is a duration string `<n>s` or `<n>m` from 30 s to 60 m. `max_wakes_per_hour` is an integer from 1 to 60. |
| `credential_source` | `null` (per-home), or the fragment's `source-projection-v1` `{source, source_generation, policy_generation}` (§9). Its exact types follow curator-spec `protocol/local-setup-token-source.md` §5: `source` is `{mode: "node", source_ref: "local:<32 lowercase hex>", harness_channel: "CLAUDE_CODE_OAUTH_TOKEN"}`, and both generations are positive JSON safe integers. The manager re-resolves the source itself, and a different projection refuses `credential_source_stale`. |
| `native_tail` | The selector-free tail after the module's typed-intent elevation (SPEC §4.8). |
| `detach` | Boolean. |

**Scalar grammars.** These apply in every §5 schema:

- `model`, `effort` and `service_tier` match `[A-Za-z0-9._:/@+-]{1,128}`,
  the same grammar as the selector values of §3.4. Admitting a value is
  still the module's decision.
- `weights_id` matches `gguf-sha256:[0-9a-f]{64}`.
- The `describe` `version` matches `[A-Za-z0-9._+-]{1,64}`.
- Profile names follow Curator's fragment profile-name grammar.
- Timestamps use exactly the form `YYYY-MM-DDTHH:MM:SSZ`: UTC, whole
  seconds, no fraction, and the `Z` suffix. Any other spelling is refused,
  so equal instants have equal bytes and equal request digests.

**Secret check for `child_env`.** This check is a closed rule set,
versioned with this document.

1. **Name check.** A name is refused if it equals `TOKEN`, `SECRET`,
   `PASSWORD`, `PASSWD`, `API_KEY` or `APIKEY`. It is also refused if it
   ends with `_TOKEN`, `_SECRET`, `_PASSWORD`, `_PASSWD`, `_API_KEY`,
   `_APIKEY`, `_PRIVATE_KEY`, `_CREDENTIALS`, `_COOKIE` or `_SESSION_KEY`,
   or if it starts with `AWS_SECRET`.
2. **Value check.** A value is refused if it contains `-----BEGIN`. It is
   also refused if it starts with `sk-`, `ghp_`, `gho_`, `ghs_`,
   `github_pat_`, `xoxa-`, `xoxb-`, `xoxp-`, `glpat-` or `AKIA`, or if it is
   a JWT-shaped value: three base64url segments separated by dots, the
   first starting with `eyJ`.
3. **No allowlist.** The name rules are suffix-based, so `GIT_AUTHOR_NAME`,
   `SSH_AUTH_SOCK` and `RELEASE_SIGNING_KEY` pass by construction.
   Reserved credential names (§9.2) are refused separately, as
   `credential_reserved_name`.

This check is a guard against mistakes, not a secret classifier. A secret
that matches none of the rules still must not be put there.

**Schemas.** S3 publishes these in the repository as JSON Schemas under
`schemas/session-manager/`: the launch request, the resume request, the
session record, the operation outcome, the list page and the refusal. Each
comes with positive and negative vectors that cover every nullable and
optional combination, the cross-field rules (sandbox roots against `none`;
selector-free tail), and the bounds. The plugin and the launcher both
validate against them.

**Resume** — `urn:relux:curator-run:delegated-resume` `1.0.0`, closed. It
carries the launch members `operation_id`, `submitted_at`,
`admission_key`, `runtime`, `profile`, `credential_source`, `doorbell` and
`detach`, plus the following. The §5.7 expiry rule applies to resume as
well.

| Member | Content |
| --- | --- |
| `selector` | `{kind: "latest"}`, `{kind: "handle", handle}`, or `{kind: "provider_id", id}` (SPEC §4.8). |
| `scope` | `{cwd, labels}`. `latest` resolves within this scope. |

Resume rules:

- Resume never creates a session. Refusals (all exit 1, CIP-0016 §6):
  - `session_resume_not_found`;
  - `session_resume_ambiguous`;
  - `session_resume_unreadable`;
  - `session_resume_location_mismatch`;
  - `session_resume_identity_mismatch`;
  - `session_liveness_unknown`.
- `scope.labels` is an exact key/value subset match, combined with the
  canonical cwd and the runtime.
- `latest` orders candidates by their current generation's `started_at`. A
  resumed generation's `started_at` is the time of its resume.
- `latest` with a tie, or with a potentially matching candidate whose
  record is unreadable, is `session_resume_ambiguous`. An explicitly
  selected unreadable handle is `session_resume_unreadable`.
- `provider_id` looks up an existing manager record in scope. It never
  adopts an arbitrary native conversation.
- A resumed session keeps everything from its stored, admitted record:
  `board_goal`, policy, labels, model, effort, sandbox and child
  environment. Resume cannot change them. The request's profile,
  credential projection and doorbell are re-validated against the stored
  binding; they do not replace it.
- **Refusal precedence.**
  1. A stored session's admitted Curator identity that drifted from the
     current resolve keeps the manager's own
     `session_curator_identity_stale`. This includes retry and recovery,
     and it is checked first.
  2. Then, a selected existing session with another profile name is
     `session_resume_identity_mismatch`.
  3. A fresh launch whose request differs from the manager's own resolve
     is `policy_refused`.
- Native resume flags in `native_tail` never select.

What the plugin may add:

- It MUST NOT add profile content, change any resolved value, add
  instruction files or MCP servers, or enable Remote Control.
- It MAY add its own host plumbing only:
  - the PTY or app-server endpoint;
  - private session state;
  - the per-session owner (§8.2);
  - the goal adapter, when `board_goal` is `workspace`.
- Anything it cannot apply exactly is a refusal.

### 5.4 The session record

Schema `urn:relux:curator-run:session` `1.0.0`:

```json
{
  "protocol": "curator-session-manager/1",
  "ok": true,
  "session": "SES-…",
  "operation_id": "…",
  "manager": "task-board",
  "runtime": "codex_cli",
  "native_id": "019a…",
  "readable": true,
  "state": "starting|idle|busy|draining|down|unknown",
  "labels": {"werkstatt": "curator-run-dev1"},
  "generation": 3,
  "started_at": "…Z",
  "last_activity_at": "…Z",
  "doorbell": {"state": "armed|unavailable|off", "uncertain": false,
               "generation": "…", "reason": "…"}
}
```

Validation (§5.6 applies to the whole record):

- `session` matches `SES-[A-Za-z0-9]{1,48}`.
- `native_id` matches `[A-Za-z0-9-]{1,64}`. It is `null` before the native
  identity exists, and for unreadable records.
- `last_activity_at` is `null` until provider activity has been observed.
  It is set only from defined provider activity events, never from
  record-update times.
- `generation` is an integer from 1 to 2^53−1.
- `state` comes from three separate observations: the lifecycle, proven
  liveness, and provider activity (`idle`, `busy` or `unknown`). Missing
  evidence gives `unknown`, and `unknown` never maps to `idle` or `down`.
- `manager` equals the registry id.
- `state` is one of the six values. `unknown` MUST NOT be read as `down`.
- Timestamps are RFC 3339 UTC.
- `doorbell.reason` comes from the closed set `unsupported_version`,
  `not_requested`, `owner_unavailable`, `bind_failed` and
  `previous_generation_uncertain`.
- `doorbell.uncertain` is the durable a2a uncertainty flag of §8.2. It is
  independent of `state`.
- `doorbell` is always present, with exactly the four members `state`,
  `uncertain`, `generation` and `reason`. `generation` is `null` or
  matches `[A-Za-z0-9._:-]{1,128}`. Rules by state:
  - `armed`: `generation` non-null, `reason` null, `uncertain` false;
  - `unavailable`: `reason` required, one of `unsupported_version`,
    `owner_unavailable`, `bind_failed` or `previous_generation_uncertain`;
    `generation` may be null;
  - `off`: `reason` is null, `not_requested` or
    `previous_generation_uncertain`. `generation` is the most recent
    generation that was armed: the one being retired once its gate closes
    (§8.2 step 1) and while it is abandoned, otherwise the last retired one,
    or null if there never was one. `off` means that no wakes are delivered.
    It does **not** mean that retirement is complete; the manager's
    persisted step index records that.
  - When `uncertain` is true and the state is not `armed`, `reason` is
    `previous_generation_uncertain`.

### 5.5 Conformance

`internal/sessionmanager/conformance` runs a candidate plugin through every
operation it advertises. Each negative case has a passing control.

Wire:

- an unknown operation;
- malformed, over-bound and exit/`ok`-mismatched responses;
- a protocol mismatch;
- a missing capability;
- an unsupported runtime.

Values:

- a profile digest mismatch;
- a secret, or a reserved name, in `child_env`;
- detail values outside their vocabulary: arbitrary text, control
  characters, a secret sentinel. Each yields `session_host_protocol_error`
  and the sentinel never reaches output.

Admission:

- `stop` of an unknown session;
- `stop` with `not-busy` on a busy session: refused, and no state, gate or
  process change observed;
- concurrent launch-vs-launch, launch-vs-resume and resume-vs-resume on one
  `admission_key`: exactly one active generation;
- a retried `operation_id`: the same result, and no second process;
- a lost reply before admission, after admission, and after a failed start,
  each reconciled to the correct outcome;
- `cancel` of a prepared operation, followed by a late delivery: never
  starts.

Resume:

- missing, ambiguous and mismatched targets: refused, with no session
  created.

Liveness and doorbell:

- `unknown` liveness;
- an uncertain doorbell kept across stop and rebind.

Credentials:

- no credential byte in stdout, stderr, responses or the manager's records,
  checked by a grep with a sentinel.

Attach:

- a status record on fd 4, versus a plain exit status.

**Two tiers.**

- **Tier A, black-box.** These cases need no control over the plugin.
  - Wire and bounds: the normal wire and request-side probes. Intentionally
    malformed, oversized or poisoned *responses* are tier B (fault
    injection) or instrument controls, because a correct plugin never
    produces them on its own.
  - Unknown operations and capabilities.
  - Idempotency and conflict, and cancel tombstones.
  - Generation mismatch.
  - Pagination over sessions the suite creates itself.
- **Tier B, controlled.** These cases need to set a state the plugin wire
  cannot set: activity (busy or draining), identity drift, injected faults,
  the clock for expiry, and an unreadable index. Tier B drives a separate
  **fixture-adapter executable**, given as `-fixture <abs path>
  [-fixture-prefix …]`. It speaks its own small JSON contract, published
  with the suite: `set_activity`, `set_identity_drift`, `inject_fault`,
  `set_clock`, `corrupt_index` and `reset`.
  - Fixture controls never pass through the plugin's environment or its
    requests, and the plugin wire stays closed.
  - Without `-fixture`, tier B cases report `unqualified`. They are never
    counted as passed, and the overall result is "not conforming (tier B
    unqualified)".

**Instrument control.** The suite ships a known-bad control: flags on the
fake manager that violate each property, one at a time. The suite must
fail each of them, so it detects what it claims.

`cmd/curator-run-fake-manager` is the reference plugin. It ships its own
fixture adapter. A manager is
**conforming** when it passes tiers A and B for the capabilities it
advertises.
Werkstätten use only protocol operations.

### 5.6 Refusals and value validation

The refusal object is `{protocol, ok: false, code, details}`.

Validation:

- `code` comes from the SPEC §6 registry plus §11. Any other code is
  `session_host_protocol_error`.
- **Who may emit a code.**
  - **A plugin may emit only these codes:**
    - `werkstatt_already_up`, `werkstatt_busy`;
    - `session_board_goal_unavailable`;
    - `session_host_unavailable`, `session_host_capability_missing`,
      `session_host_provider_unsupported`;
    - the `session_resume_*` codes, `session_liveness_unknown`;
    - `session_operation_conflict`, `session_operation_expired`,
      `session_generation_mismatch`;
    - `session_curator_identity_stale`;
    - `policy_refused`, `secret_policy_violation`, `launch_plan_invalid`;
    - the SPEC §6 plan codes, for example `plan_refused`;
    - the Curator local-credential codes.
  - **All other §11 codes are launcher-local.** These are the
    `werkstatt_*` workspace and record codes, the registry, selector,
    migrate, credential-guard and fragment codes, and
    `doorbell_setup_failed`. A plugin response that carries one of them is
    poisoned (`session_host_protocol_error`).
  - Launcher-local details such as `path`, `field` and `identity` are
    therefore always sourced by the launcher.
  - **Scope.** This allowlist covers every plugin JSON response, including
    the `code` of a historical `failed` outcome (§5.7). It does not cover
    the fd-4 status records of `attach` and `launch-exact`. Those keep the
    SPEC §4.9 classifier and registry unchanged, because their transport is
    CIP-0016's byte for byte.
- Each code admits only the detail fields that §11 lists for it. Each field
  has a closed vocabulary or a fixed grammar, also listed in §11.
- **Any** field or value outside its vocabulary, an unexpected field, or a
  non-string value makes the whole response `session_host_protocol_error`
  (6). The response is poisoned, not partially kept, which matches SPEC
  §4.9.

What the launcher prints:

- the constant message of the code;
- enumerated detail values;
- values it can source from its own side: the configuration path it read,
  the record field it validated, its own request's identifiers.
- A plugin-supplied path or free string is never printed.

Exit status follows the registry.

### 5.7 Admission, idempotency and reconciliation

**Composition with existing admission.** `admission/1` adds to a manager's
own admission checks and never replaces them. task-board's Curator digest
admission on attach, resume and recovery (`session_curator_identity_stale`
on a fragment or lock digest change, TASK-261014-28uxsf) keeps running, and
its refusal passes through unchanged. In the §5.7 outcomes it appears as
`failed` with that code; the code is added to the reused set in §11.

**Admission.** Under `admission/1`, launch and resume share one atomic
transition per `admission_key`, from "no active generation" to "generation
N admitted". Admission is per manager. Cross-manager exclusion is the
launcher's job (§6.2 step 4).

A second launch or resume for the key while a generation is active refuses
`werkstatt_already_up` (1) and has no effect.

**Operation outcomes.** `status {operation_id}` and `cancel
{operation_id}` are ordinary operations, and both answer `ok: true` with
this payload in the common envelope:

```json
{ "protocol": "curator-session-manager/1", "ok": true,
  "outcome": "not_found|prepared|admitted|started|failed|cancelled|unknown",
  "session": { …session record… },   // only with started
  "code": "…" }                     // only with failed
```

- A historical failed start is a successful `status` answer that carries
  the failure's `code`. It is not a refusal of `status` itself.
- `outcome.session` appears only with `started`.
  - It is an envelope-free record; the envelope belongs to the `status` or
    `cancel` response that carries it.
  - It must have `readable: true`. A manager that cannot read the record of
    a started operation answers `unknown` instead.
  - A `started` outcome whose record has `readable: false` is poisoned
    (`session_host_protocol_error`).
  - Unreadable projections are valid only in `list` rows and in direct
    `status {session}` responses.
- The status operation itself always exits 0 with `ok: true`. The
  launcher's own exit for a reconciled `failed` outcome is the registry
  exit of its `code`.
- `code` is drawn from the plugin-emittable set of §5.6, which is part of
  the union of:
  - the SPEC §6 registry, for example `plan_refused` and
    `launch_plan_invalid`;
  - §11 of this document;
  - the Curator local-credential codes (§11).
- A code outside that union poisons the response
  (`session_host_protocol_error`).
- **Idempotency.**
  - A repeated `operation_id` with the same canonical request digest
    (CCJ-1 over the request) returns the recorded outcome and never starts
    a second process.
  - The same `operation_id` with a different digest refuses
    `session_operation_conflict` (1).
  - Idempotency applies with `admission_key: null` too. It does not
    serialise plain sessions onto one key.
- **Durability.** Operations, outcomes and tombstones belong to the
  daemon's durable ledger, not to a plugin process.
  - Admission and the ledger reservation precede the provider start.
  - A crash between start and result keeps `unknown`, and holds the
    generation until runner evidence establishes the outcome.
  - Retention is at least 7 days.
  - An operation whose `submitted_at` is older than the retention window
    refuses `session_operation_expired` (1), and never starts.
  - Failed outcomes are kept even after failed host state has been cleaned
    up.
- **Fencing.** `cancel` on a `not_found` or `prepared` operation records a
  tombstone and answers `cancelled`. A late delivery of that operation never
  starts. `cancel` on any other state changes nothing and answers the
  current outcome.

**Reconciliation loop.** A launch or resume whose answer was lost runs this
loop. So does any later invocation that finds a pending operation. All
calls go to the operation's **own** manager (below), and the whole loop is
bounded by 60 s.

```text
r := status{operation_id}
loop:
  started   → success: write liveness, clear op.json, done
  failed    → refuse with r.code, clear op.json, done
  cancelled → refuse session_host_unavailable (reason=cancelled), clear op.json, done
  admitted  → wait 2 s; r := status{operation_id}; continue
  not_found, prepared → r := cancel{operation_id}; continue   // consume cancel's own outcome
  unknown   → refuse session_liveness_unknown, keep op.json, done
on transport failure, timeout or a poisoned response of status/cancel:
  refuse session_liveness_unknown, keep op.json, done
on deadline while admitted:
  refuse session_liveness_unknown, keep op.json, done
```

A `cancel` that fails or is lost is not a confirmed cancellation. The
operation stays pending.

Two traces settle the review's cases:

- **Crash before send.** `status` answers `not_found`, `cancel` answers
  `cancelled`, and `op.json` is cleared.
- **Start that races the cancel.** The `cancel` answers `started`, and the
  loop completes as success.

**Durable correlation.** Before submitting a Werkstatt launch or resume,
the launcher writes `<state>/werkstatt/<id>.op.json` atomically, under the
lock:

```json
{ "operation_id": "…", "kind": "launch|resume", "submitted_at": "…Z",
  "admission_key": "werkstatt:<id>",
  "manager": { "id": "task-board", "executable": "/abs/path", "registered_at": "…Z" } }
```

- **Ownership.**
  - A pending operation is reconciled only through the manager it records.
    That manager must still be in the effective registry with the same
    `executable`, but it need not be selected.
  - If the entry is missing, the reconciliation refuses
    `werkstatt_operation_unresolved` (1), `reason=manager_missing`.
  - If the executable differs, it refuses the same code with
    `reason=manager_changed`.
  - Neither case ever reroutes the operation to the selected manager.
  - Re-registering the original entry, or an explicit operator
    `werkstatt status <id> --abandon-operation`, clears the refusal.
    `--abandon-operation` is a manual decision: it clears `op.json` and
    records the abandoned id in the liveness record.
- **No overlap.** No new operation is generated while one is pending.
- **Schema.** `op.json` is `werkstatt-operation-v1`, closed, with exactly
  the members shown.
  - `--abandon-operation` removes it.
  - It appends `{operation_id, abandoned_at}` to the liveness record's
    `abandoned_operations`, a list that keeps the newest 16 entries.
  - A later `status` answer for an abandoned id is never applied to the
    liveness record.
- **Exit status.** The manager's `status` operation succeeds with
  `ok: true` (exit 0) even when it reports a `failed` historical outcome.
  The launcher's own exit for that reconciliation is the reported code's
  registry exit.

**Local serialisation.** Every mutation of a Werkstatt's `op.json` or
liveness file, including those made by `status`, happens under an
exclusive lock on `<state>/werkstatt/<id>.lock`.

- `up`, `down` and `recycle` hold the lock for their whole run.
- `status` takes it to reconcile and write.
- A second invocation waits up to 5 s:
  - `up`, `down` and `recycle` then refuse `werkstatt_busy` (1),
    `operation=in_progress`;
  - `status` then prints the last liveness record read-only, and writes
    nothing. In `--json`, the Werkstatt's row in the `werkstatt-status-v1`
    array (§6.6) has `stale: true`. `stale` is an output field only and is
    never stored.
- Every liveness write is also the expected-value compare-and-swap of §6.5.
- Manager admission still decides across clients and state roots.

**Plain hosted launches** (no Werkstatt) are attached and interactive. A
lost answer runs the loop once in the same invocation, without `op.json`.
If it stays unresolved, the launcher prints the `operation_id` and refuses
`session_liveness_unknown`.

### 5.8 Activation boundary for `launch-delegated/1`

`delegated_trial` (system-layer `launcher.json`, §3.1) authorises a live
trial. The owner sets it only after `ivmbp_curator_orch` or Ivan explicitly
authorises that trial. Nothing in this document implies the authorisation.

The matrix below applies whenever the launcher **would use the
`launch-delegated/1` route**, whatever else the manager offers. That
includes a manager that also offers `launch-exact/2`, in every case where
§5.2 selects delegated. For example, `werkstatt up` and every detached
launch use delegated, because exact cannot detach:

| Invocation | Before Q1 sign-off, `delegated_trial` absent or false | Before sign-off, `delegated_trial: true` | After sign-off |
| --- | --- | --- | --- |
| `--hosted` | Refuse `session_host_delegated_unauthorized` (16) | Hosted | Hosted |
| `werkstatt up` | Refuse `session_host_delegated_unauthorized` (16) | Hosted | Hosted |
| No flag, `host_mode: auto` | Native with `notice-unselected` | Native with `notice-unselected` | Hosted |
| No flag, `host_mode: hosted` | Refuse `session_host_missing`, `reason=no manager selected` | Refuse, the same | Hosted |
| `--native`; or `host_mode: native` with neither `--hosted` nor `werkstatt up` (`werkstatt up` is always hosted, and its row above applies) | Native | Native | Native |

- The trial flag never changes the two implicit rows.
- Fake-manager tests set `delegated_trial` in their fixture configuration.
- At sign-off, the next revision lifts the implicit-row restriction, and the
  flag is withdrawn.
- Launches that use the `launch-exact/2` route are outside this matrix.

The paired test vectors are the 15 cells of the matrix, each with its
expected outcome.

## 6. Werkstätten

### 6.1 The record [decided Q4]

A Werkstatt is project × role profile × environment, with its own managed
home, clone and identity. Its record is `werkstatt-record-v1`, closed. It
carries `"schema": "werkstatt-record-v1"` as a required member, because a
person edits the file. The launcher-internal `op.json` and liveness files
carry no `schema` member; their file names and locations identify them.

| Field | Meaning |
| --- | --- |
| `id` | `[a-z0-9][a-z0-9-]{0,47}`. |
| `project` | Project slug. |
| `role` | `developer`, `reviewer` or `orchestrator`. |
| `runtime` | Canonical env-id. |
| `model`, `effort` | Pinned at create or restart. A per-task effort travels in the task, not the record (dispatch returns (station, effort)). |
| `weights_id` | Optional exact local-weights id, passed to the spawn plane as the model's weights pin. |
| `profile` | Curator profile name. |
| `clone` | Absolute path of the worktree. |
| `git_dir` | Absolute path of the separate git dir, or absent. |
| `identity` | a2a identity. |
| `room` | The room tasks arrive in. |
| `orchestrator` | The only a2a identity the Werkstatt accepts tasks from. |
| `context_mode` | `continue` (default) or `fresh`. |
| `harness_memory` | Boolean; the default is `true` in `continue`. |
| `sandbox` | `none` (current decision) or `workspace-write`. |
| `child_env` | Optional map under the §5.3 `child_env` rules (§6.3). |
| `trust_class` | Router vocabulary; stored, not interpreted. |
| `doorbell` | `{heartbeat: "1m", max_wakes_per_hour: 6}`, or `null` for none. |
| `config_decision_id` | Optional router decision id. |

Sources, in order:

1. The board's `station` element, read with
   `task-board station get <id> --json`, once that element exists. Its
   fields, together with the liveness fields of §6.5, stay identical to the
   router's station snapshot (`id, identity, role, project, runtime, model,
   effort, weights_id, trust_class, state, current_task, queue_depth,
   last_heartbeat_at, config_decision_id`).
2. Otherwise `${XDG_CONFIG_HOME:-~/.config}/swarma/launcher/werkstatt/<id>.json`,
   with the SPEC §4.7 file checks.

When both exist, the board decides, and a differing local file prints one
warning. `werkstatt up` prints the source it used.

### 6.2 `werkstatt up <id>`

Ordered steps. Steps 1–7 start no process and change nothing at the
manager. Two of them write locally, as stated: step 6 may let Curator repair
the managed home, and step 7 writes the transitional hook file. Step 8 is
the only call that starts anything. For Werkstätten the manager is described
before the profile is resolved. That is a deliberate exception to §3.3: the
`describe` call is read-only, and the already-up check must come before any
local write.

1. **Lock and reconcile.** Take the §5.7 lock. Reconcile any pending
   `op.json` first. If that stays `unknown`, the result is
   `session_liveness_unknown` (1).
2. **Record.** Load and validate it.
   - Missing: `werkstatt_not_found` (1).
   - Invalid: `werkstatt_record_invalid` (2).
3. **Manager.** Resolve the selected manager (§4.2) and `describe` it. The
   Werkstatt capability set is required (§5.2), and §5.8 gates the selected
   delegated route.
   - An empty or unselected registry is
     `werkstatt_session_manager_required` (1).
   - `up` is always hosted, whatever the machine mode is.
4. **Owner and existing sessions.**
   1. **Owner check.** This step always runs when a liveness record exists
      and its state is not `down`, whether or not the binding's id equals
      the selected id. The binding is the record's `manager` member (§6.5).
      - The registry has no entry with the binding's id: fail closed with
        `werkstatt_operation_unresolved` (1), `reason=manager_missing`.
      - The entry's `executable` or `registered_at` differs from the
        binding: fail closed with `werkstatt_operation_unresolved` (1),
        `reason=manager_changed`. A same-id re-registration (§4.3 rule 4)
        is therefore never trusted to own an older generation.
      - Otherwise, `status {session}` goes through the bound manager:
        - a state other than `down` is `werkstatt_already_up` (1);
        - an unreachable manager, or `unknown`, is
          `session_liveness_unknown` (1);
        - `down` means the owner check passes.
   2. **Selected manager.** `list {labels: {werkstatt: id}}` on the
      selected manager.
      - A session there in a state other than `down` is
        `werkstatt_already_up` (1).
      - An `unknown` session there is `session_liveness_unknown` (1).
   3. **Resume target.** This applies with `context_mode` `continue` and no
      `--new`.
      - First choice: the liveness record's session, when two conditions
        hold:
        - the binding equals the selected entry in full (id, executable,
          `registered_at`);
        - that session is `down`, either confirmed by step 4.1 or listed
          as `down` by step 4.2, which covers a record already marked
          `down`.
      - Otherwise it is the newest `down` session on the selected manager
        by `started_at`.
      - A tie, or an unreadable candidate, is `session_resume_ambiguous`
        (1).
      - If the bound manager is not the selected one, there is no
        cross-manager resume: the launch is new. A manager switch is
        therefore the one case in which `continue` starts a new
        conversation (§6.4).
   4. **Recovery.** Two explicit commands leave the fail-closed state.
      - `werkstatt status <id> --rebind-manager`:
        - asks the current entry with the binding's id for `status
          {session}`;
        - if it answers with a record for exactly that session handle, the
          binding is updated to the entry;
        - any other answer refuses, and the binding is unchanged.
      - `werkstatt status <id> --abandon-operation` is an **operator
        override** (§6.5). It is a decision, not evidence that the old
        generation stopped.
   5. **Routing.** While the binding exists, `status`, `stop`, `resume` and
      `down` for this Werkstatt go to the **bound** manager, not to the
      selected one.

   With these checks, the launcher never automatically starts a second
   generation while the first may live. The only exception is the explicit
   operator override.
5. **Workspace checks.** All of them are read-only, and an inconclusive
   check refuses.
   - The clone exists and is a directory (`werkstatt_clone_missing`).
   - If `git_dir` is set, the layout must hold: `git_dir` exists, its
     physical path is outside the clone's, the worktree has no `.git`
     entry, and `core.worktree` names the clone. Otherwise
     `werkstatt_git_layout_invalid`.
   - **No context leak.** No strict ancestor up to `/` contains any of the
     four files `CLAUDE.md`, `CLAUDE.local.md`, `AGENTS.md` or
     `AGENTS.override.md`. Both the lexical path as given and the physical
     path, with all symlinks resolved, are checked. A found file is
     `werkstatt_context_leak`. An unreadable
     ancestor is `werkstatt_context_check_failed`, never clean. Files
     inside config directories, such as `~/.claude/CLAUDE.md`, are not
     ancestors.
   - The a2a credential file exists, checked by presence only
     (`werkstatt_identity_missing`).
6. **Context plane and admission.**
   - Resolve the fragment for `profile` (SPEC §4.1, `--repair`).
   - Admit `model` and `effort` (SPEC §4.3).
   - Read the fragment's credential projection (§9). A node source refuses
     here until §9.2 allows it.
7. **Doorbell preparation.** For `claude_code`, write the transitional
   hooks (§8.1).
8. **Start.**
   - Write `op.json` (§5.7).
   - Make exactly one call, with `admission_key: werkstatt:<id>`:
     - `resume {selector: {kind: handle, handle: <target>}, scope: {cwd: clone, labels: {werkstatt: id}}}`
       when step 4 found a target;
     - otherwise `launch`, with:
       - from the record: `runtime`, `profile`, `model`, `effort`,
         `weights_id`, `sandbox`, `harness_memory`, `child_env` and
         `doorbell`;
       - `cwd = clone`;
       - `board_goal: none`;
       - `system_prompt: null`;
       - `detach: true`;
       - `credential_source` from the fragment;
       - the labels `werkstatt=<id>` and `identity`. There is no generation
         label, because each manager numbers its own generations (§6.5).
   - Reconcile per §5.7.
9. **Liveness.** Write the §6.5 record, including the owner binding copied
   from `op.json`'s `manager` and the settled `operation_id`. Only then
   remove `op.json`.
   - If the liveness write fails, the session is still up: print the
     handle, and refuse `werkstatt_liveness_write_failed` (1).
   - The next `status` rewrites the record.
10. **Attach.** With `--attach`, attach afterwards. A detach leaves the
    Werkstatt up.

`--new` forces a new thread in the continue case. `context_mode` `fresh`
always launches new.

### 6.3 Child environment for a Werkstatt

The launcher adds:

- `A2A_AGENT=<identity>`;
- `GIT_DIR` and `GIT_WORK_TREE`, when `git_dir` is set.

The manager adds `A2A_SESSION_ID=<native id>`, once known (§8).

Everything else comes from the record's `child_env`, written by whoever
creates the record. The launcher sources no helper and reads no
repository-controlled file for it. For a signing developer Werkstatt that
means:

- `GIT_AUTHOR_*` and `GIT_COMMITTER_*`;
- `GIT_CONFIG_COUNT` with the `user.signingkey`, `gpg.format` and
  `gpg.ssh.program` triples;
- `GIT_SSH_COMMAND` and `SSH_AUTH_SOCK` (the agent socket);
- `RELEASE_SIGNING_KEY`, the public-key path of the host's separate agent
  release key;
- build caches such as `GOCACHE` and `GOPATH` under the Werkstatt directory.

Only public-key paths and socket paths appear here. Private keys never move.

**Commit key and release key are separate.**

- `user.signingkey` is the commit key and signs commits only.
- Release tags are signed with
  `git -c user.signingkey="$RELEASE_SIGNING_KEY" tag -s …`.
- The commit key MUST NOT sign release tags.
- A host without a release key gives no `RELEASE_SIGNING_KEY`, and a tag
  attempt then fails visibly.

### 6.4 Context modes

- **`continue`:** one long-lived session. Tasks arrive by doorbell into the
  same thread, and the harness compacts natively. After a crash, a machine
  restart or `down`, the next `up` resumes the same native conversation
  (§6.2 steps 4 and 8). A new thread starts only through `--new`,
  `recycle` or a manager switch (§6.2 step 4.3).
- **`fresh`:** the orchestrator runs `werkstatt recycle <id>` before each
  task.
- Rework after review continues the task's thread in both modes. That is
  the orchestrator's choice.

### 6.5 Liveness

The state file is
`${XDG_STATE_HOME:-~/.local/state}/swarma/launcher/werkstatt/<id>.json`, with
schema `werkstatt-liveness-v1`, written atomically:

```json
{ "id": "...", "state": "starting|idle|busy|draining|down|unknown",
  "manager": { "id": "task-board", "executable": "/abs/path", "registered_at": "…Z" },
  "operation_id": "…", "state_source": "manager|operator_override",
  "session": "SES-…", "generation": 3,
  "started_at": "…Z", "last_heartbeat_at": "…Z", "last_activity_at": "…Z",
  "doorbell": "armed|unavailable|off", "doorbell_uncertain": false,
  "current_task": null, "queue_depth": 0, "abandoned_operations": [] }
```

- **Who writes:** curator-run writes the file on `up`, `down`, `recycle` and
  every `status`. Each time it copies these fields from the bound manager's
  session record, and infers nothing else:
  - `state`, `session`, `generation`;
  - `started_at`, `last_activity_at`;
  - `doorbell.state`, `doorbell.uncertain`.

  `state_source` is then `manager`.
- **`manager`** is the owner binding: the registry entry the operation was
  submitted to, frozen at submission. It is immutable within a generation.
  It changes only two ways:
  - a successful new generation installs its own binding (§6.2 step 9);
  - `--rebind-manager`.

  After a confirmed-down generation on manager A and a new launch on B,
  every later `status`, `stop` and `resume` goes to B.
- **`operation_id`** is the last settled operation, and `null` before the
  first one. Together with `generation` it is the compare-and-swap key of
  §5.7, used as an **expected-value** check, never as an ordering of UUIDs.
  A writer reads `(manager, operation_id, generation)` under the lock, and
  commits only if the file still holds exactly that tuple. One of four
  transitions must then hold:
  - the same operation keeps its generation (a status refresh);
  - a newly settled operation **on the same binding** moves `generation` up
    by exactly one;
  - a newly settled operation **on a new binding** takes the new manager's
    own generation, whatever its value. This is allowed only when the
    previous record is `down`, by manager evidence or by operator override.
    `generation` is always the bound manager's number, and there is no
    cross-manager counter;
  - the first settled operation after an override that came **before the
    first start** (`generation: null`), on the same binding or a new one,
    takes that manager's generation, whatever its value.

  Every other write is dropped.
  - **Abandoned ids.** The drop applies to *operation outcomes* for an id in
    `abandoned_operations`: late `status {operation_id}` answers and
    reconciliation results.
  - An ordinary `status {session}` read through a valid binding is manager
    evidence, not an operation outcome. It may always replace an operator
    override, even when the record's `operation_id` is an abandoned id.
- **Operator override.** `--abandon-operation` sets `state: down`,
  `state_source: operator_override`, and appends the abandoned id to
  `abandoned_operations`.
  - If no session ever started, `manager` is the binding copied from
    `op.json`, and these fields are `null`, because nothing is fabricated:
    `session`, `generation`, `operation_id`, `started_at` and
    `last_heartbeat_at`. `last_activity_at` is `null` as well.
  - In that case `doorbell` is `off` and `doorbell_uncertain` is `false`.
  - This override-only record is the only shape in which `session`,
    `generation`, `operation_id`, `started_at` or `last_heartbeat_at` may be
    `null`. `last_activity_at` may be `null` in any record, because a
    readable session may have no provider activity yet (§5.4).
  - It does not cancel or stop anything remote.
  - `status` and the router show `state_source`. The router treats an
    override as not established, so it cannot be used as evidence of
    termination.
  - The next manager read through a valid binding replaces it with
    `state_source: manager`.
- **No record yet.** If `status` cannot take the lock and no liveness file
  exists, for example during the first `up`, it outputs the normal
  `werkstatt-status-v1` array. The Werkstatt's row is `{record, liveness:
  null, manager_session: null, a2a: null, stale: true}`, and `status` exits
  0.
- **Unreadable manager record.** When the bound manager returns an
  unreadable record (`readable: false`, §5.2), the liveness write sets only
  `state: unknown`, `state_source: manager` and `last_heartbeat_at`.
  - The manager answered, but it could not establish the state; that answer
    replaces an earlier operator override as the newest evidence.
  - Every other field keeps its previous value; the projection's nulls are
    never copied in. `up` and
  `down` refuse `session_liveness_unknown`. `readable` itself is never
  stored in the liveness file.
- **`last_heartbeat_at`** is the time of that manager read. A stale value
  means nobody looked, not that the session is down.
- **`current_task` and `queue_depth`** are written only by the orchestrator,
  through `werkstatt status <id> --set-task/--clear-task/--queue-depth`.
  These edits require a liveness record with a non-null `session`. A
  Werkstatt that was never up refuses `werkstatt_not_up` (1). That covers
  both a missing liveness file and an override-only record from before the
  first start. There is no dispatch-only liveness shape.
- **`unknown`** is outside the router's five states. It means the manager
  answered `unknown`, or did not answer. The router treats it as not
  dispatchable.
- **Mirroring:** the router reads this file. Once `station` exists, the
  launcher also mirrors `state`, `last_heartbeat_at` and `session` to it.
- **a2a liveness** (`a2a status set` / `who`) is the Werkstatt's own
  heartbeat. `status` shows both, unmerged.

### 6.6 `down`, `recycle`, `status`, `list`

- **`down <id>`:** under the lock, one call:
  `stop {session, expected_generation, mode: graceful, precondition: not-busy}`.
  Before that call, `down` runs the **owner read**: the binding validation
  and the `status` read of §6.2 step 4.1, **without** its up-only
  `werkstatt_already_up` decision.
  - A missing or changed binding refuses `werkstatt_operation_unresolved`,
    as in `up`.
  - `unknown`, or an unreachable manager, refuses `session_liveness_unknown`.
  - A `down` session exits 0 with one line, and nothing is called.
  - A live session yields the target `session` and its current
    `generation`, which become the `stop` request's `expected_generation`.
    It is never taken from a default or inferred.
  - If the generation changes between the read and the stop, the manager
    refuses `session_generation_mismatch`, with no effect.
  - The manager checks the precondition atomically. A busy session refuses
    `werkstatt_busy` (1), with the detail `state=busy`, and nothing changes:
    no gate, no stop, no record.
  - Only after the manager accepts the stop does it run the doorbell
    retirement of §8.2.
  - `--force` sends `mode: force, precondition: none`, with the same
    `expected_generation`.
  - The liveness record is written from the answer.
- **`recycle <id>`:** `down` (graceful, `not-busy`), then `up --new`,
  under one lock. If `down` refuses, nothing else happens.
  - If `down` answers `draining`, `recycle` polls `status` every 1 s for up
    to 20 s, until `down`.
  - If the session is still not `down` after that, `recycle` refuses
    `werkstatt_busy` with `state=draining`, and nothing else happens.
  - `up` admission keeps refusing while the old generation lives, so a
    `draining` answer is never treated as "may start".
- **`status [<id>]` and `list`:** the record, the manager status, the a2a
  `who` line and the doorbell state. `--json` gives `werkstatt-status-v1`: a
  bare JSON array, with no wrapper, of rows of exactly
  `{record, liveness, manager_session, a2a, stale}`:
  - `record`: the Werkstatt record;
  - `liveness`: the liveness record, or `null` when none exists;
  - `manager_session`: the bound manager's session record as returned,
    including `readable`, or `null` when no call was made or possible;
  - `a2a`: the a2a `who` result for the identity, or `null`. It is read
    with `a2a who --json --stale-after <d>`. `<d>` is three times the
    record's `doorbell.heartbeat` when one is set, and `3m` otherwise. The
    threshold is for display only. The launcher never derives liveness or
    `state` from it;
  - `stale`: a boolean, true when the lock could not be taken (§5.7).

## 7. Muse

- The SPEC §4.2 `muse` row stays. Muse launches require
  `launch-env-fragment-v4`. A v1–v3 fragment for `muse` refuses
  `fragment_version_unsupported` (2), with the details `environment=muse`
  and `required=launch-env-fragment-v4`. v4's `root_context` and
  `credential_store` are read as environments §10.2 defines them.
- **Native:** the plan comes from agents-management v0.5.66 or later (the
  v4 consumer, CIP-0017). The explicit `root_context` is passed to the
  module and never inferred.
- **Hosted:** the manager must list `muse` in `runtimes` with a launch shape
  and `attach`. task-board does this once its v4 consumer lands
  (TASK-261018-yzorfb). Before that, a hosted Muse launch refuses
  `session_host_provider_unsupported` (6), with no native fallback.

## 8. a2a doorbells for hosted sessions

### 8.1 Claude

Claude rings through hooks on `SessionStart`, `UserPromptSubmit` and
`Stop`. Idle wake is the session's own: the hook template tells it to keep
one background `a2a wait --timeout 0`, whose completion wakes it. No
terminal typing is used for doorbells, for any runtime.

**Transitional path (v1).** For `--a2a <identity>` and for Werkstätten, the
launcher ensures the hooks before the start call:

- It runs `a2a hook install --agent <identity> --runtime claude --settings
  <path>`. The command is idempotent.
- It reads the file back. It checks only the three hook entries and
  `env.A2A_AGENT`.
- A failure is `doorbell_setup_failed` (1), with the detail
  `step=install|verify`.

**Containment.** The target is `<cwd>/.claude/settings.local.json`. A check
of the pathname followed by a write through the pathname is not enough,
because the directory can be swapped in between. So the helper never writes
the target directly, and the launcher writes in exactly two places:

- a fresh private staging directory under its state directory;
- the `.claude` directory it validated in step 1, identified by its open
  descriptor rather than its pathname: one temporary file there, and the
  `settings.local.json` entry that replaces the old one.

It never writes through a substituted symlink, and never into a file or
directory it did not validate.

1. **Validate and pin.**
   - The launcher opens `<cwd>`, then `.claude` relative to it, with no
     symlink following at either step, and keeps the `.claude` descriptor
     pinned.
   - Checked on the descriptor, `.claude` must be a directory owned by the
     user and not group- or world-writable.
   - An existing `settings.local.json` must be a regular file with one link,
     owned by the user. It is read through the pinned descriptor, with no
     symlink following and a bounded size.
   - A missing `.claude` is created relative to the `<cwd>` descriptor, with
     mode `0700`, then opened the same way.
   - Any failure here is `step=install`, and nothing has been written.
2. **Stage.**
   - The launcher copies those bytes to a staged file in a fresh private
     directory (mode `0700`). An absent target gives an absent staged file.
   - The helper runs with `--settings <staged file>`. The settings bytes it
     writes depend only on the input bytes, the identity, the runtime and the
     `a2a` executable, never on the path.
3. **Check the staged result** with the predicate above. A failure is
   `step=verify`, and the target is left untouched.
4. **Commit** through the pinned descriptor.
   - The launcher creates a temporary file there exclusively, with no symlink
     following, writes it and syncs it.
   - The file keeps the permission bits of the file read in step 1. A new
     file gets `0600`. This is the one way staging differs from running the
     helper on the real path, and it affects metadata only, never content.
   - Just before the rename, the launcher checks that the entry is still the
     file it read in step 1: same device and inode, or still absent. If not,
     it removes the temporary file and refuses `step=install`.
   - It then renames the temporary file over `settings.local.json` within the
     same directory.

   The check only detects a conflict that is visible at the moment it runs.
   It is not an atomic compare-and-swap. A replacement or an in-place edit
   made between the check and the rename is overwritten. This is accepted:
   the launcher is the only expected writer of this file at launch.
5. **Read back and confirm.**
   - The launcher reads the target back through the pinned descriptor and
     applies the predicate.
   - It then opens `<cwd>` afresh, and checks the `.claude` entry relative to
     it with no symlink following. The entry must be a directory, with the
     pinned descriptor's device and inode and the step 1 owner and mode
     constraints. A symlink fails, even one that resolves to the pinned
     directory.
   - Any failure is `step=verify`.

If the pinned directory was moved during steps 1–4, step 4 has already
committed into it at its new location. That location is still the user's own
validated directory, never a target chosen by whoever moved it. Step 5 then
refuses `step=verify`, so a verify refusal does not mean that nothing was
written. Step 5 sees only the state at the moment it runs: a move that is
undone before it runs is not detected.

The private staging directory is removed on every path.

`doorbell-claude/1` means the manager's Claude composition keeps
project-local settings active. It never means task-board's board-notice
delivery, which types into the terminal; that path keeps its own contract.

**Target path.** Under CIP-0002 strict mode, project settings are not
honoured, and hooks come from the managed home through Curator's
manifest-declared hooks. That Curator item is still to be specified; it is
Ivan's "zero manual steps" requirement. Once it ships:

- the launcher stops writing `settings.local.json`;
- the a2a hooks come from the profile;
- `doorbell-claude/1` means home-level hooks are honoured.

### 8.2 Codex

Codex rings through `a2a runtime bind`. Only the owner of the thread's
app-server attachment may bind, so it belongs to the manager
(`doorbell-codex/1`).

**The per-session owner.** The manager runs one owner process per hosted
Codex session. It holds that session's thread attachment and lives exactly
as long as the binding. Its PID is the `--owner-pid`. The long-lived daemon
is never the owner, so stopping one session never needs another session's
owner to die.

**Bind.** After the thread exists on the manager's private app-server:

```text
A2A_AGENT=<identity> A2A_SESSION_ID=<thread>
a2a runtime bind --codex-thread <thread> --endpoint unix://<socket>
    --owner-pid <owner pid> --heartbeat <h> --max-wakes-per-hour <n>
    --gate ready --hooks installed --watch
```

- The manager supervises the `--watch` consumer.
- `A2A_SESSION_ID=<thread>` also goes into the thread's own command
  environment.
- The bind generation is reported in the session record.

**Retirement.** The order below applies on `stop` and before any rebind. A
`not-busy` refusal happens before step 1.

1. Run `a2a runtime gate --generation G stopping`. Atomically with the gate
   change, the doorbell record becomes `state: off`, `generation: G`,
   `reason: null`. A closing gate delivers no more wakes, so `off` is the
   valid projection from here on.
2. Read `a2a runtime status`. Wait for an in-flight attempt up to the stop
   deadline. An attempt that is still uncertain sets, in one write,
   `doorbell.uncertain: true` and `reason: previous_generation_uncertain`.
   The state stays `off`. The manager persists that flag in its own
   session state, and the liveness record mirrors it.
3. Stop the thread.
4. The owner exits.
5. The manager observes the owner's death and runs
   `a2a runtime abandon --generation G`.
6. `doorbell.state` becomes `off`. **`uncertain` is not cleared.**
   Abandoning a generation does not reconcile its attempts, as the a2a
   contract states.

**Retirement failures.** The manager persists the step index of the
retirement sequence. After a crash it resumes from that step.

- If `a2a runtime status` cannot be read, the gate stays closed, and the
  doorbell is `unavailable`. The reason is `previous_generation_uncertain`
  when `uncertain` is already true; otherwise it is `owner_unavailable`.
  This is the §5.4 rule: persisted uncertainty always wins the reason.
- If owner evidence cannot be read, the result is the same.
- Neither failure is ever read as "no uncertain attempt".

**Rebind gate.** Before binding a new generation for the identity, the
manager reads a2a's durable ledger (`a2a runtime status`) for the previous
generation. The gate opens only when both conditions hold:

- a2a reports no uncertain attempt;
- the manager's `uncertain` flag is false.

The flag is cleared only when a2a's ledger shows that the attempt was
reconciled. Neither `off` nor a successful abandon clears it. While the gate
is closed:

- the session starts without a doorbell, with `doorbell.state: off`,
  `uncertain: true` and reason `previous_generation_uncertain`;
- `up` prints one line;
- reconciliation stays with a2a and the orchestrator (§12).

**Unsupported versions.** The a2a native adapter verifies app-server
0.160.0–0.161.0. On the installed Codex 0.162.1 the bind reports
unavailable, so the record shows `doorbell.state: unavailable`, reason
`unsupported_version`, and `up` prints one line. The launch is not refused.
Qualifying 0.162.x is an a2a item.

## 9. Claude setup-token

### 9.1 Curator owns the source

The protected local setup-token source is already normative in curator-spec
[`protocol/local-setup-token-source.md`](https://github.com/relux-works/curator-spec/blob/main/protocol/local-setup-token-source.md)
(revision 1, owner decision 2026-10-16). curator-run consumes it and adds no
source of its own. It has no `credentials.json`, no Keychain reader, no
enrolment command and no fd-borne secret. That protocol fixes:

- **Selection.**
  `environments.credential_source.<profile>.claude_code = per-home | node:<label>`.
  This is user-owned machine configuration and no system lock can select
  it. Absent means `per-home`.
- **Custody.**
  - The per-user file `~/.curator/auth/<label>/secret` (0700 directory,
    0600 file), on macOS and Linux.
  - **No Keychain backend is admitted.**
  - Enrolment is `curator env credential enrol claude_code --node <label>
    --token-stdin`.
- **One token for every home.** Every profile selects the same
  `node:<label>`.
- **Carriers.** Fragment v6 carries `source-projection-v1`, and the
  required extension is `works.relux.curator.credential/1`.
- **The final boundary.** Curator's public in-process API
  `github.com/relux-works/curator/pkg/localcredential.AuthorizeAndStart`
  is the only way a process starts with the token. It re-resolves the
  protected `credential_binding/1`, which covers UID, host reference,
  profile, harness, node, source reference and generation, channel,
  endpoint, executable SHA-256 and policy generation. It also rejects every
  competing input, rechecks under the node lock, and injects
  `CLAUDE_CODE_OAUTH_TOKEN` only into the approved child.

**[decided Q5] is superseded.** It accepted a Keychain interim source. That
contradicts the accepted protocol's "no node Keychain backend", so this
revision drops it (§0.1 C10).

### 9.2 What curator-run does

- **Per-home.** With a `per-home` source (the default), nothing changes.
- **Node source, native launch.** curator-run is the final executor. It
  starts Claude through `localcredential.AuthorizeAndStart` with:
  - the composed invocation (absolute binary, its SHA-256, cwd, argv, the
    admitted non-secret baseline environment, the endpoint);
  - the fragment's source projection;
  - its process IO.

  It never handles the token. A refusal is passed through with its closed
  code, never followed by a native fallback.
- **The interactive gap.** The API refuses a non-nil `Terminal` until
  Curator qualifies the interactive mode (protocol §8, §7 "Interactive
  hosted PTY"). Until then, an interactive node-source launch refuses
  `credential_injection_unavailable` (2). This is Curator's
  qualification item, and §12 asks for it.
- **Node source, hosted launch.** The protocol refuses this today: "A later
  hosted token route needs an explicit separately reviewed amendment and
  qualification", and it forbids daemon lookup. Appendix B proposes that
  amendment:
  - the manager's per-session owner, running under the same UID, calls
    `AuthorizeAndStart` with its PTY as `Terminal`;
  - the request carries only the source projection, which the manager
    compares with its own resolve (`credential_source_stale` on mismatch);
  - restart and resume call the API again; nothing is stored.

  Until the amendment is accepted and qualified, a hosted node-source
  launch refuses `credential_injection_unavailable` before contacting the
  manager. A manager that advertises `credential-node/1` passes only the
  capability gate, never the authorisation itself.
- **Reserved names (the launcher's guard, closed).** The local
  setup-token protocol names its competing inputs only by category (§6
  step 3). The launcher therefore enforces this concrete closed list for
  `child_env`:
  - exact names:
    - `CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`;
    - `ANTHROPIC_BASE_URL`, `ANTHROPIC_CUSTOM_HEADERS`;
    - `CLAUDE_CODE_USE_BEDROCK`, `CLAUDE_CODE_USE_VERTEX`;
    - `OPENAI_API_KEY`, `CODEX_API_KEY`;
    - `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `XDG_CONFIG_HOME`,
      `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`;
    - `TBH_CREDENTIAL_BACKEND`;
  - the prefixes `LD_`, `DYLD_`, `NODE_` and `PYTHON`.

  The home and XDG names are the fragment's channel. A Werkstatt must not
  redirect them. A match is `credential_reserved_name` (2). This list is a
  guard: the authoritative check is Curator's, at its final boundary.
- **No Remote Control.** The launcher adds no RC flag, and the source does
  not carry RC.

### 9.3 Consequence for the mission

Mission item 5 ("`CLAUDE_CODE_OAUTH_TOKEN` from a protected source, no
per-home login") is delivered through Curator:

| Step | Owner | State |
| --- | --- | --- |
| Source, enrolment, `pkg/localcredential` (T1–T3a) | Curator | Specified; not in rc.7 |
| Interactive mode qualification | Curator | Open |
| Hosted token route amendment (Appendix B) and its qualification | Curator spec, the manager, curator-run | Proposed here |
| curator-run consumption: native via the API; hosted through `credential-node/1` | curator-run (S5) | Implementable once the API is pinned |

S5 lands the launcher side behind the API pin. It shows no token behaviour
until Curator publishes the pin.

## 10. Signed distribution

`curator-run` ships in the signed Curator release, built from a pinned,
signed launcher tag:

- The Curator release workflow builds both binaries with the release
  ldflags, signs and notarizes both, and packs both into one archive and
  installer.
- The release notes record the launcher tag.
- `curator --version --json` reports it as `providers.curator-run`. This
  version coupling is new (C8).

The installer places `curator-run` in Curator's install directory, which
is the first trust root (environments §11, E4). Under **revision B** that
guarantees `curator run` resolves it there. Under **revision A**, which is
`provider-trust-roots: revision-A` on ivmbp today, `PATH` still selects,
and a `curator-run` outside the roots runs with a warning. The installer
therefore also checks that the install directory comes before any other
`curator-run` on the operator's `PATH`, and prints the fix if it does not.

Other rules:

- `curator-*` names cannot come from profiles (environments §9.4,
  `environment_reserved_command_name`). Nothing here relaxes that.
- **Release tags** are signed with the separate agent release key, never
  the commit key (§6.3).
- The repository supplies a reproducible `make release-binary VERSION=…`
  target and the signed tag.
- The release-workflow change is owned by `ivmbp_curator_orch`.
- The launcher repository's own releases are for development. Hosts take
  the distribution's.

## 11. Diagnostics added

Detail vocabularies are closed (§5.6). Grammar names:

| Name | Values |
| --- | --- |
| `<handle>` | `SES-[A-Za-z0-9]{1,48}` |
| `<id>` | the manager or Werkstatt id grammars of §§4.2, 6.1 |
| `<cap>` | a §5.2 capability token, or one of `launch-exact-detached`, `launch-exact-doorbell`, `weights-pin` |
| `<env>` | a §4.2 SPEC env-id |
| `<path>`, `<field>` | sourced only by the launcher from its own configuration or validated record, never from a plugin |

| Code | Exit | When | Details (field: vocabulary) |
| --- | ---: | --- | --- |
| `host_member_withdrawn` | 2 | `defaults.json` has `host` (§3.1), or `migrate` meets mixed values | `path`; `reason`: `present`, `mixed_values` |
| `session_manager_registry_invalid` | 2 | registry unreadable or failing the checks (§4.2) | `path`; `reason`: `unreadable`, `parse`, `schema`, `symlink`, `owner`, `mode` |
| `session_manager_registry_shadowed` | 2 | user registration while a system registry exists | `path` |
| `session_manager_not_registered` | 2 | `select`/`unregister` of an unknown id | `id`: `<id>` |
| `session_manager_registry_busy` | 1 | another registry mutation holds the lock (§4.3 rule 8) | none |
| `session_operation_conflict` | 1 | a reused `operation_id` with a different request digest (§5.7) | none |
| `session_operation_expired` | 1 | `submitted_at` is older than the retention window (§5.7) | none |
| `session_generation_mismatch` | 1 | `expected_generation` differs from the current one (§5.2) | none |
| `session_board_goal_unavailable` | 1 | manager capable, no workspace board | none |
| `werkstatt_not_found` | 1 | no record | `id` |
| `werkstatt_record_invalid` | 2 | the record fails the schema | `id`, `field` |
| `werkstatt_session_manager_required` | 1 | `up` with an empty or unselected registry | `state`: `empty`, `unselected` |
| `werkstatt_already_up` | 1 | an active generation holds the key | `session`: `<handle>` |
| `werkstatt_clone_missing` | 1 | the clone is absent or not a directory | `path` |
| `werkstatt_git_layout_invalid` | 2 | bad separate-git-dir layout | `reason`: `missing`, `inside`, `dotgit_present`, `worktree_mismatch` |
| `werkstatt_context_leak` | 2 | an instruction file above the clone | `path` |
| `werkstatt_context_check_failed` | 2 | an ancestor could not be checked | `path` |
| `werkstatt_identity_missing` | 1 | no a2a credential file | `identity`: from the record |
| `werkstatt_not_up` | 1 | a dispatch edit with no liveness record, or with only an override-only record (§6.5) | `id` |
| `werkstatt_busy` | 1 | `not-busy` refusal, the lock is held, or `recycle` timed out while draining | `state`: `busy`, `draining`; `operation`: `in_progress` |
| `werkstatt_liveness_write_failed` | 1 | session up, liveness file not written | `session`: `<handle>` |
| `doorbell_setup_failed` | 1 | hook install or verification failed | `step`: `install`, `verify` |
| `fragment_version_unsupported` | 2 | Muse with a pre-v4 fragment | `environment`: `muse`; `required`: `launch-env-fragment-v4` |
| `credential_reserved_name` | 2 | a reserved credential name in `child_env` | `name`: one of the reserved names |
| `migrate_destination_exists` | 2 | `migrate` would overwrite a different file | `path` |
| `selector_unavailable` | 1 | enforcing selector failed, timed out or answered invalidly (§3.4) | `reason`: `timeout`, `exit`, `invalid` |
| `selector_abstained` | 1 | enforcing selector returned a typed abstention (§3.4) | `code`: `[a-z0-9_]{1,64}` |
| `werkstatt_operation_unresolved` | 1 | the manager owning a pending operation or a live generation is missing or changed (§§5.7, 6.2) | `reason`: `manager_missing`, `manager_changed`; `operation`: UUID |
| `session_host_delegated_unauthorized` | 16 | the selected route is delegated, before Q1 sign-off and without a trial authorisation (§5.8) | none |

**Reused with their meaning.** These codes keep their meaning. Plugin-borne
detail values are allowed only from the vocabularies shown:

- `session_host_missing`, with `reason`: `no manager registered`,
  `no manager selected` or `registered executable missing`;
- `session_host_unavailable`, with `reason`: `timeout`, `transport`,
  `not_found`, `prepared`, `cancelled`, `unreachable` or
  `index_unreadable`;
- `session_host_protocol_error`, `session_host_protocol_unsupported`;
- `session_host_capability_missing`, with `capability`: `<cap>`;
- `session_host_provider_unsupported`, with `environment`: `<env>`;
- the `session_resume_*` codes, `session_liveness_unknown` (`operation`:
  UUID), `policy_refused`, `secret_policy_violation`, `launch_plan_invalid`
  and `host_configuration_conflict`;
- task-board's `session_curator_identity_stale`, passed through from manager
  admission (§5.7);
- the Curator local-credential refusals (protocol §9), passed through with
  no details: `credential_injection_unavailable`,
  `credential_source_conflict`, `credential_node_missing`,
  `credential_store_unreadable`, `credential_binding_mismatch`,
  `credential_source_stale`, `credential_source_revoked`,
  `credential_source_expired` and `environment_credential_store_unsupported`.

`session_host_default_not_ready` is retired [decided Q2].

## 12. What other lanes must deliver

| Owner | Item |
| --- | --- |
| task-board (infra-exit, `#skill-project-management`) | Host review TASK-261018-m9kukq estimates 18–30 engineer-days for the usable Claude/Codex surface. Registration goes through curator-run's registry: task-board emits its manifest and never writes the registry itself. If curator-run is missing at install, that is reported as a deferred registration, never claimed as done. Conditions from infra-exit: the board-bound mode stays byte-identical, and the board-less state root is keyed so that it never collides with a board's manager lock. 1. `task-board session-manager register --for curator-run` (§4.3), called by its installer. 2. A plugin entry `task-board session-manager plugin <op>` implementing §5: `launch-delegated/1`, `resume/1`, `attach/1`, `status/1` with operation outcomes, `cancel`, `list/1`, `stop/1` with `not-busy`, `admission/1` covering launch and resume, `labels/1`, `board-goal/1`, `doorbell-codex/1` with a per-session owner and the rebind gate (§8.2), `doorbell-claude/1`, and later `credential-node/1` (Appendix B). It must pass the §5.5 suite. 3. **A board-less session daemon mode:** keyed by the user state root, not a board directory; sessions keyed by cwd and labels; no primary goal unless `board_goal: workspace`. 4. `--profile` for hosted sessions (TASK-261018-cliw6c, in flight). 5. The v4 Muse consumer (TASK-261018-yzorfb). 6. `task-board claude\|codex` become aliases of `curator run <env> --hosted --board-goal` (Decision 0021 §5). This is not a blocker. |
| Router lane (`#model-routing`) | Provide `cmr recommend --format json` answering the §3.4 request and response. Freeze the `station` fields with task-board against §6.1. Read `werkstatt-liveness-v1` as the snapshot liveness source, with `unknown` treated as not dispatchable. |
| `ivmbp_curator_orch` (Curator) | 1. The release-workflow change (§10, C8). 2. The CIP-0016 amendment (Appendix A), after Ivan's sign-off. 3. Implement `local-setup-token-source` T1–T3a and publish the `pkg/localcredential` pin, plus fragment v6. 4. Qualify the interactive mode, and file Appendix B (C10). 5. Manifest-declared home-level hooks under strict mode (§8.1). 6. The Curator config-home CIP. |
| Session-host extraction (`swarma-session-host`) | The first conforming manager after task-board. The extraction research (`#curator` › curator-run, 2026-10-18) recommends four things that map onto this protocol: operation ids with generation fencing (§5.7), per-session runners that survive a host restart (`survives-upgrade`), a stop tombstone, and resume that never becomes a new conversation (§5.3). |
| a2a (Courier) | 1. Verify the native Codex adapter on app-server 0.162.x (§8.2). 2. Optionally, a live-owner release for a generation, so that a manager can retire a binding without the owner's death. |

## 13. Compatibility and migration

- **Default behaviour.** A machine without `launcher.json`, a registry and
  `ax.json` behaves as `auto` with an empty registry: native, plus
  `notice-empty`. Compared with today, that line is the only visible
  difference.
- **`curator-run migrate`** first computes a complete plan and prints it.
  Then it applies the plan all or nothing, and on any failure it restores
  from the backups it made. `--dry-run` prints the plan and stops. It never
  applies anything, writes no file and makes no backup. The plan's steps:
  - **Copy.** Each old-path file is copied to the same layer's new
    directory. User-layer files are always copied. System-layer files are
    copied only when the process can write `/etc/swarma/launcher/`;
    otherwise one line lists them. Old files are never moved or deleted.
  - **Existing destination.** The launcher compares an existing destination
    with the bytes that migrate **would produce** from the current old file,
    after the `host` transformation below.
    - Equal bytes count as already migrated.
    - A destination that is an exact copy of the raw old file is transformed
      in place, and its pre-rewrite bytes are kept in `.bak`.
    - Any other bytes refuse `migrate_destination_exists` (2), and nothing is
      written. A newly generated `launcher.json` has no prior file, so it
      gets no backup.
  - **The `host` member.** It is removed from every `defaults.json` entry.
    An entry that held only `host` is removed entirely, because an empty
    entry is invalid. Then:
    - **System layer:** if every entry that had `host` had the same value,
      that value becomes the system `launcher.json` `host_mode`. Mixed
      values refuse `host_member_withdrawn` with `reason=mixed_values`, and
      nothing changes.
    - **User layer:** the `host` values are dropped, because `host_mode` is
      machine-only. One line names them, so the operator can ask the
      machine owner.
  - **Backups.** Each rewritten file keeps `<name>.bak` beside it.
  - **Writes** are atomic.
  - **Idempotent.** A second run reports that nothing is left to do.
- **Exact plan.** The dormant SPEC §4.9 payload and the `task-board session
  launch-plan` receiver are kept. They become `launch-exact/2` once the
  CIP-0016 successor is published (§0.1).
- **`--hosted`** keeps its spelling. It now means "the selected session
  manager" (C4).
- **Pins.** The pinned agents-management moves to a release carrying v4
  Muse and the managed Codex plan fields (v0.5.68 or later), as a separate
  leaf.

## 14. Delivery slices

Each slice is one board task, implemented by the developer Werkstatt and
reviewed by the reviewer Werkstatt. Every slice carries negative tests for
its refusals, next to positive controls.

| # | Slice | Depends on | Acceptance without task-board? |
| --- | --- | --- | --- |
| S1 | Config home and fallback, `launcher.json`, `host` withdrawal, `migrate` (§§3.1, 4.1, 13) | spec | Yes |
| S2 | Registry, states, `session-manager` commands, registration rules (§§4.2–4.3) | S1 | Yes |
| S3b | The model-selection seam (§3.4): shadow and enforce, against a fake selector | S1 | Yes |
| S3 | Plugin client: the transport table, bounds, stderr discard, fd 4 status reuse, value validation and poisoning, outcomes, `cancel` and durable `op.json` correlation (§§5.1–5.2, 5.6–5.7). The conformance suite and the fake manager (§5.5). | S2 | Yes |
| S4 | Host-mode resolution and notices (§3.2); the activation boundary (§5.8); hosted `launch`, `resume` and `attach` through the plugin (§5.3) | S3 | Yes, as launcher logic. A fake host is not runtime qualification. |
| S5 | Credential projection reading; native start through `pkg/localcredential`; the hosted `credential-node/1` gate; reserved names (§9) | S3, Curator's API pin | Launcher side, behind the pin. Token behaviour needs Curator's T1–T3a and the Appendix B qualification. |
| S6 | Werkstatt record, `up`/`down`/`recycle`/`status`/`list`, admission, locks, liveness (§6), with injectable record and a2a-presence sources | S4, S5 | Yes |
| S7 | Doorbell requests and the Claude transitional hooks (§8.1); Codex doorbell request handling against the fake (§8.2) | S6 | Launcher side yes; real delivery in S10 |
| S8 | Muse v4 natively, and hosted forwarding against the fake (§7) | S4 | Launcher side yes; real hosting in S10 |
| S9 | Reproducible release target and tag (§10) | S4 | The repository part yes. Signed and notarized distribution: Curator. |
| S10 | Live e2e on ivmbp through task-board's plugin: one Claude and one Codex Werkstatt, a full task round trip, the doorbell, resume after restart; Muse once its consumer lands | S6–S8, task-board items 1–3 | This is the first claim that anything works for real. For `launch-exact/2`, the three-runtime release rule holds (C2). |

## 15. Questions and answers (2026-10-18)

Asked of `ivmbp_curator_orch` in `#curator` › `curator-run`, and answered as
Ivan's delegate.

1. **Q1: transitional launch shape. Answer (b), pending Ivan.**
   - `launch-exact` remains the target.
   - `launch-delegated/1` is declared per manager and per runtime. It is
     retired for a runtime once the manager advertises an exact form for it
     that also covers detached launches (Appendix A).
   - It is recorded as a proposed CIP-0016 amendment (Appendix A), with the
     stated limits.
   - It needs Ivan's sign-off, because it deviates from an accepted owner
     design.
   - It is not used as a default before that sign-off (§5.2).
2. **Q2: upgrade without hang-up. Yes.** The machine mode replaces the
   gate, and the upgrade behaviour becomes a disclosure line (§3.3).
3. **Q3: config element. `launcher`** (§4.1).
4. **Q4: Werkstatt record. As proposed.** The board's `station` element
   when it exists, the local file until then. The fields stay identical to
   the router snapshot (§6.1, §6.5).
5. **Q5: setup-token interim source. The Keychain is accepted**, with these
   constraints:
   - it is read non-interactively and refused if it would prompt;
   - it is never written to a home file, argv, plan or fragment;
   - it is redacted in diagnostics.

   **Superseded in revision 3 (C10).** The accepted
   `local-setup-token-source.md` admits no node Keychain backend, and it
   makes Curator the owner of the source, the binding and the final start
   API. curator-run consumes that API (§9). Appendix B proposes the
   interactive and hosted routes the mission needs.

Still open, not blocking:

- `host_mode_locked`, which would forbid `--native`;
- a per-env host mode;
- whether `werkstatt up` should post the Werkstatt's arrival to its room;
- an a2a live-owner release (§8.2).

## Appendix A. Proposed CIP-0016 amendment: delegated local launches

Proposed text, for `ivmbp_curator_orch` to file in curator-spec once Ivan
signs off Q1.

> **Amendment (proposed 2026-10-18): delegated local launches.** Until a
> session manager advertises the exact-plan successor capability, a
> registered local session manager MAY host interactive sessions through
> `launch-delegated/1` of the curator-run session-manager protocol.
>
> **Division of work.**
> - `curator-run` remains the front door. It resolves the profile and the
>   fragment, the model and effort, the permissions, the system-prompt
>   opt-in, the typed resume intent and the credentials.
> - It hands the manager a closed request that names them.
> - The manager's caller-side component composes the process through the
>   same agents-management construction site (Decision 0019), against the
>   profile's managed home.
> - The manager MUST refuse when its fragment digest differs from the
>   request's.
> - The manager MUST NOT add profile content, change any resolved value, or
>   enable Remote Control.
>
> **Stated limits.**
> - There is no session snapshot. A running session uses the live managed
>   home, so repair, profile switching and skill sync can change it
>   underneath the session. The CIP-0014 isolation does not hold.
> - "A host never composes" holds for the daemon only, not for the
>   manager's caller-side component.
> - The CIP-0016 digest-covered provenance is replaced by the request's
>   profile pin and fragment digest.
>
> **Retained rules.** Typed resume never creates a new session. Hosted
> refusals never fall back to native.
>
> **Activation.** It is per runtime: Claude Code and Codex first, Muse once
> the manager's v4 consumer is qualified. The joint three-runtime rule
> continues to govern the exact-plan release.
>
> **Retirement and backend selection.** The mode is declared per manager
> and per runtime. For a given runtime it is retired once the manager
> advertises the exact-plan capability for that runtime, together with a
> detached form; curator-run then uses exact plan for attached launches of
> that runtime. Until the exact form supports detach, detached launches
> keep the delegated route. Exact support for one runtime retires nothing
> for another. The backend-selection
> question of section 2 is answered by the curator-run session-manager
> registry. An absent backend selection becomes "no registered manager",
> and task-board's installer registers task-board.

## Appendix B. Proposed amendment to the local setup-token source: interactive and hosted routes

Proposed text, for `ivmbp_curator_orch` to file in curator-spec as an
amendment to `protocol/local-setup-token-source.md` §7.

> **Amendment (proposed 2026-10-18): interactive and delegated-hosted
> setup-token routes.** Two routes are added to the §7 qualification table.
> Each route is admitted only after its own behavioural qualification.
>
> 1. **Direct local interactive.** `curator-run` is the final executor and
>    calls `localcredential.AuthorizeAndStart` with a non-nil `Terminal`.
>    Qualification adds to §7 the interactive-specific leak surfaces:
>    - terminal scrollback and transcript;
>    - `/login` inside the session, which then shows the CIP-0010 C3.6
>      warning;
>    - settings reload while the session runs.
> 2. **Delegated local hosted, same UID** (only with the CIP-0016 delegated
>    amendment).
>    - The manager's per-session owner process, running under the enrolling
>      UID, calls `AuthorizeAndStart` with its own PTY as `Terminal`.
>    - The caller supplies only the fragment's `source-projection-v1`. The
>      owner compares it with its own Curator resolve and refuses
>      `credential_source_stale` on any difference.
>    - Restart and resume call the API again, under the node lock. No
>      material, binding or token-derived value is stored by the daemon,
>      the plan, the session record or any receipt.
>    - The daemon itself never looks up material.
>    - This route does not change the exact-plan matrix (§7, last
>      paragraph). Exact-plan hosting keeps its own future token route.
>
> For both routes, `credential-injection/1` is advertised per qualified
> tuple: route, platform, harness release and configuration. Without
> qualification, the routes refuse `credential_injection_unavailable`.

**Publication gates for Appendix B.** Neither route is published by copying
`source-projection-v1`. Before activation, the amendment must publish four
things:

- the route strings;
- the `pkg/localcredential` version pin;
- for the hosted route, the carrier rule;
- the qualification tuples.

**The carrier rule.** In the delegated request, `credential_source` is a
**non-authoritative staleness reference**. The manager's per-session owner
re-resolves the source through Curator and compares it with the reference,
and the request confers no authority. It is not an embedding of source
metadata into a jointly published plan, which local-setup-token-source §5
would require to be digest-covered. The amendment must either confirm that
classification, or define canonical digest coverage of the whole delegated
request.
