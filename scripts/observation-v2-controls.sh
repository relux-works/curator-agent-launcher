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
)

run_named() { # package, named test, log
  local pattern="^${2//\//\$/^}\$"
  go test "$1" -run "$pattern" -count=1 -vet=off -v >>"$3" 2>&1
}

status=0
summary="$out/summary.tsv"
printf 'plant\tfile\tnamed_test\tcontrol_passed\tapplied\ttest_exit\tnamed_test_failed\tverdict\n' >"$summary"
for p in "${plants[@]}"; do
  id="${p%%@@*}"; rest="${p#*@@}"
  if [ -n "${CONTROLS_ONLY:-}" ] && ! printf ',%s,' "$CONTROLS_ONLY" | grep -q ",$id,"; then continue; fi
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
column -t -s $'\t' "$summary" 2>/dev/null || cat "$summary"
echo "evidence: $out"
exit "$status"
