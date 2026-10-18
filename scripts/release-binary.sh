#!/bin/sh
# Builds the reproducible release binary for the signed Curator distribution
# (FRONT-DOOR §10). Run it through `make release-binary`, which passes the
# inputs as environment data:
#   RELEASE_VERSION  required, a semantic version; it goes into -X
#   RELEASE_GOOS     optional, default: go env GOOS
#   RELEASE_GOARCH   optional, default: go env GOARCH
#   RELEASE_DIST     optional output directory, default: dist
# Every input is validated before it reaches a command line, and an invalid
# input refuses with exit 2 before anything is written.
#
# The build settings are the ones ivmbp_curator_orch confirmed: CGO off,
# -trimpath, -buildvcs=false, -ldflags "-s -w -buildid= -X
# main.buildVersion=<version>", -mod=readonly, and the toolchain pinned
# from go.mod. The checksum file is in sha256sum format.
set -eu

refuse() {
	echo "release-binary: $1" >&2
	exit 2
}

# matches VALUE ERE: true when VALUE is exactly one line and matches ERE in
# full. A value with a newline never matches, even if one of its lines would.
matches() {
	case $1 in
	*"
"*) return 1 ;;
	esac
	printf '%s\n' "$1" | grep -Eqx -- "$2"
}

version=${RELEASE_VERSION:-}
matches "$version" '[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?' ||
	refuse "VERSION must be a one-line semantic version, got '$version'"

goos=${RELEASE_GOOS:-$(go env GOOS)}
goarch=${RELEASE_GOARCH:-$(go env GOARCH)}
for value in "$goos" "$goarch"; do
	matches "$value" '[a-z0-9]+' || refuse "GOOS and GOARCH must be one lowercase alphanumeric word, got '$value'"
done

dist=${RELEASE_DIST:-dist}
matches "$dist" '[A-Za-z0-9._/+][A-Za-z0-9._/+-]*' ||
	refuse "DIST must be a plain path ([A-Za-z0-9._/+-], not starting with '-'), got '$dist'"

toolchain=go$(sed -n 's/^go //p' go.mod)
matches "$toolchain" 'go[0-9]+\.[0-9]+(\.[0-9]+)?' || refuse "cannot read the go line from go.mod"

name="curator-run-$version-$goos-$goarch"
mkdir -p -- "$dist"
CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch GOFLAGS=-mod=readonly GOTOOLCHAIN=$toolchain \
	go build -trimpath -buildvcs=false \
	-ldflags "-s -w -buildid= -X main.buildVersion=$version" \
	-o "$dist/$name" ./cmd/curator-run

cd -- "$dist"
if command -v sha256sum >/dev/null 2>&1; then
	sha256sum -- "$name"
else
	shasum -a 256 -- "$name"
fi >"$name.sha256"
cat -- "$name.sha256"
