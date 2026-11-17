#!/usr/bin/env bash
# Each plant changes a scratch copy only: a named green test, one diff,
# that same test failing behaviorally, then a green test after restoration.
set -eu -o pipefail

root=$(cd "$(dirname "$0")/.." && pwd -P)
pkg="$root/internal/m0adapter"
if [ "$(uname -s)" != Linux ]; then
	echo "m0-adapter-controls: requires Linux" >&2
	exit 2
fi
go_version=$(awk '$1 == "go" { print $2; exit }' "$root/go.mod")
if [ -z "$go_version" ]; then
	echo "m0-adapter-controls: no go directive in go.mod" >&2
	exit 2
fi
scratch=$(mktemp -d "${TMPDIR:-/tmp}/m0-adapter-controls.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
export GOWORK=off GOFLAGS=-mod=readonly

# Fields: id|file|named test|exact anchor|replacement|claim.
# Multiple edits of one plant use ^^ between corresponding parts.
mutants=(
	'M1|parse.go|TestGrammarNegatives|return jvalue{}, refuse(ReasonEntryDuplicate)|break|drop the duplicate-name refusal'
	'M2|fd_linux.go|TestProcessPlanDuplicate|return id == plan|return false|drop the plan-object duplicate sweep'
	'M3|env.go|TestEnvironment|return refuse(ReasonEnvUnexpected)|continue|silently skip extra environment variables'
	'M4|parse.go|TestTokenBound|maxTokens       = 512|maxTokens       = 513|widen the token bound'
	'M5|mapping.go|TestTimeoutVectors|t := rem / 1000|t := (rem + 999) / 1000|round T upward'
	'M6|fd_linux.go|TestPromptPipeCapacity|if size < len(prompt) {|if false && size < len(prompt) {|skip the capacity refusal'
	'M7|fd_linux.go|TestProcessSpecimenExecsRunner|err = ops.close(w)|err = ops.dup(w, 2, 0)|leak the writer to fd 2'
	'M8|fd_linux.go|TestProcessSpecimenExecsRunner|err = ops.close(w)^^} else if id == *stdin {|err = ops.dup(w, 2, 0)^^} else if false && id == *stdin {|leak the writer to fd 2, caught by the standard-identity check'
	'eintr-bound|fd_linux.go|TestPromptPipeEINTRBound|n == 0 && attempt == 0|n == 0|retry a second EINTR'
	'environment-name|env.go|TestEnvironmentNameDenied|lit.deniedMarker(fixedEnvNames[i])^^for i, v := range values {|false^^for _, v := range values {|omit the environment name scan'
	'origin-required|literals.go|TestOriginSocketMarkerRequired|l.DeniedPathPrefix, l.OriginSocketMarker}|l.DeniedPathPrefix}|omit the required origin member'
	'capacity-equality|fd_linux.go|TestPromptPipeCapacity|if size < len(prompt) {|if size <= len(prompt) {|require capacity greater than prompt length'
	'unknown-runner|mapping.go|TestUnknownRunnerLiteralRefuses|switch lit.RunnerCommand {|switch codexRunnerCommand {|admit an unknown runner literal'
	'raw-environment|run_linux.go|TestProcessRawEnvironment|readEnv = readRawEnvironment|readEnv = func() ([]string, error) { return syscall.Environ(), nil }|normalize the kernel environment'
	'final-argv|mapping.go|TestProcessFinalArgvDenied|return checkArgv(x.Argv, x.lit)|return nil|omit the final argv scan'
	'coverage-name|controls_test.go|TestForbiddenControlsClassified|"TestPromptPipePartialWrite"|"TestPromptPipePartialWriteAbsent"|cite an absent top-level test'
	'missing-instance|entry_test.go|TestForbiddenControlsClassified|{"missing-timing-deadline", "missing-timing",|{"missing-timing-deadline", "",|omit a listed timing instance'
	'arguments-input|run.go|TestForbiddenControlsClassified|type Config struct {|type Config struct { Arguments []string;|add an adapter argument input'
	'standard-identity|fd_linux.go|TestProcessStandardDescriptors|id != plan.standard[fd]|false|omit standard object identity'
	'standard-flags|fd_linux.go|TestProcessStandardDescriptors|(fd <= 2) != inh|fd > 2 && inh|omit standard close-on-exec check'
	'native-nonblock|fd_linux.go|TestProcessNativePipeProperties|ops.setNonblock(w, true)|error(nil)|omit nonblocking setup on the real pipe'
	'direct-capacity|fd_linux.go|TestNoCapacitySyscall|const fGetPipeSz = 1032|const forbiddenCapacityCommand = 1031; const fGetPipeSz = 1032|introduce a direct capacity-raise command'
	'environ-before-c|run_linux.go|TestNothingRunsBeforeC|readEnv := cfg.readEnviron^^env, err := readEnv()^^if err := cfg.WaitForC(); err != nil {|readEnv = cfg.readEnviron^^env, err = env, nil^^readEnv := cfg.readEnviron; if readEnv == nil { readEnv = readRawEnvironment }; env, err := readEnv(); if err != nil { return reasonOf(err) }; if err := cfg.WaitForC(); err != nil {|move the environ read before C'
)
expected=23
if [ "${#mutants[@]}" -ne "$expected" ]; then
	echo "m0-adapter-controls: plant count mismatch expected=$expected actual=${#mutants[@]}" >&2
	exit 1
fi

copy_pkg() {
	local dest=$1
	mkdir -p "$dest/internal" "$dest/cmd"
	printf 'module github.com/relux-works/curator-agent-launcher\n\ngo %s\n' "$go_version" >"$dest/go.mod"
	cp -R "$pkg" "$dest/internal/m0adapter"
	cp -R "$root/cmd/curator-run-m0-adapter" "$dest/cmd/"
}

run_test() {
	local dir=$1 test=$2 log=$3 rc=0
	echo "command: go test ./internal/m0adapter/ -count=1 -v -run ^${test}\$"
	(cd "$dir" && go test ./internal/m0adapter/ -count=1 -v -run "^${test}\$") >"$log" 2>&1 || rc=$?
	if [ "$rc" -eq 0 ] && { ! grep -Eq "^=== RUN   ${test}\$" "$log" || ! grep -Eq "^--- PASS: ${test}( |\$)" "$log"; }; then
		echo "named test did not run and pass" >>"$log"
		rc=3
	fi
	cat "$log"
	echo "exit=$rc"
	return "$rc"
}

status=0
evaluated=0
baselines=0
kills=0
restores=0
for m in "${mutants[@]}"; do
	IFS='|' read -r id file test anchor replacement desc <<<"$m"
	dir="$scratch/$id"
	copy_pkg "$dir"
	target="$dir/internal/m0adapter/$file"
	cp "$target" "$dir/original.go"
	if ! run_test "$dir" "$test" "$dir/baseline.log"; then
		echo "$id: baseline FAIL - $desc" >&2
		status=1
		continue
	fi
	baselines=$((baselines + 1))
	echo "$id: baseline PASS ($test)"
	anchors=()
	replacements=()
	while [ -n "$anchor" ]; do
		anchors+=("${anchor%%^^*}")
		if [ "$anchor" = "${anchor#*^^}" ]; then anchor=''; else anchor=${anchor#*^^}; fi
	done
	while [ -n "$replacement" ]; do
		replacements+=("${replacement%%^^*}")
		if [ "$replacement" = "${replacement#*^^}" ]; then replacement=''; else replacement=${replacement#*^^}; fi
	done
	if [ "${#anchors[@]}" -ne "${#replacements[@]}" ]; then
		echo "$id: NOT_APPLIED (part counts differ)" >&2
		status=1
		continue
	fi
	applied=1
	for i in "${!anchors[@]}"; do
		if ! ANCHOR="${anchors[$i]}" REPLACEMENT="${replacements[$i]}" perl -0777 -i -pe \
			'BEGIN { $n = 0 } $n += s/\Q$ENV{ANCHOR}\E/$ENV{REPLACEMENT}/g; END { exit($n == 1 ? 0 : 3) }' "$target"; then
			applied=0
		fi
	done
	if [ "$applied" -ne 1 ] || cmp -s "$target" "$dir/original.go"; then
		echo "$id: NOT_APPLIED (anchor not unique or file unchanged)" >&2
		status=1
		continue
	fi
	diff_rc=0
	diff -u --label "a/$file" --label "b/$file" "$dir/original.go" "$target" || diff_rc=$?
	if [ "$diff_rc" -ne 1 ]; then
		echo "$id: INVALID diff exit=$diff_rc" >&2
		status=1
		continue
	fi
	evaluated=$((evaluated + 1))
	log="$dir/planted.log"
	# Meta-test plants must fail for their own reason, not any broad failure.
	expected_msg=''
	case "$id" in
	coverage-name) expected_msg='missing named test' ;;
	missing-instance) expected_msg='exercises 1 instances, fixture lists 2' ;;
	arguments-input) expected_msg='runner_argument_input' ;;
	esac
	if run_test "$dir" "$test" "$log"; then
		echo "$id: SURVIVED - $desc" >&2
		status=1
	elif grep -Eq "^--- FAIL: ${test}( |\$)" "$log" && { [ -z "$expected_msg" ] || grep -Fq "$expected_msg" "$log"; }; then
		kills=$((kills + 1))
		echo "$id: KILLED by $test - $desc"
	else
		echo "$id: INVALID (no named behavioral failure or expected message) - $desc" >&2
		status=1
	fi
	cp "$dir/original.go" "$target"
	if run_test "$dir" "$test" "$dir/restored.log"; then
		restores=$((restores + 1))
		echo "$id: restored PASS ($test)"
	else
		echo "$id: restored FAIL ($test)" >&2
		status=1
	fi
done
for count in "$evaluated" "$baselines" "$kills" "$restores"; do
	if [ "$count" -ne "$expected" ]; then
		echo "m0-adapter-controls: incomplete evidence expected=$expected actual=$count" >&2
		status=1
	fi
done
echo "m0-adapter-controls: expected=$expected applied=$evaluated baselines=$baselines named_kills=$kills restores=$restores exit=$status"
exit "$status"
