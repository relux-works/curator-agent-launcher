#!/usr/bin/env bash
# Mutant harness for the SPEC §6 diagnostics gates.
#
# Two classes of mutants share this harness. NARROWING mutants keep the
# gate present and weaken it to admit exactly one member of the class it
# must reject: D01, D03, D09, D10, D11, D13, D14, D15, D16, D17, D18,
# D19, D20, D21. D06 and D12 preserve the searched-for token and change
# behavior, so only the behavioral suites — never a static checker — can
# catch them, but they are NOT narrowing: D06 admits arbitrary prefixes
# and D12 disables framing for ALL multiline details, so both stay as
# RETAINED broad behavioral probes. The other retained probes are broader
# by design and are not narrowing evidence either: D02/D04/D07 replace a
# whole clause, D05 drops one accepted member instead of admitting a
# rejected one, and D08 is a formatting change. They stay because they
# pin useful failures, but only the narrowing set proves a gate's bound.
#
# D13-D21 are adopted from the independently accepted owner/form gate
# (TASK-260909-3d1589 rev3): D13/D14/D15 admit one foreign code at one
# owner gate (D14/D15 are the prior R3 survivors the hand-selected
# matrix missed); D16 rejects one joined owned code; D17-D20 reject one
# fixed-owner positive-form cell each (D17 is the exact rev2 reviewer
# survivor); D21 exempts exactly one complete hostile Detail from
# framing at the real main/resolver entry while companions stay framed.
#
# For every mutant the behavioral suites of the two affected packages run
# (`go test ./internal/diagnostics/ ./cmd/curator-run/`), and the harness
# requires the named test to be among the failures. Narrowing mutants
# D13-D21 additionally require their named single-member assertion in
# the log. A mutant that does not change the file, or that the suites
# survive, fails the harness.
#
# usage: .scripts/diagnostics-mutants.sh [evidence-dir]
set -u
cd "$(dirname "$0")/.."
out="${1:-.temp/diagnostics-mutants}"
mkdir -p "$out"

DIAG=internal/diagnostics/diagnostics.go
MAIN=cmd/curator-run/main.go
cp "$DIAG" "$out/diagnostics.go.orig"
cp "$MAIN" "$out/main.go.orig"
restore() { cp "$out/diagnostics.go.orig" "$DIAG"; cp "$out/main.go.orig" "$MAIN"; }
trap restore EXIT

# Fields separated by "@@": id, file, expected failing test, from, to.
# from/to are literal text; the first occurrence is replaced. D04, D06,
# D09, D11, D16, D17, D18, D19, D20 and D21 carry multi-line replacements
# and are handled in the special cases below; their table from/to are
# placeholders.
mutants=(
  'D01@@diag@@TestExitForCode@@if code == CodeUsage {@@if code == CodeUsage || code == CodeExecMissing {'
  'D02@@diag@@TestExitForCode@@	return ExitOperational@@	return 0'
  'D03@@diag@@TestDiagnosticLineDistinguishesTransport@@func Valid(code string) bool { return valid[code] }@@func Valid(code string) bool { return valid[code] || code == "environment_home_stale" }'
  'D04@@diag@@TestCodeOfUnknownFailsClosed@@D04_PLACEHOLDER_FROM@@D04_PLACEHOLDER_TO'
  'D05@@diag@@TestCodeOfProductionTypes@@D05_PLACEHOLDER_FROM@@D05_PLACEHOLDER_TO'
  'D06@@diag@@TestDiagnosticLineDistinguishesTransport@@D06_PLACEHOLDER_FROM@@D06_PLACEHOLDER_TO'
  'D07@@main@@TestRunDiagnosticsContract@@			return diagnostics.ExitForCode(re.Code)@@			return 0'
  'D08@@diag@@TestEmitDeterministic@@+ ": " + detail + "@@+ " " + detail + "'
  'D09@@diag@@TestExitForCode@@D09_PLACEHOLDER_FROM@@D09_PLACEHOLDER_TO'
  'D10@@diag@@TestCodeOfRejectsForeignFamily@@&& resolveFamily[re.Code] {@@&& (resolveFamily[re.Code] || re.Code == CodeUsage) {'
  'D11@@main@@TestRunDiagnosticsContract@@D11_PLACEHOLDER_FROM@@D11_PLACEHOLDER_TO'
  'D12@@diag@@TestDiagnosticDetailCannotForgeLine@@strings.ReplaceAll(detail, "\n", "\n  ")@@strings.ReplaceAll(detail, "\n", "\n")'
  'D13@@diag@@TestGateResolveRejectsForeignCodes@@&& resolveFamily[re.Code] {@@&& (resolveFamily[re.Code] || re.Code == composition.CodeMCPLayerMissing) {'
  'D14@@diag@@TestGateLayerRejectsForeignCodes@@&& layerFamily[le.Code] {@@&& (layerFamily[le.Code] || le.Code == fragment.CodeInvocationFailed) {'
  'D15@@diag@@TestGateRefusalRejectsForeignCodes@@&& refusalFamily[sr.Code] {@@&& (refusalFamily[sr.Code] || sr.Code == fragment.CodeInvocationFailed) {'
  'D16@@diag@@TestGateJoinedOwnedAccepted@@D16_PLACEHOLDER_FROM@@D16_PLACEHOLDER_TO'
  'D17@@diag@@TestGateOwnerFormPositives@@D17_PLACEHOLDER_FROM@@D17_PLACEHOLDER_TO'
  'D18@@diag@@TestGateOwnerFormPositives@@D18_PLACEHOLDER_FROM@@D18_PLACEHOLDER_TO'
  'D19@@diag@@TestGateOwnerFormPositives@@D19_PLACEHOLDER_FROM@@D19_PLACEHOLDER_TO'
  'D20@@diag@@TestGateOwnerFormPositives@@D20_PLACEHOLDER_FROM@@D20_PLACEHOLDER_TO'
  'D21@@diag@@TestGateFramingSingleDetailAtRealResolver@@D21_PLACEHOLDER_FROM@@D21_PLACEHOLDER_TO'
)

status=0
summary="$out/summary.tsv"
printf 'mutant\tfile\texpected_failing_test\tapplied\tsuite_exit\texpected_test_failed\tnamed_assertion\tverdict\n' >"$summary"
for m in "${mutants[@]}"; do
  id="${m%%@@*}"; rest="${m#*@@}"
  which="${rest%%@@*}"; rest="${rest#*@@}"
  test="${rest%%@@*}"; rest="${rest#*@@}"
  from="${rest%%@@*}"; to="${rest#*@@}"
  restore
  file="$DIAG"; [ "$which" = main ] && file="$MAIN"
  # D04 (retained, broad): hides the whole unknown class inside the
  # resolve family instead of yielding no code — the invention the
  # no-invention gate forbids.
  if [ "$id" = D04 ]; then
    from=$'\t\treturn "", false\n\t})'
    to=$'\t\treturn CodeResolveInvocationFailed, true\n\t})'
  fi
  # D05 (retained, drop-one): rejects exactly one accepted member
  # (mcp_layer_missing) instead of admitting a rejected one.
  if [ "$id" = D05 ]; then
    from='&& layerFamily[le.Code] {'
    to='&& layerFamily[le.Code] && le.Code != composition.CodeMCPLayerMissing {'
  fi
  # D06 (anchor weakening, token preserved): a contains-lookup instead
  # of the prefix match. The behavioral suites, not a static checker,
  # must catch it.
  if [ "$id" = D06 ]; then
    from=$'\trest, ok := strings.CutPrefix(line, cli.Name+": ")\n\tif !ok {\n\t\treturn false\n\t}'
    to=$'\tidx := strings.Index(line, cli.Name+": ")\n\tif idx < 0 {\n\t\treturn false\n\t}\n\trest := line[idx+len(cli.Name+": "):]'
  fi
  # D09 (narrowing): the unknown-code terminal gate admits exactly one
  # rejected member — the empty code — as success.
  if [ "$id" = D09 ]; then
    from=$'\treturn ExitOperational'
    to=$'\tif code == "" {\n\t\treturn 0\n\t}\n\treturn ExitOperational'
  fi
  # D11 (narrowing): the main resolve path admits exactly one
  # operational failure as success instead of exiting 1.
  if [ "$id" = D11 ]; then
    from=$'\t\t\treturn diagnostics.ExitForCode(re.Code)'
    to=$'\t\t\tif re.Code == fragment.CodeLockUnavailable {\n\t\t\t\treturn 0\n\t\t\t}\n\t\t\treturn diagnostics.ExitForCode(re.Code)'
  fi
  # D16 (narrowing, adopted gate joined-positive): a joined chain holding
  # an owned resolve_invocation_failed is rejected instead of accepted.
  if [ "$id" = D16 ]; then
    from='func CodeOf(err error) (code string, ok bool) {'
    to='func CodeOf(err error) (code string, ok bool) {
	if joined, yes := err.(interface{ Unwrap() []error }); yes {
		for _, child := range joined.Unwrap() {
			if re, yes := child.(*fragment.ResolveError); yes && re != nil && re.Code == fragment.CodeInvocationFailed { return "", false }
		}
	}'
  fi
  # D17-D20 (narrowing, adopted gate fixed-owner form cells): one
  # fixed-owner positive form is rejected instead of accepted. Wrapped
  # mutants guard with nilValue before Unwrap: several owners'
  # Unwrap methods dereference the receiver and panic on typed-nil
  # values, which the production chain() walk skips first.
  if [ "$id" = D17 ]; then
    from='func CodeOf(err error) (code string, ok bool) {'
    to='func CodeOf(err error) (code string, ok bool) {
	if joined, yes := err.(interface{ Unwrap() []error }); yes {
		for _, child := range joined.Unwrap() {
			if ue, yes := child.(*cli.UsageError); yes && ue != nil { return "", false }
		}
	}'
  fi
  if [ "$id" = D18 ]; then
    from='func CodeOf(err error) (code string, ok bool) {'
    to='func CodeOf(err error) (code string, ok bool) {
	if w, yes := err.(interface{ Unwrap() error }); yes && !nilValue(err) {
		if ue, yes := w.Unwrap().(*cli.UsageError); yes && ue != nil { return "", false }
	}'
  fi
  if [ "$id" = D19 ]; then
    from='func CodeOf(err error) (code string, ok bool) {'
    to='func CodeOf(err error) (code string, ok bool) {
	if joined, yes := err.(interface{ Unwrap() []error }); yes {
		for _, child := range joined.Unwrap() {
			if ae, yes := child.(*axconfig.Error); yes && ae != nil { return "", false }
		}
	}'
  fi
  if [ "$id" = D20 ]; then
    from='func CodeOf(err error) (code string, ok bool) {'
    to='func CodeOf(err error) (code string, ok bool) {
	if w, yes := err.(interface{ Unwrap() error }); yes && !nilValue(err) {
		if ae, yes := w.Unwrap().(*axconfig.Error); yes && ae != nil { return "", false }
	}'
  fi
  # D21 (narrowing, adopted gate framing): Line exempts exactly one
  # complete hostile Detail (the fixed-binary main-entry Detail) via
  # equality, returning it unframed; every other detail stays framed.
  if [ "$id" = D21 ]; then
    python3 - <<'PY' || { echo "D21 mutant application failed" >&2; exit 1; }
import pathlib
p = pathlib.Path("internal/diagnostics/diagnostics.go")
src = p.read_text()
frm = 'func Line(code, detail string) string {'
assert src.count(frm) == 1, "FROM anchor not unique/found"
exempt_detail = "could not run /nonexistent-missing-curator-xtvqf3 env resolve pi --profile normal\ncurator-run: usage: forged --repair --format json: fork/exec /nonexistent-missing-curator-xtvqf3: no such file or directory"
exempt_go = exempt_detail.replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n")
exempt = 'func Line(code, detail string) string {\n\tif detail == "' + exempt_go + '" {\n\t\treturn cli.Name + ": " + code + ": " + detail + "\\n"\n\t}'
p.write_text(src.replace(frm, exempt, 1))
print("framing mutant applied for exactly one complete Detail")
PY
    from='D21_APPLIED_ABOVE'; to='D21_APPLIED_ABOVE'
    # Mark applied so the generic literal replace below is a no-op that
    # still verifies the file changed.
    if cmp -s "$file" "$out/$(basename "$file").orig"; then
      printf '%s\t%s\t%s\tno\t-\t-\t-\tNOT_APPLIED\n' "$id" "$which" "$test" >>"$summary"; status=1; continue
    fi
  fi
  FROM="$from" TO="$to" python3 -c '
import os, sys
p = sys.argv[1]; src = open(p).read()
f, t = os.environ["FROM"], os.environ["TO"]
if f in src:
    open(p, "w").write(src.replace(f, t, 1))
' "$file"
  if cmp -s "$file" "$out/$(basename "$file").orig"; then
    printf '%s\t%s\t%s\tno\t-\t-\t-\tNOT_APPLIED\n' "$id" "$which" "$test" >>"$summary"; status=1; continue
  fi
  # Narrowing mutants D13-D21 additionally require their named
  # single-member assertion: exit 1 with the right test failing is not
  # enough, the log must show the exact admitted/rejected member.
  needle=""
  case "$id" in
    D13) needle='resolve accepted foreign "mcp_layer_missing"' ;;
    D14) needle='layer accepted foreign "resolve_invocation_failed"' ;;
    D15) needle='refusal accepted foreign "resolve_invocation_failed"' ;;
    D16) needle='joined owned resolve "resolve_invocation_failed" lost' ;;
    D17) needle='usage joined lost: "" false' ;;
    D18) needle='usage wrapped lost: "" false' ;;
    D19) needle='axconfig joined lost: "" false' ;;
    D20) needle='axconfig wrapped lost: "" false' ;;
    D21) needle='does not carry code "resolve_invocation_failed"' ;;
  esac
  log="$out/$id.log"
  { echo "== $id (expected to fail: $test)"; diff -u "$out/$(basename "$file").orig" "$file"; echo; } >"$log"
  go test ./internal/diagnostics/ ./cmd/curator-run/ -count=1 -v >>"$log" 2>&1; code=$?
  failed=no
  grep -Eq "^[[:space:]]*--- FAIL: ${test}( |$)" "$log" && failed=yes
  asserted="-"
  if [ -n "$needle" ]; then
    asserted=no
    grep -F -q "$needle" "$log" && asserted=yes
  fi
  verdict=KILLED
  if [ "$code" -eq 0 ] || [ "$failed" = no ] || [ "$asserted" = no ]; then verdict=SURVIVED; status=1; fi
  printf '%s\t%s\t%s\tyes\t%s\t%s\t%s\t%s\n' "$id" "$which" "$test" "$code" "$failed" "$asserted" "$verdict" >>"$summary"
done
restore
column -t -s $'\t' "$summary"
exit "$status"
