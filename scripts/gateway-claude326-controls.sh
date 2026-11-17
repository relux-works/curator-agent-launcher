#!/usr/bin/env bash
# Plant controls for the Claude Code 2.1.326 gateway admission changes.
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
# usage: scripts/gateway-claude326-controls.sh [evidence-dir]
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

NATIVE=internal/network/native_args.go
GUARD=internal/network/settings_guard.go
NETPKG=./internal/network

# Fields separated by "@@": id, file, package, named test, from, to.
# from/to are literal text; the first occurrence is replaced.
plants=(
  # Unreserve the whole-argv trust token: it passes as a value again.
  "G1@@$NATIVE@@$NETPKG@@TestPrepareGateway326DispatchWords/ignore-trust_token_as_a_required_value@@--ignore-persisted-trust --managed-settings-in-initialize@@--managed-settings-in-initialize"
  # Unreserve workspace-trust: the command word passes as a prompt again.
  "G2@@$NATIVE@@$NETPKG@@TestPrepareGateway326DispatchWords/workspace-trust_as_the_prompt@@worktree-hook workspace-trust@@worktree-hook"
  # Unreserve worktree-hook: the command word passes as a prompt again.
  "G2b@@$NATIVE@@$NETPKG@@TestPrepareGateway326DispatchWords/worktree-hook_as_the_prompt@@worktree-hook workspace-trust@@workspace-trust"
  # Unreserve the settings-source token: it passes after -- again.
  "G3@@$NATIVE@@$NETPKG@@TestPrepareGateway326DispatchWords/managed-settings-in-initialize_after_--@@--ignore-persisted-trust --managed-settings-in-initialize@@--ignore-persisted-trust"
  # Admit the trust selector in the closed namespace: a supplied value passes.
  "G4@@$GUARD@@$NETPKG@@TestPrepareGatewayIgnorePersistedTrustEnv/supplied_in_fragment@@\"CLAUDE_CODE_OAUTH_TOKEN\":              true,@@\"CLAUDE_CODE_OAUTH_TOKEN\":              true, \"CLAUDE_CODE_IGNORE_PERSISTED_TRUST\": true,"
  # Never refuse the case-variant listing: the guard reads the legacy file
  # while the 2.1.326 chooser reads the new one.
  "G5@@$GUARD@@$NETPKG@@TestPrepareGatewayGlobalFileCaseVariant/case-variant_listing_refuses@@if err != nil || !exact {@@if err != nil && !exact {"
  # Never select a non-empty legacy file: its route-bearing env goes unchecked.
  "G6@@$GUARD@@$NETPKG@@TestPrepareGatewayGlobalFile326Cases/route_in_the_selected_legacy_file,_no_new_file@@if info.Size() > 0 {@@if false && info.Size() > 0 {"
  # Admit the catalog-URL selector in the closed namespace: a supplied
  # file: URL reaches the child. Beside the owned off it cannot fetch
  # (env_off precedes the file: bypass; record claim 6), but the
  # selector must stay closed if the pair ever changes.
  "G7@@$GUARD@@$NETPKG@@TestPrepareGatewayModelCatalogURLEnv/supplied_in_fragment@@\"CLAUDE_CODE_OAUTH_TOKEN\":              true,@@\"CLAUDE_CODE_OAUTH_TOKEN\":              true, \"CLAUDE_CODE_MODEL_CATALOG_URL\": true,"
  # Make the production listing check answer listed for any entry: a
  # case-variant legacy name then passes as exact, and the listing
  # refusal can never fire through the real function.
  "G8@@$GUARD@@$NETPKG@@TestExactLegacyListedReal/not_listed_under_exact_name@@if entry.Name() == base {@@if entry.Name() != \"\" {"
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
