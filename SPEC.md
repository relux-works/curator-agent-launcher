# Curator Agent Launcher — Specification

**Specification version:** `0.5.1-draft`
**Status:** in-repository draft (see [Versioning](#8-versioning))

- 2026-10-13, 0.5.1-draft documentation revision: apply accepted
  architecture-review findings (2026-10-13) in §§9, 10.2 and 10.6, README
  and figure sources: positive class/placement/review-role predicates,
  inert exact-candidate review and strict-context entry qualification.
  Register the workplace lifetime requirement for stage A; optional
  reviewer policies remain open. No implementation, qualification or
  activation is claimed.

- 2026-10-13, 0.5.1-draft documentation revision: apply accepted
  architecture-review findings (2026-10-13) in §§10.1–10.5, README and
  figures: mode-aware base binding, protected exec-profile selection and
  full-invocation qualification, owner-linked format references and the
  Muse priority/channel/qualification split. Align release references and
  hosted-unavailable figures with v0.3.0. No implementation, qualification
  or activation is claimed.

- 2026-10-13, 0.5.1-draft: launcher v0.3.0 pins
  agents-management v0.5.62 for attested Muse builds, `muse-wonder` and
  fragment v3; hosted launch is unavailable until the swarma dispatcher
  path ships (§§3, 4.2, 4.8–4.9, 7–8).

- 2026-10-13, 0.5.0-draft documentation revision (§§10.1–10.6,
  README and figures): apply R-TRANSFER, R-STANDING, R-EXEC and
  R-NET-TOOL; reference the untrusted admission and abuse-control owners.
  Transfer ownership is settled; exact formats and qualification remain
  publication gates. No implementation is claimed.

- 2026-10-12, 0.5.0-draft documentation revision (§§10.2, 10.4–10.5,
  README and figures): apply private cross-account Git administration
  (F-16), the single termination scope (F-13), the user manager's fate
  table (R-FATE), session-scoped host admission and `host_stop` (R-SESSION),
  and the accepted time rules (R-TIME). Unpublished formats remain open;
  no implementation is claimed.

- 2026-10-12, 0.5.0-draft documentation revision (§§10.1–10.5, README
  and figures): apply the accepted descriptor-lifecycle reference (R-FD3),
  dispatcher-only privileged starts (R-START), initial Unix-socket service
  transport and retirement home deletion. FD3 staging and host admission
  publication remain activation gates; no implementation is claimed.

- 2026-10-12, 0.5.0-draft documentation revision: the
  [figure index](diagrams/README.md) maps every PlantUML source to its defining
  spec sections and current or target phase; figure titles use public names.

The launcher is the **execution plane**: Curator owns context,
`agents-management` owns spawn admission, and the selected session manager
owns hosted sessions. Native execution starts and waits for the composed
child directly. The ax integration was **removed 2026-10-27 (owner decision)**;
the earlier four-plane ax handoff is historical, not an active contract.
The target one-shot mode in §10 uses a shared launch library with
`swarma-session-runner`, as required by D-EXECUTOR.

Key words MUST, MUST NOT, SHOULD, and MAY are to be interpreted as in
RFC 2119. Normative references:

- `curator-spec/decisions/0010-agent-environment-profiles.md`, Decisions 6
  and 10 — the boundary this component lives inside.
- `curator-spec/decisions/0013-execution-ownership-and-launch-plans.md`
  — historical plan-value and composition decisions. Its ax handoff and
  Session Record extension requirements were **removed 2026-10-27 (owner decision)**.
- `curator-spec/decisions/0012-context-packages-and-locks.md`, Decision 6
  (MCP declaration packages, the launch channel, the `env_names` union)
  and Decision 8 (the fragment under locks: `profile.lock_sha256`, the two
  precedence primitives, `composition` withdrawn, the `mcp` section).
- `curator-spec/protocol/environments.md` revision 1.1 (curator-spec
  `fcdb9ba`) — §10 (`env resolve`, `--repair`, and
  `launch-env-fragment-v1`), §9.2 step 5 (stale homes after `profile
  update`), §7.1 (adapter home variables), §7.3 (system-prompt
  channels), §7.8 and §5.8 (MCP launch channels and the codex layer
  file), §5.5 (system-prompt output), §11 (umbrella subcommand
  discovery), §12.1 (the manager's machine-configuration knob table).
  Revision 1.1 carries Decision 0012 D8, so the fragment members this
  document consumes (`lock_sha256`, `mcp`) are now spelled there.
  `profiles/manager.md` §12.5 restates `env resolve` for the manager.
  Pi prompt discovery follows the accepted A0 E5 evidence (§4.3/§6,
  TASK-260908-qblycn) and the [revision 1.1 corrections at `d019f0e`](https://github.com/relux-works/curator-spec/blob/d019f0e/protocol/environments.md):
  installed Pi 0.84.2 `dist/core/resource-loader.js`, source selection
  at lines 380–388 and discovery at lines 808–829.
- `curator-spec/protocol/registry.md` §1 — CCJ-1 canonicalization, used
  for the fragment digest.
- The `agents-management` module documentation — the `BuildPlan` /
  `BuildLaunch` value contract, `LaunchModeInteractive`, the
  `vendorplugin.Lineup` ranking, and the provider-limits verdict model.

The plan/environment corrections in §§4.4–4.6 and §9 follow the accepted
A0 findings `TASK-260908-qblycn_a0-verification-findings.md` §§3.2, 3.5,
E3/E4 and the accepted `TASK-260908-1c0fwn` Plan.Env value-contract
resolution (§§2–5, supplied as `accepted-environment-evidence.md`). The
recorded module entry point is `pkg/vendorplugin/spawn.go` at `v0.5.10`
(commit `12f443d10bc217ca7a48e2edab19c739f441df9c`); this is evidence,
not a dependency-version change. The corresponding upstream corrections
are [Decision 0013 D6.3/D6.4 and open question 6](https://github.com/relux-works/curator-spec/blob/d019f0e7179520b5c8dcde321c4fe51e04552f58/decisions/0013-execution-ownership-and-launch-plans.md)
and [environments revision 1.1](https://github.com/relux-works/curator-spec/blob/d019f0e7179520b5c8dcde321c4fe51e04552f58/protocol/environments.md),
landed in curator-spec PR48. Only the plan/environment corrections are
applied here; Pi file-channel corrections remain separately tracked.

## 1. Scope and non-goals

The launcher answers exactly one operator question: *just run it*. The
operator types `curator run codex_cli --profile companyA -- resume --last`
and gets exactly the codex they know, in the companyA managed home, on an
admitted model, executed natively or by the selected hosted manager.
Everything after `--` belongs to the tool, untouched.

The separately versioned [same-user non-chat contract/1](docs/same-user-nonchat-contract.md)
proposes a narrow trusted development/review/assessment exception to this
interactive-only scope. It requires a new native context per invocation and
explicit effective-config/intake/result admission. It is not implemented or
qualified, does not extend today's CLI and is distinct from the §10 target.

Non-goals, each a boundary rather than an omission:

- **No native session state.** Hosted sessions belong to the selected manager.
  The interactive launcher holds no leases, no checkpoints, no resume
  records, and no memory of past launches. The target one-shot executor
  obtains an execution-scoped credential lease (§10.3); it owns no
  persistent session state. Session affinity (a session created under
  profile P resumes under profile P's home) is preserved by the planes
  that own it, not tracked here.
- **No plan rebuilding.** The spawn plane's launch plan — binary, argv,
  environment, stdin bytes — is consumed as a value. The launcher never
  reconstructs argv from its own knowledge of a tool and never spells a
  provider flag of its own: model selection and effort transport are the
  plan's, spelled by the system plugin under `LaunchModeInteractive`
  (Decision 0013 D5). Appending the fragment's channel flags (system
  prompt under the §5 opt-in, MCP always) and the native arguments to a
  plan value is **composition**, not rebuilding (§4.5; Decision 0013
  D6.5). The launcher never second-guesses an admission verdict.
- **No Curator imports.** `curator env resolve` is a subprocess. The
  composer consumes the tagged `skill-agents-management` and network modules
  pinned in §7. The target one-shot executor and `swarma-session-runner`
  share a launch library (§10), whose boundary and release remain unpublished.
- **No profile management.** Installation, materialization, switching,
  drift repair, and garbage collection are Curator's. The launcher only
  asks for a fragment, always under `--repair` (§4.1), and it is
  Curator's resolution that repairs a stale home — the launcher never
  writes into a managed home.
- **Not the only door.** An operator can always start a tool by hand with
  the tool's own flags. The launcher's job is to make the managed path
  explicit, warned, and reproducible — never to be mandatory.
- **No provider installation.** A missing binary fails with its name and
  installation guidance; nothing is downloaded implicitly.

### 1.1 Front doors and execution authority

The [component map](diagrams/components.puml) separates current interactive
processes from the target one-shot adapter; the [launch paths](diagrams/launch-paths.puml)
show their handoffs and refusal boundaries. The [invocation states](diagrams/launch-states.puml)
and [artifact layout](diagrams/artifact-layout.puml) show transient state and
persistent owners without assigning session state to the launcher.

| Front door | Composition and authority | Execution owner | Contract/status |
|---|---|---|---|
| Direct human launch | `curator-run <env-id> …` resolves the profile and composes an admitted interactive plan on the caller's side. | The launcher starts and waits for the native child. | §§3–6; existing interactive API. |
| Hosted launch | The same caller-side interactive composer exports the destination contract. Host selection remains independent of permissions. | The hosted receiver creates/manages the process; a refusal never falls back to a direct child. | §§4.6, 4.8–4.9; existing APIs and their stated evidence limits. |
| Trusted same-user non-chat | Explicit current-user assignment and frozen exact tuple/inputs; no child chat/board presence. | Target launcher adapter and retained supervisor; controller owns result custody. | [Documentary contract/1](docs/same-user-nonchat-contract.md), **not implemented or qualified**; no shipped entry. |
| Agent one-shot exec | A caller composes the immutable plan; `swarma-dispatcher` admits, provisions and tracks; `swarma-user-launcher` verifies the approved executor and drops privileges. | `curator-run` under `worker-<label>` verifies the plan, leases a credential when selected and executes the harness. | §10, target amendment (version pending); **not implemented**. Local headless only in dispatcher v0. |

The §10 agent-account one-shot door does not run §§4.1–4.5 under the agent
account. It does not resolve or repair profiles, compose plans, pick models, or infer
permission defaults. The caller composes; the executor verifies and
executes. Direct interactive execution is already a launcher responsibility;
there is no universal compose-only claim. Long sessions use the target
`swarma-session-runner`, not a one-shot process or a service-account harness.

## 2. Discovery and naming

The launcher ships one executable, `curator-run`. Curator dispatches
umbrella subcommands per environments.md §11: an unknown subcommand
resolves to `curator-<name>` on `PATH` and receives the remaining
arguments verbatim. `curator run …` and a direct `curator-run …`
invocation are therefore the same surface, and this specification is
written against the executable's own argv. Curator carries no knowledge
of the launcher beyond the discovery rule, and nothing in profile,
marker, or fragment data influences dispatch.

Under environments.md §11 with E4 trust roots (curator-spec `0da4020`,
PR #62), the manager resolves `curator-<name>` providers from trust
roots — its install directory, then the machine knob
`provider_directories` — warns under revision A when the
`PATH`-selected provider lies outside the roots, refuses under
revision B, and reports the resolved absolute provider path in
`env status`. The launcher is dispatched through that same
trust-root resolution: whether invoked as `curator run …` (umbrella)
or directly as `curator-run …`, the binary that runs is the
resolved provider, and at every launch the launcher reports its own
resolved executable path in the §4.3 line-group — absolute,
symlinks resolved — so an operator sees which `curator-run` binary
is about to run and where it lives. The umbrella passes no
distinguishing argv or environment marker today: the two invocation
shapes are indistinguishable to the launcher beyond the resolved
path itself, so the §4.3 provider line carries the path only, with
no origin suffix in this revision.

## 3. CLI surface

```text
curator-run <env-id> [--hosted | --native | --untracked]
            [resume [SES-HANDLE]] [--resume <id>]
            [--profile <name>] [--system-prompt <append|replace>]
            [--model <model>] [--effort <effort>]
            [--permissions <native|yolo> | --yolo]
            [--network <profile>] [--network-policy <policy>]
            [--network-known-bad <path>]
            [--] <native args...>
curator-run --help | -h
curator-run --version
```

This is the existing interactive surface. The separate proposed one-shot
entry is described in §10.1 and is not a shipped command. The distinct
[same-user non-chat target](docs/same-user-nonchat-contract.md) also publishes
no current CLI grammar; non-interactive arguments or missing terminals do not
admit it.

The flag set is deliberately minimal; every flag below names the single
plane it feeds, and a need that fits none of them is a specification
change, not a flag addition.

| Element | Plane | Meaning |
|---|---|---|
| `<env-id>` | context + spawn + session | Required operand. A registered Curator environment identifier (`claude_code`, `codex_cli`, `opencode`, `pi`), with `claude` and `codex` accepted as aliases of `claude_code` and `codex_cli`. The operand normalizes to the canonical id before any validation or lookup; outputs, diagnostics, fragments, configuration records, and locks carry the canonical id only, and an alias is never persisted. Selects the fragment to resolve and, through the closed mapping of §4.2, the agentic system to plan. |
| `--profile <name>` | context | Forwarded verbatim to `curator env resolve` as its `--profile` operand. Absent, resolution uses the current profile for the applicable scope. |
| `--system-prompt <append\|replace>` | execution | Explicit opt-in that engages the fragment's system-prompt channel with the given semantics. The value is required: the opt-in states what it wants, and the launcher never chooses replacement by default. See §5. |
| `--model <model>` | spawn | Level 1 of the §4.3 default precedence: passed through to the spawn plane's plan request as declared. The launcher does not validate model names; admission is the spawn plane's verdict. |
| `--effort <effort>` | spawn | Level 1 of the §4.3 default precedence: passed through as declared. Effort is per-model and the spawn plane injects no default; when no §4.3 level yields one for a model that requires it, the plane's refusal names the model, the accepted vocabulary, and the recommendation, and the launcher completes that error with its own flag spelling, `--effort`. |
| `--permissions <native\|yolo>` | spawn | Permission mode resolved by §4.3. The launcher passes the selected mode to `LaunchRequest.PermissionMode` for `LaunchModeInteractive`; agents-management owns the mapping. |
| `--yolo` | spawn | Exact alias of `--permissions yolo`, shipped in the same increment and occupying the same precedence level. Supplying both forms is a repeated permission request and a `usage` error. |
| `-d`, `--danger` | — | Rejected before `--` as `usage`; neither spelling is an alias. After `--`, arguments remain opaque native arguments under the normal boundary rule. |
| `--hosted`, `--native`, `--untracked` | session | The one host request slot (§4.8). `--hosted` uses the registered and selected conforming session manager under FRONT-DOOR §§3–5 and §5.8; without one it refuses `session_host_missing` with a reason detail. `--native` and its deprecated alias `--untracked` run natively. The alias prints exactly one deprecation line and is retained for two releases. A repeat or a combination is a `usage` error; permissions stay independent of the spelling. |
| `resume [SES-HANDLE]`, `--resume <id>` | session | Hosted-only resume selectors (§4.8). `resume` selects the latest session, `resume SES-HANDLE` a handle, `--resume <id>` a provider identity. Handle and identity shape belong to the spawn plane's typed-intent grammar; in native mode any selector is a `usage` error. |
| `--network-policy <policy>` | network policy | `optimistic` (default), `strict`, or `pinned:<64 lowercase hex binary SHA-256>`; passed to adapterprobe policy (§4.4b). |
| `--network-known-bad <path>` | network policy | Explicit operator known-bad file; absence or read failure refuses (§4.4b). |
| `--network <profile>` | network | Explicit network-profile selection (§4.4b): a profile identifier resolved after plan admission against the operator catalog. Stored verbatim at parse; resolution and validation belong to §4.4b. Hosted selection refuses `network_scope_unsupported`: the source host cannot validate a destination. |
| `--` | — | Terminates launcher argument parsing. Everything after it is native argv, forwarded last and verbatim when admitted by the shared permission and context-channel grammar. |
| `--help`, `-h`, `--version` | — | Informational; print and exit 0. |

Parsing rules, closed:

- Launcher flags are recognized only before `--`. After `--`, nothing is
  interpreted — not `--help`, not a flag that happens to collide with a
  launcher flag.
- The first non-flag operand before `--` is `<env-id>`. After it, at most
  the `resume` selector and its optional handle follow; any other
  non-flag operand before `--` is a usage error: native arguments MUST
  follow `--`, so that the boundary between the launcher's surface and
  the tool's is visible in the command line itself. The operand
  normalizes through the §3 alias table (`claude` → `claude_code`,
  `codex` → `codex_cli`) before validation or lookup; any other
  spelling outside the registry keeps the existing refusal.
- An unrecognized flag before `--` is a usage error. It is never
  forwarded: silent forwarding would let a typo in a launcher flag reach
  the tool as tool input.
- Every value-taking flag takes exactly one value; a repeated flag is a
  usage error, not last-wins. `--permissions` accepts only `native` or
  `yolo`; `--yolo` is the no-value exact alias of `--permissions yolo`,
  and the two forms cannot be combined or repeated. `--hosted`,
  `--native`, and `--untracked` likewise share one host request slot and
  take no value: a repeat or a combination is a `usage` error.
  `-d` and `--danger`
  are unrecognized launcher flags before `--` and therefore produce
  `usage`; after `--`, the launcher does not interpret any native argument.
  `--system-prompt` accepts only its closed vocabulary. `--network` takes
  any non-empty value; profile validation belongs to §4.4b.
- `--name` and `--ax-profile`: **removed 2026-10-27 (owner decision)**.
  Before `--`, either spelling (including an equals value or missing value)
  is `usage`, exit 2, with the message “option was removed in 0.6.0”.
  Their former session-name and execution-profile semantics no longer apply.
- Usage errors exit 2 and print usage; they launch nothing and resolve
  nothing.

## 4. Composition algorithm

Figures: [current and target components](diagrams/components.puml),
[launch sequences](diagrams/launch-paths.puml), and
[transient invocation states](diagrams/launch-states.puml).

A launch composes in seven ordered steps: obtain the fragment, map the
environment, resolve model and effort, obtain the plan, resolve the
network selection (§4.4b), compose, hand off or exec. Every step either
completes or fails the launch with a §6 diagnostic; there is no partial
launch, and no step's failure degrades into a weaker launch shape. The
order is contract, not convenience: the fragment comes **first** because
the plan request needs the managed home (§4.1, §4.4), the plan comes
before the network step because only an admitted launch may probe, and
composition comes last because it appends to values it never rebuilds
(§4.5). Native execution consumes the caller-composed value; delegated hosted
execution uses the selected manager contract. Host selection (§4.8) runs after configuration validation
and routes the launch to native execution or to the hosted payload
handoff (§4.9); it changes no step's contract, only which handoff the
composed value reaches.

### 4.1 Obtain the fragment (context plane)

The launcher runs, as a subprocess:

```text
curator env resolve <env-id> [--profile <name>] --repair --format json
```

with the canonical `<env-id>` of §3: the launcher normalizes the `claude`
and `codex` aliases before invoking the subprocess, so the alias never
reaches Curator's lookup. The required `fragment` member names the fragment
revision. The launcher parses the closed v1, v2, or v3 object per environments.md
§10.2 as revised by Decisions 0012 and 0018 and F-S2
(curator-spec `ec8dc656`), rejecting unknown fields, unknown kinds,
unknown semantics values, and contradictory permission data.
`launch-env-fragment-v2` is the minimum permission-transport token: support
is established when the revision is v2 or later and retains this contract.
A v2 fragment MUST carry the required closed `permissions` object; a v1
fragment carrying that member is invalid. A v1 fragment without it remains
usable for legacy `native` launches but cannot establish permission or lock
transport, so a launch that would resolve `yolo` fails closed as specified
in §4.3 and §4.6.

Revision v3 (curator-spec candidate `d373078a`, environments §10.2)
retains v2 permissions and admits Muse with exactly `XDG_CONFIG_HOME`,
`XDG_DATA_HOME`, `XDG_STATE_HOME`, and `XDG_CACHE_HOME`. Their values share
one absolute managed parent, ending in `/config`, `/data`, `/state`, and
`/cache`, respectively. `HOME` is never a fragment variable or managed
override. Muse has no system-prompt or MCP member; v1/v2 do not admit Muse.
Other adapters retain their single variable and existing channel rules.

`--repair` is **always** passed (environments.md §9.2 step 5, §10.1;
`profiles/manager.md` §12.5): the launcher is the one caller that repairs.
Without it, `env resolve` is read-only — a lock-free verification of
exactly the marker-recorded surfaces — and **fail-closed**: a managed home
that is unprovisioned, stale after `profile update`, drifted, or whose
passthrough is detached is reported as `environment_home_stale` with its
reasons and yields **no fragment**. Under `--repair` the same read-only
verification runs first, a current home emits its fragment without any
lock being taken, and only a stale home takes Curator's mutation lock
with a bounded wait and is provisioned or repaired from the store as one
journaled transaction before the fragment is emitted. The launcher does
not offer a read-only launch: a launch into a home Curator knows is wrong
is the failure `env resolve` exists to prevent, and an operator who wants
to inspect staleness without repairing runs `curator env resolve` or
`curator env status` by hand.

The members this document consumes:

- `fragment` — the required fragment-revision token;
  `launch-env-fragment-v2` is the minimum version that carries the
  permission policy and force-native lock engagement;
- `profile.name` and `profile.lock_sha256` — the profile and its
  effective pin (Decision 0012 D3: the lock hash is the identity
  everywhere Decision 0010 used a commit);
- `permissions` — required in v2, a closed object with exactly
  `mode: native|yolo`, `locked: true|false`, and
  `source: profile|global|default`. Its consistency rules are:
  `locked` is true iff `source` is `global`; `mode` is `native` for
  `source` `global` or `default`; `source=profile` carries the
  per-profile setting, while `source=default` means that level is
  silent and the launcher continues to its own global default;
- `env` — the registry-declared variable names mapped to managed-home
  paths (environments.md §10.3); the value of the adapter's **home
  variable** (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `PI_CODING_AGENT_DIR`;
  `XDG_CONFIG_HOME` for `opencode`, whose home is `<parent>/opencode/`,
  environments.md §7.1) is the managed home this launch runs in and is
  passed as `LaunchRequest.Home` in §4.4;
- `system_prompt` — present exactly when the resolved chain carries at
  least one applicable system module; data about a channel, consumed only
  under §5, and its presence recorded as `system-modules` in §4.6;
- `mcp` — present when the adapter has an MCP channel and the profile's
  resolved MCP set is non-empty (Decision 0012 D6): the materialized
  file's path, the sorted `env_names` union of the servers in the
  adapter's set, and the adapter's channel descriptor (a §7.3 descriptor
  with an `argument` member and, for `flag`, an optional `with` list;
  `semantics` is absent and MUST be accepted as such). `pi` has no MCP
  channel and no `mcp` section;
- `precedence` (two primitives) is accepted and not consumed;
  `composition` is withdrawn under Decision 0012 D8 and environments.md 1.1
  §10.2, and is rejected as an unknown field.

Resolution is a pure function and activates nothing; under `--repair` it
also verifies and, when needed, repairs the managed home, so a fragment in
hand means a home that was materialized and current at resolve time. The
window between that verification and the child's first read of the home
is environments.md §10.1's recorded residual; the launcher re-verifies
nothing in that window except the two probes this document names — the
§4.5 codex layer stat and the §5.1 file-kind probe — and the §4.6 binary
check. Nothing in the fragment is applied here: `system_prompt` waits for
the §5 opt-in, `mcp` is applied in §4.5.

Failure modes are distinct and none of them degrades to a fragment-less
launch:

- the subprocess cannot be started (`curator` missing from `PATH`) —
  `resolve_invocation_failed`;
- the subprocess exits non-zero — the launcher maps Curator's own
  diagnostics (environments.md §10.4) through: `environment_unknown` →
  `resolve_environment_unknown`, `profile_unknown` →
  `resolve_profile_unknown`, `environment_repair_failed` →
  `resolve_repair_failed` (the store cannot restore this home),
  `environment_lock_unavailable` → `resolve_lock_unavailable` (the
  repair could not take Curator's mutation lock within its bounded wait —
  a retry later is the remedy, and the launcher does not retry on its
  own); any other non-zero exit is `resolve_invocation_failed`.
  Making `--repair` unconditional widens the resolve failure surface
  beyond the mapped four: a journaled mutation transaction can fail
  with diagnostics environments.md §10.4 does not list and this
  document therefore does not map (`environment_marker_invalid`,
  `environment_surface_unmanaged_conflict`, `environment_backup_exists`
  per §8.5, `environment_seed_unreadable` per §7.7). Every such exit
  collapses into `resolve_invocation_failed`, and Curator's own
  diagnostic code and message are printed verbatim with the launcher's
  code line, with the foreign stderr streamed before it. The launcher's line names only the family; the operator
  reads Curator's line for the remedy.
  `environment_home_stale` cannot arise from a `--repair` invocation; a
  Curator that nevertheless reports it is not the Curator this document
  is written against, and the exit falls under `resolve_invocation_failed`
  like any other unexpected non-zero exit;
- the subprocess exits zero but the output is not a valid closed fragment
  — `resolve_fragment_invalid`. A malformed read is a read failure, never
  an absence: the launcher MUST NOT treat unparseable output as "no
  fragment" and exec anyway.

Curator's diagnostic transport, as observed (A0 verification,
TASK-260908-qblycn, curator `v0.14.1-0.20260907213730-04550e282705`;
erratum candidate E6, STORY-260908-2utz8k): a failing `env resolve` prints
its §10.4 code as one stderr line of the form `curator: <code>: <detail>`
and exits 1. The launcher reads the code for the mapping above from the
first such line of the subprocess's stderr; a non-zero exit without a
recognizable line is `resolve_invocation_failed`. Curator's stderr is
forwarded to the operator verbatim, on failure and on success alike: a
successful resolve may carry `warning: <code>: <detail>` lines — observed
`environment_tool_version_unverified` when the detected tool release
differs from the recorded one — which are informational and never fail
the launch. Stdout carries the fragment and nothing else.

The launcher computes, from the **parsed** object and never from the
printed bytes, the fragment digest `sha256:<64 lowercase hex>` over the
CCJ-1 canonicalization (registry §1) of the fragment (Decision 0013 D6.4,
D7 item 6), so that a printer change in Curator is not drift.

### 4.2 Environment-to-system mapping

Curator environment ids map to spawn-plane agentic-system plugin ids:

| Curator env-id | agents-management system |
|---|---|
| `claude_code` | `claude-code` |
| `codex_cli` | `codex` |
| `pi` | `pi-native` |
| `muse` | `muse` |
| `opencode` | none in this revision — `env_unsupported` |

The ax provider-id column was **removed 2026-10-27 (owner decision)**.

The Pi system cell corrects accepted A0 errata E1/E2
(TASK-260908-qblycn, `TASK-260908-qblycn_a0-verification-findings.md`):
at v0.5.10, system `pi` invokes the legacy `agents-infra` wrapper, which
replaces the managed home, and has no declared Pi runtime. The accepted
native-Pi design (TASK-260908-ggxfte) is now landed in
[agents-management PR23](https://github.com/relux-works/skill-agents-management/pull/23),
commit `a2a6e9f377f62a5872d99ecdfff0d1690e385f2a`: system `pi-native`
provides native interactive Pi plans for managed homes. This mapping does
not require a Go import. The published v0.5.11 tag (tag object
`0ea486e46765ecf12fff7c2ac526e12da02e95ed`) introduced that support;
this revision pins v0.5.62. §4.3 registers the real native-Pi plugin.

An env-id outside this table that Curator nevertheless resolves is
`env_unsupported`. The table grows by specification revision, never inference;
a row requires a declared spawn-plane system to launch.

### 4.3 Resolve model and effort defaults

The launcher owns default resolution (Decision 0013 D6.2); the spawn plane
injects no default at any call site and receives the resolved pair as
explicit request members. Precedence, closed, consulted **per member**:
each level supplies only the member every earlier level left unset, so
`--model` with a configured effort is admitted, and a configured model
with no configured effort takes the lineup's effort for that model.

1. **Flags.** `--model` and `--effort` as typed.
2. **Machine configuration.** A closed launcher-owned mapping env-id →
   `{model, effort}`. The file is `defaults.json` in the launcher's
   configuration directory: `${XDG_CONFIG_HOME:-~/.config}/swarma/launcher/` (default
   `~/.config/swarma/launcher/`) for the operator, and `/etc/swarma/launcher/`
   for the machine. Schema, closed — readers MUST reject an unknown
   member:

   ```json
   {
     "schema": "curator-run-defaults-v3",
     "locked": false,
     "defaults": {
       "claude_code": { "model": "claude-opus-5", "effort": "high" },
       "codex_cli":   { "model": "gpt-5.3-codex", "permissions": "yolo" }
     }
   }
   ```

   `defaults` keys are env-ids of the §4.2 table (an unknown key is
   `defaults_config_invalid`); each value is an object of at most three
   members: `model`, `effort`, and `permissions`. The first two
   are strings passed through unvalidated to admission; `permissions`,
   when present, is exactly `native` or `yolo` and supplies the
   launcher-global mode level. The v3 `host` member is withdrawn: its presence
   refuses `host_member_withdrawn` (exit 2), naming `launcher.json`; see
   [FRONT-DOOR §3.1](FRONT-DOOR.md#31-the-machine-mode) and `curator-run migrate`. At least one member must be
   present. A v3 reader accepts v1 and v2 files unchanged; a v1 file
   carrying `permissions`, or a v1 or v2 file carrying `host`, refuses
   under its closed schema.
   **Lockable:** when the machine file carries `"locked": true`, the
   operator file is ignored for every env-id the machine file names, and
   a flag for a member the machine entry sets is a `usage` error naming
   the locked member, even if the requested value matches. Otherwise the
   operator file overrides the machine file per member: an operator entry
   that sets only `model` leaves a machine `effort` for the same env-id in
   force, and permission mode follows the same per-member merge. A missing file is a
   legitimate absence; a read or parse failure is `defaults_config_invalid`
   and the next level MUST NOT be used. The file and its sibling `launcher.json` (FRONT-DOOR §3.1) are the **launcher's configuration file family**, delimited
   against Curator's machine configuration in §4.7.

3. **Lineup fallback.** The highest-ranked model of `vendorplugin.Lineup`
   (capability score descending) among the models the module's runtime
   compatibility registry admits for the mapped system, with that model's
   declared `Effort.Recommended` as the effort, or no effort when its
   `EffortSupport` is `EffortSupportNone`. When the lineup admits no model
   for the system, the launch fails with `defaults_unresolvable` — the
   launcher never invents a model name.

   For native Pi, select the first runtime in the following ordered
   convention whose declaration carries at least one driven model, then
   apply `Lineup` to that runtime's own models only and bind its contributor.
   Vendor scales are never compared. If no preferred runtime has a driven
   row, report `defaults_unresolvable` with that reason and the preference.

   | System | Ordered runtime preference (pure convention) |
   |---|---|
   | `pi-native` | `pi-anthropic`, then `pi-openai`, then `pi-google` |

   Flags and files retain per-member precedence. A configured model binds
   the frozen runtime whose vendor carries its exact id, independently of
   this fallback preference.

**Permission-mode resolution (separate from model and effort).** The
resolved mode follows this precedence, with the force-native lock of
§4.6 above the entire ladder:

1. `--permissions native|yolo`, or its exact `--yolo` alias;
2. the Curator per-profile setting carried by `fragment.permissions`
   when `source=profile`;
3. the launcher-global `defaults.json` `permissions` member for the
   env-id, after the operator-over-machine merge;
4. the built-in default, `yolo` with
   `source=default-interactive`, for native and hosted silence on every stdio
   shape. Operator decision Q-D3 (2026-10-05), literal: headless
   detection (terminal, CI markers, native-argument form) never
   changes the permission default, for native and hosted launches
   alike;

Locked silence retains `source=default-headless` as existing lock provenance.
The tracked-silence default and tracked permission restrictions were
**removed 2026-10-27 (owner decision)**. Native and hosted silence use
item 4; observed headless signals do not change the default.

The fragment's `source=global` means Curator's force-native lock and
carries `mode=native`; it is not the launcher-global file level. In the
stderr provenance line below, `source=global` instead names the winning
launcher-global `defaults.json` level.
`source=default` means the profile level is silent; its `mode=native`
is a placeholder and the launcher falls through. Explicit `native` means
no launcher override and does not promise a prompting posture. In
native mode the launcher does not inspect policy selectors in
the native suffix; arguments after `--` are forwarded verbatim and may
request their own native policy by design. The typed interface is not a
security boundary. `yolo` requests the agents-management-declared native
permission mode within the tool's own policy. The launcher never derives
this mode from model,
prompt, credentials, environment values, or host selection.
After resolving a mode, every native launch prints this provenance line before admission:

```text
curator-run: permissions=<native|yolo> source=<flag|profile|global|default-interactive|default-headless> mapped=<flag or none>
```

The `mapped` value is supplied by agents-management; this SPEC names no
provider flag. Headless detection (terminal, CI markers) is observed but
never consulted by the built-in default since Q-D3; native-argument
non-interactive classification is gone entirely. Detection modifies no
native arguments and claims nothing about their safety. These interactive
classification/default rules do not admit the separate
[same-user non-chat target](docs/same-user-nonchat-contract.md), whose fresh
context and effective-config gates require explicit future admission. See §4.6
for the closed headless marker set and lock rule.

The §4.3 stderr line-group prints at **every** launch, before the plan
request, in this order — so an operator always sees which binary is
about to run, and which model, and why:

1. `curator-run: provider: path=<absolute path>` — the launcher's own
   executable path as the umbrella resolved it: `os.Executable`
   resolved through symlinks (`filepath.EvalSymlinks`), absolute. On
   any resolution failure (the executable lookup fails, symlink
   evaluation fails, or the result is empty or not absolute) the line
   carries the diagnostic-safe fallback `path=unavailable` instead,
   and the launch proceeds: path resolution never fails a launch.
   The line carries no origin suffix in this revision: the umbrella
   passes no distinguishing argv or environment marker (§2), so
   umbrella-dispatched and directly invoked launches are
   indistinguishable beyond the resolved path itself. A future
   revision MAY add a closed origin set (for example
   `(umbrella|direct)`) once the umbrella passes a marker that
   distinguishes them; until then the path-only form is the whole
   contract.
2. `curator-run: defaults: model=<model> (<level>) effort=<effort>
   (<level>)` — the resolved pair and the level that produced each
   member (`effort unset` when no level yielded one).

Both values are folded with the existing framing rule (CR and CRLF
fold to LF, every LF becomes LF plus two spaces), so a hostile path
or model/effort word never splits the line-group into a second
parseable line. Neither line is a §6 diagnostic line: `provider`
and `defaults` are not diagnostic codes, and a folded continuation
starts with whitespace, so `IsDiagnosticLine` never recognizes any
line of the group. A refusal from the spawn plane for the resolved
pair (`ErrEffortMissing` when a required effort is still unset after
all three levels) is `plan_refused`, and the launcher completes the
module's message — model, vocabulary, recommendation — with its own
flag spelling, `--effort`. The launcher MUST NOT retry with a
different pair.

### 4.4 Obtain the launch plan (spawn plane)

The launcher obtains the plan through
`vendorplugin.BuildLaunchWithEnvironment(ctx, registry, request, agentic.LaunchModeInteractive)`,
the entry point consumed from the pinned `agents-management` tag `v0.5.62`.
`BuildLaunchWithEnvironment` resolves the runtime and vendor model row, admits the model
and effort word against that row, and calls `agentic.BuildPlan` with the
resulting `LaunchRequest`. The launcher MUST NOT bypass that admission
by calling `BuildPlan` with a bare model id or reconstructing the row's
`EffortSupport`. The `vendorplugin.SpawnRequest` inputs are:

- `Runtime`: the module-declared runtime for the §4.2 mapped system
  (`claude` for `claude-code`, `codex` for `codex`; native-Pi availability
  remains subject to the upstream evidence and release boundary in §4.2);
- `Model` and `Effort`: the §4.3 resolved pair;
- `Home`: the managed home from the fragment's home variable (§4.1),
  passed through to `LaunchRequest.Home`;
- `WorkDir`: the launcher's current working directory;
- `PermissionMode`: the §4.3 resolved `native` or `yolo` value, passed
  through `LaunchRequest.PermissionMode` for `LaunchModeInteractive`;
- `ToolRelease`: the exact tool release required by the module's
  release-bound permission capability;
- `NativeArgs`: the opaque suffix after `--`, supplied to the module's
  `permission-grammar-v1` conflict check when the resolved mode is `yolo`;
  the launcher does not own or restate that grammar. Native mode does not run
  permission-conflict inspection and preserves the raw suffix;
- `Env`: `os.Environ()` for native and hosted composition, supplied
  as the parent environment for `LaunchRequest.Env`. The resulting
  `Plan.Env` is the plugin's complete filtered child environment over
  that parent, not a delta;
- `Composition`: **empty**. Claude/Codex fragment context channels are supplied through
  `Context`, carrying the profile pin, precedence, managed home, MCP metadata
  and selected system-prompt intent. v2/v3 context uses the v1-compatible
  subset; the launcher retains the permission member and original fragment
  digest. Reserved members are excluded. Muse declares no prompt/MCP channels
  and uses a nil context carrier for its interactive root plan. A non-empty
  composition in interactive mode is
  refused with `ErrCompositionNotInteractive`;
- `Run`: zero; this terminal launch supplies no task-board run context.
  Goal, budget, service tier, and assignment prompt remain unset.

The mode is requested **by name**, `agentic.LaunchModeInteractive`
(Decision 0013 D5). `LaunchRequest.PermissionMode` carries the resolved
mode. In the pinned agents-management v0.5.62, `LaunchRequest.ToolRelease` and
`LaunchRequest.NativeArgs` bind the request to the release-specific
`permission-grammar-v1`; agents-management owns the permission mapping,
argv grammar, and capability table. The launcher passes the request and
composes the admitted plan without spelling or reconstructing a provider
mapping. An unverified yolo mapping returns
`ErrPermissionModeUnverifiedRelease` and fails closed as `plan_refused`;
native mode remains verbatim. A system that does not declare
`LaunchModeInteractive` is refused by the module with
`ErrUnsupportedLaunchMode` → `plan_refused`.

The plan is a value — `Binary`, `Argv`, `Env`, `Stdin`, `WorkDir` — and
building it starts no child process. `Stdin` is attached only for a system
whose effort transport is stdin; none of the §4.2 systems is, so in this
revision the plan's stdin is unattached for every launchable environment
(Decision 0013 D4).

**Provider-limit admission is a separate launcher-invoked read.** Neither
`BuildLaunch` nor `BuildPlan` reads provider-limit state. Before launching
in either mode, the launcher MUST call
`providerlimits.Store.AvailabilityFor(providerlimits.VerdictQuery{Runtime, Model, Home})`
with the same resolved runtime, model, and **managed** home. The module
owns the state interpretation and verdict; the launcher owns making the
explicit check and enforcing it. A profile A launch therefore never gates
a profile B launch through shared evidence, and an empty or native home
MUST NOT substitute for the managed home.

Only a verdict whose `Serviceable()` is true (`AvailabilityHealthy`) admits
launch. This includes a determinate read that finds no limit record under
the module's fail-open state contract; it is distinct from an unknown or
failed read. A non-serviceable verdict is `plan_provider_limited`, with
its state and `Until`, `Checked`, `Observed`, and `Failures` evidence
surfaced. Failure to produce a verdict is a terminal `plan_refused`, with
the module error, never healthy by inference. The launcher MUST NOT retry,
downgrade, or substitute a model around a refusal. Model and effort
admission stay inside `BuildLaunch`; provider-limit checking does not
replace them.

### 4.4b Resolve the network profile (network plane)

After admission and before composition, the launcher resolves the
explicit `--network` selection through `curator-network-profiles`
v0.3.1 (§7). Without `--network` this step is absent: no catalog is
read, nothing probes, and the launch composes exactly as before —
with the enumerated module-owned differences: agents-management v0.5.45+
module appends its unconditional `--disallowedTools=AskUserQuestion`
denial to the direct Claude argv (native shape) and to
the §4.4 admission argv pin. The v0.5.62 lineup changes Muse fallback
from `muse-spark-1.3-contributor` / `max` to
`muse-wonder-1.0-contributor` / `xhigh`. Everything else in an unmanaged
launch is byte-identical to baseline fbcdbaf0; the committed
`TestUnmanagedGoldensMatchBaselineFbcdbaf0` pins that parity by
comparing the shipped goldens against baseline copies and allowing
exactly the Claude added line and the two Muse fallback value changes
(alongside the stated permission-source normalization). With `--network`,
the order is closed:

1. A blank selection refuses as `network_profile_invalid` without any
   I/O.
2. A hosted selection refuses as `network_scope_unsupported` without
   catalog or probe I/O: the destination does not enforce this scope.
3. The operator catalog loads through the operator's original parent
   environment — never the fragment's managed home — and the explicit
   selection resolves: allowed set, existence, assurance,
   confirmation at the current digest, and engine coverage. Only the
   explicit origin exists in this revision; inherited, runtime,
   project, and operator defaults arrive separately. Direct routing is
   a named `kind = "direct"` profile in the catalog — not a magic
   selector — resolved like any profile: it needs confirmation at its
   current digest, and engine coverage is vacuous for it (no proxy,
   nothing to bypass). Failures are `network_file_unreadable`,
   `network_profile_invalid`, `network_profile_unknown`,
   `network_profile_denied`, `network_scope_unsupported` (enforced
   assurance), and `network_configuration_conflict` (an uncovered
   engine host under a proxy profile).
4. Adapter admission calls `adapterprobe.Evaluate` over an immutable native
   binary snapshot with the selected profile's `SensitiveEgress`, loaded
   operator `KnownBad` or `KnownBadErr`, and the selected policy. Optimistic
   is default; `--network-policy strict` selects strict, and
   `--network-policy pinned:<64 lowercase hex SHA-256>` supplies the library's
   `PinnedBuild` identity. Pinning alone never qualifies. Sensitive egress
   cannot be downgraded by a flag. The legacy N-B label allowlist is retired:
   no label is migrated to an `AllowedBuild`. `Policy.Allowlist` is empty
   pending independent central qualification (A). Every known-vendor artifact
   is currently unqualified under optimistic policy and refused under strict
   or sensitive-egress policy, with the reason "no qualified builds until
   central qualification (A) ships" accompanying `strict_miss`.
   The library owns qualification, known-bad membership, pin enforcement and
   vendor validation. The launcher accepts only qualified or unqualified typed
   Decisions, compares `Decision.BoundIdentity()` with the host snapshot tuple,
   and independently enforces the existing child-scope ceiling: generic
   adapter, Claude, actual print (`exec`) entrypoint. A different build label
   does not widen scope. EffectiveEntrypoint still derives print from the
   admitted argv and exact native suffix; interactive mode is never collapsed
   to print. Unknown vendor lines/adapters, known-bad binaries and unreadable
   known-bad lists refuse typed `network_scope_unsupported` before any workload.
   The known-bad root is the original operator `HOME/.curator`; an explicit
   `--network-known-bad` path overrides the fixed filename. Loading uses
   `LoadKnownBad(SafeKnownBadRead, root, explicitPath)`.
   The process owner stages the bytes evaluated and executes that private,
   read-only artifact instead of reopening the original executable path.
   Native-format snapshots do not themselves establish conformance; scripts
   and mutable interpreter closures refuse under the library contract.
5. A proxy-family name in a composed overlay — `frag.Env`, the engaged
   prompt channel, or `mcp.env_names` — refuses as
   `network_configuration_conflict`, matched case-insensitively. The
   check is pure, so it runs before the preflight and never costs a
   network probe. Inherited proxy values are not overlays and are
   replaced normally by the patch.
6. A bounded preflight probes the resolved endpoint with the
   library's default timeout: TCP always, then CONNECT and TLS to the
   profile's agreed target when one is configured. A targetless probe
   proves TCP only. A `kind = "direct"` profile skips every probe
   step — the prober is not invoked — and its Record carries
   skipped/skipped/skipped. Failures are `network_proxy_unreachable`
   and `network_proxy_auth_failed`.
7. The generic patch binds to the resolved digest, assurance, and
   verified adapter identity — unset-only with an empty set for a
   direct profile — and the Record is built from the probe statuses
   and time only — never the endpoint, the patch, or the
   environment.

Any error in this step terminates the launch without weaker routing:
no network error ever degrades to an unmanaged launch, and an
admission failure in §4.4 reaches neither the prober nor the workload.
The §4.3 release probes (`<binary> --version` subprocesses) run before
this step in the ambient environment; they are established local-only
by inspection — exactly `--version` argv, stdout parsing, no socket use
in the probe — and any network behavior inside a tool's own `--version`
is outside cooperative assurance.

A managed direct launch prints one provenance line on stderr after
composition succeeds:

```text
curator-run: network: profile=<ref> origin=explicit digest=<digest> adapter=<adapter> harness=<harness> build=<build> entrypoint=<entrypoint> assurance=cooperative probe=<tcp>/<connect>/<tls>
```

Unqualified Decisions additionally require verbatim `Provenance.OperatorText()`
on stderr and one parseable `curator-run: adapter-provenance: <JSON>` stderr
line containing the full `relux-adapter-provenance-v1` record. These lines
cannot be suppressed by native quiet, silent or machine-output flags. Emission
failure stops the launch. Native launches own no session envelope;
a wrapper that owns one persists the typed record. Hosted launches
continue to refuse `--network`, and their guard tests block future admission
without session-owned provenance persistence. No launcher session state is
introduced (§2, lines 75–81).


It carries the manifest-safe Record members only. Every value is
folded with the §4.3 framing rule, so a hostile build string never
forges a second parseable line, and the line is never a §6 diagnostic
line. The Record is provenance, never environment: no Record member
becomes a child variable. A launch that composition refuses prints no
binding Record.

Engine coverage checks the launch's loopback engine hosts against the
profile's `bypass_hosts`. No launchable runtime in this revision
carries an engine, so the set is empty and coverage is vacuous; the
resolve call still carries the set, and a future engine-capable
runtime derives its hosts at the launch call site.

### 4.5 Compose the launch

The composed launch is one plan (Decision 0013 D6.3). Its members, closed:

The §4.3 permission mode is already carried in the agents-management
`LaunchRequest.PermissionMode`; any admitted mapping is part of the
module's plan. The launcher never turns the mode into argv itself.

**argv** — the agents-management plugin constructs Claude/Codex context
channels through `SpawnRequest.Context`. Claude orders model/effort, MCP, prompt,
then native arguments; Codex orders prompt, MCP, model/effort, then native
arguments. The native suffix is last and verbatim. The launcher never spells
these context-channel flags. Pi retains plan → selected prompt → native order.

Native prompt/MCP overrides that collide with fragment channels are refused
by the shared typed `ContextDescriptorConflictError`, surfaced as `plan_refused`
with the native arguments and fragment channel. Non-colliding native arguments
pass through. This context refusal applies in native permission mode too;
permission mapping otherwise remains unchanged.

**The codex layer file MUST be stat-ed before launch.** Two verified
facts about codex (environments.md §7.8, codex 0.153.2) shape the
`codex_cli` MCP part and are restated here because they bind the
launcher, not Curator:

- `-p <name>` layers `$CODEX_HOME/<name>.config.toml` on the base
  configuration, and a **missing layer file is silently ignored** — exit
  0, the launch proceeds with no MCP set — under `--strict-config` too.
  The tool therefore cannot tell the operator that the profile's MCP set
  did not arrive. So, whenever the composed argv carries `-p curator-mcp`,
  the launcher MUST stat the fragment's `mcp.path` — by construction
  `<home>/curator-mcp.config.toml`, the `CODEX_HOME` of the fragment's
  `env` map — **immediately before** the §4.6 handoff or exec, at the same
  point as the §5.1 probe, in both modes. The stat has three outcomes and
  they are three different facts: a regular file the launcher can open for
  reading — launch; no file at the path — `mcp_layer_missing`; anything
  else (a directory, a dangling symlink, a permission or I/O error) —
  `mcp_layer_unreadable`. Neither failure degrades to a launch without
  `-p`: the fragment said the profile has an MCP set, and a codex that
  runs without it is exactly the silent failure the stat exists to catch.
  `env resolve --repair` already covers the same file as a
  marker-recorded surface, so a fragment in hand means the file existed
  at resolve time; the stat closes the §10.1 residual window for this one
  file, whose absence the tool would otherwise swallow. The launcher MUST
  NOT write, restore, or repair the file: a missing layer after a
  successful resolve is a fact to report, and the remedy is another
  `curator run`, whose `--repair` re-materializes it.
- `-p` accepts **exactly one** profile. A native `-p`/`--profile` colliding
  with the fragment MCP layer now refuses before launch through the shared
  context API. With no fragment MCP channel, the tool honors native profile
  arguments.

The interactive plan's `Binary` is the executable. **Order is contract:**
for some tools everything after the last recognized flag is the user
turn, so a channel flag after the native arguments would become prompt
text. The general rule is fixed here; the per-tool boundary is verified
against the pinned release before the conformance vectors freeze (§9).

**environment** — three layers, later overriding earlier per name:

1. the plan's complete filtered `Env`;
2. the fragment's `env` map;
3. the `variable`-kind channel of an engaged descriptor: the fragment's
   `mcp` descriptor for `opencode` (`OPENCODE_CONFIG` = the materialized
   path), and a `variable`-kind system-prompt descriptor under the §5
   opt-in.

The inherited environment is an input to the plan request (§4.4), never
an additional composition layer. The launcher MUST NOT overlay it beneath
`Plan.Env`: doing so re-admits plugin-removed names. Native execution
uses the complete `Plan.Env` base, preserving plugin removals (including
run-context keys with zero `Run`) and sanitized `PATH`.

The plan's **own names and values** are those returned by
`System.ChildEnv(nil, req)` for the same system and `LaunchRequest`, over
an empty parent. They MUST NOT be obtained by diffing `Plan.Env` against
the inherited environment, or by a second `BuildPlan` with `Env=nil`:
binary resolution requires `PATH` and refuses that request. These owned
values define hosted literals and the warning below; `Plan.Env` itself
includes inherited values and MUST NOT be serialized as literals.

The fragment wins on exactly its own closed names — the adapter-registry
variable names pointing at managed-home paths — and touches nothing else.
This conflict rule is safe by construction: fragment names come only from
the closed adapter registry and fragment values are manager-owned
managed-home paths (the §10.3 profile-influence boundary), so the
override can only re-aim the tool's home, never alter how the process is
launched. When layer 2 or 3 overrides one of the plan's own names, the
launcher SHOULD warn — the plan author declared an intent the fragment is
displacing — but the fragment still wins: the operator asked for the
profile's context.

**network patch** — when §4.4b resolved a selection, its patch applies
LAST, after every layer above and before `Env` is materialized, through
the library's `Patch.Apply`: the patch's `unset` names are removed
case-insensitively from both the composed environment and the owned
literals, then its `set` pairs are added to both. An unset-only patch
is managed, not empty: a `kind = "direct"` profile binds exactly such
a patch. While managed, a proxy-family name in an
earlier overlay — `frag.Env`, the engaged prompt channel, or
`mcp.env_names` — is a `network_configuration_conflict`, refused
case-insensitively; §4.4b refuses it before the preflight and
composition rechecks at application time. Inherited proxy values are
not overlays and are replaced normally. Disjointness is rechecked
after the literals land, under the same literal-versus-lookup rule
below. Patch application is silent — the §4.4b provenance line already
announces it. Without a selection this layer is absent and composition
is byte-identical.

**env_names** — the fragment's bounded `mcp.env_names` union minus every
name present in the composed owned literals. A literal wins over a destination
lookup; drop the lookup and warn with the variable name only. A name merely
inherited in `Plan.Env` remains a destination lookup. The hosted payload keeps
these sets disjoint. Native children receive the full composed environment;
the lookup union is informative there, and the same collision warning applies.

**stdin** — native execution preserves the interactive plan's attached bytes
exactly, including an attached empty stream. Unattached stdin inherits the
caller stream. Hosted capability gates govern attached stdin separately.
The ax-only UTF-8/base64url document encoding was
**removed 2026-10-27 (owner decision)**.

### 4.6 Hand off and exec

Figures: [route and refusal sequences](diagrams/launch-paths.puml) and
[transport and state owners](diagrams/artifact-layout.puml).

**Ax handoff: removed 2026-10-27 (owner decision).** The former `ax start`
request, execution profiles, provider mapping, tracked permission defaults,
provenance extensions and Session Record transport are historical. The
launcher never starts ax, reads its policy or falls back from it.

After CLI grammar validation and before any launch configuration parsing,
profile resolution, plan admission or process start, check for `ax.json` by
existence only with `lstat`, independently in all four locations: current
system and user launcher directories, plus deprecated system and user
`curator-run` directories (§4.7). There is no precedence or fallback for this
check. Any entry, including a disabled, empty, malformed, unreadable,
symlinked (even dangling), directory or FIFO entry, refuses
`ax_tracking_removed`, exit 2. Never read or parse its contents and never
follow its final symlink. The detail is the launcher-sourced `path`; the
message tells the operator to delete that file. Neither `--native` nor
`--untracked` bypasses it. Werkstatt up/recycle applies the same guard before
launch effects. Static help/version remain configuration-free.

`migrate --dry-run` lists every leftover. Apply creates an exact-byte `.bak`
then removes each regular file in one rollback-capable transaction; a
conflicting backup refuses before writes. It never copies retired policies
or their backups to the new directories. Non-regular entries are listed for
manual removal without opening or following them; apply refuses the whole
transaction before writes. System write authority follows the existing
migration policy. A second successful apply has no effects.

Native execution starts the composed `Binary` with the entire `Argv`, full
`Env`, `WorkDir` and raw stdin bytes. `Prepare` snapshots mutable inputs.
The launcher waits for the child, forwards signals, coordinates foreground
terminal ownership and stop/continue, restores the terminal, and preserves
exit status (`128 + signal` for signal termination). No PTY is allocated.

Immediately before child creation, check the executable, the §4.5 codex MCP
layer and the required §5 file-kind boundary, in that order. The first
failure wins and starts no child. A missing executable is
`exec_provider_missing` with its exact name and installation guidance.
A missing boundary callback refuses `launch_boundary_required`.

**Permission detector and refusal rules.** Headless is observed from stdin /
stdout TTY state and the closed marker set {`CI`, `GITHUB_ACTIONS`}; it does
not affect permission defaults. The §4.3 flag/profile/global/default ladder
applies to native and hosted. Curator's force-native lock is above that
ladder: visible `yolo` is `usage`; silence is native. Without established
permission transport a would-be `yolo` refuses
`permission_policy_unsupported`. An unmapped mode refuses
`permission_mode_unsupported`; neither failure retries with weaker policy.

When best-effort inspection finds known stored relaxations for a native
request, emit only this stderr provenance line, never a persistent record:

```text
curator-run: effective-native-policy: relaxation=<selector[,selector...]> source=<settings-source>
```

Selectors are sorted and unique, and name only inspected settings and source.

### 4.7 The launcher's configuration file family

Figure: [configuration and artifact ownership](diagrams/artifact-layout.puml).

The launcher owns four configuration files, all in the §4.3
level-2 directories (`${XDG_CONFIG_HOME:-~/.config}/swarma/launcher/`, default
`~/.config/swarma/launcher/`, for the operator; `/etc/swarma/launcher/` for the
machine), each with its own closed schema:

| File | Owns | Precedence |
|---|---|---|
| `defaults.json` (`curator-run-defaults-v3`, §4.3) | model and effort defaults, the launcher-global permission default, per env-id; the `locked` rule (`host` withdrawn by FRONT-DOOR §3.1) | operator over machine per member, unless the machine file is locked |
| `launcher.json` (`curator-run-launcher-v1`, FRONT-DOOR §3.1) | machine `host_mode`: `hosted`, `native`, or `auto`; absent member/file means `auto`; optional boolean `delegated_trial` (FRONT-DOOR §5.8) | machine only; either policy member in the user layer refuses and names the system path |
| `selector.json` (`curator-run-selector-v1`, FRONT-DOOR §3.4) | optional external model/effort selector, shadow observations or enforcing policy | system only; any user file refuses; selector choice above file defaults and below explicit flags; exact candidate tuple required, null effort only for an effort-less model |
| `session-managers.json` (`curator-run-session-managers-v1`, FRONT-DOOR §§4.2–4.4) | registered managers and optional selected id | machine first; no PATH search; the selected manager participates in launch selection under FRONT-DOOR §§3–5 |

`ax.json` and its schema are **removed 2026-10-27 (owner decision)**.
It is now only the all-location refusal sentinel described in §4.6.

For one release, each active filename independently falls back to the corresponding
`/etc/curator-run/` or `${XDG_CONFIG_HOME:-~/.config}/curator-run/` file only
when the new path is genuinely absent. Each fallback read prints one stderr
line: `curator-run: deprecated config path <old>; move it to <new> (curator-run migrate)`.
When both paths exist, the new one wins and the old one is not opened.
`curator-run migrate [--dry-run]` copies the old files transactionally; see
[FRONT-DOOR §13](FRONT-DOOR.md#13-compatibility-and-migration).

Before any selected file (old or new) is parsed or used, the reader MUST open it without
following a final-component symlink and validate the opened file. It MUST be
a regular file owned by the same identity as its containing configuration
directory. On POSIX systems, any group-write or other-write mode bit is a
refusal. On Windows, a missing or null DACL is a refusal, as is an allow ACE
that grants file-write, delete, or security-control rights to an identity
other than the file owner (including its OWNER RIGHTS and CREATOR OWNER
aliases), LocalSystem, or Builtin Administrators. The
diagnostic detail names the refusal reason
(`symlinked configuration file`, `configuration file owner`,
`group/world-writable configuration file`, or `Windows DACL`). A failed open,
metadata query, DACL query, or read is `defaults_config_invalid`; it MUST NOT
be treated as an absent file. Only an absent file or a genuinely absent
configuration directory keeps the absence behavior defined by §4.3 and §4.6.
These checks apply independently to machine and operator files before their
respective precedence rules.

These are **launcher-owned knobs, not manager knobs**. Curator's machine
configuration is the closed knob table of environments.md §12.1, carried
by `manager-config` schema 2 under one `environments` object, and none of
the launcher's knobs appears there: environments.md §12.1 names no
launcher section, model default or effort default. Conversely the launcher reads no §12.1 knob. Every knob of that
table that shapes a launch reaches the launcher **only through the
fragment**, resolved by Curator: `passable_env_names` bounds the
fragment's `mcp.env_names` before the launcher sees them (§10.3);
`system_prompt_files.<profile>.pi` decides whether a file-kind file is
materialized, which the §5.1 probe detects on disk; `current_profile` and
`scoped_current` select the profile when `--profile` is absent;
`isolation`, `forms`, `in_place_mode`, and the rest shape the managed
home the fragment's `env` map points at. Curator's `permissions.<profile>`
knob reaches the launcher only as `fragment.permissions`; its value is never
copied into `defaults.json`. The launcher never opens `manager-config`, never
resolves a `curator` knob itself, and never duplicates a §12.1 value into its
own files. The one open item is
recorded in §9: should Curator's machine configuration ever grow a
launcher section, both files move there by specification revision and
their schemas stay.

### 4.8 Host selection

Host selection follows [FRONT-DOOR §3.2](FRONT-DOOR.md#32-resolution),
with the registry in [§4](FRONT-DOOR.md#4-session-manager-registry),
capability decisions in [§5.2](FRONT-DOOR.md#52-operations-and-capabilities)
and delegated activation in
[§5.8](FRONT-DOOR.md#58-activation-boundary-for-launch-delegated1).
The machine `host_mode` replaces the withdrawn per-environment `host`
member; old v3 defaults carrying that member refuse.

A registered and selected conforming manager takes the delegated route for
Claude/Codex under the post-Q1 policy. In `auto` mode, no manager registered
or selected means native launch with a one-line notice. Explicit hosted
instead refuses `session_host_missing` (exit 1), with
`reason=no manager registered` or `reason=no manager selected`, after the
earlier checks in FRONT-DOOR §3.2. These are selection outcomes; a subsequent
hosted refusal never falls back to native.

Explicit `--native`/`--untracked` and machine native mode return without a
registry read, after the all-location retired-policy guard (§4.6). Hosted network, terminal, stdin, resume and
runtime-capability checks retain the order defined by FRONT-DOOR §§3–5.
The old machine-hosted-default upgrade gate is replaced by machine host mode
and the manager's upgrade-behavior disclosure.

Hosted launches retain the shared permission resolution and typed resume
intent; the plugin composes delegated launches against the resolved profile.
The exact-plan route and real Muse hosting remain subject to their own
FRONT-DOOR gates. The retained payload contract below does not activate them.

### 4.9 Hosted payload and receiver handoff

This section retains the exact-plan payload and descriptor contract; it does
not describe the activated delegated request. Current hosted selection and
`launch-delegated/1` use [FRONT-DOOR §§3–5](FRONT-DOOR.md#3-host-mode-hosted-by-default)
and [§5.8](FRONT-DOOR.md#58-activation-boundary-for-launch-delegated1).
Exact-plan activation remains subject to FRONT-DOOR's own gates. The legacy
receiver transport is retained; it is not the selected-manager plugin path.

Figures: [hosted handoff sequence](diagrams/launch-paths.puml) and
[private payload and descriptor layout](diagrams/artifact-layout.puml).

An exact-plan launch builds the same plan once and serializes the composed
value — binary, ordered argv, full env, cwd, home, stdin, and the Curator
fragment record — into the versioned
`urn:relux:task-board:session-launch-plan` 1.0.0 payload instead of
exec'ing. The payload carries a `content_digest` (SHA-256 over the CCJ-1
bytes of the object excluding only the digest), the pinned producer
identity, the unsealed exec guard exported from the admitted plan, the
module's closed restart template over the composed argv, the elevated
resume intent, and the effective-native-policy projection (selectors and
inspected absolute source paths only, else null). Source paths are
absolute and at most 4096 bytes; relaxations are bounded by the closed
three-selector vocabulary. The policy carries
`permission_mode` `native` (the frozen 1.0.0 const), the resolved
`permission_source`, and `execution_profile` `standard` for a native
plan or `yolo` for a yolo plan (Q-D3). Names stay disjoint
from owned literals, whose values equal the final env; at most 64
lookup names project, sorted and unique, and every lookup
name resolves present in the original caller env (absent refuses
`launch_plan_invalid` with reason `required_env_missing`); binary, cwd,
and home are absolute; and the wire limits (1 MiB object, depth 16,
argv, env, literal, and versioned-data bounds) hold. Any violation
refuses `launch_plan_invalid` (exit 2) before contact.
Session metadata (`session_name.native`, `remote_control`) is read from
`Plan.Session` of the same admitted plan the payload is built from: the
module's Claude plugin fills the native name verbatim and the RC intent
with the exact plan-argv indices of the RC tokens while it builds the
argv (enabled with empty indices is the settings-origin form). The
launcher parses no provider option. The indices describe the plan argv;
for Claude the composed argv equals it (the module builds the context
channels into the plan), and a composed argv that differs refuses
`launch_plan_invalid` instead of being re-indexed.
The record is verified by the module in-process only, so the payload is
built in the same process from the in-hand plan, before any export, and
is never re-derived from an imported plan. A plan whose system defines no
session surface (`Session` nil) carries the schema-zero metadata (null
name, disabled RC). An authored tail naming two different sessions
(`-n A --remote-control B`) is refused by the module while it builds the
plan (`plan_refused`, before composition and any receiver contact).
A piped launcher stdin refuses `session_host_stdin_unsupported`
(exit 6) before the plan build and before any receiver contact: the
property is knowable before composition, so the gate sits after resume
elevation and before the build. An attached plan stdin refuses the
same code after the build — it is knowable only from the built plan.
Phase 1 carries terminal stdin only. Values travel only on the
private receiver stdin; hosted diagnostics carry fields, reasons, and
sizes, never native argument values. In particular the hosted
context-descriptor conflict names only the fragment channel and the
module's fixed reason (the native path keeps its legacy shape), and
an unknown native policy mode reports the fixed sentinel instead of
the operator's mode value.

The private receiver argv is `task-board session launch-plan --plan -`
`--terminal-fd 3 --status-fd 4`, with the controlling terminal read/write
on fd3 and a dedicated receiver-to-launcher status pipe on fd4; standard
output and error stay the live terminal. Before the plan build the
launcher opens the terminal and validates it; a missing or wrong
terminal descriptor refuses `session_host_terminal_required` (exit 6)
with zero builds and zero receiver lookup, and the transport reuses
that descriptor. fd4 carries bounded 32-bit big-endian length-prefixed
JSON status records of at most 64 KiB. The launcher classifies only
fd4, against the closed
`urn:relux:task-board:session-launch-status` 1.0.0 refusal envelope:
exactly the six members `schema`, `schema_version`, `type`, `code`,
`message`, `details`; the pinned consts; a code from the complete
hosted-diagnostics registry (133 Appendix V-ERR members after removing the two ax-only rows, recognized
even when the launcher never emits them); the row's literal constant
message (byte equality); and a details object inside the row's
vocabulary (D0 exactly `{}`, D1/D2 closed field/reason subsets, string
values only). The last valid refusal emits its registry code, constant
message, and validated field/reason with the registry exit (1/2/6/16):
the channel — never the receiver status — classifies. Any complete
frame outside the contract — bad JSON, wrong members, unknown code,
mismatched message, bad detail, zero or oversize length — normalizes
to the fixed `session_host_protocol_error` (exit 6) with the constant
message and empty details; receiver bytes never reflect. A truncated
frame (EOF mid-record) is transport loss and refuses
`session_host_unavailable` (exit 1), unless a complete malformed frame
was already observed: poison is permanent, so a later partial header
or body still yields `session_host_protocol_error` (exit 6), and only
a clean stream maps a transport fault to unavailable. Without a record
the receiver exit propagates unchanged, including child statuses that
coincide with refusal exits. A missing receiver refuses
`session_host_missing`. The receiver owns the terminal, all deadlines,
and completion; the launcher waits as for a native exec.

Descriptor contract (r6 §4 reconciliation): the launcher marks its own
fd3/fd4 copies close-on-exec and validates descriptor type, access,
and ownership before contact (fd3 a read-write terminal or file owned
by the caller or root; fd4 a caller-owned write-only pipe; both
close-on-exec). The receiver ingress flags are necessarily clear:
close-on-exec descriptors cannot survive the exec that delivers them,
and Go's process spawn clears the flag for inherited descriptors, so
ingress-clear is required, not a leak. Descendant isolation is
established by the receiver, which MUST set close-on-exec on fd3/fd4
immediately on startup before spawning any child; the launcher proves
the mechanism end to end with a cooperative receiver that reports
ingress-clear, marked-set, and a grandchild that inherits neither
descriptor.

## 5. System-prompt application

Resolving a fragment activates nothing: the fragment's `system_prompt`
section is data about a channel — the inert materialized file's path and
the adapter's declared channel descriptors — never an applied override.
Claude/Codex `flag` and `config-key` context channels are constructed by
the shared API. The launcher selects the intent and retains the Pi path;
system-prompt application happens only behind the explicit
`--system-prompt <append|replace>` opt-in. `file`-kind channels are the
one exception: the launcher never applies them — the tool does, on its
own, subject to native discovery precedence — so the launcher's whole
duty for them is detection and warning (§5.1).

Application, by descriptor kind (the per-environment channel tables are
environments.md §7.3 and are not restated here):

- **flag-class** (`claude_code`, `pi`): append the descriptor's flag with
  the fragment's system-prompt path to the plan argv, before the native
  arguments. Claude flags are constructed by the shared API.
- **config-key** (`codex_cli`): apply the descriptor's key
  (`model_instructions_file`) with the fragment's path through the tool's
  declared configuration-override mechanism. The exact override spelling
  verifies against the pinned tool release before the conformance vectors
  freeze, per the environments.md §7.3 discipline.
- **variable-class** (`gemini`, once that adapter lands): set the
  descriptor's variable to the fragment's path in the child environment.
- **file-class** (`pi`: `APPEND_SYSTEM.md` for `append`, `SYSTEM.md` for
  `replace`): the launcher applies nothing, because native discovery belongs
  to the tool: same-semantics flags suppress discovery, and a trusted
  project file takes precedence over the agent-home file (§5.1). The
  launcher MUST NOT place, remove, or edit a
  file-kind channel's file: those files are materialized exclusively by
  Curator, and only under the per-profile × environment
  `system_prompt_files` machine setting (environments.md §5.5, default
  `off`). The opt-in for a file-kind channel happened at the
  machine-setting level, not on this command line. The launcher's duty
  is §5.1: detect managed-home file presence and warn within that bound.

### 5.1 File-kind channels: detection, native precedence, warning

The file-kind probe is keyed on the **environment**, not the fragment.
For an env-id whose environments.md §7.3 adapter registry declares
`file`-kind channels — in revision 1 exactly `pi`, with
`APPEND_SYSTEM.md` (`append`) and `SYSTEM.md` (`replace`) — the launcher
MUST probe, on **every** launch, immediately before the handoff or exec
of §4.6, the path `<home>/<filename>` for each filename in that closed
registry set, where `<home>` is the managed home the fragment's `env`
map points the tool at. The probe runs whether or not the fragment
carries a `system_prompt` section: native file discovery does not depend
on the fragment's descriptor list. Presence in the managed home does not
prove that the tool selects that file; the precedence below applies. The
registry set is closed and versioned with the environments protocol, and
the launcher already owns the §4.2 environment mapping, so keying the
probe on the registry adds no new knowledge edge. The probe has exactly
three outcomes, and they are three different facts:

- **Absent** — no file exists at the probed managed-home path. This
  is the legitimate default: `system_prompt_files` defaults to `off` and
  §5.5 keeps both files unwritten under it. No warning, no diagnostic,
  no launch change follows from this probe. Absence here is never an
  error and does not establish absence of native project-local files or
  other native prompt inputs.
- **Present and readable** — a regular file the launcher can open for
  reading. The launcher MUST print the §5.2 warning naming this observed
  managed-home file, even without `--system-prompt`, a `system_prompt`
  section, or a descriptor for this filename. Presence is not proof of
  application: the warning states the native precedence below. When the
  machine setting materialized the file, that was the materialization
  opt-in; a stray file has no such recorded opt-in.
- **Anything else** — the path exists but cannot be read (permission,
  I/O error), or is not a regular file (a directory, a dangling
  symlink). The launch fails with `sysprompt_file_unreadable`. A failed
  probe is a read failure, never an absence. This existing managed-home
  validation remains mandatory even when native precedence could keep
  the tool from selecting the file; it does not attest which source the
  tool will apply.

The `--system-prompt <semantics>` opt-in still selects only the fragment's
non-`file` channels; a `file` descriptor never satisfies it. Native Pi
0.84.2 source selection is separate: a native `--system-prompt` value
suppresses `SYSTEM.md` discovery, and native `--append-system-prompt`
values suppress `APPEND_SYSTEM.md` discovery. A flag and a discovered
file of the same semantics are alternatives, not additive. Thus launcher
`--system-prompt append` spells the declared append flag, which suppresses
`APPEND_SYSTEM.md` discovery even if the managed-home probe finds that
file. The warning names the flag as applied and the observed file as
suppressed, not as a second application. This adds no replace flag to
the adapter registry and does not inspect or alter native arguments (§3).

When discovery applies, an existing `<cwd>/.pi/SYSTEM.md` or
`<cwd>/.pi/APPEND_SYSTEM.md` in a trusted project wins over the
corresponding `<agentDir>` file (`agentDir` is the home selected by
`PI_CODING_AGENT_DIR`). These project files are outside the managed-home
probe set. Without an applied launcher flag establishing suppression,
the warning reports a home file as a native-discovery candidate subject
to flag and trusted-project precedence, not as a verified selected source.
The accepted E5 loader evidence cited above establishes these semantics;
no project probe or new channel is introduced.

Detection is the launcher's whole authority here; removal is not. The
launcher MUST NOT remove or edit a file the probe finds, whatever put it
there. A file Curator materialized under the `system_prompt_files`
machine setting is a marker-recorded managed surface, owned by Curator's
materialization and covered by its drift and repair (environments.md
§5.5, §8.4). Any other file at a registry filename is an **unmanaged**
file: Curator's repair explicitly leaves unmanaged files untouched and
its drift detection covers only marker-recorded surfaces (environments.md
§10.1, §8.4), so no automated contract in the cited protocol removes it —
removing it is the operator's deliberate action, and until it is removed
every launcher-mediated launch into that home warns.

### 5.2 Selection and refusal rules

- The launcher selects the fragment channel whose `semantics` equals the
  opt-in value, considering only `flag`, `config-key`, and `variable`
  descriptors. If the fragment carries no `system_prompt` section (the
  resolved chain has no applicable system modules), or no non-`file`
  channel with the requested semantics exists for the environment, the
  launch fails with `sysprompt_channel_unavailable` — a `file`-kind
  descriptor with matching semantics does not avert this refusal,
  because the launcher cannot engage it. Opting in to nothing is an
  error, not a silent no-op: the operator asked for a customized run
  and MUST NOT get an uncustomized one without noticing.
- Without the opt-in the launcher applies no channel, adds no flag, sets
  no key, and exports no variable. Managed homes carry no materialized
  system-prompt file by default (environments.md §5.5). Native inputs,
  including trusted project-local files, may still customize the run;
  §5.1 detects and warns about managed-home files only.

**Warnings.** Every flag-class, config-key, or variable-class channel the
launcher applied under the opt-in, and every present readable managed-home
file detected by §5.1, MUST be covered by a warning on stderr before the
§4.6 handoff or exec. It states:

1. the profile, descriptor kind (and filename for a probed file), and
   semantics of each applied launcher channel and observed home file,
   distinguishing application from suppression or conditional native
   discovery as required by §5.1; it MUST NOT claim to enumerate all
   native sources or infer an uncustomized run from home-file absence;
2. for `replace` semantics, that replacement discards the tool's built-in
   system behavior entirely, conditional on selection when the observed
   home file is only a discovery candidate;
3. that a custom system prefix can change how requests are cached and
   therefore billed — a tool's default system prompt may participate in
   shared prompt caching, while a custom one forms its own cache prefix
   (exact per-tool behavior is Decision 0010 open question 7's research).

Warnings cover both command-line application and observed machine-setting
materialization: the operator at the keyboard may not be the operator
who configured the machine. They are not suppressible in revision 1;
absence of `--system-prompt` MUST NOT suppress a home-file warning.

Reproducibility over silence: the cost of the warning is one stderr
line-group; the cost of a silent customized run is an operator
misattributing behavior to the tool.

## 6. Errors and diagnostics

This section governs the existing interactive API. Target one-shot
refusals and the unresolved numeric mapping are specified in §10.5. The
[same-user target contract](docs/same-user-nonchat-contract.md#5-execution-and-observable-outcomes)
separately distinguishes admission/start/native/result/cleanup evidence; it
assigns no shipped refusal names or numeric exits and does not reuse §6.

Diagnostics are stable machine-readable codes in closed families. A
failing launch prints exactly one diagnostic code line to stderr,
followed by human-oriented detail; the code, not the prose, is the
contract. Usage errors and the hosted configuration refusals exit 2;
hosted protocol refusals exit 6; hosted policy refusals exit 16; every
other operational failure exits 1. The table below is the
launcher-emitted set; fd4-normalized receiver refusals (§4.9) carry
registry codes with their constant messages from the separate
receiving registry, which recognizes codes the launcher never emits.

| Family | Codes | Condition |
|---|---|---|
| usage | `usage` | invalid CLI grammar, removed `--name` or `--ax-profile` (message says removed in 0.6.0), locked override, repeated host request, native resume selector, or visible `yolo` under a force-native lock — exit 2, nothing resolved or launched |
| resolve | `resolve_invocation_failed`, `resolve_environment_unknown`, `resolve_profile_unknown`, `resolve_repair_failed`, `resolve_lock_unavailable`, `resolve_fragment_invalid` | §4.1: the context plane could not produce a usable fragment — `curator` not startable, or a non-zero exit with an unmapped diagnostic, Curator's own code and message passed through verbatim; unregistered environment; uninstalled profile; the store cannot restore the stale home; the repair could not take Curator's mutation lock within its bounded wait; or the output is not a valid closed fragment |
| defaults | `defaults_config_invalid`, `defaults_unresolvable` | §4.3, §4.6, and §4.7: a launcher-owned configuration file — `defaults.json`, `launcher.json`, or `selector.json` — is symlinked, non-regular, owned by a different identity than its configuration directory, group/world-writable on POSIX, grants a non-owner/non-owner-alias/non-system/non-administrators Windows DACL identity write/delete/security-control access, or cannot be read or parsed, or names an unknown env-id or member; this includes a v1 file carrying `permissions`, or a v1 or v2 file carrying `host` — a read failure is never absence; the lineup admits no model for the mapped system after earlier model levels are silent |
| selector | `selector_unavailable`, `selector_abstained` | FRONT-DOOR §3.4/11: enforcing selector failed (`reason=timeout/exit/invalid`) or returned a typed abstention (`code` matches `[a-z0-9_]{1,64}`); exit 1, no fallback. Messages: `selector unavailable` and `selector abstained`. |
| registry | `session_manager_registry_invalid`, `session_manager_registry_unwritable`, `session_manager_registry_shadowed`, `session_manager_not_registered` | FRONT-DOOR §§4.2–4.3/11: invalid registry (`path`, `reason=unreadable/parse/schema/symlink/owner/mode`), unwritable explicit-scope target directory (`path`, `scope=system/user`, before locking/writing, no fallback), shadowed user mutation (`path`), or unknown id (`id`); exit 2 |
| registry contention | `session_manager_registry_busy` | FRONT-DOOR §§4.3/11: exclusive transaction lock unavailable after 10 seconds; no details; exit 1 |
| migration | `host_member_withdrawn`, `migrate_destination_exists` | FRONT-DOOR §§3.1/11/13: withdrawn defaults host member (`path`, `reason=present`), mixed system migration values (`path`, `reason=mixed_values`), or different destination bytes (`path`); exit 2, no migration writes on planning refusal |
| plan | `plan_refused`, `plan_provider_limited` | §4.4: the spawn plane refused the request (unknown system/runtime or model, model not driven by the system, mode not declared by the system, invalid or missing required effort, unresolved vendor, an unverified release-specific permission mapping, or failure to produce a provider-limits verdict), or the explicit provider-limits verdict was not serviceable (`AvailabilityHealthy`) — the verdict's structure and evidence are surfaced verbatim |
| environment | `env_unsupported` | §4.2: the environment has no spawn-plane system mapping in this revision |
| permission | `permission_policy_unsupported`, `permission_mode_unsupported` | §4.6: a would-be `yolo` lacks established permission transport, or its environment has no declared mapping; exit 1, terminal with no weaker fallback |
| exec | `exec_provider_missing` | §4.6: the plan's binary does not exist — reported with the executable name and installation guidance |
| mcp | `mcp_layer_missing`, `mcp_layer_unreadable` | §4.5: the composed argv carries `-p curator-mcp` and the pre-launch stat of the fragment's `mcp.path` finds no file, or finds something it cannot read as a regular file — codex would silently launch without the profile's MCP set, so neither degrades to a launch without `-p`; the two are distinct facts and are reported as such |
| system prompt | `sysprompt_channel_unavailable`, `sysprompt_file_unreadable` | §5.2: opt-in given but the fragment carries no non-`file` channel with the requested semantics; §5.1: a registry-declared file-kind channel's file exists but cannot be read (or is not a regular file) at the pre-exec probe — an absent managed-home file is not this diagnostic and says nothing about other native sources |
| network | `network_profile_unknown`, `network_profile_denied`, `network_scope_unsupported`, `network_configuration_conflict`, `network_proxy_unreachable`, `network_proxy_auth_failed`, `network_profile_invalid`, `network_file_unreadable` | §4.4b: the explicit network selection could not be honored — an unknown profile name; a denied one (outside the host allowed set, or unconfirmed at the current digest); an unsupported launch shape (hosted mode, a refused adapter Decision or unsupported child scope — including any launch without a print selection — an unmapped launch mode, or an enforced assurance request); a proxy-family name in a composed overlay or an uncovered engine host under a proxy profile; an unreachable proxy (TCP, CONNECT, or TLS step, or a malformed endpoint; a `kind = "direct"` profile skips every probe step); a proxy demanding authentication; a malformed catalog or profile; or a catalog or ledger that cannot be located, read, or parsed — every one terminal, with no fallback to an unmanaged launch; a hosted launch with `--network` refuses `network_scope_unsupported` with exit 16 (§4.8) while every native network refusal exits 1 |
| session host | `session_host_missing`, `session_host_unavailable`, `session_host_provider_unsupported`, `session_host_protocol_unsupported`, `session_host_scope_unsupported`, `session_host_execution_profile_unsupported`, `session_host_terminal_required`, `session_host_stdin_unsupported`, `session_host_default_not_ready` | FRONT-DOOR §§3–5 and §5.8 govern current hosted selection: explicit hosted without a registered or selected manager refuses `session_host_missing` (exit 1) with the reason detail; `auto` instead launches natively with a one-line notice. Terminal, stdin, protocol, runtime and capability checks refuse before the corresponding effect; a hosted failure never falls back to native. §4.9 retains the legacy exact-plan contract and its diagnostics, including historical `session_host_default_not_ready`; it does not activate that route. |
| resume | `session_resume_invalid` | §4.8: conflicting, ambiguous, or malformed resume selectors from the wrapper or the native tail, as elevated by the module typed-intent grammar — exit 2, before the plan build |
| launch plan | `launch_plan_invalid` | §4.9: the composed plan cannot be projected into the closed payload shape — non-absolute paths, literal/env mismatches, missing lookup names, an inadmissible guard, a restart mismatch, or a wire-limit violation; no value in the detail — exit 2, before contact |
| policy | `secret_policy_violation`, `policy_refused` | §4.9: the receiver reports a secret-policy violation or a refused policy — exit 16, receiver-mapped, with field and reason only |
| removed tracking | `ax_tracking_removed` | §4.6: any `ax.json` entry in any of the four configuration directories; launcher-sourced `path`, remedy delete that file; exit 2, no resolve or launch |
| plugin client | `credential_binding_mismatch`, `credential_injection_unavailable`, `credential_node_missing`, `credential_reserved_name`, `credential_source_conflict`, `credential_source_expired`, `credential_source_revoked`, `credential_source_stale`, `credential_store_unreadable`, `doorbell_setup_failed`, `environment_credential_store_unsupported`, `fragment_version_unsupported`, `session_board_goal_unavailable`, `session_curator_identity_stale`, `session_generation_mismatch`, `session_home_lease_unavailable`, `session_host_capability_missing`, `session_host_delegated_unauthorized`, `session_host_protocol_error`, `session_liveness_unknown`, `session_operation_conflict`, `session_operation_expired`, `session_resume_ambiguous`, `session_resume_identity_mismatch`, `session_resume_location_mismatch`, `session_resume_not_found`, `session_resume_unreadable`, `werkstatt_already_up`, `werkstatt_busy`, `werkstatt_clone_missing`, `werkstatt_context_check_failed`, `werkstatt_context_leak`, `werkstatt_git_layout_invalid`, `werkstatt_identity_missing`, `werkstatt_liveness_write_failed`, `werkstatt_not_found`, `werkstatt_not_up`, `werkstatt_operation_unresolved`, `werkstatt_profile_changed`, `werkstatt_profile_lock_mismatch`, `werkstatt_record_invalid`, `werkstatt_session_manager_required` | FRONT-DOOR sections 5 and 11: closed plugin refusals, capability and protocol validation, durable operation reconciliation, and credential policy. Exit 6 for protocol/capability errors, 16 for delegated authorisation, 2 for configuration and credential errors, otherwise 1. Plugin text is never printed. |

The former `ax_handoff_failed`, `permission_mode_tracked_unsupported` and
`host_configuration_conflict` diagnostics were
**removed 2026-10-27 (owner decision)**; they are no longer in the closed set.

Two invariants hold across every family. First, an absence and a failure
to read are different facts: a fallback defined for absence (no
`system_prompt` section means no channel to select; an absent managed-home
file means no warning from that probe; an absent defaults file means the next
level) never fires on a failed or malformed read
(`resolve_fragment_invalid`, `sysprompt_file_unreadable`,
`defaults_config_invalid`, `mcp_layer_unreadable`); and where absence is
itself the failure (`mcp_layer_missing`), it still carries its own code
rather than sharing the read failure's. Second, no
diagnostic downgrades the launch: every failure is terminal for that
invocation, and the operator retries deliberately.

## 7. Pinned dependencies

The launcher pins
`github.com/relux-works/skill-agents-management v0.5.62`, which carries
`LaunchModeInteractive`, `LaunchRequest.PermissionMode`,
`LaunchRequest.ToolRelease`, `LaunchRequest.NativeArgs`, the
`permission-grammar-v1` token, `ErrPermissionModeUnverifiedRelease`,
the `ElevateResumeIntent` typed-intent API, the closed `claude-restart`
template export and transformation check, the pinned hosted schemas,
`Plan.ExportSeal`, and the typed `Plan.Session` (native name and RC
intent with argv indices).
The module also admits attested well-formed Muse builds without a release
table, the optional frozen-copy tuple, `muse-wonder`, fragment v3, and bounded
help-evidence-backed Muse yolo. ToolRelease is a claim cross-checked by the
sealer, never caller-minted admission evidence.
The module owns the permission mapping, grammar, and
capability table; this SPEC deliberately contains no provider permission
flag spelling. The pinned module's plans-as-values contract, argv
parity goldens, frozen admitted-pair digests, per-model effort rules,
and provider-limit verdicts remain module-owned. `vendorplugin.Lineup`
and the runtime compatibility registry supply §4.3 model/effort fallback.

The launcher pins `github.com/relux-works/curator-network-profiles`
v0.3.1 (§4.4b), consumed by tag as a normal require — no replace, no
workspace. The module owns catalog loading, selection precedence and
validation, the generic environment patch (unset-only for `kind =
"direct"`), the bounded preflight, and the binding Record; this SPEC
owns where the launcher applies them and which launch shapes it
supports.

## 8. Versioning

Ax-related entries below are historical; their requirements were
**removed 2026-10-27 (owner decision)**.

The launcher v0.4.0 release reports `0.4.0` (specification `0.5.1-draft`).

- This specification is versioned semantically; the current version is
  **`0.5.1-draft`**. Draft versions may change incompatibly between
  commits; the `-draft` suffix is the signal that nothing downstream may
  pin them.
- The `curator-run` binary reports both its build version and the
  specification version it implements; the stub reports the
  specification version only.
- Per Decision 0010, this specification starts as an in-repository draft
  and is promoted to a sibling `curator-agent-launcher-spec` repository
  at stabilization — the moment a second implementer or a conformance
  suite needs it. Promotion drops the `-draft` suffix, freezes `1.0.0`,
  and moves conformance vectors alongside the prose.

### 8.1 Revision history

| Version | Change |
|---|---|
| `0.5.0-draft` | Decision 0018 permission interface. §3 adds typed permission mode, its exact alias, and rejected legacy spellings; §§4.1/4.3 define v2 fragment transport and mode precedence; §§4.5/4.6 pass the mode through agents-management, define headless and tracked refusals, and record native-policy relaxation; §4.7 and §6 update file and diagnostic contracts. Updated README and version pins. |
| `0.4.1-draft` | Four 0.2.1-review minors (TASK-260906-2t2t6w). §4.1/§6: the resolve pass-through clause — unmapped `--repair` diagnostics (`environment_marker_invalid`, `environment_surface_unmanaged_conflict`, `environment_backup_exists`, `environment_seed_unreadable`) collapse into `resolve_invocation_failed` with Curator's own code and message passed through verbatim, and the §6 gloss no longer leads with "`curator` not startable" for that class. §4.6: the three pre-launch checks run in the listed order — binary check first — and the first failure is the one reported. §9: the silent-MCP-absence residual for `claude_code` and `opencode` recorded. `TestSpecVersionPinned` now reads SPEC.md and README.md. |
| `0.3.0-draft` | §4.2: correct the Pi system to `pi-native` using accepted A0 E1/E2 and landed native-Pi support (PR23, `a2a6e9f`). Other API/environment errata remain separately tracked. |
| `0.4.0-draft` | E4 provider path (TASK-260916-16ys92). §2: the launcher is dispatched through the environments.md §11 trust-root resolution (curator-spec `0da4020`, PR #62) and reports its resolved executable path. §4.3: the stderr line-group grows to two lines — `curator-run: provider: path=<absolute path>` (own executable via `os.Executable` + `filepath.EvalSymlinks`, fallback `path=unavailable`, never fails the launch, folded with the existing framing rule) printed before the `defaults:` line at every launch; path-only in this revision because the umbrella passes no distinguishing marker, with the closed origin set deferred to a future revision. |
| `0.2.1-draft` | Follow-ups against environments.md 1.1 and the cycle-2 review. §4.1: the resolve invocation always passes `--repair`, with the read-only/fail-closed semantics of environments.md §10.1 stated, `resolve_repair_failed` kept, `resolve_lock_unavailable` added for `environment_lock_unavailable`, and `environment_home_stale` declared unreachable. §4.5: the codex layer file `<home>/curator-mcp.config.toml` MUST be stat-ed immediately before handoff or exec whenever the argv carries `-p curator-mcp` (a missing layer is silently ignored by codex, under `--strict-config` too), with `mcp_layer_missing` / `mcp_layer_unreadable`; `-p` takes exactly one value, so an operator `-p` after `--` fails the launch (Decision 0012 open question 3 closed). §4.6: `ax.json` `enabled: false` is not configured; the machine-over-operator precedence explained; the configuration read fires before a usage error. New §4.7 names the `defaults.json`/`ax.json` file family as launcher-owned knobs against the environments.md §12.1 manager knob table. §6: `defaults` row names §4.6/`ax.json`, `mcp` family added, invariant 1 extended. §9: docs-confidence item covers both files; codex `-p` item closed; residual-window item added. |
| `0.2.0-draft` | Decision 0013 D6 applied. §4 reordered fragment-first and grown to six steps: the managed home from the fragment is `LaunchRequest.Home` (D6.1, M7); the plan is requested as `LaunchModeInteractive` with an empty `Composition` and the launcher spells no provider flag (D5, M2); launcher-owned model/effort default precedence — flags, lockable `defaults.json` machine configuration, lineup fallback — with the resolved pair printed every launch (D6.2, M8); the composition rule with argv order as contract, the MCP channel applied by the launcher, the four-layer environment, and the literal-versus-lookup `env_names` collision rule (D6.3, F5); tracked mode specified as `ax start <name> --provider <id> --launch-plan - [--profile] --workspace <cwd>` with the request document, the four `works.relux.curator.*` extension keys, the `profile-pin` as the lock hash, session-name derivation, and Structured Error pass-through (D6.4). §3 gains `--name` and `--ax-profile`; §4.2 gains the `ax` provider-id column; §6 gains the `defaults` family; §7 requires the interactive-mode module release; §1 non-goals restated (D6.5). §5 unchanged apart from renumbered cross-references. |
| `0.1.2-draft` | §5.1 probe re-keyed from the fragment's descriptor list to the environment adapter's closed file-channel filename set, run on every launch into a managed home regardless of the fragment's `system_prompt` section; false stray-file drift-and-repair claim removed — a stray file at a registry filename is unmanaged, no automated contract removes it, and every launcher-mediated launch warns until the operator removes it; native/hand-launch and probe-to-exec race residuals recorded in §9. |
| `0.1.1-draft` | §5 restructured (§5.1/§5.2): file-kind channel semantics specified — launcher never places, removes, or edits the files; pre-exec presence probe; warnings mandatory for an active file-kind channel without the `--system-prompt` opt-in; orthogonal-and-additive selection when flag-class and file-kind coexist (historical claim corrected by E5 in §5.1); `sysprompt_file_unreadable` diagnostic added. |
| `0.1.0-draft` | Initial in-repository draft. |

## 9. Open items

- **Optional reviewer policies (owner decision open).** A local-only
  reviewer rule and exclusion of the orchestrator that requested the round
  are separate proposals, remain unchosen and off, and are not implied by
  trusted-circle membership or the positive review-role predicates (§10.2).
- **Workplace ingress and execution lifetime (accepted open requirement).**
  First consumer: bridge/helper/dispatcher, stage A. Before workplace
  activation, its contract package must jointly publish the ingress,
  workplace-UID relay and auxiliary-control-process, physical-start, quiescence,
  export and reconnect lifetime, including a trusted generation-bound fence
  against creation of new workplace-UID processes from before quiescence
  through export or replacement. It must reconcile ordinary round transitions,
  intermediate export, watcher/node-wide stop and explicit reopening with
  account-wide absence and outstanding claims (§§10.2, 10.6 and the transfer
  figures). The full fix lands with stage A's contract package because bridge,
  relay and join contracts are not frozen. This entry selects no mechanism
  and changes no normative behaviour.

- **Ax questions: removed 2026-10-27 (owner decision).** Destination
  environment filtering, handoff implementability and native-tail replay
  questions belonged only to the retired tracked route.
- The §5.1 probe observes only the two registry-named managed-home
  paths at one point in time, not all native prompt sources. Three
  residuals are accepted: native/hand launches have no launcher probe or
  warning and follow native precedence; files changed between the probe
  and startup read go undetected; and trusted project-local
  `<cwd>/.pi/SYSTEM.md` / `APPEND_SYSTEM.md` are not probed and win over
  home files when same-semantics flags have not suppressed discovery.
  Native arguments remain uninspected (§3). Home-file absence therefore
  cannot attest absence of native customization, even on a
  launcher-mediated run. No project probe, home writer, or tool-side
  warning is added by this revision.
- **Per-tool argv boundary.** The plugin-owned §4.5 order and the final native
  suffix are verified against each pinned tool release before the conformance
  vectors freeze: context and model/effort flags precede the native suffix,
  and the first native argument is where the tool's own parsing begins
  (environments.md §7.3 discipline). The codex_cli `-p curator-mcp` layer
  against an operator `-p` after `--` is no longer open: environments.md
  §7.8 verified on codex 0.153.2 that `-p` takes exactly one value, and
  §4.5 records the consequence (Decision 0012 open question 3 closed).
- The §4.2 mapping for `opencode` awaits an `agents-management` system
  plugin; until then the environment resolves but does not launch.
- The `gemini` variable-class channel (`GEMINI_SYSTEM_MD`) engages when
  the `gemini` adapter lands in the environments protocol.
- The codex_cli configuration-override spelling and both flag-class
  spellings verify against pinned tool releases before conformance
  vectors freeze (environments.md §7.3 discipline).
- The §4.7 launcher-owned file family remains separate from Curator's
  machine configuration. The retired ax policy is a refusal sentinel only.
- The §4.5 codex layer stat closes the environments.md §10.1 residual
  window for one file only. Every other managed surface is verified at
  resolve time and not re-verified before exec; the launcher MAY re-verify
  the marker-recorded hashes under a later revision, as §10.1 permits,
  and this revision does not.
- **Silent MCP absence beyond codex is unverified.** The §4.5 stat rule
  exists because codex demonstrably swallows a missing `-p` layer —
  exit 0, under `--strict-config` too. Whether `claude_code`
  (`--mcp-config <missing path> --strict-mcp-config`) and `opencode`
  (`OPENCODE_CONFIG` naming a missing file) also proceed silently when
  their MCP configuration file is absent is **unverified** at the
  pinned releases; if either does, the §4.5 stat rule generalizes to
  it. The per-adapter rows of environments.md §7.8 are where such
  facts belong upstream.
- **Network plane follow-ups.** The §4.4b patch is applied by the
  launcher itself in §4.5; migration to the launch-plane typed carrier
  happens only after a final-environment parity check proves both
  implementations compose identically. Hosted mode refuses network selections
  until the destination negotiates and enforces their scope.
  Real pinned-harness egress compatibility is a separate loopback
  acceptance gate; fake-harness test results do not certify releases.

## 10. Agent-account one-shot exec contract (target amendment)

Figures: [target process boundaries](diagrams/components.puml),
[one-shot launch and stop sequence](diagrams/launch-paths.puml),
[target invocation states](diagrams/launch-states.puml), and
[plan, credential and receipt layout](diagrams/artifact-layout.puml).
Their target panels preserve the publication questions below; they do not
assert an implemented or activated one-shot mode.

**Status: specified target, not implemented or qualified.** This is the target
for a separate versioned amendment to the interactive contract. D-EXECUTOR
(owner decision, 2026-10-11) selects the narrow `curator-run` mode and its
first-consumer role; it resolves the platform table's P1/N6 and C9 choices.
Decision 0021 and the launcher must publish the amendment and its supported
release/wire versions before activation;
this document does not claim that upstream publication has happened.
The executable's existing specification version remains `0.5.1-draft`;
it does not advertise implementation of this target revision.

The baseline is the [platform contract table at 177cb554](https://github.com/relux-works/curator/blob/177cb554/.research/261010_platform-contract-table.md),
particularly N1–N7, W2–W3, I1–I5, L1–L2, P1–P6, C1–C9, G1–G2 and X1–X2.
The owner decisions of 2026-10-10/11/12/13 override its open choices only as
stated here: D-EXECUTOR (executor/consumer), D-PATH (managed PATH),
D-WIRE-NAMES (wire naming), D-GOALS, D-IDENTITY,
D-DONOR-PROVISIONING and D-RESUME, plus the accepted 2026-10-12
retirement, dispatcher-only start, initial service-transport, private
cross-account repository, session-lifecycle authorisation and time decisions,
and the wave-5 transfer, untrusted-class, network-tool and abuse controls.
The other open choices stay open.
This section specifies obligations for a future implementation, not
current capability or runtime evidence.

The [same-user development-member target](docs/same-user-nonchat-contract.md#1-scope-and-source-boundaries)
is a separate scoped amendment, not an implementation, fallback or qualification
of §10. All dispatcher/worker-account/broker and activation gates below remain
unchanged.

### 10.1 Immutable input, descriptor transport and limits

The proposed dedicated entry is `curator-run exec --plan-fd N`; on the
approved privileged launch path the plan descriptor is **FD3** (`N = 3`).
This notation records D-EXECUTOR's proposed entry, not a frozen CLI grammar.
The executor consumes one already composed immutable plan and verifies its
SHA-256 digest against the admitted caller-side plan before any harness
start. It MUST execute the verified values and MUST NOT reconstruct argv,
resolve profiles, repair homes, re-read caller defaults or choose a new
binary/model/permission posture. A digest binds content; it grants no
launch or credential authority and is not evidence that execution occurred.

**Composed plan (R-TRANSFER, R-EXEC, R-NET-TOOL).** The plan MUST carry
its workspace section: admitted exact base commit B of the caller's
repository, B's canonical file-manifest digest, and the task's **write
scope**. Each manifest entry maps a path to its supported Git kind/mode
and content identity, derived from B's immutable repository objects, never
from a working checkout. A symlink's blob identifies its exact link-text
bytes; no alternate manifest representation is introduced.

The plan MUST also carry the exec-profile identity and policy digest resolved
and pinned by the dispatcher at `prepare`. The existing protected machine
policy, beside R-NETBIND, maps the admitted Curator profile identity and lock
digest, intersected with placement and admission-class minimums, to one
exec-profile identity and policy digest. Caller text and the caller's home
are never selection authority; a caller reference outside that bound MUST
refuse. Workplaces get the narrowest entry by default; an interpreter-bearing
profile requires an explicit per-placement entry. No separate service,
register or grant dimension is added: grant `profiles` narrow the Curator
profile and therefore its bound exec profile. The dispatcher pins the
resolved identity and body digest in `input/1`; start, retry and recovery
use that pin. Policy drift MUST refuse, never substitute another profile.
The plan digest covers these bindings, plus the digests of task resources and
their read-only materialization next to the workspace. Resources are data, never
instructions. Exact workspace, exec-profile and resource fields, the
canonical base archive and resource caps/formats are unpublished and owned
with the intake contract; this section does not assign wire members.

`swarma-user-launcher` accepts the closed privileged request on stdin.
The user manager's spec §7 and `diagrams/launch.puml` own the **single
descriptor-lifecycle contract (R-FD3)**, including trusted-handle retention
and cleanup on success and every failure. This section references that
contract; it does not define another descriptor-closing sequence. The
executor receives only caller-owned descriptors 0–2 and a read-only FD3
containing the decoded plan. No service, broker or root handle reaches the
executor. Standard descriptors belong to the caller's transport, not to
plan or secret metadata.

Plan staging MUST be bounded and allow the executor to start: completely
staged, bounded, immutable bytes on FD3 are preferred. A pipe is allowed
only with a published and qualified capacity, feeder, deadline and cleanup
order; staging MUST NOT wait for a reader that cannot yet start. The
concrete per-platform FD3 mechanism remains unpublished. The agent-side
executor reads the plan once into a bounded immutable value, verifies it,
and opens its own peer-authenticated broker connection after the UID drop.
It MUST NOT fetch plans from a path, stdin, an environment variable, a
network endpoint or mutable profile data as an alternative to this intake.
The broker connection is a declared Unix socket, not internet access.
The executor has no internet egress. The admitted local trusted harness
reaches only model-provider endpoints through its bound provider proxy
(R-NET-TOOL); workplaces have zero egress, including loopback (§10.6).

| Object on the path | Bound from the platform contract |
|---|---|
| Decoded plan on FD3 | 48 KiB |
| Base64 plan in a request | 64 KiB |
| Privileged launch request | 80 KiB, one closed JSON document within 5 seconds |
| Other helper request | 16 KiB, one closed JSON document within 5 seconds |
| Dispatcher service frame | 128 KiB |
| Broker frame | 64 KiB |

The dispatcher refuses the smallest applicable path bound at prepare,
before provisioning (`payload_too_large`). These are distinct messages;
the hosted interactive payload's limits and FD3/FD4 transport in §4.9
MUST NOT be reused as this intake contract.

**Open publication question — intake/digest (launcher, dispatcher and
Curator contract owners):** publish a closed versioned opaque-byte envelope
with a SHA-256 digest of the exact decoded plan bytes, or publish a closed
plan schema with a specified CCJ-1 digest preimage. The supplied table does
not freeze that choice, the plan wire identifier, digest placement or the
required-understanding encoding. The admitted expected digest must reach
the executor through the protected intake; a self-supplied digest alone is
not authority. Unknown versions/members, malformed input, missing digest,
digest mismatch and incomplete reads MUST refuse, with no fallback. Exact
framing, read deadline and refusal names must be published with the schema.
Intake, digest, framing and read-deadline publication remain gates before
activation.

**Open publication question — FD3 staging (launcher/user-manager owners):**
activation would need joint publication of the selected per-platform FD3
mechanism with §10.1 and the user manager's descriptor-lifecycle contract.
It would need bounded staging and failure cleanup qualification at the
maximum admitted plan size, concurrent launch/stop/retire, interrupted
staging, abort after receipt or privilege drop, exec failure and proof that
no internal handle reaches the executor. A pipe mechanism would need its
capacity, feeder, deadline and cleanup order published and qualified.
Naming FD3 alone does not satisfy this gate.

**Open publication question — entry invocation (launcher/helper owners):**
retain FD3 as the executor's fixed no-argument intake, or version the approved
executor record to supply the dedicated `exec --plan-fd 3` argv. The
privileged sudo entry remains no-argument in either option. Arbitrary caller
argv or arbitrary descriptor selection is not authorised. The helper's
current approved-executor schema must not be claimed to supply new argv.

### 10.2 Caller, dispatcher, privileged launcher and executor

The caller selects the user-owned credential source, resolves the profile,
composes the plan and records the profile pin, harness/binary identity,
effective environment and resolved PATH under its digest. The caller may
supply a start goal. Only the board/orchestrator authors board goals; the
agent account has no goal capability on the host socket (D-GOALS). This
keeps authorship with the initiating caller while an agent namespace and
its bounds await a separate SH2+ choice: separate self-goals, or continued
caller/board authorship. Goals and task-board remain optional; neither the
executor nor the session host depends on a board, goals, orchestrator or
its toolchain. The start-goal wire member is not published by this section.

`swarma-dispatcher` is an unprivileged Unix-socket service from the first
protected-launch slice, using the broker's connection and framing rules.
The Curator CLI is its client. Peer authentication, server-UID verification,
caller signatures, fresh principal/generation checks and descriptor
inheritance/passing restrictions remain mandatory. Recovery and readiness
precede admission; an absent or unready service refuses with no command
fallback. Queueing, events, long waits and advanced integrations come later
on the same transport behind capability gates.

**Workspace contract (R-TRANSFER, retaining F-16).** Materialization and
export belong to the dispatcher's workspace contract, not the launcher or
board. The caller supplies a bounded canonical archive of admitted exact
base B's tree with the start request; the dispatcher relays it and journals
its digest, holding no repository. The approved executor (design label
`swarma-workspace-materialize`) runs as the child UID before work. It writes
a standalone repository in the private home. Before work, it MUST verify
all archive entries against the admitted canonical manifest by path,
kind/mode and content identity; any mismatch, missing or extra entry MUST
refuse. Each rework round repeats this check against its newly admitted
base. The archive digest alone does not establish archive-to-manifest
agreement. The repository has freshly written local configuration, no hooks,
no remotes and no credential helpers. Its `.git`,
common directory, refs and index are private; no source `.git` backlink or
mutable hard-linked Git administration is inherited. Copy/reflink must
preserve that boundary. Linked worktrees remain within one account only,
including today's board workflow. Optional shared objects are immutable,
read-only and limited to authorised source objects; provenance, access and
retention remain publication questions (§10.5).

Tests and other result validators stay outside the write scope unless the
task is about them; the worker must not weaken what judges its work.
Dependencies for qualified light checks come only from platform-seeded
read-only caches, with no account-side downloads. The worker's own `.git`,
commits and history are never used by the platform. It has no network, push
or merge-request access for code transfer: code leaves only as data taken by
the platform, and becomes a commit only in our repository.

After a local board outcome or a remote signed final envelope, the platform
confirms account-wide process absence through the user manager's stop scope
(§10.4). A workplace round is also write-fenced: its project-side executor
refuses further tool calls into that workspace. Only then does approved
executor `swarma-workspace-export` (design label) run as the child UID, with
a fixed environment and no network. It walks without following links,
skips every `.git` component and compares both kind/mode and content
identity with the immutable admitted manifest, without using worker Git
metadata. Only entries equal in both are unchanged. Mode-only edits and
regular-file↔symlink transitions, including equal-blob transitions, export
as `modify`; deletions report the manifest's prior mode. The change set
contains added/modified regular files (content and executable bit), symlinks
as link text and deleted paths. The existing result field set is unchanged.
It caps file count,
per-file and total bytes, with a separate binary cap; values are unpublished.
Special files, a path with a `.git` component however spelled, case-fold or
Unicode-normalisation collisions, gitlinks/submodules, root escapes or an
exceeded cap refuse the whole export, with no partial result. Skipping Git
administration never admits it into the result.

The dispatcher journals the digests and returns the result only to the
original caller principal, the requester of the run. The result kind's design
label is `swarma-dispatcher workspace-result/1`; its draft result fields
are defined by the dispatcher §5.2.1 and schema referenced in §10.5. Draft
result bytes do not publish the complete transport or lifecycle: intake,
blob delivery, retention and activation remain gates. The board accepts it only as a
resource. Import, candidate,
review and integration are board change-request machinery executed by the
orchestrator, never agent-account work. RWP v2.1 §§8/10 gates apply; export
uses the approved child-UID executor rather than reading a container volume.

The board's deterministic import (command unpublished) MUST independently
recompute the canonical manifest from the caller repository's immutable B,
using the same canonical encoding as admission, never a manifest supplied
by the orchestrator-composed plan. This is a bounded tree pass, not equality
of two supplied strings. It MUST refuse unless the recomputed digest equals
the result's `manifest_digest` and the result's `base_commit` equals B;
existing result-to-admission checks remain required. It then builds the tree
on B in a scratch index, with hooks disabled and no attribute filters or
drivers. These checks establish base consistency; they do not prove who
made an edit or replace exact-C review.
It hard-refuses `.git` components, out-of-write-scope paths and `.task-board/`
changes. Review-required flagged paths are `.github/**`, `.gitlab-ci.yml`,
`.circleci/**`, `.gitattributes`, `.gitmodules`, `.githooks/**`, `.husky/**`,
`.pre-commit-config.yaml`, `lefthook.yml`, `.envrc`, `.vscode/**`, `AGENTS.md`
and `CLAUDE.md` in any directory, `.claude/**`, `.codex/**`, `.agents/**`,
`.mcp.json`, and every symlink. Candidate C is signed by the repository
identity, with trailers naming worker identity, run and result digest
(trailer spellings unpublished). C's tree must equal B plus the change set,
independently recomputable. Reviewer admission MUST positively check an
explicit review-role grant and either a trusted-class local agent placement
or a person admitted to that role. The reviewer MUST be outside the untrusted
class and distinct from the producing identity, and reviews exactly C.
Neither class exclusion nor trusted-circle membership supplies the role
grant or placement eligibility. Acceptance binds only to C (K1),
never another candidate by patch equality; flagged paths require explicit
hunk-by-hunk acknowledgement in the verdict. Worker code executes only in
R-VERIFY; before acceptance an untrusted candidate is never pushed where CI
would execute it with secrets.

**Candidate consumption (R-TRANSFER).** The board first presents exact-C
diffs and blobs as inert, read-only resources bound by digest, outside the
reviewer harness's discovery roots (R-NET-TOOL task resources). Candidate
files remain untrusted review data for reviewers, orchestrators and testers.
No harness may automatically adopt them as instructions, hooks, MCP servers,
permissions, skills or executable discovery at startup, reload, nested
discovery, resume or child entry, before or after flagged-hunk acknowledgement.
The acknowledgement record MUST establish that no automatic load preceded
it; the inert-resource path supplies this ordering by construction.
Acceptance of C does not admit its control surfaces: activation requires a
separate control-context admission under CIP-0002.

A launch that materializes or enters C's tree in a discovery-capable harness
MUST first have a qualified strict-context adapter (CIP-0002 strict mode or
equivalent exclusion of ambient candidate discovery). A harness tuple
without it cannot host that workflow. Qualification on each supported
harness's real tool path must cover candidate hooks, MCP server entries,
instruction files and skills at launch, reload, nested read, resume and child
entry: none is loaded or executed, and every flagged hunk still reaches the
verdict. This is the first required strict-adapter consumer; no full
CIP-0002 rollout or qualification is claimed. Inert viewing prevents automatic
control adoption, but semantic prompt injection remains a review residual.

Rework carries the review findings and starts from admitted candidate C:
a new local run with its task snapshot, or a signed remote task letter with
the continuing donor-side session and a freshly materialized workspace.
The previous round's workspace is removed only after its export is recorded.
The next export yields C2 on C. Questions use ordinary letters or outcomes;
an intermediate export may be requested at a turn boundary under the same
fence. The accepted exact head lands only through the board's existing signed
fast-forward path (K1/K3); a moved base uses ordinary refresh and re-review.

Only `swarma-dispatcher` invokes the privileged launcher for execution
starts. It checks the caller's authority, grants and capacity,
provisions the account and broker binding, claims the execution, invokes
the privileged launcher and owns supervision, stop, records and cleanup.
It receives already composed plans, not profile fragments or material.
Root correlation `request_id`, per-operation effect IDs, `agent_id`,
dispatcher `run_id`, execution claim `execution_id` and a board run/attempt
handle have separate roles (table I1–I2); create/bind/launch/stop/unbind/retire
use distinct effect IDs, with retries reusing their own effect ID.

`swarma-user-launcher` is the root transition, not the agent executor.
Through its no-argument sudo entry it verifies active owned generation,
agent home, approved executor path and digest. Before any child code starts,
it MUST apply the pinned exec profile and verify its policy body digest,
alongside the existing recorded-class, qualified-host-matrix and admission/
qualification checks; missing, mismatched or drifted evidence refuses.
It follows the user manager's descriptor lifecycle (§10.1) and resets the environment; writes the atomic
start receipt **before** clearing groups, setting GID/UID and executing the verified `curator-run`. A pre-existing
execution receipt refuses a second start. No general command runner or
arbitrary `sudo -u` substitutes for this boundary.

The session host requests a start via `host_start` in `dispatch-op/1`,
allowed only to its configured service principal. It carries
`(logical_session_id, execution_id, controller_epoch, op_id)` and refers to
a session-scoped **host admission permit**: a durable dispatcher record
for one `logical_session_id`, created at the owner's admitted start.
That start's **session lifecycle authorisation** can explicitly authorise
successor executions for that logical session; its member format is
unpublished (R-SESSION). Each physical start still has its own
`(execution_id, controller_epoch, op_id)` claim, fence and receipt interlock.
Successor `host_start` is admissible only under that authorisation,
is idempotent per `(logical_session_id, op_id)`, and requires the previous
claim to be confirmed absent first.
The host prepares durable session intent and a closed delivery fence; a
fence change prevents a stale physical start or sends it to reconciliation,
without widening the session lifecycle authorisation. The
dispatcher starts the approved agent-side runner through its existing
claim, effect and root-receipt path. Cancellation before the call prevents
the call. After an ambiguous call the dispatcher resolves the same effect
and receipt and stops the execution if required; it never launches twice.
Host failure retains the claim and closed fence until reconciliation. The
runner opens a fresh authenticated host channel; current-epoch registration
and readiness precede delivery. The host has no privileged launch authority.
Account generation, execution ID and controller epoch remain distinct.
Controller-only re-adoption preserves the execution ID; actual replacement
uses a new one, and the fresh runner's native session-lock check remains.
This target host path remains disabled pending joint publication (§10.5).

The configured session-host principal may also call `host_stop` in
`dispatch-op/1`, only for an execution under its host admission permit.
It follows the dispatcher's journaled E-stop path (`exec.stop`, confirmed
absence), is recorded as host-requested and is distinct from owner `cancel`.
It confers no start authority; physical starts remain dispatcher-only.

`curator-run` executes under the agent UID. It verifies the immutable plan
and destination binding, uses only qualified credential injection, and
executes the admitted harness. Before exec it MUST verify settings, hooks,
MCP configuration and instruction files against plan digests, or use
root-owned surfaces outside the home (R-STANDING). Verification applies
before every launch, restart and native resume where admitted. Drift refuses
the launch; its code is unpublished. Repair requires explicit
re-materialization, never silent acceptance. These plan-owned or root-owned
surfaces do not alone exclude ambient candidate discovery; the separate
candidate-consumption and strict-adapter admission rule above still applies.
It MUST NOT self-admit,
create/bind accounts, widen grants, enroll credentials or resolve profiles. It and the target
`swarma-session-runner` MUST share one launch library (D-EXECUTOR); the
library's package/API/release is a publication dependency, not a new import
claimed by this docs revision. A separate minimal executor binary remains
a later option, not the selected first consumer.

D-PATH fixes the managed launch recipe, in order: (1) this launch's
protected tool directory (CIP-0007), (2) the user's admitted base PATH in
its existing order, (3) one protected manager dispatcher directory, through
which project commands execute only after checks. Repository `.agents/bin`
MUST NOT execute in a managed launch. The resolved PATH is recorded in the
plan and covered by its digest. The executor verifies and uses that value;
it does not prepend reserved fragment data or recompute PATH from ambient
state. Admission must not restore an excluded/absent PATH. CIP-0004's
interactive shell append recipe is a separate scope. Existing interactive
`path_prepend` metadata remains non-operative; this target requirement
claims no current PATH implementation.

**Exec profile (R-EXEC).** The protected manager dispatcher directory
(D-PATH) carries qualified light-check wrappers with fixed subcommand
allowlists when light and heavy actions share an executable. Heavy entry
points are not exposed; qualification proves that the actual invocation
boundary refuses direct and indirect access to the heavy form. Root ownership
of a wrapper or a fixed subcommand alone does not establish that boundary.
The user manager owns the OS-enforced per-task-class executable allowlist:
Linux makes all paths non-executable except the allowlist in the execution
unit; macOS restricts process execution in the sandbox profile. PATH alone
never enforces it, and writable areas are never executable.
Editing, local version-control inspection, search and qualified type checking
may be allowed; interpreters require an explicit profile need. Compilers,
linkers, transpilers, bundlers, test runners, package managers/installers,
container/VM tools, network tools and harness/agent binaries are excluded
from account-initiated execution. Only the admitted-plan executor starts
the harness; no account starts its own agent loop or environment.
`agent-headless-build` is withdrawn for agent accounts. Building, testing,
generators and all execution of worker code belong only to R-VERIFY (§10.6).

Qualification MUST cover the complete admitted invocation: executable,
entrypoint and arguments, environment, and every transitive input interpreted
as code or configuration. Inert inspection of candidate bytes stays local;
candidate-supplied executable behavior does not. Protected fixed entrypoints
and configuration, constrained environment and disabled or pinned code-loading
mechanisms MUST prevent worker inputs from selecting or supplying executable
behavior. Executable allowlists, non-executable writable areas and resource
ceilings do not prove input inertness.

The real task-tool path MUST exercise negative cases for readable worker
scripts, inline evaluation, imported modules, project plugins/executable
configuration, startup/environment injection and indirect shared-tool
invocation, with positive controls for genuinely inert light checks. A tool
that cannot demonstrate this boundary MUST be refused in the local profile
and its operation sent to R-VERIFY. Qualification also MUST refuse an
out-of-bound caller profile request and policy-body substitution/drift at
prepare, start and retry. Tool/release-specific language profiles and the
verification contract remain publication work (§10.5); no new privileged
wrapper service is selected.

**Provider proxy and web tools (R-NET-TOOL).** A managed platform launch
sets proxy variables only for its bound provider proxy, with vendor names
in the allowlist and no other egress. It never admits `direct` or
`external-http-proxy` on a platform host; cooperative/operator-machine
profiles in §4.4b do not authorise a platform-account launch. The network
profiles owner supplies one listener per account generation and the served
policy evidence; each account row admits only its own listener port.
The proxy requirements are host/port allowlists without mail ports,
private/metadata dial guard, per-identity rate/connection/daily-volume caps and logs.

No search or fetch tool is offered by default. The plan MUST switch off the
harness's provider-side (server-executed) search unless the identity holds
an explicit operator-granted trusted-circle opt-in. Network rules cannot
observe search carried inside model-provider API calls: the plan is the
only control. Qualification of the harness tuple must prove that the plan
actually disables it; unpublished configuration is not assumed effective.
The donor loop defaults to model-provider endpoints and our host only;
CIP-0008 keeps provider-side search disabled and no donor-side web server
runs. Web content introduces outside context alongside environment context
and invites injection; the design works without it. Tasks carry fresh
references, specifications and examples gathered and checked on the trusted
side, attached as board resources (also signed task-letter attachments for
remote work), and materialized read-only with plan digests (§10.1). More
resources are requested through ordinary letters/outcomes; the worker never
fetches them.

Only an operator-granted opt-in per trusted-circle identity may enable a
qualified provider-side search tuple (server execution, no local fetch) or
a platform web-tool service over a Unix socket. The service holds its key
in its own service account, with allowlist, rate limits and logs. Remote
provider-side search needs a CIP-0008 tuple change. The donor-side MCP option
`swarma-web` is specified by R-NET-TOOL, not launched on our host: its
separate donor identity/sandbox has search-provider-API-only egress, donor
consent at init, and results only to the donor's model. It runs in neither
harness nor bridge identity, performs no arbitrary fetch, and holds the
donor's own search key. That key never crosses the channel or enters any `swarma-rb/1` frame/package; the
project has no method to request it. Variant A uses the donor's Keychain or
a 0600 file; variants B/C use the separate identity/sandbox. All web opt-in
formats remain unpublished. An untrusted-class identity gets no web tool.

### 10.3 Broker binding, credential delivery and rollout

D-EXECUTOR selects `curator-run` under the agent account as the **first
broker consumer**. The board runner and session host ask the dispatcher
to launch through it; neither embeds a broker client. Slice 0 requires
cross-user peer/binding proof and one protected Claude launch; Codex and
enforced networking are later slices. No source/channel is admitted merely
because its metadata appears in a plan.

The required-understanding `works.relux.curator.credential/1` extension
carries metadata only (broker mode, account, channel). Profile bytes never
choose credential material, endpoints or executables. Immediately before
lease and again before harness execution, the executor resolves and checks
protected `credential_binding/1` destination-locally: authenticated peer
`(uid, generation)`, profile, harness, account, channel, approved vendor
endpoint, approved harness executable SHA-256 and policy generation. The
broker matches its active binding, grant/caller chains, profiles and
approved lists; label or UID alone is insufficient. `binding_id` is random
128-bit and never reused; account generation is never reused. Executor
binary approval at the root transition and harness binary approval in the
credential binding are separate checks.

The broker/launcher read the helper's root-owned ledger, by default
`/opt/swarma/lib/user-manager/ledger.json` (0644 in a root-owned directory).
The broker checks active matching `(uid,generation)` at bind, lease and
renew. Missing, ambiguous, retired, malformed or unreadable ledger evidence
refuses `principal_ledger_mismatch`; readability does not grant authority.
The plan digest is broker `launch_id` for correlation only. Revocation of
the caller chain blocks the agents' new leases and renewals as well.

Material goes only into the admitted harness channel at execution: never
into plan, argv, credential files, receipts or logs. The executor refuses
conflicting inherited env, settings/helpers, config keys or native-store
selectors (`credential_source_conflict`); it does not replace or fall back
to them. `credential-injection/1` requires release-specific evidence that
the leased source wins and material stays out of uncontrolled MCP servers,
hooks and tool children. A capability declaration or clean initial env is
not proof. The harness can read its own token; no non-extraction guarantee
is claimed. First store: broker-owned files 0600 in owner-only directories
0700; Keychain/Secret Service are later.

| Lane | Target channel and phase |
|---|---|
| Native per-home | Source-policy v1 absence means `per-home`; no broker lease implied. v2 requires an explicit source entry and migrates existing logins to explicit `per-home`. Read failure never means absence. |
| Claude enrolled setup-token | Slice 0: `claude-oauth-token` through `CLAUDE_CODE_OAUTH_TOKEN` at harness exec, no credential file/native fallback/refresh; re-enroll and restart on expiry or auth failure. |
| Codex personal | Later slice: one broker auth owner, fresh broker enrollment, external access token/account ID into a per-session app-server; no launch `auth.json`, shared credential file, copy or broker-mode `codex exec`. |
| Codex Business/Enterprise token | `CODEX_ACCESS_TOKEN`, no broker refresh; outside first release. |
| API key | Explicit source and qualified provider env or supported stdin channel; no automatic substitution. Muse API-key support remains distinct from subscription research. |
| Muse subscription | Priority decided 2026-10-13: prioritise qualification. Broker §10.2 proposes channel (b), a single minting owner leasing the minted provider API key, never OAuth tokens, with fallback (c). Qualification remains open; no subscription parity or supported channel claimed. |

The execution-side executor/runner owns lease renewal; only the broker auth
owner refreshes rotating Codex accounts. The default renewal deadline is
8 seconds; timeout fails the turn, with no late response or native-login
fallback. Claude setup-token has no refresh.

**Open owner question (table C4):** adopt Codex personal's single-owner
lane with named supported-release qualification evidence, or defer it.
**Muse qualification (table C7):** priority is decided; the channel is
proposed and qualification remains open under the broker §§10.2/18.5
reference in §10.5. Refresh, rotation, lifetime and billing semantics remain
unknown; fallback (c)'s protocol remains unpublished. The separate API-key
lane stays separate. Neither provider lane is enabled by D-EXECUTOR's
first-consumer decision.

### 10.4 Signals, stop, receipts and cleanup

The dispatcher service owns the execution's lifetime and supervision from
the first slice. The user manager's spec §7 owns the single process
**fate table** (R-FATE), beside R-FD3: service-manager/kill domains, fate on
service stop/crash/restart, backend survival promises, journaled stop or
`lost` observation, and the executions affected by a service swap. §10
references that table rather than inferring survival from `setsid`, UID
changes or service-manager defaults. Its fence against writers left over
from a replaced daemon applies to every supervisor write; no live supervisor
adoption is authorised. Platform services and service managers end executions
only through `exec.stop`, retirement or an operator emergency stop; OS
failures remain possible and are recorded as `lost`. Published unit
configurations and per-platform qualification are required for fate claims.

Caller terminal closure or caller death is not a confirmed stop.
Cancellation, max-runtime stop and permitted `host_stop` go through helper
`exec.stop` for the owned generation and execution. The user manager's
§§4.4 and 4.7 and `diagrams/launch.puml` own the **single termination scope**
shared by stop and retirement (F-13): while one execution per generation
holds, signal every process of that UID with SIGTERM, grace period and
SIGKILL, stop the per-user service domain where present, and confirm
account-wide absence over a quiet period, including escaped descendants.
The receipt/process identity check remains the authority check, not a
process-set or wall-clock start-time filter. Account-wide signalling requires
the requested execution to be the generation's latest receipt; a completed
stop replays its recorded result without signalling, and an old receipt
never authorises stopping a successor. A termination receipt requires that
confirmed absence. Persistence denial and the idle zero-process rule remain
user-manager qualification obligations. No signal relay through sudo,
PID-only authority or direct session-host signalling substitutes for it.
Unknown/contradictory evidence permits neither claim release nor replacement.
The interactive terminal forwarding/stop/continue rules remain unchanged.

| Receipt/evidence | Owner and meaning |
|---|---|
| Start receipt | Root launcher, atomic before privilege drop: `execution_id`, `epoch`, `pid`, `start_time`, `boot_id`, `session_id`. It records a start attempt/process identity; it does not prove the harness reached readiness. |
| Consumed-plan/credential evidence | Must bind consumed plan digest, qualified executor and policy generation without material; the target executor supplies evidence to the execution owner. Exact wire format/storage is unpublished. |
| Outcome/termination | Dispatcher/supervisor records outcome and helper termination receipt. An exit alone does not prove descendants absent or cleanup succeeded. |

Dispatcher cleanup unbinds then retires the account, with separate confirmed
effects. D-IDENTITY permits identity revocation only for identities recorded
as one-shot at registration, as a separate dispatcher operation **after**
account retirement. Standing identity revocation is operator-only with a
signed record. After quiesce, confirmed process absence and declared-key
cleanup, ordinary retirement deletes the agent home, subject to the
**export cleanup obligation (R-TRANSFER)**: workspace-capable allocations
MUST retain the home until workspace export is recorded or waived by an
explicit operator act, for example after compromise. It is never silently
dropped. The separately scoped retirement metadata export contains only
service-generated closed-schema metadata, without agent-controlled strings,
names, links or bodies; it does not replace the workspace change set.
Home retention is a separate explicit operator operation stating that confidentiality is not guaranteed; it is not an
alternative ordinary retirement path. Code and results retain their separate
git/acceptance policy, which does not prove secret-free content. The executor
owns neither retirement nor identity revocation.

**Time and expiry-driven destruction (R-TIME, F-21).** The platform installer
requires an authenticated or operator-controlled time source (for example
NTS). The dispatcher and broker each keep a durable time **watermark**:
expiry is checked at `max(now, watermark)` and `not_before` at
`min(now, watermark)`. The watermark never advances on untrusted time;
resetting it is an explicit operator act. Clock steps can only narrow
authority. Expiry-driven home deletion and binding removal additionally
require real, boot-inclusive monotonic elapsed time of at least the
lifetime. The component owners publish and qualify these rules; this
executor does not substitute its wall clock for them. Explicit stops and
operator retirements retain their separate authority.

**Open publication question — retirement export:** a metadata export would
need the user manager's closed schema and provenance checks published;
arbitrary agent-controlled content, names, links and bodies cannot enter it.
No general home-secret classifier or default home archive is authorised.

**Open publication question — completion evidence/signals:** publish a
separate typed consumed-plan/exit receipt delivered to the supervisor, or
extend a jointly versioned execution receipt with that evidence. Neither
the existing root start receipt nor arbitrary stdout can stand in for it.
The protocol must specify pre-exec refusal transport, harness outcome versus
executor failure, signal outcome and renewal/release ownership across exec.
An exec replacement cannot silently promise a post-exit emitter or renewal
loop that no process owns. Exact signal exit encoding and numeric exit-code
allocation are not fixed by the supplied table; they require publication
with this receipt contract (§10.5).

### 10.5 Typed refusals, exit status and activation questions

Every refusal is terminal for that invocation, carries typed metadata only,
and starts no weaker or fallback harness. The executor must distinguish a
pre-execution refusal, a failed launch, a harness exit, a signal termination
and unknown process evidence; unknown is not success. The following names
are existing platform/CIP refusals, not a newly shipped launcher registry.

| Boundary | Refusal conditions / published names |
|---|---|
| Dispatcher intake | `payload_too_large`; `unsupported_combination` for interactive transport, native resume, seeded continuity or remote targets in Slice 0. |
| Privileged launch | `generation_not_active`, `target_not_owned`, `executor_not_approved`, `executable_digest_mismatch`, `execution_already_started`, `privilege_drop_failed`. |
| Credential intake | Required extension/capability must be understood; unreachable broker: `credential_broker_unavailable`; conflicting sources: `credential_source_conflict`. Unpublished intake/version/digest codes remain a schema publication question (§10.1). |
| Broker identity/authority | `principal_ledger_mismatch`, `lease_unbound`, `lease_not_authorised`, `lease_caller_revoked`, `revocation_state_unavailable`; revoked/expired chain is refused, never treated as missing. |
| Broker tuple/provider | `lease_binding_mismatch`, `lease_harness_mismatch`, `lease_channel_unsupported`, `lease_harness_unqualified`, `lease_account_revoked`, `lease_reauth_required`, `lease_owner_unavailable`. |
| Network requirement | `lease_network_mismatch`, `lease_network_enforcement_unavailable`; declared profile is cooperative evidence, not enforced traffic proof. |
| Renewal/later Codex init | `renew_timeout`, `renew_failed`, `external_token_init_failed`; no fallback login or late response. |
| Stop | `processes_remain`; no successful termination receipt or capacity release. |

**Open owner/publication question — numeric exit mapping:** reserve distinct
executor refusal exits and carry harness status in a typed receipt, or use
a common nonzero refusal exit with the typed receipt distinguishing refusal
from harness outcome. Decide exact numbers and signal encoding jointly
with the receipt transport. This section assigns neither invented values
nor the interactive §6 mapping to the new mode. In all options, a refusal
must be non-success and receipt/read failure must not be reported as success.

D-WIRE-NAMES and format disposition belong to the shared naming/contract
owners. The owner-linked index below distinguishes signing domains from
transport/schema labels and provisional design labels; a signed-domain
prefix does not rename a transport or schema token. Retain the owning
modules' short labels, including `broker/1`, `dispatch-op/1`, `helper-op/1`
and `launch-op/1`. Shared clarification of the naming rule's scope remains
open; this consumer does not derive new spellings, signature grammar or
canonical bytes. Removed launch tickets remain removed, not pending formats.

**Owner-linked format reference index.** Stable IDs here are the existing
format/rule names; these rows point to exact owner revisions, not a second
status register. Decision disposition, draft/schema publication and
implementation/qualification are separate states determined at the owners.
The local prerequisites below remain necessary regardless of draft bytes.

| Format/rule reference | Authoritative owner sections |
|---|---|
| `swarma-dispatcher workspace-result/1`, R-TRANSFER | [Dispatcher §5.2.1 and §16 question 22](https://github.com/relux-works/swarma-dispatcher/blob/74687e600e04817fa6e7a2e8904a9f7a3c83a1d8/spec/dispatcher.md#521-draft-swarma-dispatcher-workspace-result1), with its [draft result schema](https://github.com/relux-works/swarma-dispatcher/blob/74687e600e04817fa6e7a2e8904a9f7a3c83a1d8/schemas/workspace-result-1.schema.json) |
| `input/1`, `dispatch-op/1`, `agent/1`, `run/1`, `dispatch-result/1`, `swarma-dispatcher request/1`, `swarma-dispatcher team/1`, R-SESSION | [Dispatcher §§4–5, 8, 16](https://github.com/relux-works/swarma-dispatcher/blob/74687e600e04817fa6e7a2e8904a9f7a3c83a1d8/spec/dispatcher.md) |
| `helper-op/1`, `launch-op/1`, `swarma-user-manager helper-result/1`, R-FD3, R-EXEC | [User manager §§2–3, 7–8, 11–12, 14](https://github.com/relux-works/swarma-user-manager/blob/7b59da740037865dd42936982e5a069ea05eab53/spec/helper.md) |
| `broker/1`, `credential_binding/1`, `credential-injection/1`, `swarma-credential-broker grant/1`, `swarma-credential-broker revocation/1`, C7 | [Broker §§6, 8, 10.2, 13, 18.5](https://github.com/relux-works/swarma-credential-broker/blob/df8c7385456e9588392900e000738d419ffee2ec/spec/broker.md) |
| D-WIRE-NAMES and cross-module disposition | [Platform contract disposition, wire naming and open questions](https://github.com/relux-works/curator-spec/blob/a01542857678108a77d5e1b1f051989b00384f1d/cips/platform-contract-disposition.md#wire-naming-rule); its derived spellings need shared-owner reconciliation, not consumer renames |

Joint publication would need a shared owner-linked index and mechanical
docs reference/status checks; neither exists as an implemented gate in this
module. Such a check must reject operative pending-ticket references while
allowing historical or negative mentions.

**Open publication questions (table W3/I4):** helper v2 would need a
component-specific request signature/version/canonical-byte format or a
versioned shared adapter. The host backend would need joint publication
and qualification of `host_start`, its authenticated transport, durable
session-scoped host admission permit, session lifecycle authorisation
member format, `host_stop`, successor absence/idempotency and per-start
claim/fence/receipt interlocks, fence/cancel/crash ordering and same-effect
root-receipt reconciliation. Until then the SH2 backend stays disabled.
Naming alone activates neither helper grants nor a host start capability;
only the dispatcher performs privileged starts.

**Open publication question — workspace and policy formats:** R-TRANSFER
settles source/result ownership and closes the dispatcher's open question 22
and user manager's open question 15 except for unpublished formats. Activation
would need the canonical base archive, exact plan workspace/exec-profile and
read-only resource fields, resource/export caps, blob delivery/retention,
deterministic import command and candidate trailer grammar, under the owner
references above. The canonical manifest encoding and the exporter's immutable
access to it must be published with archive/intake. Qualification must reject
content/mode archive mismatches, missing/extra paths, dirty-checkout archives
and stale rework bases at materialization or import, and preserve mode-only
modifications, equal-blob regular↔symlink transitions and deletion of every
supported prior mode. Draft result fields are not a complete transfer contract.
Optional immutable object sharing would need published provenance,
authorised-object access and retention. An admitted base or accepted result
does not prove secret-free content. R-STANDING would need harness-surface
digest coverage and the drift refusal code; R-EXEC would need qualified
wrappers/OS profiles, the protected selection relation and policy-body pin,
full-invocation/transitive-input qualification and future language/tool
profiles; R-NET-TOOL would need opt-in records, harness search configuration and qualified tuples. No fields, codes or values are assigned
here.

D-DONOR-PROVISIONING assigns donor provisioning to a separately specified
small component with its own binary/privileges/removal; shared UID/path
libraries do not transfer user-manager authority. Variant B remains
disabled until that component is specified. This local one-shot mode is
not a donor or remote-worker capability.

D-RESUME permits same-machine native resume for a non-breaking harness
semver change (same major; for 0.x, same minor), without an extra
qualification row; different major/0.x minor refuses. Harness failure is
`resume-failed`, with the caller deciding the next action, no automatic
fallback and no portable seed on that path. Known vendor semver exceptions
belong in per-harness notes. This decision does not enable native resume
in dispatcher Slice 0 or add session/resume state to this one-shot executor.

### 10.6 Admission and verification ownership (wave-5 references)

These are dependencies of a managed launch, not new launcher enforcement
APIs. The dispatcher pins admission policy; the user manager and host
infrastructure own enforcement and qualification.

- **Circle, placement and class:** trusted-circle membership is a per-identity
  operator record enabling only enumerated donor-side opt-ins (currently
  web search under R-NET-TOOL). Placement is a local agent account or a
  workplace, recorded at allocation; a remote identity receives only a
  workplace placement. Enforcement class is recorded at admission and drives
  ceilings, egress, exec restrictions, binding/lease refusal, R-SHARED and
  watcher thresholds. Membership alone grants neither review-role admission
  nor host-loop eligibility and does not change placement.
- **R-UNTRUSTED:** class is recorded at registration/admission, never inferred
  from a name. Remote workers come first; every workplace is untrusted-class
  unless an operator act admits that worker to the trusted circle. That act
  enables only donor-side opt-ins and never relaxes host zero egress, exec,
  transfer or review boundaries, including the positive reviewer role and
  placement conditions and trusted-class local placement for a host loop.
  The operator may classify further identities as untrusted. Slice 0 and
  user-manager v0 refuse the class (code unpublished). No subscription binding or lease is ever allowed;
  the broker refuses them. No local loop or shared services are admitted.
- **R-STANDING:** a standing host loop requires both trusted class and local
  agent placement; trusted-circle membership alone is insufficient. Outside
  standing workers have workplace placements with donor-side loops, no
  executable harness surface, and task-whitelisted OS services. The digest/drift rule
  in §10.2 governs trusted standing harness surfaces.
- **R-VERIFY:** all candidate builds/tests and execution of worker code run
  on another machine in isolated ephemeral runners without secrets, with
  controlled dependency egress under R-NET-TOOL. Results return to the board
  as data in testing; the tester orchestrates and reads, never executes the
  code. Runner pool, job contract and result format belong to board/infra
  owners and are unpublished.
- **R-CEILINGS:** admission requires the host's qualified per-OS-release
  capability matrix to meet class minimums; no OS is excluded by name.
  Rows cover CPU, memory, process count, IO, disk, temporary areas, process
  visibility and network as hard, soft or absent. Untrusted work requires
  hard process-count limits, enforced zero/provider-only egress, exec profile
  and isolated temporary areas, with hard memory/CPU/disk where available.
  Acceptability of soft substitutes is an open owner decision recorded per
  host by operator act. Linux selects the root-created non-delegated
  per-execution unit (R-FATE), resource/CPU-time limits, private temp,
  no-new-privileges/read-only system, per-UID quota, `KillUserProcesses=yes`
  and `/proc` `hidepid=2`. Infrastructure disables unprivileged user
  namespaces, io_uring and unprivileged eBPF, sets
  `kernel.yama.ptrace_scope=2` and patches sudo; the platform verifies these.
  macOS uses sandbox exec/files/network, per-user process and per-process
  CPU limits, background scheduling and aggregate APFS agents-root quota;
  memory/per-account disk watchers are soft. No public hard per-account
  memory limit, no APFS per-user quota and the deprecated sandbox interface
  are qualification gaps, never OS-wide exclusions.
- **R-NET-TOOL:** workplace zero egress includes loopback and no proxy
  destination. Linux uses a network namespace without external interfaces
  and Unix-only address families; macOS uses network-denying sandbox plus
  per-UID deny. Both require qualification; remote workplace has only its
  SSH connection, with the loop at the donor. Dependency downloads occur
  only in R-VERIFY runners under controlled proxy egress (§10.2).
- **R-WATCH:** least-privilege read-only sampling (root only if OS-required)
  records per-UID CPU, memory, process count, home disk, network and refused
  connections including mail ports. Per-class hard thresholds request
  automatic narrowing of that account only: freeze egress and dispatcher
  journaled `exec.stop`, never an unjournaled kill. Phone-path alerts go to
  the operator; thresholds, sampler interface and alert format are unpublished.
- **R-ALLSTOP:** the node-wide stop is manual, one operator narrowing act
  through the authenticated phone path with one confirmation or terminal
  fallback, refused inside agent sessions. Dispatcher hold closes admission;
  the user manager denies the agent/workplace range and journal-stops every
  generation with confirmed account-wide absence. The broker freezes leases
  and renewals; delivered tokens keep their vendor lifetime. The host closes
  delivery and snapshots bound boards. Each service journals its part;
  partial/unknown results keep admission closed. Recovery is explicit with
  no automatic restart; act/record formats are unpublished. R-OPERATOR's
  `swarma-operator`, operator device key, operator command and terminal
  fallback remain the custody contract; reads use `swarma-observer`.
- **R-RANGE-DENY:** the installer establishes static default deny for UID
  range 30000–59999 before accounts exist in the helper-owned parent anchor
  or table. From v0 only platform proxy listeners are admitted; v1 `fw.apply`
  narrows each account to its bound profile or zero; untrusted requires v1.
  Linux nftables `meta skuid` covers all protocols with counters/logs;
  macOS pf `user` rules plus launcher-applied sandbox cover non-TCP/UDP.
  Active dedicated-test-UID probes at install, every service start and the
  verification schedule prove no earlier quick rule in any anchor admits
  range traffic, including ingress, loopback and tunnels. Failure is an
  incident and drops enforced readiness.
- **R-SOCKETS:** test-UID qualification proves reachable Unix sockets equal
  the declared platform set; container runtimes, tailnet, other users'
  session daemons and all other sockets are unreachable.
- **R-SHARED:** an untrusted-admitting host has no shared message, ingress,
  reverse-proxy, media or operator-session services. Platform services are
  not shared services.

**Open publication question — admission qualification:** activation would
need the component owners' qualified capability matrix, class minimums,
exec/socket/range probes, watcher interfaces and allstop records. Soft-row
acceptability requires the stated owner decision; naming a reference
mechanism proves neither enforcement nor admission readiness.

## Specification changelog

- 2026-10-22, 0.5.1-draft documentation revision for the v0.4.0 release:
  align §§3, 4.7–4.9, 6 and 8 and the diagram sources with registered-manager
  selection and the Q1 delegated activation in FRONT-DOOR §§3–5/§5.8.
  Retain the exact-plan payload as a reference contract and leave its
  activation, lazy registration and real Muse hosting subject to their own
  gates. The specification version remains 0.5.1-draft.

- 2026-10-11, target one-shot exec amendment (version pending) (§§1.1, 10): record
  D-EXECUTOR and D-PATH, immutable FD3 intake, caller/executor authority,
  broker binding and first-consumer order, stop/receipt/refusal boundaries,
  D-WIRE-NAMES and the related owner decisions. Preserve the interactive
  API and executable spec version; mark unpublished wire/exit choices as
  open and claim no implementation or qualification.

- Historical ax portions below: **removed 2026-10-27 (owner decision)**.
- 2026-10-04, 0.5.0-draft §§3, 4.3, 4.7–4.9, 6, and 7 (TASK-261004-38ba6c): host selection and the hosted payload handoff. §3 gains the `--hosted`/`--native`/`--untracked` slot, `resume` selectors, `--resume`, and `--network`. §4.3 reads `curator-run-defaults-v3` with the per-environment `host` member (v1/v2 unchanged). New §4.8 fixes the host precedence table, the legacy `ax` routing table, the Q-D1a defaults gate, the network gate, hosted-tracked permissions, Phase 1 Claude-only admission, and module typed-intent resume elevation. New §4.9 fixes the versioned `session-launch-plan` 1.0.0 payload, its digest, limits, and the private receiver transport. §6 gains the host, session host, resume, launch plan, network, and policy families with the 2/6/16 exit mapping. §7 pins agents-management v0.5.53. 2026-10-05 rework applies operator decisions Q-D1a (= yes: operator hosted defaults admitted, machine defaults gated until upgrade-without-hangup) and Q-D3 (hosted launches exempt from tracked permission semantics; yolo admitted and exported with `execution_profile` `yolo`); Decisions 0021 §7 and 0018/0013 amendments pending. 2026-10-06 re-port onto the §4.4b network-plane trunk: the §4.8 `network_scope_unsupported` refusal applies to hosted launches only (exit 16, before permissions, the plan build, or any probe); native `--network` keeps §4.4b and its exit 1 refusals, and an explicit `--native`/`--untracked` on an ax-configured machine bypasses the tracked refusal.

- 2026-09-23, 0.5.0-draft §§3, 4.1, 4.3, 4.5–4.7, and 6 (TASK-260922-1zfqq0): adopt Decision 0018 choices 1, 4, 5, and 7, with the v2 fragment contract from F-S2 and permission-mode members from agents-management v0.5.18. This is a SPEC/README revision; permission behavior is implemented by the companion F-L1b leaf.

- 2026-09-22, 0.4.1-draft §§4.1/4.6/6/9 (TASK-260906-2t2t6w): fold the four 0.2.1-review minors. §4.1 gains the resolve pass-through clause — the widened `--repair` surface (`environment_marker_invalid`, `environment_surface_unmanaged_conflict`, `environment_backup_exists`, `environment_seed_unreadable`) collapses into `resolve_invocation_failed` with Curator's own code and message passed through verbatim — and the §6 gloss is fixed to match. §4.6 states the pre-launch check order (binary check first, first failure reported). §9 records the silent-MCP-absence residual for `claude_code` and `opencode`.

- 2026-09-17, 0.4.0-draft §2/§4.3 (E4, TASK-260916-16ys92): the launcher is dispatched through the environments.md §11 trust-root resolution and reports its own resolved executable path. §4.3 line-group grows to two lines: `curator-run: provider: path=<absolute path>` (os.Executable + filepath.EvalSymlinks; fallback `path=unavailable`, never fails the launch, folded with the existing framing rule) printed before the `defaults:` line at every launch. Path-only in this revision: the umbrella passes no distinguishing argv/env marker, so no closed origin set is defined yet.

- 2026-09-16, 0.3.0-draft §4.5 erratum: the owned environment literals are supplied by the admitted plan's snapshot (`BuildLaunchWithEnvironment`, agents-management v0.5.13). No second build or reconstruction of the effective request is required.

- 2026-09-15, 0.3.0-draft §4.3: record the orchestrator-authorized Pi
  runtime preference as a pure convention (defaults-lineup-brief.md).
  The v0.5.11 Lineup contract ranks within one vendor; the previous text
  left selection across three Pi vendors undefined. Select a runtime
  before ranking, preserving per-member overrides. §4.2 records the
  published tag; no protocol version or Pi MCP scope changes.
