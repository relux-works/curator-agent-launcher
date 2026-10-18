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
#   - negative: an invalid VERSION is refused by the guard and writes no binary.
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

# Negative: an invalid VERSION is refused by the version guard, not by a
# later build failure, and nothing is written. make reports any recipe failure
# as exit 2, so the guard's message is the evidence.
set +e
refusal=$(make -s -C "$B" release-binary VERSION='1.0 -X main.x=y' DIST=bad-dist 2>&1 >/dev/null)
status=$?
set -e
if [ "$status" -eq 0 ] || [ -e "$B/bad-dist" ] ||
	! printf '%s\n' "$refusal" | grep -q "VERSION must be a semantic version"; then
	echo "release-repro: invalid VERSION was not refused by the guard (exit $status): $refusal" >&2
	exit 1
fi

echo "release-repro: OK $(sum "$A/dist/$name" | cut -d' ' -f1) $name"
