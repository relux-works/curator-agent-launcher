#!/bin/sh
# Admission only: never build, sign binaries, push a tag or publish a release.
set -eu
refuse() {
	echo "release-gate: $1" >&2
	exit 1
}

tag=${1:-}
# Stable vX.Y.Z only; in particular, never interpret a ref as a Git option.
printf '%s\n' "$tag" | grep -Eqx 'v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)' &&
	[ "$(printf '%s' "$tag" | wc -l | tr -d ' ')" = 0 ] ||
	refuse 'expected a stable vX.Y.Z tag'

root=$(git rev-parse --show-toplevel)
cd "$root"
ref=refs/tags/$tag
[ "$(git cat-file -t "$ref" 2>/dev/null)" = tag ] ||
	refuse 'release tag must be annotated and signed'
[ "$(git cat-file tag "$ref" | sed -n '/^$/q; s/^tag //p')" = "$tag" ] ||
	refuse 'signed tag name does not match release ref'
git cat-file tag "$ref" | grep -q '^-----BEGIN SSH SIGNATURE-----$' ||
	refuse 'release tag must carry an SSH signature'
git -c gpg.format=ssh -c gpg.ssh.program=ssh-keygen \
	-c gpg.openpgp.program=false -c gpg.x509.program=false \
	-c gpg.ssh.allowedSignersFile="$root/.github/release-signers" \
	verify-tag "$ref" || refuse 'release tag signature is not trusted'
[ "$(git rev-parse "$ref^{commit}")" = "$(git rev-parse HEAD)" ] ||
	refuse 'release tag does not name the checked-out commit'

version=$(sed -n 's/^var buildVersion = "\([^"]*\)"$/\1/p' cmd/curator-run/main.go)
[ "$tag" = "v$version" ] || refuse 'release tag does not match buildVersion'
printf 'release-gate: accepted %s\n' "$tag"
