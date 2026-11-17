#!/usr/bin/env bash
# Plant controls for the home-lease bind consumer.
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
# usage: scripts/home-lease-bind-controls.sh [evidence-dir]
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

RECONCILE=internal/sessionmanager/client/reconcile_unix.go
HOMELEASE=internal/sessionmanager/client/homelease_unix.go
CLIENT=./internal/sessionmanager/client

# Fields separated by "@@": id, file, package, named test, from, to.
# from/to are literal text; the first occurrence is replaced.
plants=(
  # Bind without an admitted tuple: drop the frozen token check, so a tuple
  # from an unadmitted decode binds.
  "B1@@$RECONCILE@@$CLIENT@@TestBindNoAdmittedTuple/fake-process-without-token@@ProcessObservation(Describe{Capabilities: p.Lease.Capabilities})@@true"
  # Mutate one tuple byte before bind: change the retained pid.
  "B2@@$HOMELEASE@@$CLIENT@@TestBindDirectLaunchExactHolder/linux-stat-starttime@@h.Process = process@@h.Process = json.RawMessage(bytes.Replace([]byte(process), []byte(\"4242\"), []byte(\"4243\"), 1))"
  # Bind with a fresh revision instead of the acquired one.
  "B3@@$HOMELEASE@@$CLIENT@@TestBindDirectLaunchExactHolder/linux-stat-starttime@@strconv.FormatInt(ifRevision, 10)@@strconv.FormatInt(ifRevision+1, 10)"
  # Send a release after a successful bind.
  "B4@@$HOMELEASE@@$CLIENT@@TestBindNeverReleases/success-no-release@@return r, nil@@_, _ = c.call(ctx, \"release\", \"--lease\", leaseID); return r, nil"
  # Settle a bound lease on a non-started outcome: forget the bound record.
  "B5@@$RECONCILE@@$CLIENT@@TestBindLostReplySettledViaAcquireReplay/bound-failed-no-settle@@return p.Lease != nil && p.Lease.Record != nil && isBoundHolder(p.Lease.Record.Holder)@@return false"
  # Report a bound started outcome with no process as a tuple mismatch.
  "B6@@$RECONCILE@@$CLIENT@@TestBindLostReplySettledViaAcquireReplay/bound-started-no-process-unavailable@@case !admitted:@@case false:"
  # Bind with a prepared replay revision that differs from the journaled one.
  "B7@@$HOMELEASE@@$CLIENT@@TestBindPreparedReplayRevisionGuard/prepared-replay-revision-differs@@if l.Record != nil && r.Revision != l.Record.Revision {@@if false {"
  # Label the local holder-mismatch refusal as a Curator result again.
  "B8@@$HOMELEASE@@$CLIENT@@TestBindLostReplySettledViaAcquireReplay/process-mutated@@\"source\": \"launcher\"@@\"curator_code\": \"environment_lease_holder_mismatch\""
  # Retry a refused bind within the same invocation.
  "B9@@$RECONCILE@@$CLIENT@@TestBindInvalidHolderOneBindPerInvocation/invalid-holder-once-per-invocation@@if _, err := r.leaseCLI().Bind(ctx, p, bound); err != nil {@@if _, err := r.leaseCLI().Bind(ctx, p, bound); err != nil { _, _ = r.leaseCLI().Bind(ctx, p, bound)"
  # Accept an unknown-phase bound replay instead of the protocol error.
  "B10@@$HOMELEASE@@$CLIENT@@TestBindLostReplySettledViaAcquireReplay/bound-unknown-phase-protocol-error@@if r.Phase == \"active\" && isBoundHolder(r.Holder) && preparedMembersEqual(l.Holder, r.Holder) {@@if (r.Phase == \"active\" || r.Phase == \"unknown\") && isBoundHolder(r.Holder) && preparedMembersEqual(l.Holder, r.Holder) {"
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
