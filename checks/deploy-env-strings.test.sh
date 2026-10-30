#!/usr/bin/env bash
# Tests checks/deploy-env-strings.sh on the invented kustomizations in checks/testdata/deploy-env-strings:
# good/ must pass, and every other case must fail naming its env var BAD.
set -uo pipefail

# shellcheck source=checks/testlib.sh
. "$(dirname "$0")/testlib.sh"
testlib_start deploy-env-strings.test

for dir in checks/testdata/deploy-env-strings/*/; do
  name=$(basename "$dir")
  out=$(bash checks/deploy-env-strings.sh "$dir" 2>&1)
  got=$?
  if [ "$name" = good ]; then
    [ "$got" -eq 0 ] || fail "$name: exit = $got, want 0; output: $out"
  elif [ "$got" -ne 1 ] || ! grep -q ': BAD is ' <<<"$out"; then
    fail "$name: exit = $got, want 1 naming BAD; output: $out"
  fi
done

testlib_finish
