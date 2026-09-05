# Curator Agent Launcher — Specification

**Specification version:** `0.2.0-draft`
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
- `curator-spec/protocol/environments.md` — §10 (`env resolve` and
  `launch-env-fragment-v1`), §7.1 (adapter home variables), §7.3
  (system-prompt channels), §5.5 (system-prompt output), §11 (umbrella
  subcommand discovery). §10 is cited as it will read in the revision 1.1
  rewrite that carries Decision 0012 D8; where this document names a
  fragment member revision 1 does not yet spell (`lock_sha256`, `mcp`),
  Decision 0012 D8 is the authority until that rewrite lands.
- `curator-spec/protocol/registry.md` §1 — CCJ-1 canonicalization, used
  for the fragment digest.
- `agent-session-manager-spec/SPEC.md` (`ax`) — §2.1 (session-name
  grammar), §2.4 (execution profiles), §5.1 (Launch Plan), §7.1 (built-in
  provider ids), §14.1 (`ax start`), §15.1 (Structured Error), as revised
  by pull request #1 under Decision 0013 D7.
- The `agents-management` module documentation — the `BuildPlan` /
  `BuildLaunch` value contract, `LaunchModeInteractive`, the
  `vendorplugin.Lineup` ranking, and the provider-limits verdict model.

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
  asks for a fragment; resolution itself repairs a stale home.
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

## 3. CLI surface

```text
curator-run <env-id> [--profile <name>] [--system-prompt <append|replace>]
            [--model <model>] [--effort <effort>]
            [--name <session-name>] [--ax-profile <standard|yolo>]
            [--] <native args...>
curator-run --help | -h
curator-run --version
```

The flag set is deliberately minimal; every flag below names the single
plane it feeds, and a need that fits none of them is a specification
change, not a flag addition.

| Element | Plane | Meaning |
|---|---|---|
| `<env-id>` | context + spawn + session | Required operand. A registered Curator environment identifier (`claude_code`, `codex_cli`, `opencode`, `pi`). Selects the fragment to resolve and, through the closed mapping of §4.2, both the agentic system to plan and the `ax` provider id to hand off to. |
| `--profile <name>` | context | Forwarded verbatim to `curator env resolve` as its `--profile` operand. Absent, resolution uses the current profile for the applicable scope. |
| `--system-prompt <append\|replace>` | execution | Explicit opt-in that engages the fragment's system-prompt channel with the given semantics. The value is required: the opt-in states what it wants, and the launcher never chooses replacement by default. See §5. |
| `--model <model>` | spawn | Level 1 of the §4.3 default precedence: passed through to the spawn plane's plan request as declared. The launcher does not validate model names; admission is the spawn plane's verdict. |
| `--effort <effort>` | spawn | Level 1 of the §4.3 default precedence: passed through as declared. Effort is per-model and the spawn plane injects no default; when no §4.3 level yields one for a model that requires it, the plane's refusal names the model, the accepted vocabulary, and the recommendation, and the launcher completes that error with its own flag spelling, `--effort`. |
| `--name <session-name>` | session | Tracked mode only (§4.6): the `ax` session name, replacing the default `<env-id>-<utc-stamp>`. MUST satisfy the `ax` §2.1 grammar `[A-Za-z0-9][A-Za-z0-9._-]{0,63}`; a longer or ill-formed value is a `usage` error naming `--name`, never a silent truncation. On an untracked machine the flag is accepted and has no effect. |
| `--ax-profile <standard\|yolo>` | session | Tracked mode only (§4.6): the `ax` execution profile, forwarded as `ax start --profile <value>`. Absent, no `--profile` is passed and `ax`'s own default applies. This flag is the **only** way `--profile yolo` reaches `ax` from a launcher-mediated launch (Decision 0013 D6.4); the launcher never derives it from the fragment, the plan, or the native arguments. On an untracked machine the flag is a `usage` error: an execution profile is `ax`'s concept, and a value that would be silently discarded is a value the operator was misled about. |
| `--` | — | Terminates launcher argument parsing. Everything after it is native argv, forwarded to the tool verbatim, in order, uninspected. |
| `--help`, `-h`, `--version` | — | Informational; print and exit 0. |

Parsing rules, closed:

- Launcher flags are recognized only before `--`. After `--`, nothing is
  interpreted — not `--help`, not a flag that happens to collide with a
  launcher flag.
- The first non-flag operand before `--` is `<env-id>`. Any further
  non-flag operand before `--` is a usage error: native arguments MUST
  follow `--`, so that the boundary between the launcher's surface and
  the tool's is visible in the command line itself.
- An unrecognized flag before `--` is a usage error. It is never
  forwarded: silent forwarding would let a typo in a launcher flag reach
  the tool as tool input.
- Every value-taking flag takes exactly one value; a repeated flag is a
  usage error, not last-wins. `--system-prompt` and `--ax-profile` accept
  only their closed vocabularies; `--name` is validated against the `ax`
  §2.1 grammar at parse time, before anything is resolved.
- Usage errors exit 2 and print usage; they launch nothing and resolve
  nothing.

## 4. Composition algorithm

A launch composes in six ordered steps: obtain the fragment, map the
environment, resolve model and effort, obtain the plan, compose, hand
off or exec. Every step either completes or fails the launch with a §6
diagnostic; there is no partial launch, and no step's failure degrades
into a weaker launch shape. The order is contract, not convenience: the
fragment comes **first** because the plan request needs the managed home
(§4.1, §4.4), and the plan comes before composition because composition
appends to a value it never rebuilds (§4.5). `curator-run` is the single
composer in both modes (Decision 0013 D1): a tracked and an untracked
launch differ only in who creates the process.

### 4.1 Obtain the fragment (context plane)

The launcher runs, as a subprocess:

```text
curator env resolve <env-id> [--profile <name>] --format json
```

and parses the closed `launch-env-fragment-v1` object per environments.md
§10.2 as revised by Decision 0012 D8, rejecting unknown fields, unknown
kinds, and unknown semantics values. The members this document consumes:

- `profile.name` and `profile.lock_sha256` — the profile and its
  effective pin (Decision 0012 D3: the lock hash is the identity
  everywhere Decision 0010 used a commit);
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
  `composition` is withdrawn under Decision 0012 D8 and is rejected as an
  unknown field once the revision 1.1 rewrite lands.

Resolution is a pure function and activates nothing; it also verifies and,
when needed, repairs the managed home, so a fragment in hand means a home
that is materialized and current. Nothing in the fragment is applied here:
`system_prompt` waits for the §5 opt-in, `mcp` is applied in §4.5.

Failure modes are distinct and none of them degrades to a fragment-less
launch:

- the subprocess cannot be started (`curator` missing from `PATH`) —
  `resolve_invocation_failed`;
- the subprocess exits non-zero — the launcher maps Curator's own
  diagnostics through: `environment_unknown` → `resolve_environment_unknown`,
  `profile_unknown` → `resolve_profile_unknown`,
  `environment_repair_failed` → `resolve_repair_failed`; any other
  non-zero exit is `resolve_invocation_failed`;
- the subprocess exits zero but the output is not a valid closed fragment
  — `resolve_fragment_invalid`. A malformed read is a read failure, never
  an absence: the launcher MUST NOT treat unparseable output as "no
  fragment" and exec anyway.

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
| `pi` | `pi` | `pi` |
| `opencode` | none in this revision — `env_unsupported` | none — `env_unsupported` |

An env-id outside this table that Curator nevertheless resolves is
`env_unsupported`: the launcher refuses rather than guessing a system or
a provider. The table grows by specification revision, never by
inference, and a row is launchable only when both non-Curator columns
are filled — the launcher does not exec untracked what it could not hand
off tracked.

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
     "schema": "curator-run-defaults-v1",
     "locked": false,
     "defaults": {
       "claude_code": { "model": "claude-opus-5", "effort": "high" },
       "codex_cli":   { "model": "gpt-5.3-codex" }
     }
   }
   ```

   `defaults` keys are env-ids of the §4.2 table (an unknown key is
   `defaults_config_invalid`); each value is an object of at most the two
   members `model` and `effort`, at least one present, both strings that
   are passed through unvalidated — admission stays the spawn plane's.
   **Lockable:** when the machine file carries `"locked": true`, the
   operator file is ignored for every env-id the machine file names, and
   a `--model`/`--effort` flag for a member the machine entry sets is a
   `usage` error naming the locked member, so a locked default is never
   silently overridden and never silently applied. Otherwise the operator
   file overrides the machine file per member: an operator entry that sets
   only `model` leaves a machine `effort` for the same env-id in force, the
   same per-member resolution as the rest of this section. A file that is
   absent is a legitimate absence and the
   level yields nothing; a file that exists but cannot be read or parsed
   is `defaults_config_invalid` — a read failure is not an absence, and
   the lineup fallback MUST NOT fire past it.
3. **Lineup fallback.** The highest-ranked model of `vendorplugin.Lineup`
   (capability score descending) among the models the module's runtime
   compatibility registry admits for the mapped system, with that model's
   declared `Effort.Recommended` as the effort, or no effort when its
   `EffortSupport` is `EffortSupportNone`. When the lineup admits no model
   for the system, the launch fails with `defaults_unresolvable` — the
   launcher never invents a model name.

The resolved pair and the level that produced each member are printed on
stderr at **every** launch, before the plan request, in one line-group —
so an operator always sees which model is about to run and why. A
refusal from the spawn plane for the resolved pair (`ErrEffortMissing`
when a required effort is still unset after all three levels) is
`plan_refused`, and the launcher completes the module's message — model,
vocabulary, recommendation — with its own flag spelling, `--effort`.
The launcher MUST NOT retry with a different pair.

### 4.4 Obtain the launch plan (spawn plane)

The launcher builds a plan request and obtains a plan through the
`agents-management` module's declared entry point `BuildPlan`. The
request, closed:

- system: the §4.2 mapped system;
- mode: `LaunchModeInteractive` (Decision 0013 D5), requested **by
  name**. The launcher never spells a provider flag; the plan's argv
  carries only model selection and the effort transport, and it is the
  system plugin's to spell. A system that does not declare the mode is
  refused by the module with `ErrUnsupportedLaunchMode` → `plan_refused`;
- `Model` and effort: the §4.3 resolved pair;
- `Home`: the managed home from the fragment's home variable (§4.1).
  This is why the fragment is obtained first: the module keys on-disk
  provider-limit state by (provider, home), so the limit evidence that
  admits or refuses this launch is the managed home's own — a profile A
  launch never gates a profile B launch through shared evidence, and the
  native home is never consulted for a managed launch;
- `WorkDir`: the launcher's current working directory;
- `Composition`: **empty**. The fragment's MCP channel is applied by the
  launcher in §4.5, not by the module; a non-empty composition in
  interactive mode is refused by the module (`ErrCompositionNotInteractive`),
  and the launcher never sends one.

The plan is a value — `Binary`, `Argv`, `Env`, `Stdin`, `WorkDir` — and
building it starts no process. `Stdin` is attached only for a system
whose effort transport is stdin; none of the §4.2 systems is, so in this
revision the plan's stdin is unattached for every launchable
environment (Decision 0013 D4).

Admission is the spawn plane's: a provider-limits verdict that is not
*observed healthy* is not serviceable, and the launcher surfaces the
structured verdict (limited-until with its evidence, unreachable with its
evidence, or unknown) as `plan_provider_limited` instead of launching.
The launcher MUST NOT retry, downgrade, or substitute a model to route
around a refusal; "checked and found nothing", "nobody looked", and "the
read failed" are three different answers and are reported as such.

### 4.5 Compose the launch

The composed launch is one plan (Decision 0013 D6.3). Its members, closed:

**argv** — in this order, each part verbatim:

1. the interactive plan's `Argv` (model selection, effort transport);
2. the system-prompt channel flags — only under the §5 opt-in, from the
   fragment's `system_prompt` descriptor of the requested semantics;
3. the MCP channel flags, whenever the fragment carries an `mcp` section
   whose descriptor is argv-carried, from that descriptor: for
   `claude_code` the `flag` with its `argument: path` and the `with`
   companions — `--mcp-config <path> --strict-mcp-config`; for
   `codex_cli` the `flag` with `argument: name` — `-p curator-mcp`; for
   `opencode` nothing — its channel is a variable and goes to the
   environment. No opt-in governs this part: a managed home launched
   without the channel carries no MCP configuration, and the profile's
   MCP set is the profile's context;
4. the native arguments after `--`, verbatim, uninspected.

The interactive plan's `Binary` is the executable. **Order is contract:**
for some tools everything after the last recognized flag is the user
turn, so a channel flag after the native arguments would become prompt
text. The general rule is fixed here; the per-tool boundary is verified
against the pinned release before the conformance vectors freeze (§9).

**environment** — four layers, later overriding earlier per name:

1. the launcher's inherited process environment;
2. the plan's `Env`;
3. the fragment's `env` map;
4. the `variable`-kind channel of an engaged descriptor: the fragment's
   `mcp` descriptor for `opencode` (`OPENCODE_CONFIG` = the materialized
   path), and a `variable`-kind system-prompt descriptor under the §5
   opt-in.

The fragment wins on exactly its own closed names — the adapter-registry
variable names pointing at managed-home paths — and touches nothing else.
This conflict rule is safe by construction: fragment names come only from
the closed adapter registry and fragment values are manager-owned
managed-home paths (the §10.3 profile-influence boundary), so the
override can only re-aim the tool's home, never alter how the process is
launched. When layer 3 or 4 overrides a name layer 2 actually set, the
launcher SHOULD warn — the plan author declared an intent the fragment is
displacing — but the fragment still wins: the operator asked for the
profile's context.

**env_names** — the fragment's `mcp.env_names` union (already bounded,
before it reaches the launcher, by the reserved-name exclusion and the
lockable passable-names allowlist of Decision 0012 D6), **minus** every
name that also appears in the composed `env_literals` of §4.6 — layers
2–4 above. This is the **literal-versus-lookup collision rule** (Decision
0013 D6.3 as amended by review finding F5): a literal the composer set is
an explicit intent and wins over a destination-local lookup, so the name
is dropped from `env_names` and a warning naming the variable is printed
on stderr. The reserved-name exclusion keeps registry adapter names out of
`env_names`, but a system plugin's plan `Env` is not bounded by it, so
this rule is what makes the composed document disjoint by construction:
`ax` §5.1 disjointness never fires for a composed document, and a
collision is never an `ax_handoff_failed`. Untracked mode has no
`env_names` — the child inherits the operator's environment directly and
the union is informative only — but the warning is printed in both modes
so that the two modes report the same facts.

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
absent file in both places means not configured. The file is read once
per invocation before argument handling completes, so the §3 usage rules
for `--ax-profile` and `--name` can name the fact; a file that exists but
cannot be read or parsed is `defaults_config_invalid`, never "not
configured". A configured integration is not
a per-launch option: there is no `--no-ax` flag, and bypassing tracking is
a configuration change, not a flag. The launcher composes the Decision 0013 D3.2 request document and
invokes, as a subprocess:

```text
ax start <name> --provider <id> --launch-plan - [--profile <ax-profile>] --workspace <cwd>
```

writing the document to `ax`'s standard input (`--launch-plan -` reads
the plan document, never the child's stdin). The operands and flags:

- `<name>`: `--name` when given, else `<env-id>-<utc-stamp>` with the
  stamp `YYYYMMDDTHHMMSSZ` in UTC at composition time (for example
  `codex_cli-20260905T073254Z`). The default always fits the `ax` §2.1
  grammar: the longest §4.2 env-id is 11 characters and the stamp is 16.
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
| `argv_suffix` | the composed argv without its element 0. The interactive plan's `Argv` is already the tail after the executable; `Binary` is the plugin's to resolve, so the composer never sends element 0 and never uses the `argv` form. |
| `env_names` | as composed, with the §4.5 collision rule already applied — disjoint from `env_literals` before `ax` sees the document. Sorted, unique. |
| `env_literals` | the **composer's own names only**: plan `Env` ⊕ fragment `env` ⊕ the engaged variable-kind channel (§4.5 layers 2–4). Never a copy of the inherited environment: the inherited layer of a tracked launch is whatever `ax`'s terminal backend gives the child on the destination. |
| `stdin` | as composed (§4.5). |
| `extensions` | exactly the four `works.relux.curator.*` keys below. |

The extension keys, set by the composer and copied verbatim by `ax` into
the Session Record's top-level `extensions` (Decision 0013 D6.4, D7):

| Key | Derivation |
|---|---|
| `works.relux.curator.profile-name` | the fragment's `profile.name` |
| `works.relux.curator.profile-pin` | the fragment's `profile.lock_sha256`, spelled `sha256:<64 lowercase hex>` — the profile's effective pin under Decision 0012 D3 |
| `works.relux.curator.fragment-digest` | the §4.1 digest: `sha256:` over the CCJ-1 canonical bytes of the parsed fragment object |
| `works.relux.curator.system-modules` | boolean, `true` exactly when the fragment carries a `system_prompt` section — environments.md §10.2 makes that presence equivalent to "the resolved chain carries at least one applicable system module" |

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

**Without the integration**, the launcher execs the composed plan
directly: the plan's `Binary` with the composed argv (§4.5, element 0 the
binary), the composed environment, and the composed stdin — the same
plan, with no `ax` residue. Untracked is the honest answer on such a
machine.

In both shapes the plan's binary missing from the filesystem or `PATH`
is `exec_provider_missing`, reported with the exact executable name and
installation guidance; in tracked mode the launcher checks this before
the handoff so that `ax` is never asked to record a session for a binary
that is not there.

## 5. System-prompt application

Resolving a fragment activates nothing: the fragment's `system_prompt`
section is data about a channel — the inert materialized file's path and
the adapter's declared channel descriptors — never an applied override.
The launcher is the one component that applies a `flag`, `config-key`, or
`variable` channel, and only behind the explicit
`--system-prompt <append|replace>` opt-in. `file`-kind channels are the
one exception: the launcher never applies them — the tool does, on its
own, whenever the file is present — so the launcher's whole duty for them
is detection and warning (§5.1).

Application, by descriptor kind (the per-environment channel tables are
environments.md §7.3 and are not restated here):

- **flag-class** (`claude_code`, `pi`): append the descriptor's flag with
  the fragment's system-prompt path to the plan argv, before the native
  arguments.
- **config-key** (`codex_cli`): apply the descriptor's key
  (`model_instructions_file`) with the fragment's path through the tool's
  declared configuration-override mechanism. The exact override spelling
  verifies against the pinned tool release before the conformance vectors
  freeze, per the environments.md §7.3 discipline.
- **variable-class** (`gemini`, once that adapter lands): set the
  descriptor's variable to the fragment's path in the child environment.
- **file-class** (`pi`: `APPEND_SYSTEM.md` for `append`, `SYSTEM.md` for
  `replace`): the launcher applies nothing, because there is nothing left
  to apply — the tool reads the file from its home unconditionally when
  it is present. The launcher MUST NOT place, remove, or edit a
  file-kind channel's file: those files are materialized exclusively by
  Curator, and only under the per-profile × environment
  `system_prompt_files` machine setting (environments.md §5.5, default
  `off`). The opt-in for a file-kind channel happened at the
  machine-setting level, not on this command line. The launcher's duty
  is §5.1: detect an active file-kind channel and warn.

### 5.1 File-kind channels: detection, orthogonality, warning

The file-kind probe is keyed on the **environment**, not the fragment.
For an env-id whose environments.md §7.3 adapter registry declares
`file`-kind channels — in revision 1 exactly `pi`, with
`APPEND_SYSTEM.md` (`append`) and `SYSTEM.md` (`replace`) — the launcher
MUST probe, on **every** launch, immediately before the handoff or exec
of §4.6, the path `<home>/<filename>` for each filename in that closed
registry set, where `<home>` is the managed home the fragment's `env`
map points the tool at. The probe runs whether or not the fragment
carries a `system_prompt` section: the tool applies a present file
unconditionally (environments.md §7.3), so which channels the fragment
happens to name has no bearing on which files the tool will read. The
registry set is closed and versioned with the environments protocol, and
the launcher already owns the §4.2 environment mapping, so keying the
probe on the registry adds no new knowledge edge. The probe has exactly
three outcomes, and they are three different facts:

- **Absent** — no file exists at the path. The channel is inactive. This
  is the legitimate default: `system_prompt_files` defaults to `off` and
  §5.5 keeps both files unwritten under it. No warning, no diagnostic,
  no launch change. Absence of a file-kind file is never an error,
  because the registry declares what channels *exist*, not which are
  engaged: an inactive channel is the normal state of a managed home.
- **Present and readable** — a regular file the launcher can open for
  reading. The channel is **active**: the tool will apply it. The
  launcher MUST print the §5.2 warning naming this channel, even when
  `--system-prompt` was not given, and even when the fragment carries no
  `system_prompt` section or no descriptor for this filename. A plain
  launch into such a home is a customized run regardless of how the file
  got there: when the machine setting materialized it, the opt-in
  happened at the machine-setting level; when something else wrote it,
  no one opted in at all, and the warning is the only thing standing
  between the operator and a silently customized run.
- **Anything else** — the path exists but cannot be read (permission,
  I/O error), or is not a regular file (a directory, a dangling
  symlink). The launch fails with `sysprompt_file_unreadable`. A failed
  probe is a read failure, never an absence: the tool itself will
  attempt the file at startup, so exec'ing past an indeterminate probe
  would start a run whose customization state the launcher cannot
  state — and the warning contract of §5.2 would be unsatisfiable.

File-kind presence is **orthogonal and additive** to the flag-class
opt-in. The `--system-prompt <semantics>` opt-in selects among the
fragment's non-`file` channels only; a `file`-kind descriptor never
satisfies the opt-in, and the opt-in never suppresses, removes, or
substitutes for an active file-kind channel. On a `pi` launch with
`--system-prompt append` into a home where `APPEND_SYSTEM.md` is present,
both channels engage — the launcher appends the flag, the tool applies
the file — and the warning enumerates both. The launcher MUST NOT try to
deduplicate the two applications: which one the tool honors, and in what
order, is the tool's documented behavior, not the launcher's to arbitrate.

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
  no key, and exports no variable. Managed homes carry no active
  system-prompt file by default (environments.md §5.5), so a plain launch
  runs the tool's built-in behavior — except where a file-kind
  channel's file is present in the home, which §5.1 detects and warns
  about without altering the launch.

**Warnings.** Every activation — a flag-class, config-key, or
variable-class channel the launcher applied under the opt-in, and every
active file-kind channel detected by the §5.1 probe — MUST be covered by
a warning printed to stderr, before the §4.6 handoff or exec, that
states all three of:

1. this run's system prompt is customized, enumerating **every** active
   channel: for each, the profile, the descriptor kind (and, for
   file-kind, the filename), and the semantics applied;
2. for `replace` semantics on any active channel, that replacement
   discards the tool's built-in system behavior entirely;
3. that a custom system prefix can change how requests are cached and
   therefore billed — a tool's default system prompt may participate in
   shared prompt caching, while a custom one forms its own cache prefix
   (exact per-tool behavior is Decision 0010 open question 7's research).

The warning set is identical whether a channel was engaged by the
command-line opt-in or by machine-setting materialization: the operator
at the keyboard may not be the operator who configured the machine, and
the warning exists for the one at the keyboard. The warning is not
suppressible in revision 1, and the absence of `--system-prompt` on the
command line MUST NOT suppress it for file-kind channels.
Reproducibility over silence: the cost of the warning is one stderr
line-group; the cost of a silent customized run is an operator
misattributing behavior to the tool.

## 6. Errors and diagnostics

Diagnostics are stable machine-readable codes in closed families. A
failing launch prints exactly one diagnostic code line to stderr,
followed by human-oriented detail; the code, not the prose, is the
contract. Usage errors exit 2; every operational failure exits 1.

| Family | Codes | Condition |
|---|---|---|
| usage | `usage` | unknown flag, missing `<env-id>`, stray operand before `--`, repeated flag, invalid `--system-prompt` or `--ax-profile` value, `--name` outside the `ax` §2.1 grammar or over 64 characters, `--ax-profile` on an untracked machine, a flag overriding a locked default — exit 2, nothing resolved, nothing launched |
| resolve | `resolve_invocation_failed`, `resolve_environment_unknown`, `resolve_profile_unknown`, `resolve_repair_failed`, `resolve_fragment_invalid` | §4.1: the context plane could not produce a usable fragment |
| defaults | `defaults_config_invalid`, `defaults_unresolvable` | §4.3: a machine-configuration file exists but cannot be read or parsed, or names an unknown env-id or member — a read failure, never an absence; the lineup admits no model for the mapped system after the flag and configuration levels left it unset |
| plan | `plan_refused`, `plan_provider_limited` | §4.4: the spawn plane refused the request (unknown system, mode not declared by the system, invalid or missing required effort, unresolved vendor), or the provider-limits verdict was not observed healthy — the verdict's structure and evidence are surfaced verbatim |
| environment | `env_unsupported` | §4.2: the environment has no spawn-plane or `ax` provider mapping in this revision |
| exec | `exec_provider_missing` | §4.6: the plan's binary does not exist — reported with the executable name and installation guidance |
| ax | `ax_handoff_failed` | §4.6: the configured `ax` could not take the launch — `ax` not startable, or `ax start` exited non-zero, its Structured Error passed through; no untracked fallback |
| system prompt | `sysprompt_channel_unavailable`, `sysprompt_file_unreadable` | §5.2: opt-in given but the fragment carries no non-`file` channel with the requested semantics; §5.1: a registry-declared file-kind channel's file exists but cannot be read (or is not a regular file) at the pre-exec probe — an absent file is not this diagnostic, it is the channel's legitimate inactive state |

Two invariants hold across every family. First, an absence and a failure
to read are different facts: a fallback defined for absence (no
`system_prompt` section means no channel to select; an absent file-kind
file means an inactive channel; an absent defaults file means the next
level) never fires on a failed or malformed read
(`resolve_fragment_invalid`, `sysprompt_file_unreadable`,
`defaults_config_invalid`). Second, no
diagnostic downgrades the launch: every failure is terminal for that
invocation, and the operator retries deliberately.

## 7. Planned dependency

The stub imports nothing beyond the standard library. The implementation
will consume `github.com/relux-works/skill-agents-management` as its one
Go module dependency (public module, no replace, no vendoring; sibling
development through a gitignored `go.work`), and Curator and `ax` as CLI
contracts only. The module version that carries `LaunchModeInteractive`,
`ErrCompositionNotInteractive`, and the per-system interactive goldens
(Decision 0013 D5) is **required**: this specification cannot be
implemented against `91bf945` or any earlier main, whose closed
`LaunchMode` set is exec, dry-run, and managed-session only. The exact
version requirement is pinned in `go.mod` and restated here when that
release exists. The module's own invariants — plans as values, argv
parity goldens, frozen admitted-pair digests, per-model effort with no
injected default, fail-open limit state with indeterminate-read-is-unknown
— are relied upon, not re-implemented; `vendorplugin.Lineup` and the
runtime compatibility registry are consumed for §4.3 level 3 and never
reordered.

## 8. Versioning

- This specification is versioned semantically; the current version is
  **`0.2.0-draft`**. Draft versions may change incompatibly between
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
| `0.2.0-draft` | Decision 0013 D6 applied. §4 reordered fragment-first and grown to six steps: the managed home from the fragment is `LaunchRequest.Home` (D6.1, M7); the plan is requested as `LaunchModeInteractive` with an empty `Composition` and the launcher spells no provider flag (D5, M2); launcher-owned model/effort default precedence — flags, lockable `defaults.json` machine configuration, lineup fallback — with the resolved pair printed every launch (D6.2, M8); the composition rule with argv order as contract, the MCP channel applied by the launcher, the four-layer environment, and the literal-versus-lookup `env_names` collision rule (D6.3, F5); tracked mode specified as `ax start <name> --provider <id> --launch-plan - [--profile] --workspace <cwd>` with the request document, the four `works.relux.curator.*` extension keys, the `profile-pin` as the lock hash, session-name derivation, and Structured Error pass-through (D6.4). §3 gains `--name` and `--ax-profile`; §4.2 gains the `ax` provider-id column; §6 gains the `defaults` family; §7 requires the interactive-mode module release; §1 non-goals restated (D6.5). §5 unchanged apart from renumbered cross-references. |
| `0.1.2-draft` | §5.1 probe re-keyed from the fragment's descriptor list to the environment adapter's closed file-channel filename set, run on every launch into a managed home regardless of the fragment's `system_prompt` section; false stray-file drift-and-repair claim removed — a stray file at a registry filename is unmanaged, no automated contract removes it, and every launcher-mediated launch warns until the operator removes it; native/hand-launch and probe-to-exec race residuals recorded in §9. |
| `0.1.1-draft` | §5 restructured (§5.1/§5.2): file-kind channel semantics specified — launcher never places, removes, or edits the files; pre-exec presence probe; warnings mandatory for an active file-kind channel without the `--system-prompt` opt-in; orthogonal-and-additive selection when flag-class and file-kind coexist; `sysprompt_file_unreadable` diagnostic added. |
| `0.1.0-draft` | Initial in-repository draft. |

## 9. Open items

- The §5.1 warning contract is complete only for launcher-mediated
  launches, and the probe is a point-in-time check. Two residuals are
  known and accepted in this revision: a tool started by hand in a
  managed home applies a present file-kind file with no probe and no
  warning (§1: the launcher is never mandatory), and a file written
  between the §5.1 probe and the tool's own startup read goes
  undetected. Neither residual is closable from the launcher's seat;
  closing the first would require the tool or Curator to warn, which
  the cited protocol does not provide.
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
- **Per-tool argv boundary.** The §4.5 order — plan argv, system-prompt
  flags, MCP flags, native arguments — is verified against each pinned
  tool release before the conformance vectors freeze: that the channel
  flags are recognized after the plan's model and effort flags, and that
  the first native argument is where the tool's own parsing begins
  (environments.md §7.3 discipline). The codex_cli `-p curator-mcp` layer
  against an operator `-p` after `--` is Decision 0012 open question 3
  and is verified in the same pass.
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
- The §4.3 `defaults.json` location and lock rule are this document's
  own; if Curator's machine configuration grows a launcher section, the
  file moves there by specification revision and the schema stays.
