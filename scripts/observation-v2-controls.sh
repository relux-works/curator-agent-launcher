#!/usr/bin/env bash
# Plant controls for the observation-v2 launcher consumer.
#
# Each plant weakens exactly one gate. The harness first runs every named
# test on an unmodified scratch copy and requires it to pass (the valid
# control), then applies one plant at a time and requires the named test to
# fail. A plant that does not change its file, or that its named test
# survives, fails the harness.
#
# Everything happens in a scratch copy of the tree; the working tree is never
# modified.
#
# usage: scripts/observation-v2-controls.sh [evidence-dir]
# CONTROLS_ONLY (comma-separated ids) restricts the run to a subset.
set -u
root="$(cd "$(dirname "$0")/.." && pwd)"
out="${1:-$(mktemp -d)}"
mkdir -p "$out"
out="$(cd "$out" && pwd)"
scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
(cd "$root" && tar --exclude=.git --exclude=.temp -cf - .) | (cd "$scratch" && tar -xf -) || exit 1
cd "$scratch" || exit 1

PROTOCOL=internal/sessionmanager/client/protocol.go
TOKEN=internal/sessionmanager/client/process_observation.go
OBSERVE=cmd/curator-run/observe.go
PARSER=internal/cli/session_manager.go
RECONCILE=internal/sessionmanager/client/reconcile_unix.go
ADMISSION=internal/sessionmanager/client/admission_unix.go
WERKSTATT=internal/werkstatt/service_unix.go
CLIENT=./internal/sessionmanager/client
WERKPKG=./internal/werkstatt

# Fields separated by "@@": id, file, package, named test, from, to.
# from/to are literal text; the first occurrence is replaced.
plants=(
  # Remove the token check: the members are admitted from any manager.
  "P1@@$PROTOCOL@@./internal/sessionmanager/client@@TestProcessObservationMembersNeedToken/status-process@@if !admission.ProcessObservation && carriesObservationV2Members(body) {@@if false && carriesObservationV2Members(body) {"
  # Remove the token check at its source: status/1 alone counts as the token.
  "P1b@@$TOKEN@@./cmd/curator-run@@TestObserveV2ExtensionRefusals/no-token-process-v2@@return slices.Contains(d.Capabilities, \"status/1\") && slices.Contains(d.Capabilities, ProcessObservationCapability)@@return slices.Contains(d.Capabilities, \"status/1\")"
  # Remove the registered_at comparison: a re-registered id is accepted.
  "P2@@$OBSERVE@@./cmd/curator-run@@TestObserveV2FrozenBinding/re-registered@@entry.RegisteredAt != command.ExpectRegisteredAt || @@"
  # Strip invalid members instead of refusing them.
  'P3@@'"$PROTOCOL"'@@./cmd/curator-run@@TestObserveV2ExtensionRefusals/malformed-process-v1@@	if managerschema.Validate(schema+".schema.json", body) != nil {@@	if stripped, e := SessionRecordV1(body); e == nil {
		body = stripped
	}
	if managerschema.Validate(schema+".schema.json", body) != nil {'
  # Default to v2.
  "P4@@$PARSER@@./cmd/curator-run@@TestObserveDefaultSchemaIsV1@@c.Schema = \"v1\"@@c.Schema = \"v2\""
  # Drop the frozen generation comparison: a reused handle is accepted.
  "P5@@$PROTOCOL@@./cmd/curator-run@@TestObserveV2FrozenBinding/reused-handle-other-generation@@ || result.Session.Generation != admission.FrozenGeneration@@"
  # Drop the frozen executable comparison: a replaced manager is accepted.
  "P6@@$OBSERVE@@./cmd/curator-run@@TestObserveV2FrozenBinding/changed-executable@@ || (command.ExpectExecutable != \"\" && entry.Executable != command.ExpectExecutable)@@"
  # Skip the v1 projection: v1 output keeps the 1.3.0 members.
  "P7@@$OBSERVE@@./cmd/curator-run@@TestObserveV2CapableAndLegacy/capable-v1-projection-process@@if record, err = client.SessionRecordV1(record); err != nil {@@if false {"
  # Drop the admission the operation's owner carries, so the reconciliation
  # decode after a launch refuses the outcome's process member again.
  "P8@@$RECONCILE@@$CLIENT@@TestLeasedLaunchAdmitsProcessObservation/reconciled-outcome-process@@cl.WithAdmission(frozenAdmission(p, entry))@@cl.WithAdmission(Admission{})"
  # Call recovery on a re-registered manager (routing gate).
  "P9@@$RECONCILE@@$CLIENT@@TestHomeLeaseCrashAndDrift/registration-changed@@ || p.Lease != nil && entry.RegisteredAt != p.Manager.RegisteredAt@@"
  # Ignore status/1 in the token rule, witnessed on a non-observe path.
  "P10@@$TOKEN@@$CLIENT@@TestFrozenAdmission@@return slices.Contains(d.Capabilities, \"status/1\") && slices.Contains(d.Capabilities, ProcessObservationCapability)@@return slices.Contains(d.Capabilities, ProcessObservationCapability)"
  # Read the frozen capability without the owner comparison.
  "P11@@$ADMISSION@@$CLIENT@@TestFrozenAdmission@@if p.Lease == nil || entry.Executable != p.Manager.Executable || entry.RegisteredAt != p.Manager.RegisteredAt {@@if p.Lease == nil {"
  # Remove the cross-manager gate: up admits another manager's owner read
  # under the selected manager's describe.
  "P12@@$WERKSTATT@@$WERKPKG@@TestServiceProcessObservationAdmission/up-owner-other-manager-process@@if l.Manager.ID == managerID {@@if true {"
  # Remove the admission on the inventory remote in up: a capable
  # manager's list rows refuse again.
  'P13@@'"$WERKSTATT"'@@'"$WERKPKG"'@@TestServiceProcessObservationAdmission/up-list-process-termination-admitted@@	admission := client.Admission{ProcessObservation: client.ProcessObservation(*response.Describe)}
	remote = admittedRemote(remote, admission)@@	admission := client.Admission{ProcessObservation: client.ProcessObservation(*response.Describe)}'
  # Admit the down stop decode: a termination member on stop is accepted.
  "P14@@$WERKSTATT@@$WERKPKG@@TestServiceProcessObservationAdmission/down-stop-termination@@call(ctx, s.remote(l.Manager.ID, e), \"stop\"@@call(ctx, admittedRemote(s.remote(l.Manager.ID, e), client.Admission{ProcessObservation: true}), \"stop\""
  # Admit the rebind read: a process member on the rebind status is accepted
  # and the binding is rewritten before the next read.
  "P15@@$WERKSTATT@@$WERKPKG@@TestServiceProcessObservationAdmission/rebind-process@@call(ctx, s.remote(l.Manager.ID, e), \"status\"@@call(ctx, admittedRemote(s.remote(l.Manager.ID, e), client.Admission{ProcessObservation: true}), \"status\""
  # Admit the recycle draining poll: a process member on the poll is
  # accepted and the loop polls once more past the script.
  "P16@@$WERKSTATT@@$WERKPKG@@TestServiceProcessObservationAdmission/recycle-poll-process@@s.ownerRead(pollCtx, reg, l)@@s.ownerReadAdmitted(pollCtx, reg, l, client.Admission{ProcessObservation: true}, l.Manager.ID)"
)

# A requested id that names no plant, or a run that evaluates none, must
# never pass as if every plant had been killed.
ids=""
for p in "${plants[@]}"; do ids="$ids ${p%%@@*}"; done
# Normalise CONTROLS_ONLY once: commas and whitespace separate items, empty
# items drop out and duplicates collapse. The normalised list drives both
# the known-id check above the run and the per-plant filter below, so a
# requested id can never pass the check yet be skipped by the filter.
only=""
only_count=0
if [ -n "${CONTROLS_ONLY:-}" ]; then
  requested=$(printf '%s' "$CONTROLS_ONLY" | tr ',' ' ')
  for want in $requested; do
    found=0
    for id in $ids; do
      if [ "$want" = "$id" ]; then found=1; fi
    done
    if [ "$found" = 0 ]; then
      echo "unknown plant id in CONTROLS_ONLY: $want (known:$ids)" >&2
      exit 1
    fi
    case ",$only," in
      *",$want,"*) ;;
      *)
        if [ -n "$only" ]; then only="$only,$want"; else only="$want"; fi
        only_count=$((only_count + 1))
        ;;
    esac
  done
  if [ -z "$only" ]; then
    echo "no plants evaluated" >&2
    exit 1
  fi
fi

run_named() { # package, named test, log
  local pattern="^${2//\//\$/^}\$"
  go test "$1" -run "$pattern" -count=1 -vet=off -v >>"$3" 2>&1
}

status=0
evaluated=0
summary="$out/summary.tsv"
printf 'plant\tfile\tnamed_test\tcontrol_passed\tapplied\ttest_exit\tnamed_test_failed\tverdict\n' >"$summary"
for p in "${plants[@]}"; do
  id="${p%%@@*}"; rest="${p#*@@}"
  if [ -n "$only" ] && ! printf ',%s,' "$only" | grep -q ",$id,"; then continue; fi
  evaluated=$((evaluated + 1))
  file="${rest%%@@*}"; rest="${rest#*@@}"
  pkg="${rest%%@@*}"; rest="${rest#*@@}"
  test="${rest%%@@*}"; rest="${rest#*@@}"
  from="${rest%%@@*}"; to="${rest#*@@}"
  log="$out/$id.log"
  : >"$log"

  # Valid control: the named test passes on the unmodified copy.
  cp "$root/$file" "$file"
  echo "== $id control (expected to pass: $test)" >>"$log"
  control=no
  if run_named "$pkg" "$test" "$log" && grep -Eq "^[[:space:]]*--- PASS: ${test}( |\$)" "$log"; then control=yes; fi
  if [ "$control" = no ]; then
    printf '%s\t%s\t%s\tno\t-\t-\t-\tCONTROL_FAILED\n' "$id" "$file" "$test" >>"$summary"; status=1; continue
  fi

  FROM="$from" TO="$to" python3 -c '
import os, sys
p = sys.argv[1]; src = open(p).read()
f, t = os.environ["FROM"], os.environ["TO"]
if src.count(f) == 1:
    open(p, "w").write(src.replace(f, t, 1))
' "$file"
  if cmp -s "$file" "$root/$file"; then
    printf '%s\t%s\t%s\tyes\tno\t-\t-\tNOT_APPLIED\n' "$id" "$file" "$test" >>"$summary"; status=1; continue
  fi
  { echo; echo "== $id plant (expected to fail: $test)"; diff -u "$root/$file" "$file"; echo; } >>"$log"
  planted="$out/$id.plant.log"
  : >"$planted"
  run_named "$pkg" "$test" "$planted"; code=$?
  cat "$planted" >>"$log"
  failed=no
  grep -Eq "^[[:space:]]*--- FAIL: ${test}( |\$)" "$planted" && failed=yes
  rm -f "$planted"
  verdict=KILLED
  if [ "$code" -eq 0 ] || [ "$failed" = no ]; then verdict=SURVIVED; status=1; fi
  printf '%s\t%s\t%s\tyes\tyes\t%s\t%s\t%s\n' "$id" "$file" "$test" "$code" "$failed" "$verdict" >>"$summary"
  cp "$root/$file" "$file"
done
if [ "$evaluated" -eq 0 ]; then
  echo "no plants evaluated" >&2
  status=1
fi
if [ -n "$only" ] && [ "$evaluated" -ne "$only_count" ]; then
  echo "evaluated $evaluated plants, $only_count distinct ids requested" >&2
  status=1
fi
column -t -s $'\t' "$summary" 2>/dev/null || cat "$summary"
echo "evidence: $out"
exit "$status"
