# Profile-bound conversion and terminal hosting

The launcher owns host selection. Curator Core owns the profile lock and the
managed home for each runtime. The converter owns the private transcript bundle,
the target writer and native read-back. These boundaries follow the Swarma book
chapters [08, session launch and hosting](https://github.com/relux-works/swarma-docs/blob/main/book/ru/08-zapusk-i-host-sessiy.md) and [10, Curator](https://github.com/relux-works/swarma-docs/blob/main/book/ru/10-curator.md), Curator's environments protocol and the
launcher front-door contract. The conversion CLI is installed by the transfer skill.

## Host selection

Use `--host <registered-id>` to select one host for an invocation without
changing the registry. `--native` selects direct execution. With no flag, the
existing system `host_mode` and the selected registry entry apply. The existing
machine-over-user registry precedence also applies to named selections.
A missing or unavailable explicitly requested host fails before provider start.

The existing registration manifest accepts either `curator-session-manager/1`
or `tmux-cli/1`. A tmux registration names an absolute tmux executable and an
empty argv prefix. It provides a persistent terminal for a native provider.
The manager protocol retains its describe and operation capability negotiation.
Swarma Session Host can register through that protocol when its productive
manager handler is qualified. An ordinary PTY has no manager-owned goal,
notification enrollment, observation lease or authority to transfer a live lane.

Register tmux, optionally selecting it for launches which use the registry:

```sh
tmux_path=$(command -v tmux)
python3 -c 'import json,sys; print(json.dumps(dict(id="tmux", protocol="tmux-cli/1", executable=sys.argv[1], argv_prefix=[], registered_by="operator")))' "$tmux_path" |
  curator run session-manager register --manifest - --select
curator run codex --profile keeper --host tmux --detach -- resume TARGET_UUID
```

Each launch uses a private tmux server. Its short name is printed for attach.
The target receives the provider's native argv through a private OS socket.
A launcher in the pane resolves the same profile again, checks the fragment
digest and performs ordinary execution admission. Detached startup waits for
the provider process to start. A matching UUID resume can reattach to the
same pane. Concurrent startup or changed settings refuse reuse until reconciled.
For an interrupted start, inspect the printed socket and bundle before retrying.

## Conversion

```sh
curator run codex from muse --resume SOURCE_UUID --host tmux --a2a LANE --detach
curator run codex from muse --resume SOURCE_UUID --host tmux --history-only --detach
```

The external `agent-session-convert locate` command locates the source in
registered managed stores. An optional `--profile` disambiguates the source.
The launcher resolves the destination in that profile, checks the lock and native
resume grammar, and invokes `convert --apply` with the pinned home, source ID
and lock. It accepts only a published receipt matching the canonical target,
provider and workspace. The target UUID goes through the normal native or tmux
launcher. Controller-owned import on a delegated manager requires its own
admission contract. A filtered publication also requires provider support.

Reusable private bundles live under the operator's OS config directory in
`swarma/session-conversions/`. A pending receipt, failed read-back or changed
source stops reuse. Inspect the evidence before starting a new transfer.
Unsupported writer tuples keep the converter's existing qualification gates.

## Notification handoff

Conversion requires a deliberate notification choice. `--history-only`
requests a transcript copy. `--a2a LANE` requests notification setup with an
explicit identity. Before a new notified publication, reconcile the old owner.
A native a2a binding or an active or indeterminate terminal binding refuses.
The tmux receiver uses the sanctioned installer to prepare Claude hooks or
Codex hooks for the destination. For Codex, matching project hooks are reused;
otherwise new hooks belong to the managed home. Conflicting bindings refuse.
The provider's SessionStart hook in the new pane owns the exclusive a2a claim.

A newly installed Codex hook may require interactive approval in `/hooks`.
The launcher reads effective trust through Codex app-server `hooks/list` in the target home and workspace. Detached startup prints the attach command and returns a policy diagnostic when required hooks are disabled, missing, conflicting or awaiting approval. The running pane remains available for inspection. Only the user grants approval in the target profile. Provider process
startup, hook trust, association to the specific thread and an idle wake are
separate observations. Qualify the live handoff by checking the provider's
effective hooks and exact binding, then observing a counts-only notice and a
subsequent inbox read on that target thread. Keep an existing wake route until
that check passes. Muse's tmux hook handoff awaits separate qualification.
