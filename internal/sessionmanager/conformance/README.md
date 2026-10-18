# Session-manager qualification

Run the reference plugin and its instrument controls:

```sh
go test ./internal/sessionmanager/conformance -count=1 -v
go test ./internal/sessionmanager/conformance -race -count=5
```

Tests build the real plugin and separate `curator-run-fake-manager-fixture`
executables in `t.TempDir`.
Every invocation has a deadline, an isolated process group, explicit PID/group
cleanup and a no-survivor assertion. The fake provider is a durable no-op
lifecycle; no harness, daemon, a2a owner or credential store is started.

To qualify another registered executable, supply each prefix argument separately:

```sh
go test ./internal/sessionmanager/conformance -run '^TestConformance$' -v \
  -plugin /absolute/plugin -manager example \
  -prefix=session-manager -prefix=plugin \
  -fixture /absolute/fixture-adapter \
  -fixture-prefix=session-manager -fixture-prefix=conformance-fixture
```

Use an isolated, disposable manager state root. `reset` is an explicit fixture
operation. The suite creates sessions, stops them and injects faults only in
that fixture. The plugin executable and adapter executable need not be the same.
No controls travel through the plugin environment or RPC requests. The normal
protocol capability set remains closed. The plugin refuses `fixture` as an
unknown operation; adapter JSON cannot reset state through the plugin executable.

For the reference executable, the plugin and adapter select the same isolated state:

```sh
go build -o .temp/curator-run-fake-manager ./cmd/curator-run-fake-manager
go build -o .temp/curator-run-fake-manager-fixture ./cmd/curator-run-fake-manager-fixture
go test ./internal/sessionmanager/conformance -run '^TestConformance$' -v \
  -plugin "$PWD/.temp/curator-run-fake-manager" \
  -prefix=--state -prefix="$PWD/.temp/conformance-state" \
  -fixture "$PWD/.temp/curator-run-fake-manager-fixture" \
  -fixture-prefix=--state -fixture-prefix="$PWD/.temp/conformance-state"
```

## Tiers and verdicts

**Tier A** uses only black-box RPCs: valid wire/describe, unknown operations,
missing capabilities, unsupported runtimes, advertised operations, secret and
reserved child environment rejection, unknown stop, idempotency and semantic
JSON replay, conflict, cancel tombstones, generation fencing, exact label
subsets, complete pagination, missing/location-mismatched resume, fd4 refusals
and all three concurrent admission pairings. Re-stopping an old down handle
must not release a newer handle's admission key.

Without an adapter, supply `-launch /absolute/valid-launch-template.json` for
Tier A. The suite refreshes `operation_id`, `submitted_at` and its isolation
label; all profile, policy and working-directory fields come from this known
valid template. Each scenario declares its operation/capability prerequisites
and checks both capabilities and the template runtime's operation cells from
`describe` before exercising them. Missing advertised prerequisites produce
**SKIP: not_applicable**, never pass or failure. Each prerequisite belongs to
an operation or assertion actually exercised. Launch validation, semantic replay,
conflict and launch expiry remain applicable to launch-only plugins. Replay list
membership, resume expiry, stale attach and stale stop have separate scenarios
so unrelated optional operations cannot suppress these checks. Operation-outcome
lookup requires status support; admission-key checks require `admission/1`.
The advertised-operations case probes each runtime's available cells independently
and does not require stop, list, attach or resume when absent. Only label subset
and filtered missing-label evidence require `labels/1`. Other scenarios use empty
labels when that capability is absent; their disposable fixture provides isolation.
Without an adapter, the caller must likewise supply isolated manager state.
If a plugin supports every optional capability or all three
delegated runtimes, the corresponding absence case is explicitly not applicable.
No absent feature is invented just to obtain a refusal.

**Tier B** requires the separate adapter: profile digest refusal, identity drift
and precedence, ambiguous resume, preserved policy, busy/unknown stop with no
state/gate/provider effects, unknown liveness, seven-day retention and expiry,
event timestamps that remain stable under polling/listing and advance only
on new activity, stable snapshot values and membership, unreadable projections and whole-page
index refusals, unreadable historical outcomes, start/reservation crash windows,
armed doorbell gate closure and preserved generation/uncertainty across
retirement/rebind, graceful draining, credential
sentinel scans, plain terminal exit, lost replies and poisoned response handling.

Without `-fixture`, every selected Tier B case reports **SKIP: UNQUALIFIED**.
The overall result fails with `not conforming (tier B unqualified)`. These cases
are never counted as passed. Setup errors and unexpected timeouts are failures,
never qualification evidence or successful known-bad detections.

Response validation is the existing S3 client/schema validator. Raw subprocess
calls are intentional: rejected requests must reach the actual plugin instead
of being stopped by the launcher's outbound validator. Terminal records use the
existing SPEC status classifier, including `session_generation_stale` on fd4.
The fd3 request and fd4 status channels use bounded pipes. The suite scans full stdout/stderr streams for a split sentinel while retaining
bounded prefixes; hostile bytes never become diagnostic text.

## Separate fixture adapter contract

The separate adapter executable reads one JSON request on stdin and returns one JSON object on
stdout, then exits. Input is bounded to 64 KiB; evidence output is bounded to
8 MiB. Exit 0 requires `ok:true`; fixture errors are nonzero with
`{"ok":false,"code":"fixture_invalid"}`. No plugin envelope is involved.
All fixture changes must be serialized with the manager's normal transactions.
Times are whole-second UTC RFC3339. Session handles are previously returned
handles. No request or response contains real credentials.

| `operation` | Members | Required effect |
| --- | --- | --- |
| `reset` | optional fake-only `config` | Reset disposable fixture state; return a known valid `launch` template. |
| `set_activity` | `session`, `activity`, optional `enabled` | Set `idle`, `busy`, `unknown`, `draining`; idle/busy events persist the logical clock as activity time, while unknown/draining retain the previous event time; `uncertain` persists prior doorbell uncertainty; `graceful-hang` simulates a live provider after its graceful share; `provider-gone` allows persisted retirement to finish. |
| `set_identity_drift` | `enabled` | Drift/restore the template profile's persisted binding without changing the admitted session identity. |
| `inject_fault` | `name`; optional reference-only `bad` array | Select a fault below; empty name restores ordinary replies. `bad` persists across fault changes and is cleared by reset. |
| `set_clock` | `now` | Set the manager's logical clock, never the host clock. |
| `corrupt_index` | `kind`, `session` when applicable, `enabled` | `unreadable` makes that record unreadable; `missing-labels` removes indexed label evidence; `missing-handle` invalidates index handle evidence; `repair` restores all three. |
| `observe` | none | Read-only evidence of provider/gate/retirement state and admitted policy digests. |

Every successful response includes `observation`:

```json
{"ok":true,"observation":{"starts":0,"alive":{},"gates":{},"retirement":{},"policy_digests":{},"records":{}}}
```

`starts` is a monotonic provider-start counter. `alive` and `gates` map handles
to booleans. `retirement` is 0 before retirement, 1 while draining and 4 after
completion. `policy_digests` maps handles to SHA256 of their original admitted
nonsecret policy representation, which resume must preserve. `records` contains
all manager-owned persisted operation/session metadata for sentinel scanning;
it is evidence only and the suite does not depend on its private layout.
`reset` additionally returns `launch`, a complete delegated launch object.

Fault names:

- `before-admission`: lose the request before any ledger/provider mutation.
- `prepared`: persist prepared evidence and lose the reply, allowing cancel.
- `after-admission`: commit a start and lose its reply; status recovers started.
- `failed-start`: persist the typed failure, release admission, lose the reply.
- `crash-after-reservation`: terminate the executable immediately after the
  durable reservation, bypassing deferred and final saves; no provider starts.
- `crash-after-start`: retain unknown start evidence and a live no-op provider;
  the admission key remains held.
- `malformed`, `over-bound`, `protocol`, `exit-ok`: deliberately invalid replies.
- `detail-text`, `detail-control`, `detail-secret`: invalid closed detail values.
- `started-unreadable`: deliberately emit a started outcome with an unreadable
  record, which the shared client must poison without keeping any fields.
- `plain-attach-exit`: terminal exit 7 with no fd4 refusal record.

## Reference state and controls

`curator-run-fake-manager --state <absolute-directory> <operation>` selects the
state root. Without `--state`, it uses
`$XDG_STATE_HOME/curator-run/fake-manager`, falling back to
`$HOME/.local/state/curator-run/fake-manager`.

The owner-only state directory contains `config.json`, `control.json`,
`state.json` and `lock`. The first two are out-of-band fixture inputs. The fake
uses the existing atomic writer and an OS advisory lock; it rejects symlink,
unsafe and malformed state files instead of overwriting them. A configured
root's symlink spelling is resolved before persistence. State is bounded to
8 MiB and ledgers/tombstones are retained indefinitely (at least seven days).
A previously unseen operation older than seven days refuses expired; an existing
operation replays retained evidence. Exactly seven days is still admissible.

`config.json` can override `id`, `capabilities`, `runtimes`, `profiles` and the
nonsecret `credential_source` binding. An optional fake-only `models` allowlist
restricts launch policy; reset supplies its first value in the known-valid
template. The policy-preservation control uses the adapter's original model and
policy values, without substituting provider identifiers. Profile `lock_sha256` is 64 lowercase
hex characters without a prefix; `fragment_digest` is `sha256:` followed by
64 lowercase hex characters, matching the shared launch/resume schemas.
Defaults enable the implemented delegated
lifecycle, admission, labels and simulated doorbell records for Claude and Codex. Exact launch and upgrade
claims are never advertised. Doorbell capabilities enable simulated record
shapes only; this fixture sends no a2a traffic. Configuring an unimplemented
capability refuses instead of advertising a scaffold.

`TestKnownBadControls` runs each normal scenario again with its corresponding
reference-only `bad` flag and requires that exact property to fail. A setup
failure does not count. Activity-on-read, armed gate-closure, lost-generation
and falsely advertised resume controls must reach their specific assertions.
`TestAdvertisedSubsets` runs the same qualification driver against individual capability omissions,
launch-only and runtime-without-resume configurations, reporting absent features
as not_applicable. `TestScenarioPrerequisites` checks every declared prerequisite
in both capability and runtime directions. `TestMandatoryLaunchPrerequisites`
independently pins launch-only properties. `TestLimitedCapabilityValidation`
executes the actual gated qualification driver in bounded test subprocesses for
labels-free and launch-only configurations: valid controls must pass all three
profile/child-env checks, while active request-validation and profile-digest
plants must fail their exact properties. Skips and setup failures never count.
Poison cases verify the exact injected wire sample reached
the validator; known-bad wire controls corrupt ordinary status instead of skipping
the injection. Additional busy controls preserve the refusal while changing state,
and the credential control preserves the refusal while leaking into records, so
the effect and sentinel scanners must detect the intended violation. These are deliberately nonconforming fixtures, never
runtime fallbacks. `.scripts/fake-manager-mutants.py` separately weakens source
guards and requires the corresponding unmodified conformance assertion to fail;
it restores the exact source bytes after every probe.
