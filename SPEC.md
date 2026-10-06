# Curator Agent Launcher — Specification

**Specification version:** `0.5.0-draft`
**Status:** in-repository draft (see [Versioning](#8-versioning))

The launcher is the **execution plane** of the four-plane composition fixed
by curator-spec Decision 0010: Curator owns what the agent reads (context),
`agents-management` owns who runs (spawn), `ax` owns what happens to a
running session (session), and the launcher composes the three and execs.
Every contract between the planes is a CLI invocation plus a closed object,
except the launcher's one declared module edge: it consumes
`agents-management` as a Go module. This document is the launcher's own
contract: its CLI surface, its composition algorithm, its `ax` handoff, its
system-prompt opt-in and warnings, its diagnostics, and its versioning.

Key words MUST, MUST NOT, SHOULD, and MAY are to be interpreted as in
RFC 2119. Normative references:

- `curator-spec/decisions/0010-agent-environment-profiles.md`, Decisions 6
  and 10 — the boundary this component lives inside.
- `curator-spec/decisions/0013-execution-ownership-and-launch-plans.md`
  — the ownership model (Decision 1), the `ax start --launch-plan`
  operation and its document (Decisions 3.2, 3.6, 4), the interactive
  plan mode (Decision 5), what this revision must say (Decision 6), and
  the Session Record extension keys (Decision 7). "Decision 0013 D*n*"
  below names its Decision *n*.
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
- `agent-session-manager-spec/SPEC.md` (`ax`) — §2.1 (session-name
  grammar), §2.4 (execution profiles), §5.1 (Launch Plan), §7.1 (built-in
  provider ids), §14.1 (`ax start`), §15.1 (Structured Error), as revised
  by pull request #1 under Decision 0013 D7.
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
admitted model, tracked by `ax` when the integration is configured.
Everything after `--` belongs to the tool, untouched.

Non-goals, each a boundary rather than an omission:

- **No session state.** Fire is the launcher's verb; manage is `ax`'s.
  The launcher holds no leases, no checkpoints, no resume records, and no
  memory of past launches. Session affinity (a session created under
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
- **No Curator or `ax` imports.** Curator and `ax` are CLI contracts:
  `curator env resolve` is a subprocess, the `ax` handoff is an exec.
  The only Go module the launcher consumes is `agents-management`
  (planned: `github.com/relux-works/skill-agents-management`; not yet
  imported by the stub). No shared libraries, no other import edges.
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
            [--name <session-name>] [--ax-profile <standard|yolo>]
            [--network <profile>]
            [--] <native args...>
curator-run --help | -h
curator-run --version
```

The flag set is deliberately minimal; every flag below names the single
plane it feeds, and a need that fits none of them is a specification
change, not a flag addition.

| Element | Plane | Meaning |
|---|---|---|
| `<env-id>` | context + spawn + session | Required operand. A registered Curator environment identifier (`claude_code`, `codex_cli`, `opencode`, `pi`), with `claude` and `codex` accepted as aliases of `claude_code` and `codex_cli`. The operand normalizes to the canonical id before any validation or lookup; outputs, diagnostics, fragments, configuration records, and locks carry the canonical id only, and an alias is never persisted. Selects the fragment to resolve and, through the closed mapping of §4.2, both the agentic system to plan and the `ax` provider id to hand off to. |
| `--profile <name>` | context | Forwarded verbatim to `curator env resolve` as its `--profile` operand. Absent, resolution uses the current profile for the applicable scope. |
| `--system-prompt <append\|replace>` | execution | Explicit opt-in that engages the fragment's system-prompt channel with the given semantics. The value is required: the opt-in states what it wants, and the launcher never chooses replacement by default. See §5. |
| `--model <model>` | spawn | Level 1 of the §4.3 default precedence: passed through to the spawn plane's plan request as declared. The launcher does not validate model names; admission is the spawn plane's verdict. |
| `--effort <effort>` | spawn | Level 1 of the §4.3 default precedence: passed through as declared. Effort is per-model and the spawn plane injects no default; when no §4.3 level yields one for a model that requires it, the plane's refusal names the model, the accepted vocabulary, and the recommendation, and the launcher completes that error with its own flag spelling, `--effort`. |
| `--permissions <native\|yolo>` | spawn | Permission mode resolved by §4.3. The launcher passes the selected mode to `LaunchRequest.PermissionMode` for `LaunchModeInteractive`; agents-management owns the mapping. |
| `--yolo` | spawn | Exact alias of `--permissions yolo`, shipped in the same increment and occupying the same precedence level. Supplying both forms is a repeated permission request and a `usage` error. |
| `-d`, `--danger` | — | Rejected before `--` as `usage`; neither spelling is an alias. After `--`, arguments remain opaque native arguments under the normal boundary rule. |
| `--hosted`, `--native`, `--untracked` | session | The one host request slot (§4.8). `--hosted` runs on the managed session host; `--native` and its exact synonym `--untracked` run natively. A repeat or a combination is a `usage` error; permissions stay independent of the spelling. |
| `resume [SES-HANDLE]`, `--resume <id>` | session | Hosted-only resume selectors (§4.8). `resume` selects the latest session, `resume SES-HANDLE` a handle, `--resume <id>` a provider identity. Handle and identity shape belong to the spawn plane's typed-intent grammar; in native mode any selector is a `usage` error. |
| `--name <session-name>` | session | Tracked mode only (§4.6): the `ax` session name, replacing the default `<env-id>-<utc-stamp>`. MUST satisfy the `ax` §2.1 grammar `[A-Za-z0-9][A-Za-z0-9._-]{0,63}`; a longer or ill-formed value is a `usage` error naming `--name`, never a silent truncation. On an untracked machine the flag is accepted and has no effect. |
| `--ax-profile <standard\|yolo>` | session | Tracked mode only (§4.6): the `ax` execution profile, forwarded as `ax start --profile <value>`. Absent, no `--profile` is passed and `ax`'s own default applies. This flag is the **only** way `--profile yolo` reaches `ax` from a launcher-mediated launch (Decision 0013 D6.4); the launcher never derives it from the fragment, the plan, or the native arguments. On an untracked machine the flag is a `usage` error: an execution profile is `ax`'s concept, and a value that would be silently discarded is a value the operator was misled about. |
| `--network <profile>` | network | Explicit network-profile selection (§4.4b): a profile identifier resolved after plan admission against the operator catalog. Stored verbatim at parse; resolution and validation belong to §4.4b. On a tracked machine the selection is admitted at parse and refused as `network_scope_unsupported` in §4.4b: the source host cannot validate a destination. |
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
  `--system-prompt` and `--ax-profile` accept only their closed
  vocabularies; `--name` is validated against the `ax` §2.1 grammar at
  parse time, before anything is resolved. `--network` takes any
  non-empty value at parse — a repeated or missing value is a usage
  error like every other value flag — and profile validation belongs
  to §4.4b.
  `--ax-profile` is also a
  `usage` error with an explicit `--native` or `--untracked` flag, which
  bypasses `ax` and would discard the profile.
- Usage errors exit 2 and print usage; they launch nothing and resolve
  nothing.

## 4. Composition algorithm

A launch composes in seven ordered steps: obtain the fragment, map the
environment, resolve model and effort, obtain the plan, resolve the
network selection (§4.4b), compose, hand off or exec. Every step either
completes or fails the launch with a §6 diagnostic; there is no partial
launch, and no step's failure degrades into a weaker launch shape. The
order is contract, not convenience: the fragment comes **first** because
the plan request needs the managed home (§4.1, §4.4), the plan comes
before the network step because only an admitted launch may probe, and
composition comes last because it appends to values it never rebuilds
(§4.5). `curator-run` is the single composer in both modes (Decision
0013 D1): a tracked and an untracked launch differ only in who creates
the process. Host selection (§4.8) runs after configuration validation
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
  code line — streamed before it, the resolve transport's form of the
  pass-through `ax_handoff_failed` gives `ax`'s Structured Error after
  its line. The launcher's line names only the family; the operator
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

The spawn plane speaks agentic-system plugin ids, `ax` speaks provider
ids, and Curator speaks environment ids. The launcher owns the closed
mapping between the three:

| Curator env-id | agents-management system | `ax` provider id (ax §7.1) |
|---|---|---|
| `claude_code` | `claude-code` | `claude` |
| `codex_cli` | `codex` | `codex` |
| `pi` | `pi-native` | `pi` |
| `muse` | `muse` | `muse` |
| `opencode` | none in this revision — `env_unsupported` | none — `env_unsupported` |

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
this revision pins v0.5.48. §4.3 registers the real native-Pi plugin.

An env-id outside this table that Curator nevertheless resolves is
`env_unsupported`: the launcher refuses rather than guessing a system or
a provider. The table grows by specification revision, never by
inference, and a row is launchable only when both non-Curator columns
are filled — the launcher does not exec untracked what it could not hand
off tracked.

The Muse row follows the binding 2026-10-01 task decision. Mapping does
not attest launch-mode support. The pinned agents-management v0.5.48
plugin declares interactive mode, implements ToolReleaseProber, and supplies
release-pinned native/yolo permissions for Muse 1.4.1 and 1.4.2. The launcher
probes the installed release before permission mapping and refuses unlisted
releases. Interactive plans preserve inherited HOME and overlay only the four
fragment XDG parents. Its declaration-owned
model rows are the module's system-only launch authority, without a
resolved vendor; defaults consumes those rows as BuildLaunch does.

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
   configuration directory: `$XDG_CONFIG_HOME/curator-run/` (default
   `~/.config/curator-run/`) for the operator, and `/etc/curator-run/`
   for the machine. Schema, closed — readers MUST reject an unknown
   member:

   ```json
   {
     "schema": "curator-run-defaults-v3",
     "locked": false,
     "defaults": {
       "claude_code": { "model": "claude-opus-5", "effort": "high", "host": "native" },
       "codex_cli":   { "model": "gpt-5.3-codex", "permissions": "yolo" }
     }
   }
   ```

   `defaults` keys are env-ids of the §4.2 table (an unknown key is
   `defaults_config_invalid`); each value is an object of at most four
   members: `model`, `effort`, `permissions`, and `host`. The first two
   are strings passed through unvalidated to admission; `permissions`,
   when present, is exactly `native` or `yolo` and supplies the
   launcher-global mode level; `host`, when present, is exactly `native`
   or `hosted` and supplies the §4.8 default. At least one member must be
   present. A v3 reader accepts v1 and v2 files unchanged; a v1 file
   carrying `permissions`, or a v1 or v2 file carrying `host`, refuses
   under its closed schema.
   **Lockable:** when the machine file carries `"locked": true`, the
   operator file is ignored for every env-id the machine file names, and
   a flag for a member the machine entry sets is a `usage` error naming
   the locked member, even if the requested value matches. Otherwise the
   operator file overrides the machine file per member: an operator entry
   that sets only `model` leaves a machine `effort` for the same env-id in
   force, and permission mode follows the same per-member merge. The host
   member resolves separately under §4.8 and never joins the model/effort
   pair. A missing file is a
   legitimate absence; a read or parse failure is `defaults_config_invalid`
   and the next level MUST NOT be used. The file and its sibling `ax.json`
   (§4.6) are the **launcher's configuration file family**, delimited
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
   `source=default-interactive`, for untracked silence on every stdio
   shape. Operator decision Q-D3 (2026-10-05), literal: headless
   detection (terminal, CI markers, native-argument form) never
   changes the permission default, for native and hosted launches
   alike;
5. the built-in tracked-silence default, `native` with
   `source=default-headless`, for tracked native silence only. This
   term is not headless detection: tracked native launches cannot use
   `yolo` at all, so the default cannot be `yolo` there without
   refusing every unconfigured tracked launch. Hosted launches are
   exempt from tracked permission semantics, so hosted silence is
   `yolo`.

The fragment's `source=global` means Curator's force-native lock and
carries `mode=native`; it is not the launcher-global file level. In the
stderr provenance line below, `source=global` instead names the winning
launcher-global `defaults.json` level.
`source=default` means the profile level is silent; its `mode=native`
is a placeholder and the launcher falls through. Explicit `native` means
no launcher override and does not promise a prompting posture. In
untracked native mode the launcher does not inspect policy selectors in
the native suffix; arguments after `--` are forwarded verbatim and may
request their own native policy by design. The typed interface is not a
security boundary. `yolo` requests the agents-management-declared native
permission mode within the tool's own policy. The launcher never derives
this mode from model,
prompt, credentials, environment values, or tracking configuration.
After resolving a mode, every untracked launch prints this provenance line before admission:

```text
curator-run: permissions=<native|yolo> source=<flag|profile|global|default-interactive|default-headless> mapped=<flag or none>
```

The `mapped` value is supplied by agents-management; this SPEC names no
provider flag. Headless detection (terminal, CI markers) is observed but
never consulted by the built-in default since Q-D3; native-argument
non-interactive classification is gone entirely. Detection modifies no
native arguments and claims nothing about their safety. See §4.6 for
the closed headless marker set, the lock rule, and tracked refusals.

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
the entry point verified at `agents-management` tag `v0.5.48`.
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
- `Env`: `os.Environ()` in **both** tracked and untracked modes, supplied
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
mode. In agents-management v0.5.48, `LaunchRequest.ToolRelease` and
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
v0.2.1 (§7). Without `--network` this step is absent: no catalog is
read, nothing probes, and the launch composes exactly as before —
with one authorized exception: the agents-management v0.5.45+
module appends its unconditional `--disallowedTools=AskUserQuestion`
denial to the direct Claude argv (direct and tracked shapes) and to
the §4.4 admission argv pin. Everything else in an unmanaged launch
is byte-identical to baseline fbcdbaf0; the committed
`TestUnmanagedGoldensMatchBaselineFbcdbaf0` pins that parity by
comparing the shipped goldens against baseline copies and allowing
exactly that one added line. With `--network`, the order is closed:

1. A blank selection refuses as `network_profile_invalid` without any
   I/O.
2. A tracked selection refuses as `network_scope_unsupported` without
   catalog or probe I/O: the source host cannot validate the
   destination, which does not enforce the network scope in this
   revision (§4.6, §9). There is no fallback to direct execution and
   no silent loss of tracking.
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
4. The launch shape is verified against the STRICT network policy:
   an explicit allowlist of verified (adapter, harness, build,
   entrypoint) tuples, matched exactly on all four members at the
   ACTUAL admitted entrypoint — the EFFECTIVE shape, not the
   construction mode alone — and never collapsed. The build is the
   probed tool release, compared verbatim — no prefix, range, or
   non-emptiness rule. The list holds exactly one verified tuple:
   `(generic-env-v1, claude-code, 2.1.287, exec)` — the one-shot
   `claude -p` shape, the only shape the pinned verification covers.
   The launcher constructs interactive launches, and the module
   forwards the native tail verbatim into the interactive argv — so
   an explicit print invocation (the tail carries `-p`/`--print` in
   flag position, read as the plan carries it) is that verified
   entrypoint and stays admitted. Every other tuple — codex, muse,
   any other build, any launch without a print selection — refuses
   as `network_scope_unsupported`, as does an unknown harness, an
   unknown or unparsable version, a missing build, or an unmapped
   launch mode, before any probe or workload. A managed launch
   without a print selection refuses until pinned compatibility
   evidence covers interactive mode; the launcher MUST NOT silently
   switch the interactive product behavior to print mode to satisfy
   this gate. An optimistic policy is a future option
   (TASK-261005-yoogtw), not the default.
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
`Plan.Env`: doing so re-admits plugin-removed names. Untracked execution
uses the complete `Plan.Env` base, preserving plugin removals (including
run-context keys with zero `Run`) and sanitized `PATH`.

The plan's **own names and values** are those returned by
`System.ChildEnv(nil, req)` for the same system and `LaunchRequest`, over
an empty parent. They MUST NOT be obtained by diffing `Plan.Env` against
the inherited environment, or by a second `BuildPlan` with `Env=nil`:
binary resolution requires `PATH` and refuses that request. These owned
values define tracked literals and the warning below; `Plan.Env` itself
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

**env_names** — the fragment's `mcp.env_names` union (already bounded,
before it reaches the launcher, by the reserved-name exclusion and the
lockable passable-names allowlist of Decision 0012 D6), **minus** every
name that also appears in the composed `env_literals` of §4.6 — the
plan's own names plus fragment/channel names. This is the
**literal-versus-lookup collision rule** (Decision 0013 D6.3 as amended by review finding F5): a literal the composer set is
an explicit intent and wins over a destination-local lookup, so the name
is dropped from `env_names` and a warning naming the variable is printed
on stderr. The reserved-name exclusion keeps registry adapter names out of
`env_names`, but a system plugin's own names are not bounded by it, so
this rule is what makes the composed document disjoint by construction:
`ax` §5.1 disjointness never fires for a composed document, and a
collision is never an `ax_handoff_failed`. Untracked mode has no
`env_names` lookup — the child receives the composed environment based
on `Plan.Env`, and the union is informative only — but the warning is
printed in both modes so that the two modes report the same facts. A name merely inherited in
`Plan.Env`, such as an allowed `FIGMA_API_KEY`, is not a literal collision
and MUST remain in tracked `env_names` for the destination-local lookup.
Warnings name variables, never their values.

**stdin** — the interactive plan's `Stdin` under the Decision 0013 D4
mapping: `null` when unattached; otherwise `{ "encoding", "bytes" }` with
`utf-8` when the bytes are valid UTF-8, else `base64url`; an attached
empty stream is present with zero bytes, not `null`. In this revision it
is `null` for every launchable environment (§4.4).

### 4.6 Hand off and exec

**With the `ax` integration configured** on the machine, the launcher
ALWAYS routes the composed launch through `ax` so the session is tracked
from birth. Whether the integration is configured is one fact read from
the launcher's configuration directories of §4.3 level 2: a sibling file
`ax.json` with the closed schema `{ "schema": "curator-run-ax-v1",
"enabled": <boolean> }` (readers MUST reject an unknown member); the
machine file decides when it exists, otherwise the operator file, and an
absent file in both places means not configured. A present file whose
`enabled` is `false` also means **not configured**: the launch is
untracked, exactly as if no file existed, and the only difference is that
the operator wrote the answer down. The precedence is deliberately the
inverse of `defaults.json`'s operator-over-machine: whether sessions on
this machine are tracked is machine policy, so a machine file that exists
always wins, without a `locked` member — the operator file is consulted
only where the machine is silent. Model and effort defaults are a
per-operator preference, so there the operator wins unless the machine
locks. The file is read once per invocation before argument handling
completes, so the §3 usage rules for `--ax-profile` and `--name` can name
the fact; a file that exists but cannot be read or parsed is
`defaults_config_invalid`, never "not configured", and because that read
precedes argument validation it is the diagnostic that fires when the
command line would also have been a usage error — both are terminal, and
the configuration fault is the one the operator cannot see from the
command line. A configured integration is not a per-launch option: there
is no `--no-ax` flag, and bypassing tracking is a configuration change,
not a flag. The launcher composes the Decision 0013 D3.2 request document
and invokes, as a subprocess:

```text
ax start <name> --provider <id> --launch-plan - [--profile <ax-profile>] --workspace <cwd>
```

writing the document to `ax`'s standard input (`--launch-plan -` reads
the plan document, never the child's stdin). The operands and flags:

- `<name>`: `--name` when given, else `<env-id>-<utc-stamp>` with the
  canonical `<env-id>` of §3 and the stamp `YYYYMMDDTHHMMSSZ` in UTC at
  composition time (for example `codex_cli-20260905T073254Z`). The
  default always fits the `ax` §2.1 grammar: the longest §4.2 env-id is
  11 characters and the stamp is 16.
  The profile name is deliberately not part of it — it travels in the
  `profile-name` extension, and an ordinary profile name would push the
  session name past 64 characters. Same-second collisions are Decision
  0013 open question 1 and are `ax`'s to report.
- `--provider <id>`: the §4.2 provider-id column.
- `--profile <ax-profile>`: present exactly when `--ax-profile` was given
  (§3), with its value; absent otherwise, so `ax`'s default profile
  applies. The composed document never carries a permission-bypass or
  unrestricted-mode flag — the interactive plan forbids it (Decision 0013
  D5) and the launcher spells none — and `ax`'s plugin refuses one that
  arrives through the native arguments (Decision 0013 D3.6).
- `--workspace <cwd>`: the launcher's current working directory, the same
  value as the plan's `WorkDir`.

The document, `schema` `urn:ax:schema:launch-plan-request`,
`schema_version` `1.0.0`, members from the composed plan of §4.5:

| Member | Derivation |
|---|---|
| `argv_suffix` | the **entire** composed `Argv`, verbatim. The interactive plan's `Argv` already excludes `Binary`; retain every argument, including the first plugin argument. The composer never sends `Binary` and never uses the `argv` form; `ax` resolves its executable. |
| `env_names` | as composed, with the §4.5 collision rule already applied — disjoint from `env_literals` before `ax` sees the document. Sorted, unique. |
| `env_literals` | the **composer's own names only**: the plan's own names and values (`System.ChildEnv(nil, req)`) ⊕ fragment `env` ⊕ the engaged variable-kind channel (§4.5). Never serialize `Plan.Env` or copy inherited `HOME`, `PATH`, or secrets. The inherited layer of a tracked launch is whatever `ax`'s terminal backend gives the child on the destination. |
| `stdin` | as composed (§4.5). |
| `extensions` | the four base `works.relux.curator.*` keys below, plus the conditionally present launcher-SPEC-owned `works.relux.curator.effective-native-policy` key. |

The extension keys, set by the composer and copied verbatim by `ax` into
the Session Record's top-level `extensions` (Decision 0013 D6.4, D7):

| Key | Derivation |
|---|---|
| `works.relux.curator.profile-name` | the fragment's `profile.name` |
| `works.relux.curator.profile-pin` | the fragment's `profile.lock_sha256`, spelled `sha256:<64 lowercase hex>` — the profile's effective pin under Decision 0012 D3 |
| `works.relux.curator.fragment-digest` | the §4.1 digest: `sha256:` over the CCJ-1 canonical bytes of the parsed fragment object |
| `works.relux.curator.system-modules` | boolean, `true` exactly when the fragment carries a `system_prompt` section — environments.md §10.2 makes that presence equivalent to "the resolved chain carries at least one applicable system module" |
| `works.relux.curator.effective-native-policy` | Conditional: present only for a tracked `native` request when inspection finds known relaxations. Value: `{"relaxations":["<selector>",...],"source":"<settings-source>"}`; selectors are sorted and unique, and the source is the detector's stable identifier. It records only detected facts. |

These are what resume fidelity rests on: `ax` re-resolves the profile on
resume and compares the pin, and refuses by default when
`system-modules` is `true` and the pin drifted (Decision 0013 D7 item 5).

A handoff that fails is `ax_handoff_failed`: an `ax` that cannot be
started, a non-zero `ax start` — including a refused document
(`launch_plan_invalid`, `capability_unavailable`, `invalid_arguments`,
`secret_policy_violation`) — is reported with `ax`'s Structured Error (ax
§15.1) passed through verbatim after the launcher's own code line. The
launcher MUST NOT fall back to an untracked direct exec, because a machine
configured for tracking has declared that untracked sessions are the
failure mode, not the fallback. After a successful handoff the launcher's
work is over: process creation, the terminal, and the session are `ax`'s.

A tracked launch with `--network` never reaches handoff: §4.4b refuses
it as `network_scope_unsupported` before composition, so no document
carries a network selection and no Record extension exists in this
revision (§9).

**Without the integration**, the launcher execs the composed plan
directly: the plan's `Binary` with the entire composed argument tail
(§4.5; prepend `Binary` only for an exec API requiring executable element
0), the composed environment, and the composed stdin — the same
plan, with no `ax` residue. Untracked is the honest answer on such a
machine.

In both shapes the plan's binary missing from the filesystem or `PATH`
is `exec_provider_missing`, reported with the exact executable name and
installation guidance; in tracked mode the launcher checks this before
the handoff so that `ax` is never asked to record a session for a binary
that is not there. The three pre-launch checks — this binary check, the
§4.5 codex layer stat, and the §5.1 file-kind probe — run in this order,
immediately before the handoff or exec, in both modes, and the first
failure is the one reported, so that `ax` is never asked to record a
session the launcher already knows is wrong.

**Permission headless detector and refusal rules (Decision 0018).** A
launch is headless when stdin or stdout is not a TTY, when a
non-interactive marker from the closed set
{`CI`, `GITHUB_ACTIONS`} is present, or whenever tracking is enabled.
This marker set is versioned here in SPEC `0.5.0-draft` §4.6 and mirrored
in environments §10.1; additions require a later paired specification
revision and the list changes only in that revision. Since operator
decision Q-D3 (2026-10-05) the signal is observed but never consulted
by the permission default; the former native-argument
non-interactive classification is removed.

Untracked silence resolves to `yolo` with
`source=default-interactive` on every stdio shape (operator decision
Q-D3, 2026-10-05, literal: headless signals never change the
default); tracked native silence resolves to `native` with
`source=default-headless` (§4.3 item 5). Explicit flag, profile, or
launcher-global values still participate in §4.3 precedence. The
force-native lock is above that whole ladder: if the v2 fragment says
`permissions.locked=true`, any launcher-visible `yolo` from a flag or
launcher-global setting is a `usage` error; silence resolves to `native`.
The fragment's closed lattice makes a profile-level `yolo` with this lock
invalid.

With established v2 permission transport, a tracked native launch whose
effective mode is `yolo` from any precedence level fails with
`permission_mode_tracked_unsupported` and exit 1. It never retries as an
untracked launch. Hosted launches are exempt from tracked permission
semantics (operator decision Q-D3, 2026-10-05; Decisions 0018/0013
amendments pending): the §4.3 ladder applies, silence defaults as on
the native path, and `yolo` is admitted and exported (see §4.8, §4.9).
If permission transport is not established, any launch
that would resolve `yolo` fails with `permission_policy_unsupported`
instead; tracked native silence still proceeds as native, while
untracked silence on a legacy fragment without transport now refuses
(the unconfigured default is `yolo` everywhere). A `yolo` mode for an
environment with no declared mapping fails with
`permission_mode_unsupported` and exit 1.

When a native request's best-effort inspection finds known stored
settings that relax native posture, emit this exact stderr line:

```text
curator-run: effective-native-policy: relaxation=<selector[,selector...]> source=<settings-source>
```

Selectors are sorted and unique; the line names only inspected selectors
and their settings source. In tracked mode, record the same values under
`works.relux.curator.effective-native-policy` as an object with
`relaxations` and `source`. In untracked mode the line is stderr
provenance only and creates no persistent record.

### 4.7 The launcher's configuration file family

The launcher owns exactly two configuration files, both in the §4.3
level-2 directories (`$XDG_CONFIG_HOME/curator-run/`, default
`~/.config/curator-run/`, for the operator; `/etc/curator-run/` for the
machine), each with its own closed schema:

| File | Owns | Precedence |
|---|---|---|
| `defaults.json` (`curator-run-defaults-v3`, §4.3) | model and effort defaults, the launcher-global permission default, and the host default per env-id; the `locked` rule | operator over machine per member, unless the machine file is locked |
| `ax.json` (`curator-run-ax-v1`, §4.6) | whether the `ax` integration is configured | machine over operator; `enabled: false` is not configured |

Before either file is parsed or used, the reader MUST open it without
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
launcher section, no model default, no effort default, and no `ax`
switch. Conversely the launcher reads no §12.1 knob. Every knob of that
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

After normalization and configuration validation, and before permissions
and the plan build, the launcher resolves the host: native execution, or
the managed session host of §4.9. The host flag (§3) is the explicit
level; the v3 `host` member (§4.3) is the configured level. Precedence,
closed, first match wins:

| First match | Result |
|---|---|
| Machine locked and names the environment | Ignore the entire operator entry; a flag with a machine host present refuses `usage`, even when equal; otherwise flag, then machine host, then native |
| Flag present | Flag |
| Operator host present | Operator |
| Machine host present | Machine |
| Otherwise | Native |

The machine lock keeps its §4.3 named-environment scope. No flag and no
configured default means native with zero receiver lookup and zero daemon
probe, including through legacy `ax` routing. There is no silent
fallback in either direction: a hosted launch that cannot reach its
receiver refuses, and a native launch never contacts one.

The legacy `ax` table applies next, before the defaults gate:

| Effective `ax.json` | Host selection | Routing |
|---|---|---|
| Absent or disabled | Any | The host table above |
| Enabled | Implicit native (absent flag and default) | Existing `ax` behavior |
| Enabled | Configured native | Existing `ax` behavior; a native default does not imply explicit bypass |
| Enabled | Explicit `--native` or `--untracked` | The existing untracked native path; `--ax-profile` would be discarded and is a `usage` error |
| Enabled | Hosted flag or default | `host_configuration_conflict`, exit 2, before the defaults gate |

Effective `ax` authority stays machine-first (§4.6, §4.7).

The defaults gate follows: a resolved machine hosted default refuses
`session_host_default_not_ready` (exit 16) until
upgrade-without-hangup (Decision 0021 section 7 amendment pending). An
operator hosted default is the operator's own opt-in and passes the
gate (operator decision Q-D1a, 2026-10-05, = yes), as does an explicit
`--hosted` flag. Next, a
hosted launch with a `--network` selection refuses
`network_scope_unsupported` (exit 16, the hosted policy mapping) before
permissions, the plan build, or any probe: the receiver cannot enforce a
source-side network scope. A native launch with `--network` is the §4.4b
path and keeps its own exit 1 refusals; only the hosted refusal exits 16,
so the exit status of this one code depends on the host. An explicit
`--native` or `--untracked` bypasses `ax`, so on an ax-configured machine
it also bypasses the tracked refusal of §4.4b and the launch is the
untracked direct path. Resume selectors in a
native launch refuse `usage`: handles are hosted-only and the native
tail stays the provider's own grammar.

Hosted launches run tracked but are exempt from tracked permission
semantics (operator decision Q-D3, 2026-10-05; Decisions 0018/0013
amendments pending): the §4.3 ladder resolves flag, then profile,
then the launcher-global file value, then the built-in default, and
the built-in default is `yolo` with `source=default-interactive` on
every stdio shape, exactly as on the native path — headless signals
(non-terminal stdio, CI markers) never change it. `yolo` from any
level is admitted and builds the same plan once for export. Phase 1
admits `claude_code`
only; any other mapped environment
refuses `session_host_provider_unsupported` (exit 6) before the plan
build. Resume elevation runs the module typed-intent API over the
wrapper selectors and the verbatim native tail; conflicting, ambiguous,
or malformed selectors refuse `session_resume_invalid` (exit 2), and
the plan builds from the selector-free tail. The launcher never parses
provider flags itself.

### 4.9 Hosted payload and receiver handoff

A hosted launch builds the same plan once and serializes the composed
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
hosted-diagnostics registry (135 Appendix V-ERR members, recognized
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
| usage | `usage` | unknown flag, missing `<env-id>`, stray operand before `--`, repeated flag, invalid permission value, repeated or combined permission forms, rejected `-d`/`--danger`, invalid `--system-prompt` or `--ax-profile` value, `--name` outside the `ax` §2.1 grammar or over 64 characters, `--ax-profile` on an untracked machine, `--ax-profile` with an explicit native host flag, a repeated or combined host request, resume selectors in a native launch, a flag overriding a member set by a locked machine `defaults.json`, or visible `yolo` under an established force-native lock — exit 2, nothing resolved, nothing launched |
| resolve | `resolve_invocation_failed`, `resolve_environment_unknown`, `resolve_profile_unknown`, `resolve_repair_failed`, `resolve_lock_unavailable`, `resolve_fragment_invalid` | §4.1: the context plane could not produce a usable fragment — `curator` not startable, or a non-zero exit with an unmapped diagnostic, Curator's own code and message passed through verbatim; unregistered environment; uninstalled profile; the store cannot restore the stale home; the repair could not take Curator's mutation lock within its bounded wait; or the output is not a valid closed fragment |
| defaults | `defaults_config_invalid`, `defaults_unresolvable` | §4.3, §4.6, and §4.7: a launcher-owned configuration file — `defaults.json` or `ax.json` — is symlinked, non-regular, owned by a different identity than its configuration directory, group/world-writable on POSIX, grants a non-owner/non-owner-alias/non-system/non-administrators Windows DACL identity write/delete/security-control access, or cannot be read or parsed, or names an unknown env-id or member; this includes a v1 file carrying `permissions`, or a v1 or v2 file carrying `host` — a read failure is never absence; the lineup admits no model for the mapped system after earlier model levels are silent |
| plan | `plan_refused`, `plan_provider_limited` | §4.4: the spawn plane refused the request (unknown system/runtime or model, model not driven by the system, mode not declared by the system, invalid or missing required effort, unresolved vendor, an unverified release-specific permission mapping, or failure to produce a provider-limits verdict), or the explicit provider-limits verdict was not serviceable (`AvailabilityHealthy`) — the verdict's structure and evidence are surfaced verbatim |
| environment | `env_unsupported` | §4.2: the environment has no spawn-plane or `ax` provider mapping in this revision |
| permission | `permission_policy_unsupported`, `permission_mode_tracked_unsupported`, `permission_mode_unsupported` | §4.1/§4.6: the fragment cannot establish v2 permission and lock transport for a would-be `yolo`; or a tracked native launch resolves `yolo` from any level (hosted launches are exempt, Q-D3); or the environment has no declared `yolo` mapping — exit 1, terminal refusal with no fallback to untracked execution |
| exec | `exec_provider_missing` | §4.6: the plan's binary does not exist — reported with the executable name and installation guidance |
| ax | `ax_handoff_failed` | §4.6: the configured `ax` could not take the launch — `ax` not startable, or `ax start` exited non-zero, its Structured Error passed through; no untracked fallback |
| mcp | `mcp_layer_missing`, `mcp_layer_unreadable` | §4.5: the composed argv carries `-p curator-mcp` and the pre-launch stat of the fragment's `mcp.path` finds no file, or finds something it cannot read as a regular file — codex would silently launch without the profile's MCP set, so neither degrades to a launch without `-p`; the two are distinct facts and are reported as such |
| system prompt | `sysprompt_channel_unavailable`, `sysprompt_file_unreadable` | §5.2: opt-in given but the fragment carries no non-`file` channel with the requested semantics; §5.1: a registry-declared file-kind channel's file exists but cannot be read (or is not a regular file) at the pre-exec probe — an absent managed-home file is not this diagnostic and says nothing about other native sources |
| network | `network_profile_unknown`, `network_profile_denied`, `network_scope_unsupported`, `network_configuration_conflict`, `network_proxy_unreachable`, `network_proxy_auth_failed`, `network_profile_invalid`, `network_file_unreadable` | §4.4b: the explicit network selection could not be honored — an unknown profile name; a denied one (outside the host allowed set, or unconfirmed at the current digest); an unsupported launch shape (tracked mode, an unverified harness/build/entrypoint tuple — including any launch without a print selection, since the pinned verification covers the one-shot `claude -p` shape only — an unmapped launch mode, or an enforced assurance request); a proxy-family name in a composed overlay or an uncovered engine host under a proxy profile; an unreachable proxy (TCP, CONNECT, or TLS step, or a malformed endpoint; a `kind = "direct"` profile skips every probe step); a proxy demanding authentication; a malformed catalog or profile; or a catalog or ledger that cannot be located, read, or parsed — every one terminal, with no fallback to an unmanaged launch; a hosted launch with `--network` refuses `network_scope_unsupported` with exit 16 (§4.8) while every native network refusal exits 1 |
| host | `host_configuration_conflict` | §4.8: a hosted flag or default with the `ax` integration enabled — exit 2, before the defaults gate, with no fallback to either side |
| session host | `session_host_missing`, `session_host_unavailable`, `session_host_provider_unsupported`, `session_host_protocol_unsupported`, `session_host_scope_unsupported`, `session_host_execution_profile_unsupported`, `session_host_terminal_required`, `session_host_stdin_unsupported`, `session_host_default_not_ready` | §4.8, §4.9: the receiver is not on PATH or cannot start (exit 1); the status channel is corrupt (exit 1); a non-Claude environment seeks the Phase 1 host (exit 6); the receiver reports an unsupported protocol, scope, or execution profile (exit 6, receiver-mapped); no controlling terminal is available or the descriptor fails validation, checked before the plan build with zero lookup (exit 6); stdin is attached (exit 6); a machine hosted default awaits upgrade-without-hangup while an operator hosted default is admitted (Q-D1a = yes, exit 16) |
| resume | `session_resume_invalid` | §4.8: conflicting, ambiguous, or malformed resume selectors from the wrapper or the native tail, as elevated by the module typed-intent grammar — exit 2, before the plan build |
| launch plan | `launch_plan_invalid` | §4.9: the composed plan cannot be projected into the closed payload shape — non-absolute paths, literal/env mismatches, missing lookup names, an inadmissible guard, a restart mismatch, or a wire-limit violation; no value in the detail — exit 2, before contact |
| policy | `secret_policy_violation`, `policy_refused` | §4.9: the receiver reports a secret-policy violation or a refused policy — exit 16, receiver-mapped, with field and reason only |

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
`github.com/relux-works/skill-agents-management v0.5.53`, which carries
`LaunchModeInteractive`, `LaunchRequest.PermissionMode`,
`LaunchRequest.ToolRelease`, `LaunchRequest.NativeArgs`, the
`permission-grammar-v1` token, `ErrPermissionModeUnverifiedRelease`,
the `ElevateResumeIntent` typed-intent API, the closed `claude-restart`
template export and transformation check, the pinned hosted schemas,
`Plan.ExportSeal`, and the typed `Plan.Session` (native name and RC
intent with argv indices).
The module owns the release-bound permission mapping, grammar, and
capability table; this SPEC deliberately contains no provider permission
flag spelling. The pinned module's plans-as-values contract, argv
parity goldens, frozen admitted-pair digests, per-model effort rules,
and provider-limit verdicts remain module-owned. `vendorplugin.Lineup`
and the runtime compatibility registry supply §4.3 model/effort fallback.

The launcher pins `github.com/relux-works/curator-network-profiles`
v0.2.1 (§4.4b), consumed by tag as a normal require — no replace, no
workspace. The module owns catalog loading, selection precedence and
validation, the generic environment patch (unset-only for `kind =
"direct"`), the bounded preflight, and the binding Record; this SPEC
owns where the launcher applies them and which launch shapes it
supports.

## 8. Versioning

- This specification is versioned semantically; the current version is
  **`0.5.0-draft`**. Draft versions may change incompatibly between
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

- **Tracked destination environment filtering.** Decision 0013 D3.2 has
  no destination environment-unset or `PATH`-transform member. Plugin
  strips (nesting markers, runtime/token-pointer names, run-context keys)
  and `PATH` sanitization in `Plan.Env` survive untracked execution but
  cannot be transported by the tracked literals-only document. A
  destination supplying `CLAUDECODE=1` can still cause Claude's nested
  session refusal. Whether `ax` independently filters these values is
  unknown here. This is a recorded limit, not a new `ax` field, schema,
  implementation, or permission bypass (Decision 0013 open question 6).
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
- **Implementability.** The behavioral contract of this revision is
  implementable only once two upstream changes land: the
  `ax start --launch-plan` operation (Decision 0013 D3, carried by the
  open revision of `agent-session-manager-spec` pull request #1, which
  the `ax` maintainer decides and this project never merges) for §4.6
  tracked mode, and the `agents-management` release carrying
  `LaunchModeInteractive` (Decision 0013 D5) for §4.4 in both modes
  (Decision 0013 Consequences; §7). Until then the stub stays a stub, and
  nothing downstream may pin this draft.
- The §4.6 `ax` invocation shape is fixed by Decision 0013 D6.4 and
  restated here; the document schema, `launch_plan_invalid`, the
  `caller_launch_plan` capability, and the `ax` version that carries them
  (proposed v0.6.0) are the PR #1 revision's, and a change the `ax`
  maintainer makes there is a change here.
- **Per-tool argv boundary.** The plugin-owned §4.5 order and the final native
  suffix are verified against each pinned tool release before the conformance
  vectors freeze: context and model/effort flags precede the native suffix,
  and the first native argument is where the tool's own parsing begins
  (environments.md §7.3 discipline). The codex_cli `-p curator-mcp` layer
  against an operator `-p` after `--` is no longer open: environments.md
  §7.8 verified on codex 0.153.2 that `-p` takes exactly one value, and
  §4.5 records the consequence (Decision 0012 open question 3 closed).
- **Native tail on resume.** An operator's `-- resume --last` is a
  one-shot turn; `ax` replays the recorded `argv_suffix` on resume, which
  may double-resume. Whether the composer marks the native tail as
  non-replayable is Decision 0013 open question 5 and belongs to this
  revision's review together with its question 2 (suffix elements
  colliding with the plugin's base flags).
- The §4.2 mapping for `opencode` awaits an `agents-management` system
  plugin; until then the environment resolves but does not launch. Its
  `ax` provider id is filled in the same revision.
- The `gemini` variable-class channel (`GEMINI_SYSTEM_MD`) engages when
  the `gemini` adapter lands in the environments protocol.
- The codex_cli configuration-override spelling and both flag-class
  spellings verify against pinned tool releases before conformance
  vectors freeze (environments.md §7.3 discipline).
- The §4.7 file family — `defaults.json` with its location and lock
  rule, and `ax.json` with its machine-over-operator precedence — is this
  document's own drafting choice, not a fact recorded from any tool or
  from environments.md §12.1; if Curator's machine configuration grows a
  launcher section, both files move there by specification revision and
  their schemas stay.
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
  implementations compose identically. Tracked mode keeps refusing
  `network_scope_unsupported` until the destination negotiates and
  enforces the network scope (no Record extension exists before then).
  Real pinned-harness egress compatibility is a separate loopback
  acceptance gate; fake-harness test results do not certify releases.

## Specification changelog

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
