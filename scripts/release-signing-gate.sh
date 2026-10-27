#!/bin/sh
# Presence preflight only: do not print values, build, sign or contact Apple.
# Empty values count as absent, matching GoReleaser's isEnvSet predicate.
set -eu

present=0
for value in "${MACOS_SIGN_P12:-}" "${MACOS_SIGN_P12_PASSWORD:-}" \
	"${ASC_ISSUER_ID:-}" "${ASC_KEY_ID:-}" "${ASC_KEY_P8:-}"; do
	if [ -n "$value" ]; then
		present=$((present + 1))
	fi
done

case $present in
0) printf 'release-signing-gate: disabled (no signing fields)\n' ;;
5) printf 'release-signing-gate: enabled (all five fields present)\n' ;;
*)
	printf 'release-signing-gate: partial signing configuration; set all five signing fields or none\n' >&2
	exit 1
	;;
esac
