#!/usr/bin/env bash
# Print which CI areas a change reaches, as area=true|false lines for $GITHUB_OUTPUT.
# With no base commit (main, dev, schedules, manual runs) every area runs.
#
# Usage: scripts/ci-changes.sh [base commit]
set -euo pipefail

cd "$(dirname "$0")/.." || exit 1

base=${1:-}
common='^(mise\.toml|scripts/ci\.sh|scripts/ci-changes\.sh|scripts/install-mise\.sh|\.github/workflows/ci\.yml)$'
declare -A paths=(
  [go]='^backend/'
  # The engine's tests apply the migrations and share cases with the Go port.
  [engine]='^(engine/|backend/migrations/|backend/internal/ratingdelta/testdata/)'
  # The TS types are generated from the spec; the infra workers' tests run here too.
  [web]='^(web/|backend/api/openapi\.yaml$|checks/csp-inline\.mjs$|infra/)'
)

if [ -z "$base" ]; then
  changed=""
else
  git cat-file -e "$base^{commit}" 2>/dev/null || git fetch --quiet --depth=1 origin "$base"
  changed=$(git diff --name-only "$base" HEAD)
fi

for area in go engine web; do
  if [ -z "$base" ] || grep -qE -e "${paths[$area]}" -e "$common" <<<"$changed"; then
    echo "$area=true"
  else
    echo "$area=false"
  fi
done
