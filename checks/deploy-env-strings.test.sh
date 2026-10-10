#!/usr/bin/env bash
# Tests checks/deploy-env-strings.sh on the invented kustomizations in checks/testdata/deploy-env-strings:
# good/ must pass, and every other case must fail naming its env var BAD.
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

failed=0
for dir in checks/testdata/deploy-env-strings/*/; do
  name=$(basename "$dir")
  out=$(bash checks/deploy-env-strings.sh "$dir" 2>&1)
  got=$?
  if [ "$name" = good ]; then
    if [ "$got" -ne 0 ]; then
      echo "  $name: exit = $got, want 0; output: $out" >&2
      failed=1
    fi
  elif [ "$got" -ne 1 ] || ! grep -q ': BAD is ' <<<"$out"; then
    echo "  $name: exit = $got, want 1 naming BAD; output: $out" >&2
    failed=1
  fi
done

if [ "$failed" -ne 0 ]; then
  echo "deploy-env-strings.test: failed" >&2
  exit 1
fi
echo "deploy-env-strings.test: every case as expected"
