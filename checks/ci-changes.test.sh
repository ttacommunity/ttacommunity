#!/usr/bin/env bash
# Tests scripts/ci-changes.sh in throwaway git repos: a base commit, then one changed file per case, which must
# turn on exactly the CI areas named.
set -uo pipefail

# shellcheck source=checks/testlib.sh
. "$(dirname "$0")/testlib.sh"
testlib_start ci-changes.test

# case: name, the file changed after the base (- for no base commit given), the areas that must run.
cases=(
  "no-base-runs-everything|-|go engine web"
  "go-code|backend/internal/store/x.go|go"
  "a-migration|backend/migrations/ratings/00099_x.sql|go engine"
  "ratingdelta-cases|backend/internal/ratingdelta/testdata/x.json|go engine"
  "the-spec|backend/api/openapi.yaml|go web"
  "engine-code|engine/src/x.py|engine"
  "web-code|web/site/src/x.ts|web"
  "the-csp-check|checks/csp-inline.mjs|web"
  "a-worker|infra/mail-forward/x.js|web"
  "tool-versions|mise.toml|go engine web"
  "ci-itself|scripts/ci.sh|go engine web"
  "the-path-filter|scripts/ci-changes.sh|go engine web"
  "installing-mise|scripts/install-mise.sh|go engine web"
  "the-workflow|.github/workflows/ci.yml|go engine web"
  "another-workflow|.github/workflows/images.yml|"
  "docs|docs/architecture.md|"
  "another-check|checks/dco.sh|"
  "a-name-that-only-starts-alike|backend-notes.md|"
)

for c in "${cases[@]}"; do
  IFS='|' read -r name file want <<<"$c"
  dir=$work/$name
  mkdir -p "$dir/scripts"
  cp scripts/ci-changes.sh "$dir/scripts/"
  git -C "$dir" init -q
  git -C "$dir" add -A
  commit_in "$dir" -m base
  base=$(git -C "$dir" rev-parse HEAD)
  if [ "$file" = - ]; then
    base=""
  else
    mkdir -p "$dir/$(dirname "$file")"
    echo "# change" >>"$dir/$file"
    git -C "$dir" add -A
    commit_in "$dir" -m change
  fi
  if ! out=$(bash "$dir/scripts/ci-changes.sh" ${base:+"$base"} 2>&1); then
    fail "$name: exit not 0; output: $out"
    continue
  fi
  got=$(sed -n 's/=true$//p' <<<"$out" | paste -sd' ' -)
  [ "$got" = "$want" ] || fail "$name: areas = '$got', want '$want'; output: $out"
done

testlib_finish
