# curator run as the front door — specification amendment

**Amends:** [SPEC](SPEC.md) `0.5.1-draft` → `0.6.0-draft` (target; nothing
here is implemented yet).
**Status:** draft for review by `ivmbp_curator_orch` and Ivan. Implementation
starts only after acceptance.
**Board:** `relux-works/curator-run-board`, EPIC-261018-cuz7sp `front-door`,
TASK-261018-st9yo0.

Sections marked **[pending Qn]** follow the recommendation sent to
`ivmbp_curator_orch` on 2026-10-18 (`#curator` › `curator-run`). They change
if the answer differs. Section 15 lists the questions.

## 0. Sources and decisions applied

| Source | What it fixes here |
| --- | --- |
| Ivan, mission brief, 2026-10-18 | The seven deliverables: hosted by default, pluggable session managers, `werkstatt up/down`, Muse, setup-token, doorbells, signed distribution. The managed home comes from the profile and has no binding to a board's primary goal. `--native`. Mode `auto`. Machine config `hosted\|native\|auto`. |
| Ivan, via `ivmbp_curator_orch`, 2026-10-18 (binding) | A session manager registers once, at install. It is automatic when no other manager is registered. When one or more are, the installer asks once, and an agent installer asks its human. After that the choice is fixed in config, with no prompt on any launch. curator-run is installed first, together with Curator, in the signed distribution. Without a session manager an ordinary launch goes native and prints one line, and `werkstatt up` refuses. |
| Curator Decision 0010 §6 | The launcher composes three planes: spawn (agents-management), context (`curator env resolve`) and session (now: a session-manager plugin). |
| Decision 0021 and its amendment v2; CIP-0016 / `protocol/local-hosted-launches.md` | One interactive entry point. A host never composes in the exact-plan mode. No fallback after a hosted refusal. Snapshots, not live homes, for exact-plan sessions. Section 8 publication question 2 (backend configuration) is answered by §4 here. |
| CIP-0010 (draft) | setup-token: protected store, final-executor injection, never in plans or fragments. |
| CIP-0017, environments §10.2 | `launch-env-fragment-v4`, Muse `root_context`. |
| environments §9.4, §11 | `curator-*` providers come only from Curator's trust roots, never from profiles. |
| Swarm config-home convention (Ivan, 2026-10-18) | `~/.config/swarma/<element>/`, `/etc/swarma/<element>/`, `~/.local/state/swarma/<element>/`. Precedence: Werkstatt, then user, then system, then default. The old path is read for one release. |
| Werkstatt note (`swarma-docs` `editorial/werkstatt.ru.md`) and pilot runbook | The Werkstatt model, the clone layout with the git dir outside the worktree, no `CLAUDE.md`/`AGENTS.md` above the clone root, context modes, setup-token for every home, no Remote Control. |
| Router lane, `#model-routing` › status, 2026-10-18 | Station snapshot fields and the liveness vocabulary `idle\|busy\|starting\|draining\|down`. Dispatch returns (station, effort). |

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

The launcher keeps its non-goals (SPEC §1). It holds no session state of its
own, except the Werkstatt liveness record of §6.5. That record is
observation, not authority: the session manager remains the truth about
processes.

## 2. Surface

```text
curator-run <env-id> [--hosted | --native] [--board-goal]
            [--a2a <identity>] [existing SPEC §3 flags] [--] <native args...>

curator-run werkstatt up <id> [--attach] [--generation-note <text>]
curator-run werkstatt down <id> [--force]
curator-run werkstatt recycle <id>
curator-run werkstatt status [<id>] [--json]
curator-run werkstatt list [--json]

curator-run session-manager list [--json]
curator-run session-manager show <id> [--json]
curator-run session-manager register --manifest <path|-> [--select | --no-select]
curator-run session-manager select <id>
curator-run session-manager unregister <id>
```

- `werkstatt` and `session-manager` are reserved operands. They are not
  environment ids. A future environment id with either name is a
  specification conflict.
- `--untracked` remains an exact synonym of `--native` (SPEC §3).
- `--board-goal` (new) asks the session manager to bind the workspace
  board's primary goal (§3.3). Without it no goal is bound.
- `--a2a <identity>` (new) asks for doorbell wiring for that a2a identity
  (§8). It is valid only with a hosted launch. With `--native` it is a
  `usage` error.
- The `curator run …` umbrella spelling and the direct `curator-run …`
  spelling stay the same surface (SPEC §2).

## 3. Host mode: hosted by default

### 3.1 The machine mode

The host mode is machine policy. It lives in one closed file, `launcher.json`
(schema `curator-run-launcher-v1`), in the launcher's system directory
(§4.1):

```json
{ "schema": "curator-run-launcher-v1", "host_mode": "auto" }
```

- `host_mode` is exactly `hosted`, `native` or `auto`. Any other value, or an
  unknown member, is `defaults_config_invalid`.
- An absent file, or a file that omits `host_mode`, means `auto`.
- An operator file may not set `host_mode`: the machine fixes the mode. An
  operator `launcher.json` that carries `host_mode` is
  `defaults_config_invalid`, naming the machine file.
- The file follows the SPEC §4.7 open-without-following-symlink and owner
  checks.

The SPEC §4.3 `defaults.json` per-environment `host` member is withdrawn.
A v3 file that still carries `host` refuses with `host_member_withdrawn`
(exit 2) and names `launcher.json`. Migration: `curator-run migrate` rewrites
it, with the old file kept as `defaults.json.bak` (§13).

### 3.2 Resolution

The per-launch flag decides first, then the machine mode. Closed table,
first match wins:

| Flag | Machine mode | Selected manager (§4) | Result |
| --- | --- | --- | --- |
| `--native` | any | any | Native. Zero manager lookup, zero daemon probe. |
| `--hosted` | any | present | Hosted through that manager. |
| `--hosted` | any | absent | Refuse `session_host_missing` (exit 1). |
| none | `native` | any | Native. Zero lookup. |
| none | `hosted` | present | Hosted. |
| none | `hosted` | absent | Refuse `session_host_missing` (exit 1). |
| none | `auto` | present | Hosted. |
| none | `auto` | absent | Native, and exactly one stderr line (below). |

The `auto` notice line is fixed:

```text
curator-run: no session manager registered; launching natively (curator run session-manager list)
```

Rules that carry over unchanged:

- A hosted refusal never falls back to native, to another manager, or to
  another runtime (CIP-0016 §2). `auto` falls back only at selection time,
  when no manager is registered. It never falls back after a manager has been
  selected and fails.
- `--native` beats the machine mode on every machine. The machine mode is a
  default for launches, not a lock. **[pending Q2]** A future
  `"host_mode_locked": true` member may forbid `--native`. It is not part of
  v1.
- Legacy `ax.json`: when enabled, any hosted result from the table is
  `host_configuration_conflict` (exit 2), as in SPEC §4.8. `ax` routing
  applies only when the table yields native without an explicit `--native`.
  Nothing changes on machines without `ax.json`.
- The SPEC §4.8 refusals keep their order and codes: hosted `--network` is
  `network_scope_unsupported` (16), a missing terminal is
  `session_host_terminal_required` (6), and a piped stdin is
  `session_host_stdin_unsupported` (6). They are checked before any manager
  contact.
- Hosted permissions stay as SPEC §4.8 (Q-D3): the §4.3 ladder, and `yolo`
  is admitted.

### 3.3 The managed home and the board goal

- The managed home is the profile's: `--profile <name>`, else Curator's
  current profile for the scope. The launcher resolves it through
  `curator env resolve <env> [--profile P] --repair --format json` (SPEC
  §4.1) before any manager contact. Nothing binds it to a board.
- A hosted session binds **no board goal** by default. The launch request
  carries `"board_goal": "none"`. With `--board-goal` it carries
  `"workspace"`. The manager then binds the primary goal of the board it
  finds for the workspace, or refuses `session_board_goal_unavailable`
  (exit 1) if there is none. It never binds silently. This narrows Decision
  0021 §4 ("board behaviour follows the workspace") to an explicit opt-in.
  Orchestrator sessions that want the goal pass `--board-goal`, and the
  `task-board claude|codex` aliases pass it for compatibility.
- **[pending Q2]** Upgrade without hang-up (Decision 0021 §7, SPEC §4.8
  `session_host_default_not_ready`): the machine mode replaces the old
  defaults gate. A hosted launch whose manager advertises neither
  `survives-upgrade` nor `drains-on-upgrade` (§5.2) proceeds, and prints one
  stderr line:

  ```text
  curator-run: session manager <id> ends hosted sessions when it is upgraded
  ```

  `session_host_default_not_ready` is retired.

## 4. Session-manager registry

### 4.1 Locations [pending Q3]

| Layer | Directory | Who writes |
| --- | --- | --- |
| System | `/etc/swarma/launcher/` | A privileged installer only (root-owned, 0644 files) |
| User | `${XDG_CONFIG_HOME:-~/.config}/swarma/launcher/` | The operator's installers (owned by the user, no group or world write) |
| State | `${XDG_STATE_HOME:-~/.local/state}/swarma/launcher/` | curator-run (Werkstatt liveness, §6.5) |

The old `~/.config/curator-run/` and `/etc/curator-run/` are read for one
release when the new directory has no file of that name. Each such read
prints one deprecation line that names the new path. `curator-run migrate`
copies the files. Nothing is deleted automatically.

### 4.2 The registry file

`session-managers.json`, schema `curator-run-session-managers-v1`, closed:

```json
{
  "schema": "curator-run-session-managers-v1",
  "selected": "task-board",
  "managers": {
    "task-board": {
      "executable": "/Users/iv/.local/bin/task-board",
      "argv_prefix": ["session-manager", "plugin"],
      "protocol": "curator-session-manager/1",
      "registered_at": "2026-10-18T09:00:00Z",
      "registered_by": "task-board 0.9.x installer"
    }
  }
}
```

- `managers` keys match `[a-z0-9][a-z0-9-]{0,31}`. They are the manager ids
  (`task-board`, later `swarma-session-host`, any third party).
- `executable` is absolute. It is resolved once, at registration, with
  symlinks resolved. The launcher never searches `PATH` for a manager and
  never accepts an endpoint, socket or binary from a repository, a profile,
  a fragment or the environment (CIP-0016 §2).
- `argv_prefix` is at most 4 elements, each `[a-z0-9-]{1,32}`. The launcher
  invokes `executable argv_prefix… <operation>`.
- `selected` names one key of `managers`, or is absent when nothing is
  selected.
- Effective registry: if the system file exists, it decides, and the user
  file is ignored. Otherwise the user file decides. This is the same
  machine-first rule as `ax.json` (SPEC §4.6). A user cannot slip a manager
  under a machine that has registered its own.
- Every read applies the SPEC §4.7 file checks. A file that fails them, or
  does not parse, is `session_manager_registry_invalid` (exit 2). It never
  counts as "no manager".

"A manager is registered" in §3.2 means: the effective file names a
`selected` manager, its `executable` exists, is a regular file and is
executable. A selected entry whose executable is gone is
`session_host_missing`, with the detail `registered executable missing`.
It is not absence, so `auto` does not fall back to native.

### 4.3 Registration happens once, at install

A session manager's installer registers it:

```text
curator-run session-manager register --manifest - [--select | --no-select]
```

task-board exposes this as `task-board session-manager register --for
curator-run`, which runs the command above with its own manifest. The
manifest is the entry of §4.2 plus `id`, without `registered_at`. The
launcher fills in that timestamp.

Rules, closed:

1. Re-registering the same id with the same executable updates the entry
   and keeps the selection. An upgrade therefore never asks again.
2. No manager registered yet: the new one is registered **and selected**,
   with no question. This holds whether or not a TTY is attached.
3. One or more already registered, and neither `--select` nor
   `--no-select` given:
   - With a TTY, the command asks once. The question names the current
     selection and the new manager, and the answer defaults to keeping the
     current one. The answer is written to `selected`.
   - Without a TTY, it registers but does not select. It exits 0 and prints
     one line: `registered <id>; selection unchanged (<current>); run:
     curator run session-manager select <id>`. An agent installer runs
     without a TTY. It asks its human in its own channel and then passes
     `--select` or `--no-select` explicitly, or runs `select` afterwards.
4. Writes go to the user file. A privileged installer writes the system
   file. Each write is atomic (temp file in the same directory, fsync,
   rename).

After registration, no launch ever prompts about managers. `select` and
`unregister` are explicit operator commands. Unregistering the selected
manager leaves `selected` absent.

### 4.4 Install order

1. The signed Curator distribution installs `curator` and `curator-run`
   together (§10).
2. Session managers install afterwards and register (§4.3).
3. A machine with only step 1 works: launches go native under `auto` (with
   the notice), and `werkstatt up` refuses.

## 5. The session-manager plugin protocol

### 5.1 Transport

A session manager is an executable plugin, in the same family as git
credential helpers. Each operation is one subprocess:

```text
<executable> <argv_prefix…> <operation>
```

- The request is one JSON object on stdin, at most 64 KiB. Exception: the
  `attach` operation inherits the terminal and takes its request on fd 3.
- The response is one JSON object on stdout, at most 64 KiB, followed by
  EOF. Exit status 0 means `ok` in the response. A non-zero exit carries a
  refusal object (§5.6) on stdout.
- Secrets travel only on a dedicated pipe, fd 4, as one length-prefixed JSON
  object (32-bit big-endian length, at most 16 KiB). They never appear in
  stdin, argv, env, stdout, stderr or logs (§9).
- The launcher starts the plugin with an environment it composes:
  - `PATH` and `HOME` from the caller;
  - `CURATOR_RUN_PROTOCOL=curator-session-manager/1`;
  - nothing else from the caller, except names a request lists explicitly.

  The plugin owns its own transport to its daemon (task-board: the private
  control socket of `tb-sessiond`). The launcher never sees that transport.
- Every response carries
  `"protocol": "curator-session-manager/1"`. A response that is not valid
  JSON, is over the bound, or carries a different protocol is
  `session_host_protocol_error` (exit 6). The plugin's bytes are never
  echoed back (SPEC §4.9 discipline).
- Deadlines: 10 s for `describe`, `status`, `list` and `stop`; 60 s for
  `launch`. `attach` has none: it is the session. A deadline overrun kills
  the plugin's process group and is `session_host_unavailable` (exit 1).

### 5.2 Operations

| Operation | Request | Response | Required |
| --- | --- | --- | --- |
| `describe` | `{}` | `id`, `version`, `capabilities[]`, `runtimes{env-id: [ops]}` | always |
| `launch` | launch request (§5.3) | session record (§5.4) | `launch-delegated/1` or `launch-exact/2` |
| `attach` | `{session}` on fd 3, terminal on fd 0–2 | exit status = session detach/exit | `attach/1` |
| `status` | `{session}` | session record | `status/1` |
| `list` | `{labels?}` | `sessions[]` | `list/1` |
| `stop` | `{session, mode: "graceful"\|"force"}` | session record with state `down` | `stop/1` |

Capability tokens (closed in v1; unknown tokens confer nothing):

- `launch-delegated/1`: the transitional launch shape **[pending Q1]**. The
  plugin composes through the same agents-management construction site, from
  the profile the request names (§5.3).
- `launch-exact/2`: the CIP-0016 exact-plan shape. Its wire is the jointly
  published successor (CIP-0016 §8 question 1). This protocol reserves only
  the token. The request member `plan` carries that payload unchanged.
- `attach/1`, `status/1`, `list/1`, `stop/1`.
- `board-goal/1`: the plugin can bind a workspace board goal (§3.3).
- `doorbell-claude/1`, `doorbell-codex/1`: §8.
- `credential-env/1`: the plugin accepts fd-4 secrets and injects them only
  into the provider child's environment (§9).
- `survives-upgrade`, `drains-on-upgrade`: the plugin's upgrade behaviour
  (§3.3).
- `labels/1`: sessions carry the request's `labels` and `list` filters by
  them.

`runtimes` maps each env-id the manager hosts to the operations it supports
for it. A launch for an env-id it lacks is
`session_host_provider_unsupported` (exit 6), checked from `describe` before
`launch`.

The launcher calls `describe` once per invocation, before composition, and
checks the capabilities it needs for this launch. A missing one is
`session_host_capability_missing` (exit 6), and the detail names the token.

### 5.3 The launch request (`launch-delegated/1`)

Schema `urn:relux:curator-run:delegated-launch` `1.0.0`, closed:

| Member | Content |
| --- | --- |
| `runtime` | Canonical env-id: `claude_code`, `codex_cli` or `muse`. |
| `profile` | `{name, lock_sha256, fragment_digest}` from the launcher's own resolve (§3.3). The plugin MUST resolve the same profile and refuse `policy_refused` (16) if its own fragment digest differs. This catches drift between the two resolves. |
| `cwd` | Absolute workspace. |
| `model`, `effort` | Resolved by SPEC §4.3. They are passed as values, never as provider flags. |
| `permissions` | `{mode: native\|yolo, source}` from SPEC §4.3. |
| `sandbox` | `{mode: "none"\|"workspace-write", writable_roots[]}`. This is policy, not a flag. The plugin spells it (Codex `sandbox_mode` / `--dangerously-bypass-approvals-and-sandbox`). |
| `harness_memory` | Boolean. The plugin spells it (Codex `features.memories`). |
| `session_name` | SPEC §3 `--name` grammar. The default is `<env-id>-<utc-stamp>`, or `werkstatt-<id>` for a Werkstatt. |
| `labels` | Map of at most 16 entries; keys `[a-z0-9.-]{1,32}`, values at most 128 bytes. A Werkstatt uses `werkstatt=<id>`, `generation=<n>`, `identity=<a2a id>`. |
| `child_env` | Map of non-secret literals for the provider child, at most 32 names. Names match `[A-Z][A-Z0-9_]{0,63}` and are not in the reserved set (§9.2). Values are at most 4096 bytes. The launcher runs its secret-pattern check (SPEC §4.9 secret policy). A Werkstatt passes `A2A_AGENT`, `A2A_SESSION_ID`, `GIT_DIR`, `GIT_WORK_TREE` and the git signing config names (§6.3). |
| `board_goal` | `none` or `workspace` (§3.3). |
| `doorbell` | Absent, or `{identity, runtime_bind: {heartbeat, max_wakes_per_hour}}` (§8). |
| `credentials` | List of names delivered on fd 4, for example `["CLAUDE_CODE_OAUTH_TOKEN"]`. It names values, never contains them. |
| `native_tail` | The verbatim argv after `--`. The plugin passes it to its composer as the native suffix, not interpreted. |
| `detach` | Boolean. When true, the launcher does not call `attach` after `launch`. |

The plugin MUST NOT add profile content, change the model or effort, add
instruction files, add MCP servers, or turn on Remote Control. It MAY add
only its own host plumbing: its PTY or app-server endpoint, its private
session state, and the goal adapter when `board_goal` is `workspace`.
Anything it cannot apply exactly (a sandbox mode its runtime lacks, say) is
a refusal, not an approximation.

### 5.4 The session record

Schema `urn:relux:curator-run:session` `1.0.0`:

```json
{
  "protocol": "curator-session-manager/1",
  "ok": true,
  "session": "SES-…",
  "manager": "task-board",
  "runtime": "codex_cli",
  "native_id": "019a…",
  "state": "starting|up|busy|idle|draining|down|unknown",
  "labels": {"werkstatt": "curator-run-dev1"},
  "started_at": "…Z",
  "last_activity_at": "…Z",
  "doorbell": {"state": "armed|unavailable|off", "generation": "…", "reason": "…"}
}
```

`session` is the manager's handle and is opaque to the launcher. `unknown`
is a real state: the launcher MUST NOT read it as `down`.

### 5.5 Conformance

The repository ships `internal/sessionmanager/conformance`. It is a suite
that runs a candidate plugin through every operation it advertises,
including the negative cases:

- an unknown operation;
- a malformed or over-bound response;
- a protocol mismatch;
- a missing capability;
- an unsupported runtime;
- a profile digest mismatch;
- a secret in `child_env`;
- a `stop` of an unknown session;
- `unknown` liveness.

The repository also ships `cmd/curator-run-fake-manager`, a reference plugin
for the suite and the launcher's own tests. A manager is **conforming** when
it passes the suite for the capabilities it advertises. Werkstätten (§6) use
only protocol operations. Any conforming manager that advertises
`launch-delegated/1` (or `launch-exact/2`), `attach/1`, `status/1`,
`stop/1`, `labels/1` and `list/1` hosts them.

### 5.6 Refusals

The refusal object is
`{protocol, ok: false, code, message, details}`. `code` comes from the
SPEC §6 registry plus the new codes of §11. A code outside the registry is
`session_host_protocol_error`. The launcher prints its own constant message
for the code, plus the plugin's `details` fields filtered to the registry
vocabulary. It never prints the plugin's `message` bytes. Exit status
follows the registry.

## 6. Werkstätten

### 6.1 The record [pending Q4]

A Werkstatt is project × role profile × environment, with its own managed
home, clone and identity. Its record is `werkstatt-record-v1`, closed:

| Field | Meaning |
| --- | --- |
| `id` | `[a-z0-9][a-z0-9-]{0,47}`, for example `curator-run-dev1`. |
| `project` | Project slug. |
| `role` | `developer`, `reviewer` or `orchestrator`. |
| `runtime` | Canonical env-id. |
| `model`, `effort` | Pinned at create or restart. The router's `config_decision_id` records why. The per-task effort override travels in the task (router decision: dispatch returns (station, effort)). |
| `profile` | Curator profile name. |
| `clone` | Absolute path of the worktree. |
| `git_dir` | Absolute path of the separate git dir, or absent for a plain clone. |
| `identity` | a2a identity, for example `ivmbp_curator-run_dev1`. |
| `room` | The room tasks arrive in, for example `#curator-run`. |
| `orchestrator` | The only a2a identity the Werkstatt accepts tasks from. |
| `context_mode` | `continue` (default) or `fresh`. |
| `harness_memory` | Boolean. Default `true` while `context_mode` is `continue`. |
| `sandbox` | `none` (current decision) or `workspace-write`. |
| `trust_class` | Router vocabulary (open with Ivan); stored, not interpreted by the launcher. |
| `doorbell` | `{heartbeat: "1m", max_wakes_per_hour: 6}`. |
| `config_decision_id` | Optional router decision id. |

Sources, in order:

1. the board's `station` element, read with
   `task-board station get <id> --json`, once that element exists (the
   router and task-board lanes freeze its fields; this list is the proposal);
2. otherwise `~/.config/swarma/launcher/werkstatt/<id>.json`.

When both exist, the board decides, and a differing local file is a warning.
`werkstatt up` prints the source it used.

### 6.2 `werkstatt up <id>`

Ordered steps. Each one completes or refuses, and nothing is half-started:

1. **Record.** Load and validate it. Missing: `werkstatt_not_found` (1).
   Invalid: `werkstatt_record_invalid` (2).
2. **Manager.** Resolve the selected manager (§4.2) and `describe` it. None
   registered: `werkstatt_session_manager_required` (exit 1). This is
   always hosted, and never native, whatever the machine mode is.
3. **Already up?** `list` with `labels: {werkstatt: id}`. A session in a
   state other than `down` is `werkstatt_already_up` (1), and the detail
   names the session. `unknown` refuses too
   (`session_liveness_unknown`, 1).
4. **Workspace checks.**
   - The clone exists.
   - If `git_dir` is set, it is outside the worktree and the worktree has no
     `.git` entry.
   - No `CLAUDE.md`, `CLAUDE.local.md`, `AGENTS.md` or
     `AGENTS.override.md` lies in any strict ancestor of the clone up to
     `/`. Otherwise `werkstatt_context_leak` (2), naming the path, because
     Claude and Codex read these upward and `CLAUDE_CONFIG_DIR` /
     `CODEX_HOME` do not stop that. User-level files inside the config
     directories (`~/.claude/CLAUDE.md`) are not ancestors and are replaced
     by the managed home anyway.
   - The a2a credential for `identity` exists (checked by presence, never
     read).
5. **Context plane.** Resolve the fragment for `profile` (SPEC §4.1, with
   `--repair`).
6. **Spawn-plane defaults.** `model` and `effort` come from the record. The
   lineup admits them, or the launch refuses as SPEC §4.3.
7. **Credentials.** For `claude_code`, resolve the setup token (§9).
8. **Doorbell preparation.** For `claude_code`, ensure the a2a hooks for
   `identity` in the clone's `.claude/settings.local.json` (§8.1).
9. **Launch** with `detach: true`. The request carries:
   - the record's `runtime`, `profile`, `model`, `effort`, `sandbox` and
     `harness_memory`;
   - `cwd = clone`;
   - `board_goal: none`;
   - the labels `werkstatt`, `generation` and `identity`;
   - `child_env` (§6.3);
   - `doorbell`;
   - the credential names.

   A first-run bootstrap prompt is not part of `up`. The orchestrator's
   first task message is the bootstrap (§6.4).
10. **Liveness.** Write the record of §6.5 with state `starting`, then the
    state from the session record. Print the session handle and the
    doorbell state.
11. `--attach` attaches the terminal afterwards. Detaching leaves the
    Werkstatt up.

### 6.3 Child environment for a Werkstatt

- `A2A_AGENT=<identity>`, and `A2A_SESSION_ID`, which the manager fills with
  the native id once known (§8.2).
- `GIT_DIR` and `GIT_WORK_TREE` when `git_dir` is set.
- The git identity and signing configuration, as non-secret names:
  - `GIT_AUTHOR_*` and `GIT_COMMITTER_*`;
  - `GIT_CONFIG_COUNT` with the `user.signingkey`, `gpg.format` and
    `gpg.ssh.program` triples;
  - `GIT_SSH_COMMAND`;
  - `SSH_AUTH_SOCK`, the swarm agent socket.

  Values come from the host's agent-git environment, which the launcher
  sources in a subprocess and filters to exactly these names. The signing
  key itself never moves: the socket and the public key path are the only
  values.
- Per-Werkstatt build caches (`GOCACHE`, `GOPATH`) under the Werkstatt
  directory. They come from an optional `child_env` member of the record,
  under the same name rules.

### 6.4 Context modes

- `continue`: one long-lived session. Tasks arrive by doorbell into the same
  thread, and the harness compacts natively. The session is restarted only
  by `recycle`, a crash, or a machine restart.
- `fresh`: the orchestrator runs `werkstatt recycle <id>` before each task.
  That is `down` and `up` with the same identity, home and clone, and the
  generation incremented.
- Rework after review always continues the task's thread, in both modes.
  This is the orchestrator's choice, not the launcher's.

### 6.5 Liveness

State file: `~/.local/state/swarma/launcher/werkstatt/<id>.json`, schema
`werkstatt-liveness-v1`:

```json
{ "id": "...", "state": "starting|idle|busy|draining|down|unknown",
  "manager": "task-board", "session": "SES-…", "generation": 3,
  "started_at": "…Z", "observed_at": "…Z", "last_activity_at": "…Z",
  "doorbell": "armed|unavailable|off", "current_task": null }
```

- curator-run writes it on `up`, `down`, `recycle` and every `status`. Each
  time, it is computed from the manager's `status`, and nothing else is
  inferred. `current_task` is written by the orchestrator through
  `werkstatt status <id> --set-task <ID>|--clear-task`. The launcher never
  derives it.
- The router's snapshot reads this record. Once the board's `station`
  element exists, the launcher also mirrors `state`, `observed_at` and
  `session` to it.
- a2a liveness (`a2a status set` / `who`) stays the Werkstatt's own
  heartbeat. `status` shows both and does not merge them.

### 6.6 `down`, `recycle`, `status`, `list`

- `down <id>`:
  - It first sets the doorbell gate to `stopping` (§8.2).
  - It calls `stop` (`graceful`) and records `down`.
  - In state `busy` it refuses `werkstatt_busy` (1) unless `--force`, which
    stops `force`.
  - Already `down` is exit 0 with one line.
- `recycle <id>`: `down` (graceful; refused while busy), then `up`. The
  generation goes up by one.
- `status [<id>]` and `list`: the record, the manager status, the a2a `who`
  line and the doorbell state. `--json` gives
  `werkstatt-status-v1`, an array of `{record, liveness, a2a}`.

## 7. Muse

- The SPEC §4.2 `muse` row stays. Muse launches require
  `launch-env-fragment-v4`. A v3 fragment for `muse` refuses
  `fragment_version_unsupported` (exit 2) with upgrade guidance. The
  launcher already parses v1–v3. v4 adds `root_context` and
  `credential_store`, read as environments §10.2 defines them.
- Native: the plan comes from agents-management v0.5.66 or later (v4
  consumer, CIP-0017). It uses the four XDG parents and
  `TBH_CREDENTIAL_BACKEND` from the fragment, and the explicit
  `root_context` is passed to the module, never inferred.
- Hosted: the manager must list `muse` in `runtimes` with `launch` and
  `attach`. task-board does so once its v4 consumer lands (TASK-261018-yzorfb).
  Before that, a hosted Muse launch refuses
  `session_host_provider_unsupported` (6). It does not fall back to native.
- `werkstatt up` for a Muse record follows §6.2 unchanged.

## 8. a2a doorbells for hosted sessions

### 8.1 Claude

Claude rings through hooks.

- For `--a2a <identity>` and for Werkstätten, the launcher ensures the hooks
  before launch by running
  `a2a hook install --agent <identity> --runtime claude --settings
  <cwd>/.claude/settings.local.json`.
  - That command is idempotent.
  - The launcher checks the result by reading the file back. It looks only
    for the three hook entries and `env.A2A_AGENT`.
  - A failure is `doorbell_setup_failed` (1).
- `doorbell-claude/1` means the manager's Claude composition keeps the
  workspace's project-local settings (`.claude/settings.local.json`)
  active, and passes `A2A_AGENT` through `child_env`. Hooks ring on
  `SessionStart`, `UserPromptSubmit` and `Stop`.
- Idle wake for Claude is the session's own: the hook template tells it to
  keep one background `a2a wait --timeout 0`, whose completion wakes it
  (a2a `docs/cli.md`). The manager types nothing into the PTY. No terminal
  typing is used for doorbells, for any runtime.
- A manager without `doorbell-claude/1` (one that strips project settings,
  say) makes a doorbell-requesting Claude launch refuse
  `session_host_capability_missing`.

### 8.2 Codex

Codex rings through `a2a runtime bind`. Only the owner of the app-server
endpoint can run it, so it belongs to the manager (`doorbell-codex/1`).

- The manager creates the thread on its private app-server.
- Then it runs:

  ```text
  a2a runtime bind --codex-thread <thread> --endpoint unix://<its socket>
      --owner-pid <its pid> --heartbeat <h> --max-wakes-per-hour <n>
      --gate ready --hooks installed --watch
  ```

  The run uses `A2A_AGENT=<identity>` and `A2A_SESSION_ID=<thread>`. The
  `<thread>` value also goes into the thread's own command environment
  (`shell_environment_policy.set`).
- The manager supervises the `--watch` consumer and owns its gates. On
  `stop` it sets the gate to `stopping` and abandons the generation.
- The manager reports the generation in the session record.
- The a2a native adapter verifies the app-server versions 0.160.0–0.161.0.
  On the installed Codex 0.162.1 the bind reports unavailable, and the
  session record says so (`doorbell.state: "unavailable"`). The launcher
  then prints one line. The launch is not refused, because the Werkstatt
  still works and the orchestrator can ring by relaunch. Qualifying 0.162.x
  is an a2a item (§12).

## 9. Claude setup-token injection [pending Q5]

### 9.1 Source

The setup token reaches the child as `CLAUDE_CODE_OAUTH_TOKEN`. It comes from
a credential source named in the machine file `credentials.json`, schema
`curator-run-credentials-v1`, which is system layer only:

```json
{ "schema": "curator-run-credentials-v1",
  "claude_code": { "source": "macos-keychain",
                   "service": "swarma.claude-setup-token", "account": "ivmbp" } }
```

| `source` | Behaviour |
| --- | --- |
| `curator-node` | Through Curator's CIP-0010 node and protected binding, once it ships. This is the target. |
| `macos-keychain` | One generic-password item, read with the Security framework without user interaction (`kSecUseAuthenticationUIFail`). If the item would prompt or is absent, the read refuses `credential_source_unavailable` (1). It never falls back. |
| `none` | No injection. The home's own login applies (per-home, today's behaviour). This is the default when the file is absent. |

- Enrolment is out of band. `curator-run credential enroll claude_code`
  reads the token from a TTY with echo off and stores it in the configured
  source. It is never given as an argument.
- One token serves every home (Werkstatt decision). No home gets a copy.

### 9.2 Injection rules

- **Reserved names:**
  - `CLAUDE_CODE_OAUTH_TOKEN`;
  - `ANTHROPIC_API_KEY`;
  - `ANTHROPIC_AUTH_TOKEN`;
  - every name the fragment's credential section reserves.

  These names may not come from `child_env`, from fragment `env`, from the
  plan's own literals, or from `--` native env. Any such source is
  `credential_reserved_name` (2).
- If the caller's ambient environment already has `CLAUDE_CODE_OAUTH_TOKEN`
  and a source other than `none` is configured, the launch refuses
  `credential_source_conflict` (2). It neither silently prefers one nor
  merges them.
- **Native:** the token goes only into the exec'd child's environment.
- **Hosted:** the token goes only on fd 4 to a manager that advertises
  `credential-env/1`. The manager puts it only into the provider child's
  environment and MUST NOT persist it: not in session records, plans, logs
  or restart templates. A manager without `credential-env/1` makes a
  token-configured hosted Claude launch refuse
  `session_host_capability_missing`.
- A manager-side restart or cold resume that needs the token goes back
  through `curator run` (`resume`), which resolves the token again. The
  manager never stores it to restart by itself (CIP-0016 §6).
- **Redaction:** the token value never appears in diagnostics, `--json`
  output or the liveness record. Only the source kind and the item's
  service and account names may appear.
- No Remote Control. The launcher adds no RC flag. A hosted request whose
  `native_tail` contains `--remote-control` keeps the SPEC §4.9 refusal
  path.

### 9.3 Qualification items

These items need a live probe before acceptance:

- Claude Code with `CLAUDE_CODE_OAUTH_TOKEN` in a fresh managed home must
  start interactively without the first-run login or onboarding screen.
- An existing per-home Keychain login must not take precedence over the
  token.

## 10. Signed distribution

- `curator-run` ships in the signed Curator release, built from a pinned
  launcher tag.
  - The Curator release workflow builds both binaries from their signed tags
    with the release ldflags, signs and notarizes both, and puts both into
    the same archive and installer.
  - The launcher's signed tag (`vX.Y.Z`) is a release input of Curator. It
    is recorded in the Curator release notes and in `curator --version
    --json` as `providers.curator-run`.
- The installer places `curator-run` in Curator's install directory. That
  directory is the first trust root (environments §11, E4), so
  `curator run` resolves it there. A `curator-run` elsewhere on `PATH` gets
  the existing revision-A warning, or the revision-B refusal.
- `curator-*` names cannot come from profiles. Materialization already
  refuses them (environments §9.4, `environment_reserved_command_name`).
  Nothing here relaxes that.
- `curator-run --version` reports its own tag and commit, and `providers`
  of `curator --version --json` must match. A mismatch is a warning in
  `curator env status`.
- The launcher repository keeps its own release for development builds.
  Hosts take only the distribution's.
- Owner of the release-workflow change: `ivmbp_curator_orch` (curator
  repository). This repository supplies a reproducible build target
  (`make release-binary VERSION=…`) and the tag.

## 11. Diagnostics added

| Code | Exit | When |
| --- | ---: | --- |
| `host_member_withdrawn` | 2 | `defaults.json` still has `host` (§3.1) |
| `session_manager_registry_invalid` | 2 | registry unreadable or failed checks (§4.2) |
| `session_manager_not_registered` | 2 | `select` or `unregister` of an unknown id |
| `session_board_goal_unavailable` | 1 | `--board-goal` without a workspace board, or the manager lacks `board-goal/1` (§3.3) |
| `werkstatt_not_found` | 1 | no record (§6.2) |
| `werkstatt_record_invalid` | 2 | record fails the schema |
| `werkstatt_session_manager_required` | 1 | `up` with no manager |
| `werkstatt_already_up` | 1 | a live session carries the label |
| `werkstatt_context_leak` | 2 | instruction files above the clone |
| `werkstatt_busy` | 1 | `down`/`recycle` while busy without `--force` |
| `doorbell_setup_failed` | 1 | hook install or verification failed (§8.1) |
| `credential_source_unavailable` | 1 | configured source cannot be read without interaction (§9.1) |
| `credential_source_conflict` | 2 | ambient token plus configured source |
| `credential_reserved_name` | 2 | reserved credential name from a non-credential channel |

Existing codes reused with their meaning:

- `session_host_missing`, `session_host_unavailable`;
- `session_host_protocol_error`, `session_host_protocol_unsupported`;
- `session_host_capability_missing`, `session_host_provider_unsupported`;
- `session_liveness_unknown`, `policy_refused`, `secret_policy_violation`;
- `launch_plan_invalid`, `host_configuration_conflict`.

`session_host_default_not_ready` is retired [pending Q2].

## 12. What other lanes must deliver

| Owner | Item |
| --- | --- |
| task-board (infra-exit, `#skill-project-management`) | 1. `task-board session-manager register --for curator-run` (§4.3), called by its installer. 2. A plugin entry `task-board session-manager plugin <op>` implementing §5 with `launch-delegated/1`, `attach/1`, `status/1`, `list/1`, `stop/1`, `labels/1`, `board-goal/1`, `credential-env/1`, `doorbell-codex/1` (and `doorbell-claude/1` when ready). 3. **A board-less session daemon mode**: the session daemon keyed by user state root, not by board directory; sessions keyed by cwd and labels; no primary goal unless `board_goal: workspace`. 4. `--profile` for hosted sessions (TASK-261018-cliw6c, in flight). 5. The v4 Muse consumer (TASK-261018-yzorfb). 6. `task-board claude\|codex` become aliases of `curator run <env> --hosted --board-goal` (Decision 0021 §5); not a blocker. |
| Router lane (`#model-routing`) | Freeze the `station` fields with task-board, against §6.1. Read `werkstatt-liveness-v1` (§6.5) as the snapshot liveness source. |
| `ivmbp_curator_orch` (Curator) | 1. Release-workflow change for §10. 2. CIP-0010 node (`curator-node` source). 3. CIP for Curator under the swarma config home (already Curator's item). 4. A CIP-0016 amendment recording `launch-delegated/1` as an approved transitional shape, if Q1 goes that way. |
| a2a (Courier) | Verify the native Codex adapter on app-server 0.162.x (§8.2). |

## 13. Compatibility and migration

- Machines without `launcher.json`, without a registry and without `ax.json`
  behave as `auto` with no manager: native, and one notice line. Compared
  with today, the only visible difference is that line.
- `curator-run migrate`:
  - moves `~/.config/curator-run/*` and `/etc/curator-run/*` (with
    privilege) to the swarma paths;
  - strips the `host` member from `defaults.json` into `launcher.json`. A
    machine `"host": "hosted"` for every env becomes `host_mode: hosted`;
    mixed per-env values refuse and ask the operator to choose, because v1
    has no per-env mode;
  - keeps backups.
- The dormant SPEC §4.9 payload and the `task-board session launch-plan`
  receiver are not removed. They become the `launch-exact/2` path once the
  CIP-0016 successor wire is published.
- `--hosted` keeps its spelling. Its meaning widens from "the receiver of
  §4.9" to "the selected session manager".
- Pinned agents-management moves to the release carrying v4 Muse and the
  managed Codex plan fields (≥ v0.5.68), as a separate leaf.

## 14. Delivery slices

Each slice is one board task, implemented by the developer Werkstatt and
reviewed by the reviewer Werkstatt. Every slice carries negative tests for
its refusals, next to positive controls.

| # | Slice | Depends on |
| --- | --- | --- |
| S1 | Config home and migration (§4.1, §13), `launcher.json`, `defaults.json` `host` withdrawal | spec |
| S2 | Registry and `session-manager` commands, install-time registration rules (§4.2–4.3) | S1 |
| S3 | Plugin client, `describe`, refusals, conformance suite, fake manager (§5) | S2 |
| S4 | Host-mode resolution and the `auto` notice (§3.2), hosted launch through `launch-delegated/1` and `attach` | S3 |
| S5 | Setup-token sources and injection (§9) | S3 |
| S6 | Werkstatt record, `up`/`down`/`recycle`/`status`/`list`, liveness (§6) | S4, S5 |
| S7 | Doorbell wiring (§8) | S6, task-board item 2 |
| S8 | Muse v4 (§7) | S4, task-board item 5 |
| S9 | Release target for the signed distribution (§10) | S4 |
| S10 | Real manager e2e on ivmbp: task-board plugin, one Claude and one Codex Werkstatt, live round trip | S6, S7, task-board items 1–3 |

S1–S6 run against the fake manager and do not wait for task-board. S10 is
the first claim that anything works for real.

## 15. Open questions sent to `ivmbp_curator_orch` (2026-10-18)

1. **Q1, transitional launch shape.** Accept `launch-delegated/1` (the
   manager composes through the same construction site, against the live
   profile home, with stated limits) until `launch-exact/2` activates?
   Without it, hosted-by-default and `werkstatt up` wait for CIP-0014
   snapshots and the CIP-0016 successor wire.
2. **Q2, upgrade without hang-up.** The machine mode replaces the old
   defaults gate, and the upgrade behaviour becomes a disclosure line
   instead of a refusal?
3. **Q3, config element name:** `swarma/launcher/` or `swarma/curator-run/`?
4. **Q4, Werkstatt record:** a local file until the board's `station` lands,
   or the board only from day one?
5. **Q5, setup-token interim source:** the macOS Keychain without UI, until
   the CIP-0010 node ships?

Also open, not blocking the draft:

- `host_mode_locked`, which would forbid `--native` on a machine;
- a per-env host mode;
- whether `werkstatt up` should post the Werkstatt's arrival to its room
  (today: no, the orchestrator does).
