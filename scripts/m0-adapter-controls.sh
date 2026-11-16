#!/usr/bin/env bash
# Plant controls for internal/m0adapter (DRAFT M0 executor adapter).
#
# Each mutant weakens exactly one adapter gate in a scratch copy of the
# package (its own throwaway module, standard library only) and requires the
# named behavioral test to FAIL with a "--- FAIL:" line for that test. First
# the unmutated copy must pass every named test (positive control). A mutant
# whose anchor is not found exactly once, that leaves the file unchanged, or
# whose test passes or fails only to build, is a control failure. The real
# tree is never modified. Linux only: the descriptor mutant needs the Linux
# process tests.
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
mkdir -p "$root/.temp"
scratch=$(mktemp -d "$root/.temp/m0-adapter-controls.XXXXXX")
scratch=$(cd "$scratch" && pwd -P)
trap 'rm -rf "$scratch"' EXIT
export GOWORK=off GOFLAGS=-mod=mod

# Fields separated by "|": id, file, test, anchor, replacement, description.
# Anchors are exact literal text; the replacement keeps the code compiling.
mutants=(
	'M1|parse.go|TestGrammarNegatives|return jvalue{}, refuse(ReasonEntryDuplicate)|break|drop the duplicate-name refusal'
	'M2|fd_linux.go|TestProcessPlanDuplicate|return id == plan|return false|drop the plan-object duplicate sweep'
	'M3|env.go|TestEnvironment|return refuse(ReasonEnvUnexpected)|continue|skip extra environment variables instead of refusing'
	'M4|parse.go|TestTokenBound|maxTokens       = 512|maxTokens       = 513|widen the token bound to 513'
	'M5|mapping.go|TestTimeoutVectors|t := rem / 1000|t := (rem + 999) / 1000|round T up instead of down'
	'M6|fd_linux.go|TestPromptPipeCapacity|if size < len(prompt) {|if false && size < len(prompt) {|skip the up-front pipe capacity refusal'
)

copy_pkg() {
	local dest=$1
	mkdir -p "$dest"
	printf 'module m0adaptercontrols\n\ngo %s\n' "$go_version" >"$dest/go.mod"
	cp -R "$pkg" "$dest/m0adapter"
}

run_tests() {
	local dir=$1 pattern=$2 log=$3
	(cd "$dir" && go test ./m0adapter/ -count=1 -v -run "$pattern") >"$log" 2>&1
}

status=0
names=()
for m in "${mutants[@]}"; do
	IFS='|' read -r _ _ test _ _ _ <<<"$m"
	names+=("$test")
done
pattern="^($(IFS='|'; echo "${names[*]}"))\$"

copy_pkg "$scratch/baseline"
if run_tests "$scratch/baseline" "$pattern" "$scratch/baseline.log"; then
	echo "baseline: PASS ($pattern)"
else
	echo "baseline: FAIL; the unmutated copy must pass every named test" >&2
	cat "$scratch/baseline.log" >&2
	exit 1
fi

for m in "${mutants[@]}"; do
	IFS='|' read -r id file test anchor replacement desc <<<"$m"
	dir="$scratch/$id"
	copy_pkg "$dir"
	target="$dir/m0adapter/$file"
	cp "$target" "$dir/original.go"
	if ! ANCHOR="$anchor" REPLACEMENT="$replacement" perl -0777 -i -pe \
		'BEGIN { $n = 0 } $n += s/\Q$ENV{ANCHOR}\E/$ENV{REPLACEMENT}/g; END { exit($n == 1 ? 0 : 3) }' "$target"; then
		echo "$id: NOT_APPLIED (anchor not found exactly once in $file) - $desc" >&2
		status=1
		continue
	fi
	if cmp -s "$target" "$dir/original.go"; then
		echo "$id: NOT_APPLIED (file unchanged) - $desc" >&2
		status=1
		continue
	fi
	log="$scratch/$id.log"
	if run_tests "$dir" "^${test}\$" "$log"; then
		echo "$id: SURVIVED ($test passed) - $desc" >&2
		cat "$log" >&2
		status=1
	elif grep -Eq "^--- FAIL: ${test}( |\$)" "$log"; then
		echo "$id: KILLED by $test - $desc"
	else
		echo "$id: INVALID ($test did not report its own failure; build or setup error) - $desc" >&2
		cat "$log" >&2
		status=1
	fi
done
exit "$status"
