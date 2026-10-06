#!/usr/bin/env bash
# Narrowing-mutant harness for the SPEC §4.8/§4.9 hosted gates.
#
# Each mutant keeps the gate present and weakens it to admit exactly one
# member of the class it must reject (or, for the fallback and probe
# mutants, reroutes one outcome). For every mutant the whole behavioral
# suite runs (`go test ./...`), and the harness requires the named test to
# be among the failures. The harness fails closed: a mutant whose anchor
# matches no source (the production text drifted) is reported as
# NOT_APPLIED by name on stderr and in the summary, and a mutant the suite
# survives is SURVIVED; either makes the script exit non-zero.
# internal/mutantguard runs the required mutants (M-H1, M-H2, M-H3) through
# this script in a scratch copy of the tree on every `go test ./...`.
#
# usage: .scripts/hosted-mutants.sh [evidence-dir]
# MUTANT_PKGS (go test package patterns, default ./...) bounds the suite a
# mutant runs against; the named test must live in those packages.
# MUTANT_RUN (a go test -run mask) narrows it further to the named test.
# MUTANTS_ONLY (comma-separated ids) restricts the run to a subset for
# bounded sequential batches; unset runs every mutant.
set -u
cd "$(dirname "$0")/.."
out="${1:-.temp/hosted-mutants}"
mkdir -p "$out"

CLI=internal/cli/cli.go
MAIN=cmd/curator-run/main.go
DEFAULTS=internal/defaults/defaults.go
PERM=internal/execution/permission.go
PAYLOAD=internal/hosted/payload.go
STATUSF=internal/hosted/status.go
cp "$CLI" "$out/cli.go.orig"
cp "$MAIN" "$out/main.go.orig"
cp "$DEFAULTS" "$out/defaults.go.orig"
cp "$PERM" "$out/permission.go.orig"
cp "$PAYLOAD" "$out/payload.go.orig"
cp "$STATUSF" "$out/status.go.orig"
restore() { cp "$out/cli.go.orig" "$CLI"; cp "$out/main.go.orig" "$MAIN"; cp "$out/defaults.go.orig" "$DEFAULTS"; cp "$out/permission.go.orig" "$PERM"; cp "$out/payload.go.orig" "$PAYLOAD"; cp "$out/status.go.orig" "$STATUSF"; }
trap restore EXIT

# Fields separated by "@@": id, file, expected failing test, from, to.
# from/to are literal text; the first occurrence is replaced.
mutants=(
  'M-H1@@defaults@@TestResolveHostPrecedence/locked-ignores-operator@@lockedHost := f.machine.locked && named@@lockedHost := f.machine.locked && named && false'
  'M-H2@@main@@TestHostedRefusalsBeforeContact/receiver-missing@@return hosted.Transport{Lookup: deps.receiverLookup, Terminal: deps.receiverTerminal, PreopenedTerminal: preopenedTTY}.Run(wire, stdout, stderr)@@code := hosted.Transport{Lookup: deps.receiverLookup, Terminal: deps.receiverTerminal, PreopenedTerminal: preopenedTTY}.Run(wire, stdout, stderr); if code != diagnostics.ExitOperational { return code }; return runNative(inv, frag, target, value, decision, inspection, stdout, stderr, deps, boundary, at)'
  'M-H3@@main@@TestNativePerformsZeroReceiverLookup@@var nativePolicy *execution.EffectiveNativePolicy@@var nativePolicy *execution.EffectiveNativePolicy; if deps.receiverLookup != nil { _, _ = deps.receiverLookup() }'
  'M-H4@@main@@TestHostedRefusalsBeforeContact/machine-hosted-default-not-ready@@if isHosted && host.Origin == defaults.OriginHostMachine {@@if isHosted && host.Origin == defaults.OriginHostOperator {'
  'M-H5@@main@@TestHostedRefusalsBeforeContact/network-direct@@if isHosted && inv.NetworkSet {@@if isHosted && inv.NetworkSet && inv.Network != "direct" {'
  'M-H6@@defaults@@TestResolveHostPrecedence/locked-equal-hosted-flag-still-refuses@@if flag.Present && machine.Host.Present {@@if flag.Present && machine.Host.Present && hostFlag != "--hosted" {'
  'M-H7@@cli@@TestParseRejected/ax_profile_with_untracked_flag@@if inv.AxProfile != AxProfileNone && ExplicitNative(inv.HostFlag) {@@if inv.AxProfile != AxProfileNone && inv.HostFlag == "--native" {'
  'M-H8@@main@@TestHostedRefusalsBeforeContact/provider-pi@@if isHosted && frag.Environment != fragment.EnvClaudeCode {@@if isHosted && frag.Environment != fragment.EnvClaudeCode && frag.Environment != fragment.EnvPi {'
  'M-H9@@perm@@TestQd3UnconfiguredDefaultIsYolo@@case tracked:@@case tracked || req.Hosted:'
  'M-H10@@perm@@TestHostedSuccessEndToEnd/permission-sources/flag-yolo@@if decision.Mode == agentic.PermissionModeYolo && tracked {@@if decision.Mode == agentic.PermissionModeYolo && (tracked || req.Hosted) {'
  'M-H11@@perm@@TestQd3UnconfiguredDefaultIsYolo@@case tracked:@@case tracked || req.Headless:'
  'M-H12@@status@@TestNormalizeStatusRecord@@if haveField && !statusD1Fields[field] {@@if haveField && !statusD1Fields[field] && false {'
  'M-H13@@main@@TestHostedPipedStdinRefusesBeforeBuild/hosted-refuses-with-zero-builds@@if !terminal() {@@if !terminal() && inv.ResumeRequested {'
  'M-H14@@payload@@TestHostedEnvNamesMaxItems@@if len(names) > 64 {@@if len(names) > 65 {'
  'M-H15@@main@@TestHostedConflictDiagnosticCarriesNoValues@@return emitFailure(stderr, diagnostics.CodePlanRefused, fmt.Errorf("native arguments conflict with fragment channel %s: %s", conflict.Channel, conflict.Reason))@@return emitFailure(stderr, diagnostics.CodePlanRefused, fmt.Errorf("native arguments %q conflict with fragment channel %s: %s", tail, conflict.Channel, conflict.Reason))'
  'M-H17@@main@@TestHostedParityWithFakeReceiver/rc-alone@@hosted.SessionFromPlan(base.Session, base.Argv, value.Argv)@@hosted.SessionFromPlan(nil, base.Argv, value.Argv)'
  'M-H18@@payload@@TestHostedParityWithFakeReceiver/rc-alone@@out.RCIndices = append(out.RCIndices, index)@@_ = index'
  'M-H19@@payload@@TestSessionFromPlan/composed-inserts-tokens@@if !slices.Equal(planArgv, composedArgv) {@@if len(planArgv) > len(composedArgv) {'
  'M-H20@@payload@@TestHostedParityWithFakeReceiver/name-alone@@if session.Name != nil {@@if session.Name != nil && false {'
  'M-H21@@main@@TestNativeHostFlagsComposeWithNetwork@@if isHosted && inv.NetworkSet {@@if inv.NetworkSet {'
  'M-H16@@payload@@TestBuildPayloadEncodedLimits/session-projection@@return projection.NativeName, projection.RCEnabled, out, nil@@return projection.NativeName, projection.RCEnabled, []int{}, nil'
)

status=0
summary="$out/summary.tsv"
printf 'mutant\tfile\texpected_failing_test\tapplied\tsuite_exit\texpected_test_failed\tverdict\n' >"$summary"
for m in "${mutants[@]}"; do
  id="${m%%@@*}"; rest="${m#*@@}"
  if [ -n "${MUTANTS_ONLY:-}" ] && ! printf ',%s,' "$MUTANTS_ONLY" | grep -q ",$id,"; then continue; fi
  which="${rest%%@@*}"; rest="${rest#*@@}"
  test="${rest%%@@*}"; rest="${rest#*@@}"
  from="${rest%%@@*}"; to="${rest#*@@}"
  restore
  file="$MAIN"; [ "$which" = cli ] && file="$CLI"; [ "$which" = defaults ] && file="$DEFAULTS"; [ "$which" = perm ] && file="$PERM"; [ "$which" = payload ] && file="$PAYLOAD"; [ "$which" = status ] && file="$STATUSF"
  FROM="$from" TO="$to" python3 -c '
import os, sys
p = sys.argv[1]; src = open(p).read()
f, t = os.environ["FROM"], os.environ["TO"]
if f in src:
    open(p, "w").write(src.replace(f, t, 1))
' "$file"
  if cmp -s "$file" "$out/$(basename "$file").orig"; then
    printf '%s\t%s\t%s\tno\t-\t-\tNOT_APPLIED\n' "$id" "$which" "$test" >>"$summary"
    echo "NOT_APPLIED: $id matched no source in $file; the anchor drifted from production" >&2
    status=1; continue
  fi
  log="$out/$id.log"
  { echo "== $id (expected to fail: $test)"; diff -u "$out/$(basename "$file").orig" "$file"; echo; } >"$log"
  go test ${MUTANT_PKGS:-./...} ${MUTANT_RUN:+-run "$MUTANT_RUN"} -count=1 -v >>"$log" 2>&1; code=$?
  failed=no
  grep -Eq "^[[:space:]]*--- FAIL: ${test}( |$)" "$log" && failed=yes
  verdict=KILLED
  if [ "$code" -eq 0 ] || [ "$failed" = no ]; then verdict=SURVIVED; status=1; fi
  printf '%s\t%s\t%s\tyes\t%s\t%s\t%s\n' "$id" "$which" "$test" "$code" "$failed" "$verdict" >>"$summary"
done
restore
column -t -s $'\t' "$summary" 2>/dev/null || cat "$summary"
exit "$status"
