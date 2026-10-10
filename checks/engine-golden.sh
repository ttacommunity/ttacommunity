#!/usr/bin/env bash
# Fail when engine/tests/golden.json changes or drops the hash of a version already on main: a change
# that moves ratings is a new engine version, never a new hash for an old one.
# Usage: checks/engine-golden.sh [base]   (default: origin/main)
set -uo pipefail

cd "$(dirname "$0")/.." || exit 2
base=${1:-origin/main}
file=engine/tests/golden.json

if ! git rev-parse --verify --quiet "$base^{commit}" >/dev/null; then
  echo "engine-golden: no $base to compare with" >&2
  exit 2
fi
if [ -z "$(git ls-tree --name-only "$base" -- "$file")" ]; then
  echo "engine-golden: $base has no $file yet"
  exit 0
fi
if ! changed=$(jq -rn --argjson old "$(git show "$base:$file")" --slurpfile new "$file" \
  '$old | to_entries[] | select($new[0][.key] != .value) | .key'); then
  echo "engine-golden: could not compare $file with $base" >&2
  exit 2
fi
if [ -n "$changed" ]; then
  for version in $changed; do
    echo "  $file: engine $version's hash changed or went since $base. Bump the version instead" >&2
  done
  exit 1
fi
echo "engine-golden: every version on $base keeps its hash"
