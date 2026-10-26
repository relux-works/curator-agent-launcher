#!/bin/sh
# GoReleaser's Go tool, using the same toolchain selection as release-binary.
set -eu
toolchain=go$(sed -n 's/^go //p' go.mod)
printf '%s\n' "$toolchain" | grep -Eqx 'go[0-9]+\.[0-9]+(\.[0-9]+)?' || {
	echo 'release-go: cannot read the go line from go.mod' >&2
	exit 2
}
export GOTOOLCHAIN=$toolchain
exec go "$@"
