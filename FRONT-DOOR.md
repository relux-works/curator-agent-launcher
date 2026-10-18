# curator run as the front door — specification amendment

**Amends:** [SPEC](SPEC.md) `0.5.1-draft` → `0.6.0-draft` (target; nothing
here is implemented yet).
**Status:** revision 2, draft. `ivmbp_curator_orch` accepted revision 1
(0ed45c5) as the basis on 2026-10-18 with four must-change points. Revision 2
folds those in, together with the independent review `review-1` (verdict
rework: four blocking and four should-fix findings). Implementation of S1–S3
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

The launcher keeps its non-goals (SPEC §1). Its only session-related state
is the Werkstatt liveness record (§6.5). That record is observation, not
authority: the session manager remains the truth about processes.

## 2. Surface

```text
curator-run <env-id> [--hosted | --native] [--board-goal]
            [--a2a <identity>] [existing SPEC §3 flags] [--] <native args...>

curator-run werkstatt up <id> [--attach] [--new]
curator-run werkstatt down <id> [--force]
curator-run werkstatt recycle <id>
curator-run werkstatt status [<id>] [--json]
            [--set-task <ID> | --clear-task] [--queue-depth N]
curator-run werkstatt list [--json]

curator-run session-manager list [--json]
curator-run session-manager show <id> [--json]
curator-run session-manager register --manifest <path|-> [--select | --no-select]
curator-run session-manager select <id>
curator-run session-manager unregister <id>

curator-run credential enroll <env-id>
curator-run migrate [--dry-run]
```

- `werkstatt`, `session-manager`, `credential` and `migrate` are reserved
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
| none | `auto` | selected | Hosted, subject to the default-use restriction of §5.2. |
| none | `auto` | empty | Native, and exactly one stderr line, `notice-empty`. |
| none | `auto` | unselected | Native, and exactly one stderr line, `notice-unselected`. |

The two `auto` lines are fixed:

```text
notice-empty:      curator-run: no session manager registered; launching natively (curator run session-manager list)
notice-unselected: curator-run: no session manager selected (<N> registered); launching natively (curator run session-manager select <id>)
```

A registry that exists but is invalid (§4.2) is
`session_manager_registry_invalid` in every row except `--native` and
mode `native`. It never counts as empty.

Rules that carry over:

- A hosted refusal never falls back to native, to another manager, or to
  another runtime (CIP-0016 §2). `auto` decides native only from the
  registry state. Once a manager is selected, any failure refuses.
- `--native` beats the machine mode. The mode is a default for launches,
  not a lock (`host_mode_locked` is open, §15).
- Legacy `ax.json`: when it is enabled, any hosted row is
  `host_configuration_conflict` (2), as in SPEC §4.8. `ax` routing applies
  when the table yields native without an explicit `--native`.
- The SPEC §4.8 refusals keep their order and codes, and come before any
  registry read:
  - hosted `--network`: `network_scope_unsupported` (16);
  - a missing terminal: `session_host_terminal_required` (6);
  - a piped stdin: `session_host_stdin_unsupported` (6).

  `werkstatt up` without `--attach` needs no terminal.
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
  - It never binds a goal silently.
  - The `task-board claude|codex` aliases pass `--board-goal` for
    compatibility.
- **[decided Q2] Upgrade without hang-up.** The machine mode replaces the
  old defaults gate (C3). A hosted launch through a manager that advertises
  neither `survives-upgrade` nor `drains-on-upgrade` proceeds and prints one
  line: `curator-run: session manager <id> ends hosted sessions when it is
  upgraded`. The code `session_host_default_not_ready` is retired.

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
- **`argv_prefix`:** at most 4 elements, each `[a-z0-9-]{1,32}`.
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

After registration, no launch prompts about managers. `select <id>` and
`unregister <id>` are explicit commands, and an unknown id is
`session_manager_not_registered` (2). Unregistering the selected manager
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

Descriptors and streams by operation:

| Operation | stdin | stdout | fd 3 | fd 4 | fd 5 | Terminal |
| --- | --- | --- | --- | --- | --- | --- |
| `describe`, `status`, `list`, `stop` | request JSON | response JSON | — | — | — | none |
| `launch`, `resume` | request JSON | response JSON | — | — | secrets, if any | none |
| `attach` | terminal | terminal | request JSON | status records | — | fd 0–2 |
| `launch-exact` | CIP-0016 plan payload | terminal | terminal (read/write) | status records | — | as SPEC §4.9 |

Rules for the request/response operations:

- **Request:** one JSON object, at most 64 KiB, then EOF.
- **Response:** one JSON object on stdout, at most 64 KiB, then EOF.
  - Exit 0 carries `ok: true`. Any other exit carries a refusal object
    (§5.6).
  - An exit/`ok` mismatch, invalid JSON, an over-bound body, or a missing
    or different `protocol` member is `session_host_protocol_error` (6).
- **stderr** is captured, bounded at 64 KiB, and discarded. Its bytes never
  reach the launcher's output. Only their count appears, in `--json`
  diagnostics.

Rules for `attach` and `launch-exact` (the terminal operations):

- They reuse the SPEC §4.9 status channel exactly: fd 4 carries 32-bit
  big-endian length-prefixed JSON refusal records of at most 64 KiB,
  classified against the closed registry. The poison and truncation rules
  also apply.
- A typed refusal exists only as an fd-4 record. Without a record, the exit
  status is the session's own (detach is 0, otherwise the provider's), and
  it is never reclassified.
- For `launch-exact`, the arguments, fd 3, fd 4 and the plan on stdin are
  the CIP-0016 receiver transport byte for byte. That shape is unchanged
  (§0.1).

Secrets (fd 5):

- One JSON object, 32-bit big-endian length-prefixed, at most 16 KiB:
  `{"credentials":[{"name":…,"value":…,"binding":{…}}]}` (§9.3).
- The launcher writes it and closes its end before reading stdout.
- The plugin MUST read fd 5 to EOF and close it before it starts any child
  process or contacts its daemon. It MUST also set close-on-exec on fd 5 as
  its first action at startup.
- Secrets never appear in stdin, argv, environment, stdout, stderr,
  responses, records or logs.

The plugin's environment is composed by the launcher:

- `PATH` and `HOME` from the caller;
- `CURATOR_RUN_PROTOCOL=curator-session-manager/1`;
- nothing else.

The plugin owns its transport to its daemon (task-board: the private
control socket of `tb-sessiond`).

Every launcher-side copy of fds 3–5 is close-on-exec. The launcher hands the
plugin exactly the descriptors in the table.

Deadlines:

- `describe`, `status`, `list`, `stop`: 10 s;
- `launch`, `resume`: 60 s;
- `attach` and `launch-exact`: none, because they are the session.

An overrun kills the plugin's process group and is
`session_host_unavailable` (1). For `launch` and `resume`, the overrun is
followed by reconciliation (§5.7).

### 5.2 Operations and capabilities

| Operation | Request | Response | Capability |
| --- | --- | --- | --- |
| `describe` | `{}` | `{id, version, capabilities[], runtimes{env-id: [operation…]}}` | always |
| `launch` | launch request (§5.3) | session record (§5.4) | `launch-delegated/1` |
| `launch-exact` | CIP-0016 successor payload | terminal session | `launch-exact/2` |
| `resume` | resume request (§5.3) | session record | `resume/1` |
| `attach` | `{session}` on fd 3 | session exit status | `attach/1` |
| `status` | `{session}` or `{operation_id}` | session record, or `{outcome}` (§5.7) | `status/1` |
| `list` | `{labels?}` | `{sessions: [record…]}` | `list/1` |
| `stop` | `{session, mode: "graceful"\|"force", precondition: "none"\|"not-busy"}` | session record | `stop/1` |

Capability tokens. The v1 set is closed, and an unknown token confers
nothing:

- **`launch-delegated/1` [pending Q1, Ivan].** The plugin composes through
  the same agents-management construction site, from the profile the request
  names. It is a proposed CIP-0016 amendment (Appendix A).
  - **Default-use restriction, until Ivan signs off:** a manager that offers
    only `launch-delegated/1` is used for explicit `--hosted` and for
    `werkstatt up`.
  - Under machine `host_mode` `hosted` or `auto` without a flag, such a
    manager counts as **unselected**. The launch therefore goes native with
    `notice-unselected` under `auto`, and refuses under `hosted`.
  - The next revision of this document lifts the restriction after the
    sign-off.
- **`launch-exact/2`.** The CIP-0016 exact-plan shape. Its payload is the
  jointly published successor (CIP-0016 §8 question 1). When a manager
  offers both shapes, the launcher uses `launch-exact/2`.
- **`resume/1`, `attach/1`, `status/1`, `list/1`, `stop/1`.**
- **`admission/1`.** The manager enforces `admission_key` uniqueness and
  `operation_id` idempotency atomically (§5.7).
- **`board-goal/1`.** Workspace board goal binding (§3.3).
- **`doorbell-claude/1`, `doorbell-codex/1`.** See §8.
- **`credential-env/1`.** fd-5 credential intake, under the rules of §9.3.
  Advertising it is necessary, not sufficient: the manager also re-resolves
  the binding.
- **`labels/1`.** Sessions keep the request's labels, and `list` filters by
  them.
- **`survives-upgrade`, `drains-on-upgrade`.** Upgrade behaviour (§3.3).

`runtimes` maps each env-id the manager hosts to the operations it supports
for that env-id. The launcher calls `describe` once per invocation, before
composition, and computes the needed set from the request:

- a launch shape;
- `attach/1` unless `detach`;
- `resume/1` for resume selectors;
- `board-goal/1` for `--board-goal`;
- `credential-env/1` when credentials are configured (§9);
- the doorbell capability for the runtime when a doorbell is requested;
- for Werkstätten, also `admission/1`, `labels/1`, `list/1`, `status/1`
  and `stop/1`.

A missing token is `session_host_capability_missing` (6), and the detail
names the token. A missing env-id is `session_host_provider_unsupported` (6).
Both are checked before any `launch` or `resume`.

### 5.3 Launch and resume requests

**Launch** — `urn:relux:curator-run:delegated-launch` `1.0.0`, closed:

| Member | Content |
| --- | --- |
| `operation_id` | UUIDv4 chosen by the launcher, unique per invocation (§5.7). |
| `admission_key` | Absent, or `werkstatt:<id>` (§6). |
| `runtime` | Canonical env-id: `claude_code`, `codex_cli` or `muse`. |
| `profile` | `{name, lock_sha256, fragment_digest}` from the launcher's resolve. The plugin MUST resolve the same profile, and refuse `policy_refused` (16) when its fragment digest differs. |
| `cwd` | Absolute workspace. |
| `model`, `effort` | Resolved by SPEC §4.3; values, never flags. |
| `permissions` | `{mode: native\|yolo, source}` from SPEC §4.3. |
| `system_prompt` | `null`, or `{semantics: "append"\|"replace"}`: the SPEC §5 opt-in. The plugin passes it to the module. Leaving it out is not an option, because `null` is explicit. |
| `sandbox` | `{mode: "none"\|"workspace-write", writable_roots: [abs…]}`. Policy, which the plugin spells. |
| `harness_memory` | Boolean; the plugin spells it (Codex `features.memories`). |
| `session_name` | The SPEC §3 `--name` grammar. The default is `<env-id>-<utc-stamp>`, or `werkstatt-<id>` for a Werkstatt. |
| `labels` | At most 16 entries; keys `[a-z0-9.-]{1,32}`, values at most 128 bytes. |
| `child_env` | Non-secret literals for the provider child: at most 32 names matching `[A-Z][A-Z0-9_]{0,63}`, none reserved (§9.2), values at most 4096 bytes, passing the SPEC §4.9 secret-pattern check (otherwise `secret_policy_violation`, 16). |
| `board_goal` | `none` or `workspace`. |
| `doorbell` | Absent, or `{identity, heartbeat, max_wakes_per_hour}` (§8). |
| `credentials` | The names delivered on fd 5, for example `["CLAUDE_CODE_OAUTH_TOKEN"]`. Names only. |
| `native_tail` | The selector-free native tail after the module's typed-intent elevation (SPEC §4.8), verbatim. |
| `detach` | Boolean; when true, the launcher does not attach. |

**Resume** — `urn:relux:curator-run:delegated-resume` `1.0.0`, closed. It
carries the launch members `operation_id`, `profile`, `credentials`,
`doorbell`, `detach` and `runtime`, plus:

| Member | Content |
| --- | --- |
| `selector` | `{kind: "latest"}`, `{kind: "handle", handle}`, or `{kind: "provider_id", id}`: the typed intent of SPEC §4.8. |
| `scope` | `{cwd, labels?}`. `latest` is resolved within this scope. |

Resume rules:

- **No new session.** Resume never creates a session. Missing, ambiguous,
  unreadable or mismatched targets refuse:
  - `session_resume_not_found`;
  - `session_resume_ambiguous`;
  - `session_resume_unreadable`;
  - `session_resume_location_mismatch`;
  - `session_resume_identity_mismatch`.

  Unknown liveness refuses `session_liveness_unknown` (all exit 1, as
  CIP-0016 §6).
- **Same session only.** The manager resumes the same native conversation,
  with the profile from that session's record. A request profile whose name
  differs is `session_resume_identity_mismatch`. A request profile with the
  same name but a different fragment digest is `policy_refused` (16), the
  drift rule.
- **Credentials again.** Credentials are delivered again on fd 5 (§9.3).
- **Selectors stay typed.** Native resume flags in `native_tail` are never
  used to select.

What the plugin may and may not add:

- It MUST NOT add profile content, change model, effort or permissions, add
  instruction files or MCP servers, or enable Remote Control.
- It MAY add only its own host plumbing: the PTY or app-server endpoint,
  private session state, the per-session owner (§8.2), and the goal adapter
  when `board_goal` is `workspace`.
- Anything it cannot apply exactly is a refusal, never an approximation.

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
  "state": "starting|idle|busy|draining|down|unknown",
  "labels": {"werkstatt": "curator-run-dev1"},
  "generation": 3,
  "started_at": "…Z",
  "last_activity_at": "…Z",
  "doorbell": {"state": "armed|unavailable|off|unreconciled", "generation": "…", "reason": "…"}
}
```

- `state` uses the router's five states plus `unknown`, and the same
  vocabulary as the liveness record (§6.5).
- `unknown` is a real state, and the launcher MUST NOT read it as `down`.
- `session` is the manager's opaque handle.
- `doorbell.reason` comes from the closed set `unsupported_version`,
  `not_requested`, `owner_unavailable`, `bind_failed` and
  `previous_generation_uncertain`.

### 5.5 Conformance

The repository ships `internal/sessionmanager/conformance`. The suite runs a
candidate plugin through every operation it advertises. Each negative case
has a passing control:

- an unknown operation;
- malformed, over-bound and exit/`ok`-mismatched responses;
- a protocol mismatch;
- a missing capability;
- an unsupported runtime;
- a profile digest mismatch;
- a secret in `child_env`;
- a reserved name in `child_env`;
- `stop` of an unknown session;
- `stop` with `not-busy` on a busy session: refused, and no state, gate or
  process change is observed;
- concurrent `launch` with one `admission_key`: exactly one session;
- a retried `operation_id`: the same result, with no second process;
- `resume` of a missing, an ambiguous and a down-mismatched target: refused,
  and no session created;
- `unknown` liveness;
- fd 5 not inherited by any grandchild;
- no secret byte in stdout, stderr, responses or the manager's records (a
  grep over every artifact);
- for `attach`: a status record on fd 4 versus a plain exit status.

`cmd/curator-run-fake-manager` is the reference plugin for the suite and for
the launcher's tests. It is in-memory and in-process per invocation, with a
state file under a test directory. A manager is **conforming** when it
passes the suite for the capabilities it advertises. Werkstätten (§6) use
only protocol operations, so any conforming manager with the Werkstatt set
(§5.2) hosts them.

### 5.6 Refusals

The refusal object is `{protocol, ok: false, code, details}`. Rules:

- `code` comes from the SPEC §6 registry plus §11. A code outside it is
  `session_host_protocol_error`.
- `details` is a flat object of string values. Only the fields §11 lists
  for that code are kept; anything else is dropped silently.
- The launcher prints its own constant message for the code and the kept
  details. Plugin text is never printed.
- Exit status follows the registry.

### 5.7 Admission, idempotency and reconciliation

- **Admission.** With `admission/1`, the manager admits at most one
  non-`down` session per `admission_key`, atomically. A second launch is
  refused `werkstatt_already_up` (1), with the detail `session=<handle>`,
  and has no effect.
- **Idempotency.** A launch or resume whose `operation_id` the manager has
  already seen returns the recorded outcome and never starts a second
  process.
- **Lost answer.** When a launch or resume times out or loses its answer,
  the launcher calls `status {operation_id}`. The outcome is one of
  `not_found`, `prepared`, `admitted`, `started` (with the session record)
  or `unknown`.
  - `not_found` and `prepared` are refused `session_host_unavailable` (1).
    The manager releases a prepared reservation.
  - `admitted` and `started` complete the invocation normally.
  - `unknown`, or no answer at all, is `session_liveness_unknown` (1). The
    launcher never retries under a new `operation_id`.
  - A later `werkstatt status` reconciles the result.
- **Local serialisation.** `werkstatt up`, `down` and `recycle` hold an
  exclusive lock on `<state>/werkstatt/<id>.lock` for their whole run. A
  second invocation waits up to 5 s, then refuses `werkstatt_busy` (1) with
  the detail `operation=in_progress`. Manager admission still decides
  across hosts and crashed locks.

## 6. Werkstätten

### 6.1 The record [decided Q4]

A Werkstatt is project × role profile × environment, with its own managed
home, clone and identity. Its record is `werkstatt-record-v1`, closed:

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

Ordered steps. Steps 1–8 have no effect outside the launcher. Step 9 is the
only one that creates anything:

1. **Lock** (§5.7).
2. **Record.** Load and validate it.
   - Missing: `werkstatt_not_found` (1).
   - Invalid: `werkstatt_record_invalid` (2), with the detail
     `field=<name>`.
3. **Manager.** Resolve the selected manager (§4.2) and `describe` it. The
   Werkstatt capability set is required (§5.2).
   - Empty or unselected registry: `werkstatt_session_manager_required` (1).
     `up` is always hosted, whatever the machine mode is.
   - A delegated-only manager is allowed here (§5.2).
4. **Workspace checks.** Each is read-only. An inconclusive check refuses.
   - The clone exists and is a directory. Otherwise
     `werkstatt_clone_missing` (1).
   - If `git_dir` is set:
     - it exists;
     - its physical path is outside the clone's physical path;
     - the worktree has no `.git` entry;
     - `git_dir`'s `core.worktree` names the clone.

     Otherwise `werkstatt_git_layout_invalid` (2), with the detail
     `reason=<missing|inside|dotgit_present|worktree_mismatch>`.
   - **Context leak.** For both the lexical path as given and the physical
     path (all symlinks resolved), no strict ancestor up to `/` contains
     `CLAUDE.md`, `CLAUDE.local.md`, `AGENTS.md` or `AGENTS.override.md`.
     - Claude and Codex read these upward, and `CLAUDE_CONFIG_DIR` /
       `CODEX_HOME` do not stop that.
     - A found file is `werkstatt_context_leak` (2), with the detail
       `path=<path>`.
     - An ancestor that cannot be read or stat'ed is
       `werkstatt_context_check_failed` (2), with the detail
       `path=<path>`. It is never treated as clean.
     - Files inside config directories, such as `~/.claude/CLAUDE.md`, are
       not ancestors.
   - The a2a credential file for `identity` exists. This is checked by
     presence, never read. Otherwise `werkstatt_identity_missing` (1).
5. **Context plane.** Resolve the fragment for `profile` (SPEC §4.1, with
   `--repair`).
6. **Spawn-plane admission** of the record's `model` and `effort`, as SPEC
   §4.3.
7. **Credentials** for the runtime (§9). For `claude_code` with a source
   configured, this includes the conflict checks of §9.2.
8. **Existing session.** `list {labels: {werkstatt: id}}`.
   - A non-`down` session is `werkstatt_already_up` (1), with the detail
     `session=<handle>`.
   - An `unknown` session is `session_liveness_unknown` (1).
   - A `down` session with `context_mode` `continue` and without `--new` is
     the resume target: step 9 resumes it.
9. **Start.** Exactly one manager call, with a fresh `operation_id` and
   `admission_key: werkstatt:<id>`:
   - `resume {selector: {kind: handle, handle: <down session>}}` in the
     continue case of step 8;
   - otherwise `launch`, with:
     - the record's `runtime`, `profile`, `model`, `effort`, `sandbox`,
       `harness_memory`, `child_env` and `doorbell`;
     - `cwd = clone`;
     - `board_goal: none`;
     - `system_prompt: null`;
     - `detach: true`;
     - the credential names;
     - the labels `werkstatt=<id>`, `generation=<previous + 1>` and
       `identity=<identity>`.

   Both calls are subject to §5.7. There is no first-run bootstrap prompt:
   the orchestrator's first task message is the bootstrap.
10. **Liveness.** Write the §6.5 record from the returned session record.
    If the write fails, the session is still up: print the handle and
    refuse `werkstatt_liveness_write_failed` (1). The next `status`
    rewrites the record.
11. **Attach.** `--attach` attaches afterwards. Detaching leaves the
    Werkstatt up.

`--new` forces a new session (a new thread) in the continue case.
`context_mode` `fresh` always launches new.

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
  (§6.2 step 8). Only `--new` or `recycle` starts a new thread.
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
  "manager": "task-board", "session": "SES-…", "generation": 3,
  "started_at": "…Z", "last_heartbeat_at": "…Z", "last_activity_at": "…Z",
  "doorbell": "armed|unavailable|off|unreconciled",
  "current_task": null, "queue_depth": 0 }
```

- **Who writes:** curator-run writes the file on `up`, `down`, `recycle` and
  every `status`. Each time it copies `state`, `session`, `generation`,
  `started_at`, `last_activity_at` and `doorbell.state` from the manager's
  session record, and infers nothing else.
- **`last_heartbeat_at`** is the time of that manager read. A stale value
  means nobody looked, not that the session is down.
- **`current_task` and `queue_depth`** are written only by the orchestrator,
  through `werkstatt status <id> --set-task/--clear-task/--queue-depth`.
- **`unknown`** is outside the router's five states. It means the manager
  answered `unknown`, or did not answer. The router treats it as not
  dispatchable.
- **Mirroring:** the router reads this file. Once `station` exists, the
  launcher also mirrors `state`, `last_heartbeat_at` and `session` to it.
- **a2a liveness** (`a2a status set` / `who`) is the Werkstatt's own
  heartbeat. `status` shows both, unmerged.

### 6.6 `down`, `recycle`, `status`, `list`

- **`down <id>`:** under the lock, one call:
  `stop {session, mode: graceful, precondition: not-busy}`.
  - The manager checks the precondition atomically. A busy session refuses
    `werkstatt_busy` (1), with the detail `state=busy`, and nothing changes:
    no gate, no stop, no record.
  - Only after the manager accepts the stop does it run the doorbell
    retirement of §8.2.
  - `--force` sends `mode: force, precondition: none`.
  - An already `down` session exits 0 with one line.
  - The liveness record is written from the answer.
- **`recycle <id>`:** `down` (graceful, `not-busy`), then `up --new`, in
  one lock. If `down` refuses, nothing else happens.
- **`status [<id>]` and `list`:** the record, the manager status, the a2a
  `who` line and the doorbell state. `--json` gives `werkstatt-status-v1`, an
  array of `{record, liveness, a2a}`.

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
  <cwd>/.claude/settings.local.json`. The command is idempotent.
- It reads the file back. It checks only the three hook entries and
  `env.A2A_AGENT`.
- A failure is `doorbell_setup_failed` (1), with the detail
  `step=install|verify`.

`doorbell-claude/1` means the manager's Claude composition keeps
project-local settings active.

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

**Retirement**, in this order, on `stop` and before any rebind:

1. `a2a runtime gate --generation G stopping`.
2. Read `a2a runtime status`. An in-flight attempt is waited for up to the
   stop deadline. If it is still uncertain after that, the doorbell becomes
   `unreconciled`, and that state is recorded.
3. Stop the thread.
4. The owner exits.
5. The manager observes the owner's death and runs
   `a2a runtime abandon --generation G`.
6. The doorbell becomes `off`.

A `not-busy` precondition refusal happens before step 1 (§6.6).

**Rebind.** A new `up` for the same identity does not bind while the
previous generation is `unreconciled`. The session starts with
`doorbell.state: unreconciled`, reason `previous_generation_uncertain`, and
`up` prints one line. Reconciling it is the orchestrator's decision, outside
v1. a2a may add a lifecycle extension for live-owner release later (§12).

**Unsupported versions.** The a2a native adapter verifies app-server
0.160.0–0.161.0. On the installed Codex 0.162.1 the bind reports
unavailable, so the record shows `doorbell.state: unavailable`, reason
`unsupported_version`, and `up` prints one line. The launch is not refused.
Qualifying 0.162.x is an a2a item.

## 9. Claude setup-token injection [decided Q5]

### 9.1 Source

The token reaches the provider as `CLAUDE_CODE_OAUTH_TOKEN`.

**The source choice is user-owned** (CIP-0010 C3.1).
`credentials.json`, schema `curator-run-credentials-v1`, lives in the
**user layer only** (§4.1):

- A system-layer `credentials.json` is `defaults_config_invalid`. No system
  lock can select a source.
- A single-user host needs no root, and the Keychain item stays in the
  user's login keychain.
- An agent account with its own OS user has its own user layer.
- This follows CIP-0010 in place of the machine-first wording of the
  Curator review point 2. The aim of that point, no root on single-user
  hosts, is met.

```json
{ "schema": "curator-run-credentials-v1",
  "claude_code": { "source": "macos-keychain", "generation": 1,
                   "service": "swarma.claude-setup-token", "account": "default",
                   "profiles": ["*"] } }
```

| `source` | Behaviour |
| --- | --- |
| `curator-node` | Through Curator's CIP-0010 node and its protected binding, once that ships. This is the target. |
| `macos-keychain` | One generic-password item, read through the Security framework with `kSecUseAuthenticationUIFail`. If reading it would prompt, or the item is absent, the launch refuses `credential_store_unreadable` (1). There is no fallback. |
| `none` | No injection; the home's own login applies. This is the default when the file or the entry is absent. |

**Enrolment** is out of band:

- `curator-run credential enroll claude_code` reads the token from a TTY
  with echo off, stores it in the configured item and increments
  `generation`.
- The token is never an argument.
- One token serves every home. No home gets a copy.

### 9.2 Conflicts

When a source other than `none` applies to the launch, these refuse
`credential_source_conflict` (2). The detail is `source=<kind>`, where kind
is one of `ambient_env`, `api_key_helper`, `native_login`, `cloud_provider`
or `child_env`:

- **`ambient_env`:** any of `CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_API_KEY`
  or `ANTHROPIC_AUTH_TOKEN` in the caller's environment, the fragment `env`
  or the plan's own literals.
- **`cloud_provider`:** `CLAUDE_CODE_USE_BEDROCK` or `CLAUDE_CODE_USE_VERTEX`
  set.
- **`api_key_helper`:** an `apiKeyHelper` key in the managed home's
  `settings.json`, or in the workspace's `.claude/settings.json` or
  `.claude/settings.local.json`.
- **`native_login`:** the managed home holds its own login. Two forms count:
  - a `.credentials.json` in `CLAUDE_CONFIG_DIR`;
  - the Keychain item that Claude keys by that `CLAUDE_CONFIG_DIR`. That
    item is detected by an attribute-only query (no secret data) with
    `kSecUseAuthenticationUIFail`. A query that cannot complete without
    interaction refuses `credential_store_unreadable`.
- **`child_env`:** a reserved name in a request's `child_env`. This is
  refused earlier, as `credential_reserved_name` (2).

The reserved names are `CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_API_KEY`,
`ANTHROPIC_AUTH_TOKEN` and every name the fragment's credential section
reserves.

### 9.3 Binding and injection

**The binding.** The binding is the user-layer `credentials.json` entry,
with these fields:

- env-id;
- source kind and item reference (service, account);
- `generation`;
- the profile allowlist (`profiles`, where `"*"` admits any);
- the file's owner UID.

The **final executor** re-resolves it at exec time. A caller-supplied
reference never substitutes for that check.

**Native launches.** curator-run is the final executor:

1. It re-reads the entry.
2. It checks the env-id, the profile allowlist and the UID.
3. It reads the token.
4. It puts the token only into the exec'd provider's environment.

**Hosted launches.** The manager is the final executor:

- The fd-5 object carries `{name, value, binding: {env, profile,
  generation, source_kind, service, account}}`.
- The manager MUST re-read the same user-layer `credentials.json` itself:
  - the same path rules;
  - the same file checks;
  - the same UID as the manager process.
- It compares env, profile allowlist, `generation` and item reference.
  Any mismatch refuses `credential_binding_mismatch` (2), and the value is
  dropped.
- It injects the value only into the provider process's environment.
- It persists the value nowhere: not in records, restart templates, plans
  or logs.
- `credential-env/1` alone never authorises injection.
- A resume or cold restart gets the token again from `curator run …
  resume` (§5.3). The manager never stores it in order to restart on its
  own.

**MCP children.** CIP-0010 C3.4 requires that injected names never reach
MCP children.

- The launcher asks the module, and the fragment's MCP channel, to strip
  the reserved names from every MCP server's environment. For Claude, that
  is an `env -u` wrapper or an equivalent declared by Curator's MCP
  materialization. This is a Curator and agents-management item (§12).
- Until a probe shows that no MCP child sees the token, S5 may be
  implemented but **not activated**. An activated launch that carries both
  a token and MCP servers would otherwise break C3.4.

**Residual.** The harness itself can read its own environment, and so can
its shell-tool children. CIP-0010 states this residual.

**Redaction.** The value never appears in diagnostics, `--json`, liveness
records or `status`. Only the source kind, the service, the account and the
generation may appear.

No Remote Control: the launcher adds no RC flag.

### 9.4 Qualification before activation

Each item is a live probe, with a positive control and a negative control:

- **Fresh home.** Claude Code with `CLAUDE_CODE_OAUTH_TOKEN` in a fresh
  managed home starts interactively, with no login screen and no first-run
  onboarding screen.
- **Effective source.** No Keychain read of the native item occurs while the
  token is present. This is CIP-0010's effective-source check.
- **Native login refuses.** A home with a native login and a configured
  source refuses `credential_source_conflict`. The token does not win
  silently.
- **MCP children.** A probe MCP server does not see the token.
- **fd 5 lifetime.** A grandchild of the manager inherits no fd 5.
- **No leak.** A grep over the plan, the fragment, the records, the logs
  and `status` output finds no token byte.

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

Each row's `details` lists the only fields that may be kept (§5.6).

| Code | Exit | When | Details |
| --- | ---: | --- | --- |
| `host_member_withdrawn` | 2 | `defaults.json` still has `host` (§3.1), or `migrate` meets mixed values | `path`, `reason` (`present`, `mixed_values`) |
| `session_manager_registry_invalid` | 2 | registry unreadable or failing the checks (§4.2) | `path`, `reason` |
| `session_manager_registry_shadowed` | 2 | user registration while a system registry exists (§4.3) | `system_path` |
| `session_manager_not_registered` | 2 | `select` or `unregister` of an unknown id | `id` |
| `session_board_goal_unavailable` | 1 | `--board-goal`, manager capable, no workspace board (§3.3) | `cwd` |
| `werkstatt_not_found` | 1 | no record (§6.2) | `id` |
| `werkstatt_record_invalid` | 2 | the record fails the schema | `id`, `field` |
| `werkstatt_session_manager_required` | 1 | `up` with an empty or unselected registry | `state` (`empty`, `unselected`) |
| `werkstatt_already_up` | 1 | a non-`down` session holds the admission key | `session` |
| `werkstatt_clone_missing` | 1 | the clone path is absent or not a directory | `path` |
| `werkstatt_git_layout_invalid` | 2 | bad separate-git-dir layout | `reason` |
| `werkstatt_context_leak` | 2 | an instruction file above the clone | `path` |
| `werkstatt_context_check_failed` | 2 | an ancestor could not be checked | `path` |
| `werkstatt_identity_missing` | 1 | no a2a credential file for the identity | `identity` |
| `werkstatt_busy` | 1 | `down`/`recycle` refused by `not-busy`, or the lock is held | `state`, `operation` |
| `werkstatt_liveness_write_failed` | 1 | the session is up but the liveness file was not written | `session` |
| `doorbell_setup_failed` | 1 | hook install or verification failed (§8.1) | `step` |
| `fragment_version_unsupported` | 2 | Muse with a pre-v4 fragment (§7) | `environment`, `required` |
| `credential_store_unreadable` | 1 | the source cannot be read without interaction, or is absent (§9.1) | `source_kind` |
| `credential_source_conflict` | 2 | a competing credential source (§9.2) | `source` |
| `credential_reserved_name` | 2 | a reserved name from a non-credential channel | `name` |
| `credential_binding_mismatch` | 2 | the manager's binding re-read differs (§9.3) | `field` |
| `migrate_destination_exists` | 2 | `migrate` would overwrite a different destination file | `path` |

The last three credential codes come from CIP-0010's refusal set where it
names them.

Existing codes reused with their meaning:

- `session_host_missing` and `session_host_unavailable` (detail `reason`);
- `session_host_protocol_error` and `session_host_protocol_unsupported`;
- `session_host_capability_missing` (detail `capability`);
- `session_host_provider_unsupported` (detail `environment`);
- the `session_resume_*` codes and `session_liveness_unknown`;
- `policy_refused`, `secret_policy_violation`, `launch_plan_invalid` and
  `host_configuration_conflict`.

`session_host_default_not_ready` is retired [decided Q2].

## 12. What other lanes must deliver

| Owner | Item |
| --- | --- |
| task-board (infra-exit, `#skill-project-management`) | 1. `task-board session-manager register --for curator-run` (§4.3), called by its installer. 2. A plugin entry `task-board session-manager plugin <op>` implementing §5: `launch-delegated/1`, `resume/1`, `attach/1`, `status/1` with `operation_id`, `list/1`, `stop/1` with `not-busy`, `admission/1`, `labels/1`, `board-goal/1`, `credential-env/1` with the §9.3 binding re-read, `doorbell-codex/1` with a per-session owner (§8.2), and `doorbell-claude/1`. It must pass the §5.5 suite. 3. **A board-less session daemon mode:** keyed by the user state root, not a board directory; sessions keyed by cwd and labels; no primary goal unless `board_goal: workspace`. 4. `--profile` for hosted sessions (TASK-261018-cliw6c, in flight). 5. The v4 Muse consumer (TASK-261018-yzorfb). 6. `task-board claude\|codex` become aliases of `curator run <env> --hosted --board-goal` (Decision 0021 §5). This is not a blocker. |
| Router lane (`#model-routing`) | Freeze the `station` fields with task-board against §6.1. Read `werkstatt-liveness-v1` as the snapshot liveness source, with `unknown` treated as not dispatchable. |
| `ivmbp_curator_orch` (Curator) | 1. The release-workflow change (§10, C8). 2. The CIP-0016 amendment (Appendix A) after Ivan's sign-off. 3. Manifest-declared home-level hooks under strict mode (§8.1). 4. Reserved credential names stripped from MCP children in the materialized MCP channel (§9.3). 5. The Curator config-home CIP. 6. The CIP-0010 node. |
| agents-management | Strip the reserved credential names from MCP child environments in the plans it builds (§9.3), with Curator. |
| a2a (Courier) | 1. Verify the native Codex adapter on app-server 0.162.x (§8.2). 2. Optionally, a live-owner release for a generation, so that a manager can retire a binding without the owner's death. |

## 13. Compatibility and migration

- **Default behaviour.** A machine without `launcher.json`, a registry and
  `ax.json` behaves as `auto` with an empty registry: native, plus
  `notice-empty`. Compared with today, that line is the only visible
  difference.
- **`curator-run migrate [--dry-run]`** first computes a complete plan and
  prints it. Then it applies the plan all or nothing: on any failure it
  restores from the backups it made. Its steps:
  - **Copy.** Each old-path file is copied to the same layer's new
    directory. User-layer files are always copied. System-layer files are
    copied only when the process can write `/etc/swarma/launcher/`;
    otherwise one line lists them. Old files are never moved or deleted.
  - **Existing destination.** A destination that exists with different
    bytes refuses `migrate_destination_exists` (2), and nothing is written.
    Identical bytes count as already migrated.
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
| S3 | Plugin client: transport table, bounds, stderr discard, fd 4 status reuse, fd 5 intake rules, refusal filtering, reconciliation client (§§5.1–5.2, 5.6–5.7). The conformance suite and the fake manager (§5.5). | S2 | Yes |
| S4 | Host-mode resolution and notices (§3.2), with the default-use restriction; hosted `launch`, `resume` and `attach` through the plugin (§5.3) | S3 | Yes, as launcher logic. A fake host is not runtime qualification. |
| S5 | Credential sources, conflicts, binding, native injection and fd-5 delivery (§9) | S3 | Implementation yes; activation needs the §9.4 live probes |
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
   - `launch-delegated/1` is declared per manager and retired once that
     manager advertises `launch-exact`.
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

   Revision 2 keeps CIP-0010's user-owned selection, binding and
   child-isolation rules unchanged (§9). It does not trade them for the
   interim source.

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
> and retired for a manager once that manager advertises the exact-plan
> capability. curator-run then uses exact plan. The backend-selection
> question of section 2 is answered by the curator-run session-manager
> registry. An absent backend selection becomes "no registered manager",
> and task-board's installer registers task-board.
