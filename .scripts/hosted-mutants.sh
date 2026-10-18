#!/usr/bin/env bash
# Compatibility entry point; controls follow the active session-manager route.
# MUTANTS_ONLY selects comma-separated ids. Source is copied under the evidence
# directory and every plant requires a passing control plus its exact assertion.
set -eu
cd "$(dirname "$0")/.."
exec python3 .scripts/host-launch-mutants.py "${1:-.temp/hosted-mutants}"
