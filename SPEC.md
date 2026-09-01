# Curator Agent Launcher — Specification

**Specification version:** `0.1.0-draft`
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
- `curator-spec/protocol/environments.md` — §10 (`env resolve` and
  `launch-env-fragment-v1`), §7.3 (system-prompt channels), §5.5
  (system-prompt output), §11 (umbrella subcommand discovery).
- The `agents-management` module documentation — the `BuildPlan` /
  `BuildLaunch` value contract and the provider-limits verdict model.

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
  reconstructs argv from its own knowledge of a tool, never patches a
  plan except through the declared system-prompt channels of §5, and
  never second-guesses an admission verdict.
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
            [--model <model>] [--effort <effort>] [--] <native args...>
curator-run --help | -h
curator-run --version
```

The flag set is deliberately minimal; every flag below names the single
plane it feeds, and a need that fits none of them is a specification
change, not a flag addition.

| Element | Plane | Meaning |
|---|---|---|
| `<env-id>` | context + spawn | Required operand. A registered Curator environment identifier (`claude_code`, `codex_cli`, `opencode`, `pi`). Selects both the fragment to resolve and, through the closed mapping of §4.2, the agentic system to plan. |
| `--profile <name>` | context | Forwarded verbatim to `curator env resolve` as its `--profile` operand. Absent, resolution uses the current profile for the applicable scope. |
| `--system-prompt <append\|replace>` | execution | Explicit opt-in that engages the fragment's system-prompt channel with the given semantics. The value is required: the opt-in states what it wants, and the launcher never chooses replacement by default. See §5. |
| `--model <model>` | spawn | Passed through to the spawn plane's plan request as declared. The launcher does not validate model names; admission is the spawn plane's verdict. |
| `--effort <effort>` | spawn | Passed through as declared. Effort is per-model and required by the spawn plane, which injects no default; a refusal names the model, the accepted vocabulary, and the recommendation, and the launcher completes that error with its own flag spelling, `--effort`. |
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
- Usage errors exit 2 and print usage; they launch nothing and resolve
  nothing.

## 4. Composition algorithm

A launch composes in five ordered steps. Every step either completes or
fails the launch with a §6 diagnostic; there is no partial launch, and no
step's failure degrades into a weaker launch shape.

### 4.1 Obtain the launch plan (spawn plane)

The launcher builds a plan request from `<env-id>` (mapped per §4.2),
`--model`, and `--effort`, and obtains a launch plan through the
`agents-management` module's declared entry points (`BuildPlan`, or
`BuildLaunch` for a legacy runtime id). The plan is a value — binary,
argv, environment, stdin bytes — and building it starts no process.

Admission is the spawn plane's: a provider-limits verdict that is not
*observed healthy* is not serviceable, and the launcher surfaces the
structured verdict (limited-until with its evidence, unreachable with its
evidence, or unknown) instead of launching. The launcher MUST NOT retry,
downgrade, or substitute a model to route around a refusal; "checked and
found nothing", "nobody looked", and "the read failed" are three different
answers and are reported as such.

### 4.2 Environment-to-system mapping

The spawn plane speaks agentic-system plugin ids; Curator speaks
environment ids. The launcher owns the closed mapping between them:

| Curator env-id | agents-management system |
|---|---|
| `claude_code` | `claude-code` |
| `codex_cli` | `codex` |
| `pi` | `pi` |
| `opencode` | none in revision 1 — `env_unsupported` |

An env-id outside this table that Curator nevertheless resolves is
`env_unsupported`: the launcher refuses rather than guessing a system.
The table grows by specification revision, never by inference.

### 4.3 Obtain the fragment (context plane)

The launcher runs, as a subprocess:

```text
curator env resolve <env-id> [--profile <name>] --format json
```

and parses the closed `launch-env-fragment-v1` object per environments.md
§10.2, rejecting unknown fields, unknown kinds, and unknown semantics
values. Resolution is a pure function and activates nothing; it also
verifies and, when needed, repairs the managed home, so a fragment in hand
means a home that is materialized and current.

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

### 4.4 Merge the child environment

The child environment is composed from exactly three layers, later
overriding earlier on a per-name basis:

1. the launcher's inherited process environment;
2. the plan's environment;
3. the fragment's `env` map.

The fragment wins on exactly its own closed names — the adapter-registry
variable names pointing at managed-home paths — and touches nothing else.
This conflict rule is safe by construction: fragment names come only from
the closed adapter registry and fragment values are manager-owned
managed-home paths (the §10.3 profile-influence boundary), so the
override can only re-aim the tool's home, never alter how the process is
launched. When layer 3 overrides a name layer 2 actually set, the
launcher SHOULD warn — the plan author declared an intent the fragment is
displacing — but the fragment still wins: the operator asked for the
profile's context.

### 4.5 Hand off and exec

**With the `ax` integration configured** on the machine, the launcher
ALWAYS routes the composed launch through `ax`'s instrumentation so the
session is tracked from birth. A configured integration is not a
per-launch option: there is no `--no-ax` flag, and bypassing tracking is
a configuration change, not a flag. For resume fidelity the launcher
hands `ax` the fragment data Decision 0010 recommends recording in the
Session Record extensions — profile name, effective commit (or
`state_sha256` for a `local` profile), and the fragment digest — so that
resume can re-resolve the same profile and detect drift. A handoff that
fails is `ax_handoff_failed`; the launcher MUST NOT fall back to an
untracked direct exec, because a machine configured for tracking has
declared that untracked sessions are the failure mode, not the fallback.

**Without the integration**, the launcher execs the plan directly:
the plan's binary with the plan's argv, the native arguments after `--`
appended verbatim, the merged environment of §4.4, and the plan's stdin
bytes. Untracked is the honest answer on such a machine, and the launch
carries no `ax` residue.

In both shapes the plan's binary missing from the filesystem or `PATH`
is `exec_provider_missing`, reported with the exact executable name and
installation guidance.

## 5. System-prompt application

Resolving a fragment activates nothing: the fragment's `system_prompt`
section is data about a channel — the inert materialized file's path and
the adapter's declared channel descriptors — never an applied override.
The launcher is the one component that applies a channel, and only behind
the explicit `--system-prompt <append|replace>` opt-in.

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

Selection and refusal rules:

- The launcher selects the fragment channel whose `semantics` equals the
  opt-in value. If the fragment carries no `system_prompt` section (the
  resolved chain has no applicable system modules), or no channel with the
  requested semantics exists for the environment, the launch fails with
  `sysprompt_channel_unavailable`. Opting in to nothing is an error, not
  a silent no-op: the operator asked for a customized run and MUST NOT
  get an uncustomized one without noticing.
- Without the opt-in the launcher applies no channel, adds no flag, sets
  no key, and exports no variable. Managed homes carry no active
  system-prompt file by default (environments.md §5.5), so a plain launch
  runs the tool's built-in behavior.

**Warnings.** Every activation MUST print, to stderr, before the exec, a
warning that states all three of:

1. this run's system prompt is customized (naming the profile and the
   semantics applied);
2. for `replace` semantics, that replacement discards the tool's built-in
   system behavior entirely;
3. that a custom system prefix can change how requests are cached and
   therefore billed — a tool's default system prompt may participate in
   shared prompt caching, while a custom one forms its own cache prefix
   (exact per-tool behavior is Decision 0010 open question 7's research).

The warning is not suppressible in revision 1. Reproducibility over
silence: the cost of the warning is one stderr line-group; the cost of a
silent customized run is an operator misattributing behavior to the tool.

## 6. Errors and diagnostics

Diagnostics are stable machine-readable codes in closed families. A
failing launch prints exactly one diagnostic code line to stderr,
followed by human-oriented detail; the code, not the prose, is the
contract. Usage errors exit 2; every operational failure exits 1.

| Family | Codes | Condition |
|---|---|---|
| usage | `usage` | unknown flag, missing `<env-id>`, stray operand before `--`, invalid `--system-prompt` value — exit 2, nothing resolved, nothing launched |
| resolve | `resolve_invocation_failed`, `resolve_environment_unknown`, `resolve_profile_unknown`, `resolve_repair_failed`, `resolve_fragment_invalid` | §4.3: the context plane could not produce a usable fragment |
| plan | `plan_refused`, `plan_provider_limited` | §4.1: the spawn plane refused the request (unknown system, invalid effort, unresolved vendor), or the provider-limits verdict was not observed healthy — the verdict's structure and evidence are surfaced verbatim |
| environment | `env_unsupported` | §4.2: the environment has no spawn-plane mapping in this revision |
| exec | `exec_provider_missing` | §4.5: the plan's binary does not exist — reported with the executable name and installation guidance |
| ax | `ax_handoff_failed` | §4.5: the configured `ax` instrumentation could not take the launch; no untracked fallback |
| system prompt | `sysprompt_channel_unavailable` | §5: opt-in given but the fragment carries no channel with the requested semantics |

Two invariants hold across every family. First, an absence and a failure
to read are different facts: a fallback defined for absence (no
`system_prompt` section means no channel to select) never fires on a
failed or malformed read (`resolve_fragment_invalid`). Second, no
diagnostic downgrades the launch: every failure is terminal for that
invocation, and the operator retries deliberately.

## 7. Planned dependency

The stub imports nothing beyond the standard library. The implementation
will consume `github.com/relux-works/skill-agents-management` as its one
Go module dependency (public module, no replace, no vendoring; sibling
development through a gitignored `go.work`), and Curator and `ax` as CLI
contracts only. The module's own invariants — plans as values, argv
parity goldens, frozen admitted-pair digests, per-model effort with no
injected default, fail-open limit state with indeterminate-read-is-unknown
— are relied upon, not re-implemented.

## 8. Versioning

- This specification is versioned semantically; the current version is
  **`0.1.0-draft`**. Draft versions may change incompatibly between
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

## 9. Open items

- The §4.2 mapping for `opencode` awaits an `agents-management` system
  plugin; until then the environment resolves but does not launch.
- The `gemini` variable-class channel (`GEMINI_SYSTEM_MD`) engages when
  the `gemini` adapter lands in the environments protocol.
- The codex_cli configuration-override spelling and both flag-class
  spellings verify against pinned tool releases before conformance
  vectors freeze (environments.md §7.3 discipline).
- The `ax` handoff invocation shape (argv contract of the configured
  instrumentation) is specified when the Decision 10 pull request against
  `agent-session-manager-spec` lands; until then §4.5 fixes only the
  behavioral contract: always-when-configured, no untracked fallback,
  fragment data recorded.
