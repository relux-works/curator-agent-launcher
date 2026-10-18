#!/bin/sh
# Reproducibility check for `make release-binary` (FRONT-DOOR §10).
#
# It exports the committed HEAD into two checkout directories with different
# paths, builds the same VERSION in each, and requires byte-identical
# binaries, since that is what -trimpath and -buildid= must survive.
# Uncommitted changes are not part of the check.
#
# It also checks:
#   - each checksum file verifies against its binary (sha256sum -c);
#   - the binary reports VERSION in --version;
#   - control: a different VERSION gives different bytes, so the comparison
#     is not vacuous;
#   - negatives: crafted VERSION, GOOS, GOARCH and DIST values (spaces,
#     $(shell), backticks, $$(), ;, a leading -, a second line) are refused by the input guard,
#     run nothing and write nothing.
#
# Usage: scripts/release-repro.sh  (from the repository root; needs git, make, go)
set -eu

VERSION=0.0.0-repro
ROOT=$(git rev-parse --show-toplevel)
WORK=$(mktemp -d "${TMPDIR:-/tmp}/curator-run-repro.XXXXXX")
trap 'rm -rf "$WORK"' EXIT

sum() {
	if command -v sha256sum >/dev/null 2>&1; then sha256sum "$@"; else shasum -a 256 "$@"; fi
}

checkout() {
	mkdir -p "$1"
	git -C "$ROOT" archive HEAD | tar -x -C "$1"
}

A="$WORK/a"
B="$WORK/checkout-b/with/a/longer/path"
checkout "$A"
checkout "$B"

host_os=$(go env GOOS)
host_arch=$(go env GOARCH)
name="curator-run-$VERSION-$host_os-$host_arch"

make -s -C "$A" release-binary VERSION="$VERSION" >/dev/null
make -s -C "$B" release-binary VERSION="$VERSION" >/dev/null

for dir in "$A" "$B"; do
	(cd "$dir/dist" && sum -c "$name.sha256" >/dev/null) || { echo "release-repro: checksum file does not verify in $dir" >&2; exit 1; }
done

if ! cmp -s "$A/dist/$name" "$B/dist/$name"; then
	echo "release-repro: FAIL: the two checkouts produced different bytes" >&2
	sum "$A/dist/$name" "$B/dist/$name" >&2
	exit 1
fi

"$A/dist/$name" --version | grep -q "curator-run $VERSION " || { echo "release-repro: --version does not report $VERSION" >&2; exit 1; }

# Control: a different version must change the bytes.
make -s -C "$B" release-binary VERSION=0.0.1-repro >/dev/null
if cmp -s "$A/dist/$name" "$B/dist/curator-run-0.0.1-repro-$host_os-$host_arch"; then
	echo "release-repro: control FAIL: different VERSION produced identical bytes" >&2
	exit 1
fi

# Negatives: crafted inputs are data. Each one is refused by the input guard
# (exit 2 from the script, which make reports as a failure). None of them
# runs (no marker file appears), and nothing is written (no output
# directory). Every payload would create its marker if it were evaluated by
# make or by the shell.
MARK="$WORK/marks"
mkdir -p "$MARK"
refused() {
	label=$1
	shift
	set +e
	out=$(make -s -C "$B" release-binary "$@" 2>&1 >/dev/null)
	status=$?
	set -e
	if [ "$status" -eq 0 ] || [ -e "$B/bad-dist" ] || [ -n "$(ls -A "$MARK")" ] ||
		! printf '%s\n' "$out" | grep -q "^release-binary: "; then
		echo "release-repro: negative '$label' FAIL (exit $status, marks: $(ls -A "$MARK")): $out" >&2
		exit 1
	fi
}
refused version-space VERSION='1.0 -X main.x=y' DIST=bad-dist
refused version-make-shell VERSION="\$(shell touch $MARK/v1)" DIST=bad-dist
refused version-backtick VERSION="\`touch $MARK/v2\`" DIST=bad-dist
refused version-dollar-paren VERSION="\$\$(touch $MARK/v3)" DIST=bad-dist
refused version-semicolon VERSION="0.6.0;touch $MARK/v4" DIST=bad-dist
refused goos-make-shell VERSION=0.6.0 GOOS="\$(shell touch $MARK/g1)" DIST=bad-dist
refused goarch-backtick VERSION=0.6.0 GOARCH="\`touch $MARK/g2\`" DIST=bad-dist
refused dist-make-shell VERSION=0.6.0 DIST="\$(shell touch $MARK/d1)"
refused dist-option VERSION=0.6.0 DIST=-rf
# Multiline values whose first line is valid (S9R2-S1).
NL='
'
refused version-multiline VERSION="0.6.0${NL}0.6.1" DIST=bad-dist
refused goos-multiline VERSION=0.6.0 GOOS="linux${NL}darwin" DIST=bad-dist
refused goarch-multiline VERSION=0.6.0 GOARCH="amd64${NL}arm64" DIST=bad-dist
refused dist-multiline VERSION=0.6.0 DIST="bad-dist${NL}other"

echo "release-repro: OK $(sum "$A/dist/$name" | cut -d' ' -f1) $name"
