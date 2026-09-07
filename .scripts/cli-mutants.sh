#!/usr/bin/env bash
# Narrowing-mutant harness for the SPEC §3 CLI gates.
#
# Each mutant keeps the gate present and weakens it to admit exactly one
# member of the class it must reject (or, for the exit-code mutants,
# misreports one outcome). For every mutant the whole behavioral suite runs
# (`go test ./...`), and the harness requires the named test to be among the
# failures. A mutant that does not change the file, or that the suite
# survives, fails the harness.
#
# usage: .scripts/cli-mutants.sh [evidence-dir]
set -u
cd "$(dirname "$0")/.."
out="${1:-.temp/cli-mutants}"
mkdir -p "$out"

CLI=internal/cli/cli.go
MAIN=cmd/curator-run/main.go
cp "$CLI" "$out/cli.go.orig"
cp "$MAIN" "$out/main.go.orig"
restore() { cp "$out/cli.go.orig" "$CLI"; cp "$out/main.go.orig" "$MAIN"; }
trap restore EXIT

# Fields separated by "@@": id, file, expected failing test, from, to.
# from/to are literal text; the first occurrence is replaced.
mutants=(
  'M01@@cli@@TestParseRejected/repeated_profile@@if seen[flag] {@@if seen[flag] && flag != "--profile" {'
  'M02@@cli@@TestParseRejected/unknown_flag@@if !valueFlags[flag] {@@if !valueFlags[flag] { if tok == "--nope" { continue }'
  'M03@@cli@@TestParseRejected/missing_value_empty_string@@if value == "" || strings.HasPrefix(value, "-") {@@if strings.HasPrefix(value, "-") {'
  'M04@@cli@@TestParseRejected/stray_operand@@if envSet {@@if envSet && tok != "resume" {'
  'M05@@cli@@TestParseRejected/system_prompt_bad_value@@case SystemPromptAppend, SystemPromptReplace:@@case SystemPromptAppend, SystemPromptReplace, "prepend":'
  'M06@@cli@@TestParseRejected/ax_profile_bad_value_tracked@@case AxProfileStandard, AxProfileYolo:@@case AxProfileStandard, AxProfileYolo, "unsafe":'
  'M07@@cli@@TestParseRejected/ax_profile_on_untracked_machine@@if !opts.AxConfigured {@@if !opts.AxConfigured && value != "standard" {'
  'M08@@cli@@TestParseRejected/name_65_chars@@{0,63}$`)@@{0,64}$`)'
  'M09@@cli@@TestParseRejected/name_leading_dot@@`^[A-Za-z0-9][A-Za-z0-9._-]@@`^[A-Za-z0-9.][A-Za-z0-9._-]'
  'M10@@cli@@TestParseRejected/no_arguments@@if !envSet {@@if !envSet && len(args) > 0 {'
  'M11@@cli@@TestNativeTailVerbatim@@inv.Native = append([]string{}, args[i:]...)@@inv.Native = []string{}; for _, a := range args[i:] { if a != "" { inv.Native = append(inv.Native, a) } }'
  'M12@@cli@@TestParseRejected/unknown_flag_before_help@@return inv, usageErr("unknown flag %q before --", tok)@@if tok != "--nope" { return inv, usageErr("unknown flag %q before --", tok) }; continue'
  'M13@@main@@TestRunUsageErrorsExit2@@return cli.ExitCode@@return 1'
  'M14@@main@@TestRunParsedLaunchResolvesThenRefuses@@name, frag.Environment, frag.Profile.Name, frag.Home(), frag.Digest)
	return 1@@name, frag.Environment, frag.Profile.Name, frag.Home(), frag.Digest)
	return 0'
)

status=0
summary="$out/summary.tsv"
printf 'mutant\tfile\texpected_failing_test\tapplied\tsuite_exit\texpected_test_failed\tverdict\n' >"$summary"
for m in "${mutants[@]}"; do
  id="${m%%@@*}"; rest="${m#*@@}"
  which="${rest%%@@*}"; rest="${rest#*@@}"
  test="${rest%%@@*}"; rest="${rest#*@@}"
  from="${rest%%@@*}"; to="${rest#*@@}"
  restore
  file="$CLI"; [ "$which" = main ] && file="$MAIN"
  FROM="$from" TO="$to" python3 -c '
import os, sys
p = sys.argv[1]; src = open(p).read()
f, t = os.environ["FROM"], os.environ["TO"]
if f in src:
    open(p, "w").write(src.replace(f, t, 1))
' "$file"
  if cmp -s "$file" "$out/$(basename "$file").orig"; then
    printf '%s\t%s\t%s\tno\t-\t-\tNOT_APPLIED\n' "$id" "$which" "$test" >>"$summary"; status=1; continue
  fi
  log="$out/$id.log"
  { echo "== $id (expected to fail: $test)"; diff -u "$out/$(basename "$file").orig" "$file"; echo; } >"$log"
  go test ./... -count=1 -v >>"$log" 2>&1; code=$?
  failed=no
  grep -Eq "^[[:space:]]*--- FAIL: ${test}( |$)" "$log" && failed=yes
  verdict=KILLED
  if [ "$code" -eq 0 ] || [ "$failed" = no ]; then verdict=SURVIVED; status=1; fi
  printf '%s\t%s\t%s\tyes\t%s\t%s\t%s\n' "$id" "$which" "$test" "$code" "$failed" "$verdict" >>"$summary"
done
restore
column -t -s $'\t' "$summary"
exit "$status"
