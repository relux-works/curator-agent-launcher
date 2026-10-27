#!/bin/sh
# Real Git/SSH controls in a disposable repo; no network or real release tags.
# Each rejection must name the intended admission failure and leave refs and
# working files unchanged. The trusted signed control prevents reject-all.
set -eu
root=$(git rev-parse --show-toplevel)
work=$(mktemp -d "$root/.temp/release-gate-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
ssh-keygen -q -t ed25519 -N '' -f "$work/trusted"
ssh-keygen -q -t ed25519 -N '' -f "$work/foreign"
mkdir "$work/repo"
cd "$work/repo"
git init -q
# CI runners have no author configuration. This synthetic fixture is local
# only and uses no operator identity, credentials or release signing key.
git config user.name 'Release gate fixture'
git config user.email 'release-gate-fixture@example.invalid'
git config gpg.format ssh
git config gpg.ssh.program ssh-keygen
git config user.signingkey "$work/trusted"
git config commit.gpgsign true
mkdir -p .github cmd/curator-run
printf 'release-test namespaces="git" %s\n' "$(cat "$work/trusted.pub")" > .github/release-signers
printf 'var buildVersion = "1.2.3"\n' > cmd/curator-run/main.go
git add .github cmd
# Command-line options override any inherited agent-git-env configuration.
fixture_git() {
	git -c gpg.format=ssh -c gpg.ssh.program=ssh-keygen \
		-c user.signingkey="$work/trusted" "$@"
}
fixture_git commit -S -q -m 'Release gate fixture'
fixture_git tag -s v1.2.3 -m 'Trusted release'
before=$(git show-ref)
"$root/scripts/release-gate.sh" v1.2.3 > "$work/positive.log" 2>&1
grep -q '^release-gate: accepted v1.2.3$' "$work/positive.log"
[ "$before" = "$(git show-ref)" ]
[ -z "$(git status --porcelain)" ]
echo 'release-gate-test: trusted signed control accepted'

refused() {
	label=$1
	tag=$2
	reason=$3
	before_refs=$(git show-ref)
	before_diff=$(git diff --binary HEAD)
	set +e
	"$root/scripts/release-gate.sh" "$tag" > "$work/refusal.log" 2>&1
	result=$?
	set -e
	[ "$result" -eq 1 ] && grep -Fqx "release-gate: $reason" "$work/refusal.log" &&
		[ "$before_refs" = "$(git show-ref)" ] &&
		[ "$before_diff" = "$(git diff --binary HEAD)" ] &&
		[ -z "$(git ls-files --others --exclude-standard)" ] || {
		cat "$work/refusal.log" >&2
		echo "release-gate-test: FAIL $label (exit $result or state changed)" >&2
		exit 1
	}
	echo "release-gate-test: $label refused without effects"
}

original_tag=$(git rev-parse refs/tags/v1.2.3)
tampered_tag=$(git cat-file tag "$original_tag" | sed 's/Trusted release/Tampered release/' | git hash-object -t tag -w --stdin)
git update-ref refs/tags/v1.2.3 "$tampered_tag"
refused tampered-signature v1.2.3 'release tag signature is not trusted'
git update-ref refs/tags/v1.2.3 "$original_tag"

fixture_git tag -a v1.2.4 -m unsigned
refused unsigned v1.2.4 'release tag must carry an SSH signature'
fixture_git -c user.signingkey="$work/foreign" tag -s v1.2.5 -m foreign
refused foreign-signer v1.2.5 'release tag signature is not trusted'
fixture_git tag -s v1.2.6 -m wrong-version
refused version-mismatch v1.2.6 'release tag does not match buildVersion'
fixture_git tag v1.2.7
refused lightweight v1.2.7 'release tag must be annotated and signed'
refused missing v1.2.8 'release tag must be annotated and signed'
refused prerelease v1.2.3-rc.1 'expected a stable vX.Y.Z tag'
refused option --help 'expected a stable vX.Y.Z tag'
refused leading-zero v01.2.3 'expected a stable vX.Y.Z tag'
refused multiline "$(printf 'v1.2.3\nv1.2.4')" 'expected a stable vX.Y.Z tag'
git update-ref refs/tags/v1.2.9 refs/tags/v1.2.3
refused aliased-signed-tag v1.2.9 'signed tag name does not match release ref'

# A valid signature from the fixture key must fail against the real allowlist.
# This checks production trust independently of the synthetic positive control.
cp "$root/.github/release-signers" .github/release-signers
refused production-allowlist v1.2.3 'release tag signature is not trusted'
git show HEAD:.github/release-signers > .github/release-signers

fixture_git commit -S -q --allow-empty -m 'Different checkout'
refused checkout-mismatch v1.2.3 'release tag does not name the checked-out commit'

# Presence controls use only synthetic values and never invoke a signer.
# Exhaust all 32 subsets: none/all are accepted; the other 30, including each
# single missing field, must fail for partial configuration without effects.
signing_case() {
	label=$1
	expected_status=$2
	expected_message=$3
	shift 3
	before_refs=$(git show-ref)
	before_diff=$(git diff --binary HEAD)
	set +e
	env -u MACOS_SIGN_P12 -u MACOS_SIGN_P12_PASSWORD \
		-u ASC_ISSUER_ID -u ASC_KEY_ID -u ASC_KEY_P8 \
		"$@" "$root/scripts/release-signing-gate.sh" > "$work/signing.log" 2>&1
	result=$?
	set -e
	[ "$result" -eq "$expected_status" ] &&
		grep -Fqx "$expected_message" "$work/signing.log" &&
		! grep -q 'fixture-secret-not-for-logging' "$work/signing.log" &&
		[ "$(wc -l < "$work/signing.log" | tr -d ' ')" = 1 ] &&
		[ "$before_refs" = "$(git show-ref)" ] &&
		[ "$before_diff" = "$(git diff --binary HEAD)" ] &&
		[ -z "$(git ls-files --others --exclude-standard)" ] || {
		cat "$work/signing.log" >&2
		echo "release-gate-test: FAIL signing-$label (exit $result or state/log changed)" >&2
		exit 1
	}
	echo "release-gate-test: signing-$label exit=$result without effects or values"
}

mask=0
while [ "$mask" -lt 32 ]; do
	set --
	[ $((mask & 1)) -eq 0 ] || set -- "$@" 'MACOS_SIGN_P12=fixture-secret-not-for-logging'
	[ $((mask & 2)) -eq 0 ] || set -- "$@" 'MACOS_SIGN_P12_PASSWORD=fixture-secret-not-for-logging'
	[ $((mask & 4)) -eq 0 ] || set -- "$@" 'ASC_ISSUER_ID=fixture-secret-not-for-logging'
	[ $((mask & 8)) -eq 0 ] || set -- "$@" 'ASC_KEY_ID=fixture-secret-not-for-logging'
	# A multiline PEM-like value still counts as one present field.
	[ $((mask & 16)) -eq 0 ] || set -- "$@" "ASC_KEY_P8=$(printf 'fixture-secret-not-for-logging\nsecond-line')"
	case $mask in
	0) signing_case none 0 'release-signing-gate: disabled (no signing fields)' "$@" ;;
	31) signing_case all 0 'release-signing-gate: enabled (all five fields present)' "$@" ;;
	*) signing_case "partial-mask-$mask" 1 'release-signing-gate: partial signing configuration; set all five signing fields or none' "$@" ;;
	esac
	mask=$((mask + 1))
done
# GitHub resolves unavailable secrets to explicit empty environment values.
signing_case empty 0 'release-signing-gate: disabled (no signing fields)' \
	MACOS_SIGN_P12= MACOS_SIGN_P12_PASSWORD= ASC_ISSUER_ID= ASC_KEY_ID= ASC_KEY_P8=
echo 'release-gate-test: OK'
